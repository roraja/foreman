#!/usr/bin/env bash
# =============================================================================
# Dev — Linting, formatting, and cleanup
# =============================================================================
# Sourced by .vscode/commands.sh. Requires FOREMAN_ROOT to be set.

Dev.Lint() {
    echo "=== Running go vet ==="
    cd "$FOREMAN_ROOT"
    go vet ./...
    echo "✅ No issues found"
}

Dev.Fmt() {
    echo "=== Formatting ==="
    cd "$FOREMAN_ROOT"
    gofmt -w .
    echo "✅ Formatted"
}

Dev.Clean() {
    echo "=== Cleaning ==="
    cd "$FOREMAN_ROOT"
    rm -rf bin/
    echo "✅ Cleaned"
}
