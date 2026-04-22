// Package featureflags centralizes runtime feature flag defaults and resolution.
package featureflags

import (
	"sync/atomic"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// Snapshot is the fully resolved runtime feature-flag state.
type Snapshot struct {
	Routing RoutingFlags
	Gemini  GeminiFlags
}

// RoutingFlags groups routing-related feature flags.
type RoutingFlags struct {
	AutoModelResolution bool
}

// GeminiFlags groups Gemini-related feature flags.
type GeminiFlags struct {
	AttachDefaultSafetySettings bool
}

var current atomic.Pointer[Snapshot]

func init() {
	Apply(Defaults())
}

// Defaults returns the repository's default feature-flag state.
func Defaults() Snapshot {
	return Snapshot{
		Routing: RoutingFlags{
			AutoModelResolution: true,
		},
		Gemini: GeminiFlags{
			AttachDefaultSafetySettings: true,
		},
	}
}

// Resolve applies config overrides on top of default feature-flag values.
func Resolve(cfg *config.Config) Snapshot {
	resolved := Defaults()
	if cfg == nil || cfg.FeatureFlags == nil {
		return resolved
	}

	if value := cfg.FeatureFlags.Routing.AutoModelResolution; value != nil {
		resolved.Routing.AutoModelResolution = *value
	}
	if value := cfg.FeatureFlags.Gemini.AttachDefaultSafetySettings; value != nil {
		resolved.Gemini.AttachDefaultSafetySettings = *value
	}

	return resolved
}

// Apply stores a fully resolved runtime snapshot.
func Apply(snapshot Snapshot) Snapshot {
	next := snapshot
	current.Store(&next)
	return next
}

// ApplyConfig resolves and stores feature flags from the provided config.
func ApplyConfig(cfg *config.Config) Snapshot {
	return Apply(Resolve(cfg))
}

// Current returns the currently active runtime snapshot.
func Current() Snapshot {
	if snapshot := current.Load(); snapshot != nil {
		return *snapshot
	}
	return Apply(Defaults())
}

// RoutingAutoModelResolutionEnabled reports whether "auto" model names should resolve dynamically.
func RoutingAutoModelResolutionEnabled() bool {
	return Current().Routing.AutoModelResolution
}

// GeminiAttachDefaultSafetySettingsEnabled reports whether Gemini translators should add fallback safety settings.
func GeminiAttachDefaultSafetySettingsEnabled() bool {
	return Current().Gemini.AttachDefaultSafetySettings
}
