package tester

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SubscriptionWatcher watches subscription URLs and tests them periodically
type SubscriptionWatcher struct {
	SubURL       string
	OutputDir    string
	TargetSites  []TargetSite
	TestSettings TestSettings
	Interval     time.Duration
	LastTestTime time.Time
	LastReport   TestReport
	StopChan     chan struct{}
	Wg           sync.WaitGroup
	Mu           sync.Mutex
}

// NewSubscriptionWatcher creates a new subscription watcher
func NewSubscriptionWatcher(subURL, outputDir string, sites []TargetSite, settings TestSettings, interval time.Duration) *SubscriptionWatcher {
	return &SubscriptionWatcher{
		SubURL:       subURL,
		OutputDir:    outputDir,
		TargetSites:  sites,
		TestSettings: settings,
		Interval:     interval,
		StopChan:     make(chan struct{}),
	}
}

// Start starts the watcher
func (w *SubscriptionWatcher) Start(ctx context.Context) error {
	w.Wg.Add(1)
	go func() {
		defer w.Wg.Done()

		// Initial test
		if err := w.testOnce(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "initial test failed: %v\n", err)
		}

		// Periodic testing
		ticker := time.NewTicker(w.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-w.StopChan:
				return
			case <-ticker.C:
				if err := w.testOnce(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "periodic test failed: %v\n", err)
				}
			}
		}
	}()

	return nil
}

// Stop stops the watcher
func (w *SubscriptionWatcher) Stop() {
	close(w.StopChan)
	w.Wg.Wait()
}

// testOnce performs a single test
func (w *SubscriptionWatcher) testOnce(ctx context.Context) error {
	w.Mu.Lock()
	defer w.Mu.Unlock()

	w.LastTestTime = time.Now().UTC()

	// Test the subscription
	report, err := TestSubscription(ctx, w.SubURL, w.TargetSites, w.TestSettings)
	if err != nil {
		return err
	}

	w.LastReport = report

	// Save report
	filename := sanitizeFilename(w.SubURL)
	if filename == "" {
		filename = "subscription_" + w.LastTestTime.Format("20060102_150405")
	}

	// Save to subscription-specific directory
	subDir := filepath.Join(w.OutputDir, "subscriptions", filename)
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return err
	}

	// Save JSON report
	jsonPath := filepath.Join(subDir, w.LastTestTime.Format("20060102_150405")+".json")
	if err := SaveReport(report, jsonPath); err != nil {
		return err
	}

	// Save Markdown report
	mdPath := filepath.Join(subDir, w.LastTestTime.Format("20060102_150405")+".md")
	if err := SaveMarkdownReport(report, mdPath); err != nil {
		return err
	}

	// Save latest report
	latestJSONPath := filepath.Join(subDir, "latest.json")
	latestMDPath := filepath.Join(subDir, "latest.md")

	// Create symlinks or copy files
	if err := copyFile(jsonPath, latestJSONPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update latest JSON: %v\n", err)
	}
	if err := copyFile(mdPath, latestMDPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update latest MD: %v\n", err)
	}

	return nil
}

// GetLastReport returns the last test report
func (w *SubscriptionWatcher) GetLastReport() TestReport {
	w.Mu.Lock()
	defer w.Mu.Unlock()
	return w.LastReport
}

// GetLastTestTime returns the time of the last test
func (w *SubscriptionWatcher) GetLastTestTime() time.Time {
	w.Mu.Lock()
	defer w.Mu.Unlock()
	return w.LastTestTime
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// WatchSubscriptions watches multiple subscriptions
func WatchSubscriptions(ctx context.Context, subscriptions []string, outputDir string, sites []TargetSite, settings TestSettings, interval time.Duration) []*SubscriptionWatcher {
	watchers := []*SubscriptionWatcher{}

	for _, subURL := range subscriptions {
		watcher := NewSubscriptionWatcher(subURL, outputDir, sites, settings, interval)
		if err := watcher.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to start watcher for %s: %v\n", subURL, err)
			continue
		}
		watchers = append(watchers, watcher)
	}

	return watchers
}

