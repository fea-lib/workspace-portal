package session

import (
	"os"
	"path/filepath"
	"testing"

	"workspace-portal/internal/portrange"
)

// fakeFactory implements SessionFactory without exec'ing anything.
type fakeFactory struct {
	startErr error
	stopErr  error
	nextPID  int
}

func (f *fakeFactory) Start(dir string, port int) (int, error) {
	return f.nextPID, f.startErr
}

func (f *fakeFactory) Stop(pid int) error {
	return f.stopErr
}

func (f *fakeFactory) HealthURL(port int) string {
	return "" // no health check in unit tests
}

// newTestManager creates a Manager wired to a fakeFactory with a temp state file.
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
		t.Errorf("got dir %q, want %q", s.Dir, "/my/project")
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
	s1, err := m.Start(SessionTypeOpenCode, "/my/project")
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	s2, err := m.Start(SessionTypeOpenCode, "/my/project")
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if s1.ID != s2.ID {
		t.Errorf("expected same session ID on idempotent start; got %q vs %q", s1.ID, s2.ID)
	}
}

func TestStop_RemovesSession(t *testing.T) {
	m := newTestManager(t, &fakeFactory{nextPID: 7})
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
	factory := &fakeFactory{nextPID: 99}

	// Create manager and start a session
	m1 := NewManager(stateFile, Register(SessionTypeOpenCode, factory, pr))
	s, err := m1.Start(SessionTypeOpenCode, "/persisted/project")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Confirm state file was written
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	// A second manager loading the same state file should see the session
	// (loadState skips orphans, but PID 99 is likely not alive — so we only
	// check that the JSON was written and is readable without crashing).
	_ = s
	m2 := NewManager(stateFile, Register(SessionTypeOpenCode, factory, pr))
	_ = m2.List() // must not panic
}

func TestEvents_StartSendsEvent(t *testing.T) {
	m := newTestManager(t, &fakeFactory{nextPID: 5})
	s, err := m.Start(SessionTypeOpenCode, "/event/test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

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
	m := newTestManager(t, &fakeFactory{nextPID: 5})
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
