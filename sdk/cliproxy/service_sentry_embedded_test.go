package cliproxy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/watcher"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestServiceSentryEmbeddedHostIsolation(t *testing.T) {
	hostClient, err := sentry.NewClient(sentry.ClientOptions{
		Dsn: "https://public@example.com/host",
	})
	if err != nil {
		t.Fatalf("failed to create host sentry client: %v", err)
	}

	sentry.CurrentHub().BindClient(hostClient)
	t.Cleanup(func() {
		cleanupSentryHubClient(sentry.CurrentHub())
		if err := logging.ConfigureErrorTracking(&config.Config{}); err != nil {
			t.Fatalf("reset CLIProxyAPI error tracking: %v", err)
		}
	})

	if err := logging.ConfigureErrorTracking(&config.Config{}); err != nil {
		t.Fatalf("reset CLIProxyAPI error tracking before startup: %v", err)
	}

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("debug: false\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	started := make(chan struct{})
	service, err := NewBuilder().
		WithConfig(&config.Config{
			Host:    "127.0.0.1",
			Port:    0,
			AuthDir: filepath.Join(tempDir, "auth"),
			Sentry: internalconfig.SentryConfig{
				DSN: "https://public@example.com/cliproxy",
			},
		}).
		WithConfigPath(configPath).
		WithTokenClientProvider(noopTokenClientProvider{}).
		WithAPIKeyClientProvider(noopAPIKeyClientProvider{}).
		WithWatcherFactory(noopWatcherFactory).
		WithHooks(Hooks{
			OnAfterStart: func(*Service) {
				close(started)
			},
		}).
		Build()
	if err != nil {
		t.Fatalf("build embedded service: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- service.Run(ctx)
	}()

	select {
	case <-started:
	case err := <-runErr:
		t.Fatalf("service exited before startup completed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for embedded service startup")
	}

	assertHostClientUnchanged(t, hostClient)

	reconfigured := *service.cfg
	reconfigured.Sentry.DSN = "https://public@example.com/cliproxy-reconfigured"
	service.server.UpdateClients(&reconfigured)
	assertHostClientUnchanged(t, hostClient)

	disabled := reconfigured
	disabled.Sentry.DSN = ""
	service.server.UpdateClients(&disabled)
	assertHostClientUnchanged(t, hostClient)

	cancel()

	select {
	case err := <-runErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("service returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for embedded service shutdown")
	}
}

type noopTokenClientProvider struct{}

func (noopTokenClientProvider) Load(ctx context.Context, cfg *config.Config) (*TokenClientResult, error) {
	return &TokenClientResult{}, nil
}

type noopAPIKeyClientProvider struct{}

func (noopAPIKeyClientProvider) Load(ctx context.Context, cfg *config.Config) (*APIKeyClientResult, error) {
	return &APIKeyClientResult{}, nil
}

func noopWatcherFactory(configPath, authDir string, reload func(*config.Config)) (*WatcherWrapper, error) {
	return &WatcherWrapper{
		start: func(ctx context.Context) error { return nil },
		stop:  func() error { return nil },
		setConfig: func(cfg *config.Config) {
		},
		snapshotAuths: func() []*coreauth.Auth { return nil },
		setUpdateQueue: func(queue chan<- watcher.AuthUpdate) {
		},
		dispatchRuntimeUpdate: func(update watcher.AuthUpdate) bool { return false },
	}, nil
}

func assertHostClientUnchanged(t *testing.T, hostClient *sentry.Client) {
	t.Helper()

	if got := sentry.CurrentHub().Client(); got != hostClient {
		t.Fatal("expected embedded CLIProxyAPI lifecycle to leave the host sentry client bound globally")
	}
}

func cleanupSentryHubClient(hub *sentry.Hub) {
	if hub == nil {
		return
	}
	client := hub.Client()
	if client != nil {
		client.Flush(200 * time.Millisecond)
		client.Close()
	}
	hub.BindClient(nil)
}
