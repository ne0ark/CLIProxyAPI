// Package config provides configuration management for the CLI Proxy API server.
// It handles loading and parsing YAML configuration files, and provides structured
// access to application settings including server port, authentication directory,
// debug settings, proxy configuration, and API keys.
package config

import (
	"path"
	"strings"
	"sync/atomic"
)

// APIKeyDefaultPolicyAllowAll permits any model when a key has no explicit
// AllowedModels list.
const APIKeyDefaultPolicyAllowAll = "allow-all"

// APIKeyDefaultPolicyDenyAll forbids every model when a key has no explicit
// AllowedModels list.
const APIKeyDefaultPolicyDenyAll = "deny-all"

// APIKeyPolicy describes per-client-key access controls. AllowedModels supports
// path.Match-style globs (for example "claude-3-*" or "gpt-4o*"). An empty
// AllowedModels list defers to APIKeyDefaultPolicy on the parent SDKConfig.
type APIKeyPolicy struct {
	// Key is the bearer/API key value this policy applies to.
	Key string `yaml:"key" json:"key"`

	// AllowedModels lists glob patterns of model identifiers this key may target.
	AllowedModels []string `yaml:"allowed-models,omitempty" json:"allowed-models,omitempty"`
}

// SDKConfig represents the application's configuration, loaded from a YAML file.
type SDKConfig struct {
	// ProxyURL is the URL of an optional proxy server to use for outbound requests.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// EnableGeminiCLIEndpoint controls whether Gemini CLI internal endpoints (/v1internal:*) are enabled.
	// Default is false for safety; when false, /v1internal:* requests are rejected.
	EnableGeminiCLIEndpoint bool `yaml:"enable-gemini-cli-endpoint" json:"enable-gemini-cli-endpoint"`

	// ForceModelPrefix requires explicit model prefixes (e.g., "teamA/gemini-3-pro-preview")
	// to target prefixed credentials. When false, unprefixed model requests may use prefixed
	// credentials as well.
	ForceModelPrefix bool `yaml:"force-model-prefix" json:"force-model-prefix"`

	// RequestLog enables or disables detailed request logging functionality.
	RequestLog bool `yaml:"request-log" json:"request-log"`

	// APIKeys is a list of keys for authenticating clients to this proxy server.
	APIKeys []string `yaml:"api-keys" json:"api-keys"`

	// APIKeyPolicies stores per-key access policies (allowed model globs).
	// Keys present here must also exist in APIKeys to be accepted by the auth
	// provider; entries without a matching APIKeys row are ignored.
	APIKeyPolicies []APIKeyPolicy `yaml:"api-key-policies,omitempty" json:"api-key-policies,omitempty"`

	// policyIndex is a lazily-built lookup from API key value to its policy.
	// The cache is populated from immutable snapshots so the hot read path does
	// not need locking during ACL checks.
	policyIndex atomic.Pointer[map[string]APIKeyPolicy]

	// APIKeyDefaultPolicy controls behavior for keys with no entry in
	// APIKeyPolicies, or whose entry has an empty AllowedModels list. Valid
	// values are "allow-all" (default, backward compatible) and "deny-all".
	APIKeyDefaultPolicy string `yaml:"api-key-default-policy,omitempty" json:"api-key-default-policy,omitempty"`

	// PassthroughHeaders controls whether upstream response headers are forwarded to downstream clients.
	// Default is false (disabled).
	PassthroughHeaders bool `yaml:"passthrough-headers" json:"passthrough-headers"`

	// Streaming configures server-side streaming behavior (keep-alives and safe bootstrap retries).
	Streaming StreamingConfig `yaml:"streaming" json:"streaming"`

	// NonStreamKeepAliveInterval controls how often blank lines are emitted for non-streaming responses.
	// <= 0 disables keep-alives. Value is in seconds.
	NonStreamKeepAliveInterval int `yaml:"nonstream-keepalive-interval,omitempty" json:"nonstream-keepalive-interval,omitempty"`
}

// IsModelAllowedForKey reports whether the given client API key may target the
// given model. Matching uses path.Match glob semantics. An unknown key, or a
// key whose AllowedModels list is empty, falls back to APIKeyDefaultPolicy:
// "deny-all" rejects, anything else (including the default empty value) allows.
//
// The model argument is matched after stripping a single leading "<prefix>/"
// segment so policies stay portable across prefixed credentials. Matching is
// also attempted against the original model string so policies authored against
// a prefixed form continue to work.
func (c *SDKConfig) IsModelAllowedForKey(key, model string) bool {
	if c == nil {
		return true
	}

	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return c.defaultAllows()
	}

	candidate := model
	if idx := strings.Index(candidate, "/"); idx >= 0 && idx < len(candidate)-1 {
		candidate = candidate[idx+1:]
	}

	policy, ok := c.lookupPolicy(trimmedKey)
	if !ok {
		return c.defaultAllows()
	}
	if len(policy.AllowedModels) == 0 {
		return c.defaultAllows()
	}

	for _, pattern := range policy.AllowedModels {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == candidate || pattern == model {
			return true
		}
		if matched, err := path.Match(pattern, candidate); err == nil && matched {
			return true
		}
		if matched, err := path.Match(pattern, model); err == nil && matched {
			return true
		}
	}

	return false
}

// IsAPIKeyModelRestricted reports whether the given key is effectively subject
// to model restrictions under the current policy set. Keys become restricted
// either by an explicit non-empty allowlist, or by an empty/missing allowlist
// when the default policy is deny-all.
func (c *SDKConfig) IsAPIKeyModelRestricted(key string) bool {
	if c == nil {
		return false
	}

	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return false
	}

	policy, ok := c.lookupPolicy(trimmedKey)
	if !ok {
		return !c.defaultAllows()
	}

	if len(policy.AllowedModels) > 0 {
		return true
	}

	return !c.defaultAllows()
}

