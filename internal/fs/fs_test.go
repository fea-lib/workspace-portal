package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestList(t *testing.T) {
	// Build a temp tree:
	// root/
	//   project-a/         (standard git repo — .git/ dir)
	//   project-b/         (no git, has one visible child)
	//     src/
	//   project-b/node_modules/  (pruned — not counted as visible child)
	//   project-c/         (worktree — .git is a regular file)
	//   project-d/         (bare repo — .bare/HEAD exists)
	//   project-e/         (leaf — no subdirs at all)
	//   node_modules/      (pruned by defaultPrune)
	//   .secrets/          (dotdir — must appear)
	//   ignored-dir/       (excluded by root .gitignore)
	//   subdir/
	//     nested-ignored/  (excluded by subdir .gitignore)
	//     visible/
	root, _ := os.MkdirTemp("", "portal-fs*")
	defer os.RemoveAll(root)

	// project-a: standard git repo
	os.MkdirAll(filepath.Join(root, "project-a", ".git"), 0755)

	// project-b: not a git repo, has one visible child (src) and one pruned child
	os.MkdirAll(filepath.Join(root, "project-b", "src"), 0755)
	os.MkdirAll(filepath.Join(root, "project-b", "node_modules"), 0755)

	// project-c: worktree (.git is a regular file)
	os.MkdirAll(filepath.Join(root, "project-c"), 0755)
	os.WriteFile(filepath.Join(root, "project-c", ".git"), []byte("gitdir: ../.bare/worktrees/main"), 0644)

	// project-d: bare repo (.bare/HEAD exists)
	os.MkdirAll(filepath.Join(root, "project-d", ".bare"), 0755)
	os.WriteFile(filepath.Join(root, "project-d", ".bare", "HEAD"), []byte("ref: refs/heads/main"), 0644)

	// project-e: leaf — no subdirs
	os.MkdirAll(filepath.Join(root, "project-e"), 0755)
	os.WriteFile(filepath.Join(root, "project-e", "README.md"), []byte("hello"), 0644)

	// pruned by defaultPrune
	os.MkdirAll(filepath.Join(root, "node_modules"), 0755)

	// dotdir — must appear
	os.MkdirAll(filepath.Join(root, ".secrets"), 0755)

	// excluded by root .gitignore
	os.MkdirAll(filepath.Join(root, "ignored-dir"), 0755)
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored-dir\n"), 0644)

	// subdir with its own .gitignore
	os.MkdirAll(filepath.Join(root, "subdir", "nested-ignored"), 0755)
	os.MkdirAll(filepath.Join(root, "subdir", "visible"), 0755)
	os.WriteFile(filepath.Join(root, "subdir", ".gitignore"), []byte("nested-ignored\n"), 0644)

	entries, err := List(root, root)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]DirEntry)
	for _, e := range entries {
		byName[e.Name] = e
	}

	// defaultPrune entries must be absent
	if _, ok := byName["node_modules"]; ok {
		t.Error("node_modules should be pruned")
	}

	// .gitignore-excluded entries must be absent
	if _, ok := byName["ignored-dir"]; ok {
		t.Error("ignored-dir should be excluded by .gitignore")
	}

	// dotdir must appear
	if _, ok := byName[".secrets"]; !ok {
		t.Error(".secrets should appear")
	}

	// Path must be relative to root
	if e, ok := byName["project-a"]; ok {
		want := "project-a"
		if e.Path != want {
			t.Errorf("project-a Path: got %q, want %q", e.Path, want)
		}
	}

	// IsGit: standard repo
	if !byName["project-a"].IsGit {
		t.Error("project-a should be detected as git repo (standard .git dir)")
	}

	// IsGit: worktree
	if !byName["project-c"].IsGit {
		t.Error("project-c should be detected as git repo (worktree .git file)")
	}

	// IsGit: bare repo
	if !byName["project-d"].IsGit {
		t.Error("project-d should be detected as git repo (bare .bare/HEAD)")
	}

	// not a git repo
	if byName["project-b"].IsGit {
		t.Error("project-b should not be a git repo")
	}

	// HasChildren: true when a visible subdir exists (pruned siblings don't count)
	if !byName["project-b"].HasChildren {
		t.Error("project-b should have children (src/ is visible)")
	}

	// HasChildren: false for a leaf with no subdirs
	if byName["project-e"].HasChildren {
		t.Error("project-e should not have children")
	}

	// ancestor .gitignore: listing subdir should hide nested-ignored
	subdirEntries, err := List(filepath.Join(root, "subdir"), root)
	if err != nil {
		t.Fatal(err)
	}
	subdirByName := make(map[string]DirEntry)
	for _, e := range subdirEntries {
		subdirByName[e.Name] = e
	}
	if _, ok := subdirByName["nested-ignored"]; ok {
		t.Error("nested-ignored should be excluded by subdir/.gitignore")
	}
	if _, ok := subdirByName["visible"]; !ok {
		t.Error("visible should appear in subdir listing")
	}
}

func TestIsGitRepo(t *testing.T) {
	root, _ := os.MkdirTemp("", "gitrepo*")
	defer os.RemoveAll(root)

	t.Run("standard repo", func(t *testing.T) {
		dir := filepath.Join(root, "standard")
		os.MkdirAll(filepath.Join(dir, ".git"), 0755)
		if !isGitRepo(dir) {
			t.Error("expected true for standard .git dir")
		}
	})
	t.Run("worktree", func(t *testing.T) {
		dir := filepath.Join(root, "worktree")
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../.bare/worktrees/main"), 0644)
		if !isGitRepo(dir) {
			t.Error("expected true for .git file (worktree)")
		}
	})
	t.Run("bare repo", func(t *testing.T) {
		dir := filepath.Join(root, "bare")
		os.MkdirAll(filepath.Join(dir, ".bare"), 0755)
		os.WriteFile(filepath.Join(dir, ".bare", "HEAD"), []byte("ref: refs/heads/main"), 0644)
		if !isGitRepo(dir) {
			t.Error("expected true for .bare/HEAD (bare repo)")
		}
	})
	t.Run("not a repo", func(t *testing.T) {
		dir := filepath.Join(root, "plain")
		os.MkdirAll(dir, 0755)
		if isGitRepo(dir) {
			t.Error("expected false for plain dir")
		}
	})
}
