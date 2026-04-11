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

// Runner is implemented by each session type (OpenCode, VS Code).
type Runner interface {
	// Start launches the process. Returns when the process has started
	// (not necessarily healthy yet).
	Start(dir string, port int) (pid int, err error)
	// Stop terminates the process.
	Stop(pid int) error
	// HealthURL returns the URL to poll for the health check.
	HealthURL(port int) string
}
