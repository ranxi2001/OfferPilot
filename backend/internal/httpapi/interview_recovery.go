package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"offerpilot/backend/internal/interview"
)

type interviewRecoveryService interface {
	Snapshot(context.Context, string) (interview.SessionSnapshot, error)
	SessionEvents(context.Context, string, int64, int) (interview.SessionEventPage, error)
}

func (s *Server) handleInterviewSnapshot(response http.ResponseWriter, request *http.Request) {
	recovery, ok := s.interview.(interviewRecoveryService)
	if !ok {
		writeAPIError(response, http.StatusServiceUnavailable, string(interview.CodeUnavailable), "Interview recovery is not configured", true, "")
		return
	}
	interviewID := strings.TrimSpace(request.PathValue("interviewId"))
	if interviewID == "" {
		interviewID = strings.TrimSpace(request.URL.Query().Get("interviewId"))
	}
	snapshot, err := recovery.Snapshot(request.Context(), interviewID)
	if err != nil {
		writeInterviewError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, mapSnapshot(snapshot))
}

func (s *Server) handleInterviewEvents(response http.ResponseWriter, request *http.Request) {
	recovery, ok := s.interview.(interviewRecoveryService)
	if !ok {
		writeAPIError(response, http.StatusServiceUnavailable, string(interview.CodeUnavailable), "Interview recovery is not configured", true, "")
		return
	}
	interviewID := strings.TrimSpace(request.PathValue("interviewId"))
	after, err := parseNonNegativeQuery(request, "after", 0)
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, string(interview.CodeValidation), err.Error(), false, "after")
		return
	}
	limit64, err := parseNonNegativeQuery(request, "limit", 100)
	if err != nil || limit64 == 0 || limit64 > 1000 {
		writeAPIError(response, http.StatusBadRequest, string(interview.CodeValidation), "limit must be between 1 and 1000", false, "limit")
		return
	}
	page, serviceErr := recovery.SessionEvents(request.Context(), interviewID, after, int(limit64))
	if serviceErr != nil {
		writeInterviewError(response, serviceErr)
		return
	}
	writeJSON(response, http.StatusOK, page)
}

func parseNonNegativeQuery(request *http.Request, key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(request.URL.Query().Get(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, &queryValueError{key: key}
	}
	return parsed, nil
}

type queryValueError struct{ key string }

func (e *queryValueError) Error() string { return e.key + " must be a non-negative integer" }

func mapSnapshot(snapshot interview.SessionSnapshot) map[string]any {
	turns := make([]webTurn, 0, len(snapshot.Turns))
	for index, turn := range snapshot.Turns {
		feedback := deferredFeedback(turn.Question.ID)
		if !turn.Feedback.Deferred {
			feedback = mapFeedback(
				turn.Question.ID,
				turn.Feedback.Assessment,
				turn.Feedback.Summary,
				feedbackFocus(turn.Feedback.Focus),
			)
		}
		turns = append(turns, webTurn{
			Question: mapQuestion(turn.Question, index+1, &snapshot.Profile),
			Answer:   turn.Answer.Text,
			Feedback: feedback,
		})
	}
	var current *webQuestion
	if snapshot.CurrentQuestion != nil {
		mapped := mapQuestion(*snapshot.CurrentQuestion, snapshot.Progress.Current, &snapshot.Profile)
		current = &mapped
	}
	return map[string]any{
		"interviewId":     snapshot.InterviewID,
		"state":           webState(snapshot.State),
		"profile":         mapProfile(snapshot.Profile),
		"currentQuestion": current,
		"turns":           turns,
		"progress":        mapProgress(snapshot.Progress),
		"reportReady":     snapshot.ReportReady,
	}
}
