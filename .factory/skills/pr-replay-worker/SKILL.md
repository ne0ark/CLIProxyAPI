---
name: pr-replay-worker
description: Review an approved upstream PR or release-tag slice, recreate the accepted delta on local dev, and validate it strictly without pushing.
---

# PR Replay Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the work procedure for replaying approved upstream PR behavior onto local `dev`.

## When to Use This Skill

Use this skill for features that:

- inspect an approved upstream PR from `router-for-me/CLIProxyAPI`
- or inspect an approved upstream release-tag commit/slice from `router-for-me/CLIProxyAPI`
- compare it against the current local `dev` branch
- recreate only the accepted missing delta on local `dev`
- prove the result with strict targeted tests plus repository validation

This worker does not push to GitHub. It prepares validated local commits only.

## Required Skills

- `CLIProxyAPI change playbook` — invoke before tracing or editing code so the touched subsystem matches repo architecture and guardrails.
- `review` — invoke after the local diff is ready and before the final validation sweep to adversarially review the recreated change for correctness gaps.

## Windows Compatibility Notes

- If `.factory/init.sh` cannot be executed directly in the current Windows shell, run the equivalent PowerShell-safe setup commands manually: `go mod download`.
- If the preferred review path is unavailable in this environment, use the available `worker` subagent or a manual adversarial review as the fallback reviewer and note that substitution in the handoff.

## Work Procedure

1. **Anchor the replay**
   - Confirm you are on local `dev`.
   - Inspect the upstream source using the form that matches the milestone:
     - PR milestone: use `gh pr view` and `gh pr diff`.
     - Release-tag / commit milestone: use `gh api "repos/router-for-me/CLIProxyAPI/commits/<sha>"` (or equivalent commit inspection) and compare the changed files plus commit message/body.
   - Record the upstream PR number or commit SHA, title/summary, target branch or release context, changed files, and any review comments or acceptance notes that change the acceptance bar.
   - Compare upstream intent to the current local `dev` state before editing. If local `dev` already contains equivalent behavior, keep only the missing delta.

2. **Trace the owning subsystem**
   - Invoke `CLIProxyAPI change playbook`.
   - Read the touched files and adjacent tests.
   - Identify the smallest correct layer for the replay.
   - If the work would spill into unrelated subsystems or blocked translator-only territory, stop and return to the orchestrator.

3. **Write tests first**
   - Add or expand targeted regression tests before changing implementation.
   - Use the mission validation contract as the acceptance source.
   - If the PR already has some equivalent local behavior, add tests that prove the missing gap before editing implementation.
   - Keep tests focused on touched packages and the specific regression surface.

4. **Implement the minimal accepted delta**
   - Recreate only the approved behavior needed on local `dev`.
   - Do not mechanically copy the upstream patch if local `dev` has diverged.
   - Keep the diff tightly scoped to the approved PR surface plus directly related tests.
   - Do not stage or modify `pr2914.diff` or `pr2914.utf8.diff`.

5. **Review diff scope before validation**
   - Compare the local changed-file list against the upstream PR file list.
   - If extra files are needed because local `dev` has diverged, confirm they are truly required for the approved behavior and note why in the handoff.
   - If the replay reveals that the upstream PR should be rejected or deferred under the strict review bar, stop and return to the orchestrator instead of forcing it through.

6. **Run strict validation**
   - Run `gofmt` on every changed Go file.
   - Run targeted `go test` commands for touched packages first.
   - Run disposable server compile verification.
   - Run full `go test ./...`.
   - If the feature touches the PR `#2914` ACL/config surface, also run the targeted race command from `.factory/services.yaml`.

7. **Adversarial review**
   - Invoke `review` on the final local diff or branch delta.
   - If the review path is unavailable or returns no usable output, use the available `worker` subagent as the first fallback reviewer.
   - If no reviewer subagent path is usable, perform a manual adversarial review yourself by checking `git diff --cached` against the upstream PR or commit intent, the validation contract, and nearby tests, then record that fallback in the handoff.
   - Fix any high-confidence correctness issues surfaced by that review.
   - Re-run targeted tests and any broader validation affected by the fix.

