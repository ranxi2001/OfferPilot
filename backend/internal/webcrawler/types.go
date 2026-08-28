package webcrawler

import (
	"context"
	"errors"
)

var (
	ErrInvalidURL     = errors.New("webcrawler: invalid URL")
	ErrBlockedURL     = errors.New("webcrawler: URL is not public")
	ErrResponseLarge  = errors.New("webcrawler: response is too large")
	ErrNoContent      = errors.New("webcrawler: no useful page content")
	ErrUpstreamFailed = errors.New("webcrawler: upstream request failed")
)

type Request struct {
	URL string `json:"url"`
}

type Result struct {
	Text     string   `json:"text"`
	Title    string   `json:"title,omitempty"`
	Source   string   `json:"source"`
	Provider string   `json:"provider"`
	Strategy string   `json:"strategy,omitempty"`
	Agent    string   `json:"agent,omitempty"`
	Tool     string   `json:"tool,omitempty"`
	TraceID  string   `json:"traceId,omitempty"`
	TraceIDs []string `json:"traceIds,omitempty"`
}

type ContentFetcher interface {
	Fetch(context.Context, Request) (Result, error)
}

type PageObservation struct {
	URL           string   `json:"url"`
	Title         string   `json:"title,omitempty"`
	Text          string   `json:"text,omitempty"`
	ContentType   string   `json:"contentType,omitempty"`
	ScriptURLs    []string `json:"scriptUrls,omitempty"`
	CandidateURLs []string `json:"candidateUrls,omitempty"`
	SourceExcerpt string   `json:"sourceExcerpt,omitempty"`
}

type ResourceRequest struct {
	URL     string `json:"url"`
	Referer string `json:"referer,omitempty"`
}

type ResourceObservation struct {
	URL           string   `json:"url"`
	ContentType   string   `json:"contentType,omitempty"`
	Content       string   `json:"content"`
	CandidateURLs []string `json:"candidateUrls,omitempty"`
	APITemplates  []string `json:"apiTemplates,omitempty"`
}

type ScriptScanObservation struct {
	PageURL        string   `json:"pageUrl"`
	ScriptsScanned []string `json:"scriptsScanned"`
	Content        string   `json:"content"`
	CandidateURLs  []string `json:"candidateUrls,omitempty"`
	APITemplates   []string `json:"apiTemplates,omitempty"`
}

type FallbackTools interface {
	InspectPage(context.Context, Request) (PageObservation, error)
	ScanScripts(context.Context, Request) (ScriptScanObservation, error)
	FetchResource(context.Context, ResourceRequest) (ResourceObservation, error)
}
