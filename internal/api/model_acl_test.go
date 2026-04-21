package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	accesspkg "github.com/router-for-me/CLIProxyAPI/v6/internal/access"
	management "github.com/router-for-me/CLIProxyAPI/v6/internal/api/handlers/management"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v6/sdk/access"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestRouter(cfg *config.Config, apiKey string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if apiKey != "" {
			c.Set("apiKey", apiKey)
		}
		c.Next()
	})
	router.Use(ModelACLMiddleware(func() *config.Config { return cfg }))

	router.POST("/v1/chat/completions", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", body)
	})
	router.POST("/v1beta/models/*action", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "path": c.Request.URL.Path})
	})
	router.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": []string{}})
	})
	return router
}

func newACLRouteStackRouter(t *testing.T, cfg *config.Config) *gin.Engine {
	t.Helper()

	manager := sdkaccess.NewManager()
	if _, err := accesspkg.ApplyAccessProviders(manager, nil, cfg); err != nil {
		t.Fatalf("failed to apply access providers: %v", err)
	}

	router := gin.New()
	cfgFn := func() *config.Config { return cfg }

	v1 := router.Group("/v1")
	v1.Use(AuthMiddleware(manager))
	v1.Use(ModelACLMiddleware(cfgFn))
	{
		v1.GET("/models", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"data": []string{}}) })
		v1.POST("/chat/completions", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		v1.GET("/responses", func(c *gin.Context) { c.Status(http.StatusOK) })
	}

	v1beta := router.Group("/v1beta")
	v1beta.Use(AuthMiddleware(manager))
	v1beta.Use(ModelACLMiddleware(cfgFn))
	{
		v1beta.GET("/models", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"models": []string{}}) })
		v1beta.POST("/models/*action", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	}

	provider := router.Group("/api/provider/:provider")
	provider.Use(AuthMiddleware(manager))
	provider.Use(ModelACLMiddleware(cfgFn))
	{
		provider.GET("/models", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"data": []string{}}) })
		provider.POST("/chat/completions", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

		v1Amp := provider.Group("/v1")
		v1Amp.GET("/models", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"data": []string{}}) })
		v1Amp.POST("/messages", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		v1Amp.POST("/messages/count_tokens", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		v1Amp.POST("/chat/completions", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

		v1betaAmp := provider.Group("/v1beta")
		v1betaAmp.GET("/models", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"models": []string{}}) })
		v1betaAmp.POST("/models/*action", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		provider.POST("/v1beta1/*path", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	}

	return router
}

