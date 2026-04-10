package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v11"
	"gopkg.in/yaml.v3"
)

// Config holds all portal configuration.
type Config struct {
	WorkspacesRoot string    `yaml:"workspaces_root" env:"PORTAL_WORKSPACES_ROOT"`
	PortalPort     int       `yaml:"portal_port"      env:"PORTAL_PORT"`
	SecretsDir     string    `yaml:"secrets_dir"`
	OC             OCConfig  `yaml:"oc"               envPrefix:"PORTAL_OC_"`
	VSCode         VSCConfig `yaml:"vscode"           envPrefix:"PORTAL_VSCODE_"`
	FS             FSConfig  `yaml:"fs"`
}

// PortRange is a [lo, hi] port pair that unmarshals from "lo-hi" strings in both
// YAML and environment variables (e.g. "4100-4199"). Implementing
// encoding.TextUnmarshaler means both yaml.v3 and caarlos0/env pick it up
// automatically — no custom parsing code needed at the call site.
type PortRange [2]int

func (p *PortRange) UnmarshalText(text []byte) error {
	parts := strings.SplitN(string(text), "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("port range must be in lo-hi format, got %q", string(text))
	}
	lo, err1 := strconv.Atoi(parts[0])
	hi, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return fmt.Errorf("invalid port range %q", string(text))
	}
	*p = PortRange{lo, hi}
	return nil
}

type OCConfig struct {
	Binary    string    `yaml:"binary"     env:"BINARY"`
	PortRange PortRange `yaml:"port_range" env:"PORT_RANGE"`
	Flags     []string  `yaml:"flags"      env:"FLAGS"`
}

type VSCConfig struct {
	Binary    string    `yaml:"binary"     env:"BINARY"`
	PortRange PortRange `yaml:"port_range" env:"PORT_RANGE"`
}

type FSConfig struct {
	PruneDirs []string `yaml:"prune_dirs"`
}

// defaults returns a Config populated with sensible defaults.
func defaults() *Config {
	return &Config{
		PortalPort: 4000,
		SecretsDir: ".secrets",
		OC: OCConfig{
			Binary:    "opencode",
			PortRange: PortRange{4100, 4199},
			Flags:     []string{"web", "--mdns"},
		},
		VSCode: VSCConfig{
			Binary:    "code-server",
			PortRange: PortRange{4200, 4299},
		},
	}
}

// Load reads config from the given YAML file path, applies env var overrides,
// and validates required fields.
func Load(path string) (*Config, error) {
	cfg := defaults()

	// Read and parse YAML if file exists
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file %s: %w", path, err)
		}
	}

	// Apply env var overrides for all tagged fields automatically.
	// env.Parse only writes a field when its env var is actually present —
	// it never touches fields whose env var is unset, so YAML-loaded and
	// defaults()-populated values are preserved unless explicitly overridden.
	// PortRange fields are covered automatically because PortRange implements
	// encoding.TextUnmarshaler — env parses "4100-4199" into PortRange{4100,4199}.
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("applying env overrides: %w", err)
	}

	// Expand ~ in workspaces root
	if strings.HasPrefix(cfg.WorkspacesRoot, "~/") {
		home, _ := os.UserHomeDir()
		cfg.WorkspacesRoot = filepath.Join(home, cfg.WorkspacesRoot[2:])
	}

	// Resolve secrets dir relative to config file location
	if !filepath.IsAbs(cfg.SecretsDir) {
		cfg.SecretsDir = filepath.Join(filepath.Dir(path), cfg.SecretsDir)
	}

	// Validate required fields
	if cfg.WorkspacesRoot == "" {
		return nil, fmt.Errorf("workspaces_root is required (set in config.yaml or PORTAL_WORKSPACES_ROOT)")
	}

	return cfg, nil
}

// Secret reads a secret value by name. Resolution order:
//  1. Environment variable PORTAL_{UPPER_NAME} (e.g. PORTAL_VSCODE_PASSWORD)
//  2. File at cfg.SecretsDir/{name}
//  3. File at /run/secrets/{name}  (Docker secrets convention)
func (cfg *Config) Secret(name string) string {
	// 1. Env var
	envKey := "PORTAL_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	// 2. secrets dir
	if v, err := os.ReadFile(filepath.Join(cfg.SecretsDir, name)); err == nil {
		return strings.TrimSpace(string(v))
	}
	// 3. Docker secrets
	if v, err := os.ReadFile(filepath.Join("/run/secrets", name)); err == nil {
		return strings.TrimSpace(string(v))
	}
	log.Printf("warning: secret %q not found (checked env var %s, %s, /run/secrets/%s)", name, envKey, filepath.Join(cfg.SecretsDir, name), name)
	return ""
}
