#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
import sys
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

COMPLEXITY_THRESHOLDS = (10, 15, 20)
TOP_ENTRY_LIMIT = 10
REPO_ROOT = Path(__file__).resolve().parents[2]
MODULE_PREFIX = ""

COVERAGE_LINE_RE = re.compile(
    r"^(?P<path>.*):\d+\.\d+,\d+\.\d+\s+(?P<statements>\d+)\s+(?P<count>\d+(?:\.\d+)?)$"
)
GOCYCLO_LINE_RE = re.compile(
    r"^(?P<complexity>\d+)\s+(?P<package>\S+)\s+(?P<function>\S+)\s+(?P<path>.*):(?P<line>\d+):(?P<column>\d+)$"
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Generate consolidated code quality metrics from coverage, gocyclo, technical debt, "
            "build performance, test performance, and optional flaky-test detection inputs."
        )
    )
    parser.add_argument("--coverage", required=True, help="Path to go coverage profile output")
    parser.add_argument("--complexity", required=True, help="Path to gocyclo text output")
    parser.add_argument("--tech-debt", required=True, help="Path to technical debt JSON output")
    parser.add_argument("--build-performance", help="Path to canonical build performance JSON output")
    parser.add_argument("--test-performance", help="Path to full-suite test performance JSON output")
    parser.add_argument("--flaky-test-report", help="Path to flaky test detection JSON output")
    parser.add_argument("--json-out", required=True, help="Path to the generated JSON report")
    parser.add_argument("--md-out", required=True, help="Path to the generated Markdown report")
    return parser.parse_args()


def percent(numerator: float, denominator: float) -> float:
    if denominator == 0:
        return 0.0
    return round((numerator / denominator) * 100, 2)


