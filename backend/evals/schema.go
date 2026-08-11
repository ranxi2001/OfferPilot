// Package evals provides a provider-free, deterministic quality gate for
// recorded interview outputs. It intentionally has no dependency on the
// production interview service or any model client.
package evals

type Corpus struct {
	SchemaVersion    string               `json:"schemaVersion"`
	CorpusVersion    string               `json:"corpusVersion"`
	Description      string               `json:"description"`
	RequiredCoverage CoverageRequirements `json:"requiredCoverage"`
	Thresholds       Thresholds           `json:"thresholds"`
	Cases            []Case               `json:"cases"`
}

type CoverageRequirements struct {
	Modes       []string `json:"modes"`
	Seniorities []string `json:"seniorities"`
}

type Thresholds struct {
	MaxDuplicateQuestionRate     float64 `json:"maxDuplicateQuestionRate"`
	MinEvidenceValidityRate      float64 `json:"minEvidenceValidityRate"`
	MinModeCoverageRate          float64 `json:"minModeCoverageRate"`
	MinSeniorityCoverageRate     float64 `json:"minSeniorityCoverageRate"`
	MinModeSeniorityCoverageRate float64 `json:"minModeSeniorityCoverageRate"`
	MinModeConformanceRate       float64 `json:"minModeConformanceRate"`
	MaxPrivacyMarkerHits         int     `json:"maxPrivacyMarkerHits"`
}

type Case struct {
	ID              string       `json:"id"`
	Title           string       `json:"title"`
	Mode            string       `json:"mode"`
	Seniority       string       `json:"seniority"`
	EvidenceCatalog []Evidence   `json:"evidenceCatalog"`
	PrivateMarkers  []string     `json:"privateMarkers"`
	Questions       []Question   `json:"questions"`
	PublicTexts     []PublicText `json:"publicTexts"`
}

type Evidence struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type Question struct {
	ID             string   `json:"id"`
	Text           string   `json:"text"`
	Kind           string   `json:"kind"`
	Difficulty     string   `json:"difficulty"`
	EvidenceRefIDs []string `json:"evidenceRefIds"`
}

type PublicText struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Report struct {
	SchemaVersion string          `json:"schemaVersion"`
	CorpusVersion string          `json:"corpusVersion"`
	Passed        bool            `json:"passed"`
	Thresholds    Thresholds      `json:"thresholds"`
	Metrics       Metrics         `json:"metrics"`
	Cases         []CaseResult    `json:"cases"`
	Failures      []MetricFailure `json:"failures"`
}

type Metrics struct {
	CaseCount                    int      `json:"caseCount"`
	QuestionCount                int      `json:"questionCount"`
	PublicTextCount              int      `json:"publicTextCount"`
	DuplicateQuestionCount       int      `json:"duplicateQuestionCount"`
	DuplicateQuestionRate        float64  `json:"duplicateQuestionRate"`
	EvidenceQuestionCount        int      `json:"evidenceQuestionCount"`
	ValidEvidenceQuestionCount   int      `json:"validEvidenceQuestionCount"`
	EvidenceValidityRate         float64  `json:"evidenceValidityRate"`
	EvidenceRefCount             int      `json:"evidenceRefCount"`
	ValidEvidenceRefCount        int      `json:"validEvidenceRefCount"`
	MissingEvidenceQuestionCount int      `json:"missingEvidenceQuestionCount"`
	InvalidEvidenceRefCount      int      `json:"invalidEvidenceRefCount"`
	EvidenceKindMismatchCount    int      `json:"evidenceKindMismatchCount"`
	CoveredModes                 []string `json:"coveredModes"`
	ModeCoverageRate             float64  `json:"modeCoverageRate"`
	CoveredSeniorities           []string `json:"coveredSeniorities"`
	SeniorityCoverageRate        float64  `json:"seniorityCoverageRate"`
	CoveredModeSeniorityPairs    []string `json:"coveredModeSeniorityPairs"`
	ModeSeniorityCoverageRate    float64  `json:"modeSeniorityCoverageRate"`
	ModeConformantCaseCount      int      `json:"modeConformantCaseCount"`
	ModeConformanceRate          float64  `json:"modeConformanceRate"`
	PrivacyFieldCount            int      `json:"privacyFieldCount"`
	PrivacyMarkerHitCount        int      `json:"privacyMarkerHitCount"`
	PrivacyFieldsWithHits        int      `json:"privacyFieldsWithHits"`
	PrivacyLeakFreeRate          float64  `json:"privacyLeakFreeRate"`
}

type CaseResult struct {
	ID        string    `json:"id"`
	Mode      string    `json:"mode"`
	Seniority string    `json:"seniority"`
	Passed    bool      `json:"passed"`
	Findings  []Finding `json:"findings"`
}

type Finding struct {
	Code              string `json:"code"`
	Path              string `json:"path"`
	Message           string `json:"message"`
	RelatedID         string `json:"relatedId,omitempty"`
	MarkerFingerprint string `json:"markerFingerprint,omitempty"`
}

type MetricFailure struct {
	Metric     string  `json:"metric"`
	Comparator string  `json:"comparator"`
	Actual     float64 `json:"actual"`
	Threshold  float64 `json:"threshold"`
	Message    string  `json:"message"`
}
