#!/usr/bin/env bash
set -euo pipefail

PLIST_NAME="com.workspace-portal"
PLIST_PATH="$HOME/Library/LaunchAgents/${PLIST_NAME}.plist"

# Stop and unload
launchctl unload "$PLIST_PATH" 2>/dev/null && echo "Unloaded $PLIST_NAME" || echo "Was not loaded"

# Remove plist
if [ -f "$PLIST_PATH" ]; then
  rm "$PLIST_PATH"
  echo "Removed $PLIST_PATH"
fi

echo ""
echo "The portal binary and config were NOT removed."
echo "To fully clean up:"
echo "  rm /usr/local/bin/portal"
echo "  rm -rf ~/.config/workspace-portal"
echo "  rm -rf ~/.local/share/workspace-portal"
echo "  rm ~/Library/Logs/workspace-portal.log"