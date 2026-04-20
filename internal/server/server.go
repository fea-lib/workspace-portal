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
	manager session.ManagerInterface
	tmpl    *template.Template
	mux     *http.ServeMux
}

func New(cfg *config.Config, mgr session.ManagerInterface) *Server {
	tmpl, err := template.ParseFS(assets.TemplateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	h := &handler{cfg: cfg, manager: mgr, tmpl: tmpl}

	s := &Server{
		cfg:     cfg,
		manager: mgr,
		tmpl:    tmpl,
		mux:     http.NewServeMux(),
	}

	s.mux.HandleFunc("GET /", h.index)
	s.mux.HandleFunc("GET /fs/list", h.fsList)
	s.mux.HandleFunc("GET /sessions", h.sessions)
	s.mux.HandleFunc("POST /sessions/start", h.sessionsStart)
	s.mux.HandleFunc("POST /sessions/stop", h.sessionsStop)
	s.mux.HandleFunc("GET /events", h.events)
	s.mux.HandleFunc("GET /static/", h.static)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Start builds all dependencies and starts the HTTP server.
func Start(cfg *config.Config) error {
	stateDir, _ := os.UserHomeDir()
	stateFile := filepath.Join(stateDir, ".local", "share", "workspace-portal", "sessions.json")

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

	srv := New(cfg, manager)

	addr := fmt.Sprintf(":%d", cfg.PortalPort)
	log.Printf("listening on %s", addr)

	return http.ListenAndServe(addr, srv)
}
