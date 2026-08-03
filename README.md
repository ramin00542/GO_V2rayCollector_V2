# V2rayCollector

A clean Go collector for **publicly accessible** Telegram channel pages, configured subscription URLs, and explicitly configured files in public GitHub forks.

> Add only public sources you are allowed to retrieve. This project does not access private channels, bypass logins, evade rate limits, or bypass access controls.

## What it does

- Fetches public Telegram channel pages listed in `config/channels.csv`.
- Fetches HTTPS subscription sources listed in `config/sources.json`.
- Optionally discovers explicit file paths in public forks of one configured GitHub repository.
- Parses supported URL-style proxy/VPN formats; validates basic syntax; rejects explicit insecure transport flags; canonicalizes items; and deduplicates them with SHA-256 fingerprints.
- Builds deduplicated, source-separated snapshots after every collection run.
- Keeps a daily archive for 30 days and a rolling `archive/all` snapshot for 7 days.
- Tracks channel and source health without deleting user-managed input configuration.

## Repository layout

```text
config/               User-managed inputs and policies
data/state/           Persistent runtime state, committed by Actions
output/temporary/     Current UTC-day snapshot, rebuilt every collection run
archive/daily/        Daily snapshots, retained for 30 days
archive/all/          Rolling 7-day snapshot
reports/              Latest collector and health reports
```

## Configuration

### Telegram channels — `config/channels.csv`

```csv
url,enabled,note
https://t.me/s/example_channel,true,public source
@example_channel,true,also accepted
```

Channel URLs are normalized and deduplicated. Disabled channels stay in the file but are not fetched.

### Subscription sources — `config/sources.json`

```json
{
  "version": 1,
  "sources": [
    {
      "url": "https://example.org/subscription.txt",
      "enabled": true,
      "name": "Example",
      "kind": "subscription"
    }
  ]
}
```

Only HTTPS URLs are accepted. A source is never removed automatically because of a temporary error.

### GitHub discovery — `config/github.json`

```json
{
  "enabled": false,
  "repository": "owner/repository",
  "max_forks": 30,
  "max_pages": 1,
  "paths": ["sub.txt", "subscription.txt", "configs.txt"]
}
```

When enabled, the collector discovers only these explicit paths in a bounded number of public forks. Set `GITHUB_TOKEN` as an environment variable or GitHub Actions secret for better API limits; never put a token in configuration files.

### Retention and output — `config/collector.json`

```json
{
  "version": 1,
  "retention": {"daily_days": 30, "rolling_days": 7},
  "output": {"keep_unknown": false, "write_per_channel": true}
}
```

## Output structure

Telegram and Subscription outputs remain separate. HTTPS is a dedicated protocol file. MTProto and Telegram SOCKS are also separate from normal protocols.

```text
output/temporary/
├── telegram/
│   ├── protocols/
│   │   ├── vless.txt
│   │   ├── vmess.txt
│   │   ├── https.txt
│   │   └── ...
│   ├── telegram-proxies/
│   │   ├── mtproto.txt
│   │   └── telegram-socks.txt
│   └── channels/<channel>/<protocol>.txt
└── subscription/
    ├── protocols/
    └── telegram-proxies/
```

The same layout exists in `archive/daily/YYYY-MM-DD/` and `archive/all/`.

### Deduplication policy: selected mode A

- A configuration occurs at most once inside each generated file.
- A configuration seen again on a later day can occur once in that later day's daily snapshot.
- A display fragment such as `#server-name` does not make the same endpoint a new item.
- The internal state records multiple origins for the same fingerprint.

## Commands

```bash
go run ./cmd/v2collector check-config
go run ./cmd/v2collector collect
go run ./cmd/v2collector scan-channels
go run ./cmd/v2collector revive-channels
go run ./cmd/v2collector check-sources
```

Before a first upload, run:

```bash
go mod download
gofmt -w $(find . -name '*.go' -not -path './vendor/*')
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/v2collector
```

## GitHub Actions

- `validate.yml`: runs on pushes and pull requests; formats, vets, tests, builds, and validates configuration.
- `collect.yml`: scheduled every 20 minutes; writes only `data/state/`, `output/`, `archive/`, and `reports/`.
- `channel-health.yml`: daily channel health scan.
- `source-health.yml`: weekly source health scan.

All writer workflows share one concurrency lock, so they do not commit simultaneously. In GitHub repository settings, enable:

```text
Settings → Actions → General → Workflow permissions → Read and write permissions
```

## Supported protocol registry

The registry covers the common URL-style formats used by the collector: VMess, VLESS, Trojan, Shadowsocks, ShadowsocksR, Hysteria, Hysteria2/Hy2, TUIC, WireGuard URL, WARP, SOCKS, SOCKS5, HTTP, HTTPS, MTProto, Telegram SOCKS, SSH, NaiveProxy, Brook, Argo, Slipnet, and Invizible.

Some file-oriented or multiline formats, such as OpenVPN and WireGuard `.conf`, require dedicated parsers and are intentionally not accepted as generic URLs until those parsers are added.

## Security and reliability rules

- No private source access or access-control bypass.
- Bounded HTTP response size, timeouts, retry, HTTPS-only redirects, and context cancellation.
- Explicit `allowInsecure`, `insecure`, and `allow_insecure` flags are rejected.
- A network failure is `unknown_error`, not a dead channel/source.
- Output/state/archive writes use temporary paths and atomic rename.

## License

MIT. See [LICENSE](LICENSE).

## Discovery and automatic promotion

Public links discovered inside monitored Telegram content, subscription content, and configured GitHub fork files enter a persistent candidate queue. They are not immediately added to the user-managed lists.

- Candidate channels must themselves contain valid configurations; merely forwarding another link is not enough.
- Candidate subscriptions must return at least one valid configuration.
- Promotion requires 3 independent successful checks separated by at least 6 hours.
- Per-run request budgets are replacement budgets, not discard limits: failed/no-config candidates are replaced with the next queued candidate, and untested candidates remain queued for later runs.
- The defaults are 200 candidate channel fetches and 200 candidate source fetches per run, with a target of 50 qualified candidates. All values are configurable in `config/collector.json`.
- Candidates expire after 14 days by default. A source/channel with no successful configuration for 365 days becomes `dormant` and is skipped by normal health scans.

Reports are refreshed in `reports/`: `collector_stats.md`, `channels_report.md`, `sources_report.md`, `discovery_report.md`, `links.md`, `manifest.json`, and `history.csv`.
