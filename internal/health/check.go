package health

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/RaminTabriz/V2rayCollector/internal/config"
	"github.com/RaminTabriz/V2rayCollector/internal/domain"
	"github.com/RaminTabriz/V2rayCollector/internal/fetch"
	"github.com/RaminTabriz/V2rayCollector/internal/provider"
	"github.com/RaminTabriz/V2rayCollector/internal/repository"
)

type CheckResult struct {
	Target     string
	Status     Status
	Accepted   int
	HTTPStatus int
	Error      string
}

func CheckChannels(ctx context.Context, paths config.Paths, reviveOnly bool, now time.Time) ([]CheckResult, error) {
	channels, err := repository.LoadChannels(paths.ChannelsFile())
	if err != nil {
		return nil, err
	}
	settings, err := repository.LoadCollectorSettings(paths.SettingsFile())
	if err != nil {
		return nil, err
	}
	store, err := Open(filepath.Join(paths.DataDir, "state", "health.json"))
	if err != nil {
		return nil, err
	}
	client, err := fetch.NewClient(fetch.DefaultConfig())
	if err != nil {
		return nil, err
	}
	limiter, err := fetch.NewLimiter(5)
	if err != nil {
		return nil, err
	}
	defer limiter.Close()
	telegram := provider.NewTelegramProvider(client, limiter, settings.Output.KeepUnknown)
	results := make([]CheckResult, 0)
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		if record, ok := store.Channel(channel.Name); ok && ShouldDormant(record, now.UTC(), settings.Discovery.DormantAfterDays) {
			store.MarkChannelDormant(channel.Name, now.UTC())
			continue
		}
		if reviveOnly {
			if record, ok := store.Channel(channel.Name); !ok || (record.Status != StatusInactive && record.Status != StatusNotFound) {
				continue
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := telegram.Fetch(ctx, channel)
		status := classify(item)
		store.UpdateChannel(channel.Name, status, item.Error, now.UTC())
		results = append(results, CheckResult{Target: channel.Name, Status: status, Accepted: item.Accepted, HTTPStatus: item.HTTPStatus, Error: item.Error})
	}
	if err := store.Save(); err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Target < results[j].Target })
	return results, nil
}

func CheckSources(ctx context.Context, paths config.Paths, now time.Time) ([]CheckResult, error) {
	sources, err := repository.LoadSources(paths.SourcesFile())
	if err != nil {
		return nil, err
	}
	settings, err := repository.LoadCollectorSettings(paths.SettingsFile())
	if err != nil {
		return nil, err
	}
	store, err := Open(filepath.Join(paths.DataDir, "state", "health.json"))
	if err != nil {
		return nil, err
	}
	client, err := fetch.NewClient(fetch.DefaultConfig())
	if err != nil {
		return nil, err
	}
	limiter, err := fetch.NewLimiter(10)
	if err != nil {
		return nil, err
	}
	defer limiter.Close()
	subscription := provider.NewSubscriptionProvider(client, limiter, settings.Output.KeepUnknown)
	results := make([]CheckResult, 0)
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		if record, ok := store.Source(source.URL); ok && ShouldDormant(record, now.UTC(), settings.Discovery.DormantAfterDays) {
			store.MarkSourceDormant(source.URL, now.UTC())
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := subscription.Fetch(ctx, source)
		status := classify(item)
		store.UpdateSource(source.URL, status, item.Error, now.UTC())
		results = append(results, CheckResult{Target: source.URL, Status: status, Accepted: item.Accepted, HTTPStatus: item.HTTPStatus, Error: item.Error})
	}
	if err := store.Save(); err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Target < results[j].Target })
	return results, nil
}

func classify(result domain.ProviderResult) Status {
	if result.Error != "" {
		if result.HTTPStatus == 404 {
			return StatusNotFound
		}
		return StatusUnknown
	}
	if result.Accepted > 0 {
		return StatusActive
	}
	return StatusInactive
}

func FormatReport(title string, results []CheckResult) string {
	active, inactive, missing, unknown := 0, 0, 0, 0
	for _, result := range results {
		switch result.Status {
		case StatusActive:
			active++
		case StatusInactive:
			inactive++
		case StatusNotFound:
			missing++
		case StatusUnknown:
			unknown++
		}
	}
	return fmt.Sprintf("# %s\n\n- Checked: %d\n- Active: %d\n- Inactive: %d\n- Not found: %d\n- Unknown errors: %d\n", title, len(results), active, inactive, missing, unknown)
}
