// Package profile extracts grounded job and candidate facts from JD and
// resume documents. It owns its schema so interview and other domains can
// consume the same profile without creating package cycles.
package profile

import "context"

type SourceKind string

const (
	SourceJD     SourceKind = "jd"
	SourceResume SourceKind = "resume"
)

type DocumentInput struct {
	SourceID string `json:"sourceId,omitempty"`
	Name     string `json:"name,omitempty"`
	Text     string `json:"text"`
}

type Input struct {
	JD     DocumentInput `json:"jd"`
	Resume DocumentInput `json:"resume"`
}

type SourceDocument struct {
	ID   string     `json:"id"`
	Kind SourceKind `json:"kind"`
	Name string     `json:"name,omitempty"`
}

type SourceAnchor struct {
	ID       string     `json:"id"`
	SourceID string     `json:"sourceId"`
	Kind     SourceKind `json:"kind"`
	Locator  string     `json:"locator"`
	Text     string     `json:"text"`
}

type EvidenceRef struct {
	SourceID string     `json:"sourceId"`
	Kind     SourceKind `json:"kind"`
	AnchorID string     `json:"anchorId"`
	Locator  string     `json:"locator"`
	Quote    string     `json:"quote"`
}

// Fact is intentionally extractive in v0.3.0-alpha.1: Value must occur in at
// least one canonical evidence quote. Semantic normalization can be added as a
// separately versioned projection without weakening the grounding contract.
type Fact struct {
	ID           string        `json:"id"`
	Value        string        `json:"value"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
}

type JobProfile struct {
	Title               *Fact  `json:"title,omitempty"`
	SenioritySignals    []Fact `json:"senioritySignals"`
	MustHave            []Fact `json:"mustHave"`
	NiceToHave          []Fact `json:"niceToHave"`
	Responsibilities    []Fact `json:"responsibilities"`
	TechnicalTopics     []Fact `json:"technicalTopics"`
	BusinessConstraints []Fact `json:"businessConstraints"`
}

type ProjectProfile struct {
	ID               string `json:"id"`
	Name             Fact   `json:"name"`
	Responsibilities []Fact `json:"responsibilities"`
	Metrics          []Fact `json:"metrics"`
	Technologies     []Fact `json:"technologies"`
	Highlights       []Fact `json:"highlights"`
}

type CandidateProfile struct {
	Headline         *Fact            `json:"headline,omitempty"`
	Skills           []Fact           `json:"skills"`
	Projects         []ProjectProfile `json:"projects"`
	Responsibilities []Fact           `json:"responsibilities"`
	Metrics          []Fact           `json:"metrics"`
}

type Profile struct {
	Job       JobProfile       `json:"job"`
	Candidate CandidateProfile `json:"candidate"`
}

// AgentRequest is the complete evidence universe available to a semantic
// extractor. An Agent may only cite anchors in this request.
type AgentRequest struct {
	Documents []SourceDocument `json:"documents"`
	Anchors   []SourceAnchor   `json:"anchors"`
}

// ProfileExtractor is the application port used by future interview,
// matching, and diagnosis services.
type ProfileExtractor interface {
	Extract(context.Context, Input) (Profile, error)
}

// AgentPort allows a model-backed extractor to propose the same typed schema.
// AgentExtractor validates every proposal against AgentRequest before return.
type AgentPort interface {
	ProposeProfile(context.Context, AgentRequest) (Profile, error)
}
