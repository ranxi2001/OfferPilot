package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"offerpilot/backend/evals"
)

const (
	exitPassed  = 0
	exitFailed  = 1
	exitInvalid = 2
)

type options struct {
	corpusPath               string
	pretty                   bool
	printSchema              bool
	maxDuplicateRate         float64
	minEvidenceValidity      float64
	minModeCoverage          float64
	minSeniorityCoverage     float64
	minModeSeniorityCoverage float64
	minModeConformance       float64
	maxPrivacyHits           int
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitPassed
		}
		writeError(stderr, err)
		return exitInvalid
	}

	if opts.printSchema {
		schema, schemaErr := evals.JSONSchema()
		if schemaErr != nil {
			writeError(stderr, schemaErr)
			return exitInvalid
		}
		if _, schemaErr = stdout.Write(append(schema, '\n')); schemaErr != nil {
			writeError(stderr, fmt.Errorf("write schema: %w", schemaErr))
			return exitInvalid
		}
		return exitPassed
	}

	corpus, err := loadCorpus(opts.corpusPath)
	if err != nil {
		writeError(stderr, err)
		return exitInvalid
	}
	thresholds, err := applyOverrides(corpus.Thresholds, opts)
	if err != nil {
		writeError(stderr, err)
		return exitInvalid
	}
	runner, err := evals.NewRunner(thresholds)
	if err != nil {
		writeError(stderr, err)
		return exitInvalid
	}
	report, err := runner.Run(corpus)
	if err != nil {
		writeError(stderr, err)
		return exitInvalid
	}
	if err := encodeJSON(stdout, report, opts.pretty); err != nil {
		writeError(stderr, fmt.Errorf("write report: %w", err))
		return exitInvalid
	}
	if !report.Passed {
		return exitFailed
	}
	return exitPassed
}

func parseOptions(args []string, output io.Writer) (options, error) {
	opts := options{}
	flags := flag.NewFlagSet("offerpilot-eval", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&opts.corpusPath, "corpus", "", "path to an external corpus; defaults to the embedded release corpus")
	flags.BoolVar(&opts.pretty, "pretty", true, "indent JSON output")
	flags.BoolVar(&opts.printSchema, "schema", false, "print the embedded corpus JSON Schema and exit")
	flags.Float64Var(&opts.maxDuplicateRate, "max-duplicate-rate", -1, "override the maximum duplicate question rate")
	flags.Float64Var(&opts.minEvidenceValidity, "min-evidence-validity", -1, "override the minimum evidence validity rate")
	flags.Float64Var(&opts.minModeCoverage, "min-mode-coverage", -1, "override the minimum mode coverage rate")
	flags.Float64Var(&opts.minSeniorityCoverage, "min-seniority-coverage", -1, "override the minimum seniority coverage rate")
	flags.Float64Var(&opts.minModeSeniorityCoverage, "min-matrix-coverage", -1, "override the minimum mode and seniority matrix coverage rate")
	flags.Float64Var(&opts.minModeConformance, "min-mode-conformance", -1, "override the minimum case mode conformance rate")
	flags.IntVar(&opts.maxPrivacyHits, "max-privacy-hits", -1, "override the maximum privacy marker hit count")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	return opts, nil
}

func loadCorpus(path string) (evals.Corpus, error) {
	if path == "" {
		return evals.LoadDefaultCorpus()
	}
	return evals.LoadCorpusFile(path)
}

func applyOverrides(thresholds evals.Thresholds, opts options) (evals.Thresholds, error) {
	floatOverrides := []struct {
		name   string
		value  float64
		target *float64
	}{
		{"max-duplicate-rate", opts.maxDuplicateRate, &thresholds.MaxDuplicateQuestionRate},
		{"min-evidence-validity", opts.minEvidenceValidity, &thresholds.MinEvidenceValidityRate},
		{"min-mode-coverage", opts.minModeCoverage, &thresholds.MinModeCoverageRate},
		{"min-seniority-coverage", opts.minSeniorityCoverage, &thresholds.MinSeniorityCoverageRate},
		{"min-matrix-coverage", opts.minModeSeniorityCoverage, &thresholds.MinModeSeniorityCoverageRate},
		{"min-mode-conformance", opts.minModeConformance, &thresholds.MinModeConformanceRate},
	}
	for _, override := range floatOverrides {
		if override.value == -1 {
			continue
		}
		if override.value < 0 || override.value > 1 {
			return evals.Thresholds{}, fmt.Errorf("-%s must be between 0 and 1", override.name)
		}
		*override.target = override.value
	}
	if opts.maxPrivacyHits != -1 {
		if opts.maxPrivacyHits < 0 {
			return evals.Thresholds{}, errors.New("-max-privacy-hits must be non-negative")
		}
		thresholds.MaxPrivacyMarkerHits = opts.maxPrivacyHits
	}
	return thresholds, nil
}

func encodeJSON(writer io.Writer, value any, pretty bool) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}

func writeError(writer io.Writer, err error) {
	_ = encodeJSON(writer, struct {
		Error string `json:"error"`
	}{Error: err.Error()}, false)
}
