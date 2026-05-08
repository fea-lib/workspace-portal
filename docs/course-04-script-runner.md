---
title: "Course 04 — Script Runner"
---

# Course 04 — Script Runner

**Goal:** Wire the script runner end-to-end — from detecting `package.json` in the directory tree, through the `<dialog>`-based script picker in the UI, to spawning the chosen npm/pnpm/yarn/bun script as a managed session.  
**Prerequisite:** [Course 03 — HTMX and Server-Sent Events](./course-03-htmx-and-sse.md)  
**Output:** Directory rows with a `package.json` show a "Scripts" button. Tapping it opens a dialog listing all scripts. Selecting a script spawns it as a web server process (with `--port` injected) and tracks it in the running sessions list alongside OpenCode and VS Code sessions.

---

## Lesson 1 — What the Script Runner Does

### The problem

Modern front-end projects define development servers, Storybook, documentation sites, and other tools as npm scripts. Launching them from a mobile device currently requires SSH or an already-running terminal. The portal solves this by exposing them in the tree UI.

### The design

When `internal/fs.List` annotates a directory as having a `package.json`, the tree row renders a "Scripts" button. Tapping it opens a native HTML `<dialog>` element (no external library) showing all scripts from that `package.json`. The user picks one; the portal:

1. Assigns a free port from `scripts.port_range` (default `[4300, 4399]`)
2. Spawns the script using `{pm} run {scriptName} -- --port {port}` with `cmd.Dir` set to the project directory
3. Health-checks by polling `http://localhost:{port}/` until 200 or 30-second timeout
4. Registers the session in the manager with `Type = SessionTypeScript` and `Label = scriptName`
5. Emits `session.started` and (on health) `session.healthy` SSE events

The script session then appears in the running sessions list with its script name as the label, exactly like any OpenCode or VS Code session.

### Why native `<dialog>`?

The HTML `<dialog>` element is a modal that:
- Is keyboard-accessible out of the box (Escape closes it, focus is trapped inside while open)
- Needs zero JavaScript to open from a button (`showModal()` is the only JS call)
- Renders above all other content without CSS stacking-context tricks
- Is supported in all modern browsers (including mobile Safari 15.4+)

No library, no custom overlay div, no z-index battles.

---

## Lesson 2 — `internal/fs`: Detecting `package.json`

Add `HasPackageJSON bool` to `DirEntry` in `internal/fs/fs.go`, then wire the detection logic inside `List`.

### Adding the field to `DirEntry`

```go
type DirEntry struct {
    Path           string
    Name           string
    IsGit          bool
    HasChildren    bool
    HasPackageJSON bool
}
```

### Adding the detection to `List`

In `internal/fs/fs.go`, after the `IsGit` check, add a stat for `package.json`:

```go
// Inside the loop that populates each DirEntry:

entry := DirEntry{
    Path:        relPath,
    Name:        name,
    IsGit:       isGitRepo(absPath),
    HasChildren: hasVisibleSubdirs(absPath, root, matchers),
}

// Check for package.json in this directory.
if _, err := os.Stat(filepath.Join(absPath, "package.json")); err == nil {
    entry.HasPackageJSON = true
}

result = append(result, entry)
```

### Test coverage

Add a test case to `internal/fs/fs_test.go`:

```go
// TestHasPackageJSON verifies that HasPackageJSON is set correctly.
func TestHasPackageJSON(t *testing.T) {
    root := t.TempDir()
    // Create a directory with a package.json
    withPkg := filepath.Join(root, "with-pkg")
    os.Mkdir(withPkg, 0755)
    os.WriteFile(filepath.Join(withPkg, "package.json"), []byte(`{"scripts":{}}`), 0644)
    // Create a directory without
    noPkg := filepath.Join(root, "no-pkg")
    os.Mkdir(noPkg, 0755)

    entries, err := List(root, root)
    if err != nil {
        t.Fatal(err)
    }
    byName := make(map[string]DirEntry)
    for _, e := range entries {
        byName[e.Name] = e
    }
    if !byName["with-pkg"].HasPackageJSON {
        t.Error("expected HasPackageJSON=true for dir with package.json")
    }
    if byName["no-pkg"].HasPackageJSON {
        t.Error("expected HasPackageJSON=false for dir without package.json")
    }
}
```

---

## Lesson 3 — Package Manager Detection

The package manager is detected at start time by checking for lockfiles in the project directory. This logic lives in a new file `internal/session/script.go`.

