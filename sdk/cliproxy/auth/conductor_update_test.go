package auth

import (
	"context"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type stoppableSelectorStub struct {
	stops int
}

func (s *stoppableSelectorStub) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	return nil, nil
}

func (s *stoppableSelectorStub) Stop() {
	s.stops++
}

type nonComparableSelectorStub struct {
	stops *int
	tags  []string
}

func (s nonComparableSelectorStub) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	return nil, nil
}

func (s nonComparableSelectorStub) Stop() {
	if s.stops != nil {
		*s.stops++
	}
}

func TestManager_Update_PreservesModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	model := "test-model"
	backoffLevel := 7

	if _, errRegister := m.Register(context.Background(), &Auth{
		ID:       "auth-1",
		Provider: "claude",
		Metadata: map[string]any{"k": "v"},
		ModelStates: map[string]*ModelState{
			model: {
				Quota: QuotaState{BackoffLevel: backoffLevel},
			},
		},
	}); errRegister != nil {
		t.Fatalf("register auth: %v", errRegister)
	}

	if _, errUpdate := m.Update(context.Background(), &Auth{
		ID:       "auth-1",
		Provider: "claude",
		Metadata: map[string]any{"k": "v2"},
	}); errUpdate != nil {
		t.Fatalf("update auth: %v", errUpdate)
	}

	updated, ok := m.GetByID("auth-1")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) == 0 {
		t.Fatalf("expected ModelStates to be preserved")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatalf("expected model state to be present")
	}
	if state.Quota.BackoffLevel != backoffLevel {
		t.Fatalf("expected BackoffLevel to be %d, got %d", backoffLevel, state.Quota.BackoffLevel)
	}
}

func TestManager_Update_DisabledExistingDoesNotInheritModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	// Register a disabled auth with existing ModelStates.
	if _, err := m.Register(context.Background(), &Auth{
		ID:       "auth-disabled",
		Provider: "claude",
		Disabled: true,
		Status:   StatusDisabled,
		ModelStates: map[string]*ModelState{
			"stale-model": {
				Quota: QuotaState{BackoffLevel: 5},
			},
		},
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// Update with empty ModelStates — should NOT inherit stale states.
	if _, err := m.Update(context.Background(), &Auth{
		ID:       "auth-disabled",
		Provider: "claude",
		Disabled: true,
		Status:   StatusDisabled,
	}); err != nil {
		t.Fatalf("update auth: %v", err)
	}

	updated, ok := m.GetByID("auth-disabled")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) != 0 {
		t.Fatalf("expected disabled auth NOT to inherit ModelStates, got %d entries", len(updated.ModelStates))
	}
}

func TestManager_Update_ActiveToDisabledDoesNotInheritModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	// Register an active auth with ModelStates (simulates existing live auth).
	if _, err := m.Register(context.Background(), &Auth{
		ID:       "auth-a2d",
		Provider: "claude",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			"stale-model": {
				Quota: QuotaState{BackoffLevel: 9},
			},
		},
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// File watcher deletes config → synthesizes Disabled=true auth → Update.
	// Even though existing is active, incoming auth is disabled → skip inheritance.
	if _, err := m.Update(context.Background(), &Auth{
		ID:       "auth-a2d",
		Provider: "claude",
		Disabled: true,
		Status:   StatusDisabled,
	}); err != nil {
		t.Fatalf("update auth: %v", err)
	}

	updated, ok := m.GetByID("auth-a2d")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) != 0 {
		t.Fatalf("expected active→disabled transition NOT to inherit ModelStates, got %d entries", len(updated.ModelStates))
	}
}

func TestManager_Update_DisabledToActiveDoesNotInheritStaleModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	// Register a disabled auth with stale ModelStates.
	if _, err := m.Register(context.Background(), &Auth{
		ID:       "auth-d2a",
		Provider: "claude",
		Disabled: true,
		Status:   StatusDisabled,
		ModelStates: map[string]*ModelState{
			"stale-model": {
				Quota: QuotaState{BackoffLevel: 4},
			},
		},
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// Re-enable: incoming auth is active, existing is disabled → skip inheritance.
	if _, err := m.Update(context.Background(), &Auth{
		ID:       "auth-d2a",
		Provider: "claude",
		Status:   StatusActive,
	}); err != nil {
		t.Fatalf("update auth: %v", err)
	}

	updated, ok := m.GetByID("auth-d2a")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) != 0 {
		t.Fatalf("expected disabled→active transition NOT to inherit stale ModelStates, got %d entries", len(updated.ModelStates))
	}
}

