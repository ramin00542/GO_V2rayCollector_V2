package state

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
)

func TestUpsertTracksObservationsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	config := domain.ParsedConfig{Value: "vless://id@example.com:443?security=tls", Protocol: domain.ProtocolVLESS, Fingerprint: "fingerprint"}
	if !store.Upsert(config, domain.SourceTelegram, "example", now) {
		t.Fatal("first insert must be new")
	}
	if store.Upsert(config, domain.SourceSubscription, "", now.Add(time.Minute)) {
		t.Fatal("second insert must be duplicate")
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := reopened.Data().Entries["fingerprint"]
	if len(entry.Observations) != 2 {
		t.Fatalf("got %d observations", len(entry.Observations))
	}
}

