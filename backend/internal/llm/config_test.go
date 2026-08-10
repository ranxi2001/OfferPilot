package llm

import (
	"testing"
	"time"
)

func TestConfigTimeoutDefaultsAndEnvironmentParsing(t *testing.T) {
	t.Setenv("OPENAI_TIMEOUT", "")
	if got := ConfigFromEnv().Timeout; got != 90*time.Second {
		t.Fatalf("default timeout = %s, want 90s", got)
	}

	t.Setenv("OPENAI_TIMEOUT", "2m")
	if got := ConfigFromEnv().Timeout; got != 2*time.Minute {
		t.Fatalf("duration timeout = %s, want 2m", got)
	}

	t.Setenv("OPENAI_TIMEOUT", "1500")
	if got := ConfigFromEnv().Timeout; got != 1500*time.Millisecond {
		t.Fatalf("millisecond timeout = %s, want 1500ms", got)
	}

	t.Setenv("OPENAI_TIMEOUT", "invalid")
	if got := ConfigFromEnv().Timeout; got != 90*time.Second {
		t.Fatalf("invalid timeout fallback = %s, want 90s", got)
	}

	normalized, err := (Config{APIKey: "test", Timeout: 0}).normalized()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Timeout != 90*time.Second {
		t.Fatalf("normalized timeout = %s, want 90s", normalized.Timeout)
	}
}
