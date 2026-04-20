# Architecture

How the affected parts of CLIProxyAPI fit together for this mission.

**What belongs here:** high-level components, relationships, data flow, invariants, and the subsystem boundaries touched by the approved PR batch.
**What does NOT belong here:** feature task lists, validation assertions, or shell command recipes.

---

## Mission-level architecture

This mission is not building a new product surface. It is reviewing and replaying approved upstream behavior onto the local `dev` branch of an existing Go monorepo while preserving repository architecture and branch hygiene.

At a high level, each PR replay follows the same flow:

1. Inspect the upstream PR intent and changed-file surface.
2. Compare that intent to the current local `dev` state.
3. Apply only the accepted delta that is still missing on local `dev`.
4. Prove the resulting behavior through targeted tests plus repository validation.

The important invariant is that the replay is behavior-driven, not patch-driven: if local `dev` already contains some or all of the intended behavior, workers should keep the local architecture and add only the missing deltas needed to satisfy the validation contract.

## Authoritative review sources

- GitHub PR metadata, changed-file lists, comments, and diffs are the authoritative source of upstream intent.
- Local review artifacts such as `pr2914.diff` and `pr2914.utf8.diff` may help explain prior investigation but are not canonical inputs for replay decisions and must remain outside committed diffs.

## Subsystems touched by the approved batch

### PR 2896 — Claude OAuth tool-name remap path

- Main area: `internal/runtime/executor/claude_executor.go`
- Validation area: `internal/runtime/executor/claude_executor_test.go`

This slice lives in the Claude executor and governs how client-visible tool names are translated for Claude OAuth requests and restored on responses/streams. The core invariant is per-request symmetry: only names that were forward-renamed may be reverse-restored.

Workers should treat this as a multi-surface executor slice, not a single helper tweak. The same remap contract must hold across:

- non-stream responses
- translated stream responses
- passthrough stream responses
- count-tokens preparation
- `tool_use` and `tool_reference` name restoration

The ordering invariant is important: request preparation must perform rename before prefixing, and response handling must remove prefixes before restoring the client-visible name.

### PR 2895 — auth conductor count-tokens cooldown path

- Main area: `sdk/cliproxy/auth/conductor.go`
- Validation area: `sdk/cliproxy/auth/conductor_overrides_test.go`

This slice lives in auth selection and cooldown accounting. The core invariant is that `count_tokens` 404 behavior must not incorrectly suspend an auth, while other cooldown/result-recording behavior remains intact.

The exception is intentionally narrow: only `CountTokens` HTTP 404 behavior is exempted from suspension/cooldown. Normal execute-path 404 handling and non-404 count-tokens cooldown/result-recording behavior remain part of the existing architecture and must keep working.

### PR 2923 — management auth upload synthesis path

- Main area: `internal/api/handlers/management/auth_files.go`
- Supporting synthesis area: `internal/watcher/synthesizer/`

This slice sits at the boundary between management uploads and runtime auth registration. The important relationship is that upload-time auth rebuild must populate the same runtime-relevant fields that watcher-based synthesis would provide for a single uploaded auth record.

For this mission, the worker should reason from concrete propagated runtime fields, not a vague “synthesis parity” idea. The key immediate fields are:

- `priority`
- `prefix`
- `proxy_url`
- `disabled`
- `note`
- custom headers / derived runtime attributes

### PR 2914 — per-key model ACL path

- Route wiring: `internal/api/server.go`
- ACL logic: `internal/api/model_acl.go`
- Management sync: `internal/api/handlers/management/config_lists.go`
- Policy/config state: `internal/config/sdk_config.go`

This is the widest slice in the mission. It joins together:

- management writes of API keys and policies
- config-layer storage and lookup of per-key policies
- runtime ACL checks on `/v1` and `/v1beta` routes
- HTTP and websocket enforcement behavior

The core invariant is that per-key restrictions behave consistently across management persistence, config lookup, and runtime enforcement.

Important ownership/data-flow boundaries in this slice:

- management handlers accept and normalize client payloads for API keys and policies
- config state owns durable policy lookup and invalidation behavior
- ACL middleware enforces the same restriction semantics on HTTP JSON routes, Gemini-style `/v1beta/models/...` routes, and websocket upgrades

Key invariants for workers to preserve:

- default-policy fallback behavior, especially `deny-all`
- request-body preservation after model inspection
- fail-closed behavior for oversized or unreadable request bodies
- coherent policy reads after mutation, including concurrency-sensitive paths

## Cross-cutting invariants

### Branch and replay invariants

- All replay work stays on local `dev`.
- Only `origin/dev` is a valid publication target.
- Review artifacts `pr2914.diff` and `pr2914.utf8.diff` must remain outside committed diffs.

### Validation invariants

- Each PR replay must be justified against upstream intent and current local `dev`.
- Each milestone is PR-specific and validated independently.
- Full-suite repo validation is required after targeted package validation.

### Repository architecture invariants

- Respect the guidance in `AGENTS.md`, especially the executor helper boundary, timeout rules, and translator-change restrictions.
- Avoid unrelated refactors while replaying upstream behavior.
- Prefer narrow fixes with regression tests over broad structural changes.
