package interview

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestAnswerRequiresClientAnswerID(t *testing.T) {
	request := AnswerRequest{
		Action: ActionAnswer, InterviewID: "interview-1", QuestionID: "question-1",
		Answer: AnswerPayload{Text: "answer", InputMode: InputModeText},
	}
	err := validateAnswer(request)
	if !IsCode(err, CodeValidation) {
		t.Fatalf("validateAnswer() error = %v, want validation", err)
	}
	var domainErr *DomainError
	if !errors.As(err, &domainErr) || domainErr.Field != "clientAnswerId" {
		t.Fatalf("validateAnswer() field = %+v, want clientAnswerId", domainErr)
	}
}

func TestSQLiteAnswerReplayAndConflicts(t *testing.T) {
	store := openTestSQLiteStore(t)
	agent := groundedAgent()
	service := newTestService(agent, store)
	materials := standardMaterials()
	materials.JD.Text += "\nSECRET_JD_IDEMPOTENCY"
	materials.Resume.Text += "\nSECRET_RESUME_IDEMPOTENCY"
	started, err := service.Start(context.Background(), startRequest(2, FocusMixed, materials))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	request := answerRequest(started, "SECRET_ANSWER_IDEMPOTENCY")
	request.ClientAnswerID = "client-answer-stable"

	first, err := service.Answer(context.Background(), request)
	if err != nil {
		t.Fatalf("first Answer() error = %v", err)
	}
	replayed, err := service.Answer(context.Background(), request)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("replayed Answer() = %+v, %v; want %+v", replayed, err, first)
	}

	changedPayload := request
	changedPayload.Answer.Text = "different payload"
	if _, err = service.Answer(context.Background(), changedPayload); !errors.Is(err, ErrIdempotencyConflict) || !IsCode(err, CodeConflict) {
		t.Fatalf("same key different payload error = %v, want ErrIdempotencyConflict", err)
	}
	changedKey := request
	changedKey.ClientAnswerID = "client-answer-other"
	if _, err = service.Answer(context.Background(), changedKey); !errors.Is(err, ErrAnswerAlreadyCommitted) || !IsCode(err, CodeConflict) {
		t.Fatalf("same question different key error = %v, want ErrAnswerAlreadyCommitted", err)
	}
	if agent.assessCalls != 1 || agent.questionCalls != 2 {
		t.Fatalf("agent calls = assess %d question %d, want 1/2", agent.assessCalls, agent.questionCalls)
	}
	loaded, err := store.Load(context.Background(), started.InterviewID)
	if err != nil || len(loaded.Answers) != 1 || loaded.Version != 2 {
		t.Fatalf("stored session = answers %d version %d error %v, want 1/2", len(loaded.Answers), loaded.Version, err)
	}
	if count := sqliteRowCount(t, store, "interview_commands"); count != 1 {
		t.Fatalf("command rows = %d, want 1", count)
	}

	events, err := store.ListSessionEvents(context.Background(), started.InterviewID, 0, 20)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != 2 || events[0].Type != "answer.started" || events[1].Type != "answer.committed" {
		t.Fatalf("answer events = %+v, want started then committed", events)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"SECRET_ANSWER_IDEMPOTENCY", "SECRET_JD_IDEMPOTENCY", "SECRET_RESUME_IDEMPOTENCY",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("answer events leaked %q: %s", secret, encoded)
		}
	}
}

