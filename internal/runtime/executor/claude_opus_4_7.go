package executor

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// taskBudgetsBeta is the Anthropic beta token required for
// output_config.task_budget support on Claude Opus 4.7+ requests.
const taskBudgetsBeta = "task-budgets-2026-03-13"

// ensureTaskBudgetsBeta appends the task-budget beta token when the request
// body includes output_config.task_budget.
func ensureTaskBudgetsBeta(betas []string, body []byte) []string {
	if !gjson.GetBytes(body, "output_config.task_budget").Exists() {
		return betas
	}
	for _, beta := range betas {
		if strings.EqualFold(strings.TrimSpace(beta), taskBudgetsBeta) {
			return betas
		}
	}
	return append(betas, taskBudgetsBeta)
}

// stripSamplingParamsForOpus47 removes sampling parameters that Anthropic no
// longer accepts for Claude Opus 4.7+ request bodies.
func stripSamplingParamsForOpus47(body []byte, baseModel string) []byte {
	if !isOpus47OrLater(baseModel) {
		return body
	}
	for _, path := range []string{"temperature", "top_p", "top_k"} {
		body, _ = sjson.DeleteBytes(body, path)
	}
	return body
}

// isOpus47OrLater reports whether the model belongs to the Claude Opus 4.7+
// family handled by the runtime sampling/task-budget adjustments.
func isOpus47OrLater(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	const prefix = "claude-opus-4-"
	if !strings.HasPrefix(lower, prefix) {
		return false
	}

	rest := lower[len(prefix):]
	var digits strings.Builder
	for _, c := range rest {
		if c < '0' || c > '9' {
			break
		}
		digits.WriteRune(c)
	}
	if digits.Len() == 0 {
		return false
	}
	if digits.Len() == 8 && digits.Len() == len(rest) {
		return false
	}

	version, err := strconv.Atoi(digits.String())
	if err != nil {
		return false
	}
	return version >= 7
}
