package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const contractPrivateReference = "PRIVATE_REFERENCE_CONTRACT_7D91"

func TestPrivacyContractReporterDoesNotReceiveKnowledgeReference(t *testing.T) {
	session, _, resumeRef := contractSession(t, contractPrivateReference)
	requests := make([]GenerateReportRequest, 0, 2)
	agent := &contractAgent{
		reportFn: func(request GenerateReportRequest) (ReportDraft, error) {
			requests = append(requests, request)
			if len(requests) == 1 {
				return ReportDraft{EvidenceRefs: []EvidenceRef{resumeRef}}, nil
			}
			return contractSafeReport(resumeRef), nil
		},
	}
	service := NewService(Dependencies{Agent: agent, Store: NewMemoryStore(), Clock: contractClock{}, IDs: &contractIDs{}})

	if _, err := service.generateReport(context.Background(), session); err != nil {
		t.Fatalf("generateReport() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("report calls = %d, want initial call plus repair", len(requests))
	}
	for index, request := range requests {
		assertContractPayloadIsPublic(t, fmt.Sprintf("report request %d", index+1), request)
	}
}

func TestPrivacyContractPlannerDoesNotReceiveOldAssessmentLeakText(t *testing.T) {
	session, knowledgeRef, _ := contractSession(t, contractPrivateReference)
	currentPoint := contractCoveragePointForAnchor(t, session.Profile, knowledgeRef.AnchorID)
	var alternate CoveragePoint
	for _, point := range session.Profile.Coverage {
		if point.ID != currentPoint.ID {
			alternate = point
			break
		}
	}
	if alternate.ID == "" {
		t.Fatal("privacy fixture needs an alternate coverage point")
	}

	question := Question{
		ID: "question-old", RootID: "root-old", Text: "旧知识题",
		Kind: QuestionKnowledge, Difficulty: DifficultyMedium,
		CoveragePointID: currentPoint.ID, EvidenceRefs: []EvidenceRef{knowledgeRef},
	}
	previous := AnswerRecord{
		Question: question,
		Answer:   AnswerPayload{Text: "candidate answer", InputMode: InputModeText},
		Assessment: Assessment{
			Correctness: 3, Depth: 3, Specificity: 3, Ownership: 1, Metrics: 1, Tradeoffs: 3,
			FactualErrors: []string{"参考答案：" + contractPrivateReference},
			Strengths:     []string{"reference answer: " + contractPrivateReference},
			Gaps:          []string{"参考内容：" + contractPrivateReference},
			EvidenceRefs:  []EvidenceRef{knowledgeRef},
			ClaimChecks: []ClaimCheck{{
				Claim: "private claim " + contractPrivateReference, Verdict: ClaimUnverified,
			}},
		},
		Decision: PolicyDecision{Action: PolicyAdvance, CoveragePointID: currentPoint.ID, RootID: "root-old"},
	}
	session.Config.QuestionCount = 4
	session.State = StateAwaitingAnswer
	session.CurrentQuestion = &question
	session.Answers = []AnswerRecord{previous}

	var captured PlanCoverageRequest
	planner := &contractPlanner{planFn: func(request PlanCoverageRequest) (CoverageSelection, error) {
		captured = request
		return CoverageSelection{
			CoveragePointID: alternate.ID,
			Reason:          "cover a different public target",
			Signals:         []string{"coverage debt"},
		}, nil
	}}
	service := NewService(Dependencies{Planner: planner, Store: NewMemoryStore(), Clock: contractClock{}, IDs: &contractIDs{}})

	if _, err := service.planNextCoverage(context.Background(), session, previous); err != nil {
		t.Fatalf("planNextCoverage() error = %v", err)
	}
	assertContractPayloadIsPublic(t, "planner request", captured)
}

func TestPrivacyContractAssessmentFreeTextLeakRepairsOrFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name            string
		leakAfterRepair bool
		wantError       bool
	}{
		{name: "valid repair is accepted"},
		{name: "leaking repair fails closed", leakAfterRepair: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, knowledgeRef, _ := contractSession(t, contractPrivateReference)
			point := contractCoveragePointForAnchor(t, session.Profile, knowledgeRef.AnchorID)
			question := Question{
				ID: "question-assessment", RootID: "root-assessment", Text: "请回答公开知识题",
				Kind: QuestionKnowledge, Difficulty: DifficultyMedium,
				CoveragePointID: point.ID, EvidenceRefs: []EvidenceRef{knowledgeRef},
			}
			calls := 0
			agent := &contractAgent{assessFn: func(request AssessAnswerRequest) (Assessment, error) {
				calls++
				assessment := contractSafeAssessment(knowledgeRef)
				if calls == 1 || test.leakAfterRepair {
					assessment.Gaps = []string{"参考答案是 " + contractPrivateReference}
				} else {
					if request.Repair == nil {
						t.Error("assessment repair call is missing repair instructions")
					}
					assessment.Gaps = []string{"需要补充公开可核验的解释"}
				}
				return assessment, nil
			}}
			service := NewService(Dependencies{Agent: agent, Store: NewMemoryStore(), Clock: contractClock{}, IDs: &contractIDs{}})

			_, err := service.assessAnswer(context.Background(), session, question, AnswerPayload{Text: "candidate answer", InputMode: InputModeText})
			if test.wantError && err == nil {
				t.Fatal("leaking assessment repair was accepted")
			}
			if !test.wantError && err != nil {
				t.Fatalf("repaired assessment error = %v", err)
			}
			if calls != 2 {
				t.Fatalf("assessment calls = %d, want initial call plus one repair", calls)
			}
		})
	}
}

