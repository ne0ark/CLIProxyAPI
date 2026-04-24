package logging

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

const sentryTestDSN = "https://public@example.com/1"

func TestGinSentryMiddlewareCapturesPanicWithContext(t *testing.T) {
	events := captureSentryEvents(t)
	engine := newErrorTrackingTestEngine()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("apiKey", "sk-super-secret")
		c.Set("accessProvider", "bearer")
		SetSentryUserContext(c)
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?auth_token=top-secret", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer sk-super-secret")
	req.Header.Set("Cookie", "session=very-secret")

	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}

	event := waitForCapturedSentryEvent(t, events)
	if len(event.Exception) == 0 && len(event.Threads) == 0 {
		t.Fatal("expected panic event to include exception or thread stack data")
	}
	if got := event.Tags["request_id"]; got == "" {
		t.Fatal("expected request_id tag to be populated")
	}
	if got := event.Tags["access_provider"]; got != "bearer" {
		t.Fatalf("expected access_provider tag to be bearer, got %q", got)
	}
	if event.User.ID != pseudonymousSentryUserID("bearer", "sk-super-secret") {
		t.Fatalf("unexpected pseudonymous user id: %q", event.User.ID)
	}
	if event.Request == nil {
		t.Fatal("expected request context to be attached to the event")
	}
	if strings.Contains(event.Request.URL, "top-secret") {
		t.Fatalf("expected sensitive query values to be masked, got URL %q", event.Request.URL)
	}
	if strings.Contains(event.Request.QueryString, "top-secret") {
		t.Fatalf("expected sensitive query values to be masked, got query %q", event.Request.QueryString)
	}
	if got := headerValue(event.Request.Headers, "Authorization"); strings.Contains(got, "sk-super-secret") {
		t.Fatalf("expected Authorization header to be masked, got %q", got)
	}
	if _, ok := headerPresence(event.Request.Headers, "Cookie"); ok {
		t.Fatal("expected Cookie header to be removed from the sentry payload")
	}
	if !hasBreadcrumb(event.Breadcrumbs, "http.request", "request started") {
		t.Fatal("expected request start breadcrumb to be attached")
	}
}

func TestGinSentryContextCapturesServerErrorsWithLifecycleBreadcrumbs(t *testing.T) {
	events := captureSentryEvents(t)
	engine := newErrorTrackingTestEngine()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("apiKey", "sk-super-secret")
		c.Set("accessProvider", "bearer")
		SetSentryUserContext(c)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "upstream failure"})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?token=top-secret", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer sk-super-secret")

	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}

	event := waitForCapturedSentryEvent(t, events)
	if event.Message != serverErrorCaptureMessage {
		t.Fatalf("expected event message %q, got %q", serverErrorCaptureMessage, event.Message)
	}
	if event.Level != sentry.LevelError {
		t.Fatalf("expected error level event, got %q", event.Level)
	}
	if got := event.Tags["status_code"]; got != "500" {
		t.Fatalf("expected status_code tag to be 500, got %q", got)
	}
	if event.User.ID != pseudonymousSentryUserID("bearer", "sk-super-secret") {
		t.Fatalf("unexpected pseudonymous user id: %q", event.User.ID)
	}
	if !hasBreadcrumb(event.Breadcrumbs, "http.response", "request finished") {
		t.Fatal("expected request completion breadcrumb to be attached")
	}
	if event.Request == nil {
		t.Fatal("expected request context to be attached to the event")
	}
	if strings.Contains(event.Request.URL, "top-secret") {
		t.Fatalf("expected sensitive query values to be masked, got URL %q", event.Request.URL)
	}
	if strings.Contains(event.Request.QueryString, "top-secret") {
		t.Fatalf("expected sensitive query values to be masked, got query %q", event.Request.QueryString)
	}
}

