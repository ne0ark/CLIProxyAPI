#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

TOP_ENTRY_LIMIT = 10
SEVERITY_ORDER = {"CRITICAL": 4, "HIGH": 3, "MEDIUM": 2, "LOW": 1, "UNKNOWN": 0}
CONFIDENCE_ORDER = {"HIGH": 3, "MEDIUM": 2, "LOW": 1, "UNKNOWN": 0}
REPO_ROOT = Path(__file__).resolve().parents[2]
MODULE_PREFIX = ""


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Generate a consolidated automated security review summary from govulncheck, "
            "gosec JSON, and gosec SARIF reports."
        )
    )
    parser.add_argument("--govulncheck", required=True, help="Path to the govulncheck JSON report")
    parser.add_argument("--govulncheck-exit-code", type=int, default=0, help="govulncheck exit code")
    parser.add_argument("--gosec", required=True, help="Path to the gosec JSON report")
    parser.add_argument("--gosec-exit-code", type=int, default=0, help="gosec JSON exit code")
    parser.add_argument("--sarif", required=True, help="Path to the gosec SARIF report")
    parser.add_argument("--sarif-exit-code", type=int, default=0, help="gosec SARIF exit code")
    parser.add_argument("--json-out", required=True, help="Path to the generated JSON summary")
    parser.add_argument("--md-out", required=True, help="Path to the generated Markdown summary")
    return parser.parse_args()


def load_module_prefix() -> str:
    go_mod = REPO_ROOT / "go.mod"
    if not go_mod.exists():
        return ""

    for line in go_mod.read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.removeprefix("module ").strip()
    return ""


def read_text(path: Path) -> str:
    raw = path.read_bytes()
    for encoding in ("utf-8", "utf-8-sig", "utf-16", "utf-16-le", "utf-16-be"):
        try:
            return raw.decode(encoding).lstrip("\ufeff")
        except UnicodeDecodeError:
            continue
    raise UnicodeDecodeError("unknown", raw, 0, 1, f"unable to decode {path}")


def read_json(path: Path) -> Any:
    return json.loads(read_text(path))


def table_escape(value: Any) -> str:
    if value is None:
        return ""
    return str(value).replace("|", r"\|").replace("\n", " ").strip()


def compact_text(value: str, limit: int = 160) -> str:
    compacted = " ".join(value.split())
    if len(compacted) <= limit:
        return compacted
    return f"{compacted[: limit - 3].rstrip()}..."


def safe_int(value: Any) -> int:
    if value in (None, ""):
        return 0
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


def normalize_report_path(path_value: str) -> str:
    if not path_value:
        return ""

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


def normalize_module_path(path_value: str) -> str:
    if not path_value:
        return ""

    normalized = path_value.replace("\\", "/")
    prefix = MODULE_PREFIX or load_module_prefix()
    if prefix and normalized.startswith(f"{prefix}/"):
        return normalized[len(prefix) + 1 :]
    if prefix and normalized == prefix:
        return "."
    return normalized


def parse_json_stream(path: Path) -> list[dict[str, Any]]:
    text = read_text(path)
    decoder = json.JSONDecoder()
    objects: list[dict[str, Any]] = []
    index = 0

    while True:
        while index < len(text) and text[index].isspace():
            index += 1
        if index >= len(text):
            break

        obj, end = decoder.raw_decode(text, index)
        if not isinstance(obj, dict):
            raise ValueError(f"expected JSON object in {path}, got {type(obj).__name__}")
        objects.append(obj)
        index = end

    if not objects:
        raise ValueError(f"report does not contain any JSON objects: {path}")

    return objects


