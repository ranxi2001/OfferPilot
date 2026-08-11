package harness

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"offerpilot/backend/internal/interview"
	"offerpilot/backend/internal/llm"
)

type recordingStructuredClient struct {
	mu                sync.Mutex
	messages          [][]llm.Message
	coverageSelection interview.CoverageSelection
}

func (client *recordingStructuredClient) ChatJSON(_ context.Context, messages []llm.Message, out any) error {
	client.mu.Lock()
	copyMessages := append([]llm.Message(nil), messages...)
	client.messages = append(client.messages, copyMessages)
	client.mu.Unlock()
	switch output := out.(type) {
	case *interview.QuestionDraft:
		*output = interview.QuestionDraft{Text: "question", EvidenceRefs: []interview.EvidenceRef{}}
	case *interview.Assessment:
		*output = interview.Assessment{
			Correctness: 3, Depth: 3, Specificity: 3, Ownership: 3, Metrics: 3, Tradeoffs: 3,
			FactualErrors: []string{}, Strengths: []string{}, Gaps: []string{},
			EvidenceRefs: []interview.EvidenceRef{}, ClaimChecks: []interview.ClaimCheck{},
		}
	case *interview.ReportDraft:
		*output = interview.ReportDraft{Summary: "report", Strengths: []string{}, Gaps: []string{}, EvidenceRefs: []interview.EvidenceRef{}}
	case *interview.CoverageSelection:
		*output = client.coverageSelection
	}
	return nil
}