// StopAllWatchers stops all watchers
func StopAllWatchers(watchers []*SubscriptionWatcher) {
	for _, watcher := range watchers {
		watcher.Stop()
	}
}

// TestAndWatchSubscription tests a subscription and sets up a watcher
func TestAndWatchSubscription(ctx context.Context, subURL, outputDir string, sites []TargetSite, settings TestSettings, interval time.Duration) (*SubscriptionWatcher, error) {
	// Create a safe directory name from the URL
	safeName := sanitizeFilename(subURL)
	if safeName == "" {
		safeName = "subscription_" + time.Now().Format("20060102_150405")
	}

	subDir := filepath.Join(outputDir, "subscriptions", safeName)
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return nil, err
	}

	// Test once immediately
	fmt.Printf("Testing subscription: %s\n", subURL)
	report, err := TestSubscription(ctx, subURL, sites, settings)
	if err != nil {
		return nil, err
	}

	// Save initial report
	timestamp := time.Now().UTC().Format("20060102_150405")
	jsonPath := filepath.Join(subDir, timestamp+".json")
	mdPath := filepath.Join(subDir, timestamp+".md")

	if err := SaveReport(report, jsonPath); err != nil {
		return nil, err
	}
	if err := SaveMarkdownReport(report, mdPath); err != nil {
		return nil, err
	}

	// Create latest symlinks
	latestJSON := filepath.Join(subDir, "latest.json")
	latestMD := filepath.Join(subDir, "latest.md")

	if err := os.Symlink(jsonPath, latestJSON); err != nil {
		// If symlink fails, copy the file
		if err := copyFile(jsonPath, latestJSON); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create latest symlink: %v\n", err)
		}
	}
	if err := os.Symlink(mdPath, latestMD); err != nil {
		if err := copyFile(mdPath, latestMD); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create latest symlink: %v\n", err)
		}
	}

	// Start watcher if interval > 0
	if interval > 0 {
		watcher := NewSubscriptionWatcher(subURL, subDir, sites, settings, interval)
		if err := watcher.Start(ctx); err != nil {
			return nil, err
		}
		return watcher, nil
	}

	return nil, nil
}

// LoadSubscriptionsFromFile loads subscription URLs from a file
func LoadSubscriptionsFromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	subscriptions := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			subscriptions = append(subscriptions, line)
		}
	}

	return subscriptions, nil
}

// SaveSubscriptionResults saves results for all subscriptions
func SaveSubscriptionResults(subscriptions []string, outputDir string, sites []TargetSite, settings TestSettings) error {
	ctx := context.Background()

	for _, subURL := range subscriptions {
		// Create a safe directory name
		safeName := sanitizeFilename(subURL)
		if safeName == "" {
			safeName = "subscription_" + time.Now().Format("20060102_150405")
		}

		subDir := filepath.Join(outputDir, "subscriptions", safeName)
		if err := os.MkdirAll(subDir, 0755); err != nil {
			return err
		}

		// Test the subscription
		report, err := TestSubscription(ctx, subURL, sites, settings)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to test %s: %v\n", subURL, err)
			continue
		}

		// Save report
		timestamp := time.Now().UTC().Format("20060102_150405")
		jsonPath := filepath.Join(subDir, timestamp+".json")
		mdPath := filepath.Join(subDir, timestamp+".md")

		if err := SaveReport(report, jsonPath); err != nil {
			return err
		}
		if err := SaveMarkdownReport(report, mdPath); err != nil {
			return err
		}

		// Update latest
		latestJSON := filepath.Join(subDir, "latest.json")
		latestMD := filepath.Join(subDir, "latest.md")

		if err := copyFile(jsonPath, latestJSON); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update latest for %s: %v\n", subURL, err)
		}
		if err := copyFile(mdPath, latestMD); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update latest for %s: %v\n", subURL, err)
		}

		fmt.Printf("Tested %s: %d configs, %d working\n", subURL, report.TotalConfigs, report.WorkingConfigs)
	}

	return nil
}
