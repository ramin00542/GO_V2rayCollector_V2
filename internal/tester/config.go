package tester

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/parser"
)

// TargetSite represents a site to test against
type TargetSite struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	Category       string `json:"category"`
	ExpectedStatus int    `json:"expected_status"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// TargetSitesConfig represents the configuration for target sites
type TargetSitesConfig struct {
	Version      int          `json:"version"`
	Sites        []TargetSite `json:"sites"`
	TestSettings TestSettings `json:"test_settings"`
}

// TestSettings contains settings for the testing process
type TestSettings struct {
	MaxConcurrentTests int    `json:"max_concurrent_tests"`
	RequestTimeout     int    `json:"request_timeout"`
	RetryCount         int    `json:"retry_count"`
	UserAgent          string `json:"user_agent"`
}

// ConfigTestResult represents the result of testing a single config
type ConfigTestResult struct {
	ConfigValue    string                `json:"config_value"`
	ConfigType     domain.Protocol       `json:"config_type"`
	IsValid        bool                  `json:"is_valid"`
	ValidationErr  string                `json:"validation_error,omitempty"`
	SiteResults    map[string]SiteResult `json:"site_results"`
	TotalSuccess   int                   `json:"total_success"`
	TotalFailed    int                   `json:"total_failed"`
	TotalTested    int                   `json:"total_tested"`
	AverageLatency time.Duration         `json:"average_latency_ms"`
	TestTimestamp  time.Time             `json:"test_timestamp"`
}

// SiteResult represents the result of testing a config against a specific site
type SiteResult struct {
	StatusCode  int           `json:"status_code"`
	Success     bool          `json:"success"`
	Latency     time.Duration `json:"latency_ms"`
	Error       string        `json:"error,omitempty"`
	RedirectURL string        `json:"redirect_url,omitempty"`
	TestedAt    time.Time     `json:"tested_at"`
}

// TestReport represents the complete test report
type TestReport struct {
	GeneratedAt    time.Time                 `json:"generated_at"`
	TotalConfigs   int                       `json:"total_configs"`
	ValidConfigs   int                       `json:"valid_configs"`
	TestedConfigs  int                       `json:"tested_configs"`
	WorkingConfigs int                       `json:"working_configs"`
	ConfigResults  []ConfigTestResult        `json:"config_results"`
	SiteStatistics map[string]SiteStatistics `json:"site_statistics"`
	Summary        string                    `json:"summary"`
}

// SiteStatistics represents statistics for a specific site
type SiteStatistics struct {
	TotalTested   int      `json:"total_tested"`
	TotalSuccess  int      `json:"total_success"`
	SuccessRate   float64  `json:"success_rate"`
	AccessibleVia []string `json:"accessible_via"`
}

// LoadTargetSites loads target sites configuration from a file
func LoadTargetSites(path string) (TargetSitesConfig, error) {
	var config TargetSitesConfig

	// Use default config if file doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return getDefaultTargetSitesConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return TargetSitesConfig{}, fmt.Errorf("failed to read target sites config: %w", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return TargetSitesConfig{}, fmt.Errorf("failed to parse target sites config: %w", err)
	}

	if config.Version != 1 {
		return TargetSitesConfig{}, fmt.Errorf("unsupported target sites config version: %d", config.Version)
	}

	// Set defaults if not provided
	if config.TestSettings.UserAgent == "" {
		config.TestSettings.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ConfigTester/1.0"
	}
	if config.TestSettings.MaxConcurrentTests <= 0 {
		config.TestSettings.MaxConcurrentTests = 10
	}
	if config.TestSettings.RequestTimeout <= 0 {
		config.TestSettings.RequestTimeout = 15
	}

	return config, nil
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

// TestConfig tests a single config against all target sites
func TestConfig(ctx context.Context, configValue string, sites []TargetSite, settings TestSettings) ConfigTestResult {
	result := ConfigTestResult{
		ConfigValue:   configValue,
		ConfigType:    domain.ProtocolUnknown,
		SiteResults:   make(map[string]SiteResult),
		TestTimestamp: time.Now().UTC(),
	}

	// Validate the config
	parsed, err := parser.Parse(configValue, true)
	if err != nil {
		result.IsValid = false
		result.ValidationErr = err.Error()
		return result
	}

	result.ConfigType = parsed.Protocol
	result.IsValid = true

	// Test against each site
	var totalLatency time.Duration
	for _, site := range sites {
		if err := ctx.Err(); err != nil {
			break
		}

		siteResult := testSiteAccess(ctx, configValue, site, settings)
		result.SiteResults[site.Name] = siteResult
		result.TotalTested++

		if siteResult.Success {
			result.TotalSuccess++
			totalLatency += siteResult.Latency
		} else {
			result.TotalFailed++
		}
	}

	// Calculate average latency
	if result.TotalSuccess > 0 {
		result.AverageLatency = totalLatency / time.Duration(result.TotalSuccess)
	}

	return result
}

// testSiteAccess tests if a config can access a specific site
func testSiteAccess(ctx context.Context, configValue string, site TargetSite, settings TestSettings) SiteResult {
	result := SiteResult{
		TestedAt: time.Now().UTC(),
	}

	// Create a custom transport that uses the config as a proxy
	// For now, we'll use a direct connection since implementing proxy for all protocols is complex
	// In a real implementation, you would need to convert the config to a proxy address

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   time.Duration(settings.RequestTimeout) * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: time.Duration(settings.RequestTimeout) * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
	}

	client := &http.Client{
		Timeout:   time.Duration(settings.RequestTimeout) * time.Second,
		Transport: transport,
	}

	// Parse the target URL
	parsedURL, err := url.Parse(site.URL)
	if err != nil {
		result.Error = fmt.Sprintf("invalid URL: %v", err)
		return result
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		result.Error = fmt.Sprintf("unsupported URL scheme: %s", parsedURL.Scheme)
		return result
	}
	if parsedURL.Host == "" {
		result.Error = fmt.Sprintf("invalid URL: missing host in %q", site.URL)
		return result
	}

	// Try with retries
	var lastErr error
	for attempt := 0; attempt <= settings.RetryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				result.Error = ctx.Err().Error()
				return result
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, site.URL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		// Set headers
		req.Header.Set("User-Agent", settings.UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Connection", "keep-alive")

		resp, err := client.Do(req)
		latency := time.Since(start)

		if err != nil {
			lastErr = err
			// Check if it's a timeout
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				result.Error = fmt.Sprintf("request timeout after %v", latency)
			} else {
				result.Error = err.Error()
			}
			continue
		}

		result.Latency = latency
		result.StatusCode = resp.StatusCode

		// Check for redirect
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location != "" {
				result.RedirectURL = location
				// Follow redirect
				redirectURL, err := url.Parse(location)
				if err == nil && redirectURL.IsAbs() {
					// Test the redirect URL
					resp2, err := client.Get(redirectURL.String())
					if err == nil {
						result.StatusCode = resp2.StatusCode
						resp2.Body.Close()
					}
				}
			}
		}

		// Check if successful
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result.Success = true
		} else {
			result.Success = false
			result.Error = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
		}

		// Read and close the body
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		// If successful, break out of retry loop
		if result.Success {
			break
		}

		lastErr = fmt.Errorf("status code: %d", resp.StatusCode)
	}

	if !result.Success && lastErr != nil {
		result.Error = lastErr.Error()
	}

	return result
}

// TestConfigs tests multiple configs against target sites with concurrency control
func TestConfigs(ctx context.Context, configs []string, sites []TargetSite, settings TestSettings, maxConfigs int) ([]ConfigTestResult, error) {
	if maxConfigs <= 0 {
		maxConfigs = len(configs)
	}
	if maxConfigs > len(configs) {
		maxConfigs = len(configs)
	}

	// Limit the number of configs to test
	configsToTest := configs[:maxConfigs]

	results := make([]ConfigTestResult, 0, len(configsToTest))
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create a semaphore for concurrency control
	sem := make(chan struct{}, settings.MaxConcurrentTests)

	for _, config := range configsToTest {
		if err := ctx.Err(); err != nil {
			break
		}

		wg.Add(1)
		go func(cfg string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			result := TestConfig(ctx, cfg, sites, settings)

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(config)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Sort results by success rate (descending)
	sort.Slice(results, func(i, j int) bool {
		iRate := float64(results[i].TotalSuccess) / float64(max(results[i].TotalTested, 1))
		jRate := float64(results[j].TotalSuccess) / float64(max(results[j].TotalTested, 1))
		return iRate > jRate
	})

	return results, nil
}

// GenerateReport generates a comprehensive test report
func GenerateReport(results []ConfigTestResult, sites []TargetSite) TestReport {
	report := TestReport{
		GeneratedAt:    time.Now().UTC(),
		TotalConfigs:   len(results),
		ConfigResults:  results,
		SiteStatistics: make(map[string]SiteStatistics),
	}

	// Count valid and tested configs
	for _, result := range results {
		if result.IsValid {
			report.ValidConfigs++
		}
		if result.TotalTested > 0 {
			report.TestedConfigs++
		}
		if result.TotalSuccess > 0 {
			report.WorkingConfigs++
		}
	}

	// Calculate site statistics
	for _, site := range sites {
		stats := SiteStatistics{}
		for _, result := range results {
			if siteResult, ok := result.SiteResults[site.Name]; ok {
				stats.TotalTested++
				if siteResult.Success {
					stats.TotalSuccess++
					stats.AccessibleVia = append(stats.AccessibleVia, result.ConfigValue[:min(50, len(result.ConfigValue))])
				}
			}
		}
		if stats.TotalTested > 0 {
			stats.SuccessRate = float64(stats.TotalSuccess) / float64(stats.TotalTested) * 100
		}
		report.SiteStatistics[site.Name] = stats
	}

	// Generate summary
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Tested %d configs\n", report.TotalConfigs))
	summary.WriteString(fmt.Sprintf("- Valid: %d\n", report.ValidConfigs))
	summary.WriteString(fmt.Sprintf("- Tested: %d\n", report.TestedConfigs))
	summary.WriteString(fmt.Sprintf("- Working: %d\n", report.WorkingConfigs))

	if report.ValidConfigs > 0 {
		summary.WriteString(fmt.Sprintf("- Valid rate: %.1f%%\n", float64(report.ValidConfigs)/float64(report.TotalConfigs)*100))
	}
	if report.WorkingConfigs > 0 {
		summary.WriteString(fmt.Sprintf("- Working rate: %.1f%%\n", float64(report.WorkingConfigs)/float64(report.ValidConfigs)*100))
	}

	report.Summary = summary.String()

	return report
}

// SaveReport saves the test report to a file
func SaveReport(report TestReport, path string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Write to temporary file first
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	// Rename to final path (atomic operation)
	return os.Rename(tmpPath, path)
}

// SaveMarkdownReport saves the test report as a markdown file
func SaveMarkdownReport(report TestReport, path string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	var sb strings.Builder

	// Header
	sb.WriteString("# Config Test Report\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", report.GeneratedAt.Format(time.RFC3339)))

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total configs**: %d\n", report.TotalConfigs))
	sb.WriteString(fmt.Sprintf("- **Valid configs**: %d\n", report.ValidConfigs))
	sb.WriteString(fmt.Sprintf("- **Tested configs**: %d\n", report.TestedConfigs))
	sb.WriteString(fmt.Sprintf("- **Working configs**: %d\n", report.WorkingConfigs))

	if report.ValidConfigs > 0 {
		sb.WriteString(fmt.Sprintf("- **Valid rate**: %.1f%%\n", float64(report.ValidConfigs)/float64(report.TotalConfigs)*100))
	}
	if report.WorkingConfigs > 0 && report.ValidConfigs > 0 {
		sb.WriteString(fmt.Sprintf("- **Working rate**: %.1f%%\n", float64(report.WorkingConfigs)/float64(report.ValidConfigs)*100))
	}
	sb.WriteString("\n")

	// Top Working Configs
	sb.WriteString("## Top Working Configs\n\n")
	sb.WriteString("| Rank | Config (short) | Type | Success Rate | Working Sites | Avg Latency |\n")
	sb.WriteString("|------|----------------|------|--------------|---------------|-------------|\n")

	workingConfigs := []ConfigTestResult{}
	for _, result := range report.ConfigResults {
		if result.TotalSuccess > 0 {
			workingConfigs = append(workingConfigs, result)
		}
	}

	// Sort by success rate
	sort.Slice(workingConfigs, func(i, j int) bool {
		iRate := float64(workingConfigs[i].TotalSuccess) / float64(max(workingConfigs[i].TotalTested, 1))
		jRate := float64(workingConfigs[j].TotalSuccess) / float64(max(workingConfigs[j].TotalTested, 1))
		return iRate > jRate
	})

	// Show top 10
	for i, result := range workingConfigs {
		if i >= 10 {
			break
		}
		successRate := float64(result.TotalSuccess) / float64(max(result.TotalTested, 1)) * 100
		workingSites := []string{}
		for site, siteResult := range result.SiteResults {
			if siteResult.Success {
				workingSites = append(workingSites, site)
			}
		}

		shortConfig := result.ConfigValue
		if len(shortConfig) > 40 {
			shortConfig = shortConfig[:40] + "..."
		}

		sb.WriteString(fmt.Sprintf("| %d | `%s` | %s | %.1f%% | %s | %v |\n",
			i+1, shortConfig, result.ConfigType, successRate, strings.Join(workingSites, ", "), result.AverageLatency))
	}
	sb.WriteString("\n")

	// Site Statistics
	sb.WriteString("## Site Accessibility\n\n")
	sb.WriteString("| Site | Category | Tested | Success | Success Rate |\n")
	sb.WriteString("|------|----------|--------|---------|--------------|\n")

	// Sort sites by name
	siteNames := []string{}
	for name := range report.SiteStatistics {
		siteNames = append(siteNames, name)
	}
	sort.Strings(siteNames)

	for _, name := range siteNames {
		stats := report.SiteStatistics[name]
		sb.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %.1f%% |\n",
			name, getSiteCategory(name, report.ConfigResults), stats.TotalTested, stats.TotalSuccess, stats.SuccessRate))
	}
	sb.WriteString("\n")

	// Invalid Configs
	invalidConfigs := []ConfigTestResult{}
	for _, result := range report.ConfigResults {
		if !result.IsValid {
			invalidConfigs = append(invalidConfigs, result)
		}
	}

	if len(invalidConfigs) > 0 {
		sb.WriteString("## Invalid Configs\n\n")
		sb.WriteString("| Config (short) | Error |\n")
		sb.WriteString("|----------------|-------|\n")

		for _, result := range invalidConfigs {
			shortConfig := result.ConfigValue
			if len(shortConfig) > 50 {
				shortConfig = shortConfig[:50] + "..."
			}
			sb.WriteString(fmt.Sprintf("| `%s` | %s |\n", shortConfig, result.ValidationErr))
		}
		sb.WriteString("\n")
	}

	// Write to file
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// getSiteCategory returns the category of a site by looking it up in the
// known target sites. Falls back to "unknown" when the site is not part of the
// default site list.
func getSiteCategory(siteName string, results []ConfigTestResult) string {
	for _, site := range DefaultTargetSites() {
		if site.Name == siteName {
			return site.Category
		}
	}
	return "unknown"
}

// LoadConfigsFromFile loads configs from a file
func LoadConfigsFromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Split by newlines and filter empty lines
	lines := strings.Split(string(data), "\n")
	configs := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			configs = append(configs, line)
		}
	}

	return configs, nil
}

// LoadConfigsFromSubscription fetches and parses configs from a subscription URL
func LoadConfigsFromSubscription(ctx context.Context, url string, client *http.Client) ([]string, error) {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "ConfigTester/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Try to decode as base64 first
	if decoded, ok := parser.DecodeBase64Text(string(body)); ok {
		body = []byte(decoded)
	}

	// Parse configs from the body
	configs, _ := parser.Extract(string(body), true)

	results := []string{}
	for _, config := range configs {
		results = append(results, config.Value)
	}

	return results, nil
}

// TestSubscription tests a subscription URL
func TestSubscription(ctx context.Context, subURL string, sites []TargetSite, settings TestSettings) (TestReport, error) {
	// Load configs from subscription
	configs, err := LoadConfigsFromSubscription(ctx, subURL, nil)
	if err != nil {
		return TestReport{}, fmt.Errorf("failed to load subscription: %w", err)
	}

	// Test all configs
	results, err := TestConfigs(ctx, configs, sites, settings, 0) // 0 means test all
	if err != nil {
		return TestReport{}, err
	}

	// Generate report
	report := GenerateReport(results, sites)
	report.Summary = fmt.Sprintf("Subscription: %s\n%s", subURL, report.Summary)

	return report, nil
}

// TestFile tests a local file containing configs
func TestFile(ctx context.Context, filePath string, sites []TargetSite, settings TestSettings) (TestReport, error) {
	// Load configs from file
	configs, err := LoadConfigsFromFile(filePath)
	if err != nil {
		return TestReport{}, fmt.Errorf("failed to load configs from file: %w", err)
	}

	// Test all configs
	results, err := TestConfigs(ctx, configs, sites, settings, 0) // 0 means test all
	if err != nil {
		return TestReport{}, err
	}

	// Generate report
	report := GenerateReport(results, sites)
	report.Summary = fmt.Sprintf("File: %s\n%s", filePath, report.Summary)

	return report, nil
}

// helper function
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
