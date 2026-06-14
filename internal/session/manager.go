package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"workspace-portal/internal/portrange"
)

const defaultHealthTimeout = 30 * time.Second

type healthTimeoutProvider interface {
	HealthStartupTimeout() time.Duration
}

// registeredFactory pairs a SessionType, its SessionFactory, and its port range.
// Keeping them together means the Manager stays fully abstract —
// it never needs to name a concrete type.
type registeredFactory struct {
	sessionType SessionType
	factory     SessionFactory
	portRange   portrange.PortRange
}

// Register constructs a registeredFactory. This is the only way to create one
// outside this package — the struct itself is unexported.
func Register(sessionType SessionType, factory SessionFactory, portRange portrange.PortRange) registeredFactory {
	return registeredFactory{sessionType: sessionType, factory: factory, portRange: portRange}
}

// ManagerInterface defines the methods the HTTP handlers need.
// The concrete Manager implements this interface.
type ManagerInterface interface {
	Start(sessionType SessionType, dir string) (*Session, error)
	Stop(id string) error
	List() []*Session
	Get(id string) (*Session, bool)
	Events() <-chan Event
}

// Manager manages the lifecycle of all running sessions.
type Manager struct {
	mu        sync.Mutex
	factories map[SessionType]registeredFactory
	sessions  map[string]*Session
	cmds      map[string]*exec.Cmd // in-memory only; not persisted
	stateFile string
	events    chan Event
	registrar Registrar // nil-safe: always set, NoopRegistrar when disabled
	tsFQDN    string    // empty when Tailscale is disabled
	registry  *PortRegistry
}

// EventType identifies the lifecycle event emitted on the SSE channel.
// Using a named string type (rather than bare string) makes the compiler
// reject accidental string literals wherever an EventType is expected.
type EventType string

const (
	EventTypeStarted EventType = "started"
	EventTypeHealthy EventType = "healthy"
	EventTypeStopped EventType = "stopped"
)

// Event is sent on the SSE channel when session state changes.
type Event struct {
	Type    EventType
	Session *Session
}

// NewManager creates a Manager, loads persisted state, and removes orphans.
// Each factory is registered via Register() and passed as a variadic argument,
// keeping the unexported registeredFactory type out of the caller's namespace.
func NewManager(stateFile string, registrar Registrar, tsFQDN string, registrations ...registeredFactory) *Manager {
	factories := make(map[SessionType]registeredFactory, len(registrations))
	for _, r := range registrations {
		factories[r.sessionType] = r
	}

	m := &Manager{
		sessions:  make(map[string]*Session),
		cmds:      make(map[string]*exec.Cmd),
		stateFile: stateFile,
		events:    make(chan Event, 64),
		factories: factories,
		registrar: registrar,
		tsFQDN:    tsFQDN,
	}
	m.loadState()

	registryPath := filepath.Join(filepath.Dir(stateFile), "port-registry.json")
	m.registry = NewPortRegistry(registryPath)
	if loaded, err := LoadPortRegistry(registryPath); err == nil {
		m.registry = loaded
	}

	// Reconcile: ensure all restored sessions have a registry entry.
	// Guards against divergence between sessions.json and port-registry.json
	// after crashes or config changes that purged entries of other types.
	for _, s := range m.sessions {
		key := registryKey(s.Type, s.Dir)
		if _, ok := m.registry.Get(key); !ok {
			m.registry.Set(key, &PortEntry{
				Dir:         s.Dir,
				Type:        s.Type,
				Port:        s.Port,
				LastStarted: s.StartedAt,
			})
		}
	}
	m.registry.Save()

	return m
}

// Events returns the channel for SSE subscribers.
func (m *Manager) Events() <-chan Event {
	return m.events
}

// Get returns the session with the given ID, or false if not found.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns all current sessions.
func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}

	return out
}

// Start launches a new session for the given directory and type.
func (m *Manager) Start(sessionType SessionType, dir string) (*Session, error) {
	reg, ok := m.factories[sessionType]
	if !ok {
		return nil, fmt.Errorf("unknown session type: %s", sessionType)
	}

	// Return existing session if one is already running for this dir+type
	if existing := m.findByDirAndType(dir, sessionType); existing != nil {
		// Update last_started in registry for idempotent returns (AC 8)
		key := registryKey(sessionType, dir)
		m.registry.Set(key, &PortEntry{
			Dir:         dir,
			Type:        sessionType,
			Port:        existing.Port,
			LastStarted: time.Now(),
		})
		m.registry.Save()
		return existing, nil
	}

	// --- registry lookup ---
	m.registry.PurgeStale(reg.portRange, time.Now().Add(-14*24*time.Hour))
	m.registry.PurgeOutOfRange(reg.portRange, sessionType)

	key := registryKey(sessionType, dir)
	var port int
	if entry, ok := m.registry.Get(key); ok && entry.Port >= reg.portRange[0] && entry.Port <= reg.portRange[1] && portFree(entry.Port) {
		port = entry.Port
	} else {
		allocated, err := m.nextPort(reg.portRange)
		if err != nil {
			return nil, err
		}
		port = allocated
	}

	cmd, err := reg.factory.Start(dir, port)
	if err != nil {
		return nil, err
	}

	m.registry.Set(key, &PortEntry{
		Dir:         dir,
		Type:        sessionType,
		Port:        port,
		LastStarted: time.Now(),
	})
	m.registry.Save()
	// --- end registry lookup ---

	s := &Session{
		ID:        uuid.New().String(),
		Type:      sessionType,
		Dir:       dir,
		Port:      port,
		PID:       cmd.Process.Pid,
		StartedAt: time.Now(),
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.cmds[s.ID] = cmd
	m.mu.Unlock()
	m.saveState()

	// Emit the started event immediately so subscribers see session creation.
	m.events <- Event{Type: EventTypeStarted, Session: s}

	// Reap the process when it exits so it doesn't become a zombie and
	// so we can detect exit immediately (Signal(0) is unreliable for zombies).
	go func() {
		cmd.Wait() // blocks until process exits; reaps it
		log.Printf("session %s (pid %d) process exited", s.ID, s.PID)
		// Stop cleans up if still present (waitHealthy may have beaten us here).
		m.Stop(s.ID) //nolint:errcheck
	}()

	// Health check + tailscale registration run in a goroutine — it blocks
	// until the process responds, then updates s.URL and sends the "healthy" event.
	healthTimeout := defaultHealthTimeout
	if p, ok := reg.factory.(healthTimeoutProvider); ok {
		if t := p.HealthStartupTimeout(); t > 0 {
			healthTimeout = t
		}
	}

	go m.waitHealthy(s, reg.factory.HealthURL(port), healthTimeout)

	return s, nil
}

// Stop terminates a session by ID.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %s not found", id)
	}
	delete(m.sessions, id)
	delete(m.cmds, id)
	m.mu.Unlock()

	// In Manager.Stop(), after reg.factory.Stop(s.PID):
	m.registrar.Deregister(s.Port)

	if reg, ok := m.factories[s.Type]; ok {
		reg.factory.Stop(s.PID)
	}

	m.saveState()
	m.events <- Event{Type: EventTypeStopped, Session: s}

	return nil
}

