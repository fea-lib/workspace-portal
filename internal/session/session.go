package session

import (
	"os/exec"
	"time"
)

// SessionType identifies which editor a session runs.
// Using a named string type (rather than bare string) makes the compiler
// reject accidental string literals wherever a SessionType is expected.
type SessionType string

const (
	SessionTypeOpenCode SessionType = "opencode"
	SessionTypeVSCode   SessionType = "vscode"
	SessionTypeDocs     SessionType = "docs"
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

// SessionFactory is implemented by each session type (OpenCode, VS Code, Docs).
// It is a configured factory — it captures everything that is fixed at startup
// (binary path, flags, credentials) so that Start only needs what varies per
// session (directory and port).
type SessionFactory interface {
	// Start launches the process. Returns the exec.Cmd (process not yet healthy).
	Start(dir string, port int) (cmd *exec.Cmd, err error)
	// Stop terminates the process.
	Stop(pid int) error
	// HealthURL returns the URL to poll for the health check.
	HealthURL(port int) string
}

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
