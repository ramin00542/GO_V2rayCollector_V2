package health

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive_no_config"
	StatusNotFound Status = "not_found"
	StatusUnknown  Status = "unknown_error"
	StatusDormant  Status = "dormant"
)

type Record struct {
	Status              Status    `json:"status"`
	LastCheckedAt       time.Time `json:"last_checked_at"`
	LastSuccessfulAt    time.Time `json:"last_successful_at,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastError           string    `json:"last_error,omitempty"`
}

type Data struct {
	Channels map[string]Record `json:"channels"`
	Sources  map[string]Record `json:"sources"`
}
type Store struct {
	path string
	data Data
}

func Open(path string) (*Store, error) {
	store := &Store{path: path, data: Data{Channels: map[string]Record{}, Sources: map[string]Record{}}}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read health state: %w", err)
	}
	if err := json.Unmarshal(content, &store.data); err != nil {
		return nil, fmt.Errorf("decode health state: %w", err)
	}
	if store.data.Channels == nil {
		store.data.Channels = map[string]Record{}
	}
	if store.data.Sources == nil {
		store.data.Sources = map[string]Record{}
	}
	return store, nil
}

func (s *Store) Channel(name string) (Record, bool) {
	record, ok := s.data.Channels[name]
	return record, ok
}
func (s *Store) Source(url string) (Record, bool) {
	record, ok := s.data.Sources[url]
	return record, ok
}
func (s *Store) UpdateChannel(name string, status Status, errorText string, at time.Time) {
	s.data.Channels[name] = update(s.data.Channels[name], status, errorText, at)
}
func (s *Store) UpdateSource(url string, status Status, errorText string, at time.Time) {
	s.data.Sources[url] = update(s.data.Sources[url], status, errorText, at)
}
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.path + ".next"
	if err := os.WriteFile(temporary, content, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, s.path)
}

func update(previous Record, status Status, errorText string, at time.Time) Record {
	next := Record{Status: status, LastCheckedAt: at, LastSuccessfulAt: previous.LastSuccessfulAt}
	if status == StatusUnknown {
		next.ConsecutiveFailures = previous.ConsecutiveFailures + 1
		next.LastError = errorText
		return next
	}
	next.ConsecutiveFailures = 0
	if status == StatusActive {
		next.LastSuccessfulAt = at
	}
	return next
}

func (s *Store) Data() Data { return s.data }

func ShouldDormant(record Record, now time.Time, days int) bool {
	if record.Status == StatusDormant {
		return true
	}
	cutoff := now.AddDate(0, 0, -days)
	if !record.LastSuccessfulAt.IsZero() {
		return record.LastSuccessfulAt.Before(cutoff)
	}
	return !record.LastCheckedAt.IsZero() && record.LastCheckedAt.Before(cutoff)
}
func (s *Store) MarkChannelDormant(name string, now time.Time) {
	record := s.data.Channels[name]
	record.Status = StatusDormant
	record.LastCheckedAt = now
	s.data.Channels[name] = record
}
func (s *Store) MarkSourceDormant(url string, now time.Time) {
	record := s.data.Sources[url]
	record.Status = StatusDormant
	record.LastCheckedAt = now
	s.data.Sources[url] = record
}
