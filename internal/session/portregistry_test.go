package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"workspace-portal/internal/portrange"
)

func TestRegistryKey_Format(t *testing.T) {
	key := registryKey("opencode", "/my/proj")
	if key != "opencode:/my/proj" {
		t.Errorf("got %q, want %q", key, "opencode:/my/proj")
	}
}

func TestPortRegistry_LoadMissingFile(t *testing.T) {
	r, err := LoadPortRegistry(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(r.entries) != 0 {
		t.Errorf("expected empty registry, got %d entries", len(r.entries))
	}
}

func TestPortRegistry_LoadCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := LoadPortRegistry(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(r.entries) != 0 {
		t.Errorf("expected empty registry after corrupt file, got %d entries", len(r.entries))
	}
}

func TestPortRegistry_SaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "port-registry.json")

	r := NewPortRegistry(path)
	r.Set("opencode:/my/proj", &PortEntry{
		Dir:         "/my/proj",
		Type:        SessionTypeOpenCode,
		Port:        5101,
		LastStarted: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	r.Set("docs:/other", &PortEntry{
		Dir:         "/other",
		Type:        SessionTypeDocs,
		Port:        5102,
		LastStarted: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	})

	if err := r.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadPortRegistry(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	e1, ok := loaded.Get("opencode:/my/proj")
	if !ok {
		t.Fatal("missing entry opencode:/my/proj")
	}
	if e1.Port != 5101 || e1.Dir != "/my/proj" || e1.Type != SessionTypeOpenCode {
		t.Errorf("unexpected entry: %+v", e1)
	}

	e2, ok := loaded.Get("docs:/other")
	if !ok {
		t.Fatal("missing entry docs:/other")
	}
	if e2.Port != 5102 {
		t.Errorf("unexpected port: %d", e2.Port)
	}
}

func TestPortRegistry_GetSetDelete(t *testing.T) {
	r := NewPortRegistry(filepath.Join(t.TempDir(), "test.json"))

	// Get on empty
	_, ok := r.Get("missing")
	if ok {
		t.Error("expected false for missing key")
	}

	// Set and Get
	r.Set("opencode:/my/dir", &PortEntry{
		Dir:  "/my/dir",
		Type: SessionTypeOpenCode,
		Port: 5101,
	})
	e, ok := r.Get("opencode:/my/dir")
	if !ok {
		t.Fatal("expected entry after Set")
	}
	if e.Port != 5101 {
		t.Errorf("got port %d, want 5101", e.Port)
	}

	// Delete and Get
	r.Delete("opencode:/my/dir")
	_, ok = r.Get("opencode:/my/dir")
	if ok {
		t.Error("expected false after Delete")
	}
}

func TestPortRegistry_PurgeStale(t *testing.T) {
	r := NewPortRegistry(filepath.Join(t.TempDir(), "test.json"))
	cutoff := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	rng := portrange.PortRange{5000, 5099}

	// Entry within range and older than cutoff → stale
	r.Set("stale", &PortEntry{
		Port:        5050,
		LastStarted: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	// Entry within range and newer than cutoff → not stale
	r.Set("fresh", &PortEntry{
		Port:        5060,
		LastStarted: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	// Entry outside range and older than cutoff → not stale (different range)
	r.Set("outside", &PortEntry{
		Port:        6000,
		LastStarted: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	count := r.PurgeStale(rng, cutoff)
	if count != 1 {
		t.Errorf("expected 1 stale entry purged, got %d", count)
	}

	if _, ok := r.Get("stale"); ok {
		t.Error("stale entry should have been purged")
	}
	if _, ok := r.Get("fresh"); !ok {
		t.Error("fresh entry should still exist")
	}
	if _, ok := r.Get("outside"); !ok {
		t.Error("outside entry should still exist")
	}
}

func TestPortRegistry_PurgeOutOfRange(t *testing.T) {
	r := NewPortRegistry(filepath.Join(t.TempDir(), "test.json"))
	rng := portrange.PortRange{5000, 5099}

	r.Set("above", &PortEntry{Type: SessionTypeOpenCode, Port: 6000})
	r.Set("below", &PortEntry{Type: SessionTypeOpenCode, Port: 4000})
	r.Set("within", &PortEntry{Type: SessionTypeOpenCode, Port: 5050})

	count := r.PurgeOutOfRange(rng, SessionTypeOpenCode)
	if count != 2 {
		t.Errorf("expected 2 out-of-range entries purged, got %d", count)
	}

	if _, ok := r.Get("above"); ok {
		t.Error("above-range entry should have been purged")
	}
	if _, ok := r.Get("below"); ok {
		t.Error("below-range entry should have been purged")
	}
	if _, ok := r.Get("within"); !ok {
		t.Error("within-range entry should still exist")
	}
}

func TestPortRegistry_PurgeOutOfRange_OnlyPurgesMatchingType(t *testing.T) {
	r := NewPortRegistry(filepath.Join(t.TempDir(), "test.json"))

	r.Set("opencode:/a", &PortEntry{
		Dir: "/a", Type: SessionTypeOpenCode, Port: 9100,
	})
	r.Set("vscode:/b", &PortEntry{
		Dir: "/b", Type: SessionTypeVSCode, Port: 9200,
	})

	// Purging out-of-range for docs must NOT touch opencode or vscode entries
	count := r.PurgeOutOfRange(portrange.PortRange{9300, 9399}, SessionTypeDocs)
	if count != 0 {
		t.Errorf("expected 0 purged entries, got %d", count)
	}

	if _, ok := r.Get("opencode:/a"); !ok {
		t.Error("opencode entry was incorrectly purged by docs PurgeOutOfRange")
	}
	if _, ok := r.Get("vscode:/b"); !ok {
		t.Error("vscode entry was incorrectly purged by docs PurgeOutOfRange")
	}
}

func TestPortRegistry_ConcurrentAccess(t *testing.T) {
	r := NewPortRegistry(filepath.Join(t.TempDir(), "test.json"))

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := registryKey(SessionTypeOpenCode, "/dir")
			r.Set(key, &PortEntry{
				Dir:         "/dir",
				Type:        SessionTypeOpenCode,
				Port:        5000 + i,
				LastStarted: time.Now(),
			})
			r.Get(key)
			r.PurgeStale(portrange.PortRange{5000, 5099}, time.Now())
			r.PurgeOutOfRange(portrange.PortRange{5000, 5099}, SessionTypeOpenCode)
			r.Delete("other-" + string(rune(i)))
		}(i)
	}
	wg.Wait()
}
