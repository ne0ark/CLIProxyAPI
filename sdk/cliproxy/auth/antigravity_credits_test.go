package auth

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type fixedFirstSelector struct{}

func (fixedFirstSelector) Pick(_ context.Context, _ string, _ string, _ cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	if len(auths) == 0 {
		return nil, &Error{Code: "auth_not_found", Message: "no auth available"}
	}
	return auths[0], nil
}

type providerOrderSelector struct {
	providers []string
}

func (s providerOrderSelector) Pick(_ context.Context, _ string, _ string, _ cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	if len(auths) == 0 {
		return nil, &Error{Code: "auth_not_found", Message: "no auth available"}
	}
	for _, provider := range s.providers {
		for _, auth := range auths {
			if auth != nil && auth.Provider == provider {
				return auth, nil
			}
		}
	}
	return auths[0], nil
}

type antigravityCreditsFallbackExecutor struct {
	executeCreditsRequested []bool
	executeAuthIDs          []string
	streamCreditsRequested  []bool
	streamAuthIDs           []string
}

func (e *antigravityCreditsFallbackExecutor) Identifier() string { return "antigravity" }

func (e *antigravityCreditsFallbackExecutor) Execute(ctx context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	creditsRequested := AntigravityCreditsRequested(ctx)
	e.executeCreditsRequested = append(e.executeCreditsRequested, creditsRequested)
	if auth != nil {
		e.executeAuthIDs = append(e.executeAuthIDs, auth.ID)
	} else {
		e.executeAuthIDs = append(e.executeAuthIDs, "")
	}
	if !creditsRequested {
		return cliproxyexecutor.Response{}, &Error{HTTPStatus: http.StatusTooManyRequests, Message: "quota exhausted"}
	}
	return cliproxyexecutor.Response{Payload: []byte("credits fallback")}, nil
}

func (e *antigravityCreditsFallbackExecutor) ExecuteStream(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, _ cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	creditsRequested := AntigravityCreditsRequested(ctx)
	e.streamCreditsRequested = append(e.streamCreditsRequested, creditsRequested)
	if auth != nil {
		e.streamAuthIDs = append(e.streamAuthIDs, auth.ID)
	} else {
		e.streamAuthIDs = append(e.streamAuthIDs, "")
	}
	ch := make(chan cliproxyexecutor.StreamChunk, 1)
	if !creditsRequested {
		ch <- cliproxyexecutor.StreamChunk{Err: &Error{HTTPStatus: http.StatusTooManyRequests, Message: "quota exhausted"}}
		close(ch)
		return &cliproxyexecutor.StreamResult{Headers: http.Header{"X-Initial": {req.Model}}, Chunks: ch}, nil
	}
	ch <- cliproxyexecutor.StreamChunk{Payload: []byte("credits fallback")}
	close(ch)
	return &cliproxyexecutor.StreamResult{Headers: http.Header{"X-Credits": {req.Model}}, Chunks: ch}, nil
}

func (e *antigravityCreditsFallbackExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

func (e *antigravityCreditsFallbackExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, &Error{HTTPStatus: http.StatusNotImplemented, Message: "CountTokens not implemented"}
}

func (e *antigravityCreditsFallbackExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, &Error{HTTPStatus: http.StatusNotImplemented, Message: "HttpRequest not implemented"}
}

type failingProviderExecutor struct {
	provider string
}

func (e *failingProviderExecutor) Identifier() string { return e.provider }

func (e *failingProviderExecutor) Execute(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, &Error{HTTPStatus: http.StatusInternalServerError, Message: "other provider failed"}
}

func (e *failingProviderExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, &Error{HTTPStatus: http.StatusInternalServerError, Message: "other provider failed"}
}

func (e *failingProviderExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

func (e *failingProviderExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, &Error{HTTPStatus: http.StatusInternalServerError, Message: "other provider failed"}
}

func (e *failingProviderExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, &Error{HTTPStatus: http.StatusInternalServerError, Message: "other provider failed"}
}

func TestManagerTryAntigravityCreditsExecuteStream_UsesCreditsContext(t *testing.T) {
	const model = "claude-opus-4-6-thinking"
	executor := &antigravityCreditsFallbackExecutor{}
	manager := NewManager(nil, fixedFirstSelector{}, nil)
	manager.SetConfig(&internalconfig.Config{
		QuotaExceeded: internalconfig.QuotaExceeded{AntigravityCredits: true},
	})
	manager.RegisterExecutor(executor)
	registry.GetGlobalRegistry().RegisterClient("ag-credits", "antigravity", []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("ag-credits") })
	if _, errRegister := manager.Register(context.Background(), &Auth{ID: "ag-credits", Provider: "antigravity"}); errRegister != nil {
		t.Fatalf("register auth: %v", errRegister)
	}
	manager.mu.Lock()
	manager.auths["ag-credits"] = &Auth{ID: "ag-credits", Provider: "antigravity"}
	manager.mu.Unlock()

	streamResult, ok := manager.tryAntigravityCreditsExecuteStream(context.Background(), []string{"antigravity"}, "", cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{})
	if !ok {
		t.Fatal("tryAntigravityCreditsExecuteStream() = false, want true")
	}

	var payload []byte
	for chunk := range streamResult.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		payload = append(payload, chunk.Payload...)
	}
	if string(payload) != "credits fallback" {
		t.Fatalf("payload = %q, want %q", string(payload), "credits fallback")
	}
	if got := streamResult.Headers.Get("X-Credits"); got != model {
		t.Fatalf("X-Credits header = %q, want routed model", got)
	}
	if len(executor.streamCreditsRequested) != 1 {
		t.Fatalf("stream calls = %d, want 1", len(executor.streamCreditsRequested))
	}
	if !executor.streamCreditsRequested[0] {
		t.Fatalf("credits flags = %v, want [true]", executor.streamCreditsRequested)
	}
}

