// Package cliproxy provides the core service implementation for the CLI Proxy API.
// It includes service lifecycle management, authentication handling, file watching,
// and integration with various AI service providers through a unified interface.
package cliproxy

import (
	"fmt"
	"strings"
	"time"

	configaccess "github.com/router-for-me/CLIProxyAPI/v6/internal/access/config_access"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v6/sdk/access"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v6/sdk/auth"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	log "github.com/sirupsen/logrus"
)

// Builder constructs a Service instance with customizable providers.
// It provides a fluent interface for configuring all aspects of the service
// including authentication, file watching, HTTP server options, and lifecycle hooks.
type Builder struct {
	// cfg holds the application configuration.
	cfg *config.Config

	// configPath is the path to the configuration file.
	configPath string

	// tokenProvider handles loading token-based clients.
	tokenProvider TokenClientProvider

	// apiKeyProvider handles loading API key-based clients.
	apiKeyProvider APIKeyClientProvider

	// watcherFactory creates file watcher instances.
	watcherFactory WatcherFactory

	// hooks provides lifecycle callbacks.
	hooks Hooks

	// authManager handles legacy authentication operations.
	authManager *sdkAuth.Manager

	// accessManager handles request authentication providers.
	accessManager *sdkaccess.Manager

	// coreManager handles core authentication and execution.
	coreManager *coreauth.Manager

	// serverOptions contains additional server configuration options.
	serverOptions []api.ServerOption
}

func normalizeSelectorStrategy(strategy string) string {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "fill-first", "fillfirst", "ff":
		return "fill-first"
	default:
		return "round-robin"
	}
}

func sessionAffinityEnabled(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.Routing.ClaudeCodeSessionAffinity || cfg.Routing.SessionAffinity
}

func buildManagedCoreSelector(cfg *config.Config) coreauth.Selector {
	strategy := "round-robin"
	sessionAffinity := false
	sessionAffinityTTL := time.Hour
	codexWebsocketStrictAffinity := false
	if cfg != nil {
		strategy = normalizeSelectorStrategy(cfg.Routing.Strategy)
		sessionAffinity = sessionAffinityEnabled(cfg)
		codexWebsocketStrictAffinity = cfg.Routing.CodexWebsocketStrictAffinity
		if ttlStr := strings.TrimSpace(cfg.Routing.SessionAffinityTTL); ttlStr != "" {
			if parsed, err := time.ParseDuration(ttlStr); err == nil && parsed > 0 {
				sessionAffinityTTL = parsed
			}
		}
	}
	if codexWebsocketStrictAffinity && !sessionAffinity {
		log.Warn("routing.codex-websocket-strict-affinity requires routing.session-affinity (or legacy routing.claude-code-session-affinity); strict Codex websocket bindings will remain inactive until session affinity is enabled")
	}

	var selector coreauth.Selector
	switch strategy {
	case "fill-first":
		selector = &coreauth.FillFirstSelector{}
	default:
		selector = &coreauth.RoundRobinSelector{}
	}

	if sessionAffinity {
		selector = coreauth.NewSessionAffinitySelectorWithConfig(coreauth.SessionAffinityConfig{
			Fallback:                     selector,
			TTL:                          sessionAffinityTTL,
			CodexWebsocketStrictAffinity: codexWebsocketStrictAffinity,
		})
	}
	return selector
}

func warnIgnoredSelectorConfigWithCustomCoreManager(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if normalizeSelectorStrategy(cfg.Routing.Strategy) != "round-robin" ||
		sessionAffinityEnabled(cfg) ||
		cfg.Routing.CodexWebsocketStrictAffinity {
		log.Warn("cliproxy: WithCoreAuthManager overrides config-managed selector routing; routing.strategy, routing.session-affinity, and routing.codex-websocket-strict-affinity must be configured on the injected core manager")
	}
}

// Hooks allows callers to plug into service lifecycle stages.
// These callbacks provide opportunities to perform custom initialization
// and cleanup operations during service startup and shutdown.
type Hooks struct {
	// OnBeforeStart is called before the service starts, allowing configuration
	// modifications or additional setup.
	OnBeforeStart func(*config.Config)

	// OnAfterStart is called after the service has started successfully,
	// providing access to the service instance for additional operations.
	OnAfterStart func(*Service)
}

// NewBuilder creates a Builder with default dependencies left unset.
// Use the fluent interface methods to configure the service before calling Build().
//
// Returns:
//   - *Builder: A new builder instance ready for configuration
func NewBuilder() *Builder {
	return &Builder{}
}

// WithConfig sets the configuration instance used by the service.
//
// Parameters:
//   - cfg: The application configuration
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithConfig(cfg *config.Config) *Builder {
	b.cfg = cfg
	return b
}

// WithConfigPath sets the absolute configuration file path used for reload watching.
//
// Parameters:
//   - path: The absolute path to the configuration file
//
// Returns:
//   - *Builder: The builder instance for method chaining
func (b *Builder) WithConfigPath(path string) *Builder {
	b.configPath = path
	return b
}