// waitHealthy polls until the session responds, registers with tailscale,
// then marks it healthy. It times out after 30 seconds to avoid leaking
// goroutines for processes that fail to start.
func (m *Manager) waitHealthy(s *Session, healthURL string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("session %s (pid %d) did not become healthy in time — removing", s.ID, s.PID)
			m.Stop(s.ID)
			return
		case <-ticker.C:
			resp, err := http.Get(healthURL)
			if err == nil && resp.StatusCode < 500 {
				resp.Body.Close()

				// Register with tailscale now that the process is actually listening.
				url, err := m.registrar.Register(s.Port)
				if err != nil {
					log.Printf("tailscale register port %d: %v", s.Port, err)
					// Non-fatal — session is still usable at localhost
				} else if url != "" {
					m.mu.Lock()
					s.URL = url
					m.mu.Unlock()
				} else if m.tsFQDN != "" {
					// tailscale.Serve.Register returns "" — the FQDN is known at startup.
					m.mu.Lock()
					s.URL = fmt.Sprintf("https://%s:%d", m.tsFQDN, s.Port)
					m.mu.Unlock()
				}

				if s.URL == "" {
					m.mu.Lock()
					s.URL = healthURL
					m.mu.Unlock()
				}

				m.saveState()
				m.events <- Event{Type: EventTypeHealthy, Session: s}
				return
			}
		}
	}
}

// nextPort finds the first available port in the given range.
// It checks both the in-use session map (fast) and then attempts to bind
// the port on both 127.0.0.1 and 0.0.0.0 (authoritative — catches ports
// used by processes that bind on any interface, e.g. opencode with --mdns).
func (m *Manager) nextPort(r portrange.PortRange) (int, error) {
	m.mu.Lock()
	inUse := make(map[int]bool)
	for _, s := range m.sessions {
		inUse[s.Port] = true
	}
	m.mu.Unlock()

	for port := r[0]; port <= r[1]; port++ {
		if inUse[port] {
			continue
		}

		if portFree(port) {
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", r[0], r[1])
}

// portFree returns true only if the port is bindable on both 127.0.0.1 and
// 0.0.0.0. This catches processes that listen on a specific interface as well
// as those that listen on all interfaces (e.g. opencode serve --mdns).
func portFree(port int) bool {
	for _, addr := range []string{"127.0.0.1", "0.0.0.0"} {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", addr, port))
		if err != nil {
			return false
		}
		ln.Close()
	}
	return true
}

// findByDirAndType returns an existing session if one is already running.
func (m *Manager) findByDirAndType(dir string, sessionType SessionType) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.sessions {
		if s.Dir == dir && s.Type == sessionType {
			return s
		}
	}

	return nil
}

// saveState persists current sessions to disk as JSON.
func (m *Manager) saveState() {
	m.mu.Lock()
	defer m.mu.Unlock()

	os.MkdirAll(filepath.Dir(m.stateFile), 0755)
	data, err := json.Marshal(m.sessions)
	if err == nil {
		os.WriteFile(m.stateFile, data, 0644)
	}
}

// loadState reads persisted sessions and removes orphans (processes no longer alive).
// For each orphan, it also deregisters the port from the external registrar so
// that the port range is fully reclaimed on restart.
func (m *Manager) loadState() {
	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		return // no state file yet - fresh start
	}

	var loaded map[string]*Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}

	for id, s := range loaded {
		proc, err := os.FindProcess(s.PID)
		// On macOS, FindProcess always succeeds; send signal 0 to check liveness.
		// Zombie processes also return nil here, but since we have no cmd to Wait()
		// on for sessions loaded from a previous run, we accept the race and rely
		// on Deregister + port re-use rather than perfect zombie detection.
		if err != nil || proc.Signal(syscall.Signal(0)) != nil {
			// Orphan — process is gone. Clean up its external registrations
			// so the port is returned to the available range.
			log.Printf("deregistering orphaned session %s (port %d, pid %d)", id, s.Port, s.PID)
			m.registrar.Deregister(s.Port)
			continue
		}
		m.sessions[id] = s
	}
}
