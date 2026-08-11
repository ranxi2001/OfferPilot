package interview

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"offerpilot/backend/internal/executiontrace"
)

const maxKnowledgeEvidencePerQuestion = 5

type Service struct {
	agent     Agent
	planner   CoveragePlanner
	retriever KnowledgeRetriever
	store     Store
	clock     Clock
	ids       IDGenerator
}

func NewService(dependencies Dependencies) *Service {
	store := dependencies.Store
	if store == nil {
		store = NewMemoryStore()
	}
	clock := dependencies.Clock
	if clock == nil {
		clock = systemClock{}
	}
	ids := dependencies.IDs
	if ids == nil {
		ids = randomIDGenerator{}
	}
	planner := dependencies.Planner
	if planner == nil {
		planner, _ = dependencies.Agent.(CoveragePlanner)
	}
	return &Service{
		agent:     dependencies.Agent,
		planner:   planner,
		retriever: dependencies.Retriever,
		store:     store,
		clock:     clock,
		ids:       ids,
	}
}

func (s *Service) Start(ctx context.Context, request StartRequest) (StartResponse, error) {
	request.Config = withConfigDefaults(request.Config)
	validationSpan := executiontrace.Start(ctx, "request", "Validate interview start", "")
	if err := validateStart(request); err != nil {
		validationSpan.End(err, "")
		return StartResponse{}, err
	}
	validationSpan.End(nil, "")

	materialSpan := executiontrace.Start(ctx, "materials", "Prepare interview evidence", "")
	profile, sources := buildProfile(request.Materials, nil, request.Config.Focus)
	if len(profile.Coverage) == 0 && s.retriever != nil {
		seed := s.retrieveKnowledge(ctx, KnowledgeQuery{
			Model:  request.Model,
			Focus:  request.Config.Focus,
			JD:     materialText(request.Materials.JD),
			Resume: materialText(request.Materials.Resume),
		}, "Seed knowledge-only interview")
		profile, sources = buildProfile(request.Materials, seed, request.Config.Focus)
	}
	materialSpan.End(nil, fmt.Sprintf("coveragePoints=%d", len(profile.Coverage)))
	if len(profile.Coverage) == 0 {
		return StartResponse{}, validation("materials", "JD, resume, or retrieved knowledge must contain at least one usable anchor")
	}

	interviewID := s.ids.NewID("interview")
	session := InterviewSession{
		ID:              interviewID,
		ClientSessionID: request.ClientSessionID,
		Model:           request.Model,
		Config:          request.Config,
		State:           StateAwaitingAnswer,
		Profile:         profile,
		Sources:         sources,
		Answers:         make([]AnswerRecord, 0, request.Config.QuestionCount),
		CoverageCursor:  0,
		StartedAt:       s.clock.Now(),
		Version:         1,
	}
	initialPoint := s.bindKnowledgeContext(ctx, &session, profile.Coverage[0], Question{}, nil)
	decision := PolicyDecision{
		Action:          PolicyInitial,
		Reason:          "start with the highest-priority uncovered material anchor",
		Difficulty:      request.Config.Difficulty,
		CoveragePointID: initialPoint.ID,
		RootID:          s.ids.NewID("root"),
		FollowUpDepth:   0,
	}
	question, err := s.generateQuestion(ctx, session, initialPoint, decision, "")
	if err != nil {
		return StartResponse{}, err
	}
	session.CurrentQuestion = &question
	persistenceSpan := executiontrace.Start(ctx, "persistence", "Persist interview session", "")
	if err := s.store.Create(ctx, session); err != nil {
		persistenceSpan.End(err, "")
		if errors.Is(err, ErrStoreConflict) {
			return StartResponse{}, conflict("interview identifier already exists", err)
		}
		return StartResponse{}, &DomainError{Code: CodeInternal, Message: "could not persist interview", Cause: err}
	}
	persistenceSpan.End(nil, "")

	return StartResponse{
		InterviewID: interviewID,
		State:       session.State,
		Profile:     session.Profile,
		Question:    question,
		Progress:    progressFor(session),
	}, nil
}

