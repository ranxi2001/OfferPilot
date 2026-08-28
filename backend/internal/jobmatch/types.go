package jobmatch

import "context"

type Request struct {
	JD     string `json:"jd"`
	Resume string `json:"resume"`
}

type Breakdown struct {
	MustHave         int `json:"mustHave"`
	Responsibilities int `json:"responsibilities"`
	EvidenceQuality  int `json:"evidenceQuality"`
	Bonus            int `json:"bonus"`
}

type Evidence struct {
	Requirement    string `json:"requirement"`
	ResumeEvidence string `json:"resumeEvidence"`
	Verdict        string `json:"verdict"`
}

type Result struct {
	Score       int        `json:"score"`
	Matched     []string   `json:"matched"`
	Missing     []string   `json:"missing"`
	Suggestions []string   `json:"suggestions"`
	Level       string     `json:"level"`
	Focus       []string   `json:"focus"`
	Summary     string     `json:"summary"`
	Breakdown   Breakdown  `json:"breakdown"`
	Evidence    []Evidence `json:"evidence"`
	Agent       string     `json:"agent,omitempty"`
	TraceID     string     `json:"traceId,omitempty"`
	TraceIDs    []string   `json:"traceIds,omitempty"`
}

type Matcher interface {
	Match(context.Context, Request) (Result, error)
}
