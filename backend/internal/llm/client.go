package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	config Config
	http   *http.Client
}

type completionRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
}

type completionResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type HTTPError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("llm: provider returned HTTP %d: %s", e.StatusCode, e.Body)
}

func New(config Config) (*Client, error) {
	return NewWithHTTPClient(config, nil)
}

func NewFromEnv() (*Client, error) {
	return New(ConfigFromEnv())
}

func NewWithHTTPClient(config Config, httpClient *http.Client) (*Client, error) {
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	if _, err := url.ParseRequestURI(normalized.BaseURL); err != nil {
		return nil, fmt.Errorf("llm: invalid OPENAI_BASE_URL: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{config: normalized, http: httpClient}, nil
}

// ChatJSON asks the provider for a response constrained to out's JSON shape.
// Invalid model output gets exactly one repair call. Providers that reject
// json_schema get exactly one compatibility fallback to json_object.
func (c *Client) ChatJSON(ctx context.Context, messages []Message, out any) error {
	if err := validateOutputTarget(out); err != nil {
		return err
	}
	schema, err := schemaFor(out)
	if err != nil {
		return err
	}

	content, err := c.complete(ctx, messages, strictResponseFormat(schema))
	if err != nil && schemaUnsupported(err) {
		// The compatibility fallback consumes the single recovery budget.
		// Network retries remain independently bounded inside complete.
		content, err = c.complete(ctx, ensureJSONInstruction(messages, schema), map[string]any{"type": "json_object"})
		if err != nil {
			return err
		}
		if decodeErr := DecodeJSON(content, out); decodeErr != nil {
			return fmt.Errorf("llm: invalid structured response after one compatibility fallback: %w", decodeErr)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := DecodeJSON(content, out); err == nil {
		return nil
	} else {
		repair := appendCopy(messages,
			Message{Role: RoleAssistant, Content: compactForPrompt(content, 12000)},
			Message{Role: RoleUser, Content: "Return the same answer again as one valid JSON object matching the required schema. Do not use Markdown fences or add commentary. Fix this validation error: " + err.Error()},
		)
		repaired, repairErr := c.complete(ctx, repair, strictResponseFormat(schema))
		if repairErr != nil {
			return fmt.Errorf("llm: repair structured response: %w", repairErr)
		}
		if decodeErr := DecodeJSON(repaired, out); decodeErr != nil {
			return fmt.Errorf("llm: invalid structured response after one repair: %w", decodeErr)
		}
		return nil
	}
}

func strictResponseFormat(schema map[string]any) map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "structured_response",
			"strict": true,
			"schema": schema,
		},
	}
}

func ensureJSONInstruction(messages []Message, schema map[string]any) []Message {
	encodedSchema, _ := json.Marshal(schema)
	return appendCopy(messages, Message{
		Role:    RoleUser,
		Content: "Return exactly one JSON object matching this JSON Schema, with no Markdown or commentary:\n" + string(encodedSchema),
	})
}

func appendCopy(messages []Message, extra ...Message) []Message {
	result := make([]Message, 0, len(messages)+len(extra))
	result = append(result, messages...)
	return append(result, extra...)
}

func (c *Client) complete(ctx context.Context, messages []Message, format map[string]any) (string, error) {
	request := completionRequest{
		Model:          c.config.Model,
		Messages:       messages,
		ResponseFormat: format,
		MaxTokens:      c.config.MaxTokens,
		Temperature:    c.config.Temperature,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("llm: encode request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := waitForRetry(ctx, retryDelay(c.config.RetryBaseWait, attempt, lastErr)); err != nil {
				return "", err
			}
		}
		content, err := c.doRequest(ctx, body)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if !retryable(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("llm: request failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

func (c *Client) doRequest(parent context.Context, body []byte) (string, error) {
	ctx, cancel := context.WithTimeout(parent, c.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	response, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("llm: request timeout after %s: %w", c.config.Timeout, context.DeadlineExceeded)
		}
		return "", fmt.Errorf("llm: request: %w", err)
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, 4<<20)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("llm: read response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", &HTTPError{
			StatusCode: response.StatusCode,
			Body:       compactForPrompt(providerError(data), 2000),
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After")),
		}
	}

	var decoded completionResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("llm: decode provider response: %w", err)
	}
	if decoded.Error != nil {
		return "", fmt.Errorf("llm: provider error: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("llm: provider response has no choices")
	}
	content, err := decodeContent(decoded.Choices[0].Message.Content)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", errors.New("llm: provider returned empty content")
	}
	return content, nil
}

func decodeContent(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	// A few compatible gateways expose multimodal content parts even for text.
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var result strings.Builder
		for _, part := range parts {
			if part.Type == "text" || part.Type == "output_text" || part.Type == "" {
				result.WriteString(part.Text)
			}
		}
		return result.String(), nil
	}
	return "", errors.New("llm: provider message content is not text")
}

func providerError(data []byte) string {
	var decoded completionResponse
	if json.Unmarshal(data, &decoded) == nil && decoded.Error != nil && decoded.Error.Message != "" {
		return decoded.Error.Message
	}
	return strings.TrimSpace(string(data))
}

func retryable(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusRequestTimeout ||
			httpErr.StatusCode == http.StatusTooManyRequests ||
			httpErr.StatusCode >= 500
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var networkErr net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkErr)
}

func schemaUnsupported(err error) bool {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		return false
	}
	body := strings.ToLower(httpErr.Body)
	return strings.Contains(body, "response_format") ||
		strings.Contains(body, "json_schema") ||
		strings.Contains(body, "structured output")
}

func retryDelay(base time.Duration, attempt int, previous error) time.Duration {
	var httpErr *HTTPError
	if errors.As(previous, &httpErr) && httpErr.RetryAfter > 0 {
		return httpErr.RetryAfter
	}
	shift := min(attempt-1, 6)
	delay := base * time.Duration(1<<shift)
	// Full jitter avoids synchronized retries across interview requests.
	return time.Duration(rand.Int64N(max(int64(delay), 1)))
}

func waitForRetry(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if timestamp, err := http.ParseTime(value); err == nil {
		return max(time.Until(timestamp), 0)
	}
	return 0
}
