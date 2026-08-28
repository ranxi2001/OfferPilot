// Package httpapi exposes the Go backend through the compatibility HTTP
// surface consumed by the Next.js BFF.
package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"offerpilot/backend/internal/chat"
	"offerpilot/backend/internal/interview"
	"offerpilot/backend/internal/jobmatch"
	"offerpilot/backend/internal/session"
	"offerpilot/backend/internal/speech"
	"offerpilot/backend/internal/webcrawler"
)

const chatSystemPrompt = `你是 OfferPilot，一名严谨的 AI Agent / LLM 工程面试教练。基于用户实际提供的问题和材料作答；区分事实、候选人陈述与推断，不编造简历经历。诊断回答时关注原理、个人职责、量化口径、方案取舍和边界条件，并给出可执行改进。`

type InterviewService interface {
	Start(context.Context, interview.StartRequest) (interview.StartResponse, error)
	Answer(context.Context, interview.AnswerRequest) (interview.AnswerResponse, error)
	Report(context.Context, interview.ReportRequest) (interview.ReportResponse, error)
}

type ChatClient interface {
	Stream(context.Context, string, []chat.Message, func(chat.Delta) error) (chat.Usage, error)
}

type SpeechClient interface {
	Transcribe(context.Context, speech.TranscribeInput) (string, error)
	Synthesize(context.Context, speech.SynthesizeInput) (speech.Audio, error)
}

type WebCrawler interface {
	Crawl(context.Context, webcrawler.Request) (webcrawler.Result, error)
}

type ResumeMatcher interface {
	Match(context.Context, jobmatch.Request) (jobmatch.Result, error)
}

type Config struct {
	Version             string
	APIKey              string
	RequireAuth         bool
	AllowedOrigins      []string
	MaxJSONBodyBytes    int64
	MaxInterviewBytes   int64
	MaxAudioBodyBytes   int64
	MaxMessageChars     int
	MaxTTSTextChars     int
	MaxURLChars         int
	ReadHeaderTimeout   time.Duration
	IdleTimeout         time.Duration
	InterviewRunTimeout time.Duration
	KnowledgeEntries    int
	ModelConfigured     bool
	SpeechConfigured    bool
}

type Dependencies struct {
	Interview InterviewService
	Chat      ChatClient
	Speech    SpeechClient
	Crawler   WebCrawler
	Matcher   ResumeMatcher
	Sessions  *session.Store
	Memory    *session.Memory
	Logger    *slog.Logger
}

type Server struct {
	config    Config
	interview InterviewService
	chat      ChatClient
	speech    SpeechClient
	crawler   WebCrawler
	matcher   ResumeMatcher
	sessions  *session.Store
	memory    *session.Memory
	logger    *slog.Logger
	handler   http.Handler
}

func New(config Config, dependencies Dependencies) (*Server, error) {
	config = withDefaults(config)
	if config.RequireAuth && strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("httpapi: OFFERPILOT_API_KEY is required when authentication is enabled")
	}
	if dependencies.Interview == nil {
		return nil, errors.New("httpapi: interview service is required")
	}
	if dependencies.Sessions == nil {
		dependencies.Sessions = session.NewStore()
	}
	if dependencies.Memory == nil {
		dependencies.Memory = session.NewMemory(40)
	}
	if dependencies.Logger == nil {
		dependencies.Logger = slog.Default()
	}
	server := &Server{
		config:    config,
		interview: dependencies.Interview,
		chat:      dependencies.Chat,
		speech:    dependencies.Speech,
		crawler:   dependencies.Crawler,
		matcher:   dependencies.Matcher,
		sessions:  dependencies.Sessions,
		memory:    dependencies.Memory,
		logger:    dependencies.Logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.handleHealth)
	mux.HandleFunc("GET /health/live", server.handleLiveness)
	mux.HandleFunc("GET /health/ready", server.handleReadiness)
	mux.HandleFunc("POST /api/session", server.requireAuth(server.handleSession))
	mux.HandleFunc("POST /api/chat", server.requireAuth(server.handleChat))
	mux.HandleFunc("POST /api/interview", server.requireAuth(server.handleInterview))
	mux.HandleFunc("POST /api/interview/stream", server.requireAuth(server.handleInterviewStream))
	mux.HandleFunc("POST /api/v1/interview", server.requireAuth(server.handleInterview))
	mux.HandleFunc("GET /api/v1/interviews/{interviewId}", server.requireAuth(server.handleInterviewSnapshot))
	mux.HandleFunc("GET /api/v1/interviews/{interviewId}/review", server.requireAuth(server.handleInterviewReview))
	mux.HandleFunc("GET /api/v1/interviews/{interviewId}/events", server.requireAuth(server.handleInterviewEvents))
	mux.HandleFunc("POST /api/transcribe", server.requireAuth(server.handleTranscribe))
	mux.HandleFunc("POST /api/tts", server.requireAuth(server.handleTTS))
	mux.HandleFunc("POST /api/crawl", server.requireAuth(server.handleCrawl))
	mux.HandleFunc("POST /api/v1/crawl", server.requireAuth(server.handleCrawl))
	mux.HandleFunc("POST /api/match", server.requireAuth(server.handleMatch))
	mux.HandleFunc("POST /api/v1/match", server.requireAuth(server.handleMatch))
	server.handler = server.withMiddleware(mux)
	return server, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) HTTPServer(address string) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           s.Handler(),
		ReadHeaderTimeout: s.config.ReadHeaderTimeout,
		IdleTimeout:       s.config.IdleTimeout,
	}
}

