package tailscale_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"workspace-portal/internal/tailscale"
)

// withFakeBinary prepends the testdata directory to PATH so that
// exec.Command("tailscale", ...) resolves to the fake shell script
// instead of a real Tailscale installation.
func withFakeBinary(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(file), "testdata")
	orig := os.Getenv("PATH")
	os.Setenv("PATH", testdata+":"+orig)
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	return "tailscale"
}

func TestServeRegister(t *testing.T) {
	binary := withFakeBinary(t)
	s := &tailscale.Serve{Binary: binary}

	url, err := s.Register(4101)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if url != "" {
		t.Errorf("expected empty URL, got %q", url)
	}
}

func TestServeDeregister(t *testing.T) {
	binary := withFakeBinary(t)
	s := &tailscale.Serve{Binary: binary}

	if err := s.Deregister(4101); err != nil {
		t.Fatalf("Deregister returned error: %v", err)
	}
}
