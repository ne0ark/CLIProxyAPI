# User Testing

Testing surfaces, validation tools, and concurrency guidance for this mission.

**What belongs here:** validation surfaces, tooling, setup notes, evidence expectations, and concurrency guidance.
**What does NOT belong here:** implementation details that belong in `architecture.md` or feature-specific instructions that belong in worker skills.

---

## Validation Surface

This mission is repository-centric rather than browser-centric.

### Surface: GitHub PR inspection

- Use `gh pr view`, `gh pr diff`, and related read-only GitHub inspection to confirm upstream intent, file scope, review comments, and PR status.
- Evidence should capture the upstream PR number, title, target branch, and changed-file scope.

### Surface: Local git replay state

- Use `git status`, `git diff`, `git log`, `git reflog`, and worker handoff command logs to verify replay happens on local `dev`, stays scoped, and does not touch review artifacts or off-limits branches/remotes.
- Evidence should show branch state, local diff scope, and the absence of unintended staged/committed files.

### Surface: Go validation

- Use targeted `go test` commands for touched packages first.
- Use disposable server compile verification after code changes.
- Use full-suite `go test ./...` before a milestone is considered merge-ready.
- For PR `#2914`, targeted `go test -race` on `internal/api`, `internal/api/handlers/management`, and `internal/config` is part of the expected validation surface.

## Validation Concurrency

### Repository/GitHub validation surface

- Max concurrent validators: `2`
- Rationale:
  - Machine headroom is high, but `go test` already parallelizes internally.
  - Full-suite runtime is short (~10 seconds in the dry run), so aggressive validator fan-out is unnecessary.
  - Keeping concurrency at `2` reduces cache/disk contention and avoids noisy overlap while still allowing efficient milestone validation.

## Expected evidence shape

- Command outputs for targeted package tests
- Command output for disposable `./cmd/server` build verification
- Command output for full `go test ./...`
- GitHub PR metadata for the replayed PR
- Local git diff/log evidence showing replay scope and branch hygiene
