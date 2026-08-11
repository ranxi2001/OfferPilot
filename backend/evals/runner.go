package evals

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	CorpusSchemaVersion = "1.0.0"
	ReportSchemaVersion = "1.0.0"
)

var (
	validModes             = stringSet("knowledge", "projects", "mixed")
	validSeniorities       = stringSet("junior", "mid", "senior")
	validKinds             = stringSet("knowledge", "project", "behavioral", "follow_up", "prerequisite")
	validDifficulties      = stringSet("easy", "medium", "hard")
	validEvidenceKinds     = stringSet("jd", "resume", "knowledge")
	standardPrivacyMarkers = []string{
		"参考内容", "参考答案", "reference content", "reference answer",
	}
)

type Runner struct {
	thresholds Thresholds
}

func NewRunner(thresholds Thresholds) (*Runner, error) {
	if err := validateThresholds(thresholds); err != nil {
		return nil, err
	}
	return &Runner{thresholds: thresholds}, nil
}

func (r *Runner) Run(corpus Corpus) (Report, error) {
	if r == nil {
		return Report{}, errors.New("evals: runner is nil")
	}
	if err := ValidateCorpus(corpus); err != nil {
		return Report{}, err
	}

	report := Report{
		SchemaVersion: ReportSchemaVersion,
		CorpusVersion: corpus.CorpusVersion,
		Thresholds:    r.thresholds,
		Cases:         make([]CaseResult, 0, len(corpus.Cases)),
		Failures:      make([]MetricFailure, 0),
	}
	metrics := &report.Metrics
	metrics.CaseCount = len(corpus.Cases)
	coveredModes := make(map[string]struct{})
	coveredSeniorities := make(map[string]struct{})
	coveredPairs := make(map[string]struct{})
	seenQuestions := make(map[string]string)

	for _, evalCase := range corpus.Cases {
		caseResult := evaluateCase(evalCase, metrics, seenQuestions)
		report.Cases = append(report.Cases, caseResult)
		coveredModes[evalCase.Mode] = struct{}{}
		coveredSeniorities[evalCase.Seniority] = struct{}{}
		coveredPairs[pairKey(evalCase.Mode, evalCase.Seniority)] = struct{}{}
	}

	metrics.DuplicateQuestionRate = ratio(metrics.DuplicateQuestionCount, metrics.QuestionCount)
	metrics.EvidenceValidityRate = ratio(metrics.ValidEvidenceQuestionCount, metrics.EvidenceQuestionCount)
	metrics.CoveredModes, metrics.ModeCoverageRate = coverage(corpus.RequiredCoverage.Modes, coveredModes)
	metrics.CoveredSeniorities, metrics.SeniorityCoverageRate = coverage(corpus.RequiredCoverage.Seniorities, coveredSeniorities)
	requiredPairs := make([]string, 0, len(corpus.RequiredCoverage.Modes)*len(corpus.RequiredCoverage.Seniorities))
	for _, mode := range corpus.RequiredCoverage.Modes {
		for _, seniority := range corpus.RequiredCoverage.Seniorities {
			requiredPairs = append(requiredPairs, pairKey(mode, seniority))
		}
	}
	metrics.CoveredModeSeniorityPairs, metrics.ModeSeniorityCoverageRate = coverage(requiredPairs, coveredPairs)
	metrics.ModeConformanceRate = ratio(metrics.ModeConformantCaseCount, metrics.CaseCount)
	metrics.PrivacyLeakFreeRate = ratio(metrics.PrivacyFieldCount-metrics.PrivacyFieldsWithHits, metrics.PrivacyFieldCount)

	report.Failures = thresholdFailures(*metrics, r.thresholds)
	report.Passed = len(report.Failures) == 0
	return report, nil
}