def format_duration(seconds: float) -> str:
    if seconds < 60:
        return f"{seconds:.3f} s"

    minutes = int(seconds // 60)
    remaining_seconds = seconds - (minutes * 60)
    return f"{minutes}m {remaining_seconds:.3f}s"


def format_bytes(size_bytes: int) -> str:
    units = ("B", "KiB", "MiB", "GiB", "TiB")
    size = float(size_bytes)

    for unit in units:
        if size < 1024 or unit == units[-1]:
            if unit == "B":
                return f"{int(size)} {unit}"
            return f"{size:.2f} {unit}"
        size /= 1024


def load_module_prefix() -> str:
    go_mod = REPO_ROOT / "go.mod"
    if not go_mod.exists():
        return ""

    for line in go_mod.read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.removeprefix("module ").strip()
    return ""


def normalize_report_path(path_value: str) -> str:
    normalized = path_value.replace("\\", "/")
    prefix = MODULE_PREFIX or load_module_prefix()
    if prefix and normalized.startswith(f"{prefix}/"):
        return normalized[len(prefix) + 1 :]

    candidate = Path(path_value)
    if candidate.is_absolute():
        try:
            return candidate.resolve().relative_to(REPO_ROOT.resolve()).as_posix()
        except ValueError:
            return normalized

    return normalized


def read_text(path: Path) -> str:
    raw = path.read_bytes()
    for encoding in ("utf-8", "utf-8-sig", "utf-16", "utf-16-le", "utf-16-be"):
        try:
            return raw.decode(encoding).lstrip("\ufeff")
        except UnicodeDecodeError:
            continue
    raise UnicodeDecodeError("unknown", raw, 0, 1, f"unable to decode {path}")


def read_lines(path: Path) -> list[str]:
    return read_text(path).splitlines()


def parse_coverage(path: Path) -> dict[str, Any]:
    per_file: dict[str, dict[str, int]] = defaultdict(lambda: {"total": 0, "covered": 0})

    for raw_line in read_lines(path):
        line = raw_line.strip()
        if not line or line.startswith("mode:"):
            continue

        match = COVERAGE_LINE_RE.match(line)
        if match is None:
            raise ValueError(f"unsupported coverage line: {line}")

        file_path = normalize_report_path(match.group("path"))
        statements = int(match.group("statements"))
        count = float(match.group("count"))

        per_file[file_path]["total"] += statements
        if count > 0:
            per_file[file_path]["covered"] += statements

    total_statements = sum(item["total"] for item in per_file.values())
    covered_statements = sum(item["covered"] for item in per_file.values())

    uncovered_files = []
    for file_path, item in per_file.items():
        uncovered_statements = item["total"] - item["covered"]
        uncovered_files.append(
            {
                "path": file_path,
                "coverage_percent": percent(item["covered"], item["total"]),
                "covered_statements": item["covered"],
                "total_statements": item["total"],
                "uncovered_statements": uncovered_statements,
            }
        )

    uncovered_files.sort(
        key=lambda item: (
            -item["uncovered_statements"],
            item["coverage_percent"],
            item["path"],
        )
    )

    return {
        "profile": str(path),
        "files": len(per_file),
        "covered_statements": covered_statements,
        "total_statements": total_statements,
        "percent": percent(covered_statements, total_statements),
        "top_uncovered_files": uncovered_files[:TOP_ENTRY_LIMIT],
    }


def parse_complexity(path: Path) -> dict[str, Any]:
    entries = []

    for raw_line in read_lines(path):
        line = raw_line.strip()
        if not line or line.startswith("Average:"):
            continue

        match = GOCYCLO_LINE_RE.match(line)
        if match is None:
            raise ValueError(f"unsupported gocyclo line: {line}")

        entries.append(
            {
                "complexity": int(match.group("complexity")),
                "package": match.group("package"),
                "function": match.group("function"),
                "path": normalize_report_path(match.group("path")),
                "line": int(match.group("line")),
                "column": int(match.group("column")),
            }
        )

    if not entries:
        raise ValueError("gocyclo report does not contain any functions")

    entries.sort(
        key=lambda item: (
            -item["complexity"],
            item["path"],
            item["function"],
        )
    )

    threshold_counts = {
        str(threshold): sum(1 for item in entries if item["complexity"] > threshold)
        for threshold in COMPLEXITY_THRESHOLDS
    }

    total_complexity = sum(item["complexity"] for item in entries)

    return {
        "report": str(path),
        "functions": len(entries),
        "average_cyclomatic_complexity": round(total_complexity / len(entries), 2),
        "max_cyclomatic_complexity": entries[0]["complexity"],
        "threshold_counts": threshold_counts,
        "top_functions": entries[:TOP_ENTRY_LIMIT],
    }


def parse_tech_debt(path: Path) -> dict[str, Any]:
    payload = json.loads(read_text(path))
    findings = payload.get("findings") or []
    scanned_files = int(payload.get("scanned_files", 0))

    impacted_files = sorted({finding["path"] for finding in findings})
    impacted_file_count = len(impacted_files)
    debt_free_files = max(scanned_files - impacted_file_count, 0)

    return {
        "report": str(path),
        "technical_debt_findings": len(findings),
        "scanned_files": scanned_files,
        "impacted_files": impacted_file_count,
        "debt_free_files": debt_free_files,
        "debt_free_rate": percent(debt_free_files, scanned_files),
        "finding_density_per_100_files": round((len(findings) / scanned_files) * 100, 2)
        if scanned_files
        else 0.0,
        "findings_preview": findings[:TOP_ENTRY_LIMIT],
    }


def parse_command_performance_payload(path: Path) -> tuple[dict[str, Any], list[Any], str]:
    payload = json.loads(read_text(path))
    command = payload.get("command") or []
    command_display = payload.get("command_display")
    if not command_display and isinstance(command, list):
        command_display = " ".join(str(part) for part in command)

    if not command_display:
        raise ValueError(f"performance report does not contain a command: {path}")

    return payload, command, command_display


def parse_build_performance(path: Path) -> dict[str, Any]:
    payload, command, command_display = parse_command_performance_payload(path)

    return {
        "report": str(path),
        "command": command,
        "command_display": command_display,
        "started_at_utc": str(payload["started_at_utc"]),
        "finished_at_utc": str(payload["finished_at_utc"]),
        "duration_seconds": round(float(payload["duration_seconds"]), 3),
        "exit_code": int(payload.get("exit_code", 0)),
        "output_path": str(payload["output_path"]),
        "output_size_bytes": int(payload["output_size_bytes"]),
    }


def parse_test_performance(path: Path) -> dict[str, Any]:
    payload, command, command_display = parse_command_performance_payload(path)

    return {
        "report": str(path),
        "command": command,
        "command_display": command_display,
        "started_at_utc": str(payload["started_at_utc"]),
        "finished_at_utc": str(payload["finished_at_utc"]),
        "duration_seconds": round(float(payload["duration_seconds"]), 3),
        "exit_code": int(payload.get("exit_code", 0)),
        "coverage_profile_path": str(payload.get("coverage_profile_path", "")),
        "coverage_profile_exists": bool(payload.get("coverage_profile_exists", False)),
        "coverage_profile_size_bytes": int(payload.get("coverage_profile_size_bytes", 0)),
    }


def parse_flaky_test_report(path: Path) -> dict[str, Any]:
    payload = json.loads(read_text(path))
    summary = payload.get("summary") or {}
    probe_configuration = payload.get("probe_configuration") or {}
    flaky_tests = payload.get("flaky_tests") or []
    missing_requested_packages = payload.get("missing_requested_packages") or []

    return {
        "report": str(path),
        "command_template": str(probe_configuration.get("command_template", "")),
        "repeat_count": int(probe_configuration.get("repeat_count", 0)),
        "requested_packages": int(summary.get("requested_packages", 0)),
        "requested_packages_without_events": int(summary.get("requested_packages_without_events", 0)),
        "observed_packages": int(summary.get("observed_packages", 0)),
        "runs_executed": int(summary.get("runs_executed", 0)),
        "failing_runs": int(summary.get("failing_runs", 0)),
        "unique_tests_observed": int(summary.get("unique_tests_observed", 0)),
        "flaky_tests": int(summary.get("flaky_tests", 0)),
        "packages_with_flaky_tests": int(summary.get("packages_with_flaky_tests", 0)),
        "clean": bool(summary.get("clean", False)),
        "missing_requested_packages_preview": missing_requested_packages[:TOP_ENTRY_LIMIT],
        "flaky_tests_preview": flaky_tests[:TOP_ENTRY_LIMIT],
    }


def build_report(
    coverage_path: Path,
    complexity_path: Path,
    tech_debt_path: Path,
    build_performance_path: Path | None = None,
    test_performance_path: Path | None = None,
    flaky_test_report_path: Path | None = None,
) -> dict[str, Any]:
    coverage = parse_coverage(coverage_path)
    complexity = parse_complexity(complexity_path)
    maintainability = parse_tech_debt(tech_debt_path)
    build_performance = (
        parse_build_performance(build_performance_path) if build_performance_path is not None else None
    )
    test_performance = (
        parse_test_performance(test_performance_path) if test_performance_path is not None else None
    )
    flaky_test_report = (
        parse_flaky_test_report(flaky_test_report_path) if flaky_test_report_path is not None else None
    )

    summary = {
        "coverage_percent": coverage["percent"],
        "average_cyclomatic_complexity": complexity["average_cyclomatic_complexity"],
        "max_cyclomatic_complexity": complexity["max_cyclomatic_complexity"],
        "technical_debt_findings": maintainability["technical_debt_findings"],
        "technical_debt_clean": maintainability["technical_debt_findings"] == 0,
    }
    if build_performance is not None:
        summary.update(
            {
                "build_duration_seconds": build_performance["duration_seconds"],
                "build_exit_code": build_performance["exit_code"],
                "build_output_size_bytes": build_performance["output_size_bytes"],
                "build_succeeded": build_performance["exit_code"] == 0,
            }
        )
    if test_performance is not None:
        summary.update(
            {
                "test_duration_seconds": test_performance["duration_seconds"],
                "test_exit_code": test_performance["exit_code"],
                "test_coverage_profile_size_bytes": test_performance["coverage_profile_size_bytes"],
                "test_succeeded": test_performance["exit_code"] == 0,
            }
        )
    if flaky_test_report is not None:
        summary.update(
            {
                "flaky_runs_executed": flaky_test_report["runs_executed"],
                "flaky_failing_runs": flaky_test_report["failing_runs"],
                "flaky_tests_detected": flaky_test_report["flaky_tests"],
                "flaky_packages": flaky_test_report["packages_with_flaky_tests"],
                "flaky_signal_clean": flaky_test_report["clean"],
            }
        )

    return {
        "generated_at_utc": datetime.now(timezone.utc).replace(microsecond=0).isoformat(),
        "inputs": {
            "coverage": str(coverage_path),
            "complexity": str(complexity_path),
            "tech_debt": str(tech_debt_path),
            "build_performance": str(build_performance_path) if build_performance_path is not None else None,
            "test_performance": str(test_performance_path) if test_performance_path is not None else None,
            "flaky_test_report": str(flaky_test_report_path) if flaky_test_report_path is not None else None,
        },
        "summary": summary,
        "coverage": coverage,
        "complexity": complexity,
        "maintainability": maintainability,
        "build_performance": build_performance,
        "test_performance": test_performance,
        "flaky_test_report": flaky_test_report,
    }


def table_escape(value: Any) -> str:
    return str(value).replace("|", "\\|").replace("\n", " ")


def render_markdown(report: dict[str, Any]) -> str:
    coverage = report["coverage"]
    complexity = report["complexity"]
    maintainability = report["maintainability"]
    build_performance = report.get("build_performance")
    test_performance = report.get("test_performance")
    flaky_test_report = report.get("flaky_test_report")

    lines = [
        "# Code Quality Metrics",
        "",
        f"_Generated: {report['generated_at_utc']}_",
        "",
        "This report consolidates repository-wide coverage, cyclomatic complexity, maintainability, and performance signals.",
        "",
        "## Scorecard",
        "",
        "| Area | Metric | Value |",
        "| --- | --- | ---: |",
        f"| Coverage | Statement coverage | {coverage['percent']:.2f}% |",
        f"| Coverage | Covered statements | {coverage['covered_statements']} / {coverage['total_statements']} |",
        f"| Coverage | Files in profile | {coverage['files']} |",
        f"| Complexity | Functions analyzed | {complexity['functions']} |",
        f"| Complexity | Average cyclomatic complexity | {complexity['average_cyclomatic_complexity']:.2f} |",
        f"| Complexity | Max cyclomatic complexity | {complexity['max_cyclomatic_complexity']} |",
        f"| Complexity | Functions over 10 | {complexity['threshold_counts']['10']} |",
        f"| Complexity | Functions over 15 | {complexity['threshold_counts']['15']} |",
        f"| Complexity | Functions over 20 | {complexity['threshold_counts']['20']} |",
        f"| Maintainability | Technical debt findings | {maintainability['technical_debt_findings']} |",
        f"| Maintainability | Impacted files | {maintainability['impacted_files']} |",
        f"| Maintainability | Debt-free scan rate | {maintainability['debt_free_rate']:.2f}% |",
        f"| Maintainability | Finding density / 100 files | {maintainability['finding_density_per_100_files']:.2f} |",
    ]

    if build_performance is not None:
        lines.extend(
            [
                f"| Build | Canonical server build duration | {format_duration(build_performance['duration_seconds'])} |",
                f"| Build | Build exit code | {build_performance['exit_code']} |",
                f"| Build | Build output size | {format_bytes(build_performance['output_size_bytes'])} |",
            ]
        )

    if test_performance is not None:
        lines.extend(
            [
                f"| Tests | Full-suite coverage test duration | {format_duration(test_performance['duration_seconds'])} |",
                f"| Tests | Test exit code | {test_performance['exit_code']} |",
                f"| Tests | Coverage profile size | {format_bytes(test_performance['coverage_profile_size_bytes'])} |",
            ]
        )
    if flaky_test_report is not None:
        lines.extend(
            [
                f"| Flaky tests | Repeated runs executed | {flaky_test_report['runs_executed']} |",
                f"| Flaky tests | Failing repeated runs | {flaky_test_report['failing_runs']} |",
                f"| Flaky tests | Flaky tests detected | {flaky_test_report['flaky_tests']} |",
                f"| Flaky tests | Clean signal | {str(flaky_test_report['clean']).lower()} |",
            ]
        )

    lines.extend(
        [
            "",
            "## Inputs",
            "",
            "- Coverage: `go test -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...`",
            "- Complexity: `go run github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0 -over 0 .`",
            "- Maintainability: `go run ./cmd/checktechdebt --format json --output technical-debt-report.json`",
        ]
    )

    if build_performance is not None:
        lines.append(f"- Build: `{table_escape(build_performance['command_display'])}`")
    if test_performance is not None:
        lines.append(f"- Test performance: `{table_escape(test_performance['command_display'])}`")
    if flaky_test_report is not None and flaky_test_report["command_template"]:
        lines.append(
            f"- Flaky test detection: `{table_escape(flaky_test_report['command_template'])}` repeated `{flaky_test_report['repeat_count']}` times per curated package"
        )

    lines.extend([""])

    if build_performance is not None:
        lines.extend(
            [
                "## Build Performance",
                "",
                "| Metric | Value |",
                "| --- | --- |",
                f"| Command | `{table_escape(build_performance['command_display'])}` |",
                f"| Duration | {format_duration(build_performance['duration_seconds'])} |",
                f"| Started (UTC) | {table_escape(build_performance['started_at_utc'])} |",
                f"| Finished (UTC) | {table_escape(build_performance['finished_at_utc'])} |",
                f"| Output | `{table_escape(build_performance['output_path'])}` |",
                f"| Output size | {format_bytes(build_performance['output_size_bytes'])} ({build_performance['output_size_bytes']} bytes) |",
                "",
            ]
        )

    if test_performance is not None:
        lines.extend(
            [
                "## Test Performance",
                "",
                "| Metric | Value |",
                "| --- | --- |",
                f"| Command | `{table_escape(test_performance['command_display'])}` |",
                f"| Duration | {format_duration(test_performance['duration_seconds'])} |",
                f"| Started (UTC) | {table_escape(test_performance['started_at_utc'])} |",
                f"| Finished (UTC) | {table_escape(test_performance['finished_at_utc'])} |",
                f"| Coverage profile | `{table_escape(test_performance['coverage_profile_path'])}` |",
                f"| Coverage profile present | {str(test_performance['coverage_profile_exists']).lower()} |",
                f"| Coverage profile size | {format_bytes(test_performance['coverage_profile_size_bytes'])} ({test_performance['coverage_profile_size_bytes']} bytes) |",
                "",
            ]
        )

    if flaky_test_report is not None:
        lines.extend(
            [
                "## Flaky Test Detection",
                "",
                "| Metric | Value |",
                "| --- | --- |",
                f"| Command template | `{table_escape(flaky_test_report['command_template'])}` |",
                f"| Repeat count | {flaky_test_report['repeat_count']} |",
                f"| Requested packages | {flaky_test_report['requested_packages']} |",
                f"| Observed packages | {flaky_test_report['observed_packages']} |",
                f"| Repeated runs executed | {flaky_test_report['runs_executed']} |",
                f"| Failing repeated runs | {flaky_test_report['failing_runs']} |",
                f"| Unique tests observed | {flaky_test_report['unique_tests_observed']} |",
                f"| Flaky tests detected | {flaky_test_report['flaky_tests']} |",
                f"| Packages with flaky tests | {flaky_test_report['packages_with_flaky_tests']} |",
                f"| Missing requested packages | {flaky_test_report['requested_packages_without_events']} |",
                "",
            ]
        )

        if flaky_test_report["missing_requested_packages_preview"]:
            lines.extend(
                [
                    "### Missing Requested Packages",
                    "",
                ]
            )
            for package_target in flaky_test_report["missing_requested_packages_preview"]:
                lines.append(f"- `{table_escape(package_target)}`")
            lines.append("")

        if flaky_test_report["flaky_tests"] == 0:
            lines.extend(["No flaky tests were detected across the curated repeated probes.", ""])
        else:
            lines.extend(
                [
                    "### Flaky Tests Preview",
                    "",
                    "| Package | Test | Passes | Fails | Attempts |",
                    "| --- | --- | ---: | ---: | ---: |",
                ]
            )
            for item in flaky_test_report["flaky_tests_preview"]:
                lines.append(
                    f"| `{table_escape(item['package'])}` | `{table_escape(item['test'])}` | {item['pass_count']} | {item['fail_count']} | {item['attempts']} |"
                )
            lines.append("")

    lines.extend(
        [
            "## Highest Complexity Functions",
            "",
            "| Complexity | Package | Function | Location |",
            "| ---: | --- | --- | --- |",
        ]
    )

    for item in complexity["top_functions"]:
        location = f"{item['path']}:{item['line']}:{item['column']}"
        lines.append(
            f"| {item['complexity']} | {table_escape(item['package'])} | {table_escape(item['function'])} | `{table_escape(location)}` |"
        )

    lines.extend(
        [
            "",
            "## Largest Coverage Gaps",
            "",
            "| Uncovered Statements | Coverage | File |",
            "| ---: | ---: | --- |",
        ]
    )

    for item in coverage["top_uncovered_files"]:
        lines.append(
            f"| {item['uncovered_statements']} | {item['coverage_percent']:.2f}% | `{table_escape(item['path'])}` |"
        )

    lines.extend(["", "## Technical Debt Findings", ""])

    if maintainability["technical_debt_findings"] == 0:
        lines.append("No technical debt findings were reported.")
    else:
        lines.extend(
            [
                "| Path | Line | Marker | Comment |",
                "| --- | ---: | --- | --- |",
            ]
        )
        for finding in maintainability["findings_preview"]:
            lines.append(
                f"| `{table_escape(finding['path'])}` | {finding['line']} | {table_escape(finding['marker'])} | {table_escape(finding['comment'])} |"
            )

    lines.append("")
    return "\n".join(lines)


def write_file(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def main() -> int:
    global MODULE_PREFIX

    args = parse_args()
    MODULE_PREFIX = load_module_prefix()

    coverage_path = Path(args.coverage)
    complexity_path = Path(args.complexity)
    tech_debt_path = Path(args.tech_debt)
    build_performance_path = Path(args.build_performance) if args.build_performance else None
    test_performance_path = Path(args.test_performance) if args.test_performance else None
    flaky_test_report_path = Path(args.flaky_test_report) if args.flaky_test_report else None
    json_out = Path(args.json_out)
    md_out = Path(args.md_out)

    report = build_report(
        coverage_path,
        complexity_path,
        tech_debt_path,
        build_performance_path,
        test_performance_path,
        flaky_test_report_path,
    )
    markdown = render_markdown(report)

    json_out.parent.mkdir(parents=True, exist_ok=True)
    json_out.write_text(f"{json.dumps(report, indent=2)}\n", encoding="utf-8")
    write_file(md_out, markdown)

    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1)
