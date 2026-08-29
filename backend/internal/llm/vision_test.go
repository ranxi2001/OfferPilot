package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChatJSONWithImagesUsesOpenAIContentParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		parts, ok := payload.Messages[len(payload.Messages)-1].Content.([]any)
		if !ok || len(parts) != 2 {
			t.Fatalf("multimodal content=%#v", payload.Messages[len(payload.Messages)-1].Content)
		}
		imagePart := parts[1].(map[string]any)
		imageURL := imagePart["image_url"].(map[string]any)
		if imagePart["type"] != "image_url" || imageURL["url"] != "data:image/jpeg;base64,abc" || imageURL["detail"] != "high" {
			t.Fatalf("image part=%#v", imagePart)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"ok\"}"}}]}`))
	}))
	defer server.Close()

	client, err := NewWithHTTPClient(Config{
		APIKey: "secret", BaseURL: server.URL, Model: "vision-model",
		Timeout: 5 * time.Second, MaxRetries: 0, MaxTokens: 100,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Summary string `json:"summary"`
	}
	err = client.ChatJSONWithImages(context.Background(), []Message{
		{Role: RoleSystem, Content: "system"},
		{Role: RoleUser, Content: "analyze resume"},
	}, []ImageInput{{URL: "data:image/jpeg;base64,abc", Detail: "high"}}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.Summary != "ok" {
		t.Fatalf("output=%#v", output)
	}
}
