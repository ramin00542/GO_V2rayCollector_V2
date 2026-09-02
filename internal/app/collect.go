package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/concurrency"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/fetch"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/output"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/provider"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/report"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/repository"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/state"
)

type RunResult struct {
	StartedAt       time.Time
	FinishedAt      time.Time
	NewConfigs      int
	ProviderResults []domain.ProviderResult
	Summary         provider.Summary
}

func Collect(ctx context.Context, paths config.Paths, now time.Time) (RunResult, error) {
	result := RunResult{StartedAt: now.UTC()}
	settings, err := repository.LoadCollectorSettings(paths.SettingsFile())
	if err != nil {
		return result, err
	}
	channels, err := repository.LoadChannels(paths.ChannelsFile())
	if err != nil {
		return result, err
	}
	sources, err := repository.LoadSources(paths.SourcesFile())
	if err != nil {
		return result, err
	}
	githubSettings, err := repository.LoadGitHubSettings(paths.GitHubFile())
	if err != nil {
		return result, err
	}
	store, err := state.Open(filepath.Join(paths.DataDir, "state", "configs.json"))
	if err != nil {
		return result, err
	}
	candidates, err := state.OpenCandidates(filepath.Join(paths.DataDir, "state", "candidates.json"))
	if err != nil {
		return result, err
	}
	if err := rollover(store, paths, settings, now.UTC()); err != nil {
		return result, err
	}

	client, err := fetch.NewClient(fetch.DefaultConfig())
	if err != nil {
		return result, err
	}
	telegramLimiter, err := fetch.NewLimiter(5)
	if err != nil {
		return result, err
	}
	defer telegramLimiter.Close()
	sourceLimiter, err := fetch.NewLimiter(10)
	if err != nil {
		return result, err
	}
	defer sourceLimiter.Close()

	telegramProvider := provider.NewTelegramProvider(client, telegramLimiter, settings.Output.KeepUnknown)
	subscriptionProvider := provider.NewSubscriptionProvider(client, sourceLimiter, settings.Output.KeepUnknown)
	
	// Use worker pool for concurrent fetching
	// Process channels with concurrency
	channelResults := make([]domain.ProviderResult, 0, len(channels))
	channelMu := &sync.Mutex{}
	
	channelPool := concurrency.NewWorkerPool(5) // 5 concurrent channel fetches
	channelPool.Start()
	defer channelPool.Stop()
	
	for _, channel := range channels {
		if err := ctx.Err(); err != nil {
			channelPool.Stop()
			return result, err
		}
		
		// Capture channel for closure
		ch := channel
		channelPool.Submit(func() {
			item := telegramProvider.Fetch(ctx, ch)
			channelMu.Lock()
			channelResults = append(channelResults, item)
			channelMu.Unlock()
		})
	}
	
	// Wait for all channel fetches to complete
	channelPool.Wait()
	
	// Process channel results
	for _, item := range channelResults {
		result.ProviderResults = append(result.ProviderResults, item)
		result.NewConfigs += addConfigs(store, item, item.SourceURL, now.UTC())
		// Extract channel name from URL for candidate observation
		channelName := ""
		if len(item.SourceURL) > 0 {
			channelName = filepath.Base(item.SourceURL)
		}
		observeCandidates(candidates, item, "telegram:"+channelName, now.UTC())
	}
	
	// Process GitHub discovery if enabled
	if githubSettings.Enabled {
		discovery := provider.NewGitHubDiscoverer(client, sourceLimiter)
		discovered, err := discovery.Discover(ctx, githubSettings)
		if err != nil {
			return result, err
		}
		sources = append(sources, discovered...)
	}
	
	// Process sources with concurrency
	sourceResults := make([]domain.ProviderResult, 0, len(sources))
	sourceMu := &sync.Mutex{}
	
	sourcePool := concurrency.NewWorkerPool(10) // 10 concurrent source fetches
	sourcePool.Start()
	defer sourcePool.Stop()
	
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			sourcePool.Stop()
			return result, err
		}
		
		// Capture source for closure
		src := source
		sourcePool.Submit(func() {
			item := subscriptionProvider.Fetch(ctx, src)
			sourceMu.Lock()
			sourceResults = append(sourceResults, item)
			sourceMu.Unlock()
		})
	}
	
	// Wait for all source fetches to complete
	sourcePool.Wait()
	
	// Process source results
	for _, item := range sourceResults {
		result.ProviderResults = append(result.ProviderResults, item)
		result.NewConfigs += addConfigs(store, item, "", now.UTC())
		observeCandidates(candidates, item, "source:"+item.SourceURL, now.UTC())
	}

	if err := validateCandidates(ctx, candidates, channels, sources, telegramProvider, subscriptionProvider, paths, settings, now.UTC()); err != nil {
		return result, err
	}
	if err := candidates.Save(); err != nil {
		return result, err
	}
	store.Prune(now.UTC().AddDate(0, 0, -settings.Retention.DailyDays))
	store.SetCurrentDay(now.UTC().Format("2006-01-02"))
	if err := store.Save(); err != nil {
		return result, err
	}
	if err := publish(store.Data(), paths, settings, now.UTC()); err != nil {
		return result, err
	}
	result.Summary = provider.Summarize(result.ProviderResults)
	result.FinishedAt = time.Now().UTC()
	if err := report.Write(paths, report.RunResult{StartedAt: result.StartedAt, FinishedAt: result.FinishedAt, NewConfigs: result.NewConfigs, Requests: result.Summary.Requests, Succeeded: result.Summary.Succeeded, Failed: result.Summary.Failed, Accepted: result.Summary.Accepted, Rejected: result.Summary.Rejected, BytesRead: result.Summary.BytesRead}, candidates.Data()); err != nil {
		return result, err
	}
	return result, nil
}

