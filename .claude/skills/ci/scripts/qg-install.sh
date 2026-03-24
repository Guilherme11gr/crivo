#!/usr/bin/env bash
# crivo-install.sh — Portable quality-gate installer for CI
# Usage: ./crivo-install.sh [version]
# Example: ./crivo-install.sh          # latest
#          ./crivo-install.sh v0.2.0   # specific version
set -euo pipefail

VERSION="${1:-latest}"
REPO="guilherme11gr/crivo"
INSTALL_DIR="${CRIVO_INSTALL_DIR:-/usr/local/bin}"

# Detect OS and architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)              echo "Error: unsupported architecture $ARCH"; exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *)            echo "Error: unsupported OS $OS"; exit 1 ;;
esac

# Resolve version
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4 || echo "")
  if [ -z "$VERSION" ]; then
    echo "No release found on GitHub. Falling back to building from source..."
    if ! command -v go &>/dev/null; then
      echo "Error: no release available and Go is not installed."
      echo "Install Go (https://go.dev) or create a release first."
      exit 1
    fi
    echo "Building from source: go install github.com/$REPO/cmd/crivo@latest"
    go install "github.com/$REPO/cmd/crivo@latest"
    echo "Installed crivo to $(go env GOPATH)/bin/crivo"
    exit 0
  fi
  echo "Latest version: $VERSION"
fi

# Download
EXT="tar.gz"
[ "$OS" = "windows" ] && EXT="zip"

FILENAME="quality-gate_${OS}_${ARCH}.${EXT}"
URL="https://github.com/$REPO/releases/download/$VERSION/$FILENAME"
CHECKSUM_URL="https://github.com/$REPO/releases/download/$VERSION/checksums.txt"

echo "Downloading $URL ..."
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

curl -fsSL "$URL" -o "$TMPDIR/$FILENAME"
curl -fsSL "$CHECKSUM_URL" -o "$TMPDIR/checksums.txt"

# Verify checksum
cd "$TMPDIR"
EXPECTED=$(grep "$FILENAME" checksums.txt | awk '{print $1}')
if [ -n "$EXPECTED" ]; then
  if command -v sha256sum &>/dev/null; then
    ACTUAL=$(sha256sum "$FILENAME" | awk '{print $1}')
  elif command -v shasum &>/dev/null; then
    ACTUAL=$(shasum -a 256 "$FILENAME" | awk '{print $1}')
  else
    echo "Warning: no sha256 tool found, skipping checksum verification"
    ACTUAL="$EXPECTED"
  fi

  if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "Error: checksum mismatch!"
    echo "  Expected: $EXPECTED"
    echo "  Actual:   $ACTUAL"
    exit 1
  fi
  echo "Checksum verified."
fi

# Extract
if [ "$EXT" = "tar.gz" ]; then
  tar xzf "$FILENAME"
else
  unzip -q "$FILENAME"
fi

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv crivo "$INSTALL_DIR/"
else
  sudo mv crivo "$INSTALL_DIR/"
fi

echo "Installed crivo $(crivo version) to $INSTALL_DIR/crivo"
