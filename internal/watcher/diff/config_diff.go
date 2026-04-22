package diff

import (
	"net/url"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// BuildConfigChangeDetails computes a redacted, human-readable list of config changes.
// Secrets are never printed; only structural or non-sensitive fields are surfaced.
func BuildConfigChangeDetails(oldCfg, newCfg *config.Config) []string {
	changes := make([]string, 0, 16)
	if oldCfg == nil || newCfg == nil {
		return changes
	}

	changes = appendConfigScalarChanges(changes, oldCfg, newCfg)
	changes = appendQuotaExceededChanges(changes, oldCfg, newCfg)
	changes = appendRoutingChange(changes, oldCfg, newCfg)
	changes = appendFeatureFlagChanges(changes, oldCfg, newCfg)
	changes = appendAPIKeysChanges(changes, oldCfg, newCfg)
	changes = appendGeminiKeyChanges(changes, oldCfg, newCfg)
	changes = appendClaudeKeyChanges(changes, oldCfg, newCfg)
	changes = appendCodexKeyChanges(changes, oldCfg, newCfg)
	changes = appendAmpCodeChanges(changes, oldCfg, newCfg)
	changes = appendOAuthChanges(changes, oldCfg, newCfg)
	changes = appendRemoteManagementChanges(changes, oldCfg, newCfg)
	changes = appendOpenAICompatibilityChanges(changes, oldCfg, newCfg)
	changes = appendVertexCompatKeyChanges(changes, oldCfg, newCfg)

	return changes
}

func trimStrings(in []string) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = strings.TrimSpace(in[i])
	}
	return out
}

func equalStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func formatProxyURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "<none>"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "<redacted>"
	}
	host := strings.TrimSpace(parsed.Host)
	scheme := strings.TrimSpace(parsed.Scheme)
	if host == "" {
		// Allow host:port style without scheme.
		parsed2, err2 := url.Parse("http://" + trimmed)
		if err2 == nil {
			host = strings.TrimSpace(parsed2.Host)
		}
		scheme = ""
	}
	if host == "" {
		return "<redacted>"
	}
	if scheme == "" {
		return host
	}
	return scheme + "://" + host
}

func equalStringSet(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	aSet := make(map[string]struct{}, len(a))
	for _, k := range a {
		aSet[strings.TrimSpace(k)] = struct{}{}
	}
	bSet := make(map[string]struct{}, len(b))
	for _, k := range b {
		bSet[strings.TrimSpace(k)] = struct{}{}
	}
	if len(aSet) != len(bSet) {
		return false
	}
	for k := range aSet {
		if _, ok := bSet[k]; !ok {
			return false
		}
	}
	return true
}

// equalUpstreamAPIKeys compares two slices of AmpUpstreamAPIKeyEntry for equality.
// Comparison is done by count and content (upstream key and client keys).
func equalUpstreamAPIKeys(a, b []config.AmpUpstreamAPIKeyEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].UpstreamAPIKey) != strings.TrimSpace(b[i].UpstreamAPIKey) {
			return false
		}
		if !equalStringSet(a[i].APIKeys, b[i].APIKeys) {
			return false
		}
	}
	return true
}
