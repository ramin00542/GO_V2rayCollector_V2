// Package compare provides functionality for comparing test results over time
package compare

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/tester"
)

// ComparisonResult represents the result of comparing two test reports
type ComparisonResult struct {
	Report1Name string `json:"report1_name"`
	Report2Name string `json:"report2_name"`
	Time1       time.Time `json:"time1"`
	Time2       time.Time `json:"time2"`
	
	// Summary statistics
	ConfigsAdded     int `json:"configs_added"`
	ConfigsRemoved   int `json:"configs_removed"`
	ConfigsUnchanged int `json:"configs_unchanged"`
	
	// Performance changes
	ImprovedConfigs  []ConfigChange `json:"improved_configs"`
	DeclinedConfigs  []ConfigChange `json:"declined_configs"`
	NewConfigs       []string       `json:"new_configs"`
	LostConfigs      []string       `json:"lost_configs"`
	
	// Site accessibility changes
	SiteImprovements map[string]SiteChange `json:"site_improvements"`
	SiteDeclines     map[string]SiteChange `json:"site_declines"`
	
	// Overall trends
	OverallSuccessRateChange float64 `json:"overall_success_rate_change"`
	AverageLatencyChange     float64 `json:"average_latency_change"`
}

// ConfigChange represents a change in a config's performance
type ConfigChange struct {
	Config      string  `json:"config"`
	OldRate     float64 `json:"old_rate"`
	NewRate     float64 `json:"new_rate"`
	Change      float64 `json:"change"`
	OldLatency  float64 `json:"old_latency_ms"`
	NewLatency  float64 `json:"new_latency_ms"`
	LatencyDiff float64 `json:"latency_diff_ms"`
}

// SiteChange represents a change in site accessibility
type SiteChange struct {
	Site        string  `json:"site"`
	OldRate    float64 `json:"old_rate"`
	NewRate    float64 `json:"new_rate"`
	Change     float64 `json:"change"`
	OldAccessible int    `json:"old_accessible"`
	NewAccessible int    `json:"new_accessible"`
}

// Comparator provides comparison functionality
type Comparator struct {
	reportsDir string
}

// NewComparator creates a new comparator
func NewComparator(reportsDir string) *Comparator {
	return &Comparator{
		reportsDir: reportsDir,
	}
}

