#!/usr/bin/env sh
set -eu

# Ensure module deps are downloaded so workers can build/test immediately.
go mod download

# Refresh the upstream remote so PR-replay and delta-doc workers can read
# router-for-me/CLIProxyAPI history without re-running fetches mid-flow.
# Failure here is non-fatal — workers can fall back to whatever upstream/* refs
# are already present locally.
git fetch upstream --prune || true
