package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RaminTabriz/V2rayCollector/internal/domain"
)

type Observation struct {
	Kind       domain.SourceKind `json:"kind"`
	Channel    string            `json:"channel,omitempty"`
	LastSeenAt time.Time         `json:"last_seen_at"`
}

type Entry struct {
	Value        string                 `json:"value"`
	Protocol     domain.Protocol        `json:"protocol"`
	Fingerprint  string                 `json:"fingerprint"`
	FirstSeenAt  time.Time              `json:"first_seen_at"`
	LastSeenAt   time.Time              `json:"last_seen_at"`
	Observations map[string]Observation `json:"observations"`
}

type Data struct {
	CurrentDay string           `json:"current_day"`
	Entries    map[string]Entry `json:"entries"`
}

type Store struct {
	path string
	data Data
}

func Open(path string) (*Store, error) {
	store := &Store{path: path, data: Data{Entries: make(map[string]Entry)}}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(content, &store.data); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	if store.data.Entries == nil {
		store.data.Entries = make(map[string]Entry)
	}
	return store, nil
}

func (s *Store) Data() Data               { return s.data }
func (s *Store) SetCurrentDay(day string) { s.data.CurrentDay = day }

func (s *Store) Upsert(config domain.ParsedConfig, kind domain.SourceKind, channel string, at time.Time) bool {
	entry, exists := s.data.Entries[config.Fingerprint]
	if !exists {
		entry = Entry{Value: config.Value, Protocol: config.Protocol, Fingerprint: config.Fingerprint, FirstSeenAt: at, Observations: make(map[string]Observation)}
	}
	entry.LastSeenAt = at
	if entry.Observations == nil {
		entry.Observations = make(map[string]Observation)
	}
	key := string(kind) + ":" + strings.ToLower(channel)
	entry.Observations[key] = Observation{Kind: kind, Channel: strings.ToLower(channel), LastSeenAt: at}
	s.data.Entries[config.Fingerprint] = entry
	return !exists
}

func (s *Store) Prune(before time.Time) {
	for fingerprint, entry := range s.data.Entries {
		if entry.LastSeenAt.Before(before) {
			delete(s.data.Entries, fingerprint)
		}
	}
}

func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	content, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	temporary := s.path + ".next"
	if err := os.WriteFile(temporary, content, 0600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(temporary, s.path); err != nil {
		return fmt.Errorf("publish state: %w", err)
	}
	return nil
}

func EntriesForWindow(data Data, start, end time.Time) []Entry {
	entries := make([]Entry, 0)
	for _, entry := range data.Entries {
		for _, observation := range entry.Observations {
			if !observation.LastSeenAt.Before(start) && observation.LastSeenAt.Before(end) {
				entries = append(entries, entry)
				break
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Fingerprint < entries[j].Fingerprint })
	return entries
}
