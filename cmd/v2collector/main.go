package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/app"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/cdn"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/compare"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/health"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/logging"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/notification"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/repository"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/tester"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/updater"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/web"
)

func main() {
	root := flag.String("root", ".", "project root containing config/")
	flag.Parse()
	if flag.NArg() < 1 {
		usage()
	}
	paths := config.DefaultPaths(*root)
	switch flag.Arg(0) {
	case "check-config":
		checkConfig(paths)
	case "collect":
		collect(paths)
	case "scan-channels":
		healthCheck(paths, false)
	case "revive-channels":
		healthCheck(paths, true)
	case "check-sources":
		sourceCheck(paths)
	case "test-configs":
		testConfigs(paths, flag.Args()[1:]...)
	case "test-subscription":
		if flag.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "error: test-subscription requires a URL argument")
			os.Exit(1)
		}
		testSubscription(paths, flag.Arg(1))
	case "test-file":
		if flag.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "error: test-file requires a file path argument")
			os.Exit(1)
		}
		testFile(paths, flag.Arg(1))
	case "test-manual":
		if flag.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "error: test-manual requires a config or subscription URL")
			os.Exit(1)
		}
		testManual(paths, flag.Arg(1))
	case "web":
		runWebServer(paths)
	case "update":
		runUpdater(paths)
	case "compare":
		if flag.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "error: compare requires at least one report file")
			os.Exit(1)
		}
		runCompare(paths, flag.Args()[1:])
	case "upload-cdn":
		runCDNUpload(paths)
	default:
		usage()
	}
}

func checkConfig(paths config.Paths) {
	channels, err := repository.LoadChannels(paths.ChannelsFile())
	if err != nil {
		exitError(err)
	}
	sources, err := repository.LoadSources(paths.SourcesFile())
	if err != nil {
		exitError(err)
	}
	settings, err := repository.LoadCollectorSettings(paths.SettingsFile())
	if err != nil {
		exitError(err)
	}
	github, err := repository.LoadGitHubSettings(paths.GitHubFile())
	if err != nil {
		exitError(err)
	}
	fmt.Printf("configuration valid: channels=%d sources=%d daily_retention=%d rolling_retention=%d github_enabled=%t\n", len(channels), len(sources), settings.Retention.DailyDays, settings.Retention.RollingDays, github.Enabled)
}

func collect(paths config.Paths) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := app.Collect(ctx, paths, time.Now().UTC())
	if err != nil {
		exitError(err)
	}
	fmt.Printf("collection complete: new=%d requests=%d succeeded=%d failed=%d accepted=%d rejected=%d\n", result.NewConfigs, result.Summary.Requests, result.Summary.Succeeded, result.Summary.Failed, result.Summary.Accepted, result.Summary.Rejected)
}

func healthCheck(paths config.Paths, reviveOnly bool) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	results, err := health.CheckChannels(ctx, paths, reviveOnly, time.Now().UTC())
	if err != nil {
		exitError(err)
	}
	title := "Channel scan"
	if reviveOnly {
		title = "Channel revive scan"
	}
	if err := os.MkdirAll(paths.ReportsDir, 0755); err != nil {
		exitError(err)
	}
	if err := os.WriteFile(paths.ReportsDir+"/channels-health.md", []byte(health.FormatReport(title, results)), 0644); err != nil {
		exitError(err)
	}
	fmt.Printf("%s complete: checked=%d\n", title, len(results))
}
func sourceCheck(paths config.Paths) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	results, err := health.CheckSources(ctx, paths, time.Now().UTC())
	if err != nil {
		exitError(err)
	}
	if err := os.MkdirAll(paths.ReportsDir, 0755); err != nil {
		exitError(err)
	}
	if err := os.WriteFile(paths.ReportsDir+"/sources-health.md", []byte(health.FormatReport("Source health check", results)), 0644); err != nil {
		exitError(err)
	}
	fmt.Printf("source health check complete: checked=%d\n", len(results))
}
func usage() {
	fmt.Fprintln(os.Stderr, `usage: v2collector [-root PATH] <command> [args]

Commands:
  check-config       Validate configuration files
  collect           Collect configs from all sources
  scan-channels     Check health of all Telegram channels
  revive-channels    Check and revive inactive Telegram channels
  check-sources      Check health of all subscription sources
  test-configs       Test all collected configs against target sites
  test-subscription  Test a specific subscription URL
  test-file          Test configs from a local file
  test-manual        Test a single config or subscription URL manually
  web               Start the web dashboard (use -host and -port flags)
  update            Run subscription updater
  compare           Compare test reports
  upload-cdn        Upload configs to CDN`)
	os.Exit(2)
}
func exitError(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }

