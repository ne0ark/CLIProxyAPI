package management

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestOpenAICompatibilityWithAuthIndex_PropagatesDisabled(t *testing.T) {
	t.Parallel()

	h := &Handler{
		cfg: &config.Config{
			OpenAICompatibility: []config.OpenAICompatibility{{
				Name:     "provider-a",
				BaseURL:  "https://provider-a.example.com",
				Disabled: true,
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
					APIKey:   "provider-key",
					ProxyURL: "http://proxy.example.com:8080",
				}},
			}},
		},
		authManager: coreauth.NewManager(nil, nil, nil),
	}
	disabledAuth := &coreauth.Auth{
		ID:       "openai-compatibility:provider-a:123",
		Provider: "provider-a",
		Disabled: true,
		Status:   coreauth.StatusDisabled,
		Attributes: map[string]string{
			"api_key":      "provider-key",
			"compat_name":  "provider-a",
			"provider_key": "provider-a",
		},
	}
	if _, errRegister := h.authManager.Register(context.Background(), disabledAuth); errRegister != nil {
		t.Fatalf("register disabled auth: %v", errRegister)
	}

	entries := h.openAICompatibilityWithAuthIndex()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if !entries[0].Disabled {
		t.Fatal("expected disabled flag to be exposed in management list output")
	}
	if entries[0].AuthIndex != "" {
		t.Fatalf("auth index = %q, want empty for disabled provider", entries[0].AuthIndex)
	}
	if len(entries[0].APIKeyEntries) != 1 {
		t.Fatalf("api key entries len = %d, want 1", len(entries[0].APIKeyEntries))
	}
	if entries[0].APIKeyEntries[0].AuthIndex != "" {
		t.Fatalf("api key auth index = %q, want empty for disabled provider", entries[0].APIKeyEntries[0].AuthIndex)
	}
}

func TestPatchOpenAICompat_UpdatesDisabledFlag(t *testing.T) {
	t.Parallel()
	setGinTestMode()

	h := &Handler{
		cfg: &config.Config{
			OpenAICompatibility: []config.OpenAICompatibility{{
				Name:    "provider-a",
				BaseURL: "https://provider-a.example.com",
			}},
		},
		configFilePath: writeTestConfigFile(t),
	}

	body := []byte(`{"name":"provider-a","value":{"disabled":true}}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/openai-compatibility", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PatchOpenAICompat(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !h.cfg.OpenAICompatibility[0].Disabled {
		t.Fatal("expected patch to persist disabled=true")
	}
}
