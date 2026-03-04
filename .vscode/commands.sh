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
    help|*)
        echo "Foreman Development Commands"
        echo ""
        echo "Usage: bash tools/foreman/.vscode/commands.sh <command>"
        echo ""
        echo "Commands:"
        echo "  build         Build foreman binary (debug)"
        echo "  build-release Build foreman binary (release, stripped)"
        echo "  run [config]  Run foreman with go run (default: foreman.yaml)"
        echo "  test          Run all tests"
        echo "  lint          Run go vet"
        echo "  fmt           Format all Go files"
        echo "  clean         Remove build artifacts"
        echo "  cross-build   Build for all platforms"
        echo "  help          Show this help"
        ;;
esac
