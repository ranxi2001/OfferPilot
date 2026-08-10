package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"offerpilot/backend/internal/interview"
)

type interviewStub struct {
	startRequest interview.StartRequest
	start        interview.StartResponse
	answer       interview.AnswerResponse
	report       interview.ReportResponse
	err          error
}

func (s *interviewStub) Start(_ context.Context, request interview.StartRequest) (interview.StartResponse, error) {
	s.startRequest = request
	return s.start, s.err
}

func (s *interviewStub) Answer(context.Context, interview.AnswerRequest) (interview.AnswerResponse, error) {
	return s.answer, s.err
}

func (s *interviewStub) Report(context.Context, interview.ReportRequest) (interview.ReportResponse, error) {
	return s.report, s.err
}

func TestHealthExposesRuntimeReadinessWithoutAuthentication(t *testing.T) {
	server := newTestServer(t, Config{
		APIKey: "secret", RequireAuth: true, KnowledgeEntries: 403, ModelConfigured: true,
	}, &interviewStub{})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["service"] != "offerpilot-go" || payload["knowledgeEntries"] != float64(403) || payload["harness"] != "ready" || payload["readiness"] != "ready" || payload["live"] != true || payload["ready"] != true {
		t.Fatalf("unexpected health payload: %#v", payload)
	}
	readiness := httptest.NewRecorder()
	server.Handler().ServeHTTP(readiness, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if readiness.Code != http.StatusOK {
		t.Fatalf("readiness status = %d, body = %s", readiness.Code, readiness.Body.String())
	}
}

func TestHealthSeparatesLivenessFromModelReadiness(t *testing.T) {
	server := newTestServer(t, Config{ModelConfigured: false}, &interviewStub{})

	live := httptest.NewRecorder()
	server.Handler().ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, body = %s", live.Code, live.Body.String())
	}
	var livePayload map[string]any
	if err := json.Unmarshal(live.Body.Bytes(), &livePayload); err != nil {
		t.Fatal(err)
	}
	if livePayload["status"] != "live" || livePayload["live"] != true || livePayload["ready"] != false || livePayload["harness"] != "not_ready" {
		t.Fatalf("unexpected liveness payload: %#v", livePayload)
	}

	ready := httptest.NewRecorder()
	server.Handler().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, body = %s", ready.Code, ready.Body.String())
	}
	var readyPayload map[string]any
	if err := json.Unmarshal(ready.Body.Bytes(), &readyPayload); err != nil {
		t.Fatal(err)
	}
	if readyPayload["status"] != "not_ready" || readyPayload["live"] != true || readyPayload["ready"] != false || readyPayload["readiness"] != "not_ready" {
		t.Fatalf("unexpected readiness payload: %#v", readyPayload)
	}

	compatibility := httptest.NewRecorder()
	server.Handler().ServeHTTP(compatibility, httptest.NewRequest(http.MethodGet, "/health", nil))
	if compatibility.Code != http.StatusOK {
		t.Fatalf("compatibility health status = %d, body = %s", compatibility.Code, compatibility.Body.String())
	}
	var compatibilityPayload map[string]any
	if err := json.Unmarshal(compatibility.Body.Bytes(), &compatibilityPayload); err != nil {
		t.Fatal(err)
	}
	if compatibilityPayload["status"] != "ok" || compatibilityPayload["ready"] != false || compatibilityPayload["harness"] != "not_ready" {
		t.Fatalf("unexpected compatibility health payload: %#v", compatibilityPayload)
	}
}

func TestProtectedRouteRequiresBearerAndRejectsUnknownOrigin(t *testing.T) {
	server := newTestServer(t, Config{
		APIKey: "secret", RequireAuth: true, AllowedOrigins: []string{"http://localhost:3000"},
	}, &interviewStub{})

	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/session", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Origin", "https://attacker.example")
	forbidden := httptest.NewRecorder()
	server.Handler().ServeHTTP(forbidden, request)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d", forbidden.Code)
	}
}