func ValidateCorpus(corpus Corpus) error {
	if corpus.SchemaVersion != CorpusSchemaVersion {
		return fmt.Errorf("evals: schemaVersion must be %q", CorpusSchemaVersion)
	}
	if strings.TrimSpace(corpus.CorpusVersion) == "" {
		return errors.New("evals: corpusVersion is required")
	}
	if strings.TrimSpace(corpus.Description) == "" {
		return errors.New("evals: description is required")
	}
	if err := validateThresholds(corpus.Thresholds); err != nil {
		return err
	}
	if err := validateRequiredValues("requiredCoverage.modes", corpus.RequiredCoverage.Modes, validModes); err != nil {
		return err
	}
	if err := validateRequiredValues("requiredCoverage.seniorities", corpus.RequiredCoverage.Seniorities, validSeniorities); err != nil {
		return err
	}
	if len(corpus.Cases) == 0 {
		return errors.New("evals: at least one case is required")
	}

	caseIDs := make(map[string]struct{}, len(corpus.Cases))
	for caseIndex, evalCase := range corpus.Cases {
		path := fmt.Sprintf("cases[%d]", caseIndex)
		if err := requireUniqueID(path+".id", evalCase.ID, caseIDs); err != nil {
			return err
		}
		if strings.TrimSpace(evalCase.Title) == "" {
			return fmt.Errorf("evals: %s.title is required", path)
		}
		if _, ok := validModes[evalCase.Mode]; !ok {
			return fmt.Errorf("evals: %s.mode %q is invalid", path, evalCase.Mode)
		}
		if _, ok := validSeniorities[evalCase.Seniority]; !ok {
			return fmt.Errorf("evals: %s.seniority %q is invalid", path, evalCase.Seniority)
		}
		if len(evalCase.EvidenceCatalog) == 0 {
			return fmt.Errorf("evals: %s.evidenceCatalog must not be empty", path)
		}
		evidenceIDs := make(map[string]struct{}, len(evalCase.EvidenceCatalog))
		for evidenceIndex, evidence := range evalCase.EvidenceCatalog {
			evidencePath := fmt.Sprintf("%s.evidenceCatalog[%d]", path, evidenceIndex)
			if err := requireUniqueID(evidencePath+".id", evidence.ID, evidenceIDs); err != nil {
				return err
			}
			if _, ok := validEvidenceKinds[evidence.Kind]; !ok {
				return fmt.Errorf("evals: %s.kind %q is invalid", evidencePath, evidence.Kind)
			}
			if strings.TrimSpace(evidence.Label) == "" {
				return fmt.Errorf("evals: %s.label is required", evidencePath)
			}
		}
		if len(evalCase.PrivateMarkers) == 0 {
			return fmt.Errorf("evals: %s.privateMarkers must not be empty", path)
		}
		for markerIndex, marker := range evalCase.PrivateMarkers {
			if normalizeComparable(marker) == "" {
				return fmt.Errorf("evals: %s.privateMarkers[%d] is empty after normalization", path, markerIndex)
			}
		}
		if len(evalCase.Questions) == 0 {
			return fmt.Errorf("evals: %s.questions must not be empty", path)
		}
		questionIDs := make(map[string]struct{}, len(evalCase.Questions))
		for questionIndex, question := range evalCase.Questions {
			questionPath := fmt.Sprintf("%s.questions[%d]", path, questionIndex)
			if err := requireUniqueID(questionPath+".id", question.ID, questionIDs); err != nil {
				return err
			}
			if normalizeComparable(question.Text) == "" {
				return fmt.Errorf("evals: %s.text is empty after normalization", questionPath)
			}
			if _, ok := validKinds[question.Kind]; !ok {
				return fmt.Errorf("evals: %s.kind %q is invalid", questionPath, question.Kind)
			}
			if _, ok := validDifficulties[question.Difficulty]; !ok {
				return fmt.Errorf("evals: %s.difficulty %q is invalid", questionPath, question.Difficulty)
			}
		}
		publicTextIDs := make(map[string]struct{}, len(evalCase.PublicTexts))
		for textIndex, publicText := range evalCase.PublicTexts {
			textPath := fmt.Sprintf("%s.publicTexts[%d]", path, textIndex)
			if err := requireUniqueID(textPath+".id", publicText.ID, publicTextIDs); err != nil {
				return err
			}
			if strings.TrimSpace(publicText.Text) == "" {
				return fmt.Errorf("evals: %s.text is required", textPath)
			}
		}
	}
	return nil
}

