package harness

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestFunctionToolRequiresAgentPermissionAndEmitsSanitizedTrace(t *testing.T) {
	runtime, err := NewRuntime(&trackingClient{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.RegisterTool(FunctionTool{
		Name: "fetch_web_content",
		Risk: ToolRiskExternalRead,
		InputSchema: map[string]any{
			"type": "object",
		},
		Handler: func(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
			if !strings.Contains(string(raw), "SECRET_URL") {
				t.Fatalf("tool input = %s", raw)
			}
			return json.RawMessage(`{"text":"SECRET_RESULT"}`), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(Agent{
		ID: "web_crawler", SystemPrompt: "Fetch only public pages.", Tools: []string{"fetch_web_content"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Register(Agent{ID: "reporter", SystemPrompt: "Report only."}); err != nil {
		t.Fatal(err)
	}

	var output struct {
		Text string `json:"text"`
	}
	traceID, err := runtime.CallToolJSONTrace(context.Background(), "web_crawler", "fetch_web_content", map[string]string{"url": "SECRET_URL"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.Text != "SECRET_RESULT" || traceID == "" {
		t.Fatalf("output=%#v traceID=%q", output, traceID)
	}
	traces := runtime.Traces(traceID)
	if len(traces) != 3 {
		t.Fatalf("trace count=%d, want 3", len(traces))
	}
	encoded, _ := json.Marshal(traces)
	for _, secret := range []string{"SECRET_URL", "SECRET_RESULT"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("tool trace leaked %q: %s", secret, encoded)
		}
	}
	if traces[0].AgentID != "web_crawler" || traces[0].ToolName != "fetch_web_content" {
		t.Fatalf("unexpected trace metadata: %#v", traces[0])
	}

	if err := runtime.CallToolJSON(context.Background(), "reporter", "fetch_web_content", map[string]string{"url": "https://example.com"}, &output); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unauthorized tool call error=%v", err)
	}
}

func TestAgentCannotRegisterUnknownFunctionTool(t *testing.T) {
	runtime, err := NewRuntime(&trackingClient{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Register(Agent{ID: "crawler", SystemPrompt: "crawl", Tools: []string{"missing_tool"}})
	if err == nil || !strings.Contains(err.Error(), "unregistered tool") {
		t.Fatalf("Register() error=%v", err)
	}
}
