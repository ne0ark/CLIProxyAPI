package registry

import "testing"

func TestCodexStaticModelsIncludeGPT55(t *testing.T) {
	t.Run("free", func(t *testing.T) {
		model := findModelInfo(GetCodexFreeModels(), "gpt-5.5")
		if model != nil {
			t.Fatalf("expected free source to omit gpt-5.5")
		}
	})

	modelSources := map[string]func() *ModelInfo{
		"team": func() *ModelInfo {
			return findModelInfo(GetCodexTeamModels(), "gpt-5.5")
		},
		"plus": func() *ModelInfo {
			return findModelInfo(GetCodexPlusModels(), "gpt-5.5")
		},
		"pro": func() *ModelInfo {
			return findModelInfo(GetCodexProModels(), "gpt-5.5")
		},
		"lookup": func() *ModelInfo {
			return LookupStaticModelInfo("gpt-5.5")
		},
	}

	for _, source := range []string{"team", "plus", "pro", "lookup"} {
		source := source
		t.Run(source, func(t *testing.T) {
			model := modelSources[source]()
			if model == nil {
				t.Fatalf("expected %s source to include gpt-5.5", source)
			}
			assertGPT55ModelInfo(t, source, model)
		})
	}
}

func TestCodexStaticModelsIncludeCodexAutoReview(t *testing.T) {
	modelSources := map[string]func() *ModelInfo{
		"free": func() *ModelInfo {
			return findModelInfo(GetCodexFreeModels(), "codex-auto-review")
		},
		"team": func() *ModelInfo {
			return findModelInfo(GetCodexTeamModels(), "codex-auto-review")
		},
		"plus": func() *ModelInfo {
			return findModelInfo(GetCodexPlusModels(), "codex-auto-review")
		},
		"pro": func() *ModelInfo {
			return findModelInfo(GetCodexProModels(), "codex-auto-review")
		},
		"lookup": func() *ModelInfo {
			return LookupStaticModelInfo("codex-auto-review")
		},
	}

	for _, source := range []string{"free", "team", "plus", "pro", "lookup"} {
		source := source
		t.Run(source, func(t *testing.T) {
			model := modelSources[source]()
			if model == nil {
				t.Fatalf("expected %s source to include codex-auto-review", source)
			}
			assertCodexAutoReviewModelInfo(t, source, model)
		})
	}
}

func findModelInfo(models []*ModelInfo, id string) *ModelInfo {
	for _, model := range models {
		if model != nil && model.ID == id {
			return model
		}
	}
	return nil
}

func assertGPT55ModelInfo(t *testing.T, source string, model *ModelInfo) {
	t.Helper()

	if model.ID != "gpt-5.5" {
		t.Fatalf("%s id mismatch: got %q", source, model.ID)
	}
	if model.Object != "model" {
		t.Fatalf("%s object mismatch: got %q", source, model.Object)
	}
	if model.Created != 1776902400 {
		t.Fatalf("%s created timestamp mismatch: got %d", source, model.Created)
	}
	if model.OwnedBy != "openai" {
		t.Fatalf("%s owned_by mismatch: got %q", source, model.OwnedBy)
	}
	if model.Type != "openai" {
		t.Fatalf("%s type mismatch: got %q", source, model.Type)
	}
	if model.DisplayName != "GPT 5.5" {
		t.Fatalf("%s display name mismatch: got %q", source, model.DisplayName)
	}
	if model.Version != "gpt-5.5" {
		t.Fatalf("%s version mismatch: got %q", source, model.Version)
	}
	if model.Description != "Frontier model for complex coding, research, and real-world work." {
		t.Fatalf("%s description mismatch: got %q", source, model.Description)
	}
	if model.ContextLength != 272000 {
		t.Fatalf("%s context length mismatch: got %d", source, model.ContextLength)
	}
	if model.MaxCompletionTokens != 128000 {
		t.Fatalf("%s max completion tokens mismatch: got %d", source, model.MaxCompletionTokens)
	}
	if len(model.SupportedParameters) != 1 || model.SupportedParameters[0] != "tools" {
		t.Fatalf("%s supported parameters mismatch: got %v", source, model.SupportedParameters)
	}
	if model.Thinking == nil {
		t.Fatalf("%s missing thinking support", source)
	}

	wantLevels := []string{"low", "medium", "high", "xhigh"}
	if len(model.Thinking.Levels) != len(wantLevels) {
		t.Fatalf("%s thinking level count mismatch: got %d, want %d", source, len(model.Thinking.Levels), len(wantLevels))
	}
	for i, level := range wantLevels {
		if model.Thinking.Levels[i] != level {
			t.Fatalf("%s thinking level %d mismatch: got %q, want %q", source, i, model.Thinking.Levels[i], level)
		}
	}
}

func assertCodexAutoReviewModelInfo(t *testing.T, source string, model *ModelInfo) {
	t.Helper()

	if model.ID != "codex-auto-review" {
		t.Fatalf("%s id mismatch: got %q", source, model.ID)
	}
	if model.Object != "model" {
		t.Fatalf("%s object mismatch: got %q", source, model.Object)
	}
	if model.Created != 1776902400 {
		t.Fatalf("%s created timestamp mismatch: got %d", source, model.Created)
	}
	if model.OwnedBy != "openai" {
		t.Fatalf("%s owned_by mismatch: got %q", source, model.OwnedBy)
	}
	if model.Type != "openai" {
		t.Fatalf("%s type mismatch: got %q", source, model.Type)
	}
	if model.DisplayName != "Codex Auto Review" {
		t.Fatalf("%s display name mismatch: got %q", source, model.DisplayName)
	}
	if model.Version != "Codex Auto Review" {
		t.Fatalf("%s version mismatch: got %q", source, model.Version)
	}
	if model.Description != "Automatic approval review model for Codex." {
		t.Fatalf("%s description mismatch: got %q", source, model.Description)
	}
	if model.ContextLength != 272000 {
		t.Fatalf("%s context length mismatch: got %d", source, model.ContextLength)
	}
	if model.MaxCompletionTokens != 128000 {
		t.Fatalf("%s max completion tokens mismatch: got %d", source, model.MaxCompletionTokens)
	}
	if len(model.SupportedParameters) != 1 || model.SupportedParameters[0] != "tools" {
		t.Fatalf("%s supported parameters mismatch: got %v", source, model.SupportedParameters)
	}
	if model.Thinking == nil {
		t.Fatalf("%s missing thinking support", source)
	}

	wantLevels := []string{"low", "medium", "high", "xhigh"}
	if len(model.Thinking.Levels) != len(wantLevels) {
		t.Fatalf("%s thinking level count mismatch: got %d, want %d", source, len(model.Thinking.Levels), len(wantLevels))
	}
	for i, level := range wantLevels {
		if model.Thinking.Levels[i] != level {
			t.Fatalf("%s thinking level %d mismatch: got %q, want %q", source, i, model.Thinking.Levels[i], level)
		}
	}
}