func TestManager_Update_ActiveInheritsModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	model := "active-model"
	backoffLevel := 3

	// Register an active auth with ModelStates.
	if _, err := m.Register(context.Background(), &Auth{
		ID:       "auth-active",
		Provider: "claude",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			model: {
				Quota: QuotaState{BackoffLevel: backoffLevel},
			},
		},
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// Update with empty ModelStates — both sides active → SHOULD inherit.
	if _, err := m.Update(context.Background(), &Auth{
		ID:       "auth-active",
		Provider: "claude",
		Status:   StatusActive,
	}); err != nil {
		t.Fatalf("update auth: %v", err)
	}

	updated, ok := m.GetByID("auth-active")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) == 0 {
		t.Fatalf("expected active auth to inherit ModelStates")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatalf("expected model state to be present")
	}
	if state.Quota.BackoffLevel != backoffLevel {
		t.Fatalf("expected BackoffLevel to be %d, got %d", backoffLevel, state.Quota.BackoffLevel)
	}
}

func TestManager_SetSelector_StopsPreviousStoppableSelector(t *testing.T) {
	previous := &stoppableSelectorStub{}
	manager := NewManager(nil, previous, nil)

	manager.SetSelector(&RoundRobinSelector{})

	if previous.stops != 1 {
		t.Fatalf("previous selector stop count = %d, want 1", previous.stops)
	}
}

func TestManager_SetSelector_NonComparableSelectorDoesNotPanic(t *testing.T) {
	stops := 0
	previous := nonComparableSelectorStub{stops: &stops, tags: []string{"previous"}}
	manager := NewManager(nil, previous, nil)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("SetSelector() panicked with non-comparable selector: %v", recovered)
		}
	}()

	manager.SetSelector(nonComparableSelectorStub{stops: &stops, tags: []string{"next"}})

	if stops != 1 {
		t.Fatalf("non-comparable previous selector stop count = %d, want 1", stops)
	}
}

func TestManager_SetSelector_SessionAffinityPreservesBindings(t *testing.T) {
	previous := NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback: &RoundRobinSelector{},
		TTL:      time.Hour,
	})
	manager := NewManager(nil, previous, nil)

	auths := []*Auth{
		{ID: "auth-a", Provider: "codex"},
		{ID: "auth-b", Provider: "codex"},
	}
	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"session-transfer-test"}}`),
	}

	first, errPick := previous.Pick(context.Background(), "codex", "gpt-5.4", opts, auths)
	if errPick != nil {
		t.Fatalf("previous.Pick() error = %v", errPick)
	}
	if first == nil {
		t.Fatal("previous.Pick() returned nil auth")
	}

	cacheKey := "codex::user:session-transfer-test::gpt-5.4"
	if entry, ok := previous.cache.GetEntry(cacheKey); !ok {
		t.Fatalf("previous selector cache missing %q after initial Pick()", cacheKey)
	} else if entry.authID != first.ID {
		t.Fatalf("previous selector cache auth = %q, want %q", entry.authID, first.ID)
	}

	next := NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback:                     &RoundRobinSelector{},
		TTL:                          time.Hour,
		CodexWebsocketStrictAffinity: true,
	})

	manager.SetSelector(next)

	entry, ok := next.cache.GetEntry(cacheKey)
	if !ok {
		t.Fatalf("next selector cache missing %q after SetSelector()", cacheKey)
	}
	if entry.authID != first.ID {
		t.Fatalf("next selector cache auth = %q, want %q", entry.authID, first.ID)
	}
}

func TestManager_SetSelector_SessionAffinityRebasesBindingsToNewTTL(t *testing.T) {
	previous := NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback: &RoundRobinSelector{},
		TTL:      time.Hour,
	})
	manager := NewManager(nil, previous, nil)

	auths := []*Auth{
		{ID: "auth-a", Provider: "codex"},
		{ID: "auth-b", Provider: "codex"},
	}
	opts := cliproxyexecutor.Options{
		OriginalRequest: []byte(`{"metadata":{"user_id":"session-transfer-ttl-test"}}`),
	}

	first, errPick := previous.Pick(context.Background(), "codex", "gpt-5.4", opts, auths)
	if errPick != nil {
		t.Fatalf("previous.Pick() error = %v", errPick)
	}
	if first == nil {
		t.Fatal("previous.Pick() returned nil auth")
	}

	next := NewSessionAffinitySelectorWithConfig(SessionAffinityConfig{
		Fallback: &RoundRobinSelector{},
		TTL:      2 * time.Second,
	})

	manager.SetSelector(next)

	cacheKey := "codex::user:session-transfer-ttl-test::gpt-5.4"
	next.cache.mu.RLock()
	entry, ok := next.cache.entries[cacheKey]
	next.cache.mu.RUnlock()
	if !ok {
		t.Fatalf("next selector cache missing %q after SetSelector()", cacheKey)
	}

	remaining := time.Until(entry.expiresAt)
	if remaining <= time.Second || remaining > 5*time.Second {
		t.Fatalf("transferred binding remaining TTL = %s, want rebased to about 2s", remaining)
	}
}