// CompareReports compares two test reports
func (c *Comparator) CompareReports(report1Path, report2Path string) (ComparisonResult, error) {
	// Load both reports
	report1, err := loadReport(report1Path)
	if err != nil {
		return ComparisonResult{}, fmt.Errorf("failed to load first report: %w", err)
	}
	
	report2, err := loadReport(report2Path)
	if err != nil {
		return ComparisonResult{}, fmt.Errorf("failed to load second report: %w", err)
	}
	
	// Extract report names
	report1Name := filepath.Base(report1Path)
	report2Name := filepath.Base(report2Path)
	
	// Create comparison result
	result := ComparisonResult{
		Report1Name:       report1Name,
		Report2Name:       report2Name,
		SiteImprovements: make(map[string]SiteChange),
		SiteDeclines:     make(map[string]SiteChange),
		ImprovedConfigs:  []ConfigChange{},
		DeclinedConfigs:  []ConfigChange{},
	}
	
	// Set times (try to extract from filename)
	result.Time1 = extractTimeFromFilename(report1Name)
	result.Time2 = extractTimeFromFilename(report2Name)
	
	// Create maps for quick lookup
	configMap1 := make(map[string]tester.ConfigTestResult)
	configMap2 := make(map[string]tester.ConfigTestResult)
	
	for _, config := range report1.ConfigResults {
		configMap1[config.ConfigValue] = config
	}
	
	for _, config := range report2.ConfigResults {
		configMap2[config.ConfigValue] = config
	}
	
	// Find new, lost, and common configs
	for _, config := range report2.ConfigResults {
		if _, exists := configMap1[config.ConfigValue]; !exists {
			result.NewConfigs = append(result.NewConfigs, config.ConfigValue)
		}
	}
	
	for _, config := range report1.ConfigResults {
		if _, exists := configMap2[config.ConfigValue]; !exists {
			result.LostConfigs = append(result.LostConfigs, config.ConfigValue)
		}
	}
	
	// Count unchanged configs
	commonConfigs := 0
	for _, config := range report1.ConfigResults {
		if _, exists := configMap2[config.ConfigValue]; exists {
			commonConfigs++
		}
	}
	result.ConfigsUnchanged = commonConfigs
	result.ConfigsAdded = len(report2.ConfigResults) - commonConfigs
	result.ConfigsRemoved = len(report1.ConfigResults) - commonConfigs
	
	// Compare common configs
	for configValue, config1 := range configMap1 {
		config2, exists := configMap2[configValue]
		if !exists {
			continue
		}
		
		// Calculate success rates
		oldRate := float64(config1.TotalSuccess) / float64(max(config1.TotalTested, 1)) * 100
		newRate := float64(config2.TotalSuccess) / float64(max(config2.TotalTested, 1)) * 100
		change := newRate - oldRate
		
		// Calculate average latency
		oldLatency := float64(config1.AverageLatency) / float64(time.Millisecond)
		newLatency := float64(config2.AverageLatency) / float64(time.Millisecond)
		latencyDiff := newLatency - oldLatency
		
		if change > 0 {
			result.ImprovedConfigs = append(result.ImprovedConfigs, ConfigChange{
				Config:      configValue,
				OldRate:     oldRate,
				NewRate:     newRate,
				Change:      change,
				OldLatency:  oldLatency,
				NewLatency:  newLatency,
				LatencyDiff: latencyDiff,
			})
		} else if change < 0 {
			result.DeclinedConfigs = append(result.DeclinedConfigs, ConfigChange{
				Config:      configValue,
				OldRate:     oldRate,
				NewRate:     newRate,
				Change:      change,
				OldLatency:  oldLatency,
				NewLatency:  newLatency,
				LatencyDiff: latencyDiff,
			})
		}
	}
	
	// Sort improved and declined configs by change
	sort.Slice(result.ImprovedConfigs, func(i, j int) bool {
		return result.ImprovedConfigs[i].Change > result.ImprovedConfigs[j].Change
	})
	sort.Slice(result.DeclinedConfigs, func(i, j int) bool {
		return result.DeclinedConfigs[i].Change < result.DeclinedConfigs[j].Change
	})
	
	// Compare site accessibility
	for site, stats1 := range report1.SiteStatistics {
		stats2, exists := report2.SiteStatistics[site]
		if !exists {
			continue
		}
		
		change := stats2.SuccessRate - stats1.SuccessRate
		
		if change > 0 {
			result.SiteImprovements[site] = SiteChange{
				Site:        site,
				OldRate:     stats1.SuccessRate,
				NewRate:     stats2.SuccessRate,
				Change:      change,
				OldAccessible: int(float64(stats1.TotalTested) * stats1.SuccessRate / 100),
				NewAccessible: int(float64(stats2.TotalTested) * stats2.SuccessRate / 100),
			}
		} else if change < 0 {
			result.SiteDeclines[site] = SiteChange{
				Site:        site,
				OldRate:     stats1.SuccessRate,
				NewRate:     stats2.SuccessRate,
				Change:      change,
				OldAccessible: int(float64(stats1.TotalTested) * stats1.SuccessRate / 100),
				NewAccessible: int(float64(stats2.TotalTested) * stats2.SuccessRate / 100),
			}
		}
	}
	
	// Calculate overall trends
	result.OverallSuccessRateChange = report2.WorkingConfigs/float64(max(report2.ValidConfigs, 1)) * 100 -
		report1.WorkingConfigs/float64(max(report1.ValidConfigs, 1)) * 100
	
	// Calculate average latency change
	var totalLatency1, totalLatency2 float64
	var count1, count2 int
	
	for _, config := range report1.ConfigResults {
		if config.TotalSuccess > 0 {
			totalLatency1 += float64(config.AverageLatency)
			count1++
		}
	}
	
	for _, config := range report2.ConfigResults {
		if config.TotalSuccess > 0 {
			totalLatency2 += float64(config.AverageLatency)
			count2++
		}
	}
	
	if count1 > 0 && count2 > 0 {
		avgLatency1 := totalLatency1 / float64(count1) / float64(time.Millisecond)
		avgLatency2 := totalLatency2 / float64(count2) / float64(time.Millisecond)
		result.AverageLatencyChange = avgLatency2 - avgLatency1
	}
	
	return result, nil
}

// loadReport loads a test report from file
func loadReport(path string) (tester.TestReport, error) {
	var report tester.TestReport
	
	data, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	
	if err := json.Unmarshal(data, &report); err != nil {
		return report, err
	}
	
	return report, nil
}

// extractTimeFromFilename tries to extract time from filename
func extractTimeFromFilename(filename string) time.Time {
	// Try to parse different filename formats
	// Example: config_test_20240101_120000.json
	
	// Remove extension
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	
	// Try to find timestamp in filename
	parts := strings.Split(base, "_")
	for i := len(parts) - 1; i >= 0; i-- {
		// Try to parse as timestamp
		if t, err := time.Parse("20060102_150405", parts[i]); err == nil {
			return t
		}
		if t, err := time.Parse("20060102", parts[i]); err == nil {
			return t
		}
	}
	
	// If no timestamp found, return current time
	return time.Now().UTC()
}