def parse_govulncheck(path: Path, exit_code: int) -> dict[str, Any]:
    events = parse_json_stream(path)
    config = next((event["config"] for event in events if "config" in event), {})
    sbom = next((event["SBOM"] for event in events if "SBOM" in event), {})
    progress = [event["progress"] for event in events if "progress" in event]
    osv_catalog = {event["osv"]["id"]: event["osv"] for event in events if "osv" in event}
    raw_findings = [event["finding"] for event in events if "finding" in event]

    findings = []
    grouped_findings: dict[tuple[str, str, str, str], dict[str, Any]] = {}
    module_counts: Counter[str] = Counter()
    package_counts: Counter[str] = Counter()

    for finding in raw_findings:
        trace = finding.get("trace") or []
        modules = [normalize_module_path(step.get("module", "")) for step in trace if step.get("module")]
        packages = [normalize_module_path(step.get("package", "")) for step in trace if step.get("package")]
        primary_module = modules[0] if modules else ""
        primary_package = packages[0] if packages else ""
        osv_id = str(finding.get("osv", ""))
        osv = osv_catalog.get(osv_id, {})

        if primary_module:
            module_counts[primary_module] += 1
        if primary_package:
            package_counts[primary_package] += 1

        fixed_version = str(finding.get("fixed_version", ""))
        summary = compact_text(str(osv.get("summary") or osv.get("details") or ""))
        finding_entry = {
            "osv": osv_id,
            "fixed_version": fixed_version,
            "module": primary_module,
            "package": primary_package,
            "summary": summary,
            "aliases": list(osv.get("aliases") or []),
        }
        findings.append(finding_entry)

        group_key = (osv_id, primary_module, primary_package, fixed_version)
        if group_key not in grouped_findings:
            grouped_findings[group_key] = {**finding_entry, "occurrences": 0}
        grouped_findings[group_key]["occurrences"] += 1

    findings.sort(key=lambda item: (item["osv"], item["module"], item["package"], item["fixed_version"]))
    grouped_preview = sorted(
        grouped_findings.values(),
        key=lambda item: (item["osv"], item["module"], item["package"], item["fixed_version"]),
    )

    modules = sbom.get("modules") or []
    scanner_name = str(config.get("scanner_name", "govulncheck"))
    scanner_version = str(config.get("scanner_version", ""))

    return {
        "report": str(path),
        "exit_code": exit_code,
        "scanner_name": scanner_name,
        "scanner_version": scanner_version,
        "database": str(config.get("db", "")),
        "database_last_modified": str(config.get("db_last_modified", "")),
        "go_version": str(config.get("go_version") or sbom.get("go_version") or ""),
        "scan_level": str(config.get("scan_level", "")),
        "scan_mode": str(config.get("scan_mode", "")),
        "events": len(events),
        "progress_updates": len(progress),
        "modules_in_sbom": len(modules),
        "findings": len(findings),
        "unique_vulnerabilities": len({item["osv"] for item in findings}),
        "unique_modules": len(module_counts),
        "unique_packages": len(package_counts),
        "unique_impacts": len(grouped_findings),
        "top_modules": [
            {"module": module, "findings": count}
            for module, count in module_counts.most_common(TOP_ENTRY_LIMIT)
        ],
        "findings_preview": grouped_preview[:TOP_ENTRY_LIMIT],
    }


def parse_gosec(path: Path, exit_code: int) -> dict[str, Any]:
    payload = read_json(path)
    raw_issues = payload.get("Issues") or []
    raw_golang_errors = payload.get("Golang errors") or {}
    stats = payload.get("Stats") or {}

    severity_counts: Counter[str] = Counter()
    confidence_counts: Counter[str] = Counter()
    rule_counts: Counter[str] = Counter()
    issues = []

    for issue in raw_issues:
        severity = str(issue.get("severity", "UNKNOWN")).upper()
        confidence = str(issue.get("confidence", "UNKNOWN")).upper()
        rule_id = str(issue.get("rule_id", "UNKNOWN"))
        path_value = normalize_report_path(str(issue.get("file", "")))
        line = safe_int(issue.get("line"))
        column = safe_int(issue.get("column"))
        cwe = issue.get("cwe") or {}
        cwe_id = str(cwe.get("id", ""))

        severity_counts[severity] += 1
        confidence_counts[confidence] += 1
        rule_counts[rule_id] += 1

        issues.append(
            {
                "severity": severity,
                "confidence": confidence,
                "rule_id": rule_id,
                "cwe_id": cwe_id,
                "details": compact_text(str(issue.get("details", ""))),
                "path": path_value,
                "line": line,
                "column": column,
            }
        )

    issues.sort(
        key=lambda item: (
            -SEVERITY_ORDER.get(item["severity"], 0),
            -CONFIDENCE_ORDER.get(item["confidence"], 0),
            item["path"],
            item["line"],
            item["column"],
            item["rule_id"],
        )
    )

    golang_errors_preview = []
    for key, value in raw_golang_errors.items():
        if isinstance(value, list):
            for message in value:
                golang_errors_preview.append({"package": str(key), "message": compact_text(str(message))})
        else:
            golang_errors_preview.append({"package": str(key), "message": compact_text(str(value))})
    golang_errors_preview.sort(key=lambda item: (item["package"], item["message"]))

    return {
        "report": str(path),
        "exit_code": exit_code,
        "issues": len(issues),
        "unique_files": len({item["path"] for item in issues if item["path"]}),
        "severity_counts": dict(sorted(severity_counts.items())),
        "confidence_counts": dict(sorted(confidence_counts.items())),
        "top_rules": [
            {"rule_id": rule_id, "count": count}
            for rule_id, count in rule_counts.most_common(TOP_ENTRY_LIMIT)
        ],
        "stats": stats,
        "golang_errors": len(golang_errors_preview),
        "golang_errors_preview": golang_errors_preview[:TOP_ENTRY_LIMIT],
        "issues_preview": issues[:TOP_ENTRY_LIMIT],
    }


