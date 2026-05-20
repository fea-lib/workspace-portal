---
title: 'workspace-portal'
---

# workspace-portal

A self-hosted, mobile-friendly portal for launching and managing
[OpenCode](https://opencode.ai), [code-server](https://github.com/coder/code-server), and
[`fea-docs`](https://github.com/fea-lib/fea-docs)
sessions across your workspaces directory.

Built with Go + HTMX. Single binary.

---

## Requirements

- macOS
- Go 1.22+ (to build from source) or download a pre-built binary from [GitHub Releases](https://github.com/yourusername/workspace-portal/releases)
- [opencode](https://opencode.ai) installed and on `PATH`
- [code-server](https://github.com/coder/code-server) installed and on `PATH`
- Node.js 18+ and `npx` on `PATH` (for docs sessions via `fea-docs`)

---

## Quick Start (macOS native)

### 1. Clone and build

```bash
git clone https://github.com/yourusername/workspace-portal
cd workspace-portal
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
go run ./cmd/portal --config ~/.config/workspace-portal/config.yaml
# Open http://localhost:4000
```

### 4. Install as a background service

```bash
make install
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

**Docs session won't start** — ensure `node` and `npx` are available in the launchd PATH, and that `docs.package` in your config points to a valid package spec (default: `fea-docs@latest`).

**Docs files changed unexpectedly** — `fea-docs` may auto-inject missing frontmatter titles into scanned `.md/.mdx` files. This is expected upstream behavior.

**Sessions not persisting across restarts** — state is written to `~/.local/share/workspace-portal/sessions.json`. If the directory is not writable, session state is lost. The portal logs an error at startup.
