package repository

import (
	"encoding/csv"
	"fmt"
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
		name := NormalizeTelegramChannel(row[0])
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
	for _, prefix := range []string{"https://t.me/s/", "http://t.me/s/", "https://t.me/", "http://t.me/"} {
		value = strings.TrimPrefix(value, prefix)
	}
	value = strings.Trim(value, "/ ")
	if value == "" || strings.ContainsAny(value, "?#/ ") {
		return ""
	}
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
