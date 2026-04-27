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
	provider        string
	executeFn       func(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error)
	executeStreamFn func(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error)
	countFn         func(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error)
	refreshFn       func(context.Context, *Auth) (*Auth, error)
}

func (e schedulerProviderTestExecutor) Identifier() string { return e.provider }

func (e schedulerProviderTestExecutor) Execute(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	if e.executeFn != nil {
		return e.executeFn(ctx, auth, req, opts)
	}
	return cliproxyexecutor.Response{}, nil
}

func (e schedulerProviderTestExecutor) ExecuteStream(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	if e.executeStreamFn != nil {
		return e.executeStreamFn(ctx, auth, req, opts)
	}
	return nil, nil
}

func (e schedulerProviderTestExecutor) Refresh(ctx context.Context, auth *Auth) (*Auth, error) {
	if e.refreshFn != nil {
		return e.refreshFn(ctx, auth)
	}
	return auth, nil
}

func (e schedulerProviderTestExecutor) CountTokens(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	if e.countFn != nil {
		return e.countFn(ctx, auth, req, opts)
	}
	return cliproxyexecutor.Response{}, nil
}

func (e schedulerProviderTestExecutor) HttpRequest(ctx context.Context, auth *Auth, req *http.Request) (*http.Response, error) {
	return nil, nil
}

type strictBoundAuthFailure interface {
	BoundAuthStrictFailure() bool
}

func assertStrictBoundAuthCooldownError(t *testing.T, err error, want time.Duration) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want strict bound auth cooldown error")
	}
	var strictErr strictBoundAuthFailure
	if !errors.As(err, &strictErr) || strictErr == nil || !strictErr.BoundAuthStrictFailure() {
		t.Fatalf("error = %T %v, want strict bound auth failure", err, err)
	}
	var cooldownErr *modelCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("error = %T %v, want wrapped *modelCooldownError", err, err)
	}
	retryAfter := retryAfterFromError(err)
	if retryAfter == nil {
		t.Fatal("retryAfterFromError() = nil, want cooldown wait")
	}
	delta := want - *retryAfter
	if delta < 0 {
		delta = -delta
	}
	if delta > 2*time.Second {
		t.Fatalf("retryAfterFromError() = %s, want approximately %s", *retryAfter, want)
	}
}

func assertStrictBoundAuthFailureMessage(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want strict bound auth failure")
	}
	var strictErr strictBoundAuthFailure
	if !errors.As(err, &strictErr) || strictErr == nil || !strictErr.BoundAuthStrictFailure() {
		t.Fatalf("error = %T %v, want strict bound auth failure", err, err)
	}
	if err.Error() != want {
		t.Fatalf("error message = %q, want %q", err.Error(), want)
	}
	if retryAfter := retryAfterFromError(err); retryAfter != nil {
		t.Fatalf("retryAfterFromError() = %v, want nil for plain failure", retryAfter)
	}
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

func TestManager_PickNext_StrictCodexWebsocketAffinity_PreservesBoundAuthAcrossPriorityChanges(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-low", "codex-high")

	highUnavailableUntil := time.Now().Add(30 * time.Minute)
	low := &Auth{
		ID:         "codex-low",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
	}
	high := &Auth{
		ID:         "codex-high",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			model: {
				Unavailable:    true,
				NextRetryAfter: highUnavailableUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: highUnavailableUntil,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), low); errRegister != nil {
		t.Fatalf("register low auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), high); errRegister != nil {
		t.Fatalf("register high auth: %v", errRegister)
	}

	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_codex-priority-recovery-test"}}`),
	}

	first, executor, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNext() initial error = %v", errPick)
	}
	if executor == nil {
		t.Fatal("pickNext() initial executor = nil")
	}
	if first == nil || first.ID != low.ID {
		t.Fatalf("pickNext() initial auth = %v, want %q", first, low.ID)
	}

	recoveredHigh := high.Clone()
	recoveredHigh.ModelStates = map[string]*ModelState{
		model: {},
	}
	if _, errUpdate := manager.Update(context.Background(), recoveredHigh); errUpdate != nil {
		t.Fatalf("update high auth: %v", errUpdate)
	}

	second, _, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNext() after higher-priority recovery error = %v", errPick)
	}
	if second == nil || second.ID != low.ID {
		t.Fatalf("pickNext() after higher-priority recovery auth = %v, want still-bound %q", second, low.ID)
	}
}

