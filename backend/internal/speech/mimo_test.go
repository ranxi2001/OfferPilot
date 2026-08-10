package speech

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscribeSendsGroundedAudioRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" || request.Header.Get("api-key") != "secret" {
			t.Fatal("missing MiMo authentication headers")
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		messages := body["messages"].([]any)
		content := messages[0].(map[string]any)["content"].([]any)
		data := content[0].(map[string]any)["input_audio"].(map[string]any)["data"].(string)
		want := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString([]byte("wav"))
		if data != want {
			t.Fatalf("audio data = %q, want %q", data, want)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"转写结果"}}]}`))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "secret", BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	text, err := client.Transcribe(context.Background(), TranscribeInput{
		Audio: []byte("wav"), FileName: "answer.wav", ContentType: "audio/wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "转写结果" {
		t.Fatalf("text = %q", text)
	}
}

func TestSynthesizeDecodesAudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		encoded := base64.StdEncoding.EncodeToString([]byte("audio"))
		_, _ = response.Write([]byte(`{"choices":[{"message":{"audio":{"data":"` + encoded + `"}}}]}`))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "secret", BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	audio, err := client.Synthesize(context.Background(), SynthesizeInput{Text: "下一题", Format: "mp3"})
	if err != nil {
		t.Fatal(err)
	}
	if string(audio.Data) != "audio" || audio.ContentType != "audio/mpeg" {
		t.Fatalf("unexpected audio response: %#v", audio)
	}
}

func TestTranscribeRejectsUnsupportedFormat(t *testing.T) {
	client, err := New(Config{APIKey: "secret"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Transcribe(context.Background(), TranscribeInput{
		Audio: []byte("webm"), FileName: "answer.webm", ContentType: "audio/webm",
	})
	if err == nil {
		t.Fatal("expected unsupported format error")
	}
}