func withDefaults(config Config) Config {
	if strings.TrimSpace(config.Version) == "" {
		config.Version = "dev"
	}
	if config.MaxJSONBodyBytes <= 0 {
		config.MaxJSONBodyBytes = 256 << 10
	}
	if config.MaxInterviewBytes <= 0 {
		config.MaxInterviewBytes = 2 << 20
	}
	if config.MaxAudioBodyBytes <= 0 {
		config.MaxAudioBodyBytes = 25 << 20
	}
	if config.MaxMessageChars <= 0 {
		config.MaxMessageChars = 20000
	}
	if config.MaxTTSTextChars <= 0 {
		config.MaxTTSTextChars = 5000
	}
	if config.MaxURLChars <= 0 {
		config.MaxURLChars = 4096
	}
	if config.ReadHeaderTimeout <= 0 {
		config.ReadHeaderTimeout = 5 * time.Second
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = 90 * time.Second
	}
	if config.InterviewRunTimeout <= 0 {
		config.InterviewRunTimeout = 5 * time.Minute
	}
	if len(config.AllowedOrigins) == 0 {
		config.AllowedOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	}
	return config
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = randomID()
		}
		response.Header().Set("X-Request-ID", requestID)
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "no-referrer")

		origin := request.Header.Get("Origin")
		if origin != "" {
			if !s.originAllowed(origin) {
				writeAPIError(response, http.StatusForbidden, "origin_not_allowed", "Origin is not allowed", false, "")
				return
			}
			response.Header().Set("Access-Control-Allow-Origin", origin)
			response.Header().Add("Vary", "Origin")
		}
		response.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		response.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-File-Name, X-Request-ID")
		if request.Method == http.MethodOptions {
			response.WriteHeader(http.StatusNoContent)
			return
		}

		started := time.Now()
		status := &statusRecorder{ResponseWriter: response, status: http.StatusOK}
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("http panic", "request_id", requestID, "method", request.Method, "path", request.URL.Path)
				if !status.wroteHeader {
					writeAPIError(status, http.StatusInternalServerError, "internal", "Internal server error", true, "")
				}
			}
			s.logger.Info("http request", "request_id", requestID, "method", request.Method, "path", request.URL.Path, "status", status.status, "duration_ms", time.Since(started).Milliseconds())
		}()
		next.ServeHTTP(status, request)
	})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if s.authorized(request) {
			next(response, request)
			return
		}
		message := "Unauthorized"
		if strings.TrimSpace(s.config.APIKey) == "" {
			message = "Server authentication is not configured"
		}
		writeAPIError(response, http.StatusUnauthorized, "unauthorized", message, false, "")
	}
}

func (s *Server) authorized(request *http.Request) bool {
	expected := strings.TrimSpace(s.config.APIKey)
	if expected == "" {
		return !s.config.RequireAuth
	}
	provided := strings.TrimSpace(request.Header.Get("Authorization"))
	if len(provided) >= 7 && strings.EqualFold(provided[:7], "Bearer ") {
		provided = strings.TrimSpace(provided[7:])
	} else {
		return false
	}
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *Server) originAllowed(origin string) bool {
	for _, allowed := range s.config.AllowedOrigins {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}

func (s *Server) handleHealth(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, s.healthPayload("ok"))
}

func (s *Server) handleLiveness(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, s.healthPayload("live"))
}