func runUpdater(paths config.Paths) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load updater configuration
	updaterConfig, err := updater.LoadUpdaterConfig(filepath.Join(paths.ConfigDir, "updater.json"))
	if err != nil {
		exitError(err)
	}

	// Create logger
	logger := logging.NewLogger()
	logger.SetLevel(logging.LevelInfo)

	// Create notifier (optional)
	notifier, err := notification.CreateNotifiersFromConfig(filepath.Join(paths.ConfigDir, "notifiers.json"))
	if err != nil {
		logger.Warn("failed to create notifier", "error", err)
		notifier = []notification.Notifier{&notification.NopNotifier{}}
	}
	multiNotifier := notification.NewMultiNotifier(notifier...)

	// Create updater
	updater, err := updater.NewUpdater(updaterConfig, paths, logger, multiNotifier)
	if err != nil {
		exitError(err)
	}

	// Start updater
	fmt.Println("Starting subscription updater...")
	fmt.Println("Press Ctrl+C to stop")

	if err := updater.Start(ctx); err != nil {
		exitError(err)
	}

	// Wait for interrupt
	<-ctx.Done()

	// Stop updater
	updater.Stop()
	fmt.Println("Updater stopped")
}

func runCompare(paths config.Paths, args []string) {
	// Create comparator
	comparator := compare.NewComparator(paths.ReportsDir)

	if len(args) == 0 {
		// Compare latest reports
		comparison, err := comparator.CompareLatest()
		if err != nil {
			exitError(err)
		}

		// Print comparison
		fmt.Println(comparator.GenerateComparisonReport(comparison))

		// Save comparison
		timestamp := time.Now().Format("20060102_150405")
		comparisonPath := filepath.Join(paths.ReportsDir, fmt.Sprintf("comparison_%s.md", timestamp))
		if err := comparator.SaveComparison(comparison, comparisonPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to save comparison: %v\n", err)
		} else {
			fmt.Printf("Comparison saved to: %s\n", comparisonPath)
		}

	} else if len(args) == 1 {
		// Compare with latest
		reportPath := filepath.Join(paths.ReportsDir, args[0])
		comparison, err := comparator.CompareReports(reportPath, "")
		if err != nil {
			exitError(err)
		}

		fmt.Println(comparator.GenerateComparisonReport(comparison))

	} else if len(args) >= 2 {
		// Compare two specific reports
		reportPath1 := filepath.Join(paths.ReportsDir, args[0])
		reportPath2 := filepath.Join(paths.ReportsDir, args[1])

		comparison, err := comparator.CompareReports(reportPath1, reportPath2)
		if err != nil {
			exitError(err)
		}

		fmt.Println(comparator.GenerateComparisonReport(comparison))

		// Save comparison
		timestamp := time.Now().Format("20060102_150405")
		comparisonPath := filepath.Join(paths.ReportsDir, fmt.Sprintf("comparison_%s_%s_%s.md", args[0], args[1], timestamp))
		if err := comparator.SaveComparison(comparison, comparisonPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to save comparison: %v\n", err)
		} else {
			fmt.Printf("Comparison saved to: %s\n", comparisonPath)
		}
	}
}

