package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStartBuildsGroundedProfilesAfterQuestionRepair(t *testing.T) {
	tests := []struct {
		name      string
		focus     Focus
		materials MaterialsInput
		wantKind  SourceKind
	}{
		{
			name:      "JD knowledge anchor",
			focus:     FocusKnowledge,
			materials: MaterialsInput{JD: &MaterialInput{Text: "高级 Go 工程师\n负责设计高并发支付平台\n要求理解 Go 并发、Redis 和 Kafka"}},
			wantKind:  SourceJD,
		},
		{
			name:      "resume project anchor",
			focus:     FocusProjects,
			materials: MaterialsInput{Resume: &MaterialInput{Text: "后端工程师\n项目 OfferPilot：我负责设计自适应面试 Agent\n将响应延迟从 500ms 降至 120ms"}},
			wantKind:  SourceResume,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewMemoryStore()
			agent := &scriptedAgent{
				questionFn: func(request GenerateQuestionRequest, call int) (QuestionDraft, error) {
					if call == 1 {
						return QuestionDraft{Text: "ungrounded", EvidenceRefs: []EvidenceRef{{SourceID: "fake", Kind: SourceJD, AnchorID: "fake:999", Locator: "nowhere", Quote: "fabricated"}}}, nil
					}
					return QuestionDraft{
						Text:         "Grounded question about " + request.Anchors[0].Text,
						EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])},
					}, nil
				},
			}
			service := newTestService(agent, store)
			response, err := service.Start(context.Background(), startRequest(3, test.focus, test.materials))
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			if agent.questionCalls != 2 {
				t.Fatalf("GenerateQuestion calls = %d, want repair call", agent.questionCalls)
			}
			if len(response.Question.EvidenceRefs) == 0 {
				t.Fatal("repaired question has no evidence")
			}
			ref := response.Question.EvidenceRefs[0]
			if ref.Kind != test.wantKind {
				t.Fatalf("evidence kind = %q, want %q", ref.Kind, test.wantKind)
			}
			if !strings.Contains(response.Question.Text, ref.Quote) {
				t.Fatalf("repaired question %q is not anchored to quote %q", response.Question.Text, ref.Quote)
			}
			session, err := store.Load(context.Background(), response.InterviewID)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			anchor, exists := session.Sources.Anchors[ref.AnchorID]
			if !exists || anchor.Text != ref.Quote || anchor.SourceID != ref.SourceID {
				t.Fatalf("evidence was not canonical: ref=%+v anchor=%+v", ref, anchor)
			}
			if len(response.Profile.Coverage) == 0 {
				t.Fatal("profile has no coverage points")
			}
			if document := session.Sources.Documents[ref.SourceID]; strings.TrimSpace(document.Content) == "" {
				t.Fatal("source document content was lost in the store snapshot")
			}
		})
	}
}

func TestAnswerPolicyChangesWithAnswerQuality(t *testing.T) {
	tests := []struct {
		name           string
		answer         string
		assessment     Assessment
		wantTrigger    PolicyAction
		wantAxis       string
		wantDifficulty Difficulty
		wantSameRoot   bool
	}{
		{
			name:        "factual error lowers to prerequisite",
			answer:      "Redis 是关系型数据库，所以可以直接依赖 SQL join。",
			assessment:  Assessment{Correctness: 1, Depth: 2, Specificity: 3, Ownership: 2, Metrics: 1, Tradeoffs: 1, FactualErrors: []string{"Redis 不是关系型数据库"}},
			wantTrigger: PolicyPrerequisite, wantDifficulty: DifficultyEasy, wantSameRoot: true,
		},
		{
			name:        "vague answer probes ownership",
			answer:      "我们做了这个项目，最后效果不错。",
			assessment:  Assessment{Correctness: 3, Depth: 2, Specificity: 1, Ownership: 1, Metrics: 1, Tradeoffs: 1},
			wantTrigger: PolicyFollowUp, wantAxis: "ownership", wantDifficulty: DifficultyMedium, wantSameRoot: true,
		},
		{
			name:        "strong answer rotates and raises difficulty",
			answer:      "我负责设计并实现队列削峰方案，把峰值延迟从 500ms 降到 120ms。我们评估了 Kafka 与 Redis Streams，我选择 Kafka，但权衡是运维复杂度更高，因此补了告警、回放和容量压测，最终在两轮灰度后上线。",
			assessment:  Assessment{Correctness: 5, Depth: 5, Specificity: 5, Ownership: 5, Metrics: 5, Tradeoffs: 5, Strengths: []string{"决策可验证"}},
			wantTrigger: PolicyAdvance, wantDifficulty: DifficultyHard, wantSameRoot: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			agent := groundedAgent()
			agent.assessFn = func(_ AssessAnswerRequest, _ int) (Assessment, error) { return test.assessment, nil }
			service := newTestService(agent, NewMemoryStore())
			started, err := service.Start(context.Background(), startRequest(3, FocusMixed, standardMaterials()))
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			answered, err := service.Answer(context.Background(), answerRequest(started, test.answer))
			if err != nil {
				t.Fatalf("Answer() error = %v", err)
			}
			if answered.NextQuestion == nil {
				t.Fatal("NextQuestion is nil")
			}
			next := answered.NextQuestion
			if next.Adaptation.Trigger != test.wantTrigger {
				t.Errorf("trigger = %q, want %q", next.Adaptation.Trigger, test.wantTrigger)
			}
			if next.Adaptation.FollowUpAxis != test.wantAxis {
				t.Errorf("axis = %q, want %q", next.Adaptation.FollowUpAxis, test.wantAxis)
			}
			if next.Difficulty != test.wantDifficulty {
				t.Errorf("difficulty = %q, want %q", next.Difficulty, test.wantDifficulty)
			}
			if got := next.RootID == started.Question.RootID; got != test.wantSameRoot {
				t.Errorf("same root = %v, want %v", got, test.wantSameRoot)
			}
		})
	}
}

