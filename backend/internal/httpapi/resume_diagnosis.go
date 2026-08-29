package httpapi

import (
	"net/http"
	"strings"

	"offerpilot/backend/internal/resumediagnosis"
)

const (
	maxResumeImages   = 3
	maxResumeImageURL = 3 << 20
)

func (s *Server) handleResumeDiagnosis(response http.ResponseWriter, request *http.Request) {
	if s.resumeDiagnostician == nil {
		writeAPIError(response, http.StatusServiceUnavailable, "resume_diagnostician_unavailable", "Resume diagnostician Agent is not configured", true, "")
		return
	}
	var input resumediagnosis.Request
	if err := readJSON(response, request, s.config.MaxResumeDiagnosisBytes, &input); err != nil {
		writeReadError(response, err)
		return
	}
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" {
		writeAPIError(response, http.StatusBadRequest, "validation_error", "resume content is required", false, "content")
		return
	}
	if len(input.Images) > maxResumeImages {
		writeAPIError(response, http.StatusBadRequest, "too_many_images", "at most three resume page images are allowed", false, "images")
		return
	}
	for _, image := range input.Images {
		if len(image) > maxResumeImageURL ||
			(!strings.HasPrefix(image, "data:image/jpeg;base64,") && !strings.HasPrefix(image, "data:image/png;base64,")) {
			writeAPIError(response, http.StatusBadRequest, "invalid_image", "resume images must be bounded JPEG or PNG data URLs", false, "images")
			return
		}
	}
	result, err := s.resumeDiagnostician.Diagnose(request.Context(), input)
	if err != nil {
		s.logger.Warn("resume diagnostician failed", "error", err)
		writeAPIError(response, http.StatusServiceUnavailable, "resume_diagnosis_failed", "Multimodal resume diagnosis is temporarily unavailable", true, "")
		return
	}
	writeJSON(response, http.StatusOK, result)
}
