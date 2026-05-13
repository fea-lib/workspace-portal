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
	"workspace-portal/internal/tailscale"
)

type Server struct {
	cfg     *config.Config
	manager session.ManagerInterface
	tmpl    *template.Template
	mux     *http.ServeMux
}

func New(cfg *config.Config, mgr session.ManagerInterface) *Server {
	tmpl := template.Must(template.ParseFS(assets.TemplateFS, "templates/*.html"))

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

	var tailscalRegistrar session.Registrar
	var tsFQDN string
	if cfg.Tailscale.Enabled {
		ts := &tailscale.Serve{Binary: cfg.Tailscale.Binary}
		tsFQDN = ts.FDQN()
		tailscalRegistrar = ts
	} else {
		tailscalRegistrar = &session.NoopRegistrar{}
	}

	corsOrigin := ""
	if tsFQDN != "" {
		corsOrigin = fmt.Sprintf("https://%s:%d", tsFQDN, cfg.PortalPort)
	}

	manager := session.NewManager(
		stateFile,
		tailscalRegistrar,
		tsFQDN,
		session.Register(
			session.SessionTypeOpenCode,
			&session.OCSessionFactory{Binary: cfg.OC.Binary, Flags: cfg.OC.Flags, CORSOrigin: corsOrigin},
			cfg.OC.PortRange,
		),
		session.Register(
			session.SessionTypeVSCode,
			&session.VSCodeSessionFactory{Binary: cfg.VSCode.Binary, Password: cfg.Secret("vscode-password")},
			cfg.VSCode.PortRange,
		),
	)

	srv := New(cfg, manager)

	// Bind only on loopback so tailscale serve can bind the same port number
	// on the Tailscale interface without a conflict.
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.PortalPort)
	log.Printf("listening on %s", addr)

	return http.ListenAndServe(addr, srv)
}