// CompareLatest compares the latest report with the previous one
func (c *Comparator) CompareLatest() (ComparisonResult, error) {
	// Find all report files
	files, err := filepath.Glob(filepath.Join(c.reportsDir, "config_test_*.json"))
	if err != nil {
		return ComparisonResult{}, err
	}
	
	if len(files) < 2 {
		return ComparisonResult{}, fmt.Errorf("need at least 2 reports to compare")
	}
	
	// Sort files by modification time (newest first)
	sort.Slice(files, func(i, j int) bool {
		infoI, _ := os.Stat(files[i])
		infoJ, _ := os.Stat(files[j])
		return infoI.ModTime().After(infoJ.ModTime())
	})
	
	// Compare the two latest reports
	return c.CompareReports(files[0], files[1])
}

// CompareAll compares all reports and returns trends
func (c *Comparator) CompareAll() ([]ComparisonResult, error) {
	// Find all report files
	files, err := filepath.Glob(filepath.Join(c.reportsDir, "config_test_*.json"))
	if err != nil {
		return nil, err
	}
	
	if len(files) < 2 {
		return nil, fmt.Errorf("need at least 2 reports to compare")
	}
	
	// Sort files by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		infoI, _ := os.Stat(files[i])
		infoJ, _ := os.Stat(files[j])
		return infoI.ModTime().Before(infoJ.ModTime())
	})
	
	// Compare consecutive reports
	var comparisons []ComparisonResult
	for i := 1; i < len(files); i++ {
		comparison, err := c.CompareReports(files[i-1], files[i])
		if err != nil {
			continue
		}
		comparisons = append(comparisons, comparison)
	}
	
	return comparisons, nil
}

// GenerateTrendReport generates a trend report from multiple comparisons
func (c *Comparator) GenerateTrendReport(comparisons []ComparisonResult) TrendReport {
	if len(comparisons) == 0 {
		return TrendReport{}
	}
	
	report := TrendReport{
		Comparisons: comparisons,
		StartTime:    comparisons[0].Time1,
		EndTime:      comparisons[len(comparisons)-1].Time2,
		Trends:       make(map[string]Trend),
	}
	
	// Calculate overall trends
	var totalConfigChange int
	var totalSuccessRateChange float64
	var totalLatencyChange float64
	
	for _, comp := range comparisons {
		totalConfigChange += comp.ConfigsAdded - comp.ConfigsRemoved
		totalSuccessRateChange += comp.OverallSuccessRateChange
		totalLatencyChange += comp.AverageLatencyChange
	}
	
	report.TotalConfigChange = totalConfigChange
	report.AverageSuccessRateChange = totalSuccessRateChange / float64(len(comparisons))
	report.AverageLatencyChange = totalLatencyChange / float64(len(comparisons))
	
	// Calculate trends for individual configs
	configTrends := make(map[string][]float64)
	for _, comp := range comparisons {
		for _, change := range comp.ImprovedConfigs {
			configTrends[change.Config] = append(configTrends[change.Config], change.Change)
		}
		for _, change := range comp.DeclinedConfigs {
			configTrends[change.Config] = append(configTrends[change.Config], change.Change)
		}
	}
	
	// Find most consistent configs
	for config, changes := range configTrends {
		var sum float64
		for _, change := range changes {
			sum += change
		}
		avgChange := sum / float64(len(changes))
		
		// Classify trend
		var trend string
		if avgChange > 5 {
			trend = "improving"
		} else if avgChange < -5 {
			trend = "declining"
		} else {
			trend = "stable"
		}
		
		report.Trends[config] = Trend{
			Config:   config,
			AverageChange: avgChange,
			Trend:    trend,
			DataPoints: len(changes),
		}
	}
	
	return report
}

// TrendReport represents a trend analysis report
type TrendReport struct {
	Comparisons   []ComparisonResult `json:"comparisons"`
	StartTime     time.Time            `json:"start_time"`
	EndTime       time.Time            `json:"end_time"`
	TotalConfigChange int               `json:"total_config_change"`
	AverageSuccessRateChange float64    `json:"average_success_rate_change"`
	AverageLatencyChange     float64    `json:"average_latency_change"`
	Trends                   map[string]Trend `json:"trends"`
}

// Trend represents the trend for a specific config
type Trend struct {
	Config        string  `json:"config"`
	AverageChange float64 `json:"average_change"`
	Trend         string  `json:"trend"` // "improving", "declining", "stable"
	DataPoints    int     `json:"data_points"`
}

