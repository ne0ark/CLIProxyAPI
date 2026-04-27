package cliproxy

import (
	"context"
	"testing"

	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

type serviceSelectorReloadStub struct {
	stops int
}

func (s *serviceSelectorReloadStub) Pick(ctx context.Context, provider, model string, opts executor.Options, auths []*coreauth.Auth) (*coreauth.Auth, error) {
	return nil, nil
}

func (s *serviceSelectorReloadStub) Stop() {
	s.stops++
}

func TestServiceApplySelectorConfigChange_DoesNotReplaceInjectedCoreManagerSelector(t *testing.T) {
	selector := &serviceSelectorReloadStub{}
	service := &Service{
		coreManager:        coreauth.NewManager(nil, selector, nil),
		manageCoreSelector: false,
	}

	previousCfg := &config.Config{}
	nextCfg := &config.Config{}
	nextCfg.Routing.CodexWebsocketStrictAffinity = true

	service.applySelectorConfigChange(previousCfg, nextCfg)

	if selector.stops != 0 {
		t.Fatalf("selector stop count = %d, want 0 for injected core manager", selector.stops)
	}
}

func TestServiceApplySelectorConfigChange_ReplacesManagedSelectorOnStrictToggle(t *testing.T) {
	selector := &serviceSelectorReloadStub{}
	service := &Service{
		coreManager:        coreauth.NewManager(nil, selector, nil),
		manageCoreSelector: true,
	}

	previousCfg := &config.Config{}
	previousCfg.Routing.SessionAffinity = true
	nextCfg := &config.Config{}
	nextCfg.Routing.SessionAffinity = true
	nextCfg.Routing.CodexWebsocketStrictAffinity = true

	service.applySelectorConfigChange(previousCfg, nextCfg)

	if selector.stops != 1 {
		t.Fatalf("selector stop count = %d, want 1 for managed selector replacement", selector.stops)
	}
}
