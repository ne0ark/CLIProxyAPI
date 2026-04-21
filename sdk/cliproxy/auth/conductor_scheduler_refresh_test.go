package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type schedulerProviderTestExecutor struct {
	provider  string
	refreshFn func(context.Context, *Auth) (*Auth, error)
}

func (e schedulerProviderTestExecutor) Identifier() string { return e.provider }

func (e schedulerProviderTestExecutor) Execute(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e schedulerProviderTestExecutor) ExecuteStream(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, nil
}

func (e schedulerProviderTestExecutor) Refresh(ctx context.Context, auth *Auth) (*Auth, error) {
	if e.refreshFn != nil {
		return e.refreshFn(ctx, auth)
	}
	return auth, nil
}

func (e schedulerProviderTestExecutor) CountTokens(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e schedulerProviderTestExecutor) HttpRequest(ctx context.Context, auth *Auth, req *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestManager_RefreshSchedulerEntry_RebuildsSupportedModelSetAfterModelRegistration(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name  string
		prime func(*Manager, *Auth) error
	}{
		{
			name: "register",
			prime: func(manager *Manager, auth *Auth) error {
				_, errRegister := manager.Register(ctx, auth)
				return errRegister
			},
		},
		{
			name: "update",
			prime: func(manager *Manager, auth *Auth) error {
				_, errRegister := manager.Register(ctx, auth)
				if errRegister != nil {
					return errRegister
				}
				updated := auth.Clone()
				updated.Metadata = map[string]any{"updated": true}
				_, errUpdate := manager.Update(ctx, updated)
				return errUpdate
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			manager := NewManager(nil, &RoundRobinSelector{}, nil)
			auth := &Auth{
				ID:       "refresh-entry-" + testCase.name,
				Provider: "gemini",
			}
			if errPrime := testCase.prime(manager, auth); errPrime != nil {
				t.Fatalf("prime auth %s: %v", testCase.name, errPrime)
			}

			registerSchedulerModels(t, "gemini", "scheduler-refresh-model", auth.ID)

			got, errPick := manager.scheduler.pickSingle(ctx, "gemini", "scheduler-refresh-model", cliproxyexecutor.Options{}, nil)
			var authErr *Error
			if !errors.As(errPick, &authErr) || authErr == nil {
				t.Fatalf("pickSingle() before refresh error = %v, want auth_not_found", errPick)
			}
			if authErr.Code != "auth_not_found" {
				t.Fatalf("pickSingle() before refresh code = %q, want %q", authErr.Code, "auth_not_found")
			}
			if got != nil {
				t.Fatalf("pickSingle() before refresh auth = %v, want nil", got)
			}

			manager.RefreshSchedulerEntry(auth.ID)

			got, errPick = manager.scheduler.pickSingle(ctx, "gemini", "scheduler-refresh-model", cliproxyexecutor.Options{}, nil)
			if errPick != nil {
				t.Fatalf("pickSingle() after refresh error = %v", errPick)
			}
			if got == nil || got.ID != auth.ID {
				t.Fatalf("pickSingle() after refresh auth = %v, want %q", got, auth.ID)
			}
		})
	}
}

