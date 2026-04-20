package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"workspace-portal/internal/assets"
	"workspace-portal/internal/config"
	"workspace-portal/internal/session"
)

type Server struct {
	cfg     *config.Config
	manager *session.Manager
	tmpl    *template.Template
	mux     *http.ServeMux
}

func New(cfg *config.Config, mgr *session.Manager) *Server {
	tmpl, err := template.ParseFS(assets.TemplateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	s := &Server{
		cfg:     cfg,
		manager: mgr,
		tmpl:    tmpl,
		mux:     http.NewServeMux(),
	}

	// routes wired in Lesson 9

	return s
}

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

	tmpl, err := template.ParseFS(assets.TemplateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	// HTTP mux
	mux := http.NewServeMux()
	h := &handler{
		cfg:     cfg,
		manager: manager,
		tmpl:    tmpl,
	}

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
