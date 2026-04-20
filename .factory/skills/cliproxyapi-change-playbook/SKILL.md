---
name: CLIProxyAPI change playbook
description: Repo-specific guidance for choosing the correct subsystem, preserving architecture rules, and validating changes in CLIProxyAPI.
---

# CLIProxyAPI Change Playbook

Use this skill when you need to modify CLIProxyAPI and want a fast repo map before touching code. The goal is to help you land changes in the right package, avoid the architectural traps called out in `AGENTS.md`, and finish with the validation this repository expects.

## Start Here

1. Read `AGENTS.md` and the root `README.md`.
2. Confirm which external surface is changing:
   - OpenAI-compatible API
   - Gemini-compatible API
   - Claude-compatible API
   - Codex support
   - Amp CLI / IDE integration
   - SDK embedding
   - TUI / management API / storage / watchers
3. Trace the request path before editing:
   - entrypoint
   - API route or SDK entry
   - config / registry lookup
   - translator / executor
   - usage accounting / storage / websocket relay if involved

If you cannot name the exact subsystem you are changing, keep reading the repo map below before editing.

## Repo Map: Where Changes Usually Belong

### Entry and high-level wiring

- `cmd/server/` — process startup, flags, bootstrapping
- `sdk/cliproxy/` — embeddable service entry, builder, watchers, pipeline

### HTTP/API behavior

- `internal/api/` — Gin routes, handlers, middleware, modules
- `internal/api/modules/amp/` — Amp-specific routes, proxy behavior, provider route aliases
- `internal/managementasset/` — management assets and config snapshots

### Model selection, config, and runtime

- `internal/config/` — config loading and normalization
- `internal/registry/` — model registry and remote updater
- `internal/runtime/executor/` — provider-specific execution logic
- `internal/runtime/executor/helps/` — helper/support code for executors
- `internal/wsrelay/` — websocket relay sessions

### Protocol translation and provider shaping

- `internal/translator/` — wire-format translators and shared protocol conversion

### Cross-cutting behavior

- `internal/thinking/` — thinking/reasoning pipeline, suffix parsing, canonical `ThinkingConfig`, validation, provider-specific application
- `internal/store/` — storage backends, secret resolution
- `internal/cache/` — request signature caching
- `internal/usage/` — usage and token accounting
- `internal/watcher/` — hot reload and watcher logic
- `internal/tui/` — Bubble Tea terminal UI

## Hard Guardrails From This Repo

### 1) Do not make standalone translator-only changes lightly

`AGENTS.md` explicitly says not to make standalone changes to `internal/translator/` unless the task is part of a broader change. If your proposed change appears to live only there, stop and confirm that the surrounding API, config, registry, or executor behavior is also accounted for.

### 2) Preserve the thinking pipeline architecture

For `internal/thinking/`, keep this flow intact:

1. parse suffixes
2. normalize into the canonical `ThinkingConfig`
3. validate centrally
4. translate outward through provider-specific application

Do **not** bypass the canonical representation with ad-hoc provider branches.

### 3) Keep executor helpers out of executor root

`internal/runtime/executor/` should contain executors and executor unit tests only. Shared helper/supporting code belongs in `internal/runtime/executor/helps/`.

### 4) Respect the timeout rule

Timeouts are only allowed during credential acquisition. After the upstream connection is established, do not introduce new network timeouts except for the explicit exceptions listed in `AGENTS.md`.

### 5) Prefer safe server behavior

- avoid `log.Fatal` / `log.Fatalf`
- avoid panics in HTTP handlers
- use structured logrus logging
- never leak secrets or tokens in logs
- wrap errors with useful context

## Task-to-Subsystem Cheatsheet

### If the task is “add or adjust an API route”

Look first in:

- `internal/api/`
- `internal/api/modules/amp/` for Amp-specific endpoints or provider route aliases

Check:

- request binding and validation
- middleware interaction
- response shape
- whether the route chooses a model alias that later resolves through registry / executor logic

### If the task is “change how a model name or alias resolves”

Look first in:

- `internal/registry/`
- config loading/normalization under `internal/config/`

Check:

- local vs remote model registry behavior
- whether `--local-model` must prevent remote updates
- whether user-visible aliases overlap across providers

### If the task is “change provider request/response behavior”

Trace across:

- `internal/api/` or `sdk/cliproxy/`
- `internal/translator/`
- `internal/runtime/executor/`

Do not edit translation in isolation unless the surrounding routing and executor assumptions still hold.

### If the task is “change thinking / reasoning controls”

Work in:

- `internal/thinking/apply.go`
- `internal/thinking/suffix.go`
- `internal/thinking/types.go`
- `internal/thinking/validate.go`
- `internal/thinking/convert.go`

Checklist:

- suffix behavior still overrides correctly when required
- canonical config still represents the feature cleanly
- validation remains centralized
- provider-specific application happens after normalization

### If the task is “change OAuth, account, or secret storage”

Look first in:

- `internal/auth/`
- `internal/store/`
- management/API wiring if there is a user-facing flow

Check:

- on-disk auth material under `auths/`
- secret resolution behavior
- storage backend compatibility if file, Postgres, git store, or object store are involved

### If the task is “change websocket or long-lived session behavior”

Look first in:

- `internal/runtime/executor/` for provider session behavior
- `internal/wsrelay/` for relay session handling

Check:

- deadline rules
- lifecycle cleanup
- usage accounting

## Practical Change Workflow

1. **Read before editing**
   - `AGENTS.md`
   - `README.md`
   - the owning package of the change
2. **Find the smallest correct layer**
   - route only?
   - config only?
   - registry + executor?
   - thinking normalization + provider apply?
3. **Edit narrowly**
   - keep changes small
   - avoid unrelated refactors
4. **Verify neighboring behavior**
   - tests in the touched package
   - cross-module tests if the path spans API → registry → executor
5. **Run repo validation**
   - after Go changes: `gofmt -w .`
   - always run `go test ./...`
   - verify compile with `go build -o test-output ./cmd/server` and remove the artifact afterward

## Examples

### Example: add a new thinking flag for one provider family

Bad approach:

- add a one-off branch in a translator or executor
- skip canonical `ThinkingConfig`

Better approach:

1. extend canonical thinking types
2. update suffix parsing if needed
3. validate centrally
4. apply in provider-specific translation only after normalization

### Example: add a new Amp provider route alias

Look first in `internal/api/modules/amp/`, then verify:

- route shape matches Amp expectations
- model alias resolution still maps to the intended backend
- overlapping model names do not accidentally route to the wrong executor

### Example: add a helper used by multiple executors

Bad approach:

- drop the helper directly into `internal/runtime/executor/`

Better approach:

- place reusable support code in `internal/runtime/executor/helps/`
- keep executor package root focused on executor implementations and tests

## Final Pre-PR Checklist

- Did I edit the correct subsystem?
- Did I avoid translator-only changes unless they were part of a broader feature/fix?
- Did I preserve the canonical thinking pipeline if I touched thinking?
- Did I avoid forbidden timeout behavior?
- Did I keep executor helpers in `internal/runtime/executor/helps/`?
- Did I avoid leaking secrets or adding fatal exits/panics?
- Did I run the repo validation commands expected by `AGENTS.md`?

If any answer is “no” or “not sure”, inspect the owning package again before finishing.
