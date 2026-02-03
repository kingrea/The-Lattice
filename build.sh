#!/bin/bash

# build.sh
#
# Builds the Lattice binary and optionally installs it.
#
# Usage:
#   ./build.sh          # Just build
#   ./build.sh install  # Build and install to ~/.local/bin

set -e  # Exit on error

echo "🔨 Building Lattice..."

# Make sure we're in the project root
cd "$(dirname "$0")"

# Download dependencies without mutating go.mod/go.sum
echo "📦 Downloading dependencies..."
go mod download

# Build the binary
echo "🏗️  Compiling..."
go build -o lattice ./cmd/lattice

echo "✅ Built: ./lattice"

# Install if requested
if [ "$1" == "install" ]; then
    # Create ~/.local/bin if it doesn't exist
    mkdir -p ~/.local/bin
    
    # Copy the binary
    cp lattice ~/.local/bin/
    
    echo "✅ Installed to ~/.local/bin/lattice"
    echo ""
    echo "Make sure ~/.local/bin is in your PATH!"
    echo "Add this to your ~/.bashrc or ~/.zshrc if needed:"
    echo '  export PATH="$HOME/.local/bin:$PATH"'
fi

echo ""
echo "🚀 Usage:"
echo "   ./lattice           # Run from here"
echo "   lattice             # Run from anywhere (if installed)"