func evaluateCase(evalCase Case, metrics *Metrics, seenQuestions map[string]string) CaseResult {
	result := CaseResult{
		ID:        evalCase.ID,
		Mode:      evalCase.Mode,
		Seniority: evalCase.Seniority,
		Findings:  make([]Finding, 0),
	}
	evidence := make(map[string]Evidence, len(evalCase.EvidenceCatalog))
	for _, item := range evalCase.EvidenceCatalog {
		evidence[item.ID] = item
	}

	for questionIndex, question := range evalCase.Questions {
		metrics.QuestionCount++
		metrics.EvidenceQuestionCount++
		questionPath := fmt.Sprintf("questions[%d]", questionIndex)
		normalized := normalizeComparable(question.Text)
		if previousID, exists := seenQuestions[normalized]; exists {
			metrics.DuplicateQuestionCount++
			result.Findings = append(result.Findings, Finding{
				Code:      "duplicate_question",
				Path:      questionPath + ".text",
				Message:   "question duplicates an earlier normalized question in the corpus",
				RelatedID: previousID,
			})
		} else {
			seenQuestions[normalized] = evalCase.ID + "/" + question.ID
		}

		validQuestionEvidence := true
		if len(question.EvidenceRefIDs) == 0 {
			validQuestionEvidence = false
			metrics.MissingEvidenceQuestionCount++
			result.Findings = append(result.Findings, Finding{
				Code:    "missing_evidence",
				Path:    questionPath + ".evidenceRefIds",
				Message: "question has no evidence reference",
			})
		}
		seenRefs := make(map[string]struct{}, len(question.EvidenceRefIDs))
		resolvedKinds := make(map[string]struct{})
		for refIndex, refID := range question.EvidenceRefIDs {
			metrics.EvidenceRefCount++
			refPath := fmt.Sprintf("%s.evidenceRefIds[%d]", questionPath, refIndex)
			if _, duplicate := seenRefs[refID]; duplicate {
				validQuestionEvidence = false
				metrics.InvalidEvidenceRefCount++
				result.Findings = append(result.Findings, Finding{
					Code:      "duplicate_evidence_ref",
					Path:      refPath,
					Message:   "evidence reference is repeated within the question",
					RelatedID: refID,
				})
				continue
			}
			seenRefs[refID] = struct{}{}
			item, exists := evidence[refID]
			if !exists {
				validQuestionEvidence = false
				metrics.InvalidEvidenceRefCount++
				result.Findings = append(result.Findings, Finding{
					Code:      "unknown_evidence_ref",
					Path:      refPath,
					Message:   "evidence reference is absent from the case catalog",
					RelatedID: refID,
				})
				continue
			}
			metrics.ValidEvidenceRefCount++
			resolvedKinds[item.Kind] = struct{}{}
		}
		if len(question.EvidenceRefIDs) > 0 && !evidenceKindCompatible(question.Kind, resolvedKinds) {
			validQuestionEvidence = false
			metrics.EvidenceKindMismatchCount++
			result.Findings = append(result.Findings, Finding{
				Code:    "evidence_kind_mismatch",
				Path:    questionPath + ".evidenceRefIds",
				Message: "resolved evidence kinds do not support the question kind",
			})
		}
		if validQuestionEvidence {
			metrics.ValidEvidenceQuestionCount++
		}
	}

	if modeConformant(evalCase) {
		metrics.ModeConformantCaseCount++
	} else {
		result.Findings = append(result.Findings, Finding{
			Code:    "mode_conformance",
			Path:    "questions",
			Message: "question kinds do not cover the case mode",
		})
	}

	markers := uniqueNormalizedMarkers(append(append([]string{}, standardPrivacyMarkers...), evalCase.PrivateMarkers...))
	privacyFields := make([]struct{ path, text string }, 0, len(evalCase.Questions)+len(evalCase.PublicTexts))
	for index, question := range evalCase.Questions {
		privacyFields = append(privacyFields, struct{ path, text string }{
			path: fmt.Sprintf("questions[%d].text", index), text: question.Text,
		})
	}
	for index, publicText := range evalCase.PublicTexts {
		privacyFields = append(privacyFields, struct{ path, text string }{
			path: fmt.Sprintf("publicTexts[%d].text", index), text: publicText.Text,
		})
	}
	metrics.PublicTextCount += len(evalCase.PublicTexts)
	metrics.PrivacyFieldCount += len(privacyFields)
	for _, field := range privacyFields {
		normalized := normalizeComparable(field.text)
		fieldHasHit := false
		for _, marker := range markers {
			if !strings.Contains(normalized, marker.normalized) {
				continue
			}
			fieldHasHit = true
			metrics.PrivacyMarkerHitCount++
			result.Findings = append(result.Findings, Finding{
				Code:              "privacy_marker",
				Path:              field.path,
				Message:           "public text contains a normalized private marker",
				MarkerFingerprint: marker.fingerprint,
			})
		}
		if fieldHasHit {
			metrics.PrivacyFieldsWithHits++
		}
	}

	result.Passed = len(result.Findings) == 0
	return result
}

func thresholdFailures(metrics Metrics, thresholds Thresholds) []MetricFailure {
	failures := make([]MetricFailure, 0)
	checkMax := func(metric string, actual, threshold float64) {
		if actual <= threshold {
			return
		}
		failures = append(failures, MetricFailure{Metric: metric, Comparator: "<=", Actual: actual, Threshold: threshold, Message: "metric exceeds maximum threshold"})
	}
	checkMin := func(metric string, actual, threshold float64) {
		if actual >= threshold {
			return
		}
		failures = append(failures, MetricFailure{Metric: metric, Comparator: ">=", Actual: actual, Threshold: threshold, Message: "metric is below minimum threshold"})
	}
	checkMax("duplicateQuestionRate", metrics.DuplicateQuestionRate, thresholds.MaxDuplicateQuestionRate)
	checkMin("evidenceValidityRate", metrics.EvidenceValidityRate, thresholds.MinEvidenceValidityRate)
	checkMin("modeCoverageRate", metrics.ModeCoverageRate, thresholds.MinModeCoverageRate)
	checkMin("seniorityCoverageRate", metrics.SeniorityCoverageRate, thresholds.MinSeniorityCoverageRate)
	checkMin("modeSeniorityCoverageRate", metrics.ModeSeniorityCoverageRate, thresholds.MinModeSeniorityCoverageRate)
	checkMin("modeConformanceRate", metrics.ModeConformanceRate, thresholds.MinModeConformanceRate)
	checkMax("privacyMarkerHitCount", float64(metrics.PrivacyMarkerHitCount), float64(thresholds.MaxPrivacyMarkerHits))
	return failures
}

