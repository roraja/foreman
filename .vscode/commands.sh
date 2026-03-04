#!/bin/bash
# Foreman — Build & Development Commands
# Usage: bash tools/foreman/.vscode/commands.sh <command>
set -euo pipefail

FOREMAN_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$FOREMAN_ROOT"

export GO111MODULE=on
# Avoid GOPATH conflicts
if [[ "$FOREMAN_ROOT" == "${GOPATH:-}"* ]] && [[ -n "${GOPATH:-}" ]]; then
    export GOPATH=/tmp/foreman-gopath
fi

case "${1:-help}" in
    build)
        echo "=== Building Foreman ==="
        go build -o bin/foreman ./cmd/foreman
        echo "✅ Built: $FOREMAN_ROOT/bin/foreman"
        ls -lh bin/foreman
        ;;
    build-release)
        echo "=== Building Foreman (release, stripped) ==="
        CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/foreman ./cmd/foreman
        echo "✅ Built: $FOREMAN_ROOT/bin/foreman"
        ls -lh bin/foreman
        ;;
    run)
        CONFIG="${2:-foreman.yaml}"
        echo "=== Running Foreman (config: $CONFIG) ==="
        go run ./cmd/foreman -c "$CONFIG"
        ;;
    test)
        echo "=== Running Tests ==="
        go test ./... -v
        ;;
    lint)
        echo "=== Running go vet ==="
        go vet ./...
        echo "✅ No issues found"
        ;;
    install)
        echo "=== Installing Foreman ==="
        CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/foreman ./cmd/foreman
        INSTALL_DIR="$HOME/.local/bin"
        mkdir -p "$INSTALL_DIR"
        cp bin/foreman "$INSTALL_DIR/foreman"
        echo "✅ Installed: $INSTALL_DIR/foreman"
        ls -lh "$INSTALL_DIR/foreman"
        ;;
    clean)
        echo "=== Cleaning ==="
        rm -rf bin/
        echo "✅ Cleaned"
        ;;
    fmt)
        echo "=== Formatting ==="
        gofmt -w .
        echo "✅ Formatted"
        ;;
    cross-build)
        echo "=== Cross-platform build ==="
        for PLATFORM in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
            OS="${PLATFORM%/*}"
            ARCH="${PLATFORM#*/}"
            EXT=""; [ "$OS" = "windows" ] && EXT=".exe"
            echo "  Building: $OS/$ARCH"
            GOOS=$OS GOARCH=$ARCH CGO_ENABLED=0 go build -ldflags="-s -w" -o "bin/foreman-${OS}-${ARCH}${EXT}" ./cmd/foreman
        done
        echo "✅ All platforms built"
        ls -lh bin/
        ;;
    release)
        echo "=== Release New Version ==="

        # Ensure working tree is clean
        if ! git diff --quiet || ! git diff --cached --quiet; then
            echo "Error: working tree has uncommitted changes. Commit or stash first."
            exit 1
        fi

        # Get the latest version tag (default v0.0.0 if none)
        latest=$(git tag --sort=-v:refname --list 'v*' | head -n1)
        if [ -z "$latest" ]; then
            latest="v0.0.0"
        fi

        # Parse major.minor.patch
        version="${latest#v}"
        IFS='.' read -r major minor patch <<< "$version"
        major="${major:-0}"; minor="${minor:-0}"; patch="${patch:-0}"

        echo "Current version: v${major}.${minor}.${patch}"
        echo ""
        echo "Bump type:"
        echo "  1) patch  (v${major}.${minor}.$((patch+1)))"
        echo "  2) minor  (v${major}.$((minor+1)).0)"
        echo "  3) major  (v$((major+1)).0.0)"
        printf "Select [1/2/3] (default: 1): "
        read -r choice

        case "${choice:-1}" in
            1) patch=$((patch+1)) ;;
            2) minor=$((minor+1)); patch=0 ;;
            3) major=$((major+1)); minor=0; patch=0 ;;
            *) echo "Invalid choice"; exit 1 ;;
        esac

        new_tag="v${major}.${minor}.${patch}"
        echo ""
        echo "New version: $new_tag"
        printf "Proceed? [y/N]: "
        read -r confirm
        if [ "${confirm,,}" != "y" ]; then
            echo "Aborted."
            exit 0
        fi

        git tag -a "$new_tag" -m "Release $new_tag" || exit 1
        git push origin "$new_tag" || exit 1
        echo ""
        echo "✅ Pushed tag $new_tag — GitHub Actions will create the release."
        ;;
    help|*)
        echo "Foreman Development Commands"
        echo ""
        echo "Usage: bash .vscode/commands.sh <command>"
        echo ""
        echo "Commands:"
        echo "  build         Build foreman binary (debug)"
        echo "  build-release Build foreman binary (release, stripped)"
        echo "  install       Build and install foreman to ~/.local/bin"
        echo "  run [config]  Run foreman with go run (default: foreman.yaml)"
        echo "  test          Run all tests"
        echo "  lint          Run go vet"
        echo "  fmt           Format all Go files"
        echo "  clean         Remove build artifacts"
        echo "  cross-build   Build for all platforms"
        echo "  release       Increment version tag and trigger a GitHub release"
        echo "  help          Show this help"
        ;;
esac
