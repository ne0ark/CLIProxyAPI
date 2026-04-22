package util

import (
	"testing"

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
