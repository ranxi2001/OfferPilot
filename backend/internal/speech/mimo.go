package speech

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "https://api.xiaomimimo.com/v1"
	defaultASRModel = "mimo-v2.5-asr"
	defaultTTSModel = "mimo-v2.5-tts"
	defaultVoice    = "mimo_default"
	maxResponseSize = 32 << 20
	asrMaxAttempts  = 3
	asrRetryDelay   = 100 * time.Millisecond
)

var (
	errProviderUnavailable = errors.New("speech provider is temporarily unavailable")
	errProviderRejected    = errors.New("speech provider rejected the request")
	errInvalidResponse     = errors.New("speech provider returned an invalid response")
	errAudioRequired       = errors.New("audio is required")
	errUnsupportedAudio    = errors.New("unsupported audio format")
	errEmptyTranscript     = errors.New("empty transcript")
)

type providerRejectedError struct {
	status int
}

func (e *providerRejectedError) Error() string {
	return errProviderRejected.Error()
}

func (e *providerRejectedError) Unwrap() error {
	return errProviderRejected
}

type TranscriptionFailure struct {
	Status    int
	Message   string
	Retryable bool
}

func ClassifyTranscriptionError(err error) TranscriptionFailure {
	switch {
	case errors.Is(err, errAudioRequired):
		return TranscriptionFailure{Status: http.StatusBadRequest, Message: "没有收到录音，请重新录制"}
	case errors.Is(err, errUnsupportedAudio):
		return TranscriptionFailure{Status: http.StatusUnsupportedMediaType, Message: "当前录音格式无法识别，请重新录制"}
	case errors.Is(err, errEmptyTranscript):
		return TranscriptionFailure{Status: http.StatusUnprocessableEntity, Message: "未识别到有效语音，请重新录制"}
	case errors.Is(err, errProviderRejected):
		var rejected *providerRejectedError
		if errors.As(err, &rejected) && (rejected.status == http.StatusUnauthorized || rejected.status == http.StatusForbidden) {
			return TranscriptionFailure{Status: http.StatusServiceUnavailable, Message: "语音识别服务配置异常，请联系管理员"}
		}
		return TranscriptionFailure{Status: http.StatusUnprocessableEntity, Message: "录音无法被语音服务处理，请重新录制"}
	case errors.Is(err, errProviderUnavailable):
		return TranscriptionFailure{Status: http.StatusServiceUnavailable, Message: "语音识别服务暂时不可用，请重试", Retryable: true}
	case errors.Is(err, errInvalidResponse):
		return TranscriptionFailure{Status: http.StatusBadGateway, Message: "语音识别服务响应异常，请重试", Retryable: true}
	default:
		return TranscriptionFailure{Status: http.StatusBadGateway, Message: "语音识别服务暂时不可用，请重试", Retryable: true}
	}
}

type Config struct {
	APIKey      string
	BaseURL     string
	ASRModel    string
	TTSModel    string
	TTSVoice    string
	ASRLanguage string
	Timeout     time.Duration
}

type Client struct {
	config        Config
	http          *http.Client
	retryAttempts int
	retryDelay    time.Duration
}

type TranscribeInput struct {
	Audio       []byte
	FileName    string
	ContentType string
}

type SynthesizeInput struct {
	Text   string
	Voice  string
	Format string
}

type Audio struct {
	Data        []byte
	ContentType string
}

func New(config Config, httpClient *http.Client) (*Client, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	if config.APIKey == "" {
		return nil, errors.New("speech: MIMO_API_KEY is required")
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	if strings.TrimSpace(config.ASRModel) == "" {
		config.ASRModel = defaultASRModel
	}
	if strings.TrimSpace(config.TTSModel) == "" {
		config.TTSModel = defaultTTSModel
	}
	if strings.TrimSpace(config.TTSVoice) == "" {
		config.TTSVoice = defaultVoice
	}
	if strings.TrimSpace(config.ASRLanguage) == "" {
		config.ASRLanguage = "auto"
	}
	if config.Timeout <= 0 {
		config.Timeout = 60 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.Timeout}
	}
	return &Client{
		config:        config,
		http:          httpClient,
		retryAttempts: asrMaxAttempts,
		retryDelay:    asrRetryDelay,
	}, nil
}