func TestManager_PickNext_StrictCodexWebsocketAffinity_ReturnsCooldownForBoundAuth(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-low", "codex-high")

	highUnavailableUntil := time.Now().Add(30 * time.Minute)
	low := &Auth{
		ID:         "codex-low",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
	}
	high := &Auth{
		ID:         "codex-high",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			model: {
				Unavailable:    true,
				NextRetryAfter: highUnavailableUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: highUnavailableUntil,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), low); errRegister != nil {
		t.Fatalf("register low auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), high); errRegister != nil {
		t.Fatalf("register high auth: %v", errRegister)
	}

	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_codex-cooldown-error-test"}}`),
	}

	first, _, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNext() initial error = %v", errPick)
	}
	if first == nil || first.ID != low.ID {
		t.Fatalf("pickNext() initial auth = %v, want %q", first, low.ID)
	}

	recoveredHigh := high.Clone()
	recoveredHigh.ModelStates = map[string]*ModelState{
		model: {},
	}
	if _, errUpdate := manager.Update(context.Background(), recoveredHigh); errUpdate != nil {
		t.Fatalf("update high auth: %v", errUpdate)
	}

	lowUnavailableUntil := time.Now().Add(45 * time.Minute)
	blockedLow := low.Clone()
	blockedLow.ModelStates = map[string]*ModelState{
		model: {
			Unavailable:    true,
			NextRetryAfter: lowUnavailableUntil,
			Quota: QuotaState{
				Exceeded:      true,
				NextRecoverAt: lowUnavailableUntil,
			},
		},
	}
	if _, errUpdate := manager.Update(context.Background(), blockedLow); errUpdate != nil {
		t.Fatalf("update low auth: %v", errUpdate)
	}

	second, _, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick == nil {
		t.Fatalf("pickNext() after bound auth cooldown = %v, want error", second)
	}
	if second != nil {
		t.Fatalf("pickNext() after bound auth cooldown auth = %v, want nil", second)
	}
	var cooldownErr *modelCooldownError
	if !errors.As(errPick, &cooldownErr) {
		t.Fatalf("pickNext() error = %T %v, want *modelCooldownError", errPick, errPick)
	}
}

func TestManager_PickNext_StrictCodexWebsocketAffinity_UsesSelectionModelForBoundAuth(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	const (
		model          = "prefix/gpt-5.4"
		selectionModel = "gpt-5.4"
	)
	registerSchedulerModels(t, "codex", selectionModel, "codex-low-prefix", "codex-high-prefix")

	highUnavailableUntil := time.Now().Add(30 * time.Minute)
	low := &Auth{
		ID:         "codex-low-prefix",
		Provider:   "codex",
		Prefix:     "prefix",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
	}
	high := &Auth{
		ID:         "codex-high-prefix",
		Provider:   "codex",
		Prefix:     "prefix",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			selectionModel: {
				Unavailable:    true,
				NextRetryAfter: highUnavailableUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: highUnavailableUntil,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), low); errRegister != nil {
		t.Fatalf("register low auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), high); errRegister != nil {
		t.Fatalf("register high auth: %v", errRegister)
	}

	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_codex-selection-model-test"}}`),
	}

	first, _, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNext() initial error = %v", errPick)
	}
	if first == nil || first.ID != low.ID {
		t.Fatalf("pickNext() initial auth = %v, want %q", first, low.ID)
	}

	recoveredHigh := high.Clone()
	recoveredHigh.ModelStates = map[string]*ModelState{
		selectionModel: {},
	}
	if _, errUpdate := manager.Update(context.Background(), recoveredHigh); errUpdate != nil {
		t.Fatalf("update high auth: %v", errUpdate)
	}

	lowUnavailableUntil := time.Now().Add(45 * time.Minute)
	blockedLow := low.Clone()
	blockedLow.ModelStates = map[string]*ModelState{
		selectionModel: {
			Unavailable:    true,
			NextRetryAfter: lowUnavailableUntil,
			Quota: QuotaState{
				Exceeded:      true,
				NextRecoverAt: lowUnavailableUntil,
			},
		},
	}
	if _, errUpdate := manager.Update(context.Background(), blockedLow); errUpdate != nil {
		t.Fatalf("update low auth: %v", errUpdate)
	}

	second, _, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick == nil {
		t.Fatalf("pickNext() after bound selection-model cooldown = %v, want error", second)
	}
	if second != nil {
		t.Fatalf("pickNext() after bound selection-model cooldown auth = %v, want nil", second)
	}
	var cooldownErr *modelCooldownError
	if !errors.As(errPick, &cooldownErr) {
		t.Fatalf("pickNext() error = %T %v, want *modelCooldownError", errPick, errPick)
	}
}

