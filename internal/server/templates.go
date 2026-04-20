package server

import (
	"strings"

	"workspace-portal/internal/fs"
	"workspace-portal/internal/session"
)

// pageData is passed to layout.html for the initial full-page render.
type pageData struct {
	Root        string // workspaces root path (display only)
	RootEntries []treeRowData
	Sessions    []*session.Session
}

// treeRowData is passed to tree-row.html for each directory entry.
type treeRowData struct {
	fs.DirEntry
	// Expanded is set server-side when rendering children inline.
	// For lazily-loaded rows it is always false on first render.
	Expanded bool
}

func (t treeRowData) SafeID() string {
	// Replace path separators and dots with underscores
	r := strings.NewReplacer("/", "_", ".", "_", " ", "_")

	return r.Replace(t.Path)
}

// sessionRowData is passed to session-row.html for each session entry.
type sessionRowData struct {
	session.Session
}
