#!/bin/sh
# HomelabMon installer -- detects OS/arch and downloads the latest binary.
# Usage: curl -fsSL https://raw.githubusercontent.com/dx111ge/homelabmon/main/install.sh | sh
set -e

REPO="dx111ge/homelabmon"
INSTALL_DIR="/usr/local/bin"
BINARY="homelabmon"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)   ARCH="amd64" ;;
    aarch64|arm64)   ARCH="arm64" ;;
    armv7l|armhf)    ARCH="arm" ;;
    *)               echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Detected: ${OS}/${ARCH}"
echo "Downloading ${URL} ..."

TMP=$(mktemp)
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$TMP"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$TMP" "$URL"
else
    echo "Error: curl or wget required"; exit 1
fi

chmod +x "$TMP"

# Install (use sudo if not root)
if [ "$(id -u)" -eq 0 ]; then
    mv "$TMP" "${INSTALL_DIR}/${BINARY}"
else
    echo "Installing to ${INSTALL_DIR}/${BINARY} (requires sudo) ..."
    sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
fi

echo ""
echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
echo ""
${INSTALL_DIR}/${BINARY} version
echo ""
echo "Quick start:"
echo "  homelabmon --ui                     # dashboard on :9600"
echo "  homelabmon setup --systemd          # install as systemd service"
echo ""
