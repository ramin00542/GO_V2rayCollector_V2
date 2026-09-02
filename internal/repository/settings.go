package repository

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
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
		// If file doesn't exist, return default disabled settings
		if os.IsNotExist(err) {
			return domain.GitHubSettings{Enabled: false}, nil
		}
		return settings, fmt.Errorf("read GitHub settings: %w", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("decode GitHub settings: %w", err)
	}
	if !settings.Enabled {
		return settings, nil
	}
	
	// Extract repository owner and name from various formats
	repo := strings.TrimSpace(settings.Repository)
	if repo == "" {
		return settings, fmt.Errorf("repository is required")
	}
	
	// Handle different repository formats
	var owner, name string
	
	// Format: owner/repository
	if strings.Contains(repo, "/") && !strings.HasPrefix(repo, "http") {
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) == 2 {
			owner = strings.TrimSpace(parts[0])
			name = strings.TrimSpace(parts[1])
		}
	} else if strings.HasPrefix(repo, "http") {
		// Format: https://github.com/owner/repository
		parsed, err := url.Parse(repo)
		if err != nil {
			return settings, fmt.Errorf("invalid repository URL: %w", err)
		}
		pathParts := strings.SplitN(strings.Trim(parsed.Path, "/"), "/", 2)
		if len(pathParts) == 2 {
			owner = strings.TrimSpace(pathParts[0])
			name = strings.TrimSpace(pathParts[1])
		}
	}
	
	if owner == "" || name == "" {
		return settings, fmt.Errorf("repository must use owner/repository format (e.g., 'owner/repo' or 'https://github.com/owner/repo')")
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
