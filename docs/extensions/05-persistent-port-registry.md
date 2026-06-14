---
title: "Extension: Persistent Port Registry"
---

# Extension: Persistent Port Registry

## Problem Statement

The workspace portal currently assigns ports dynamically on every session start by scanning the configured port range for the first free port. This means the same directory + session type combination gets a different port every time it is restarted, even if the previously assigned port is still available.

From a user perspective, this creates three daily friction points:

1. **Unstable bookmarks and browser tabs.** If a user bookmarks or navigates directly to a session URL, stopping and restarting the session changes the URL. The user must return to the portal to find the new URL — the old bookmark is broken.

2. **No port identity across reboots.** After a machine or portal restart, previously running sessions that have not yet been restarted lose their port identity entirely. Ports are re-assigned dynamically only when a user manually starts the session again.

3. **Unpredictable tailscale serve URLs.** When Tailscale integration is enabled, each session's Tailscale HTTPS URL includes the port number (e.g. `https://hostname:5101`). A restart can silently change this URL, breaking links shared with colleagues.

The core problem is that the portal has no durable memory of which ports "belong" to which directory+type combinations across restarts and session lifecycles.

## Solution

Add a persistent port registry that maps `(directory, session_type)` composite keys to previously assigned port numbers. The registry lives on disk in a new JSON file alongside the existing session state file.

On every session start, the manager checks the registry first. If a cached port exists, is still within the configured port range for that session type, and is not occupied by any other process, the cached port is reused. Otherwise, a new port is allocated from the range and the registry is updated.

To prevent the registry from accumulating stale entries, any mapping whose port falls within a range being allocated from is purged if its `last_started` timestamp is older than 14 days. Additionally, any cached port that falls outside the current configured range is silently removed and replaced with a fresh allocation.

From the user perspective, the behaviour is:
- **Start a session for dir X → receives port P. Stop it. Start it again → same port P.**
- **Reboot the machine, open the portal, start a session for dir X → same port P as last time.**
- **Reconfigure port ranges (e.g. OpenCode moves from 5100-5199 to 6000-6099) → cached ports outside the new range are replaced on next start.**
- **If an external process has taken the cached port → a new port is allocated and cached.**
- **If a cached entry has not been started in 14+ days → the entry is purged when a session from that range is started, freeing the port for reuse.**

## User Stories

1. As a portal user, I want every session to receive the same port every time I start it for the same directory, so that session URLs are stable across stops and starts.

2. As a portal user, I want port stickiness to survive a full machine reboot, so that I can rely on session URLs being stable across daily use.

3. As a portal user, I want a cached port to be replaced with a new one if the port range for that session type has been reconfigured, so that I never end up on a port outside the intended allocation range.

4. As a portal user, I want a cached port to be replaced with a new one if an external process is already listening on it, so that session startup never fails due to a port conflict.

5. As a portal user, I want stale port assignments (no start in 14+ days) to be cleaned up automatically, so that the port range does not become fragmented with unused reservations.

6. As a maintainer, I want the port registry to be a separate file from the session state, so that the two concerns (live process tracking vs. durable port assignments) can evolve independently.

7. As a maintainer, I want staleness cleanup to happen on-demand (when a session is started for that range) rather than via a background goroutine, so that the portal does not require additional background threads for this feature.

8. As a maintainer, I want the port registry to use the same `(directory, session_type)` composite key as the existing session deduplication logic, so that the mapping is unambiguous.

9. As a maintainer, I want the `last_started` timestamp to update on every `Start()` call (including idempotent returns of an already-running session), so that actively-used mappings never go stale.

10. As a maintainer, I want cached ports to be validated before reuse (range check + port-free check), so that the registry never causes a conflicting port to be reused.

11. As a maintainer, I want `nextPort()` to remain the single source of truth for port allocation, so that all port-free and in-use checks stay consistent.

## Implementation Decisions

