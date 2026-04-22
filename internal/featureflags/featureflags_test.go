package featureflags

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestResolveDefaults(t *testing.T) {
	resolved := Resolve(nil)
	if !resolved.Routing.AutoModelResolution {
		t.Fatal("expected routing.auto-model-resolution to default to true")
	}
	if !resolved.Gemini.AttachDefaultSafetySettings {
		t.Fatal("expected gemini.attach-default-safety-settings to default to true")
	}
}

func TestApplyConfigUsesOverrides(t *testing.T) {
	previous := Current()
	t.Cleanup(func() {
		Apply(previous)
	})

	autoModelResolution := false
	attachDefaultSafetySettings := false
	cfg := &config.Config{
		FeatureFlags: &config.FeatureFlagsConfig{
			Routing: config.FeatureFlagRoutingConfig{
				AutoModelResolution: &autoModelResolution,
			},
			Gemini: config.FeatureFlagGeminiConfig{
				AttachDefaultSafetySettings: &attachDefaultSafetySettings,
			},
		},
	}

	resolved := ApplyConfig(cfg)
	if resolved.Routing.AutoModelResolution {
		t.Fatal("expected routing.auto-model-resolution override to disable the feature")
	}
	if resolved.Gemini.AttachDefaultSafetySettings {
		t.Fatal("expected gemini.attach-default-safety-settings override to disable the feature")
	}

	if current := Current(); current != resolved {
		t.Fatalf("expected current snapshot %+v, got %+v", resolved, current)
	}
}
