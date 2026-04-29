package cliproxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func TestServiceRefreshModelRegistrationForAuth_UsesLatestSnapshotAndRebuildsScheduler(t *testing.T) {
	ctx := context.Background()
	server := newClaudeCountTokensTestServer()
	defer server.Close()

	service := newClaudeRefreshTestService()
	authID := "service-refresh-latest-snapshot"
	t.Cleanup(func() {
		GlobalModelRegistry().UnregisterClient(authID)
	})

	current := &coreauth.Auth{
		ID:       authID,
		Provider: "claude",
		Attributes: map[string]string{
			"api_key":  "key-123",
			"base_url": server.URL,
		},
	}
	service.applyCoreAuthAddOrUpdate(ctx, current)

	latest := current.Clone()
	latest.Attributes["excluded_models"] = "claude-opus-4-7"
	if _, err := service.coreManager.Update(ctx, latest); err != nil {
		t.Fatalf("update latest auth snapshot: %v", err)
	}

	if _, err := service.coreManager.ExecuteCount(ctx, []string{"claude"}, cliproxyexecutor.Request{
		Model:   "claude-opus-4-7",
		Payload: minimalClaudeMessagesPayload(),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}); err != nil {
		t.Fatalf("ExecuteCount() before refresh error = %v", err)
	}

	if ok := service.refreshModelRegistrationForAuth(current); !ok {
		t.Fatal("refreshModelRegistrationForAuth() = false, want true")
	}

	if clientHasModel(authID, "claude-opus-4-7") {
		t.Fatalf("expected latest auth snapshot exclusions to remove claude-opus-4-7 for %s", authID)
	}

	if _, err := service.coreManager.ExecuteCount(ctx, []string{"claude"}, cliproxyexecutor.Request{
		Model:   "claude-opus-4-7",
		Payload: minimalClaudeMessagesPayload(),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}); !isAuthNotFound(err) {
		t.Fatalf("expected excluded model to be unavailable after refresh, got %v", err)
	}
}

func TestServiceRefreshModelRegistrationForAuth_DropsStaleAvailabilityForDisabledOrMissingAuths(t *testing.T) {
	ctx := context.Background()
	server := newClaudeCountTokensTestServer()
	defer server.Close()

	t.Run("disabled latest auth", func(t *testing.T) {
		service := newClaudeRefreshTestService()
		authID := "service-refresh-disabled-auth"
		t.Cleanup(func() {
			GlobalModelRegistry().UnregisterClient(authID)
		})

		current := &coreauth.Auth{
			ID:       authID,
			Provider: "claude",
			Attributes: map[string]string{
				"api_key":  "key-123",
				"base_url": server.URL,
			},
		}
		service.applyCoreAuthAddOrUpdate(ctx, current)

		disabled := current.Clone()
		disabled.Disabled = true
		disabled.Status = coreauth.StatusDisabled
		if _, err := service.coreManager.Update(ctx, disabled); err != nil {
			t.Fatalf("update disabled auth snapshot: %v", err)
		}

		if ok := service.refreshModelRegistrationForAuth(current); ok {
			t.Fatal("refreshModelRegistrationForAuth() = true, want false for disabled auth")
		}
		if models := registry.GetGlobalRegistry().GetModelsForClient(authID); len(models) != 0 {
			t.Fatalf("expected no registered models for disabled auth, got %d", len(models))
		}
		if _, err := service.coreManager.ExecuteCount(ctx, []string{"claude"}, cliproxyexecutor.Request{
			Model:   "claude-opus-4-7",
			Payload: minimalClaudeMessagesPayload(),
		}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}); !isAuthNotFound(err) {
			t.Fatalf("expected disabled auth to be unavailable after refresh, got %v", err)
		}
	})

	t.Run("missing latest auth", func(t *testing.T) {
		service := newClaudeRefreshTestService()
		authID := "service-refresh-missing-auth"
		t.Cleanup(func() {
			GlobalModelRegistry().UnregisterClient(authID)
		})

		current := &coreauth.Auth{
			ID:       authID,
			Provider: "claude",
			Attributes: map[string]string{
				"api_key":  "key-123",
				"base_url": server.URL,
			},
		}

		if ok := service.refreshModelRegistrationForAuth(current); ok {
			t.Fatal("refreshModelRegistrationForAuth() = true, want false for missing auth")
		}
		if models := registry.GetGlobalRegistry().GetModelsForClient(authID); len(models) != 0 {
			t.Fatalf("expected no registered models for missing auth, got %d", len(models))
		}
		if _, err := service.coreManager.ExecuteCount(ctx, []string{"claude"}, cliproxyexecutor.Request{
			Model:   "claude-opus-4-7",
			Payload: minimalClaudeMessagesPayload(),
		}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}); !isAuthNotFound(err) {
			t.Fatalf("expected missing auth to be unavailable after refresh, got %v", err)
		}
	})
}

func newClaudeRefreshTestService() *Service {
	return &Service{
		cfg:         &config.Config{},
		coreManager: coreauth.NewManager(nil, &coreauth.RoundRobinSelector{}, nil),
	}
}

func newClaudeCountTokensTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":42}`))
	}))
}

func minimalClaudeMessagesPayload() []byte {
	return []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
}

func clientHasModel(authID, modelID string) bool {
	for _, model := range registry.GetGlobalRegistry().GetModelsForClient(authID) {
		if model != nil && model.ID == modelID {
			return true
		}
	}
	return false
}

func isAuthNotFound(err error) bool {
	if err == nil {
		return false
	}
	var authErr *coreauth.Error
	return errors.As(err, &authErr) && authErr != nil && authErr.Code == "auth_not_found"
}
