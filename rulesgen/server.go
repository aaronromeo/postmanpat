package rulesgen

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed templates/*
var templatesFS embed.FS

// Server handles HTTP requests for the rules generation UI
type Server struct {
	templates *template.Template
	port      int
}

// NewServer creates a new rules generation server
func NewServer(port int) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		templates: tmpl,
		port:      port,
	}, nil
}

// Handler returns the HTTP handler for the server
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

// Run starts the HTTP server
func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.port)
	return http.ListenAndServe(addr, s.Handler())
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