func TestPrivacyContractReportFreeTextLeakRepairsOrFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name            string
		leakAfterRepair bool
		wantError       bool
	}{
		{name: "valid repair is accepted"},
		{name: "leaking repair fails closed", leakAfterRepair: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, _, resumeRef := contractSession(t, contractPrivateReference)
			calls := 0
			agent := &contractAgent{reportFn: func(request GenerateReportRequest) (ReportDraft, error) {
				calls++
				draft := contractSafeReport(resumeRef)
				if calls == 1 || test.leakAfterRepair {
					draft.Summary = "参考答案：" + contractPrivateReference
					draft.Gaps = []string{"reference content: " + contractPrivateReference}
				} else if request.Repair == nil {
					t.Error("report repair call is missing repair instructions")
				}
				return draft, nil
			}}
			service := NewService(Dependencies{Agent: agent, Store: NewMemoryStore(), Clock: contractClock{}, IDs: &contractIDs{}})

			_, err := service.generateReport(context.Background(), session)
			if test.wantError && err == nil {
				t.Fatal("leaking report repair was accepted")
			}
			if !test.wantError && err != nil {
				t.Fatalf("repaired report error = %v", err)
			}
			if calls != 2 {
				t.Fatalf("report calls = %d, want initial call plus one repair", calls)
			}
		})
	}
}

