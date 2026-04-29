package registry

import "testing"

func TestCodexFreeModelsExcludeGPT55(t *testing.T) {
	model := findModelInfo(GetCodexFreeModels(), "gpt-5.5")
	if model != nil {
		t.Fatal("expected codex free tier to NOT include gpt-5.5")
	}
}

func TestCodexStaticModelsIncludeGPT55(t *testing.T) {
	tierModels := map[string][]*ModelInfo{
		"team": GetCodexTeamModels(),
		"plus": GetCodexPlusModels(),
		"pro":  GetCodexProModels(),
	}

	for tier, models := range tierModels {
		t.Run(tier, func(t *testing.T) {
			model := findModelInfo(models, "gpt-5.5")
			if model == nil {
				t.Fatalf("expected codex %s tier to include gpt-5.5", tier)
			}
			assertGPT55ModelInfo(t, tier, model)
		})
	}

	model := LookupStaticModelInfo("gpt-5.5")
	if model == nil {
		t.Fatal("expected LookupStaticModelInfo to find gpt-5.5")
	}
	assertGPT55ModelInfo(t, "lookup", model)
}

func TestClaudeStaticModelsIncludeOpus47CurrentMetadata(t *testing.T) {
	model := findModelInfo(GetClaudeModels(), "claude-opus-4-7")
	if model == nil {
		t.Fatal("expected claude models to include claude-opus-4-7")
	}
	assertClaudeOpus47ModelInfo(t, "claude list", model)

	lookup := LookupStaticModelInfo("claude-opus-4-7")
	if lookup == nil {
		t.Fatal("expected LookupStaticModelInfo to find claude-opus-4-7")
	}
	assertClaudeOpus47ModelInfo(t, "lookup", lookup)
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

	want := []string{"low", "medium", "high", "xhigh"}
	if len(model.Thinking.Levels) != len(want) {
		t.Fatalf("%s thinking level count mismatch: got %d, want %d", source, len(model.Thinking.Levels), len(want))
	}
	for i, level := range want {
		if model.Thinking.Levels[i] != level {
			t.Fatalf("%s thinking level %d mismatch: got %q, want %q", source, i, model.Thinking.Levels[i], level)
		}
	}
}

func assertClaudeOpus47ModelInfo(t *testing.T, source string, model *ModelInfo) {
	t.Helper()

	if model.DisplayName != "Claude Opus 4.7" {
		t.Fatalf("%s display name mismatch: got %q", source, model.DisplayName)
	}
	if model.Description != "Premium model combining maximum intelligence with practical performance" {
		t.Fatalf("%s description mismatch: got %q", source, model.Description)
	}
	if model.ContextLength != 1000000 {
		t.Fatalf("%s context length mismatch: got %d", source, model.ContextLength)
	}
	if model.MaxCompletionTokens != 128000 {
		t.Fatalf("%s max completion tokens mismatch: got %d", source, model.MaxCompletionTokens)
	}
	if model.Thinking == nil {
		t.Fatalf("%s missing thinking support", source)
	}
	if model.Thinking.Min != 1024 {
		t.Fatalf("%s thinking min mismatch: got %d", source, model.Thinking.Min)
	}
	if model.Thinking.Max != 128000 {
		t.Fatalf("%s thinking max mismatch: got %d", source, model.Thinking.Max)
	}
	if !model.Thinking.ZeroAllowed {
		t.Fatalf("%s zero_allowed mismatch: got false, want true", source)
	}

	want := []string{"low", "medium", "high", "xhigh", "max"}
	if len(model.Thinking.Levels) != len(want) {
		t.Fatalf("%s thinking level count mismatch: got %d, want %d", source, len(model.Thinking.Levels), len(want))
	}
	for i, level := range want {
		if model.Thinking.Levels[i] != level {
			t.Fatalf("%s thinking level %d mismatch: got %q, want %q", source, i, model.Thinking.Levels[i], level)
		}
	}
}
