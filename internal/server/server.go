package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"workspace-portal/internal/config"
	"workspace-portal/internal/session"
)

// Start builds all dependencies and starts the HTTP server.
func Start(cfg *config.Config) error {
	// State file lives in the user's local data directory
	stateDir, _ := os.UserHomeDir()
	stateFile := filepath.Join(stateDir, ".local", "share", "workspace-portal", "sessions.json")

	// Build manager — each factory is paired with its type and port range via Register().
	// The registeredFactory type is unexported; Register() is the only way in.
	manager := session.NewManager(
		stateFile,
		session.Register(
			session.SessionTypeOpenCode,
			&session.OCSessionFactory{Binary: cfg.OC.Binary, Flags: cfg.OC.Flags},
			cfg.OC.PortRange,
		),
		session.Register(
			session.SessionTypeVSCode,
			&session.VSCodeSessionFactory{Binary: cfg.VSCode.Binary, Password: cfg.Secret("vscode-password")},
			cfg.VSCode.PortRange,
		),
	)

	// HTTP mux
	mux := http.NewServeMux()
	h := &handler{cfg: cfg, manager: manager}

	mux.HandleFunc("GET /", h.index)
	mux.HandleFunc("GET /fs/list", h.fsList)
	mux.HandleFunc("GET /sessions", h.sessions)
	mux.HandleFunc("POST /sessions/start", h.sessionsStart)
	mux.HandleFunc("POST /sessions/stop", h.sessionsStop)
	mux.HandleFunc("GET /events", h.events)
	mux.HandleFunc("GET /static/", h.static)

	addr := fmt.Sprintf(":%d", cfg.PortalPort)
	log.Printf("listening on %s", addr)

	return http.ListenAndServe(addr, mux)
}
