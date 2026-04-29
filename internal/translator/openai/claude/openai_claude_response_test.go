package claude

import (
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

type nonStreamOpenAIToClaudeConverter struct {
	name    string
	convert func(t *testing.T, rawJSON []byte) []byte
}

func TestConvertOpenAIResponseToClaude_StreamUsageKeepsPromptTokensAndCachedReuse(t *testing.T) {
	t.Parallel()

	rawChunk := []byte(`data: {"id":"chatcmpl-stream","model":"gpt-4.1","created":1,"choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":"stop"}],"usage":{"prompt_tokens":150000,"completion_tokens":42,"prompt_tokens_details":{"cached_tokens":149900}}}`)

	var param any
	out := ConvertOpenAIResponseToClaude(
		context.Background(),
		"",
		[]byte(`{"stream":true}`),
		nil,
		rawChunk,
		&param,
	)

	messageDeltaPayload := mustFindSSEEventPayload(t, out, "message_delta")
	assertClaudeUsage(t, messageDeltaPayload, 150000, 42, 149900, true)
}

func TestConvertOpenAIResponseToClaude_NonStreamUsageKeepsPromptTokensAcrossEntryPoints(t *testing.T) {
	t.Parallel()

	rawJSON := []byte(`{"id":"chatcmpl-nonstream","model":"gpt-4.1","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1200,"completion_tokens":37,"prompt_tokens_details":{"cached_tokens":1180}}}`)

	for _, converter := range nonStreamOpenAIToClaudeConverters() {
		t.Run(converter.name, func(t *testing.T) {
			out := converter.convert(t, rawJSON)
			assertClaudeUsage(t, out, 1200, 37, 1180, true)
		})
	}
}

func TestConvertOpenAIResponseToClaude_NonStreamUsageDoesNotSynthesizeCacheReadTokensAcrossEntryPoints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		rawJSON []byte
	}{
		{
			name:    "missing cached token details",
			rawJSON: []byte(`{"id":"chatcmpl-missing-cache","model":"gpt-4.1","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":800,"completion_tokens":11}}`),
		},
		{
			name:    "zero cached tokens",
			rawJSON: []byte(`{"id":"chatcmpl-zero-cache","model":"gpt-4.1","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":800,"completion_tokens":11,"prompt_tokens_details":{"cached_tokens":0}}}`),
		},
	}

	for _, converter := range nonStreamOpenAIToClaudeConverters() {
		for _, tc := range testCases {
			t.Run(converter.name+"/"+tc.name, func(t *testing.T) {
				out := converter.convert(t, tc.rawJSON)
				assertClaudeUsage(t, out, 800, 11, 0, false)
			})
		}
	}
}

func TestConvertOpenAIResponseToClaude_NonStreamUsageDoesNotClampPromptTokensAcrossEntryPoints(t *testing.T) {
	t.Parallel()

	rawJSON := []byte(`{"id":"chatcmpl-clamp-edge","model":"gpt-4.1","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":9,"prompt_tokens_details":{"cached_tokens":150}}}`)

	for _, converter := range nonStreamOpenAIToClaudeConverters() {
		t.Run(converter.name, func(t *testing.T) {
			out := converter.convert(t, rawJSON)
			assertClaudeUsage(t, out, 100, 9, 150, true)
		})
	}
}

func nonStreamOpenAIToClaudeConverters() []nonStreamOpenAIToClaudeConverter {
	return []nonStreamOpenAIToClaudeConverter{
		{
			name: "ConvertOpenAIResponseToClaude stream=false wrapper",
			convert: func(t *testing.T, rawJSON []byte) []byte {
				t.Helper()

				var param any
				out := ConvertOpenAIResponseToClaude(
					context.Background(),
					"",
					[]byte(`{"stream":false}`),
					nil,
					append([]byte("data: "), rawJSON...),
					&param,
				)
				if len(out) != 1 {
					t.Fatalf("expected 1 non-stream wrapper chunk, got %d", len(out))
				}
				return out[0]
			},
		},
		{
			name: "ConvertOpenAIResponseToClaudeNonStream",
			convert: func(t *testing.T, rawJSON []byte) []byte {
				t.Helper()
				return ConvertOpenAIResponseToClaudeNonStream(context.Background(), "", nil, nil, rawJSON, nil)
			},
		},
	}
}

func mustFindSSEEventPayload(t *testing.T, chunks [][]byte, event string) []byte {
	t.Helper()

	prefix := "event: " + event + "\ndata: "
	for _, chunk := range chunks {
		chunkText := string(chunk)
		if strings.HasPrefix(chunkText, prefix) {
			return []byte(strings.TrimSpace(strings.TrimPrefix(chunkText, prefix)))
		}
	}

	t.Fatalf("event %q not found in %d chunks", event, len(chunks))
	return nil
}

func assertClaudeUsage(t *testing.T, payload []byte, wantInputTokens, wantOutputTokens, wantCachedTokens int64, wantCachedField bool) {
	t.Helper()

	if got := gjson.GetBytes(payload, "usage.input_tokens").Int(); got != wantInputTokens {
		t.Fatalf("usage.input_tokens = %d, want %d", got, wantInputTokens)
	}
	if got := gjson.GetBytes(payload, "usage.output_tokens").Int(); got != wantOutputTokens {
		t.Fatalf("usage.output_tokens = %d, want %d", got, wantOutputTokens)
	}

	cachedTokens := gjson.GetBytes(payload, "usage.cache_read_input_tokens")
	if cachedTokens.Exists() != wantCachedField {
		t.Fatalf("usage.cache_read_input_tokens existence = %v, want %v", cachedTokens.Exists(), wantCachedField)
	}
	if wantCachedField && cachedTokens.Int() != wantCachedTokens {
		t.Fatalf("usage.cache_read_input_tokens = %d, want %d", cachedTokens.Int(), wantCachedTokens)
	}
}
