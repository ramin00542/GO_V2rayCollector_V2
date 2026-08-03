package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
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
	for _, channel := range channels {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		item := telegramProvider.Fetch(ctx, channel)
		result.ProviderResults = append(result.ProviderResults, item)
		result.NewConfigs += addConfigs(store, item, channel.Name, now.UTC())
		observeCandidates(candidates, item, "telegram:"+channel.Name, now.UTC())
	}
	if githubSettings.Enabled {
		discovery := provider.NewGitHubDiscoverer(client, sourceLimiter)
		discovered, err := discovery.Discover(ctx, githubSettings)
		if err != nil {
			return result, err
		}
		sources = append(sources, discovered...)
	}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		item := subscriptionProvider.Fetch(ctx, source)
		result.ProviderResults = append(result.ProviderResults, item)
		result.NewConfigs += addConfigs(store, item, "", now.UTC())
		observeCandidates(candidates, item, "source:"+source.URL, now.UTC())
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
	options := output.SnapshotOptions{KeepUnknown: settings.Output.KeepUnknown, WritePerChannel: settings.Output.WritePerChannel}
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
	for _, candidate := range candidates.Eligible(domain.DiscoveryChannel, now, settings.Discovery.ChannelFetchBudget, settings.Discovery.CandidateExpiryDays) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if knownChannels[candidate.Value] {
			continue
		}
		item := telegram.Fetch(ctx, domain.Channel{URL: "https://t.me/s/" + candidate.Value, Name: candidate.Value, Enabled: true})
		status := candidateStatus(item)
		updated := candidates.Result(candidate.ID, status, now, settings.Discovery.PromotionSuccessCount, interval)
		if updated.Status == state.CandidatePromoted {
			if err := repository.AddChannel(paths.ChannelsFile(), candidate.Value); err != nil {
				return err
			}
			knownChannels[candidate.Value] = true
		}
	}
	for _, candidate := range candidates.Eligible(domain.DiscoverySource, now, settings.Discovery.SourceFetchBudget, settings.Discovery.CandidateExpiryDays) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if knownSources[candidate.Value] {
			continue
		}
		item := subscription.Fetch(ctx, domain.Source{URL: candidate.Value, Enabled: true, Kind: domain.SourceSubscription, Name: "auto-discovered"})
		status := candidateStatus(item)
		updated := candidates.Result(candidate.ID, status, now, settings.Discovery.PromotionSuccessCount, interval)
		if updated.Status == state.CandidatePromoted {
			if err := repository.AddSource(paths.SourcesFile(), domain.Source{URL: candidate.Value, Enabled: true, Name: "auto-discovered", Kind: domain.SourceSubscription}); err != nil {
				return err
			}
			knownSources[candidate.Value] = true
		}
	}
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

