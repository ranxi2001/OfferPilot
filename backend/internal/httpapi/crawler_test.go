package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"offerpilot/backend/internal/webcrawler"
)

type crawlerStub struct {
	request webcrawler.Request
	result  webcrawler.Result
	err     error
}

func (stub *crawlerStub) Crawl(_ context.Context, request webcrawler.Request) (webcrawler.Result, error) {
	stub.request = request
	return stub.result, stub.err
}

func TestCrawlEndpointReturnsAgentResult(t *testing.T) {
	crawler := &crawlerStub{result: webcrawler.Result{
		Text: "完整职位描述", Title: "Agent Infra工程师", Source: "https://jobs.example/1",
		Provider: "test", Agent: webcrawler.AgentID, Tool: webcrawler.FetchWebToolName, TraceID: "trace-1",
	}}
	server, err := New(Config{}, Dependencies{Interview: &interviewStub{}, Crawler: crawler})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/crawl", strings.NewReader(`{"url":"https://jobs.example/1"}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if crawler.request.URL != "https://jobs.example/1" {
		t.Fatalf("request=%#v", crawler.request)
	}
	var result webcrawler.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Agent != webcrawler.AgentID || result.Tool != webcrawler.FetchWebToolName || result.Text != "完整职位描述" {
		t.Fatalf("result=%#v", result)
	}
}

func TestCrawlEndpointMapsCrawlerErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "blocked", err: ErrWrap(webcrawler.ErrBlockedURL), status: http.StatusBadRequest, code: "url_not_allowed"},
		{name: "large", err: ErrWrap(webcrawler.ErrResponseLarge), status: http.StatusRequestEntityTooLarge, code: "page_too_large"},
		{name: "empty", err: ErrWrap(webcrawler.ErrNoContent), status: http.StatusUnprocessableEntity, code: "page_content_missing"},
		{name: "upstream", err: ErrWrap(webcrawler.ErrUpstreamFailed), status: http.StatusBadGateway, code: "crawl_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, err := New(Config{}, Dependencies{Interview: &interviewStub{}, Crawler: &crawlerStub{err: test.err}})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/crawl", strings.NewReader(`{"url":"https://jobs.example/1"}`))
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func ErrWrap(err error) error {
	return errors.Join(errors.New("harness wrapper"), err)
}
