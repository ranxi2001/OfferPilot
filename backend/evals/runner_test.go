package evals

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestDefaultCorpusPassesStrictGate(t *testing.T) {
	corpus, err := LoadDefaultCorpus()
	if err != nil {
		t.Fatalf("LoadDefaultCorpus() error = %v", err)
	}
	runner, err := NewRunner(corpus.Thresholds)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	report, err := runner.Run(corpus)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !report.Passed {
		t.Fatalf("report did not pass: %+v", report.Failures)
	}
	if report.Metrics.CaseCount != 30 || report.Metrics.QuestionCount != 90 {
		t.Fatalf("unexpected corpus size: cases=%d questions=%d", report.Metrics.CaseCount, report.Metrics.QuestionCount)
	}
	if report.Metrics.ModeSeniorityCoverageRate != 1 || report.Metrics.EvidenceValidityRate != 1 {
		t.Fatalf("unexpected strict metrics: %+v", report.Metrics)
	}
	if report.Metrics.DuplicateQuestionCount != 0 || report.Metrics.PrivacyMarkerHitCount != 0 {
		t.Fatalf("default corpus contains quality findings: %+v", report.Metrics)
	}
}

func TestDefaultCorpusQuestionsAreUniqueAcrossCases(t *testing.T) {
	corpus, err := LoadDefaultCorpus()
	if err != nil {
		t.Fatalf("LoadDefaultCorpus() error = %v", err)
	}

	seen := make(map[string]string, 90)
	for _, evalCase := range corpus.Cases {
		for _, question := range evalCase.Questions {
			normalized := normalizeComparable(question.Text)
			if previous, exists := seen[normalized]; exists {
				t.Errorf("question %s/%s duplicates %s after normalization", evalCase.ID, question.ID, previous)
				continue
			}
			seen[normalized] = evalCase.ID + "/" + question.ID
		}
	}
	if len(seen) != 90 {
		t.Fatalf("unique normalized questions = %d, want 90", len(seen))
	}
}

func TestRunnerRejectsDuplicateQuestionsAcrossCases(t *testing.T) {
	corpus, err := LoadDefaultCorpus()
	if err != nil {
		t.Fatal(err)
	}
	corpus.Cases[1].Questions[0].Text = corpus.Cases[0].Questions[0].Text
	runner, err := NewRunner(corpus.Thresholds)
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Metrics.DuplicateQuestionCount != 1 {
		t.Fatalf("cross-case duplicate escaped the gate: %+v", report.Metrics)
	}
	findings := report.Cases[1].Findings
	if len(findings) == 0 || findings[0].Code != "duplicate_question" || findings[0].RelatedID == "" {
		t.Fatalf("cross-case duplicate finding = %+v", findings)
	}
}

func TestRunnerReportsEveryQualityMetricFailure(t *testing.T) {
	corpus := failingCorpus()
	runner, err := NewRunner(corpus.Thresholds)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	report, err := runner.Run(corpus)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Passed {
		t.Fatal("report unexpectedly passed")
	}
	want := map[string]bool{
		"duplicateQuestionRate":     false,
		"evidenceValidityRate":      false,
		"modeCoverageRate":          false,
		"seniorityCoverageRate":     false,
		"modeSeniorityCoverageRate": false,
		"modeConformanceRate":       false,
		"privacyMarkerHitCount":     false,
	}
	for _, failure := range report.Failures {
		if _, exists := want[failure.Metric]; exists {
			want[failure.Metric] = true
		}
	}
	for metric, found := range want {
		if !found {
			t.Errorf("missing failure for %s: %+v", metric, report.Failures)
		}
	}
	if report.Metrics.DuplicateQuestionCount != 1 {
		t.Errorf("DuplicateQuestionCount = %d, want 1", report.Metrics.DuplicateQuestionCount)
	}
	if report.Metrics.InvalidEvidenceRefCount == 0 || report.Metrics.EvidenceKindMismatchCount == 0 {
		t.Errorf("evidence defects were not counted: %+v", report.Metrics)
	}
}