func TestManager_PickNext_StrictCodexWebsocketAffinity_AllCandidatesUnavailableUsesBoundAuthCooldown(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-a", "codex-b")

	boundUntil := time.Now().Add(45 * time.Minute)
	otherUntil := time.Now().Add(5 * time.Minute)
	bound := &Auth{
		ID:       "codex-a",
		Provider: "codex",
		Metadata: map[string]any{"websockets": true},
	}
	other := &Auth{
		ID:       "codex-b",
		Provider: "codex",
		Metadata: map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			model: {
				Unavailable:    true,
				NextRetryAfter: otherUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: otherUntil,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_codex-all-unavailable-test"}}`),
	}

	first, _, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNext() initial error = %v", errPick)
	}
	if first == nil || first.ID != bound.ID {
		t.Fatalf("pickNext() initial auth = %v, want %q", first, bound.ID)
	}

	blockedBound := bound.Clone()
	blockedBound.ModelStates = map[string]*ModelState{
		model: {
			Unavailable:    true,
			NextRetryAfter: boundUntil,
			Quota: QuotaState{
				Exceeded:      true,
				NextRecoverAt: boundUntil,
			},
		},
	}
	if _, errUpdate := manager.Update(context.Background(), blockedBound); errUpdate != nil {
		t.Fatalf("update bound auth: %v", errUpdate)
	}

	second, _, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick == nil {
		t.Fatalf("pickNext() after all candidates unavailable = %v, want error", second)
	}
	if second != nil {
		t.Fatalf("pickNext() after all candidates unavailable auth = %v, want nil", second)
	}
	var cooldownErr *modelCooldownError
	if !errors.As(errPick, &cooldownErr) {
		t.Fatalf("pickNext() error = %T %v, want *modelCooldownError", errPick, errPick)
	}
	if retryAfter := retryAfterFromError(errPick); retryAfter == nil || *retryAfter != 45*time.Minute {
		t.Fatalf("retryAfterFromError() = %v, want %s", retryAfter, 45*time.Minute)
	}
}

