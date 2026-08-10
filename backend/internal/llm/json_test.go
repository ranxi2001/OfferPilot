package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type structuredAnswer struct {
	Answer string   `json:"answer"`
	Score  int      `json:"score"`
	Notes  []string `json:"notes"`
}

func TestDecodeJSONStrictAndAtomic(t *testing.T) {
	t.Parallel()

	want := structuredAnswer{Answer: "kept", Score: 9}
	got := want
	if err := DecodeJSON(`{"answer":"changed","score":3,"notes":[],"extra":true}`, &got); err == nil {
		t.Fatal("DecodeJSON accepted an unknown field")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("destination changed after invalid JSON: got %#v want %#v", got, want)
	}

	if err := DecodeJSON("```json\n{}\n```", &got); err == nil {
		t.Fatal("DecodeJSON accepted Markdown-wrapped JSON")
	}
	if err := DecodeJSON(`{"answer":"ok","score":4,"notes":["clear"]}`, &got); err != nil {
		t.Fatalf("DecodeJSON rejected valid JSON: %v", err)
	}
	if got.Answer != "ok" || got.Score != 4 || len(got.Notes) != 1 {
		t.Fatalf("unexpected decoded value: %#v", got)
	}
}

func TestChatJSONRepairsOnce(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var payload struct {
			ResponseFormat map[string]any `json:"response_format"`
			Messages       []Message      `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.ResponseFormat["type"] != "json_schema" {
			t.Errorf("response format = %v, want json_schema", payload.ResponseFormat["type"])
		}
		response.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"not json"}}]}`))
			return
		}
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"repaired\",\"score\":5,\"notes\":[]}"}}]}`))
	}))
	defer server.Close()

	client, err := NewWithHTTPClient(Config{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		Model:      "test-model",
		Timeout:    time.Second,
		MaxRetries: 0,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var answer structuredAnswer
	if err := client.ChatJSON(context.Background(), []Message{{Role: RoleUser, Content: "answer"}}, &answer); err != nil {
		t.Fatalf("ChatJSON: %v", err)
	}
	if answer.Answer != "repaired" || calls.Load() != 2 {
		t.Fatalf("answer=%#v calls=%d", answer, calls.Load())
	}
}

func TestChatJSONFallsBackOnceWhenSchemaIsUnsupported(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var payload struct {
			ResponseFormat map[string]any `json:"response_format"`
			Messages       []Message      `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		call := calls.Add(1)
		response.Header().Set("Content-Type", "application/json")
		if call == 1 {
			if payload.ResponseFormat["type"] != "json_schema" {
				t.Errorf("first response format = %v", payload.ResponseFormat["type"])
			}
			response.WriteHeader(http.StatusBadRequest)
			_, _ = response.Write([]byte(`{"error":{"message":"response_format json_schema is unsupported"}}`))
			return
		}
		if payload.ResponseFormat["type"] != "json_object" {
			t.Errorf("fallback response format = %v", payload.ResponseFormat["type"])
		}
		if len(payload.Messages) == 0 || !strings.Contains(payload.Messages[len(payload.Messages)-1].Content, `"properties"`) {
			t.Errorf("fallback prompt does not contain the JSON schema: %#v", payload.Messages)
		}
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"fallback\",\"score\":4,\"notes\":[]}"}}]}`))
	}))
	defer server.Close()

	client, err := NewWithHTTPClient(Config{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		Model:      "test-model",
		Timeout:    time.Second,
		MaxRetries: 0,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var answer structuredAnswer
	if err := client.ChatJSON(context.Background(), []Message{{Role: RoleUser, Content: "answer"}}, &answer); err != nil {
		t.Fatalf("ChatJSON: %v", err)
	}
	if answer.Answer != "fallback" || calls.Load() != 2 {
		t.Fatalf("answer=%#v calls=%d", answer, calls.Load())
	}
}
