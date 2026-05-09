package rulesgen

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

	"github.com/aaronromeo/postmanpat/analysis"
	"github.com/aaronromeo/postmanpat/envmgr"
)

//go:embed templates/*
var templatesFS embed.FS

// Server handles HTTP requests for the rules generation UI
type Server struct {
	templates  *template.Template
	port       int
	imapclient *IMAPConnector
	analyzer   *Analyzer
	store      Store
	// rulesCfg  *envmgr.RulesConfig
}

// ServerConfig contains configuration for the server
type ServerConfig struct {
	StorePath  string
	WatchOut   string
	CleanupOut string
	OnetimeOut string
	ConfigPath string
	Addr       string
	Username   string
	Password   string
	Port       int

	Cfg           envmgr.RulesConfig
	RulesGenStore []byte
}

type Option func(*ServerConfig)

func WithRulesConfig(configPath string) Option {
	return func(s *ServerConfig) {
		s.ConfigPath = strings.TrimSpace(configPath)
	}
}

func WithRulesGenStore(storePath string) Option {
	return func(s *ServerConfig) {
		s.StorePath = strings.TrimSpace(storePath)
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

func WithPort(port int) Option {
	return func(c *ServerConfig) {
		c.Port = port
	}
}

func WithAddr(a string) Option {
	return func(c *ServerConfig) {
		c.Addr = a
	}
}

func WithCreds(username string, password string) Option {
	return func(c *ServerConfig) {
		c.Username = username
		c.Password = password
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
	cfg, err := envmgr.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := envmgr.Validate(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	sc.Cfg = cfg

	if sc.StorePath == "" {
		return fmt.Errorf("rulesgen store path is required")
	}

	if sc.WatchOut == "" {
		return fmt.Errorf("watch out path is required")
	}

	if sc.CleanupOut == "" {
		return fmt.Errorf("cleanup out path is required")
	}

	if sc.OnetimeOut == "" {
		return fmt.Errorf("onetime cleanup out path is required")
	}

	if sc.Addr == "" {
		return fmt.Errorf("imap address is required")
	}

	if sc.Username == "" {
		return fmt.Errorf("imap username is required")
	}

	if sc.Password == "" {
		return fmt.Errorf("imap password is required")
	}

	if sc.Port == 0 {
		return fmt.Errorf("web server port is required")
	}

	// Validate that rules have server matchers
	for _, rule := range sc.Cfg.Rules {
		if rule.Server == nil {
			return fmt.Errorf("rule %q must define server matchers for rulesgen", rule.Name)
		}
	}

	return nil
}

// NewServer creates a new rules generation server
func NewServer(config *ServerConfig) (*Server, error) {
	// Initialize the SQLite store
	store, err := LoadServerStore(config.StorePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}
	// Note: store is closed when Server.Run() returns or server is stopped

	return NewServerWithStore(config, store)
}

// NewServerWithStore creates a new rules generation server (used for tests)
func NewServerWithStore(config *ServerConfig, store Store) (*Server, error) {
	// Create template with js function for escaping JavaScript
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"js": func(s string) string {
			// Simple escaping for use in HTML IDs and JavaScript
			return strings.ReplaceAll(s, ":", "_")
		},
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}
	analyzer := NewAnalyzer(&config.Cfg)
	imapclient := &IMAPConnector{
		Addr:     config.Addr,
		Username: config.Username,
		Password: config.Password,
	}

	return &Server{
		templates:  tmpl,
		port:       config.Port,
		imapclient: imapclient,
		analyzer:   analyzer,
		store:      store,
		// config:    config,
	}, nil
}

// Handler returns the HTTP handler for the server
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/analysis", s.handleAnalysis)
	mux.HandleFunc("/api/clusters", s.handleClusters)
	mux.HandleFunc("/api/decisions", s.handleDecisions)
	mux.HandleFunc("/api/yaml-preview", s.handleYamlPreview)
	return mux
}

// Run starts the HTTP server
func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.port)
	return http.ListenAndServe(addr, s.Handler())
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	options := DefaultAnalyzeOptions()

	report, err := s.analyzer.Run(ctx, s.imapclient, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("Analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Load decided clusters from store
	decidedClusters, err := s.store.GetAllDecidedClusterIDs()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load decisions: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert clusters to ClusterView, filtering out decided ones
	var clusters []ClusterView
	clusters = append(clusters, convertLensToViews(report.Indexes.ListLens, decidedClusters)...)
	clusters = append(clusters, convertLensToViews(report.Indexes.SenderLens, decidedClusters)...)
	clusters = append(clusters, convertLensToViews(report.Indexes.TemplateLens, decidedClusters)...)
	clusters = append(clusters, convertLensToViews(report.Indexes.RecipientTagLens, decidedClusters)...)

	data := struct {
		Clusters []ClusterView
		Total    int
	}{
		Clusters: clusters,
		Total:    len(clusters),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
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

	report, err := s.analyzer.Run(ctx, s.imapclient, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("Analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	options := DefaultAnalyzeOptions()

	report, err := s.analyzer.Run(ctx, s.imapclient, options)
	if err != nil {
		http.Error(w, fmt.Sprintf("Analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Load decided clusters from store
	decidedClusters, err := s.store.GetAllDecidedClusterIDs()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load decisions: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert clusters to ClusterView, filtering out decided ones
	var clusters []ClusterView
	clusters = append(clusters, convertLensToViews(report.Indexes.ListLens, decidedClusters)...)
	clusters = append(clusters, convertLensToViews(report.Indexes.SenderLens, decidedClusters)...)
	clusters = append(clusters, convertLensToViews(report.Indexes.TemplateLens, decidedClusters)...)
	clusters = append(clusters, convertLensToViews(report.Indexes.RecipientTagLens, decidedClusters)...)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(clusters); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetDecisions(w, r)
	case http.MethodPost:
		s.handlePostDecisions(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetDecisions(w http.ResponseWriter, r *http.Request) {
	decisions, err := s.store.GetAllDecisions()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load decisions: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(decisions); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

func (s *Server) handlePostDecisions(w http.ResponseWriter, r *http.Request) {
	var decisions []Decision
	if err := json.NewDecoder(r.Body).Decode(&decisions); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate and store each decision
	for _, decision := range decisions {
		if err := decision.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("Invalid decision: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Start a transaction
	tx, err := s.store.BeginTransaction()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start transaction: %v", err), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Store each decision
	for _, decision := range decisions {
		if _, err := s.store.CreateDecisionTx(tx, decision); err != nil {
			http.Error(w, fmt.Sprintf("Failed to store decision: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to commit transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// TODO: Generate YAML files in Step 5

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   len(decisions),
	})
}

func (s *Server) handleYamlPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		ClusterID   string `json:"cluster_id"`
		Lens        string `json:"lens"`
		Decision    string `json:"decision"` // ignore, watch, cleanup
		Action      string `json:"action"`   // delete, move
		Destination string `json:"destination,omitempty"`
		AgeWindow   string `json:"age_window,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if request.ClusterID == "" {
		http.Error(w, "cluster_id is required", http.StatusBadRequest)
		return
	}
	if request.Lens == "" {
		http.Error(w, "lens is required", http.StatusBadRequest)
		return
	}
	if request.Decision == "" {
		http.Error(w, "decision is required", http.StatusBadRequest)
		return
	}

	// Create a mock cluster for YAML generation
	cluster := analysis.Cluster{
		ClusterID: request.ClusterID,
		Count:     1, // Default count
		Keys:      make(map[string]any),
		Examples: analysis.ClusterExamples{
			SubjectRaw:             []string{"Example Subject"},
			SenderDomains:          []string{"example.com"},
			ListUnsubscribeTargets: []string{},
		},
		Signals: analysis.ClusterSignals{
			HasListID:          strings.Contains(request.Lens, "list"),
			HasListUnsubscribe: strings.Contains(request.Lens, "unsub"),
		},
	}

	// Add lens-specific data
	switch request.Lens {
	case "list_lens":
		cluster.Keys["ListID"] = strings.Split(request.ClusterID, ":")[1] + "-list-id"
	case "sender_unsub_lens":
		cluster.Keys["SenderDomain"] = "example.com"
	case "recipient_tag_lens":
		cluster.Keys["recipient_tag"] = strings.Split(request.ClusterID, ":")[1] + "-tag"
	}

	// Generate YAML preview
	preview, err := GenerateRulePreview(cluster, request.Decision, request.Action, request.Destination, request.AgeWindow)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate YAML: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the preview
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"preview": preview,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

// convertLensToViews converts analysis.Lens clusters to ClusterView, filtering decided clusters
func convertLensToViews(lens analysis.Lens, decidedClusters map[string]bool) []ClusterView {
	var views []ClusterView
	for _, cluster := range lens.Clusters {
		lensName := extractLensFromClusterID(cluster.ClusterID)
		clusterID := extractClusterIDWithoutLens(cluster.ClusterID)
		key := fmt.Sprintf("%s:%s", lensName, clusterID)
		if decidedClusters[key] {
			continue // Skip decided clusters
		}

		// Generate YAML previews
		watchYAML := ""
		cleanupYAML := ""
		
		// Generate Watch rule (delete action as default)
		watchPreview, err := GenerateRulePreview(cluster, "watch", "delete", "", "")
		if err == nil && watchPreview.WatchRule != "" {
			watchYAML = watchPreview.WatchRule
		}
		
		// Generate Cleanup rule (30d age window as default)
		cleanupPreview, err := GenerateRulePreview(cluster, "cleanup", "delete", "", "30d")
		if err == nil && cleanupPreview.CleanupRule != "" {
			cleanupYAML = cleanupPreview.CleanupRule
		}

		view := ClusterView{
			ClusterID:   cluster.ClusterID,
			Lens:        lensName,
			Count:       cluster.Count,
			Keys:        cluster.Keys,
			Examples:    cluster.Examples,
			HasDecision: false,
			WatchYAML:   watchYAML,
			CleanupYAML: cleanupYAML,
		}
		views = append(views, view)
	}
	return views
}

// extractClusterIDWithoutLens extracts the unique cluster ID without the lens prefix
func extractClusterIDWithoutLens(clusterID string) string {
	parts := strings.SplitN(clusterID, ":", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return clusterID
}

// extractLensFromClusterID extracts the lens name from a cluster ID (e.g., "list_lens:abc123" -> "list_lens")
func extractLensFromClusterID(clusterID string) string {
	parts := strings.SplitN(clusterID, ":", 2)
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

// LoadServerStore loads or creates the SQLite store for the server
func LoadServerStore(storePath string) (Store, error) {
	if storePath == "" {
		return nil, fmt.Errorf("store path is empty")
	}
	// Ensure the directory exists
	if idx := strings.LastIndex(storePath, string(os.PathSeparator)); idx >= 0 {
		dir := storePath[:idx]
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create store directory: %w", err)
			}
		}
	}
	return NewStore(storePath)
}

// Close closes the server and its resources
func (s *Server) Close() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}

// GetStore returns the server's store (for testing)
func (s *Server) GetStore() Store {
	return s.store
}

// Verify Store implements StoreInterface
var _ Store = (*SqlStore)(nil)
