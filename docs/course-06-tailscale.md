---
title: "Course 06 — Tailscale Setup"
---

# Course 06 — Tailscale Setup

**Goal:** Install Tailscale on macOS, enable MagicDNS and HTTPS certificates in the admin console, expose the portal and its sessions securely over your tailnet, and implement the `internal/tailscale` Go module that wires this into the portal.  
**Prerequisite:** [Course 05 — Deployment](./course-05-deployment.md). Tailscale is optional — the portal works without it — but this course unlocks HTTPS URLs for all sessions.  
**Output:** The portal and all OpenCode/VS Code/script sessions accessible at `https://<your-machine>.ts.net` from any device on your tailnet. No port forwarding, no self-signed certificates.

---

## Lesson 1 — What Tailscale Does and Why

### The core idea

Tailscale creates a private, encrypted mesh network (called a **tailnet**) between all your devices using WireGuard under the hood. Every device on the tailnet gets:

- A stable private IP in the `100.x.x.x` range (stays the same regardless of which Wi-Fi network you're on)
- A DNS name via **MagicDNS**: `<machine-name>.<tailnet-name>.ts.net`
- The option to serve local ports over **valid HTTPS** using `tailscale serve`

For the workspace-portal, this means:

| Without Tailscale | With Tailscale |
|---|---|
| `http://localhost:4000` — only on the machine | `https://my-mac.tail1234.ts.net` — any device on the tailnet |
| `http://localhost:4101` — OpenCode session, local only | `https://my-mac.tail1234.ts.net:4101` — OpenCode from phone |
| Self-signed cert or no TLS | Valid Let's Encrypt cert, auto-provisioned |
| Code Server complains about insecure context | Code Server works: requires HTTPS for clipboard, PWA features |

### Why `tailscale serve` specifically

`tailscale serve` is Tailscale's built-in reverse proxy. When you run:

```bash
tailscale serve --bg --https=443 http://localhost:4000
```

Tailscale:
1. Registers a DNS-01 challenge with Let's Encrypt on your behalf
2. Provisions a TLS certificate for `<machine>.ts.net` (or `<machine>.ts.net:443`)
3. Terminates HTTPS on the Tailscale daemon and forwards plaintext to `localhost:4000`
4. Persists this configuration across reboots (the `--bg` flag)

The portal shells out to the `tailscale` binary to call this for each session port — no SDK, no Tailscale API keys required.

---

## Lesson 2 — Installing Tailscale on macOS

There are three install variants. They differ in what the `tailscale` CLI binary can do and where it lives.

### Option A — Standalone `.pkg` (recommended)

Download from [pkgs.tailscale.com/stable/#macos](https://pkgs.tailscale.com/stable/#macos). This is the variant Tailscale recommends for developer machines.

```bash
# After installing the .pkg, the CLI binary is at:
/Applications/Tailscale.app/Contents/MacOS/Tailscale

# Tailscale adds a symlink automatically:
which tailscale   # → /usr/local/bin/tailscale
```

The `.pkg` installs a menu-bar app that starts `tailscaled` automatically on login. This is what the launchd `PATH` in Course 05 expects: `/usr/local/bin` is on the default path.

### Option B — Mac App Store

Works for day-to-day VPN use but has one critical limitation for the portal:

> **Mac App Store variant cannot serve files or directories** due to the macOS App Sandbox. `tailscale serve /some/path` will fail. `tailscale serve http://localhost:PORT` works fine.

Since the portal only uses port-forwarding mode (`http://localhost:PORT`), the App Store variant is technically sufficient. But if you ever want to use `tailscale serve` for file serving outside the portal, use Option A.

### Option C — Homebrew CLI only

Use this if you want no menu-bar app and manage the daemon yourself (e.g. headless server, Docker host).

```bash
brew install tailscale

# Start the daemon (must run once; not started automatically by Homebrew):
sudo tailscaled &

# Or install it as a launchd daemon (run once):
sudo tailscaled install-system-daemon
```

After `install-system-daemon`, `tailscaled` starts on boot as a system daemon (root). The `tailscale` CLI is at `/opt/homebrew/bin/tailscale` (Apple Silicon) or `/usr/local/bin/tailscale` (Intel).

> **Important for the portal's launchd plist:** The launchd `PATH` in Course 05 includes `/opt/homebrew/bin`. If you used the standalone `.pkg`, the symlink at `/usr/local/bin/tailscale` is also included. Either install path works.

### Verify the install

```bash
tailscale version
# → 1.xx.x
```

---

## Lesson 3 — Connecting to Your Tailnet

### Sign in

```bash
tailscale up
# Opens a browser window to authenticate
```

If the machine is headless (no browser):

```bash
tailscale up --qr
# Prints a QR code to scan with your phone
```

### Check status

```bash
tailscale status
# my-mac          100.x.x.x    macOS   -
# my-phone        100.x.x.y    iOS     -
```

The machine name shown here becomes the subdomain in your HTTPS URL. If it is something ugly like `tobias-macbook-pro-2023`, rename it now before provisioning a TLS certificate — the certificate binds to the name and cannot be changed after provisioning.

### Rename the machine (optional but recommended)

In the [Tailscale admin console → Machines](https://login.tailscale.com/admin/machines), find the machine, click `…` → **Edit machine name**. Choose something short and stable: `dev-mac`, `homelab`, `workstation`.

After renaming:

```bash
tailscale status
# dev-mac         100.x.x.x    macOS   -
```

Your HTTPS URL will be `https://dev-mac.<tailnet>.ts.net`.

### Disable key expiry (recommended for always-on machines)

By default, Tailscale node keys expire after 90 days, requiring re-authentication. For a machine running the portal as a launchd service, key expiry breaks remote access silently.

In the admin console → Machines → select the machine → **Disable key expiry**.

Alternatively, if you use [tags](https://tailscale.com/kb/1068/acl-tags), key expiry is disabled by default on tagged nodes.

---

## Lesson 4 — Enabling MagicDNS

MagicDNS registers DNS names for every device on your tailnet automatically. Without it, `tailscale serve --https` cannot provision certificates (it needs a DNS name to put on the cert).

### Check if MagicDNS is already enabled

```bash
tailscale status
```

If your machine shows a `.ts.net` hostname like `dev-mac.tail1234.ts.net`, MagicDNS is active. Tailnets created after October 2022 have it enabled by default.

If the output only shows IP addresses and no `.ts.net` name, you need to enable it.

### Enable MagicDNS (admin console)

There is no CLI command to enable MagicDNS — it is a tailnet-wide setting in the admin console:

1. Go to [login.tailscale.com/admin/dns](https://login.tailscale.com/admin/dns)
2. Under **DNS**, find the **MagicDNS** toggle
3. Click **Enable MagicDNS**
4. If prompted to add a nameserver, you can skip it — Tailscale v1.20+ does not require one

After enabling, verify:

```bash
tailscale status
# dev-mac         100.x.x.x    macOS   dev-mac.tail1234.ts.net
```

The `.ts.net` FQDN now appears.

---

## Lesson 5 — Enabling HTTPS Certificates

HTTPS certificates let Tailscale provision a valid TLS cert for your machine's MagicDNS name via Let's Encrypt. This is what makes `tailscale serve --https` work without browser warnings.

### Enable HTTPS in the admin console

1. Go to [login.tailscale.com/admin/dns](https://login.tailscale.com/admin/dns)
2. Under **HTTPS Certificates**, click **Enable HTTPS**
3. Read the acknowledgement: your **machine names** will appear in the public [Certificate Transparency](https://en.wikipedia.org/wiki/Certificate_Transparency) ledger. The tailnet name (e.g. `tail1234.ts.net`) is already public, but so will the machine names of any machine you run `tailscale cert` on. If this is a concern, rename machines to non-identifying names before proceeding.
4. Confirm

### Provision the certificate on your machine

```bash
tailscale cert dev-mac.tail1234.ts.net
# Wrote dev-mac.tail1234.ts.net.crt
# Wrote dev-mac.tail1234.ts.net.key
```

This uses a DNS-01 ACME challenge — Tailscale handles it automatically. The cert and key files are written to the current directory. For `tailscale serve`, you do **not** need to manage these files manually; `tailscale serve --https` provisions its own cert internally. The `tailscale cert` command is mainly used when you want the cert files for another process (Caddy, nginx, etc.).

> **Certificate renewal:** Let's Encrypt certs expire after 90 days. `tailscale serve` manages renewal automatically. If you used `tailscale cert` to export files for another server, *you* are responsible for renewal — either re-run `tailscale cert` before expiry, or use Caddy's Tailscale integration which renews automatically.

### Verify HTTPS is working

```bash
tailscale serve --bg --https=8080 http://localhost:8080
# Serve started.
# Available within your tailnet:
# https://dev-mac.tail1234.ts.net:8080

tailscale serve status
# https://dev-mac.tail1234.ts.net:8080 (tailnet only)
# |-- / http://localhost:8080

# Clean up the test
tailscale serve --https=8080 off
```

If `tailscale serve --https` fails with "HTTPS not available", ensure the HTTPS toggle is on in the admin console and that MagicDNS is enabled.

---

## Lesson 6 — Exposing the Portal Over Tailscale

With MagicDNS and HTTPS enabled, exposing the portal is a single command.

### Register the portal

```bash
tailscale serve --bg --https=443 http://localhost:4000
```

The portal is now accessible at `https://dev-mac.tail1234.ts.net` from any device on your tailnet.

```bash
tailscale serve status
# https://dev-mac.tail1234.ts.net (tailnet only)
# |-- / http://localhost:4000
```

The `--bg` flag persists this across reboots and Tailscale restarts. If you restart the machine or restart `tailscaled`, `tailscale serve` automatically resumes.

### Remove the portal registration

```bash
tailscale serve --https=443 off
```

### Verify from another device

On your phone or another machine on the tailnet:

```
https://dev-mac.tail1234.ts.net
```

You should see the portal UI with a valid HTTPS certificate, no browser warnings.

---

## Lesson 7 — Tailscale in `internal/config`

Before implementing `internal/tailscale`, it is worth understanding how Tailscale is represented in the config module. The config structs were scaffolded in Course 02 so that `config.go` compiles before the Tailscale integration exists. This lesson explains those decisions in detail.

### `TSConfig` — the config struct

```go
type TSConfig struct {
    Enabled bool   `yaml:"enabled" env:"ENABLED"`
    Binary  string `yaml:"binary"  env:"BINARY"`
}
```

`Enabled` is `false` by default — opting in requires an explicit `tailscale.enabled: true` in `config.yaml`. This makes Tailscale strictly opt-in: a portal deployed without Tailscale never calls the `tailscale` binary.

`Binary` defaults to `"tailscale"` (resolved via `PATH`). Override it if the binary lives at a non-standard path (e.g. `/opt/homebrew/bin/tailscale` when using Homebrew on Apple Silicon without a `PATH` fix in the launchd plist).

### `Config` struct field

```go
type Config struct {
    // ...other fields...
    Tailscale TSConfig `yaml:"tailscale" envPrefix:"PORTAL_TAILSCALE_"`
}
```

The field is present regardless of whether Tailscale is enabled. This is intentional: the `TSConfig` struct is always unmarshalled from YAML, so you can stage `tailscale.enabled: false` in your config file and flip it to `true` when ready, without any Go code changes.

### Default in `defaults()`

```go
Tailscale: TSConfig{
    Binary: "tailscale",
},
```

Only `Binary` gets a default. `Enabled` is left as the zero value (`false`) — requiring an explicit opt-in.

### Env var override

`TSConfig` fields carry `env` struct tags, so `env.Parse` in `Load` picks them up automatically using the `PORTAL_TAILSCALE_` prefix declared on the parent `Config` field:

| Env var | Field | Example |
|---|---|---|
| `PORTAL_TAILSCALE_ENABLED` | `Tailscale.Enabled` | `true` |
| `PORTAL_TAILSCALE_BINARY` | `Tailscale.Binary` | `/opt/homebrew/bin/tailscale` |

Env vars take precedence over the YAML file. No manual `os.Getenv` call is needed — the same `env.Parse` pass that handles `PORTAL_PORT`, `PORTAL_OC_BINARY`, etc. covers Tailscale as well.

### Sample `config.yaml` with Tailscale enabled

```yaml
workspaces_root: ~/workspaces
portal_port: 4000

tailscale:
  enabled: true
  binary: tailscale  # or /usr/local/bin/tailscale
```

---

## Lesson 8 — `internal/tailscale`: The Go Module

This lesson implements the Go module that the portal uses to register and deregister session ports with `tailscale serve`. This code was introduced in the module scaffold in Course 02 but deferred here.

### Why shell out instead of using the Tailscale SDK

The Tailscale Go SDK exists but adds significant dependency weight and requires the portal to understand Tailscale's internal state. Shelling out to the `tailscale` CLI is:
- Simpler — the binary is already installed and authenticated
- More loosely coupled — the portal doesn't need to know anything about Tailscale's internals
- Easier to test — a fake `tailscale` shell script is a complete stub

### `internal/tailscale/serve.go`

```go
package tailscale

import (
    "fmt"
    "os/exec"
    "strconv"
)

// Serve implements session.Registrar using the tailscale CLI.
type Serve struct {
    Binary string // path to the tailscale binary, e.g. "tailscale" or "/usr/local/bin/tailscale"
}

// Register runs: tailscale serve --bg --https={port} http://localhost:{port}
// The returned URL is empty — the caller constructs it from the machine's FQDN.
func (s *Serve) Register(port int) (string, error) {
    p := strconv.Itoa(port)
    cmd := exec.Command(s.Binary,
        "serve", "--bg", "--https="+p,
        "http://localhost:"+p,
    )
    if out, err := cmd.CombinedOutput(); err != nil {
        return "", fmt.Errorf("tailscale serve: %w\n%s", err, out)
    }
    // URL construction is the caller's responsibility — it knows the machine FQDN.
    return "", nil
}

// Deregister removes the serve config for the given port.

// Uses best-effort: if the port was already deregistered, this is a no-op.
func (s *Serve) Deregister(port int) error {
    p := strconv.Itoa(port)
    cmd := exec.Command(s.Binary, "serve", "--https="+p, "off")
    cmd.Run() // intentionally best-effort
    return nil
}
```

`cmd.CombinedOutput()` runs the command, waits for it to finish, and returns the merged stdout+stderr output as a byte slice along with any error. Using it instead of separate `cmd.Stdout`/`cmd.Stderr` means that if `tailscale serve` fails, the error message includes the CLI's own output — making the wrapped error much easier to diagnose.

`Deregister` uses `cmd.Run()` instead — it discards output and the return value intentionally, since a "port not found" failure on deregister is harmless.

### Step 1 — Define `Registrar` in `internal/session/session.go`

`Serve` in `internal/tailscale` implements a `Registrar` interface, but that interface doesn't exist yet. Add it to `session.go` alongside the existing `SessionFactory`:

```go
// Registrar registers and deregisters a session port with an external proxy
// (e.g. tailscale serve). NoopRegistrar is used when Tailscale is disabled.
type Registrar interface {
    Register(port int) (url string, err error)
    Deregister(port int) error
}

// NoopRegistrar is used when Tailscale is disabled.
// Register and Deregister are no-ops — sessions are still started and
// assigned ports, but no external proxy registration is performed.
type NoopRegistrar struct{}

func (n *NoopRegistrar) Register(port int) (string, error) { return "", nil }
func (n *NoopRegistrar) Deregister(port int) error         { return nil }
```

### Step 2 — Add `registrar` and `tsFQDN` to `Manager`

The `Manager` struct needs to hold the registrar and the machine's Tailscale FQDN (used to construct session URLs when Tailscale is enabled). Update the struct and `NewManager` in `internal/session/manager.go`:

```go
type Manager struct {
    mu        sync.Mutex
    factories map[SessionType]registeredFactory
    sessions  map[string]*Session
    stateFile string
    events    chan Event
    registrar Registrar // nil-safe: always set, NoopRegistrar when disabled
    tsFQDN    string    // empty when Tailscale is disabled
}
```

Update `NewManager` to accept both:

```go
func NewManager(stateFile string, registrar Registrar, tsFQDN string, registrations ...registeredFactory) *Manager {
    factories := make(map[SessionType]registeredFactory, len(registrations))
    for _, r := range registrations {
        factories[r.sessionType] = r
    }

    m := &Manager{
        sessions:  make(map[string]*Session),
        stateFile: stateFile,
        events:    make(chan Event, 64),
        factories: factories,
        registrar: registrar,
        tsFQDN:    tsFQDN,
    }
    m.loadState()

    return m
}
```

`tsFQDN` is captured once at startup rather than queried per session — the machine FQDN does not change while the portal is running.

### Step 3 — Call the registrar in `waitHealthy()`

Registration must happen **after** the process is confirmed healthy — calling `tailscale serve` before opencode is listening results in 502 errors from the Tailscale reverse proxy. Update `waitHealthy` to register and set the session URL as part of the healthy path:

```go
// In waitHealthy(), replace the s.URL = healthURL line:
resp, err := http.Get(healthURL)
if err == nil && resp.StatusCode < 500 {
    resp.Body.Close()

    // Register with tailscale now that the process is actually listening.
    url, err := m.registrar.Register(s.Port)
    if err != nil {
        log.Printf("tailscale register port %d: %v", s.Port, err)
        // Non-fatal — session is still usable at localhost
    } else if url != "" {
        m.mu.Lock()
        s.URL = url
        m.mu.Unlock()
    } else if m.tsFQDN != "" {
        // tailscale.Serve.Register returns "" — the FQDN is known at startup.
        m.mu.Lock()
        s.URL = fmt.Sprintf("https://%s:%d", m.tsFQDN, s.Port)
        m.mu.Unlock()
    }

    if s.URL == "" {
        m.mu.Lock()
        s.URL = healthURL
        m.mu.Unlock()
    }

    m.saveState()
    m.events <- Event{Type: EventTypeHealthy, Session: s}
    return
}
```

Also call `Deregister` in `Stop()`, after removing the session from the map:

```go
// In Manager.Stop(), after delete(m.sessions, id):
m.registrar.Deregister(s.Port)
```

### Step 4 — Resolve the Tailscale FQDN at startup

The FQDN is obtained once in `server.go` before constructing the manager. Add a helper to `internal/tailscale/serve.go`:

```go
// FQDN returns the machine's Tailscale MagicDNS name (e.g. "dev-mac.tail1234.ts.net").
// Returns empty string if the tailscale binary cannot be called or the machine
// is not connected.
func (s *Serve) FQDN() string {
    out, err := exec.Command(s.Binary, "status", "--json").Output()
    if err != nil {
        return ""
    }
    var status struct {
        Self struct {
            DNSName string `json:"DNSName"`
        } `json:"Self"`
    }
    if err := json.Unmarshal(out, &status); err != nil {
        return ""
    }
    // DNSName has a trailing dot — trim it.
    return strings.TrimSuffix(status.Self.DNSName, ".")
}
```

This requires adding `"encoding/json"` and `"strings"` to the imports in `serve.go`.

### Step 5 — Wire it all together in `internal/server/server.go`

Now that `NewManager` accepts a `Registrar` and `tsFQDN`, update `Start()`:

```go
import (
    // ...
    "workspace-portal/internal/tailscale"
)

func Start(cfg *config.Config) error {
    stateDir, _ := os.UserHomeDir()
    stateFile := filepath.Join(stateDir, ".local", "share", "workspace-portal", "sessions.json")

    var registrar session.Registrar
    var tsFQDN string
    if cfg.Tailscale.Enabled {
        ts := &tailscale.Serve{Binary: cfg.Tailscale.Binary}
        registrar = ts
        tsFQDN = ts.FQDN()
    } else {
        registrar = &session.NoopRegistrar{}
    }

    manager := session.NewManager(
        stateFile,
        registrar,
        tsFQDN,
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

    srv := New(cfg, manager)

    addr := fmt.Sprintf(":%d", cfg.PortalPort)
    log.Printf("listening on %s", addr)

    return http.ListenAndServe(addr, srv)
}
```

When `tailscale.enabled: false`, `NoopRegistrar` is used and `tsFQDN` is empty — `Register` and `Deregister` are no-ops and session URLs remain `http://localhost:{port}`.

### Testing without Tailscale installed

There are two layers to test: the session manager (which uses the `Registrar` interface) and the `internal/tailscale` package itself (which shells out to the binary).

#### Update existing `NewManager` calls in `manager_test.go`

Adding `registrar` and `tsFQDN` to `NewManager`'s signature is a breaking change — every existing call site must be updated. In `manager_test.go`, the existing tests don't exercise Tailscale behaviour, so pass `nil` and `""` as neutral values. The `newTestManager` helper and the two direct `NewManager` calls in `TestStateFile_RoundTrip` all need updating:

```go
// newTestManager
return NewManager(stateFile, &NoopRegistrar{}, "", Register(SessionTypeOpenCode, factory, pr))

// TestStateFile_RoundTrip
m1 := NewManager(stateFile, &NoopRegistrar{}, "", Register(SessionTypeOpenCode, factory, pr))
m2 := NewManager(stateFile, &NoopRegistrar{}, "", Register(SessionTypeOpenCode, factory, pr))
```

`nil` would compile but causes a nil pointer panic — `Manager.Start` calls `m.registrar.Register(...)` unconditionally. `&NoopRegistrar{}` is the correct neutral value: `Register` and `Deregister` are no-ops, so existing tests are unaffected.

#### Unit testing the session manager with a mock registrar

Because `Manager` depends on the `Registrar` interface rather than `tailscale.Serve` directly, you can inject a mock in tests without Tailscale installed at all.

Create a new file `internal/session/registrar_test.go`. Using `package session_test` (the black-box variant) keeps it isolated from the internal `package session` tests in `manager_test.go` — no package declaration conflict, and no need to touch the existing test file.

```go
package session_test

import (
    "fmt"
    "os/exec"
    "testing"

    "workspace-portal/internal/portrange"
    "workspace-portal/internal/session"
)

type mockRegistrar struct {
    registered   []int
    deregistered []int
}

func (m *mockRegistrar) Register(port int) (string, error) {
    m.registered = append(m.registered, port)
    return fmt.Sprintf("https://mock.ts.net:%d", port), nil
}

func (m *mockRegistrar) Deregister(port int) error {
    m.deregistered = append(m.deregistered, port)
    return nil
}

// noopFactory is a SessionFactory that starts a no-op process without exec'ing anything real.
type noopFactory struct{}

func (f *noopFactory) Start(dir string, port int) (*exec.Cmd, error) {
    cmd := exec.Command("true")
    cmd.Start()
    return cmd, nil
}
func (f *noopFactory) Stop(pid int) error        { return nil }
func (f *noopFactory) HealthURL(port int) string { return "" }

func TestManagerRegistersPortOnStart(t *testing.T) {
    registrar := &mockRegistrar{}
    mgr := session.NewManager(
        t.TempDir()+"/sessions.json",
        registrar,
        "mock.ts.net",
        session.Register(
            session.SessionTypeOpenCode,
            &noopFactory{},
            portrange.PortRange{9100, 9199},
        ),
    )

    s, err := mgr.Start(session.SessionTypeOpenCode, t.TempDir())
    if err != nil {
        t.Fatal(err)
    }

    if len(registrar.registered) != 1 || registrar.registered[0] != s.Port {
        t.Errorf("expected port %d to be registered, got %v", s.Port, registrar.registered)
    }
    if s.URL != fmt.Sprintf("https://mock.ts.net:%d", s.Port) {
        t.Errorf("unexpected session URL: %s", s.URL)
    }
}

func TestManagerDeregistersPortOnStop(t *testing.T) {
    registrar := &mockRegistrar{}
    mgr := session.NewManager(
        t.TempDir()+"/sessions.json",
        registrar,
        "mock.ts.net",
        session.Register(
            session.SessionTypeOpenCode,
            &noopFactory{},
            portrange.PortRange{9100, 9199},
        ),
    )

    s, _ := mgr.Start(session.SessionTypeOpenCode, t.TempDir())
    mgr.Stop(s.ID)

    if len(registrar.deregistered) != 1 || registrar.deregistered[0] != s.Port {
        t.Errorf("expected port %d to be deregistered, got %v", s.Port, registrar.deregistered)
    }
}
```

Go allows both `package session` and `package session_test` files to coexist in the same directory — the compiler treats them as separate test binaries. `manager_test.go` stays untouched as `package session`; `registrar_test.go` uses `package session_test` and accesses only exported identifiers. The `noopFactory` here mirrors the `fakeFactory` in `manager_test.go` but is redeclared locally since it cannot cross package boundaries.

Run the tests:

```bash
go test ./internal/session/...
```

#### Integration testing `internal/tailscale` with a fake binary

To test that `Serve.Register` and `Serve.Deregister` call the CLI with the right arguments, replace the `tailscale` binary with a shell script during the test.

**Create the fake binary** at `internal/tailscale/testdata/tailscale`:

```bash
#!/usr/bin/env bash
# Fake tailscale binary — records arguments and exits cleanly.
# Used by integration tests in the internal/tailscale package.
echo "fake-tailscale called with: $@"
exit 0
```

Make it executable:

```bash
chmod +x internal/tailscale/testdata/tailscale
```

Go's test tooling ignores `testdata/` directories when building — the script will never be compiled, only executed by the test.

**Create `internal/tailscale/serve_test.go`** with a helper that prepends `testdata/` to `PATH` before each test, so `exec.Command("tailscale", ...)` resolves to the fake script rather than any real installation:

```go
package tailscale_test

import (
    "os"
    "path/filepath"
    "runtime"
    "testing"

    "workspace-portal/internal/tailscale"
)

func withFakeBinary(t *testing.T) string {
    t.Helper()
    _, file, _, _ := runtime.Caller(0)
    testdata := filepath.Join(filepath.Dir(file), "testdata")
    orig := os.Getenv("PATH")
    os.Setenv("PATH", testdata+":"+orig)
    t.Cleanup(func() { os.Setenv("PATH", orig) })
    return "tailscale" // the binary name — resolved via the updated PATH
}

func TestServeRegister(t *testing.T) {
    binary := withFakeBinary(t)
    s := &tailscale.Serve{Binary: binary}

    url, err := s.Register(4101)
    if err != nil {
        t.Fatalf("Register returned error: %v", err)
    }
    if url != "" {
        t.Errorf("expected empty URL, got %q", url)
    }
}

func TestServeDeregister(t *testing.T) {
    binary := withFakeBinary(t)
    s := &tailscale.Serve{Binary: binary}

    if err := s.Deregister(4101); err != nil {
        t.Fatalf("Deregister returned error: %v", err)
    }
}
```

Run the integration tests:

```bash
go test ./internal/tailscale/...
```

Both test suites pass with no Tailscale installation required.

---

## Lesson 9 — Troubleshooting

### `tailscale serve status` — check what's registered

```bash
tailscale serve status
# https://dev-mac.tail1234.ts.net (tailnet only)
# |-- / http://localhost:4000
# https://dev-mac.tail1234.ts.net:4101 (tailnet only)
# |-- / http://localhost:4101
```

This shows every active serve route. If the portal started a session but you don't see the port here, `tailscale.enabled` is likely `false` in config.

### `tailscale serve reset` — clear everything

```bash
tailscale serve reset
```

Removes all serve routes. Use this if sessions accumulate stale routes after portal crashes.

> The portal calls `Deregister` on clean `stop` — but if the portal crashes mid-session, routes can leak. `tailscale serve reset` clears them all at once. Run it before restarting the portal after a crash.

### "HTTPS not available" error

The `tailscale serve --https` command requires both:
1. MagicDNS enabled (admin console → DNS page)
2. HTTPS certificates enabled (same page)

Check both toggles.

### Certificate errors in the browser

If `https://dev-mac.tail1234.ts.net` shows a certificate warning:

```bash
# Check the current cert expiry
tailscale cert dev-mac.tail1234.ts.net 2>&1
# Or check via openssl:
openssl s_client -connect dev-mac.tail1234.ts.net:443 </dev/null 2>/dev/null | openssl x509 -noout -dates
```

If the cert is expired, `tailscale serve` usually auto-renews. If it doesn't, run `tailscale serve reset && tailscale serve --bg --https=443 http://localhost:4000` to force re-registration.

### Key expiry breaks remote access

If the machine's Tailscale key expires, all serve routes become unreachable — but the routes remain registered. The fix:

```bash
tailscale up  # re-authenticate
```

To avoid this: disable key expiry in the admin console for the portal machine (see Lesson 3).

### Port conflicts between `tailscale serve` routes

Each port can only have one serve target. If the portal assigns port 4101 to a session and `tailscale serve` already has a route for `:4101` from a previous (crashed) session, `Register` will fail with an error like "already in use".

Mitigation: call `tailscale serve reset` before starting the portal after any unclean shutdown. Or add a startup cleanup step to `internal/tailscale`:

```go
// Optional: clear all serve routes on portal startup before registering new ones.
func (s *Serve) Reset() error {
    cmd := exec.Command(s.Binary, "serve", "reset")
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("tailscale serve reset: %w\n%s", err, out)
    }
    return nil
}
```

Call `Reset()` in `Start()` before the session manager is initialised, only when `tailscale.enabled: true`.

---

## Lesson 10 — Checklist

Before relying on Tailscale for daily remote access:

### Tailscale setup
- [ ] `tailscale status` shows the machine connected with a `.ts.net` FQDN
- [ ] Machine name is short and stable (not `your-macbook-pro-m2-2023`)
- [ ] Key expiry is disabled on the portal machine (admin console → Machines → machine → Disable key expiry)
- [ ] MagicDNS is enabled (admin console → DNS page)
- [ ] HTTPS certificates are enabled (same DNS page)

### Portal exposure
- [ ] `tailscale serve --bg --https=443 http://localhost:4000` has been run
- [ ] `tailscale serve status` shows the portal route
- [ ] `https://<machine>.ts.net` opens the portal from another device on the tailnet
- [ ] The browser shows a valid certificate (no warning)

### Session exposure
- [ ] `tailscale.enabled: true` is set in `config.yaml`
- [ ] Starting an OC or script session from the portal produces an HTTPS URL
- [ ] `tailscale serve status` shows the session port after starting
- [ ] Stopping the session removes its port from `tailscale serve status`

### Resilience
- [ ] Restarted the portal (`launchctl stop / start`) and confirmed serve routes persisted (they are `--bg` registered, not managed by the portal process itself)
- [ ] Rebooted the machine and confirmed the portal is accessible and `tailscale serve status` shows the portal route

---

## Summary

The portal is now fully integrated with Tailscale:

1. **Installed** — standalone `.pkg` (recommended) or Homebrew CLI
2. **Connected** — machine on tailnet, named and key expiry disabled
3. **MagicDNS enabled** — `<machine>.ts.net` resolves across all tailnet devices
4. **HTTPS enabled** — valid Let's Encrypt certs provisioned via Tailscale
5. **Portal exposed** — `https://<machine>.ts.net` accessible from phone, tablet, and any tailnet device
6. **Sessions exposed** — each OpenCode/VS Code/script session gets its own HTTPS port, registered on start and deregistered on stop
7. **Go module implemented** — `internal/tailscale.Serve` shells out to the CLI; `NoopRegistrar` handles the disabled path transparently

The complete round-trip from "tap Open OpenCode in mobile browser" to "OpenCode running in a browser tab, accessible via HTTPS" is now in place.