// ModelACLConfigured reports whether model ACL enforcement can affect request
// handling under the current settings.
func (c *SDKConfig) ModelACLConfigured() bool {
	if c == nil {
		return false
	}
	return c.hasAPIKeyPolicies() || !c.defaultAllows()
}

// InvalidatePolicyIndex clears the cached lookup map. Call this after direct
// in-place mutation of APIKeyPolicies so subsequent reads rebuild the index.
func (c *SDKConfig) InvalidatePolicyIndex() {
	if c == nil {
		return
	}
	c.policyIndex.Store(nil)
}

// SetAPIKeyPolicies atomically replaces the policy slice with a defensive
// deep copy and publishes a fresh lookup snapshot for concurrent readers.
func (c *SDKConfig) SetAPIKeyPolicies(policies []APIKeyPolicy) {
	if c == nil {
		return
	}

	cloned := cloneAPIKeyPolicies(policies)
	if len(cloned) == 0 {
		c.APIKeyPolicies = nil
		c.policyIndex.Store(nil)
		return
	}

	c.APIKeyPolicies = cloned
	index := buildAPIKeyPolicyIndex(cloned)
	c.policyIndex.Store(&index)
}

// SanitizeAPIKeyPolicies normalizes the configured default policy and replaces
// APIKeyPolicies with a sanitized deep copy.
func (c *SDKConfig) SanitizeAPIKeyPolicies() {
	if c == nil {
		return
	}
	c.APIKeyDefaultPolicy = normalizeAPIKeyDefaultPolicy(c.APIKeyDefaultPolicy)
	c.SetAPIKeyPolicies(c.APIKeyPolicies)
}

func (c *SDKConfig) lookupPolicy(key string) (APIKeyPolicy, bool) {
	if c == nil {
		return APIKeyPolicy{}, false
	}

	mp := c.policyIndex.Load()
	if mp == nil {
		cloned := cloneAPIKeyPolicies(c.APIKeyPolicies)
		if len(cloned) == 0 {
			return APIKeyPolicy{}, false
		}
		index := buildAPIKeyPolicyIndex(cloned)
		if c.policyIndex.CompareAndSwap(nil, &index) {
			mp = &index
		} else {
			mp = c.policyIndex.Load()
		}
	}
	if mp == nil {
		return APIKeyPolicy{}, false
	}

	policy, ok := (*mp)[key]
	return policy, ok
}

func (c *SDKConfig) defaultAllows() bool {
	if c == nil {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(c.APIKeyDefaultPolicy), APIKeyDefaultPolicyDenyAll)
}

func (c *SDKConfig) hasAPIKeyPolicies() bool {
	if c == nil {
		return false
	}

	mp := c.policyIndex.Load()
	if mp != nil {
		return len(*mp) > 0
	}

	return len(cloneAPIKeyPolicies(c.APIKeyPolicies)) > 0
}

func buildAPIKeyPolicyIndex(policies []APIKeyPolicy) map[string]APIKeyPolicy {
	index := make(map[string]APIKeyPolicy, len(policies))
	for _, policy := range policies {
		index[policy.Key] = policy
	}
	return index
}

func cloneAPIKeyPolicies(policies []APIKeyPolicy) []APIKeyPolicy {
	if len(policies) == 0 {
		return nil
	}

	cloned := make([]APIKeyPolicy, 0, len(policies))
	for _, policy := range policies {
		key := strings.TrimSpace(policy.Key)
		if key == "" {
			continue
		}

		entry := APIKeyPolicy{Key: key}
		if len(policy.AllowedModels) > 0 {
			seen := make(map[string]struct{}, len(policy.AllowedModels))
			entry.AllowedModels = make([]string, 0, len(policy.AllowedModels))
			for _, raw := range policy.AllowedModels {
				pattern := strings.TrimSpace(raw)
				if pattern == "" {
					continue
				}
				if _, exists := seen[pattern]; exists {
					continue
				}
				seen[pattern] = struct{}{}
				entry.AllowedModels = append(entry.AllowedModels, pattern)
			}
			if len(entry.AllowedModels) == 0 {
				entry.AllowedModels = nil
			}
		}

		cloned = append(cloned, entry)
	}

	if len(cloned) == 0 {
		return nil
	}

	return cloned
}

func normalizeAPIKeyDefaultPolicy(policy string) string {
	trimmed := strings.TrimSpace(policy)
	switch {
	case strings.EqualFold(trimmed, APIKeyDefaultPolicyAllowAll):
		return APIKeyDefaultPolicyAllowAll
	case strings.EqualFold(trimmed, APIKeyDefaultPolicyDenyAll):
		return APIKeyDefaultPolicyDenyAll
	default:
		return trimmed
	}
}

// StreamingConfig holds server streaming behavior configuration.
type StreamingConfig struct {
	// KeepAliveSeconds controls how often the server emits SSE heartbeats (": keep-alive\n\n").
	// <= 0 disables keep-alives. Default is 0.
	KeepAliveSeconds int `yaml:"keepalive-seconds,omitempty" json:"keepalive-seconds,omitempty"`

	// BootstrapRetries controls how many times the server may retry a streaming request before any bytes are sent,
	// to allow auth rotation / transient recovery.
	// <= 0 disables bootstrap retries. Default is 0.
	BootstrapRetries int `yaml:"bootstrap-retries,omitempty" json:"bootstrap-retries,omitempty"`
}
