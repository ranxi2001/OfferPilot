package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		key   string
		value string
		ok    bool
	}{
		{name: "plain", input: "PORT=3001", key: "PORT", value: "3001", ok: true},
		{name: "quoted", input: `OPENAI_MODEL="gpt-5.5"`, key: "OPENAI_MODEL", value: "gpt-5.5", ok: true},
		{name: "export", input: "export FEATURE=true", key: "FEATURE", value: "true", ok: true},
		{name: "comment", input: " # ignored", ok: false},
		{name: "invalid", input: "NOT A KEY=value", ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, value, ok := parseEnvLine(test.input)
			if key != test.key || value != test.value || ok != test.ok {
				t.Fatalf("parseEnvLine(%q) = (%q, %q, %v), want (%q, %q, %v)", test.input, key, value, ok, test.key, test.value, test.ok)
			}
		})
	}
}

func TestLoadDotEnvUsesConfiguredPathAndPreservesNonEmptyEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.env")
	if err := os.WriteFile(path, []byte("OFFERPILOT_TEST_FILE_VALUE=from-file\nOFFERPILOT_TEST_OVERRIDE=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OFFERPILOT_CONFIG_PATH", path)
	t.Setenv("OFFERPILOT_TEST_FILE_VALUE", "")
	t.Setenv("OFFERPILOT_TEST_OVERRIDE", "from-process")

	loaded, err := LoadDotEnv()
	if err != nil {
		t.Fatal(err)
	}
	if loaded != path {
		t.Fatalf("loaded path = %q, want %q", loaded, path)
	}
	if got := os.Getenv("OFFERPILOT_TEST_FILE_VALUE"); got != "from-file" {
		t.Fatalf("file value = %q, want from-file", got)
	}
	if got := os.Getenv("OFFERPILOT_TEST_OVERRIDE"); got != "from-process" {
		t.Fatalf("override = %q, want from-process", got)
	}
}

func TestLoadDotEnvIgnoresMissingConfiguredPath(t *testing.T) {
	t.Setenv("OFFERPILOT_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.env"))
	loaded, err := LoadDotEnv()
	if err != nil {
		t.Fatal(err)
	}
	if loaded != "" {
		t.Fatalf("loaded path = %q, want empty", loaded)
	}
}
