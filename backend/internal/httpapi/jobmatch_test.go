package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"offerpilot/backend/internal/jobmatch"
)

type matcherStub struct {
	request jobmatch.Request
	result  jobmatch.Result
	err     error
}

func (stub *matcherStub) Match(_ context.Context, request jobmatch.Request) (jobmatch.Result, error) {
	stub.request = request
	return stub.result, stub.err
}

func TestMatchEndpointReturnsHarnessResult(t *testing.T) {
	matcher := &matcherStub{result: jobmatch.Result{
		Score: 78, Matched: []string{"Agent Runtime：具备生产级沙箱开发经验"},
		Missing:     []string{"博士学历：简历当前为硕士在读"},
		Suggestions: []string{"准备高性能网络基础并诚实说明直接经验边界"},
		Level:       "博士专项校招岗位", Focus: []string{"AI Infra"}, Agent: jobmatch.AgentID, TraceID: "trace-1",
	}}
	server, err := New(Config{}, Dependencies{Interview: &interviewStub{}, Matcher: matcher})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/match", strings.NewReader(`{"jd":"  完整 JD  ","resume":"  中文简历  "}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if matcher.request.JD != "完整 JD" || matcher.request.Resume != "中文简历" {
		t.Fatalf("request=%#v", matcher.request)
	}
	if !strings.Contains(response.Body.String(), `"agent":"resume_matcher"`) || !strings.Contains(response.Body.String(), `"score":78`) {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestMatchEndpointFailsClosedWithoutMatcherOrOnAgentError(t *testing.T) {
	t.Run("unconfigured", func(t *testing.T) {
		server := newTestServer(t, Config{}, &interviewStub{})
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/match", strings.NewReader(`{"jd":"jd","resume":"resume"}`)))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "matcher_unavailable") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("agent failure", func(t *testing.T) {
		server, err := New(Config{}, Dependencies{
			Interview: &interviewStub{}, Matcher: &matcherStub{err: errors.New("SECRET_MODEL_FAILURE")},
		})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/match", strings.NewReader(`{"jd":"jd","resume":"resume"}`)))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "matching_failed") || strings.Contains(response.Body.String(), "SECRET_MODEL_FAILURE") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}