### Why check at start time, not config time?

Different projects in the same workspaces root can use different package managers. Detecting from lockfiles per-project is more accurate than a global config default. The detection is cheap (four `os.Stat` calls) and happens only when the script is started.

### The detection order matters

`bun.lockb` is checked first because bun projects may also have a `package-lock.json` (created by npm for compatibility). Checking bun first ensures the correct runner is used.

### Testing the detection

```go
// internal/session/script_test.go
package session

import (
    "os"
    "path/filepath"
    "testing"
)

func TestDetectPackageManager(t *testing.T) {
    tests := []struct {
        name    string
        files   []string
        want    string
    }{
        {"bun wins over npm", []string{"bun.lockb", "package-lock.json"}, "bun"},
        {"pnpm", []string{"pnpm-lock.yaml"}, "pnpm"},
        {"yarn", []string{"yarn.lock"}, "yarn"},
        {"npm explicit", []string{"package-lock.json"}, "npm"},
        {"fallback", []string{}, "npm"},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            dir := t.TempDir()
            for _, f := range tc.files {
                os.WriteFile(filepath.Join(dir, f), []byte{}, 0644)
            }
            got := detectPackageManager(dir)
            if got != tc.want {
                t.Errorf("got %q, want %q", got, tc.want)
            }
        })
    }
}
```

---

## Lesson 4 — `ReadScripts`: Reading `package.json`

Add `ReadScripts(dir string) (map[string]string, error)` to `internal/session/script.go`. It reads `package.json` in the given directory and returns the `scripts` map.

### Test coverage

```go
func TestReadScripts(t *testing.T) {
    dir := t.TempDir()
    pkg := `{"name":"my-app","scripts":{"dev":"vite","build":"vite build","storybook":"storybook dev"}}`
    os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

    scripts, err := ReadScripts(dir)
    if err != nil {
        t.Fatal(err)
    }
    if scripts["dev"] != "vite" {
        t.Errorf("got dev=%q, want %q", scripts["dev"], "vite")
    }
    if len(scripts) != 3 {
        t.Errorf("got %d scripts, want 3", len(scripts))
    }
}

func TestReadScripts_NoFile(t *testing.T) {
    dir := t.TempDir()
    _, err := ReadScripts(dir)
    if err == nil {
        t.Error("expected error for missing package.json")
    }
}
```

---

## Lesson 5 — UI Wiring: Scripts in the Tree

With `HasPackageJSON` detected and `ReadScripts` available, we can wire them into the server-side rendering pipeline so the tree shows a "Scripts" button for directories that have a `package.json`.

### Add `Scripts` to `treeRowData`

In `internal/server/templates.go`, extend `treeRowData` with a `Scripts` field:

```go
// treeRowData is passed to tree-row.html for each directory entry.
type treeRowData struct {
    fs.DirEntry
    // Expanded is set server-side when rendering children inline.
    // For lazily-loaded rows it is always false on first render.
    Expanded bool
    // Scripts is populated from package.json when HasPackageJSON is true.
    // Keys are script names, values are the command strings.
    Scripts map[string]string
}
```

### Populate `Scripts` in the `index` handler

In `internal/server/handlers.go`, update the row-building loop in both `index` and `fsList`:

```go
// index handler
rows := make([]treeRowData, len(entries))
for i, e := range entries {
    row := treeRowData{DirEntry: e}
    if e.HasPackageJSON {
        scripts, _ := session.ReadScripts(filepath.Join(h.cfg.WorkspacesRoot, e.Path))
        row.Scripts = scripts
    }
    rows[i] = row
}
```

```go
// fsList handler
rows := make([]treeRowData, len(entries))
for i, e := range entries {
    row := treeRowData{DirEntry: e}
    if e.HasPackageJSON {
        scripts, _ := session.ReadScripts(absPath)
        row.Scripts = scripts
    }
    rows[i] = row
}
```

> We ignore the `ReadScripts` error here with `_`. If reading fails (corrupted JSON, permission error), `row.Scripts` stays `nil` — the template's `{{range .Scripts}}` produces no output, so the dialog is empty but no crash occurs. A production hardening step could log the error.

### Add the Scripts button and `<dialog>` to `tree-row.html`

Update `internal/assets/templates/tree-row.html`:

```html
{{define "tree-row.html"}}
<li class="tree-item" id="item-{{.SafeID}}">
  <div class="tree-row">
    {{if .HasChildren}}
    <span class="tree-icon"
          hx-get="/fs/list?path={{.Path}}"
          hx-target="#children-{{.SafeID}}"
          hx-swap="innerHTML"
          hx-on:click="toggleChildren(this, '{{.SafeID}}')"
          title="Expand">▶</span>
    {{else}}
    <span class="tree-icon" style="color:#2d3748">—</span>
    {{end}}

    <span class="tree-name{{if .IsGit}} git{{end}}">{{.Name}}</span>

    <div class="tree-actions">
      <button class="btn btn-oc"
              hx-post="/sessions/start"
              hx-vals='{"type":"opencode","dir":"{{.Path}}"}'
              hx-target="#sessions"
              hx-swap="innerHTML"
              hx-indicator="#sessions-indicator">
        OpenCode
      </button>
      <button class="btn btn-vs"
              hx-post="/sessions/start"
              hx-vals='{"type":"vscode","dir":"{{.Path}}"}'
              hx-target="#sessions"
              hx-swap="innerHTML"
              hx-indicator="#sessions-indicator">
        VS
      </button>
      {{if .HasPackageJSON}}
      <button class="btn btn-scripts"
              onclick="document.getElementById('scripts-dialog-{{.SafeID}}').showModal()">
        Scripts
      </button>
      {{end}}
    </div>
  </div>

  {{if .HasPackageJSON}}
  <dialog id="scripts-dialog-{{.SafeID}}" class="scripts-dialog">
    <form method="dialog">
      <h3>Run a script in <code>{{.Name}}</code></h3>
      <ul class="scripts-list">
        {{range $name, $cmd := .Scripts}}
        <li>
          <button class="btn btn-script-run"
                  hx-post="/sessions/start"
                  hx-vals='{"type":"script","dir":"{{$.Path}}","script":"{{$name}}"}'
                  hx-target="#sessions"
                  hx-swap="innerHTML"
                  hx-indicator="#sessions-indicator"
                  onclick="this.closest('dialog').close()">
            <span class="script-name">{{$name}}</span>
            <span class="script-cmd">{{$cmd}}</span>
          </button>
        </li>
        {{end}}
      </ul>
      <button class="btn btn-close" value="cancel">Cancel</button>
    </form>
  </dialog>
  {{end}}

  {{if .HasChildren}}
  <ul class="tree children tree-indent" id="children-{{.SafeID}}">
    <!-- children loaded lazily via hx-get above -->
  </ul>
  {{end}}
</li>
{{end}}
```

> **`$.Path` inside `{{range}}`:** Inside a `{{range}}` block, dot (`.`) becomes the iteration value (`$name`, `$cmd`). To access the outer `treeRowData`, use `$` — the initial dot captured before the range. `{{$.Path}}` gives the directory path of the row, not the script name.

---

## Lesson 6 — Wiring the `sessionsStart` Handler

The `POST /sessions/start` handler already handles `type=opencode` and `type=vscode`. We need to extend it to handle `type=script`.

### The `script` form value

When the user clicks a script button in the dialog, the HTMX request includes:

```
type=script
dir=/relative/path/to/project
script=docs:dev
```

### Updated handler

Find `sessionsStart` in `internal/server/handlers.go` and add the script case:

```go
func (h *handler) sessionsStart(w http.ResponseWriter, r *http.Request) {
    r.ParseForm()
    sessionType := session.SessionType(r.FormValue("type"))
    dir := r.FormValue("dir")
    scriptName := r.FormValue("script") // only set when type=script

    // Resolve and validate the directory
    absDir := filepath.Join(h.cfg.WorkspacesRoot, dir)
    if !strings.HasPrefix(absDir, h.cfg.WorkspacesRoot) {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

    switch sessionType {
    case session.SessionTypeOpenCode, session.SessionTypeVSCode:
        // De-duplicate: if a session of this type is already running for this dir,
        // just re-render the sessions list (the button becomes a link).
        if existing := h.manager.FindByDirAndType(absDir, sessionType); existing != nil {
            if err := h.tmpl.ExecuteTemplate(w, "sessions.html", toSessionRows(h.manager.List())); err != nil {
                log.Printf("render sessionsStart: %v", err)
            }
            return
        }
        if _, err := h.manager.Start(sessionType, absDir); err != nil {
            http.Error(w, "start session: "+err.Error(), http.StatusInternalServerError)
            return
        }

    case session.SessionTypeScript:
        if scriptName == "" {
            http.Error(w, "script name required", http.StatusBadRequest)
            return
        }
        // Scripts: multiple scripts can run in the same directory simultaneously.
        // De-duplicate only if the exact same script is already running.
        if existing := h.manager.FindByDirAndLabel(absDir, scriptName); existing != nil {
            if err := h.tmpl.ExecuteTemplate(w, "sessions.html", toSessionRows(h.manager.List())); err != nil {
                log.Printf("render sessionsStart (dup script): %v", err)
            }
            return
        }
        if _, err := h.manager.StartScript(absDir, scriptName); err != nil {
            http.Error(w, "start script: "+err.Error(), http.StatusInternalServerError)
            return
        }

    default:
        http.Error(w, "unknown session type", http.StatusBadRequest)
        return
    }

    if err := h.tmpl.ExecuteTemplate(w, "sessions.html", toSessionRows(h.manager.List())); err != nil {
        log.Printf("render sessionsStart: %v", err)
    }
}
```

