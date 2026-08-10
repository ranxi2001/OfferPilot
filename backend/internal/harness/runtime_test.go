package harness

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"offerpilot/backend/internal/executiontrace"
	"offerpilot/backend/internal/llm"
)

type trackingClient struct {
	mu      sync.Mutex
	active  int
	maximum int
}

func (client *trackingClient) ChatJSON(_ context.Context, _ []llm.Message, out any) error {
	client.mu.Lock()
	client.active++
	if client.active > client.maximum {
		client.maximum = client.active
	}
	client.mu.Unlock()
	time.Sleep(15 * time.Millisecond)
	client.mu.Lock()
	client.active--
	client.mu.Unlock()
	return nil
}

func TestRuntimeBoundsSubAgentConcurrencyAndTraces(t *testing.T) {
	t.Parallel()

	client := &trackingClient{}
	runtime, err := NewRuntime(client, Options{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(Agent{ID: "interviewer", SystemPrompt: "Probe the candidate."}); err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	for call := 0; call < 3; call++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			var output struct{}
			if err := runtime.CallJSON(context.Background(), "interviewer", "ask", "resume", &output); err != nil {
				t.Errorf("CallJSON: %v", err)
			}
		}()
	}
	wait.Wait()
	client.mu.Lock()
	maximum := client.maximum
	client.mu.Unlock()
	if maximum != 1 {
		t.Fatalf("maximum concurrency=%d, want 1", maximum)
	}
	traces := runtime.Traces("")
	if len(traces) != 9 {
		t.Fatalf("trace count=%d, want 9", len(traces))
	}
}

type failingStructuredClient struct {
	err error
}

func (client *failingStructuredClient) ChatJSON(context.Context, []llm.Message, any) error {
	return client.err
}

func TestRuntimeEmitsSanitizedRequestEventsAndRetainsGlobalTrace(t *testing.T) {
	providerErr := errors.New("SECRET_PROVIDER_BODY contains prompt and resume")
	globalEvents := make([]TraceEvent, 0)
	runtime, err := NewRuntime(&failingStructuredClient{err: providerErr}, Options{
		TraceSink: func(event TraceEvent) { globalEvents = append(globalEvents, event) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(Agent{ID: AssessorAgentID, SystemPrompt: "SECRET_SYSTEM_PROMPT"}); err != nil {
		t.Fatal(err)
	}

	requestEvents := make([]executiontrace.Event, 0)
	ctx := executiontrace.WithSink(context.Background(), func(event executiontrace.Event) {
		requestEvents = append(requestEvents, event)
	})
	var output struct{}
	callErr := runtime.CallJSON(ctx, AssessorAgentID, "assess SECRET_CANDIDATE_ANSWER", "SECRET_JD_RESUME_EVIDENCE", &output)
	if callErr == nil {
		t.Fatal("CallJSON() error = nil")
	}

	if len(requestEvents) != 3 {
		t.Fatalf("request event count = %d, want 3", len(requestEvents))
	}
	wantStatuses := []executiontrace.Status{
		executiontrace.StatusQueued, executiontrace.StatusRunning, executiontrace.StatusFailed,
	}
	for index, event := range requestEvents {
		if event.Status != wantStatuses[index] || event.Agent != AssessorAgentID || event.ID == "" {
			t.Fatalf("requestEvents[%d] = %#v", index, event)
		}
	}
	encoded, err := json.Marshal(requestEvents)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET_PROVIDER_BODY", "SECRET_SYSTEM_PROMPT", "SECRET_CANDIDATE_ANSWER", "SECRET_JD_RESUME_EVIDENCE"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("request trace leaked %q: %s", secret, encoded)
		}
	}

	retained := runtime.Traces("")
	if len(retained) != 3 || len(globalEvents) != 3 {
		t.Fatalf("retained=%d globalSink=%d, want 3 each", len(retained), len(globalEvents))
	}
	if !strings.Contains(retained[len(retained)-1].Error, "SECRET_PROVIDER_BODY") {
		t.Fatalf("global diagnostic trace lost provider error: %#v", retained[len(retained)-1])
	}
}