8. **Prepare the local commit**
   - Stage only the intended files.
   - Review `git diff --cached` before committing.
   - Create one local commit tied to the upstream PR slice.
   - Do not push.

9. **Return a detailed handoff**
   - Include the upstream PR or commit inspected, what behavior already existed on local `dev`, what delta you added, every validation command you ran, and any reasons the local diff intentionally differs from the upstream patch or release commit.

## Example Handoff

```json
{
  "salientSummary": "Reviewed upstream PR #2923, found local dev already preserved metadata but not runtime priority propagation, added upload-path synthesis coverage plus the minimal handler change, and validated it with targeted management tests, compilecheck, and full go test. No push was performed.",
  "whatWasImplemented": "Recreated the accepted PR #2923 behavior on local dev by updating the management auth upload rebuild path to populate immediate runtime attributes from synthesized auth data, then added focused upload regression tests covering multipart/raw-body parity and immediate selector-visible priority propagation.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {
        "command": "gh pr view 2923 --repo router-for-me/CLIProxyAPI --json number,title,baseRefName,files,comments",
        "exitCode": 0,
        "observation": "Confirmed PR target is dev and scoped to auth_files.go plus upload tests."
      },
      {
        "command": "gofmt -w internal/api/handlers/management/auth_files.go internal/api/handlers/management/auth_files_upload_test.go",
        "exitCode": 0,
        "observation": "Formatted only the touched Go files."
      },
      {
        "command": "go test ./internal/api/handlers/management -run 'UploadAuthFile|AuthFiles' -count=1",
        "exitCode": 0,
        "observation": "Targeted management upload regressions passed, including immediate runtime priority propagation."
      },
      {
        "command": "go build -o test-output.exe ./cmd/server; if (Test-Path test-output.exe) { Remove-Item test-output.exe }",
        "exitCode": 0,
        "observation": "Disposable server compile verification passed."
      },
      {
        "command": "go test -count=1 -p 16 ./...",
        "exitCode": 0,
        "observation": "Full repository test suite passed after the replay."
      }
    ],
    "interactiveChecks": [
      {
        "action": "Compared upstream PR #2923 file scope and intent to the local dev diff before commit",
        "observed": "Local replay stayed inside the approved upload-handler slice and directly related tests."
      },
      {
        "action": "Ran review skill on the final local diff",
        "observed": "No additional high-confidence correctness bugs remained after the final fix pass."
      }
    ]
  },
  "tests": {
    "added": [
      {
        "file": "internal/api/handlers/management/auth_files_upload_test.go",
        "cases": [
          {
            "name": "TestUploadAuthFile_PropagatesPriorityImmediately",
            "verifies": "Uploaded top-level priority becomes an immediate runtime auth attribute."
          },
          {
            "name": "TestUploadAuthFile_MultipartAndRawBodyParity",
            "verifies": "Both supported upload entry points produce equivalent runtime auth state."
          }
        ]
      }
    ]
  },
  "discoveredIssues": [
    {
      "severity": "medium",
      "description": "Local dev already contained adjacent auth upload changes, so the replay had to be adapted rather than copied verbatim from upstream.",
      "suggestedFix": "Future replays in this area should always diff upstream intent against current local dev before editing."
    }
  ]
}
```

## When to Return to Orchestrator

- The upstream PR fails the strict review bar and should be rejected or deferred.
- Local `dev` already contains substantially equivalent behavior and the remaining delta is ambiguous.
- Recreating the approved behavior would require pulling in deferred PRs or unrelated subsystems.
- The replay needs changes in a blocked area that violates repo guardrails or mission boundaries.
- Validation is blocked by a pre-existing repository problem you cannot resolve within the feature scope.