func (s *Server) handleReadiness(response http.ResponseWriter, _ *http.Request) {
	if !s.config.ModelConfigured {
		writeJSON(response, http.StatusServiceUnavailable, s.healthPayload("not_ready"))
		return
	}
	writeJSON(response, http.StatusOK, s.healthPayload("ready"))
}

func (s *Server) healthPayload(status string) map[string]any {
	harnessState := "ready"
	if !s.config.ModelConfigured {
		harnessState = "not_ready"
	}
	return map[string]any{
		"status":            status,
		"service":           "offerpilot-go",
		"version":           s.config.Version,
		"live":              true,
		"ready":             s.config.ModelConfigured,
		"readiness":         harnessState,
		"harness":           harnessState,
		"modelConfigured":   s.config.ModelConfigured,
		"speechConfigured":  s.config.SpeechConfigured,
		"crawlerConfigured": s.crawler != nil,
		"matcherConfigured": s.matcher != nil,
		"knowledgeEntries":  s.config.KnowledgeEntries,
	}
}

func (s *Server) handleSession(response http.ResponseWriter, _ *http.Request) {
	created, err := s.sessions.Create()
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "session_create_failed", "Could not create session", true, "")
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"sessionId": created.ID})
}

func (s *Server) handleChat(response http.ResponseWriter, request *http.Request) {
	if s.chat == nil {
		writeAPIError(response, http.StatusServiceUnavailable, "chat_unavailable", "Text model is not configured", true, "")
		return
	}
	var input struct {
		Message   string `json:"message"`
		SessionID string `json:"sessionId"`
		Model     string `json:"model"`
	}
	if err := readJSON(response, request, s.config.MaxJSONBodyBytes, &input); err != nil {
		writeReadError(response, err)
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		writeAPIError(response, http.StatusBadRequest, "validation", "message is required", false, "message")
		return
	}
	if utf8.RuneCountInString(input.Message) > s.config.MaxMessageChars {
		writeAPIError(response, http.StatusRequestEntityTooLarge, "message_too_large", fmt.Sprintf("message exceeds %d characters", s.config.MaxMessageChars), false, "message")
		return
	}

	active, ok := s.sessions.Get(input.SessionID)
	if !ok {
		var err error
		active, err = s.sessions.Create(input.SessionID)
		if err != nil {
			writeAPIError(response, http.StatusInternalServerError, "session_create_failed", "Could not create session", true, "")
			return
		}
	}
	_ = s.memory.Add(active.ID, "user", input.Message)
	history := s.memory.Messages(active.ID)
	messages := make([]chat.Message, 0, len(history)+1)
	messages = append(messages, chat.Message{Role: "system", Content: chatSystemPrompt})
	for _, item := range history {
		if item.Role == "user" || item.Role == "assistant" {
			messages = append(messages, chat.Message{Role: item.Role, Content: item.Content})
		}
	}

	stream := newSSEWriter(response)
	stream.start()
	stream.send(map[string]any{"type": "session", "sessionId": active.ID})
	var assistant strings.Builder
	usage, err := s.chat.Stream(request.Context(), input.Model, messages, func(delta chat.Delta) error {
		if delta.Thinking != "" {
			stream.send(map[string]any{"type": "thinking_delta", "content": delta.Thinking})
		}
		if delta.Text != "" {
			assistant.WriteString(delta.Text)
			stream.send(map[string]any{"type": "text_delta", "content": delta.Text})
		}
		if request.Context().Err() != nil {
			return request.Context().Err()
		}
		return nil
	})
	if err != nil {
		stream.send(map[string]any{"type": "error", "message": err.Error()})
	} else {
		if assistant.Len() > 0 {
			_ = s.memory.Add(active.ID, "assistant", assistant.String())
		}
		stream.send(map[string]any{"type": "done", "usage": usage})
	}
	stream.done()
}

func (s *Server) handleTranscribe(response http.ResponseWriter, request *http.Request) {
	if s.speech == nil {
		writeJSON(response, http.StatusServiceUnavailable, map[string]any{
			"error": "语音识别服务尚未配置，请联系管理员", "retryable": false,
		})
		return
	}
	audio, err := readBody(response, request, s.config.MaxAudioBodyBytes)
	if err != nil {
		writeLegacyReadError(response, err)
		return
	}
	if len(audio) == 0 {
		writeLegacyError(response, http.StatusBadRequest, "audio body is required")
		return
	}
	text, err := s.speech.Transcribe(request.Context(), speech.TranscribeInput{
		Audio: audio, FileName: request.Header.Get("X-File-Name"), ContentType: request.Header.Get("Content-Type"),
	})
	if err != nil {
		s.logger.Warn("transcribe failed", "error", err.Error())
		if request.Context().Err() != nil {
			return
		}
		failure := speech.ClassifyTranscriptionError(err)
		writeJSON(response, failure.Status, map[string]any{
			"error": failure.Message, "retryable": failure.Retryable,
		})
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"text": text})
}

