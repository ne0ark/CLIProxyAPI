# User Testing Guide

## Validation Concurrency

Max concurrent validators: **2**

The primary validation surface for this project is repository-level Go testing (not browser UI).
Tests run in-process with no external service dependencies. Concurrency is bounded by
machine CPU/memory rather than shared state conflicts.

## Flow Validator Guidance: go-test

The Go test surface validates behavioral contracts through unit tests in
`internal/runtime/executor/`. Tests are fully isolated (no shared state between tests)
and can run concurrently without interference.

### Isolation rules
- Each test creates its own `httptest.NewServer` instance
- No shared global state between tests
- Tests do not depend on external services or network access

### Tools
- `go test ./internal/runtime/executor/... -count=1 -v` for targeted executor tests
- `go test -count=1 -p 16 ./...` for full suite
- `go build -o test-output.exe ./cmd/server` for compile verification
- `gh pr view` and `git diff` for scope checks
