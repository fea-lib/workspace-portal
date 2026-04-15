package session

import (
	"fmt"
	"os"
	"os/exec"
)

// VSCodeSessionFactory is a configured factory for VS Code (code-server) sessions.
type VSCodeSessionFactory struct {
	Binary   string
	Password string
}

func (r *VSCodeSessionFactory) Start(dir string, port int) (int, error) {
	cmd := exec.Command(r.Binary,
		"--bind-addr", fmt.Sprintf("127.0.0.1:%d", port),
		"--auth", "password",
		dir,
	)

	cmd.Env = append(os.Environ(), "PASSWORD="+r.Password)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting code-server: %w", err)
	}

	return cmd.Process.Pid, nil
}

func (r *VSCodeSessionFactory) Stop(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}

	return proc.Kill()
}

func (r *VSCodeSessionFactory) HealthURL(port int) string {
	return fmt.Sprintf("http://localhost:%d", port)
}
