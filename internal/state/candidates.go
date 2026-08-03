package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RaminTabriz/V2rayCollector/internal/domain"
)

type CandidateStatus string

const (
	CandidatePending   CandidateStatus = "pending"
	CandidateQualified CandidateStatus = "qualified"
	CandidatePromoted  CandidateStatus = "promoted"
	CandidateNoConfig  CandidateStatus = "no_config"
	CandidateNotFound  CandidateStatus = "not_found"
	CandidateUnknown   CandidateStatus = "unknown_error"
	CandidateExpired   CandidateStatus = "expired"
)

type Candidate struct {
	ID            string               `json:"id"`
	Kind          domain.DiscoveryKind `json:"kind"`
	Value         string               `json:"value"`
	Status        CandidateStatus      `json:"status"`
	FirstSeenAt   time.Time            `json:"first_seen_at"`
	LastSeenAt    time.Time            `json:"last_seen_at"`
	LastCheckedAt time.Time            `json:"last_checked_at,omitempty"`
	LastSuccessAt time.Time            `json:"last_success_at,omitempty"`
	Successes     int                  `json:"successes"`
	NoConfigCount int                  `json:"no_config_count"`
	NotFoundCount int                  `json:"not_found_count"`
	Origins       map[string]time.Time `json:"origins"`
}
type CandidateData struct {
	Candidates map[string]Candidate `json:"candidates"`
}
type CandidateStore struct {
	path string
	data CandidateData
}

func OpenCandidates(path string) (*CandidateStore, error) {
	s := &CandidateStore{path: path, data: CandidateData{Candidates: map[string]Candidate{}}}
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(b, &s.data); e != nil {
		return nil, e
	}
	if s.data.Candidates == nil {
		s.data.Candidates = map[string]Candidate{}
	}
	return s, nil
}
func (s *CandidateStore) Observe(kind domain.DiscoveryKind, value, origin string, now time.Time) {
	id := string(kind) + ":" + strings.ToLower(value)
	c, ok := s.data.Candidates[id]
	if !ok {
		c = Candidate{ID: id, Kind: kind, Value: value, Status: CandidatePending, FirstSeenAt: now, Origins: map[string]time.Time{}}
	}
	c.LastSeenAt = now
	if c.Origins == nil {
		c.Origins = map[string]time.Time{}
	}
	c.Origins[origin] = now
	s.data.Candidates[id] = c
}
func (s *CandidateStore) Eligible(kind domain.DiscoveryKind, now time.Time, budget int, expiryDays int) []Candidate {
	var out []Candidate
	for _, c := range s.data.Candidates {
		if c.Kind != kind || c.Status == CandidatePromoted || c.Status == CandidateExpired {
			continue
		}
		if now.Sub(c.FirstSeenAt) > time.Duration(expiryDays)*24*time.Hour {
			c.Status = CandidateExpired
			s.data.Candidates[c.ID] = c
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	if len(out) > budget {
		out = out[:budget]
	}
	return out
}
func score(c Candidate) int {
	return c.Successes*100 + len(c.Origins)*20 - int(c.NoConfigCount)*30 - int(c.NotFoundCount)*50 + int(c.LastSeenAt.Unix()%17)
}
func (s *CandidateStore) Result(id string, status CandidateStatus, now time.Time, minSuccess int, minInterval time.Duration) Candidate {
	c := s.data.Candidates[id]
	c.LastCheckedAt = now
	c.Status = status
	if status == CandidateQualified {
		if c.LastSuccessAt.IsZero() || now.Sub(c.LastSuccessAt) >= minInterval {
			c.Successes++
			c.LastSuccessAt = now
		}
		if c.Successes >= minSuccess {
			c.Status = CandidatePromoted
		}
	}
	if status == CandidateNoConfig {
		c.NoConfigCount++
	}
	if status == CandidateNotFound {
		c.NotFoundCount++
	}
	s.data.Candidates[id] = c
	return c
}
func (s *CandidateStore) Data() CandidateData { return s.data }
func (s *CandidateStore) Save() error {
	if e := os.MkdirAll(filepath.Dir(s.path), 0755); e != nil {
		return e
	}
	b, e := json.MarshalIndent(s.data, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path + ".next"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path)
}
