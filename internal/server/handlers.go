package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"workspace-portal/internal/assets"
	"workspace-portal/internal/config"
	fsmod "workspace-portal/internal/fs"
	"workspace-portal/internal/session"
)

type handler struct {
	cfg     *config.Config
	manager session.ManagerInterface
	tmpl    *template.Template
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	entries, err := fsmod.List(h.cfg.WorkspacesRoot, h.cfg.WorkspacesRoot)
	if err != nil {
		http.Error(w, "list root: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rows := make([]treeRowData, len(entries))
	for i, e := range entries {
		rows[i] = treeRowData{DirEntry: e}
	}

	data := pageData{
		Root:        h.cfg.WorkspacesRoot,
		RootEntries: rows,
		Sessions:    toSessionRows(h.manager.List()),
	}

	if err := h.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("render index: %v", err)
	}
}

func (h *handler) fsList(w http.ResponseWriter, r *http.Request) {
	// Sanitise and resolve the requested path
	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	// Prevent path traversal: the resolved path must stay inside workspaces root
	absPath := filepath.Join(h.cfg.WorkspacesRoot, relPath)
	if !strings.HasPrefix(absPath, h.cfg.WorkspacesRoot) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	entries, err := fsmod.List(absPath, h.cfg.WorkspacesRoot)
	if err != nil {
		http.Error(w, "list: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rows := make([]treeRowData, len(entries))
	for i, e := range entries {
		rows[i] = treeRowData{DirEntry: e}
	}

	if err := h.tmpl.ExecuteTemplate(w, "tree-children.html", rows); err != nil {
		log.Printf("render fsList: %v", err)
	}
}

func (h *handler) sessions(w http.ResponseWriter, r *http.Request) {
	if err := h.tmpl.ExecuteTemplate(w, "sessions.html", toSessionRows(h.manager.List())); err != nil {
		log.Printf("render sessions: %v", err)
	}
}

func (h *handler) sessionsStart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.tmpl.ExecuteTemplate(w, "sessions-error.html", "bad request: "+err.Error())
		return
	}
	sessionType := session.SessionType(r.FormValue("type"))
	dir := r.FormValue("dir")

	absDir := filepath.Join(h.cfg.WorkspacesRoot, dir)

	// Check for an existing session with the same type and directory
	for _, s := range h.manager.List() {
		if s.Type == sessionType && s.Dir == absDir {
			// Already running — just re-render the sessions list
			h.tmpl.ExecuteTemplate(w, "sessions.html", toSessionRows(h.manager.List()))
			return
		}
	}

	_, err := h.manager.Start(sessionType, absDir)
	if err != nil {
		h.tmpl.ExecuteTemplate(w, "sessions-error.html", "start session: "+err.Error())
		return
	}

	h.tmpl.ExecuteTemplate(w, "sessions.html", toSessionRows(h.manager.List()))
}

func (h *handler) sessionsStop(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.tmpl.ExecuteTemplate(w, "sessions-error.html", "bad request: "+err.Error())
		return
	}
	id := r.FormValue("id")
	if id == "" {
		h.tmpl.ExecuteTemplate(w, "sessions-error.html", "id required")
		return
	}

	if err := h.manager.Stop(id); err != nil {
		h.tmpl.ExecuteTemplate(w, "sessions-error.html", "stop session: "+err.Error())
		return
	}

	if err := h.tmpl.ExecuteTemplate(w, "sessions.html", toSessionRows(h.manager.List())); err != nil {
		log.Printf("render sessionsStop: %v", err)
	}
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
	// Strip the /static/ prefix and look up the file
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	switch name {
	case "htmx-2.0.8.min.js":
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(assets.HTMXJS)
	case "htmx-ext-sse.min.js":
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(assets.HTMXSSEJS)
	default:
		http.NotFound(w, r)
	}
}
