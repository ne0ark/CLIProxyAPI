package config

import (
	"sync"
	"testing"
)

func TestPolicyCache_RebuildsAfterMutationViaSetter(t *testing.T) {
	t.Parallel()

	cfg := &SDKConfig{}
	cfg.SetAPIKeyPolicies([]APIKeyPolicy{
		{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
	})

	if !cfg.IsModelAllowedForKey("sk-a", "gpt-4o-mini") {
		t.Fatalf("expected sk-a to allow gpt-4o-mini initially")
	}

	cfg.SetAPIKeyPolicies([]APIKeyPolicy{
		{Key: "sk-b", AllowedModels: []string{"claude-3-*"}},
	})

	if !cfg.IsModelAllowedForKey("sk-a", "gpt-4o-mini") {
		t.Fatalf("after swap, sk-a should fall back to default allow-all")
	}
	if !cfg.IsModelAllowedForKey("sk-b", "claude-3-5-sonnet-20241022") {
		t.Fatalf("after swap, sk-b must allow claude-3-5-sonnet-20241022")
	}
	if cfg.IsModelAllowedForKey("sk-b", "gpt-4o-mini") {
		t.Fatalf("after swap, sk-b must reject gpt-4o-mini per new glob")
	}
}

func TestPolicyCache_InvalidateAfterInPlaceEdit(t *testing.T) {
	t.Parallel()

	cfg := &SDKConfig{
		APIKeyPolicies: []APIKeyPolicy{
			{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
		},
	}
	if !cfg.IsModelAllowedForKey("sk-a", "gpt-4o") {
		t.Fatalf("initial allow failed")
	}

	cfg.APIKeyPolicies[0].AllowedModels = []string{"claude-3-*"}
	cfg.InvalidatePolicyIndex()

	if cfg.IsModelAllowedForKey("sk-a", "gpt-4o") {
		t.Fatalf("after tightening, sk-a must no longer allow gpt-4o")
	}
	if !cfg.IsModelAllowedForKey("sk-a", "claude-3-5-sonnet-20241022") {
		t.Fatalf("after tightening, sk-a must allow claude-3-*")
	}
}

func TestPolicyCache_NilSafe(t *testing.T) {
	t.Parallel()

	var cfg *SDKConfig
	cfg.InvalidatePolicyIndex()
	cfg.SetAPIKeyPolicies([]APIKeyPolicy{{Key: "sk-x"}})
	if !cfg.IsModelAllowedForKey("sk-x", "anything") {
		t.Fatalf("nil SDKConfig must allow (legacy no-config behavior)")
	}
}

func TestPolicyCache_ConcurrentReadsAreSafe(t *testing.T) {
	t.Parallel()

	cfg := &SDKConfig{}
	cfg.SetAPIKeyPolicies([]APIKeyPolicy{
		{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
		{Key: "sk-b", AllowedModels: []string{"claude-3-*"}},
		{Key: "sk-c", AllowedModels: []string{"gemini-*"}},
	})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				_ = cfg.IsModelAllowedForKey("sk-a", "gpt-4o")
				_ = cfg.IsModelAllowedForKey("sk-b", "claude-3-haiku")
				_ = cfg.IsModelAllowedForKey("sk-c", "gemini-2.0-flash")
				_ = cfg.IsModelAllowedForKey("sk-unknown", "gpt-4o")
			}
		}()
	}
	wg.Wait()
}

func TestPolicyCache_SetAPIKeyPoliciesCopiesSlices(t *testing.T) {
	t.Parallel()

	cfg := &SDKConfig{}
	policies := []APIKeyPolicy{
		{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
	}
	cfg.SetAPIKeyPolicies(policies)

	policies[0].Key = "sk-hacked"
	policies[0].AllowedModels[0] = "claude-3-*"

	if !cfg.IsModelAllowedForKey("sk-a", "gpt-4o") {
		t.Fatalf("caller mutation must not leak into cfg; sk-a should still match")
	}
	if cfg.IsModelAllowedForKey("sk-a", "claude-3-5-sonnet-20241022") {
		t.Fatalf("caller mutation of AllowedModels must not leak into cfg")
	}
}

func TestPolicyCache_SetEmptyClears(t *testing.T) {
	t.Parallel()

	cfg := &SDKConfig{
		APIKeyDefaultPolicy: APIKeyDefaultPolicyDenyAll,
	}
	cfg.SetAPIKeyPolicies([]APIKeyPolicy{
		{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
	})
	if !cfg.IsModelAllowedForKey("sk-a", "gpt-4o") {
		t.Fatalf("initial allow failed")
	}

	cfg.SetAPIKeyPolicies(nil)

	if cfg.APIKeyPolicies != nil {
		t.Fatalf("SetAPIKeyPolicies(nil) must nil the slice, got %#v", cfg.APIKeyPolicies)
	}
	if cfg.IsModelAllowedForKey("sk-a", "gpt-4o") {
		t.Fatalf("after clearing, sk-a must be denied under deny-all default")
	}
}

func TestPolicyCache_ConcurrentMutationAndRead(t *testing.T) {
	t.Parallel()

	cfg := &SDKConfig{}
	cfg.SetAPIKeyPolicies([]APIKeyPolicy{
		{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
	})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if i%2 == 0 {
				cfg.SetAPIKeyPolicies([]APIKeyPolicy{{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}}})
			} else {
				cfg.SetAPIKeyPolicies([]APIKeyPolicy{{Key: "sk-a", AllowedModels: []string{"claude-3-*"}}})
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = cfg.IsModelAllowedForKey("sk-a", "gpt-4o-mini")
			_ = cfg.IsModelAllowedForKey("sk-a", "claude-3-5-sonnet-20241022")
		}
	}()

	wg.Wait()
	close(stop)
}
