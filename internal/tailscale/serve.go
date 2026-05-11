package tailscale

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Serve implements session.Registrar using the tailscale CLI.
type Serve struct {
	Binary string // path to the tailscale binary, e.g. "tailscale" or "/usr/local/bin/tailscale"
}

// Register runs: tailscale serve --bg --https={port} http://localhost:{port}
// The returned URL is empty — the caller constructs it from the machine's FQDN.
func (s *Serve) Register(port int) (string, error) {
	p := strconv.Itoa(port)
	cmd := exec.Command(s.Binary,
		"serve", "--bg", "--https"+p,
		"http://localhost:"+p,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tailscale serve: %w\n%s", err, out)
	}

	// URL construction is the caller's responsibility — it knows the machine FQDN.
	return "", nil
}

// Deregister removes the serve config for the given port.
// Uses best-effort: if the port was already deregistered, this is a no-op.
func (s *Serve) Deregister(port int) error {
	p := strconv.Itoa(port)
	cmd := exec.Command(s.Binary, "serve", "--https"+p, "off")
	cmd.Run()
	return nil
}

// FQDN returns the machine's Tailscale MagicDNS name (e.g. "dev-mac.tail1234.ts.net").
// Returns empty string if the tailscale binary cannot be called or the machine
// is not connected.
func (s *Serve) FDQN() string {
	out, err := exec.Command(s.Binary, "status", "--json").Output()
	if err != nil {
		return ""
	}

	var status struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return ""
	}

	// DNSName has a trailing dot — trim it.
	return strings.TrimSuffix(status.Self.DNSName, ".")
}
