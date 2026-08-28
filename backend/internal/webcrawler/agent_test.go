package webcrawler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"offerpilot/backend/internal/harness"
	"offerpilot/backend/internal/llm"
)

type noopStructuredClient struct{}

func (noopStructuredClient) ChatJSON(context.Context, []llm.Message, any) error { return nil }

type staticFetcher struct {
	request Request
	result  Result
}

type scriptedCrawlerClient struct {
	decisions []crawlDecision
	calls     int
}

func (client *scriptedCrawlerClient) ChatJSON(_ context.Context, messages []llm.Message, out any) error {
	if client.calls >= len(client.decisions) {
		return errors.New("unexpected crawler model call")
	}
	if len(messages) < 2 || !strings.Contains(messages[len(messages)-1].Content, "observations") {
		return errors.New("crawler model did not receive observations")
	}
	decision, ok := out.(*crawlDecision)
	if !ok {
		return errors.New("unexpected crawler output type")
	}
	*decision = client.decisions[client.calls]
	client.calls++
	return nil
}

type fallbackFetcher struct {
	inspectCalls  int
	resourceCalls int
	resourceURLs  []string
}

func (fetcher *fallbackFetcher) Fetch(context.Context, Request) (Result, error) {
	return Result{}, ErrNoContent
}

func (fetcher *fallbackFetcher) InspectPage(_ context.Context, request Request) (PageObservation, error) {
	fetcher.inspectCalls++
	return PageObservation{
		URL: request.URL, Title: "Careers", SourceExcerpt: `<script src="https://cdn.example/app.js"></script>`,
		ScriptURLs: []string{"https://cdn.example/app.js"},
	}, nil
}

func (fetcher *fallbackFetcher) ScanScripts(_ context.Context, request Request) (ScriptScanObservation, error) {
	fetcher.resourceCalls++
	fetcher.resourceURLs = append(fetcher.resourceURLs, "scan:"+request.URL)
	return ScriptScanObservation{
		PageURL: request.URL, ScriptsScanned: []string{"https://cdn.example/app.js"},
		Content:       `SCRIPT https://cdn.example/app.js\nconst endpoint = "/api/jobs/42";`,
		CandidateURLs: []string{"https://jobs.example/api/jobs/42"},
	}, nil
}

func (fetcher *fallbackFetcher) FetchResource(_ context.Context, request ResourceRequest) (ResourceObservation, error) {
	fetcher.resourceCalls++
	fetcher.resourceURLs = append(fetcher.resourceURLs, request.URL)
	if request.URL == "https://cdn.example/app.js" {
		return ResourceObservation{
			URL: request.URL, ContentType: "application/javascript",
			Content:       `const endpoint = "/api/jobs/42";`,
			CandidateURLs: []string{"https://jobs.example/api/jobs/42"},
		}, nil
	}
	return ResourceObservation{
		URL: request.URL, ContentType: "application/json",
		Content: `{"title":"Distributed Systems Engineer","description":"Design and operate a reliable compute platform for large-scale AI workloads, including scheduling, storage, networking, and observability.","requirements":"Strong Go and Kubernetes skills, distributed systems experience, and production incident response ownership."}`,
	}, nil
}

func (fetcher *staticFetcher) Fetch(_ context.Context, request Request) (Result, error) {
	fetcher.request = request
	return fetcher.result, nil
}

