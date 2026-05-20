package server

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"workspace-portal/internal/fs"
	"workspace-portal/internal/session"
)

// pageData is passed to layout.html for the initial full-page render.
type pageData struct {
	Root        string // workspaces root path (display only)
	RootRow     treeRowData
	RootEntries []treeRowData
	Sessions    []sessionGroupData
}

// treeRowData is passed to tree-row.html for each directory entry.
type treeRowData struct {
	fs.DirEntry
	// Expanded is set server-side when rendering children inline.
	// For lazily-loaded rows it is always false on first render.
	Expanded bool
}

func (t treeRowData) Actions() []actionDescriptor {
	return orderedActions()
}

func (t treeRowData) SafeID() string {
	// Replace path separators and dots with underscores
	r := strings.NewReplacer("/", "_", ".", "_", " ", "_")

	return r.Replace(t.Path)
}

type actionDescriptor struct {
	Type  session.SessionType
	Label string
	Icon  string
}

type sessionActionData struct {
	ID          string
	Type        session.SessionType
	Label       string
	Icon        string
	Port        int
	OpenURL     string
	IsStarting  bool
	StopLabel   string
	StopConfirm string
}

type sessionGroupData struct {
	DirLabel string
	Actions  []sessionActionData
}

func orderedActions() []actionDescriptor {
	return []actionDescriptor{
		{Type: session.SessionTypeOpenCode, Label: "OpenCode", Icon: "opencode.svg"},
		{Type: session.SessionTypeVSCode, Label: "VS Code", Icon: "vscode.svg"},
		{Type: session.SessionTypeDocs, Label: "Docs", Icon: "docs.svg"},
	}
}

func toSessionGroups(root string, sessions []*session.Session) []sessionGroupData {
	type grouped struct {
		dir   string
		label string
		byTyp map[session.SessionType]*session.Session
	}

	byDir := map[string]*grouped{}
	for _, s := range sessions {
		g, ok := byDir[s.Dir]
		if !ok {
			g = &grouped{
				dir:   s.Dir,
				label: sessionDirLabel(root, s.Dir),
				byTyp: map[session.SessionType]*session.Session{},
			}
			byDir[s.Dir] = g
		}
		g.byTyp[s.Type] = s
	}

	sorted := make([]*grouped, 0, len(byDir))
	for _, g := range byDir {
		sorted = append(sorted, g)
	}
	sort.Slice(sorted, func(i, j int) bool {
		left := strings.ToLower(sorted[i].label)
		right := strings.ToLower(sorted[j].label)
		if left == right {
			return sorted[i].dir < sorted[j].dir
		}
		return left < right
	})

	out := make([]sessionGroupData, 0, len(sorted))
	for _, g := range sorted {
		group := sessionGroupData{DirLabel: g.label}
		for _, action := range orderedActions() {
			s, ok := g.byTyp[action.Type]
			if !ok {
				continue
			}
			group.Actions = append(group.Actions, sessionActionData{
				ID:          s.ID,
				Type:        action.Type,
				Label:       action.Label,
				Icon:        action.Icon,
				Port:        s.Port,
				OpenURL:     openURLForSession(*s),
				IsStarting:  s.URL == "",
				StopLabel:   fmt.Sprintf("%s (:%d)", action.Label, s.Port),
				StopConfirm: fmt.Sprintf("Stop %s for %s?", action.Label, g.label),
			})
		}
		out = append(out, group)
	}

	return out
}

func sessionDirLabel(root string, dir string) string {
	cleanRoot := filepath.Clean(root)
	cleanDir := filepath.Clean(dir)
	if cleanDir == cleanRoot {
		return filepath.Base(cleanRoot)
	}
	rel, err := filepath.Rel(cleanRoot, cleanDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.Base(cleanDir)
	}
	return filepath.ToSlash(rel)
}

// OpenCode SPA route: /{base64url(dir)}/session
func openURLForSession(s session.Session) string {
	if s.URL == "" {
		return ""
	}
	if s.Type != session.SessionTypeOpenCode {
		return s.URL
	}
	slug := base64.RawURLEncoding.EncodeToString([]byte(s.Dir))
	return fmt.Sprintf("%s/%s/session", s.URL, slug)
}
