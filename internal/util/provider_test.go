package util

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/featureflags"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

func TestResolveAutoModelHonorsFeatureFlag(t *testing.T) {
	previous := featureflags.Current()
	registryRef := registry.GetGlobalRegistry()
	const clientID = "feature-flag-auto-model-test"

	t.Cleanup(func() {
		featureflags.Apply(previous)
		registryRef.UnregisterClient(clientID)
	})

	registryRef.UnregisterClient(clientID)
	registryRef.RegisterClient(clientID, "gemini", []*registry.ModelInfo{
		{
			ID:      "gemini-2.5-pro",
			Object:  "model",
			Created: 9_999_999_999,
		},
	})

	featureflags.Apply(featureflags.Defaults())
	if got := ResolveAutoModel("auto"); got != "gemini-2.5-pro" {
		t.Fatalf("expected auto model to resolve when flag enabled, got %q", got)
	}

	disabled := featureflags.Defaults()
	disabled.Routing.AutoModelResolution = false
	featureflags.Apply(disabled)
	if got := ResolveAutoModel("auto"); got != "auto" {
		t.Fatalf("expected auto model to remain unresolved when flag disabled, got %q", got)
	}
}

func TestIsOpenAICompatibilityAlias_SkipsDisabledProviders(t *testing.T) {
	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:     "enabled",
				Disabled: false,
				Models: []config.OpenAICompatibilityModel{
					{Alias: "enabled-alias"},
				},
			},
			{
				Name:     "disabled",
				Disabled: true,
				Models: []config.OpenAICompatibilityModel{
					{Alias: "disabled-alias"},
				},
			},
		},
	}

	if !IsOpenAICompatibilityAlias("enabled-alias", cfg) {
		t.Fatal("expected enabled alias to resolve")
	}
	if IsOpenAICompatibilityAlias("disabled-alias", cfg) {
		t.Fatal("expected disabled alias to be skipped")
	}
}

func TestGetOpenAICompatibilityConfig_SkipsDisabledProviders(t *testing.T) {
	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:     "enabled",
				Disabled: false,
				Models: []config.OpenAICompatibilityModel{
					{Name: "upstream-enabled", Alias: "enabled-alias"},
				},
			},
			{
				Name:     "disabled",
				Disabled: true,
				Models: []config.OpenAICompatibilityModel{
					{Name: "upstream-disabled", Alias: "disabled-alias"},
				},
			},
		},
	}

	compat, model := GetOpenAICompatibilityConfig("enabled-alias", cfg)
	if compat == nil || model == nil {
		t.Fatal("expected enabled alias to resolve")
	}
	if compat.Name != "enabled" {
		t.Fatalf("compat name = %q, want %q", compat.Name, "enabled")
	}
	if model.Name != "upstream-enabled" {
		t.Fatalf("model name = %q, want %q", model.Name, "upstream-enabled")
	}

	compat, model = GetOpenAICompatibilityConfig("disabled-alias", cfg)
	if compat != nil || model != nil {
		t.Fatalf("expected disabled alias to be skipped, got compat=%v model=%v", compat, model)
	}
}
