package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAgentsMDFreshness(t *testing.T) {
	root := repoRoot(t)
	agentsPath := filepath.Join(root, "AGENTS.md")

	agentsBytes, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read AGENTS.md: %v", err)
	}
	agents := strings.ReplaceAll(string(agentsBytes), "\r\n", "\n")

	commands := extractCommands(t, agents)
	expectedCommands := []string{
		"gofmt -w .",
		"go build -o cli-proxy-api ./cmd/server",
		"go run ./cmd/server",
		"go test ./...",
		"go test -v -run TestName ./path/to/pkg",
		"go build -o test-output ./cmd/server && rm test-output",
	}

	for _, want := range expectedCommands {
		if !containsCommand(commands, want) {
			t.Fatalf("AGENTS.md command %q is missing from the Commands section", want)
		}
	}

	configTargets := []string{
		"config.example.yaml",
		"auths",
	}

	for _, relativePath := range configTargets {
		if _, err := os.Stat(filepath.Join(root, relativePath)); err != nil {
			t.Fatalf("AGENTS.md references %q, but it is not present: %v", relativePath, err)
		}
	}

	for _, relativePath := range extractArchitecturePaths(t, agents) {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relativePath))); err != nil {
			t.Fatalf("AGENTS.md architecture path %q does not exist: %v", relativePath, err)
		}
	}

	helpOutput := runCommand(t, root, "go", "run", "./cmd/server", "-h")
	if !strings.Contains(helpOutput, "Usage of") {
		t.Fatalf("server help output did not contain usage information:\n%s", helpOutput)
	}

	for _, flagName := range extractDocumentedFlags(t, agents) {
		usageFlag := "-" + strings.TrimPrefix(flagName, "--")
		if !strings.Contains(helpOutput, usageFlag) {
			t.Fatalf("AGENTS.md documents flag %q, but server help output is missing %q", flagName, usageFlag)
		}
	}

	gofmtInput := "package main\nimport \"fmt\"\nfunc main( ){fmt.Println(\"hello\")}\n"
	gofmtFile := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(gofmtFile, []byte(gofmtInput), 0o644); err != nil {
		t.Fatalf("failed to write gofmt smoke-test file: %v", err)
	}

	runCommand(t, root, "gofmt", "-w", gofmtFile)

	formatted, err := os.ReadFile(gofmtFile)
	if err != nil {
		t.Fatalf("failed to read gofmt smoke-test file: %v", err)
	}
	if string(formatted) == gofmtInput {
		t.Fatal("gofmt -w smoke test did not rewrite the file")
	}
	if strings.Contains(string(formatted), "main( )") {
		t.Fatalf("gofmt -w smoke test left the function signature unformatted:\n%s", string(formatted))
	}

	diffOutput := runCommand(t, root, "gofmt", "-d", gofmtFile)
	if strings.TrimSpace(diffOutput) != "" {
		t.Fatalf("gofmt -d reported remaining formatting differences:\n%s", diffOutput)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file path")
	}

	return filepath.Dir(filepath.Dir(currentFile))
}

func extractCommands(t *testing.T, agents string) []string {
	t.Helper()

	section := sectionBetween(t, agents, "## Commands", "## Config")
	matches := regexp.MustCompile("(?s)```bash\\n(.*?)\\n```").FindStringSubmatch(section)
	if len(matches) != 2 {
		t.Fatal("failed to locate bash command block in AGENTS.md")
	}

	lines := strings.Split(matches[1], "\n")
	commands := make([]string, 0, len(lines))
	for _, line := range lines {
		command := strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if command == "" {
			continue
		}
		commands = append(commands, command)
	}

	return commands
}

func extractArchitecturePaths(t *testing.T, agents string) []string {
	t.Helper()

	section := sectionBetween(t, agents, "## Architecture", "## Code Conventions")
	matches := regexp.MustCompile("(?m)^- `([^`]+)`").FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		t.Fatal("failed to locate architecture paths in AGENTS.md")
	}

	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		paths = append(paths, match[1])
	}

	return paths
}

func extractDocumentedFlags(t *testing.T, agents string) []string {
	t.Helper()

	section := sectionBetween(t, agents, "## Commands", "## Config")
	matches := regexp.MustCompile("--[a-z0-9-]+").FindAllString(section, -1)
	if len(matches) == 0 {
		t.Fatal("failed to locate documented flags in AGENTS.md")
	}

	return matches
}

func sectionBetween(t *testing.T, content, startHeading, endHeading string) string {
	t.Helper()

	start := strings.Index(content, startHeading)
	if start == -1 {
		t.Fatalf("failed to find section %q", startHeading)
	}

	section := content[start:]
	end := strings.Index(section, endHeading)
	if end == -1 {
		t.Fatalf("failed to find end heading %q after %q", endHeading, startHeading)
	}

	return section[:end]
}

func containsCommand(commands []string, want string) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}

	return false
}

func runCommand(t *testing.T, dir, name string, args ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("%s timed out after 2m", strings.Join(append([]string{name}, args...), " "))
	}
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", strings.Join(append([]string{name}, args...), " "), err, string(output))
	}

	return string(output)
}
