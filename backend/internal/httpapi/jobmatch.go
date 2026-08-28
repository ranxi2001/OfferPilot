package httpapi

import (
	"net/http"
	"strings"

	"offerpilot/backend/internal/jobmatch"
)

func (s *Server) handleMatch(response http.ResponseWriter, request *http.Request) {
	if s.matcher == nil {
		writeAPIError(response, http.StatusServiceUnavailable, "matcher_unavailable", "Resume matcher Agent is not configured", true, "")
		return
	}
	var input jobmatch.Request
	if err := readJSON(response, request, s.config.MaxInterviewBytes, &input); err != nil {
		writeReadError(response, err)
		return
	}
	input.JD = strings.TrimSpace(input.JD)
	input.Resume = strings.TrimSpace(input.Resume)
	if input.JD == "" || input.Resume == "" {
		writeAPIError(response, http.StatusBadRequest, "validation_error", "jd and resume are required", false, "")
		return
	}
	result, err := s.matcher.Match(request.Context(), input)
	if err != nil {
		s.logger.Warn("resume matcher failed", "error", err)
		writeAPIError(response, http.StatusServiceUnavailable, "matching_failed", "Semantic matching is temporarily unavailable", true, "")
		return
	}
	writeJSON(response, http.StatusOK, result)
}