func runCDNUpload(paths config.Paths) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load CDN configuration
	cdnConfig, err := cdn.LoadCDNConfig(filepath.Join(paths.ConfigDir, "cdn.json"))
	if err != nil {
		exitError(err)
	}

	// Create logger
	logger := logging.NewLogger()
	logger.SetLevel(logging.LevelInfo)

	// Create CDN manager
	cdnManager, err := cdn.NewCDNManager(cdnConfig, logger)
	if err != nil {
		exitError(err)
	}

	// Load all configs
	configFiles := []string{
		filepath.Join(paths.OutputDir, "temporary", "telegram", "protocols"),
		filepath.Join(paths.OutputDir, "temporary", "subscription", "protocols"),
		filepath.Join(paths.ArchiveDir, "all"),
	}

	var allConfigs []string
	for _, dir := range configFiles {
		configs, err := loadConfigsFromDirectory(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load configs from %s: %v\n", dir, err)
			continue
		}
		allConfigs = append(allConfigs, configs...)
	}

	// Remove duplicates
	allConfigs = removeDuplicateConfigs(allConfigs)

	if len(allConfigs) == 0 {
		exitError(fmt.Errorf("no configs found to upload"))
	}

	fmt.Printf("Uploading %d configs to CDN...\n", len(allConfigs))

	// Upload all configs
	files, err := cdnManager.UploadAllConfigs(ctx, allConfigs)
	if err != nil {
		exitError(err)
	}

	fmt.Printf("Uploaded %d files to CDN\n", len(files))

	// Generate subscription link
	if len(files) > 0 {
		subscriptionURL, err := cdnManager.GenerateSubscriptionLink(ctx, allConfigs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to generate subscription link: %v\n", err)
		} else {
			fmt.Printf("Subscription URL: %s\n", subscriptionURL)
		}
	}

	// List all files on CDN
	cdnFiles, err := cdnManager.ListFiles(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to list CDN files: %v\n", err)
	} else {
		fmt.Printf("\nFiles on CDN:\n")
		for _, file := range cdnFiles {
			fmt.Printf("  - %s (%d bytes)\n", file.Name, file.Size)
		}
	}
}

func runWebServer(paths config.Paths) {
	// Parse additional flags for web server
	webFlags := flag.NewFlagSet("web", flag.ExitOnError)
	host := webFlags.String("host", "localhost", "host to bind the web server to")
	port := webFlags.Int("port", 8080, "port to bind the web server to")

	// Parse flags
	if err := webFlags.Parse(flag.Args()[1:]); err != nil {
		exitError(err)
	}

	fmt.Printf("Starting web server on %s:%d\n", *host, *port)
	fmt.Println("Press Ctrl+C to stop")

	// Create context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start web server
	server := web.NewServer(web.ServerConfig{
		Host:   *host,
		Port:   *port,
		Paths:  paths,
		Logger: logging.GetGlobalLogger(),
	})

	if err := server.Start(); err != nil {
		exitError(err)
	}

	// Wait for interrupt signal
	<-ctx.Done()

	// Stop server
	if err := server.Stop(); err != nil {
		exitError(err)
	}
}

