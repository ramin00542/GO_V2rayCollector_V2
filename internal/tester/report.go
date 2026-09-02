package tester

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReportConfig contains configuration for generating reports
type ReportConfig struct {
	OutputDir      string
	ReportName     string
	MarkdownReport bool
	JSONReport     bool
}

// GenerateAndSaveReport generates and saves a test report
func GenerateAndSaveReport(results []ConfigTestResult, sites []TargetSite, config ReportConfig) (TestReport, error) {
	report := GenerateReport(results, sites)

	// Save JSON report
	if config.JSONReport {
		jsonPath := filepath.Join(config.OutputDir, config.ReportName+".json")
		if err := SaveReport(report, jsonPath); err != nil {
			return TestReport{}, fmt.Errorf("failed to save JSON report: %w", err)
		}
	}

	// Save Markdown report
	if config.MarkdownReport {
		mdPath := filepath.Join(config.OutputDir, config.ReportName+".md")
		if err := SaveMarkdownReport(report, mdPath); err != nil {
			return TestReport{}, fmt.Errorf("failed to save Markdown report: %w", err)
		}
	}

	return report, nil
}

// SaveSubscriptionTestReport saves a report for a specific subscription
func SaveSubscriptionTestReport(report TestReport, subURL string, outputDir string) error {
	// Create a safe filename from the URL
	filename := sanitizeFilename(subURL)
	if filename == "" {
		filename = "subscription_test_" + time.Now().Format("20060102_150405")
	}

	// Save JSON report
	jsonPath := filepath.Join(outputDir, filename+".json")
	if err := SaveReport(report, jsonPath); err != nil {
		return err
	}

	// Save Markdown report
	mdPath := filepath.Join(outputDir, filename+".md")
	if err := SaveMarkdownReport(report, mdPath); err != nil {
		return err
	}

	return nil
}

// SaveFileTestReport saves a report for testing a specific file
func SaveFileTestReport(report TestReport, filePath string, outputDir string) error {
	// Create a safe filename from the file path
	baseName := filepath.Base(filePath)
	filename := strings.TrimSuffix(baseName, filepath.Ext(baseName)) + "_test"

	// Save JSON report
	jsonPath := filepath.Join(outputDir, filename+".json")
	if err := SaveReport(report, jsonPath); err != nil {
		return err
	}

	// Save Markdown report
	mdPath := filepath.Join(outputDir, filename+".md")
	if err := SaveMarkdownReport(report, mdPath); err != nil {
		return err
	}

	return nil
}

// SaveIndividualConfigReports saves individual reports for each config
func SaveIndividualConfigReports(results []ConfigTestResult, outputDir string) error {
	// Create configs directory
	configsDir := filepath.Join(outputDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return err
	}

	for _, result := range results {
		// Create a safe filename
		filename := sanitizeFilename(result.ConfigValue)
		if filename == "" {
			filename = fmt.Sprintf("config_%d", time.Now().UnixNano())
		}

		// Create a simple report for this config
		individualReport := map[string]interface{}{
			"config":         result.ConfigValue,
			"type":           result.ConfigType,
			"is_valid":       result.IsValid,
			"test_timestamp": result.TestTimestamp.Format(time.RFC3339),
			"total_success":  result.TotalSuccess,
			"total_failed":   result.TotalFailed,
			"total_tested":   result.TotalTested,
			"site_results":   result.SiteResults,
		}

		data, err := json.MarshalIndent(individualReport, "", "  ")
		if err != nil {
			continue
		}

		filePath := filepath.Join(configsDir, filename+".json")
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			continue
		}
	}

	return nil
}

// LoadTestReport loads a previously saved test report
func LoadTestReport(path string) (TestReport, error) {
	var report TestReport

	data, err := os.ReadFile(path)
	if err != nil {
		return TestReport{}, err
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return TestReport{}, err
	}

	return report, nil
}

// CompareReports compares two test reports and returns the differences
func CompareReports(oldReport, newReport TestReport) map[string]interface{} {
	comparison := map[string]interface{}{
		"old_total_configs":    oldReport.TotalConfigs,
		"new_total_configs":    newReport.TotalConfigs,
		"configs_added":         newReport.TotalConfigs - oldReport.TotalConfigs,
		"old_working_configs":  oldReport.WorkingConfigs,
		"new_working_configs":  newReport.WorkingConfigs,
		"working_configs_diff": newReport.WorkingConfigs - oldReport.WorkingConfigs,
	}

	// Find new working configs
	oldWorking := make(map[string]bool)
	for _, result := range oldReport.ConfigResults {
		if result.TotalSuccess > 0 {
			oldWorking[result.ConfigValue] = true
		}
	}

	newWorking := make(map[string]bool)
	for _, result := range newReport.ConfigResults {
		if result.TotalSuccess > 0 {
			newWorking[result.ConfigValue] = true
		}
	}

	// Find configs that are now working
	nowWorking := []string{}
	for config, working := range newWorking {
		if working && !oldWorking[config] {
			nowWorking = append(nowWorking, config)
		}
	}
	comparison["now_working"] = nowWorking

	// Find configs that stopped working
	stoppedWorking := []string{}
	for config, working := range oldWorking {
		if working && !newWorking[config] {
			stoppedWorking = append(stoppedWorking, config)
		}
	}
	comparison["stopped_working"] = stoppedWorking

	return comparison
}

