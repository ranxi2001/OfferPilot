package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"offerpilot/backend/internal/chat"
	"offerpilot/backend/internal/session"
)

type recordingChatClient struct {
	calls [][]chat.Message
}

func (client *recordingChatClient) Stream(_ context.Context, _ string, messages []chat.Message, onDelta func(chat.Delta) error) (chat.Usage, error) {
	client.calls = append(client.calls, append([]chat.Message(nil), messages...))
	return chat.Usage{}, onDelta(chat.Delta{Text: "请介绍一个你做过的 RAG 项目。"})
}

func TestChatKeepsHistoryForClientProvidedSession(t *testing.T) {
	chatClient := &recordingChatClient{}
	sessions := session.NewStore()
	server, err := New(Config{ModelConfigured: true}, Dependencies{
		Interview: &interviewStub{}, Chat: chatClient, Sessions: sessions, Memory: session.NewMemory(40),
	})
	if err != nil {
		t.Fatal(err)
	}

	serveChat := func(message string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(
			`{"sessionId":"browser-session","message":"`+message+`"}`,
		))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		return response
	}

	first := serveChat("开始面试")
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"sessionId":"browser-session"`) {
		t.Fatalf("first response = %d %s", first.Code, first.Body.String())
	}
	second := serveChat("这是我的 RAG 项目回答")
	if second.Code != http.StatusOK {
		t.Fatalf("second response = %d %s", second.Code, second.Body.String())
	}
	if len(chatClient.calls) != 2 {
		t.Fatalf("chat calls = %d, want 2", len(chatClient.calls))
	}
	want := []chat.Message{
		{Role: "system", Content: chatSystemPrompt},
		{Role: "user", Content: "开始面试"},
		{Role: "assistant", Content: "请介绍一个你做过的 RAG 项目。"},
		{Role: "user", Content: "这是我的 RAG 项目回答"},
	}
	got := chatClient.calls[1]
	if len(got) != len(want) {
		t.Fatalf("second call messages = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("message %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}
