package domain

import "time"

type SourceKind string

const (
	SourceTelegram     SourceKind = "telegram"
	SourceSubscription SourceKind = "subscription"
	SourceGitHubFork   SourceKind = "github_fork"
)

type Channel struct {
	URL     string
	Name    string
	Enabled bool
	Note    string
}

type Source struct {
	URL     string     `json:"url"`
	Enabled bool       `json:"enabled"`
	Name    string     `json:"name,omitempty"`
	Kind    SourceKind `json:"kind"`
}

type Retention struct {
	DailyDays   int `json:"daily_days"`
	RollingDays int `json:"rolling_days"`
}

type OutputPolicy struct {
	KeepUnknown     bool `json:"keep_unknown"`
	WritePerChannel bool `json:"write_per_channel"`
}

type DiscoveryPolicy struct {
	ChannelFetchBudget      int `json:"channel_candidate_fetch_budget"`
	SourceFetchBudget       int `json:"source_candidate_fetch_budget"`
	QualifiedTarget         int `json:"qualified_target"`
	PromotionSuccessCount   int `json:"promotion_success_count"`
	PromotionMinIntervalHrs int `json:"promotion_min_interval_hours"`
	CandidateExpiryDays     int `json:"candidate_expiry_days"`
	DormantAfterDays        int `json:"dormant_after_days"`
}

type CollectorSettings struct {
	Version   int             `json:"version"`
	Retention Retention       `json:"retention"`
	Output    OutputPolicy    `json:"output"`
	Discovery DiscoveryPolicy `json:"discovery"`
}

type GitHubSettings struct {
	Enabled    bool     `json:"enabled"`
	Repository string   `json:"repository"`
	MaxForks   int      `json:"max_forks"`
	MaxPages   int      `json:"max_pages"`
	Paths      []string `json:"paths"`
}

type ConfigRecord struct {
	Value       string
	Protocol    Protocol
	Fingerprint string
	Sources     []SourceKind
	Channels    []string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
}

type ParsedConfig struct {
	Value       string
	Protocol    Protocol
	Canonical   string
	Fingerprint string
}

type ProviderResult struct {
	SourceURL  string
	SourceKind SourceKind
	HTTPStatus int
	BytesRead  int
	Extracted  int
	Accepted   int
	Rejected   int
	Configs    []ParsedConfig
	Error      string
	Discovered []DiscoveredLink
}

type DiscoveryKind string

const (
	DiscoveryChannel DiscoveryKind = "channel"
	DiscoverySource  DiscoveryKind = "source"
)

type DiscoveredLink struct {
	Kind  DiscoveryKind
	Value string
}