// GenerateComparisonReport generates a markdown comparison report
func GenerateComparisonReport(oldReport, newReport TestReport, path string) error {
	comparison := CompareReports(oldReport, newReport)

	var sb strings.Builder

	sb.WriteString("# Config Test Report Comparison\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Old | New | Difference |\n")
	sb.WriteString("|--------|-----|-----|------------|\n")
	sb.WriteString(fmt.Sprintf("| Total Configs | %d | %d | %d |\n",
		comparison["old_total_configs"], comparison["new_total_configs"], comparison["configs_added"]))
	sb.WriteString(fmt.Sprintf("| Working Configs | %d | %d | %d |\n",
		comparison["old_working_configs"], comparison["new_working_configs"], comparison["working_configs_diff"]))

	if nowWorking, ok := comparison["now_working"].([]string); ok && len(nowWorking) > 0 {
		sb.WriteString("\n## Now Working\n\n")
		sb.WriteString("Configs that are now working:\n\n")
		for _, config := range nowWorking {
			sb.WriteString(fmt.Sprintf("- `%s`\n", config))
		}
	}

	if stoppedWorking, ok := comparison["stopped_working"].([]string); ok && len(stoppedWorking) > 0 {
		sb.WriteString("\n## Stopped Working\n\n")
		sb.WriteString("Configs that stopped working:\n\n")
		for _, config := range stoppedWorking {
			sb.WriteString(fmt.Sprintf("- `%s`\n", config))
		}
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// GenerateDailyReport generates a daily summary report
func GenerateDailyReport(reports []TestReport, path string) error {
	if len(reports) == 0 {
		return fmt.Errorf("no reports to generate daily summary")
	}

	var sb strings.Builder

	sb.WriteString("# Daily Config Test Summary\n\n")
	sb.WriteString(fmt.Sprintf("Date: %s\n\n", time.Now().Format("2006-01-02")))

	// Collect all data
	totalConfigs := 0
	totalValid := 0
	totalWorking := 0
	configSuccessRates := make(map[string][]float64)

	for _, report := range reports {
		totalConfigs += report.TotalConfigs
		totalValid += report.ValidConfigs
		totalWorking += report.WorkingConfigs

		// Track success rates for each config
		for _, result := range report.ConfigResults {
			if result.TotalTested > 0 {
				rate := float64(result.TotalSuccess) / float64(result.TotalTested) * 100
				configSuccessRates[result.ConfigValue] = append(configSuccessRates[result.ConfigValue], rate)
			}
		}
	}

	// Summary
	sb.WriteString("## Overall Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Tests Run**: %d\n", len(reports)))
	sb.WriteString(fmt.Sprintf("- **Total Configs Tested**: %d\n", totalConfigs))
	sb.WriteString(fmt.Sprintf("- **Valid Configs**: %d (%.1f%%)\n",
		totalValid, float64(totalValid)/float64(max(totalConfigs, 1))*100))
	sb.WriteString(fmt.Sprintf("- **Working Configs**: %d (%.1f%%)\n",
		totalWorking, float64(totalWorking)/float64(max(totalValid, 1))*100))

	// Most consistent configs
	sb.WriteString("\n## Most Consistent Configs\n\n")
	sb.WriteString("| Rank | Config (short) | Avg Success Rate | Tests |\n")
	sb.WriteString("|------|----------------|------------------|-------|\n")

	// Calculate average success rate for each config
	type configStats struct {
		config   string
		avgRate  float64
		count    int
	}
	stats := []configStats{}
	for config, rates := range configSuccessRates {
		var sum float64
		for _, rate := range rates {
			sum += rate
		}
		avgRate := sum / float64(len(rates))
		stats = append(stats, configStats{config: config, avgRate: avgRate, count: len(rates)})
	}

	// Sort by average rate
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].avgRate > stats[j].avgRate
	})

	// Show top 10
	for i, stat := range stats {
		if i >= 10 {
			break
		}
		shortConfig := stat.config
		if len(shortConfig) > 40 {
			shortConfig = shortConfig[:40] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %d | `%s` | %.1f%% | %d |\n",
			i+1, shortConfig, stat.avgRate, stat.count))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// sanitizeFilename creates a safe filename from a string
func sanitizeFilename(input string) string {
	// Remove invalid characters
	input = strings.ReplaceAll(input, "/", "_")
	input = strings.ReplaceAll(input, "\\", "_")
	input = strings.ReplaceAll(input, ":", "_")
	input = strings.ReplaceAll(input, "*", "_")
	input = strings.ReplaceAll(input, "?", "_")
	input = strings.ReplaceAll(input, "\"", "_")
	input = strings.ReplaceAll(input, "<", "_")
	input = strings.ReplaceAll(input, ">", "_")
	input = strings.ReplaceAll(input, "|", "_")

	// Trim to reasonable length
	if len(input) > 100 {
		input = input[:100]
	}

	// Remove leading/trailing spaces and dots
	input = strings.TrimSpace(input)
	input = strings.Trim(input, ".")

	if input == "" {
		return ""
	}

	return input
}

// GetReportPath returns the path for a report based on timestamp
func GetReportPath(outputDir string, reportType string) string {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("config_test_%s_%s", reportType, timestamp)
	return filepath.Join(outputDir, filename)
}

// GetSubscriptionReportPath returns the path for a subscription report
func GetSubscriptionReportPath(outputDir string, subURL string) string {
	filename := sanitizeFilename(subURL)
	if filename == "" {
		filename = "subscription_" + time.Now().Format("20060102_150405")
	}
	timestamp := time.Now().Format("20060102_150405")
	return filepath.Join(outputDir, fmt.Sprintf("%s_%s", filename, timestamp))
}
