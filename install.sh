#!/bin/sh
# Foreman installer — downloads the latest binary to ~/.local/bin
set -eu

BASE_URL="https://github.com/roraja/foreman/releases/download"
VERSION="${FOREMAN_VERSION:-v0.0.2}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY="foreman-${OS}-${ARCH}"
URL="${BASE_URL}/${VERSION}/${BINARY}"

echo "Downloading foreman ${VERSION} (${OS}/${ARCH})..."
mkdir -p "$INSTALL_DIR"

if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "${INSTALL_DIR}/foreman"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "${INSTALL_DIR}/foreman" "$URL"
else
    echo "Error: curl or wget is required"; exit 1
fi

chmod +x "${INSTALL_DIR}/foreman"
echo "Installed foreman to ${INSTALL_DIR}/foreman"

# Check if INSTALL_DIR is in PATH
case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *) echo "Note: Add ${INSTALL_DIR} to your \$PATH if it isn't already." ;;
esac
