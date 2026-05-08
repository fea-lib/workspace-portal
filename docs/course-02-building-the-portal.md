---
title: "Course 02 — Building the Portal in Go"
---

# Course 02 — Building the Portal in Go

**Goal:** Implement every module of workspace-portal from scratch, producing a working Go binary.  
**Prerequisite:** [Course 01 — Go Foundations](./course-01-go-foundations.md)  
**Output:** A compiled `workspace-portal` binary that serves an HTTP server, lists directories, and manages OpenCode/VS Code processes. The UI is wired up in Course 03.

---

## Lesson 1 — Scaffolding the Repository

Before writing a line of application code, you need a working project skeleton: a directory, a Git repo, a module, the package layout, and the tooling files (`Makefile`, `.gitignore`). Getting this right up front means every subsequent lesson starts from a clean, compilable state.

### Create the directory and initialise the module

```bash
mkdir -p ~/workspaces/fea/lib/workspace-portal
cd ~/workspaces/fea/lib/workspace-portal
git init
go mod init workspace-portal
```

`go mod init` creates `go.mod`, which declares the module name. Every import path inside this project will be rooted here — for example, `workspace-portal/internal/config`. Because this is a personal tool you won't be publishing, a plain name without a domain works fine.

### Create the directory structure

```bash
mkdir -p cmd/portal
mkdir -p internal/config
mkdir -p internal/fs
mkdir -p internal/session
mkdir -p internal/server
mkdir -p internal/assets/static
mkdir -p internal/assets/templates
mkdir -p deploy/launchd
```

The directories map directly to the modules you'll build:

| Directory | Purpose |
|---|---|
| `cmd/portal/` | The binary entry point (`main.go`). One sub-directory per binary is the convention. |
| `internal/config/` | YAML loading, env var overrides, secrets resolution. |
| `internal/fs/` | Directory tree listing with pruning and git detection. |
| `internal/session/` | Process lifecycle — start, stop, health check, state persistence. |
| `internal/assets/` | Embedded static assets and HTML templates. |
| `internal/server/` | HTTP mux, route handlers, SSE streaming. |
| `deploy/` | Deployment configs — launchd plist. |

> **No `src/` directory:** Go does not treat `src/` as special. The idiomatic layout places packages directly under named directories (`cmd/`, `internal/`, etc.) at the module root. A top-level `src/` is a Java/Maven habit — avoid it in Go.

### `go.mod` — add external dependencies

The portal has three external dependencies: `gopkg.in/yaml.v3` for parsing the config file, `github.com/caarlos0/env/v11` for struct-tag-driven environment variable overrides, and `github.com/sabhiram/go-gitignore` for parsing `.gitignore` files when listing directories. The first two have zero transitive dependencies. Everything else — HTTP, file I/O, process management, JSON, concurrency — is covered by the Go standard library.

```bash
go get gopkg.in/yaml.v3
go get github.com/caarlos0/env/v11
go get github.com/sabhiram/go-gitignore
```

Your `go.mod` will now look like:

```
module workspace-portal

go 1.22

require (
    github.com/caarlos0/env/v11 v11.4.0
    github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06
    gopkg.in/yaml.v3 v3.0.1
)
```

### `cmd/portal/main.go` — the entry point

`main.go` is intentionally thin. Its only job is to wire together three things:

1. **Parse CLI flags** — so the config path can be overridden at the command line.
2. **Load config** — delegate to `internal/config`, fail fast and loudly if it's wrong.
3. **Start the server** — delegate to `internal/server`, which owns all HTTP concerns.