> **`StartScript` vs `Start`:** `Start(type, dir)` takes a `SessionType` and a directory. For scripts we also need the script name. Rather than adding a third parameter to the existing `Start` method (which changes the `ManagerInterface`), a dedicated `StartScript(dir, scriptName string)` method is added. Both go through the same port assignment and state persistence machinery internally.

---

## Lesson 7 — Extending the Session Manager

The manager needs two additions:

1. `StartScript(dir, scriptName string) (*Session, error)` — wraps `ScriptSessionFactory` with the given script name
2. `FindByDirAndLabel(dir, label string) *Session` — for de-duplicating script sessions

### Adding `StartScript` to `manager.go`

```go
// StartScript starts a script session for the given directory and script name.
func (m *Manager) StartScript(dir, scriptName string) (*Session, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    port, err := m.assignPort(m.cfg.Scripts.PortRange)
    if err != nil {
        return nil, fmt.Errorf("no free port in scripts range: %w", err)
    }

    factory := &session.ScriptSessionFactory{ScriptName: scriptName}
    pid, err := factory.Start(dir, port)
    if err != nil {
        return nil, fmt.Errorf("starting script %q: %w", scriptName, err)
    }

    s := &Session{
        ID:        newID(),
        Type:      SessionTypeScript,
        Dir:       dir,
        Port:      port,
        PID:       pid,
        Label:     scriptName,
        StartedAt: time.Now(),
    }
    m.sessions[s.ID] = s
    m.persist()
    m.events <- Event{Type: EventTypeStarted, Session: s}

    go m.healthCheck(s, factory)

    return s, nil
}

// FindByDirAndLabel returns the running session with the given directory and label,
// or nil if none exists.
func (m *Manager) FindByDirAndLabel(dir, label string) *Session {
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, s := range m.sessions {
        if s.Dir == dir && s.Label == label {
            return s
        }
    }
    return nil
}
```

### Adding to `ManagerInterface`

Update `session.ManagerInterface` in `internal/server/templates.go` (or wherever the interface is defined) to include the new methods:

```go
type ManagerInterface interface {
    Start(sessionType session.SessionType, dir string) (*session.Session, error)
    StartScript(dir, scriptName string) (*session.Session, error)
    Stop(id string) error
    List() []*session.Session
    FindByDirAndType(dir string, sessionType session.SessionType) *session.Session
    FindByDirAndLabel(dir, label string) *session.Session
}
```

