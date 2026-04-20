package server

import (
	"encoding/base64"
	"fmt"
	"strings"

	"workspace-portal/internal/fs"
	"workspace-portal/internal/session"
)

// pageData is passed to layout.html for the initial full-page render.
type pageData struct {
	Root        string // workspaces root path (display only)
	RootEntries []treeRowData
	Sessions    []sessionRowData
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

// toSessionRows wraps a slice of sessions into sessionRowData for template rendering.
func toSessionRows(sessions []*session.Session) []sessionRowData {
	rows := make([]sessionRowData, len(sessions))
	for i, s := range sessions {
		rows[i] = sessionRowData{Session: *s}
	}
	return rows
}

// OpenURL returns the URL to open the session in the browser, navigating
// directly to the project directory using OpenCode's URL-safe base64 slug.
// OpenCode SPA route: /{base64url(dir)}/session
func (s sessionRowData) OpenURL() string {
	if s.URL == "" {
		return ""
	}
	if s.Type != session.SessionTypeOpenCode {
		return s.URL
	}
	slug := base64.RawURLEncoding.EncodeToString([]byte(s.Dir))
	return fmt.Sprintf("%s/%s/session", s.URL, slug)
}
