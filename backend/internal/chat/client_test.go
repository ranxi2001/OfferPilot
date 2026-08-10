package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamForwardsTextThinkingAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing authorization")
		}
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = response.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"分析\"}}]}\n\n"))
		_, _ = response.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"回答\"}}]}\n\n"))
		_, _ = response.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3}}\n\n"))
		_, _ = response.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "secret", BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var text, thinking strings.Builder
	usage, err := client.Stream(context.Background(), "", []Message{{Role: "user", Content: "test"}}, func(delta Delta) error {
		text.WriteString(delta.Text)
		thinking.WriteString(delta.Thinking)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if text.String() != "回答" || thinking.String() != "分析" {
		t.Fatalf("unexpected deltas: text=%q thinking=%q", text.String(), thinking.String())
	}
	if usage.InputTokens != 12 || usage.OutputTokens != 3 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestStreamAcceptsNonStreamingCompatibilityResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"完整回答"}}],"usage":{"prompt_tokens":2,"completion_tokens":4}}`))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "secret", BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var result string
	_, err = client.Stream(context.Background(), "", []Message{{Role: "user", Content: "test"}}, func(delta Delta) error {
		result += delta.Text
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "完整回答" {
		t.Fatalf("result = %q", result)
	}
}