def parse_sarif(path: Path, exit_code: int) -> dict[str, Any]:
    payload = read_json(path)
    runs = payload.get("runs") or []
    level_counts: Counter[str] = Counter()
    results_preview = []
    rule_count = 0
    result_count = 0

    for run in runs:
        driver = ((run.get("tool") or {}).get("driver") or {})
        rule_count += len(driver.get("rules") or [])

        for result in run.get("results") or []:
            result_count += 1
            level = str(result.get("level", "note")).lower()
            level_counts[level] += 1

            if len(results_preview) >= TOP_ENTRY_LIMIT:
                continue

            locations = result.get("locations") or []
            location = locations[0] if locations else {}
            physical = location.get("physicalLocation") or {}
            artifact = physical.get("artifactLocation") or {}
            region = physical.get("region") or {}
            message = result.get("message") or {}

            results_preview.append(
                {
                    "level": level,
                    "rule_id": str(result.get("ruleId", "")),
                    "path": normalize_report_path(str(artifact.get("uri", ""))),
                    "line": safe_int(region.get("startLine")),
                    "column": safe_int(region.get("startColumn")),
                    "message": compact_text(str(message.get("text", ""))),
                }
            )

    results_preview.sort(
        key=lambda item: (
            item["level"],
            item["path"],
            item["line"],
            item["column"],
            item["rule_id"],
        )
    )

    return {
        "report": str(path),
        "exit_code": exit_code,
        "runs": len(runs),
        "results": result_count,
        "rule_count": rule_count,
        "level_counts": dict(sorted(level_counts.items())),
        "results_preview": results_preview,
    }


def build_report(args: argparse.Namespace) -> dict[str, Any]:
    return {
        "generated_at_utc": datetime.now(timezone.utc).replace(microsecond=0).isoformat(),
        "govulncheck": parse_govulncheck(Path(args.govulncheck), args.govulncheck_exit_code),
        "gosec": parse_gosec(Path(args.gosec), args.gosec_exit_code),
        "sarif": parse_sarif(Path(args.sarif), args.sarif_exit_code),
    }


