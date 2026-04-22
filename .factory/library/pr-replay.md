# PR Replay Workflow

Approved upstream batch and replay guidance for this mission.

**What belongs here:** approved PR list, replay order, known overlaps, and replay-specific review guidance.
**What does NOT belong here:** step-by-step worker procedure (that belongs in the worker skill).

---

## Approved batch

1. `#2896` — `fix(claude): only reverse-remap OAuth tool names that were forward-renamed`
2. `#2895` — `fix(conductor): skip auth suspension for count_tokens 404s`
3. `#2923` — `Fix auth upload priority propagation`
4. `#2914` — `feat(access): per-api-key model allowlist with glob matching (v2, review fixes)`

## Approved release/tag additions

After the PR milestones above:

5. `v6.9.31` auth refresh ineffective-backoff
6. `v6.9.31` Codex streaming output backfill/perf follow-up
7. `v6.9.31` `HEAD /healthz`
8. `v6.9.32` Kimi K2.6 registry entry
9. `v6.9.32` GPT-Image-2 Codex image routes
10. `v6.9.33` OpenAI image handler `n` parameter removal (handling)
11. `v6.9.34` OpenAI image handler `n` parameter removal (references)

Evaluated and already satisfied locally:

- `v6.9.31` Host-header forwarding fix

## Replay order

- Replay and validate in the order above.
- Each PR gets its own milestone.
- Each approved `v6.9.31` addition gets its own milestone.
- Later PR work must be re-reviewed against the evolving local `dev` branch rather than assuming the upstream diff can be copied verbatim.

## Known local-state caveats

- Local `dev` may already contain related or partial equivalents for some approved PRs.
- Workers must check for existing local commits/diffs before replaying; if equivalent behavior already exists, add only the missing fixes/tests needed to satisfy the contract.

## High-risk slice

- `#2914` is the broadest change in the batch and touches config, API routing, management handlers, and concurrency-sensitive policy lookup logic.
- Treat `#2914` as the likeliest PR to require additional replay fixes or expanded regression coverage.
- `v6.9.31` Codex stream output is the highest-value post-PR release slice.
- `v6.9.31` auth refresh backoff overlaps `sdk/cliproxy/auth/conductor.go`, so it must be replayed carefully on top of the already sealed PR-2895 milestone.

## Deferred PRs

Do not pull work from these PRs into this mission unless the user explicitly expands scope:

- `#2926` — translator-only policy-risky fix
- `#2912` — larger overlapping auth/conductor rewrite
- `#2885` — larger overlapping auth suspension feature
