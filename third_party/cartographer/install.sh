#!/bin/bash
# CMP Installation Script for Linux/Mac
# Run this script to install CMP globally

set -e

echo "========================================"
echo "CMP Installation Script"
echo "========================================"
echo ""

# Check if Rust is installed
echo "[1/4] Checking Rust installation..."
if ! command -v cargo &> /dev/null; then
    echo "❌ Rust not found. Please install Rust first:"
    echo "   curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
    exit 1
fi
echo "✓ Rust found"
echo ""

# Build CMP
echo "[2/4] Building CMP (this may take a few minutes)..."
cd mapper-core/cartographer
cargo build --release
cd ..
echo "✓ Build successful"
echo ""

# Create bin directory
echo "[3/4] Installing CMP..."
mkdir -p ~/.local/bin

# Copy binary
cp cartographer/target/release/cartographer ~/.local/bin/
chmod +x ~/.local/bin/cartographer
echo "✓ Binary copied to: ~/.local/bin/cartographer"
echo ""

# Add to PATH
echo "[4/4] Updating PATH..."
SHELL_RC=""
if [ -f ~/.bashrc ]; then
    SHELL_RC=~/.bashrc
elif [ -f ~/.zshrc ]; then
    SHELL_RC=~/.zshrc
fi

if [ -n "$SHELL_RC" ]; then
    if ! grep -q 'export PATH="$HOME/.local/bin:$PATH"' "$SHELL_RC"; then
        echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$SHELL_RC"
        echo "✓ Added to PATH in $SHELL_RC"
    else
        echo "✓ Already in PATH"
    fi
fi
echo ""

# Verify installation
echo "========================================"
echo "Installation Complete!"
echo "========================================"
echo ""

# Test command
echo "Testing installation..."
export PATH="$HOME/.local/bin:$PATH"
if cartographer --version &> /dev/null; then
    VERSION=$(cartographer --version)
    echo "✓ CMP is working: $VERSION"
else
    echo "⚠️  Please restart your terminal for PATH changes to take effect"
fi
echo ""

echo "Next steps:"
echo "  1. Restart your terminal (if needed)"
echo "  2. Set your UC API key:"
echo "     echo 'ULTRA_CONTEXT=uc_live_your_key' > .env.local"
echo "  3. Initialize your project:"
echo "     cartographer init --cloud --project my-project"
echo "  4. Start using CMP:"
echo "     cartographer source && cartographer push"
echo ""
echo "Documentation: UC_INTEGRATION.md"
echo "Quick Start: QUICKSTART.md"
echo ""
