#!/bin/bash

# build.sh
#
# Builds the Lattice binary and optionally installs it.
#
# Usage:
#   ./build.sh          # Just build
#   ./build.sh install  # Build and install to ~/.local/bin (or $LATTICE_INSTALL_DIR)

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"
GO_BIN="$(command -v go)"
GO_VERSION="$(go version 2>/dev/null || echo "unknown")"
INSTALL_DIR="${LATTICE_INSTALL_DIR:-$HOME/.local/bin}"
LATTICE_SOURCE_DIR="$PROJECT_ROOT"
MODULE_PATH=$(awk '/^module / {print $2; exit}' "$PROJECT_ROOT/go.mod")
CONFIG_PKG="$MODULE_PATH/internal/config"

LDFLAGS=""
if [[ -z "$MODULE_PATH" ]]; then
	echo "⚠️  Unable to detect module path from go.mod; skipping embedded LATTICE_ROOT default."
elif [[ "$LATTICE_SOURCE_DIR" =~ [[:space:]] ]]; then
	echo "⚠️  Project path contains whitespace; skipping embedded LATTICE_ROOT default. Set LATTICE_ROOT manually."
else
	LDFLAGS="-X $CONFIG_PKG.defaultLatticeRoot=$LATTICE_SOURCE_DIR"
    echo "   📍 Embedding LATTICE_ROOT default: $LATTICE_SOURCE_DIR"
fi

echo "🔨 Building Lattice"
echo "   📁 Project root: $PROJECT_ROOT"
echo "   🧰 Go binary:   ${GO_BIN:-not found}"
echo "   🧾 Go version:  $GO_VERSION"

cd "$PROJECT_ROOT"

echo "📦 Downloading dependencies (go mod download)"
go mod download

echo "🏗️  Compiling ./cmd/lattice"
if [ -n "$LDFLAGS" ]; then
    go build -ldflags "$LDFLAGS" -o lattice ./cmd/lattice
else
    go build -o lattice ./cmd/lattice
fi

BUILD_SUM="$(sha256sum lattice | awk '{print $1}')"
echo "✅ Built ./lattice (sha256: $BUILD_SUM)"

if [ "${1:-}" == "install" ]; then
    echo "📥 Installing lattice -> $INSTALL_DIR"
    mkdir -p "$INSTALL_DIR"
    install -m 0755 lattice "$INSTALL_DIR/lattice"
    echo "✅ Installed to $INSTALL_DIR/lattice"
    if command -v lattice >/dev/null 2>&1; then
        echo "   ⚙️ Detected lattice on PATH at: $(command -v lattice)"
    else
        echo "   ⚠ lattice not on PATH. Add: export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
fi

echo ""
echo "🚀 Usage"
echo "   ./lattice            # Run from repo"
echo "   lattice              # Run globally (after install)"
