package repository

import (
	"encoding/csv"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
)

func LoadChannels(path string) ([]domain.Channel, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open channels file: %w", err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read channels CSV: %w", err)
	}

	seen := make(map[string]bool)
	channels := make([]domain.Channel, 0, len(rows))
	for index, row := range rows {
		if len(row) == 0 || (index == 0 && strings.EqualFold(strings.TrimSpace(row[0]), "url")) {
			continue
		}

		// Extract the first column (URL or name)
		rawURL := strings.TrimSpace(row[0])

		// Try to extract channel name from URL or use as-is
		name := NormalizeTelegramChannel(rawURL)

		// If normalization failed, try to extract from URL format
		if name == "" {
			// Try to parse as URL
			if strings.Contains(rawURL, "t.me") || strings.Contains(rawURL, "telegram.me") {
				// Extract path from URL
				parsed, err := url.Parse(rawURL)
				if err == nil && parsed.Path != "" {
					// Remove leading slash and "s/" if present
					path := strings.TrimPrefix(parsed.Path, "/s/")
					path = strings.TrimPrefix(path, "/")
					name = NormalizeTelegramChannel(path)
				}
			}
		}

		if name == "" || seen[name] {
			continue
		}
		enabled := true
		if len(row) > 1 && strings.EqualFold(strings.TrimSpace(row[1]), "false") {
			enabled = false
		}
		note := ""
		if len(row) > 2 {
			note = strings.TrimSpace(row[2])
		}
		seen[name] = true
		channels = append(channels, domain.Channel{
			URL: "https://t.me/s/" + name, Name: name, Enabled: enabled, Note: note,
		})
	}
	return channels, nil
}

func NormalizeTelegramChannel(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimPrefix(value, "@")

	// Remove various Telegram URL prefixes
	prefixes := []string{
		"https://t.me/s/",
		"http://t.me/s/",
		"https://t.me/",
		"http://t.me/",
		"t.me/s/",
		"t.me/",
	}

	for _, prefix := range prefixes {
		value = strings.TrimPrefix(value, prefix)
	}

	// Remove trailing characters
	value = strings.Trim(value, "/ .,;:!?#")

	// Validate the channel name
	if value == "" || strings.ContainsAny(value, "?#/ ") {
		return ""
	}

	// Check if all characters are valid for Telegram channel names
	for _, r := range value {
		if !(unicode.IsLower(r) || unicode.IsDigit(r) || r == '_') {
			return ""
		}
	}

	return value
}

func AddChannel(path string, name string) error {
	channels, err := LoadChannels(path)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if channel.Name == name {
			return nil
		}
	}
	channels = append(channels, domain.Channel{URL: "https://t.me/s/" + name, Name: name, Enabled: true, Note: "auto-discovered"})
	rows := [][]string{{"url", "enabled", "note"}}
	for _, channel := range channels {
		rows = append(rows, []string{channel.URL, strconv.FormatBool(channel.Enabled), channel.Note})
	}
	temporary := path + ".next"
	file, err := os.Create(temporary)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	err = writer.WriteAll(rows)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, path)
}