func TestCorpusValidationRejectsInvalidInputs(t *testing.T) {
	t.Run("missing required fields", func(t *testing.T) {
		if err := ValidateCorpus(Corpus{}); err == nil {
			t.Fatal("ValidateCorpus() error = nil")
		}
	})

	t.Run("unknown JSON field", func(t *testing.T) {
		_, err := DecodeCorpus(strings.NewReader(`{"schemaVersion":"1.0.0","unknown":true}`))
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("DecodeCorpus() error = %v", err)
		}
	})

	t.Run("multiple JSON values", func(t *testing.T) {
		_, err := DecodeCorpus(strings.NewReader(`{} {}`))
		if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
			t.Fatalf("DecodeCorpus() error = %v", err)
		}
	})

	t.Run("invalid threshold", func(t *testing.T) {
		_, err := NewRunner(Thresholds{MaxDuplicateQuestionRate: 1.1})
		if err == nil {
			t.Fatal("NewRunner() error = nil")
		}
	})

	t.Run("not a number threshold", func(t *testing.T) {
		_, err := NewRunner(Thresholds{MinEvidenceValidityRate: math.NaN()})
		if err == nil {
			t.Fatal("NewRunner() error = nil")
		}
	})

	t.Run("unsupported schema version", func(t *testing.T) {
		corpus := failingCorpus()
		corpus.SchemaVersion = "2.0.0"
		if err := ValidateCorpus(corpus); err == nil || !strings.Contains(err.Error(), "schemaVersion") {
			t.Fatalf("ValidateCorpus() error = %v", err)
		}
	})

	t.Run("embedded schema is JSON", func(t *testing.T) {
		schema, err := JSONSchema()
		if err != nil {
			t.Fatalf("JSONSchema() error = %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(schema, &decoded); err != nil {
			t.Fatalf("schema is invalid JSON: %v", err)
		}
		if decoded["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("unexpected schema declaration: %v", decoded["$schema"])
		}
	})
}

func TestPrivacyFindingsDoNotRevealRawMarker(t *testing.T) {
	corpus := failingCorpus()
	runner, err := NewRunner(corpus.Thresholds)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	report, err := runner.Run(corpus)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	marker := corpus.Cases[0].PrivateMarkers[0]
	if strings.Contains(string(encoded), marker) {
		t.Fatalf("report leaked raw marker %q", marker)
	}
	if !strings.Contains(string(encoded), "markerFingerprint") {
		t.Fatal("report did not expose a marker fingerprint")
	}
}

func TestReportOrderingIsDeterministic(t *testing.T) {
	corpus := failingCorpus()
	corpus.Cases[0].PrivateMarkers = []string{"ZETA_PRIVATE_MARKER", "ALPHA_PRIVATE_MARKER"}
	corpus.Cases[0].PublicTexts[0].Text = "Both ALPHA_PRIVATE_MARKER and ZETA_PRIVATE_MARKER are private."
	runner, err := NewRunner(corpus.Thresholds)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	var baseline []byte
	for run := 0; run < 50; run++ {
		report, runErr := runner.Run(corpus)
		if runErr != nil {
			t.Fatalf("Run() error = %v", runErr)
		}
		encoded, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			t.Fatalf("Marshal() error = %v", marshalErr)
		}
		if run == 0 {
			baseline = encoded
			continue
		}
		if string(encoded) != string(baseline) {
			t.Fatalf("run %d produced nondeterministic output", run)
		}
	}
}

func failingCorpus() Corpus {
	return Corpus{
		SchemaVersion: "1.0.0",
		CorpusVersion: "test-failing",
		Description:   "Synthetic failing corpus",
		RequiredCoverage: CoverageRequirements{
			Modes:       []string{"knowledge", "projects", "mixed"},
			Seniorities: []string{"junior", "mid", "senior"},
		},
		Thresholds: Thresholds{
			MaxDuplicateQuestionRate:     0,
			MinEvidenceValidityRate:      1,
			MinModeCoverageRate:          1,
			MinSeniorityCoverageRate:     1,
			MinModeSeniorityCoverageRate: 1,
			MinModeConformanceRate:       1,
			MaxPrivacyMarkerHits:         0,
		},
		Cases: []Case{
			{
				ID:        "broken-case",
				Title:     "Broken synthetic output",
				Mode:      "knowledge",
				Seniority: "junior",
				EvidenceCatalog: []Evidence{
					{ID: "knowledge-1", Kind: "knowledge", Label: "Synthetic knowledge"},
				},
				PrivateMarkers: []string{"DO_NOT_EXPOSE_RAW_MARKER"},
				Questions: []Question{
					{ID: "q1", Text: "Explain the tradeoff.", Kind: "project", Difficulty: "easy", EvidenceRefIDs: []string{"knowledge-1"}},
					{ID: "q2", Text: "EXPLAIN, THE TRADEOFF!", Kind: "knowledge", Difficulty: "medium", EvidenceRefIDs: []string{"unknown"}},
					{ID: "q3", Text: "Which prerequisite is missing?", Kind: "prerequisite", Difficulty: "hard", EvidenceRefIDs: nil},
				},
				PublicTexts: []PublicText{
					{ID: "leak", Text: "The hidden token is DO_NOT_EXPOSE_RAW_MARKER."},
				},
			},
		},
	}
}