func TestManager_PickNextMixed_StrictCodexWebsocketAffinity_BlocksMixedFailover(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-mixed")
	registerSchedulerModels(t, "gemini", model, "gemini-mixed")

	geminiUnavailableUntil := time.Now().Add(30 * time.Minute)
	codexAuth := &Auth{
		ID:       "codex-mixed",
		Provider: "codex",
		Metadata: map[string]any{"websockets": true},
	}
	geminiAuth := &Auth{
		ID:       "gemini-mixed",
		Provider: "gemini",
		Metadata: map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			model: {
				Unavailable:    true,
				NextRetryAfter: geminiUnavailableUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: geminiUnavailableUntil,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), codexAuth); errRegister != nil {
		t.Fatalf("register codex auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), geminiAuth); errRegister != nil {
		t.Fatalf("register gemini auth: %v", errRegister)
	}

	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_codex-mixed-failover-test"}}`),
	}

	first, _, provider, errPick := manager.pickNextMixed(ctx, []string{"codex", "gemini"}, model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNextMixed() initial error = %v", errPick)
	}
	if first == nil || first.ID != codexAuth.ID {
		t.Fatalf("pickNextMixed() initial auth = %v, want %q", first, codexAuth.ID)
	}
	if provider != "codex" {
		t.Fatalf("pickNextMixed() initial provider = %q, want codex", provider)
	}

	recoveredGemini := geminiAuth.Clone()
	recoveredGemini.ModelStates = map[string]*ModelState{
		model: {},
	}
	if _, errUpdate := manager.Update(context.Background(), recoveredGemini); errUpdate != nil {
		t.Fatalf("update gemini auth: %v", errUpdate)
	}

	codexUnavailableUntil := time.Now().Add(45 * time.Minute)
	blockedCodex := codexAuth.Clone()
	blockedCodex.ModelStates = map[string]*ModelState{
		model: {
			Unavailable:    true,
			NextRetryAfter: codexUnavailableUntil,
			Quota: QuotaState{
				Exceeded:      true,
				NextRecoverAt: codexUnavailableUntil,
			},
		},
	}
	if _, errUpdate := manager.Update(context.Background(), blockedCodex); errUpdate != nil {
		t.Fatalf("update codex auth: %v", errUpdate)
	}

	second, _, provider, errPick := manager.pickNextMixed(ctx, []string{"codex", "gemini"}, model, opts, nil)
	if errPick == nil {
		t.Fatalf("pickNextMixed() after bound codex cooldown = %v, want error", second)
	}
	if second != nil {
		t.Fatalf("pickNextMixed() after bound codex cooldown auth = %v, want nil", second)
	}
	if provider != "" {
		t.Fatalf("pickNextMixed() after bound codex cooldown provider = %q, want empty", provider)
	}
	var cooldownErr *modelCooldownError
	if !errors.As(errPick, &cooldownErr) {
		t.Fatalf("pickNextMixed() error = %T %v, want *modelCooldownError", errPick, errPick)
	}
}

func TestManager_PickNextMixed_StrictCodexWebsocketAffinity_AllCandidatesUnavailableUsesBoundAuthCooldown(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-mixed-a")
	registerSchedulerModels(t, "gemini", model, "gemini-mixed-b")

	boundUntil := time.Now().Add(45 * time.Minute)
	otherUntil := time.Now().Add(5 * time.Minute)
	bound := &Auth{
		ID:       "codex-mixed-a",
		Provider: "codex",
		Metadata: map[string]any{"websockets": true},
	}
	other := &Auth{
		ID:       "gemini-mixed-b",
		Provider: "gemini",
		Metadata: map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			model: {
				Unavailable:    true,
				NextRetryAfter: otherUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: otherUntil,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_codex-mixed-all-unavailable-test"}}`),
	}

	first, _, provider, errPick := manager.pickNextMixed(ctx, []string{"codex", "gemini"}, model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNextMixed() initial error = %v", errPick)
	}
	if first == nil || first.ID != bound.ID {
		t.Fatalf("pickNextMixed() initial auth = %v, want %q", first, bound.ID)
	}
	if provider != "codex" {
		t.Fatalf("pickNextMixed() initial provider = %q, want codex", provider)
	}

	blockedBound := bound.Clone()
	blockedBound.ModelStates = map[string]*ModelState{
		model: {
			Unavailable:    true,
			NextRetryAfter: boundUntil,
			Quota: QuotaState{
				Exceeded:      true,
				NextRecoverAt: boundUntil,
			},
		},
	}
	if _, errUpdate := manager.Update(context.Background(), blockedBound); errUpdate != nil {
		t.Fatalf("update bound auth: %v", errUpdate)
	}

	second, _, provider, errPick := manager.pickNextMixed(ctx, []string{"codex", "gemini"}, model, opts, nil)
	if errPick == nil {
		t.Fatalf("pickNextMixed() after all candidates unavailable = %v, want error", second)
	}
	if second != nil {
		t.Fatalf("pickNextMixed() after all candidates unavailable auth = %v, want nil", second)
	}
	if provider != "" {
		t.Fatalf("pickNextMixed() after all candidates unavailable provider = %q, want empty", provider)
	}
	var cooldownErr *modelCooldownError
	if !errors.As(errPick, &cooldownErr) {
		t.Fatalf("pickNextMixed() error = %T %v, want *modelCooldownError", errPick, errPick)
	}
	if retryAfter := retryAfterFromError(errPick); retryAfter == nil || *retryAfter != 45*time.Minute {
		t.Fatalf("retryAfterFromError() = %v, want %s", retryAfter, 45*time.Minute)
	}
}

func TestManager_PickNext_StrictCodexWebsocketAffinity_TriedBoundAuthStillUsesBoundCooldown(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-bound-tried", "codex-other-tried")

	bound := &Auth{
		ID:         "codex-bound-tried",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
	}
	other := &Auth{
		ID:         "codex-other-tried",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
	}

	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_codex-tried-bound-test"}}`),
	}

	first, _, errPick := manager.pickNext(ctx, "codex", model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNext() initial error = %v", errPick)
	}
	if first == nil || first.ID != bound.ID {
		t.Fatalf("pickNext() initial auth = %v, want %q", first, bound.ID)
	}

	boundUntil := time.Now().Add(45 * time.Minute)
	blockedBound := bound.Clone()
	blockedBound.ModelStates = map[string]*ModelState{
		model: {
			Unavailable:    true,
			NextRetryAfter: boundUntil,
			Quota: QuotaState{
				Exceeded:      true,
				NextRecoverAt: boundUntil,
			},
		},
	}
	if _, errUpdate := manager.Update(context.Background(), blockedBound); errUpdate != nil {
		t.Fatalf("update bound auth: %v", errUpdate)
	}

	second, _, errPick := manager.pickNext(ctx, "codex", model, opts, map[string]struct{}{bound.ID: {}})
	if errPick == nil {
		t.Fatalf("pickNext() after tried bound auth = %v, want error", second)
	}
	if second != nil {
		t.Fatalf("pickNext() after tried bound auth auth = %v, want nil", second)
	}
	assertStrictBoundAuthCooldownError(t, errPick, 45*time.Minute)
}

func TestManager_PickNextMixed_StrictCodexWebsocketAffinity_TriedBoundAuthStillUsesBoundCooldown(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "codex"})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-bound-tried-mixed")
	registerSchedulerModels(t, "gemini", model, "gemini-other-tried-mixed")

	bound := &Auth{
		ID:         "codex-bound-tried-mixed",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
	}
	other := &Auth{
		ID:         "gemini-other-tried-mixed",
		Provider:   "gemini",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
	}

	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_codex-tried-bound-mixed-test"}}`),
	}

	first, _, provider, errPick := manager.pickNextMixed(ctx, []string{"codex", "gemini"}, model, opts, nil)
	if errPick != nil {
		t.Fatalf("pickNextMixed() initial error = %v", errPick)
	}
	if first == nil || first.ID != bound.ID {
		t.Fatalf("pickNextMixed() initial auth = %v, want %q", first, bound.ID)
	}
	if provider != "codex" {
		t.Fatalf("pickNextMixed() initial provider = %q, want codex", provider)
	}

	boundUntil := time.Now().Add(45 * time.Minute)
	blockedBound := bound.Clone()
	blockedBound.ModelStates = map[string]*ModelState{
		model: {
			Unavailable:    true,
			NextRetryAfter: boundUntil,
			Quota: QuotaState{
				Exceeded:      true,
				NextRecoverAt: boundUntil,
			},
		},
	}
	if _, errUpdate := manager.Update(context.Background(), blockedBound); errUpdate != nil {
		t.Fatalf("update bound auth: %v", errUpdate)
	}

	second, _, provider, errPick := manager.pickNextMixed(ctx, []string{"codex", "gemini"}, model, opts, map[string]struct{}{bound.ID: {}})
	if errPick == nil {
		t.Fatalf("pickNextMixed() after tried bound auth = %v, want error", second)
	}
	if second != nil {
		t.Fatalf("pickNextMixed() after tried bound auth auth = %v, want nil", second)
	}
	if provider != "" {
		t.Fatalf("pickNextMixed() after tried bound auth provider = %q, want empty", provider)
	}
	assertStrictBoundAuthCooldownError(t, errPick, 45*time.Minute)
}

