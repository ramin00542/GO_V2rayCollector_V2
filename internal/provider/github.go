package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/fetch"
)

type GitHubDiscoverer struct {
	client  *fetch.Client
	limiter *fetch.Limiter
	apiBase string
}

type githubFork struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
}

func NewGitHubDiscoverer(client *fetch.Client, limiter *fetch.Limiter) *GitHubDiscoverer {
	return &GitHubDiscoverer{client: client, limiter: limiter, apiBase: "https://api.github.com"}
}

func (d *GitHubDiscoverer) SetAPIBaseURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("GitHub API base URL must be HTTPS")
	}
	d.apiBase = strings.TrimRight(rawURL, "/")
	return nil
}

// Discover returns bounded, explicit raw-file sources. Downloading and parsing
// those files remains the responsibility of SubscriptionProvider.
func (d *GitHubDiscoverer) Discover(ctx context.Context, settings domain.GitHubSettings) ([]domain.Source, error) {
	if !settings.Enabled {
		return nil, nil
	}
	if d == nil || d.client == nil {
		return nil, fmt.Errorf("GitHub discoverer is not initialized")
	}
	parts := strings.Split(settings.Repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("repository must use owner/repository format")
	}
	seen := make(map[string]bool)
	result := make([]domain.Source, 0)
	remaining := settings.MaxForks
	for pageNumber := 1; pageNumber <= settings.MaxPages && remaining > 0; pageNumber++ {
		endpoint := fmt.Sprintf("%s/repos/%s/forks?per_page=%d&page=%d", d.apiBase, settings.Repository, min(remaining, 100), pageNumber)
		response, err := d.client.Get(ctx, endpoint, d.limiter)
		if err != nil {
			return nil, err
		}
		var forks []githubFork
		if err := json.Unmarshal(response.Body, &forks); err != nil {
			return nil, fmt.Errorf("decode GitHub forks response: %w", err)
		}
		if len(forks) == 0 {
			break
		}
		for _, fork := range forks {
			if remaining == 0 {
				break
			}
			remaining--
			if fork.FullName == "" || fork.DefaultBranch == "" {
				continue
			}
			for _, item := range settings.Paths {
				item = strings.TrimPrefix(path.Clean("/"+item), "/")
				if item == "." || strings.HasPrefix(item, "../") {
					continue
				}
				rawURL := "https://raw.githubusercontent.com/" + fork.FullName + "/" + fork.DefaultBranch + "/" + item
				if !seen[rawURL] {
					seen[rawURL] = true
					result = append(result, domain.Source{URL: rawURL, Enabled: true, Name: fork.FullName + "/" + item, Kind: domain.SourceGitHubFork})
				}
			}
		}
		if len(forks) < min(remaining+len(forks), 100) {
			break
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URL < result[j].URL })
	return result, nil
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
