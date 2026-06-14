package session

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"workspace-portal/internal/portrange"
)

// PortEntry represents a single port assignment in the registry.
type PortEntry struct {
	Dir         string      `json:"dir"`
	Type        SessionType `json:"type"`
	Port        int         `json:"port"`
	LastStarted time.Time   `json:"last_started"`
}

// PortRegistry provides durable storage for port-to-session mappings.
// It uses a separate mutex from Manager to avoid deadlocks during portFree calls.
type PortRegistry struct {
	mu        sync.Mutex
	entries   map[string]*PortEntry
	stateFile string
}

// registryKey builds the composite key from session type and directory.
func registryKey(sessionType SessionType, dir string) string {
	return string(sessionType) + ":" + dir
}

// NewPortRegistry creates an empty registry with the given file path.
func NewPortRegistry(stateFile string) *PortRegistry {
	return &PortRegistry{
		entries:   make(map[string]*PortEntry),
		stateFile: stateFile,
	}
}

// LoadPortRegistry reads the registry from disk. If the file does not exist
// or is corrupt, an empty registry is returned (a warning is logged on corruption).
func LoadPortRegistry(stateFile string) (*PortRegistry, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return NewPortRegistry(stateFile), nil
		}
		return nil, err
	}

	var entries map[string]*PortEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("port registry: corrupt file %s: %v — resetting to empty", stateFile, err)
		return NewPortRegistry(stateFile), nil
	}

	return &PortRegistry{
		entries:   entries,
		stateFile: stateFile,
	}, nil
}

// Get retrieves a port entry by composite key.
func (r *PortRegistry) Get(key string) (*PortEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[key]
	return e, ok
}

// Set stores a port entry by composite key.
func (r *PortRegistry) Set(key string, entry *PortEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[key] = entry
}

// Delete removes a port entry by composite key.
func (r *PortRegistry) Delete(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
}

// Save persists the registry to disk as JSON.
func (r *PortRegistry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(r.stateFile), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(r.entries)
	if err != nil {
		return err
	}
	return os.WriteFile(r.stateFile, data, 0644)
}

// PurgeStale removes entries whose port falls within rng and whose LastStarted
// is before the cutoff time. Returns the number of purged entries.
func (r *PortRegistry) PurgeStale(rng portrange.PortRange, cutoff time.Time) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for k, e := range r.entries {
		if e.Port >= rng[0] && e.Port <= rng[1] && e.LastStarted.Before(cutoff) {
			delete(r.entries, k)
			count++
		}
	}
	if count > 0 {
		log.Printf("port registry: purged %d stale entries in range %d-%d", count, rng[0], rng[1])
	}
	return count
}

// PurgeOutOfRange removes entries whose port falls outside rng.
// Returns the number of purged entries.
func (r *PortRegistry) PurgeOutOfRange(rng portrange.PortRange, sessionType SessionType) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for k, e := range r.entries {
		if e.Type == sessionType && (e.Port < rng[0] || e.Port > rng[1]) {
			delete(r.entries, k)
			count++
		}
	}
	if count > 0 {
		log.Printf("port registry: purged %d out-of-range entries of type %q (range %d-%d)", count, string(sessionType), rng[0], rng[1])
	}
	return count
}
