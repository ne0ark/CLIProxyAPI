package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/buildinfo"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	log "github.com/sirupsen/logrus"
)

const (
	defaultSentryFlushTimeout = 2 * time.Second
	serverErrorCaptureMessage = "request completed with server error"
)

var (
	errorTrackingHubMu sync.Mutex
	errorTrackingHub   = sentry.NewHub(nil, sentry.NewScope())
)

func currentErrorTrackingHub() *sentry.Hub {
	return errorTrackingHub
}

// ConfigureErrorTracking initializes contextual Sentry error tracking for the current configuration.
func ConfigureErrorTracking(cfg *config.Config) error {
	errorTrackingHubMu.Lock()
	defer errorTrackingHubMu.Unlock()

	hub := currentErrorTrackingHub()
	currentClient := hub.Client()

	dsn := ""
	if cfg != nil {
		dsn = strings.TrimSpace(cfg.Sentry.DSN)
	}

	if dsn == "" {
		flushSentryClient(currentClient, defaultSentryFlushTimeout)
		hub.BindClient(nil)
		return nil
	}

	clientOptions := sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      configuredSentryEnvironment(cfg),
		Release:          configuredSentryRelease(cfg),
		SampleRate:       configuredSentrySampleRate(cfg),
		AttachStacktrace: configuredSentryAttachStacktrace(cfg),
		Debug:            cfg != nil && cfg.Sentry.Debug,
		SendDefaultPII:   false,
		MaxBreadcrumbs:   32,
		BeforeSend:       sanitizeSentryEvent,
	}

	client, err := sentry.NewClient(clientOptions)
	if err != nil {
		hub.BindClient(nil)
		return fmt.Errorf("initialize sentry: %w", err)
	}
	if currentClient != nil && strings.TrimSpace(currentClient.Options().Dsn) != "" {
		flushSentryClient(currentClient, defaultSentryFlushTimeout)
	}
	hub.BindClient(client)

	log.WithField("environment", clientOptions.Environment).Info("sentry error tracking enabled")
	return nil
}

// FlushErrorTracking flushes pending Sentry events.
func FlushErrorTracking(timeout time.Duration) bool {
	return flushSentryClient(currentErrorTrackingHub().Client(), timeout)
}

func flushSentryClient(client *sentry.Client, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = defaultSentryFlushTimeout
	}
	if client == nil {
		return false
	}
	return client.Flush(timeout)
}

// GinSentryMiddleware installs the Sentry Gin integration so panics are captured with request scope.
func GinSentryMiddleware() gin.HandlerFunc {
	middleware := sentrygin.New(sentrygin.Options{
		Repanic:         true,
		WaitForDelivery: false,
		Timeout:         defaultSentryFlushTimeout,
	})
	return func(c *gin.Context) {
		requestHub := currentErrorTrackingHub().Clone()
		c.Request = c.Request.WithContext(sentry.SetHubOnContext(c.Request.Context(), requestHub))
		middleware(c)
	}
}

// GinSentryContext enriches each request-scoped Sentry hub with request metadata and 5xx capture.
func GinSentryContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		hub := sentrygin.GetHubFromContext(c)
		if hub == nil || hub.Client() == nil {
			c.Next()
			return
		}

		applySentryRequestContext(c, hub)

		start := time.Now()
		c.Next()

		statusCode := c.Writer.Status()
		route := sentryRoute(c)
		hub.Scope().SetTag("route", route)
		hub.Scope().SetContext("cliproxy_response", map[string]interface{}{
			"status_code":  statusCode,
			"duration_ms":  time.Since(start).Milliseconds(),
			"content_type": c.Writer.Header().Get("Content-Type"),
		})
		hub.AddBreadcrumb(&sentry.Breadcrumb{
			Type:      "http",
			Category:  "http.response",
			Message:   "request finished",
			Level:     sentry.LevelInfo,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"method":      c.Request.Method,
				"path":        c.Request.URL.Path,
				"route":       route,
				"status_code": statusCode,
				"request_id":  GetGinRequestID(c),
			},
		}, nil)

		if statusCode < http.StatusInternalServerError {
			return
		}

		setSentryUserContext(c, hub)
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelError)
			scope.SetTag("status_code", fmt.Sprintf("%d", statusCode))
			hub.CaptureMessage(serverErrorCaptureMessage)
		})
	}
}

