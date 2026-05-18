package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"workspace-portal/internal/portrange"
)

// fakeFactory implements SessionFactory without exec'ing a real editor.
// It starts a "sleep 9999" subprocess to satisfy the cmd.Process.Pid requirement.
type fakeFactory struct {
	startErr error
	stopErr  error
}

func (f *fakeFactory) Start(dir string, port int) (*exec.Cmd, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	cmd := exec.Command("sleep", "9999")
	if err := cmd.Start(); err != nil {
		// Fallback: if sleep is not available, use a shell one-liner.
		cmd = exec.Command("sh", "-c", "sleep 9999")
		if err2 := cmd.Start(); err2 != nil {
			return nil, err2
		}
	}
	return cmd, nil
}

func (f *fakeFactory) Stop(pid int) error {
	if f.stopErr != nil {
		return f.stopErr
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	proc.Kill() //nolint:errcheck
	return nil
}

func (f *fakeFactory) HealthURL(port int) string {
	return "" // no health check in unit tests
}

// newTestManager creates a Manager wired to a fakeFactory with a temp state file.
func newTestManager(t *testing.T, factory *fakeFactory) *Manager {
	t.Helper()
	stateFile := filepath.Join(t.TempDir(), "sessions.json")
	pr := portrange.PortRange{40000, 40099}
	return NewManager(stateFile, &NoopRegistrar{}, "", Register(SessionTypeOpenCode, factory, pr))
}

func TestStart_UnknownType(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})
	_, err := m.Start("vscode", "/some/dir")
	if err == nil {
		t.Fatal("expected error for unknown session type, got nil")
	}
}

func TestStart_CreatesSession(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})
	s, err := m.Start(SessionTypeOpenCode, "/my/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { m.Stop(s.ID) }) //nolint:errcheck

	if s.Type != SessionTypeOpenCode {
		t.Errorf("got type %q, want %q", s.Type, SessionTypeOpenCode)
	}
	if s.Dir != "/my/project" {
		t.Errorf("got dir %q, want %q", s.Dir, "/my/project")
	}
	if s.PID <= 0 {
		t.Errorf("got pid %d, want > 0", s.PID)
	}
	if s.Port < 40000 || s.Port > 40099 {
		t.Errorf("port %d out of range", s.Port)
	}
	if s.ID == "" {
		t.Error("session ID is empty")
	}
}

func TestStart_Idempotent(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})
	s1, err := m.Start(SessionTypeOpenCode, "/my/project")
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	t.Cleanup(func() { m.Stop(s1.ID) }) //nolint:errcheck

	s2, err := m.Start(SessionTypeOpenCode, "/my/project")
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if s1.ID != s2.ID {
		t.Errorf("expected same session ID on idempotent start; got %q vs %q", s1.ID, s2.ID)
	}
}

func TestStop_RemovesSession(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})
	s, _ := m.Start(SessionTypeOpenCode, "/my/project")

	if err := m.Stop(s.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}

	sessions := m.List()
	for _, existing := range sessions {
		if existing.ID == s.ID {
			t.Error("session still present after Stop()")
		}
	}
}

func TestStop_UnknownID(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})
	err := m.Stop("does-not-exist")
	if err == nil {
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
	factory := &fakeFactory{}

	// Create manager and start a session
	m1 := NewManager(stateFile, &NoopRegistrar{}, "", Register(SessionTypeOpenCode, factory, pr))
	s, err := m1.Start(SessionTypeOpenCode, "/persisted/project")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { m1.Stop(s.ID) }) //nolint:errcheck

	// Confirm state file was written
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	// A second manager loading the same state file should see the session
	// (loadState skips orphans, but the PID may or may not be alive — we only
	// check that the JSON was written and is readable without crashing).
	m2 := NewManager(stateFile, &NoopRegistrar{}, "", Register(SessionTypeOpenCode, factory, pr))
	_ = m2.List() // must not panic
}

func TestEvents_StartSendsEvent(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})
	s, err := m.Start(SessionTypeOpenCode, "/event/test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { m.Stop(s.ID) }) //nolint:errcheck

	select {
	case ev := <-m.Events():
		if ev.Type != EventTypeStarted {
			t.Errorf("got event type %q, want %q", ev.Type, EventTypeStarted)
		}
		if ev.Session.ID != s.ID {
			t.Errorf("event session ID mismatch")
		}
	default:
		t.Error("no event sent after Start()")
	}
}

func TestEvents_StopSendsEvent(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})
	s, _ := m.Start(SessionTypeOpenCode, "/event/stop")
	<-m.Events() // drain the started event

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