def render_markdown(report: dict[str, Any]) -> str:
    govulncheck = report["govulncheck"]
    gosec = report["gosec"]
    sarif = report["sarif"]

    lines = [
        "# Automated Security Review",
        "",
        f"Generated at `{report['generated_at_utc']}`.",
        "",
        "## Scanner Overview",
        "",
        "| Tool | Exit Code | Findings | Notes |",
        "| --- | ---: | ---: | --- |",
        (
            f"| govulncheck | {govulncheck['exit_code']} | {govulncheck['findings']} | "
            f"{govulncheck['unique_vulnerabilities']} unique vulnerabilities across "
            f"{govulncheck['unique_modules']} modules |"
        ),
        (
            f"| gosec (JSON) | {gosec['exit_code']} | {gosec['issues']} | "
            f"{gosec['unique_files']} files with findings |"
        ),
        (
            f"| gosec (SARIF) | {sarif['exit_code']} | {sarif['results']} | "
            f"{sarif['rule_count']} SARIF rules across {sarif['runs']} runs |"
        ),
        "",
    ]

    lines.extend(
        [
            "## govulncheck",
            "",
            "| Scanner Version | Go Version | Scan Level | Scan Mode | SBOM Modules |",
            "| --- | --- | --- | --- | ---: |",
            (
                f"| {table_escape(govulncheck['scanner_version'])} | {table_escape(govulncheck['go_version'])} | "
                f"{table_escape(govulncheck['scan_level'])} | {table_escape(govulncheck['scan_mode'])} | "
                f"{govulncheck['modules_in_sbom']} |"
            ),
            "",
        ]
    )

    if govulncheck["findings"] == 0:
        lines.extend(["No govulncheck findings were reported.", ""])
    else:
        lines.extend(
            [
                "### Top Affected Modules",
                "",
                "| Module | Findings |",
                "| --- | ---: |",
            ]
        )
        for item in govulncheck["top_modules"]:
            lines.append(f"| `{table_escape(item['module'])}` | {item['findings']} |")
        lines.extend(
            [
                "",
                "### Findings Preview",
                "",
                "| OSV | Fixed Version | Module | Package | Occurrences | Summary |",
                "| --- | --- | --- | --- | ---: | --- |",
            ]
        )
        for item in govulncheck["findings_preview"]:
            lines.append(
                f"| `{table_escape(item['osv'])}` | `{table_escape(item['fixed_version'])}` | "
                f"`{table_escape(item['module'])}` | `{table_escape(item['package'])}` | "
                f"{item['occurrences']} | "
                f"{table_escape(item['summary'])} |"
            )
        lines.append("")

    lines.extend(["## gosec", ""])

    if gosec["issues"] == 0:
        lines.extend(["No gosec issues were reported.", ""])
    else:
        lines.extend(
            [
                "### Severity Breakdown",
                "",
                "| Severity | Count |",
                "| --- | ---: |",
            ]
        )
        for severity, count in gosec["severity_counts"].items():
            lines.append(f"| {table_escape(severity)} | {count} |")

        lines.extend(
            [
                "",
                "### Top Rules",
                "",
                "| Rule | Count |",
                "| --- | ---: |",
            ]
        )
        for item in gosec["top_rules"]:
            lines.append(f"| `{table_escape(item['rule_id'])}` | {item['count']} |")

        lines.extend(
            [
                "",
                "### Findings Preview",
                "",
                "| Severity | Confidence | Rule | Location | Details |",
                "| --- | --- | --- | --- | --- |",
            ]
        )
        for item in gosec["issues_preview"]:
            location = f"{item['path']}:{item['line']}:{item['column']}"
            lines.append(
                f"| {table_escape(item['severity'])} | {table_escape(item['confidence'])} | "
                f"`{table_escape(item['rule_id'])}` | `{table_escape(location)}` | "
                f"{table_escape(item['details'])} |"
            )
        lines.append("")

    lines.extend(["## SARIF Overview", ""])

    if sarif["results"] == 0:
        lines.extend(["The SARIF export did not contain any results.", ""])
    else:
        lines.extend(
            [
                "| Level | Count |",
                "| --- | ---: |",
            ]
        )
        for level, count in sarif["level_counts"].items():
            lines.append(f"| {table_escape(level)} | {count} |")
        lines.extend(
            [
                "",
                "### SARIF Results Preview",
                "",
                "| Level | Rule | Location | Message |",
                "| --- | --- | --- | --- |",
            ]
        )
        for item in sarif["results_preview"]:
            location = f"{item['path']}:{item['line']}:{item['column']}"
            lines.append(
                f"| {table_escape(item['level'])} | `{table_escape(item['rule_id'])}` | "
                f"`{table_escape(location)}` | {table_escape(item['message'])} |"
            )
        lines.append("")

    if gosec["golang_errors"] > 0:
        lines.extend(["## gosec Import Errors", "", "| Package | Message |", "| --- | --- |"])
        for item in gosec["golang_errors_preview"]:
            lines.append(f"| `{table_escape(item['package'])}` | {table_escape(item['message'])} |")
        lines.append("")

    lines.extend(
        [
            "## Notes",
            "",
            "- Reports are generated even when findings exist; non-zero exit codes are preserved for debugging.",
            "- Paths are normalized to repository-relative form when possible for easier review.",
            "",
        ]
    )

    return "\n".join(lines)


def write_file(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def main() -> int:
    global MODULE_PREFIX

    args = parse_args()
    MODULE_PREFIX = load_module_prefix()

    report = build_report(args)
    markdown = render_markdown(report)

    write_file(Path(args.json_out), f"{json.dumps(report, indent=2)}\n")
    write_file(Path(args.md_out), markdown)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1)
