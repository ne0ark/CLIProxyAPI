# Environment

Environment variables, external dependencies, and setup notes for this mission.

**What belongs here:** required tools, authenticated services, repo-local constraints, and external dependencies.
**What does NOT belong here:** validation contract assertions, feature definitions, or service ports/commands.

---

## Repo and GitHub context

- Working repository: `G:\Droid.Claude\CLIProxyAPI`
- Current working branch for replay work: `dev`
- Remotes:
  - `origin = https://github.com/ne0ark/CLIProxyAPI`
  - `upstream = https://github.com/router-for-me/CLIProxyAPI.git`
- GitHub CLI is authenticated as `ne0ark`

## Mission constraints

- Recreate accepted upstream PR behavior on local `dev` only.
- Never push to `upstream`.
- Never update contributor PR branches.
- Treat `pr2914.diff` and `pr2914.utf8.diff` as review artifacts; do not stage or commit them.

## Tooling

- Go toolchain is available and validated locally (`go1.26.2 windows/amd64` during dry run).
- Primary validation commands for this mission are local Go build/test commands plus GitHub PR inspection.
- No external database, cache, browser automation, or long-running service is required for the approved batch.

## Validation gotchas

- On this Windows host, the working compiler path for race validation is:
  - `C:\Users\Janak Kalaria\AppData\Local\Microsoft\WinGet\Packages\MartinStorsjo.LLVM-MinGW.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\llvm-mingw-20260407-ucrt-x86_64\bin`
- That compiler is not on `PATH` by default for this shell, so race runs must prepend that bin directory to `PATH` and set:
  - `CGO_ENABLED=1`
  - `CC=gcc`
- The first successful PR-2914 race launch no longer failed on environment setup; it surfaced real race failures in tests under `internal/api/handlers/management`, notably concurrent `gin.SetMode` writes.
- Treat the race gate as actionable code/test work now, not as an external environment blocker.

## Windows pre-commit fallback

- The `pre-commit` CLI launcher may not be on `PATH` on this Windows host.
- If `pre-commit run ...` fails with "command not found", use `python3 -m pre_commit run ...` as the fallback.
- The `services.yaml` `prepush` command uses the standard `pre-commit` invocation; workers should use the `python3 -m pre_commit` form only when the direct launcher is unavailable.