func rollover(store *state.Store, paths config.Paths, settings domain.CollectorSettings, now time.Time) error {
	current := now.Format("2006-01-02")
	previous := store.Data().CurrentDay
	if previous == "" || previous == current {
		return nil
	}
	day, err := time.Parse("2006-01-02", previous)
	if err != nil {
		return fmt.Errorf("invalid saved current day: %w", err)
	}
	if err := output.PublishDaily(paths.ArchiveDir, output.SortedEntries(store.Data()), day, output.SnapshotOptions{KeepUnknown: settings.Output.KeepUnknown, WritePerChannel: settings.Output.WritePerChannel}); err != nil {
		return err
	}
	return output.PruneDaily(paths.ArchiveDir, now, settings.Retention.DailyDays)
}

func addConfigs(store *state.Store, result domain.ProviderResult, channel string, now time.Time) int {
	if result.Error != "" {
		return 0
	}
	created := 0
	for _, config := range result.Configs {
		if store.Upsert(config, result.SourceKind, channel, now) {
			created++
		}
	}
	return created
}

func publish(data state.Data, paths config.Paths, settings domain.CollectorSettings, now time.Time) error {
	start, end := output.DayBounds(now)
	entries := state.EntriesForWindow(data, start, end)
	options := output.SnapshotOptions{KeepUnknown: settings.Output.KeepUnknown, WritePerChannel: settings.Output.WritePerChannel, WriteProtocols: true, WriteCombined: true}
	if err := output.Publish(filepath.Join(paths.OutputDir, "temporary"), entries, start, end, options); err != nil {
		return err
	}
	return output.PublishRolling(paths.ArchiveDir, output.SortedEntries(data), now, settings.Retention.RollingDays, options)
}

func observeCandidates(store *state.CandidateStore, result domain.ProviderResult, origin string, now time.Time) {
	for _, link := range result.Discovered {
		store.Observe(link.Kind, link.Value, origin, now)
	}
}

