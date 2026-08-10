package speech

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "https://api.xiaomimimo.com/v1"
	defaultASRModel = "mimo-v2.5-asr"
	defaultTTSModel = "mimo-v2.5-tts"
	defaultVoice    = "alloy"
	maxResponseSize = 32 << 20
)

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
	config Config
	http   *http.Client
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
	return &Client{config: config, http: httpClient}, nil
}

func (c *Client) Transcribe(ctx context.Context, input TranscribeInput) (string, error) {
	if len(input.Audio) == 0 {
		return "", errors.New("speech: audio is required")
	}
	mime, ok := normalizedAudioMIME(input.ContentType, input.FileName)
	if !ok {
		return "", errors.New("speech: MiMo ASR supports only WAV or MP3 audio")
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
	if err := c.post(ctx, payload, &result); err != nil {
		return "", fmt.Errorf("speech: MiMo ASR: %w", err)
	}
	text := strings.TrimSpace(result.Text)
	if text == "" && len(result.Choices) > 0 {
		text = extractText(result.Choices[0].Message.Content)
	}
	if text == "" {
		return "", errors.New("speech: MiMo ASR returned an empty transcript")
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-key", c.config.APIKey)
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxResponseSize {
		return errors.New("response exceeds 32 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, compact(string(data), 2000))
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
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

func compact(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
