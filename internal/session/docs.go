package session

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// DocsSessionFactory is a configured factory for fea-docs sessions.
type DocsSessionFactory struct {
	Binary         string
	Package        string
	StartupTimeout time.Duration
}

func (r *DocsSessionFactory) Start(dir string, port int) (*exec.Cmd, error) {
	if _, err := exec.LookPath("node"); err != nil {
		return nil, fmt.Errorf("starting docs session: missing prerequisite 'node' (install Node.js 18+ and ensure it is on PATH)")
	}
	if _, err := exec.LookPath(r.Binary); err != nil {
		return nil, fmt.Errorf("starting docs session: missing prerequisite %q (install Node.js/npm and ensure %s is on PATH)", r.Binary, r.Binary)
	}

	args := []string{"--yes", r.Package, "start", "--port", strconv.Itoa(port), "--expose"}
	cmd := exec.Command(r.Binary, args...)
	cmd.Dir = dir
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting docs session: %w", err)
	}

	attachCaffeinate(cmd.Process.Pid)

	return cmd, nil
}

func (r *DocsSessionFactory) Stop(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}

	return proc.Kill()
}

func (r *DocsSessionFactory) HealthURL(port int) string {
	return fmt.Sprintf("http://localhost:%d", port)
}

func (r *DocsSessionFactory) HealthStartupTimeout() time.Duration {
	return r.StartupTimeout
}