func TestInterviewStartMapsWebContractToDomainAndBack(t *testing.T) {
	anchor := interview.EvidenceRef{
		SourceID: "resume", Kind: interview.SourceResume, AnchorID: "resume:001", Locator: "segment:1", Quote: "星舟 Agent 平台将路由延迟从 1200ms 降到 650ms",
	}
	profile := interview.Profile{
		JD:       interview.JDProfile{Title: "Agent 平台工程师"},
		Resume:   interview.ResumeProfile{Projects: []interview.ProfilePoint{{ID: "p1", Label: "星舟 Agent 平台", EvidenceRefs: []interview.EvidenceRef{anchor}}}},
		Coverage: []interview.CoveragePoint{{ID: "project-1", Area: interview.FocusProjects, Label: "星舟 Agent 平台", EvidenceRefs: []interview.EvidenceRef{anchor}}},
	}
	stub := &interviewStub{start: interview.StartResponse{
		InterviewID: "interview-1", State: interview.StateAwaitingAnswer, Profile: profile,
		Question: interview.Question{
			ID: "question-1", Text: "你在星舟 Agent 平台中如何设计 provider router？", Kind: interview.QuestionProject,
			Difficulty: interview.DifficultyHard, CoveragePointID: "project-1", EvidenceRefs: []interview.EvidenceRef{anchor},
			Adaptation: interview.QuestionAdaptation{Trigger: interview.PolicyInitial},
		},
		Progress: interview.Progress{Answered: 0, Total: 5, Current: 1},
	}}
	server := newTestServer(t, Config{ModelConfigured: true}, stub)
	body := `{
		"action":"start",
		"config":{"focus":"project","difficulty":"hard","questionCount":5,"language":"zh-CN","feedbackMode":"after_each"},
		"materials":{"resume":{"name":"resume.md","text":"星舟 Agent 平台将路由延迟从 1200ms 降到 650ms","source":"upload"}}
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/interview", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if stub.startRequest.Config.Focus != interview.FocusProjects || stub.startRequest.Config.FeedbackMode != interview.FeedbackImmediate {
		t.Fatalf("request was not normalized: %#v", stub.startRequest.Config)
	}
	if stub.startRequest.ClientSessionID == "" {
		t.Fatal("missing generated client session id")
	}
	var payload struct {
		State    string `json:"state"`
		Question struct {
			Focus        string           `json:"focus"`
			Kind         string           `json:"kind"`
			Topic        string           `json:"topic"`
			EvidenceRefs []webEvidenceRef `json:"evidenceRefs"`
		} `json:"question"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.State != "questioning" || payload.Question.Focus != "project" || payload.Question.Kind != "opening" || payload.Question.Topic != "星舟 Agent 平台" {
		t.Fatalf("unexpected response projection: %#v", payload)
	}
	if len(payload.Question.EvidenceRefs) != 1 || !strings.Contains(payload.Question.EvidenceRefs[0].Excerpt, "1200ms") {
		t.Fatalf("question is not evidence-grounded: %#v", payload.Question.EvidenceRefs)
	}
}

