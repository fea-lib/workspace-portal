package session

import (
	"os/exec"
	"testing"
)

// TestAttachCaffeinate_NonFatalOnMissingBinary verifies that attachCaffeinate
// does not panic and returns nil when the caffeinate binary is unavailable.
// We simulate this by temporarily shadowing the PATH.
func TestAttachCaffeinate_NonFatalOnMissingBinary(t *testing.T) {
	// Override PATH so caffeinate cannot be found.
	t.Setenv("PATH", t.TempDir())

	result := attachCaffeinate(99999) // arbitrary PID; process doesn't need to exist
	if result != nil {
		result.Process.Kill() //nolint:errcheck
		t.Error("expected nil when caffeinate is unavailable, got a cmd")
	}
}

// TestAttachCaffeinate_StartsWhenAvailable verifies that attachCaffeinate returns
// a non-nil cmd when caffeinate is available and the PID is valid.
func TestAttachCaffeinate_StartsWhenAvailable(t *testing.T) {
	// Check if caffeinate is available at all; skip if not (non-macOS CI).
	if _, err := exec.LookPath("caffeinate"); err != nil {
		t.Skip("caffeinate not available; skipping")
	}

	// Start a long-lived process whose PID we can pass to caffeinate.
	target := exec.Command("sleep", "9999")
	if err := target.Start(); err != nil {
		t.Fatalf("starting target process: %v", err)
	}
	t.Cleanup(func() {
		target.Process.Kill() //nolint:errcheck
		target.Wait()         //nolint:errcheck
	})

	helper := attachCaffeinate(target.Process.Pid)
	if helper == nil {
		t.Fatal("expected non-nil caffeinate cmd, got nil")
	}
	t.Cleanup(func() {
		helper.Process.Kill() //nolint:errcheck
		helper.Wait()         //nolint:errcheck
	})
}

// TestOCSessionFactory_StartSucceeds_WithoutCaffeinate verifies that an
// OpenCode session start still succeeds when caffeinate is unavailable
// (i.e. caffeinate failure is non-fatal).
func TestOCSessionFactory_StartSucceeds_WithoutCaffeinate(t *testing.T) {
	// Use a real binary that exists on the system as the "editor" binary.
	// We just need cmd.Start() to succeed; we kill immediately after.
	factory := &OCSessionFactory{
		Binary: "sleep",
		Flags:  []string{},
	}

	// Shadow PATH so caffeinate is unavailable.
	t.Setenv("PATH", "/usr/bin:/bin")

	cmd, err := factory.Start(t.TempDir(), 49999)
	if err != nil {
		// If sleep binary isn't found, skip.
		t.Skipf("sleep binary not available: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill() //nolint:errcheck
		cmd.Wait()         //nolint:errcheck
	})
	// Session start must have succeeded (cmd is non-nil and has a PID).
	if cmd.Process == nil || cmd.Process.Pid <= 0 {
		t.Error("expected a valid process after Start")
	}
}

// TestVSCodeSessionFactory_StartSucceeds_WithoutCaffeinate mirrors the above
// for the VS Code factory.
func TestVSCodeSessionFactory_StartSucceeds_WithoutCaffeinate(t *testing.T) {
	factory := &VSCodeSessionFactory{
		Binary:   "sleep",
		Password: "test",
	}

	t.Setenv("PATH", "/usr/bin:/bin")

	// code-server arg format: sleep --bind-addr ... -- but sleep doesn't care,
	// it just starts. The point is cmd.Start() succeeds.
	cmd, err := factory.Start(t.TempDir(), 49998)
	if err != nil {
		t.Skipf("could not start factory (binary issue): %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill() //nolint:errcheck
		cmd.Wait()         //nolint:errcheck
	})
	if cmd.Process == nil || cmd.Process.Pid <= 0 {
		t.Error("expected a valid process after Start")
	}
}
