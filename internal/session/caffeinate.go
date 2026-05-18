package session

import (
	"fmt"
	"log"
	"os/exec"
)

// attachCaffeinate starts a best-effort "caffeinate -s -w <pid>" helper that
// holds a system sleep assertion for the lifetime of the given process.
//
// Failure is non-fatal: if caffeinate is unavailable or fails to start, a
// warning is logged and nil is returned so callers can continue normally.
// The returned *exec.Cmd is already started; callers do not need to call Start
// on it. The helper process exits automatically when the watched PID exits.
func attachCaffeinate(pid int) *exec.Cmd {
	cmd := exec.Command("caffeinate", "-s", "-w", fmt.Sprintf("%d", pid))
	if err := cmd.Start(); err != nil {
		log.Printf("caffeinate helper for pid %d: failed to start (non-fatal): %v", pid, err)
		return nil
	}
	return cmd
}
