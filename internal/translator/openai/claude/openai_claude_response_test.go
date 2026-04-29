package claude

import (
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func parseClaudeSSEEvent(t *testing.T, chunk []byte) (string, gjson.Result) {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(string(chunk)), "\n")
	if len(lines) < 2 {
		t.Fatalf("unexpected SSE chunk: %q", chunk)
	}

	event := strings.TrimSpace(strings.TrimPrefix(lines[0], "event:"))
	dataLine := strings.TrimSpace(strings.TrimPrefix(lines[1], "data:"))
	if !gjson.Valid(dataLine) {
		t.Fatalf("invalid SSE data JSON: %q", dataLine)
	}

	return event, gjson.Parse(dataLine)
}

func streamClaudeUsageDelta(t *testing.T, usageJSON string) gjson.Result {
	t.Helper()

	request := []byte(`{"stream":true}`)
	var param any

	finishReasonChunk := []byte(`data: {"choices":[{"index":0,"finish_reason":"stop"}]}`)
	ConvertOpenAIResponseToClaude(context.Background(), "gpt-test", request, request, finishReasonChunk, &param)

	usageChunk := []byte(`data: {"usage":` + usageJSON + `}`)
	chunks := ConvertOpenAIResponseToClaude(context.Background(), "gpt-test", request, request, usageChunk, &param)

	for _, chunk := range chunks {
		event, data := parseClaudeSSEEvent(t, chunk)
		if event == "message_delta" {
			return data
		}
	}

	t.Fatalf("expected message_delta chunk in %q", chunks)
	return gjson.Result{}
}

func assertClaudeUsage(t *testing.T, usage gjson.Result, wantInputTokens, wantOutputTokens int64, wantCachedTokens *int64) {
	t.Helper()

	if got := usage.Get("input_tokens").Int(); got != wantInputTokens {
		t.Fatalf("unexpected usage.input_tokens: got %d want %d", got, wantInputTokens)
	}
	if got := usage.Get("output_tokens").Int(); got != wantOutputTokens {
		t.Fatalf("unexpected usage.output_tokens: got %d want %d", got, wantOutputTokens)
	}

	cachedUsage := usage.Get("cache_read_input_tokens")
	if wantCachedTokens == nil {
		if cachedUsage.Exists() {
			t.Fatalf("expected cache_read_input_tokens to be omitted, got %s", cachedUsage.Raw)
		}
		return
	}

	if !cachedUsage.Exists() {
		t.Fatalf("expected cache_read_input_tokens %d, but field was omitted", *wantCachedTokens)
	}
	if got := cachedUsage.Int(); got != *wantCachedTokens {
		t.Fatalf("unexpected cache_read_input_tokens: got %d want %d", got, *wantCachedTokens)
	}
}

func TestConvertOpenAIResponseToClaude_StreamUsageAccounting(t *testing.T) {
	tests := []struct {
		name             string
		usageJSON        string
		wantInputTokens  int64
		wantOutputTokens int64
		wantCachedTokens *int64
	}{
		{
			name:             "preserves prompt tokens with cached details",
			usageJSON:        `{"prompt_tokens":1200,"completion_tokens":34,"prompt_tokens_details":{"cached_tokens":1100}}`,
			wantInputTokens:  1200,
			wantOutputTokens: 34,
			wantCachedTokens: int64Ptr(1100),
		},
		{
			name:             "keeps prompt tokens when cached tokens exceed prompt tokens",
			usageJSON:        `{"prompt_tokens":25,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":250}}`,
			wantInputTokens:  25,
			wantOutputTokens: 4,
			wantCachedTokens: int64Ptr(250),
		},
		{
			name:             "omits cache_read_input_tokens when cached details are zero",
			usageJSON:        `{"prompt_tokens":87,"completion_tokens":9,"prompt_tokens_details":{"cached_tokens":0}}`,
			wantInputTokens:  87,
			wantOutputTokens: 9,
			wantCachedTokens: nil,
		},
		{
			name:             "omits cache_read_input_tokens when cached details are missing",
			usageJSON:        `{"prompt_tokens":64,"completion_tokens":8}`,
			wantInputTokens:  64,
			wantOutputTokens: 8,
			wantCachedTokens: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := streamClaudeUsageDelta(t, tt.usageJSON)
			assertClaudeUsage(t, data.Get("usage"), tt.wantInputTokens, tt.wantOutputTokens, tt.wantCachedTokens)
		})
	}
}

func TestConvertOpenAIResponseToClaudeNonStream_UsageAccounting(t *testing.T) {
	tests := []struct {
		name             string
		rawJSON          string
		wantInputTokens  int64
		wantOutputTokens int64
		wantCachedTokens *int64
	}{
		{
			name: "preserves prompt tokens with cached details",
			rawJSON: `{
				"id":"resp_cached",
				"model":"gpt-test",
				"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":980,"completion_tokens":21,"prompt_tokens_details":{"cached_tokens":640}}
			}`,
			wantInputTokens:  980,
			wantOutputTokens: 21,
			wantCachedTokens: int64Ptr(640),
		},
		{
			name: "keeps prompt tokens when cached tokens exceed prompt tokens",
			rawJSON: `{
				"id":"resp_clamp_edge",
				"model":"gpt-test",
				"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":13,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":130}}
			}`,
			wantInputTokens:  13,
			wantOutputTokens: 2,
			wantCachedTokens: int64Ptr(130),
		},
		{
			name: "omits cache_read_input_tokens when cached details are zero",
			rawJSON: `{
				"id":"resp_zero_cache",
				"model":"gpt-test",
				"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":77,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":0}}
			}`,
			wantInputTokens:  77,
			wantOutputTokens: 5,
			wantCachedTokens: nil,
		},
		{
			name: "omits cache_read_input_tokens when cached details are missing",
			rawJSON: `{
				"id":"resp_missing_cache",
				"model":"gpt-test",
				"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":55,"completion_tokens":6}
			}`,
			wantInputTokens:  55,
			wantOutputTokens: 6,
			wantCachedTokens: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ConvertOpenAIResponseToClaudeNonStream(context.Background(), "gpt-test", nil, nil, []byte(tt.rawJSON), nil)
			assertClaudeUsage(t, gjson.GetBytes(out, "usage"), tt.wantInputTokens, tt.wantOutputTokens, tt.wantCachedTokens)
		})
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