func (c *Client) Transcribe(ctx context.Context, input TranscribeInput) (string, error) {
	if len(input.Audio) == 0 {
		return "", errAudioRequired
	}
	mime, ok := normalizedAudioMIME(input.ContentType, input.FileName)
	if !ok {
		return "", errUnsupportedAudio
	}
	payload := map[string]any{
		"model": c.config.ASRModel,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_audio",
				"input_audio": map[string]string{
					"data": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(input.Audio),
				},
			}},
		}},
		"asr_options": map[string]string{"language": c.config.ASRLanguage},
	}

	var result struct {
		Text    string `json:"text"`
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := c.postWithRetry(ctx, payload, &result); err != nil {
		return "", fmt.Errorf("speech: MiMo ASR: %w", err)
	}
	text := strings.TrimSpace(result.Text)
	if text == "" && len(result.Choices) > 0 {
		text = extractText(result.Choices[0].Message.Content)
	}
	if text == "" {
		return "", errEmptyTranscript
	}
	return text, nil
}

func (c *Client) Synthesize(ctx context.Context, input SynthesizeInput) (Audio, error) {
	if strings.TrimSpace(input.Text) == "" {
		return Audio{}, errors.New("speech: text is required")
	}
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "" {
		format = "wav"
	}
	if format != "wav" && format != "mp3" {
		return Audio{}, errors.New("speech: TTS format must be wav or mp3")
	}
	voice := strings.TrimSpace(input.Voice)
	if voice == "" {
		voice = c.config.TTSVoice
	}
	payload := map[string]any{
		"model": c.config.TTSModel,
		"messages": []map[string]string{
			{"role": "user", "content": "用自然、清晰、适合中文面试反馈的语气朗读。"},
			{"role": "assistant", "content": input.Text},
		},
		"audio": map[string]string{"format": format, "voice": voice},
	}

	var result struct {
		Choices []struct {
			Message struct {
				Audio struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := c.post(ctx, payload, &result); err != nil {
		return Audio{}, fmt.Errorf("speech: MiMo TTS: %w", err)
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Audio.Data) == "" {
		return Audio{}, errors.New("speech: MiMo TTS returned empty audio")
	}
	data, err := base64.StdEncoding.DecodeString(result.Choices[0].Message.Audio.Data)
	if err != nil {
		return Audio{}, fmt.Errorf("speech: decode MiMo TTS audio: %w", err)
	}
	contentType := "audio/wav"
	if format == "mp3" {
		contentType = "audio/mpeg"
	}
	return Audio{Data: data, ContentType: contentType}, nil
}

func (c *Client) post(ctx context.Context, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	_, err = c.postOnce(ctx, body, output)
	return err
}

// ASR is read-only, so transient failures can be retried safely. TTS continues
// to use post directly because generating the same audio twice may incur a
// duplicate provider-side operation.
func (c *Client) postWithRetry(ctx context.Context, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	attempts := c.retryAttempts
	if attempts < 1 {
		attempts = 1
	}
	delay := c.retryDelay
	if delay < 0 {
		delay = 0
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		retryable, requestErr := c.postOnce(ctx, body, output)
		if requestErr == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !retryable || attempt == attempts {
			return requestErr
		}
		if err := waitForRetry(ctx, delay*time.Duration(1<<(attempt-1))); err != nil {
			return err
		}
	}
	return errProviderUnavailable
}

func (c *Client) postOnce(ctx context.Context, body []byte, output any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return false, errors.New("create speech provider request")
	}
	req.Header.Set("api-key", c.config.APIKey)
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(req)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		return isRetryableTransportError(err), errProviderUnavailable
	}
	data, err := readAndClose(response.Body)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		if isRetryableTransportError(err) {
			return true, errProviderUnavailable
		}
		return false, errInvalidResponse
	}
	if len(data) > maxResponseSize {
		return false, errInvalidResponse
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
			return true, errProviderUnavailable
		}
		return false, &providerRejectedError{status: response.StatusCode}
	}
	if err := json.Unmarshal(data, output); err != nil {
		return false, errInvalidResponse
	}
	return false, nil
}

func readAndClose(body io.ReadCloser) ([]byte, error) {
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, maxResponseSize+1))
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableTransportError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary()) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection reset") ||
		strings.Contains(message, "forcibly closed") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "server closed idle connection") ||
		strings.Contains(message, "server sent goaway")
}

func normalizedAudioMIME(contentType, fileName string) (string, bool) {
	mime := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	name := strings.ToLower(strings.TrimSpace(fileName))
	switch {
	case mime == "audio/wav" || mime == "audio/x-wav" || strings.HasSuffix(name, ".wav"):
		return "audio/wav", true
	case mime == "audio/mpeg" || mime == "audio/mp3" || strings.HasSuffix(name, ".mp3"):
		return "audio/mpeg", true
	default:
		return "", false
	}
}

func extractText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var result strings.Builder
	for _, part := range parts {
		result.WriteString(part.Text)
	}
	return strings.TrimSpace(result.String())
}
