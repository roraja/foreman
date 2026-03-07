#!/usr/bin/env bash
# =============================================================================
# Run — Run foreman from source
# =============================================================================
# Sourced by .vscode/commands.sh. Requires FOREMAN_ROOT to be set.

Run.Foreman() {
    local config="${1:-foreman.yaml}"
    echo "=== Running Foreman (config: $config) ==="
    cd "$FOREMAN_ROOT"
    go run ./cmd/foreman -c "$config"
}
