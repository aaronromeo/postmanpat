package rulesgen

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
)

//go:embed templates/*
var templatesFS embed.FS

// Server handles HTTP requests for the rules generation UI
type Server struct {
	templates *template.Template
	port      int
	config    *appconfig.Config
	analyzer  *Analyzer
}

// ServerConfig contains configuration for the server
type ServerConfig struct {
	Port       int
	storePath  string
	WatchOut   string
	CleanupOut string
	OnetimeOut string
	ConfigPath string

	Cfg           appconfig.Config
	RulesGenStore []byte
}

type Option func(*ServerConfig)

func WithConfig(configPath string) Option {
	return func(s *ServerConfig) {
		s.ConfigPath = strings.TrimSpace(configPath)
	}
}

func WithRulesGenStore(storePath string) Option {
	return func(s *ServerConfig) {
		s.storePath = strings.TrimSpace(storePath)
	}
}

func WithWatchOut(watchOutPath string) Option {
	return func(s *ServerConfig) {
		s.WatchOut = strings.TrimSpace(watchOutPath)
	}
}

func WithCleanupOut(cleanupOutPath string) Option {
	return func(s *ServerConfig) {
		s.CleanupOut = strings.TrimSpace(cleanupOutPath)
	}
}

func WithOneTimeOut(onetimeOutPath string) Option {
	return func(s *ServerConfig) {
		s.OnetimeOut = strings.TrimSpace(onetimeOutPath)
	}
}

func NewServerConfig(options ...Option) *ServerConfig {
	serverConfig := ServerConfig{}
	for _, option := range options {
		option(&serverConfig)
	}
	return &serverConfig
}

func (sc *ServerConfig) Validate() error {
	if sc.ConfigPath == "" {
		return fmt.Errorf("config path is required")
	}
	configPath := sc.ConfigPath
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := appconfig.Validate(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	sc.Cfg = cfg

	if sc.storePath == "" {
		return fmt.Errorf("rulesgen store path is required")
	}
	rulesGenStore, err := os.ReadFile(sc.storePath)
	if err != nil {
		return err
	}
	sc.RulesGenStore = rulesGenStore

	if sc.WatchOut == "" {
		return fmt.Errorf("watch out path is required")
	}

	if sc.CleanupOut == "" {
		return fmt.Errorf("cleanup out path is required")
	}

	if sc.OnetimeOut == "" {
		return fmt.Errorf("onetime cleanup out path is required")
	}

	return nil
}

// NewServer creates a new rules generation server
func NewServer(port int, config *appconfig.Config) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	analyzer := NewAnalyzer(config)

	return &Server{
		templates: tmpl,
		port:      port,
		config:    config,
		analyzer:  analyzer,
	}, nil
}

// Handler returns the HTTP handler for the server
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/analysis", s.handleAnalysis)
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

func (s *Server) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	options := DefaultAnalyzeOptions()

	report, err := s.analyzer.Run(ctx, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("Analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}
