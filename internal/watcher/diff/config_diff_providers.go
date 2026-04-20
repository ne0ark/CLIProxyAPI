package diff

import (
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func appendGeminiKeyChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	if len(oldCfg.GeminiKey) != len(newCfg.GeminiKey) {
		return append(changes, fmt.Sprintf("gemini-api-key count: %d -> %d", len(oldCfg.GeminiKey), len(newCfg.GeminiKey)))
	}

	for i := range oldCfg.GeminiKey {
		oldKey := oldCfg.GeminiKey[i]
		newKey := newCfg.GeminiKey[i]

		changes = appendEntryTrimmedStringChange(changes, "gemini", i, "base-url", oldKey.BaseURL, newKey.BaseURL)
		changes = appendEntryProxyURLChange(changes, "gemini", i, oldKey.ProxyURL, newKey.ProxyURL)
		changes = appendEntryTrimmedStringChange(changes, "gemini", i, "prefix", oldKey.Prefix, newKey.Prefix)
		changes = appendEntryUpdatedChange(changes, "gemini", i, "api-key", strings.TrimSpace(oldKey.APIKey) != strings.TrimSpace(newKey.APIKey))
		changes = appendEntryUpdatedChange(changes, "gemini", i, "headers", !equalStringMap(oldKey.Headers, newKey.Headers))

		oldModels := SummarizeGeminiModels(oldKey.Models)
		newModels := SummarizeGeminiModels(newKey.Models)
		changes = appendEntrySummaryChange(changes, "gemini", i, "models", oldModels.hash, newModels.hash, oldModels.count, newModels.count)

		oldExcluded := SummarizeExcludedModels(oldKey.ExcludedModels)
		newExcluded := SummarizeExcludedModels(newKey.ExcludedModels)
		changes = appendEntrySummaryChange(changes, "gemini", i, "excluded-models", oldExcluded.hash, newExcluded.hash, oldExcluded.count, newExcluded.count)
	}

	return changes
}

func appendClaudeKeyChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	if len(oldCfg.ClaudeKey) != len(newCfg.ClaudeKey) {
		return append(changes, fmt.Sprintf("claude-api-key count: %d -> %d", len(oldCfg.ClaudeKey), len(newCfg.ClaudeKey)))
	}

	for i := range oldCfg.ClaudeKey {
		oldKey := oldCfg.ClaudeKey[i]
		newKey := newCfg.ClaudeKey[i]

		changes = appendEntryTrimmedStringChange(changes, "claude", i, "base-url", oldKey.BaseURL, newKey.BaseURL)
		changes = appendEntryProxyURLChange(changes, "claude", i, oldKey.ProxyURL, newKey.ProxyURL)
		changes = appendEntryTrimmedStringChange(changes, "claude", i, "prefix", oldKey.Prefix, newKey.Prefix)
		changes = appendEntryUpdatedChange(changes, "claude", i, "api-key", strings.TrimSpace(oldKey.APIKey) != strings.TrimSpace(newKey.APIKey))
		changes = appendEntryUpdatedChange(changes, "claude", i, "headers", !equalStringMap(oldKey.Headers, newKey.Headers))

		oldModels := SummarizeClaudeModels(oldKey.Models)
		newModels := SummarizeClaudeModels(newKey.Models)
		changes = appendEntrySummaryChange(changes, "claude", i, "models", oldModels.hash, newModels.hash, oldModels.count, newModels.count)

		oldExcluded := SummarizeExcludedModels(oldKey.ExcludedModels)
		newExcluded := SummarizeExcludedModels(newKey.ExcludedModels)
		changes = appendEntrySummaryChange(changes, "claude", i, "excluded-models", oldExcluded.hash, newExcluded.hash, oldExcluded.count, newExcluded.count)

		if oldKey.Cloak != nil && newKey.Cloak != nil {
			changes = appendEntryTrimmedStringChange(changes, "claude", i, "cloak.mode", oldKey.Cloak.Mode, newKey.Cloak.Mode)
			changes = appendEntryBoolChange(changes, "claude", i, "cloak.strict-mode", oldKey.Cloak.StrictMode, newKey.Cloak.StrictMode)
			changes = appendEntryCountChange(changes, "claude", i, "cloak.sensitive-words", len(oldKey.Cloak.SensitiveWords), len(newKey.Cloak.SensitiveWords))
		}
	}

	return changes
}

func appendCodexKeyChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	if len(oldCfg.CodexKey) != len(newCfg.CodexKey) {
		return append(changes, fmt.Sprintf("codex-api-key count: %d -> %d", len(oldCfg.CodexKey), len(newCfg.CodexKey)))
	}

	for i := range oldCfg.CodexKey {
		oldKey := oldCfg.CodexKey[i]
		newKey := newCfg.CodexKey[i]

		changes = appendEntryTrimmedStringChange(changes, "codex", i, "base-url", oldKey.BaseURL, newKey.BaseURL)
		changes = appendEntryProxyURLChange(changes, "codex", i, oldKey.ProxyURL, newKey.ProxyURL)
		changes = appendEntryTrimmedStringChange(changes, "codex", i, "prefix", oldKey.Prefix, newKey.Prefix)
		changes = appendEntryBoolChange(changes, "codex", i, "websockets", oldKey.Websockets, newKey.Websockets)
		changes = appendEntryUpdatedChange(changes, "codex", i, "api-key", strings.TrimSpace(oldKey.APIKey) != strings.TrimSpace(newKey.APIKey))
		changes = appendEntryUpdatedChange(changes, "codex", i, "headers", !equalStringMap(oldKey.Headers, newKey.Headers))

		oldModels := SummarizeCodexModels(oldKey.Models)
		newModels := SummarizeCodexModels(newKey.Models)
		changes = appendEntrySummaryChange(changes, "codex", i, "models", oldModels.hash, newModels.hash, oldModels.count, newModels.count)

		oldExcluded := SummarizeExcludedModels(oldKey.ExcludedModels)
		newExcluded := SummarizeExcludedModels(newKey.ExcludedModels)
		changes = appendEntrySummaryChange(changes, "codex", i, "excluded-models", oldExcluded.hash, newExcluded.hash, oldExcluded.count, newExcluded.count)
	}

	return changes
}

func appendVertexCompatKeyChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	if len(oldCfg.VertexCompatAPIKey) != len(newCfg.VertexCompatAPIKey) {
		return append(changes, fmt.Sprintf("vertex-api-key count: %d -> %d", len(oldCfg.VertexCompatAPIKey), len(newCfg.VertexCompatAPIKey)))
	}

	for i := range oldCfg.VertexCompatAPIKey {
		oldKey := oldCfg.VertexCompatAPIKey[i]
		newKey := newCfg.VertexCompatAPIKey[i]

		changes = appendEntryTrimmedStringChange(changes, "vertex", i, "base-url", oldKey.BaseURL, newKey.BaseURL)
		changes = appendEntryProxyURLChange(changes, "vertex", i, oldKey.ProxyURL, newKey.ProxyURL)
		changes = appendEntryTrimmedStringChange(changes, "vertex", i, "prefix", oldKey.Prefix, newKey.Prefix)
		changes = appendEntryUpdatedChange(changes, "vertex", i, "api-key", strings.TrimSpace(oldKey.APIKey) != strings.TrimSpace(newKey.APIKey))

		oldModels := SummarizeVertexModels(oldKey.Models)
		newModels := SummarizeVertexModels(newKey.Models)
		changes = appendEntrySummaryChange(changes, "vertex", i, "models", oldModels.hash, newModels.hash, oldModels.count, newModels.count)

		oldExcluded := SummarizeExcludedModels(oldKey.ExcludedModels)
		newExcluded := SummarizeExcludedModels(newKey.ExcludedModels)
		changes = appendEntrySummaryChange(changes, "vertex", i, "excluded-models", oldExcluded.hash, newExcluded.hash, oldExcluded.count, newExcluded.count)
		changes = appendEntryUpdatedChange(changes, "vertex", i, "headers", !equalStringMap(oldKey.Headers, newKey.Headers))
	}

	return changes
}

func appendEntryTrimmedStringChange(changes []string, provider string, index int, field string, oldValue, newValue string) []string {
	oldTrimmed := strings.TrimSpace(oldValue)
	newTrimmed := strings.TrimSpace(newValue)
	if oldTrimmed != newTrimmed {
		return append(changes, fmt.Sprintf("%s[%d].%s: %s -> %s", provider, index, field, oldTrimmed, newTrimmed))
	}
	return changes
}

func appendEntryProxyURLChange(changes []string, provider string, index int, oldValue, newValue string) []string {
	if strings.TrimSpace(oldValue) != strings.TrimSpace(newValue) {
		return append(changes, fmt.Sprintf("%s[%d].proxy-url: %s -> %s", provider, index, formatProxyURL(oldValue), formatProxyURL(newValue)))
	}
	return changes
}

func appendEntryUpdatedChange(changes []string, provider string, index int, field string, changed bool) []string {
	if changed {
		return append(changes, fmt.Sprintf("%s[%d].%s: updated", provider, index, field))
	}
	return changes
}

func appendEntryBoolChange(changes []string, provider string, index int, field string, oldValue, newValue bool) []string {
	if oldValue != newValue {
		return append(changes, fmt.Sprintf("%s[%d].%s: %t -> %t", provider, index, field, oldValue, newValue))
	}
	return changes
}

func appendEntryCountChange(changes []string, provider string, index int, field string, oldValue, newValue int) []string {
	if oldValue != newValue {
		return append(changes, fmt.Sprintf("%s[%d].%s: %d -> %d", provider, index, field, oldValue, newValue))
	}
	return changes
}

func appendEntrySummaryChange(changes []string, provider string, index int, field string, oldHash, newHash string, oldCount, newCount int) []string {
	if oldHash != newHash {
		return append(changes, fmt.Sprintf("%s[%d].%s: updated (%d -> %d entries)", provider, index, field, oldCount, newCount))
	}
	return changes
}
