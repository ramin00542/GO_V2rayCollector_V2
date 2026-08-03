package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RaminTabriz/V2rayCollector/internal/domain"
	"github.com/RaminTabriz/V2rayCollector/internal/state"
)

type SnapshotOptions struct {
	KeepUnknown     bool
	WritePerChannel bool
}

func Publish(root string, entries []state.Entry, start, end time.Time, options SnapshotOptions) error {
	next := root + ".next"
	if err := os.RemoveAll(next); err != nil {
		return err
	}
	if err := writeSnapshot(next, entries, start, end, options); err != nil {
		os.RemoveAll(next)
		return err
	}
	if err := os.RemoveAll(root); err != nil {
		os.RemoveAll(next)
		return err
	}
	return os.Rename(next, root)
}

func writeSnapshot(root string, entries []state.Entry, start, end time.Time, options SnapshotOptions) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return fmt.Errorf("create snapshot root: %w", err)
	}
	files := make(map[string]map[string]bool)
	for _, entry := range entries {
		if entry.Protocol == domain.ProtocolUnknown && !options.KeepUnknown {
			continue
		}
		info, ok := domain.ProtocolInfoFor(entry.Protocol)
		if !ok {
			continue
		}
		for _, observation := range entry.Observations {
			if observation.LastSeenAt.Before(start) || !observation.LastSeenAt.Before(end) {
				continue
			}
			sourceDir := "subscription"
			if observation.Kind == domain.SourceTelegram {
				sourceDir = "telegram"
			}
			directory := filepath.Join(root, sourceDir, "protocols")
			if info.TelegramProxy {
				directory = filepath.Join(root, sourceDir, "telegram-proxies")
			}
			add(files, filepath.Join(directory, info.FileName), entry.Value)
			if options.WritePerChannel && observation.Kind == domain.SourceTelegram && observation.Channel != "" {
				add(files, filepath.Join(root, "telegram", "channels", observation.Channel, info.FileName), entry.Value)
			}
		}
	}
	for filename, values := range files {
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
		items := make([]string, 0, len(values))
		for value := range values {
			items = append(items, value)
		}
		sort.Strings(items)
		if err := os.WriteFile(filename, []byte(strings.Join(items, "\n")+"\n"), 0644); err != nil {
			return fmt.Errorf("write output %s: %w", filename, err)
		}
	}
	return nil
}

func add(files map[string]map[string]bool, filename, value string) {
	if files[filename] == nil {
		files[filename] = make(map[string]bool)
	}
	files[filename][value] = true
}

func DayBounds(day time.Time) (time.Time, time.Time) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 0, 1)
}
