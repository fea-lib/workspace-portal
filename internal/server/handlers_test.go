package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"workspace-portal/internal/config"
	"workspace-portal/internal/server"
	"workspace-portal/internal/session"
)

// fakeManager satisfies session.ManagerInterface for testing.
type fakeManager struct {
	sessions []*session.Session
	started  []*session.Session
	stopped  []string
}

func (f *fakeManager) Start(t session.SessionType, dir string) (*session.Session, error) {
	s := &session.Session{ID: "test-id", Type: t, Dir: dir, Port: 4100}
	f.sessions = append(f.sessions, s)
	f.started = append(f.started, s)
	return s, nil
}

func (f *fakeManager) Stop(id string) error {
	f.stopped = append(f.stopped, id)
	for i, s := range f.sessions {
		if s.ID == id {
			f.sessions = append(f.sessions[:i], f.sessions[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeManager) List() []*session.Session { return f.sessions }
func (f *fakeManager) Get(id string) (*session.Session, bool) {
	for _, s := range f.sessions {
		if s.ID == id {
			return s, true
		}
	}
	return nil, false
}
func (f *fakeManager) Events() <-chan session.Event {
	ch := make(chan session.Event)
	return ch
}

func newTestServer(t *testing.T, mgr session.ManagerInterface) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		WorkspacesRoot: t.TempDir(),
		PortalPort:     4000,
	}
	srv := server.New(cfg, mgr)
	return httptest.NewServer(srv)
}

func TestIndex(t *testing.T) {
	ts := newTestServer(t, &fakeManager{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Workspace Portal") {
		t.Error("response does not contain expected heading")
	}
}

func TestFsListPathTraversal(t *testing.T) {
	ts := newTestServer(t, &fakeManager{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/fs/list?path=../../etc")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403, got %d", resp.StatusCode)
	}
}

func TestSessionsStart(t *testing.T) {
	mgr := &fakeManager{}
	ts := newTestServer(t, mgr)
	defer ts.Close()

	form := url.Values{"type": {"opencode"}, "dir": {"my-project"}}
	resp, err := http.PostForm(ts.URL+"/sessions/start", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if len(mgr.started) != 1 {
		t.Fatalf("want 1 session started, got %d", len(mgr.started))
	}
	if mgr.started[0].Type != session.SessionTypeOpenCode {
		t.Errorf("want type opencode, got %s", mgr.started[0].Type)
	}
}

func TestSessionsStop(t *testing.T) {
	mgr := &fakeManager{
		sessions: []*session.Session{{ID: "abc", Type: session.SessionTypeOpenCode, Dir: "/foo", Port: 4100}},
	}
	ts := newTestServer(t, mgr)
	defer ts.Close()

	form := url.Values{"id": {"abc"}}
	resp, err := http.PostForm(ts.URL+"/sessions/stop", form)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if len(mgr.stopped) != 1 || mgr.stopped[0] != "abc" {
		t.Errorf("want stopped=[abc], got %v", mgr.stopped)
	}
}

func TestEventsContentType(t *testing.T) {
	ts := newTestServer(t, &fakeManager{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close() // close immediately so the SSE handler exits via context cancellation
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("want text/event-stream, got %s", ct)
	}
}
