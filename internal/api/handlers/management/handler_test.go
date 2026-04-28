package management

import (
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestAuthenticateManagementKey_LocalhostIPBan_BlocksCorrectKeyDuringBan(t *testing.T) {
	h := &Handler{
		cfg:            &config.Config{},
		failedAttempts: make(map[string]*attemptInfo),
		envSecret:      "test-secret",
	}

	for i := 0; i < 5; i++ {
		allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "wrong-secret")
		if allowed {
			t.Fatalf("expected auth to be denied at attempt %d", i+1)
		}
		if statusCode != http.StatusUnauthorized || errMsg != "invalid management key" {
			t.Fatalf("unexpected auth failure at attempt %d: status=%d msg=%q", i+1, statusCode, errMsg)
		}
	}

	allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "test-secret")
	if allowed {
		t.Fatalf("expected correct key to be denied while banned")
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("expected forbidden status while banned, got %d", statusCode)
	}
	if !strings.HasPrefix(errMsg, "IP banned due to too many failed attempts. Try again in") {
		t.Fatalf("unexpected banned message: %q", errMsg)
	}
}

func TestAuthenticateManagementKey_LocalPasswordOnlyAllowsLocalhost(t *testing.T) {
	h := &Handler{
		cfg:            &config.Config{},
		failedAttempts: make(map[string]*attemptInfo),
		localPassword:  "local-password",
	}

	allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "local-password")
	if !allowed || statusCode != 0 || errMsg != "" {
		t.Fatalf("expected localhost local password auth to succeed, got allowed=%t status=%d msg=%q", allowed, statusCode, errMsg)
	}

	allowed, statusCode, errMsg = h.AuthenticateManagementKey("127.0.0.1", true, "wrong")
	if allowed || statusCode != http.StatusUnauthorized || errMsg != "invalid management key" {
		t.Fatalf("expected wrong localhost local password to fail with unauthorized, got allowed=%t status=%d msg=%q", allowed, statusCode, errMsg)
	}

	allowed, statusCode, errMsg = h.AuthenticateManagementKey("10.0.0.5", false, "local-password")
	if allowed || statusCode != http.StatusForbidden || errMsg != "remote management disabled" {
		t.Fatalf("expected remote client to stay blocked without remote management secret, got allowed=%t status=%d msg=%q", allowed, statusCode, errMsg)
	}
}
