// Package web provides a web server for the config collector
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/logging"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/state"
)

// ServerConfig configures a Server
type ServerConfig struct {
	Host   string
	Port   int
	Paths  Paths
	Logger *logging.Logger
}

// Server represents the web server
type Server struct {
	paths      config.Paths
	host       string
	port       int
	logger     *logging.Logger
	server     *http.Server
	wg         sync.WaitGroup
	stopChan   chan struct{}
	stopOnce   sync.Once
	startTime  time.Time
	lastUpdate time.Time
	mu         sync.RWMutex
	apiHandler *APIHandler
}

// NewServer creates a new web server
func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = logging.NewLogger()
		cfg.Logger.SetLevel(logging.LevelInfo)
	}

	host := cfg.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := cfg.Port
	if port == 0 {
		port = 8080
	}

	startTime := time.Now().UTC()

	// Create API handler
	apiHandler := NewAPIHandler(cfg.Paths)
	apiHandler.SetStartTime(startTime)

	return &Server{
		paths:      cfg.Paths,
		host:       host,
		port:       port,
		logger:     cfg.Logger,
		startTime:  startTime,
		lastUpdate: startTime,
		stopChan:   make(chan struct{}),
		apiHandler: apiHandler,
	}
}

// Addr returns the address the server listens on
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.host, s.port)
}

// Start starts the web server
func (s *Server) Start() error {
	// Create HTTP server
	s.server = &http.Server{
		Addr:              s.Addr(),
		Handler:           s.createRouter(),
		ReadHeaderTimeout: 30 * time.Second,
	}

	s.logger.Info("Starting web server", "address", s.server.Addr)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Web server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the web server. It is safe to call more than once.
func (s *Server) Stop() error {
	s.stopOnce.Do(func() { close(s.stopChan) })

	// Give some time for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Error shutting down web server", "error", err)
		return err
	}

	s.wg.Wait()
	s.logger.Info("Web server stopped")

	return nil
}

// UpdateLastUpdate updates the last update timestamp
func (s *Server) UpdateLastUpdate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUpdate = time.Now().UTC()
}

// createRouter creates the HTTP router
func (s *Server) createRouter() *http.ServeMux {
	mux := http.NewServeMux()

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(s.paths.Root, "web", "static")))))

	// API endpoints - using the APIHandler
	mux.HandleFunc("/api/health", s.apiHandler.GetHealthHandler(s))
	mux.HandleFunc("/api/stats", s.apiHandler.GetStatsHandler(s))
	mux.HandleFunc("/api/configs", s.apiHandler.GetConfigsHandler(s))
	mux.HandleFunc("/api/configs/", s.apiHandler.GetConfigHandler(s))
	mux.HandleFunc("/api/sites", s.apiHandler.GetSitesHandler(s))
	mux.HandleFunc("/api/reports", s.apiHandler.GetReportsHandler(s))
	mux.HandleFunc("/api/test", s.apiHandler.GetTestHandler(s))
	mux.HandleFunc("/reports/", s.apiHandler.ServeReport)

	// Web pages
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/dashboard", s.handleDashboard)
	mux.HandleFunc("/configs", s.handleConfigsPage)
	mux.HandleFunc("/reports", s.handleReportsPage)
	mux.HandleFunc("/test", s.handleTestPage)

	return mux
}

// API Handlers

// Page Handlers

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "index.html", nil)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := s.loadStats()
	if err != nil {
		stats = map[string]interface{}{}
	}

	s.serveTemplate(w, "dashboard.html", stats)
}

func (s *Server) handleConfigsPage(w http.ResponseWriter, r *http.Request) {
	configs, err := s.loadConfigs("", "100", "0")
	if err != nil {
		configs = []interface{}{}
	}

	s.serveTemplate(w, "configs.html", map[string]interface{}{
		"configs": configs,
	})
}

func (s *Server) handleReportsPage(w http.ResponseWriter, r *http.Request) {
	reports, err := s.loadReports()
	if err != nil {
		reports = []interface{}{}
	}

	s.serveTemplate(w, "reports.html", map[string]interface{}{
		"reports": reports,
	})
}

func (s *Server) handleTestPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Handle form submission
		config := r.FormValue("config")
		if config != "" {
			result, err := s.testConfig(config)
			if err != nil {
				s.serveTemplate(w, "test.html", map[string]interface{}{
					"error":  err.Error(),
					"config": config,
				})
				return
			}

			s.serveTemplate(w, "test.html", map[string]interface{}{
				"result": result,
				"config": config,
			})
			return
		}
	}

	s.serveTemplate(w, "test.html", nil)
}