func TestPrivacyContractDeferredAnswerResponseOmitsAssessment(t *testing.T) {
	agent := &contractAgent{
		questionFn: func(request GenerateQuestionRequest) (QuestionDraft, error) {
			if len(request.Anchors) == 0 {
				return QuestionDraft{}, errors.New("question request has no anchor")
			}
			return QuestionDraft{
				Text:         "请说明你在项目中的具体职责。",
				EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])},
			}, nil
		},
		assessFn: func(request AssessAnswerRequest) (Assessment, error) {
			if len(request.Anchors) == 0 {
				return Assessment{}, errors.New("assessment request has no anchor")
			}
			return contractSafeAssessment(evidenceFromAnchor(request.Anchors[0])), nil
		},
	}
	service := NewService(Dependencies{Agent: agent, Store: NewMemoryStore(), Clock: contractClock{}, IDs: &contractIDs{}})
	started, err := service.Start(context.Background(), StartRequest{
		Action: ActionStart, ClientSessionID: "contract-client",
		Config: InterviewConfig{
			Focus: FocusProjects, Difficulty: DifficultyMedium, QuestionCount: 1,
			Language: "zh-CN", FeedbackMode: FeedbackDeferred,
		},
		Materials: MaterialsInput{Resume: &MaterialInput{Text: "项目 OfferPilot：我负责面试服务的架构设计与实现。"}},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	response, err := service.Answer(context.Background(), AnswerRequest{
		Action: ActionAnswer, InterviewID: started.InterviewID, QuestionID: started.Question.ID, ClientAnswerID: "contract-answer-1",
		Answer: AnswerPayload{Text: "我负责领域模型与持久化边界。", InputMode: InputModeText},
	})
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if feedback, ok := payload["feedback"].(map[string]any); ok {
		if _, leaked := feedback["assessment"]; leaked {
			t.Fatalf("deferred AnswerResponse exposed assessment: %s", encoded)
		}
	}
}

func TestScoreReportNormalizesEachTurnBeforeAggregation(t *testing.T) {
	records := []AnswerRecord{
		{
			Question: Question{Kind: QuestionKnowledge},
			Assessment: Assessment{
				Correctness: 5, Depth: 5, Specificity: 5, Ownership: 5, Metrics: 5, Tradeoffs: 5,
			},
		},
		{
			Question: Question{Kind: QuestionProject},
			Assessment: Assessment{
				Correctness: 1, Depth: 1, Specificity: 1, Ownership: 1, Metrics: 1, Tradeoffs: 1,
			},
		},
	}

	if score := scoreReport(records); score != 60 {
		t.Fatalf("mixed normalized score = %d, want 60 (mean of 100 and 20)", score)
	}
}

func contractSession(t *testing.T, secret string) (InterviewSession, EvidenceRef, EvidenceRef) {
	t.Helper()
	profile, sources := buildProfile(
		MaterialsInput{Resume: &MaterialInput{Text: "项目 OfferPilot：负责领域服务和持久化边界。"}},
		[]KnowledgeDocument{{
			ID: "kb-contract", Title: "Contract privacy",
			Content: "知识主题：隐私边界\n问题：如何隔离模型上下文？\n参考内容：" + secret + "\n参考答案：" + secret + "\n来源：contract.md",
		}},
		FocusMixed,
	)
	knowledgeRef := contractEvidenceByKind(t, sources, SourceKnowledge)
	resumeRef := contractEvidenceByKind(t, sources, SourceResume)
	point := contractCoveragePointForAnchor(t, profile, knowledgeRef.AnchorID)
	question := Question{
		ID: "question-contract", RootID: "root-contract", Text: "如何隔离模型上下文？",
		Kind: QuestionKnowledge, Difficulty: DifficultyMedium,
		CoveragePointID: point.ID, EvidenceRefs: []EvidenceRef{knowledgeRef},
	}
	return InterviewSession{
		ID: "interview-contract", ClientSessionID: "client-contract",
		Config: InterviewConfig{
			Focus: FocusMixed, Difficulty: DifficultyMedium, QuestionCount: 2,
			Language: "zh-CN", FeedbackMode: FeedbackImmediate,
		},
		State: StateCompleted, Profile: profile, Sources: sources,
		Answers: []AnswerRecord{{
			Question:   question,
			Answer:     AnswerPayload{Text: "candidate answer", InputMode: InputModeText},
			Assessment: contractSafeAssessment(knowledgeRef),
			Decision:   PolicyDecision{Action: PolicyComplete, CoveragePointID: point.ID, RootID: "root-contract"},
			AnsweredAt: time.Unix(1, 0).UTC(),
		}},
		StartedAt: time.Unix(0, 0).UTC(), CompletedAt: time.Unix(1, 0).UTC(), Version: 2,
	}, knowledgeRef, resumeRef
}

func contractCoveragePointForAnchor(t *testing.T, profile Profile, anchorID string) CoveragePoint {
	t.Helper()
	for _, point := range profile.Coverage {
		for _, ref := range point.EvidenceRefs {
			if ref.AnchorID == anchorID {
				return point
			}
		}
	}
	t.Fatalf("coverage point for anchor %q not found", anchorID)
	return CoveragePoint{}
}

func contractEvidenceByKind(t *testing.T, index SourceIndex, kind SourceKind) EvidenceRef {
	t.Helper()
	for _, anchorID := range index.Order {
		anchor, exists := index.Anchors[anchorID]
		if exists && anchor.Kind == kind {
			return evidenceFromAnchor(anchor)
		}
	}
	t.Fatalf("source anchor of kind %q not found", kind)
	return EvidenceRef{}
}

func contractSafeAssessment(ref EvidenceRef) Assessment {
	return Assessment{
		Correctness: 3, Depth: 3, Specificity: 3, Ownership: 3, Metrics: 3, Tradeoffs: 3,
		FactualErrors: []string{}, Strengths: []string{"回答有清晰结构"},
		Gaps: []string{"需要补充可核验细节"}, EvidenceRefs: []EvidenceRef{ref},
		ClaimChecks: []ClaimCheck{},
	}
}

func contractSafeReport(ref EvidenceRef) ReportDraft {
	return ReportDraft{
		Summary: "完成结构化评估。", Strengths: []string{"回答结构清晰"},
		Gaps: []string{"继续补充公开证据"}, EvidenceRefs: []EvidenceRef{ref},
	}
}

func assertContractPayloadIsPublic(t *testing.T, name string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		contractPrivateReference,
		"参考内容", "参考答案", "reference content", "reference answer",
	} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
			t.Fatalf("%s leaked %q: %s", name, forbidden, encoded)
		}
	}
}

type contractAgent struct {
	questionFn func(GenerateQuestionRequest) (QuestionDraft, error)
	assessFn   func(AssessAnswerRequest) (Assessment, error)
	reportFn   func(GenerateReportRequest) (ReportDraft, error)
}

func (a *contractAgent) GenerateQuestion(_ context.Context, request GenerateQuestionRequest) (QuestionDraft, error) {
	if a.questionFn == nil {
		return QuestionDraft{}, errors.New("contract question agent is not configured")
	}
	return a.questionFn(request)
}

func (a *contractAgent) AssessAnswer(_ context.Context, request AssessAnswerRequest) (Assessment, error) {
	if a.assessFn == nil {
		return Assessment{}, errors.New("contract assessment agent is not configured")
	}
	return a.assessFn(request)
}

func (a *contractAgent) GenerateReport(_ context.Context, request GenerateReportRequest) (ReportDraft, error) {
	if a.reportFn == nil {
		return ReportDraft{}, errors.New("contract report agent is not configured")
	}
	return a.reportFn(request)
}

type contractPlanner struct {
	planFn func(PlanCoverageRequest) (CoverageSelection, error)
}

func (p *contractPlanner) PlanCoverage(_ context.Context, request PlanCoverageRequest) (CoverageSelection, error) {
	return p.planFn(request)
}

type contractClock struct{}

func (contractClock) Now() time.Time { return time.Unix(2, 0).UTC() }

type contractIDs struct {
	next int
}

func (ids *contractIDs) NewID(prefix string) string {
	ids.next++
	return fmt.Sprintf("%s-contract-%d", prefix, ids.next)
}
