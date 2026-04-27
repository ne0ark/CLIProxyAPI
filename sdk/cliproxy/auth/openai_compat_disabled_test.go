package auth

import (
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestResolveOpenAICompatConfig_SkipsDisabledProviders(t *testing.T) {
	cfg := &internalconfig.Config{
		OpenAICompatibility: []internalconfig.OpenAICompatibility{
			{
				Name:     "disabled-provider",
				Disabled: true,
			},
			{
				Name:     "enabled-provider",
				Disabled: false,
			},
		},
	}

	if got := resolveOpenAICompatConfig(cfg, "disabled-provider", "", ""); got != nil {
		t.Fatalf("expected disabled provider to be skipped, got %+v", got)
	}

	got := resolveOpenAICompatConfig(cfg, "enabled-provider", "", "")
	if got == nil {
		t.Fatal("expected enabled provider to resolve")
	}
	if got.Name != "enabled-provider" {
		t.Fatalf("resolved name = %q, want %q", got.Name, "enabled-provider")
	}
}