func validateCandidates(ctx context.Context, candidates *state.CandidateStore, channels []domain.Channel, sources []domain.Source, telegram *provider.TelegramProvider, subscription *provider.SubscriptionProvider, paths config.Paths, settings domain.CollectorSettings, now time.Time) error {
	knownChannels := map[string]bool{}
	for _, channel := range channels {
		knownChannels[channel.Name] = true
	}
	knownSources := map[string]bool{}
	for _, source := range sources {
		knownSources[source.URL] = true
	}
	interval := time.Duration(settings.Discovery.PromotionMinIntervalHrs) * time.Hour
	
	// Process channel candidates with concurrency
	channelCandidates := candidates.EligibleAll(domain.DiscoveryChannel, now, settings.Discovery.CandidateExpiryDays)
	
	// Use worker pool for concurrent candidate validation
	candidatePool := concurrency.NewWorkerPool(5) // 5 concurrent candidate checks
	candidatePool.Start()
	defer candidatePool.Stop()
	
	var candidateErr error
	var candidateMu sync.Mutex
	
	for _, candidate := range channelCandidates {
		if err := ctx.Err(); err != nil {
			candidatePool.Stop()
			return err
		}
		
		// Capture candidate for closure
		cand := candidate
		candidatePool.Submit(func() {
			if knownChannels[cand.Value] {
				return
			}
			item := telegram.Fetch(ctx, domain.Channel{URL: "https://t.me/s/" + cand.Value, Name: cand.Value, Enabled: true})
			status := candidateStatus(item)
			updated := candidates.Result(cand.ID, status, now, settings.Discovery.PromotionSuccessCount, interval)
			if updated.Status == state.CandidatePromoted {
				candidateMu.Lock()
				if err := repository.AddChannel(paths.ChannelsFile(), cand.Value); err != nil {
					candidateErr = err
				}
				knownChannels[cand.Value] = true
				candidateMu.Unlock()
			}
		})
	}
	
	// Wait for all channel candidate checks to complete
	candidatePool.Wait()
	
	if candidateErr != nil {
		return candidateErr
	}
	
	// Process source candidates with concurrency
	sourceCandidates := candidates.EligibleAll(domain.DiscoverySource, now, settings.Discovery.CandidateExpiryDays)
	
	sourcePool := concurrency.NewWorkerPool(10) // 10 concurrent source candidate checks
	sourcePool.Start()
	defer sourcePool.Stop()
	
	var sourceErr error
	var sourceMu sync.Mutex
	
	for _, candidate := range sourceCandidates {
		if err := ctx.Err(); err != nil {
			sourcePool.Stop()
			return err
		}
		
		// Capture candidate for closure
		cand := candidate
		sourcePool.Submit(func() {
			if knownSources[cand.Value] {
				return
			}
			item := subscription.Fetch(ctx, domain.Source{URL: cand.Value, Enabled: true, Kind: domain.SourceSubscription, Name: "auto-discovered"})
			status := candidateStatus(item)
			updated := candidates.Result(cand.ID, status, now, settings.Discovery.PromotionSuccessCount, interval)
			if updated.Status == state.CandidatePromoted {
				sourceMu.Lock()
				if err := repository.AddSource(paths.SourcesFile(), domain.Source{URL: cand.Value, Enabled: true, Name: "auto-discovered", Kind: domain.SourceSubscription}); err != nil {
					sourceErr = err
				}
				knownSources[cand.Value] = true
				sourceMu.Unlock()
			}
		})
	}
	
	// Wait for all source candidate checks to complete
	sourcePool.Wait()
	
	if sourceErr != nil {
		return sourceErr
	}
	
	// Prune expired candidates to prevent state file from growing indefinitely
	candidates.Prune(now.AddDate(0, 0, -settings.Discovery.CandidateExpiryDays*2), settings.Discovery.CandidateExpiryDays)
	
	return nil
}

func candidateStatus(result domain.ProviderResult) state.CandidateStatus {
	if result.Error != "" {
		if result.HTTPStatus == 404 {
			return state.CandidateNotFound
		}
		return state.CandidateUnknown
	}
	if result.Accepted > 0 {
		return state.CandidateQualified
	}
	return state.CandidateNoConfig
}
