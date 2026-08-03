package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/app"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/health"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/repository"
)

func main() {
	root := flag.String("root", ".", "project root containing config/")
	flag.Parse()
	if flag.NArg() != 1 {
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
	fmt.Fprintln(os.Stderr, "usage: v2collector [-root PATH] check-config|collect|scan-channels|revive-channels|check-sources")
	os.Exit(2)
}
func exitError(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }
