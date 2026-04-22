package techdebt

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	markerPattern  = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\b`)
	trackedPattern = regexp.MustCompile(`^(TODO|FIXME|XXX|HACK)\(DEBT-\d+\):\s+\S`)
)

type Finding struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Marker  string `json:"marker"`
	Comment string `json:"comment"`
}

type Report struct {
	Root         string    `json:"root"`
	ScannedFiles int       `json:"scanned_files"`
	Findings     []Finding `json:"findings"`
}

func (r Report) HasFindings() bool {
	return len(r.Findings) > 0
}

func Scan(root string) (Report, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve root path: %w", err)
	}

	report := Report{Root: filepath.Clean(absRoot)}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		scanner, ok := scannerForPath(path)
		if !ok {
			return nil
		}

		findings, errScan := scanner(path, absRoot)
		if errScan != nil {
			return fmt.Errorf("scan %s: %w", path, errScan)
		}

		report.ScannedFiles++
		report.Findings = append(report.Findings, findings...)
		return nil
	})
	if err != nil {
		return Report{}, err
	}

	sort.Slice(report.Findings, func(i, j int) bool {
		left := report.Findings[i]
		right := report.Findings[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Marker != right.Marker {
			return left.Marker < right.Marker
		}
		return left.Comment < right.Comment
	})

	return report, nil
}

type fileScanner func(path string, root string) ([]Finding, error)

func scannerForPath(path string) (fileScanner, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return scanGoFile, true
	case ".yml", ".yaml", ".sh", ".bash", ".zsh", ".ps1", ".py", ".toml":
		return scanHashCommentFile, true
	}

	switch filepath.Base(path) {
	case "Dockerfile", ".gitignore", ".dockerignore":
		return scanHashCommentFile, true
	default:
		return nil, false
	}
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".factory", "dist", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func scanGoFile(path string, root string) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return nil, err
	}

	relPath := relativePath(root, path)
	findings := make([]Finding, 0)

	for _, group := range file.Comments {
		for _, comment := range group.List {
			startLine := fset.Position(comment.Pos()).Line
			findings = append(findings, scanRawComment(relPath, startLine, comment.Text)...)
		}
	}

	return findings, nil
}

func scanHashCommentFile(path string, root string) ([]Finding, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	relPath := relativePath(root, path)
	findings := make([]Finding, 0)

	for lineNumber, line := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		commentStart := findHashCommentStart(line)
		if commentStart < 0 {
			continue
		}

		commentText := strings.TrimSpace(line[commentStart+1:])
		findings = append(findings, scanCommentLine(relPath, lineNumber+1, commentText)...)
	}

	return findings, nil
}

func scanRawComment(path string, startLine int, raw string) []Finding {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")

	if strings.HasPrefix(normalized, "//") {
		return scanCommentLine(path, startLine, strings.TrimSpace(strings.TrimPrefix(normalized, "//")))
	}

	if strings.HasPrefix(normalized, "/*") {
		body := strings.TrimPrefix(normalized, "/*")
		body = strings.TrimSuffix(body, "*/")
		lines := strings.Split(body, "\n")
		findings := make([]Finding, 0)
		for index, line := range lines {
			cleaned := strings.TrimSpace(line)
			cleaned = strings.TrimPrefix(cleaned, "*")
			cleaned = strings.TrimSpace(cleaned)
			findings = append(findings, scanCommentLine(path, startLine+index, cleaned)...)
		}
		return findings
	}

	return scanCommentLine(path, startLine, strings.TrimSpace(normalized))
}

func scanCommentLine(path string, line int, comment string) []Finding {
	if comment == "" {
		return nil
	}

	matches := markerPattern.FindAllStringIndex(comment, -1)
	if len(matches) == 0 {
		return nil
	}

	findings := make([]Finding, 0, len(matches))
	for _, match := range matches {
		marker := comment[match[0]:match[1]]
		if trackedPattern.MatchString(comment[match[0]:]) {
			continue
		}

		findings = append(findings, Finding{
			Path:    path,
			Line:    line,
			Marker:  marker,
			Comment: comment,
		})
	}

	return findings
}

func findHashCommentStart(line string) int {
	quote := rune(0)
	escaped := false

	for index, current := range line {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}

			if current == '\\' {
				escaped = true
				continue
			}

			if current == quote {
				quote = 0
			}
			continue
		}

		switch current {
		case '\'', '"':
			quote = current
		case '#':
			if index == 0 {
				return index
			}
			previous := rune(line[index-1])
			if unicode.IsSpace(previous) {
				return index
			}
		}
	}

	return -1
}

func relativePath(root string, path string) string {
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relPath)
}