func TestManager_ExecuteMixedOnce_PrefersStrictBoundAuthErrorOverPreviousExecutionError(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{
		provider: "codex",
		executeFn: func(_ context.Context, _ *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
			return cliproxyexecutor.Response{}, &retryAfterStatusError{
				message:    "codex cooldown",
				status:     http.StatusTooManyRequests,
				retryAfter: 45 * time.Minute,
			}
		},
	})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-execute-strict")
	registerSchedulerModels(t, "gemini", model, "gemini-execute-strict")

	bound := &Auth{
		ID:         "codex-execute-strict",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
	}
	other := &Auth{
		ID:         "gemini-execute-strict",
		Provider:   "gemini",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
	}
	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	_, errExec := manager.executeMixedOnce(ctx, []string{"codex", "gemini"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_execute-mixed-strict-test"}}`),
	}, 2)
	assertStrictBoundAuthCooldownError(t, errExec, 45*time.Minute)
}

func TestManager_ExecuteCountMixedOnce_PrefersStrictBoundAuthErrorOverPreviousExecutionError(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{
		provider: "codex",
		countFn: func(_ context.Context, _ *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
			return cliproxyexecutor.Response{}, &retryAfterStatusError{
				message:    "codex cooldown",
				status:     http.StatusTooManyRequests,
				retryAfter: 45 * time.Minute,
			}
		},
	})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-count-strict")
	registerSchedulerModels(t, "gemini", model, "gemini-count-strict")

	bound := &Auth{
		ID:         "codex-count-strict",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
	}
	other := &Auth{
		ID:         "gemini-count-strict",
		Provider:   "gemini",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
	}
	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	_, errExec := manager.executeCountMixedOnce(ctx, []string{"codex", "gemini"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_execute-count-mixed-strict-test"}}`),
	}, 2)
	assertStrictBoundAuthCooldownError(t, errExec, 45*time.Minute)
}

func TestManager_ExecuteStreamMixedOnce_PrefersStrictBoundAuthErrorOverPreviousBootstrapError(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{
		provider: "codex",
		executeStreamFn: func(_ context.Context, _ *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
			chunks := make(chan cliproxyexecutor.StreamChunk, 1)
			chunks <- cliproxyexecutor.StreamChunk{Err: &retryAfterStatusError{
				message:    "codex cooldown",
				status:     http.StatusTooManyRequests,
				retryAfter: 45 * time.Minute,
			}}
			close(chunks)
			return &cliproxyexecutor.StreamResult{
				Headers: http.Header{"X-Test": []string{"bootstrap"}},
				Chunks:  chunks,
			}, nil
		},
	})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-stream-strict")
	registerSchedulerModels(t, "gemini", model, "gemini-stream-strict")

	bound := &Auth{
		ID:         "codex-stream-strict",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
	}
	other := &Auth{
		ID:         "gemini-stream-strict",
		Provider:   "gemini",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
	}
	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	streamResult, errStream := manager.executeStreamMixedOnce(ctx, []string{"codex", "gemini"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_execute-stream-mixed-strict-test"}}`),
	}, 2)
	if streamResult != nil {
		t.Fatalf("executeStreamMixedOnce() streamResult = %v, want nil when returning strict bound auth error", streamResult)
	}
	assertStrictBoundAuthCooldownError(t, errStream, 45*time.Minute)
}

func TestManager_ExecuteMixedOnce_StrictCodexWebsocketAffinity_PreservesPlainBoundAuthError(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{
		provider: "codex",
		executeFn: func(_ context.Context, _ *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
			return cliproxyexecutor.Response{}, errors.New("plain codex failure")
		},
	})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-execute-plain")
	registerSchedulerModels(t, "gemini", model, "gemini-execute-plain")

	bound := &Auth{
		ID:         "codex-execute-plain",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
	}
	otherUntil := time.Now().Add(5 * time.Minute)
	other := &Auth{
		ID:         "gemini-execute-plain",
		Provider:   "gemini",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			model: {
				Unavailable:    true,
				NextRetryAfter: otherUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: otherUntil,
				},
			},
		},
	}
	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	_, errExec := manager.executeMixedOnce(ctx, []string{"codex", "gemini"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_execute-mixed-plain-strict-test"}}`),
	}, 2)
	assertStrictBoundAuthFailureMessage(t, errExec, "plain codex failure")
}