func TestAgentSemanticsDriveFollowUpWithoutKeywordScoring(t *testing.T) {
	tests := []struct {
		name       string
		answer     string
		assessment Assessment
		wantAxis   string
	}{
		{
			name:       "semantic ownership gap",
			answer:     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			assessment: Assessment{Correctness: 3, Depth: 3, Specificity: 2, Ownership: 1, Metrics: 5, Tradeoffs: 5},
			wantAxis:   "ownership",
		},
		{
			name:       "semantic metrics gap",
			answer:     "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
			assessment: Assessment{Correctness: 3, Depth: 3, Specificity: 2, Ownership: 5, Metrics: 1, Tradeoffs: 5},
			wantAxis:   "metrics",
		},
		{
			name:       "semantic tradeoff gap",
			answer:     "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC",
			assessment: Assessment{Correctness: 3, Depth: 3, Specificity: 2, Ownership: 5, Metrics: 5, Tradeoffs: 1},
			wantAxis:   "tradeoff",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			agent := groundedAgent()
			agent.assessFn = func(request AssessAnswerRequest, _ int) (Assessment, error) {
				if request.Answer.Text != test.answer {
					t.Fatalf("agent received answer %q, want %q", request.Answer.Text, test.answer)
				}
				return test.assessment, nil
			}
			service := newTestService(agent, NewMemoryStore())
			started, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			answered, err := service.Answer(context.Background(), answerRequest(started, test.answer))
			if err != nil {
				t.Fatalf("Answer() error = %v", err)
			}
			if answered.NextQuestion == nil || answered.NextQuestion.Adaptation.FollowUpAxis != test.wantAxis {
				t.Fatalf("next question = %+v, want semantic axis %q", answered.NextQuestion, test.wantAxis)
			}
		})
	}
}

