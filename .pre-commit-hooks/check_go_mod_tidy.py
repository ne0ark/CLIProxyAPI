#!/usr/bin/env python3
"""Verify go.mod and go.sum already match `go mod tidy` semantics."""

from __future__ import annotations

import difflib
import subprocess
import sys
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent.parent
MODULE_FILES = ("go.mod", "go.sum")


def read_file(path: Path) -> bytes | None:
    if not path.exists():
        return None
    return path.read_bytes()


def normalize(data: bytes | None) -> bytes:
    if data is None:
        return b""
    return data.replace(b"\r\n", b"\n")


def restore_file(path: Path, original: bytes | None) -> None:
    if original is None:
        path.unlink(missing_ok=True)
        return
    path.write_bytes(original)


def emit_diff(name: str, current: bytes | None, tidied: bytes | None) -> None:
    current_lines = normalize(current).decode("utf-8", errors="replace").splitlines(keepends=True)
    tidied_lines = normalize(tidied).decode("utf-8", errors="replace").splitlines(keepends=True)
    diff = difflib.unified_diff(
        current_lines,
        tidied_lines,
        fromfile=f"current/{name}",
        tofile=f"tidy/{name}",
    )
    for line in diff:
        sys.stderr.write(line)


def main() -> int:
    originals = {name: read_file(PROJECT_ROOT / name) for name in MODULE_FILES}

    try:
        result = subprocess.run(
            ["go", "mod", "tidy"],
            cwd=PROJECT_ROOT,
            capture_output=True,
            text=True,
            check=False,
        )

        if result.returncode != 0:
            print("go mod tidy failed.", file=sys.stderr)
            if result.stdout:
                print(result.stdout, file=sys.stderr, end="")
            if result.stderr:
                print(result.stderr, file=sys.stderr, end="")
            return result.returncode

        tidied = {name: read_file(PROJECT_ROOT / name) for name in MODULE_FILES}
        changed = False
        for name in MODULE_FILES:
            if normalize(originals[name]) == normalize(tidied[name]):
                continue
            if not changed:
                print(
                    "Go module files are not tidy. Run `go mod tidy` and commit the updated files.",
                    file=sys.stderr,
                )
            emit_diff(name, originals[name], tidied[name])
            changed = True

        if changed:
            return 1

        return 0
    finally:
        for name, original in originals.items():
            restore_file(PROJECT_ROOT / name, original)


if __name__ == "__main__":
    sys.exit(main())