func newProviderAliasTestRouter(cfg *config.Config, apiKey string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if apiKey != "" {
			c.Set("apiKey", apiKey)
		}
		c.Next()
	})
	router.Use(ModelACLMiddleware(func() *config.Config { return cfg }))

	router.POST("/api/provider/openai/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	router.POST("/api/provider/anthropic/v1/messages", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	router.POST("/api/provider/google/v1beta/models/*action", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	router.POST("/api/provider/google/v1beta1/*path", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	return router
}

func TestModelACLMiddleware_NoPoliciesAllowsAllByDefault(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	router := newTestRouter(cfg, "sk-anything")

	body, _ := json.Marshal(map[string]any{"model": "gpt-5"})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "gpt-5") {
		t.Fatalf("downstream handler did not see body: %s", w.Body.String())
	}
}

func TestModelACLMiddleware_AllowedModelPasses(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-narrow")

	body, _ := json.Marshal(map[string]any{"model": "gpt-4o-mini", "messages": []any{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_DisallowedModelRejected(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-narrow")

	body, _ := json.Marshal(map[string]any{"model": "claude-3-5-sonnet-20241022"})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "model_not_allowed_for_key") {
		t.Fatalf("expected error code in body, got %s", w.Body.String())
	}
}

func TestModelACLMiddleware_DenyAllDefaultRejectsUnknownKey(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeyDefaultPolicy: config.APIKeyDefaultPolicyDenyAll,
		},
	}
	router := newTestRouter(cfg, "sk-anything")

	body, _ := json.Marshal(map[string]any{"model": "gpt-5"})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_ExplicitEmptyPolicyUsesDenyAllDefault(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeyDefaultPolicy: config.APIKeyDefaultPolicyDenyAll,
			APIKeys:             []string{"sk-empty"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-empty", AllowedModels: nil},
			},
		},
	}
	router := newTestRouter(cfg, "sk-empty")

	body, _ := json.Marshal(map[string]any{"model": "gpt-4o-mini"})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_GeminiPathExtraction(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-gemini"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-gemini", AllowedModels: []string{"gemini-2.0-flash"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-gemini")

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.0-flash:generateContent", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("allowed gemini model: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-pro:generateContent", strings.NewReader("{}"))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("disallowed gemini model: expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_GeminiPathPreservesPrefix(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-prefixed"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-prefixed", AllowedModels: []string{"gemini-3-*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-prefixed")

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/teamA/gemini-3-pro:generateContent", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("allowed prefixed gemini model: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1beta/models/teamA/claude-3-sonnet:generateContent", strings.NewReader("{}"))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("disallowed prefixed gemini model: expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_OversizedBodyRejectedWith413(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-narrow")

	filler := bytes.Repeat([]byte("a"), int(modelACLMaxBodyBytes)+128)
	body := append([]byte(`{"model":"gpt-4o","pad":"`), filler...)
	body = append(body, '"', '}')

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "request_too_large") {
		t.Fatalf("expected request_too_large error type, got %s", w.Body.String())
	}
}

func TestModelACLMiddleware_OversizedBodyRejectedViaContentLength(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-narrow")

	body := []byte(`{"model":"gpt-4o"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = modelACLMaxBodyBytes + 1
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 via Content-Length, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_ReadErrorFailsClosed(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-narrow")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(errorReader{err: errors.New("boom")}))
	req.ContentLength = -1
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_request_body") {
		t.Fatalf("expected invalid_request_body error type, got %s", w.Body.String())
	}
}

func TestModelACLMiddleware_ModelInPeekLargeBodyPassesThrough(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-narrow")

	padSize := int(modelACLPeekBytes) * 4
	filler := bytes.Repeat([]byte("x"), padSize)
	body := append([]byte(`{"model":"gpt-4o-mini","pad":"`), filler...)
	body = append(body, '"', '}')

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String()[:min(200, len(w.Body.String()))])
	}
	if w.Body.Len() != len(body) {
		t.Fatalf("downstream handler received %d bytes, want %d", w.Body.Len(), len(body))
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatalf("downstream handler received mutated body")
	}
}

func TestModelACLMiddleware_ModelAfterPeekFallsBackCorrectly(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-narrow")

	padSize := int(modelACLPeekBytes) * 2
	filler := bytes.Repeat([]byte("y"), padSize)
	body := append([]byte(`{"pad":"`), filler...)
	body = append(body, '"', ',')
	body = append(body, []byte(`"model":"gpt-4o-late"}`)...)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (model allowed), got %d", w.Code)
	}
	if w.Body.Len() != len(body) {
		t.Fatalf("downstream body length %d != request length %d", w.Body.Len(), len(body))
	}
}

func TestModelACLMiddleware_WebsocketUpgradeRejectedForRestrictedKey(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("apiKey", "sk-narrow")
		c.Next()
	})
	router.Use(ModelACLMiddleware(func() *config.Config { return cfg }))

	handlerRan := false
	router.GET("/v1/responses", func(c *gin.Context) {
		handlerRan = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for ws upgrade on restricted key, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "websocket_not_allowed_for_restricted_key") {
		t.Fatalf("expected specific error type, got %s", w.Body.String())
	}
	if handlerRan {
		t.Fatalf("downstream websocket handler must not run when upgrade is rejected")
	}
}

func TestModelACLMiddleware_WebsocketUpgradeRejectedForExplicitEmptyPolicyUnderDenyAll(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeyDefaultPolicy: config.APIKeyDefaultPolicyDenyAll,
			APIKeys:             []string{"sk-empty"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-empty", AllowedModels: nil},
			},
		},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("apiKey", "sk-empty")
		c.Next()
	})
	router.Use(ModelACLMiddleware(func() *config.Config { return cfg }))
	router.GET("/v1/responses", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 under explicit empty policy + deny-all, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_WebsocketUpgradeAllowedForUnrestrictedKey(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-unrestricted", "sk-other"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-other", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("apiKey", "sk-unrestricted")
		c.Next()
	})
	router.Use(ModelACLMiddleware(func() *config.Config { return cfg }))
	router.GET("/v1/responses", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for ws upgrade on unrestricted key, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_OversizedBodyDoesNotDrainRemainder(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-narrow")

	prefix := append([]byte(`{"prompt":"`), bytes.Repeat([]byte("a"), int(modelACLMaxBodyBytes)+2048)...)
	body := &blockAfterReader{data: prefix}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.ContentLength = -1
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("middleware did not return within 2s; appears to be draining the body")
	}

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_UnrestrictedKeyBypassesInspectionCap(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-open", "sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newTestRouter(cfg, "sk-open")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini"}`))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = modelACLMaxBodyBytes + 1

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected unrestricted key to bypass ACL body cap, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_ListEndpointAlwaysAllowed(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeyDefaultPolicy: config.APIKeyDefaultPolicyDenyAll,
		},
	}
	router := newTestRouter(cfg, "sk-anything")
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on listing endpoint, got %d", w.Code)
	}
}

func TestModelACLMiddleware_RouteStackRejectsRestrictedKeysOnV1AndV1Beta(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newACLRouteStackRouter(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-3-5-sonnet-20241022"}`))
	req.Header.Set("Authorization", "Bearer sk-narrow")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("/v1 route expected 403, got %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-pro:generateContent", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer sk-narrow")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("/v1beta route expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_RouteStackRejectsRestrictedKeysOnProviderAliases(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-narrow"},
			APIKeyPolicies: []config.APIKeyPolicy{
				{Key: "sk-narrow", AllowedModels: []string{"gpt-4o*"}},
			},
		},
	}
	router := newProviderAliasTestRouter(cfg, "sk-narrow")

	req := httptest.NewRequest(http.MethodPost, "/api/provider/openai/chat/completions", strings.NewReader(`{"model":"claude-3-5-sonnet-20241022"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("/api/provider openai route expected 403, got %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/provider/anthropic/v1/messages", strings.NewReader(`{"model":"claude-3-5-sonnet-20241022"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("/api/provider anthropic route expected 403, got %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/provider/google/v1beta/models/gemini-1.5-pro:generateContent", strings.NewReader(`{}`))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("/api/provider google v1beta route expected 403, got %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/provider/google/v1beta1/publishers/google/models/gemini-1.5-pro:generateContent", strings.NewReader(`{}`))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("/api/provider google v1beta1 route expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_RouteStackDiscoveryRoutesRemainAccessible(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys:             []string{"sk-anything"},
			APIKeyDefaultPolicy: config.APIKeyDefaultPolicyDenyAll,
		},
	}
	router := newACLRouteStackRouter(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sk-anything")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/v1/models expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	req.Header.Set("Authorization", "Bearer sk-anything")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/v1beta/models expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelACLMiddleware_ConcurrentManagementMutationsAndReads(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"sk-race"},
		},
	}
	handler := management.NewHandler(cfg, configPath, nil)
	router := newTestRouter(cfg, "sk-race")

	primeRecorder := httptest.NewRecorder()
	primeCtx, _ := gin.CreateTestContext(primeRecorder)
	primeCtx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/api-keys", bytes.NewBufferString(`[{"key":"sk-race","allowedModels":["gpt-4o*"]}]`))
	primeCtx.Request.Header.Set("Content-Type", "application/json")
	handler.PutAPIKeys(primeCtx)
	if primeRecorder.Code != http.StatusOK {
		t.Fatalf("failed to prime policy state: %d %s", primeRecorder.Code, primeRecorder.Body.String())
	}

	done := make(chan struct{})
	errCh := make(chan error, 2)

	go func() {
		for i := 0; i < 80; i++ {
			body := `[{"key":"sk-race","allowedModels":["gpt-4o*"]}]`
			if i%2 == 1 {
				body = `[{"key":"sk-race","allowedModels":["claude-3-*"]}]`
			}
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/api-keys", bytes.NewBufferString(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			handler.PutAPIKeys(ctx)
			if rec.Code != http.StatusOK {
				errCh <- errors.New(rec.Body.String())
				close(done)
				return
			}
		}
		close(done)
	}()

	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK && w.Code != http.StatusForbidden {
				errCh <- errors.New(w.Body.String())
				return
			}
		}
	}()

	select {
	case <-done:
	case err := <-errCh:
		t.Fatalf("unexpected concurrent result: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent mutation/read test timed out")
	}

	select {
	case err := <-errCh:
		t.Fatalf("unexpected concurrent result: %v", err)
	default:
	}
}

type blockAfterReader struct {
	data []byte
	pos  int
}

func (r *blockAfterReader) Read(p []byte) (int, error) {
	if r.pos < len(r.data) {
		n := copy(p, r.data[r.pos:])
		r.pos += n
		return n, nil
	}
	select {}
}

type errorReader struct {
	err error
}

func (r errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}
