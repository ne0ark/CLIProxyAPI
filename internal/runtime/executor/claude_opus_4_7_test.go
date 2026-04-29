package executor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestIsOpus47OrLater(t *testing.T) {
	testCases := []struct {
		model string
		want  bool
	}{
		{model: "claude-opus-4-7", want: true},
		{model: "CLAUDE-OPUS-4-7", want: true},
		{model: "  claude-opus-4-7  ", want: true},
		{model: "claude-opus-4-7-20260416", want: true},
		{model: "claude-opus-4-8", want: true},
		{model: "claude-opus-4-10", want: true},
		{model: "claude-opus-4-6", want: false},
		{model: "claude-opus-4-5", want: false},
		{model: "claude-opus-4-20250514", want: false},
		{model: "claude-opus-4-1-20250805", want: false},
		{model: "claude-opus-4-", want: false},
		{model: "claude-sonnet-4", want: false},
		{model: "claude-opus-5-0", want: false},
		{model: "", want: false},
	}

	for _, testCase := range testCases {
		if got := helps.IsOpus47OrLater(testCase.model); got != testCase.want {
			t.Fatalf("IsOpus47OrLater(%q) = %v, want %v", testCase.model, got, testCase.want)
		}
	}
}

func TestClaudeExecutor_Opus47StripsSamplingFieldsOnAllEntryPoints(t *testing.T) {
	payload := []byte(`{
		"temperature": 0.2,
		"top_p": 0.9,
		"top_k": 17,
		"messages": [{"role":"user","content":[{"type":"text","text":"hi"}]}]
	}`)

	for _, entryPoint := range []string{"Execute", "ExecuteStream", "CountTokens"} {
		t.Run(entryPoint, func(t *testing.T) {
			seenBody, _ := captureClaudeUpstreamRequestForEntryPoint(t, entryPoint, context.Background(), "claude-opus-4-7-20260416", payload)
			assertSamplingFieldsAbsent(t, seenBody)
		})
	}
}

func TestClaudeExecutor_NonOpusClaudeModelsKeepSamplingFields(t *testing.T) {
	payload := []byte(`{
		"temperature": 0.2,
		"top_p": 0.9,
		"top_k": 17,
		"messages": [{"role":"user","content":[{"type":"text","text":"hi"}]}]
	}`)

	for _, model := range []string{"claude-opus-4-6", "claude-sonnet-4"} {
		for _, entryPoint := range []string{"Execute", "ExecuteStream", "CountTokens"} {
			t.Run(model+"/"+entryPoint, func(t *testing.T) {
				seenBody, _ := captureClaudeUpstreamRequestForEntryPoint(t, entryPoint, context.Background(), model, payload)
				assertSamplingFieldsPresent(t, seenBody)
			})
		}
	}
}

func TestClaudeExecutor_PayloadOverrideToOpus47StripsSamplingFields(t *testing.T) {
	payload := []byte(`{
		"temperature": 0.2,
		"top_p": 0.9,
		"top_k": 17,
		"messages": [{"role":"user","content":[{"type":"text","text":"hi"}]}]
	}`)
	cfg := &config.Config{
		Payload: config.PayloadConfig{
			Override: []config.PayloadRule{{
				Models: []config.PayloadModelRule{{
					Name:     "claude-sonnet-4",
					Protocol: "claude",
				}},
				Params: map[string]any{
					"model": "claude-opus-4-7",
				},
			}},
		},
	}

	for _, entryPoint := range []string{"Execute", "ExecuteStream"} {
		t.Run(entryPoint, func(t *testing.T) {
			seenBody, _ := captureClaudeUpstreamRequestForEntryPointWithConfig(t, entryPoint, context.Background(), "claude-sonnet-4", payload, cfg)
			if got := gjson.GetBytes(seenBody, "model").String(); got != "claude-opus-4-7" {
				t.Fatalf("final model = %q, want claude-opus-4-7", got)
			}
			assertSamplingFieldsAbsent(t, seenBody)
		})
	}
}

func TestClaudeExecutor_PayloadOverrideAwayFromOpus47KeepsSamplingFields(t *testing.T) {
	payload := []byte(`{
		"temperature": 0.2,
		"top_p": 0.9,
		"top_k": 17,
		"messages": [{"role":"user","content":[{"type":"text","text":"hi"}]}]
	}`)
	cfg := &config.Config{
		Payload: config.PayloadConfig{
			Override: []config.PayloadRule{{
				Models: []config.PayloadModelRule{{
					Name:     "claude-opus-4-7",
					Protocol: "claude",
				}},
				Params: map[string]any{
					"model": "claude-sonnet-4",
				},
			}},
		},
	}

	for _, entryPoint := range []string{"Execute", "ExecuteStream"} {
		t.Run(entryPoint, func(t *testing.T) {
			seenBody, _ := captureClaudeUpstreamRequestForEntryPointWithConfig(t, entryPoint, context.Background(), "claude-opus-4-7", payload, cfg)
			if got := gjson.GetBytes(seenBody, "model").String(); got != "claude-sonnet-4" {
				t.Fatalf("final model = %q, want claude-sonnet-4", got)
			}
			assertSamplingFieldsPresent(t, seenBody)
		})
	}
}

