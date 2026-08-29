package resumediagnosis

import "context"

type Request struct {
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type SectionDiagnosis struct {
	Section     string   `json:"section"`
	Score       int      `json:"score"`
	Evidence    []string `json:"evidence"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
	Rewrite     string   `json:"rewrite"`
}

type LayoutAssessment struct {
	Score       int      `json:"score"`
	Summary     string   `json:"summary"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
}

type Result struct {
	OverallScore int                `json:"overallScore"`
	Summary      string             `json:"summary"`
	Strengths    []string           `json:"strengths"`
	Risks        []string           `json:"risks"`
	Diagnosis    []SectionDiagnosis `json:"diagnosis"`
	Layout       LayoutAssessment   `json:"layout"`
	Mode         string             `json:"mode"`
	Agent        string             `json:"agent,omitempty"`
	TraceID      string             `json:"traceId,omitempty"`
	TraceIDs     []string           `json:"traceIds,omitempty"`
}

type Diagnostician interface {
	Diagnose(context.Context, Request) (Result, error)
}
