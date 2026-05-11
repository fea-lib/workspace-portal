#!/usr/bin/env bash
set -euo pipefail

# ─── Defaults ─────────────────────────────────────────────────────────────────
BINARY="${PORTAL_BINARY:-/usr/local/bin/portal}"
CONFIG="${PORTAL_CONFIG:-$HOME/.config/workspace-portal/config.yaml}"
LOG="${PORTAL_LOG:-$HOME/Library/Logs/workspace-portal.log}"
PLIST_NAME="com.workspace-portal"
PLIST_DST="$HOME/Library/LaunchAgents/${PLIST_NAME}.plist"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ─── Check the binary exists ──────────────────────────────────────────────────
if [ ! -f "$BINARY" ]; then
  echo "Error: portal binary not found at $BINARY"
  echo "Build it first: go build -o $BINARY ./cmd/portal"
  exit 1
fi

# ─── Create config directory ──────────────────────────────────────────────────
mkdir -p "$(dirname "$CONFIG")"
if [ ! -f "$CONFIG" ]; then
  echo "No config file found at $CONFIG"
  echo "Creating from example — edit before starting the service."
  cp "$SCRIPT_DIR/../../config.example.yaml" "$CONFIG"
fi

# ─── Substitute template ──────────────────────────────────────────────────────
sed \
  -e "s|PORTAL_BINARY|${BINARY}|g" \
  -e "s|PORTAL_CONFIG|${CONFIG}|g" \
  -e "s|PORTAL_LOG|${LOG}|g" \
  -e "s|PORTAL_HOME|${HOME}|g" \
  "$SCRIPT_DIR/com.workspace-portal.plist.tmpl" \
  > "$PLIST_DST"

echo "Wrote plist to $PLIST_DST"

# ─── Load or reload the agent ─────────────────────────────────────────────────
# Unload first in case it is already loaded (idempotent reinstall)
launchctl unload "$PLIST_DST" 2>/dev/null || true
launchctl load -w "$PLIST_DST"

echo "Agent loaded. Check status with:"
echo "  launchctl list | grep workspace-portal"
echo "  tail -f $LOG"