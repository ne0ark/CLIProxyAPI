# Upstream Delta: ne0ark/CLIProxyAPI ↔ router-for-me/CLIProxyAPI

This document is the authoritative record of how `ne0ark/dev` (this fork) relates to
`router-for-me/CLIProxyAPI/dev` (upstream). It covers every replay milestone, ne0ark-only
additions, deferred upstream work, and the file-level divergence inventory.

**Upstream remote:** `upstream` → `https://github.com/router-for-me/CLIProxyAPI.git`  
**Fork remote:** `origin` → `https://github.com/ne0ark/CLIProxyAPI.git`  
**Coverage range:** upstream `v6.9.28` through `v6.9.34`  
**Document updated:** 2026-04-22  
**Local HEAD:** `badffeff95823f2bd9ff10bd12c140b92dd3421f`  
**Upstream HEAD:** `a188159632429b3400d5dadd2b0322afba60de3c` (tip of upstream/dev, = v6.9.34)

---

## 1. Summary Mapping Table

Each row maps an upstream tag to the local milestone commit that replayed its core behavior,
the local commit SHA, and the current status.

| Upstream Tag | Upstream Commit(s) Replayed | Local Milestone | Local Commit SHA | Status |
|---|---|---|---|---|
| `v6.9.28` (PR #2896) | `5ab9afac` fix(executor): handle OAuth tool name remapping | pr-2896 | `c830e8a9` | Replayed |
| `v6.9.28` (PR #2895) | `0ea76801` fix(auth): honor disable-cooling and enrich no-auth errors | pr-2895 | `0d35e6ae`, `ac2e1cfc` | Replayed |
| `v6.9.28` (PR #2923) | `894baad8` feat(api): integrate auth index into key retrieval | pr-2923 | `761ddf08` | Replayed |
| `v6.9.28` (PR #2914) | (per-key model ACL surface) | pr-2914 | `3094474f`, `57d53acc` | Replayed |
| `v6.9.31` auth backoff | `e6866ff1` feat(auth): add refresh backoff for ineffective token updates | r6931-auth-refresh-backoff | `b76ce52f` | Replayed |
| `v6.9.31` Codex stream | `bb8408ce` fix(codex): backfill streaming response output | r6931-codex-stream-output | `e78be8f1` | Replayed |
| `v6.9.31` Codex perf | `b6781d69` perf(codex): avoid repeated output patch writes | r6932-codex-output-patch-writes | `5ab12b7a` (validation) | Replayed |
| `v6.9.31` HEAD /healthz | `1716a845` feat(api): add support for HEAD requests to /healthz | r6931-healthz-head | `bb07ca2c` | Replayed |
| `v6.9.32` Kimi K2.6 | `4fc2c619` feat(models): add Kimi K2.6 model entry to registry JSON | r6932-kimi-k2.6 | `3e458f58` | Replayed |
| `v6.9.32` GPT-Image-2 | `e935196d` feat(models): add hardcoded GPT-Image-2 model support in Codex | r6932-gpt-image-2 | `bf127a56` | Replayed |
| `v6.9.33` n-param drop | `fd71960c` fix(handlers): remove handling of unsupported n parameter | r6934-openai-image-n-param | `badffeff` | Replayed |
| `v6.9.34` n-param refs | `a1881596` fix(handlers): remove references to unsupported n parameter | r6934-openai-image-n-param | `badffeff` | Replayed (combined) |
| `v6.9.29` | `9886c916` (Merge PR #2892 fix-provider) | — | — | Deferred (auth-index surface; not approved) |
| `v6.9.30` | `d0f73ab7` (VisionCoder sponsorship doc) | — | — | Deferred (docs/sponsor only) |

---

## 2. Upstream Commits Replayed on ne0ark/dev

Listed in upstream chronological order. Each entry cites the upstream commit SHA,
the upstream subject, the upstream tag at which it shipped, and the corresponding
local commit.

### PR #2896 — Claude OAuth tool-name remap path

**Upstream SHA:** `5ab9afac` (upstream/dev)  
**Upstream subject:** `fix(executor): handle OAuth tool name remapping with rename detection and add tests`  
**Shipped in upstream tag:** `v6.9.28`  
**Local commit:** `c830e8a9` — `fix(claude): preserve per-request OAuth tool restore flow` (2026-04-20)  
**Files touched locally:** `internal/runtime/executor/claude_executor.go`,
`internal/runtime/executor/claude_executor_test.go`  
**Notes:** Upstream PR reviewed and replayed as a targeted fix ensuring only
forward-renamed tool names are reverse-restored in Claude OAuth flows.
Local commit message cites upstream PR #2896.

### PR #2895 — auth conductor count-tokens cooldown path

**Upstream SHA:** `0ea76801` — `fix(auth): honor disable-cooling and enrich no-auth errors`  
**Shipped in upstream tag:** `v6.9.28`  
**Local commits:**  
- `0d35e6ae` — `fix(conductor): skip auth suspension for count_tokens 404s` (2026-04-19)  
- `ac2e1cfc` — `test(auth): cover count_tokens cooldown replay` (2026-04-20)  
**Files touched locally:** `sdk/cliproxy/auth/conductor.go`,
`sdk/cliproxy/auth/conductor_overrides_test.go`  
**Notes:** Upstream exempted `CountTokens` HTTP 404 from auth suspension. Local
replay adds dedicated test coverage.

### PR #2923 — management auth upload synthesis

**Upstream SHA:** (management auth-index stabilization, `c26936e2`, `894baad8`)  
**Shipped in upstream tag:** `v6.9.28`  
**Local commit:** `761ddf08` — `fix(management): synthesize uploaded auth runtime state` (2026-04-20)  
**Files touched locally:** `internal/api/handlers/management/auth_files.go`,
`internal/watcher/synthesizer/` (supporting synthesis path)  
**Notes:** Upstream PR fixed auth-index mapping and exposed runtime fields on upload;
local replay replicates field synthesis (`priority`, `prefix`, `proxy_url`, `disabled`,
`note`).

### PR #2914 — per-api-key model allowlist

**Upstream SHAs:** (distributed across management + config + ACL surface)  
**Shipped in upstream tag:** `v6.9.28`  
**Local commits:**  
- `3094474f` — `feat(access): replay per-key model ACL` (2026-04-20)  
- `57d53acc` — `fix(access): close ACL replay gaps` (2026-04-21)  
**Files touched locally:** `internal/api/model_acl.go`, `internal/api/model_acl_test.go`,
`internal/api/server.go`, `internal/api/handlers/management/config_lists.go`,
`internal/config/sdk_config.go`  
**Notes:** Widest slice in the mission. Joins management writes, config-layer policy
lookup, and runtime ACL enforcement across HTTP, WebSocket, and Amp alias routes.

### v6.9.31 — auth refresh ineffective-backoff

**Upstream SHA:** `e6866ff1` — `feat(auth): add refresh backoff for ineffective token updates`  
**Shipped in upstream tag:** `v6.9.31`  
**Local commit:** `b76ce52f` — `fix(auth): back off ineffective refresh retries` (2026-04-21)  
**Files touched locally:** `sdk/cliproxy/auth/conductor.go`  
**Notes:** Prevents tight refresh loop when a successful token refresh still leaves the
auth immediately re-eligible.

### v6.9.31 — Codex streaming output backfill

**Upstream SHA:** `bb8408ce` — `fix(codex): backfill streaming response output`  
**Shipped in upstream tag:** `v6.9.31`  
**Local commit:** `e78be8f1` — `fix(codex): backfill streamed response output` (2026-04-21)  
**Files touched locally:** `internal/runtime/executor/codex_executor.go`,
`internal/runtime/executor/codex_executor_stream_output_test.go`  
**Notes:** When the completed response omits final output items, the executor
reconstructs them from streamed `response.output_item.done` events.

### v6.9.31 — HEAD /healthz

**Upstream SHA:** `1716a845` — `feat(api): add support for HEAD requests to /healthz endpoint`  
**Shipped in upstream tag:** `v6.9.31`  
**Local commit:** `bb07ca2c` — `fix(api): support HEAD /healthz readiness probes` (2026-04-21)  
**Files touched locally:** `internal/api/server.go`  
**Notes:** Readiness checks must accept both `GET /healthz` and bodyless `HEAD /healthz`.

### v6.9.32 — Codex repeated-output-patch-writes perf

**Upstream SHA:** `b6781d69` — `perf(codex): avoid repeated output patch writes`  
**Shipped in upstream tag:** `v6.9.32` (part of the Merge PR #2939 batch)  
**Local commit:** `5ab12b7a` — `chore(validation): record Codex patch-write perf equivalence` (2026-04-22)  
**Files touched locally:** `internal/runtime/executor/codex_executor.go` (already matched locally)  
**Notes:** Local implementation already satisfied the perf invariant; validation recorded
equivalence rather than applying a fresh patch.

### v6.9.32 — Kimi K2.6 registry entry

**Upstream SHA:** `4fc2c619` — `feat(models): add Kimi K2.6 model entry to registry JSON`  
**Shipped in upstream tag:** `v6.9.32`  
**Local commit:** `3e458f58` — `feat(models): replay Kimi K2.6 registry entry` (2026-04-22)  
**Files touched locally:** `internal/registry/models/models.json`

### v6.9.32 — GPT-Image-2 Codex image routes

**Upstream SHA:** `e935196d` — `feat(models): add hardcoded GPT-Image-2 model support in Codex`  
**Shipped in upstream tag:** `v6.9.32`  
**Local commit:** `bf127a56` — `feat(openai): replay GPT-Image-2 Codex image routes` (2026-04-22)  
**Files touched locally:** `sdk/api/handlers/openai/openai_images_handlers.go`

### v6.9.33 + v6.9.34 — OpenAI image handler n-parameter cleanup

**Upstream SHAs:**  
- `fd71960c` — `fix(handlers): remove handling of unsupported n parameter in OpenAI image handlers`  
- `a1881596` — `fix(handlers): remove references to unsupported n parameter in OpenAI image handlers`  
**Shipped in upstream tags:** `v6.9.33`, `v6.9.34`  
**Local commit:** `badffeff` — `fix(openai): drop unsupported image n parameter handling` (2026-04-22)  
**Files touched locally:** `sdk/api/handlers/openai/openai_images_handlers.go`  
**Notes:** Both upstream commits combined into one local commit. Handler no longer reads,
copies, or forwards the `n` (image count) field.

---

## 3. ne0ark-Only Additions

These files and directories exist in `ne0ark/dev` but are absent from `router-for-me/dev`.
They represent the factory mission infrastructure, agent-readiness tooling, validation
harness, and additional local improvements.

### Factory mission infrastructure

| Path | Purpose |
|---|---|
| `.factory/` | Full mission directory: init scripts, skills, library, validation artifacts, services manifest |
| `.factory/init.sh` | One-time environment setup (go mod download, etc.) |
| `.factory/services.yaml` | Single source of truth for commands and services used by mission workers |
| `.factory/library/architecture.md` | Mission-level architecture notes |
| `.factory/library/environment.md` | Windows host environment notes |
| `.factory/library/pr-replay.md` | Approved upstream batch and replay guidance |
| `.factory/library/user-testing.md` | Validation surface configuration |
| `.factory/skills/cliproxyapi-change-playbook/SKILL.md` | Repo-specific change-guidance skill |
| `.factory/skills/pr-replay-worker/SKILL.md` | PR replay worker procedure |
| `.factory/skills/publish-readiness-worker/SKILL.md` | Publish readiness worker procedure |
| `.factory/validation/` | Per-milestone scrutiny and user-testing synthesis records |

### Agent-readiness and CI tooling

| Path | Purpose |
|---|---|
| `.github/workflows/agents-md-freshness.yml` | CI: validates AGENTS.md command freshness on every PR |
| `.github/workflows/branch-release.yml` | CI: rolling releases for `main` and `dev` branches |
| `.github/workflows/pr-lint.yml` | CI: PR lint workflow |
| `.golangci.yml` | Project-wide golangci-lint configuration |
| `.golangci.dupl-sdk-api-handlers.yml` | Supplemental dupl check for `sdk/api/handlers` |
| `.pre-commit-config.yaml` | Pre-commit hook configuration |
| `.pre-commit-hooks/pii_check.py` | Custom pre-commit hook: blocks PII in commits |

### Documentation and schema

| Path | Purpose |
|---|---|
| `openapi.yaml` | OpenAPI 3.1 schema describing all built-in proxy routes |
| `FILTER_ARCHITECTURE.md` | Documents the thinking/reasoning pipeline filter architecture |

### Additional source additions

| Path | Purpose |
|---|---|
| `internal/api/model_acl.go` | Per-key model ACL middleware (PR #2914 replay) |
| `internal/api/model_acl_test.go` | ACL middleware tests |
| `internal/api/handlers/management/api_keys_payload_test.go` | API key payload validation tests |
| `internal/api/handlers/management/auth_files_upload_test.go` | Auth upload synthesis tests |
| `internal/api/handlers/management/config_lists_api_keys_policy_test.go` | Policy CRUD tests |
| `internal/api/handlers/management/gin_test_mode_test.go` | Test mode helper |
| `internal/config/sdk_config_cache_test.go` | SDKConfig atomic pointer cache tests |
| `internal/config/sdk_config_test.go` | SDKConfig unit tests |
| `internal/runtime/executor/claude_executor_cloaked_cache_control_test.go` | Cache-control cloaking tests |
| `internal/runtime/executor/claude_opus_4_7.go` | Claude Opus 4.7 executor extension |
| `internal/runtime/executor/claude_opus_4_7_test.go` | Claude Opus 4.7 tests |
| `internal/runtime/executor/helps/payload_helpers_test.go` | Payload helper tests |
| `internal/watcher/diff/config_diff_providers.go` | Watcher diff split: providers section |
| `internal/watcher/diff/config_diff_sections.go` | Watcher diff split: sections module |
| `sdk/api/handlers/gemini/stream_helpers.go` | Gemini stream helper utilities |
| `sdk/api/handlers/handlers_auth_manager_test.go` | Auth manager handler tests |
| `sdk/api/handlers/openai/openai_images_handlers_test.go` | OpenAI images handler tests |
| `sdk/cliproxy/auth/metadata_hydrate.go` | Auth metadata hydration logic |
| `sdk/cliproxy/auth/metadata_hydrate_test.go` | Auth metadata hydration tests |
| `test/agents_md_freshness_test.go` | Integration test: AGENTS.md command freshness |
| `test/openapi_schema_test.go` | Integration test: OpenAPI schema validity |
| `test/reasoning_effort_filter_test.go` | Integration test: reasoning effort filtering |

---

## 4. Upstream Items Intentionally Deferred or Rejected

These upstream commits or PRs were evaluated during the mission and deliberately not
replayed, per the guidance in `.factory/library/pr-replay.md`.

| Upstream Reference | Subject | Decision | Rationale |
|---|---|---|---|
| PR `#2926` | translator-only policy-risky fix | Deferred | Translator-standalone changes restricted by AGENTS.md; no WRITE permission verified for standalone translator edits |
| PR `#2912` | larger overlapping auth/conductor rewrite | Deferred | Overlaps `sdk/cliproxy/auth/conductor.go` already touched by PR #2895 replay; risk of regression outweighs benefit in current scope |
| PR `#2885` | larger overlapping auth suspension feature | Deferred | Broader auth-suspension redesign; mission scope limited to targeted fixes only |
| `v6.9.29` fix-provider (Merge PR #2892) | auth-index stabilization | Partially absorbed | Core intent covered by PR #2923 replay; residual auth-index test removals not replayed to avoid unreviewed scope expansion |
| `v6.9.30` VisionCoder sponsorship doc | docs: add VisionCoder sponsorship details | Rejected | Sponsor content is upstream-repo-specific; not applicable to ne0ark fork |
| `v6.9.28` host-header forwarding fix | `fix(util): forward custom Host header to upstream` | Already satisfied | Local `ne0ark/dev` already carried an equivalent fix at `66f3ed57` before this mission began |

---

## 5. File-Level Divergence Inventory

Summary of files that differ between `ne0ark/dev` and `router-for-me/dev` (from
`git diff --name-status upstream/dev..dev` and the inverse).

### ne0ark-only additions (A — present locally, absent upstream)

```
.factory/                              (entire factory mission directory)
.github/workflows/agents-md-freshness.yml
.github/workflows/branch-release.yml
.github/workflows/pr-lint.yml
.golangci.dupl-sdk-api-handlers.yml
.golangci.yml
.pre-commit-config.yaml
.pre-commit-hooks/pii_check.py
FILTER_ARCHITECTURE.md
openapi.yaml
internal/api/model_acl.go
internal/api/model_acl_test.go
internal/api/handlers/management/api_keys_payload_test.go
internal/api/handlers/management/auth_files_upload_test.go
internal/api/handlers/management/config_lists_api_keys_policy_test.go
internal/api/handlers/management/gin_test_mode_test.go
internal/config/sdk_config_cache_test.go
internal/config/sdk_config_test.go
internal/runtime/executor/claude_executor_cloaked_cache_control_test.go
internal/runtime/executor/claude_opus_4_7.go
internal/runtime/executor/claude_opus_4_7_test.go
internal/runtime/executor/helps/payload_helpers_test.go
internal/watcher/diff/config_diff_providers.go
internal/watcher/diff/config_diff_sections.go
sdk/api/handlers/gemini/stream_helpers.go
sdk/api/handlers/handlers_auth_manager_test.go
sdk/api/handlers/openai/openai_images_handlers_test.go
sdk/cliproxy/auth/metadata_hydrate.go
sdk/cliproxy/auth/metadata_hydrate_test.go
test/agents_md_freshness_test.go
test/openapi_schema_test.go
test/reasoning_effort_filter_test.go
```

### Files modified in both (M — same path, content diverges)

These files exist in both repositories but carry local changes from replay milestones
and ne0ark-specific additions. Divergence is intentional.

```
.github/workflows/pr-test-build.yml    (ne0ark CI tweaks)
.github/workflows/release.yaml         (ne0ark release config)
.gitignore                             (ne0ark ignores: .factory/ local artifacts excluded)
README.md                              (ne0ark fork-specific links/badges)
internal/api/handlers/management/auth_files.go        (PR #2923 replay: synthesis fields)
internal/api/handlers/management/auth_files_batch_test.go
internal/api/handlers/management/auth_files_delete_test.go
internal/api/handlers/management/auth_files_download_test.go
internal/api/handlers/management/auth_files_download_windows_test.go
internal/api/handlers/management/auth_files_patch_fields_test.go
internal/api/handlers/management/config_lists.go      (PR #2914 replay: policy CRUD)
internal/api/handlers/management/config_lists_delete_keys_test.go
internal/api/middleware/request_logging.go
internal/api/modules/amp/*                             (PR #2914 ACL applied to Amp routes)
internal/api/server.go                                (PR #2914 + HEAD /healthz replay)
internal/api/server_test.go
internal/auth/claude/anthropic_auth.go
internal/auth/codex/openai_auth.go
internal/auth/kimi/kimi.go
internal/config/config.go
internal/config/sdk_config.go                         (PR #2914: atomic policy index)
internal/logging/request_logger.go
internal/registry/model_updater.go
internal/registry/models/models.json                  (Kimi K2.6, GPT-Image-2 entries)
internal/runtime/executor/antigravity_executor.go
internal/runtime/executor/claude_executor.go           (PR #2896 OAuth remap replay)
internal/runtime/executor/codex_executor.go            (v6.9.31 stream output replay)
internal/runtime/executor/codex_websockets_executor.go
internal/runtime/executor/gemini_cli_executor.go
internal/runtime/executor/gemini_executor.go
internal/runtime/executor/gemini_vertex_executor.go
internal/runtime/executor/helps/claude_device_profile.go
internal/runtime/executor/helps/claude_system_prompt.go
internal/runtime/executor/helps/payload_helpers.go
internal/runtime/executor/helps/usage_helpers.go
internal/runtime/executor/openai_compat_executor.go
internal/store/gitstore.go
internal/store/objectstore.go
internal/store/postgresstore.go
internal/translator/antigravity/claude/signature_validation.go
internal/translator/openai/claude/openai_claude_response.go
internal/tui/dashboard.go
internal/tui/styles.go
internal/watcher/diff/config_diff.go                  (split into providers/sections)
sdk/api/handlers/gemini/gemini-cli_handlers.go
sdk/api/handlers/gemini/gemini_handlers.go
sdk/api/handlers/handlers.go
sdk/api/handlers/openai/openai_images_handlers.go     (n-param drop, GPT-Image-2)
sdk/api/handlers/openai/openai_responses_websocket.go
sdk/auth/filestore.go
sdk/cliproxy/auth/conductor.go                        (PR #2895 + v6.9.31 backoff replay)
sdk/cliproxy/auth/conductor_overrides_test.go
sdk/cliproxy/auth/conductor_scheduler_refresh_test.go
sdk/cliproxy/auth/scheduler.go
sdk/cliproxy/service.go
```

### Upstream-only deletions (D from ne0ark perspective)

The following upstream files were removed upstream and are not present in
`router-for-me/dev` but exist locally (ne0ark intentionally retains them):

- All `.factory/` paths — mission infrastructure; intentionally local
- `openapi.yaml` — local OpenAPI schema; not present upstream
- `FILTER_ARCHITECTURE.md` — local architecture doc; not present upstream
- `.golangci.yml`, `.golangci.dupl-sdk-api-handlers.yml` — local linting config
- `.pre-commit-config.yaml`, `.pre-commit-hooks/pii_check.py` — local hooks

---

## References

- Upstream repository: <https://github.com/router-for-me/CLIProxyAPI>
- ne0ark fork: <https://github.com/ne0ark/CLIProxyAPI>
- Keep a Changelog format: <https://keepachangelog.com/en/1.1.0/>
- Mission replay guidance: `.factory/library/pr-replay.md`
- Mission architecture: `.factory/library/architecture.md`
