package repository

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
)

type sourceFile struct {
	Version int             `json:"version"`
	Sources []domain.Source `json:"sources"`
}

func LoadSources(path string) ([]domain.Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// If file doesn't exist, return empty list
		if os.IsNotExist(err) {
			return []domain.Source{}, nil
		}
		return nil, fmt.Errorf("read sources file: %w", err)
	}
	var file sourceFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode sources JSON: %w", err)
	}
	
	// Support version 1 or no version (for backward compatibility)
	if file.Version != 0 && file.Version != 1 {
		return nil, fmt.Errorf("unsupported sources version: %d", file.Version)
	}

	seen := make(map[string]bool)
	result := make([]domain.Source, 0, len(file.Sources))
	for _, source := range file.Sources {
		normalized, ok := NormalizeSourceURL(source.URL)
		if !ok || seen[normalized] {
			continue
		}
		source.URL = normalized
		if source.Kind == "" {
			source.Kind = domain.SourceSubscription
		}
		if source.Kind != domain.SourceSubscription && source.Kind != domain.SourceGitHubFork {
			continue
		}
		seen[normalized] = true
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URL < result[j].URL })
	return result, nil
}

func NormalizeSourceURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", false
	}
	parsed.Fragment = ""
	return parsed.String(), true
}

func AddSource(path string, source domain.Source) error {
	sources, err := LoadSources(path)
	if err != nil {
		return err
	}
	for _, existing := range sources {
		if existing.URL == source.URL {
			return nil
		}
	}
	source.Enabled = true
	if source.Kind == "" {
		source.Kind = domain.SourceSubscription
	}
	sources = append(sources, source)
	sort.Slice(sources, func(i, j int) bool { return sources[i].URL < sources[j].URL })
	content, err := json.MarshalIndent(sourceFile{Version: 1, Sources: sources}, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".next"
	if err := os.WriteFile(temporary, content, 0644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
