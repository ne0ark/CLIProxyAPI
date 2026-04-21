---
name: publish-readiness-worker
description: Fix small repo-local blockers that prevent validated publication to origin/dev without changing intended runtime behavior.
---

# Publish Readiness Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the work procedure for small, behavior-preserving fixes that unblock final publication.

## When to Use This Skill

Use this skill for features that:

- fix lint, hook, or validation blockers discovered while publishing already-approved work
- should preserve existing runtime behavior
- are tightly scoped to a few files and one clear failure mode
- must be fully validated locally before the final publication validator pushes

## Required Skills

- `CLIProxyAPI change playbook` — invoke before editing so the touched subsystem still follows repo architecture and guardrails.
- `review` — invoke after the local diff is ready and before the final validation sweep to adversarially review the unblocker for unintended behavior changes.

## Work Procedure

1. **Anchor the blocker**
   - Read the exact hook, lint, or validation failure that blocked publication.
   - Reproduce the failure locally before editing when feasible.
   - Identify the smallest file surface that can clear the blocker without broad refactors.

2. **Trace the owning subsystem**
   - Invoke `CLIProxyAPI change playbook`.
   - Read the touched files and any nearby tests or helper call sites.
   - Confirm whether the blocker is caused by recent mission work, newly exposed pre-existing code, or both, and note that in the handoff.

3. **Add or confirm coverage**
   - If behavior could accidentally change, add or expand the narrowest targeted tests first.
   - If the blocker is purely dead code removal or non-behavioral structure, document why new tests are unnecessary.

4. **Implement the minimal unblocker**
   - Keep the diff limited to the exact files required to clear the blocker.
   - Preserve runtime semantics unless the blocker itself proves behavior is already wrong.
   - Do not push.

5. **Run publication-focused validation**
   - Run `gofmt` on every changed Go file.
   - Run targeted tests for the touched packages.
   - Run `golangci-lint run --config .golangci.yml`.
   - Run `pre-commit run --hook-stage pre-push --all-files`.
   - Run disposable server compile verification.
   - Run full `go test ./...`.

6. **Adversarial review**
   - Invoke `review` on the final local diff or branch delta.
   - If the review path is unavailable or returns no usable output, use the available `worker` subagent as the first fallback reviewer.
   - If no reviewer subagent path is usable, perform a manual adversarial review against the blocker description, nearby code, and the final validation results, then record that fallback in the handoff.
   - Fix any high-confidence correctness issues and re-run affected validation.

7. **Prepare the local commit**
   - Stage only the intended files.
   - Review `git diff --cached` before committing.
   - Create one local commit for the unblocker.

8. **Return a detailed handoff**
   - Include the original blocker, what caused it, exactly what changed, every validation command run, and whether the final push should now clear the pre-push gate.

## Example Handoff

```json
{
  "salientSummary": "Cleared the final publication blocker by removing three dead management helpers and replacing three SDKConfig copies with proxy-only config values, then validated the result with targeted tests, golangci-lint, the pre-push hook, compilecheck, and the full test suite. No push was performed.",
  "whatWasImplemented": "Updated the auth constructor proxy-override paths so they no longer copy the full SDKConfig value after the ACL replay introduced a non-copyable atomic field, and deleted the orphaned management string-list helpers that no longer have any call sites. The diff stayed scoped to the cited auth files, config_lists.go, and directly related tests only.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {
        "command": "golangci-lint run --config .golangci.yml",
        "exitCode": 0,
        "observation": "The earlier copylocks and unused warnings are gone."
      },
      {
        "command": "pre-commit run --hook-stage pre-push --all-files",
        "exitCode": 0,
        "observation": "The local pre-push gate passed after the fix."
      },
      {
        "command": "go test ./internal/auth/... ./internal/api/handlers/management/... -count=1",
        "exitCode": 0,
        "observation": "Targeted auth and management tests passed."
      },
      {
        "command": "go build -o test-output.exe ./cmd/server; if (Test-Path test-output.exe) { Remove-Item test-output.exe }",
        "exitCode": 0,
        "observation": "Disposable server compile verification passed."
      },
      {
        "command": "go test -count=1 -p 16 ./...",
        "exitCode": 0,
        "observation": "Full repository test suite passed."
      }
    ],
    "interactiveChecks": [
      {
        "action": "Compared the final changed-file list against the blocker diagnostics",
        "observed": "The unblocker stayed within the expected auth and management file surface."
      },
      {
        "action": "Ran review skill on the final local diff",
        "observed": "No unintended behavior changes remained after the final fix pass."
      }
    ]
  },
  "tests": {
    "added": [],
    "coverage": "Relied on existing targeted auth and management tests because the change preserved behavior and removed dead helper code."
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- The blocker actually requires a broader refactor than the diagnosed small fix surface.
- Clearing the blocker would change user-visible runtime behavior outside the approved mission scope.
- Validation still fails after the minimal unblocker and you discover additional unrelated publish blockers.
- The only remaining blocker is the actual remote push itself.
