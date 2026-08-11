package speech

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type trackedBody struct {
	io.Reader
	closed *int
}

func (body *trackedBody) Close() error {
	*body.closed = *body.closed + 1
	return nil
}

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
		var body struct {
			Audio struct {
				Voice string `json:"voice"`
			} `json:"audio"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Audio.Voice != "mimo_default" {
			t.Fatalf("voice = %q, want mimo_default", body.Audio.Voice)
		}
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
	failure := ClassifyTranscriptionError(err)
	if failure.Status != http.StatusUnsupportedMediaType || failure.Retryable {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestTranscribeClassifiesEmptyTranscriptAsNonRetryable(t *testing.T) {
	client := newRetryTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonHTTPResponse(http.StatusOK, `{"text":""}`, nil), nil
	}))

	_, err := client.Transcribe(context.Background(), testTranscribeInput())
	if err == nil {
		t.Fatal("expected empty transcript error")
	}
	failure := ClassifyTranscriptionError(err)
	if failure.Status != http.StatusUnprocessableEntity || failure.Retryable {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestTranscribeRetriesEOFAfterRebuildingRequestBody(t *testing.T) {
	attempts := 0
	var firstBody []byte
	client := newRetryTestClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) == 0 {
			t.Fatal("request body is empty")
		}
		if attempts == 1 {
			firstBody = append([]byte(nil), body...)
			return nil, io.EOF
		}
		if string(body) != string(firstBody) {
			t.Fatal("retry did not rebuild the original request body")
		}
		return jsonHTTPResponse(http.StatusOK, `{"choices":[{"message":{"content":"transcript"}}]}`, nil), nil
	}))

	text, err := client.Transcribe(context.Background(), testTranscribeInput())
	if err != nil {
		t.Fatal(err)
	}
	if text != "transcript" || attempts != 2 {
		t.Fatalf("text = %q, attempts = %d", text, attempts)
	}
}

func TestTranscribeRetriesOtherTransientTransportFailures(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
	}{
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "connection reset", err: errors.New("read tcp: connection reset by peer")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			attempts := 0
			client := newRetryTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts++
				if attempts == 1 {
					return nil, testCase.err
				}
				return jsonHTTPResponse(http.StatusOK, `{"text":"transcript"}`, nil), nil
			}))

			text, err := client.Transcribe(context.Background(), testTranscribeInput())
			if err != nil {
				t.Fatal(err)
			}
			if text != "transcript" || attempts != 2 {
				t.Fatalf("text = %q, attempts = %d", text, attempts)
			}
		})
	}
}

func TestTranscribeStopsAfterRetryLimitAndClosesResponses(t *testing.T) {
	attempts := 0
	closed := 0
	client := newRetryTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return jsonHTTPResponse(http.StatusServiceUnavailable, `{"error":"secret provider details"}`, &closed), nil
	}))

	_, err := client.Transcribe(context.Background(), testTranscribeInput())
	if err == nil {
		t.Fatal("expected persistent provider failure")
	}
	if attempts != asrMaxAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, asrMaxAttempts)
	}
	if closed != attempts {
		t.Fatalf("closed response bodies = %d, want %d", closed, attempts)
	}
	assertSafeProviderError(t, err)
	failure := ClassifyTranscriptionError(err)
	if failure.Status != http.StatusServiceUnavailable || !failure.Retryable {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestTranscribeDoesNotRetryAfterContextCancellation(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())
	client := newRetryTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		cancel()
		return nil, io.EOF
	}))

	_, err := client.Transcribe(ctx, testTranscribeInput())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestTranscribeDoesNotRetryNonTransient4xx(t *testing.T) {
	attempts := 0
	closed := 0
	client := newRetryTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return jsonHTTPResponse(http.StatusUnauthorized, `{"error":"key=secret https://provider.example/private"}`, &closed), nil
	}))

	_, err := client.Transcribe(context.Background(), testTranscribeInput())
	if err == nil {
		t.Fatal("expected rejected request error")
	}
	if attempts != 1 || closed != 1 {
		t.Fatalf("attempts = %d, closed = %d; want 1, 1", attempts, closed)
	}
	assertSafeProviderError(t, err)
	failure := ClassifyTranscriptionError(err)
	if failure.Status != http.StatusServiceUnavailable || failure.Retryable {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestTranscribeRetries429(t *testing.T) {
	attempts := 0
	client := newRetryTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return jsonHTTPResponse(http.StatusTooManyRequests, `{"error":"rate limited"}`, nil), nil
		}
		return jsonHTTPResponse(http.StatusOK, `{"text":"transcript"}`, nil), nil
	}))

	text, err := client.Transcribe(context.Background(), testTranscribeInput())
	if err != nil {
		t.Fatal(err)
	}
	if text != "transcript" || attempts != 2 {
		t.Fatalf("text = %q, attempts = %d", text, attempts)
	}
}

func TestTranscribeRetries408(t *testing.T) {
	attempts := 0
	client := newRetryTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return jsonHTTPResponse(http.StatusRequestTimeout, `{"error":"timeout"}`, nil), nil
		}
		return jsonHTTPResponse(http.StatusOK, `{"text":"transcript"}`, nil), nil
	}))

	text, err := client.Transcribe(context.Background(), testTranscribeInput())
	if err != nil {
		t.Fatal(err)
	}
	if text != "transcript" || attempts != 2 {
		t.Fatalf("text = %q, attempts = %d", text, attempts)
	}
}

func TestSynthesizeDoesNotRetryProviderFailure(t *testing.T) {
	attempts := 0
	client := newRetryTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return jsonHTTPResponse(http.StatusServiceUnavailable, `{"error":"unavailable"}`, nil), nil
	}))

	_, err := client.Synthesize(context.Background(), SynthesizeInput{Text: "question", Format: "mp3"})
	if err == nil {
		t.Fatal("expected TTS provider failure")
	}
	if attempts != 1 {
		t.Fatalf("TTS attempts = %d, want 1", attempts)
	}
}

func newRetryTestClient(t *testing.T, transport http.RoundTripper) *Client {
	t.Helper()
	client, err := New(Config{APIKey: "secret", BaseURL: "https://provider.example/v1"}, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	client.retryDelay = 0
	return client
}

func testTranscribeInput() TranscribeInput {
	return TranscribeInput{Audio: []byte("wav"), FileName: "answer.wav", ContentType: "audio/wav"}
}

func jsonHTTPResponse(status int, payload string, closed *int) *http.Response {
	var body io.ReadCloser = io.NopCloser(strings.NewReader(payload))
	if closed != nil {
		body = &trackedBody{Reader: strings.NewReader(payload), closed: closed}
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       body,
	}
}

func assertSafeProviderError(t *testing.T, err error) {
	t.Helper()
	message := err.Error()
	for _, forbidden := range []string{"secret", "provider.example", "https://", "EOF", "rate limited"} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("error leaks provider detail %q: %q", forbidden, message)
		}
	}
}
