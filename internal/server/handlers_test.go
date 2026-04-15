package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"workspace-portal/internal/config"
	"workspace-portal/internal/portrange"
	"workspace-portal/internal/session"
)

// fakeFactory implements session.SessionFactory without exec'ing anything.
type fakeFactory struct {
	nextPID int
}

func (f *fakeFactory) Start(dir string, port int) (int, error) { return f.nextPID, nil }
func (f *fakeFactory) Stop(pid int) error                      { return nil }
func (f *fakeFactory) HealthURL(port int) string               { return "" }

// newTestHandler wires up a handler with a temp state file and a real Manager.
func newTestHandler(t *testing.T) *handler {
	t.Helper()
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "sessions.json")

	factory := &fakeFactory{nextPID: 1}
	pr := portrange.PortRange{41000, 41099}
	mgr := session.NewManager(
		stateFile,
		session.Register(session.SessionTypeOpenCode, factory, pr),
	)

	cfg := &config.Config{
		WorkspacesRoot: tmpDir,
	}

	return &handler{cfg: cfg, manager: mgr}
}

func TestIndex(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.index(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "workspace-portal") {
		t.Errorf("body %q missing 'workspace-portal'", rec.Body.String())
	}
}

func TestFsList_DefaultPath(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fs/list", nil)
	h.fsList(rec, req)

	// tmpDir exists and is readable, so we expect 200 (empty listing is fine)
	if rec.Code != http.StatusOK {
		t.Errorf("status %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSessions_Empty(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	h.sessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for empty session list, got %q", rec.Body.String())
	}
}

func TestSessionsStart_Success(t *testing.T) {
	h := newTestHandler(t)
	dir := t.TempDir()

	form := url.Values{}
	form.Set("type", string(session.SessionTypeOpenCode))
	form.Set("dir", dir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/sessions/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	h.sessionsStart(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "started") {
		t.Errorf("body %q missing 'started'", rec.Body.String())
	}
}

func TestSessionsStart_UnknownType(t *testing.T) {
	h := newTestHandler(t)

	form := url.Values{}
	form.Set("type", "unknown-type")
	form.Set("dir", t.TempDir())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/sessions/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	h.sessionsStart(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
}

func TestSessionsStop_Success(t *testing.T) {
	h := newTestHandler(t)
	dir := t.TempDir()

	// First start a session
	form := url.Values{}
	form.Set("type", string(session.SessionTypeOpenCode))
	form.Set("dir", dir)
	startRec := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/sessions/start", strings.NewReader(form.Encode()))
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.sessionsStart(startRec, startReq)

	// Grab the session ID from the manager
	sessions := h.manager.List()
	if len(sessions) == 0 {
		t.Fatal("expected at least one session after start")
	}
	id := sessions[0].ID

	// Now stop it
	stopForm := url.Values{}
	stopForm.Set("id", id)
	stopRec := httptest.NewRecorder()
	stopReq := httptest.NewRequest(http.MethodPost, "/sessions/stop", strings.NewReader(stopForm.Encode()))
	stopReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.sessionsStop(stopRec, stopReq)

	if stopRec.Code != http.StatusOK {
		t.Errorf("status %d, want 200; body: %s", stopRec.Code, stopRec.Body.String())
	}
	if !strings.Contains(stopRec.Body.String(), "stopped") {
		t.Errorf("body %q missing 'stopped'", stopRec.Body.String())
	}
}

func TestSessionsStop_UnknownID(t *testing.T) {
	h := newTestHandler(t)

	stopForm := url.Values{}
	stopForm.Set("id", "does-not-exist")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/sessions/stop", strings.NewReader(stopForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.sessionsStop(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
}
