#!/bin/bash
set -e

echo "=== Quest Installer (macOS/Linux Developer Version) ==="

QUEST_HOME="$HOME/.quest"
BIN_DIR="$QUEST_HOME/bin"

echo "Creating directory structure under ~/.quest..."
mkdir -p "$QUEST_HOME/bin"
mkdir -p "$QUEST_HOME/apps"
mkdir -p "$QUEST_HOME/cache"
mkdir -p "$QUEST_HOME/manifests"
mkdir -p "$QUEST_HOME/receipts"

echo "Compiling quest..."
go build -o quest *.go

echo "Installing quest to ~/.quest/bin/..."
mv quest "$BIN_DIR/quest"

echo ""
echo "=== Installation Successful! ==="
echo "Please add the following to your shell configuration file (e.g. ~/.zshrc or ~/.bashrc):"
echo "  export PATH=\"\$HOME/.quest/bin:\$PATH\""
echo ""
echo "Then, restart your terminal or run: source ~/.zshrc (or source ~/.bashrc)"
echo "To test: quest search jq"
