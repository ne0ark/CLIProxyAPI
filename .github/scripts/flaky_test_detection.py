#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import shlex
import subprocess
import sys
import time
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parents[2]
MODULE_PREFIX = ""
TERMINAL_ACTIONS = {"pass", "fail", "skip"}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Run curated repeated go test probes or parse existing go test -json events, "
            "then report tests that both passed and failed across repetitions."
        )
    )
    parser.add_argument("--events-in", help="Path to an existing concatenated go test -json events file")
    parser.add_argument(
        "--events-out",
        help="Path to write concatenated go test -json events when running repeated probes",
    )
    parser.add_argument(
        "--package",
        action="append",
        dest="packages",
        default=[],
        help="Curated package target to probe (repeatable)",
    )
    parser.add_argument(
        "--repeat-count",
        type=int,
        default=1,
        help="Number of repeated executions to run per package when probes are executed",
    )
    parser.add_argument("--go-command", default="go", help="Go executable used to run repeated probes")
    parser.add_argument("--json-out", required=True, help="Path to the generated JSON report")
    parser.add_argument("--md-out", required=True, help="Path to the generated Markdown report")
    args = parser.parse_args()

    if not args.events_in and not args.packages:
        parser.error("provide --events-in or at least one --package")
    if args.packages and not args.events_out:
        parser.error("--events-out is required when --package is used to execute repeated probes")
    if args.repeat_count < 1:
        parser.error("--repeat-count must be at least 1")
    return args


def read_text(path: Path) -> str:
    raw = path.read_bytes()
    for encoding in ("utf-8", "utf-8-sig", "utf-16", "utf-16-le", "utf-16-be"):
        try:
            return raw.decode(encoding).lstrip("\ufeff")
        except UnicodeDecodeError:
            continue
    raise UnicodeDecodeError("unknown", raw, 0, 1, f"unable to decode {path}")


def load_module_prefix() -> str:
    go_mod = REPO_ROOT / "go.mod"
    if not go_mod.exists():
        return ""

    for line in go_mod.read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.removeprefix("module ").strip()
    return ""


def normalize_package_target(package_target: str) -> str:
    target = package_target.strip()
    if not target:
        return target

    if target.startswith("./"):
        relative = target[2:].strip("/")
        if not MODULE_PREFIX:
            return target
        if not relative:
            return MODULE_PREFIX
        return f"{MODULE_PREFIX}/{relative}"

    return target


def read_events(path: Path) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []

    for line_number, raw_line in enumerate(read_text(path).splitlines(), start=1):
        line = raw_line.strip()
        if not line:
            continue

        try:
            payload = json.loads(line)
        except json.JSONDecodeError as exc:
            raise ValueError(f"invalid JSON on line {line_number} of {path}: {exc}") from exc

        if not isinstance(payload, dict):
            raise ValueError(f"unsupported event payload on line {line_number} of {path}: {payload!r}")

        events.append(payload)

    return events


