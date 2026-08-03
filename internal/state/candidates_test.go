package state

import (
	"testing"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
)

func TestCandidatePromotionNeedsIndependentSuccesses(t *testing.T) {
	store, err := OpenCandidates(t.TempDir() + "/candidates.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	store.Observe(domain.DiscoveryChannel, "example", "telegram:seed", now)
	candidate := store.Eligible(domain.DiscoveryChannel, now, 10, 14)[0]
	updated := store.Result(candidate.ID, CandidateQualified, now, 3, 6*time.Hour)
	if updated.Status == CandidatePromoted || updated.Successes != 1 {
		t.Fatalf("unexpected first result: %#v", updated)
	}
	updated = store.Result(candidate.ID, CandidateQualified, now.Add(time.Hour), 3, 6*time.Hour)
	if updated.Successes != 1 {
		t.Fatal("success within interval must not count twice")
	}
	updated = store.Result(candidate.ID, CandidateQualified, now.Add(7*time.Hour), 3, 6*time.Hour)
	updated = store.Result(candidate.ID, CandidateQualified, now.Add(14*time.Hour), 3, 6*time.Hour)
	if updated.Status != CandidatePromoted {
		t.Fatalf("candidate was not promoted: %#v", updated)
	}
}

