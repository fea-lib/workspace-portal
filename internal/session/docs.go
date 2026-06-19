package session

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DocsSessionFactory is a configured factory for fea-docs sessions.
type DocsSessionFactory struct {
	Binary         string
	Package        string
	StartupTimeout time.Duration
}

func (r *DocsSessionFactory) Start(dir string, port int) (*exec.Cmd, int, error) {
	if _, err := exec.LookPath("node"); err != nil {
		return nil, 0, fmt.Errorf("starting docs session: missing prerequisite 'node' (install Node.js 18+ and ensure it is on PATH)")
	}
	if _, err := exec.LookPath(r.Binary); err != nil {
		return nil, 0, fmt.Errorf("starting docs session: missing prerequisite %q (install Node.js/npm and ensure %s is on PATH)", r.Binary, r.Binary)
	}

	args := []string{"--yes", r.Package, "start", "--port", strconv.Itoa(port), "--expose"}
	cmd := exec.Command(r.Binary, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = log.Writer()

	actualPort := port
	portCh := make(chan int, 1)
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			log.Print(line)
			if p := parseDocsPortLine(line); p > 0 {
				select {
				case portCh <- p:
				default:
				}
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return nil, 0, fmt.Errorf("starting docs session: %w", err)
	}

	select {
	case actualPort = <-portCh:
	case <-time.After(10 * time.Second):
		log.Printf("warning: docs session port not discovered in time, using assigned %d", port)
	}

	attachCaffeinate(cmd.Process.Pid)

	return cmd, actualPort, nil
}

func (r *DocsSessionFactory) Stop(pid int) error {
	syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(3 * time.Second)
	syscall.Kill(-pid, syscall.SIGKILL) //nolint:errcheck
	return nil
}

func parseDocsPortLine(line string) int {
	const prefix = "##FEA_DOCS_PORT="
	if idx := strings.Index(line, prefix); idx >= 0 {
		rest := line[idx+len(prefix):]
		end := strings.Index(rest, "##")
		if end > 0 {
			if p, err := strconv.Atoi(rest[:end]); err == nil {
				return p
			}
		}
	}
	const hostPrefix = "localhost:"
	if idx := strings.Index(line, hostPrefix); idx >= 0 {
		rest := line[idx+len(hostPrefix):]
		var portStr string
		for _, ch := range rest {
			if ch >= '0' && ch <= '9' {
				portStr += string(ch)
			} else {
				break
			}
		}
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			return p
		}
	}
	return 0
}

func (r *DocsSessionFactory) HealthURL(port int) string {
	return fmt.Sprintf("http://localhost:%d", port)
}

func (r *DocsSessionFactory) HealthStartupTimeout() time.Duration {
	return r.StartupTimeout
}
