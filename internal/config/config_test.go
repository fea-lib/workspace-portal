package config

import (
	"os"
	"testing"
)

func TestDefaults(t *testing.T) {
	// Load with a non-existent file — should fail on missing workspaces_root
	_, err := Load("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for missing workspaces_root")
	}
}

func TestLoadFromFile(t *testing.T) {
	f, _ := os.CreateTemp("", "config*.yaml")
	f.WriteString("workspaces_root: /tmp/workspaces\nportal_port: 9000\n")
	f.Close()
	defer os.Remove(f.Name())

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspacesRoot != "/tmp/workspaces" {
		t.Errorf("got %q, want /tmp/workspaces", cfg.WorkspacesRoot)
	}
	if cfg.PortalPort != 9000 {
		t.Errorf("got %d, want 9000", cfg.PortalPort)
	}
	// Defaults still apply for unset fields
	if cfg.OC.Binary != "opencode" {
		t.Errorf("expected default OpenCode binary, got %q", cfg.OC.Binary)
	}
}

func TestEnvOverride(t *testing.T) {
	f, _ := os.CreateTemp("", "config*.yaml")
	f.WriteString("workspaces_root: /tmp/workspaces\n")
	f.Close()
	defer os.Remove(f.Name())

	t.Setenv("PORTAL_PORT", "5555")

	cfg, _ := Load(f.Name())
	if cfg.PortalPort != 5555 {
		t.Errorf("env override failed, got %d", cfg.PortalPort)
	}
}

func TestSecret(t *testing.T) {
	dir, _ := os.MkdirTemp("", "secrets*")
	defer os.RemoveAll(dir)
	os.WriteFile(dir+"/vscode-password", []byte("secret123\n"), 0600)

	cfg := &Config{SecretsDir: dir}
	if got := cfg.Secret("vscode-password"); got != "secret123" {
		t.Errorf("got %q, want secret123", got)
	}
}
