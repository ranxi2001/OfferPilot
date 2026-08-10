package interview

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"offerpilot/backend/internal/executiontrace"
)

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

	knowledge := make([]KnowledgeDocument, 0)
	if s.retriever != nil {
		retrievalSpan := executiontrace.Start(ctx, "knowledge", "Retrieve knowledge references", "")
		documents, err := s.retriever.Retrieve(ctx, KnowledgeQuery{
			Model:  request.Model,
			Focus:  request.Config.Focus,
			JD:     materialText(request.Materials.JD),
			Resume: materialText(request.Materials.Resume),
		})
		if err == nil {
			knowledge = documents
			retrievalSpan.End(nil, fmt.Sprintf("documents=%d", len(documents)))
		} else {
			retrievalSpan.End(err, "")
		}
	}
	materialSpan := executiontrace.Start(ctx, "materials", "Prepare interview evidence", "")
	profile, sources := buildProfile(request.Materials, knowledge, request.Config.Focus)
	materialSpan.End(nil, fmt.Sprintf("coveragePoints=%d", len(profile.Coverage)))
	if len(profile.Coverage) == 0 {
		return StartResponse{}, validation("materials", "JD, resume, or retrieved knowledge must contain at least one usable anchor")
	}

	interviewID := s.ids.NewID("interview")
	rootID := s.ids.NewID("root")
	initialPoint := profile.Coverage[0]
	decision := PolicyDecision{
		Action:          PolicyInitial,
		Reason:          "start with the highest-priority uncovered material anchor",
		Difficulty:      request.Config.Difficulty,
		CoveragePointID: initialPoint.ID,
		RootID:          rootID,
		FollowUpDepth:   0,
	}
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
		Profile:     profile,
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

	return AnswerResponse{
		InterviewID: session.ID,
		State:       session.State,
		Feedback: AnswerFeedback{
			Assessment: assessment,
			Summary:    assessmentSummary(coveragePointByID(session.Profile, record.Question.CoveragePointID).Area, assessment),
		},
		NextQuestion: nextQuestion,
		Progress:     progressFor(session),
		ReportReady:  session.State == StateCompleted,
	}, nil
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
	selection, err := s.planner.PlanCoverage(ctx, PlanCoverageRequest{
		Config:                 session.Config,
		CurrentCoveragePointID: session.CurrentQuestion.CoveragePointID,
		PreviousQuestion:       previous.Question,
		PreviousAssessment:     previous.Assessment,
		Candidates:             candidates,
		QuestionKindCounts:     questionKindCounts,
		History:                session.Answers,
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
		candidates = append(candidates, CoverageCandidate{
			CoveragePointID: point.ID,
			Area:            point.Area,
			Label:           point.Label,
			Priority:        coveragePriority(session.Config.Focus, point),
			QuestionCount:   counts[point.ID],
			LastAskedTurn:   lastAsked[point.ID],
			EvidenceRefs:    point.EvidenceRefs,
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
	anchors := anchorsForEvidence(session.Sources, point.EvidenceRefs)
	request := GenerateQuestionRequest{
		Profile:  session.Profile,
		Decision: decision,
		Anchors:  anchors,
		History:  session.Answers,
	}
	draft, err := s.agent.GenerateQuestion(ctx, request)
	if err != nil {
		return Question{}, unavailable("interview question generation is temporarily unavailable", fmt.Errorf("generate question: %w", err))
	}
	if validationErr := validateQuestionDraft(session.Sources, draft, allowed); validationErr != nil {
		request.Repair = &RepairInstruction{
			Reason:          validationErr.Error(),
			AllowedEvidence: canonicalEvidence(session.Sources, point.EvidenceRefs),
		}
		draft, err = s.agent.GenerateQuestion(ctx, request)
		if err != nil {
			return Question{}, unavailable("interview question generation is temporarily unavailable", fmt.Errorf("repair question after %v: %w", validationErr, err))
		}
		if repairValidationErr := validateQuestionDraft(session.Sources, draft, allowed); repairValidationErr != nil {
			return Question{}, unavailable("interview question generation could not produce a grounded result", fmt.Errorf("question repair validation: %w", repairValidationErr))
		}
	}

	evidence := canonicalEvidence(session.Sources, draft.EvidenceRefs)
	if len(point.EvidenceRefs) > 0 {
		primary := canonicalEvidence(session.Sources, point.EvidenceRefs[:1])
		if len(primary) > 0 {
			evidence = append(primary, evidence...)
			evidence = canonicalEvidence(session.Sources, evidence)
		}
	}
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
			Reason:            decision.Reason,
			BasedOnQuestionID: basedOn,
			FollowUpAxis:      decision.FollowUpAxis,
			Depth:             decision.FollowUpDepth,
		},
	}, nil
}

func (s *Service) assessAnswer(ctx context.Context, session InterviewSession, question Question, answer AnswerPayload) (result Assessment, resultErr error) {
	span := executiontrace.Start(ctx, "assessor", "Assess candidate answer", "assessor")
	defer func() { span.End(resultErr, "") }()
	if s.agent == nil {
		return Assessment{}, unavailable("interview assessment is temporarily unavailable", errors.New("interview agent is not configured"))
	}
	request := AssessAnswerRequest{
		Question: question,
		Answer:   answer,
		Anchors:  assessmentAnchors(session, question),
		History:  session.Answers,
	}
	assessment, err := s.agent.AssessAnswer(ctx, request)
	if err != nil {
		return Assessment{}, unavailable("interview assessment is temporarily unavailable", fmt.Errorf("assess answer: %w", err))
	}
	if validationErr := validateAssessment(session.Sources, assessment); validationErr != nil {
		request.Repair = &RepairInstruction{
			Reason:          validationErr.Error(),
			AllowedEvidence: allEvidence(session.Sources),
		}
		assessment, err = s.agent.AssessAnswer(ctx, request)
		if err != nil {
			return Assessment{}, unavailable("interview assessment is temporarily unavailable", fmt.Errorf("repair assessment after %v: %w", validationErr, err))
		}
		if repairValidationErr := validateAssessment(session.Sources, assessment); repairValidationErr != nil {
			return Assessment{}, unavailable("interview assessment could not produce a valid grounded result", fmt.Errorf("assessment repair validation: %w", repairValidationErr))
		}
	}
	assessment = normalizeAssessment(assessment)
	if len(assessment.EvidenceRefs) == 0 {
		assessment.EvidenceRefs = canonicalEvidence(session.Sources, question.EvidenceRefs)
	} else {
		assessment.EvidenceRefs = canonicalEvidence(session.Sources, assessment.EvidenceRefs)
	}
	for i := range assessment.ClaimChecks {
		assessment.ClaimChecks[i].EvidenceRefs = canonicalEvidence(session.Sources, assessment.ClaimChecks[i].EvidenceRefs)
	}
	return assessment, nil
}

func assessmentAnchors(session InterviewSession, question Question) []SourceAnchor {
	anchors := anchorsForEvidence(session.Sources, question.EvidenceRefs)
	point := coveragePointByID(session.Profile, question.CoveragePointID)
	if point.Area != FocusKnowledge {
		return anchors
	}

	seen := make(map[string]struct{}, len(anchors))
	knowledgeCount := 0
	for _, anchor := range anchors {
		seen[anchor.ID] = struct{}{}
		if anchor.Kind == SourceKnowledge {
			knowledgeCount++
		}
	}
	for _, anchorID := range session.Sources.Order {
		if knowledgeCount >= 4 {
			break
		}
		anchor, exists := session.Sources.Anchors[anchorID]
		if !exists || anchor.Kind != SourceKnowledge {
			continue
		}
		if _, exists := seen[anchor.ID]; exists {
			continue
		}
		seen[anchor.ID] = struct{}{}
		anchors = append(anchors, anchor)
		knowledgeCount++
	}
	return anchors
}

func (s *Service) generateReport(ctx context.Context, session InterviewSession) (result Report, resultErr error) {
	span := executiontrace.Start(ctx, "reporter", "Generate grounded interview report", "reporter")
	defer func() { span.End(resultErr, "") }()
	if s.agent == nil {
		return Report{}, unavailable("interview report generation is temporarily unavailable", errors.New("interview agent is not configured"))
	}
	request := GenerateReportRequest{
		Profile: session.Profile,
		Answers: session.Answers,
		Anchors: allAnchors(session.Sources),
	}
	draft, err := s.agent.GenerateReport(ctx, request)
	if err != nil {
		return Report{}, unavailable("interview report generation is temporarily unavailable", fmt.Errorf("generate report: %w", err))
	}
	if validationErr := validateReportDraft(session.Sources, draft); validationErr != nil {
		request.Repair = &RepairInstruction{
			Reason:          validationErr.Error(),
			AllowedEvidence: allEvidence(session.Sources),
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
	snapshot := cloneSession(InterviewSession{Profile: session.Profile, Answers: session.Answers})
	return Report{
		OverallScore: scoreReport(session.Answers),
		Summary:      strings.TrimSpace(draft.Summary),
		Strengths:    nonNilStrings(draft.Strengths),
		Gaps:         nonNilStrings(draft.Gaps),
		EvidenceRefs: canonicalEvidence(session.Sources, draft.EvidenceRefs),
		Profile:      snapshot.Profile,
		Turns:        snapshot.Answers,
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
	total := 0
	maximum := 0
	for _, record := range records {
		assessment := record.Assessment
		if record.Question.Kind == QuestionKnowledge || record.Question.Kind == QuestionPrerequisite && !hasResumeEvidence(record.Question) {
			total += assessment.Correctness + assessment.Depth + assessment.Specificity + assessment.Tradeoffs
			maximum += 20
			continue
		}
		total += assessment.Correctness + assessment.Depth + assessment.Specificity + assessment.Ownership + assessment.Metrics + assessment.Tradeoffs
		maximum += 30
	}
	return int(math.Round(float64(total) / float64(maximum) * 100))
}

func hasResumeEvidence(question Question) bool {
	for _, ref := range question.EvidenceRefs {
		if ref.Kind == SourceResume {
			return true
		}
	}
	return false
}

func validateQuestionDraft(index SourceIndex, draft QuestionDraft, allowed map[string]struct{}) error {
	if strings.TrimSpace(draft.Text) == "" {
		return errors.New("question text is empty")
	}
	return validateEvidenceRefs(index, draft.EvidenceRefs, allowed, true)
}

func validateReportDraft(index SourceIndex, draft ReportDraft) error {
	if strings.TrimSpace(draft.Summary) == "" {
		return errors.New("report summary is empty")
	}
	return validateEvidenceRefs(index, draft.EvidenceRefs, nil, true)
}

func validateAssessment(index SourceIndex, assessment Assessment) error {
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
	if err := validateEvidenceRefs(index, assessment.EvidenceRefs, nil, false); err != nil {
		return err
	}
	for i, check := range assessment.ClaimChecks {
		if strings.TrimSpace(check.Claim) == "" {
			return fmt.Errorf("claimChecks[%d].claim is empty", i)
		}
		switch check.Verdict {
		case ClaimSupported, ClaimContradicted:
			if err := validateEvidenceRefs(index, check.EvidenceRefs, nil, true); err != nil {
				return fmt.Errorf("claimChecks[%d]: %w", i, err)
			}
		case ClaimUnverified, ClaimNotInMaterial:
			if err := validateEvidenceRefs(index, check.EvidenceRefs, nil, false); err != nil {
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
