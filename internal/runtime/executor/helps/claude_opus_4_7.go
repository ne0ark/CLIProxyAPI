package helps

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// TaskBudgetsBeta is the Anthropic beta token required for
// output_config.task_budget support on Claude Opus 4.7+ requests.
const TaskBudgetsBeta = "task-budgets-2026-03-13"

// EnsureTaskBudgetsBeta appends the task-budget beta token when the request
// body includes output_config.task_budget.
func EnsureTaskBudgetsBeta(betas []string, body []byte) []string {
	if !gjson.GetBytes(body, "output_config.task_budget").Exists() {
		return betas
	}
	for _, beta := range betas {
		if strings.EqualFold(strings.TrimSpace(beta), TaskBudgetsBeta) {
			return betas
		}
	}
	return append(betas, TaskBudgetsBeta)
}

// StripSamplingParamsForOpus47 removes sampling parameters that Anthropic does
// not accept for Claude Opus 4.7+ request bodies.
func StripSamplingParamsForOpus47(body []byte, baseModel string) []byte {
	effectiveModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if effectiveModel == "" {
		effectiveModel = baseModel
	}
	if !IsOpus47OrLater(effectiveModel) {
		return body
	}
	for _, path := range []string{"temperature", "top_p", "top_k"} {
		body, _ = sjson.DeleteBytes(body, path)
	}
	return body
}

// IsOpus47OrLater reports whether the model belongs to the Claude Opus 4.7+
// family that shares the task-budget and sampling-parameter restrictions.
func IsOpus47OrLater(model string) bool {
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
	if digits.Len() == len(rest) && digits.Len() == 8 {
		return false
	}

	version, err := strconv.Atoi(digits.String())
	if err != nil {
		return false
	}
	return version >= 7
}