- Store the port registry in a new file `port-registry.json` in the same directory as the existing session state file (`~/.local/share/workspace-portal/`).
- Use a JSON map with composite key `"<session_type>:<dir>"` mapping to `PortEntry{Dir, Type, Port, LastStarted}`.
- Load the registry in `NewManager()` alongside the existing `loadState()` call.
- Modify the `Start()` method to:
  1. Check for existing running session (current idempotent dedupe).
  2. Before port allocation, run stale cleanup against the current port range.
  3. Before port allocation, run out-of-range cleanup for the current session type (scoped by `SessionType` to avoid cross-type purging).
  4. Check the registry for a cached entry by `(type, dir)`.
  5. If a cached entry exists and the port is within range and `portFree()`, reuse it.
  6. Otherwise, call `nextPort()` to allocate.
  7. Launch the process via `factory.Start(dir, port)`.
  8. **Only after the process starts successfully**, save the new assignment to the registry.
- Stale cleanup on `Start()`: iterate all registry entries; remove any whose port falls within the current range being allocated from and whose `LastStarted` is older than 14 days.
- Out-of-range cleanup: if a cached port is outside the configured range **for that session type**, remove it from the registry and let `nextPort()` allocate a fresh one. **Crucially, the cleanup is scoped by `SessionType` — starting a docs session will never purge opencode or vscode entries.**
- Thread safety: use a `sync.Mutex` on the registry, separate from the manager's main mutex, to avoid deadlocks during `portFree()` calls.
- Registry serialization: save to disk after every mutation (new entry, update, stale purge) using the same pattern as `saveState()`.
- The `Stop()` method does not modify the registry — port assignments persist across session lifecycles.
- **Startup reconciliation**: when `NewManager()` loads sessions from `sessions.json`, any session whose port is missing from the registry is automatically added. This prevents divergence between the two files after crashes or config changes.
- The script runner session type is not included in this extension (it is not yet implemented).

## Acceptance Criteria

1. Starting a session for a `(dir, type)` pair that has a cached, in-range, free port reuses that exact port.
2. Stopping and re-starting the same `(dir, type)` pair reuses the same port.
3. Restarting the portal binary and starting a session for a previously-cached `(dir, type)` pair reuses the cached port.
4. If the cached port is outside the configured range, a new port is allocated and the registry is updated.
5. If the cached port is in use by another process, a new port is allocated and the registry is updated.
6. Stale entries (port in range, `LastStarted` > 14 days ago) are purged when a session from that range is started.
7. Non-stale entries for other ranges are never touched during purge.
8. **Starting a session of one type never purges registry entries of another type** (cross-type safety).
9. **If `factory.Start()` fails, no registry entry is persisted** — the port is only saved after the process successfully launches.
10. **On portal restart, all live sessions restored from `sessions.json` are automatically added to the port registry** if they are missing.
11. The `last_started` field is updated on every `Start()` call, including idempotent returns.
12. The `Stop()` method never removes registry entries.
13. The registry file is created and written on first allocation.
14. Existing session start/stop/list behaviour is unchanged for all session types.
15. A corrupt or missing registry file is handled gracefully (reset to empty, no crash).

## Testing Decisions

- **Port registry unit tests** (`portregistry_test.go`)
  - Verify load/save round-trip to temp file.
  - Verify composite key generation and lookup.
  - Verify stale purge removes entries older than cutoff but keeps fresh entries.
  - Verify stale purge only removes entries whose port falls within the target range.
  - Verify out-of-range purge removes entries whose port is outside the given range.
  - Verify empty/corrupt file handling.
  - Verify concurrent access safety.

- **Manager integration tests** (`manager_test.go`)
  - Verify that starting a session with a cached free port reuses that port (assert same port returned on second start after stop).
  - Verify that starting with a cached port outside the range allocates a new in-range port.
  - Verify that a cached port occupied by an external listener forces reallocation.
  - Verify stale entries are cleaned up on `Start()` and do not affect allocation.
  - Verify factory failure does not persist a registry entry.
  - Verify sessions restored from `sessions.json` on startup are added to the registry (reconciliation).
  - Verify existing tests remain green (idempotent start, stop, list, state file round-trip, events).