func TestManager_PickNext_RebuildsSchedulerAfterModelCooldownError(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(nil, &RoundRobinSelector{}, nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	registerSchedulerModels(t, "gemini", "scheduler-cooldown-rebuild-model", "cooldown-stale-old")

	oldAuth := &Auth{
		ID:       "cooldown-stale-old",
		Provider: "gemini",
	}
	if _, errRegister := manager.Register(ctx, oldAuth); errRegister != nil {
		t.Fatalf("register old auth: %v", errRegister)
	}

	manager.MarkResult(ctx, Result{
		AuthID:   oldAuth.ID,
		Provider: "gemini",
		Model:    "scheduler-cooldown-rebuild-model",
		Success:  false,
		Error:    &Error{HTTPStatus: http.StatusTooManyRequests, Message: "quota"},
	})

	newAuth := &Auth{
		ID:       "cooldown-stale-new",
		Provider: "gemini",
	}
	if _, errRegister := manager.Register(ctx, newAuth); errRegister != nil {
		t.Fatalf("register new auth: %v", errRegister)
	}

	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(newAuth.ID, "gemini", []*registry.ModelInfo{{ID: "scheduler-cooldown-rebuild-model"}})
	t.Cleanup(func() {
		reg.UnregisterClient(newAuth.ID)
	})

	got, errPick := manager.scheduler.pickSingle(ctx, "gemini", "scheduler-cooldown-rebuild-model", cliproxyexecutor.Options{}, nil)
	var cooldownErr *modelCooldownError
	if !errors.As(errPick, &cooldownErr) {
		t.Fatalf("pickSingle() before sync error = %v, want modelCooldownError", errPick)
	}
	if got != nil {
		t.Fatalf("pickSingle() before sync auth = %v, want nil", got)
	}

	got, executor, errPick := manager.pickNext(ctx, "gemini", "scheduler-cooldown-rebuild-model", cliproxyexecutor.Options{}, nil)
	if errPick != nil {
		t.Fatalf("pickNext() error = %v", errPick)
	}
	if executor == nil {
		t.Fatal("pickNext() executor = nil")
	}
	if got == nil || got.ID != newAuth.ID {
		t.Fatalf("pickNext() auth = %v, want %q", got, newAuth.ID)
	}
}

func TestManager_RefreshAuth_BacksOffIneffectiveRefresh(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(nil, &RoundRobinSelector{}, nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{
		provider: "gemini",
		refreshFn: func(_ context.Context, auth *Auth) (*Auth, error) {
			updated := auth.Clone()
			if updated.Metadata == nil {
				updated.Metadata = make(map[string]any)
			}
			updated.Metadata["access_token"] = "refreshed-token"
			return updated, nil
		},
	})

	expiredAt := time.Now().Add(-time.Minute)
	auth := &Auth{
		ID:       "ineffective-refresh-auth",
		Provider: "gemini",
		Metadata: map[string]any{
			"email":                    "refresh@example.com",
			"access_token":             "stale-token",
			"expires_at":               expiredAt.Format(time.RFC3339),
			"refresh_interval_seconds": 300,
		},
	}

	if _, errRegister := manager.Register(ctx, auth); errRegister != nil {
		t.Fatalf("register auth: %v", errRegister)
	}

	before := time.Now()
	manager.refreshAuth(ctx, auth.ID)
	after := time.Now()

	manager.mu.RLock()
	refreshed := manager.auths[auth.ID].Clone()
	manager.mu.RUnlock()
	if refreshed == nil {
		t.Fatal("refreshed auth = nil")
	}
	if got := refreshed.Metadata["access_token"]; got != "refreshed-token" {
		t.Fatalf("access_token = %v, want refreshed-token", got)
	}
	if refreshed.NextRefreshAfter.IsZero() {
		t.Fatal("NextRefreshAfter = zero, want ineffective-refresh backoff")
	}
	lowerBound := before.Add(refreshIneffectiveBackoff)
	upperBound := after.Add(refreshIneffectiveBackoff)
	if refreshed.NextRefreshAfter.Before(lowerBound) || refreshed.NextRefreshAfter.After(upperBound) {
		t.Fatalf("NextRefreshAfter = %s, want between %s and %s", refreshed.NextRefreshAfter, lowerBound, upperBound)
	}
	if manager.shouldRefresh(refreshed, time.Now()) {
		t.Fatal("shouldRefresh() = true during ineffective-refresh backoff, want false")
	}
}

func TestManager_ShouldRefresh_AllowsRetryAfterIneffectiveRefreshBackoffExpires(t *testing.T) {
	now := time.Now()
	nextAfter := now.Add(refreshIneffectiveBackoff)
	auth := &Auth{
		ID:               "ineffective-refresh-gate",
		Provider:         "gemini",
		NextRefreshAfter: nextAfter,
		Metadata: map[string]any{
			"email":                    "refresh@example.com",
			"expires_at":               now.Add(-time.Minute).Format(time.RFC3339),
			"refresh_interval_seconds": 300,
		},
	}

	manager := NewManager(nil, &RoundRobinSelector{}, nil)

	if manager.shouldRefresh(auth, now) {
		t.Fatal("shouldRefresh() = true before ineffective-refresh backoff expires, want false")
	}
	got, ok := nextRefreshCheckAt(now, auth, refreshCheckInterval)
	if !ok {
		t.Fatal("nextRefreshCheckAt() ok = false, want true")
	}
	if !got.Equal(nextAfter) {
		t.Fatalf("nextRefreshCheckAt() = %s, want %s", got, nextAfter)
	}

	afterBackoff := nextAfter.Add(time.Nanosecond)
	if !manager.shouldRefresh(auth, afterBackoff) {
		t.Fatal("shouldRefresh() = false after ineffective-refresh backoff expires, want true")
	}
	got, ok = nextRefreshCheckAt(afterBackoff, auth, refreshCheckInterval)
	if !ok {
		t.Fatal("nextRefreshCheckAt() after backoff ok = false, want true")
	}
	if !got.Equal(afterBackoff) {
		t.Fatalf("nextRefreshCheckAt() after backoff = %s, want %s", got, afterBackoff)
	}
}