func TestClaudeExecutor_TaskBudgetBetaAddedOnAllEntryPoints(t *testing.T) {
	ctx := newClaudeHeaderTestRequest(t, http.Header{
		"Anthropic-Beta": []string{"official-beta-1"},
	}).Context()

	payload := []byte(`{
		"betas": ["custom-beta-1", "TASK-BUDGETS-2026-03-13"],
		"output_config": {"task_budget": {"budget_tokens": 2048}},
		"messages": [{"role":"user","content":[{"type":"text","text":"hi"}]}]
	}`)

	for _, entryPoint := range []string{"Execute", "ExecuteStream", "CountTokens"} {
		t.Run(entryPoint, func(t *testing.T) {
			_, headers := captureClaudeUpstreamRequestForEntryPoint(t, entryPoint, ctx, "claude-opus-4-7", payload)
			betas := headers.Get("Anthropic-Beta")
			if !strings.Contains(betas, "official-beta-1") {
				t.Fatalf("Anthropic-Beta = %q, missing official-beta-1", betas)
			}
			if !strings.Contains(betas, "custom-beta-1") {
				t.Fatalf("Anthropic-Beta = %q, missing custom-beta-1", betas)
			}
			if counts := anthropicBetaCounts(betas); counts[helps.TaskBudgetsBeta] != 1 {
				t.Fatalf("Anthropic-Beta = %q, expected exactly one %q token", betas, helps.TaskBudgetsBeta)
			}
		})
	}
}

func TestClaudeExecutor_TaskBudgetBetaOmittedWhenTaskBudgetMissing(t *testing.T) {
	payload := []byte(`{
		"betas": ["custom-beta-1"],
		"messages": [{"role":"user","content":[{"type":"text","text":"hi"}]}]
	}`)

	_, headers := captureClaudeUpstreamRequestForEntryPoint(t, "Execute", context.Background(), "claude-opus-4-7", payload)
	if counts := anthropicBetaCounts(headers.Get("Anthropic-Beta")); counts[helps.TaskBudgetsBeta] != 0 {
		t.Fatalf("Anthropic-Beta = %q, want no %q token without task_budget", headers.Get("Anthropic-Beta"), helps.TaskBudgetsBeta)
	}
}

func captureClaudeUpstreamRequestForEntryPoint(t *testing.T, entryPoint string, ctx context.Context, model string, payload []byte) ([]byte, http.Header) {
	t.Helper()

	return captureClaudeUpstreamRequestForEntryPointWithConfig(t, entryPoint, ctx, model, payload, &config.Config{})
}

func captureClaudeUpstreamRequestForEntryPointWithConfig(t *testing.T, entryPoint string, ctx context.Context, model string, payload []byte, cfg *config.Config) ([]byte, http.Header) {
	t.Helper()

	if ctx == nil {
		ctx = context.Background()
	}

	var seenBody []byte
	var seenHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seenBody = bytes.Clone(body)
		seenHeaders = r.Header.Clone()

		switch {
		case strings.Contains(r.URL.Path, "count_tokens"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"input_tokens":42}`))
		case strings.Contains(r.Header.Get("Accept"), "text/event-stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","model":"claude-opus-4-7","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
		}
	}))
	defer server.Close()

	if cfg == nil {
		cfg = &config.Config{}
	}
	executor := NewClaudeExecutor(cfg)
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}
	request := cliproxyexecutor.Request{
		Model:   model,
		Payload: payload,
	}
	options := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	switch entryPoint {
	case "Execute":
		if _, err := executor.Execute(ctx, auth, request, options); err != nil {
			t.Fatalf("Execute error: %v", err)
		}
	case "ExecuteStream":
		result, err := executor.ExecuteStream(ctx, auth, request, options)
		if err != nil {
			t.Fatalf("ExecuteStream error: %v", err)
		}
		for chunk := range result.Chunks {
			if chunk.Err != nil {
				t.Fatalf("ExecuteStream chunk error: %v", chunk.Err)
			}
		}
	case "CountTokens":
		if _, err := executor.CountTokens(ctx, auth, request, options); err != nil {
			t.Fatalf("CountTokens error: %v", err)
		}
	default:
		t.Fatalf("unsupported entry point %q", entryPoint)
	}

	if len(seenBody) == 0 {
		t.Fatalf("%s did not capture an upstream request body", entryPoint)
	}
	return seenBody, seenHeaders
}

func assertSamplingFieldsAbsent(t *testing.T, payload []byte) {
	t.Helper()

	for _, path := range []string{"temperature", "top_p", "top_k"} {
		if gjson.GetBytes(payload, path).Exists() {
			t.Fatalf("%s should be absent from payload: %s", path, string(payload))
		}
	}
}

func assertSamplingFieldsPresent(t *testing.T, payload []byte) {
	t.Helper()

	if got := gjson.GetBytes(payload, "temperature").Float(); got != 0.2 {
		t.Fatalf("temperature = %v, want 0.2 in payload: %s", got, string(payload))
	}
	if got := gjson.GetBytes(payload, "top_p").Float(); got != 0.9 {
		t.Fatalf("top_p = %v, want 0.9 in payload: %s", got, string(payload))
	}
	if got := gjson.GetBytes(payload, "top_k").Int(); got != 17 {
		t.Fatalf("top_k = %d, want 17 in payload: %s", got, string(payload))
	}
}

func anthropicBetaCounts(header string) map[string]int {
	counts := make(map[string]int)
	for _, token := range strings.Split(header, ",") {
		normalized := strings.ToLower(strings.TrimSpace(token))
		if normalized == "" {
			continue
		}
		counts[normalized]++
	}
	return counts
}
