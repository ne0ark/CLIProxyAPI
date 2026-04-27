package auth

import (
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

func TestFindAllAntigravityCreditsCandidateAuths_PrefersKnownCreditsThenUnknown(t *testing.T) {
	m := &Manager{
		auths: map[string]*Auth{
			"zz-credits": {ID: "zz-credits", Provider: "antigravity"},
			"aa-unknown": {ID: "aa-unknown", Provider: "antigravity"},
			"mm-no":      {ID: "mm-no", Provider: "antigravity"},
		},
		executors: map[string]ProviderExecutor{
			"antigravity": schedulerTestExecutor{},
		},
	}

	SetAntigravityCreditsHint("zz-credits", AntigravityCreditsHint{
		Known:     true,
		Available: true,
		UpdatedAt: time.Now(),
	})
	SetAntigravityCreditsHint("mm-no", AntigravityCreditsHint{
		Known:     true,
		Available: false,
		UpdatedAt: time.Now(),
	})

	opts := cliproxyexecutor.Options{}

	for _, authID := range []string{"zz-credits", "aa-unknown", "mm-no"} {
		registry.GetGlobalRegistry().RegisterClient(authID, "antigravity", []*registry.ModelInfo{{ID: "claude-sonnet-4-6"}})
	}
	t.Cleanup(func() {
		for _, authID := range []string{"zz-credits", "aa-unknown", "mm-no"} {
			registry.GetGlobalRegistry().UnregisterClient(authID)
		}
	})

	candidates := m.findAllAntigravityCreditsCandidateAuths("claude-sonnet-4-6", opts)
	if len(candidates) != 2 {
		t.Fatalf("candidates len = %d, want 2", len(candidates))
	}
	if candidates[0].auth.ID != "zz-credits" {
		t.Fatalf("candidates[0].auth.ID = %q, want %q", candidates[0].auth.ID, "zz-credits")
	}
	if candidates[1].auth.ID != "aa-unknown" {
		t.Fatalf("candidates[1].auth.ID = %q, want %q", candidates[1].auth.ID, "aa-unknown")
	}

	nonClaude := m.findAllAntigravityCreditsCandidateAuths("gemini-3-flash", opts)
	if len(nonClaude) != 0 {
		t.Fatalf("nonClaude len = %d, want 0", len(nonClaude))
	}

	pinnedOpts := cliproxyexecutor.Options{
		Metadata: map[string]any{cliproxyexecutor.PinnedAuthMetadataKey: "aa-unknown"},
	}
	pinned := m.findAllAntigravityCreditsCandidateAuths("claude-sonnet-4-6", pinnedOpts)
	if len(pinned) != 1 {
		t.Fatalf("pinned len = %d, want 1", len(pinned))
	}
	if pinned[0].auth.ID != "aa-unknown" {
		t.Fatalf("pinned[0].auth.ID = %q, want %q", pinned[0].auth.ID, "aa-unknown")
	}
}

func TestFindAllAntigravityCreditsCandidateAuths_UsesResolvedOAuthAlias(t *testing.T) {
	m := NewManager(nil, nil, nil)
	m.SetConfig(&internalconfig.Config{})
	m.SetOAuthModelAlias(map[string][]internalconfig.OAuthModelAlias{
		"antigravity": {{Name: "claude-sonnet-4-6", Alias: "sonnet"}},
	})
	m.auths = map[string]*Auth{
		"ag-alias": {
			ID:       "ag-alias",
			Provider: "antigravity",
			Metadata: map[string]any{"email": "alias@example.com"},
		},
	}
	m.executors = map[string]ProviderExecutor{
		"antigravity": schedulerTestExecutor{},
	}
	registry.GetGlobalRegistry().RegisterClient("ag-alias", "antigravity", []*registry.ModelInfo{{ID: "claude-sonnet-4-6"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("ag-alias") })

	candidates := m.findAllAntigravityCreditsCandidateAuths("sonnet", cliproxyexecutor.Options{})
	if len(candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(candidates))
	}
	if candidates[0].auth.ID != "ag-alias" {
		t.Fatalf("candidates[0].auth.ID = %q, want %q", candidates[0].auth.ID, "ag-alias")
	}
}

func TestFindAllAntigravityCreditsCandidateAuths_SkipsUnsupportedAuths(t *testing.T) {
	m := &Manager{
		auths: map[string]*Auth{
			"ag-blocked": {
				ID:       "ag-blocked",
				Provider: "antigravity",
			},
		},
		executors: map[string]ProviderExecutor{
			"antigravity": schedulerTestExecutor{},
		},
	}

	SetAntigravityCreditsHint("ag-blocked", AntigravityCreditsHint{
		Known:     true,
		Available: true,
		UpdatedAt: time.Now(),
	})
	registry.GetGlobalRegistry().RegisterClient("ag-blocked", "antigravity", []*registry.ModelInfo{{ID: "gemini-3-flash"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("ag-blocked") })

	candidates := m.findAllAntigravityCreditsCandidateAuths("claude-sonnet-4-6", cliproxyexecutor.Options{})
	if len(candidates) != 0 {
		t.Fatalf("candidates len = %d, want 0", len(candidates))
	}
}

func TestFindAllAntigravityCreditsCandidateAuths_RetriesStaleUnavailableHint(t *testing.T) {
	m := &Manager{
		auths: map[string]*Auth{
			"ag-stale": {ID: "ag-stale", Provider: "antigravity"},
		},
		executors: map[string]ProviderExecutor{
			"antigravity": schedulerTestExecutor{},
		},
	}

	SetAntigravityCreditsHint("ag-stale", AntigravityCreditsHint{
		Known:     true,
		Available: false,
		UpdatedAt: time.Now().Add(-2 * antigravityCreditsHintMaxAge),
	})
	registry.GetGlobalRegistry().RegisterClient("ag-stale", "antigravity", []*registry.ModelInfo{{ID: "claude-sonnet-4-6"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("ag-stale") })

	candidates := m.findAllAntigravityCreditsCandidateAuths("claude-sonnet-4-6", cliproxyexecutor.Options{})
	if len(candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(candidates))
	}
	if candidates[0].auth.ID != "ag-stale" {
		t.Fatalf("candidates[0].auth.ID = %q, want %q", candidates[0].auth.ID, "ag-stale")
	}
}

func TestFindAllAntigravityCreditsCandidateAuths_SkipsDisabledAndNonQuotaBlockedModels(t *testing.T) {
	now := time.Now()
	const model = "claude-sonnet-4-6"

	m := &Manager{
		auths: map[string]*Auth{
			"ag-ready": {ID: "ag-ready", Provider: "antigravity"},
			"ag-disabled": {
				ID:       "ag-disabled",
				Provider: "antigravity",
				ModelStates: map[string]*ModelState{
					model: {Status: StatusDisabled},
				},
			},
			"ag-payment": {
				ID:       "ag-payment",
				Provider: "antigravity",
				ModelStates: map[string]*ModelState{
					model: {
						Unavailable:    true,
						NextRetryAfter: now.Add(10 * time.Minute),
						Quota: QuotaState{
							Exceeded: false,
						},
					},
				},
			},
		},
		executors: map[string]ProviderExecutor{
			"antigravity": schedulerTestExecutor{},
		},
	}

	for _, authID := range []string{"ag-ready", "ag-disabled", "ag-payment"} {
		SetAntigravityCreditsHint(authID, AntigravityCreditsHint{
			Known:     true,
			Available: true,
			UpdatedAt: now,
		})
		registry.GetGlobalRegistry().RegisterClient(authID, "antigravity", []*registry.ModelInfo{{ID: model}})
	}
	t.Cleanup(func() {
		for _, authID := range []string{"ag-ready", "ag-disabled", "ag-payment"} {
			registry.GetGlobalRegistry().UnregisterClient(authID)
		}
	})

	candidates := m.findAllAntigravityCreditsCandidateAuths(model, cliproxyexecutor.Options{})
	if len(candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(candidates))
	}
	if candidates[0].auth.ID != "ag-ready" {
		t.Fatalf("candidates[0].auth.ID = %q, want %q", candidates[0].auth.ID, "ag-ready")
	}
}

func TestFindAllAntigravityCreditsCandidateAuths_AllowsQuotaCooldownReplayCandidate(t *testing.T) {
	now := time.Now()
	const model = "claude-sonnet-4-6"

	m := &Manager{
		auths: map[string]*Auth{
			"ag-cooldown": {
				ID:       "ag-cooldown",
				Provider: "antigravity",
				ModelStates: map[string]*ModelState{
					model: {
						Unavailable:    true,
						NextRetryAfter: now.Add(10 * time.Minute),
						Quota: QuotaState{
							Exceeded:      true,
							Reason:        "quota",
							NextRecoverAt: now.Add(10 * time.Minute),
						},
					},
				},
			},
		},
		executors: map[string]ProviderExecutor{
			"antigravity": schedulerTestExecutor{},
		},
	}

	SetAntigravityCreditsHint("ag-cooldown", AntigravityCreditsHint{
		Known:     true,
		Available: true,
		UpdatedAt: now,
	})
	registry.GetGlobalRegistry().RegisterClient("ag-cooldown", "antigravity", []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("ag-cooldown") })

	candidates := m.findAllAntigravityCreditsCandidateAuths(model, cliproxyexecutor.Options{})
	if len(candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(candidates))
	}
	if candidates[0].auth.ID != "ag-cooldown" {
		t.Fatalf("candidates[0].auth.ID = %q, want %q", candidates[0].auth.ID, "ag-cooldown")
	}
}