func TestManager_ExecuteCountMixedOnce_StrictCodexWebsocketAffinity_PreservesPlainBoundAuthError(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{
		provider: "codex",
		countFn: func(_ context.Context, _ *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
			return cliproxyexecutor.Response{}, errors.New("plain codex count failure")
		},
	})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-count-plain")
	registerSchedulerModels(t, "gemini", model, "gemini-count-plain")

	bound := &Auth{
		ID:         "codex-count-plain",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
	}
	otherUntil := time.Now().Add(5 * time.Minute)
	other := &Auth{
		ID:         "gemini-count-plain",
		Provider:   "gemini",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			model: {
				Unavailable:    true,
				NextRetryAfter: otherUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: otherUntil,
				},
			},
		},
	}
	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	_, errExec := manager.executeCountMixedOnce(ctx, []string{"codex", "gemini"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_execute-count-plain-strict-test"}}`),
	}, 2)
	assertStrictBoundAuthFailureMessage(t, errExec, "plain codex count failure")
}

func TestManager_ExecuteStreamMixedOnce_StrictCodexWebsocketAffinity_PreservesPlainBoundAuthError(t *testing.T) {
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	manager := NewManager(nil, NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Minute,
		CodexWebsocketStrictAffinity: true,
	}), nil)
	manager.RegisterExecutor(schedulerProviderTestExecutor{
		provider: "codex",
		executeStreamFn: func(_ context.Context, _ *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
			return nil, errors.New("plain codex stream failure")
		},
	})
	manager.RegisterExecutor(schedulerProviderTestExecutor{provider: "gemini"})

	const model = "gpt-5.4"
	registerSchedulerModels(t, "codex", model, "codex-stream-plain")
	registerSchedulerModels(t, "gemini", model, "gemini-stream-plain")

	bound := &Auth{
		ID:         "codex-stream-plain",
		Provider:   "codex",
		Attributes: map[string]string{"priority": "10"},
		Metadata:   map[string]any{"websockets": true},
	}
	otherUntil := time.Now().Add(5 * time.Minute)
	other := &Auth{
		ID:         "gemini-stream-plain",
		Provider:   "gemini",
		Attributes: map[string]string{"priority": "0"},
		Metadata:   map[string]any{"websockets": true},
		ModelStates: map[string]*ModelState{
			model: {
				Unavailable:    true,
				NextRetryAfter: otherUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: otherUntil,
				},
			},
		},
	}
	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	streamResult, errStream := manager.executeStreamMixedOnce(ctx, []string{"codex", "gemini"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"user_xxx_account__session_execute-stream-plain-strict-test"}}`),
	}, 2)
	if streamResult != nil {
		t.Fatalf("executeStreamMixedOnce() streamResult = %v, want nil when returning strict bound auth error", streamResult)
	}
	assertStrictBoundAuthFailureMessage(t, errStream, "plain codex stream failure")
}

func TestManager_ShouldRetryAfterError_StrictBoundAuthUsesPinnedRetryAfter(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.requestRetry.Store(1)

	now := time.Now()
	boundUntil := now.Add(45 * time.Minute)
	otherUntil := now.Add(5 * time.Minute)
	bound := &Auth{
		ID:       "bound-codex",
		Provider: "codex",
		ModelStates: map[string]*ModelState{
			"gpt-5.4": {
				Unavailable:    true,
				NextRetryAfter: boundUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: boundUntil,
				},
			},
		},
	}
	other := &Auth{
		ID:       "other-codex",
		Provider: "codex",
		ModelStates: map[string]*ModelState{
			"gpt-5.4": {
				Unavailable:    true,
				NextRetryAfter: otherUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: otherUntil,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	errStrict := boundAuthUnavailableError([]*Auth{bound, other}, bound.ID, "codex", "codex", "gpt-5.4", now)
	wait, ok := manager.shouldRetryAfterError(errStrict, 0, []string{"codex"}, "gpt-5.4", time.Hour)
	if !ok {
		t.Fatal("shouldRetryAfterError() ok = false, want true")
	}
	if wait != 45*time.Minute {
		t.Fatalf("shouldRetryAfterError() wait = %s, want %s", wait, 45*time.Minute)
	}
}

func TestManager_ShouldRetryAfterError_StrictBoundAuthUsesPinnedRetryAfterForNonQuotaCooldown(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.requestRetry.Store(1)

	now := time.Now()
	boundUntil := now.Add(20 * time.Minute)
	bound := &Auth{
		ID:       "bound-codex-nonquota",
		Provider: "codex",
		ModelStates: map[string]*ModelState{
			"gpt-5.4": {
				Unavailable:    true,
				NextRetryAfter: boundUntil,
				LastError: &Error{
					Code:       "upstream_unavailable",
					Message:    "upstream temporarily unavailable",
					HTTPStatus: http.StatusBadGateway,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}

	errStrict := boundAuthUnavailableError([]*Auth{bound}, bound.ID, "codex", "codex", "gpt-5.4", now)
	wait, ok := manager.shouldRetryAfterError(errStrict, 0, []string{"codex"}, "gpt-5.4", time.Hour)
	if !ok {
		t.Fatal("shouldRetryAfterError() ok = false, want true")
	}
	if wait != 20*time.Minute {
		t.Fatalf("shouldRetryAfterError() wait = %s, want %s", wait, 20*time.Minute)
	}
}

func TestManager_ShouldRetryAfterError_StrictBoundAuthUnavailableDoesNotRetryOtherCooldowns(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.requestRetry.Store(1)

	now := time.Now()
	otherUntil := now.Add(5 * time.Minute)
	other := &Auth{
		ID:       "other-codex",
		Provider: "codex",
		ModelStates: map[string]*ModelState{
			"gpt-5.4": {
				Unavailable:    true,
				NextRetryAfter: otherUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: otherUntil,
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	errStrict := boundAuthUnavailableError([]*Auth{other}, "missing-codex", "mixed", "codex", "gpt-5.4", now)
	wait, ok := manager.shouldRetryAfterError(errStrict, 0, []string{"codex"}, "gpt-5.4", time.Hour)
	if ok {
		t.Fatalf("shouldRetryAfterError() = %s, true, want no retry", wait)
	}
}

func TestManager_ShouldRetryAfterError_StrictBoundAuthRespectsBoundAuthRetryOverride(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetRetryConfig(3, time.Hour, 0)

	now := time.Now()
	boundUntil := now.Add(10 * time.Minute)
	bound := &Auth{
		ID:       "bound-codex-override",
		Provider: "codex",
		Metadata: map[string]any{"request_retry": float64(0)},
		ModelStates: map[string]*ModelState{
			"gpt-5.4": {
				Unavailable:    true,
				NextRetryAfter: boundUntil,
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: boundUntil,
				},
			},
		},
	}
	other := &Auth{
		ID:       "other-codex-override",
		Provider: "codex",
		Metadata: map[string]any{"request_retry": float64(3)},
		ModelStates: map[string]*ModelState{
			"gpt-5.4": {
				Unavailable:    true,
				NextRetryAfter: now.Add(2 * time.Minute),
				Quota: QuotaState{
					Exceeded:      true,
					NextRecoverAt: now.Add(2 * time.Minute),
				},
			},
		},
	}

	if _, errRegister := manager.Register(context.Background(), bound); errRegister != nil {
		t.Fatalf("register bound auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), other); errRegister != nil {
		t.Fatalf("register other auth: %v", errRegister)
	}

	errStrict := boundAuthUnavailableError([]*Auth{bound, other}, bound.ID, "codex", "codex", "gpt-5.4", now)
	wait, ok := manager.shouldRetryAfterError(errStrict, 0, []string{"codex"}, "gpt-5.4", time.Hour)
	if ok {
		t.Fatalf("shouldRetryAfterError() = %s, true, want bound auth override to block retry", wait)
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
