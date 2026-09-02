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
		latestReport := matches[len(matches)-1]
		data, err := os.ReadFile(latestReport)
		if err == nil {
			var report struct {
				TotalConfigs   int    `json:"total_configs"`
				ValidConfigs   int    `json:"valid_configs"`
				WorkingConfigs int    `json:"working_configs"`
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
							stats.SiteAccessibility[site].Rate = float64(stats.SiteAccessibility[site].Success) / float64(stats.SiteAccessibility[site].Tested) * 100
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
		statePath := filepath.Join(h.paths.DataDir, "state", 
