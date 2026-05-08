---
title: "Course 05 — Deployment"
---

# Course 05 — Deployment

**Goal:** Deploy workspace-portal as a persistent macOS service using `launchd`, and write the `README` and `config.example.yaml` for open-source distribution.  
**Prerequisite:** [Course 04 — Script Runner](./course-04-script-runner.md)  
**Output:** The portal running permanently on your Mac as a launchd service, accessible at `http://localhost:4000`.

---

## Lesson 1 — What is launchd?

### macOS process management

On macOS, background services are managed by `launchd` — Apple's replacement for `cron`, `init`, and `inetd`. Every long-running process on the system (from `Spotlight` to `ssh-agent`) is a launchd agent or daemon.

There are two categories:

| Type | Location | Runs as | Starts when |
|---|---|---|---|
| **LaunchAgent** | `~/Library/LaunchAgents/` | Logged-in user | User logs in |
| **LaunchDaemon** | `/Library/LaunchDaemons/` | root (or specified user) | System boot |

The portal is a **LaunchAgent** — it runs as you, has access to your home directory, and starts when you log in. This is correct: it needs to read your workspaces and spawn processes as you.

### The plist format

launchd configuration is a Property List (plist) — an XML format. The key fields:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <!-- Unique identifier for the agent -->
  <key>Label</key>
  <string>com.workspace-portal</string>

  <!-- Command to run -->
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/portal</string>
    <string>--config</string>
    <string>/Users/yourname/.config/workspace-portal/config.yaml</string>
  </array>

  <!-- Start on login -->
  <key>RunAtLoad</key>
  <true/>

  <!-- Restart on crash -->
  <key>KeepAlive</key>
  <true/>

  <!-- Log stdout and stderr -->
  <key>StandardOutPath</key>
  <string>/Users/yourname/Library/Logs/workspace-portal.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/yourname/Library/Logs/workspace-portal.log</string>

  <!-- Working directory -->
  <key>WorkingDirectory</key>
  <string>/Users/yourname</string>
</dict>
</plist>
```

---

## Lesson 2 — The launchd Plist Template

Create `deploy/launchd/com.workspace-portal.plist.tmpl`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.workspace-portal</string>

  <key>ProgramArguments</key>
  <array>
    <string>PORTAL_BINARY</string>
    <string>--config</string>
    <string>PORTAL_CONFIG</string>
  </array>

  <key>RunAtLoad</key>
  <true/>

  <key>KeepAlive</key>
  <true/>

  <key>ThrottleInterval</key>
  <integer>10</integer>

  <key>StandardOutPath</key>
  <string>PORTAL_LOG</string>

  <key>StandardErrorPath</key>
  <string>PORTAL_LOG</string>

  <key>WorkingDirectory</key>
  <string>PORTAL_HOME</string>

  <!-- Environment variables available to the portal process -->
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>PORTAL_HOME</string>
    <key>PATH</key>
    <string>/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
  </dict>
</dict>
</plist>
```

**`ThrottleInterval: 10`** — if the portal crashes, launchd waits at least 10 seconds before restarting it. Without this, a crashing binary can cause a tight restart loop that pegs the CPU.

**`EnvironmentVariables`** — launchd agents do not inherit your shell's `PATH`. The portal spawns `opencode` and `code-server` by name; they must be on the `PATH` in the plist, not just in your `.zshrc`.

---

## Lesson 3 — The Install Script

Create `deploy/launchd/install.sh`:

```bash
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
```

Make it executable:

```bash
chmod +x deploy/launchd/install.sh
```

### Run the installer

```bash
# Build the binary first
go build -o /usr/local/bin/portal ./cmd/portal

# Install the agent
./deploy/launchd/install.sh
```

The portal starts immediately and on every subsequent login. It logs to `~/Library/Logs/workspace-portal.log`.

### launchctl cheat sheet

```bash
# Check if the agent is running
launchctl list | grep workspace-portal

# Stop the agent (temporary — starts again on next login)
launchctl stop com.workspace-portal

# Start the agent
launchctl start com.workspace-portal

# Unload permanently (disable autostart)
launchctl unload ~/Library/LaunchAgents/com.workspace-portal.plist

# Reload after editing the plist
launchctl unload ~/Library/LaunchAgents/com.workspace-portal.plist
launchctl load -w ~/Library/LaunchAgents/com.workspace-portal.plist

# View logs
tail -f ~/Library/Logs/workspace-portal.log
```

---

## Lesson 4 — The `config.example.yaml`

