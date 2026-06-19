package session

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"workspace-portal/internal/portrange"
)

// fakeFactory implements SessionFactory without exec'ing a real editor.
// It starts a "sleep 9999" subprocess to satisfy the cmd.Process.Pid requirement.
type fakeFactory struct {
	startErr error
	stopErr  error
}

func (f *fakeFactory) Start(dir string, port int) (*exec.Cmd, int, error) {
	if f.startErr != nil {
		return nil, 0, f.startErr
	}
	cmd := exec.Command("sleep", "9999")
	if err := cmd.Start(); err != nil {
		// Fallback: if sleep is not available, use a shell one-liner.
		cmd = exec.Command("sh", "-c", "sleep 9999")
		if err2 := cmd.Start(); err2 != nil {
			return nil, 0, err2
		}
	}
	return cmd, port, nil
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

func TestStart_IdempotentByTypeAndDir(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "sessions.json")
	pr := portrange.PortRange{40000, 40099}
	factory := &fakeFactory{}
	m := NewManager(
		stateFile,
		&NoopRegistrar{},
		"",
		Register(SessionTypeOpenCode, factory, pr),
		Register(SessionTypeDocs, factory, pr),
	)

	opencode, err := m.Start(SessionTypeOpenCode, "/my/project")
	if err != nil {
		t.Fatalf("start opencode: %v", err)
	}
	t.Cleanup(func() { m.Stop(opencode.ID) }) //nolint:errcheck

	docs, err := m.Start(SessionTypeDocs, "/my/project")
	if err != nil {
		t.Fatalf("start docs: %v", err)
	}
	t.Cleanup(func() { m.Stop(docs.ID) }) //nolint:errcheck

	if opencode.ID == docs.ID {
		t.Fatal("expected different sessions for same dir but different type")
	}

	docs2, err := m.Start(SessionTypeDocs, "/my/project")
	if err != nil {
		t.Fatalf("second docs start: %v", err)
	}
	if docs.ID != docs2.ID {
		t.Errorf("expected docs session dedupe by type+dir; got %q vs %q", docs.ID, docs2.ID)
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

func TestStart_ReusesCachedPort(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})
	s1, err := m.Start(SessionTypeOpenCode, "/cached/project")
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	port := s1.Port

	if err := m.Stop(s1.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}

	s2, err := m.Start(SessionTypeOpenCode, "/cached/project")
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	t.Cleanup(func() { m.Stop(s2.ID) }) //nolint:errcheck

	if s2.Port != port {
		t.Errorf("expected same port %d after restart, got %d", port, s2.Port)
	}
}

func TestStart_CachedPortOutOfRange(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})

	// Inject a registry entry with a port outside the manager's range
	m.registry.Set(registryKey(SessionTypeOpenCode, "/out-of-range"), &PortEntry{
		Dir:         "/out-of-range",
		Type:        SessionTypeOpenCode,
		Port:        9999,
		LastStarted: time.Now(),
	})
	m.registry.Save()

	s, err := m.Start(SessionTypeOpenCode, "/out-of-range")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { m.Stop(s.ID) }) //nolint:errcheck

	if s.Port < 40000 || s.Port > 40099 {
		t.Errorf("port %d outside expected range 40000-40099", s.Port)
	}
	if s.Port == 9999 {
		t.Error("should not have reused out-of-range cached port 9999")
	}
}

func TestStart_CachedPortInUse(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})

	// Occupy a port in the range
	occupiedPort := 40050
	ln, err := net.Listen("tcp", "127.0.0.1:40050")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Inject registry entry pointing to the occupied port
	m.registry.Set(registryKey(SessionTypeOpenCode, "/occupied"), &PortEntry{
		Dir:         "/occupied",
		Type:        SessionTypeOpenCode,
		Port:        occupiedPort,
		LastStarted: time.Now(),
	})
	m.registry.Save()

	s, err := m.Start(SessionTypeOpenCode, "/occupied")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { m.Stop(s.ID) }) //nolint:errcheck

	if s.Port == occupiedPort {
		t.Error("should not have reused occupied cached port")
	}
	if s.Port < 40000 || s.Port > 40099 {
		t.Errorf("port %d outside expected range", s.Port)
	}
}

func TestStart_StaleEntryPurged(t *testing.T) {
	m := newTestManager(t, &fakeFactory{})

	// Inject a stale entry (15 days old)
	m.registry.Set(registryKey(SessionTypeOpenCode, "/stale"), &PortEntry{
		Dir:         "/stale",
		Type:        SessionTypeOpenCode,
		Port:        40050,
		LastStarted: time.Now().Add(-15 * 24 * time.Hour),
	})
	m.registry.Save()

	// Start a session for a different dir in the same range
	s, err := m.Start(SessionTypeOpenCode, "/active")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { m.Stop(s.ID) }) //nolint:errcheck

	// The stale entry should have been purged
	_, ok := m.registry.Get(registryKey(SessionTypeOpenCode, "/stale"))
	if ok {
		t.Error("stale entry was not purged during Start()")
	}
}

func TestStart_FactoryFailure_DoesNotPersistRegistryEntry(t *testing.T) {
	m := newTestManager(t, &fakeFactory{startErr: os.ErrNotExist})

	key := registryKey(SessionTypeOpenCode, "/ephemeral")

	// Sanity: no entry before Start
	if _, ok := m.registry.Get(key); ok {
		t.Fatal("unexpected registry entry before Start")
	}

	_, err := m.Start(SessionTypeOpenCode, "/ephemeral")
	if err == nil {
		t.Fatal("expected error from factory, got nil")
	}

	// The registry must NOT contain an entry after a failed Start
	if _, ok := m.registry.Get(key); ok {
		t.Error("registry entry persisted despite factory.Start() failure")
	}
}

func TestStateFile_PopulatesRegistryOnStartup(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "sessions.json")
	pr := portrange.PortRange{40000, 40099}

	// Write a sessions.json with a live session (our own PID) directly,
	// simulating a scenario where the registry was lost but state persists.
	sessionPort := 40050
	state := map[string]*Session{
		"restored-session": {
			ID:        "restored-session",
			Type:      SessionTypeOpenCode,
			Dir:       "/restored/project",
			Port:      sessionPort,
			PID:       os.Getpid(),
			StartedAt: time.Now(),
		},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a new manager — it should load the state and reconcile.
	// NOTE: no Stop cleanup needed — the session was loaded from state, not
	// started via Start(). Using os.Getpid() means Stop() would kill our process.
	m := NewManager(stateFile, &NoopRegistrar{}, "", Register(SessionTypeOpenCode, &fakeFactory{}, pr))

	sessions := m.List()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 restored session, got %d", len(sessions))
	}

	key := registryKey(SessionTypeOpenCode, "/restored/project")
	entry, ok := m.registry.Get(key)
	if !ok {
		t.Fatal("restored session has no registry entry after reconciliation")
	}
	if entry.Port != sessionPort {
		t.Errorf("registry port %d, want %d", entry.Port, sessionPort)
	}
}


