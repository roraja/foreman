#!/usr/bin/env bash
# =============================================================================
# Install — Build and install foreman locally
# =============================================================================
# Sourced by .vscode/commands.sh. Requires FOREMAN_ROOT to be set.

Install.Local() {
    echo "=== Installing Foreman ==="
    cd "$FOREMAN_ROOT"
    CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/foreman ./cmd/foreman
    local install_dir="$HOME/.local/bin"
    mkdir -p "$install_dir"
    cp bin/foreman "$install_dir/foreman"
    echo "✅ Installed: $install_dir/foreman"
    ls -lh "$install_dir/foreman"
}
