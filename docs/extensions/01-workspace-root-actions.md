# Extension: Workspace Root Actions

## Description

Currently, the workspace portal renders OpenCode and VS Code launch buttons for every subdirectory of the configured workspace root, but not for the root itself. This extension adds a dedicated row at the top of the directory tree representing the workspace root, with the same OpenCode and VS Code buttons as every other row.

## Acceptance Criteria

1. A root row appears as the first item in the directory tree, above all subdirectory entries.
2. The root row displays the basename of the workspace root path as its name.
3. The root row has no expand arrow (it is not expandable).
4. The root row renders an **OpenCode** button that starts an OpenCode session with `dir` set to the workspace root path.
5. The root row renders a **VS** button that starts a VS Code session with `dir` set to the workspace root path.
6. The root row is visually distinguishable from regular subdirectory rows (e.g. via a `root` CSS class).
7. All existing subdirectory rows and their buttons are unaffected.
8. Clicking either button on the root row follows the same session-start flow as subdirectory buttons (HTMX `POST /sessions/start`, response updates `#sessions`).
9. After the implementation is merged, the running launchd service is updated with the new binary and restarted without manual intervention.

## Implementation Plan

### 1. `internal/fs/fs.go`
No changes required. The existing `DirEntry` struct is sufficient to represent the root.

### 2. `internal/server/templates.go`
- Add a `RootRow treeRowData` field to `pageData`.

### 3. `internal/server/handlers.go`
- In the `index` handler, construct a synthetic `fs.DirEntry` for the workspace root:
  - `Path` = `WorkspacesRoot`
  - `Name` = `filepath.Base(WorkspacesRoot)`
  - `IsGit` = result of checking whether the root itself is a git repo
  - `HasChildren` = `false`
- Assign it to `pageData.RootRow`.

### 4. `internal/assets/templates/layout.html`
- Before `{{template "tree-children.html" .RootEntries}}`, add:
  ```html
  <li class="tree-item root-row">{{template "tree-row-inner.html" .RootRow}}</li>
  ```
  Or more simply, render `{{template "tree-row.html" .RootRow}}` directly and rely on the `root` CSS class for styling.
- Add a `root` CSS class to the rendered root row to enable distinct styling.

### 5. CSS (inline in `layout.html`)
- Add a `.root-row .tree-name` rule to visually distinguish the root row (e.g. font-weight, color, or a separator).

### 6. Deploy and restart
- Run `make deploy` to build the binary and copy it to `/usr/local/bin/portal`.
- Restart the launchd service:
  ```bash
  launchctl unload ~/Library/LaunchAgents/com.workspace-portal.plist
  launchctl load -w ~/Library/LaunchAgents/com.workspace-portal.plist
  ```
- Verify the service is running:
  ```bash
  launchctl list | grep workspace-portal
  ```
