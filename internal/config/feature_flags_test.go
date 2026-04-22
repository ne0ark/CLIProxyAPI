package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_FeatureFlags(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
port: 8317
feature-flags:
  routing:
    auto-model-resolution: false
  gemini:
    attach-default-safety-settings: false
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.FeatureFlags == nil {
		t.Fatal("expected feature-flags section to be loaded")
	}
	if cfg.FeatureFlags.Routing.AutoModelResolution == nil || *cfg.FeatureFlags.Routing.AutoModelResolution {
		t.Fatalf("expected routing.auto-model-resolution=false, got %+v", cfg.FeatureFlags.Routing.AutoModelResolution)
	}
	if cfg.FeatureFlags.Gemini.AttachDefaultSafetySettings == nil || *cfg.FeatureFlags.Gemini.AttachDefaultSafetySettings {
		t.Fatalf("expected gemini.attach-default-safety-settings=false, got %+v", cfg.FeatureFlags.Gemini.AttachDefaultSafetySettings)
	}
}
