package session

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// OCRunner starts and stops OpenCode processes.
type OCRunner struct {
	Binary     string
	Flags      []string
	CORSOrigin string
}

func (r *OCRunner) Start(dir string, port int) (int, error) {
	args := append([]string{}, r.Flags...)
	args = append(args, "--port", strconv.Itoa(port))
	if r.CORSOrigin != "" {
		args = append(args, "--cors", r.CORSOrigin)
	}

	cmd := exec.Command(r.Binary, args...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting opencode: %w", err)
	}

	return cmd.Process.Pid, nil
}

func (r *OCRunner) Stop(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil // already gone
	}

	return proc.Kill()
}

func (r *OCRunner) HealthURL(port int) string {
	return fmt.Sprintf("http://localhost:%d", port)
}