// SetSentryUserContext attaches safe, pseudonymous request identity details to the current Sentry scope.
func SetSentryUserContext(c *gin.Context) {
	hub := sentrygin.GetHubFromContext(c)
	if hub == nil || hub.Client() == nil {
		return
	}
	setSentryUserContext(c, hub)
}

func applySentryRequestContext(c *gin.Context, hub *sentry.Hub) {
	requestID := GetGinRequestID(c)
	route := sentryRoute(c)
	if requestID != "" {
		hub.Scope().SetTag("request_id", requestID)
	}
	hub.Scope().SetTag("component", "api")
	hub.Scope().SetTag("method", c.Request.Method)
	hub.Scope().SetTag("route", route)
	hub.Scope().SetContext("cliproxy_request", map[string]interface{}{
		"method":     c.Request.Method,
		"path":       c.Request.URL.Path,
		"route":      route,
		"query":      util.MaskSensitiveQuery(c.Request.URL.RawQuery),
		"user_agent": c.Request.UserAgent(),
	})
	hub.AddBreadcrumb(&sentry.Breadcrumb{
		Type:      "http",
		Category:  "http.request",
		Message:   "request started",
		Level:     sentry.LevelInfo,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"route":      route,
			"request_id": requestID,
		},
	}, nil)
}

func setSentryUserContext(c *gin.Context, hub *sentry.Hub) {
	if c == nil || hub == nil {
		return
	}

	provider, _ := c.Get("accessProvider")
	providerName, _ := provider.(string)
	providerName = strings.TrimSpace(providerName)
	if providerName != "" {
		hub.Scope().SetTag("access_provider", providerName)
	}

	principal, _ := c.Get("apiKey")
	principalValue, _ := principal.(string)
	principalValue = strings.TrimSpace(principalValue)
	if principalValue == "" {
		if providerName != "" {
			hub.Scope().SetUser(sentry.User{
				Data: map[string]string{
					"provider": providerName,
				},
			})
		}
		return
	}

	user := sentry.User{
		ID: pseudonymousSentryUserID(providerName, principalValue),
	}
	if providerName != "" {
		user.Data = map[string]string{
			"provider": providerName,
		}
	}
	hub.Scope().SetUser(user)
}

func sanitizeSentryEvent(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event == nil || event.Request == nil {
		return event
	}

	event.Request.Data = ""
	event.Request.Cookies = ""
	for key, value := range event.Request.Headers {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(lowerKey, "cookie") {
			delete(event.Request.Headers, key)
			continue
		}
		event.Request.Headers[key] = util.MaskSensitiveHeaderValue(key, value)
	}

	if event.Request.URL != "" {
		parsed, err := url.Parse(event.Request.URL)
		if err == nil {
			parsed.RawQuery = util.MaskSensitiveQuery(parsed.RawQuery)
			event.Request.URL = parsed.String()
		}
	}
	event.Request.QueryString = util.MaskSensitiveQuery(event.Request.QueryString)

	return event
}

func configuredSentryEnvironment(cfg *config.Config) string {
	if cfg != nil {
		if value := strings.TrimSpace(cfg.Sentry.Environment); value != "" {
			return value
		}
		if cfg.Debug {
			return "development"
		}
	}
	return "production"
}

func configuredSentryRelease(cfg *config.Config) string {
	if cfg != nil {
		if value := strings.TrimSpace(cfg.Sentry.Release); value != "" {
			return value
		}
	}
	if version := strings.TrimSpace(buildinfo.Version); version != "" && version != "dev" {
		return "cliproxyapi@" + version
	}
	return ""
}

func configuredSentrySampleRate(cfg *config.Config) float64 {
	if cfg != nil && cfg.Sentry.SampleRate > 0 && cfg.Sentry.SampleRate <= 1 {
		return cfg.Sentry.SampleRate
	}
	return 1.0
}

func configuredSentryAttachStacktrace(cfg *config.Config) bool {
	if cfg != nil && cfg.Sentry.AttachStacktrace != nil {
		return *cfg.Sentry.AttachStacktrace
	}
	return true
}

func sentryRoute(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	if route := strings.TrimSpace(c.FullPath()); route != "" {
		return route
	}
	return c.Request.URL.Path
}

func pseudonymousSentryUserID(provider, principal string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(provider) + ":" + strings.TrimSpace(principal)))
	return hex.EncodeToString(sum[:8])
}
