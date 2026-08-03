package health

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUnknownFailureDoesNotBecomeInactive(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "health.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	store.UpdateChannel("example", StatusUnknown, "timeout", now)
	record, ok := store.Channel("example")
	if !ok || record.Status != StatusUnknown || record.ConsecutiveFailures != 1 {
		t.Fatalf("unexpected record: %#v", record)
	}
	store.UpdateChannel("example", StatusActive, "", now.Add(time.Minute))
	record, _ = store.Channel("example")
	if record.Status != StatusActive || record.ConsecutiveFailures != 0 {
		t.Fatalf("active state did not reset: %#v", record)
	}
}

