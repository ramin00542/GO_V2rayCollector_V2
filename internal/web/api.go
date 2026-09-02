package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/state"
)

// APIHandler handles API requests
type APIHandler struct {
	paths   Paths
	state   *state.Store
	cache   map[string]interface{}
	cacheTTL time.Duration
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(paths Paths) *APIHandler {
	return &APIHandler{
		paths:   paths,
		cache:   make(map[string]interface{}),
		cacheTTL: 5 * time.Minute,
	}
}

// SetState sets the state store
func (h *APIHandler) SetState(store *state.Store) {
	h.state = store
}

// StatsResponse represents the stats API response
type StatsResponse struct {
	TotalConfigs    int                    `json:"total_configs"`
	ValidConfigs    int                    `json:"valid_configs"`
	WorkingConfigs  int                    `json:"working_configs"`
	TestSuccessRate float64               `json:"test_success_rate"`
	ProtocolDistribution map[string]int   `json:"protocol_distribution"`
	SiteAccessibility map[string]struct {
		Tested   int   `json:"tested"`
		Success  int   `json:"success"`
		Rate     float64 `json:"rate"`
	} `json:"site_accessibility"`
	LastUpdate time.Time `json:"last_update"`
}

// GetStats returns statistics about configs
func (h *APIHandler) GetStats() (StatsResponse, error) {
	// Try to get from cache
	if cached, ok := h.cache["stats"]; ok {
		if cachedStats, ok := cached.(StatsResponse); ok {
			return cachedStats, nil
		}
	}
	
	stats := StatsResponse{
		ProtocolDistribution: make(map[string]int),
		SiteAccessibility:    make(map[string]struct {
			Tested   int
			Success  int
			Rate     float64
		}),
		LastUpdate: time.Now().UTC(),
	}
	
	// Load state
	if h.state == nil {
		statePath := filepath.Join(h.paths.DataDir, "state", "configs.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			return stats, err
		}
		
		var stateData state.Data
		if err := json.Unmarshal(data, &stateData); err != nil {
			return stats, err
		}
		
		// Calculate stats from state
		for _, entry := range stateData.Entries {
			stats.TotalConfigs++
			
			// Count by protocol
			protocol := string(entry.Protocol)
			if protocol == "" {
				protocol = "unknown"
			}
			stats.ProtocolDistribution[protocol]++
		}
		
		stats.ValidConfigs = stats.TotalConfigs // All in state are valid
		
	} else {
		data := h.state.Data()
		for _, entry := range data.Entries {
			stats.TotalConfigs++
			
			// Count by protocol
			protocol := string(entry.Protocol)
			if protocol == "" {
				protocol = "unknown"
			}
			stats.ProtocolDistribution[protocol]++
		}
		
		stats.ValidConfigs = stats.TotalConfigs
	}
	
	// Load test results if available
	reportPath := filepath.Join(h.paths.ReportsDir, "config_test_*.json")
	matches, err := filepath.Glob(reportPath)
	if err == nil && len(matches) > 0 {
		// Load the latest report
		latestReport := matches[matches.length-1]
		data, err := os.ReadFile(latestReport)
		if err == nil {
			var report struct {
				TotalConfigs   int     `json:"total_configs"`
				ValidConfigs   int     `json:"valid_configs"`
				WorkingConfigs int     `json:"working_configs"`
				ConfigResults  []struct {
					SiteResults map[string]struct {
						Success bool `json:"success"`
					} `json:"site_results"`
				} `json:"config_results"`
			}
			
			if err := json.Unmarshal(data, &report); err == nil {
				stats.WorkingConfigs = report.WorkingConfigs
				stats.ValidConfigs = report.ValidConfigs
				
				// Calculate test success rate
				if report.ValidConfigs > 0 {
					stats.TestSuccessRate = float64(report.WorkingConfigs) / float64(report.ValidConfigs) * 100
				}
				
				// Calculate site accessibility
				for _, config := range report.ConfigResults {
					for site, result := range config.SiteResults {
						if _, ok := stats.SiteAccessibility[site]; !ok {
							stats.SiteAccessibility[site] = struct {
								Tested   int
								Success  int
								Rate     float64
							}{}
						}
						
						if result.Success {
							stats.SiteAccessibility[site].Success++
						}
						stats.SiteAccessibility[site].Tested++
						
						// Calculate rate
						if stats.SiteAccessibility[site].Tested > 0 {
							stats.SiteAccessibility[site].Rate = float64(stats.SiteAccessibility[site].Success) / 
								float64(stats.SiteAccessibility[site].Tested) * 100
						}
					}
				}
			}
		}
	}
	
	// Cache the stats
	h.cache["stats"] = stats
	
	return stats, nil
}

// ConfigResponse represents a config in the API response
type ConfigResponse struct {
	Fingerprint string    `json:"fingerprint"`
	Value      string    `json:"value"`
	Protocol   string    `json:"protocol"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	ShortValue string    `json:"short_value"`
}

// GetConfigs returns a list of configs
func (h *APIHandler) GetConfigs(protocol, limitStr, offsetStr string) ([]ConfigResponse, int, error) {
	// Parse limit and offset
	limit := 100
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}
	
	offset := 0
	if offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}
	
	// Load state
	var entries []state.Entry
	
	if h.state == nil {
		statePath := filepath.Join(h.paths.DataDir, "state", "configs.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			return nil, 0, err
		}
		
		var stateData state.Data
		if err := json.Unmarshal(data, &stateData); err != nil {
			return nil, 0, err
		}
		
		for _, entry := range stateData.Entries {
			entries = append(entries, entry)
		}
	} else {
		entries = h.state.Data().Entries
	}
	
	// Filter by protocol
	var filtered []state.Entry
	for _, entry := range entries {
		if protocol == "" || string(entry.Protocol) == protocol {
			filtered = append(filtered, entry)
		}
	}
	
	// Apply pagination
	start := offset
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	
	// Convert to response
	var configs []ConfigResponse
	for _, entry := range filtered[start:end] {
		shortValue := entry.Value
		if len(shortValue) > 50 {
			shortValue = shortValue[:50] + "..."
		}
		
		configs = append(configs, ConfigResponse{
			Fingerprint: entry.Fingerprint,
			Value:      entry.Value,
			Protocol:   string(entry.Protocol),
			FirstSeen:  entry.FirstSeenAt,
			LastSeen:   entry.LastSeenAt,
			ShortValue: shortValue,
		})
	}
	
	return configs, len(filtered), nil
}

// GetConfig returns details of a specific config
func (h *APIHandler) GetConfig(fingerprint string) (map[string]interface{}, error) {
	// Load state
	var entry state.Entry
	found := false
	
	if h.state == nil {
		statePath := filepath.Join(h.paths.DataDir, "state", "configs.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			return nil, err
		}
		
		var stateData state.Data
		if err := json.Unmarshal(data, &stateData); err != nil {
			return nil, err
		}
		
		if e, ok := stateData.Entries[fingerprint]; ok {
			entry = e
			found = true
		}
	} else {
		if e, ok := h.state.Data().Entries[fingerprint]; ok {
			entry = e
			found = true
		}
	}
	
	if !found {
		return nil, fmt.Errorf("config not found")
	}
	
	// Convert to response
	result := map[string]interface{}{
		"fingerprint": entry.Fingerprint,
		"value":      entry.Value,
		"protocol":   string(entry.Protocol),
		"first_seen": entry.FirstSeenAt,
		"last_seen":  entry.LastSeenAt,
		"observations": entry.Observations,
	}
	
	return result, nil
}

// TestRequest represents a test request
type TestRequest struct {
	Config string `json:"config"`
}

// TestResponse represents a test response
type TestResponse struct {
	Config     string                 `json:"config"`
	Valid      bool                   `json:"valid"`
	Error      string                 `json:"error,omitempty"`
	TestedAt   time.Time              `json:"tested_at"`
	SiteResults map[string]SiteResult `json:"site_results"`
	TotalSuccess int                  `json:"total_success"`
	TotalTested  int                  `json:"total_tested"`
}

// SiteResult represents the result of testing a config against a site
type SiteResult struct {
	StatusCode int           `json:"status_code"`
	Success    bool          `json:"success"`
	Latency    time.Duration `json:"latency_ms"`
	Error      string        `json:"error,omitempty"`
	TestedAt   time.Time     `json:"tested_at"`
}

// TestConfig tests a config against all target sites
func (h *APIHandler) TestConfig(configValue string) (TestResponse, error) {
	response := TestResponse{
		Config:    configValue,
		TestedAt:  time.Now().UTC(),
		SiteResults: make(map[string]SiteResult),
	}
	
	// Validate the config
	parsed, err := ParseConfig(configValue)
	if err != nil {
		response.Valid = false
		response.Error = err.Error()
		return response, nil
	}
	
	response.Valid = true
	
	// Load target sites
	targetSites, err := LoadTargetSites(filepath.Join(h.paths.ConfigDir, "target_sites.json"))
	if err != nil {
		return response, err
	}
	
	// Test against each site
	for _, site := range targetSites.Sites {
		siteResult := h.testSiteAccess(configValue, site)
		response.SiteResults[site.Name] = siteResult
		
		if siteResult.Success {
			response.TotalSuccess++
		}
		response.TotalTested++
	}
	
	return response, nil
}

// ParseConfig parses a config string
func ParseConfig(raw string) (domain.ParsedConfig, error) {
	// Import parser package
	// This is a placeholder - in a real implementation, you would use the parser package
	return domain.ParsedConfig{
		Value:    raw,
		Protocol: domain.ProtocolUnknown,
	}, nil
}

// LoadTargetSites loads target sites from config
func LoadTargetSites(path string) (TargetSitesConfig, error) {
	var config TargetSitesConfig
	
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return getDefaultTargetSitesConfig(), nil
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return TargetSitesConfig{}, err
	}
	
	if err := json.Unmarshal(data, &config); err != nil {
		return TargetSitesConfig{}, err
	}
	
	return config, nil
}

// testSiteAccess tests if a config can access a specific site
func (h *APIHandler) testSiteAccess(configValue string, site TargetSite) SiteResult {
	result := SiteResult{
		TestedAt: time.Now().UTC(),
	}
	
	// This is a placeholder implementation
	// In a real implementation, you would:
	// 1. Convert the config to a proxy
	// 2. Create an HTTP client with the proxy
	// 3. Make a request to the site URL
	// 4. Record the results
	
	// For now, we'll just return a mock result
	result.StatusCode = 200
	result.Success = true
	result.Latency = time.Duration(100 * time.Millisecond)
	
	return result
}

// GetReports returns a list of reports
func (h *APIHandler) GetReports() ([]ReportInfo, error) {
	// List files in reports directory
	files, err := os.ReadDir(h.paths.ReportsDir)
	if err != nil {
		return nil, err
	}
	
	var reports []ReportInfo
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		// Skip non-report files
		if !strings.HasSuffix(file.Name(), ".md") && !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		
		reports = append(reports, ReportInfo{
			Name:    file.Name(),
			Size:    file.Size(),
			ModTime: file.ModTime(),
			Type:    getFileType(file.Name()),
		})
	}
	
	// Sort by modification time (newest first)
	for i := 0; i < len(reports)-1; i++ {
		for j := i + 1; j < len(reports); j++ {
			if reports[i].ModTime.Before(reports[j].ModTime) {
				reports[i], reports[j] = reports[j], reports[i]
			}
		}
	}
	
	return reports, nil
}

// ReportInfo represents information about a report file
type ReportInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	Type    string    `json:"type"`
}

// getFileType returns the type of a file based on its extension
func getFileType(filename string) string {
	if strings.HasSuffix(filename, ".md") {
		return "markdown"
	}
	if strings.HasSuffix(filename, ".json") {
		return "json"
	}
	return "text"
}

// GetReportContent returns the content of a report file
func (h *APIHandler) GetReportContent(filename string) ([]byte, error) {
	// Validate filename to prevent directory traversal
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		return nil, fmt.Errorf("invalid filename")
	}
	
	// Check file exists and is in reports directory
	reportPath := filepath.Join(h.paths.ReportsDir, filename)
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("report not found")
	}
	
	// Read file
	return os.ReadFile(reportPath)
}

// ServeReport serves a report file
func (h *APIHandler) ServeReport(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Path[len("/reports/"):]
	if filename == "" {
		http.Error(w, "filename required", http.StatusBadRequest)
		return
	}
	
	// Get report content
	content, err := h.GetReportContent(filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	
	// Set content type
	if strings.HasSuffix(filename, ".json") {
		w.Header().Set("Content-Type", "application/json")
	} else if strings.HasSuffix(filename, ".md") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	
	// Write content
	w.Write(content)
}

// TargetSitesConfig represents the configuration for target sites
type TargetSitesConfig struct {
	Version       int            `json:"version"`
	Sites         []TargetSite   `json:"sites"`
	TestSettings  TestSettings    `json:"test_settings"`
}

// TargetSite represents a site to test against
type TargetSite struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	Category        string `json:"category"`
	ExpectedStatus  int    `json:"expected_status"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

// TestSettings contains settings for the testing process
type TestSettings struct {
	MaxConcurrentTests int    `json:"max_concurrent_tests"`
	RequestTimeout     int    `json:"request_timeout"`
	RetryCount         int    `json:"retry_count"`
	UserAgent          string `json:"user_agent"`
}

// getDefaultTargetSitesConfig returns a default configuration
func getDefaultTargetSitesConfig() TargetSitesConfig {
	return TargetSitesConfig{
		Version: 1,
		Sites: []TargetSite{
			{Name: "Google", URL: "https://www.google.com", Category: "search", ExpectedStatus: 200, TimeoutSeconds: 10},
			{Name: "YouTube", URL: "https://www.youtube.com", Category: "video", ExpectedStatus: 200, TimeoutSeconds: 10},
			{Name: "GitHub", URL: "https://github.com", Category: "code", ExpectedStatus: 200, TimeoutSeconds: 10},
		},
		TestSettings: TestSettings{
			MaxConcurrentTests: 10,
			RequestTimeout:     15,
			RetryCount:         2,
			UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ConfigTester/1.0",
		},
	}
}

// startTime is the time when the API handler was created
var startTime time.Time

// SetStartTime sets the start time for uptime calculation
func (h *APIHandler) SetStartTime(t time.Time) {
	h.startTime = t
}

// GetHealthHandler returns the health check handler
func (h *APIHandler) GetHealthHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := h.GetHealth()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health)
	}
}

