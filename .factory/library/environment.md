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