func (s *Server) handleTTS(response http.ResponseWriter, request *http.Request) {
	if s.speech == nil {
		writeLegacyError(response, http.StatusServiceUnavailable, "Speech model is not configured")
		return
	}
	var input struct {
		Text   string `json:"text"`
		Voice  string `json:"voice"`
		Format string `json:"format"`
	}
	if err := readJSON(response, request, s.config.MaxJSONBodyBytes, &input); err != nil {
		writeLegacyReadError(response, err)
		return
	}
	if strings.TrimSpace(input.Text) == "" {
		writeLegacyError(response, http.StatusBadRequest, "text is required")
		return
	}
	if utf8.RuneCountInString(input.Text) > s.config.MaxTTSTextChars {
		writeLegacyError(response, http.StatusRequestEntityTooLarge, fmt.Sprintf("text exceeds %d characters", s.config.MaxTTSTextChars))
		return
	}
	audio, err := s.speech.Synthesize(request.Context(), speech.SynthesizeInput{Text: input.Text, Voice: input.Voice, Format: input.Format})
	if err != nil {
		s.logger.Warn("tts failed", "error", err.Error())
		writeLegacyError(response, http.StatusBadGateway, err.Error())
		return
	}
	response.Header().Set("Content-Type", audio.ContentType)
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(audio.Data)
}

type bodyTooLargeError struct{ limit int64 }

func (e *bodyTooLargeError) Error() string {
	return fmt.Sprintf("request body exceeds %d bytes", e.limit)
}

func readBody(response http.ResponseWriter, request *http.Request, limit int64) ([]byte, error) {
	if request.ContentLength > limit {
		return nil, &bodyTooLargeError{limit: limit}
	}
	reader := http.MaxBytesReader(response, request.Body, limit)
	data, err := io.ReadAll(reader)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, &bodyTooLargeError{limit: limit}
		}
		return nil, err
	}
	return data, nil
}

func readJSON(response http.ResponseWriter, request *http.Request, limit int64, output any) error {
	data, err := readBody(response, request, limit)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytesReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("invalid JSON: multiple values")
	}
	return nil
}

func bytesReader(data []byte) io.Reader { return strings.NewReader(string(data)) }

func writeReadError(response http.ResponseWriter, err error) {
	var tooLarge *bodyTooLargeError
	if errors.As(err, &tooLarge) {
		writeAPIError(response, http.StatusRequestEntityTooLarge, "body_too_large", tooLarge.Error(), false, "")
		return
	}
	writeAPIError(response, http.StatusBadRequest, "invalid_json", "Invalid JSON", false, "")
}

func writeLegacyReadError(response http.ResponseWriter, err error) {
	var tooLarge *bodyTooLargeError
	if errors.As(err, &tooLarge) {
		writeLegacyError(response, http.StatusRequestEntityTooLarge, tooLarge.Error())
		return
	}
	writeLegacyError(response, http.StatusBadRequest, "Invalid JSON")
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}

func writeAPIError(response http.ResponseWriter, status int, code, message string, retryable bool, field string) {
	payload := map[string]any{"code": code, "message": message, "retryable": retryable}
	if field != "" {
		payload["field"] = field
	}
	writeJSON(response, status, map[string]any{"error": payload})
}

func writeLegacyError(response http.ResponseWriter, status int, message string) {
	writeJSON(response, status, map[string]string{"error": message})
}

func randomID() string {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err == nil {
		return hex.EncodeToString(data)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(data)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

type sseWriter struct {
	response http.ResponseWriter
	flusher  http.Flusher
	mu       sync.Mutex
}

func newSSEWriter(response http.ResponseWriter) *sseWriter {
	flusher, _ := response.(http.Flusher)
	return &sseWriter{response: response, flusher: flusher}
}

func (w *sseWriter) start() {
	w.response.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.response.Header().Set("Cache-Control", "no-cache")
	w.response.Header().Set("Connection", "keep-alive")
	w.response.WriteHeader(http.StatusOK)
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

func (w *sseWriter) send(event any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w.response, "data: %s\n\n", data)
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

func (w *sseWriter) done() {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = io.WriteString(w.response, "data: [DONE]\n\n")
	if w.flusher != nil {
		w.flusher.Flush()
	}
}