// GetStatsHandler returns the stats handler
func (h *APIHandler) GetStatsHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := h.GetStats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// GetConfigsHandler returns the configs list handler
func (h *APIHandler) GetConfigsHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters
		query := r.URL.Query()
		protocol := query.Get("protocol")
		limit := query.Get("limit")
		offset := query.Get("offset")
		
		configs, total, err := h.GetConfigs(protocol, limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Create response with pagination info
		response := struct {
			Configs []ConfigResponse `json:"configs"`
			Total   int               `json:"total"`
			Limit   int               `json:"limit"`
			Offset  int               `json:"offset"`
		}{
			Configs: configs,
			Total:   total,
			Limit:   len(configs),
			Offset:  offset,
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// GetConfigHandler returns the config detail handler
func (h *APIHandler) GetConfigHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract fingerprint from URL
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			http.Error(w, "fingerprint required", http.StatusBadRequest)
			return
		}
		fingerprint := parts[2]
		
		config, err := h.GetConfig(fingerprint)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	}
}

// GetSitesHandler returns the sites list handler
func (h *APIHandler) GetSitesHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sites, err := h.LoadTargetSites(filepath.Join(h.paths.ConfigDir, "target_sites.json"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sites.Sites)
	}
}

// GetReportsHandler returns the reports list handler
func (h *APIHandler) GetReportsHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reports, err := h.GetReports()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reports)
	}
}

// GetTestHandler returns the test config handler
func (h *APIHandler) GetTestHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		// Parse request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		var request TestRequest
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		if request.Config == "" {
			http.Error(w, "config is required", http.StatusBadRequest)
			return
		}
		
		// Test the config
		result, err := h.TestConfig(request.Config)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Uptime    string    `json:"uptime"`
	Timestamp string    `json:"timestamp"`
	Version   string    `json:"version"`
}

// GetHealth returns the health status of the application
func (h *APIHandler) GetHealth() HealthResponse {
	return HealthResponse{
		Status:    "healthy",
		Uptime:    time.Since(h.startTime).String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   "2.0.0",
	}
}

// SetStartTime sets the start time for uptime calculation
func (h *APIHandler) SetStartTime(t time.Time) {
	h.startTime = t
}

// startTime is the time when the application started
var startTime time.Time

// SetStartTime sets the global start time
func SetStartTime(t time.Time) {
	startTime = t
}
