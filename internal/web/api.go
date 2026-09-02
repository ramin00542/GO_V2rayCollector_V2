package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/state"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/tester"
)

// Paths describes the project layout the web package reads its data from.
type Paths = config.Paths

// configTestTimeout bounds how long a manual config test may run.
const configTestTimeout = 90 * time.Second

// APIHandler handles API requests
type APIHandler struct {
	paths     Paths
	state     *state.Store
	cache     map[string]interface{}
	cacheTTL  time.Duration
	startTime time.Time
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(paths Paths) *APIHandler {
	return &APIHandler{
		paths:     paths,
		cache:     make(map[string]interface{}),
		cacheTTL:  5 * time.Minute,
		startTime: time.Now().UTC(),
	}
}

// SetState sets the state store
func (h *APIHandler) SetState(store *state.Store) {
	h.state = store
}

// SetStartTime sets the time the process started, used to report uptime.
func (h *APIHandler) SetStartTime(start time.Time) {
	if start.IsZero() {
		return
	}
	h.startTime = start.UTC()
}

// SiteAccessibilityStat describes how reachable a target site was during the
// last config test run.
type SiteAccessibilityStat struct {
	Tested  int     `json:"tested"`
	Success int     `json:"success"`
	Rate    float64 `json:"rate"`
}

// StatsResponse represents the stats API response
type StatsResponse struct {
	TotalConfigs         int                              `json:"total_configs"`
	ValidConfigs         int                              `json:"valid_configs"`
	WorkingConfigs       int                              `json:"working_configs"`
	TestSuccessRate      float64                          `json:"test_success_rate"`
	ProtocolDistribution map[string]int                   `json:"protocol_distribution"`
	SiteAccessibility    map[string]SiteAccessibilityStat `json:"site_accessibility"`
	LastUpdate           time.Time                        `json:"last_update"`
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
		SiteAccessibility:    make(map[string]SiteAccessibilityStat),
		LastUpdate:           time.Now().UTC(),
	}

	// Load state
	entries, err := h.loadEntries()
	if err != nil {
		return stats, err
	}

	for _, entry := range entries {
		stats.TotalConfigs++

		// Count by protocol
		protocol := string(entry.Protocol)
		if protocol == "" {
			protocol = "unknown"
		}
		stats.ProtocolDistribution[protocol]++
	}

	// Every config kept in state already passed parsing/validation.
	stats.ValidConfigs = stats.TotalConfigs

	// Load test results if available
	reportPath := filepath.Join(h.paths.ReportsDir, "config_test_*.json")
	matches, err := filepath.Glob(reportPath)
	if err == nil && len(matches) > 0 {
		// Load the latest report
		latestReport := matches[len(matches)-1]
		data, err := os.ReadFile(latestReport)
		if err == nil {
			var report struct {
				TotalConfigs   int `json:"total_configs"`
				ValidConfigs   int `json:"valid_configs"`
				WorkingConfigs int `json:"working_configs"`
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
						tally := stats.SiteAccessibility[site]

						if result.Success {
							tally.Success++
						}
						tally.Tested++

						// Calculate rate
						if tally.Tested > 0 {
							tally.Rate = float64(tally.Success) / float64(tally.Tested) * 100
						}

						stats.SiteAccessibility[site] = tally
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
	Value       string    `json:"value"`
	Protocol    string    `json:"protocol"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	ShortValue  string    `json:"short_value"`
}

// GetConfigs returns a list of configs
func (h *APIHandler) GetConfigs(protocol, limitStr, offsetStr string) ([]ConfigResponse, int, error) {
	// Parse limit and offset
	limit := 100
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}
	if limit < 0 {
		limit = 0
	}

	offset := 0
	if offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}
	if offset < 0 {
		offset = 0
	}

	// Load state
	entries, err := h.loadEntries()
	if err != nil {
		return nil, 0, err
	}

	// Filter by protocol
	filtered := make([]state.Entry, 0, len(entries))
	for _, entry := range entries {
		if protocol == "" || string(entry.Protocol) == protocol {
			filtered = append(filtered, entry)
		}
	}

	// Apply pagination
	start := offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + limit
	if end > len(filtered) || limit == 0 {
		end = len(filtered)
	}

	// Convert to response
	configs := make([]ConfigResponse, 0, end-start)
	for _, entry := range filtered[start:end] {
		shortValue := entry.Value
		if len(shortValue) > 50 {
			shortValue = shortValue[:50] + "..."
		}

		configs = append(configs, ConfigResponse{
			Fingerprint: entry.Fingerprint,
			Value:       entry.Value,
			Protocol:    string(entry.Protocol),
			FirstSeen:   entry.FirstSeenAt,
			LastSeen:    entry.LastSeenAt,
			ShortValue:  shortValue,
		})
	}

	return configs, len(filtered), nil
}

// loadEntries returns every known config, most recently seen first.
// The order is deterministic so pagination stays stable between requests.
func (h *APIHandler) loadEntries() ([]state.Entry, error) {
	entries := make([]state.Entry, 0)

	if h.state != nil {
		for _, entry := range h.state.Data().Entries {
			entries = append(entries, entry)
		}
	} else {
		statePath := filepath.Join(h.paths.DataDir, "state", "configs.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			if os.IsNotExist(err) {
				return entries, nil
			}
			return nil, err
		}

		var stateData state.Data
		if err := json.Unmarshal(data, &stateData); err != nil {
			return nil, err
		}

		for _, entry := range stateData.Entries {
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].LastSeenAt.Equal(entries[j].LastSeenAt) {
			return entries[i].Fingerprint < entries[j].Fingerprint
		}
		return entries[i].LastSeenAt.After(entries[j].LastSeenAt)
	})

	return entries, nil
}

// GetConfig returns details of a specific config
func (h *APIHandler) GetConfig(fingerprint string) (map[string]interface{}, error) {
	entries, err := h.loadEntries()
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.Fingerprint != fingerprint {
			continue
		}

		observations := make([]map[string]interface{}, 0, len(entry.Observations))
		for _, observation := range entry.Observations {
			observations = append(observations, map[string]interface{}{
				"kind":         string(observation.Kind),
				"channel":      observation.Channel,
				"last_seen_at": observation.LastSeenAt,
			})
		}
		sort.Slice(observations, func(i, j int) bool {
			return fmt.Sprint(observations[i]["kind"], observations[i]["channel"]) <
				fmt.Sprint(observations[j]["kind"], observations[j]["channel"])
		})

		return map[string]interface{}{
			"fingerprint":  entry.Fingerprint,
			"value":        entry.Value,
			"protocol":     string(entry.Protocol),
			"first_seen":   entry.FirstSeenAt,
			"last_seen":    entry.LastSeenAt,
			"observations": observations,
		}, nil
	}

	return nil, fmt.Errorf("config %q not found", fingerprint)
}

// GetHealthHandler returns the /api/health handler
func (h *APIHandler) GetHealthHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		startTime := h.startTime
		if s != nil {
			startTime = s.startTime
		}
		uptime := time.Since(startTime)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":         "healthy",
			"uptime":         uptime.String(),
			"uptime_seconds": uptime.Seconds(),
			"timestamp":      time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// GetStatsHandler returns the /api/stats handler
func (h *APIHandler) GetStatsHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		stats, err := h.GetStats()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, stats)
	}
}

// GetConfigsHandler returns the /api/configs handler.
// The response body is a bare JSON array so it can be consumed directly by the
// dashboard JavaScript; the total number of matches is exposed through the
// X-Total-Count header for pagination.
func (h *APIHandler) GetConfigsHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		query := r.URL.Query()
		configs, total, err := h.GetConfigs(query.Get("protocol"), query.Get("limit"), query.Get("offset"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.Header().Set("X-Total-Count", fmt.Sprintf("%d", total))
		writeJSON(w, http.StatusOK, configs)
	}
}

// GetConfigHandler returns the /api/configs/{fingerprint} handler
func (h *APIHandler) GetConfigHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		fingerprint := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/configs/"), "/")
		if fingerprint == "" {
			writeError(w, http.StatusBadRequest, "config fingerprint is required")
			return
		}

		config, err := h.GetConfig(fingerprint)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, config)
	}
}

// GetSitesHandler returns the /api/sites handler
func (h *APIHandler) GetSitesHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		sites, err := h.loadTargetSites()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, sites)
	}
}

// GetReportsHandler returns the /api/reports handler
func (h *APIHandler) GetReportsHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		reports, err := h.loadReports()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, reports)
	}
}

// GetTestHandler returns the /api/test handler. It validates the submitted
// config and checks it against the configured target sites.
func (h *APIHandler) GetTestHandler(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var payload struct {
			Config string `json:"config"`
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &payload); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
		}
		if payload.Config == "" {
			payload.Config = r.FormValue("config")
		}

		config := strings.TrimSpace(payload.Config)
		if config == "" {
			writeError(w, http.StatusBadRequest, "config is required")
			return
		}

		result, err := h.testConfig(r.Context(), config)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// testConfig validates a config and checks it against every target site.
func (h *APIHandler) testConfig(ctx context.Context, config string) (map[string]interface{}, error) {
	sites, err := h.loadTargetSites()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, configTestTimeout)
	defer cancel()

	result := tester.TestConfig(ctx, config, sites, defaultTestSettings())

	return map[string]interface{}{
		"config":             result.ConfigValue,
		"protocol":           string(result.ConfigType),
		"valid":              result.IsValid,
		"error":              result.ValidationErr,
		"site_results":       result.SiteResults,
		"total_success":      result.TotalSuccess,
		"total_failed":       result.TotalFailed,
		"total_tested":       result.TotalTested,
		"average_latency_ms": result.AverageLatency.Milliseconds(),
		"tested_at":          result.TestTimestamp,
		"skip_reason":        result.SkipReason,
	}, nil
}

// ServeReport serves report files from the reports directory
func (h *APIHandler) ServeReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/reports/"), "/")
	if name == "" {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}

	// Reject any attempt to escape the reports directory.
	cleaned := filepath.Clean(filepath.FromSlash(name))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}

	path := filepath.Join(h.paths.ReportsDir, cleaned)
	if !strings.HasPrefix(path, filepath.Clean(h.paths.ReportsDir)+string(filepath.Separator)) {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}

	http.ServeFile(w, r, path)
}

// loadTargetSites loads the configured target sites, falling back to the
// defaults shipped with the tester package.
func (h *APIHandler) loadTargetSites() ([]tester.TargetSite, error) {
	path := filepath.Join(h.paths.ConfigDir, "target_sites.json")
	sitesConfig, err := tester.LoadTargetSites(path)
	if err != nil {
		return nil, err
	}
	if len(sitesConfig.Sites) == 0 {
		return tester.DefaultTargetSites(), nil
	}
	return sitesConfig.Sites, nil
}

// defaultTestSettings returns conservative settings for the manual test page.
func defaultTestSettings() tester.TestSettings {
	return tester.TestSettings{
		MaxConcurrentTests: 5,
		RequestTimeout:     10,
		RetryCount:         0,
		UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ConfigTester/1.0",
	}
}

// loadReports lists the files available in the reports directory
func (h *APIHandler) loadReports() ([]map[string]interface{}, error) {
	files, err := os.ReadDir(h.paths.ReportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]interface{}{}, nil
		}
		return nil, err
	}

	reports := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".csv") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		reports = append(reports, map[string]interface{}{
			"name":     name,
			"size":     info.Size(),
			"mod_time": info.ModTime(),
		})
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[j]["mod_time"].(time.Time).Before(reports[i]["mod_time"].(time.Time))
	})

	return reports, nil
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error":   message,
		"status":  status,
		"success": false,
	})
}
