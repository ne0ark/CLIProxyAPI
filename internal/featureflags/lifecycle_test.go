package featureflags

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefinitionsHaveLifecycleMetadataAndLiveUsage(t *testing.T) {
	if err := ValidateDefinitions(time.Now()); err != nil {
		t.Fatal(err)
	}

	repoRoot := findRepositoryRoot(t)
	usageCounts, err := scanLiveUsage(repoRoot, Definitions())
	if err != nil {
		t.Fatalf("scan live feature flag usage: %v", err)
	}

	for _, definition := range Definitions() {
		if usageCounts[definition.Key] > 0 {
			continue
		}

		t.Errorf("feature flag %q has no live usage outside internal/featureflags; searched for %v", definition.Key, definition.UsageTokens)
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()

	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if info, errStat := os.Stat(goModPath); errStat == nil && !info.IsDir() {
			return currentDir
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			t.Fatalf("could not find repository root from %s", currentDir)
		}
		currentDir = parentDir
	}
}

func scanLiveUsage(root string, definitions []Definition) (map[Key]int, error) {
	counts := make(map[Key]int, len(definitions))

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			if shouldSkipUsageDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		relativePath := toRelativeSlashPath(root, path)
		if !shouldScanUsageFile(relativePath) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		fileContent := string(content)
		for _, definition := range definitions {
			for _, usageToken := range definition.UsageTokens {
				counts[definition.Key] += strings.Count(fileContent, usageToken)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return counts, nil
}

func shouldSkipUsageDir(name string) bool {
	switch name {
	case ".git", ".factory", "dist", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func shouldScanUsageFile(relativePath string) bool {
	if filepath.Ext(relativePath) != ".go" {
		return false
	}
	if strings.HasSuffix(relativePath, "_test.go") {
		return false
	}
	if strings.HasPrefix(relativePath, "internal/featureflags/") {
		return false
	}
	return true
}

func toRelativeSlashPath(root string, path string) string {
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relativePath)
}
