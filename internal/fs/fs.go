package fs

import (
	"os"
	"path/filepath"
	"sort"

	gitignore "github.com/sabhiram/go-gitignore"
)

// DirEntry describes one directory in the tree.
type DirEntry struct {
	Path        string // absolute path to this entry (includes Name as the final element)
	Name        string // basename (final element of Path)
	IsGit       bool   // contains a git repo
	HasChildren bool   // has non-pruned subdirectories
}

// defaultPrune is the set of directory names that are never shown.
var defaultPrune = map[string]bool{
	"node_modules": true, ".pnpm": true, ".pnpm-store": true,
	"dist": true, ".next": true, ".nuxt": true, ".turbo": true,
	"build": true, "out": true, "coverage": true,
	".cache": true, "__pycache__": true, ".pytest_cache": true,
	"target": true, "vendor": true, ".gradle": true,
	// git internals — prune CONTENTS of .git and .bare, not the dirs themselves
	"objects": true, "refs": true, "info": true,
	"hooks": true, "worktrees": true, "pack": true,
	"logs": true, "temp": true, "tmp": true,
}

// List returns the immediate visible subdirectories of path.
// It respects .gitignore files found in path and its ancestors up to root.
// root is the workspaces root — gitignore walk stops there.
func List(path, root string) ([]DirEntry, error) {
	matchers := loadGitignores(path, root)

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var result []DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if defaultPrune[e.Name()] {
			continue
		}
		absPath := filepath.Join(path, e.Name())
		if gitignored(absPath, root, matchers) {
			continue
		}
		entry := DirEntry{
			Path:  absPath,
			Name:  e.Name(),
			IsGit: isGitRepo(absPath),
		}
		entry.HasChildren = hasVisibleSubdirs(absPath, root, matchers)
		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// gitignored returns true if absPath is matched by any of the compiled matchers.
func gitignored(absPath, root string, matchers []*gitignore.GitIgnore) bool {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return false
	}

	for _, m := range matchers {
		if m.MatchesPath(rel) {
			return true
		}
	}

	return false
}

// loadGitignores walks from root up to path, collecting all .gitignore files,
// and returns a compiled matcher. Returns nil if no .gitignore files are found.
func loadGitignores(path, root string) []*gitignore.GitIgnore {
	// Collect .gitignore file paths from root down to path.
	var files []string
	current := path

	for {
		candidate := filepath.Join(current, ".gitignore")
		if _, err := os.Stat(candidate); err == nil {
			files = append([]string{candidate}, files...) // prepend so root wins
		}

		if current == root {
			break
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}

		current = parent
	}

	var matchers []*gitignore.GitIgnore
	for _, f := range files {
		ig, err := gitignore.CompileIgnoreFile(f)
		if err == nil {
			matchers = append(matchers, ig)
		}
	}
	return matchers
}

// isGitRepo returns true if the directory contains a git repository.
// Handles three layouts:
//   - standard:  .git/ is a directory
//   - worktree:  .git is a file
//   - bare repo: .bare/HEAD exists
func isGitRepo(path string) bool {
	if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
		return true
	}
	if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.Mode().IsRegular() {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, ".bare", "HEAD")); err == nil {
		return true
	}
	return false
}

// hasVisibleSubdirs returns true if path contains at least one subdirectory
// that is neither in the default prune set nor excluded by .gitignore rules.
// Files are not considered.
func hasVisibleSubdirs(path, root string, matchers []*gitignore.GitIgnore) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	for _, e := range entries {
		if !e.IsDir() || defaultPrune[e.Name()] {
			continue
		}
		if gitignored(filepath.Join(path, e.Name()), root, matchers) {
			continue
		}
		return true
	}
	return false
}
