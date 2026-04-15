package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"workspace-portal/internal/config"
	fsmod "workspace-portal/internal/fs"
	"workspace-portal/internal/session"
)

type handler struct {
	cfg     *config.Config
	manager *session.Manager
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
	// TODO Course 03: render full layout template
	fmt.Fprintf(w, "workspace-portal - root: %s", h.cfg.WorkspacesRoot)
}

func (h *handler) fsList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = h.cfg.WorkspacesRoot
	}

	entries, err := fsmod.List(path, h.cfg.WorkspacesRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO Course 03: render tree-row template for each entry
	for _, e := range entries {
		fmt.Fprintf(w, "%s (git=%v children=%v)\n", e.Name, e.IsGit, e.HasChildren)
	}
}

func (h *handler) sessions(w http.ResponseWriter, r *http.Request) {
	// TODO Course 03: render sessions template
	for _, s := range h.manager.List() {
		fmt.Fprintf(w, "%s %s port=%d\n", s.Type, s.Dir, s.Port)
	}
}

func (h *handler) sessionsStart(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	sessionType := session.SessionType(r.FormValue("type"))
	dir := r.FormValue("dir")

	s, err := h.manager.Start(sessionType, dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO Course 03: return sessions HTML fragment
	fmt.Fprintf(w, "started %s for %s on port %d\n", s.Type, s.Dir, s.Port)
}

func (h *handler) sessionsStop(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	id := r.FormValue("id")
	if err := h.manager.Stop(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO Course 03: return updated sessions HTML fragment
	fmt.Fprintf(w, "stopped %s\n", id)
}

// events streams Server-Sent Events to the browser.
// The connection stays open until the client disconnects (r.Context().Done()).
func (h *handler) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Heartbeat so the browser knows the connection is alive immediately
	fmt.Fprintf(w, ": heartbeat\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return // client disconnected
		case event := <-h.manager.Events():
			data, _ := json.Marshal(event.Session)
			fmt.Fprintf(w, "event: session.%s\ndata:%s\n\n", event.Type, data)
			flusher.Flush()
		}
	}
}

func (h *handler) static(w http.ResponseWriter, r *http.Request) {
	// TODO Course 03: serve embedded static files
}
