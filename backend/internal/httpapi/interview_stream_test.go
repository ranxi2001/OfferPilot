package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"offerpilot/backend/internal/executiontrace"
	"offerpilot/backend/internal/interview"
)

func TestInterviewStreamReturnsTracesAndSameMappedResult(t *testing.T) {
	anchor := interview.EvidenceRef{
		SourceID: "resume", Kind: interview.SourceResume, AnchorID: "resume:001",
		Locator: "segment:1", Quote: "Built a bounded agent runtime",
	}
	stub := &interviewStub{start: interview.StartResponse{
		InterviewID: "interview-1",
		State:       interview.StateAwaitingAnswer,
		Profile: interview.Profile{
			JD: interview.JDProfile{Title: "Agent Engineer"},
			Resume: interview.ResumeProfile{Projects: []interview.ProfilePoint{{
				ID: "project-1", Label: "OfferPilot", EvidenceRefs: []interview.EvidenceRef{anchor},
			}}},
			Coverage: []interview.CoveragePoint{{
				ID: "coverage-1", Area: interview.FocusProjects, Label: "OfferPilot",
				EvidenceRefs: []interview.EvidenceRef{anchor},
			}},
		},
		Question: interview.Question{
			ID: "question-1", Text: "How did you bound agent execution?",
			Kind: interview.QuestionProject, Difficulty: interview.DifficultyHard,
			CoveragePointID: "coverage-1", EvidenceRefs: []interview.EvidenceRef{anchor},
			Adaptation: interview.QuestionAdaptation{Trigger: interview.PolicyInitial},
		},
		Progress: interview.Progress{Total: 5, Current: 1},
	}}
	server := newTestServer(t, Config{ModelConfigured: true}, stub)
	body := `{"action":"start","config":{"focus":"project","difficulty":"hard","questionCount":5,"feedbackMode":"after_each"},"materials":{"resume":{"text":"private resume input"}}}`

	ordinary := httptest.NewRecorder()
	server.Handler().ServeHTTP(ordinary, httptest.NewRequest(http.MethodPost, "/api/interview", strings.NewReader(body)))
	if ordinary.Code != http.StatusOK {
		t.Fatalf("ordinary status = %d, body = %s", ordinary.Code, ordinary.Body.String())
	}

	streamed := httptest.NewRecorder()
	server.Handler().ServeHTTP(streamed, httptest.NewRequest(http.MethodPost, "/api/interview/stream", strings.NewReader(body)))
	if streamed.Code != http.StatusOK {
		t.Fatalf("stream status = %d, body = %s", streamed.Code, streamed.Body.String())
	}
	if got := streamed.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/x-ndjson") {
		t.Fatalf("Content-Type = %q", got)
	}

	lines := decodeInterviewStream(t, streamed.Body.String())
	traceCount := 0
	var final interviewStreamEnvelope
	for _, line := range lines {
		switch line.Type {
		case "trace":
			traceCount++
		case "result":
			final = line
		}
	}
	if traceCount < 3 {
		t.Fatalf("trace lines = %d, want at least queued/running/completed; body = %s", traceCount, streamed.Body.String())
	}
	if final.Type != "result" || final.Status != ordinary.Code {
		t.Fatalf("final envelope = %#v", final)
	}
	assertJSONEqual(t, ordinary.Body.Bytes(), final.Data)
}

func TestInterviewStreamPreservesUnavailableErrorBody(t *testing.T) {
	stub := &interviewStub{err: &interview.DomainError{
		Code: interview.CodeUnavailable, Message: "interview assessment is temporarily unavailable",
	}}
	server := newTestServer(t, Config{ModelConfigured: true}, stub)
	body := `{"action":"answer","interviewId":"interview-1","questionId":"question-1","answer":{"text":"answer","inputMode":"text"}}`

	ordinary := httptest.NewRecorder()
	server.Handler().ServeHTTP(ordinary, httptest.NewRequest(http.MethodPost, "/api/interview", strings.NewReader(body)))
	streamed := httptest.NewRecorder()
	server.Handler().ServeHTTP(streamed, httptest.NewRequest(http.MethodPost, "/api/interview/stream", strings.NewReader(body)))

	lines := decodeInterviewStream(t, streamed.Body.String())
	final := lines[len(lines)-1]
	if ordinary.Code != http.StatusServiceUnavailable || final.Type != "result" || final.Status != http.StatusServiceUnavailable {
		t.Fatalf("ordinary=%d final=%#v", ordinary.Code, final)
	}
	assertJSONEqual(t, ordinary.Body.Bytes(), final.Data)
}

type contextTracingInterviewStub struct {
	interviewStub
}

func (s *contextTracingInterviewStub) Start(ctx context.Context, request interview.StartRequest) (interview.StartResponse, error) {
	executiontrace.Emit(ctx, executiontrace.Event{
		ID: "knowledge-stage", Stage: "knowledge", Label: "Prepare knowledge references",
		Status: executiontrace.StatusCompleted, At: time.Now().UTC(), Detail: "documents=2",
	})
	return s.interviewStub.Start(ctx, request)
}