// SaveComparison saves a comparison result to file
func (c *Comparator) SaveComparison(comparison ComparisonResult, path string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		return err
	}
	
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	
	return os.Rename(tmpPath, path)
}

// GenerateComparisonReport generates a human-readable comparison report
func (c *Comparator) GenerateComparisonReport(comparison ComparisonResult) string {
	var sb strings.Builder
	
	// Header
	sb.WriteString("# گزارش مقایسه\n\n")
	sb.WriteString(fmt.Sprintf("مقایسه: %s vs %s\n\n", comparison.Report1Name, comparison.Report2Name))
	sb.WriteString(fmt.Sprintf("- زمان اول: %s\n", comparison.Time1.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("- زمان دوم: %s\n\n", comparison.Time2.Format("2006-01-02 15:04:05")))
	
	// Summary
	sb.WriteString("## خلاصه\n\n")
	sb.WriteString(fmt.Sprintf("- کانفیگ‌های اضافه شده: %d\n", comparison.ConfigsAdded))
	sb.WriteString(fmt.Sprintf("- کانفیگ‌های حذف شده: %d\n", comparison.ConfigsRemoved))
	sb.WriteString(fmt.Sprintf("- کانفیگ‌های بدون تغییر: %d\n", comparison.ConfigsUnchanged))
	sb.WriteString(fmt.Sprintf("- تغییر نرخ موفقیت کلی: %.2f%%\n", comparison.OverallSuccessRateChange))
	sb.WriteString(fmt.Sprintf("- تغییر تاخیر متوسط: %.2fms\n\n", comparison.AverageLatencyChange))
	
	// New configs
	if len(comparison.NewConfigs) > 0 {
		sb.WriteString("## کانفیگ‌های جدید\n\n")
		for _, config := range comparison.NewConfigs {
			sb.WriteString(fmt.Sprintf("- `%s`\n", config))
		}
		sb.WriteString("\n")
	}
	
	// Lost configs
	if len(comparison.LostConfigs) > 0 {
		sb.WriteString("## کانفیگ‌های از دست رفته\n\n")
		for _, config := range comparison.LostConfigs {
			sb.WriteString(fmt.Sprintf("- `%s`\n", config))
		}
		sb.WriteString("\n")
	}
	
	// Improved configs
	if len(comparison.ImprovedConfigs) > 0 {
		sb.WriteString("## کانفیگ‌های بهبود یافته\n\n")
		sb.WriteString("| کانفیگ | نرخ قدیم | نرخ جدید | تغییر |\n")
		sb.WriteString("|--------|-----------|-----------|--------|\n")
		for _, change := range comparison.ImprovedConfigs {
			sb.WriteString(fmt.Sprintf("| `%s` | %.1f%% | %.1f%% | +%.1f%% |\n",
				change.Config, change.OldRate, change.NewRate, change.Change))
		}
		sb.WriteString("\n")
	}
	
	// Declined configs
	if len(comparison.DeclinedConfigs) > 0 {
		sb.WriteString("## کانفیگ‌های بدتر شده\n\n")
		sb.WriteString("| کانفیگ | نرخ قدیم | نرخ جدید | تغییر |\n")
		sb.WriteString("|--------|-----------|-----------|--------|\n")
		for _, change := range comparison.DeclinedConfigs {
			sb.WriteString(fmt.Sprintf("| `%s` | %.1f%% | %.1f%% | %.1f%% |\n",
				change.Config, change.OldRate, change.NewRate, change.Change))
		}
		sb.WriteString("\n")
	}
	
	// Site improvements
	if len(comparison.SiteImprovements) > 0 {
		sb.WriteString("## بهبود دسترسی به سایت‌ها\n\n")
		sb.WriteString("| سایت | نرخ قدیم | نرخ جدید | تغییر |\n")
		sb.WriteString("|------|-----------|-----------|--------|\n")
		for site, change := range comparison.SiteImprovements {
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %.1f%% | +%.1f%% |\n",
				site, change.OldRate, change.NewRate, change.Change))
		}
		sb.WriteString("\n")
	}
	
	// Site declines
	if len(comparison.SiteDeclines) > 0 {
		sb.WriteString("## کاهش دسترسی به سایت‌ها\n\n")
		sb.WriteString("| سایت | نرخ قدیم | نرخ جدید | تغییر |\n")
		sb.WriteString("|------|-----------|-----------|--------|\n")
		for site, change := range comparison.SiteDeclines {
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %.1f%% | %.1f%% |\n",
				site, change.OldRate, change.NewRate, change.Change))
		}
		sb.WriteString("\n")
	}
	
	return sb.String()
}

// helper function
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