func TestAgentRegistersInHarnessWithOnlyCrawlerFunctionTool(t *testing.T) {
	runtime, err := harness.NewRuntime(noopStructuredClient{}, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &staticFetcher{result: Result{
		Text: "complete job description", Source: "https://jobs.example/1", Provider: "test",
	}}
	agent, err := NewAgent(runtime, fetcher)
	if err != nil {
		t.Fatal(err)
	}
	registered, exists := runtime.Agent(AgentID)
	if !exists || len(registered.Tools) != 1 || registered.Tools[0] != FetchWebToolName {
		t.Fatalf("registered agent=%#v exists=%v", registered, exists)
	}
	tool, exists := runtime.Tool(FetchWebToolName)
	if !exists || tool.Risk != harness.ToolRiskExternalRead {
		t.Fatalf("registered tool=%#v exists=%v", tool, exists)
	}

	result, err := agent.Crawl(context.Background(), Request{URL: "https://jobs.example/1"})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.request.URL != "https://jobs.example/1" || result.Agent != AgentID || result.Tool != FetchWebToolName || result.TraceID == "" {
		t.Fatalf("request=%#v result=%#v", fetcher.request, result)
	}
}

func TestAgentFallbackSelectsFunctionToolAndCompletesUnknownSPA(t *testing.T) {
	client := &scriptedCrawlerClient{decisions: []crawlDecision{
		{
			Action: "call_tool", Tool: ScanScriptsToolName,
			URL:    "https://jobs.example/positions/42",
			Reason: "The SPA shell exposes application scripts, so one bounded scan can reveal API candidates.",
		},
		{
			Action: "call_tool", Tool: FetchResourceToolName,
			URL:    "https://jobs.example/api/jobs/42",
			Reason: "The inspected script exposes a same-origin job API candidate.",
		},
		{
			Action: "finish", Tool: "", URL: "",
			Job: &jobDraft{
				Title:            "Distributed Systems Engineer",
				Responsibilities: "Design and operate a reliable compute platform for large-scale AI workloads, including scheduling, storage, networking, and observability.",
				Requirements:     "Strong Go and Kubernetes skills, distributed systems experience, and production incident response ownership.",
				Locations:        []string{"Shanghai"}, EmploymentType: "Full time", Organization: "Example Cloud", Identifier: "JOB-42",
			},
			Reason: "The JSON resource contains a complete title, responsibilities, and requirements.",
		},
	}}
	runtime, err := harness.NewRuntime(client, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &fallbackFetcher{}
	agent, err := NewAgentWithOptions(runtime, fetcher, AgentOptions{MaxFallbackIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	registered, _ := runtime.Agent(AgentID)
	if len(registered.Tools) != 4 {
		t.Fatalf("registered tools=%v, want four crawler tools", registered.Tools)
	}

	result, err := agent.Crawl(context.Background(), Request{URL: "https://jobs.example/positions/42"})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 || fetcher.inspectCalls != 1 || fetcher.resourceCalls != 2 {
		t.Fatalf("model=%d inspect=%d resource=%d", client.calls, fetcher.inspectCalls, fetcher.resourceCalls)
	}
	if strings.Join(fetcher.resourceURLs, ",") != "scan:https://jobs.example/positions/42,https://jobs.example/api/jobs/42" {
		t.Fatalf("resource URLs=%v", fetcher.resourceURLs)
	}
	if result.Provider != "agent-fallback" || result.Strategy != "agent_fallback" || result.Tool != FetchResourceToolName {
		t.Fatalf("result metadata=%#v", result)
	}
	if len(result.TraceIDs) != 7 || result.TraceID == "" {
		t.Fatalf("trace IDs=%v", result.TraceIDs)
	}
	for _, expected := range []string{"Distributed Systems Engineer", "岗位职责", "岗位要求与加分项", "Shanghai", "JOB-42"} {
		if !strings.Contains(result.Text, expected) {
			t.Fatalf("result text missing %q: %s", expected, result.Text)
		}
	}
}

func TestAgentFallbackRejectsUnobservedCrossDomainTarget(t *testing.T) {
	client := &scriptedCrawlerClient{decisions: []crawlDecision{
		{Action: "call_tool", Tool: FetchResourceToolName, URL: "https://unobserved.example/api/jobs/42"},
		{Action: "fail", Tool: "", URL: "", Reason: "No allowed target remains."},
	}}
	runtime, err := harness.NewRuntime(client, harness.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &fallbackFetcher{}
	agent, err := NewAgentWithOptions(runtime, fetcher, AgentOptions{MaxFallbackIterations: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = agent.Crawl(context.Background(), Request{URL: "https://jobs.example/positions/42"})
	if !errors.Is(err, ErrNoContent) {
		t.Fatalf("Crawl() error=%v, want ErrNoContent", err)
	}
	if fetcher.resourceCalls != 0 {
		t.Fatalf("unobserved host reached resource tool %d times", fetcher.resourceCalls)
	}
}
