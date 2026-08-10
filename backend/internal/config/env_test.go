package config

import "testing"

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
