package session_test

import (
	"fmt"
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

// noopFactory is a SessionFactory that returns a fixed PID without exec'ing anything.
type noopFactory struct{}

func (f *noopFactory) Start(dir string, port int) (int, error) { return 1, nil }
func (f *noopFactory) Stop(pid int) error                      { return nil }
func (f *noopFactory) HealthURL(port int) string               { return "" }

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