- **Server rendering tests** — no changes needed; port assignment is invisible to the HTTP/HTML surface.

## Out of Scope

- Script runner session types (not yet implemented).
- Background periodic staleness cleanup (on-demand only).
- Web UI for viewing or editing port assignments.
- Port reservation pre-allocation at portal startup (lazy allocation on first start only).
- Migration of existing sessions.json data into the new registry (handled automatically by startup reconciliation).
- Port range conflict detection across different session types (they have separate ranges).
- Retry logic for transient port conflicts (e.g. TIME_WAIT) — immediate fallback to new allocation.

## Further Notes

- This extension is purely additive to the session lifecycle. The port allocation contract changes only in that Start() now consults the registry before falling through to nextPort().
- The composite key format `"<session_type>:<dir>"` uses a colon separator. Directory paths on macOS never contain colons, so there is no ambiguity risk.
- The registry file is designed to be human-readable and debuggable. Operators can inspect or manually edit it if needed.
- Port stickiness improves the Tailscale experience because the same port always maps to the same directory, keeping `tailscale serve` URLs stable.

## Turn-by-Turn Implementation Plan

### Ticket 1 — `PortRegistry` data model + persistence

**File to create:** `internal/session/portregistry.go`

**What to build:**

Define the entry and registry types:

```go
type PortEntry struct {
    Dir         string      `json:"dir"`
    Type        SessionType `json:"type"`
    Port        int         `json:"port"`
    LastStarted time.Time   `json:"last_started"`
}

type PortRegistry struct {
    mu        sync.Mutex
    entries   map[string]*PortEntry
    stateFile string
}
```

Composite key helper:

```go
func registryKey(sessionType SessionType, dir string) string {
    return string(sessionType) + ":" + dir
}
```

Constructor + load:

```go
func NewPortRegistry(stateFile string) *PortRegistry
func LoadPortRegistry(stateFile string) (*PortRegistry, error)
```

`LoadPortRegistry` reads the JSON file. If the file doesn't exist or is corrupt, return an empty registry (log a warning on corruption). If valid, unmarshal into `entries`.

`NewPortRegistry` creates an empty registry with the given file path.

CRUD:

```go
func (r *PortRegistry) Get(key string) (*PortEntry, bool)
func (r *PortRegistry) Set(key string, entry *PortEntry)
func (r *PortRegistry) Delete(key string)
func (r *PortRegistry) Save() error
```

`Save` marshals `entries` to JSON and writes to `r.stateFile` with `0644` perms — same pattern as `saveState()` in manager.go.

Purge methods:

```go
func (r *PortRegistry) PurgeStale(rng portrange.PortRange, cutoff time.Time) int
func (r *PortRegistry) PurgeOutOfRange(rng portrange.PortRange) int
```

- `PurgeStale`: iterate entries; for each entry whose `Port` is within `rng` and `LastStarted.Before(cutoff)`, delete it. Return count.
- `PurgeOutOfRange`: iterate entries; for each entry whose `Port` is outside `rng`, delete it. Return count.

All public methods acquire `r.mu.Lock()`.

**Verify:**
- `go build ./...` compiles cleanly after this ticket.

---

### Ticket 2 — Wire registry into `Manager`

**File to edit:** `internal/session/manager.go`

**Changes:**

1. Add field to `Manager` struct:

```go
registry *PortRegistry
```

2. In `NewManager`, after `loadState()` and registry load, add startup reconciliation:

```go
registryPath := filepath.Join(filepath.Dir(stateFile), "port-registry.json")
m.registry = NewPortRegistry(registryPath)
if loaded, err := LoadPortRegistry(registryPath); err == nil {
    m.registry = loaded
}

// Reconcile: ensure all restored sessions have a registry entry.
// Guards against divergence after crashes or config changes.
for _, s := range m.sessions {
    key := registryKey(s.Type, s.Dir)
    if _, ok := m.registry.Get(key); !ok {
        m.registry.Set(key, &PortEntry{
            Dir:         s.Dir,
            Type:        s.Type,
            Port:        s.Port,
            LastStarted: s.StartedAt,
        })
    }
}
m.registry.Save()
```

3. In `Start()`, **after** the idempotent dedupe block and **before** the `nextPort()` call, insert:

```go
reg, ok := m.factories[sessionType]
if !ok {
    return nil, fmt.Errorf("unknown session type: %s", sessionType)
}

// --- registry lookup ---
m.registry.PurgeStale(reg.portRange, time.Now().Add(-14*24*time.Hour))
m.registry.PurgeOutOfRange(reg.portRange, sessionType)

key := registryKey(sessionType, dir)
var port int
if entry, ok := m.registry.Get(key); ok && entry.Port >= reg.portRange[0] && entry.Port <= reg.portRange[1] && portFree(entry.Port) {
    port = entry.Port
} else {
    allocated, err := m.nextPort(reg.portRange)
    if err != nil {
        return nil, err
    }
    port = allocated
}

cmd, err := reg.factory.Start(dir, port)
if err != nil {
    return nil, err
}

m.registry.Set(key, &PortEntry{
    Dir:         dir,
    Type:        sessionType,
    Port:        port,
    LastStarted: time.Now(),
})
m.registry.Save()
// --- end registry lookup ---
```

4. Remove the original `port, err := m.nextPort(reg.portRange)` line — it is superseded by the registry block above. The `port` variable is now set by the registry logic.

**Key differences from the original design:**
- `PurgeOutOfRange` receives `sessionType` to scope cleanup to entries of the current type only.
- `registry.Set` + `Save` is placed **after** `factory.Start()` — the port is only persisted once the process successfully launches.

**Verify:**
- `go build ./...` compiles.
- `go test ./internal/session/ -run TestManager -v` passes.

---

### Ticket 3 — Stale cleanup + out-of-range logging (type-scoped)

**Files to edit:** `internal/session/portregistry.go`

**Changes:**

Add `"log"` to the import block.

In `PurgeStale`, add logging:

```go
func (r *PortRegistry) PurgeStale(rng portrange.PortRange, cutoff time.Time) int {
    r.mu.Lock()
    defer r.mu.Unlock()
    count := 0
    for k, e := range r.entries {
        if e.Port >= rng[0] && e.Port <= rng[1] && e.LastStarted.Before(cutoff) {
            delete(r.entries, k)
            count++
        }
    }
    if count > 0 {
        log.Printf("port registry: purged %d stale entries in range %d-%d", count, rng[0], rng[1])
    }
    return count
}
```

In `PurgeOutOfRange`, add a `SessionType` parameter and filter by it:

```go
func (r *PortRegistry) PurgeOutOfRange(rng portrange.PortRange, sessionType SessionType) int {
    r.mu.Lock()
    defer r.mu.Unlock()
    count := 0
    for k, e := range r.entries {
        if e.Type == sessionType && (e.Port < rng[0] || e.Port > rng[1]) {
            delete(r.entries, k)
            count++
        }
    }
    if count > 0 {
        log.Printf("port registry: purged %d out-of-range entries of type %q (range %d-%d)", count, sessionType, rng[0], rng[1])
    }
    return count
}
```

**Verify:**
- `go build ./...` compiles.

---

### Ticket 4 — Tests

**File to create:** `internal/session/portregistry_test.go`

**Test cases:**

```go
func TestRegistryKey_Format(t *testing.T)
```
Verify `registryKey("opencode", "/my/proj")` returns `"opencode:/my/proj"`.

```go
func TestPortRegistry_LoadMissingFile(t *testing.T)
```
Load from non-existent path → empty registry, no error.

```go
func TestPortRegistry_LoadCorruptFile(t *testing.T)
```
Write garbage JSON, load → empty registry (logged warning, no panic).

