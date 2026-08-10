package llm

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "gpt-4o-mini"
	defaultTimeout = 90 * time.Second
)

type Config struct {
	APIKey        string
	BaseURL       string
	Model         string
	Timeout       time.Duration
	MaxRetries    int
	RetryBaseWait time.Duration
	MaxTokens     int
	Temperature   *float64
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:        strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		BaseURL:       envOr("OPENAI_BASE_URL", defaultBaseURL),
		Model:         envOr("OPENAI_MODEL", defaultModel),
		Timeout:       durationEnv("OPENAI_TIMEOUT", defaultTimeout),
		MaxRetries:    intEnv("OPENAI_MAX_RETRIES", 2),
		RetryBaseWait: durationEnv("OPENAI_RETRY_BASE_WAIT", 250*time.Millisecond),
		MaxTokens:     intEnv("OPENAI_MAX_TOKENS", 4096),
	}
}

func (c Config) normalized() (Config, error) {
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.Model = strings.TrimSpace(c.Model)
	if c.APIKey == "" {
		return Config{}, errors.New("llm: OPENAI_API_KEY is required")
	}
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.Model == "" {
		c.Model = defaultModel
	}
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.RetryBaseWait <= 0 {
		c.RetryBaseWait = 250 * time.Millisecond
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 4096
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	return value
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	// A bare number is interpreted as milliseconds for deployment friendliness.
	if millis, err := strconv.Atoi(value); err == nil && millis > 0 {
		return time.Duration(millis) * time.Millisecond
	}
	return fallback
}