func testConfigs(paths config.Paths, args ...string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load target sites configuration
	targetSitesConfig, err := tester.LoadTargetSites(filepath.Join(paths.ConfigDir, "target_sites.json"))
	if err != nil {
		exitError(err)
	}

	// Load all configs from output directory
	configFiles := []string{
		filepath.Join(paths.OutputDir, "temporary", "telegram", "protocols"),
		filepath.Join(paths.OutputDir, "temporary", "subscription", "protocols"),
		filepath.Join(paths.ArchiveDir, "all"),
	}

	var allConfigs []string
	for _, dir := range configFiles {
		configs, err := loadConfigsFromDirectory(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load configs from %s: %v\n", dir, err)
			continue
		}
		allConfigs = append(allConfigs, configs...)
	}

	// Remove duplicates
	allConfigs = removeDuplicateConfigs(allConfigs)

	if len(allConfigs) == 0 {
		exitError(fmt.Errorf("no configs found in output directories"))
	}

	fmt.Printf("Testing %d unique configs against %d target sites...\n", len(allConfigs), len(targetSitesConfig.Sites))

	// Determine max configs to test based on arguments
	maxConfigs := 0 // 0 means test all
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &maxConfigs)
	}

	// Test configs
	results, err := tester.TestConfigs(ctx, allConfigs, targetSitesConfig.Sites, targetSitesConfig.TestSettings, maxConfigs)
	if err != nil {
		exitError(err)
	}

	// Create reports directory if it doesn't exist
	if err := os.MkdirAll(paths.ReportsDir, 0755); err != nil {
		exitError(err)
	}

	// Save reports
	reportConfig := tester.ReportConfig{
		OutputDir:      paths.ReportsDir,
		ReportName:     "config_test_" + time.Now().Format("20060102_150405"),
		MarkdownReport: true,
		JSONReport:     true,
	}
	report, err := tester.GenerateAndSaveReport(results, targetSitesConfig.Sites, reportConfig)
	if err != nil {
		exitError(err)
	}

	fmt.Printf("Total: %d configs, valid: %d, working: %d\n",
		report.TotalConfigs, report.ValidConfigs, report.WorkingConfigs)

	// Save individual config results
	if err := tester.SaveIndividualConfigReports(results, filepath.Join(paths.ReportsDir, "individual")); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to save individual config reports: %v\n", err)
	}

	// Print summary
	fmt.Printf("\nTest complete!\n")
	fmt.Printf("Total configs: %d\n", report.TotalConfigs)
	fmt.Printf("Valid configs: %d\n", report.ValidConfigs)
	fmt.Printf("Tested configs: %d\n", report.TestedConfigs)
	fmt.Printf("Working configs: %d\n", report.WorkingConfigs)
	fmt.Printf("Reports saved to: %s\n", paths.ReportsDir)
}

func testSubscription(paths config.Paths, subURL string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load target sites configuration
	targetSitesConfig, err := tester.LoadTargetSites(filepath.Join(paths.ConfigDir, "target_sites.json"))
	if err != nil {
		exitError(err)
	}

	fmt.Printf("Testing subscription: %s\n", subURL)

	// Test the subscription
	report, err := tester.TestSubscription(ctx, subURL, targetSitesConfig.Sites, targetSitesConfig.TestSettings)
	if err != nil {
		exitError(err)
	}

	// Create reports directory if it doesn't exist
	if err := os.MkdirAll(paths.ReportsDir, 0755); err != nil {
		exitError(err)
	}

	// Save reports
	if err := tester.SaveSubscriptionTestReport(report, subURL, paths.ReportsDir); err != nil {
		exitError(err)
	}

	// Print summary
	fmt.Printf("\nTest complete for %s!\n", subURL)
	fmt.Printf("Total configs: %d\n", report.TotalConfigs)
	fmt.Printf("Valid configs: %d\n", report.ValidConfigs)
	fmt.Printf("Working configs: %d\n", report.WorkingConfigs)
	fmt.Printf("Reports saved to: %s\n", paths.ReportsDir)
}

func testFile(paths config.Paths, filePath string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load target sites configuration
	targetSitesConfig, err := tester.LoadTargetSites(filepath.Join(paths.ConfigDir, "target_sites.json"))
	if err != nil {
		exitError(err)
	}

	fmt.Printf("Testing file: %s\n", filePath)

	// Test the file
	report, err := tester.TestFile(ctx, filePath, targetSitesConfig.Sites, targetSitesConfig.TestSettings)
	if err != nil {
		exitError(err)
	}

	// Create reports directory if it doesn't exist
	if err := os.MkdirAll(paths.ReportsDir, 0755); err != nil {
		exitError(err)
	}

	// Save reports
	if err := tester.SaveFileTestReport(report, filePath, paths.ReportsDir); err != nil {
		exitError(err)
	}

	// Print summary
	fmt.Printf("\nTest complete for %s!\n", filePath)
	fmt.Printf("Total configs: %d\n", report.TotalConfigs)
	fmt.Printf("Valid configs: %d\n", report.ValidConfigs)
	fmt.Printf("Working configs: %d\n", report.WorkingConfigs)
	fmt.Printf("Reports saved to: %s\n", paths.ReportsDir)
}