// WithTokenClientProvider overrides the provider responsible for token-backed clients.
func (b *Builder) WithTokenClientProvider(provider TokenClientProvider) *Builder {
	b.tokenProvider = provider
	return b
}

// WithAPIKeyClientProvider overrides the provider responsible for API key-backed clients.
func (b *Builder) WithAPIKeyClientProvider(provider APIKeyClientProvider) *Builder {
	b.apiKeyProvider = provider
	return b
}

// WithWatcherFactory allows customizing the watcher factory that handles reloads.
func (b *Builder) WithWatcherFactory(factory WatcherFactory) *Builder {
	b.watcherFactory = factory
	return b
}

// WithHooks registers lifecycle hooks executed around service startup.
func (b *Builder) WithHooks(h Hooks) *Builder {
	b.hooks = h
	return b
}

// WithAuthManager overrides the authentication manager used for token lifecycle operations.
func (b *Builder) WithAuthManager(mgr *sdkAuth.Manager) *Builder {
	b.authManager = mgr
	return b
}

// WithRequestAccessManager overrides the request authentication manager.
func (b *Builder) WithRequestAccessManager(mgr *sdkaccess.Manager) *Builder {
	b.accessManager = mgr
	return b
}

// WithCoreAuthManager overrides the runtime auth manager responsible for request execution.
func (b *Builder) WithCoreAuthManager(mgr *coreauth.Manager) *Builder {
	b.coreManager = mgr
	return b
}

// WithServerOptions appends server configuration options used during construction.
func (b *Builder) WithServerOptions(opts ...api.ServerOption) *Builder {
	b.serverOptions = append(b.serverOptions, opts...)
	return b
}

// WithLocalManagementPassword configures a password that is only accepted from localhost management requests.
func (b *Builder) WithLocalManagementPassword(password string) *Builder {
	if password == "" {
		return b
	}
	b.serverOptions = append(b.serverOptions, api.WithLocalManagementPassword(password))
	return b
}

// WithPostAuthHook registers a hook to be called after an Auth record is created
// but before it is persisted to storage.
func (b *Builder) WithPostAuthHook(hook coreauth.PostAuthHook) *Builder {
	if hook == nil {
		return b
	}
	b.serverOptions = append(b.serverOptions, api.WithPostAuthHook(hook))
	return b
}

// Build validates inputs, applies defaults, and returns a ready-to-run service.
func (b *Builder) Build() (*Service, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("cliproxy: configuration is required")
	}
	if b.configPath == "" {
		return nil, fmt.Errorf("cliproxy: configuration path is required")
	}

	tokenProvider := b.tokenProvider
	if tokenProvider == nil {
		tokenProvider = NewFileTokenClientProvider()
	}

	apiKeyProvider := b.apiKeyProvider
	if apiKeyProvider == nil {
		apiKeyProvider = NewAPIKeyClientProvider()
	}

	watcherFactory := b.watcherFactory
	if watcherFactory == nil {
		watcherFactory = defaultWatcherFactory
	}

	authManager := b.authManager
	if authManager == nil {
		authManager = newDefaultAuthManager()
	}

	accessManager := b.accessManager
	if accessManager == nil {
		accessManager = sdkaccess.NewManager()
	}

	configaccess.Register(&b.cfg.SDKConfig)
	accessManager.SetProviders(sdkaccess.RegisteredProviders())

	coreManager := b.coreManager
	manageCoreSelector := coreManager == nil
	if coreManager == nil {
		tokenStore := sdkAuth.GetTokenStore()
		if dirSetter, ok := tokenStore.(interface{ SetBaseDir(string) }); ok && b.cfg != nil {
			dirSetter.SetBaseDir(b.cfg.AuthDir)
		}
		selector := buildManagedCoreSelector(b.cfg)
		coreManager = coreauth.NewManager(tokenStore, selector, nil)
	} else {
		warnIgnoredSelectorConfigWithCustomCoreManager(b.cfg)
	}
	// Attach a default RoundTripper provider so providers can opt-in per-auth transports.
	coreManager.SetRoundTripperProvider(newDefaultRoundTripperProvider())
	coreManager.SetConfig(b.cfg)
	coreManager.SetOAuthModelAlias(b.cfg.OAuthModelAlias)

	service := &Service{
		cfg:                b.cfg,
		configPath:         b.configPath,
		tokenProvider:      tokenProvider,
		apiKeyProvider:     apiKeyProvider,
		watcherFactory:     watcherFactory,
		hooks:              b.hooks,
		authManager:        authManager,
		accessManager:      accessManager,
		coreManager:        coreManager,
		manageCoreSelector: manageCoreSelector,
		serverOptions:      append([]api.ServerOption(nil), b.serverOptions...),
	}
	coreManager.SetHook(serviceAuthHook{service: service, next: coreManager.Hook()})
	return service, nil
}
