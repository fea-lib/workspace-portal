package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return path
}

func TestDocsFactoryStartBuildsExpectedCommand(t *testing.T) {
	binDir := t.TempDir()
	workDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args.txt")

	writeExecutable(t, binDir, "node", "#!/bin/sh\nexit 0\n")
	writeExecutable(t, binDir, "npx", "#!/bin/sh\nprintf '%s\n' \"$@\" > \"$ARGS_FILE\"\nexec sleep 30\n")

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	t.Setenv("ARGS_FILE", argsFile)

	f := &DocsSessionFactory{Binary: "npx", Package: "fea-docs@latest"}
	cmd, _, err := f.Start(workDir, 4311)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { f.Stop(cmd.Process.Pid) }) //nolint:errcheck

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(argsFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("args file not created: %s", argsFile)
		}
		time.Sleep(20 * time.Millisecond)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"--yes", "fea-docs@latest", "start", "--port", "4311", "--expose"}
	if len(got) != len(want) {
		t.Fatalf("args length mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestDocsFactoryStartMissingNodeIsActionable(t *testing.T) {
	binDir := t.TempDir()
	workDir := t.TempDir()
	writeExecutable(t, binDir, "npx", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir)

	f := &DocsSessionFactory{Binary: "npx", Package: "fea-docs@latest"}
	_, _, err := f.Start(workDir, 4321)
	if err == nil {
		t.Fatal("expected error when node is missing")
	}
	if !strings.Contains(err.Error(), "missing prerequisite 'node'") {
		t.Fatalf("expected actionable node error, got: %v", err)
	}
}

func TestDocsFactoryStartMissingNpxIsActionable(t *testing.T) {
	binDir := t.TempDir()
	workDir := t.TempDir()
	writeExecutable(t, binDir, "node", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir)

	f := &DocsSessionFactory{Binary: "npx", Package: "fea-docs@latest"}
	_, _, err := f.Start(workDir, 4321)
	if err == nil {
		t.Fatal("expected error when npx is missing")
	}
	if !strings.Contains(err.Error(), "missing prerequisite \"npx\"") {
		t.Fatalf("expected actionable npx error, got: %v", err)
	}
}

func TestDocsFactoryHealthURL(t *testing.T) {
	f := &DocsSessionFactory{}
	if got := f.HealthURL(4321); got != "http://localhost:4321" {
		t.Fatalf("health URL mismatch: %q", got)
	}
}

func TestDocsFactoryHealthStartupTimeout(t *testing.T) {
	f := &DocsSessionFactory{StartupTimeout: 120 * time.Second}
	if got := f.HealthStartupTimeout(); got != 120*time.Second {
		t.Fatalf("startup timeout mismatch: %s", got)
	}
}
