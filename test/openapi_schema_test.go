package test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type openAPIDocument struct {
	OpenAPI    string                    `yaml:"openapi"`
	Info       openAPIInfo               `yaml:"info"`
	Paths      map[string]map[string]any `yaml:"paths"`
	Components openAPIComponents         `yaml:"components"`
	Tags       []map[string]string       `yaml:"tags"`
	Servers    []map[string]string       `yaml:"servers"`
}

type openAPIInfo struct {
	Title   string `yaml:"title"`
	Version string `yaml:"version"`
}

type openAPIComponents struct {
	SecuritySchemes map[string]map[string]any `yaml:"securitySchemes"`
	Schemas         map[string]map[string]any `yaml:"schemas"`
}

func TestOpenAPISchema(t *testing.T) {
	root := openAPIRepoRoot(t)
	schemaPath := filepath.Join(root, "openapi.yaml")

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var doc openAPIDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	if !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("unexpected OpenAPI version %q", doc.OpenAPI)
	}
	if strings.TrimSpace(doc.Info.Title) == "" {
		t.Fatal("openapi.yaml is missing info.title")
	}
	if strings.TrimSpace(doc.Info.Version) == "" {
		t.Fatal("openapi.yaml is missing info.version")
	}
	if len(doc.Servers) == 0 {
		t.Fatal("openapi.yaml should declare at least one server")
	}
	if len(doc.Tags) < 4 {
		t.Fatalf("expected schema tags for the main API areas, got %d", len(doc.Tags))
	}
	if len(doc.Paths) < 16 {
		t.Fatalf("expected a substantive schema with many paths, got %d", len(doc.Paths))
	}

	requiredPaths := map[string][]string{
		"/healthz":                                   {"get"},
		"/":                                          {"get"},
		"/v1/models":                                 {"get"},
		"/v1/chat/completions":                       {"post"},
		"/v1/completions":                            {"post"},
		"/v1/messages":                               {"post"},
		"/v1/messages/count_tokens":                  {"post"},
		"/v1/responses":                              {"get", "post"},
		"/v1/responses/compact":                      {"post"},
		"/v1beta/models":                             {"get"},
		"/v1beta/models/{action}":                    {"get", "post"},
		"/v0/management/config":                      {"get"},
		"/v0/management/config.yaml":                 {"get", "put"},
		"/v0/management/usage":                       {"get"},
		"/v0/management/auth-files":                  {"get", "post", "delete"},
		"/v0/management/auth-files/models":           {"get"},
		"/v0/management/auth-files/status":           {"patch"},
		"/v0/management/auth-files/fields":           {"patch"},
		"/v0/management/model-definitions/{channel}": {"get"},
		"/v0/management/vertex/import":               {"post"},
		"/v0/management/anthropic-auth-url":          {"get"},
		"/v0/management/codex-auth-url":              {"get"},
		"/v0/management/gemini-cli-auth-url":         {"get"},
		"/v0/management/antigravity-auth-url":        {"get"},
		"/v0/management/kimi-auth-url":               {"get"},
		"/v0/management/oauth-callback":              {"post"},
		"/v0/management/get-auth-status":             {"get"},
	}

	for path, methods := range requiredPaths {
		item, ok := doc.Paths[path]
		if !ok {
			t.Fatalf("openapi.yaml is missing path %q", path)
		}
		for _, method := range methods {
			raw, ok := item[method]
			if !ok {
				t.Fatalf("openapi.yaml path %q is missing method %q", path, method)
			}
			operation, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("openapi.yaml path %q method %q did not decode as an operation object", path, method)
			}
			if strings.TrimSpace(asString(operation["summary"])) == "" {
				t.Fatalf("openapi.yaml path %q method %q is missing a summary", path, method)
			}
			if _, ok := operation["responses"]; !ok {
				t.Fatalf("openapi.yaml path %q method %q is missing responses", path, method)
			}
		}
	}

	requiredSchemes := []string{"accessBearer", "managementKey", "managementBearer"}
	for _, scheme := range requiredSchemes {
		if _, ok := doc.Components.SecuritySchemes[scheme]; !ok {
			t.Fatalf("openapi.yaml is missing security scheme %q", scheme)
		}
	}

	requiredSchemas := []string{"ErrorResponse", "OpenAIModelList", "ChatCompletionsRequest", "AuthFileListResponse", "OAuthBootstrapResponse"}
	for _, schema := range requiredSchemas {
		if _, ok := doc.Components.Schemas[schema]; !ok {
			t.Fatalf("openapi.yaml is missing component schema %q", schema)
		}
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func openAPIRepoRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file path")
	}

	return filepath.Dir(filepath.Dir(currentFile))
}
