package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/RaminTabriz/V2rayCollector/internal/domain"
)

func LoadCollectorSettings(pathname string) (domain.CollectorSettings, error) {
	var settings domain.CollectorSettings
	data, err := os.ReadFile(pathname)
	if err != nil {
		return settings, fmt.Errorf("read collector settings: %w", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("decode collector settings: %w", err)
	}
	if settings.Version != 1 {
		return settings, fmt.Errorf("unsupported collector settings version: %d", settings.Version)
	}
	if settings.Retention.DailyDays < 1 || settings.Retention.RollingDays < 1 {
		return settings, fmt.Errorf("retention days must be positive")
	}
	d := settings.Discovery
	if d.ChannelFetchBudget < 1 || d.SourceFetchBudget < 1 || d.QualifiedTarget < 1 || d.PromotionSuccessCount < 1 || d.PromotionMinIntervalHrs < 1 || d.CandidateExpiryDays < 1 || d.DormantAfterDays < 1 {
		return settings, fmt.Errorf("invalid discovery policy")
	}
	return settings, nil
}

func LoadGitHubSettings(pathname string) (domain.GitHubSettings, error) {
	var settings domain.GitHubSettings
	data, err := os.ReadFile(pathname)
	if err != nil {
		return settings, fmt.Errorf("read GitHub settings: %w", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("decode GitHub settings: %w", err)
	}
	if !settings.Enabled {
		return settings, nil
	}
	parts := strings.Split(settings.Repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return settings, fmt.Errorf("repository must use owner/repository format")
	}
	if settings.MaxForks < 1 || settings.MaxForks > 100 {
		return settings, fmt.Errorf("max_forks must be between 1 and 100")
	}
	if settings.MaxPages < 1 || settings.MaxPages > 10 {
		return settings, fmt.Errorf("max_pages must be between 1 and 10")
	}
	for _, item := range settings.Paths {
		clean := strings.TrimPrefix(path.Clean("/"+item), "/")
		if clean == "." || strings.HasPrefix(clean, "../") {
			return settings, fmt.Errorf("invalid GitHub path: %q", item)
		}
	}
	return settings, nil
}
