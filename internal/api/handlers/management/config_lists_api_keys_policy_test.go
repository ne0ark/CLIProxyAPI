package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func findPolicy(policies []config.APIKeyPolicy, key string) *config.APIKeyPolicy {
	for i := range policies {
		if policies[i].Key == key {
			return &policies[i]
		}
	}
	return nil
}

func TestGetAPIKeys_OmitsPoliciesWhenUnset(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-a", "sk-b"},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/api-keys", nil)

	h.GetAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, exists := payload["api-key-policies"]; exists {
		t.Fatalf("unexpected api-key-policies field in response: %s", rec.Body.String())
	}
}

func TestGetAPIKeys_IncludesPoliciesWhenPresent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-a"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/api-keys", nil)

	h.GetAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"api-key-policies"`) {
		t.Fatalf("expected api-key-policies field in response: %s", rec.Body.String())
	}
}

func TestPutAPIKeys_StructuredBodyRebuildsPoliciesAndPersists(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	configPath := writeTestConfigFile(t)
	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-stale"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-stale", AllowedModels: []string{"claude-3-*"}},
				},
			},
		},
		configFilePath: configPath,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/v0/management/api-keys", bytes.NewBufferString(`[{"key":"sk-fresh","allowedModels":["gpt-4o*"]}]`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PutAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := h.cfg.APIKeys; len(got) != 1 || got[0] != "sk-fresh" {
		t.Fatalf("APIKeys after put = %#v", got)
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-stale") != nil {
		t.Fatalf("stale sk-stale policy must be dropped by PUT")
	}
	if p := findPolicy(h.cfg.APIKeyPolicies, "sk-fresh"); p == nil || len(p.AllowedModels) != 1 {
		t.Fatalf("sk-fresh policy must be present: %#v", h.cfg.APIKeyPolicies)
	}
	if !h.cfg.IsModelAllowedForKey("sk-fresh", "gpt-4o") {
		t.Fatalf("sk-fresh must allow gpt-4o via new policy")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read persisted config: %v", err)
	}
	if !strings.Contains(string(data), "api-key-policies") {
		t.Fatalf("persisted config must contain api-key-policies, got:\n%s", string(data))
	}
}

func TestPutAPIKeys_DuplicateStructuredKeyRejected(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg:            &config.Config{},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/v0/management/api-keys", bytes.NewBufferString(`[{"key":"sk-dup"},{"key":"sk-dup","allowedModels":["gpt-4o*"]}]`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PutAPIKeys(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPatchAPIKeys_RenameViaOldNewMovesPolicy(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-old"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-old", AllowedModels: []string{"gpt-4o*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/api-keys", bytes.NewBufferString(`{"old":"sk-old","new":"sk-new"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PatchAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := h.cfg.APIKeys; len(got) != 1 || got[0] != "sk-new" {
		t.Fatalf("APIKeys after rename = %#v, want [sk-new]", got)
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-old") != nil {
		t.Fatalf("policy for sk-old must be removed after rename")
	}
	p := findPolicy(h.cfg.APIKeyPolicies, "sk-new")
	if p == nil {
		t.Fatalf("policy for sk-new must exist after rename")
	}
	if len(p.AllowedModels) != 1 || p.AllowedModels[0] != "gpt-4o*" {
		t.Fatalf("renamed policy lost AllowedModels: %#v", p.AllowedModels)
	}
	if !h.cfg.IsModelAllowedForKey("sk-new", "gpt-4o-mini") {
		t.Fatalf("sk-new must allow gpt-4o-mini via migrated policy")
	}
	if h.cfg.IsModelAllowedForKey("sk-new", "claude-3-5-sonnet-20241022") {
		t.Fatalf("sk-new must reject models outside its allowlist")
	}
}

