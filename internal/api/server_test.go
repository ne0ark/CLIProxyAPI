package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gin "github.com/gin-gonic/gin"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/featureflags"
	internallogging "github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v6/sdk/access"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"gopkg.in/yaml.v3"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Port:                   0,
		AuthDir:                authDir,
		Debug:                  true,
		LoggingToFile:          false,
		UsageStatisticsEnabled: false,
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()

	configPath := filepath.Join(tmpDir, "config.yaml")
	return NewServer(cfg, authManager, accessManager, configPath)
}

func newConfiguredTestServer(t *testing.T, cfg *proxyconfig.Config) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	if cfg == nil {
		t.Fatal("cfg must not be nil")
	}

	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}
	if cfg.AuthDir == "" {
		cfg.AuthDir = authDir
	}
	cfg.Port = 0
	cfg.Debug = true
	cfg.LoggingToFile = false
	cfg.UsageStatisticsEnabled = false

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()
	configPath := filepath.Join(tmpDir, "config.yaml")
	return NewServer(cfg, authManager, accessManager, configPath)
}

func TestHealthz(t *testing.T) {
	server := newTestServer(t)

	t.Run("GET", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		server.engine.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}

		var resp struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v; body=%s", err, rr.Body.String())
		}
		if resp.Status != "ok" {
			t.Fatalf("unexpected response status: got %q want %q", resp.Status, "ok")
		}
	})

	t.Run("HEAD", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
		rr := httptest.NewRecorder()
		server.engine.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		if rr.Body.Len() != 0 {
			t.Fatalf("expected empty body for HEAD request, got %q", rr.Body.String())
		}
	})
}

