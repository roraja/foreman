#!/bin/sh
# Foreman installer — downloads the latest binary to ~/.local/bin
set -eu

REPO="roraja/foreman"
BASE_URL="https://github.com/${REPO}/releases/download"

echo "==> Starting foreman installer"

# Resolve latest version from GitHub if not explicitly set
if [ -z "${FOREMAN_VERSION:-}" ]; then
    echo "==> Detecting latest version from GitHub..."
    if command -v curl >/dev/null 2>&1; then
        echo "    Using curl for version detection"
        VERSION="$(curl -fsSo /dev/null -w '%{redirect_url}' "https://github.com/${REPO}/releases/latest" | grep -oE '[^/]+$')"
    elif command -v wget >/dev/null 2>&1; then
        echo "    Using wget for version detection"
        VERSION="$(wget --spider --max-redirect=0 "https://github.com/${REPO}/releases/latest" 2>&1 | grep -oE 'Location: [^ ]+' | grep -oE '[^/]+$')"
    else
        echo "Error: curl or wget is required for version detection"; exit 1
    fi
    if [ -z "${VERSION:-}" ]; then
        echo "Error: could not determine latest version. Set FOREMAN_VERSION explicitly."; exit 1
    fi
    echo "    Detected version: ${VERSION}"
else
    VERSION="$FOREMAN_VERSION"
    echo "==> Using provided version: ${VERSION}"
fi
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
echo "==> Detected platform: ${OS}/${ARCH}"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Error: unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY="foreman-${OS}-${ARCH}"
URL="${BASE_URL}/${VERSION}/${BINARY}"

echo "==> Downloading foreman ${VERSION} (${OS}/${ARCH})..."
echo "    URL: ${URL}"
mkdir -p "$INSTALL_DIR"

if command -v curl >/dev/null 2>&1; then
    echo "    Using curl for download"
    if ! curl -fsSL "$URL" -o "${INSTALL_DIR}/foreman"; then
        echo "Error: download failed. Check that version ${VERSION} exists and the URL is correct."
        exit 1
    fi
elif command -v wget >/dev/null 2>&1; then
    echo "    Using wget for download"
    if ! wget -qO "${INSTALL_DIR}/foreman" "$URL"; then
        echo "Error: download failed. Check that version ${VERSION} exists and the URL is correct."
        exit 1
    fi
else
    echo "Error: curl or wget is required"; exit 1
fi

chmod +x "${INSTALL_DIR}/foreman"
echo "==> Installed foreman to ${INSTALL_DIR}/foreman"

# Verify the binary runs
if "${INSTALL_DIR}/foreman" --version >/dev/null 2>&1; then
    echo "    Verified: $(${INSTALL_DIR}/foreman --version 2>/dev/null || echo 'ok')"
else
    echo "    Warning: could not verify the binary. It may not be compatible with this platform."
fi

# Check if INSTALL_DIR is in PATH
case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) echo "==> ${INSTALL_DIR} is already in your \$PATH. You're all set!" ;;
    *) echo "==> Note: Add ${INSTALL_DIR} to your \$PATH if it isn't already." ;;
esac