func TestPatchAPIKeys_RenameViaIndexValueMovesPolicy(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-first", "sk-old"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-old", AllowedModels: []string{"claude-3-*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/api-keys", bytes.NewBufferString(`{"index":1,"value":"sk-rotated"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PatchAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := h.cfg.APIKeys; len(got) != 2 || got[1] != "sk-rotated" {
		t.Fatalf("APIKeys after rotation = %#v, want [sk-first sk-rotated]", got)
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-old") != nil {
		t.Fatalf("policy for sk-old must be removed after rotation")
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-rotated") == nil {
		t.Fatalf("policy for sk-rotated must exist after rotation")
	}
}

func TestPatchAPIKeys_RenameWithExistingTargetPolicyKeepsTarget(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-src", "sk-dst"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-src", AllowedModels: []string{"claude-3-*"}},
					{Key: "sk-dst", AllowedModels: []string{"gpt-4o*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/api-keys", bytes.NewBufferString(`{"old":"sk-src","new":"sk-dst"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PatchAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-src") != nil {
		t.Fatalf("sk-src policy must be dropped")
	}
	p := findPolicy(h.cfg.APIKeyPolicies, "sk-dst")
	if p == nil {
		t.Fatalf("sk-dst policy must survive")
	}
	if len(p.AllowedModels) != 1 || p.AllowedModels[0] != "gpt-4o*" {
		t.Fatalf("sk-dst policy mutated: %#v", p.AllowedModels)
	}
}

func TestPatchAPIKeys_AppendNewKeyLeavesExistingPolicies(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-a"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/api-keys", bytes.NewBufferString(`{"old":"sk-does-not-exist","new":"sk-brand-new"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PatchAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	p := findPolicy(h.cfg.APIKeyPolicies, "sk-a")
	if p == nil || len(p.AllowedModels) != 1 || p.AllowedModels[0] != "gpt-4o*" {
		t.Fatalf("sk-a policy mutated unexpectedly: %#v", h.cfg.APIKeyPolicies)
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-brand-new") != nil {
		t.Fatalf("sk-brand-new must not auto-create a policy")
	}
	if got := h.cfg.APIKeys; len(got) != 2 || got[1] != "sk-brand-new" {
		t.Fatalf("APIKeys after append = %#v", got)
	}
}

func TestDeleteAPIKeys_ByValueRemovesPolicy(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-a", "sk-b"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
					{Key: "sk-b", AllowedModels: []string{"claude-3-*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/api-keys?value=sk-a", nil)

	h.DeleteAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := h.cfg.APIKeys; len(got) != 1 || got[0] != "sk-b" {
		t.Fatalf("APIKeys after delete = %#v, want [sk-b]", got)
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-a") != nil {
		t.Fatalf("policy for deleted sk-a must be removed")
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-b") == nil {
		t.Fatalf("policy for sk-b must survive")
	}
	if !h.cfg.IsModelAllowedForKey("sk-a", "gpt-4o") {
		t.Fatalf("after delete, sk-a must fall back to default allow-all")
	}
}

func TestDeleteAPIKeys_ByIndexRemovesPolicy(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-a", "sk-b"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-a", AllowedModels: []string{"gpt-4o*"}},
					{Key: "sk-b", AllowedModels: []string{"claude-3-*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/api-keys?index=1", nil)

	h.DeleteAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := h.cfg.APIKeys; len(got) != 1 || got[0] != "sk-a" {
		t.Fatalf("APIKeys after delete = %#v, want [sk-a]", got)
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-b") != nil {
		t.Fatalf("policy for deleted sk-b must be removed")
	}
}

func TestDeleteAPIKeys_LastPolicyClearsSlice(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-only"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-only", AllowedModels: []string{"gpt-4o*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/api-keys?value=sk-only", nil)

	h.DeleteAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if h.cfg.APIKeyPolicies != nil {
		t.Fatalf("after removing the last policy row the slice should be nil, got %#v", h.cfg.APIKeyPolicies)
	}
}

func TestPatchAPIKeys_RenameViaOldNewRenamesAllDuplicates(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-dup", "sk-other", "sk-dup"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-dup", AllowedModels: []string{"gpt-4o*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/api-keys", bytes.NewBufferString(`{"old":"sk-dup","new":"sk-new"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PatchAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	wantKeys := []string{"sk-new", "sk-other", "sk-new"}
	if got := h.cfg.APIKeys; len(got) != len(wantKeys) {
		t.Fatalf("APIKeys after rename = %#v, want %#v", got, wantKeys)
	}
	for i := range wantKeys {
		if h.cfg.APIKeys[i] != wantKeys[i] {
			t.Fatalf("APIKeys[%d] = %q, want %q (full=%#v)", i, h.cfg.APIKeys[i], wantKeys[i], h.cfg.APIKeys)
		}
	}
	if !h.cfg.IsModelAllowedForKey("sk-new", "gpt-4o-mini") {
		t.Fatalf("sk-new must allow gpt-4o-mini via migrated policy")
	}
	if h.cfg.IsModelAllowedForKey("sk-new", "claude-3-5-sonnet-20241022") {
		t.Fatalf("sk-new must still reject models outside its allowlist")
	}
}

func TestPatchAPIKeys_IndexedReplaceLeavesPolicyWhenDuplicateRemains(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-dup", "sk-dup"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-dup", AllowedModels: []string{"gpt-4o*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v0/management/api-keys", bytes.NewBufferString(`{"index":0,"value":"sk-new"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.PatchAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-dup") == nil {
		t.Fatalf("policy for surviving sk-dup duplicate must remain")
	}
	if !h.cfg.IsModelAllowedForKey("sk-dup", "gpt-4o-mini") {
		t.Fatalf("surviving sk-dup must still allow gpt-4o-mini")
	}
	if h.cfg.IsModelAllowedForKey("sk-dup", "claude-3-5-sonnet-20241022") {
		t.Fatalf("surviving sk-dup must still reject claude")
	}
}

func TestDeleteAPIKeys_ByIndexKeepsPolicyWhenDuplicateRemains(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{
		cfg: &config.Config{
			SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-dup", "sk-dup"},
				APIKeyPolicies: []config.APIKeyPolicy{
					{Key: "sk-dup", AllowedModels: []string{"gpt-4o*"}},
				},
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/api-keys?index=0", nil)

	h.DeleteAPIKeys(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := h.cfg.APIKeys; len(got) != 1 || got[0] != "sk-dup" {
		t.Fatalf("APIKeys after indexed delete = %#v, want [sk-dup]", got)
	}
	if findPolicy(h.cfg.APIKeyPolicies, "sk-dup") == nil {
		t.Fatalf("policy for surviving sk-dup must remain")
	}
	if !h.cfg.IsModelAllowedForKey("sk-dup", "gpt-4o-mini") {
		t.Fatalf("surviving sk-dup must still allow gpt-4o-mini")
	}
	if h.cfg.IsModelAllowedForKey("sk-dup", "claude-3-5-sonnet-20241022") {
		t.Fatalf("surviving sk-dup must still reject claude")
	}
}
