# Changelog

All notable changes to `ne0ark/CLIProxyAPI` are documented in this file.

This changelog covers the local `dev` branch of the ne0ark fork of
[router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI).
Each entry maps to a replay milestone (PRs or release tags from upstream)
that was validated and committed to `ne0ark/dev`.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

No unreleased changes at this time. All work-in-progress lives in open
milestone branches.

---

## [r6934-openai-image-n-param] — 2026-04-22

Replays upstream `v6.9.33` and `v6.9.34`.

### Fixed

- **OpenAI image handler: drop unsupported `n` parameter** — the image generation
  handler no longer reads, copies, or forwards the `n` (image count) field from
  incoming requests.
  - Upstream `v6.9.33` commit `fd71960c3eecf8a56a075dfd83eb275bcd93d9b1`:
    `fix(handlers): remove handling of unsupported n parameter in OpenAI image handlers`
  - Upstream `v6.9.34` commit `a188159632429b3400d5dadd2b0322afba60de3c`:
    `fix(handlers): remove references to unsupported n parameter in OpenAI image handlers`
  - **Local commit:** `badffeff95823f2bd9ff10bd12c140b92dd3421f`
    `fix(openai): drop unsupported image n parameter handling`

---

## [r6932-gpt-image-2] — 2026-04-22

Replays upstream `v6.9.32` (GPT-Image-2 Codex model).

### Added

- **GPT-Image-2 Codex image routes** — hardcoded GPT-Image-2 model support
  added to the Codex image generation path.
  - Upstream `v6.9.32` commit `e935196df43cb9af478fea377571873d07c9a39b`:
    `feat(models): add hardcoded GPT-Image-2 model support in Codex`
  - **Local commit:** `bf127a56`
    `feat(openai): replay GPT-Image-2 Codex image routes`

---

## [r6932-kimi-k2.6] — 2026-04-22

Replays upstream `v6.9.32` (Kimi K2.6 registry entry).

### Added

- **Kimi K2.6 model registry entry** — new model entry added to the model registry JSON.
  - Upstream `v6.9.32` commit `4fc2c619`:
    `feat(models): add Kimi K2.6 model entry to registry JSON`
  - **Local commit:** `3e458f58`
    `feat(models): replay Kimi K2.6 registry entry`

---

## [r6932-codex-output-patch-writes] — 2026-04-22

Replays upstream `v6.9.32` Codex perf improvement (already locally equivalent).

### Changed

- **Codex executor: avoid repeated output patch writes** — performance improvement
  preventing redundant patch write operations in the Codex stream executor.
  Local implementation already satisfied this invariant; equivalence recorded.
  - Upstream `v6.9.32` commit `b6781d69`:
    `perf(codex): avoid repeated output patch writes`
  - **Local validation commit:** `5ab12b7a`
    `chore(validation): record Codex patch-write perf equivalence`

---

## [r6931-healthz-head] — 2026-04-21

Replays upstream `v6.9.31` HEAD /healthz support.

### Added

- **HEAD /healthz readiness probe** — the `/healthz` endpoint now accepts both
  `GET` and bodyless `HEAD` requests, enabling standard Kubernetes-style readiness
  checks.
  - Upstream `v6.9.31` commit `1716a845`:
    `feat(api): add support for HEAD requests to /healthz endpoint`
  - **Local commit:** `bb07ca2c`
    `fix(api): support HEAD /healthz readiness probes`

---

## [r6931-codex-stream-output] — 2026-04-21

Replays upstream `v6.9.31` Codex streaming output backfill.

### Fixed

- **Codex executor: backfill streamed response output** — when the completed
  response omits final output items, the executor now reconstructs them from
  streamed `response.output_item.done` events in order, without duplicating
  authoritative completed output.
  - Upstream `v6.9.31` commit `bb8408ce`:
    `fix(codex): backfill streaming response output`
  - **Local commit:** `e78be8f1`
    `fix(codex): backfill streamed response output`

---

## [r6931-auth-refresh-backoff] — 2026-04-21

Replays upstream `v6.9.31` auth refresh backoff.

### Fixed

- **Auth refresh: back off ineffective refresh retries** — a technically successful
  token refresh that still leaves the auth immediately eligible for another refresh
  now inserts a throttle point instead of causing a tight refresh loop.
  - Upstream `v6.9.31` commit `e6866ff1`:
    `feat(auth): add refresh backoff for ineffective token updates`
  - **Local commit:** `b76ce52f`
    `fix(auth): back off ineffective refresh retries`

---

## [pr-2914] — 2026-04-20 / 2026-04-21