func validateThresholds(thresholds Thresholds) error {
	checks := []struct {
		name  string
		value float64
	}{
		{"maxDuplicateQuestionRate", thresholds.MaxDuplicateQuestionRate},
		{"minEvidenceValidityRate", thresholds.MinEvidenceValidityRate},
		{"minModeCoverageRate", thresholds.MinModeCoverageRate},
		{"minSeniorityCoverageRate", thresholds.MinSeniorityCoverageRate},
		{"minModeSeniorityCoverageRate", thresholds.MinModeSeniorityCoverageRate},
		{"minModeConformanceRate", thresholds.MinModeConformanceRate},
	}
	for _, check := range checks {
		if !(check.value >= 0 && check.value <= 1) {
			return fmt.Errorf("evals: threshold %s must be between 0 and 1", check.name)
		}
	}
	if thresholds.MaxPrivacyMarkerHits < 0 {
		return errors.New("evals: threshold maxPrivacyMarkerHits must be non-negative")
	}
	return nil
}

func validateRequiredValues(path string, values []string, allowed map[string]struct{}) error {
	if len(values) == 0 {
		return fmt.Errorf("evals: %s must not be empty", path)
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if _, ok := allowed[value]; !ok {
			return fmt.Errorf("evals: %s[%d] %q is invalid", path, index, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("evals: %s contains duplicate value %q", path, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func requireUniqueID(path, value string, seen map[string]struct{}) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("evals: %s is required", path)
	}
	if _, exists := seen[value]; exists {
		return fmt.Errorf("evals: %s %q is duplicated", path, value)
	}
	seen[value] = struct{}{}
	return nil
}

func evidenceKindCompatible(questionKind string, kinds map[string]struct{}) bool {
	switch questionKind {
	case "knowledge", "prerequisite":
		_, ok := kinds["knowledge"]
		return ok
	case "project", "behavioral":
		_, resume := kinds["resume"]
		_, jd := kinds["jd"]
		return resume || jd
	case "follow_up":
		return len(kinds) > 0
	default:
		return false
	}
}

func modeConformant(evalCase Case) bool {
	hasKnowledge := false
	hasProject := false
	for _, question := range evalCase.Questions {
		switch question.Kind {
		case "knowledge", "prerequisite":
			hasKnowledge = true
		case "project", "behavioral":
			hasProject = true
		}
	}
	switch evalCase.Mode {
	case "knowledge":
		return hasKnowledge && !hasProject
	case "projects":
		return hasProject && !hasKnowledge
	case "mixed":
		return hasKnowledge && hasProject
	default:
		return false
	}
}

func coverage(required []string, covered map[string]struct{}) ([]string, float64) {
	result := make([]string, 0, len(required))
	for _, value := range required {
		if _, exists := covered[value]; exists {
			result = append(result, value)
		}
	}
	return result, ratio(len(result), len(required))
}

func pairKey(mode, seniority string) string {
	return mode + ":" + seniority
}

func ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func normalizeComparable(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r):
			return unicode.ToLower(r)
		case unicode.IsNumber(r):
			return r
		default:
			return -1
		}
	}, value)
}

type normalizedMarker struct {
	normalized  string
	fingerprint string
}

func uniqueNormalizedMarkers(markers []string) []normalizedMarker {
	fingerprints := make(map[string]string, len(markers))
	for _, marker := range markers {
		normalized := normalizeComparable(marker)
		if normalized == "" {
			continue
		}
		if _, exists := fingerprints[normalized]; exists {
			continue
		}
		hash := sha256.Sum256([]byte(marker))
		fingerprints[normalized] = hex.EncodeToString(hash[:6])
	}
	keys := make([]string, 0, len(fingerprints))
	for normalized := range fingerprints {
		keys = append(keys, normalized)
	}
	sort.Strings(keys)
	result := make([]normalizedMarker, 0, len(keys))
	for _, normalized := range keys {
		result = append(result, normalizedMarker{
			normalized: normalized, fingerprint: fingerprints[normalized],
		})
	}
	return result
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
