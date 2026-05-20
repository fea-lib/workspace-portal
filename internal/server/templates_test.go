package server

import (
	"encoding/base64"
	"fmt"
	"testing"

	"workspace-portal/internal/session"
)

func TestToSessionGroupsSortAndLabels(t *testing.T) {
	root := "/Users/test/workspaces/fea"
	sessions := []*session.Session{
		{ID: "s1", Type: session.SessionTypeVSCode, Dir: root + "/zeta", Port: 9203, URL: "http://127.0.0.1:9203"},
		{ID: "s2", Type: session.SessionTypeOpenCode, Dir: root, Port: 9103, URL: "http://127.0.0.1:9103"},
		{ID: "s3", Type: session.SessionTypeDocs, Dir: root + "/alpha", Port: 4303, URL: "http://127.0.0.1:4303"},
		{ID: "s4", Type: session.SessionTypeOpenCode, Dir: root + "/alpha", Port: 9104, URL: "http://127.0.0.1:9104"},
	}

	groups := toSessionGroups(root, sessions)
	if len(groups) != 3 {
		t.Fatalf("want 3 groups, got %d", len(groups))
	}

	if groups[0].DirLabel != "alpha" {
		t.Fatalf("want first group alpha, got %q", groups[0].DirLabel)
	}
	if groups[1].DirLabel != "fea" {
		t.Fatalf("want root label fea, got %q", groups[1].DirLabel)
	}
	if groups[2].DirLabel != "zeta" {
		t.Fatalf("want last group zeta, got %q", groups[2].DirLabel)
	}
}

func TestToSessionGroupsActionOrderAndOpenURL(t *testing.T) {
	root := "/Users/test/workspaces/fea"
	opencodeURL := "https://example.ts.net:9103"
	sessions := []*session.Session{
		{ID: "o1", Type: session.SessionTypeOpenCode, Dir: root + "/project", Port: 9103, URL: opencodeURL},
		{ID: "d1", Type: session.SessionTypeDocs, Dir: root + "/project", Port: 4303, URL: "http://127.0.0.1:4303"},
		{ID: "v1", Type: session.SessionTypeVSCode, Dir: root + "/project", Port: 9203, URL: "http://127.0.0.1:9203"},
	}

	groups := toSessionGroups(root, sessions)
	if len(groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(groups))
	}

	actions := groups[0].Actions
	if len(actions) != 3 {
		t.Fatalf("want 3 actions, got %d", len(actions))
	}

	if actions[0].Type != session.SessionTypeOpenCode || actions[1].Type != session.SessionTypeVSCode || actions[2].Type != session.SessionTypeDocs {
		t.Fatalf("unexpected action order: %s, %s, %s", actions[0].Type, actions[1].Type, actions[2].Type)
	}

	slug := base64.RawURLEncoding.EncodeToString([]byte(root + "/project"))
	wantURL := fmt.Sprintf("%s/%s/session", opencodeURL, slug)
	if actions[0].OpenURL != wantURL {
		t.Fatalf("want opencode open url %q, got %q", wantURL, actions[0].OpenURL)
	}
}

func TestToSessionGroupsStartingState(t *testing.T) {
	root := "/Users/test/workspaces/fea"
	sessions := []*session.Session{
		{ID: "o1", Type: session.SessionTypeOpenCode, Dir: root + "/project", Port: 9103, URL: ""},
	}

	groups := toSessionGroups(root, sessions)
	if len(groups) != 1 || len(groups[0].Actions) != 1 {
		t.Fatalf("unexpected group shape: groups=%d actions=%d", len(groups), len(groups[0].Actions))
	}

	if !groups[0].Actions[0].IsStarting {
		t.Fatal("expected starting action when URL is empty")
	}
}