func TestInterviewAnswerPreservesUnverifiedClaimAndAdaptiveQuestion(t *testing.T) {
	anchor := interview.EvidenceRef{SourceID: "resume", Kind: interview.SourceResume, AnchorID: "resume:001", Locator: "segment:1", Quote: "星舟 Agent 平台"}
	stub := &interviewStub{answer: interview.AnswerResponse{
		InterviewID: "interview-1", State: interview.StateAwaitingAnswer,
		Feedback: interview.AnswerFeedback{
			Summary: "延迟指标缺少测量口径。",
			Assessment: interview.Assessment{
				Correctness: 3, Depth: 3, Specificity: 2, Ownership: 3, Metrics: 2, Tradeoffs: 2,
				Strengths: []string{"说明了路由目标"}, Gaps: []string{"补充 P95 采样窗口"}, EvidenceRefs: []interview.EvidenceRef{anchor},
				ClaimChecks: []interview.ClaimCheck{{Claim: "延迟降低到 650ms", Verdict: interview.ClaimUnverified, EvidenceRefs: []interview.EvidenceRef{}}},
			},
		},
		NextQuestion: &interview.Question{
			ID: "question-2", Text: "650ms 是平均值还是 P95？", Kind: interview.QuestionFollowUp, Difficulty: interview.DifficultyHard,
			EvidenceRefs: []interview.EvidenceRef{anchor}, Adaptation: interview.QuestionAdaptation{
				Trigger: interview.PolicyFollowUp, Reason: "answer is too general", BasedOnQuestionID: "question-1", FollowUpAxis: "metrics", Depth: 1,
			},
		},
		Progress: interview.Progress{Answered: 1, Total: 5, Current: 2, FollowUpDepth: 1},
	}}
	server := newTestServer(t, Config{ModelConfigured: true}, stub)
	body := `{"action":"answer","interviewId":"interview-1","questionId":"question-1","answer":{"text":"我把延迟降到了650ms","inputMode":"text"}}`
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/interview", strings.NewReader(body)))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Feedback struct {
			ClaimChecks []webClaimCheck `json:"claimChecks"`
		} `json:"feedback"`
		NextQuestion webQuestion `json:"nextQuestion"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Feedback.ClaimChecks) != 1 || payload.Feedback.ClaimChecks[0].Verdict != "unverified" {
		t.Fatalf("claim certainty was changed: %#v", payload.Feedback.ClaimChecks)
	}
	if payload.NextQuestion.Kind != "follow_up" || payload.NextQuestion.Adaptation == nil || payload.NextQuestion.Adaptation.Strategy != "deepen" {
		t.Fatalf("adaptive question projection is wrong: %#v", payload.NextQuestion)
	}
}

func TestInterviewAnswerScoreUsesQuestionFocusInsteadOfAssessmentEvidenceKind(t *testing.T) {
	resumeRef := interview.EvidenceRef{
		SourceID: "resume", Kind: interview.SourceResume, AnchorID: "resume:001", Locator: "segment:1", Quote: "候选人简历项目",
	}
	stub := &interviewStub{answer: interview.AnswerResponse{
		InterviewID: "interview-1",
		State:       interview.StateCompleted,
		Feedback: interview.AnswerFeedback{
			Focus:   interview.FocusKnowledge,
			Summary: "知识题回答完整。",
			Assessment: interview.Assessment{
				Correctness: 5, Depth: 5, Specificity: 5, Ownership: 1, Metrics: 1, Tradeoffs: 5,
				EvidenceRefs: []interview.EvidenceRef{resumeRef},
			},
		},
		Progress:    interview.Progress{Answered: 1, Total: 1},
		ReportReady: true,
	}}
	server := newTestServer(t, Config{ModelConfigured: true}, stub)
	request := httptest.NewRequest(http.MethodPost, "/api/interview", strings.NewReader(
		`{"action":"answer","interviewId":"interview-1","questionId":"question-1","answer":{"text":"回答","inputMode":"text"}}`,
	))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Feedback struct {
			Score            int    `json:"score"`
			KnowledgeVerdict string `json:"knowledgeVerdict"`
		} `json:"feedback"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Feedback.Score != 100 || payload.Feedback.KnowledgeVerdict != "correct" {
		t.Fatalf("feedback used evidence-selected project rubric: %+v", payload.Feedback)
	}
}

func TestInterviewBodyLimitReturns413(t *testing.T) {
	server := newTestServer(t, Config{MaxInterviewBytes: 32, ModelConfigured: true}, &interviewStub{})
	request := httptest.NewRequest(http.MethodPost, "/api/interview", strings.NewReader(`{"action":"start","materials":{"resume":{"text":"this body is deliberately too long"}}}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestInterviewUnavailableMapsToRetryable503(t *testing.T) {
	server := newTestServer(t, Config{ModelConfigured: true}, &interviewStub{err: &interview.DomainError{
		Code: interview.CodeUnavailable, Message: "interview assessment is temporarily unavailable",
	}})
	request := httptest.NewRequest(http.MethodPost, "/api/interview", strings.NewReader(`{"action":"answer","interviewId":"interview-1","questionId":"question-1","answer":{"text":"answer","inputMode":"text"}}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != string(interview.CodeUnavailable) || !payload.Error.Retryable {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestInterviewWithoutModelReturnsRetryable503(t *testing.T) {
	stub := &interviewStub{}
	server := newTestServer(t, Config{ModelConfigured: false}, stub)
	request := httptest.NewRequest(http.MethodPost, "/api/interview", strings.NewReader(`{"action":"start"}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != string(interview.CodeUnavailable) || !payload.Error.Retryable {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
	if stub.startRequest.Action != "" {
		t.Fatalf("unconfigured model reached interview service: %#v", stub.startRequest)
	}
}

func newTestServer(t *testing.T, config Config, service InterviewService) *Server {
	t.Helper()
	server, err := New(config, Dependencies{Interview: service})
	if err != nil {
		t.Fatal(err)
	}
	return server
}
