package techdebt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDetectsOnlyBareMarkersInGoComments(t *testing.T) {
	root := t.TempDir()

	sourcePath := filepath.Join(root, "sample.go")
	source := `package sample

func value() string {
	message := "TODO in a string should be ignored"
	return message
}

// TODO add tracking
// TODO(DEBT-1234): tracked item
/* FIXME missing tracking */
/*
 * HACK(DEBT-9999): tracked item
 * XXX needs cleanup
 */
`

	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	report, err := Scan(root)
	if err != nil {
		t.Fatalf("scan root: %v", err)
	}

	if report.ScannedFiles != 1 {
		t.Fatalf("expected 1 scanned file, got %d", report.ScannedFiles)
	}

	if len(report.Findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(report.Findings))
	}

	expected := []Finding{
		{Path: "sample.go", Line: 8, Marker: "TODO", Comment: "TODO add tracking"},
		{Path: "sample.go", Line: 10, Marker: "FIXME", Comment: "FIXME missing tracking"},
		{Path: "sample.go", Line: 13, Marker: "XXX", Comment: "XXX needs cleanup"},
	}

	for index, want := range expected {
		got := report.Findings[index]
		if got != want {
			t.Fatalf("finding %d mismatch: got %+v want %+v", index, got, want)
		}
	}
}

func TestScanHashCommentFilesIgnoresValuesAndAllowsTrackedMarkers(t *testing.T) {
	root := t.TempDir()

	configPath := filepath.Join(root, "config.yaml")
	config := `name: "CPA-XXX"
status: "TODO in data should be ignored"
# TODO add tracking
command: "echo # not a comment"
path: https://example.com/#fragment
value: 1 # FIXME(DEBT-42): tracked item
script: echo keep # HACK without tracking
`

	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	report, err := Scan(root)
	if err != nil {
		t.Fatalf("scan root: %v", err)
	}

	if len(report.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(report.Findings))
	}

	expected := []Finding{
		{Path: "config.yaml", Line: 3, Marker: "TODO", Comment: "TODO add tracking"},
		{Path: "config.yaml", Line: 7, Marker: "HACK", Comment: "HACK without tracking"},
	}

	for index, want := range expected {
		got := report.Findings[index]
		if got != want {
			t.Fatalf("finding %d mismatch: got %+v want %+v", index, got, want)
		}
	}
}
