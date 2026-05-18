package session_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

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

// healthyFactory is a SessionFactory whose HealthURL points to a live test server.
type healthyFactory struct {
	healthURL string
}

func (f *healthyFactory) Start(dir string, port int) (*exec.Cmd, error) {
	cmd := exec.Command("sleep", "9999")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (f *healthyFactory) Stop(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	proc.Kill() //nolint:errcheck
	return nil
}

func (f *healthyFactory) HealthURL(port int) string { return f.healthURL }

// noopFactory is a SessionFactory that starts a real subprocess but whose
// HealthURL returns "" so health checks never pass. Used for stop/deregister tests.
type noopFactory struct{}

func (f *noopFactory) Start(dir string, port int) (*exec.Cmd, error) {
	cmd := exec.Command("sleep", "9999")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (f *noopFactory) Stop(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	proc.Kill() //nolint:errcheck
	return nil
}

func (f *noopFactory) HealthURL(port int) string { return "" }

func TestManagerRegistersPortOnStart(t *testing.T) {
	// Start a real HTTP server so the health check passes quickly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	registrar := &mockRegistrar{}
	mgr := session.NewManager(
		t.TempDir()+"/sessions.json",
		registrar,
		"mock.ts.net",
		session.Register(
			session.SessionTypeOpenCode,
			&healthyFactory{healthURL: srv.URL},
			portrange.PortRange{9100, 9199},
		),
	)

	s, err := mgr.Start(session.SessionTypeOpenCode, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Stop(s.ID) }) //nolint:errcheck

	// waitHealthy runs asynchronously; poll until registration completes.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(registrar.registered) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(registrar.registered) != 1 || registrar.registered[0] != s.Port {
		t.Errorf("expected port %d to be registered, got %v", s.Port, registrar.registered)
	}
	// Reload the session to get the updated URL (set by waitHealthy).
	updated, ok := mgr.Get(s.ID)
	if !ok {
		t.Fatal("session not found after registration")
	}
	if updated.URL != fmt.Sprintf("https://mock.ts.net:%d", s.Port) {
		t.Errorf("unexpected session URL: %s", updated.URL)
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