func TestUnverifiedAnswerClaimRemainsUnverified(t *testing.T) {
	agent := groundedAgent()
	agent.assessFn = func(_ AssessAnswerRequest, _ int) (Assessment, error) {
		return Assessment{
			Correctness: 3, Depth: 3, Specificity: 2, Ownership: 3, Metrics: 3, Tradeoffs: 3,
			ClaimChecks: []ClaimCheck{{
				Claim: "候选人声称将吞吐量提升了十倍", Verdict: ClaimUnverified, EvidenceRefs: make([]EvidenceRef, 0),
			}},
		}, nil
	}
	service := newTestService(agent, NewMemoryStore())
	started, err := service.Start(context.Background(), startRequest(2, FocusProjects, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	answered, err := service.Answer(context.Background(), answerRequest(started, "我把吞吐量提升了十倍"))
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	checks := answered.Feedback.Assessment.ClaimChecks
	if len(checks) != 1 || checks[0].Verdict != ClaimUnverified || len(checks[0].EvidenceRefs) != 0 {
		t.Fatalf("claim checks = %+v, want one unverified claim without invented evidence", checks)
	}
}

func TestForgedClaimEvidenceTriggersAssessmentRepair(t *testing.T) {
	agent := groundedAgent()
	agent.assessFn = func(request AssessAnswerRequest, call int) (Assessment, error) {
		base := Assessment{Correctness: 3, Depth: 3, Specificity: 2, Ownership: 3, Metrics: 3, Tradeoffs: 3}
		if call == 1 {
			base.ClaimChecks = []ClaimCheck{{
				Claim: "候选人声称吞吐量提升十倍", Verdict: ClaimSupported, EvidenceRefs: forgedQuestion().EvidenceRefs,
			}}
			return base, nil
		}
		if request.Repair == nil || len(request.Repair.AllowedEvidence) == 0 {
			t.Fatal("assessment repair did not receive evidence whitelist")
		}
		base.ClaimChecks = []ClaimCheck{{Claim: "候选人声称吞吐量提升十倍", Verdict: ClaimUnverified, EvidenceRefs: make([]EvidenceRef, 0)}}
		return base, nil
	}
	service := newTestService(agent, NewMemoryStore())
	started, err := service.Start(context.Background(), startRequest(2, FocusProjects, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	answered, err := service.Answer(context.Background(), answerRequest(started, "吞吐量提升十倍"))
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if agent.assessCalls != 2 {
		t.Fatalf("assessment calls = %d, want initial plus repair", agent.assessCalls)
	}
	checks := answered.Feedback.Assessment.ClaimChecks
	if len(checks) != 1 || checks[0].Verdict != ClaimUnverified || len(checks[0].EvidenceRefs) != 0 {
		t.Fatalf("repaired claim checks = %+v", checks)
	}
}

func TestProjectRootStopsAfterTwoFollowUps(t *testing.T) {
	agent := groundedAgent()
	agent.assessFn = func(_ AssessAnswerRequest, _ int) (Assessment, error) {
		return Assessment{Correctness: 3, Depth: 2, Specificity: 1, Ownership: 1, Metrics: 1, Tradeoffs: 1}, nil
	}
	service := newTestService(agent, NewMemoryStore())
	started, err := service.Start(context.Background(), startRequest(4, FocusProjects, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	root := started.Question.RootID
	question := started.Question
	wantAxes := []string{"ownership", "metrics"}
	for depth, wantAxis := range wantAxes {
		response, answerErr := service.Answer(context.Background(), AnswerRequest{
			Action: ActionAnswer, InterviewID: started.InterviewID, QuestionID: question.ID, ClientAnswerID: fmt.Sprintf("answer-depth-%d", depth+1),
			Answer: AnswerPayload{Text: "我们参与了这个项目，效果还可以。", InputMode: InputModeText},
		})
		if answerErr != nil {
			t.Fatalf("Answer(depth=%d) error = %v", depth+1, answerErr)
		}
		if response.NextQuestion == nil {
			t.Fatalf("Answer(depth=%d) returned nil next question", depth+1)
		}
		question = *response.NextQuestion
		if question.RootID != root || question.Adaptation.Depth != depth+1 || question.Adaptation.FollowUpAxis != wantAxis {
			t.Fatalf("follow-up %d = %+v, want root=%q depth=%d axis=%q", depth+1, question, root, depth+1, wantAxis)
		}
	}

	response, err := service.Answer(context.Background(), AnswerRequest{
		Action: ActionAnswer, InterviewID: started.InterviewID, QuestionID: question.ID, ClientAnswerID: "answer-depth-3",
		Answer: AnswerPayload{Text: "仍然只是一个没有数据的概括。", InputMode: InputModeText},
	})
	if err != nil {
		t.Fatalf("third Answer() error = %v", err)
	}
	if response.NextQuestion == nil {
		t.Fatal("third Answer() returned nil next question")
	}
	if response.NextQuestion.RootID == root || response.NextQuestion.Adaptation.Depth != 0 || response.NextQuestion.Adaptation.Trigger != PolicyAdvance {
		t.Fatalf("root was not rotated after two follow-ups: %+v", response.NextQuestion)
	}
	if response.NextQuestion.CoveragePointID == started.Question.CoveragePointID {
		t.Fatal("coverage point did not rotate")
	}
}

func TestForgedQuestionEvidenceIsRepairedOrRejected(t *testing.T) {
	tests := []struct {
		name        string
		repairValid bool
		wantText    string
	}{
		{name: "valid repair accepted", repairValid: true, wantText: "repaired grounded question"},
		{name: "invalid repair fails closed", repairValid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			agent := &scriptedAgent{}
			agent.questionFn = func(request GenerateQuestionRequest, call int) (QuestionDraft, error) {
				if call == 1 {
					return forgedQuestion(), nil
				}
				if request.Repair == nil || len(request.Repair.AllowedEvidence) == 0 {
					t.Fatal("second model call did not contain repair evidence whitelist")
				}
				if !test.repairValid {
					return forgedQuestion(), nil
				}
				return QuestionDraft{Text: test.wantText, EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])}}, nil
			}
			store := NewMemoryStore()
			service := newTestService(agent, store)
			response, err := service.Start(context.Background(), startRequest(2, FocusProjects, standardMaterials()))
			if agent.questionCalls != 2 {
				t.Fatalf("question calls = %d, want 2", agent.questionCalls)
			}
			if !test.repairValid {
				if !IsCode(err, CodeUnavailable) {
					t.Fatalf("Start() error = %v, want service unavailable", err)
				}
				if response.InterviewID != "" || len(store.sessions) != 0 {
					t.Fatalf("failed question was committed: response=%+v sessions=%d", response, len(store.sessions))
				}
				return
			}
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			if !strings.Contains(response.Question.Text, test.wantText) {
				t.Fatalf("question text = %q, want substring %q", response.Question.Text, test.wantText)
			}
			for _, ref := range response.Question.EvidenceRefs {
				if ref.AnchorID == "fake:999" {
					t.Fatal("forged evidence escaped validation")
				}
			}
		})
	}
}

func TestAgentFailuresFailClosedWithoutCommitting(t *testing.T) {
	t.Run("missing agent does not create interview", func(t *testing.T) {
		store := NewMemoryStore()
		service := newTestService(nil, store)
		response, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
		if !IsCode(err, CodeUnavailable) {
			t.Fatalf("Start() error = %v, want service unavailable", err)
		}
		if response.InterviewID != "" || len(store.sessions) != 0 {
			t.Fatalf("missing agent created an interview: response=%+v sessions=%d", response, len(store.sessions))
		}
	})

	t.Run("question call failure does not create interview", func(t *testing.T) {
		cause := errors.New("question model offline")
		agent := groundedAgent()
		agent.questionFn = func(GenerateQuestionRequest, int) (QuestionDraft, error) {
			return QuestionDraft{}, cause
		}
		store := NewMemoryStore()
		service := newTestService(agent, store)
		_, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
		if !IsCode(err, CodeUnavailable) || !errors.Is(err, cause) {
			t.Fatalf("Start() error = %v, want wrapped service unavailable", err)
		}
		if len(store.sessions) != 0 {
			t.Fatalf("question failure created %d sessions", len(store.sessions))
		}
	})

	t.Run("assessment failure does not save answer", func(t *testing.T) {
		cause := errors.New("assessment model offline")
		agent := groundedAgent()
		store := NewMemoryStore()
		service := newTestService(agent, store)
		started, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		agent.assessFn = func(AssessAnswerRequest, int) (Assessment, error) {
			return Assessment{}, cause
		}
		_, err = service.Answer(context.Background(), answerRequest(started, "A concrete answer with metrics and tradeoffs."))
		if !IsCode(err, CodeUnavailable) || !errors.Is(err, cause) {
			t.Fatalf("Answer() error = %v, want wrapped service unavailable", err)
		}
		stored, loadErr := store.Load(context.Background(), started.InterviewID)
		if loadErr != nil {
			t.Fatalf("Load() error = %v", loadErr)
		}
		if stored.Version != 1 || len(stored.Answers) != 0 || stored.CurrentQuestion == nil || stored.CurrentQuestion.ID != started.Question.ID {
			t.Fatalf("assessment failure mutated session: %+v", stored)
		}
	})

	t.Run("next question failure does not save assessment", func(t *testing.T) {
		cause := errors.New("question model offline")
		agent := groundedAgent()
		agent.questionFn = func(request GenerateQuestionRequest, call int) (QuestionDraft, error) {
			if call == 1 {
				return QuestionDraft{Text: "opening question", EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])}}, nil
			}
			return QuestionDraft{}, cause
		}
		agent.assessFn = func(AssessAnswerRequest, int) (Assessment, error) {
			return Assessment{Correctness: 5, Depth: 5, Specificity: 5, Ownership: 5, Metrics: 5, Tradeoffs: 5}, nil
		}
		store := NewMemoryStore()
		service := newTestService(agent, store)
		started, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		_, err = service.Answer(context.Background(), answerRequest(started, "A concrete answer with metrics and tradeoffs."))
		if !IsCode(err, CodeUnavailable) || !errors.Is(err, cause) {
			t.Fatalf("Answer() error = %v, want wrapped service unavailable", err)
		}
		stored, loadErr := store.Load(context.Background(), started.InterviewID)
		if loadErr != nil {
			t.Fatalf("Load() error = %v", loadErr)
		}
		if stored.Version != 1 || len(stored.Answers) != 0 || stored.CurrentQuestion == nil || stored.CurrentQuestion.ID != started.Question.ID {
			t.Fatalf("next-question failure committed an assessment: %+v", stored)
		}
	})

	t.Run("report failure does not save formal report", func(t *testing.T) {
		cause := errors.New("report model offline")
		agent := groundedAgent()
		store := NewMemoryStore()
		service := newTestService(agent, store)
		started, err := service.Start(context.Background(), startRequest(1, FocusMixed, standardMaterials()))
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		answered, err := service.Answer(context.Background(), answerRequest(started, "A concrete answer with metrics and tradeoffs."))
		if err != nil || !answered.ReportReady {
			t.Fatalf("Answer() = %+v, %v; want report ready", answered, err)
		}
		agent.reportFn = func(GenerateReportRequest, int) (ReportDraft, error) {
			return ReportDraft{}, cause
		}
		_, err = service.Report(context.Background(), ReportRequest{Action: ActionReport, InterviewID: started.InterviewID})
		if !IsCode(err, CodeUnavailable) || !errors.Is(err, cause) {
			t.Fatalf("Report() error = %v, want wrapped service unavailable", err)
		}
		stored, loadErr := store.Load(context.Background(), started.InterviewID)
		if loadErr != nil {
			t.Fatalf("Load() error = %v", loadErr)
		}
		if stored.Version != 2 || stored.Report != nil {
			t.Fatalf("report failure mutated session: version=%d report=%+v", stored.Version, stored.Report)
		}
	})
}

