package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"offerpilot/backend/internal/resumediagnosis"
)

type resumeDiagnosticianStub struct {
	request resumediagnosis.Request
	result  resumediagnosis.Result
	err     error
}

func (stub *resumeDiagnosticianStub) Diagnose(_ context.Context, request resumediagnosis.Request) (resumediagnosis.Result, error) {
	stub.request = request
	return stub.result, stub.err
}

func TestResumeDiagnosisEndpointReturnsMultimodalAgentResult(t *testing.T) {
	diagnostician := &resumeDiagnosticianStub{result: resumediagnosis.Result{
		OverallScore: 82, Mode: "multimodal", Agent: resumediagnosis.AgentID, TraceID: "trace-1",
	}}
	server, err := New(Config{}, Dependencies{Interview: &interviewStub{}, ResumeDiagnostician: diagnostician})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/resume/diagnose", strings.NewReader(
		`{"content":"  中文简历  ","images":["data:image/jpeg;base64,abc"]}`,
	))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if diagnostician.request.Content != "中文简历" || len(diagnostician.request.Images) != 1 {
		t.Fatalf("request=%#v", diagnostician.request)
	}
	if !strings.Contains(response.Body.String(), `"agent":"resume_diagnostician"`) || !strings.Contains(response.Body.String(), `"overallScore":82`) {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestResumeDiagnosisEndpointValidatesImagesAndFailsClosed(t *testing.T) {
	t.Run("invalid image", func(t *testing.T) {
		server, err := New(Config{}, Dependencies{Interview: &interviewStub{}, ResumeDiagnostician: &resumeDiagnosticianStub{}})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/resume/diagnose", strings.NewReader(
			`{"content":"resume","images":["https://attacker.example/image.jpg"]}`,
		)))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_image") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("agent failure", func(t *testing.T) {
		server, err := New(Config{}, Dependencies{
			Interview: &interviewStub{}, ResumeDiagnostician: &resumeDiagnosticianStub{err: errors.New("SECRET_VISION_FAILURE")},
		})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/resume/diagnose", strings.NewReader(`{"content":"resume"}`)))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "resume_diagnosis_failed") || strings.Contains(response.Body.String(), "SECRET_VISION_FAILURE") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}
