---
name: delta-doc-worker
description: Author an auditable changelog and upstream-delta wiki for ne0ark/CLIProxyAPI without touching Go code.
---

# Delta Doc Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the work procedure for producing the ne0ark↔router-for-me divergence documentation.

## When to Use This Skill

Use this skill for features that:

- inspect the full git history of local `dev` and upstream `dev` (plus tags)
- produce or update keep-a-changelog-style documentation
- catalog intentional ne0ark-only additions that do not exist upstream
- catalog upstream commits replayed locally with their matching local commit SHAs
- must **not** touch any Go source file

This worker is documentation-only. It never changes runtime behavior, never moves files that code depends on, and never edits `internal/`, `cmd/`, `sdk/`, or `test/`.

## Required Skills

- `CLIProxyAPI change playbook` — invoke only to confirm which subsystem each upstream commit touched so the delta doc lands the attribution in the correct section.
- `review` — invoke after the draft doc is ready and before final validation to catch factual mistakes (wrong SHA, wrong tag, wrong remote, missing categories).

## Windows Compatibility Notes

- Git is available on PATH as `git`. GitHub CLI is available as `gh` and is authenticated as `ne0ark`.
- If `gh` cannot reach `router-for-me/CLIProxyAPI` for a specific tag, fall back to `git show <sha>` or `git log <tag>` locally since `upstream` is already fetched.
- Prefer absolute repo paths when reading or writing files from tool calls.

## Work Procedure

1. **Establish ground truth**
   - Record `git rev-parse HEAD` on `dev` and the full upstream tag list via `git tag --sort=-version:refname`.
   - Record upstream `dev` tip SHA (`git rev-parse upstream/dev`).
   - Enumerate local replay milestones by grepping `git log origin/dev --grep='^feat\|^fix\|^perf\|^chore(validation)'` or by reading `.factory/validation/*/` directories.

2. **Map upstream → local replay**
   - For each upstream tag from `v6.9.28` through the latest fetched tag, list upstream commits inside that tag range.
   - For each upstream commit in scope, locate the matching local commit (search by subject or diff). If no match, categorize as "deferred" or "rejected".
   - Cite the **upstream short SHA**, **upstream subject**, **upstream tag**, **local short SHA**, **local subject** for each replayed item.

3. **Catalog ne0ark-only divergence**
   - Use `git diff --name-status dev..upstream/dev` and `git diff --name-status upstream/dev..dev` to enumerate file-level differences.
   - Categorize each divergent file as:
     - **ne0ark-only addition** (exists locally, absent upstream): factory mission infrastructure, agent-readiness workflows, PII hook, split watcher diff files, openapi.yaml, etc.
     - **upstream-only file** (exists upstream, absent locally): deletions of files intentionally kept in ne0ark, or additions not yet replayed.
     - **replayed with drift**: same file name but content diverges because ne0ark carries more tests / additional validation.

4. **Produce the artifacts**
   - Write `CHANGELOG.md` at repo root in [Keep a Changelog](https://keepachangelog.com) format with sections per local milestone (newest first). Each entry links to the upstream commit on `router-for-me/CLIProxyAPI` using the full SHA.
   - Write `docs/wiki/upstream-delta.md` organized by category:
     1. Summary table (upstream tag ↔ local milestone ↔ local SHA ↔ status)
     2. Upstream commits replayed on ne0ark (ordered by upstream date)
     3. ne0ark-only additions (factory framework, validation harness, CI, hooks, docs)
     4. Upstream items intentionally deferred or rejected (cite the rationale from `.factory/library/pr-replay.md` where available)
     5. File-level divergence inventory (condensed)

5. **Fact-check before commit**
   - Every cited SHA must exist (`git cat-file -e <sha>`).
   - Every cited tag must resolve (`git rev-parse <tag>`).
   - Every claimed local commit must actually be on `origin/dev` or `HEAD` branch `dev`.
   - Cross-reference at least two upstream ↔ local mappings by reading the diffs.

6. **Validation**
   - Markdown lint is not wired; instead run a targeted spot check: open the first and last five entries in both files and verify formatting (headers, bullet indentation, link shape).
   - `go test -count=1 -p 16 ./...` to confirm no Go test regressed (the worker does not touch Go, so the suite should be unchanged; run it anyway to demonstrate isolation).
   - Disposable server compile verification: `go build -o test-output.exe ./cmd/server; if (Test-Path test-output.exe) { Remove-Item test-output.exe }`.
   - `git diff --name-only` must contain only `CHANGELOG.md`, `docs/wiki/upstream-delta.md`, and (optionally) a new `docs/wiki/` index file; any other touched file is out of scope and must be reverted.

7. **Adversarial review**
   - Invoke `review` on the final local diff.
   - If review is unavailable, use the `worker` subagent or perform a manual review by checking each section for wrong SHAs, wrong tags, and missing categories.
   - Fix any factual or formatting issues and re-run validation.

8. **Prepare the local commit**
   - Stage only `CHANGELOG.md` and `docs/wiki/` paths.
   - Review `git diff --cached` for unrelated edits before committing.
   - Commit message follows the mission format: `docs(changelog): document ne0ark↔router-for-me delta through vX.Y.Z`.

9. **Return a detailed handoff**
   - Include: upstream tip SHA, local tip SHA, list of files written, number of replayed milestones documented, number of ne0ark-only items documented, number of deferred/rejected items, every validation command run.

## Example Handoff

```json
{
  "salientSummary": "Authored CHANGELOG.md and docs/wiki/upstream-delta.md capturing every local replay milestone from v6.9.28 through v6.9.34 plus the ne0ark-only factory/tooling divergence, validated with full go test, disposable compile, and a review pass. No Go code changed.",
  "whatWasImplemented": "Created repo-root CHANGELOG.md in keep-a-changelog format with one entry per replay milestone citing upstream SHA and local SHA, and created docs/wiki/upstream-delta.md with a summary table plus four categorized sections covering replayed upstream work, ne0ark-only additions, deferred/rejected upstream items, and the file-level divergence inventory.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {
        "command": "git log --oneline upstream/dev ^dev",
        "exitCode": 0,
        "observation": "Enumerated upstream-only commits and confirmed each one is either replayed, deferred, or rejected in the delta doc."
      },
      {
        "command": "go test -count=1 -p 16 ./...",
        "exitCode": 0,
        "observation": "Full repo test suite still passes; no Go file changed."
      },
      {
        "command": "go build -o test-output.exe ./cmd/server; if (Test-Path test-output.exe) { Remove-Item test-output.exe }",
        "exitCode": 0,
        "observation": "Disposable server compile verification passed."
      }
    ],
    "interactiveChecks": [
      {
        "action": "Spot-checked five upstream SHAs cited in the doc",
        "observed": "All resolved via git cat-file -e and matched the documented subject line."
      },
      {
        "action": "Ran review skill on the final doc diff",
        "observed": "No factual mistakes remained after the final edit pass."
      }
    ]
  },
  "tests": {
    "added": [],
    "coverage": "Documentation-only change; targeted verification is factual (SHA/tag existence) rather than behavioral."
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- The upstream and local histories disagree in a way that suggests a silent divergence not captured by any existing milestone.
- The requested doc would require touching Go source or config to achieve (scope creep).
- An upstream commit in the "replayed" list actually has no matching local commit.
- A claimed local "replay" commit does not actually implement the upstream behavior.
