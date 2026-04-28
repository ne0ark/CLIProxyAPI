package openai

import (
	"bytes"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/tidwall/gjson"
)

func TestSSEFrameAccumulatorPreservesMultipleFrames(t *testing.T) {
	t.Parallel()

	acc := &sseFrameAccumulator{}
	frames := acc.AddChunk([]byte("event: first\ndata: {\"id\":1}\n\nevent: second\ndata: {\"id\":2}\n\n"))
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	if !bytes.Contains(frames[0], []byte(`"id":1`)) {
		t.Fatalf("first frame was corrupted: %q", frames[0])
	}
	if !bytes.Contains(frames[1], []byte(`"id":2`)) {
		t.Fatalf("second frame was corrupted: %q", frames[1])
	}
}

func TestResolveImageToolModel_ResolvesAliasToBuiltInModel(t *testing.T) {
	t.Parallel()

	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient("test-image-tool-alias", "openai", []*registry.ModelInfo{
		{ID: "image-alias", DisplayName: defaultImagesToolModel},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient("test-image-tool-alias")
	})

	if got := resolveImageToolModel("image-alias"); got != defaultImagesToolModel {
		t.Fatalf("resolveImageToolModel() = %q, want %q", got, defaultImagesToolModel)
	}
}

func TestBuildImagesResponsesRequest_DoesNotPrefixExecutionModelFromToolModel(t *testing.T) {
	t.Parallel()

	tool := []byte(`{"type":"image_generation","model":"openai/gpt-image-2"}`)
	req := buildImagesResponsesRequest("draw a cat", nil, tool)

	if got := gjson.GetBytes(req, "model").String(); got != defaultImagesMainModel {
		t.Fatalf("buildImagesResponsesRequest() model = %q, want %q", got, defaultImagesMainModel)
	}
}

func TestBuildImagesResponsesRequest_DefaultsExecutionModelWithoutToolPrefix(t *testing.T) {
	t.Parallel()

	tool := []byte(`{"type":"image_generation","model":"gpt-image-2"}`)
	req := buildImagesResponsesRequest("draw a cat", nil, tool)

	if got := gjson.GetBytes(req, "model").String(); got != defaultImagesMainModel {
		t.Fatalf("buildImagesResponsesRequest() model = %q, want %q", got, defaultImagesMainModel)
	}
}
