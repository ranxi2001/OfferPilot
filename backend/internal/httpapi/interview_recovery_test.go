package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"offerpilot/backend/internal/interview"
)

type recoveryInterviewStub struct {
	interviewStub
	snapshot interview.SessionSnapshot
	review   interview.ReviewSnapshot
	events   interview.SessionEventPage
}

func (s *recoveryInterviewStub) Review(_ context.Context, id string) (interview.ReviewSnapshot, error) {
	result := s.review
	result.InterviewID = id
	return result, nil
}

func (s *recoveryInterviewStub) Snapshot(_ context.Context, id string) (interview.SessionSnapshot, error) {
	result := s.snapshot
	result.InterviewID = id
	return result, nil
}

func (s *recoveryInterviewStub) SessionEvents(_ context.Context, id string, after int64, _ int) (interview.SessionEventPage, error) {
	result := s.events
	result.InterviewID = id
	result.NextSequence = after + int64(len(result.Events))
	return result, nil
}

func TestInterviewRecoveryHandlersMapSnapshotAndEventCursor(t *testing.T) {
	service := &recoveryInterviewStub{
		snapshot: interview.SessionSnapshot{
			State:           interview.StateAwaitingAnswer,
			Profile:         interview.Profile{Coverage: []interview.CoveragePoint{{ID: "coverage-1", Area: interview.FocusProjects, Label: "OfferPilot"}}},
			CurrentQuestion: &interview.Question{ID: "question-2", Text: "下一题", CoveragePointID: "coverage-1"},
			Turns: []interview.SnapshotTurn{{
				Question: interview.Question{ID: "question-1", Text: "第一题", CoveragePointID: "coverage-1"},
				Answer:   interview.AnswerPayload{Text: "候选人回答", InputMode: interview.InputModeText},
				Feedback: interview.AnswerFeedback{Focus: interview.FocusProjects, Summary: "已提交", Assessment: interview.Assessment{Correctness: 4}},
			}},
			Progress: interview.Progress{Answered: 1, Total: 3, Current: 2},
		},
		review: interview.ReviewSnapshot{
			SchemaVersion: "1.0.0", State: interview.StateAwaitingAnswer,
			Turns: []interview.ReviewTurn{{
				Question:   interview.Question{ID: "question-1", Text: "第一题", CoveragePointID: "coverage-1"},
				Answer:     interview.AnswerPayload{Text: "候选人回答", InputMode: interview.InputModeVoice, DurationMS: 2300},
				Feedback:   interview.AnswerFeedback{Focus: interview.FocusProjects, Summary: "已提交", Assessment: interview.Assessment{Correctness: 4}},
				References: []interview.ReviewReference{{EvidenceID: "knowledge:1", Title: "参考题", Answer: "标准答案"}},
			}},
		},
		events: interview.SessionEventPage{Events: []interview.SessionEventSummary{{
			EventID: "event-2", Sequence: 2, Type: "answer.completed", CreatedAt: time.Now(),
		}}},
	}
	server := newTestServer(t, Config{}, service)

	snapshotRequest := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/interview-1", nil)
	snapshotResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(snapshotResponse, snapshotRequest)
	if snapshotResponse.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", snapshotResponse.Code, snapshotResponse.Body.String())
	}
	var snapshot map[string]any
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["interviewId"] != "interview-1" || snapshot["currentQuestion"] == nil {
		t.Fatalf("snapshot = %+v", snapshot)
	}

	reviewRequest := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/interview-1/review", nil)
	reviewResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(reviewResponse, reviewRequest)
	if reviewResponse.Code != http.StatusOK || !strings.Contains(reviewResponse.Body.String(), "标准答案") {
		t.Fatalf("review status=%d body=%s", reviewResponse.Code, reviewResponse.Body.String())
	}

	eventsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/interviews/interview-1/events?after=1&limit=10", nil)
	eventsResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(eventsResponse, eventsRequest)
	if eventsResponse.Code != http.StatusOK || !json.Valid(eventsResponse.Body.Bytes()) {
		t.Fatalf("events status=%d body=%s", eventsResponse.Code, eventsResponse.Body.String())
	}
	var events interview.SessionEventPage
	if err := json.Unmarshal(eventsResponse.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if events.InterviewID != "interview-1" || events.NextSequence != 2 || len(events.Events) != 1 {
		t.Fatalf("events = %+v", events)
	}

	invalidLimit := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidLimit, httptest.NewRequest(
		http.MethodGet, "/api/v1/interviews/interview-1/events?limit=0", nil,
	))
	if invalidLimit.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status=%d body=%s", invalidLimit.Code, invalidLimit.Body.String())
	}
}
