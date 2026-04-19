---
name: cliproxyapi-change-workflow
description: Implement or debug CLIProxyAPI changes with repository-specific architecture, safety constraints, and required Go validation steps.
---

Use this skill when changing proxy behavior, provider compatibility, OAuth/auth flows, request translation, or runtime execution in CLIProxyAPI.

## Goals

- Make minimal Go changes that match the existing architecture and conventions.
- Preserve compatibility across OpenAI, Gemini, Claude, and Codex compatible APIs.
- Validate formatting, tests, and the server build before finishing.

## Repository map

- `cmd/server/` — server entrypoint and CLI flags
- `internal/api/` — Gin routes, middleware, and modules
- `internal/api/modules/amp/` — Amp-style routes and reverse proxy behavior
- `internal/thinking/` — canonical thinking config pipeline; keep `ApplyThinking()` as suffix parse -> canonical normalize/validate -> provider-specific apply
- `internal/runtime/executor/` — provider executors only; place helpers in `internal/runtime/executor/helps/`
- `internal/translator/` — protocol translation layer; avoid standalone translator-only changes unless explicitly required and permission-checked
- `internal/registry/` — model registry and updater logic
- `internal/store/` — storage backends and secret resolution
- `internal/watcher/` — config hot-reload and watcher flows
- `sdk/cliproxy/` — embeddable SDK pipeline
- `test/` — cross-module integration coverage

## Required working style

1. Read nearby code first and match the established patterns in that package.
2. Keep changes small and direct; avoid unrelated refactors.
3. Use structured `logrus` logging, wrap errors with context, and never log secrets or tokens.
4. Do not use `log.Fatal` or `log.Fatalf`.
5. Avoid panics in HTTP handlers; return meaningful HTTP status codes instead.
6. After an upstream connection is established, do not introduce new network timeouts beyond the documented exceptions.

## High-risk areas

### Thinking pipeline

- Preserve the canonical `ThinkingConfig` representation in `internal/thinking/`.
- Do not bypass central validation or conversion by scattering provider-specific logic earlier in the flow.
- If suffix handling changes, ensure suffix-derived settings still override body-derived settings consistently.

### Translator layer

- Avoid changing only `internal/translator/` unless the task truly requires it.
- If a change is translator-only, confirm repository write permissions before proceeding.

### Runtime executors

- Keep executor packages focused on execution and their unit tests.
- Move shared helper code under `internal/runtime/executor/helps/`.

## Execution checklist

1. Identify the affected package(s) and read the nearest handlers, executors, or tests first.
2. Prefer updating existing tests near the touched code; add new tests only where the behavior is actually exercised.
3. Run targeted validation during iteration when possible:
   - `gofmt -w <changed-paths>`
   - `go test ./path/to/affected/pkg`
4. Before finishing, run repository-level validation:
   - `gofmt -w .`
   - `go test ./...`
   - `go build -o test-output ./cmd/server`
   - delete `test-output` after verifying the build succeeds
5. Summarize the behavior change, validation results, and any remaining risk.

## Common starting points

- New API behavior: start in `internal/api/` and trace into `internal/runtime/` or `internal/translator/`
- OAuth or auth bugs: inspect `auths/`, config loading, and store or secret resolution
- Model availability issues: inspect `internal/registry/`
- Hot reload or config problems: inspect `internal/watcher/` and management asset flow

## Done criteria

A change is not done until formatting, tests, and a server build all pass.
