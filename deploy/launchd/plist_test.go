package launchd_test

import (
	"os"
	"strings"
	"testing"
)

// TestPlistTemplate_ContainsCaffeinate verifies that the generated launchd
// service definition includes the caffeinate invocation as the first program
// argument, satisfying the sleep-resilience requirement (user story 1, 5, 6).
func TestPlistTemplate_ContainsCaffeinate(t *testing.T) {
	data, err := os.ReadFile("com.workspace-portal.plist.tmpl")
	if err != nil {
		t.Fatalf("reading plist template: %v", err)
	}

	content := string(data)

	if !strings.Contains(content, "/usr/bin/caffeinate") {
		t.Error("plist template does not contain /usr/bin/caffeinate")
	}

	if !strings.Contains(content, "<string>-s</string>") {
		t.Error("plist template does not include the -s flag for caffeinate")
	}

	// Verify argument ordering: caffeinate must appear before PORTAL_BINARY.
	cafIdx := strings.Index(content, "/usr/bin/caffeinate")
	binaryIdx := strings.Index(content, "PORTAL_BINARY")
	if cafIdx < 0 || binaryIdx < 0 {
		t.Fatal("plist template missing caffeinate or PORTAL_BINARY placeholder")
	}
	if cafIdx > binaryIdx {
		t.Error("caffeinate must appear before PORTAL_BINARY in ProgramArguments")
	}
}
