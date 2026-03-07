#!/bin/bash
# Foreman — Build & Development Commands
# Usage: source .vscode/commands.sh
#
# Naming convention:
#   Build.<Target>    - Compile binaries
#   Run.<Service>     - Run from source
#   Test.<Suite>      - Run tests
#   Dev.<Tool>        - Linting, formatting, cleanup
#   Install.<Target>  - Install binaries
#   Release.<Action>  - Version tagging and releases

if [ -n "${ZSH_VERSION:-}" ]; then
    FOREMAN_ROOT="$(cd "$(dirname "${(%):-%x}")/.." && pwd)"
else
    FOREMAN_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi

export GO111MODULE=on
# Avoid GOPATH conflicts
if [[ "$FOREMAN_ROOT" == "${GOPATH:-}"* ]] && [[ -n "${GOPATH:-}" ]]; then
    export GOPATH=/tmp/foreman-gopath
fi

# =============================================================================
# AVAILABLE COMMANDS REGISTRY
# =============================================================================
FOREMAN_COMMANDS=(
    "Build.Debug:Build foreman binary (debug)"
    "Build.Release:Build foreman binary (release, stripped)"
    "Build.CrossPlatform:Build for all platforms (linux, darwin, windows)"
    "Run.Foreman:Run foreman with go run (default config: foreman.yaml)"
    "Test.All:Run all tests"
    "Dev.Lint:Run go vet"
    "Dev.Fmt:Format all Go files"
    "Dev.Clean:Remove build artifacts"
    "Install.Local:Build and install foreman to ~/.local/bin"
    "Release.Tag:Increment version tag and trigger a GitHub release"
    "ff:Interactive command picker (fzf)"
)

# =============================================================================
# INTERACTIVE COMMAND PICKER (fzf)
# =============================================================================

ff() {
    if ! command -v fzf &> /dev/null; then
        echo "❌ fzf is not installed. Install with: sudo apt install fzf"
        return 1
    fi

    local selected
    selected=$(printf '%s\n' "${FOREMAN_COMMANDS[@]}" | \
        fzf --height=40% \
            --layout=reverse \
            --border \
            --prompt="🔧 Select command: " \
            --header="Foreman Development Commands" \
            --delimiter=":" \
            --with-nth=1 \
            --preview='echo "📝 {2}"' \
            --preview-window=down:1:wrap)

    if [[ -n "$selected" ]]; then
        local cmd="${selected%%:*}"
        echo "▶️  Running: $cmd"
        echo "────────────────────────────────"
        $cmd
    fi
}

# =============================================================================
# SOURCE COMMAND FILES
# =============================================================================
for _cmd_file in "$FOREMAN_ROOT"/.vscode/commands/*.sh; do
    [[ -f "$_cmd_file" ]] && source "$_cmd_file"
done
unset _cmd_file

echo "📋 Foreman Commands loaded"
echo "   💡 Tip: Use 'ff' for interactive command picker"