func (s *Service) Answer(ctx context.Context, request AnswerRequest) (AnswerResponse, error) {
	validationSpan := executiontrace.Start(ctx, "request", "Validate interview answer", "")
	if err := validateAnswer(request); err != nil {
		validationSpan.End(err, "")
		return AnswerResponse{}, err
	}
	validationSpan.End(nil, "")
	session, err := s.load(ctx, request.InterviewID)
	if err != nil {
		return AnswerResponse{}, err
	}
	if session.State == StateCompleted {
		return AnswerResponse{}, conflict("the interview no longer accepts answers", nil)
	}
	if session.State != StateAwaitingAnswer || session.CurrentQuestion == nil {
		return AnswerResponse{}, &DomainError{Code: CodeInvalidState, Message: "interview is not awaiting an answer"}
	}
	if session.CurrentQuestion.ID != request.QuestionID {
		return AnswerResponse{}, conflict("questionId is stale or does not match the active question", nil)
	}

	expectedVersion := session.Version
	assessment, err := s.assessAnswer(ctx, session, *session.CurrentQuestion, request.Answer)
	if err != nil {
		return AnswerResponse{}, err
	}
	policySpan := executiontrace.Start(ctx, "policy", "Select adaptive interview action", "")
	decision := derivePolicy(session, assessment)
	policySpan.End(nil, "decision="+string(decision.Action))
	record := AnswerRecord{
		Question:   *session.CurrentQuestion,
		Answer:     request.Answer,
		Assessment: assessment,
		Decision:   decision,
		AnsweredAt: s.clock.Now(),
	}
	session.Answers = append(session.Answers, record)

	var nextQuestion *Question
	if decision.Action == PolicyComplete {
		session.State = StateCompleted
		session.CompletedAt = s.clock.Now()
		session.CurrentQuestion = nil
	} else {
		point := coveragePointByID(session.Profile, decision.CoveragePointID)
		if decision.Action == PolicyAdvance {
			selection, selectionErr := s.planNextCoverage(ctx, session, record)
			if selectionErr != nil {
				return AnswerResponse{}, selectionErr
			}
			selectedPoint, position, exists := findCoveragePoint(session.Profile, selection.CoveragePointID)
			if !exists {
				return AnswerResponse{}, unavailable("coverage planning returned an invalid target", fmt.Errorf("unknown coverage point %q", selection.CoveragePointID))
			}
			point = selectedPoint
			session.CoverageCursor = position
			decision.CoveragePointID = point.ID
			decision.RootID = s.ids.NewID("root")
			decision.Reason = strings.TrimSpace(selection.Reason)
			session.Answers[len(session.Answers)-1].Decision = decision
		}
		point = s.bindKnowledgeContext(ctx, &session, point, record.Question, assessment.Gaps)
		question, questionErr := s.generateQuestion(ctx, session, point, decision, request.QuestionID)
		if questionErr != nil {
			return AnswerResponse{}, questionErr
		}
		session.CurrentQuestion = &question
		nextQuestion = &question
	}
	session.Version++
	persistenceSpan := executiontrace.Start(ctx, "persistence", "Persist interview answer", "")
	if err := s.store.Save(ctx, session, expectedVersion); err != nil {
		persistenceSpan.End(err, "")
		if errors.Is(err, ErrStoreConflict) {
			return AnswerResponse{}, conflict("answer raced with another update; reload the active question", err)
		}
		if errors.Is(err, ErrStoreNotFound) {
			return AnswerResponse{}, &DomainError{Code: CodeNotFound, Message: "interview not found", Cause: err}
		}
		return AnswerResponse{}, &DomainError{Code: CodeInternal, Message: "could not persist answer", Cause: err}
	}
	persistenceSpan.End(nil, "")

	feedback := AnswerFeedback{
		Focus: coveragePointByID(session.Profile, record.Question.CoveragePointID).Area,
	}
	if session.Config.FeedbackMode == FeedbackDeferred {
		feedback.Deferred = true
	} else {
		feedback.Assessment = publicAssessment(session.Sources, assessment)
		feedback.Summary = assessmentSummary(feedback.Focus, assessment)
	}

	return AnswerResponse{
		InterviewID:  session.ID,
		State:        session.State,
		Feedback:     feedback,
		NextQuestion: nextQuestion,
		Progress:     progressFor(session),
		ReportReady:  session.State == StateCompleted,
	}, nil
}

func (s *Service) retrieveKnowledge(ctx context.Context, query KnowledgeQuery, operation string) []KnowledgeDocument {
	if s.retriever == nil {
		return nil
	}
	span := executiontrace.Start(ctx, "knowledge", operation, "knowledge_retriever")
	documents, err := s.retriever.Retrieve(ctx, query)
	if err != nil {
		span.End(err, "")
		return nil
	}
	span.End(nil, fmt.Sprintf("documents=%d", len(documents)))
	return documents
}

// bindKnowledgeContext refreshes only the selected coverage point. The
// resulting refs are the complete private evidence bundle authorized for the
// next Interviewer and Assessor calls; other session knowledge never enters
// either request.
func (s *Service) bindKnowledgeContext(ctx context.Context, session *InterviewSession, point CoveragePoint, previous Question, gaps []string) CoveragePoint {
	if session == nil || s.retriever == nil {
		return point
	}
	documents := s.retrieveKnowledge(ctx, KnowledgeQuery{
		Model:           session.Model,
		Focus:           point.Area,
		JD:              sourceContent(session.Sources, SourceJD),
		Resume:          sourceContent(session.Sources, SourceResume),
		CoveragePointID: point.ID,
		Objective:       point.Label,
		Question:        previous.Text,
		PreviousGaps:    append([]string(nil), gaps...),
	}, "Retrieve evidence for coverage point")
	if len(documents) == 0 {
		return point
	}
	knowledgeRefs := mergeKnowledgeDocuments(&session.Sources, documents, maxKnowledgeEvidencePerQuestion)
	if len(knowledgeRefs) == 0 {
		return point
	}
	bundle := make([]EvidenceRef, 0, len(point.EvidenceRefs)+len(knowledgeRefs))
	for _, ref := range point.EvidenceRefs {
		if ref.Kind != SourceKnowledge {
			bundle = append(bundle, ref)
		}
	}
	bundle = append(bundle, knowledgeRefs...)
	point.EvidenceRefs = canonicalEvidence(session.Sources, bundle)
	for index := range session.Profile.Coverage {
		if session.Profile.Coverage[index].ID == point.ID {
			session.Profile.Coverage[index] = point
			break
		}
	}
	return point
}

