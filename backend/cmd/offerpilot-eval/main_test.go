package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"offerpilot/backend/evals"
)

func TestRunDefaultCorpus(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"-pretty=false"}, &stdout, &stderr); code != exitPassed {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var report evals.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not a report: %v\n%s", err, stdout.String())
	}
	if !report.Passed || report.CorpusVersion != "v0.3.0-alpha.1" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestRunReturnsOneWhenThresholdsFail(t *testing.T) {
	corpus, err := evals.LoadDefaultCorpus()
	if err != nil {
		t.Fatalf("LoadDefaultCorpus() error = %v", err)
	}
	corpus.Cases[0].Questions[1].Text = corpus.Cases[0].Questions[0].Text
	path := writeCorpus(t, corpus)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"-corpus", path, "-pretty=false"}, &stdout, &stderr); code != exitFailed {
		t.Fatalf("run() code = %d, want %d; stderr = %s", code, exitFailed, stderr.String())
	}
	var report evals.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not a report: %v", err)
	}
	if report.Passed || report.Metrics.DuplicateQuestionCount != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestRunReturnsTwoForInvalidCorpus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"-corpus", path}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("run() code = %d, want %d", code, exitInvalid)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var failure struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil {
		t.Fatalf("stderr is not JSON: %v\n%s", err, stderr.String())
	}
	if failure.Error == "" {
		t.Fatal("error response is empty")
	}
}

func TestRunPrintsSchema(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"-schema"}, &stdout, &stderr); code != exitPassed {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	var schema map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil {
		t.Fatalf("stdout is not a JSON schema: %v", err)
	}
}

func writeCorpus(t *testing.T, corpus evals.Corpus) string {
	t.Helper()
	data, err := json.Marshal(corpus)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "corpus.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
