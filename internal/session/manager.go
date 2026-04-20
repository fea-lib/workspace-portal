package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"workspace-portal/internal/portrange"
)

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
	stateFile string
	events    chan Event
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
func NewManager(stateFile string, registrations ...registeredFactory) *Manager {
	factories := make(map[SessionType]registeredFactory, len(registrations))
	for _, r := range registrations {
		factories[r.sessionType] = r
	}

	m := &Manager{
		sessions:  make(map[string]*Session),
		stateFile: stateFile,
		events:    make(chan Event, 64),
		factories: factories,
	}
	m.loadState()

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
		return existing, nil
	}

	port, err := m.nextPort(reg.portRange)
	if err != nil {
		return nil, err
	}

	pid, err := reg.factory.Start(dir, port)
	if err != nil {
		return nil, err
	}

	s := &Session{
		ID:        uuid.New().String(),
		Type:      sessionType,
		Dir:       dir,
		Port:      port,
		PID:       pid,
		StartedAt: time.Now(),
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	m.saveState()

	m.events <- Event{Type: EventTypeStarted, Session: s}

	// Health check runs in a goroutine — it blocks until the process responds,
	// then updates s.URL and sends the "healthy" event.
	go m.waitHealthy(s, reg.factory.HealthURL(port))

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
	m.mu.Unlock()

	if reg, ok := m.factories[s.Type]; ok {
		reg.factory.Stop(s.PID)
	}

	m.saveState()
	m.events <- Event{Type: EventTypeStopped, Session: s}

	return nil
}

// waitHealthy polls until the session responds, then marks it healthy.
// It times out after 30 seconds to avoid leaking goroutines for processes that
// fail to start.
func (m *Manager) waitHealthy(s *Session, healthURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return // timed out - process failed to become healthy
		case <-ticker.C:
			resp, err := http.Get(healthURL)
			if err == nil && resp.StatusCode < 500 {
				resp.Body.Close()
				m.mu.Lock()
				s.URL = healthURL
				m.mu.Unlock()
				m.saveState()
				m.events <- Event{Type: EventTypeHealthy, Session: s}
				return
			}
		}
	}
}

// nextPort finds the first available port in the given range.
// It checks both the in-use session map (fast) and then attempts to bind
// the port (authoritative — catches ports used by unrelated processes).
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

		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", r[0], r[1])
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
		if err != nil || proc.Signal(syscall.Signal(0)) != nil {
			continue // orphan - process is gone
		}
		m.sessions[id] = s
	}
}