def run_repeated_probes(
    go_command: str,
    packages: list[str],
    repeat_count: int,
    events_out: Path,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    events: list[dict[str, Any]] = []
    run_results: list[dict[str, Any]] = []
    events_out.parent.mkdir(parents=True, exist_ok=True)

    with events_out.open("w", encoding="utf-8", newline="\n") as sink:
        for package_target in packages:
            for iteration in range(1, repeat_count + 1):
                command = [go_command, "test", "-json", "-count=1", "-shuffle=on", package_target]
                started_at = datetime.now(timezone.utc)
                start = time.perf_counter()
                process = subprocess.Popen(
                    command,
                    cwd=REPO_ROOT,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    text=True,
                    encoding="utf-8",
                    errors="replace",
                )

                parsed_events = 0
                observed_packages: set[str] = set()
                raw_output_lines = 0

                if process.stdout is None:
                    raise RuntimeError(f"failed to capture stdout for {' '.join(command)}")

                for raw_line in process.stdout:
                    sink.write(raw_line)
                    raw_output_lines += 1

                    line = raw_line.strip()
                    if not line:
                        continue

                    try:
                        payload = json.loads(line)
                    except json.JSONDecodeError:
                        continue

                    if not isinstance(payload, dict):
                        continue

                    events.append(payload)
                    parsed_events += 1

                    package_name = str(payload.get("Package", "")).strip()
                    if package_name:
                        observed_packages.add(package_name)

                exit_code = process.wait()
                duration_seconds = round(time.perf_counter() - start, 3)
                finished_at = datetime.now(timezone.utc)

                run_results.append(
                    {
                        "package_target": package_target,
                        "expected_package": normalize_package_target(package_target),
                        "iteration": iteration,
                        "command": command,
                        "command_display": shlex.join(command),
                        "started_at_utc": started_at.replace(microsecond=0).isoformat(),
                        "finished_at_utc": finished_at.replace(microsecond=0).isoformat(),
                        "duration_seconds": duration_seconds,
                        "exit_code": exit_code,
                        "raw_output_lines": raw_output_lines,
                        "parsed_events": parsed_events,
                        "observed_packages": sorted(observed_packages),
                    }
                )

    return events, run_results


def build_requested_package_runs(
    requested_packages: list[str],
    observed_packages: set[str],
    run_results: list[dict[str, Any]],
) -> tuple[list[dict[str, Any]], list[str]]:
    runs_by_target: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for result in run_results:
        runs_by_target[str(result["package_target"])].append(result)

    summaries = []
    missing = []

    for package_target in requested_packages:
        expected_package = normalize_package_target(package_target)
        package_runs = runs_by_target.get(package_target, [])
        observed = sorted(
            {
                observed_package
                for result in package_runs
                for observed_package in result.get("observed_packages", [])
            }
        )
        if not observed and expected_package in observed_packages:
            observed = [expected_package]

        run_passes = sum(1 for result in package_runs if int(result["exit_code"]) == 0)
        run_failures = sum(1 for result in package_runs if int(result["exit_code"]) != 0)

        if not observed:
            missing.append(package_target)

        summaries.append(
            {
                "package_target": package_target,
                "expected_package": expected_package,
                "observed_packages": observed,
                "runs_executed": len(package_runs),
                "run_passes": run_passes,
                "run_failures": run_failures,
                "durations_seconds": [round(float(result["duration_seconds"]), 3) for result in package_runs],
                "exit_codes": [int(result["exit_code"]) for result in package_runs],
            }
        )

    summaries.sort(key=lambda item: item["package_target"])
    return summaries, missing


def build_report(
    events: list[dict[str, Any]],
    requested_packages: list[str],
    repeat_count: int,
    run_results: list[dict[str, Any]],
    events_source: Path | None,
    go_command: str,
) -> dict[str, Any]:
    test_outcomes: dict[tuple[str, str], dict[str, int]] = defaultdict(
        lambda: {"pass": 0, "fail": 0, "skip": 0}
    )
    package_outcomes: dict[str, dict[str, Any]] = defaultdict(
        lambda: {
            "tests": set(),
            "test_terminal_passes": 0,
            "test_terminal_failures": 0,
            "test_terminal_skips": 0,
            "package_passes": 0,
            "package_failures": 0,
            "package_skips": 0,
            "requested_targets": set(),
        }
    )
    observed_packages: set[str] = set()

    expected_to_target = {
        normalize_package_target(package_target): package_target for package_target in requested_packages
    }

    for event in events:
        package_name = str(event.get("Package", "")).strip()
        if not package_name:
            continue

        observed_packages.add(package_name)
        package_entry = package_outcomes[package_name]
        if package_name in expected_to_target:
            package_entry["requested_targets"].add(expected_to_target[package_name])

        action = str(event.get("Action", "")).strip()
        test_name = str(event.get("Test", "")).strip()
        if action not in TERMINAL_ACTIONS:
            continue

        if test_name:
            package_entry["tests"].add(test_name)
            test_entry = test_outcomes[(package_name, test_name)]
            test_entry[action] += 1
            if action == "pass":
                package_entry["test_terminal_passes"] += 1
            elif action == "fail":
                package_entry["test_terminal_failures"] += 1
            elif action == "skip":
                package_entry["test_terminal_skips"] += 1
            continue

        if action == "pass":
            package_entry["package_passes"] += 1
        elif action == "fail":
            package_entry["package_failures"] += 1
        elif action == "skip":
            package_entry["package_skips"] += 1

    flaky_tests = []
    for (package_name, test_name), counts in test_outcomes.items():
        if counts["pass"] > 0 and counts["fail"] > 0:
            flaky_tests.append(
                {
                    "package": package_name,
                    "test": test_name,
                    "pass_count": counts["pass"],
                    "fail_count": counts["fail"],
                    "skip_count": counts["skip"],
                    "attempts": counts["pass"] + counts["fail"] + counts["skip"],
                }
            )

    flaky_tests.sort(
        key=lambda item: (
            -item["fail_count"],
            -item["pass_count"],
            item["package"],
            item["test"],
        )
    )

    flaky_tests_by_package: dict[str, int] = defaultdict(int)
    for item in flaky_tests:
        flaky_tests_by_package[item["package"]] += 1

    package_summaries = []
    for package_name, summary in package_outcomes.items():
        package_summaries.append(
            {
                "package": package_name,
                "requested_targets": sorted(summary["requested_targets"]),
                "unique_tests_observed": len(summary["tests"]),
                "test_terminal_passes": summary["test_terminal_passes"],
                "test_terminal_failures": summary["test_terminal_failures"],
                "test_terminal_skips": summary["test_terminal_skips"],
                "package_passes": summary["package_passes"],
                "package_failures": summary["package_failures"],
                "package_skips": summary["package_skips"],
                "flaky_tests": flaky_tests_by_package.get(package_name, 0),
            }
        )

    package_summaries.sort(key=lambda item: item["package"])
    requested_package_runs, missing_packages = build_requested_package_runs(
        requested_packages,
        observed_packages,
        run_results,
    )

    failing_runs = sum(1 for result in run_results if int(result["exit_code"]) != 0)
    summary = {
        "requested_packages": len(requested_packages),
        "requested_packages_without_events": len(missing_packages),
        "observed_packages": len(observed_packages),
        "runs_executed": len(run_results),
        "failing_runs": failing_runs,
        "unique_tests_observed": len(test_outcomes),
        "flaky_tests": len(flaky_tests),
        "packages_with_flaky_tests": len(flaky_tests_by_package),
        "clean": len(flaky_tests) == 0 and failing_runs == 0 and len(missing_packages) == 0,
    }

    return {
        "generated_at_utc": datetime.now(timezone.utc).replace(microsecond=0).isoformat(),
        "probe_configuration": {
            "go_command": go_command,
            "repeat_count": repeat_count,
            "requested_packages": requested_packages,
            "events_source": str(events_source) if events_source is not None else None,
            "command_template": f"{go_command} test -json -count=1 -shuffle=on <package>",
        },
        "summary": summary,
        "requested_package_runs": requested_package_runs,
        "packages": package_summaries,
        "flaky_tests": flaky_tests,
        "missing_requested_packages": missing_packages,
        "run_results": run_results,
    }


def render_markdown(report: dict[str, Any]) -> str:
    summary = report["summary"]
    config = report["probe_configuration"]
    requested_package_runs = report["requested_package_runs"]
    packages = report["packages"]
    flaky_tests = report["flaky_tests"]
    missing_requested_packages = report["missing_requested_packages"]

    lines = [
        "# Flaky Test Detection",
        "",
        f"_Generated: {report['generated_at_utc']}_",
        "",
        "This report repeats curated `go test -json -count=1 -shuffle=on` probes and flags tests that oscillate between passing and failing across runs.",
        "",
        "## Summary",
        "",
        "| Metric | Value |",
        "| --- | ---: |",
        f"| Requested packages | {summary['requested_packages']} |",
        f"| Observed packages | {summary['observed_packages']} |",
        f"| Repeated runs executed | {summary['runs_executed']} |",
        f"| Failing repeated runs | {summary['failing_runs']} |",
        f"| Unique tests observed | {summary['unique_tests_observed']} |",
        f"| Flaky tests detected | {summary['flaky_tests']} |",
        f"| Packages with flaky tests | {summary['packages_with_flaky_tests']} |",
        f"| Clean signal | {str(summary['clean']).lower()} |",
        "",
        "## Probe Configuration",
        "",
        f"- Command template: `{config['command_template']}`",
        f"- Repeat count: `{config['repeat_count']}`",
    ]

    if config["events_source"]:
        lines.append(f"- Events source: `{config['events_source']}`")

    lines.append("")

    if missing_requested_packages:
        lines.extend(
            [
                "## Missing Requested Packages",
                "",
                "The following requested package targets did not produce any parsed go test events:",
                "",
            ]
        )
        for package_target in missing_requested_packages:
            lines.append(f"- `{package_target}`")
        lines.append("")

    if requested_package_runs:
        lines.extend(
            [
                "## Requested Package Runs",
                "",
                "| Package target | Observed package | Runs | Failures |",
                "| --- | --- | ---: | ---: |",
            ]
        )
        for item in requested_package_runs:
            observed = ", ".join(item["observed_packages"]) if item["observed_packages"] else "—"
            lines.append(
                f"| `{item['package_target']}` | `{observed}` | {item['runs_executed']} | {item['run_failures']} |"
            )
        lines.append("")

    lines.extend(["## Flaky Tests", ""])

    if not flaky_tests:
        lines.append("No flaky tests were detected across the curated repeated probes.")
    else:
        lines.extend(
            [
                "| Package | Test | Passes | Fails | Attempts |",
                "| --- | --- | ---: | ---: | ---: |",
            ]
        )
        for item in flaky_tests:
            lines.append(
                f"| `{item['package']}` | `{item['test']}` | {item['pass_count']} | {item['fail_count']} | {item['attempts']} |"
            )

    lines.extend(["", "## Observed Package Summary", ""])

    if not packages:
        lines.append("No package-level go test events were parsed.")
    else:
        lines.extend(
            [
                "| Package | Unique tests | Test fails | Package run fails | Flaky tests |",
                "| --- | ---: | ---: | ---: | ---: |",
            ]
        )
        for item in packages:
            lines.append(
                f"| `{item['package']}` | {item['unique_tests_observed']} | {item['test_terminal_failures']} | {item['package_failures']} | {item['flaky_tests']} |"
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

    events_in = Path(args.events_in) if args.events_in else None
    events_out = Path(args.events_out) if args.events_out else None
    json_out = Path(args.json_out)
    md_out = Path(args.md_out)

    if events_in is not None:
        events = read_events(events_in)
        run_results: list[dict[str, Any]] = []
    else:
        assert events_out is not None
        events, run_results = run_repeated_probes(
            args.go_command,
            args.packages,
            args.repeat_count,
            events_out,
        )

    report = build_report(
        events,
        args.packages,
        args.repeat_count,
        run_results,
        events_out if events_out is not None else events_in,
        args.go_command,
    )

    json_out.parent.mkdir(parents=True, exist_ok=True)
    json_out.write_text(f"{json.dumps(report, indent=2)}\n", encoding="utf-8")
    write_file(md_out, render_markdown(report))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1)
