#!/usr/bin/env python3
"""Pre-commit hook to block PII in commit messages."""
import re
import sys

PII_PATTERNS = [
    (re.compile(r'\b\d{10,15}\b'), 'phone number'),
    (
        re.compile(
            r"^Co-authored-by:\s+(?![^<]*\[bot\]\b)(?:[A-Za-z][A-Za-z'.-]*\s+){1,}[A-Za-z][A-Za-z'.-]*\s+<[^>]+>$"
        ),
        'human-looking co-authored-by name',
    ),
    (re.compile(r'<\d{10,}'), 'numeric-only email (possible phone number)'),
]


def main():
    if len(sys.argv) < 2:
        print("Usage: pii_check.py <commit-msg-file>", file=sys.stderr)
        return 1

    msg_file = sys.argv[1]
    with open(msg_file, "r", encoding="utf-8", errors="replace") as f:
        message = f.read()

    violations = []
    for i, line in enumerate(message.splitlines(), 1):
        for pattern, label in PII_PATTERNS:
            if pattern.search(line):
                violations.append(f"  line {i}: [{label}] {line.strip()}")

    if violations:
        print("PII detected in commit message:", file=sys.stderr)
        for v in violations:
            print(v, file=sys.stderr)
        print(
            "\nRemove personal information before committing.",
            file=sys.stderr,
        )
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
