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
	if cfg.Docs.Binary != "npx" {
		t.Errorf("expected default Docs binary, got %q", cfg.Docs.Binary)
	}
	if cfg.Docs.Package == "" {
		t.Error("expected default Docs package to be set")
	}
	if cfg.Docs.HealthStartupTimeout != 120 {
		t.Errorf("expected default docs health timeout 120, got %d", cfg.Docs.HealthStartupTimeout)
	}
}

func TestEnvOverride(t *testing.T) {
	f, _ := os.CreateTemp("", "config*.yaml")
	f.WriteString("workspaces_root: /tmp/workspaces\n")
	f.Close()
	defer os.Remove(f.Name())

	t.Setenv("PORTAL_PORT", "5555")
	t.Setenv("PORTAL_DOCS_PACKAGE", "fea-docs@latest")

	cfg, _ := Load(f.Name())
	if cfg.PortalPort != 5555 {
		t.Errorf("env override failed, got %d", cfg.PortalPort)
	}
	if cfg.Docs.Package != "fea-docs@latest" {
		t.Errorf("docs package env override failed, got %q", cfg.Docs.Package)
	}
}

func TestDocsPackageAllowsLatest(t *testing.T) {
	f, _ := os.CreateTemp("", "config*.yaml")
	f.WriteString("workspaces_root: /tmp/workspaces\ndocs:\n  package: fea-docs@latest\n")
	f.Close()
	defer os.Remove(f.Name())

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error for docs.package using @latest: %v", err)
	}
	if cfg.Docs.Package != "fea-docs@latest" {
		t.Fatalf("docs package mismatch: got %q", cfg.Docs.Package)
	}
}

func TestDocsHealthStartupTimeoutMustBePositive(t *testing.T) {
	f, _ := os.CreateTemp("", "config*.yaml")
	f.WriteString("workspaces_root: /tmp/workspaces\ndocs:\n  health_startup_timeout: 0\n")
	f.Close()
	defer os.Remove(f.Name())

	_, err := Load(f.Name())
	if err == nil {
		t.Fatal("expected error for docs.health_startup_timeout <= 0")
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
