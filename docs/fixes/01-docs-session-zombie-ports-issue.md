---
title: "Docs session zombie processes and port mismatch"
status: open
---

# Bug Report & Fix Plan: Docs session zombie processes and port mismatch

## Bug summary

When a docs (`fea-docs`) session fails or is stopped, the Astro dev server grandchild process survives, occupying the assigned port (and adjacent ports). On the next start attempt, Vite falls back to a higher port, the portal health-checks the original assigned port, and the session is marked unhealthy.

## Root causes

### Cause 1 – Child process tree not cleaned up (portal)

`DocsSessionFactory.Stop()` at `internal/session/docs.go:47` calls `proc.Kill()` which sends SIGKILL to the **npx** PID only. On macOS the `node astro dev` grandchild survives, holding the port open.

### Cause 2 – fea-docs only handles SIGINT, not SIGTERM (fea-docs)

`start.ts:127–130` registers `SIGINT` only. When the portal sends SIGKILL (unblockable) or switches to SIGTERM in the fix, the handler never fires and `adapter.stopDev()` is never called.

### Cause 3 – Portal assumes the assigned port is always used (portal)

The manager health-checks the originally assigned port (`HealthURL(port)` → `http://localhost:<assignedPort>`). Vite may bind on a different port (e.g. 9306) if 9302–9305 are occupied by zombies.

### Cause 4 – Config rewrite triggers restart mid-flight (fea-docs)

On a cache-hit materialization, `adapter.materialize({ fresh: false })` still rewrites `astro.config.mjs` and content config. Astro's file watcher picks up the change and triggers a dev-server restart, which can race with Vite's dependency pre-bundling and cause `deps_temp_*` ENOENT errors.

## Evidence from logs

```
08:57:21 Starting dev server on port 9302...
08:57:21 Configuration file updated. Restarting...
08:57:23 [ERROR] ENOENT: no such file or directory, open
  'node_modules/.vite/deps_temp_b659ec02/astro_runtime_client_dev-toolbar_entrypoint__js.js'
...
 astro  v6.4.8 ready in 2896 ms
┃ Local    http://localhost:9305/
```

Session removed 120 s later:

```
2026/06/18 08:59:20 session 00807ea0 (pid 17795) did not become healthy in time — removing
```

## Affected components

| Component | File(s) | Issue |
|-----------|---------|-------|
| Workspace portal | `internal/session/docs.go` | Process group not killed; stdout port not detected |
| Workspace portal | `internal/session/docs.go` | `port` used for health-check, not actual bound port |
| Workspace portal | `internal/session/manager.go` | `waitHealthy` polls assigned port, not actual port |
| fea-docs | `src/runtime/adapter.ts` | `startDev` knows actual port but doesn't expose it cleanly |
| fea-docs | `src/cli/commands/start.ts` | No SIGTERM handler |

---

# Fix plan

## Ticket 1: Kill full process group on docs session stop

**Files:** `internal/session/docs.go`, `src/cli/commands/start.ts` (fea-docs)

**Changes (portal — `internal/session/docs.go`):**

1. Add `"syscall"` import.

2. In `Start()`, set `SysProcAttr.Setpgid = true` so the child and its descendants share a single process group.

```go
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
```

3. In `Stop()`, send SIGTERM to the process group (negative PID), wait for graceful shutdown, then SIGKILL process group as fallback.

```go
func (r *DocsSessionFactory) Stop(pid int) error {
    // negative PID = process group
    syscall.Kill(-pid, syscall.SIGTERM)
    // Wait briefly for graceful shutdown (SIGTERM handler in fea-docs)
    time.Sleep(3 * time.Second)
    // Force-kill process group if still alive
    syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck
    return nil
}
```

**Changes (fea-docs — `src/cli/commands/start.ts`):**

4. Add a SIGTERM handler alongside the existing SIGINT handler:

```typescript
const shutdown = () => {
  adapter.stopDev();
  process.exit(0);
};
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
```

**Acceptance:**
- After stopping a docs session, no `node astro dev` processes survive (verify with `pgrep -P <npx-pid>`).
- The assigned port is freed within ~3s of `Stop()` returning.
- All existing session types (opencode, vscode) still stop correctly.

---

## Ticket 2: Discover actual port via SessionFactory interface change

**Files:** `internal/session/session.go`, `internal/session/docs.go`, `internal/session/opencode.go`, `internal/session/vscode.go`, `internal/session/manager.go`, `src/cli/commands/start.ts` (fea-docs), `src/runtime/adapter.ts` (fea-docs)

**Rationale:** The portal health-checks the assigned port, but Vite may fall back to a higher port. We need the portal to discover and use the actual bound port.

**Changes (portal — `internal/session/session.go`):**

1. Change the `SessionFactory` interface — `Start()` now returns the actual port:

```go
type SessionFactory interface {
    Start(dir string, port int) (cmd *exec.Cmd, actualPort int, err error)
    Stop(pid int) error
    HealthURL(port int) string
}
```