func sourceContent(index SourceIndex, kind SourceKind) string {
	parts := make([]string, 0, 1)
	for _, document := range index.Documents {
		if document.Kind == kind && strings.TrimSpace(document.Content) != "" {
			parts = append(parts, document.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func (s *Service) planNextCoverage(ctx context.Context, session InterviewSession, previous AnswerRecord) (result CoverageSelection, resultErr error) {
	span := executiontrace.Start(ctx, "planner", "Plan next coverage target", "coverage_planner")
	defer func() { span.End(resultErr, "") }()
	candidates := coverageCandidates(session)
	if len(candidates) == 0 {
		return CoverageSelection{}, unavailable("coverage planning has no candidate targets", errors.New("profile coverage is empty"))
	}
	if s.planner == nil {
		candidate := leastCoveredCandidate(candidates, session.CurrentQuestion.CoveragePointID)
		return CoverageSelection{
			CoveragePointID: candidate.CoveragePointID,
			Reason:          "select the highest-priority least-covered target available to this non-production planner fixture",
			Signals:         []string{"coverage count", "business priority"},
		}, nil
	}

	questionKindCounts := make(map[QuestionKind]int)
	for _, answer := range session.Answers {
		questionKindCounts[answer.Question.Kind]++
	}
	plannerHistory := publicPlannerHistory(session)
	publicPrevious := previous
	if len(plannerHistory) > 0 {
		publicPrevious = plannerHistory[len(plannerHistory)-1]
	}
	selection, err := s.planner.PlanCoverage(ctx, PlanCoverageRequest{
		Config:                 session.Config,
		CurrentCoveragePointID: session.CurrentQuestion.CoveragePointID,
		PreviousQuestion:       publicPrevious.Question,
		PreviousAssessment:     publicPrevious.Assessment,
		Candidates:             candidates,
		QuestionKindCounts:     questionKindCounts,
		History:                plannerHistory,
		RemainingQuestions:     max(0, session.Config.QuestionCount-len(session.Answers)),
	})
	if err != nil {
		return CoverageSelection{}, unavailable("coverage planning is temporarily unavailable", fmt.Errorf("plan next coverage: %w", err))
	}
	if strings.TrimSpace(selection.Reason) == "" || len(selection.Signals) == 0 {
		return CoverageSelection{}, unavailable("coverage planning returned an invalid proposal", errors.New("selection reason and signals are required"))
	}
	if _, _, exists := findCoveragePoint(session.Profile, selection.CoveragePointID); !exists {
		return CoverageSelection{}, unavailable("coverage planning returned an invalid target", fmt.Errorf("unknown coverage point %q", selection.CoveragePointID))
	}
	if len(candidates) > 1 && selection.CoveragePointID == session.CurrentQuestion.CoveragePointID {
		return CoverageSelection{}, unavailable("coverage planning returned an invalid target", errors.New("planner reselected the current coverage point despite alternatives"))
	}
	return selection, nil
}

func coverageCandidates(session InterviewSession) []CoverageCandidate {
	counts := make(map[string]int, len(session.Profile.Coverage))
	lastAsked := make(map[string]int, len(session.Profile.Coverage))
	for turn, answer := range session.Answers {
		counts[answer.Question.CoveragePointID]++
		lastAsked[answer.Question.CoveragePointID] = turn + 1
	}
	candidates := make([]CoverageCandidate, 0, len(session.Profile.Coverage))
	for _, point := range session.Profile.Coverage {
		label := point.Label
		evidence := publicQuestionEvidence(session.Sources, point.EvidenceRefs)
		if len(evidence) > 0 && evidence[0].Kind == SourceKnowledge {
			label = concise(knowledgeQuestionLabel(evidence[0].Quote), 120)
		} else if firstFoldedMarker(label, knowledgeReferenceMarkers) >= 0 {
			label = concise(knowledgeQuestionLabel(label), 120)
		}
		candidates = append(candidates, CoverageCandidate{
			CoveragePointID: point.ID,
			Area:            point.Area,
			Label:           label,
			Priority:        coveragePriority(session.Config.Focus, point),
			QuestionCount:   counts[point.ID],
			LastAskedTurn:   lastAsked[point.ID],
			EvidenceRefs:    evidence,
		})
	}
	return candidates
}

func coveragePriority(focus Focus, point CoveragePoint) int {
	priority := 50
	if focus == FocusMixed || focus == point.Area {
		priority += 20
	}
	for _, ref := range point.EvidenceRefs {
		switch ref.Kind {
		case SourceJD:
			priority += 20
		case SourceResume:
			priority += 15
		case SourceKnowledge:
			priority += 10
		}
	}
	return priority
}

func leastCoveredCandidate(candidates []CoverageCandidate, currentID string) CoverageCandidate {
	selected := candidates[0]
	foundAlternative := false
	for _, candidate := range candidates {
		if len(candidates) > 1 && candidate.CoveragePointID == currentID {
			continue
		}
		if !foundAlternative || candidate.QuestionCount < selected.QuestionCount ||
			candidate.QuestionCount == selected.QuestionCount && candidate.Priority > selected.Priority ||
			candidate.QuestionCount == selected.QuestionCount && candidate.Priority == selected.Priority && candidate.LastAskedTurn < selected.LastAskedTurn {
			selected = candidate
			foundAlternative = true
		}
	}
	return selected
}

func findCoveragePoint(profile Profile, id string) (CoveragePoint, int, bool) {
	for index, point := range profile.Coverage {
		if point.ID == id {
			return point, index, true
		}
	}
	return CoveragePoint{}, -1, false
}

func (s *Service) Report(ctx context.Context, request ReportRequest) (ReportResponse, error) {
	validationSpan := executiontrace.Start(ctx, "request", "Validate interview report", "")
	if request.Action != ActionReport {
		err := validation("action", "must be report")
		validationSpan.End(err, "")
		return ReportResponse{}, err
	}
	if strings.TrimSpace(request.InterviewID) == "" {
		err := validation("interviewId", "is required")
		validationSpan.End(err, "")
		return ReportResponse{}, err
	}
	validationSpan.End(nil, "")
	session, err := s.load(ctx, request.InterviewID)
	if err != nil {
		return ReportResponse{}, err
	}
	if session.State != StateCompleted {
		return ReportResponse{}, &DomainError{Code: CodeInvalidState, Message: "report is available only after the interview is completed"}
	}
	if session.Report != nil {
		return ReportResponse{InterviewID: session.ID, State: session.State, Report: *session.Report}, nil
	}

	expectedVersion := session.Version
	report, err := s.generateReport(ctx, session)
	if err != nil {
		return ReportResponse{}, err
	}
	session.Report = &report
	session.Version++
	persistenceSpan := executiontrace.Start(ctx, "persistence", "Persist interview report", "")
	if err := s.store.Save(ctx, session, expectedVersion); err != nil {
		persistenceSpan.End(err, "")
		if errors.Is(err, ErrStoreConflict) {
			latest, loadErr := s.store.Load(ctx, session.ID)
			if loadErr == nil && latest.Report != nil {
				return ReportResponse{InterviewID: latest.ID, State: latest.State, Report: *latest.Report}, nil
			}
			return ReportResponse{}, conflict("report raced with another update", err)
		}
		return ReportResponse{}, &DomainError{Code: CodeInternal, Message: "could not persist report", Cause: err}
	}
	persistenceSpan.End(nil, "")
	return ReportResponse{InterviewID: session.ID, State: session.State, Report: report}, nil
}

func (s *Service) generateQuestion(ctx context.Context, session InterviewSession, point CoveragePoint, decision PolicyDecision, basedOn string) (result Question, resultErr error) {
	span := executiontrace.Start(ctx, "interviewer", "Generate grounded interview question", "interviewer")
	defer func() { span.End(resultErr, "") }()
	if s.agent == nil {
		return Question{}, unavailable("interview question generation is temporarily unavailable", errors.New("interview agent is not configured"))
	}
	allowed := make(map[string]struct{}, len(point.EvidenceRefs))
	for _, ref := range point.EvidenceRefs {
		allowed[ref.AnchorID] = struct{}{}
	}
	profile, history := publicQuestionContext(session)
	anchors := publicQuestionAnchors(anchorsForEvidence(session.Sources, point.EvidenceRefs))
	publicDecision := decision
	publicDecision.Reason = publicDecisionReason(decision)
	request := GenerateQuestionRequest{
		Profile:  profile,
		Decision: publicDecision,
		Anchors:  anchors,
		History:  history,
	}
	draft, err := s.agent.GenerateQuestion(ctx, request)
	if err != nil {
		return Question{}, unavailable("interview question generation is temporarily unavailable", fmt.Errorf("generate question: %w", err))
	}
	if validationErr := validateQuestionDraft(session.Sources, draft, allowed, session.Answers); validationErr != nil {
		request.Repair = &RepairInstruction{
			Reason:          validationErr.Error(),
			AllowedEvidence: publicQuestionEvidence(session.Sources, point.EvidenceRefs),
		}
		draft, err = s.agent.GenerateQuestion(ctx, request)
		if err != nil {
			return Question{}, unavailable("interview question generation is temporarily unavailable", fmt.Errorf("repair question after %v: %w", validationErr, err))
		}
		if repairValidationErr := validateQuestionDraft(session.Sources, draft, allowed, session.Answers); repairValidationErr != nil {
			return Question{}, unavailable("interview question generation could not produce a grounded result", fmt.Errorf("question repair validation: %w", repairValidationErr))
		}
	}

	// Persist the entire per-question evidence bundle. The draft still has to
	// cite from the bundle, while assessment receives this exact same set.
	evidence := canonicalEvidence(session.Sources, point.EvidenceRefs)
	return Question{
		ID:              s.ids.NewID("question"),
		RootID:          decision.RootID,
		Text:            strings.TrimSpace(draft.Text),
		Kind:            questionKind(point, decision),
		Difficulty:      decision.Difficulty,
		CoveragePointID: point.ID,
		EvidenceRefs:    evidence,
		Adaptation: QuestionAdaptation{
			Trigger:           decision.Action,
			Reason:            publicDecisionReason(decision),
			BasedOnQuestionID: basedOn,
			FollowUpAxis:      decision.FollowUpAxis,
			Depth:             decision.FollowUpDepth,
		},
	}, nil
}

func publicQuestionContext(session InterviewSession) (Profile, []AnswerRecord) {
	snapshot := cloneSession(InterviewSession{Profile: session.Profile, Answers: session.Answers})
	publicProfileEvidence(session.Sources, &snapshot.Profile)
	for index := range snapshot.Answers {
		publicRecordEvidence(session.Sources, &snapshot.Answers[index])
	}
	return snapshot.Profile, snapshot.Answers
}

func publicPlannerHistory(session InterviewSession) []AnswerRecord {
	snapshot := cloneSession(InterviewSession{Answers: session.Answers})
	for index := range snapshot.Answers {
		publicRecordEvidence(session.Sources, &snapshot.Answers[index])
	}
	return snapshot.Answers
}

func publicRecordEvidence(index SourceIndex, record *AnswerRecord) {
	if record == nil {
		return
	}
	record.Question.EvidenceRefs = publicQuestionEvidence(index, record.Question.EvidenceRefs)
	record.Question.Adaptation.Reason = publicDecisionReason(PolicyDecision{Action: record.Question.Adaptation.Trigger})
	if questionLeaksKnowledgeReference(index, record.Question.Text, evidenceIDSet(record.Question.EvidenceRefs)) {
		record.Question.Text = publicQuestionSummary(record.Question.EvidenceRefs)
	}
	record.Assessment = publicAssessment(index, record.Assessment)
	record.Decision.Reason = publicDecisionReason(record.Decision)
}

func publicAssessment(index SourceIndex, assessment Assessment) Assessment {
	assessment.EvidenceRefs = publicQuestionEvidence(index, assessment.EvidenceRefs)
	assessment.FactualErrors = publicGeneratedStrings(index, assessment.FactualErrors)
	assessment.Strengths = publicGeneratedStrings(index, assessment.Strengths)
	assessment.Gaps = publicGeneratedStrings(index, assessment.Gaps)
	checks := make([]ClaimCheck, 0, len(assessment.ClaimChecks))
	for _, check := range assessment.ClaimChecks {
		if generatedTextLeaksKnowledgeReference(index, check.Claim) {
			continue
		}
		check.EvidenceRefs = publicQuestionEvidence(index, check.EvidenceRefs)
		checks = append(checks, check)
	}
	assessment.ClaimChecks = checks
	return assessment
}

func publicGeneratedStrings(index SourceIndex, values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || generatedTextLeaksKnowledgeReference(index, value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func generatedTextLeaksKnowledgeReference(index SourceIndex, value string) bool {
	return strings.TrimSpace(value) != "" && PublicGeneratedText(value, allEvidence(index)) == ""
}

func publicProfileEvidence(index SourceIndex, profile *Profile) {
	if profile == nil {
		return
	}
	for i := range profile.JD.Requirements {
		profile.JD.Requirements[i].EvidenceRefs = publicQuestionEvidence(index, profile.JD.Requirements[i].EvidenceRefs)
	}
	for i := range profile.JD.Responsibilities {
		profile.JD.Responsibilities[i].EvidenceRefs = publicQuestionEvidence(index, profile.JD.Responsibilities[i].EvidenceRefs)
	}
	for i := range profile.Resume.Projects {
		profile.Resume.Projects[i].EvidenceRefs = publicQuestionEvidence(index, profile.Resume.Projects[i].EvidenceRefs)
	}
	for i := range profile.Coverage {
		point := &profile.Coverage[i]
		point.EvidenceRefs = publicQuestionEvidence(index, point.EvidenceRefs)
		if len(point.EvidenceRefs) > 0 && point.EvidenceRefs[0].Kind == SourceKnowledge {
			point.Label = concise(knowledgeQuestionLabel(point.EvidenceRefs[0].Quote), 120)
		}
	}
}

func publicQuestionAnchors(anchors []SourceAnchor) []SourceAnchor {
	result := make([]SourceAnchor, 0, len(anchors))
	for _, anchor := range anchors {
		if anchor.Kind == SourceKnowledge {
			anchor.Text = publicKnowledgeQuestion(anchor.Text)
		}
		result = append(result, anchor)
	}
	return result
}

func publicQuestionEvidence(index SourceIndex, refs []EvidenceRef) []EvidenceRef {
	canonical := canonicalEvidence(index, refs)
	for i := range canonical {
		canonical[i].Quote = PublicEvidenceQuote(canonical[i])
	}
	return canonical
}

func evidenceForAnchors(anchors []SourceAnchor) []EvidenceRef {
	refs := make([]EvidenceRef, 0, len(anchors))
	for _, anchor := range anchors {
		refs = append(refs, evidenceFromAnchor(anchor))
	}
	return refs
}

func evidenceIDSet(refs []EvidenceRef) map[string]struct{} {
	result := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		result[ref.AnchorID] = struct{}{}
	}
	return result
}

func publicQuestionSummary(refs []EvidenceRef) string {
	for _, ref := range refs {
		if ref.Kind == SourceKnowledge {
			return PublicEvidenceQuote(ref)
		}
	}
	return "此前面试问题"
}

func publicDecisionReason(decision PolicyDecision) string {
	switch decision.Action {
	case PolicyInitial:
		return "按当前覆盖目标生成首题。"
	case PolicyPrerequisite:
		return "上一轮存在知识或事实风险，先核对前置理解。"
	case PolicyFollowUp:
		return "根据上一轮评估继续追问当前能力。"
	case PolicyAdvance:
		return "当前覆盖点已完成，切换到尚未覆盖的能力。"
	case PolicyComplete:
		return "达到本场问题上限，完成面试。"
	default:
		return "根据本轮评估调整后续问题。"
	}
}

func (s *Service) assessAnswer(ctx context.Context, session InterviewSession, question Question, answer AnswerPayload) (result Assessment, resultErr error) {
	span := executiontrace.Start(ctx, "assessor", "Assess candidate answer", "assessor")
	defer func() { span.End(resultErr, "") }()
	if s.agent == nil {
		return Assessment{}, unavailable("interview assessment is temporarily unavailable", errors.New("interview agent is not configured"))
	}
	questionAnchors := anchorsForEvidence(session.Sources, question.EvidenceRefs)
	allowed := make(map[string]struct{}, len(questionAnchors))
	for _, anchor := range questionAnchors {
		allowed[anchor.ID] = struct{}{}
	}
	request := AssessAnswerRequest{
		Question: question,
		Answer:   answer,
		Anchors:  questionAnchors,
		History:  session.Answers,
	}
	assessment, err := s.agent.AssessAnswer(ctx, request)
	if err != nil {
		return Assessment{}, unavailable("interview assessment is temporarily unavailable", fmt.Errorf("assess answer: %w", err))
	}
	if validationErr := validateAssessment(session.Sources, assessment, allowed); validationErr != nil {
		request.Repair = &RepairInstruction{
			Reason:          validationErr.Error(),
			AllowedEvidence: evidenceForAnchors(questionAnchors),
		}
		assessment, err = s.agent.AssessAnswer(ctx, request)
		if err != nil {
			return Assessment{}, unavailable("interview assessment is temporarily unavailable", fmt.Errorf("repair assessment after %v: %w", validationErr, err))
		}
		if repairValidationErr := validateAssessment(session.Sources, assessment, allowed); repairValidationErr != nil {
			return Assessment{}, unavailable("interview assessment could not produce a valid grounded result", fmt.Errorf("assessment repair validation: %w", repairValidationErr))
		}
	}
	assessment = normalizeAssessment(assessment)
	if len(assessment.EvidenceRefs) > 0 {
		assessment.EvidenceRefs = canonicalEvidence(session.Sources, assessment.EvidenceRefs)
	}
	for i := range assessment.ClaimChecks {
		assessment.ClaimChecks[i].EvidenceRefs = canonicalEvidence(session.Sources, assessment.ClaimChecks[i].EvidenceRefs)
	}
	return assessment, nil
}

func (s *Service) generateReport(ctx context.Context, session InterviewSession) (result Report, resultErr error) {
	span := executiontrace.Start(ctx, "reporter", "Generate grounded interview report", "reporter")
	defer func() { span.End(resultErr, "") }()
	if s.agent == nil {
		return Report{}, unavailable("interview report generation is temporarily unavailable", errors.New("interview agent is not configured"))
	}
	reporterSnapshot := cloneSession(InterviewSession{Profile: session.Profile, Answers: session.Answers})
	publicProfileEvidence(session.Sources, &reporterSnapshot.Profile)
	for index := range reporterSnapshot.Answers {
		publicRecordEvidence(session.Sources, &reporterSnapshot.Answers[index])
	}
	publicAnchors := publicQuestionAnchors(allAnchors(session.Sources))
	request := GenerateReportRequest{
		Profile: reporterSnapshot.Profile,
		Answers: reporterSnapshot.Answers,
		Anchors: publicAnchors,
	}
	draft, err := s.agent.GenerateReport(ctx, request)
	if err != nil {
		return Report{}, unavailable("interview report generation is temporarily unavailable", fmt.Errorf("generate report: %w", err))
	}
	if validationErr := validateReportDraft(session.Sources, draft); validationErr != nil {
		request.Repair = &RepairInstruction{
			Reason:          validationErr.Error(),
			AllowedEvidence: evidenceForAnchors(publicAnchors),
		}
		draft, err = s.agent.GenerateReport(ctx, request)
		if err != nil {
			return Report{}, unavailable("interview report generation is temporarily unavailable", fmt.Errorf("repair report after %v: %w", validationErr, err))
		}
		if repairValidationErr := validateReportDraft(session.Sources, draft); repairValidationErr != nil {
			return Report{}, unavailable("interview report generation could not produce a valid grounded result", fmt.Errorf("report repair validation: %w", repairValidationErr))
		}
	}

	auditTurns := make([]AuditTurn, 0, len(session.Answers))
	for _, record := range session.Answers {
		auditTurns = append(auditTurns, AuditTurn{
			QuestionID:      record.Question.ID,
			RootID:          record.Question.RootID,
			CoveragePointID: record.Question.CoveragePointID,
			EvidenceRefs:    record.Question.EvidenceRefs,
			InputMode:       record.Answer.InputMode,
			DurationMS:      record.Answer.DurationMS,
			AnsweredAt:      record.AnsweredAt,
		})
	}
	generatedAt := s.clock.Now()
	privateSnapshot := cloneSession(InterviewSession{Profile: session.Profile, Answers: session.Answers})
	return Report{
		OverallScore: scoreReport(session.Answers),
		Summary:      strings.TrimSpace(draft.Summary),
		Strengths:    nonNilStrings(draft.Strengths),
		Gaps:         nonNilStrings(draft.Gaps),
		EvidenceRefs: canonicalEvidence(session.Sources, draft.EvidenceRefs),
		Profile:      privateSnapshot.Profile,
		Turns:        privateSnapshot.Answers,
		Audit: ReportAudit{
			ClientSessionID: session.ClientSessionID,
			StartedAt:       session.StartedAt,
			CompletedAt:     session.CompletedAt,
			GeneratedAt:     generatedAt,
			Turns:           auditTurns,
		},
	}, nil
}

func scoreReport(records []AnswerRecord) int {
	if len(records) == 0 {
		return 0
	}
	total := 0.0
	for _, record := range records {
		assessment := record.Assessment
		if usesKnowledgeRubric(record.Question) {
			score := assessment.Correctness + assessment.Depth + assessment.Specificity + assessment.Tradeoffs
			total += float64(score) / 20 * 100
			continue
		}
		score := assessment.Correctness + assessment.Depth + assessment.Specificity + assessment.Ownership + assessment.Metrics + assessment.Tradeoffs
		total += float64(score) / 30 * 100
	}
	return int(math.Round(total / float64(len(records))))
}

func usesKnowledgeRubric(question Question) bool {
	switch question.Kind {
	case QuestionKnowledge:
		return true
	case QuestionProject, QuestionBehavioral:
		return false
	case QuestionFollowUp, QuestionPrerequisite:
		return !hasResumeEvidence(question)
	default:
		return false
	}
}

func hasResumeEvidence(question Question) bool {
	for _, ref := range question.EvidenceRefs {
		if ref.Kind == SourceResume {
			return true
		}
	}
	return false
}

func validateQuestionDraft(index SourceIndex, draft QuestionDraft, allowed map[string]struct{}, history []AnswerRecord) error {
	if strings.TrimSpace(draft.Text) == "" {
		return errors.New("question text is empty")
	}
	if err := validateQuestionEvidenceRefs(index, draft.EvidenceRefs, allowed, true); err != nil {
		return err
	}
	if questionLeaksKnowledgeReference(index, draft.Text, allowed) {
		return errors.New("question text exposes private knowledge reference content")
	}
	if repeatsRecentQuestion(draft.Text, history) {
		return errors.New("question text repeats a recent interview question")
	}
	return nil
}

func validateQuestionEvidenceRefs(index SourceIndex, refs []EvidenceRef, allowed map[string]struct{}, require bool) error {
	if require && len(refs) == 0 {
		return errors.New("at least one evidence reference is required")
	}
	for _, ref := range refs {
		anchor, exists := index.Anchors[ref.AnchorID]
		if !exists {
			return fmt.Errorf("unknown anchor %q", ref.AnchorID)
		}
		if allowed != nil {
			if _, exists := allowed[ref.AnchorID]; !exists {
				return fmt.Errorf("anchor %q is outside the allowed evidence set", ref.AnchorID)
			}
		}
		if ref.SourceID != anchor.SourceID || ref.Kind != anchor.Kind || ref.Locator != anchor.Locator {
			return fmt.Errorf("metadata does not match anchor %q", ref.AnchorID)
		}
		expected := evidenceFromAnchor(anchor)
		expected.Quote = PublicEvidenceQuote(expected)
		if normalizeEvidenceText(ref.Quote) != normalizeEvidenceText(expected.Quote) {
			return fmt.Errorf("public quote does not match anchor %q", ref.AnchorID)
		}
	}
	return nil
}

func questionLeaksKnowledgeReference(index SourceIndex, question string, allowed map[string]struct{}) bool {
	if firstFoldedMarker(question, knowledgeReferenceMarkers) >= 0 {
		return true
	}
	normalizedQuestion := normalizeQuestionGuardText(question)
	if normalizedQuestion == "" {
		return false
	}
	for _, anchorID := range index.Order {
		if allowed != nil {
			if _, exists := allowed[anchorID]; !exists {
				continue
			}
		}
		anchor, exists := index.Anchors[anchorID]
		if !exists || anchor.Kind != SourceKnowledge {
			continue
		}
		reference := knowledgeReferenceText(anchor.Text)
		if reference == "" {
			continue
		}
		fragments := []string{reference}
		fragments = append(fragments, strings.FieldsFunc(reference, func(r rune) bool {
			return r == '\n' || unicode.IsPunct(r)
		})...)
		for _, fragment := range fragments {
			normalizedFragment := normalizeQuestionGuardText(fragment)
			if len([]rune(normalizedFragment)) >= 6 && strings.Contains(normalizedQuestion, normalizedFragment) {
				return true
			}
		}
	}
	return false
}

func repeatsRecentQuestion(question string, history []AnswerRecord) bool {
	normalized := normalizeQuestionGuardText(question)
	if normalized == "" {
		return false
	}
	start := max(0, len(history)-6)
	for _, record := range history[start:] {
		previous := normalizeQuestionGuardText(record.Question.Text)
		if previous == "" {
			continue
		}
		if normalized == previous {
			return true
		}
		shorter, longer := normalized, previous
		if len([]rune(shorter)) > len([]rune(longer)) {
			shorter, longer = longer, shorter
		}
		shortLength := len([]rune(shorter))
		longLength := len([]rune(longer))
		if shortLength >= 12 && float64(shortLength)/float64(longLength) >= 0.85 && strings.Contains(longer, shorter) {
			return true
		}
	}
	return false
}

func normalizeQuestionGuardText(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r):
			return unicode.ToLower(r)
		case unicode.IsNumber(r):
			return r
		default:
			return -1
		}
	}, value)
}

func validateReportDraft(index SourceIndex, draft ReportDraft) error {
	if strings.TrimSpace(draft.Summary) == "" {
		return errors.New("report summary is empty")
	}
	if generatedTextLeaksKnowledgeReference(index, draft.Summary) {
		return errors.New("report summary exposes private knowledge reference content")
	}
	for _, item := range append(append([]string{}, draft.Strengths...), draft.Gaps...) {
		if generatedTextLeaksKnowledgeReference(index, item) {
			return errors.New("report narrative exposes private knowledge reference content")
		}
	}
	return validateQuestionEvidenceRefs(index, draft.EvidenceRefs, nil, true)
}

func validateAssessment(index SourceIndex, assessment Assessment, allowed map[string]struct{}) error {
	scores := []struct {
		name  string
		value int
	}{
		{name: "correctness", value: assessment.Correctness},
		{name: "depth", value: assessment.Depth},
		{name: "specificity", value: assessment.Specificity},
		{name: "ownership", value: assessment.Ownership},
		{name: "metrics", value: assessment.Metrics},
		{name: "tradeoffs", value: assessment.Tradeoffs},
	}
	for _, score := range scores {
		if score.value < 1 || score.value > 5 {
			return fmt.Errorf("%s must be between 1 and 5", score.name)
		}
	}
	for _, item := range append(append(append([]string{}, assessment.FactualErrors...), assessment.Strengths...), assessment.Gaps...) {
		if generatedTextLeaksKnowledgeReference(index, item) {
			return errors.New("assessment narrative exposes private knowledge reference content")
		}
	}
	if err := validateEvidenceRefs(index, assessment.EvidenceRefs, allowed, true); err != nil {
		return err
	}
	for i, check := range assessment.ClaimChecks {
		if strings.TrimSpace(check.Claim) == "" {
			return fmt.Errorf("claimChecks[%d].claim is empty", i)
		}
		if generatedTextLeaksKnowledgeReference(index, check.Claim) {
			return fmt.Errorf("claimChecks[%d].claim exposes private knowledge reference content", i)
		}
		switch check.Verdict {
		case ClaimSupported, ClaimContradicted:
			if err := validateEvidenceRefs(index, check.EvidenceRefs, allowed, true); err != nil {
				return fmt.Errorf("claimChecks[%d]: %w", i, err)
			}
		case ClaimUnverified, ClaimNotInMaterial:
			if err := validateEvidenceRefs(index, check.EvidenceRefs, allowed, false); err != nil {
				return fmt.Errorf("claimChecks[%d]: %w", i, err)
			}
		default:
			return fmt.Errorf("claimChecks[%d].verdict %q is invalid", i, check.Verdict)
		}
	}
	return nil
}

func validateStart(request StartRequest) error {
	if request.Action != ActionStart {
		return validation("action", "must be start")
	}
	if strings.TrimSpace(request.ClientSessionID) == "" {
		return validation("clientSessionId", "is required")
	}
	if request.Config.Focus != FocusMixed && request.Config.Focus != FocusKnowledge && request.Config.Focus != FocusProjects {
		return validation("config.focus", "must be mixed, knowledge, or projects")
	}
	if request.Config.Difficulty != DifficultyEasy && request.Config.Difficulty != DifficultyMedium && request.Config.Difficulty != DifficultyHard {
		return validation("config.difficulty", "must be easy, medium, or hard")
	}
	if request.Config.QuestionCount < 1 || request.Config.QuestionCount > 20 {
		return validation("config.questionCount", "must be between 1 and 20")
	}
	if request.Config.FeedbackMode != FeedbackImmediate && request.Config.FeedbackMode != FeedbackDeferred {
		return validation("config.feedbackMode", "must be immediate or deferred")
	}
	return nil
}

func validateAnswer(request AnswerRequest) error {
	if request.Action != ActionAnswer {
		return validation("action", "must be answer")
	}
	if strings.TrimSpace(request.InterviewID) == "" {
		return validation("interviewId", "is required")
	}
	if strings.TrimSpace(request.QuestionID) == "" {
		return validation("questionId", "is required")
	}
	if strings.TrimSpace(request.Answer.Text) == "" {
		return validation("answer.text", "is required")
	}
	if request.Answer.InputMode != InputModeText && request.Answer.InputMode != InputModeVoice {
		return validation("answer.inputMode", "must be text or voice")
	}
	if request.Answer.DurationMS < 0 {
		return validation("answer.durationMs", "must not be negative")
	}
	return nil
}

func withConfigDefaults(config InterviewConfig) InterviewConfig {
	if config.Focus == "" {
		config.Focus = FocusMixed
	}
	if config.Difficulty == "" {
		config.Difficulty = DifficultyMedium
	}
	if config.QuestionCount == 0 {
		config.QuestionCount = 6
	}
	if strings.TrimSpace(config.Language) == "" {
		config.Language = "zh-CN"
	}
	if config.FeedbackMode == "" {
		config.FeedbackMode = FeedbackImmediate
	}
	return config
}

func coveragePointByID(profile Profile, id string) CoveragePoint {
	for _, point := range profile.Coverage {
		if point.ID == id {
			return point
		}
	}
	return profile.Coverage[0]
}

func progressFor(session InterviewSession) Progress {
	current := 0
	depth := 0
	if session.CurrentQuestion != nil {
		current = len(session.Answers) + 1
		depth = session.CurrentQuestion.Adaptation.Depth
	}
	return Progress{
		Answered:      len(session.Answers),
		Total:         session.Config.QuestionCount,
		Current:       current,
		FollowUpDepth: depth,
	}
}

func assessmentSummary(area Focus, assessment Assessment) string {
	if len(assessment.FactualErrors) > 0 || assessment.Correctness <= 1 {
		return "回答存在事实或前置理解问题，下一题先验证基础。"
	}
	if vagueAssessmentForArea(area, assessment) {
		if area == FocusKnowledge {
			return "回答仍缺少关键机制、适用边界或具体场景，下一题会沿知识缺口继续追问。"
		}
		return "回答仍偏空泛，下一题会追问个人职责、量化指标或方案取舍。"
	}
	if strongAssessmentForArea(area, assessment) {
		return "回答具体且有深度，下一题提高难度并轮换覆盖点。"
	}
	return "回答达到当前要求，下一题轮换到新的材料覆盖点。"
}

func (s *Service) load(ctx context.Context, id string) (result InterviewSession, resultErr error) {
	span := executiontrace.Start(ctx, "persistence", "Load interview session", "")
	defer func() { span.End(resultErr, "") }()
	session, err := s.store.Load(ctx, id)
	if errors.Is(err, ErrStoreNotFound) {
		return InterviewSession{}, &DomainError{Code: CodeNotFound, Message: "interview not found", Cause: err}
	}
	if err != nil {
		return InterviewSession{}, &DomainError{Code: CodeInternal, Message: "could not load interview", Cause: err}
	}
	return session, nil
}

func materialText(material *MaterialInput) string {
	if material == nil {
		return ""
	}
	return material.Text
}