func TestMetricsInstrumentationTracksRequests(t *testing.T) {
	server := newTestServer(t)
	if server.metrics == nil {
		t.Fatal("expected metrics registry to be initialized")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	requestCount := promtestutil.ToFloat64(server.metrics.requestsTotal.WithLabelValues(http.MethodGet, "/healthz", "200"))
	if requestCount != 1 {
		t.Fatalf("expected /healthz request counter to be 1, got %v", requestCount)
	}

	if inFlight := promtestutil.ToFloat64(server.metrics.inFlight); inFlight != 0 {
		t.Fatalf("expected in-flight requests gauge to return to 0, got %v", inFlight)
	}

	if observed := promtestutil.CollectAndCount(server.metrics.requestDuration); observed == 0 {
		t.Fatal("expected request duration histogram to record at least one metric")
	}
}

func TestMetricsEndpointExposesPrometheusMetrics(t *testing.T) {
	server := newTestServer(t)

	seedReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	seedResp := httptest.NewRecorder()
	server.engine.ServeHTTP(seedResp, seedReq)
	if seedResp.Code != http.StatusOK {
		t.Fatalf("failed to seed request metrics: status=%d body=%s", seedResp.Code, seedResp.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("unexpected content type for /metrics: %q", contentType)
	}

	body := rr.Body.String()
	for _, want := range []string{
		"cliproxyapi_http_requests_total",
		`route="/healthz"`,
		"go_goroutines",
		"promhttp_metric_handler_requests_total",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected /metrics response to contain %q, body=%s", want, body)
		}
	}
}

func TestServer_ModelACLRejectsRestrictedKeysOnShippedRoutes(t *testing.T) {
	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: proxyconfig.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []proxyconfig.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-3-5-sonnet-20241022"}`))
	req.Header.Set("Authorization", "Bearer sk-narrow")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("/v1/chat/completions expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-pro:generateContent", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer sk-narrow")
	rr = httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("/v1beta/models/*action expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestServer_ModelACLDiscoveryAndWebsocketBehavior(t *testing.T) {
	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: proxyconfig.SDKConfig{
			APIKeys:             []string{"sk-empty", "sk-anything"},
			APIKeyDefaultPolicy: proxyconfig.APIKeyDefaultPolicyDenyAll,
			APIKeyPolicies: []proxyconfig.APIKeyPolicy{
				{Key: "sk-empty", AllowedModels: nil},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sk-anything")
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/v1/models expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	req.Header.Set("Authorization", "Bearer sk-anything")
	rr = httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/v1beta/models expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer sk-empty")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	rr = httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("/v1/responses websocket upgrade expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/backend-api/codex/responses", nil)
	req.Header.Set("Authorization", "Bearer sk-empty")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	rr = httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("/backend-api/codex/responses websocket upgrade expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAmpProviderModelRoutes(t *testing.T) {
	testCases := []struct {
		name         string
		path         string
		wantStatus   int
		wantContains string
	}{
		{
			name:         "openai root models",
			path:         "/api/provider/openai/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "groq root models",
			path:         "/api/provider/groq/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "openai models",
			path:         "/api/provider/openai/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "anthropic models",
			path:         "/api/provider/anthropic/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"data"`,
		},
		{
			name:         "google models v1",
			path:         "/api/provider/google/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"models"`,
		},
		{
			name:         "google models v1beta",
			path:         "/api/provider/google/v1beta/models",
			wantStatus:   http.StatusOK,
			wantContains: `"models"`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer test-key")

			rr := httptest.NewRecorder()
			server.engine.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("unexpected status code for %s: got %d want %d; body=%s", tc.path, rr.Code, tc.wantStatus, rr.Body.String())
			}
			if body := rr.Body.String(); !strings.Contains(body, tc.wantContains) {
				t.Fatalf("response body for %s missing %q: %s", tc.path, tc.wantContains, body)
			}
		})
	}
}

func TestDefaultRequestLoggerFactory_UsesResolvedLogDirectory(t *testing.T) {
	t.Setenv("WRITABLE_PATH", "")
	t.Setenv("writable_path", "")

	originalWD, errGetwd := os.Getwd()
	if errGetwd != nil {
		t.Fatalf("failed to get current working directory: %v", errGetwd)
	}

	tmpDir := t.TempDir()
	if errChdir := os.Chdir(tmpDir); errChdir != nil {
		t.Fatalf("failed to switch working directory: %v", errChdir)
	}
	defer func() {
		if errChdirBack := os.Chdir(originalWD); errChdirBack != nil {
			t.Fatalf("failed to restore working directory: %v", errChdirBack)
		}
	}()

	// Force ResolveLogDirectory to fallback to auth-dir/logs by making ./logs not a writable directory.
	if errWriteFile := os.WriteFile(filepath.Join(tmpDir, "logs"), []byte("not-a-directory"), 0o644); errWriteFile != nil {
		t.Fatalf("failed to create blocking logs file: %v", errWriteFile)
	}

	configDir := filepath.Join(tmpDir, "config")
	if errMkdirConfig := os.MkdirAll(configDir, 0o755); errMkdirConfig != nil {
		t.Fatalf("failed to create config dir: %v", errMkdirConfig)
	}
	configPath := filepath.Join(configDir, "config.yaml")

	authDir := filepath.Join(tmpDir, "auth")
	if errMkdirAuth := os.MkdirAll(authDir, 0o700); errMkdirAuth != nil {
		t.Fatalf("failed to create auth dir: %v", errMkdirAuth)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: proxyconfig.SDKConfig{
			RequestLog: false,
		},
		AuthDir:           authDir,
		ErrorLogsMaxFiles: 10,
	}

	logger := defaultRequestLoggerFactory(cfg, configPath)
	fileLogger, ok := logger.(*internallogging.FileRequestLogger)
	if !ok {
		t.Fatalf("expected *FileRequestLogger, got %T", logger)
	}

	errLog := fileLogger.LogRequestWithOptions(
		"/v1/chat/completions",
		http.MethodPost,
		map[string][]string{"Content-Type": []string{"application/json"}},
		[]byte(`{"input":"hello"}`),
		http.StatusBadGateway,
		map[string][]string{"Content-Type": []string{"application/json"}},
		[]byte(`{"error":"upstream failure"}`),
		nil,
		nil,
		nil,
		nil,
		nil,
		true,
		"issue-1711",
		time.Now(),
		time.Now(),
	)
	if errLog != nil {
		t.Fatalf("failed to write forced error request log: %v", errLog)
	}

	authLogsDir := filepath.Join(authDir, "logs")
	authEntries, errReadAuthDir := os.ReadDir(authLogsDir)
	if errReadAuthDir != nil {
		t.Fatalf("failed to read auth logs dir %s: %v", authLogsDir, errReadAuthDir)
	}
	foundErrorLogInAuthDir := false
	for _, entry := range authEntries {
		if strings.HasPrefix(entry.Name(), "error-") && strings.HasSuffix(entry.Name(), ".log") {
			foundErrorLogInAuthDir = true
			break
		}
	}
	if !foundErrorLogInAuthDir {
		t.Fatalf("expected forced error log in auth fallback dir %s, got entries: %+v", authLogsDir, authEntries)
	}

	configLogsDir := filepath.Join(configDir, "logs")
	configEntries, errReadConfigDir := os.ReadDir(configLogsDir)
	if errReadConfigDir != nil && !os.IsNotExist(errReadConfigDir) {
		t.Fatalf("failed to inspect config logs dir %s: %v", configLogsDir, errReadConfigDir)
	}
	for _, entry := range configEntries {
		if strings.HasPrefix(entry.Name(), "error-") && strings.HasSuffix(entry.Name(), ".log") {
			t.Fatalf("unexpected forced error log in config dir %s", configLogsDir)
		}
	}
}

func TestServerUpdateClientsAppliesFeatureFlags(t *testing.T) {
	previous := featureflags.Current()
	t.Cleanup(func() {
		featureflags.Apply(previous)
	})

	boolPtr := func(value bool) *bool { return &value }

	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: proxyconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		FeatureFlags: &proxyconfig.FeatureFlagsConfig{
			Routing: proxyconfig.FeatureFlagRoutingConfig{
				AutoModelResolution: boolPtr(false),
			},
			Gemini: proxyconfig.FeatureFlagGeminiConfig{
				AttachDefaultSafetySettings: boolPtr(false),
			},
		},
	})

	if featureflags.RoutingAutoModelResolutionEnabled() {
		t.Fatal("expected routing.auto-model-resolution to be disabled after server startup")
	}
	if featureflags.GeminiAttachDefaultSafetySettingsEnabled() {
		t.Fatal("expected gemini.attach-default-safety-settings to be disabled after server startup")
	}

	var updated proxyconfig.Config
	raw, err := yaml.Marshal(server.cfg)
	if err != nil {
		t.Fatalf("failed to copy server config: %v", err)
	}
	if err := yaml.Unmarshal(raw, &updated); err != nil {
		t.Fatalf("failed to decode copied server config: %v", err)
	}
	updated.FeatureFlags = nil
	server.UpdateClients(&updated)

	if !featureflags.RoutingAutoModelResolutionEnabled() {
		t.Fatal("expected routing.auto-model-resolution to return to default after hot reload")
	}
	if !featureflags.GeminiAttachDefaultSafetySettingsEnabled() {
		t.Fatal("expected gemini.attach-default-safety-settings to return to default after hot reload")
	}
}

func TestServerUpdateClientsKeepsLocalManagementPasswordEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: proxyconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Port:                   0,
		AuthDir:                authDir,
		Debug:                  true,
		LoggingToFile:          false,
		UsageStatisticsEnabled: false,
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()
	configPath := filepath.Join(tmpDir, "config.yaml")
	server := NewServer(cfg, authManager, accessManager, configPath, WithLocalManagementPassword("local-password"))

	if !server.managementRoutesEnabled.Load() {
		t.Fatal("expected local management password to enable management routes at startup")
	}

	var updated proxyconfig.Config
	raw, err := yaml.Marshal(server.cfg)
	if err != nil {
		t.Fatalf("failed to copy server config: %v", err)
	}
	if err := yaml.Unmarshal(raw, &updated); err != nil {
		t.Fatalf("failed to decode copied server config: %v", err)
	}
	updated.RemoteManagement.SecretKey = ""

	server.UpdateClients(&updated)

	if !server.managementRoutesEnabled.Load() {
		t.Fatal("expected local management password to keep management routes enabled after hot reload")
	}
	if server.usageQueue == nil || !server.usageQueue.Enabled() {
		t.Fatal("expected redis queue to remain enabled while local management password is configured")
	}
	if server.mgmt == nil {
		t.Fatal("expected management handler to be initialized")
	}
	allowed, statusCode, errMsg := server.mgmt.AuthenticateManagementKey("127.0.0.1", true, "local-password")
	if !allowed || statusCode != 0 || errMsg != "" {
		t.Fatalf("expected local management password auth to succeed after hot reload, got allowed=%t status=%d msg=%q", allowed, statusCode, errMsg)
	}
}