func TestDuplicateAnswerReplaysSucceededResponse(t *testing.T) {
	agent := groundedAgent()
	service := newTestService(agent, NewMemoryStore())
	started, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	request := answerRequest(started, "我负责实现该模块，但目前没有补充量化数据。")
	first, err := service.Answer(context.Background(), request)
	if err != nil {
		t.Fatalf("first Answer() error = %v", err)
	}
	second, err := service.Answer(context.Background(), request)
	if err != nil || !reflect.DeepEqual(second, first) {
		t.Fatalf("duplicate Answer() = %+v, %v; want replay %+v", second, err, first)
	}
	if agent.assessCalls != 1 || agent.questionCalls != 2 {
		t.Fatalf("agent calls after replay = assess %d generate %d, want 1/2", agent.assessCalls, agent.questionCalls)
	}
}

func TestConcurrentAnswersWithSameIDExecuteOnce(t *testing.T) {
	const callers = 20
	entered := make(chan struct{})
	release := make(chan struct{})
	agent := groundedAgent()
	agent.assessFn = func(_ AssessAnswerRequest, _ int) (Assessment, error) {
		close(entered)
		<-release
		return Assessment{Correctness: 4, Depth: 3, Specificity: 3, Ownership: 3, Metrics: 3, Tradeoffs: 3}, nil
	}
	service := newTestService(agent, NewMemoryStore())
	started, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	request := answerRequest(started, "我负责实现该模块，将延迟降到 120ms，但方案仍有运维复杂度的权衡。")
	type answerResult struct {
		response AnswerResponse
		err      error
	}
	results := make(chan answerResult, callers)
	for i := 0; i < callers; i++ {
		go func() {
			response, answerErr := service.Answer(context.Background(), request)
			results <- answerResult{response: response, err: answerErr}
		}()
	}
	<-entered
	close(release)
	var first AnswerResponse
	for i := 0; i < callers; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent Answer() error = %v", result.err)
		}
		if i == 0 {
			first = result.response
		} else if !reflect.DeepEqual(result.response, first) {
			t.Fatalf("concurrent response %d = %+v, want replay %+v", i, result.response, first)
		}
	}
	if agent.assessCalls != 1 || agent.questionCalls != 2 {
		t.Fatalf("agent calls = assess %d generate %d, want 1/2", agent.assessCalls, agent.questionCalls)
	}
	stored, err := service.store.Load(context.Background(), started.InterviewID)
	if err != nil || len(stored.Answers) != 1 || stored.Version != 2 {
		t.Fatalf("stored session = answers %d version %d error %v, want 1/2", len(stored.Answers), stored.Version, err)
	}
}

