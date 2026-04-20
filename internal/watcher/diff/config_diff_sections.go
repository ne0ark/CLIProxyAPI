package diff

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func appendConfigScalarChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	changes = appendIntChange(changes, "port", oldCfg.Port, newCfg.Port)
	if oldCfg.AuthDir != newCfg.AuthDir {
		changes = append(changes, fmt.Sprintf("auth-dir: %s -> %s", oldCfg.AuthDir, newCfg.AuthDir))
	}
	changes = appendBoolChange(changes, "debug", oldCfg.Debug, newCfg.Debug)
	changes = appendBoolChange(changes, "pprof.enable", oldCfg.Pprof.Enable, newCfg.Pprof.Enable)

	oldPprofAddr := strings.TrimSpace(oldCfg.Pprof.Addr)
	newPprofAddr := strings.TrimSpace(newCfg.Pprof.Addr)
	if oldPprofAddr != newPprofAddr {
		changes = append(changes, fmt.Sprintf("pprof.addr: %s -> %s", oldPprofAddr, newPprofAddr))
	}

	changes = appendBoolChange(changes, "logging-to-file", oldCfg.LoggingToFile, newCfg.LoggingToFile)
	changes = appendBoolChange(changes, "usage-statistics-enabled", oldCfg.UsageStatisticsEnabled, newCfg.UsageStatisticsEnabled)
	changes = appendBoolChange(changes, "disable-cooling", oldCfg.DisableCooling, newCfg.DisableCooling)
	changes = appendBoolChange(changes, "request-log", oldCfg.RequestLog, newCfg.RequestLog)
	changes = appendIntChange(changes, "logs-max-total-size-mb", oldCfg.LogsMaxTotalSizeMB, newCfg.LogsMaxTotalSizeMB)
	changes = appendIntChange(changes, "error-logs-max-files", oldCfg.ErrorLogsMaxFiles, newCfg.ErrorLogsMaxFiles)
	changes = appendIntChange(changes, "request-retry", oldCfg.RequestRetry, newCfg.RequestRetry)
	changes = appendIntChange(changes, "max-retry-credentials", oldCfg.MaxRetryCredentials, newCfg.MaxRetryCredentials)
	changes = appendIntChange(changes, "max-retry-interval", oldCfg.MaxRetryInterval, newCfg.MaxRetryInterval)

	if oldCfg.ProxyURL != newCfg.ProxyURL {
		changes = append(changes, fmt.Sprintf("proxy-url: %s -> %s", formatProxyURL(oldCfg.ProxyURL), formatProxyURL(newCfg.ProxyURL)))
	}

	changes = appendBoolChange(changes, "ws-auth", oldCfg.WebsocketAuth, newCfg.WebsocketAuth)
	changes = appendBoolChange(changes, "force-model-prefix", oldCfg.ForceModelPrefix, newCfg.ForceModelPrefix)
	changes = appendIntChange(changes, "nonstream-keepalive-interval", oldCfg.NonStreamKeepAliveInterval, newCfg.NonStreamKeepAliveInterval)

	return changes
}

func appendQuotaExceededChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	changes = appendBoolChange(changes, "quota-exceeded.switch-project", oldCfg.QuotaExceeded.SwitchProject, newCfg.QuotaExceeded.SwitchProject)
	changes = appendBoolChange(changes, "quota-exceeded.switch-preview-model", oldCfg.QuotaExceeded.SwitchPreviewModel, newCfg.QuotaExceeded.SwitchPreviewModel)
	changes = appendBoolChange(changes, "quota-exceeded.antigravity-credits", oldCfg.QuotaExceeded.AntigravityCredits, newCfg.QuotaExceeded.AntigravityCredits)
	return changes
}

func appendRoutingChange(changes []string, oldCfg, newCfg *config.Config) []string {
	if oldCfg.Routing.Strategy != newCfg.Routing.Strategy {
		changes = append(changes, fmt.Sprintf("routing.strategy: %s -> %s", oldCfg.Routing.Strategy, newCfg.Routing.Strategy))
	}
	return changes
}

func appendAPIKeysChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	if len(oldCfg.APIKeys) != len(newCfg.APIKeys) {
		return append(changes, fmt.Sprintf("api-keys count: %d -> %d", len(oldCfg.APIKeys), len(newCfg.APIKeys)))
	}
	if !reflect.DeepEqual(trimStrings(oldCfg.APIKeys), trimStrings(newCfg.APIKeys)) {
		return append(changes, "api-keys: values updated (count unchanged, redacted)")
	}
	return changes
}

func appendAmpCodeChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	oldAmpURL := strings.TrimSpace(oldCfg.AmpCode.UpstreamURL)
	newAmpURL := strings.TrimSpace(newCfg.AmpCode.UpstreamURL)
	if oldAmpURL != newAmpURL {
		changes = append(changes, fmt.Sprintf("ampcode.upstream-url: %s -> %s", oldAmpURL, newAmpURL))
	}

	oldAmpKey := strings.TrimSpace(oldCfg.AmpCode.UpstreamAPIKey)
	newAmpKey := strings.TrimSpace(newCfg.AmpCode.UpstreamAPIKey)
	switch {
	case oldAmpKey == "" && newAmpKey != "":
		changes = append(changes, "ampcode.upstream-api-key: added")
	case oldAmpKey != "" && newAmpKey == "":
		changes = append(changes, "ampcode.upstream-api-key: removed")
	case oldAmpKey != newAmpKey:
		changes = append(changes, "ampcode.upstream-api-key: updated")
	}

	changes = appendBoolChange(
		changes,
		"ampcode.restrict-management-to-localhost",
		oldCfg.AmpCode.RestrictManagementToLocalhost,
		newCfg.AmpCode.RestrictManagementToLocalhost,
	)

	oldMappings := SummarizeAmpModelMappings(oldCfg.AmpCode.ModelMappings)
	newMappings := SummarizeAmpModelMappings(newCfg.AmpCode.ModelMappings)
	if oldMappings.hash != newMappings.hash {
		changes = append(changes, fmt.Sprintf("ampcode.model-mappings: updated (%d -> %d entries)", oldMappings.count, newMappings.count))
	}

	changes = appendBoolChange(
		changes,
		"ampcode.force-model-mappings",
		oldCfg.AmpCode.ForceModelMappings,
		newCfg.AmpCode.ForceModelMappings,
	)

	oldUpstreamAPIKeysCount := len(oldCfg.AmpCode.UpstreamAPIKeys)
	newUpstreamAPIKeysCount := len(newCfg.AmpCode.UpstreamAPIKeys)
	if !equalUpstreamAPIKeys(oldCfg.AmpCode.UpstreamAPIKeys, newCfg.AmpCode.UpstreamAPIKeys) {
		changes = append(changes, fmt.Sprintf("ampcode.upstream-api-keys: updated (%d -> %d entries)", oldUpstreamAPIKeysCount, newUpstreamAPIKeysCount))
	}

	return changes
}

func appendOAuthChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	if entries, _ := DiffOAuthExcludedModelChanges(oldCfg.OAuthExcludedModels, newCfg.OAuthExcludedModels); len(entries) > 0 {
		changes = append(changes, entries...)
	}
	if entries, _ := DiffOAuthModelAliasChanges(oldCfg.OAuthModelAlias, newCfg.OAuthModelAlias); len(entries) > 0 {
		changes = append(changes, entries...)
	}
	return changes
}

func appendRemoteManagementChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	changes = appendBoolChange(
		changes,
		"remote-management.allow-remote",
		oldCfg.RemoteManagement.AllowRemote,
		newCfg.RemoteManagement.AllowRemote,
	)
	changes = appendBoolChange(
		changes,
		"remote-management.disable-control-panel",
		oldCfg.RemoteManagement.DisableControlPanel,
		newCfg.RemoteManagement.DisableControlPanel,
	)
	changes = appendBoolChange(
		changes,
		"remote-management.disable-auto-update-panel",
		oldCfg.RemoteManagement.DisableAutoUpdatePanel,
		newCfg.RemoteManagement.DisableAutoUpdatePanel,
	)

	oldPanelRepo := strings.TrimSpace(oldCfg.RemoteManagement.PanelGitHubRepository)
	newPanelRepo := strings.TrimSpace(newCfg.RemoteManagement.PanelGitHubRepository)
	if oldPanelRepo != newPanelRepo {
		changes = append(changes, fmt.Sprintf("remote-management.panel-github-repository: %s -> %s", oldPanelRepo, newPanelRepo))
	}

	if oldCfg.RemoteManagement.SecretKey != newCfg.RemoteManagement.SecretKey {
		switch {
		case oldCfg.RemoteManagement.SecretKey == "" && newCfg.RemoteManagement.SecretKey != "":
			changes = append(changes, "remote-management.secret-key: created")
		case oldCfg.RemoteManagement.SecretKey != "" && newCfg.RemoteManagement.SecretKey == "":
			changes = append(changes, "remote-management.secret-key: deleted")
		default:
			changes = append(changes, "remote-management.secret-key: updated")
		}
	}

	return changes
}

func appendOpenAICompatibilityChanges(changes []string, oldCfg, newCfg *config.Config) []string {
	if compat := DiffOpenAICompatibility(oldCfg.OpenAICompatibility, newCfg.OpenAICompatibility); len(compat) > 0 {
		changes = append(changes, "openai-compatibility:")
		for _, entry := range compat {
			changes = append(changes, "  "+entry)
		}
	}
	return changes
}

func appendIntChange(changes []string, label string, oldValue, newValue int) []string {
	if oldValue != newValue {
		return append(changes, fmt.Sprintf("%s: %d -> %d", label, oldValue, newValue))
	}
	return changes
}

func appendBoolChange(changes []string, label string, oldValue, newValue bool) []string {
	if oldValue != newValue {
		return append(changes, fmt.Sprintf("%s: %t -> %t", label, oldValue, newValue))
	}
	return changes
}