func TestInterviewAgentRegistersTypedRolesAndPreservesRepairContext(t *testing.T) {
	t.Parallel()

	client := &recordingStructuredClient{}
	runtime, err := NewRuntime(client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewInterviewAgent(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(runtime.Agents()); got != 4 {
		t.Fatalf("registered agents=%d, want 4", got)
	}
	assessor, exists := runtime.Agent(AssessorAgentID)
	if !exists || !strings.Contains(assessor.SystemPrompt, "严禁按回答字数") || !strings.Contains(assessor.SystemPrompt, "not_in_material") {
		t.Fatalf("assessor prompt lacks semantic/claim constraints: %q", assessor.SystemPrompt)
	}

	evidence := interview.EvidenceRef{
		SourceID: "resume", Kind: interview.SourceResume, AnchorID: "resume:001",
		Locator: "segment:1", Quote: "主导 <Agent> 平台",
	}
	request := interview.AssessAnswerRequest{
		Question: interview.Question{ID: "q1", Text: "你具体做了什么？"},
		Answer:   interview.AnswerPayload{Text: "我负责架构设计", InputMode: interview.InputModeText},
		Anchors:  []interview.SourceAnchor{},
		History:  []interview.AnswerRecord{},
		Repair: &interview.RepairInstruction{
			Reason:          "引用不匹配 <必须修复>",
			AllowedEvidence: []interview.EvidenceRef{evidence},
		},
	}
	if _, err := agent.AssessAnswer(context.Background(), request); err != nil {
		t.Fatalf("AssessAnswer: %v", err)
	}
	client.mu.Lock()
	messages := append([]llm.Message(nil), client.messages[len(client.messages)-1]...)
	client.mu.Unlock()
	if len(messages) != 2 || messages[0].Content != assessor.SystemPrompt {
		t.Fatalf("assessment was not routed to the assessor role: %#v", messages)
	}
	if !strings.Contains(messages[1].Content, request.Repair.Reason) || !strings.Contains(messages[1].Content, evidence.Quote) {
		t.Fatalf("repair reason/evidence missing from context: %s", messages[1].Content)
	}
}

func TestInterviewAgentPlansCoverageFromSemanticSignalsAndCandidateIDs(t *testing.T) {
	t.Parallel()

	client := &recordingStructuredClient{coverageSelection: interview.CoverageSelection{
		CoveragePointID: "jd-concurrency",
		Reason:          "未覆盖的高优先级 JD 能力，并对应上一轮的并发边界缺口",
		Signals:         []string{"gap: 并发失效边界", "questionCount: 0", "priority: 90"},
	}}
	runtime, err := NewRuntime(client, Options{})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewInterviewAgent(runtime)
	if err != nil {
		t.Fatal(err)
	}
	planner, exists := runtime.Agent(PlannerAgentID)
	if !exists || !strings.Contains(planner.SystemPrompt, "禁止按候选列表顺序") ||
		!strings.Contains(planner.SystemPrompt, "claimChecks") || !strings.Contains(planner.SystemPrompt, "candidate.priority") {
		t.Fatalf("planner prompt lacks adaptive selection constraints: %q", planner.SystemPrompt)
	}

	request := interview.PlanCoverageRequest{
		Config:                 interview.InterviewConfig{Focus: interview.FocusMixed, Difficulty: interview.DifficultyHard, QuestionCount: 7},
		CurrentCoveragePointID: "project-offerpilot",
		PreviousQuestion: interview.Question{
			ID: "q2", Kind: interview.QuestionProject, CoveragePointID: "project-offerpilot",
		},
		PreviousAssessment: interview.Assessment{
			Correctness: 4, Depth: 3, Specificity: 3, Ownership: 4, Metrics: 3, Tradeoffs: 3,
			Gaps:        []string{"没有解释并发控制的失效边界"},
			ClaimChecks: []interview.ClaimCheck{{Claim: "吞吐提升十倍", Verdict: interview.ClaimUnverified, EvidenceRefs: []interview.EvidenceRef{}}},
		},
		Candidates: []interview.CoverageCandidate{
			{CoveragePointID: "project-offerpilot", Area: interview.FocusProjects, Label: "OfferPilot 项目", Priority: 80, QuestionCount: 2, LastAskedTurn: 2, EvidenceRefs: []interview.EvidenceRef{}},
			{CoveragePointID: "jd-concurrency", Area: interview.FocusKnowledge, Label: "Go 并发与失效边界", Priority: 90, QuestionCount: 0, LastAskedTurn: 0, EvidenceRefs: []interview.EvidenceRef{}},
		},
		QuestionKindCounts: map[interview.QuestionKind]int{interview.QuestionProject: 2, interview.QuestionKnowledge: 0},
		History:            []interview.AnswerRecord{},
		RemainingQuestions: 5,
	}
	selection, err := agent.PlanCoverage(context.Background(), request)
	if err != nil {
		t.Fatalf("PlanCoverage: %v", err)
	}
	if selection.CoveragePointID != "jd-concurrency" || len(selection.Signals) != 3 {
		t.Fatalf("selection = %#v", selection)
	}

	client.mu.Lock()
	messages := append([]llm.Message(nil), client.messages[len(client.messages)-1]...)
	client.mu.Unlock()
	if len(messages) != 2 || messages[0].Content != planner.SystemPrompt {
		t.Fatalf("coverage request was not routed to planner: %#v", messages)
	}
	for _, want := range []string{"没有解释并发控制的失效边界", "unverified", "jd-concurrency", `"questionKindCounts"`, `"priority":90`} {
		if !strings.Contains(messages[1].Content, want) {
			t.Fatalf("planner context is missing %q: %s", want, messages[1].Content)
		}
	}
}

func TestInterviewAgentRejectsInvalidCoverageSelections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selection interview.CoverageSelection
		request   interview.PlanCoverageRequest
		wantError string
	}{
		{
			name:      "outside candidate set",
			selection: interview.CoverageSelection{CoveragePointID: "invented-id", Reason: "invalid", Signals: []string{"invented"}},
			request:   interview.PlanCoverageRequest{Candidates: []interview.CoverageCandidate{{CoveragePointID: "allowed-id"}}},
			wantError: "unknown candidate",
		},
		{
			name:      "reselects current while alternative exists",
			selection: interview.CoverageSelection{CoveragePointID: "current", Reason: "repeat", Signals: []string{"asked: 2"}},
			request: interview.PlanCoverageRequest{
				CurrentCoveragePointID: "current",
				Candidates:             []interview.CoverageCandidate{{CoveragePointID: "current"}, {CoveragePointID: "alternative"}},
			},
			wantError: "reselected current",
		},
		{
			name:      "omits audit signals",
			selection: interview.CoverageSelection{CoveragePointID: "allowed-id", Reason: "otherwise valid", Signals: []string{}},
			request:   interview.PlanCoverageRequest{Candidates: []interview.CoverageCandidate{{CoveragePointID: "allowed-id"}}},
			wantError: "no selection signals",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &recordingStructuredClient{coverageSelection: test.selection}
			runtime, err := NewRuntime(client, Options{})
			if err != nil {
				t.Fatal(err)
			}
			agent, err := NewInterviewAgent(runtime)
			if err != nil {
				t.Fatal(err)
			}
			_, err = agent.PlanCoverage(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("PlanCoverage error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestInterviewAgentTimeoutDefaultsEnvironmentAndCompatibleConstructor(t *testing.T) {
	keys := []string{
		"OFFERPILOT_INTERVIEWER_TIMEOUT",
		"OFFERPILOT_ASSESSOR_TIMEOUT",
		"OFFERPILOT_REPORTER_TIMEOUT",
		"OFFERPILOT_PLANNER_TIMEOUT",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}
	defaults := InterviewAgentOptionsFromEnv()
	if defaults.InterviewerTimeout != 90*time.Second || defaults.AssessorTimeout != 180*time.Second ||
		defaults.ReporterTimeout != 90*time.Second || defaults.PlannerTimeout != 90*time.Second {
		t.Fatalf("default timeouts = %#v", defaults)
	}

	t.Setenv("OFFERPILOT_INTERVIEWER_TIMEOUT", "101s")
	t.Setenv("OFFERPILOT_ASSESSOR_TIMEOUT", "2m30s")
	t.Setenv("OFFERPILOT_REPORTER_TIMEOUT", "invalid")
	t.Setenv("OFFERPILOT_PLANNER_TIMEOUT", "95000")
	configured := InterviewAgentOptionsFromEnv()
	if configured.InterviewerTimeout != 101*time.Second || configured.AssessorTimeout != 150*time.Second ||
		configured.ReporterTimeout != 90*time.Second || configured.PlannerTimeout != 95*time.Second {
		t.Fatalf("configured timeouts = %#v", configured)
	}

	runtime, err := NewRuntime(&recordingStructuredClient{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// This compatibility constructor is intentionally kept as the primary API.
	if _, err := NewInterviewAgent(runtime); err != nil {
		t.Fatal(err)
	}
	wants := map[string]time.Duration{
		InterviewerAgentID: 101 * time.Second,
		AssessorAgentID:    150 * time.Second,
		ReporterAgentID:    90 * time.Second,
		PlannerAgentID:     95 * time.Second,
	}
	for agentID, want := range wants {
		registered, exists := runtime.Agent(agentID)
		if !exists || registered.Timeout != want {
			t.Fatalf("agent %q timeout = %s, exists=%v, want %s", agentID, registered.Timeout, exists, want)
		}
	}
}
