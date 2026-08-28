package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"offerpilot/backend/internal/chat"
	"offerpilot/backend/internal/config"
	"offerpilot/backend/internal/harness"
	"offerpilot/backend/internal/httpapi"
	"offerpilot/backend/internal/interview"
	"offerpilot/backend/internal/jobmatch"
	"offerpilot/backend/internal/knowledge"
	"offerpilot/backend/internal/llm"
	"offerpilot/backend/internal/session"
	"offerpilot/backend/internal/speech"
	"offerpilot/backend/internal/webcrawler"
)

func main() {
	if _, err := config.LoadDotEnv(); err != nil {
		fatal("load environment", err)
	}
	logger := newLogger()

	knowledgeDirectory, err := resolveKnowledgeDirectory(envOr("KNOWLEDGE_DIR", "knowledge"))
	if err != nil {
		fatal("locate knowledge directory", err)
	}
	index, err := knowledge.Load(knowledgeDirectory)
	if err != nil {
		fatal("load knowledge index", err)
	}
	retriever, err := knowledge.NewInterviewRetriever(index, intEnv("OFFERPILOT_INTERVIEW_KNOWLEDGE_LIMIT", 8))
	if err != nil {
		fatal("create interview retriever", err)
	}
	databasePath, err := resolveDataPath(envOr("DB_PATH", "data/offerpilot.db"))
	if err != nil {
		fatal("resolve interview database", err)
	}
	interviewStore, err := interview.OpenSQLiteStore(databasePath)
	if err != nil {
		fatal("open interview database", err)
	}
	defer func() {
		if closeErr := interviewStore.Close(); closeErr != nil {
			logger.Error("close interview database", "error", closeErr)
		}
	}()

	var interviewAgent interview.Agent
	var chatClient httpapi.ChatClient
	var crawlerAgent *webcrawler.Agent
	var matcherAgent *jobmatch.Agent
	modelConfigured := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
	if modelConfigured {
		modelClient, modelErr := llm.NewFromEnv()
		if modelErr != nil {
			fatal("configure text model", modelErr)
		}
		runtime, runtimeErr := harness.NewRuntime(modelClient, harness.Options{
			MaxConcurrent: intEnv("OFFERPILOT_HARNESS_MAX_CONCURRENT", 4),
			TraceCapacity: intEnv("OFFERPILOT_HARNESS_TRACE_CAPACITY", 512),
			TraceSink: func(event harness.TraceEvent) {
				if event.Type == harness.TraceError {
					logger.Warn("harness call failed", "trace_id", event.TraceID, "agent", event.AgentID, "tool", event.ToolName, "duration_ms", event.Duration.Milliseconds(), "error", event.Error)
				}
			},
		})
		if runtimeErr != nil {
			fatal("create harness runtime", runtimeErr)
		}
		interviewAgent, err = harness.NewInterviewAgent(runtime)
		if err != nil {
			fatal("register interview agents", err)
		}
		crawlerAgent, err = webcrawler.NewAgentWithOptions(runtime, webcrawler.NewFetcher(webcrawler.Options{
			RequestTimeout:       durationEnv("OFFERPILOT_CRAWLER_REQUEST_TIMEOUT", 10*time.Second),
			MaxResponseBytes:     int64(intEnv("OFFERPILOT_CRAWLER_MAX_RESPONSE_BYTES", 2<<20)),
			MaxRedirects:         intEnv("OFFERPILOT_CRAWLER_MAX_REDIRECTS", 3),
			AllowBenchmarkTunnel: boolEnv("OFFERPILOT_ALLOW_TUN_FAKE_IP", !strings.EqualFold(os.Getenv("NODE_ENV"), "production")),
		}), webcrawler.AgentOptions{
			MaxFallbackIterations: intEnv("OFFERPILOT_CRAWLER_FALLBACK_MAX_ITERATIONS", 4),
			FallbackTimeout:       durationEnv("OFFERPILOT_CRAWLER_FALLBACK_TIMEOUT", 90*time.Second),
			DecisionTimeout:       durationEnv("OFFERPILOT_CRAWLER_DECISION_TIMEOUT", 60*time.Second),
		})
		if err != nil {
			fatal("register web crawler agent", err)
		}
		matcherAgent, err = jobmatch.NewAgent(runtime, jobmatch.AgentOptions{
			Timeout: durationEnv("OFFERPILOT_MATCHER_TIMEOUT", 90*time.Second),
		})
		if err != nil {
			fatal("register resume matcher agent", err)
		}
		chatClient, err = chat.New(chat.Config{
			APIKey:    os.Getenv("OPENAI_API_KEY"),
			BaseURL:   envOr("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			Model:     envOr("OPENAI_MODEL", "gpt-5.5"),
			Timeout:   durationEnv("OPENAI_CHAT_TIMEOUT", 90*time.Second),
			MaxTokens: intEnv("OPENAI_MAX_TOKENS", 4096),
		}, nil)
		if err != nil {
			fatal("configure chat model", err)
		}
	} else {
		logger.Warn("text model is not configured; interview and chat endpoints are unavailable", "required_env", "OPENAI_API_KEY")
	}

	var speechClient httpapi.SpeechClient
	speechConfigured := strings.TrimSpace(os.Getenv("MIMO_API_KEY")) != ""
	if speechConfigured {
		speechClient, err = speech.New(speech.Config{
			APIKey:      os.Getenv("MIMO_API_KEY"),
			BaseURL:     envOr("MIMO_BASE_URL", "https://api.xiaomimimo.com/v1"),
			ASRModel:    envOr("MIMO_ASR_MODEL", "mimo-v2.5-asr"),
			TTSModel:    envOr("MIMO_TTS_MODEL", "mimo-v2.5-tts"),
			TTSVoice:    envOr("MIMO_TTS_VOICE", "mimo_default"),
			ASRLanguage: envOr("MIMO_ASR_LANGUAGE", "auto"),
			Timeout:     durationEnv("MIMO_TIMEOUT", 60*time.Second),
		}, nil)
		if err != nil {
			fatal("configure speech model", err)
		}
	}

	interviewService := interview.NewService(interview.Dependencies{
		Agent:     interviewAgent,
		Retriever: retriever,
		Store:     interviewStore,
	})
	authRequired := strings.EqualFold(os.Getenv("NODE_ENV"), "production") || boolEnv("OFFERPILOT_REQUIRE_AUTH", false)
	api, err := httpapi.New(httpapi.Config{
		Version:           config.Version,
		APIKey:            os.Getenv("OFFERPILOT_API_KEY"),
		RequireAuth:       authRequired,
		AllowedOrigins:    csvEnv("OFFERPILOT_ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://127.0.0.1:3000"}),
		MaxJSONBodyBytes:  int64(intEnv("OFFERPILOT_MAX_JSON_BODY_BYTES", 256<<10)),
		MaxInterviewBytes: int64(intEnv("OFFERPILOT_MAX_INTERVIEW_BODY_BYTES", 2<<20)),
		MaxAudioBodyBytes: int64(intEnv("OFFERPILOT_MAX_AUDIO_BODY_BYTES", 25<<20)),
		MaxMessageChars:   intEnv("OFFERPILOT_MAX_MESSAGE_CHARS", 20000),
		MaxTTSTextChars:   intEnv("OFFERPILOT_MAX_TTS_TEXT_CHARS", 5000),
		MaxURLChars:       intEnv("OFFERPILOT_MAX_URL_CHARS", 4096),
		KnowledgeEntries:  index.Len(),
		ModelConfigured:   modelConfigured,
		SpeechConfigured:  speechConfigured,
	}, httpapi.Dependencies{
		Interview: interviewService,
		Chat:      chatClient,
		Speech:    speechClient,
		Crawler:   crawlerAgent,
		Matcher:   matcherAgent,
		Sessions:  session.NewStore(),
		Memory:    session.NewMemory(40),
		Logger:    logger,
	})
	if err != nil {
		fatal("create HTTP API", err)
	}

	port := intEnv("PORT", 3001)
	server := api.HTTPServer(fmt.Sprintf(":%d", port))
	errChannel := make(chan error, 1)
	go func() {
		logger.Info("OfferPilot Go API started",
			"version", config.Version,
			"port", port,
			"knowledge_entries", index.Len(),
			"knowledge_directory", knowledgeDirectory,
			"database_path", databasePath,
			"model_configured", modelConfigured,
			"speech_configured", speechConfigured,
			"crawler_configured", crawlerAgent != nil,
			"matcher_configured", matcherAgent != nil,
			"auth_required", authRequired,
		)
		if listenErr := server.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			errChannel <- listenErr
		}
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalContext.Done():
		logger.Info("shutdown requested", "signal", signalContext.Err())
	case listenErr := <-errChannel:
		fatal("serve HTTP", listenErr)
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		_ = server.Close()
	}
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func resolveKnowledgeDirectory(configured string) (string, error) {
	if filepath.IsAbs(configured) {
		if isDirectory(configured) {
			return filepath.Clean(configured), nil
		}
		return "", fmt.Errorf("%s does not exist or is not a directory", configured)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(workingDirectory, configured),
		filepath.Join(filepath.Dir(workingDirectory), configured),
	}
	for _, candidate := range candidates {
		if isDirectory(candidate) {
			absolute, absoluteErr := filepath.Abs(candidate)
			if absoluteErr != nil {
				return "", absoluteErr
			}
			return absolute, nil
		}
	}
	return "", fmt.Errorf("%s not found from %s or its parent", configured, workingDirectory)
}

func resolveDataPath(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", errors.New("database path is empty")
	}
	if filepath.IsAbs(configured) {
		if err := os.MkdirAll(filepath.Dir(configured), 0o750); err != nil {
			return "", err
		}
		return filepath.Clean(configured), nil
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	base := workingDirectory
	if strings.EqualFold(filepath.Base(workingDirectory), "backend") {
		base = filepath.Dir(workingDirectory)
	}
	path, err := filepath.Abs(filepath.Join(base, configured))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	return path, nil
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boolEnv(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func csvEnv(name string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func fatal(operation string, err error) {
	slog.Error(operation, "error", err)
	os.Exit(1)
}
