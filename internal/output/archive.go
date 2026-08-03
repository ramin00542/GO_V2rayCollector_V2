package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/state"
)

func PublishDaily(archiveRoot string, entries []state.Entry, day time.Time, options SnapshotOptions) error {
	start, end := DayBounds(day)
	destination := filepath.Join(archiveRoot, "daily", start.Format("2006-01-02"))
	return Publish(destination, entries, start, end, options)
}

func PublishRolling(archiveRoot string, entries []state.Entry, now time.Time, days int, options SnapshotOptions) error {
	if days < 1 {
		return fmt.Errorf("rolling days must be positive")
	}
	end := now.UTC().Add(time.Nanosecond)
	start := end.AddDate(0, 0, -days)
	return Publish(filepath.Join(archiveRoot, "all"), entries, start, end, options)
}

func PruneDaily(archiveRoot string, today time.Time, days int) error {
	if days < 1 {
		return fmt.Errorf("daily retention days must be positive")
	}
	root := filepath.Join(archiveRoot, "daily")
	items, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff, _ := DayBounds(today.UTC().AddDate(0, 0, -days+1))
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		day, err := time.Parse("2006-01-02", item.Name())
		if err != nil {
			continue
		}
		if day.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(root, item.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func SortedEntries(data state.Data) []state.Entry {
	entries := make([]state.Entry, 0, len(data.Entries))
	for _, entry := range data.Entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Fingerprint < entries[j].Fingerprint })
	return entries
}

