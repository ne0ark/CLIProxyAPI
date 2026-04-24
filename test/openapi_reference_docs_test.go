package test

import (
	"strings"
	"testing"
)

func TestOpenAPIReferenceDocsFreshness(t *testing.T) {
	root := repoRoot(t)

	output := runCommand(t, root, "go", "run", "./cmd/generate-openapi-docs", "--check")
	if !strings.Contains(output, "up to date:") {
		t.Fatalf("unexpected generator check output:\n%s", output)
	}
}
