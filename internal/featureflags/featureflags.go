// Package featureflags centralizes runtime feature flag defaults, lifecycle
// metadata, and resolution.
package featureflags

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

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

// Key is the canonical dotted configuration path for a runtime feature flag.
type Key string

const (
	// RoutingAutoModelResolutionKey controls dynamic resolution for model="auto".
	RoutingAutoModelResolutionKey Key = "feature-flags.routing.auto-model-resolution"
	// GeminiAttachDefaultSafetySettingsKey controls Gemini fallback safety payload injection.
	GeminiAttachDefaultSafetySettingsKey Key = "feature-flags.gemini.attach-default-safety-settings"
)

// Definition describes a feature flag's lifecycle metadata and default behavior.
type Definition struct {
	Key          Key
	Owner        string
	IntroducedOn string
	ReviewBy     string
	Rationale    string
	DefaultValue bool
	UsageTokens  []string
}

var definitions = []Definition{
	{
		Key:          RoutingAutoModelResolutionKey,
		Owner:        "routing-runtime",
		IntroducedOn: "2026-04-22",
		ReviewBy:     "2026-07-31",
		Rationale:    "Keeps auto model routing behind a short-lived escape hatch while the new resolver behavior bakes in production.",
		DefaultValue: true,
		UsageTokens: []string{
			"featureflags.RoutingAutoModelResolutionEnabled(",
		},
	},
	{
		Key:          GeminiAttachDefaultSafetySettingsKey,
		Owner:        "gemini-compat",
		IntroducedOn: "2026-04-22",
		ReviewBy:     "2026-07-31",
		Rationale:    "Allows rapid rollback of Gemini fallback safety payload injection while upstream compatibility is validated.",
		DefaultValue: true,
		UsageTokens: []string{
			"featureflags.GeminiAttachDefaultSafetySettingsEnabled(",
		},
	},
}

var current atomic.Pointer[Snapshot]

func init() {
	Apply(Defaults())
}

// Definitions returns the repository's feature flag definitions.
func Definitions() []Definition {
	return append([]Definition(nil), definitions...)
}

// ValidateDefinitions validates lifecycle metadata and review deadlines for all feature flags.
func ValidateDefinitions(now time.Time) error {
	seen := make(map[Key]struct{}, len(definitions))
	problems := make([]string, 0)

	for _, definition := range definitions {
		problems = append(problems, definition.lifecycleProblems(now)...)
		if definition.Key == "" {
			continue
		}
		if _, exists := seen[definition.Key]; exists {
			problems = append(problems, fmt.Sprintf("%q is defined more than once", definition.Key))
			continue
		}
		seen[definition.Key] = struct{}{}
	}

	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("feature flag lifecycle validation failed: %s", strings.Join(problems, "; "))
}

// Defaults returns the repository's default feature-flag state.
func Defaults() Snapshot {
	var snapshot Snapshot
	for _, definition := range definitions {
		definition.applyDefault(&snapshot)
	}
	return snapshot
}

// Resolve applies config overrides on top of default feature-flag values.
func Resolve(cfg *config.Config) Snapshot {
	resolved := Defaults()
	if cfg == nil || cfg.FeatureFlags == nil {
		return resolved
	}

	for _, definition := range definitions {
		definition.applyOverride(&resolved, cfg.FeatureFlags)
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

func (definition Definition) applyDefault(snapshot *Snapshot) {
	switch definition.Key {
	case RoutingAutoModelResolutionKey:
		snapshot.Routing.AutoModelResolution = definition.DefaultValue
	case GeminiAttachDefaultSafetySettingsKey:
		snapshot.Gemini.AttachDefaultSafetySettings = definition.DefaultValue
	}
}

func (definition Definition) applyOverride(snapshot *Snapshot, featureFlags *config.FeatureFlagsConfig) {
	if snapshot == nil || featureFlags == nil {
		return
	}

	switch definition.Key {
	case RoutingAutoModelResolutionKey:
		if value := featureFlags.Routing.AutoModelResolution; value != nil {
			snapshot.Routing.AutoModelResolution = *value
		}
	case GeminiAttachDefaultSafetySettingsKey:
		if value := featureFlags.Gemini.AttachDefaultSafetySettings; value != nil {
			snapshot.Gemini.AttachDefaultSafetySettings = *value
		}
	}
}

func (definition Definition) lifecycleProblems(now time.Time) []string {
	problems := make([]string, 0)

	if definition.Key == "" {
		problems = append(problems, "feature flag definition is missing key")
	}
	if strings.TrimSpace(definition.Owner) == "" {
		problems = append(problems, fmt.Sprintf("%q is missing owner", definition.Key))
	}
	if strings.TrimSpace(definition.Rationale) == "" {
		problems = append(problems, fmt.Sprintf("%q is missing rationale", definition.Key))
	}

	hasUsageToken := false
	for _, usageToken := range definition.UsageTokens {
		if strings.TrimSpace(usageToken) != "" {
			hasUsageToken = true
			break
		}
	}
	if !hasUsageToken {
		problems = append(problems, fmt.Sprintf("%q is missing usage tokens", definition.Key))
	}

	introducedOn, introducedErr := parseLifecycleDate(definition.IntroducedOn)
	if introducedErr != nil {
		problems = append(problems, fmt.Sprintf("%q has invalid introduced date %q: %v", definition.Key, definition.IntroducedOn, introducedErr))
	}

	reviewBy, reviewErr := parseLifecycleDate(definition.ReviewBy)
	if reviewErr != nil {
		problems = append(problems, fmt.Sprintf("%q has invalid review date %q: %v", definition.Key, definition.ReviewBy, reviewErr))
	}

	if introducedErr == nil && reviewErr == nil {
		if reviewBy.Before(introducedOn) {
			problems = append(problems, fmt.Sprintf("%q review date %s is before introduced date %s", definition.Key, definition.ReviewBy, definition.IntroducedOn))
		}
		if reviewBy.Before(dateOnlyUTC(now)) {
			problems = append(problems, fmt.Sprintf("%q review date %s is overdue", definition.Key, definition.ReviewBy))
		}
	}

	return problems
}

func parseLifecycleDate(value string) (time.Time, error) {
	return time.Parse(time.DateOnly, strings.TrimSpace(value))
}

func dateOnlyUTC(value time.Time) time.Time {
	utcValue := value.UTC()
	return time.Date(utcValue.Year(), utcValue.Month(), utcValue.Day(), 0, 0, 0, 0, time.UTC)
}