func TestSQLiteConcurrentAnswerReplayExecutesOnce(t *testing.T) {
	const callers = 20
	store := openTestSQLiteStore(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	agent := groundedAgent()
	agent.assessFn = func(_ AssessAnswerRequest, _ int) (Assessment, error) {
		close(entered)
		<-release
		return Assessment{Correctness: 4, Depth: 4, Specificity: 4, Ownership: 4, Metrics: 4, Tradeoffs: 4}, nil
	}
	service := newTestService(agent, store)
	started, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	request := answerRequest(started, "concurrent stable answer")
	request.ClientAnswerID = "client-answer-concurrent"
	type result struct {
		response AnswerResponse
		err      error
	}
	results := make(chan result, callers)
	for range callers {
		go func() {
			response, answerErr := service.Answer(context.Background(), request)
			results <- result{response: response, err: answerErr}
		}()
	}
	<-entered
	close(release)
	var first AnswerResponse
	for index := range callers {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent Answer(%d) error = %v", index, result.err)
		}
		if index == 0 {
			first = result.response
		} else if !reflect.DeepEqual(result.response, first) {
			t.Fatalf("concurrent Answer(%d) = %+v, want %+v", index, result.response, first)
		}
	}
	if agent.assessCalls != 1 || agent.questionCalls != 2 {
		t.Fatalf("agent calls = assess %d question %d, want 1/2", agent.assessCalls, agent.questionCalls)
	}
	loaded, err := store.Load(context.Background(), started.InterviewID)
	if err != nil || len(loaded.Answers) != 1 || loaded.Version != 2 {
		t.Fatalf("stored session = answers %d version %d error %v, want 1/2", len(loaded.Answers), loaded.Version, err)
	}
	if count := sqliteRowCount(t, store, "interview_commands"); count != 1 {
		t.Fatalf("command rows = %d, want 1", count)
	}
}

func TestSQLiteFailedAnswerRetriesSameCommandWithoutPartialCommit(t *testing.T) {
	store := openTestSQLiteStore(t)
	cause := errors.New("SECRET_PROVIDER_FAILURE")
	agent := groundedAgent()
	agent.assessFn = func(_ AssessAnswerRequest, call int) (Assessment, error) {
		if call == 1 {
			return Assessment{}, cause
		}
		return Assessment{Correctness: 3, Depth: 3, Specificity: 3, Ownership: 3, Metrics: 3, Tradeoffs: 3}, nil
	}
	service := newTestService(agent, store)
	started, err := service.Start(context.Background(), startRequest(2, FocusMixed, standardMaterials()))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	request := answerRequest(started, "answer survives retry")
	request.ClientAnswerID = "client-answer-retry"
	if _, err = service.Answer(context.Background(), request); !IsCode(err, CodeUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("first Answer() error = %v, want provider failure", err)
	}
	loaded, err := store.Load(context.Background(), started.InterviewID)
	if err != nil || len(loaded.Answers) != 0 || loaded.Version != 1 {
		t.Fatalf("failed answer partially committed: answers %d version %d error %v", len(loaded.Answers), loaded.Version, err)
	}

	second, err := service.Answer(context.Background(), request)
	if err != nil {
		t.Fatalf("retry Answer() error = %v", err)
	}
	replayed, err := service.Answer(context.Background(), request)
	if err != nil || !reflect.DeepEqual(replayed, second) {
		t.Fatalf("post-retry replay = %+v, %v; want %+v", replayed, err, second)
	}
	if agent.assessCalls != 2 || agent.questionCalls != 2 {
		t.Fatalf("agent calls = assess %d question %d, want 2/2", agent.assessCalls, agent.questionCalls)
	}
	if count := sqliteRowCount(t, store, "interview_commands"); count != 1 {
		t.Fatalf("command rows = %d, want 1", count)
	}
	var status CommandStatus
	if err = store.db.QueryRow(`SELECT status FROM interview_commands LIMIT 1`).Scan(&status); err != nil || status != CommandSucceeded {
		t.Fatalf("command status = %q error %v, want succeeded", status, err)
	}
	events, err := store.ListSessionEvents(context.Background(), started.InterviewID, 0, 20)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	wantTypes := []string{"answer.started", "answer.failed", "answer.started", "answer.committed"}
	if len(events) != len(wantTypes) {
		t.Fatalf("event count = %d, want %d: %+v", len(events), len(wantTypes), events)
	}
	for index, want := range wantTypes {
		if events[index].Type != want {
			t.Fatalf("event %d type = %q, want %q", index, events[index].Type, want)
		}
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), cause.Error()) || strings.Contains(string(encoded), request.Answer.Text) {
		t.Fatalf("failure events leaked private content: %s", encoded)
	}
}