func TestReportIsGroundedRepairedAndIdempotent(t *testing.T) {
	agent := groundedAgent()
	agent.reportFn = func(request GenerateReportRequest, call int) (ReportDraft, error) {
		if call == 1 {
			return ReportDraft{Summary: "forged", EvidenceRefs: forgedQuestion().EvidenceRefs}, nil
		}
		if request.Repair == nil || len(request.Repair.AllowedEvidence) == 0 {
			t.Fatal("report repair did not receive allowed evidence")
		}
		return ReportDraft{
			Summary:      "grounded report",
			Strengths:    []string{"量化表达清楚"},
			Gaps:         []string{"补充边界条件"},
			EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])},
		}, nil
	}
	clock := &incrementingClock{current: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	service := NewService(Dependencies{Agent: agent, Store: NewMemoryStore(), Clock: clock, IDs: &sequenceIDs{}})
	started, err := service.Start(context.Background(), startRequest(1, FocusMixed, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	answered, err := service.Answer(context.Background(), answerRequest(started, "我负责设计并实现服务，将延迟从 500ms 降至 120ms；权衡是提高了运维复杂度。"))
	if err != nil || !answered.ReportReady {
		t.Fatalf("Answer() = %+v, %v; want report ready", answered, err)
	}
	request := ReportRequest{Action: ActionReport, InterviewID: started.InterviewID}
	first, err := service.Report(context.Background(), request)
	if err != nil {
		t.Fatalf("first Report() error = %v", err)
	}
	second, err := service.Report(context.Background(), request)
	if err != nil {
		t.Fatalf("second Report() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reports differ:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if agent.reportCalls != 2 {
		t.Fatalf("report model calls = %d, want exactly initial+repair and no calls on replay", agent.reportCalls)
	}
	if first.Report.Summary != "grounded report" || len(first.Report.Audit.Turns) != 1 || len(first.Report.Turns) != 1 || len(first.Report.Profile.Coverage) == 0 {
		t.Fatalf("unexpected report = %+v", first.Report)
	}
	for _, ref := range first.Report.EvidenceRefs {
		if ref.AnchorID == "fake:999" {
			t.Fatal("forged report evidence escaped validation")
		}
	}
}

func TestAdvanceUsesSemanticCoveragePlannerInsteadOfCursor(t *testing.T) {
	agent := groundedAgent()
	agent.assessFn = func(_ AssessAnswerRequest, _ int) (Assessment, error) {
		return Assessment{Correctness: 5, Depth: 5, Specificity: 5, Ownership: 5, Metrics: 5, Tradeoffs: 5}, nil
	}
	planner := &scriptedPlanner{planFn: func(request PlanCoverageRequest) (CoverageSelection, error) {
		if len(request.History) != 1 || request.RemainingQuestions != 2 || len(request.Candidates) < 3 {
			t.Fatalf("planner request is missing bounded interview context: %+v", request)
		}
		selected := request.Candidates[len(request.Candidates)-1]
		if selected.CoveragePointID == request.CurrentCoveragePointID {
			t.Fatal("test fixture unexpectedly selected the current point")
		}
		return CoverageSelection{
			CoveragePointID: selected.CoveragePointID,
			Reason:          "the previous project answer was strong; cover the highest-value untested knowledge gap",
			Signals:         []string{"strong previous assessment", "zero prior questions", "JD priority"},
		}, nil
	}}
	store := NewMemoryStore()
	service := NewService(Dependencies{Agent: agent, Planner: planner, Store: store, Clock: fixedClock{value: time.Now()}, IDs: &sequenceIDs{}})
	started, err := service.Start(context.Background(), startRequest(3, FocusMixed, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	answered, err := service.Answer(context.Background(), answerRequest(started, "完整且可核验的项目回答"))
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if planner.calls != 1 || answered.NextQuestion == nil || answered.NextQuestion.CoveragePointID != planner.selectedID {
		t.Fatalf("planner calls=%d selected=%q next=%+v", planner.calls, planner.selectedID, answered.NextQuestion)
	}
	loaded, loadErr := store.Load(context.Background(), started.InterviewID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(loaded.Answers) != 1 || loaded.Answers[0].Decision.Reason == "strong answer; rotate coverage and increase difficulty" {
		t.Fatalf("planner reason was not committed: %+v", loaded.Answers)
	}
}

func TestCoveragePlannerFailureDoesNotCommitAnswer(t *testing.T) {
	agent := groundedAgent()
	agent.assessFn = func(_ AssessAnswerRequest, _ int) (Assessment, error) {
		return Assessment{Correctness: 5, Depth: 5, Specificity: 5, Ownership: 5, Metrics: 5, Tradeoffs: 5}, nil
	}
	plannerCause := errors.New("planner timeout")
	planner := &scriptedPlanner{planFn: func(PlanCoverageRequest) (CoverageSelection, error) {
		return CoverageSelection{}, plannerCause
	}}
	store := NewMemoryStore()
	service := NewService(Dependencies{Agent: agent, Planner: planner, Store: store, Clock: fixedClock{value: time.Now()}, IDs: &sequenceIDs{}})
	started, err := service.Start(context.Background(), startRequest(3, FocusMixed, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	_, err = service.Answer(context.Background(), answerRequest(started, "完整且可核验的项目回答"))
	if !IsCode(err, CodeUnavailable) || !errors.Is(err, plannerCause) {
		t.Fatalf("Answer() error = %v, want wrapped planner service unavailable", err)
	}
	loaded, loadErr := store.Load(context.Background(), started.InterviewID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if loaded.Version != 1 || len(loaded.Answers) != 0 || loaded.CurrentQuestion == nil || loaded.CurrentQuestion.ID != started.Question.ID {
		t.Fatalf("planner failure committed partial state: %+v", loaded)
	}
}

func TestKnowledgeQuestionGenerationKeepsPrivateReferenceOutOfModelContext(t *testing.T) {
	const (
		privateContent = "PRIVATE_CONTENT_7C91"
		privateAnswer  = "PRIVATE_ANSWER_4F28"
	)
	documents := []KnowledgeDocument{{
		ID:    "kb-colbert",
		Title: "ColBERT MaxSim",
		Content: "知识主题：RAG\n问题：ColBERT 的 MaxSim 如何工作？\n参考内容：" + privateContent +
			"\n参考答案：" + privateAnswer + "\n来源：rag.md",
	}}
	agent := &scriptedAgent{questionFn: func(request GenerateQuestionRequest, call int) (QuestionDraft, error) {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		contextText := string(encoded)
		for _, forbidden := range []string{"参考内容", "参考答案", privateContent, privateAnswer} {
			if strings.Contains(contextText, forbidden) {
				t.Fatalf("question call %d received private knowledge %q: %s", call, forbidden, contextText)
			}
		}
		if len(request.Anchors) != 1 || !strings.Contains(request.Anchors[0].Text, "问题：ColBERT") {
			t.Fatalf("question call %d missing public anchor: %+v", call, request.Anchors)
		}
		ref := evidenceFromAnchor(request.Anchors[0])
		if call == 1 {
			return QuestionDraft{Text: "答案是 " + privateContent, EvidenceRefs: []EvidenceRef{ref}}, nil
		}
		if request.Repair == nil || len(request.Repair.AllowedEvidence) != 1 {
			t.Fatalf("repair call is missing its public evidence whitelist: %+v", request.Repair)
		}
		return QuestionDraft{Text: "请解释 ColBERT MaxSim 的机制和适用边界。", EvidenceRefs: []EvidenceRef{ref}}, nil
	}}
	service := NewService(Dependencies{
		Agent:     agent,
		Retriever: staticKnowledgeRetriever(documents),
		Store:     NewMemoryStore(),
		Clock:     fixedClock{value: time.Now()},
		IDs:       &sequenceIDs{},
	})

	started, err := service.Start(context.Background(), startRequest(1, FocusKnowledge, MaterialsInput{}))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if agent.questionCalls != 2 {
		t.Fatalf("question calls = %d, want leak rejection plus one repair", agent.questionCalls)
	}
	if strings.Contains(started.Profile.Coverage[0].Label, privateContent) || strings.Contains(started.Profile.Coverage[0].Label, "参考") {
		t.Fatalf("knowledge coverage label is private: %q", started.Profile.Coverage[0].Label)
	}
	if strings.Contains(started.Question.Text, privateContent) || strings.Contains(started.Question.Text, privateAnswer) {
		t.Fatalf("repaired question leaked private reference: %q", started.Question.Text)
	}
}

func TestQuestionGenerationRepairsRecentDuplicate(t *testing.T) {
	documents := []KnowledgeDocument{{
		ID:      "kb-maxsim",
		Title:   "MaxSim",
		Content: "知识主题：RAG\n问题：MaxSim 如何工作？\n参考内容：分别编码查询和文档，再聚合 token 相似度。",
	}}
	agent := &scriptedAgent{
		questionFn: func(request GenerateQuestionRequest, call int) (QuestionDraft, error) {
			ref := evidenceFromAnchor(request.Anchors[0])
			switch call {
			case 1:
				return QuestionDraft{Text: "MaxSim 如何工作？", EvidenceRefs: []EvidenceRef{ref}}, nil
			case 2:
				return QuestionDraft{Text: "MaxSim 如何工作", EvidenceRefs: []EvidenceRef{ref}}, nil
			default:
				if request.Repair == nil || !strings.Contains(request.Repair.Reason, "repeats") {
					t.Fatalf("duplicate repair reason = %+v", request.Repair)
				}
				return QuestionDraft{Text: "MaxSim 在长文档检索中有哪些计算边界？", EvidenceRefs: []EvidenceRef{ref}}, nil
			}
		},
		assessFn: func(request AssessAnswerRequest, _ int) (Assessment, error) {
			return Assessment{
				Correctness: 5, Depth: 5, Specificity: 5, Ownership: 1, Metrics: 1, Tradeoffs: 5,
				EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])},
			}, nil
		},
	}
	service := NewService(Dependencies{
		Agent: agent, Retriever: staticKnowledgeRetriever(documents), Store: NewMemoryStore(),
		Clock: fixedClock{value: time.Now()}, IDs: &sequenceIDs{},
	})
	started, err := service.Start(context.Background(), startRequest(2, FocusKnowledge, MaterialsInput{}))
	if err != nil {
		t.Fatal(err)
	}
	answered, err := service.Answer(context.Background(), answerRequest(started, "它使用 late interaction。"))
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if agent.questionCalls != 3 || answered.NextQuestion == nil || answered.NextQuestion.Text == started.Question.Text {
		t.Fatalf("duplicate was not repaired: calls=%d next=%+v", agent.questionCalls, answered.NextQuestion)
	}
}

func TestAssessmentCitationsAreLimitedToCurrentQuestionEvidence(t *testing.T) {
	for _, test := range []struct {
		name       string
		forgeClaim bool
	}{
		{name: "assessment evidence"},
		{name: "claim evidence", forgeClaim: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			seedDocuments := []KnowledgeDocument{
				{ID: "kb-current", Title: "Current", Content: "问题：当前题？\n参考内容：当前题答案。"},
				{ID: "kb-other", Title: "Other", Content: "问题：其他题？\n参考内容：其他题答案。"},
			}
			retriever := &scriptedKnowledgeRetriever{retrieveFn: func(query KnowledgeQuery, _ int) ([]KnowledgeDocument, error) {
				if query.Objective == "" {
					return seedDocuments, nil
				}
				return seedDocuments[:1], nil
			}}
			agent := groundedAgent()
			agent.assessFn = func(request AssessAnswerRequest, call int) (Assessment, error) {
				if len(request.Anchors) != 1 || request.Anchors[0].SourceID != "knowledge:kb-current" {
					t.Fatalf("assessment received evidence outside the current bundle: %+v", request.Anchors)
				}
				current := evidenceFromAnchor(request.Anchors[0])
				other := EvidenceRef{
					SourceID: "knowledge:kb-other", Kind: SourceKnowledge, AnchorID: "knowledge:kb-other:block",
					Locator: "question-block", Quote: seedDocuments[1].Content,
				}
				assessment := Assessment{
					Correctness: 3, Depth: 3, Specificity: 3, Ownership: 1, Metrics: 1, Tradeoffs: 3,
					EvidenceRefs: []EvidenceRef{current},
				}
				if call == 1 {
					if test.forgeClaim {
						assessment.ClaimChecks = []ClaimCheck{{Claim: "引用了其他题", Verdict: ClaimSupported, EvidenceRefs: []EvidenceRef{other}}}
					} else {
						assessment.EvidenceRefs = []EvidenceRef{other}
					}
					return assessment, nil
				}
				if request.Repair == nil || len(request.Repair.AllowedEvidence) != 1 || request.Repair.AllowedEvidence[0].AnchorID != current.AnchorID {
					t.Fatalf("repair evidence escaped current question: %+v", request.Repair)
				}
				assessment.ClaimChecks = []ClaimCheck{{Claim: "其他题材料不能证明本题", Verdict: ClaimUnverified}}
				return assessment, nil
			}
			service := NewService(Dependencies{
				Agent: agent, Retriever: retriever, Store: NewMemoryStore(),
				Clock: fixedClock{value: time.Now()}, IDs: &sequenceIDs{},
			})
			started, err := service.Start(context.Background(), startRequest(1, FocusKnowledge, MaterialsInput{}))
			if err != nil {
				t.Fatal(err)
			}
			answered, err := service.Answer(context.Background(), answerRequest(started, "候选人回答"))
			if err != nil {
				t.Fatalf("Answer() error = %v", err)
			}
			if agent.assessCalls != 2 || len(answered.Feedback.Assessment.EvidenceRefs) != 1 || answered.Feedback.Assessment.EvidenceRefs[0].AnchorID != started.Question.EvidenceRefs[0].AnchorID {
				t.Fatalf("assessment citation was not repaired: calls=%d feedback=%+v", agent.assessCalls, answered.Feedback)
			}
		})
	}
}

func TestKnowledgeRetrievalIsScopedToSelectedCoveragePoint(t *testing.T) {
	const (
		redisPrivate = "REDIS_PRIVATE_REFERENCE_17A9"
		kafkaPrivate = "KAFKA_PRIVATE_REFERENCE_62C4"
	)
	retriever := &scriptedKnowledgeRetriever{retrieveFn: func(query KnowledgeQuery, _ int) ([]KnowledgeDocument, error) {
		switch {
		case strings.Contains(query.Objective, "Redis"):
			return []KnowledgeDocument{{
				ID: "kb-redis", Title: "Redis hotspots",
				Content: "问题：如何治理 Redis 热点缓存？\n参考内容：" + redisPrivate,
			}}, nil
		case strings.Contains(query.Objective, "Kafka"):
			return []KnowledgeDocument{{
				ID: "kb-kafka", Title: "Kafka rebalance",
				Content: "问题：如何控制 Kafka 消费者重平衡？\n参考内容：" + kafkaPrivate,
			}}, nil
		default:
			t.Fatalf("retrieval was not bound to a coverage objective: %+v", query)
			return nil, nil
		}
	}}

	agent := &scriptedAgent{}
	agent.questionFn = func(request GenerateQuestionRequest, call int) (QuestionDraft, error) {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		contextText := string(encoded)
		if strings.Contains(contextText, redisPrivate) || strings.Contains(contextText, kafkaPrivate) {
			t.Fatalf("interviewer call %d received a private reference: %s", call, contextText)
		}
		wantSource := "knowledge:kb-redis"
		forbiddenSource := "knowledge:kb-kafka"
		question := "请解释 Redis 热点缓存治理的关键取舍。"
		if call == 2 {
			wantSource, forbiddenSource = forbiddenSource, wantSource
			question = "请解释 Kafka 消费者重平衡的控制策略。"
		}
		if !requestHasAnchorSource(request.Anchors, wantSource) || requestHasAnchorSource(request.Anchors, forbiddenSource) {
			t.Fatalf("interviewer call %d evidence was not isolated: %+v", call, request.Anchors)
		}
		return QuestionDraft{Text: question, EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])}}, nil
	}
	agent.assessFn = func(request AssessAnswerRequest, _ int) (Assessment, error) {
		if !requestHasAnchorSource(request.Anchors, "knowledge:kb-redis") || requestHasAnchorSource(request.Anchors, "knowledge:kb-kafka") {
			t.Fatalf("assessor evidence was not isolated to Redis: %+v", request.Anchors)
		}
		encoded, err := json.Marshal(request.Anchors)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), redisPrivate) || strings.Contains(string(encoded), kafkaPrivate) {
			t.Fatalf("assessor received the wrong private bundle: %s", encoded)
		}
		return Assessment{
			Correctness: 5, Depth: 5, Specificity: 5, Ownership: 5, Metrics: 5, Tradeoffs: 5,
			Gaps:         []string{"需要验证 Kafka rebalance 边界"},
			EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])},
		}, nil
	}
	planner := &scriptedPlanner{planFn: func(request PlanCoverageRequest) (CoverageSelection, error) {
		for _, candidate := range request.Candidates {
			if strings.Contains(candidate.Label, "Kafka") {
				return CoverageSelection{CoveragePointID: candidate.CoveragePointID, Reason: "验证下一项岗位要求", Signals: []string{"未覆盖"}}, nil
			}
		}
		return CoverageSelection{}, errors.New("Kafka coverage point not found")
	}}
	service := NewService(Dependencies{
		Agent: agent, Planner: planner, Retriever: retriever, Store: NewMemoryStore(),
		Clock: fixedClock{value: time.Now()}, IDs: &sequenceIDs{},
	})
	started, err := service.Start(context.Background(), startRequest(2, FocusKnowledge, MaterialsInput{
		JD: &MaterialInput{Text: "高级缓存工程师\n负责 Redis 热点缓存治理\n负责 Kafka 消费者重平衡治理"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	answered, err := service.Answer(context.Background(), answerRequest(started, "我会先识别热点 key，再隔离读写路径并验证容量边界。"))
	if err != nil {
		t.Fatal(err)
	}
	if answered.NextQuestion == nil || agent.questionCalls != 2 {
		t.Fatalf("next scoped question was not generated: %+v", answered.NextQuestion)
	}
	if retriever.calls != 2 {
		t.Fatalf("retrieval calls = %d, want one per selected coverage point", retriever.calls)
	}
	secondQuery := retriever.queries[1]
	if !strings.Contains(secondQuery.Objective, "Kafka") || secondQuery.Question != started.Question.Text ||
		len(secondQuery.PreviousGaps) != 1 || secondQuery.PreviousGaps[0] != "需要验证 Kafka rebalance 边界" {
		t.Fatalf("adaptive retrieval query = %+v", secondQuery)
	}
}

func requestHasAnchorSource(anchors []SourceAnchor, sourceID string) bool {
	for _, anchor := range anchors {
		if anchor.SourceID == sourceID {
			return true
		}
	}
	return false
}

func TestAssessmentWithoutEvidenceFailsClosedInsteadOfInventingCitation(t *testing.T) {
	agent := groundedAgent()
	agent.preserveEmptyAssessmentEvidence = true
	agent.assessFn = func(AssessAnswerRequest, int) (Assessment, error) {
		return Assessment{Correctness: 3, Depth: 3, Specificity: 3, Ownership: 3, Metrics: 3, Tradeoffs: 3}, nil
	}
	store := NewMemoryStore()
	service := newTestService(agent, store)
	started, err := service.Start(context.Background(), startRequest(1, FocusProjects, standardMaterials()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Answer(context.Background(), answerRequest(started, "候选人回答"))
	if !IsCode(err, CodeUnavailable) || agent.assessCalls != 2 {
		t.Fatalf("Answer() error=%v calls=%d, want failed initial+repair", err, agent.assessCalls)
	}
	loaded, loadErr := store.Load(context.Background(), started.InterviewID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(loaded.Answers) != 0 || loaded.Version != 1 || loaded.CurrentQuestion == nil || loaded.CurrentQuestion.ID != started.Question.ID {
		t.Fatalf("ungrounded assessment was committed: %+v", loaded)
	}
}

func TestPersistedKnowledgeContextIsSanitizedForPlannerAndInterviewer(t *testing.T) {
	const secret = "PERSISTED_PRIVATE_REFERENCE_91B2"
	profile, sources := buildProfile(MaterialsInput{}, []KnowledgeDocument{{
		ID: "kb-old", Title: "Old", Content: "问题：旧会话公开问题？\n参考答案：" + secret,
	}}, FocusKnowledge)
	fullRef := profile.Coverage[0].EvidenceRefs[0]
	profile.Coverage[0].Label = fullRef.Quote
	session := InterviewSession{
		Config:  InterviewConfig{Focus: FocusKnowledge},
		Profile: profile,
		Sources: sources,
		Answers: []AnswerRecord{{
			Question: Question{
				Text: "旧题泄露 " + secret, CoveragePointID: profile.Coverage[0].ID,
				EvidenceRefs: []EvidenceRef{fullRef}, Adaptation: QuestionAdaptation{Reason: "参考答案：" + secret},
			},
			Assessment: Assessment{Strengths: []string{"参考答案：" + secret}, EvidenceRefs: []EvidenceRef{fullRef}},
			Decision:   PolicyDecision{Action: PolicyAdvance, Reason: "参考答案：" + secret},
		}},
	}
	candidates := coverageCandidates(session)
	publicProfile, publicHistory := publicQuestionContext(session)
	encoded, err := json.Marshal(struct {
		Candidates []CoverageCandidate
		Profile    Profile
		History    []AnswerRecord
	}{candidates, publicProfile, publicHistory})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, "参考答案"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("persisted model context leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestMemoryStoreRequiresExactVersionAndReturnsSnapshots(t *testing.T) {
	store := NewMemoryStore()
	session := InterviewSession{ID: "i-1", Version: 1, Answers: make([]AnswerRecord, 0)}
	if err := store.Create(context.Background(), session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	loaded, err := store.Load(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	loaded.Version = 2
	if err = store.Save(context.Background(), loaded, 0); !errors.Is(err, ErrStoreConflict) {
		t.Fatalf("Save(stale) error = %v, want conflict", err)
	}
	loaded.Version = 2
	if err = store.Save(context.Background(), loaded, 1); err != nil {
		t.Fatalf("Save(valid) error = %v", err)
	}
	loaded.Version = 99
	again, _ := store.Load(context.Background(), session.ID)
	if again.Version != 2 {
		t.Fatalf("stored snapshot mutated through caller: version=%d", again.Version)
	}
}

type scriptedAgent struct {
	mu                              sync.Mutex
	questionCalls                   int
	assessCalls                     int
	reportCalls                     int
	preserveEmptyAssessmentEvidence bool
	questionFn                      func(GenerateQuestionRequest, int) (QuestionDraft, error)
	assessFn                        func(AssessAnswerRequest, int) (Assessment, error)
	reportFn                        func(GenerateReportRequest, int) (ReportDraft, error)
}

type scriptedPlanner struct {
	calls      int
	selectedID string
	planFn     func(PlanCoverageRequest) (CoverageSelection, error)
}

func (p *scriptedPlanner) PlanCoverage(_ context.Context, request PlanCoverageRequest) (CoverageSelection, error) {
	p.calls++
	selection, err := p.planFn(request)
	p.selectedID = selection.CoveragePointID
	return selection, err
}

func (a *scriptedAgent) GenerateQuestion(_ context.Context, request GenerateQuestionRequest) (QuestionDraft, error) {
	a.mu.Lock()
	a.questionCalls++
	call := a.questionCalls
	fn := a.questionFn
	a.mu.Unlock()
	if fn == nil {
		return QuestionDraft{}, errors.New("question agent unavailable")
	}
	return fn(request, call)
}

func (a *scriptedAgent) AssessAnswer(_ context.Context, request AssessAnswerRequest) (Assessment, error) {
	a.mu.Lock()
	a.assessCalls++
	call := a.assessCalls
	fn := a.assessFn
	preserveEmpty := a.preserveEmptyAssessmentEvidence
	a.mu.Unlock()
	if fn == nil {
		return Assessment{}, errors.New("assessment agent unavailable")
	}
	assessment, err := fn(request, call)
	if err == nil && !preserveEmpty && len(assessment.EvidenceRefs) == 0 && len(request.Anchors) > 0 {
		assessment.EvidenceRefs = []EvidenceRef{evidenceFromAnchor(request.Anchors[0])}
	}
	return assessment, err
}

func (a *scriptedAgent) GenerateReport(_ context.Context, request GenerateReportRequest) (ReportDraft, error) {
	a.mu.Lock()
	a.reportCalls++
	call := a.reportCalls
	fn := a.reportFn
	a.mu.Unlock()
	if fn == nil {
		return ReportDraft{}, errors.New("report agent unavailable")
	}
	return fn(request, call)
}

func groundedAgent() *scriptedAgent {
	return &scriptedAgent{
		questionFn: func(request GenerateQuestionRequest, call int) (QuestionDraft, error) {
			return QuestionDraft{Text: fmt.Sprintf("grounded question %d", call), EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])}}, nil
		},
		assessFn: func(_ AssessAnswerRequest, _ int) (Assessment, error) {
			return Assessment{Correctness: 3, Depth: 3, Specificity: 3, Ownership: 3, Metrics: 3, Tradeoffs: 3}, nil
		},
		reportFn: func(request GenerateReportRequest, _ int) (ReportDraft, error) {
			return ReportDraft{Summary: "report", EvidenceRefs: []EvidenceRef{evidenceFromAnchor(request.Anchors[0])}}, nil
		},
	}
}

func forgedQuestion() QuestionDraft {
	return QuestionDraft{
		Text: "fabricated question",
		EvidenceRefs: []EvidenceRef{{
			SourceID: "fake", Kind: SourceResume, AnchorID: "fake:999", Locator: "segment:999", Quote: "fabricated evidence",
		}},
	}
}

func newTestService(agent Agent, store Store) *Service {
	return NewService(Dependencies{
		Agent: agent,
		Store: store,
		Clock: fixedClock{value: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)},
		IDs:   &sequenceIDs{},
	})
}

func startRequest(questionCount int, focus Focus, materials MaterialsInput) StartRequest {
	return StartRequest{
		Action:          ActionStart,
		ClientSessionID: "client-1",
		Model:           "test-model",
		Config: InterviewConfig{
			Focus: focus, Difficulty: DifficultyMedium, QuestionCount: questionCount,
			Language: "zh-CN", FeedbackMode: FeedbackImmediate,
		},
		Materials: materials,
	}
}

func standardMaterials() MaterialsInput {
	return MaterialsInput{
		JD:     &MaterialInput{Text: "高级 Go 工程师\n负责设计高并发支付平台\n要求理解 Go 并发、Redis 和 Kafka"},
		Resume: &MaterialInput{Text: "后端工程师\n项目 OfferPilot：我负责设计自适应面试 Agent\n将响应延迟从 500ms 降至 120ms\n负责消息队列削峰与故障恢复"},
	}
}

func answerRequest(started StartResponse, text string) AnswerRequest {
	return AnswerRequest{
		Action: ActionAnswer, InterviewID: started.InterviewID, QuestionID: started.Question.ID,
		ClientAnswerID: "answer-" + started.Question.ID,
		Answer:         AnswerPayload{Text: text, InputMode: InputModeText, DurationMS: 1000},
	}
}

type staticKnowledgeRetriever []KnowledgeDocument

func (r staticKnowledgeRetriever) Retrieve(context.Context, KnowledgeQuery) ([]KnowledgeDocument, error) {
	return append([]KnowledgeDocument(nil), r...), nil
}

type scriptedKnowledgeRetriever struct {
	mu         sync.Mutex
	calls      int
	queries    []KnowledgeQuery
	retrieveFn func(KnowledgeQuery, int) ([]KnowledgeDocument, error)
}

func (r *scriptedKnowledgeRetriever) Retrieve(_ context.Context, query KnowledgeQuery) ([]KnowledgeDocument, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.queries = append(r.queries, query)
	fn := r.retrieveFn
	r.mu.Unlock()
	if fn == nil {
		return nil, nil
	}
	return fn(query, call)
}

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

type incrementingClock struct {
	mu      sync.Mutex
	current time.Time
}

func (c *incrementingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	value := c.current
	c.current = c.current.Add(time.Second)
	return value
}

type sequenceIDs struct {
	mu   sync.Mutex
	next int
}

func (g *sequenceIDs) NewID(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return prefix + "-" + string(rune('a'+g.next-1))
}