func TestStatusCodeFromError_UnwrapsStreamBootstrap429(t *testing.T) {
	bootstrapErr := newStreamBootstrapError(&Error{HTTPStatus: http.StatusTooManyRequests, Message: "quota exhausted"}, nil)
	wrappedErr := fmt.Errorf("conductor stream failed: %w", bootstrapErr)

	if status := statusCodeFromError(wrappedErr); status != http.StatusTooManyRequests {
		t.Fatalf("statusCodeFromError() = %d, want %d", status, http.StatusTooManyRequests)
	}
}

func TestIsAuthBlockedForModel_ClaudeWithCreditsStillBlockedDuringCooldown(t *testing.T) {
	auth := &Auth{
		ID:       "ag-1",
		Provider: "antigravity",
		ModelStates: map[string]*ModelState{
			"claude-sonnet-4-6": {
				Unavailable:    true,
				NextRetryAfter: time.Now().Add(10 * time.Minute),
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: time.Now().Add(10 * time.Minute),
				},
			},
		},
	}

	SetAntigravityCreditsHint(auth.ID, AntigravityCreditsHint{
		Known:     true,
		Available: true,
		UpdatedAt: time.Now(),
	})

	blocked, reason, _ := isAuthBlockedForModel(auth, "claude-sonnet-4-6", time.Now())
	if !blocked || reason != blockReasonCooldown {
		t.Fatalf("expected auth to be blocked during cooldown even with credits, got blocked=%v reason=%v", blocked, reason)
	}
}

func TestIsAuthBlockedForModel_KeepsGeminiBlockedWithoutCreditsBypass(t *testing.T) {
	auth := &Auth{
		ID:       "ag-2",
		Provider: "antigravity",
		ModelStates: map[string]*ModelState{
			"gemini-3-flash": {
				Unavailable:    true,
				NextRetryAfter: time.Now().Add(10 * time.Minute),
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: time.Now().Add(10 * time.Minute),
				},
			},
		},
	}

	SetAntigravityCreditsHint(auth.ID, AntigravityCreditsHint{
		Known:     true,
		Available: true,
		UpdatedAt: time.Now(),
	})

	blocked, reason, _ := isAuthBlockedForModel(auth, "gemini-3-flash", time.Now())
	if !blocked || reason != blockReasonCooldown {
		t.Fatalf("expected gemini auth to remain blocked, got blocked=%v reason=%v", blocked, reason)
	}
}

func TestTryAntigravityCreditsExecute_PrefersTriggerAuthID(t *testing.T) {
	const model = "claude-sonnet-4-6"

	executor := &antigravityCreditsFallbackExecutor{}
	manager := NewManager(nil, fixedFirstSelector{}, nil)
	manager.SetConfig(&internalconfig.Config{
		QuotaExceeded: internalconfig.QuotaExceeded{AntigravityCredits: true},
	})
	manager.RegisterExecutor(executor)

	for _, authID := range []string{"zz-bound", "aa-other"} {
		SetAntigravityCreditsHint(authID, AntigravityCreditsHint{
			Known:     true,
			Available: true,
			UpdatedAt: time.Now(),
		})
		registry.GetGlobalRegistry().RegisterClient(authID, "antigravity", []*registry.ModelInfo{{ID: model}})
	}
	t.Cleanup(func() {
		for _, authID := range []string{"zz-bound", "aa-other"} {
			registry.GetGlobalRegistry().UnregisterClient(authID)
		}
	})

	manager.auths = map[string]*Auth{
		"zz-bound": {ID: "zz-bound", Provider: "antigravity"},
		"aa-other": {ID: "aa-other", Provider: "antigravity"},
	}

	resp, ok := manager.tryAntigravityCreditsExecute(context.Background(), []string{"antigravity", "openai"}, "zz-bound", cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{})
	if !ok {
		t.Fatal("tryAntigravityCreditsExecute() = false, want true")
	}
	if string(resp.Payload) != "credits fallback" {
		t.Fatalf("payload = %q, want %q", string(resp.Payload), "credits fallback")
	}
	if got := executor.executeAuthIDs; len(got) != 1 || got[0] != "zz-bound" {
		t.Fatalf("execute auth IDs = %v, want [zz-bound]", got)
	}
}