```go
func TestPortRegistry_SaveAndLoadRoundTrip(t *testing.T)
```
Set entries, save, load into new registry, assert entries match.

```go
func TestPortRegistry_GetSetDelete(t *testing.T)
```
Set entry, Get returns it. Delete, Get returns false.

```go
func TestPortRegistry_PurgeStale(t *testing.T)
```
- Entry older than cutoff within range → removed.
- Entry newer than cutoff within range → kept.
- Entry older than cutoff but outside range → kept.

```go
func TestPortRegistry_PurgeOutOfRange(t *testing.T)
```
- Entry port above range → removed.
- Entry port below range → removed.
- Entry port within range → kept.

```go
func TestPortRegistry_PurgeOutOfRange_OnlyPurgesMatchingType(t *testing.T)
```
- Inject entries of different types.
- PurgeOutOfRange with docs range → opencode and vscode entries survive.

```go
func TestPortRegistry_ConcurrentAccess(t *testing.T)
```
Spawn 10 goroutines doing Get/Set/Delete/Purge on the same registry; use `-race` to verify no races.

**File to edit:** `internal/session/manager_test.go`

**New tests:**

```go
func TestStart_ReusesCachedPort(t *testing.T)
```
1. Start a session → record port P.
2. Stop the session.
3. Start the same (type, dir) again → assert same port P.

```go
func TestStart_CachedPortOutOfRange(t *testing.T)
```
1. Manually inject a registry entry with a port outside the manager's range.
2. Start a session → assert assigned port is within range and different from cached port.

```go
func TestStart_CachedPortInUse(t *testing.T)
```
1. Inject a registry entry with an in-range port.
2. `net.Listen` on that port (occupy it).
3. Start a session → assert assigned port is different from occupied port.
4. Close listener.

```go
func TestStart_StaleEntryPurged(t *testing.T)
```
1. Inject a registry entry with `LastStarted` > 14 days ago.
2. Start a session for a different (type, dir) in the same range.
3. Assert the stale entry is gone from the registry.

**Test helper** — extend or add alongside `newTestManager`:

```go
func newTestManagerWithRegistry(t *testing.T, factory *fakeFactory, registry *PortRegistry) *Manager {
    t.Helper()
    stateFile := filepath.Join(t.TempDir(), "sessions.json")
    pr := portrange.PortRange{40000, 40099}
    m := NewManager(stateFile, &NoopRegistrar{}, "", Register(SessionTypeOpenCode, factory, pr))
    if registry != nil {
        m.registry = registry
        m.registry.stateFile = filepath.Join(t.TempDir(), "port-registry.json")
    }
    return m
}
```

**Verify:**
- `go test ./internal/session/ -v -race -count=1` — all tests pass, no races.

---

### Ticket 5 — Build and deploy

**Steps:**

1. Compile:
```bash
make build
```

2. Deploy (adjust to match actual deploy flow):
```bash
make deploy
```

3. Restart launchd:
```bash
launchctl unload ~/Library/LaunchAgents/com.workspace-portal.plist
launchctl load -w ~/Library/LaunchAgents/com.workspace-portal.plist
```

4. Verify service:
```bash
launchctl list | grep workspace-portal
curl -sf http://127.0.0.1:5900/ > /dev/null && echo "portal up"
```

5. Smoke-test port stickiness:
   - Open portal, start an OpenCode session for any directory.
   - Note the port from the Running Sessions list.
   - Stop the session.
   - Start it again — confirm the same port.

6. Smoke-test stale cleanup:
   - Locate `~/.local/share/workspace-portal/port-registry.json`.
   - Manually set a `last_started` to 15 days ago.
   - Start a session for that range → confirm the entry was purged (check logs or re-inspect file).

**Verify:**
- Binary compiles and service restarts cleanly.
- Port stickiness works end-to-end in the UI.
- Stale and out-of-range cleanup behave as expected.