Create `config.example.yaml` at the repo root. Every option must be documented:

```yaml
# workspace-portal — config.example.yaml
# Copy to ~/.config/workspace-portal/config.yaml and fill in your values.
# All values can also be set via environment variables (prefix: PORTAL_).
# Env vars take precedence over config file values.

# ─── Required ─────────────────────────────────────────────────────────────────

# Absolute path to the directory you want to browse and launch sessions from.
# Example: /Users/yourname/workspaces
workspaces_root: ~/workspaces

# ─── Server ───────────────────────────────────────────────────────────────────

# Port the portal HTTP server listens on.
# Env: PORTAL_PORTAL_PORT
portal_port: 4000

# ─── Secrets ──────────────────────────────────────────────────────────────────

# Directory containing secret files (one file per secret, named by secret name).
# Relative paths are resolved from the config file location.
# Docker users: secrets are also resolved from /run/secrets/{name} automatically.
# Env: PORTAL_SECRETS_DIR
secrets_dir: .secrets

# ─── OpenCode ─────────────────────────────────────────────────────────────────

oc:
  # Path to the opencode binary. Must be on PATH or absolute.
  # Env: PORTAL_OC_BINARY
  binary: opencode

  # Port range to assign to OC sessions. First free port is used.
  # Env: PORTAL_OC_PORT_RANGE (format: "4100-4199")
  port_range: [4100, 4199]

  # Extra CLI flags to pass to opencode on startup.
  # Default: ["web", "--mdns"] — starts OC in web mode with mDNS disabled.
  # Env: PORTAL_OC_FLAGS (comma-separated)
  flags:
    - web

# ─── VS Code (code-server) ────────────────────────────────────────────────────

vscode:
  # Path to the code-server binary. Must be on PATH or absolute.
  # Env: PORTAL_VSCODE_BINARY
  binary: code-server

  # Port range to assign to VS Code sessions.
  # Env: PORTAL_VSCODE_PORT_RANGE
  port_range: [4200, 4299]

# ─── Filesystem ───────────────────────────────────────────────────────────────

fs:
  # Additional directory names to hide when expanding the tree.
  # These are additive to the built-in prune list (node_modules, dist, etc.).
  # Env: PORTAL_FS_PRUNE_DIRS (comma-separated)
  prune_dirs: []
```

---

## Lesson 5 — The `.secrets.example/` Directory

Create `secrets.example/` at the repo root with example files:

```
secrets.example/
  vscode-password      ← contents: "change-me"
  README.md            ← instructions
```

`secrets.example/README.md`:

```markdown
# Secrets

Copy this directory to `.secrets/` alongside your `config.yaml`.
Fill in the actual values. Never commit `.secrets/`.

## Files

| File | Purpose |
|---|---|
| `vscode-password` | Password for code-server sessions |

## gitignore

Add `.secrets/` to your `.gitignore`:
    .secrets/
```

Add to `.gitignore`:

```
.secrets/
*.local
```

---

## Lesson 6 — The README

Create `README.md` at the repo root. Keep it task-focused: prerequisites → install → configure → run:

````markdown
# workspace-portal

