# Delta Doc Workflow

Authoring guidance for the ne0ark ↔ router-for-me divergence documentation.

**What belongs here:** source-of-truth policy for how the CHANGELOG and upstream-delta wiki should be structured, where data comes from, and how accuracy is enforced.
**What does NOT belong here:** step-by-step worker procedure (that belongs in `.factory/skills/delta-doc-worker/SKILL.md`).

---

## Deliverables

- `CHANGELOG.md` at repo root — keep-a-changelog style, newest first, human readable.
- `docs/wiki/upstream-delta.md` — categorized, auditable divergence inventory with full SHAs and tag citations.

## Source of truth

- `upstream/dev` (router-for-me) — authoritative upstream history.
- `origin/dev` + local `dev` (ne0ark) — authoritative local history.
- Upstream tag series `v6.9.28 … v6.9.34` (extend as upstream publishes more).
- `.factory/validation/*/` directories — authoritative local milestone list.
- `.factory/library/pr-replay.md` — authoritative rationale for deferred/rejected upstream items.

## Data format rules

- Every upstream commit citation must use **full 7+ char SHA** and link to `https://github.com/router-for-me/CLIProxyAPI/commit/<full-sha>`.
- Every local commit citation must use the matching short SHA and link to `https://github.com/ne0ark/CLIProxyAPI/commit/<full-sha>`.
- Tag mentions use the exact tag string (`v6.9.33`).
- Never invent a SHA; verify with `git cat-file -e <sha>`.

## Categorization taxonomy

Every file-level or commit-level divergence falls into exactly one bucket:

1. **Replayed** — upstream commit/behavior exists locally, with a citable local commit.
2. **Deferred** — upstream commit exists, not yet replayed locally, deliberately parked. Cite the rationale from `.factory/library/pr-replay.md` if available.
3. **Rejected** — upstream commit exists, explicitly not planned to land locally (e.g., translator-only refactors blocked by AGENTS.md guardrails).
4. **ne0ark-only addition** — local file/commit has no upstream counterpart and should stay that way (factory framework, agent-readiness hooks, etc.).
5. **Drift** — file exists on both sides but content differs intentionally (e.g., local carries additional tests that never made it upstream).

## Accuracy guardrails

- The worker must fact-check SHAs and tags before commit.
- The worker must not edit Go source, config, or tests while authoring delta docs; validation is the usual `go test` suite plus `git diff --name-only` scoped to `CHANGELOG.md` and `docs/wiki/`.
- The worker must not speculate about upstream intent — if the mapping between upstream commit and local commit is ambiguous, categorize it as `Drift` with a brief factual note and hand back to the orchestrator.

## Update cadence

- Update the changelog and wiki on every replay milestone as part of the mission's dedicated doc milestone.
- Do not update these files inside replay milestones themselves (keep replay commits narrowly scoped).
