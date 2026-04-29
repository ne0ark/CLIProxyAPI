package registry

import "testing"

func TestMergeEmbeddedAdditions_ReappendsEmbeddedOpus47WhenRemoteOmitsIt(t *testing.T) {
	remote := &staticModelsJSON{
		Claude: []*ModelInfo{
			{
				ID:          "claude-3-5-sonnet-20241022",
				DisplayName: "Claude 3.5 Sonnet",
			},
		},
	}

	merged := mergeEmbeddedAdditions(remote)
	model := findModelInfo(merged.Claude, "claude-opus-4-7")
	if model == nil {
		t.Fatal("expected merged catalog to re-append embedded claude-opus-4-7")
	}
	assertClaudeOpus47ModelInfo(t, "merged catalog", model)
}

func TestMergeEmbeddedAdditions_PreservesRemoteOpus47WhenDuplicateExists(t *testing.T) {
	remote := &staticModelsJSON{
		Claude: []*ModelInfo{
			{
				ID:                  "claude-opus-4-7",
				DisplayName:         "Remote Opus 4.7",
				Description:         "Remote definition should win",
				ContextLength:       321,
				MaxCompletionTokens: 654,
				Thinking: &ThinkingSupport{
					Levels: []string{"medium"},
				},
			},
		},
	}

	merged := mergeEmbeddedAdditions(remote)
	var matches int
	for _, candidate := range merged.Claude {
		if candidate != nil && candidate.ID == "claude-opus-4-7" {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("claude-opus-4-7 duplicate count = %d, want 1", matches)
	}

	model := findModelInfo(merged.Claude, "claude-opus-4-7")
	if model == nil {
		t.Fatal("expected merged catalog to keep remote claude-opus-4-7")
	}
	if model.DisplayName != "Remote Opus 4.7" {
		t.Fatalf("display name mismatch: got %q", model.DisplayName)
	}
	if model.Description != "Remote definition should win" {
		t.Fatalf("description mismatch: got %q", model.Description)
	}
	if model.ContextLength != 321 {
		t.Fatalf("context length mismatch: got %d", model.ContextLength)
	}
	if model.MaxCompletionTokens != 654 {
		t.Fatalf("max completion tokens mismatch: got %d", model.MaxCompletionTokens)
	}
	if model.Thinking == nil || len(model.Thinking.Levels) != 1 || model.Thinking.Levels[0] != "medium" {
		t.Fatalf("remote thinking metadata was not preserved: %#v", model.Thinking)
	}
}

func TestMergeEmbeddedAdditions_DoesNotRestoreNonFallbackClaudeModels(t *testing.T) {
	remote := &staticModelsJSON{
		Claude: []*ModelInfo{
			{
				ID:          "claude-3-5-sonnet-20241022",
				DisplayName: "Claude 3.5 Sonnet",
			},
		},
	}

	merged := mergeEmbeddedAdditions(remote)
	if model := findModelInfo(merged.Claude, "claude-opus-4-6"); model != nil {
		t.Fatalf("expected non-fallback embedded model to stay absent, got %q", model.ID)
	}
}