This pattern — a thin `main` that delegates immediately — keeps the entry point testable (you can't test `main` directly in Go, but you can test `config.Load` and `server.Start`).

```go
package main

import (
    "flag"
    "fmt"
    "log"
    "os"

    "workspace-portal/internal/config"
    "workspace-portal/internal/server"
)

func main() {
    // CLI flags
    configPath := flag.String("config", "", "path to config.yaml")
    flag.Parse()

    // Resolve config path: flag > env var > default
    if *configPath == "" {
        if v := os.Getenv("PORTAL_CONFIG"); v != "" {
            *configPath = v
        } else {
            *configPath = "config.yaml"
        }
    }

    // Load config
    cfg, err := config.Load(*configPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "config error: %v\n", err)
        os.Exit(1)
    }

    // Start server
    log.Printf("workspace-portal starting on :%d", cfg.PortalPort)
    if err := server.Start(cfg); err != nil {
        log.Fatal(err)
    }
}
```

`flag.String` returns a `*string`, so you dereference it with `*configPath`. The resolution order (flag → env var → default) is a common pattern for CLI tools: it lets you override in scripts without editing config files.

### `Makefile`

The Makefile wraps the commands you'll run repeatedly so you don't have to remember the flags:

```makefile
.PHONY: build run test install

build:
	go build -ldflags="-s -w" -o bin/workspace-portal ./cmd/portal

run:
	go run ./cmd/portal --config config.yaml

test:
	go test -v ./...

install: build
	cp bin/workspace-portal /usr/local/bin/workspace-portal
```

`-ldflags="-s -w"` strips the symbol table and DWARF debug info from the binary. It has no effect on behaviour but reduces binary size by ~25–30%.

### `.gitignore`

```
bin/
config.yaml
.secrets/
*.log
```

`config.yaml` is gitignored because it will contain paths specific to your machine. If you commit it, someone else's clone will silently use the wrong paths.

### Verify the scaffold compiles

The project won't run yet — `internal/config` and `internal/server` don't exist — but it should parse without errors:

```bash
go build ./...
# should produce no output (no errors)
```

If you get "no Go files" warnings, that's expected — empty directories are ignored by the toolchain.

> **No tests for `cmd/portal/main.go`:** Go does not let you test `main()` directly. Keep `main.go` thin enough that there is nothing to test — if you can write `go run ./cmd/portal` and see the expected output, the entry point is correct.

### Installing runtime dependencies

The portal shells out to two external binaries: `opencode` and `code-server`. Both must be on `$PATH` before you start any sessions.

**OpenCode**

```bash
# macOS — Homebrew
brew install opencode

# Or install via npm (requires Node 18+)
npm install -g opencode

# Verify
opencode --version
```

**code-server (VS Code in the browser)**

`code-server` is Coder's open-source package that runs VS Code Server locally and serves it over HTTP.

```bash
# macOS — Homebrew
brew install code-server

# Or the official install script (Linux / macOS)
curl -fsSL https://code-server.dev/install.sh | sh

# Or npm
npm install -g code-server

# Verify
code-server --version
```

After installation, confirm both are discoverable:

```bash
which opencode    # e.g. /usr/local/bin/opencode
which code-server # e.g. /usr/local/bin/code-server
```

If `which` returns nothing, the binary is not on `$PATH`. Check that your shell profile (`~/.zshrc`, `~/.bash_profile`, etc.) sources the correct `PATH` entries and that you have started a new terminal session after installation.

> **The portal does not fail at startup if the binaries are missing.** It only fails when you click a button to start a session. The error is visible in the sessions area of the UI (see Course 03 — Lesson 12 for the error UI). This is intentional: you can run and develop the portal on a machine without code-server installed, and only install it when you need it.

---

## Lesson 2 — `internal/portrange`: Shared Type

### What this module does

`portrange.PortRange` is a tiny shared primitive — a `[2]int` with a name and a string unmarshaller. It lives in its own package so both `internal/config` and `internal/session` can import it without either depending on the other.

### Key design decisions

**Why a dedicated `internal/portrange` package instead of defining the type in `config`?**  
If `PortRange` lived in `internal/config`, the `session` package would have to import `config` to get it. That creates an undesirable dependency: the session manager would become coupled to the full configuration layer. A tiny single-purpose package breaks the coupling — both packages import `portrange`, and neither imports the other.

**Why not `internal/types`?**  
Go favours small, focused packages over catch-all `types` or `util` packages. Naming the package after the one type it exports (`portrange`) makes the import path self-documenting: `portrange.PortRange` reads clearly at the call site. If more shared primitives accumulate later, they earn their own package names too.

### `internal/portrange/portrange.go`

```go
package portrange

import (
    "fmt"
    "strconv"
    "strings"
)

// PortRange is a [lo, hi] port pair that unmarshals from "lo-hi" strings in both
// YAML and environment variables (e.g. "4100-4199"). Implementing
// encoding.TextUnmarshaler means both yaml.v3 and caarlos0/env pick it up
// automatically — no custom parsing code needed at the call site.
type PortRange [2]int

func (p *PortRange) UnmarshalText(text []byte) error {
    parts := strings.SplitN(string(text), "-", 2)
    if len(parts) != 2 {
        return fmt.Errorf("port range must be in lo-hi format, got %q", string(text))
    }
    lo, err1 := strconv.Atoi(parts[0])
    hi, err2 := strconv.Atoi(parts[1])
    if err1 != nil || err2 != nil {
        return fmt.Errorf("invalid port range %q", string(text))
    }
    *p = PortRange{lo, hi}
    return nil
}
```

### `internal/portrange/portrange_test.go`

A table-driven test for `UnmarshalText`. It covers the happy path (valid range), a single-number input (missing separator), non-numeric parts, an empty string, and an input with too many separators.

> **No test for the zero-value PortRange** — there is nothing else to test. `PortRange` is a `[2]int`; once `UnmarshalText` is verified, the rest of the type is just array indexing.

```go
package portrange

import "testing"

func TestUnmarshalText(t *testing.T) {
    tests := []struct {
        input   string
        want    PortRange
        wantErr bool
    }{
        {"4100-4199", PortRange{4100, 4199}, false},
        {"80-80", PortRange{80, 80}, false},
        {"0-65535", PortRange{0, 65535}, false},
        {"4100", PortRange{}, true},            // missing separator
        {"abc-4199", PortRange{}, true},        // non-numeric lo
        {"4100-xyz", PortRange{}, true},        // non-numeric hi
        {"", PortRange{}, true},                // empty string
        {"4100-4199-extra", PortRange{}, true}, // too many parts
    }

    for _, tc := range tests {
        var p PortRange
        err := p.UnmarshalText([]byte(tc.input))
        if tc.wantErr {
            if err == nil {
                t.Errorf("input %q: expected error, got nil", tc.input)
            }
            continue
        }
        if err != nil {
            t.Errorf("input %q: unexpected error: %v", tc.input, err)
            continue
        }
        if p != tc.want {
            t.Errorf("input %q: got %v, want %v", tc.input, p, tc.want)
        }
    }
}
```

Run:

```bash
go test ./internal/portrange/...
```

---

## Lesson 3 — `internal/config`: Loading Configuration

### What this module does

Every other module in the portal needs configuration — the port to listen on, the path to the workspaces directory, which binary to launch for OpenCode, and so on. Rather than scatter `os.Getenv` calls throughout the codebase, all configuration is centralised here. This module is the first one you implement because nothing else can be written without it.

The design follows a three-layer resolution order:
1. **YAML file** — the primary source, written once and kept locally.
2. **Environment variables** — useful for overriding in scripts or containers without editing the file.
3. **Defaults** — sensible values so the binary is runnable with a minimal config.

### Key design decisions

**Why a `defaults()` function?**  
Go's zero values (`0`, `""`, `false`) are not always sensible defaults. A port of `0` would pick a random port; an empty binary path would fail immediately. Seeding `cfg` with defaults before unmarshalling means YAML only needs to specify what differs from the defaults — you don't have to repeat the OpenCode binary path in every config file.

**Why accept a missing file?**  
`os.IsNotExist(err)` lets the portal start with defaults + env vars even if no `config.yaml` exists. This is useful in Docker or CI environments where you inject everything via environment variables.

**Why `env` struct tags and `caarlos0/env` instead of a manual `applyEnvOverrides`?**  
The manual approach — a function that reads `os.Getenv` for each field — has a silent failure mode: if you add a new field to the config struct, you must remember to add a matching `os.Getenv` call. Forget it, and that field can never be set by env var without any error or warning. This is the worst kind of bug — it fails silently and only at runtime, in production, for someone who tried to configure their deployment.

The `env` tag approach solves this by colocating the env var name with the field declaration. When you add a field, you add its `env` tag at the same time, in the same place. There is no separate function to keep in sync. `env.ParseWithOptions` then drives all fields automatically — the caller adds no code.

`github.com/caarlos0/env/v11` was chosen specifically because it has **zero transitive dependencies** — adding it costs one line in `go.mod` with no dependency tree attached. The `env:"..."` tag convention also directly mirrors the `yaml:"..."` and `json:"..."` tags students already know.

`PortRange [2]int` is the one type that `env` cannot parse out of the box, because `[2]int` has no standard string representation. Rather than adding a helper function that calls `os.Getenv` explicitly (reintroducing exactly the maintenance problem we set out to solve), the solution is to define a named type `portrange.PortRange` and implement `encoding.TextUnmarshaler` on it. Both `yaml.v3` and `caarlos0/env` discover and call `UnmarshalText` automatically — the port range fields get normal `env` tags, no special-casing exists anywhere in `Load`, and the YAML schema and env var format are unchanged. `PortRange` lives in `internal/portrange` (not `internal/config`) so that the `session` package can share it without importing `config`.

**Why a `Secret` method?**  
Secrets (like `vscode-password`) should never live in the main config file, which is likely shared or version-controlled. The `Secret` method resolves them from env vars first, then from files in a `.secrets/` directory, then from Docker secrets (`/run/secrets/`). The caller doesn't care where the value came from. If no source provides a value, `Secret` returns an empty string and logs a warning — the empty string is intentional (it avoids a hard failure for optional secrets), but the warning makes misconfiguration visible immediately in the process log rather than producing a silent auth bypass.

### `internal/config/config.go`

```go
package config

import (
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"

    "github.com/caarlos0/env/v11"
    "gopkg.in/yaml.v3"

    "workspace-portal/internal/portrange"
)

// Config holds all portal configuration.
type Config struct {
    WorkspacesRoot string    `yaml:"workspaces_root" env:"PORTAL_WORKSPACES_ROOT"`
    PortalPort     int       `yaml:"portal_port"      env:"PORTAL_PORT"`
    SecretsDir     string    `yaml:"secrets_dir"`
    OC             OCConfig  `yaml:"oc"               envPrefix:"PORTAL_OC_"`
    VSCode         VSCConfig `yaml:"vscode"           envPrefix:"PORTAL_VSCODE_"`
}

type OCConfig struct {
    Binary    string              `yaml:"binary"     env:"BINARY"`
    PortRange portrange.PortRange `yaml:"port_range" env:"PORT_RANGE"`
    Flags     []string            `yaml:"flags"      env:"FLAGS"`
}

type VSCConfig struct {
    Binary    string              `yaml:"binary"     env:"BINARY"`
    PortRange portrange.PortRange `yaml:"port_range" env:"PORT_RANGE"`
}

// defaults returns a Config populated with sensible defaults.
func defaults() *Config {
    return &Config{
        PortalPort: 4000,
        SecretsDir: ".secrets",
        OC: OCConfig{
            Binary:    "opencode",
            PortRange: portrange.PortRange{4100, 4199},
            Flags:     []string{"--mdns"},
        },
        VSCode: VSCConfig{
            Binary:    "code-server",
            PortRange: portrange.PortRange{4200, 4299},
        },
    }
}

// Load reads config from the given YAML file path, applies env var overrides,
// and validates required fields.
func Load(path string) (*Config, error) {
    cfg := defaults()

    // Read and parse YAML if file exists
    data, err := os.ReadFile(path)
    if err != nil && !os.IsNotExist(err) {
        return nil, fmt.Errorf("reading config file %s: %w", path, err)
    }
    if err == nil {
        if err := yaml.Unmarshal(data, cfg); err != nil {
            return nil, fmt.Errorf("parsing config file %s: %w", path, err)
        }
    }

    // Apply env var overrides for all tagged fields automatically.
    // env.Parse only writes a field when its env var is actually present —
    // it never touches fields whose env var is unset, so YAML-loaded and
    // defaults()-populated values are preserved unless explicitly overridden.
    // portrange.PortRange fields are covered automatically because PortRange
    // implements encoding.TextUnmarshaler — env parses "4100-4199" into
    // portrange.PortRange{4100,4199}.
    if err := env.Parse(cfg); err != nil {
        return nil, fmt.Errorf("applying env overrides: %w", err)
    }

    // Expand ~ in workspaces root
    if strings.HasPrefix(cfg.WorkspacesRoot, "~/") {
        home, _ := os.UserHomeDir()
        cfg.WorkspacesRoot = filepath.Join(home, cfg.WorkspacesRoot[2:])
    }

    // Resolve secrets dir relative to config file location
    if !filepath.IsAbs(cfg.SecretsDir) {
        cfg.SecretsDir = filepath.Join(filepath.Dir(path), cfg.SecretsDir)
    }

    // Validate required fields
    if cfg.WorkspacesRoot == "" {
        return nil, fmt.Errorf("workspaces_root is required (set in config.yaml or PORTAL_WORKSPACES_ROOT)")
    }

    return cfg, nil
}

// Secret reads a secret value by name. Resolution order:
//  1. Environment variable PORTAL_{UPPER_NAME} (e.g. PORTAL_VSCODE_PASSWORD)
//  2. File at cfg.SecretsDir/{name}
//  3. File at /run/secrets/{name}  (Docker secrets convention)
func (cfg *Config) Secret(name string) string {
    // 1. Env var
    envKey := "PORTAL_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
    if v := os.Getenv(envKey); v != "" {
        return v
    }
    // 2. secrets dir
    if v, err := os.ReadFile(filepath.Join(cfg.SecretsDir, name)); err == nil {
        return strings.TrimSpace(string(v))
    }
    // 3. Docker secrets
    if v, err := os.ReadFile(filepath.Join("/run/secrets", name)); err == nil {
        return strings.TrimSpace(string(v))
    }
    log.Printf("warning: secret %q not found (checked env var %s, %s, /run/secrets/%s)", name, envKey, filepath.Join(cfg.SecretsDir, name), name)
    return ""
}
```

### `internal/config/config_test.go`

The tests cover the four meaningful behaviours: missing file, file with values, env var override, and secret resolution. Each test is independent — it creates its own temp files and cleans up with `defer`.

Notice the test for defaults: it expects an *error* because `workspaces_root` is required and no file is present. This is the right thing to test — "missing required field fails loudly" is a behaviour worth protecting.

The `TestEnvOverride` test sets `PORTAL_PORT` via `t.Setenv` and confirms that `Load` picks it up. This exercises `env.ParseWithOptions` indirectly — if a new field is added with a correct `env` tag, no test change is needed to cover the mechanism; you only need to add a test for the specific field's value.

```go
package config

import (
    "os"
    "testing"
)

func TestDefaults(t *testing.T) {
    // Load with a non-existent file — should fail on missing workspaces_root
    _, err := Load("nonexistent.yaml")
    if err == nil {
        t.Fatal("expected error for missing workspaces_root")
    }
}

func TestLoadFromFile(t *testing.T) {
    f, _ := os.CreateTemp("", "config*.yaml")
    f.WriteString("workspaces_root: /tmp/workspaces\nportal_port: 9000\n")
    f.Close()
    defer os.Remove(f.Name())

    cfg, err := Load(f.Name())
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.WorkspacesRoot != "/tmp/workspaces" {
        t.Errorf("got %q, want /tmp/workspaces", cfg.WorkspacesRoot)
    }
    if cfg.PortalPort != 9000 {
        t.Errorf("got %d, want 9000", cfg.PortalPort)
    }
    // Defaults still apply for unset fields
    if cfg.OC.Binary != "opencode" {
        t.Errorf("expected default OpenCode binary, got %q", cfg.OC.Binary)
    }
}

func TestEnvOverride(t *testing.T) {
    f, _ := os.CreateTemp("", "config*.yaml")
    f.WriteString("workspaces_root: /tmp/workspaces\n")
    f.Close()
    defer os.Remove(f.Name())

    t.Setenv("PORTAL_PORT", "5555")

    cfg, _ := Load(f.Name())
    if cfg.PortalPort != 5555 {
        t.Errorf("env override failed, got %d", cfg.PortalPort)
    }
}

func TestSecret(t *testing.T) {
    dir, _ := os.MkdirTemp("", "secrets*")
    defer os.RemoveAll(dir)
    os.WriteFile(dir+"/vscode-password", []byte("secret123\n"), 0600)

    cfg := &Config{SecretsDir: dir}
    if got := cfg.Secret("vscode-password"); got != "secret123" {
        t.Errorf("got %q, want secret123", got)
    }
}
```

Run:

```bash
go test ./internal/config/...
```

All four tests should pass before continuing.

---

## Lesson 4 — `internal/fs`: Directory Tree

### What this module does

The portal needs to display a navigable directory tree rooted at `workspaces_root`. Critically, it should *not* descend into build artefact directories like `node_modules`, `.next`, or `target` — these are noisy, large, and irrelevant to navigation. It should also respect `.gitignore` files, so any directory a project already marks as ignored is also hidden here — without the user having to configure anything separately.

This module provides a single function, `List`, that returns the immediate children of a given path with those directories filtered out. It also annotates each entry with two flags that the UI needs: `IsGit` (to show a git badge) and `HasChildren` (to show an expand arrow).

### Key design decisions

**Why only immediate children, not a full recursive tree?**  
The UI will load children lazily on expand — so we only need one level at a time. Recursing the whole workspace tree upfront would be slow and unnecessary.

**Why a hardcoded `defaultPrune` map?**  
Most pruned directories (`node_modules`, `dist`, etc.) are universally unwanted. Hardcoding them means users don't have to rediscover the list. There is no user-configurable extra prune list — `.gitignore` files already express user-level exclusion preferences, so duplicating that mechanism in the portal config would create two places to maintain the same information.

**Why respect `.gitignore` files?**  
Git already tracks which directories a project considers ignorable. By loading `.gitignore` files from ancestor directories (walking up from the listed path to `workspaces_root`), the portal automatically hides anything the user has already decided isn't worth tracking — generated directories, local secrets, toolchain caches. This is zero-configuration from the user's perspective: write a `.gitignore` once, get consistent filtering everywhere.

`github.com/sabhiram/go-gitignore` implements the full gitignore matching algorithm (negations, `**` globs, anchored patterns) with a single dependency and no transitive dependencies.

**Why detect git repos at this layer?**  
The directory listing and git detection happen in the same `os.ReadDir` pass. Doing it here avoids a second pass later and keeps the data clean for the handler.

### `internal/fs/tree.go`

```go
package fs

import (
    "os"
    "path/filepath"
    "sort"

    gitignore "github.com/sabhiram/go-gitignore"
)

// DirEntry describes one directory in the tree.
type DirEntry struct {
    Path        string // relative path from workspaces root to this entry
    Name        string // basename (final path element only)
    IsGit       bool   // contains a git repo
    HasChildren bool   // has non-pruned subdirectories
}

// defaultPrune is the set of directory names that are never shown.
var defaultPrune = map[string]bool{
    "node_modules": true, ".pnpm": true, ".pnpm-store": true,
    "dist": true, ".next": true, ".nuxt": true, ".turbo": true,
    "build": true, "out": true, "coverage": true,
    ".cache": true, "__pycache__": true, ".pytest_cache": true,
    "target": true, "vendor": true, ".gradle": true,
    // git internals — prune CONTENTS of .git and .bare, not the dirs themselves
    "objects": true, "refs": true, "info": true,
    "hooks": true, "worktrees": true, "pack": true,
    "logs": true,
}

// List returns the immediate visible subdirectories of path.
// It respects .gitignore files found in path and its ancestors up to root.
// root is the workspaces root — gitignore walk stops there.
func List(path, root string) ([]DirEntry, error) {
    matchers := loadGitignores(path, root)

    entries, err := os.ReadDir(path)
    if err != nil {
        return nil, err
    }

    var result []DirEntry
    for _, e := range entries {
        if !e.IsDir() {
            continue
        }
        if defaultPrune[e.Name()] {
            continue
        }
        absPath := filepath.Join(path, e.Name())
        if gitignored(absPath, root, matchers) {
            continue
        }
        relPath, err := filepath.Rel(root, absPath)
        if err != nil {
            relPath = absPath
        }
        entry := DirEntry{
            Path:  relPath,
            Name:  e.Name(),
            IsGit: isGitRepo(absPath),
        }
        entry.HasChildren = hasVisibleSubdirs(absPath, root, matchers)
        result = append(result, entry)
    }

    sort.Slice(result, func(i, j int) bool {
        return result[i].Name < result[j].Name
    })
    return result, nil
}

// gitignored returns true if absPath is matched by any of the compiled matchers.
func gitignored(absPath, root string, matchers []*gitignore.GitIgnore) bool {
    rel, err := filepath.Rel(root, absPath)
    if err != nil {
        return false
    }
    for _, m := range matchers {
        if m.MatchesPath(rel) {
            return true
        }
    }
    return false
}

// loadGitignores walks from root up to path, collecting all .gitignore files,
// and returns a slice of compiled matchers. Returns nil if none are found.
func loadGitignores(path, root string) []*gitignore.GitIgnore {
    // Collect .gitignore file paths from root down to path.
    var files []string
    current := path
    for {
        candidate := filepath.Join(current, ".gitignore")
        if _, err := os.Stat(candidate); err == nil {
            files = append([]string{candidate}, files...) // prepend so root wins
        }
        if current == root {
            break
        }
        parent := filepath.Dir(current)
        if parent == current {
            break
        }
        current = parent
    }

    var matchers []*gitignore.GitIgnore
    for _, f := range files {
        ig, err := gitignore.CompileIgnoreFile(f)
        if err == nil {
            matchers = append(matchers, ig)
        }
    }
    return matchers
}

// isGitRepo returns true if the directory contains a git repository.
// Handles three layouts:
//   - standard:  .git/ is a directory
//   - worktree:  .git is a regular file
//   - bare repo: .bare/HEAD exists
func isGitRepo(path string) bool {
    if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
        return true
    }
    if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.Mode().IsRegular() {
        return true
    }
    if _, err := os.Stat(filepath.Join(path, ".bare", "HEAD")); err == nil {
        return true
    }
    return false
}

// hasVisibleSubdirs returns true if path contains at least one subdirectory
// that is neither in the default prune set nor excluded by .gitignore rules.
// Files are not considered.
func hasVisibleSubdirs(path, root string, matchers []*gitignore.GitIgnore) bool {
    entries, err := os.ReadDir(path)
    if err != nil {
        return false
    }
    for _, e := range entries {
        if !e.IsDir() || defaultPrune[e.Name()] {
            continue
        }
        if gitignored(filepath.Join(path, e.Name()), root, matchers) {
            continue
        }
        return true
    }
    return false
}
```

### `internal/fs/tree_test.go`

The tests build a synthetic directory tree in a temp directory. This is the standard Go testing pattern for file system code — never test against real directories you don't control.

The file uses `package fs` (white-box testing), which gives access to unexported functions like `isGitRepo`. This is intentional: `TestIsGitRepo` tests the three git layouts directly against the internal function, so a regression in that detection logic is caught at the lowest level. `TestList` then tests the same layouts through the public `List` function, ensuring the wiring is correct too.

`TestList` covers every distinct behaviour of `List`:

- `node_modules` is absent (hardcoded prune)
- `ignored-dir` is absent (root `.gitignore`)
- `.secrets` appears (dotdirs are never filtered)
- `Path` is the relative path from the workspaces root to the entry
- `IsGit` is true for all three git layouts (standard, worktree, bare) via `List`
- `IsGit` is false for a plain directory
- `HasChildren` is true when a visible subdirectory exists (pruned siblings don't count)
- `HasChildren` is false for a leaf with no subdirectories
- A subdirectory's own `.gitignore` excludes entries when that subdirectory is listed

```go
package fs

import (
    "os"
    "path/filepath"
    "testing"
)

func TestList(t *testing.T) {
    // Build a temp tree:
    // root/
    //   project-a/         (standard git repo — .git/ dir)
    //   project-b/         (no git, has one visible child)
    //     src/
    //   project-b/node_modules/  (pruned — not counted as visible child)
    //   project-c/         (worktree — .git is a regular file)
    //   project-d/         (bare repo — .bare/HEAD exists)
    //   project-e/         (leaf — no subdirs at all)
    //   node_modules/      (pruned by defaultPrune)
    //   .secrets/          (dotdir — must appear)
    //   ignored-dir/       (excluded by root .gitignore)
    //   subdir/
    //     nested-ignored/  (excluded by subdir .gitignore)
    //     visible/
    root, _ := os.MkdirTemp("", "portal-fs*")
    defer os.RemoveAll(root)

    // project-a: standard git repo
    os.MkdirAll(filepath.Join(root, "project-a", ".git"), 0755)

    // project-b: not a git repo, has one visible child (src) and one pruned child
    os.MkdirAll(filepath.Join(root, "project-b", "src"), 0755)
    os.MkdirAll(filepath.Join(root, "project-b", "node_modules"), 0755)

    // project-c: worktree (.git is a regular file)
    os.MkdirAll(filepath.Join(root, "project-c"), 0755)
    os.WriteFile(filepath.Join(root, "project-c", ".git"), []byte("gitdir: ../.bare/worktrees/main"), 0644)

    // project-d: bare repo (.bare/HEAD exists)
    os.MkdirAll(filepath.Join(root, "project-d", ".bare"), 0755)
    os.WriteFile(filepath.Join(root, "project-d", ".bare", "HEAD"), []byte("ref: refs/heads/main"), 0644)

    // project-e: leaf — no subdirs
    os.MkdirAll(filepath.Join(root, "project-e"), 0755)
    os.WriteFile(filepath.Join(root, "project-e", "README.md"), []byte("hello"), 0644)

    // pruned by defaultPrune
    os.MkdirAll(filepath.Join(root, "node_modules"), 0755)

    // dotdir — must appear
    os.MkdirAll(filepath.Join(root, ".secrets"), 0755)

    // excluded by root .gitignore
    os.MkdirAll(filepath.Join(root, "ignored-dir"), 0755)
    os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored-dir\n"), 0644)

    // subdir with its own .gitignore
    os.MkdirAll(filepath.Join(root, "subdir", "nested-ignored"), 0755)
    os.MkdirAll(filepath.Join(root, "subdir", "visible"), 0755)
    os.WriteFile(filepath.Join(root, "subdir", ".gitignore"), []byte("nested-ignored\n"), 0644)

    entries, err := List(root, root)
    if err != nil {
        t.Fatal(err)
    }

    byName := make(map[string]DirEntry)
    for _, e := range entries {
        byName[e.Name] = e
    }

    // defaultPrune entries must be absent
    if _, ok := byName["node_modules"]; ok {
        t.Error("node_modules should be pruned")
    }

    // .gitignore-excluded entries must be absent
    if _, ok := byName["ignored-dir"]; ok {
        t.Error("ignored-dir should be excluded by .gitignore")
    }

    // dotdir must appear
    if _, ok := byName[".secrets"]; !ok {
        t.Error(".secrets should appear")
    }

    // Path must be relative to root
    if e, ok := byName["project-a"]; ok {
        want := "project-a"
        if e.Path != want {
            t.Errorf("project-a Path: got %q, want %q", e.Path, want)
        }
    }

    // IsGit: standard repo
    if !byName["project-a"].IsGit {
        t.Error("project-a should be detected as git repo (standard .git dir)")
    }

    // IsGit: worktree
    if !byName["project-c"].IsGit {
        t.Error("project-c should be detected as git repo (worktree .git file)")
    }

    // IsGit: bare repo
    if !byName["project-d"].IsGit {
        t.Error("project-d should be detected as git repo (bare .bare/HEAD)")
    }

    // not a git repo
    if byName["project-b"].IsGit {
        t.Error("project-b should not be a git repo")
    }

    // HasChildren: true when a visible subdir exists (pruned siblings don't count)
    if !byName["project-b"].HasChildren {
        t.Error("project-b should have children (src/ is visible)")
    }

    // HasChildren: false for a leaf with no subdirs
    if byName["project-e"].HasChildren {
        t.Error("project-e should not have children")
    }

    // ancestor .gitignore: listing subdir should hide nested-ignored
    subdirEntries, err := List(filepath.Join(root, "subdir"), root)
    if err != nil {
        t.Fatal(err)
    }
    subdirByName := make(map[string]DirEntry)
    for _, e := range subdirEntries {
        subdirByName[e.Name] = e
    }
    if _, ok := subdirByName["nested-ignored"]; ok {
        t.Error("nested-ignored should be excluded by subdir/.gitignore")
    }
    if _, ok := subdirByName["visible"]; !ok {
        t.Error("visible should appear in subdir listing")
    }
}

func TestIsGitRepo(t *testing.T) {
    root, _ := os.MkdirTemp("", "gitrepo*")
    defer os.RemoveAll(root)

    t.Run("standard repo", func(t *testing.T) {
        dir := filepath.Join(root, "standard")
        os.MkdirAll(filepath.Join(dir, ".git"), 0755)
        if !isGitRepo(dir) {
            t.Error("expected true for standard .git dir")
        }
    })
    t.Run("worktree", func(t *testing.T) {
        dir := filepath.Join(root, "worktree")
        os.MkdirAll(dir, 0755)
        os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../.bare/worktrees/main"), 0644)
        if !isGitRepo(dir) {
            t.Error("expected true for .git file (worktree)")
        }
    })
    t.Run("bare repo", func(t *testing.T) {
        dir := filepath.Join(root, "bare")
        os.MkdirAll(filepath.Join(dir, ".bare"), 0755)
        os.WriteFile(filepath.Join(dir, ".bare", "HEAD"), []byte("ref: refs/heads/main"), 0644)
        if !isGitRepo(dir) {
            t.Error("expected true for .bare/HEAD (bare repo)")
        }
    })
    t.Run("not a repo", func(t *testing.T) {
        dir := filepath.Join(root, "plain")
        os.MkdirAll(dir, 0755)
        if isGitRepo(dir) {
            t.Error("expected false for plain dir")
        }
    })
}
```

---

## Lesson 5 — `internal/session`: The SessionFactory Interface

### What this module does

This module handles the full lifecycle of a running editor session: launching the process, assigning it a port, polling until it's healthy, persisting state to disk so sessions survive a portal restart, and broadcasting events to connected browsers via SSE.

There are two session types — OpenCode and VS Code — which are launched differently (different binaries, different flags, different auth). The module is split into four files to separate these concerns:

| File | Responsibility |
|---|---|
| `session.go` | The `SessionFactory` interface and `Session` struct — the shared contract. |
| `opencode.go` | The OpenCode-specific `SessionFactory` implementation. |
| `vscode.go` | The VS Code-specific `SessionFactory` implementation. |
| `manager.go` | Orchestration — port assignment, lifecycle, state persistence, SSE. |

### Key design decisions

**Why define a `SessionFactory` interface before writing the implementations?**  
The `Manager` in `manager.go` needs to start and stop sessions without knowing which type they are. By programming against the `SessionFactory` interface, the manager is completely decoupled from the OpenCode and VS Code specifics. You can add a third session type later without touching the manager, and you can inject a fake `SessionFactory` in tests.

This is the same principle as TypeScript interfaces — the difference is that Go satisfies interfaces implicitly (no `implements` keyword).

**Why keep `Session` in `session.go` rather than `manager.go`?**  
`Session` is the data shape shared by the session factories, the manager, and the HTTP handlers. Defining it in `session.go` (the foundational file) avoids circular imports and makes it clear that it belongs to the `session` package as a whole, not just to the manager.

**Why is `Type` a named `SessionType` and not a bare `string`?**  
Go has no enum keyword. The idiomatic substitute is a **named string type** plus package-level constants:

```go
type SessionType string

const (
    SessionTypeOpenCode SessionType = "opencode"
    SessionTypeVSCode   SessionType = "vscode"
)
```

Using a named type gives you meaningful type safety: the compiler will reject a bare string literal wherever a `SessionType` is expected, so you can't accidentally pass `"opencdoe"` without it being caught. The constants also give IDEs something to autocomplete. Because `SessionType` has the underlying type `string`, it serialises to/from JSON and YAML as a plain string — no extra marshalling code needed.

Go does **not** provide exhaustiveness checking on `switch` statements over a named type (the compiler won't warn you if you add a new constant and miss a case). If you want that guarantee, the `exhaustive` linter flag can enforce it.

### `internal/session/session.go`

The `SessionFactory` interface defines the three things the manager needs from any session type: start it, stop it, and get the URL to health-check it.

```go
package session

import "time"

// SessionType identifies which editor a session runs.
// Using a named string type (rather than bare string) makes the compiler
// reject accidental string literals wherever a SessionType is expected.
type SessionType string

const (
    SessionTypeOpenCode SessionType = "opencode"
    SessionTypeVSCode   SessionType = "vscode"
)

// Session holds the state of a running session.
type Session struct {
    ID        string      `json:"id"`
    Type      SessionType `json:"type"`
    Dir       string      `json:"dir"`
    Port      int         `json:"port"`
    PID       int         `json:"pid"`
    StartedAt time.Time   `json:"started_at"`
    URL       string      `json:"url"` // set after health check passes
}

// SessionFactory is implemented by each session type (OpenCode, VS Code).
type SessionFactory interface {
    // Start launches the process. Returns when the process has started
    // (not necessarily healthy yet).
    Start(dir string, port int) (pid int, err error)
    // Stop terminates the process.
    Stop(pid int) error
    // HealthURL returns the URL to poll for the health check.
    HealthURL(port int) string
}
```

### `internal/session/opencode.go`

`OCSessionFactory` launches the `opencode` binary in **headless server mode** using the `serve` subcommand. Without `serve`, `opencode` starts its interactive TUI — which never opens an HTTP port, so the health check never succeeds and the session stays stuck at "starting…" forever.

`opencode serve` does **not** accept a positional directory argument — passing one causes it to print help and exit. The project directory is set via `cmd.Dir` (the process working directory) instead.

`cmd.Start()` (not `cmd.Run()`) is used because we want the process to keep running after `Start` returns — `Run` would block until the process exits.

> **Why `serve` and not just `opencode <dir> --port`?**
> `opencode [project]` is the TUI entrypoint. `opencode serve --port` starts a headless HTTP server that the portal can health-check and that the browser connects to directly.

```go
package session

import (
    "fmt"
    "os"
    "os/exec"
    "strconv"
)

// OCSessionFactory is a configured factory for OpenCode sessions.
type OCSessionFactory struct {
    Binary     string
    Flags      []string
    CORSOrigin string
}

func (r *OCSessionFactory) Start(dir string, port int) (int, error) {
    // Use "serve" subcommand for headless HTTP mode.
    // opencode serve does NOT accept a positional directory argument;
    // the project is selected via the working directory (cmd.Dir).
    // opencode serve --port <port> [--cors <origin>] [extra flags...]
    args := []string{"serve"}
    args = append(args, r.Flags...)
    args = append(args, "--port", strconv.Itoa(port))
    if r.CORSOrigin != "" {
        args = append(args, "--cors", r.CORSOrigin)
    }

    cmd := exec.Command(r.Binary, args...)
    cmd.Dir = dir
    if err := cmd.Start(); err != nil {
        return 0, fmt.Errorf("starting opencode: %w", err)
    }
    return cmd.Process.Pid, nil
}

func (r *OCSessionFactory) Stop(pid int) error {
    proc, err := os.FindProcess(pid)
    if err != nil {
        return nil // already gone
    }
    return proc.Kill()
}

func (r *OCSessionFactory) HealthURL(port int) string {
    return fmt.Sprintf("http://localhost:%d", port)
}
```

### `internal/session/vscode.go`

`VSCodeSessionFactory` launches `code-server`. The password is passed as an environment variable (`PASSWORD=...`) rather than a flag, because that is how code-server's auth is designed. `os.Environ()` copies the current process environment so the child inherits `PATH` and everything else it needs.

`--ignore-last-opened` is required to prevent code-server from restoring the previously opened folder. Without it, each new instance inherits VS Code's workspace state and opens the last-used directory instead of the one passed as an argument — so a second session would show the wrong project.

```go
package session

import (
    "fmt"
    "os"
    "os/exec"
)

// VSCodeSessionFactory is a configured factory for VS Code (code-server) sessions.
type VSCodeSessionFactory struct {
    Binary   string
    Password string
}

func (r *VSCodeSessionFactory) Start(dir string, port int) (int, error) {
    cmd := exec.Command(r.Binary,
        "--bind-addr", fmt.Sprintf("127.0.0.1:%d", port),
        "--auth", "password",
        "--ignore-last-opened",
        dir,
    )
    cmd.Env = append(os.Environ(), "PASSWORD="+r.Password)
    if err := cmd.Start(); err != nil {
        return 0, fmt.Errorf("starting code-server: %w", err)
    }
    return cmd.Process.Pid, nil
}

func (r *VSCodeSessionFactory) Stop(pid int) error {
    proc, err := os.FindProcess(pid)
    if err != nil {
        return nil
    }
    return proc.Kill()
}

func (r *VSCodeSessionFactory) HealthURL(port int) string {
    return fmt.Sprintf("http://localhost:%d", port)
}
```

> **No tests for `opencode.go` or `vscode.go`:** Both files shell out to real binaries (`opencode`, `code-server`). Testing them would require a fake binary on `PATH` or abstracting `exec.Command` behind an interface — added complexity with little payoff. The interface they implement (`SessionFactory`) is tested through the manager tests via `fakeFactory`. If you later need to test the exec behaviour, the correct approach is to make the command constructor injectable (e.g. a `cmdFn func(name string, args ...string) *exec.Cmd` field on the struct).

---

## Lesson 6 — `internal/session/manager.go`: The Session Manager

### What this module does

The manager is the most complex part of the portal. It owns the runtime state of every running session and handles five distinct concerns:

1. **Port assignment** — scan the configured range and find a free port before starting a process.
2. **Process lifecycle** — start a session, record its PID, and stop it cleanly on request.
3. **Health checking** — after starting, poll the process's HTTP endpoint until it responds. Only then mark the session as ready.
4. **State persistence** — write the session map to a JSON file after every change. On restart, reload it and remove any sessions whose processes are no longer alive (orphans).
5. **SSE broadcasting** — publish events (`started`, `healthy`, `stopped`) to a channel that HTTP handlers can fan out to connected browsers.

### Key design decisions

**Why a buffered `events` channel of size 64?**  
The manager sends events synchronously (no goroutine). If the HTTP handler is slow to consume them, a blocking send would deadlock the manager. A buffer of 64 means the manager can fire 64 events before it blocks — more than enough for any realistic load. This is a pragmatic choice, not a scalable pub/sub system.

**Why a `factories` map instead of named `ocFactory`/`vsFactory` fields?**  
The original design had four fields — `ocFactory SessionFactory`, `vsFactory SessionFactory`, `ocRange portrange.PortRange`, `vsRange portrange.PortRange`. The names carry meaning that the types don't enforce: nothing prevents passing an `OCSessionFactory` as `vsFactory` at the call site. The map collapses these into a single `map[SessionType]registeredFactory`, where the key is the type discriminator and the value carries both the factory and its port range as a cohesive unit. Adding a third session type later requires no struct change — just one more `Register()` call.

**Why is `registeredFactory` unexported but `Register()` exported?**  
The caller (`server.go`) needs to construct registrations but has no legitimate reason to inspect or embed the struct directly. An unexported type with an exported constructor is Go's standard way to express this: you can create values of the type via the factory function, but you can't name the type itself outside the package. This prevents the call site from bypassing the constructor and constructing a half-initialised struct directly.

**Why variadic `...registeredFactory` rather than `map[SessionType]registeredFactory`?**  
A map literal requires naming the value type — which is unexported and therefore unavailable to the caller. A variadic argument accepts any number of `registeredFactory` values returned by `Register()` without the caller ever needing to name the type.

**Why save state after every mutation?**  
State persistence is cheap (a small JSON file) and the cost of losing it (all sessions appear stopped after a portal restart) is high. Writing on every change is the right tradeoff here.

**Why is `Event.Type` a named `EventType` and not a bare `string`?**  
Same reasoning as `SessionType` — a named string type makes the compiler reject arbitrary string literals and gives IDEs something to autocomplete. The three constants (`EventTypeStarted`, `EventTypeHealthy`, `EventTypeStopped`) are the only valid values; the type makes that contract explicit without the verbosity of the interface-based union approach.

**Why check `proc.Signal(0)` to detect orphans?**  
`os.FindProcess` on Unix never returns an error — it just constructs a process handle. The only way to check if a process is actually alive is to send it signal 0, which does nothing to the process but returns an error if it doesn't exist or you don't have permission.

### `internal/session/manager.go`

Before writing this file, add the UUID dependency:

```bash
go get github.com/google/uuid
```

UUIDs are used as session IDs. The stdlib has no UUID generator — `crypto/rand` could generate random bytes, but encoding them correctly to the standard UUID format is non-trivial. `github.com/google/uuid` is a well-maintained, zero-dependency package maintained by Google. This is a case where using a dependency is clearly the right call.

```go
package session

import (
    "context"
    "encoding/json"
    "fmt"
    "net"
    "net/http"
    "os"
    "path/filepath"
    "sync"
    "syscall"
    "time"

    "github.com/google/uuid"

    "workspace-portal/internal/portrange"
)

// registeredFactory pairs a SessionType, its SessionFactory, and its port range.
// Keeping them together means the Manager stays fully abstract —
// it never needs to name a concrete type.
type registeredFactory struct {
    sessionType SessionType
    factory      SessionFactory
    portRange   portrange.PortRange
}

// Register constructs a registeredFactory. This is the only way to create one
// outside this package — the struct itself is unexported.
func Register(sessionType SessionType, factory SessionFactory, portRange portrange.PortRange) registeredFactory {
    return registeredFactory{sessionType: sessionType, factory: factory, portRange: portRange}
}

// Manager manages the lifecycle of all running sessions.
type Manager struct {
    mu        sync.Mutex
    sessions  map[string]*Session
    stateFile string
    events    chan Event // SSE event broadcast channel
    factories map[SessionType]registeredFactory
}

// EventType identifies the lifecycle event emitted on the SSE channel.
// Using a named string type (rather than bare string) makes the compiler
// reject accidental string literals wherever an EventType is expected.
type EventType string

const (
    EventTypeStarted EventType = "started"
    EventTypeHealthy EventType = "healthy"
    EventTypeStopped EventType = "stopped"
)

// Event is sent on the SSE channel when session state changes.
type Event struct {
    Type    EventType
    Session *Session
}

// NewManager creates a Manager, loads persisted state, and removes orphans.
// Each factory is registered via Register() and passed as a variadic argument,
// keeping the unexported registeredFactory type out of the caller's namespace.
func NewManager(stateFile string, registrations ...registeredFactory) *Manager {
    factories := make(map[SessionType]registeredFactory, len(registrations))
    for _, r := range registrations {
        factories[r.sessionType] = r
    }
    m := &Manager{
        sessions:  make(map[string]*Session),
        stateFile: stateFile,
        events:    make(chan Event, 64),
        factories: factories,
    }
    m.loadState()
    return m
}

// Events returns the channel for SSE subscribers.
func (m *Manager) Events() <-chan Event {
    return m.events
}

// List returns all current sessions.
func (m *Manager) List() []*Session {
    m.mu.Lock()
    defer m.mu.Unlock()
    out := make([]*Session, 0, len(m.sessions))
    for _, s := range m.sessions {
        out = append(out, s)
    }
    return out
}

// Start launches a new session for the given directory and type.
func (m *Manager) Start(sessionType SessionType, dir string) (*Session, error) {
    reg, ok := m.factories[sessionType]
    if !ok {
        return nil, fmt.Errorf("unknown session type: %s", sessionType)
    }

    // Return existing session if one is already running for this dir+type
    if existing := m.findByDirAndType(dir, sessionType); existing != nil {
        return existing, nil
    }

    port, err := m.nextPort(reg.portRange)
    if err != nil {
        return nil, err
    }

    pid, err := reg.factory.Start(dir, port)
    if err != nil {
        return nil, err
    }

    s := &Session{
        ID:        uuid.New().String(),
        Type:      sessionType,
        Dir:       dir,
        Port:      port,
        PID:       pid,
        StartedAt: time.Now(),
    }

    m.mu.Lock()
    m.sessions[s.ID] = s
    m.mu.Unlock()
    m.saveState()

    m.events <- Event{Type: EventTypeStarted, Session: s}

    // Health check runs in a goroutine — it blocks until the process responds,
    // then updates s.URL and sends the "healthy" event.
    go m.waitHealthy(s, reg.factory.HealthURL(port))

    return s, nil
}

// Stop terminates a session by ID.
func (m *Manager) Stop(id string) error {
    m.mu.Lock()
    s, ok := m.sessions[id]
    if !ok {
        m.mu.Unlock()
        return fmt.Errorf("session %s not found", id)
    }
    delete(m.sessions, id)
    m.mu.Unlock()

    if reg, ok := m.factories[s.Type]; ok {
        reg.factory.Stop(s.PID)
    }
    m.saveState()
    m.events <- Event{Type: EventTypeStopped, Session: s}
    return nil
}

// waitHealthy polls until the session responds, then marks it healthy.
// It times out after 30 seconds to avoid leaking goroutines for processes that
// fail to start.
func (m *Manager) waitHealthy(s *Session, healthURL string) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return // timed out — process failed to become healthy
        case <-ticker.C:
            resp, err := http.Get(healthURL)
            if err == nil && resp.StatusCode < 500 {
                resp.Body.Close()
                m.mu.Lock()
                s.URL = healthURL
                m.mu.Unlock()
                m.saveState()
                m.events <- Event{Type: EventTypeHealthy, Session: s}
                return
            }
        }
    }
}

// nextPort finds the first available port in the given range.
// It checks both the in-use session map (fast) and then attempts to bind
// the port (authoritative — catches ports used by unrelated processes).
func (m *Manager) nextPort(r portrange.PortRange) (int, error) {
    m.mu.Lock()
    inUse := make(map[int]bool)
    for _, s := range m.sessions {
        inUse[s.Port] = true
    }
    m.mu.Unlock()

    for port := r[0]; port <= r[1]; port++ {
        if inUse[port] {
            continue
        }
        ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
        if err == nil {
            ln.Close()
            return port, nil
        }
    }
    return 0, fmt.Errorf("no available ports in range %d-%d", r[0], r[1])
}

// findByDirAndType returns an existing session if one is already running.
func (m *Manager) findByDirAndType(dir string, sessionType SessionType) *Session {
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, s := range m.sessions {
        if s.Dir == dir && s.Type == sessionType {
            return s
        }
    }
    return nil
}

// saveState persists current sessions to disk as JSON.
func (m *Manager) saveState() {
    m.mu.Lock()
    defer m.mu.Unlock()

    os.MkdirAll(filepath.Dir(m.stateFile), 0755)
    data, err := json.Marshal(m.sessions)
    if err == nil {
        os.WriteFile(m.stateFile, data, 0644)
    }
}

// loadState reads persisted sessions and removes orphans (processes no longer alive).
func (m *Manager) loadState() {
    data, err := os.ReadFile(m.stateFile)
    if err != nil {
        return // no state file yet — fresh start
    }
    var loaded map[string]*Session
    if err := json.Unmarshal(data, &loaded); err != nil {
        return
    }
    for id, s := range loaded {
        proc, err := os.FindProcess(s.PID)
        if err != nil || proc.Signal(syscall.Signal(0)) != nil {
            continue // orphan — process is gone
        }
        m.sessions[id] = s
    }
}
```

### `internal/session/manager_test.go`

The manager tests use a `fakeFactory` that satisfies `SessionFactory` without exec'ing anything. This lets us drive the full lifecycle (start, stop, state persistence, events) without real processes.

The `fakeFactory` design:

```go
type fakeFactory struct {
    nextPID int
}
func (f *fakeFactory) Start(dir string, port int) (int, error) { return f.nextPID, nil }
func (f *fakeFactory) Stop(pid int) error                      { return nil }
func (f *fakeFactory) HealthURL(port int) string               { return "" }
```

`HealthURL` returns `""` so `waitHealthy` fires an HTTP request to `http://` (which fails immediately) and exits. This is fine — the health check goroutine is a background concern we don't need to observe in these tests.

What the tests cover:

- `TestStart_UnknownType` — unknown `SessionType` returns an error
- `TestStart_CreatesSession` — correct type, dir, PID, and port are recorded
- `TestStart_Idempotent` — starting the same dir+type twice returns the same session ID
- `TestStop_RemovesSession` — session is removed from `List()` after `Stop()`
- `TestStop_UnknownID` — stopping a non-existent ID returns an error
- `TestList_Empty` — fresh manager returns an empty slice
- `TestStateFile_RoundTrip` — state file is written; second manager can load it without panicking
- `TestEvents_StartSendsEvent` — `EventTypeStarted` event appears in `Events()` channel after `Start()`
- `TestEvents_StopSendsEvent` — `EventTypeStopped` event appears after `Stop()`

```go
package session

import (
    "os"
    "path/filepath"
    "testing"

    "workspace-portal/internal/portrange"
)

type fakeFactory struct {
    nextPID int
}

func (f *fakeFactory) Start(dir string, port int) (int, error) { return f.nextPID, nil }
func (f *fakeFactory) Stop(pid int) error                      { return nil }
func (f *fakeFactory) HealthURL(port int) string               { return "" }

func newTestManager(t *testing.T, factory *fakeFactory) *Manager {
    t.Helper()
    stateFile := filepath.Join(t.TempDir(), "sessions.json")
    pr := portrange.PortRange{40000, 40099}
    return NewManager(stateFile, Register(SessionTypeOpenCode, factory, pr))
}

func TestStart_UnknownType(t *testing.T) {
    m := newTestManager(t, &fakeFactory{nextPID: 1})
    _, err := m.Start("vscode", "/some/dir")
    if err == nil {
        t.Fatal("expected error for unknown session type, got nil")
    }
}

func TestStart_CreatesSession(t *testing.T) {
    m := newTestManager(t, &fakeFactory{nextPID: 42})
    s, err := m.Start(SessionTypeOpenCode, "/my/project")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if s.Type != SessionTypeOpenCode {
        t.Errorf("got type %q, want %q", s.Type, SessionTypeOpenCode)
    }
    if s.Dir != "/my/project" {
        t.Errorf("got dir %q, want /my/project", s.Dir)
    }
    if s.PID != 42 {
        t.Errorf("got pid %d, want 42", s.PID)
    }
    if s.Port < 40000 || s.Port > 40099 {
        t.Errorf("port %d out of range", s.Port)
    }
    if s.ID == "" {
        t.Error("session ID is empty")
    }
}

func TestStart_Idempotent(t *testing.T) {
    m := newTestManager(t, &fakeFactory{nextPID: 1})
    s1, _ := m.Start(SessionTypeOpenCode, "/my/project")
    s2, _ := m.Start(SessionTypeOpenCode, "/my/project")
    if s1.ID != s2.ID {
        t.Errorf("expected same session ID on idempotent start; got %q vs %q", s1.ID, s2.ID)
    }
}

func TestStop_RemovesSession(t *testing.T) {
    m := newTestManager(t, &fakeFactory{nextPID: 7})
    s, _ := m.Start(SessionTypeOpenCode, "/my/project")
    m.Stop(s.ID)
    for _, existing := range m.List() {
        if existing.ID == s.ID {
            t.Error("session still present after Stop()")
        }
    }
}

func TestStop_UnknownID(t *testing.T) {
    m := newTestManager(t, &fakeFactory{})
    if err := m.Stop("does-not-exist"); err == nil {
        t.Fatal("expected error for unknown session ID, got nil")
    }
}

func TestList_Empty(t *testing.T) {
    m := newTestManager(t, &fakeFactory{})
    if got := m.List(); len(got) != 0 {
        t.Errorf("expected empty list, got %d sessions", len(got))
    }
}

func TestStateFile_RoundTrip(t *testing.T) {
    dir := t.TempDir()
    stateFile := filepath.Join(dir, "sessions.json")
    pr := portrange.PortRange{40000, 40099}
    factory := &fakeFactory{nextPID: 99}

    m1 := NewManager(stateFile, Register(SessionTypeOpenCode, factory, pr))
    m1.Start(SessionTypeOpenCode, "/persisted/project")

    if _, err := os.Stat(stateFile); err != nil {
        t.Fatalf("state file not created: %v", err)
    }

    // Second manager loading the same state file must not panic
    m2 := NewManager(stateFile, Register(SessionTypeOpenCode, factory, pr))
    _ = m2.List()
}

func TestEvents_StartSendsEvent(t *testing.T) {
    m := newTestManager(t, &fakeFactory{nextPID: 5})
    s, _ := m.Start(SessionTypeOpenCode, "/event/test")
    select {
    case ev := <-m.Events():
        if ev.Type != EventTypeStarted {
            t.Errorf("got event type %q, want %q", ev.Type, EventTypeStarted)
        }
        if ev.Session.ID != s.ID {
            t.Error("event session ID mismatch")
        }
    default:
        t.Error("no event sent after Start()")
    }
}

func TestEvents_StopSendsEvent(t *testing.T) {
    m := newTestManager(t, &fakeFactory{nextPID: 5})
    s, _ := m.Start(SessionTypeOpenCode, "/event/stop")
    <-m.Events() // drain started event
    m.Stop(s.ID)
    select {
    case ev := <-m.Events():
        if ev.Type != EventTypeStopped {
            t.Errorf("got event type %q, want %q", ev.Type, EventTypeStopped)
        }
    default:
        t.Error("no event sent after Stop()")
    }
}
```

Run:

```bash
go test ./internal/session/...
```

---

## Lesson 7 — `internal/server`: Wiring It All Together

### What this module does

The server module has two files:

- `server.go` — constructs all dependencies and starts listening. This is the composition root for the whole application.
- `handlers.go` — implements each HTTP handler, reading from config/manager and writing HTTP responses.

For now, all handlers return plain text. The HTMX UI and templates are added in Course 03 — these plain-text responses are placeholders that let you verify routing is correct before touching the UI layer.

### Key design decisions

**Why is `server.go` the composition root?**  
`main.go` is deliberately kept thin. `server.Start` is where all the wiring happens: which factories, which state file, which manager, which mux. This keeps `main.go` readable and makes it easy to see the full dependency graph in one place.

**Why a `handler` struct rather than standalone functions?**  
Handlers need access to shared state — the config and the session manager. In Go, the idiomatic way to thread shared state into handler functions is to hang them on a struct and pass that struct around. The alternative (global variables) makes testing and reasoning about state harder.

**How does SSE work?**  
Server-Sent Events is a one-way HTTP stream from server to browser. The client opens a persistent `GET /events` connection; the server never closes it, instead writing `event: ...\ndata: ...\n\n` frames as events occur. The `http.Flusher` interface is required to push each frame to the client immediately rather than buffering it.

**Why cast `r.FormValue("type")` to `session.SessionType`?**  
`r.FormValue` always returns a plain `string`. `manager.Start` expects a `session.SessionType`, which is a named string type — the compiler won't accept a bare `string` where a `session.SessionType` is required, even though both have the same underlying type. The explicit cast `session.SessionType(r.FormValue("type"))` is safe here: if the value doesn't match any known constant, `manager.Start` returns an "unknown session type" error, which the handler propagates as a 500.

### `internal/server/server.go`

```go
package server

import (
    "fmt"
    "log"
    "net/http"
    "os"
    "path/filepath"

    "workspace-portal/internal/config"
    "workspace-portal/internal/session"
)

// Start builds all dependencies and starts the HTTP server.
func Start(cfg *config.Config) error {
    // State file lives in the user's local data directory
    stateDir, _ := os.UserHomeDir()
    stateFile := filepath.Join(stateDir, ".local", "share", "workspace-portal", "sessions.json")

    // Build manager — each factory is paired with its type and port range via Register().
    // The registeredFactory type is unexported; Register() is the only way in.
    manager := session.NewManager(
        stateFile,
        session.Register(
            session.SessionTypeOpenCode,
            &session.OCSessionFactory{Binary: cfg.OC.Binary, Flags: cfg.OC.Flags},
            cfg.OC.PortRange,
        ),
        session.Register(
            session.SessionTypeVSCode,
            &session.VSCodeSessionFactory{Binary: cfg.VSCode.Binary, Password: cfg.Secret("vscode-password")},
            cfg.VSCode.PortRange,
        ),
    )

    // HTTP mux
    mux := http.NewServeMux()
    h := &handler{cfg: cfg, manager: manager}

    mux.HandleFunc("GET /", h.index)
    mux.HandleFunc("GET /fs/list", h.fsList)
    mux.HandleFunc("GET /sessions", h.sessions)
    mux.HandleFunc("POST /sessions/start", h.sessionsStart)
    mux.HandleFunc("POST /sessions/stop", h.sessionsStop)
    mux.HandleFunc("GET /events", h.events)
    mux.HandleFunc("GET /static/", h.static)

    addr := fmt.Sprintf(":%d", cfg.PortalPort)
    log.Printf("listening on %s", addr)
    return http.ListenAndServe(addr, mux)
}
```

### `internal/server/handlers.go`

The `// TODO Course 03` comments mark where templates will replace the plain-text responses. This is intentional — you can verify routing and data plumbing before committing to the UI layer.

```go
package server

import (
    "encoding/json"
    "fmt"
    "net/http"

    "workspace-portal/internal/config"
    fsmod "workspace-portal/internal/fs"
    "workspace-portal/internal/session"
)

type handler struct {
    cfg     *config.Config
    manager *session.Manager
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
    // TODO Course 03: render full layout template
    fmt.Fprintf(w, "workspace-portal — root: %s", h.cfg.WorkspacesRoot)
}

func (h *handler) fsList(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Query().Get("path")
    if path == "" {
        path = h.cfg.WorkspacesRoot
    }
    entries, err := fsmod.List(path, h.cfg.WorkspacesRoot)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    // TODO Course 03: render tree-row template for each entry
    for _, e := range entries {
        fmt.Fprintf(w, "%s (git=%v children=%v)\n", e.Name, e.IsGit, e.HasChildren)
    }
}

func (h *handler) sessions(w http.ResponseWriter, r *http.Request) {
    // TODO Course 03: render sessions template
    for _, s := range h.manager.List() {
        fmt.Fprintf(w, "%s %s port=%d\n", s.Type, s.Dir, s.Port)
    }
}

func (h *handler) sessionsStart(w http.ResponseWriter, r *http.Request) {
    r.ParseForm()
    sessionType := session.SessionType(r.FormValue("type"))
    dir := r.FormValue("dir")

    s, err := h.manager.Start(sessionType, dir)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    // TODO Course 03: return sessions HTML fragment
    fmt.Fprintf(w, "started %s for %s on port %d\n", s.Type, s.Dir, s.Port)
}

func (h *handler) sessionsStop(w http.ResponseWriter, r *http.Request) {
    r.ParseForm()
    id := r.FormValue("id")
    if err := h.manager.Stop(id); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    // TODO Course 03: return updated sessions HTML fragment
    fmt.Fprintf(w, "stopped %s\n", id)
}

// events streams Server-Sent Events to the browser.
// The connection stays open until the client disconnects (r.Context().Done()).
func (h *handler) events(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    // Heartbeat so the browser knows the connection is alive immediately
    fmt.Fprintf(w, ": heartbeat\n\n")
    flusher.Flush()

    for {
        select {
        case <-r.Context().Done():
            return // client disconnected
        case event := <-h.manager.Events():
            data, _ := json.Marshal(event.Session)
            fmt.Fprintf(w, "event: session.%s\ndata: %s\n\n", event.Type, data)
            flusher.Flush()
        }
    }
}

func (h *handler) static(w http.ResponseWriter, r *http.Request) {
    // TODO Course 03: serve embedded static files
}
```

### Verify the server runs

Create a minimal `config.yaml` (gitignored, so it stays local):

```yaml
workspaces_root: ~/workspaces
portal_port: 4000
```

```bash
go run ./cmd/portal
# workspace-portal starting on :4000
# listening on :4000
```

In a second terminal:

```bash
curl http://localhost:4000/
# workspace-portal — root: /Users/yourname/workspaces

curl http://localhost:4000/fs/list
# .secrets (git=false children=false)
# cyber-security (git=false children=true)
# de (git=false children=true)
# ...
```

The server works. The responses are plain text for now — that changes in Course 03.

 > **No unit tests for `server.go` (`Start`):** `Start` calls `http.ListenAndServe`, which binds a real port and blocks. That is integration-level behaviour. Testing it requires a free port, teardown logic, and a goroutine — complexity that adds little value over testing the handlers directly. If you want integration coverage, start the server in a goroutine in a `TestMain` setup function.

### `internal/server/handlers_test.go`

The handler test suite is added in Course 03, after `ManagerInterface` is introduced. Testing the handlers properly requires mocking the manager, which is only possible once the interface exists. See [Course 03 — Lesson 10](./course-03-htmx-and-sse.md#lesson-10--server-side-tests-for-handlers).


---

## Lesson 8 — Running Tests

With all modules implemented, run the full test suite:

```bash
go test ./...
```

All tests should pass. The most likely failure at this point is an import path mismatch — make sure every import uses `workspace-portal/internal/...` (matching your `go.mod` module name), not a GitHub URL.

Once the basic suite passes, run it again with the race detector:

```bash
go test -race ./...
```

The `-race` flag compiles the binary with race detection instrumentation. It catches data races — cases where two goroutines access the same memory concurrently and at least one is writing, without synchronisation. The session manager is the most likely source: it uses `sync.Mutex` around the session map, but any mutation path you add later that skips the lock will be caught here. Run `-race` regularly throughout development, not just at the end.

---

## Summary

You now have a working Go binary with:
- Config loading from YAML + env vars + `.secrets/`
- Directory tree listing with smart pruning and git detection
- Session lifecycle management (start, stop, health check, state persistence) — for OpenCode and VS Code
- HTTP server with all routes stubbed and responding

**Next:** [Course 03 — HTMX and SSE](./course-03-htmx-and-sse.md) — replace the plain-text responses with a real HTMX-driven UI.
