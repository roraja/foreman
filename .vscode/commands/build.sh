#!/usr/bin/env bash
# =============================================================================
# Build — Compile foreman binaries
# =============================================================================
# Sourced by .vscode/commands.sh. Requires FOREMAN_ROOT to be set.

Build.Debug() {
    echo "=== Building Foreman ==="
    cd "$FOREMAN_ROOT"
    go build -o bin/foreman ./cmd/foreman
    echo "✅ Built: $FOREMAN_ROOT/bin/foreman"
    ls -lh bin/foreman
}

Build.Release() {
    echo "=== Building Foreman (release, stripped) ==="
    cd "$FOREMAN_ROOT"
    CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/foreman ./cmd/foreman
    echo "✅ Built: $FOREMAN_ROOT/bin/foreman"
    ls -lh bin/foreman
}

Build.CrossPlatform() {
    echo "=== Cross-platform build ==="
    cd "$FOREMAN_ROOT"
    for PLATFORM in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
        OS="${PLATFORM%/*}"
        ARCH="${PLATFORM#*/}"
        EXT=""; [ "$OS" = "windows" ] && EXT=".exe"
        echo "  Building: $OS/$ARCH"
        GOOS=$OS GOARCH=$ARCH CGO_ENABLED=0 go build -ldflags="-s -w" -o "bin/foreman-${OS}-${ARCH}${EXT}" ./cmd/foreman
    done
    echo "✅ All platforms built"
    ls -lh bin/
}
