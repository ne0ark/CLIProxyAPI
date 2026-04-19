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
		{"claude-opus-4-7", true},
		{"claude-opus-4-8", true},
		{"claude-opus-4-10", true},
		{"claude-opus-4-7-20260416", true},
		{"CLAUDE-OPUS-4-7", true},
		{"  claude-opus-4-7  ", true},
		{"claude-opus-4-6", false},
		{"claude-opus-4-5", false},
		{"claude-opus-4-1-20250805", false},
		{"claude-opus-4-20250514", false},
		{"claude-sonnet-4-7", false},
		{"claude-opus-5-0", false},
		{"claude-opus-4-", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isOpus47OrLater(tc.model); got != tc.want {
			t.Errorf("isOpus47OrLater(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestStripSamplingParamsForOpus47(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-7","temperature":0.7,"top_p":0.95,"top_k":40,"messages":[]}`)

	// 4.7+ strips all sampling params.
	out := stripSamplingParamsForOpus47(body, "claude-opus-4-7")
	if gjson.GetBytes(out, "temperature").Exists() {
		t.Errorf("temperature not stripped: %s", out)
	}
	if gjson.GetBytes(out, "top_p").Exists() {
		t.Errorf("top_p not stripped: %s", out)
	}
	if gjson.GetBytes(out, "top_k").Exists() {
		t.Errorf("top_k not stripped: %s", out)
	}

	// 4.6 and earlier pass through unchanged.
	out46 := stripSamplingParamsForOpus47(body, "claude-opus-4-6")
	if got := gjson.GetBytes(out46, "temperature").Float(); got != 0.7 {
		t.Errorf("temperature stripped for 4.6: %s", out46)
	}
	if !gjson.GetBytes(out46, "top_p").Exists() {
		t.Errorf("top_p stripped for 4.6: %s", out46)
	}
}

func TestEnsureTaskBudgetsBeta(t *testing.T) {
	// Body without task_budget: no beta added.
	body := []byte(`{"model":"claude-opus-4-7","messages":[]}`)
	got := ensureTaskBudgetsBeta([]string{"existing-beta"}, body)
	if len(got) != 1 || got[0] != "existing-beta" {
		t.Errorf("no task_budget: got %v, want [existing-beta]", got)
	}

	// Body with task_budget: beta appended.
	bodyWithBudget := []byte(`{"model":"claude-opus-4-7","output_config":{"task_budget":{"total_tokens":2048}},"messages":[]}`)
	got = ensureTaskBudgetsBeta([]string{"existing-beta"}, bodyWithBudget)
	if len(got) != 2 || got[1] != taskBudgetsBeta {
		t.Errorf("with task_budget: got %v, want [existing-beta %s]", got, taskBudgetsBeta)
	}

	// Idempotent: calling again doesn't add a duplicate.
	got2 := ensureTaskBudgetsBeta(got, bodyWithBudget)
	if len(got2) != 2 {
		t.Errorf("idempotency broken: got %v, want len 2", got2)
	}

	// Case-insensitive de-dupe.
	got3 := ensureTaskBudgetsBeta([]string{"TASK-BUDGETS-2026-03-13"}, bodyWithBudget)
	if len(got3) != 1 {
		t.Errorf("case-insensitive de-dupe failed: got %v", got3)
	}
}
