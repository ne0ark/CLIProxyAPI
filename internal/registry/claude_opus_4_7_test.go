package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClaudeStaticModelsKeepCurrentMainOpus47Metadata(t *testing.T) {
	model := findModelInfo(GetClaudeModels(), "claude-opus-4-7")
	assertCurrentMainClaudeOpus47Metadata(t, model)
}

func TestTryRefreshModels_PreservesCurrentMainOpus47Continuity(t *testing.T) {
	originalURLs := append([]string(nil), modelsURLs...)
	t.Cleanup(func() {
		modelsURLs = originalURLs
		if err := loadModelsFromBytes(embeddedModelsJSON, "test cleanup"); err != nil {
			t.Fatalf("restore embedded model catalog: %v", err)
		}
	})

	for _, testCase := range []struct {
		name   string
		mutate func(*staticModelsJSON)
	}{
		{
			name: "remote omits opus 4.7",
			mutate: func(remote *staticModelsJSON) {
				filtered := make([]*ModelInfo, 0, len(remote.Claude))
				for _, model := range remote.Claude {
					if model != nil && model.ID == "claude-opus-4-7" {
						continue
					}
					filtered = append(filtered, model)
				}
				remote.Claude = filtered
			},
		},
		{
			name: "remote lags opus 4.7 metadata",
			mutate: func(remote *staticModelsJSON) {
				for _, model := range remote.Claude {
					if model == nil || model.ID != "claude-opus-4-7" {
						continue
					}
					model.DisplayName = "Claude Opus 4.7 (stale)"
					model.ContextLength = 200000
					model.MaxCompletionTokens = 64000
					model.Thinking = &ThinkingSupport{
						Min:         1024,
						Max:         64000,
						ZeroAllowed: true,
						Levels:      []string{"low", "medium", "high"},
					}
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			remote := parseStaticModelsCatalog(t, embeddedModelsJSON)
			testCase.mutate(remote)

			payload, err := json.Marshal(remote)
			if err != nil {
				t.Fatalf("marshal remote catalog: %v", err)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(payload)
			}))
			defer server.Close()

			modelsURLs = []string{server.URL}
			if err := loadModelsFromBytes(embeddedModelsJSON, "test reset"); err != nil {
				t.Fatalf("reset embedded model catalog: %v", err)
			}

			tryRefreshModels(context.Background(), "test refresh")

			model := findModelInfo(GetClaudeModels(), "claude-opus-4-7")
			assertCurrentMainClaudeOpus47Metadata(t, model)
		})
	}
}

func assertCurrentMainClaudeOpus47Metadata(t *testing.T, model *ModelInfo) {
	t.Helper()

	if model == nil {
		t.Fatal("expected claude-opus-4-7 model metadata")
	}
	if model.OwnedBy != "anthropic" {
		t.Fatalf("owned_by = %q, want anthropic", model.OwnedBy)
	}
	if model.Type != "claude" {
		t.Fatalf("type = %q, want claude", model.Type)
	}
	if model.DisplayName != "Claude Opus 4.7" {
		t.Fatalf("display_name = %q, want %q", model.DisplayName, "Claude Opus 4.7")
	}
	if model.ContextLength != 1000000 {
		t.Fatalf("context_length = %d, want 1000000", model.ContextLength)
	}
	if model.MaxCompletionTokens != 128000 {
		t.Fatalf("max_completion_tokens = %d, want 128000", model.MaxCompletionTokens)
	}
	if model.Thinking == nil {
		t.Fatal("thinking metadata = nil, want configured levels")
	}
	wantLevels := []string{"low", "medium", "high", "xhigh", "max"}
	if len(model.Thinking.Levels) != len(wantLevels) {
		t.Fatalf("thinking levels = %v, want %v", model.Thinking.Levels, wantLevels)
	}
	for index, level := range wantLevels {
		if model.Thinking.Levels[index] != level {
			t.Fatalf("thinking level %d = %q, want %q", index, model.Thinking.Levels[index], level)
		}
	}
}

func parseStaticModelsCatalog(t *testing.T, data []byte) *staticModelsJSON {
	t.Helper()

	var parsed staticModelsJSON
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse static models catalog: %v", err)
	}
	return &parsed
}