func testManual(paths config.Paths, input string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load target sites configuration
	targetSitesConfig, err := tester.LoadTargetSites(filepath.Join(paths.ConfigDir, "target_sites.json"))
	if err != nil {
		exitError(err)
	}

	// Check if input is a URL (subscription) or a config
	var configs []string
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		// It's a subscription URL
		fmt.Printf("Loading configs from subscription: %s\n", input)
		configsFromSub, err := tester.LoadConfigsFromSubscription(ctx, input, nil)
		if err != nil {
			exitError(err)
		}
		configs = configsFromSub
		fmt.Printf("Loaded %d configs from subscription\n", len(configs))
	} else {
		// It's a single config
		configs = []string{input}
		fmt.Printf("Testing single config: %s\n", input)
	}

	if len(configs) == 0 {
		exitError(fmt.Errorf("no configs found"))
	}

	// Test configs
	results, err := tester.TestConfigs(ctx, configs, targetSitesConfig.Sites, targetSitesConfig.TestSettings, 0)
	if err != nil {
		exitError(err)
	}

	// Create reports directory if it doesn't exist
	if err := os.MkdirAll(paths.ReportsDir, 0755); err != nil {
		exitError(err)
	}

	// Save reports
	timestamp := time.Now().Format("20060102_150405")
	reportConfig := tester.ReportConfig{
		OutputDir:      paths.ReportsDir,
		ReportName:     fmt.Sprintf("manual_test_%s_%s", timestamp, sanitizeFilename(input)),
		MarkdownReport: true,
		JSONReport:     true,
	}
	report, err := tester.GenerateAndSaveReport(results, targetSitesConfig.Sites, reportConfig)
	if err != nil {
		exitError(err)
	}

	fmt.Printf("Total: %d configs, valid: %d, working: %d\n",
		report.TotalConfigs, report.ValidConfigs, report.WorkingConfigs)

	// Print results
	fmt.Printf("\n=== Test Results ===\n\n")
	for _, result := range results {
		fmt.Printf("Config: %s\n", result.ConfigValue)
		fmt.Printf("Type: %s\n", result.ConfigType)
		fmt.Printf("Valid: %v\n", result.IsValid)
		if !result.IsValid {
			fmt.Printf("Error: %s\n", result.ValidationErr)
		} else {
			fmt.Printf("Success: %d/%d sites\n", result.TotalSuccess, result.TotalTested)
			for site, siteResult := range result.SiteResults {
				status := "✅"
				if !siteResult.Success {
					status = "❌"
				}
				fmt.Printf("  %s %s (%.0fms)\n", status, site, float64(siteResult.Latency)/float64(time.Millisecond))
			}
		}
		fmt.Println()
	}

	fmt.Printf("Reports saved to: %s\n", paths.ReportsDir)
}

// Helper functions
func loadConfigsFromDirectory(dir string) ([]string, error) {
	var configs []string

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	// Walk through the directory
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".txt") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					configs = append(configs, line)
				}
			}
		}
		return nil
	})

	return configs, err
}

func removeDuplicateConfigs(configs []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, config := range configs {
		if !seen[config] {
			seen[config] = true
			result = append(result, config)
		}
	}
	return result
}

func sanitizeFilename(input string) string {
	input = strings.ReplaceAll(input, "/", "_")
	input = strings.ReplaceAll(input, "\\", "_")
	input = strings.ReplaceAll(input, ":", "_")
	input = strings.ReplaceAll(input, "*", "_")
	input = strings.ReplaceAll(input, "?", "_")
	input = strings.ReplaceAll(input, "\"", "_")
	input = strings.ReplaceAll(input, "<", "_")
	input = strings.ReplaceAll(input, ">", "_")
	input = strings.ReplaceAll(input, "|", "_")
	if len(input) > 100 {
		input = input[:100]
	}
	input = strings.TrimSpace(input)
	input = strings.Trim(input, ".")
	return input
}
