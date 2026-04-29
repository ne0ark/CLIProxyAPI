package executor

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestIsOpus47OrLater(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{model: "claude-opus-4-7", want: true},
		{model: "claude-opus-4-7-20260416", want: true},
		{model: "claude-opus-4-8", want: true},
		{model: "claude-opus-4-10", want: true},
		{model: "CLAUDE-OPUS-4-7", want: true},
		{model: "  claude-opus-4-7  ", want: true},
		{model: "claude-opus-4-6", want: false},
		{model: "claude-opus-4-5", want: false},
		{model: "claude-opus-4-1-20250805", want: false},
		{model: "claude-opus-4-20250514", want: false},
		{model: "claude-sonnet-4", want: false},
		{model: "claude-opus-5-0", want: false},
		{model: "claude-opus-4-", want: false},
		{model: "", want: false},
	}

	for _, tc := range cases {
		if got := isOpus47OrLater(tc.model); got != tc.want {
			t.Errorf("isOpus47OrLater(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestStripSamplingParamsForOpus47(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-7","temperature":0.7,"top_p":0.95,"top_k":40,"messages":[]}`)

	out := stripSamplingParamsForOpus47(body, "claude-opus-4-7")
	for _, path := range []string{"temperature", "top_p", "top_k"} {
		if gjson.GetBytes(out, path).Exists() {
			t.Fatalf("%s was not stripped from Opus 4.7 payload: %s", path, out)
		}
	}

	out46 := stripSamplingParamsForOpus47(body, "claude-opus-4-6")
	if got := gjson.GetBytes(out46, "temperature").Float(); got != 0.7 {
		t.Fatalf("temperature = %v for Opus 4.6 payload, want 0.7", got)
	}
	if got := gjson.GetBytes(out46, "top_p").Float(); got != 0.95 {
		t.Fatalf("top_p = %v for Opus 4.6 payload, want 0.95", got)
	}
	if got := gjson.GetBytes(out46, "top_k").Int(); got != 40 {
		t.Fatalf("top_k = %d for Opus 4.6 payload, want 40", got)
	}
}

func TestEnsureTaskBudgetsBeta(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-7","messages":[]}`)
	got := ensureTaskBudgetsBeta([]string{"existing-beta"}, body)
	if len(got) != 1 || got[0] != "existing-beta" {
		t.Fatalf("no task budget: got %v, want [existing-beta]", got)
	}

	bodyWithBudget := []byte(`{"model":"claude-opus-4-7","output_config":{"task_budget":{"total_tokens":2048}},"messages":[]}`)
	got = ensureTaskBudgetsBeta([]string{"existing-beta"}, bodyWithBudget)
	if len(got) != 2 || got[1] != taskBudgetsBeta {
		t.Fatalf("with task budget: got %v, want [existing-beta %s]", got, taskBudgetsBeta)
	}

	got = ensureTaskBudgetsBeta(got, bodyWithBudget)
	if len(got) != 2 {
		t.Fatalf("duplicate beta appended: got %v", got)
	}

	got = ensureTaskBudgetsBeta([]string{"TASK-BUDGETS-2026-03-13"}, bodyWithBudget)
	if len(got) != 1 {
		t.Fatalf("case-insensitive duplicate beta handling failed: got %v", got)
	}
}