A self-hosted, mobile-friendly portal for launching and managing
[OpenCode](https://opencode.ai) and [code-server](https://github.com/coder/code-server)
sessions across your workspaces directory.

Built with Go + HTMX. Single binary. No Node.js required.

---

## Requirements

- macOS
- Go 1.22+ (to build from source) or download a pre-built binary from [GitHub Releases](https://github.com/yourusername/workspace-portal/releases)
- [opencode](https://opencode.ai) installed and on `PATH`
- [code-server](https://github.com/coder/code-server) installed and on `PATH`

---

## Quick Start (macOS native)

### 1. Clone and build

```bash
git clone https://github.com/yourusername/workspace-portal
cd workspace-portal
go build -o /usr/local/bin/portal ./cmd/portal
```

### 2. Configure

```bash
mkdir -p ~/.config/workspace-portal
cp config.example.yaml ~/.config/workspace-portal/config.yaml
# Edit config.yaml: set workspaces_root to your workspaces directory

cp -r secrets.example .secrets
# Edit .secrets/vscode-password: set a password for code-server
```

### 3. Run (manual test)

```bash
portal --config ~/.config/workspace-portal/config.yaml
# Open http://localhost:4000
```

### 4. Install as a background service

```bash
./deploy/launchd/install.sh
# Portal starts now and on every login
# Logs: ~/Library/Logs/workspace-portal.log
```

---

## Configuration

All options are documented in `config.example.yaml` (in the portal repo root).

Environment variables override config file values. Prefix: `PORTAL_`.

Example:
```bash
PORTAL_WORKSPACES_ROOT=/home/user/projects portal
```

---

## Stopping the service

```bash
launchctl stop com.workspace-portal       # temporary stop
launchctl unload ~/Library/LaunchAgents/com.workspace-portal.plist  # permanent disable
```

---

## Troubleshooting

**Port conflicts** — if OC/VS Code sessions fail to start, check that the port ranges in config are not already in use:
```bash
lsof -iTCP -sTCP:LISTEN -P | grep 410
```

**Binary not found** — ensure `opencode` and `code-server` are on the `PATH` defined in the launchd plist (`/opt/homebrew/bin` is included by default for Homebrew users).

**Sessions not persisting across restarts** — state is written to `~/.local/share/workspace-portal/sessions.json`. If the directory is not writable, session state is lost. The portal logs an error at startup.
````

---

## Lesson 7 — The Uninstall Script

For completeness, provide an uninstall script at `deploy/launchd/uninstall.sh`:

```bash
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
```

```bash
chmod +x deploy/launchd/uninstall.sh
```

---

## Lesson 8 — Log Rotation

`launchd` does not rotate logs by default. Your portal log can grow unbounded. macOS ships with `newsyslog` for log rotation.

Create `/etc/newsyslog.d/workspace-portal.conf` (requires sudo):

```
# logfile                                          owner  mode count  size  when  flags
/Users/yourname/Library/Logs/workspace-portal.log  yourname:staff  640   7     1024  *     J
```

Fields: path, owner:group, permissions, how many rotated files to keep (7), rotate when file reaches 1024 KB, rotate at any time (`*`), `J` = compress with bzip2.

For a personal portal, simpler is fine — just truncate manually when it gets large, or add this to your `install.sh`:

```bash
# Limit log to 10 MB using launchd's built-in log size limit (macOS 13+)
# Not available in older macOS; use newsyslog instead.
```

---

## Lesson 9 — Production Checklist

Before considering the portal "production-ready" for daily use:

### Security
- [ ] The portal has no authentication. It relies on Tailscale (or your VPN/reverse proxy) for access control. Do not expose port 4000 to the public internet.
- [ ] The `.secrets/` directory has permissions `700` (owner read/write only): `chmod 700 ~/.secrets && chmod 600 ~/.secrets/*`
- [ ] `config.yaml` does not contain secrets — only references to secret names.

### Reliability
- [ ] launchd `KeepAlive: true` is set so the portal restarts on crash.
- [ ] `ThrottleInterval: 10` prevents crash loops.
- [ ] Session state is on a writable path.

### Observability
- [ ] Logs are written to `~/Library/Logs/workspace-portal.log`.
- [ ] You have run `tail -f ~/Library/Logs/workspace-portal.log` and confirmed normal startup messages appear.
- [ ] You have confirmed the portal recovers after a `launchctl stop` + `launchctl start`.

### Open Source readiness
- [ ] `config.example.yaml` is committed with all options documented.
- [ ] `secrets.example/` is committed with placeholder values.
- [ ] `.secrets/` is in `.gitignore` and has never been committed.
- [ ] The `README.md` install steps work on a clean machine (tested in a fresh shell).

---

## Summary

The portal is now:

1. **Built** — a single static Go binary at `/usr/local/bin/portal`
2. **Configured** — `~/.config/workspace-portal/config.yaml` with your workspaces root and port ranges
3. **Running persistently** — launchd starts it on login, restarts it on crash, logs to `~/Library/Logs/`
4. **Accessible locally** — `http://localhost:4000`
5. **Open-source ready** — example config and secrets, documented README, zero machine-specific values in the repo

To expose the portal and its sessions securely over HTTPS from any device, continue to **[Course 06 — Tailscale Setup](./course-06-tailscale.md)**.

---

## What to Build Next

Some natural extensions to the portal (none required, all good learning projects):

- **Portal authentication** — a simple password form with a session cookie, so the portal itself is not fully open on the tailnet
- **Auto-idle shutdown** — stop sessions that have had no HTTP traffic for N minutes (requires a small reverse-proxy wrapper)
- **Terminal emulator** — embed [ttyd](https://github.com/tsl0922/ttyd) as a session type for browser-based shell access
- **Systemd support** — a `deploy/systemd/` equivalent of the launchd scripts for Linux bare-metal deployment
- **GitHub Actions CI** — build and push pre-built binaries to GitHub Releases on every tag using `goreleaser`
