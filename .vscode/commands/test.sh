#!/usr/bin/env bash
# =============================================================================
# Test — Run test suites
# =============================================================================
# Sourced by .vscode/commands.sh. Requires FOREMAN_ROOT to be set.

Test.All() {
    echo "=== Running Tests ==="
    cd "$FOREMAN_ROOT"
    go test ./... -v
}
