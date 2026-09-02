// Package updater provides automatic update functionality for subscriptions
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/fetch"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/logging"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/notification"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/parser"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/repository"
)

// SubscriptionState tracks the state of a subscription
type SubscriptionState struct {
	URL            string    `json:"url"`
	LastUpdate     time.Time `json:"last_update"`
	LastHash       string    `json:"last_hash"`
	LastConfigs    int       `json:"last_configs"`
	CurrentHash    string    `json:"current_hash"`
	CurrentConfigs int       `json:"current_configs"`
	Updated        bool      `json:"updated"`
	Error          string    `json:"error,omitempty"`
}

// UpdaterConfig holds configuration for the updater
type UpdaterConfig struct {
	CheckInterval  time.Duration `json:"check_interval"`
	MaxRetries     int           `json:"max_retries"`
	NotifyOnUpdate bool          `json:"notify_on_update"`
	NotifyOnError  bool          `json:"notify_on_error"`
}

// LoadUpdaterConfig loads the updater configuration from a JSON file.
// A missing file falls back to the built-in defaults. In the JSON file
// "check_interval" is expressed in hours, which is what config/updater.json
// uses.
func LoadUpdaterConfig(path string) (UpdaterConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return UpdaterConfig{
			CheckInterval:  24 * time.Hour,
			MaxRetries:     3,
			NotifyOnUpdate: true,
			NotifyOnError:  true,
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return UpdaterConfig{}, err
	}

	// Intermediate representation: the on-disk format stores the interval in
	// hours while UpdaterConfig keeps a duration.
	var raw struct {
		CheckIntervalHours float64 `json:"check_interval"`
		MaxRetries         int     `json:"max_retries"`
		NotifyOnUpdate     bool    `json:"notify_on_update"`
		NotifyOnError      bool    `json:"notify_on_error"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return UpdaterConfig{}, err
	}

	cfg := UpdaterConfig{
		CheckInterval:  time.Duration(raw.CheckIntervalHours * float64(time.Hour)),
		MaxRetries:     raw.MaxRetries,
		NotifyOnUpdate: raw.NotifyOnUpdate,
		NotifyOnError:  raw.NotifyOnError,
	}
	if raw.CheckIntervalHours == 0 {
		cfg.CheckInterval = 24 * time.Hour
	}

	return cfg, nil
}

// SaveUpdaterConfig saves the updater configuration to a JSON file
func SaveUpdaterConfig(cfg UpdaterConfig, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	payload := struct {
		CheckInterval  float64 `json:"check_interval"`
		MaxRetries     int     `json:"max_retries"`
		NotifyOnUpdate bool    `json:"notify_on_update"`
		NotifyOnError  bool    `json:"notify_on_error"`
	}{
		CheckInterval:  cfg.CheckInterval.Hours(),
		MaxRetries:     cfg.MaxRetries,
		NotifyOnUpdate: cfg.NotifyOnUpdate,
		NotifyOnError:  cfg.NotifyOnError,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Updater manages automatic updates of subscriptions
type Updater struct {
	config    UpdaterConfig
	paths     config.Paths
	logger    *logging.Logger
	notifier  *notification.MultiNotifier
	client    *fetch.Client
	stateFile string
	states    map[string]SubscriptionState
	mu        sync.RWMutex
	stopChan  chan struct{}
	wg        sync.WaitGroup
	startTime time.Time
}

// NewUpdater creates a new updater instance
func NewUpdater(cfg UpdaterConfig, paths config.Paths, logger *logging.Logger, notifier *notification.MultiNotifier) (*Updater, error) {
	// Set defaults
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 24 * time.Hour
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}

	// Create fetch client
	client, err := fetch.NewClient(fetch.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch client: %w", err)
	}

	// Create state file path
	stateFile := filepath.Join(paths.DataDir, "state", "updater_state.json")

	u := &Updater{
		config:    cfg,
		paths:     paths,
		logger:    logger,
		notifier:  notifier,
		client:    client,
		stateFile: stateFile,
		states:    make(map[string]SubscriptionState),
		stopChan:  make(chan struct{}),
		startTime: time.Now().UTC(),
	}

	// Load previous state
	if err := u.loadState(); err != nil {
		logger.Warn("failed to load updater state", "error", err)
	}

	return u, nil
}

// Start starts the updater
func (u *Updater) Start(ctx context.Context) error {
	u.logger.Info("starting subscription updater", "interval", u.config.CheckInterval)

	// Initial update
	if err := u.updateAll(ctx); err != nil {
		u.logger.Error("initial update failed", "error", err)
	}

	// Periodic updates
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()

		ticker := time.NewTicker(u.config.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-u.stopChan:
				return
			case <-ticker.C:
				if err := u.updateAll(ctx); err != nil {
					u.logger.Error("periodic update failed", "error", err)
				}
			}
		}
	}()

	return nil
}

// Stop stops the updater
func (u *Updater) Stop() {
	close(u.stopChan)
	u.wg.Wait()

	// Save state before stopping
	if err := u.saveState(); err != nil {
		u.logger.Error("failed to save updater state on stop", "error", err)
	}

	u.logger.Info("subscription updater stopped")
}

// updateAll updates all subscriptions
func (u *Updater) updateAll(ctx context.Context) error {
	u.logger.Debug("starting update cycle")

	// Load subscriptions
	sources, err := repository.LoadSources(u.paths.SourcesFile())
	if err != nil {
		return fmt.Errorf("failed to load sources: %w", err)
	}

	// Filter enabled subscriptions
	var subscriptions []domain.Source
	for _, source := range sources {
		if source.Enabled && source.Kind == domain.SourceSubscription {
			subscriptions = append(subscriptions, source)
		}
	}

	if len(subscriptions) == 0 {
		u.logger.Info("no enabled subscriptions to update")
		return nil
	}

	u.logger.Info("updating subscriptions", "count", len(subscriptions))

	// Update each subscription
	for _, sub := range subscriptions {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := u.updateSubscription(ctx, sub); err != nil {
			u.logger.Error("failed to update subscription", "url", sub.URL, "error", err)

			// Send notification if configured
			if u.config.NotifyOnError && u.notifier != nil {
				go func() {
					if err := u.notifier.Send(ctx, "خطا در آپدیت ساب", map[string]interface{}{
						"url":   sub.URL,
						"error": err.Error(),
						"time":  time.Now().Format(time.RFC3339),
					}); err != nil {
						u.logger.Error("failed to send error notification", "error", err)
					}
				}()
			}
		}
	}

	// Save state
	if err := u.saveState(); err != nil {
		u.logger.Error("failed to save updater state", "error", err)
	}

	return nil
}

// updateSubscription updates a single subscription
func (u *Updater) updateSubscription(ctx context.Context, sub domain.Source) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	state, exists := u.states[sub.URL]
	if !exists {
		state = SubscriptionState{
			URL:        sub.URL,
			LastUpdate: time.Now().UTC(),
		}
	}

	// Fetch the subscription content
	content, err := u.fetchSubscription(ctx, sub.URL)
	if err != nil {
		state.Error = err.Error()
		u.states[sub.URL] = state
		return err
	}

	// Calculate hash of content
	currentHash := hashContent(content)

	// Check if content has changed
	if state.LastHash == currentHash {
		state.Updated = false
		u.states[sub.URL] = state
		return nil
	}

	// Content has changed!
	state.LastHash = currentHash
	state.CurrentHash = currentHash
	state.LastUpdate = time.Now().UTC()
	state.Updated = true
	state.Error = ""

	// Count configs in the new content
	configs, _ := parser.Extract(string(content), true)
	state.CurrentConfigs = len(configs)

	// If this is the first time, set last configs
	if state.LastConfigs == 0 && state.CurrentConfigs > 0 {
		state.LastConfigs = state.CurrentConfigs
	}

	u.states[sub.URL] = state

	// Save the new content to file
	if err := u.saveSubscriptionContent(sub.URL, content); err != nil {
		u.logger.Error("failed to save subscription content", "url", sub.URL, "error", err)
	}

	// Send notification if configured
	if u.config.NotifyOnUpdate && u.notifier != nil {
		go func() {
			if err := u.notifier.Send(ctx, "سب آپدیت شد!", map[string]interface{}{
				"url":         sub.URL,
				"new_configs": state.CurrentConfigs,
				"change":      state.CurrentConfigs - state.LastConfigs,
				"time":        time.Now().Format(time.RFC3339),
			}); err != nil {
				u.logger.Error("failed to send update notification", "error", err)
			}
		}()
	}

	// Update last configs
	state.LastConfigs = state.CurrentConfigs

	return nil
}

// fetchSubscription fetches the content of a subscription URL
func (u *Updater) fetchSubscription(ctx context.Context, url string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= u.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		response, err := u.client.Get(ctx, url, nil)
		if err != nil {
			lastErr = err
			continue
		}

		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return response.Body, nil
		}

		lastErr = fmt.Errorf("HTTP %d", response.StatusCode)
	}

	return nil, lastErr
}

// saveSubscriptionContent saves the content of a subscription to a file
func (u *Updater) saveSubscriptionContent(url string, content []byte) error {
	// Create safe filename
	filename := sanitizeFilename(url) + ".txt"
	subDir := filepath.Join(u.paths.DataDir, "subscriptions")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return err
	}

	// Save to file
	path := filepath.Join(subDir, filename)
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// loadState loads the updater state from file
func (u *Updater) loadState() error {
	data, err := os.ReadFile(u.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var states map[string]SubscriptionState
	if err := json.Unmarshal(data, &states); err != nil {
		return err
	}

	u.states = states
	return nil
}

// saveState saves the updater state to file
func (u *Updater) saveState() error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(u.stateFile), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(u.states, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := u.stateFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, u.stateFile)
}

// GetState returns the state of a subscription
func (u *Updater) GetState(url string) (SubscriptionState, bool) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	state, exists := u.states[url]
	return state, exists
}

// GetAllStates returns the state of all subscriptions
func (u *Updater) GetAllStates() map[string]SubscriptionState {
	u.mu.RLock()
	defer u.mu.RUnlock()

	// Create a copy
	states := make(map[string]SubscriptionState)
	for k, v := range u.states {
		states[k] = v
	}
	return states
}

// GetUpdatedSubscriptions returns subscriptions that were updated in the last cycle
func (u *Updater) GetUpdatedSubscriptions() []SubscriptionState {
	u.mu.RLock()
	defer u.mu.RUnlock()

	var updated []SubscriptionState
	for _, state := range u.states {
		if state.Updated {
			updated = append(updated, state)
		}
	}
	return updated
}

// ResetUpdatedFlags resets the updated flags for all subscriptions
func (u *Updater) ResetUpdatedFlags() {
	u.mu.Lock()
	defer u.mu.Unlock()

	for url, state := range u.states {
		state.Updated = false
		u.states[url] = state
	}
}

// hashContent calculates a simple hash of content
func hashContent(content []byte) string {
	// Simple hash - in production, use a proper hash function
	var sum uint32
	for _, b := range content {
		sum += uint32(b)
	}
	return fmt.Sprintf("%d-%d", sum, len(content))
}

// sanitizeFilename creates a safe filename from a URL
func sanitizeFilename(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")

	// Remove special characters
	url = strings.ReplaceAll(url, "/", "_")
	url = strings.ReplaceAll(url, "?", "_")
	url = strings.ReplaceAll(url, "=", "_")
	url = strings.ReplaceAll(url, "&", "_")
	url = strings.ReplaceAll(url, "#", "_")

	// Trim to reasonable length
	if len(url) > 100 {
		url = url[:100]
	}

	return url
}

// UpdateSubscriptionNow updates a specific subscription immediately
func (u *Updater) UpdateSubscriptionNow(ctx context.Context, url string) error {
	// Load subscriptions to find the one with matching URL
	sources, err := repository.LoadSources(u.paths.SourcesFile())
	if err != nil {
		return err
	}

	for _, sub := range sources {
		if sub.URL == url {
			return u.updateSubscription(ctx, sub)
		}
	}

	return fmt.Errorf("subscription not found: %s", url)
}

// AddSubscription adds a new subscription to the updater
func (u *Updater) AddSubscription(url string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Check if already exists
	if _, exists := u.states[url]; exists {
		return nil
	}

	// Add to state
	u.states[url] = SubscriptionState{
		URL:        url,
		LastUpdate: time.Now().UTC(),
	}

	// Save state
	return u.saveState()
}

// RemoveSubscription removes a subscription from the updater
func (u *Updater) RemoveSubscription(url string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	delete(u.states, url)

	// Save state
	return u.saveState()
}