func TestInterviewStreamTraceExcludesCandidateAndProviderContent(t *testing.T) {
	stub := &contextTracingInterviewStub{interviewStub: interviewStub{start: interview.StartResponse{
		InterviewID: "interview-1", State: interview.StateAwaitingAnswer,
		Profile: interview.Profile{Coverage: []interview.CoveragePoint{{ID: "coverage-1", Label: "safe"}}},
		Question: interview.Question{
			ID: "question-1", Text: "safe question", Kind: interview.QuestionKnowledge,
			Difficulty: interview.DifficultyMedium, CoveragePointID: "coverage-1",
		},
		Progress: interview.Progress{Total: 1, Current: 1},
	}}}
	server := newTestServer(t, Config{ModelConfigured: true}, stub)
	secrets := []string{
		"SECRET_JD_REQUIREMENT", "SECRET_RESUME_PROJECT", "SECRET_KNOWLEDGE_REFERENCE",
		"SECRET_CANDIDATE_ANSWER", "SECRET_SYSTEM_PROMPT", "SECRET_PROVIDER_BODY",
	}
	body := `{"action":"start","materials":{"jd":{"text":"SECRET_JD_REQUIREMENT SECRET_KNOWLEDGE_REFERENCE"},"resume":{"text":"SECRET_RESUME_PROJECT SECRET_CANDIDATE_ANSWER SECRET_SYSTEM_PROMPT SECRET_PROVIDER_BODY"}}}`
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/interview/stream", strings.NewReader(body)))

	for _, line := range decodeInterviewStream(t, response.Body.String()) {
		if line.Type != "trace" {
			continue
		}
		encoded, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range secrets {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("trace leaked %q: %s", secret, encoded)
			}
		}
	}
}

type detachedRunInterviewStub struct {
	started   chan struct{}
	release   chan struct{}
	completed chan error
	calls     atomic.Int32
}

func newDetachedRunInterviewStub() *detachedRunInterviewStub {
	return &detachedRunInterviewStub{
		started: make(chan struct{}), release: make(chan struct{}), completed: make(chan error, 1),
	}
}

func (s *detachedRunInterviewStub) Start(ctx context.Context, _ interview.StartRequest) (interview.StartResponse, error) {
	s.calls.Add(1)
	close(s.started)
	<-s.release
	for index := 0; index < 256; index++ {
		executiontrace.Emit(ctx, executiontrace.Event{
			ID: "after-disconnect", Stage: "test", Label: "Background progress",
			Status: executiontrace.StatusRunning, At: time.Now().UTC(),
		})
	}
	s.completed <- ctx.Err()
	return interview.StartResponse{}, nil
}

func (s *detachedRunInterviewStub) Answer(context.Context, interview.AnswerRequest) (interview.AnswerResponse, error) {
	return interview.AnswerResponse{}, nil
}

func (s *detachedRunInterviewStub) Report(context.Context, interview.ReportRequest) (interview.ReportResponse, error) {
	return interview.ReportResponse{}, nil
}

func TestInterviewStreamDisconnectDoesNotCancelRun(t *testing.T) {
	stub := newDetachedRunInterviewStub()
	server := newTestServer(t, Config{ModelConfigured: true, InterviewRunTimeout: time.Second}, stub)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/interview/stream", strings.NewReader(`{"action":"start"}`)).WithContext(requestCtx)
	handlerDone := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(httptest.NewRecorder(), request)
		close(handlerDone)
	}()

	select {
	case <-stub.started:
	case <-time.After(time.Second):
		t.Fatal("interview service did not start")
	}
	cancelRequest()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("stream handler did not return after disconnect")
	}
	close(stub.release)

	select {
	case runErr := <-stub.completed:
		if runErr != nil {
			t.Fatalf("background run inherited request cancellation: %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("background run blocked after the stream disconnected")
	}
	if calls := stub.calls.Load(); calls != 1 {
		t.Fatalf("interview service calls = %d, want 1", calls)
	}
}

type timeoutInterviewStub struct {
	completed chan error
	calls     atomic.Int32
}

func (s *timeoutInterviewStub) Start(ctx context.Context, _ interview.StartRequest) (interview.StartResponse, error) {
	s.calls.Add(1)
	<-ctx.Done()
	s.completed <- ctx.Err()
	return interview.StartResponse{}, ctx.Err()
}

func (s *timeoutInterviewStub) Answer(context.Context, interview.AnswerRequest) (interview.AnswerResponse, error) {
	return interview.AnswerResponse{}, nil
}

func (s *timeoutInterviewStub) Report(context.Context, interview.ReportRequest) (interview.ReportResponse, error) {
	return interview.ReportResponse{}, nil
}

func TestInterviewStreamRunTimeoutCancelsService(t *testing.T) {
	stub := &timeoutInterviewStub{completed: make(chan error, 1)}
	server := newTestServer(t, Config{ModelConfigured: true, InterviewRunTimeout: 25 * time.Millisecond}, stub)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(
		http.MethodPost, "/api/interview/stream", strings.NewReader(`{"action":"start"}`),
	))

	select {
	case runErr := <-stub.completed:
		if !errors.Is(runErr, context.DeadlineExceeded) {
			t.Fatalf("service context error = %v, want deadline exceeded", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("interview service did not observe its run timeout")
	}
	if calls := stub.calls.Load(); calls != 1 {
		t.Fatalf("interview service calls = %d, want 1", calls)
	}
	lines := decodeInterviewStream(t, response.Body.String())
	if final := lines[len(lines)-1]; final.Type != "result" || final.Status != http.StatusInternalServerError {
		t.Fatalf("final envelope = %#v", final)
	}
}

func decodeInterviewStream(t *testing.T, body string) []interviewStreamEnvelope {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 1024), 4<<20)
	lines := make([]interviewStreamEnvelope, 0)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var line interviewStreamEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("decode stream line %q: %v", scanner.Text(), err)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Fatal("stream returned no NDJSON lines")
	}
	return lines
}

func assertJSONEqual(t *testing.T, left, right []byte) {
	t.Helper()
	var leftValue any
	var rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		t.Fatalf("decode left JSON: %v", err)
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		t.Fatalf("decode right JSON: %v", err)
	}
	if !reflect.DeepEqual(leftValue, rightValue) {
		t.Fatalf("JSON differs:\nleft:  %s\nright: %s", left, right)
	}
}