Replays upstream PR [#2914](https://github.com/router-for-me/CLIProxyAPI/pull/2914):
per-api-key model allowlist with glob matching.

### Added

- **Per-api-key model ACL** — API keys can now carry per-key model allowlist policies
  with glob pattern matching. The ACL is enforced at runtime across HTTP, WebSocket,
  and Amp provider alias routes (`/api/provider/:provider/`).
  Fail-closed for oversized or unreadable request bodies.
  - **Local commits:**
    - `3094474f` `feat(access): replay per-key model ACL`
    - `57d53acc` `fix(access): close ACL replay gaps`

---

## [pr-2923] — 2026-04-20

Replays upstream PR [#2923](https://github.com/router-for-me/CLIProxyAPI/pull/2923):
fix auth upload priority propagation.

### Fixed

- **Management auth upload: synthesize runtime fields** — uploading an auth file via
  the management API now correctly propagates `priority`, `prefix`, `proxy_url`,
  `disabled`, and `note` fields into the runtime auth record, achieving parity with
  watcher-based synthesis.
  - **Local commit:** `761ddf08`
    `fix(management): synthesize uploaded auth runtime state`

---

## [pr-2895] — 2026-04-19 / 2026-04-20

Replays upstream PR [#2895](https://github.com/router-for-me/CLIProxyAPI/pull/2895):
skip auth suspension for count_tokens 404s.

### Fixed

- **Auth conductor: skip suspension for count_tokens 404** — a `CountTokens` HTTP
  404 response no longer incorrectly suspends or cools down the auth. Other
  cooldown and result-recording behavior is unchanged.
  - **Local commits:**
    - `0d35e6ae` `fix(conductor): skip auth suspension for count_tokens 404s`
    - `ac2e1cfc` `test(auth): cover count_tokens cooldown replay`

---

## [pr-2896] — 2026-04-19 / 2026-04-20

Replays upstream PR [#2896](https://github.com/router-for-me/CLIProxyAPI/pull/2896):
only reverse-remap OAuth tool names that were forward-renamed.

### Fixed

- **Claude executor: preserve per-request OAuth tool restore flow** — only tool
  names that were explicitly forward-renamed during Claude OAuth preparation are
  eligible for reverse restoration on the response. This prevents spurious name
  mutations for tools that were never renamed.
  - Upstream commit `5ab9afac`:
    `fix(executor): handle OAuth tool name remapping with rename detection and add tests`
  - **Local commit:** `c830e8a9`
    `fix(claude): preserve per-request OAuth tool restore flow`

---

## [ne0ark-agent-readiness] — 2026-04-19 / 2026-04-20

ne0ark-only milestone: agent-readiness tooling and infrastructure setup.
Not replayed from upstream.

### Added

- Factory mission infrastructure (`.factory/` directory with skills, services manifest,
  library, and validation harness)
- Pre-commit hooks: PII check (`pii_check.py`), commit-msg lint, duplicate detection,
  dead code detection
- GitHub Actions CI: AGENTS.md freshness check, rolling branch releases, PR lint
- Linting configuration: `.golangci.yml`, `.golangci.dupl-sdk-api-handlers.yml`
- OpenAPI schema: `openapi.yaml` describing all built-in proxy routes
- Filter architecture documentation: `FILTER_ARCHITECTURE.md`
- Watcher diff split: `config_diff_providers.go`, `config_diff_sections.go`
  (reduces cyclomatic complexity of `config_diff.go`)
- Additional integration tests: `test/agents_md_freshness_test.go`,
  `test/openapi_schema_test.go`, `test/reasoning_effort_filter_test.go`
- **Local commits include:** `a9159e0a`, `c1f7e446`, `485042ff`, `f8454203`,
  `7852a08c`, `a311ecba`, `1052d530`, `10df8c37`, `fc004404`, `66d3a7a2`

---

[Unreleased]: https://github.com/ne0ark/CLIProxyAPI/compare/v6.9.34...HEAD
[r6934-openai-image-n-param]: https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.9.34
[r6932-gpt-image-2]: https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.9.32
[r6932-kimi-k2.6]: https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.9.32
[r6932-codex-output-patch-writes]: https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.9.32
[r6931-healthz-head]: https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.9.31
[r6931-codex-stream-output]: https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.9.31
[r6931-auth-refresh-backoff]: https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.9.31
[pr-2914]: https://github.com/router-for-me/CLIProxyAPI/pull/2914
[pr-2923]: https://github.com/router-for-me/CLIProxyAPI/pull/2923
[pr-2895]: https://github.com/router-for-me/CLIProxyAPI/pull/2895
[pr-2896]: https://github.com/router-for-me/CLIProxyAPI/pull/2896
[ne0ark-agent-readiness]: https://github.com/ne0ark/CLIProxyAPI/commits/dev
