package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/techdebt"
)

func main() {
	var (
		rootPath   string
		outputPath string
		format     string
	)

	flag.StringVar(&rootPath, "root", ".", "Root directory to scan")
	flag.StringVar(&outputPath, "output", "", "Write the report to a file")
	flag.StringVar(&format, "format", "text", "Output format: text or json")
	flag.Parse()

	report, err := techdebt.Scan(rootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	payload, err := renderReport(report, format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if outputPath != "" {
		resolvedOutputPath, err := filepath.Abs(outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: resolve output path: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(resolvedOutputPath, payload, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error: write report: %v\n", err)
			os.Exit(1)
		}
	}

	if outputPath == "" {
		if _, err := os.Stdout.Write(payload); err != nil {
			fmt.Fprintf(os.Stderr, "error: write output: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(textReport(report))
	}

	if report.HasFindings() {
		os.Exit(1)
	}
}

func renderReport(report techdebt.Report, format string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal report: %w", err)
		}
		return append(payload, '\n'), nil
	case "text":
		return []byte(textReport(report)), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func textReport(report techdebt.Report) string {
	if !report.HasFindings() {
		return fmt.Sprintf("technical debt check passed (%d files scanned)\n", report.ScannedFiles)
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "technical debt check failed: %d issue(s) across %d scanned file(s)\n", len(report.Findings), report.ScannedFiles)
	for _, finding := range report.Findings {
		fmt.Fprintf(&builder, "%s:%d: %s must use %s(DEBT-1234): summary\n", finding.Path, finding.Line, finding.Marker, finding.Marker)
		fmt.Fprintf(&builder, "  %s\n", finding.Comment)
	}

	return builder.String()
}