func TestManagerExecute_MixedProvidersPreservesAntigravityCreditsReplayTrigger(t *testing.T) {
	const model = "claude-sonnet-4-6"

	executor := &antigravityCreditsFallbackExecutor{}
	manager := NewManager(nil, providerOrderSelector{providers: []string{"antigravity", "openai"}}, nil)
	manager.SetConfig(&internalconfig.Config{
		QuotaExceeded: internalconfig.QuotaExceeded{AntigravityCredits: true},
	})
	manager.RegisterExecutor(executor)
	manager.RegisterExecutor(&failingProviderExecutor{provider: "openai"})

	SetAntigravityCreditsHint("ag-mixed-exec", AntigravityCreditsHint{
		Known:     true,
		Available: true,
		UpdatedAt: time.Now(),
	})
	registry.GetGlobalRegistry().RegisterClient("ag-mixed-exec", "antigravity", []*registry.ModelInfo{{ID: model}})
	registry.GetGlobalRegistry().RegisterClient("oa-mixed-exec", "openai", []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient("ag-mixed-exec")
		registry.GetGlobalRegistry().UnregisterClient("oa-mixed-exec")
	})

	manager.auths = map[string]*Auth{
		"ag-mixed-exec": {ID: "ag-mixed-exec", Provider: "antigravity"},
		"oa-mixed-exec": {ID: "oa-mixed-exec", Provider: "openai"},
	}

	resp, err := manager.Execute(context.Background(), []string{"antigravity", "openai"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if string(resp.Payload) != "credits fallback" {
		t.Fatalf("payload = %q, want %q", string(resp.Payload), "credits fallback")
	}
	if got := executor.executeCreditsRequested; len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("execute credits flags = %v, want [false true]", got)
	}
}

func TestManagerExecuteStream_MixedProvidersPreservesAntigravityCreditsReplayTrigger(t *testing.T) {
	const model = "claude-sonnet-4-6"

	executor := &antigravityCreditsFallbackExecutor{}
	manager := NewManager(nil, providerOrderSelector{providers: []string{"antigravity", "openai"}}, nil)
	manager.SetConfig(&internalconfig.Config{
		QuotaExceeded: internalconfig.QuotaExceeded{AntigravityCredits: true},
	})
	manager.RegisterExecutor(executor)
	manager.RegisterExecutor(&failingProviderExecutor{provider: "openai"})

	SetAntigravityCreditsHint("ag-mixed-stream", AntigravityCreditsHint{
		Known:     true,
		Available: true,
		UpdatedAt: time.Now(),
	})
	registry.GetGlobalRegistry().RegisterClient("ag-mixed-stream", "antigravity", []*registry.ModelInfo{{ID: model}})
	registry.GetGlobalRegistry().RegisterClient("oa-mixed-stream", "openai", []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient("ag-mixed-stream")
		registry.GetGlobalRegistry().UnregisterClient("oa-mixed-stream")
	})

	manager.auths = map[string]*Auth{
		"ag-mixed-stream": {ID: "ag-mixed-stream", Provider: "antigravity"},
		"oa-mixed-stream": {ID: "oa-mixed-stream", Provider: "openai"},
	}

	result, err := manager.ExecuteStream(context.Background(), []string{"antigravity", "openai"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("ExecuteStream() result = nil, want non-nil")
	}
	var payload []byte
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		payload = append(payload, chunk.Payload...)
	}
	if string(payload) != "credits fallback" {
		t.Fatalf("payload = %q, want %q", string(payload), "credits fallback")
	}
	if got := result.Headers.Get("X-Credits"); got != model {
		t.Fatalf("X-Credits header = %q, want %q", got, model)
	}
	if got := executor.streamCreditsRequested; len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("stream credits flags = %v, want [false true]", got)
	}
}

func TestRetryAfterFromError_UnwrapsAntigravityCreditsReplaySignal(t *testing.T) {
	retryAfter := 2 * time.Second
	err := wrapAntigravityCreditsReplaySignal(&strictBoundAuthError{
		cause:      &Error{HTTPStatus: http.StatusTooManyRequests, Message: "cooldown"},
		authID:     "auth-1",
		retryAfter: &retryAfter,
	}, &Error{HTTPStatus: http.StatusTooManyRequests, Message: "quota exhausted"}, "auth-1")

	got := retryAfterFromError(err)
	if got == nil {
		t.Fatal("retryAfterFromError() = nil, want non-nil")
	}
	if *got != retryAfter {
		t.Fatalf("retryAfterFromError() = %s, want %s", *got, retryAfter)
	}
}

func TestAntigravityCreditsReplayAuthID_PreservedWhenCauseEqualsTrigger(t *testing.T) {
	trigger := &Error{HTTPStatus: http.StatusTooManyRequests, Message: "quota exhausted"}
	err := wrapAntigravityCreditsReplaySignal(trigger, trigger, "auth-1")

	if got := antigravityCreditsReplayAuthID(err); got != "auth-1" {
		t.Fatalf("antigravityCreditsReplayAuthID() = %q, want %q", got, "auth-1")
	}
	if got := antigravityCreditsReplayTrigger(err); got != trigger {
		t.Fatalf("antigravityCreditsReplayTrigger() = %v, want %v", got, trigger)
	}
}