Update `fakeManager` in `handlers_test.go` to implement the new methods (return `nil, nil` in tests that don't exercise scripts).

---

## Lesson 8 — The Session Label in the Sessions List

The `session-row.html` template renders the session type as the label. For script sessions, `Type` is `"script"` but `Label` is `"docs:dev"` — the label is more useful than the type.

Update `session-row.html` to show `Label` when set:

```html
{{define "session-row.html"}}
<li class="session-row">
  <span class="session-type">
    {{if .Label}}{{.Label}}{{else}}{{.Type}}{{end}}
  </span>
  <span class="session-dir">{{.Dir}}</span>
  <span class="session-port">:{{.Port}}</span>
  {{if .URL}}
  <a href="{{.OpenURL}}" target="_blank" class="btn btn-open">Open</a>
  {{else}}
  <span class="session-status starting">starting…</span>
  {{end}}
  <button class="btn btn-stop"
          hx-post="/sessions/stop"
          hx-vals='{"id":"{{.ID}}"}'
          hx-target="#sessions"
          hx-swap="innerHTML">
    Stop
  </button>
</li>
{{end}}
```

---

## Lesson 9 — CSS for the Dialog

Add these styles to the `<style>` block in `layout.html`:

```css
/* Script picker dialog */
.scripts-dialog {
  border: 1px solid #2d3748;
  border-radius: 8px;
  background: #1a202c;
  color: #e2e8f0;
  padding: 1.5rem;
  max-width: 480px;
  width: 90vw;
}
.scripts-dialog::backdrop {
  background: rgba(0, 0, 0, 0.6);
}
.scripts-dialog h3 {
  margin-bottom: 1rem;
  font-size: 1rem;
  font-weight: 600;
}
.scripts-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-bottom: 1rem;
}
.btn-script-run {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  width: 100%;
  padding: 0.5rem 0.75rem;
  background: #2d3748;
  border: 1px solid #4a5568;
  border-radius: 6px;
  cursor: pointer;
  color: #e2e8f0;
  text-align: left;
}
.btn-script-run:hover { background: #3d4a5c; }
.script-name { font-weight: 600; font-size: 0.9rem; }
.script-cmd  { font-size: 0.75rem; color: #718096; margin-top: 2px; font-family: monospace; }
.btn-close {
  background: #2d3748;
  border: 1px solid #4a5568;
  border-radius: 6px;
  color: #e2e8f0;
  padding: 0.4rem 1rem;
  cursor: pointer;
}
.btn-scripts {
  background: #2c5282;
  color: #bee3f8;
  border: 1px solid #2b6cb0;
  border-radius: 4px;
  padding: 0.25rem 0.5rem;
  font-size: 0.75rem;
  cursor: pointer;
}
.btn-scripts:hover { background: #2b6cb0; }
```

---

## Lesson 10 — The `config.example.yaml` Update

Add the `scripts` section to `config.example.yaml`:

```yaml
# ─── Script Runner ────────────────────────────────────────────────────────────

scripts:
  # Port range for npm/pnpm/yarn/bun script sessions.
  # Each running script gets its own port from this range.
  # Env: PORTAL_SCRIPTS_PORT_RANGE (format: "4300-4399")
  port_range: [4300, 4399]
```

---

## Lesson 11 — End-to-End Checklist

### `internal/fs`
- [ ] `HasPackageJSON` is `true` for a directory containing `package.json`
- [ ] `HasPackageJSON` is `false` when no `package.json` is present
- [ ] The fs tests pass: `go test ./internal/fs/...`

### `internal/session`
- [ ] `detectPackageManager` returns `bun` for a dir with `bun.lockb`
- [ ] `detectPackageManager` returns `npm` when no lockfile is found (fallback)
- [ ] `ReadScripts` returns the correct map for a known `package.json`
- [ ] The session tests pass: `go test ./internal/session/...`

### UI
- [ ] Expanding a directory with `package.json` shows a "Scripts" button
- [ ] Directories without `package.json` do not show a "Scripts" button
- [ ] Tapping "Scripts" opens the dialog with a list of script names and commands
- [ ] Pressing Escape closes the dialog (browser-native behaviour, no JS needed)
- [ ] Selecting a script closes the dialog and shows the session in "Running Sessions" as "starting…"
- [ ] Once the health check passes, the session shows an "Open" link

### Session manager
- [ ] Running the same script in the same directory twice re-renders the sessions list (no duplicate)
- [ ] Two different scripts in the same directory both appear as separate sessions
- [ ] Stopping a script session removes it from the sessions list

---

## Summary

The portal now supports npm/pnpm/yarn/bun scripts as first-class sessions alongside OpenCode and VS Code:

1. **`internal/fs`** — detects `package.json` and sets `HasPackageJSON` on `DirEntry`
2. **`internal/session/script.go`** — `ScriptSessionFactory` spawns `{pm} run {script} -- --port {port}`, detects package manager from lockfiles, provides `ReadScripts` for the picker
3. **`treeRowData.Scripts`** — populated in `index` and `fsList` handlers; `tree-row.html` renders the Scripts button and native `<dialog>` picker
4. **`internal/session/manager.go`** — `StartScript` and `FindByDirAndLabel` extend the manager for script sessions
5. **`session-row.html`** — shows `Label` (script name) instead of `Type` for script sessions
6. **`config.yaml`** — `scripts.port_range` configures the port range (default `[4300, 4399]`)

**Next:** [Course 05 — Deployment](./course-05-deployment.md) — package the portal as a launchd service and write the README for open-source distribution.