// Helper functions

func (s *Server) serveTemplate(w http.ResponseWriter, name string, data interface{}) {
	// Try to load template from web/templates directory
	templatePath := filepath.Join(s.paths.Root, "web", "templates", name)

	// Check if template exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		// Try to create a simple HTML response
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<html><body><h1>Template not found: %s</h1></body></html>", name)
		return
	}

	// Parse template
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Execute template
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Data loading functions

func (s *Server) loadStats() (map[string]interface{}, error) {
	// Load collector stats
	statsPath := filepath.Join(s.paths.ReportsDir, "collector_stats.md")
	data, err := os.ReadFile(statsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{
				"status": "No stats available",
			}, nil
		}
		return nil, err
	}

	// Parse stats from markdown
	stats := parseStatsFromMarkdown(string(data))

	// Add system info
	stats["system"] = map[string]interface{}{
		"uptime": time.Since(s.startTime).String(),
	}

	return stats, nil
}

func (s *Server) loadConfigs(protocol, limit, offset string) ([]interface{}, error) {
	// Load from state file
	statePath := filepath.Join(s.paths.DataDir, "state", "configs.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}

	var stateData state.Data
	if err := json.Unmarshal(data, &stateData); err != nil {
		return nil, err
	}

	// Convert to list
	var configs []interface{}
	for _, entry := range stateData.Entries {
		if protocol != "" && string(entry.Protocol) != protocol {
			continue
		}
		configs = append(configs, map[string]interface{}{
			"fingerprint": entry.Fingerprint,
			"value":       entry.Value,
			"protocol":    entry.Protocol,
			"first_seen":  entry.FirstSeenAt,
			"last_seen":   entry.LastSeenAt,
		})
	}

	return configs, nil
}

func (s *Server) loadConfigDetail(fingerprint string) (map[string]interface{}, error) {
	// Load from state file
	statePath := filepath.Join(s.paths.DataDir, "state", "configs.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}

	var stateData state.Data
	if err := json.Unmarshal(data, &stateData); err != nil {
		return nil, err
	}

	// Find the config
	entry, ok := stateData.Entries[fingerprint]
	if !ok {
		return nil, fmt.Errorf("config not found")
	}

	return map[string]interface{}{
		"fingerprint":  entry.Fingerprint,
		"value":        entry.Value,
		"protocol":     entry.Protocol,
		"first_seen":   entry.FirstSeenAt,
		"last_seen":    entry.LastSeenAt,
		"observations": entry.Observations,
	}, nil
}

func (s *Server) loadTargetSites() ([]interface{}, error) {
	sitesPath := filepath.Join(s.paths.ConfigDir, "target_sites.json")
	data, err := os.ReadFile(sitesPath)
	if err != nil {
		return nil, err
	}

	var sites []interface{}
	if err := json.Unmarshal(data, &sites); err != nil {
		return nil, err
	}

	return sites, nil
}

func (s *Server) loadReports() ([]interface{}, error) {
	// List all files in reports directory
	files, err := os.ReadDir(s.paths.ReportsDir)
	if err != nil {
		return nil, err
	}

	var reports []interface{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Skip non-report files
		if !strings.HasSuffix(file.Name(), ".md") && !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		reports = append(reports, map[string]interface{}{
			"name":     file.Name(),
			"size":     info.Size(),
			"mod_time": info.ModTime(),
		})
	}

	return reports, nil
}

func (s *Server) testConfig(configValue string) (map[string]interface{}, error) {
	if s.apiHandler == nil {
		return nil, fmt.Errorf("api handler is not configured")
	}

	return s.apiHandler.testConfig(context.Background(), strings.TrimSpace(configValue))
}

// Helper function to parse stats from markdown
func parseStatsFromMarkdown(content string) map[string]interface{} {
	stats := make(map[string]interface{})

	// Simple parsing - in a real implementation, use a proper markdown parser
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "| ") {
			// Parse table row
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				key := strings.TrimSpace(parts[1])
				value := strings.TrimSpace(parts[2])
				stats[key] = value
			}
		}
	}

	return stats
}

// RunServer runs the web server
func RunServer(host string, port int, paths config.Paths) error {
	cfg := ServerConfig{
		Host:   host,
		Port:   port,
		Paths:  paths,
		Logger: logging.GetGlobalLogger(),
	}

	server := NewServer(cfg)

	// Update last update time periodically
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				server.UpdateLastUpdate()
			}
		}
	}()

	return server.Start()
}