func TestGinSentryMiddlewareRemovesCapturedRequestBodyData(t *testing.T) {
	events := captureSentryEvents(t)
	engine := newErrorTrackingTestEngine()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"api_key":"sk-super-secret","messages":[{"role":"user","content":"top-secret"}]}`))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}

	event := waitForCapturedSentryEvent(t, events)
	if event.Request == nil {
		t.Fatal("expected request context to be attached to the event")
	}
	if event.Request.Data != "" {
		t.Fatalf("expected request body data to be removed, got %q", event.Request.Data)
	}
}

func TestConfigureErrorTrackingFlushesExistingClientBeforeReplacement(t *testing.T) {
	transport := &sentryTransportSpy{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       sentryTestDSN,
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("failed to create existing sentry client: %v", err)
	}

	hub := currentErrorTrackingHub()
	hub.BindClient(client)
	t.Cleanup(func() {
		cleanupErrorTrackingClient()
	})

	cfg := &config.Config{
		Sentry: config.SentryConfig{
			DSN: "https://public@example.com/2",
		},
	}
	if err := ConfigureErrorTracking(cfg); err != nil {
		t.Fatalf("ConfigureErrorTracking returned error: %v", err)
	}

	if got := transport.FlushCalls(); got != 1 {
		t.Fatalf("expected existing client to flush once before replacement, got %d", got)
	}

	currentClient := currentErrorTrackingHub().Client()
	if currentClient == nil {
		t.Fatal("expected a replacement sentry client to be bound")
	}
	if got := strings.TrimSpace(currentClient.Options().Dsn); got != cfg.Sentry.DSN {
		t.Fatalf("expected replacement client DSN %q, got %q", cfg.Sentry.DSN, got)
	}
}

func TestSentryEmbeddedHostIsolationConfigureErrorTracking(t *testing.T) {
	hostTransport := &sentryTransportSpy{}
	hostClient, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       sentryTestDSN,
		Transport: hostTransport,
	})
	if err != nil {
		t.Fatalf("failed to create host sentry client: %v", err)
	}

	cleanupErrorTrackingClient()
	sentry.CurrentHub().BindClient(hostClient)
	t.Cleanup(func() {
		cleanupErrorTrackingClient()
		cleanupCurrentSentryClient()
	})

	enabledCfg := &config.Config{
		Sentry: config.SentryConfig{
			DSN: "https://public@example.com/embedded",
		},
	}
	if err := ConfigureErrorTracking(enabledCfg); err != nil {
		t.Fatalf("ConfigureErrorTracking returned error: %v", err)
	}

	if got := sentry.CurrentHub().Client(); got != hostClient {
		t.Fatal("expected embedded configuration to leave the host sentry client bound globally")
	}

	errorTrackingClient := currentErrorTrackingHub().Client()
	if errorTrackingClient == nil {
		t.Fatal("expected dedicated CLIProxyAPI sentry client to be bound")
	}
	if errorTrackingClient == hostClient {
		t.Fatal("expected CLIProxyAPI to use a dedicated sentry client instead of the host client")
	}

	if err := ConfigureErrorTracking(&config.Config{}); err != nil {
		t.Fatalf("ConfigureErrorTracking disable returned error: %v", err)
	}

	if got := sentry.CurrentHub().Client(); got != hostClient {
		t.Fatal("expected disabling CLIProxyAPI error tracking to keep the host sentry client bound globally")
	}
	if currentErrorTrackingHub().Client() != nil {
		t.Fatal("expected dedicated CLIProxyAPI sentry client to be cleared when disabled")
	}
	if got := hostTransport.FlushCalls(); got != 0 {
		t.Fatalf("expected host sentry client to remain untouched, got %d flush calls", got)
	}
}

func newErrorTrackingTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(GinLogrusLogger())
	engine.Use(GinLogrusRecovery())
	engine.Use(GinSentryMiddleware())
	engine.Use(GinSentryContext())
	return engine
}

func captureSentryEvents(t *testing.T) func() []*sentry.Event {
	t.Helper()

	var (
		mu     sync.Mutex
		events []*sentry.Event
	)

	cleanupErrorTrackingClient()

	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:              sentryTestDSN,
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = sanitizeSentryEvent(event, hint)
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to initialize sentry client for test: %v", err)
	}
	currentErrorTrackingHub().BindClient(client)

	t.Cleanup(func() {
		cleanupErrorTrackingClient()
	})

	return func() []*sentry.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]*sentry.Event(nil), events...)
	}
}

func cleanupCurrentSentryClient() {
	cleanupSentryHubClient(sentry.CurrentHub())
}

func cleanupErrorTrackingClient() {
	cleanupSentryHubClient(currentErrorTrackingHub())
}

func cleanupSentryHubClient(hub *sentry.Hub) {
	if hub == nil {
		return
	}
	client := hub.Client()
	if client != nil {
		client.Flush(200 * time.Millisecond)
		client.Close()
	}
	hub.BindClient(nil)
}

type sentryTransportSpy struct {
	mu         sync.Mutex
	flushCalls int
}

func (s *sentryTransportSpy) Flush(timeout time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushCalls++
	return true
}

func (s *sentryTransportSpy) FlushWithContext(ctx context.Context) bool {
	return s.Flush(0)
}

func (s *sentryTransportSpy) Configure(options sentry.ClientOptions) {}

func (s *sentryTransportSpy) SendEvent(event *sentry.Event) {}

func (s *sentryTransportSpy) Close() {}

func (s *sentryTransportSpy) FlushCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushCalls
}

func waitForCapturedSentryEvent(t *testing.T, events func() []*sentry.Event) *sentry.Event {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		captured := events()
		if len(captured) > 0 {
			return captured[0]
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timed out waiting for sentry event")
	return nil
}

func headerValue(headers map[string]string, key string) string {
	for headerKey, value := range headers {
		if strings.EqualFold(headerKey, key) {
			return value
		}
	}
	return ""
}

func headerPresence(headers map[string]string, key string) (string, bool) {
	for headerKey, value := range headers {
		if strings.EqualFold(headerKey, key) {
			return value, true
		}
	}
	return "", false
}

func hasBreadcrumb(breadcrumbs []*sentry.Breadcrumb, category, message string) bool {
	for _, crumb := range breadcrumbs {
		if crumb == nil {
			continue
		}
		if crumb.Category == category && crumb.Message == message {
			return true
		}
	}
	return false
}
