package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"offerpilot/backend/internal/webcrawler"
)

func (s *Server) handleCrawl(response http.ResponseWriter, request *http.Request) {
	if s.crawler == nil {
		writeAPIError(response, http.StatusServiceUnavailable, "crawler_unavailable", "Web crawler Agent is not configured", true, "")
		return
	}
	var input webcrawler.Request
	if err := readJSON(response, request, s.config.MaxJSONBodyBytes, &input); err != nil {
		writeReadError(response, err)
		return
	}
	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		writeAPIError(response, http.StatusBadRequest, "validation_error", "url is required", false, "url")
		return
	}
	if utf8.RuneCountInString(input.URL) > s.config.MaxURLChars {
		writeAPIError(response, http.StatusRequestEntityTooLarge, "url_too_large", "url is too long", false, "url")
		return
	}

	result, err := s.crawler.Crawl(request.Context(), input)
	if err == nil {
		writeJSON(response, http.StatusOK, result)
		return
	}
	switch {
	case errors.Is(err, webcrawler.ErrInvalidURL):
		writeAPIError(response, http.StatusBadRequest, "invalid_url", "URL must be a public HTTP or HTTPS address", false, "url")
	case errors.Is(err, webcrawler.ErrBlockedURL):
		writeAPIError(response, http.StatusBadRequest, "url_not_allowed", "Private, loopback, link-local, and reserved URLs are not allowed", false, "url")
	case errors.Is(err, webcrawler.ErrResponseLarge):
		writeAPIError(response, http.StatusRequestEntityTooLarge, "page_too_large", "Web page exceeds the crawler response limit", false, "")
	case errors.Is(err, webcrawler.ErrNoContent):
		writeAPIError(response, http.StatusUnprocessableEntity, "page_content_missing", "No useful page content was found", false, "")
	case errors.Is(err, context.Canceled):
		writeAPIError(response, http.StatusRequestTimeout, "crawl_canceled", "Web crawl was canceled", true, "")
	case errors.Is(err, context.DeadlineExceeded):
		writeAPIError(response, http.StatusRequestTimeout, "crawl_timeout", "Web crawl timed out", true, "")
	default:
		writeAPIError(response, http.StatusBadGateway, "crawl_failed", "Could not fetch the requested web page", true, "")
	}
}