**Changes (portal — `internal/session/opencode.go`, `internal/session/vscode.go`):**

2. Update `OCSessionFactory.Start()` and `VSCodeSessionFactory.Start()` to return `cmd, port, nil` — these always bind on the assigned port.

**Changes (portal — `internal/session/docs.go`):**

3. Replace `cmd.Stdout = log.Writer()` with a pipe (`io.Pipe`). Read stdout in a goroutine, scanning for the actual port (match `localhost:(\d+)` emitted by Astro or `##FEA_DOCS_PORT=(\d+)##` — see fea-docs changes below).
4. Block in `Start()` until the port is discovered (with a timeout). Return `cmd, actualPort, nil`.

```go
func (r *DocsSessionFactory) Start(dir string, port int) (*exec.Cmd, int, error) {
    // ... setup ...
    pr, pw := io.Pipe()
    cmd.Stdout = pw

    // Goroutine: scan stdout for actual port
    actualPort := port  // fallback
    portCh := make(chan int, 1)
    go func() {
        scanner := bufio.NewScanner(pr)
        for scanner.Scan() {
            line := scanner.Text()
            if p := parsePortLine(line); p > 0 {
                portCh <- p
                return
            }
        }
    }()

    if err := cmd.Start(); err != nil { ... }

    // Wait for port discovery (up to health-timeout)
    select {
    case actualPort = <-portCh:
    case <-time.After(10 * time.Second):
        log.Printf("warning: docs session port not discovered in time, using assigned %d", port)
    }

    attachCaffeinate(cmd.Process.Pid)
    return cmd, actualPort, nil
}
```

**Changes (portal — `internal/session/manager.go`):**

5. Capture `actualPort` from `Start()` and use it for the `Session.Port`, registry entry, and `HealthURL`:

```go
cmd, actualPort, err := reg.factory.Start(dir, port)
// ...
s := &Session{
    Port:      actualPort,
    // ...
}
// ...
go m.waitHealthy(s, reg.factory.HealthURL(actualPort), healthTimeout)
```

**Changes (fea-docs — `src/cli/commands/start.ts`):**

6. After the dev server starts, emit a machine-parseable line with the actual port (simpler and more reliable than regex for the portal):

```typescript
const port = await adapter.startDev(config.port);
console.log(`##FEA_DOCS_PORT=${port}##`);
```

**Changes (fea-docs — `src/runtime/adapter.ts`):**

7. In `startDev()`, pipe Astro's stdout to `process.stdout` as before (this also carries the `##FEA_DOCS_PORT=##` line).

**Acceptance:**
- If fea-docs binds on a port different from the assigned one, the portal health-checks the actual bound port.
- The session becomes healthy even when port fallback occurs.
- The portal can reliably extract the actual port from stdout using a simple prefix match on `##FEA_DOCS_PORT=`.
- Works even if the assigned port is free (normal case — `actualPort == assignedPort`).

---

## Ticket 3: Add SIGTERM handler in fea-docs

*(Now merged into Ticket 1 — see process group kill above.)*

---

## Ticket 4: Suppress unnecessary config rewrites on cache hit

**File:** `src/runtime/adapter.ts`

**Current behaviour:** `materialize()` rewrites `astro.config.mjs`, `remark-rewrite-md-links.mjs`, `remark-strip-lead-h1.mjs`, `src/content.config.ts`, and `src/content/docs` symlink on every call, even `fresh: false`. This triggers Astro's file watcher → dev server restart → potential Vite dep-optimisation race.

**Changes:**

1. In `writeAstroConfig()` / `writeContentConfig()` / `writeRemarkPlugin()` / `writeStripLeadH1Plugin()`, compare the generated content with the existing file on disk. Skip writing if unchanged.
2. In `writeContentLinks()`, skip symlink creation if the symlink already points to the right target.

**Acceptance:**
- On cache hit (`fresh: false`), no file changes are written when the config hasn't actually changed.
- Astro does not restart unnecessarily.
- The `deps_temp_*` ENOENT error no longer occurs on second invocation.

---

## Implementation order

```
Ticket 1 ──→  (portal: process group + SIGTERM handler)
                    |
Ticket 2 ──→  (interface change + port discovery + ##FEA_DOCS_PORT## line)
                    |
Ticket 4 ──→  (config rewrite suppression — independent of 1-2)
```

Tickets 1 and 2 are a strict sequence (Ticket 2's port discovery depends on the process group + SIGTERM fix removing zombies). Ticket 4 is independent and can be implemented at any time.

## Verification

1. Start a docs session → verify healthy.
2. Kill the portal process (`sudo kill -9 <portal-pid>`) → verify Astro zombie is cleaned up on restart.
3. Start a second docs session while the first is still running → verify no port conflict.
4. Start docs session, stop it via the portal UI → verify port is freed.
5. Start docs session again immediately → verify it binds on the same assigned port (no fallback).
