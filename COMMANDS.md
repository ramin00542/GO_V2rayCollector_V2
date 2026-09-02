# GO_V2rayCollector_V2 - Command Reference

This document provides a complete reference for all available commands in the GO_V2rayCollector_V2 project.

## 📋 Command Overview

| Command | Description | Arguments |
|---------|-------------|-----------|
| `check-config` | Validate configuration files | None |
| `collect` | Collect configs from all sources | None |
| `scan-channels` | Check health of Telegram channels | None |
| `revive-channels` | Check and revive inactive channels | None |
| `check-sources` | Check health of subscription sources | None |
| `test-configs` | Test all collected configs | `[max_configs]` |
| `test-subscription` | Test a specific subscription URL | `<url>` |
| `test-file` | Test configs from a local file | `<file_path>` |
| `test-manual` | Test a single config or subscription | `<config_or_url>` |

---

## 🔍 Collection Commands

### check-config

Validates all configuration files to ensure they are properly formatted.

**Usage:**
```bash
./v2collector check-config
```

**What it does:**
- Validates `config/channels.csv`
- Validates `config/sources.json`
- Validates `config/collector.json`
- Validates `config/github.json`
- Reports any validation errors

**Example Output:**
```
configuration valid: channels=10 sources=5 daily_retention=30 rolling_retention=7 github_enabled=false
```

---

### collect

Collects V2Ray/Nexus configurations from all configured sources.

**Usage:**
```bash
./v2collector collect
```

**What it does:**
1. Loads Telegram channels from `config/channels.csv`
2. Fetches configs from each channel
3. Loads subscription sources from `config/sources.json`
4. Fetches configs from each subscription URL
5. If GitHub discovery is enabled, discovers configs from GitHub forks
6. Parses and validates all configs
7. Deduplicates configs using SHA-256 fingerprints
8. Saves results to:
   - `data/state/configs.json` - All configs with metadata
   - `data/state/candidates.json` - Candidate configs for discovery
   - `output/temporary/` - Current day's snapshot
   - `archive/daily/` - Daily snapshots
   - `archive/all/` - Rolling snapshot
   - `reports/` - Various reports

**Example Output:**
```
collection complete: new=25 requests=15 succeeded=12 failed=3 accepted=20 rejected=5
```

---

## 🏥 Health Check Commands

### scan-channels

Checks the health of all configured Telegram channels.

**Usage:**
```bash
./v2collector scan-channels
```

**What it does:**
- Tests each Telegram channel
- Records success/failure status
- Updates health state in `data/state/health.json`
- Generates report in `reports/channels-health.md`

**Example Output:**
```
Channel scan complete: checked=10
```

---

### revive-channels

Checks and attempts to revive inactive Telegram channels.

**Usage:**
```bash
./v2collector revive-channels
```

**What it does:**
- Only checks channels that were previously marked as inactive or not found
- Attempts to revive them if they are now working
- Updates health state
- Generates report in `reports/channels-health.md`

**Example Output:**
```
Channel revive scan complete: checked=3
```

---

### check-sources

Checks the health of all configured subscription sources.

**Usage:**
```bash
./v2collector check-sources
```

**What it does:**
- Tests each subscription source URL
- Records success/failure status
- Updates health state in `data/state/health.json`
- Generates report in `reports/sources-health.md`

**Example Output:**
```
source health check complete: checked=5
```

---

## 🧪 Testing Commands

### test-configs

Tests all collected configs against target websites.

**Usage:**
```bash
./v2collector test-configs [max_configs]
```

**Arguments:**
- `max_configs` (optional): Maximum number of configs to test (default: 50)

**What it does:**
1. Loads configs from output directories
2. Removes duplicates
3. Tests each config against all target sites from `config/target_sites.json`
4. Measures latency for each request
5. Generates comprehensive reports:
   - `reports/config_test_YYYYMMDD_HHMMSS.json` - JSON report
   - `reports/config_test_YYYYMMDD_HHMMSS.md` - Markdown report
   - `reports/individual/` - Individual config reports

**Example Output:**
```
Testing 50 unique configs against 20 target sites...

Test complete!
Total configs: 50
Valid configs: 45
Tested configs: 45
Working configs: 32
Reports saved to: reports/
```

---

### test-subscription

Tests a specific subscription URL.

**Usage:**
```bash
./v2collector test-subscription <url>
```

**Arguments:**
- `url` (required): The subscription URL to test

**What it does:**
1. Fetches configs from the subscription URL
2. Tests each config against all target sites
3. Generates reports in `reports/subscriptions/<sanitized_url>/`
4. Creates timestamped and `latest.json`/`latest.md` files

**Example:**
```bash
./v2collector test-subscription "https://example.com/sub.txt"
```

**Example Output:**
```
Testing subscription: https://example.com/sub.txt

Test complete for https://example.com/sub.txt!
Total configs: 25
Valid configs: 20
Working configs: 15
Reports saved to: reports/
```

---

### test-file

Tests configs from a local file.

**Usage:**
```bash
./v2collector test-file <file_path>
```

**Arguments:**
- `file_path` (required): Path to a file containing configs (one per line)

**What it does:**
1. Reads configs from the specified file
2. Tests each config against all target sites
3. Generates reports with the filename in the report name

**Example:**
```bash
./v2collector test-file /path/to/my_configs.txt
```

**Example Output:**
```
Testing file: /path/to/my_configs.txt

Test complete for /path/to/my_configs.txt!
Total configs: 10
Valid configs: 8
Working configs: 6
Reports saved to: reports/
```

---

### test-manual

Manually tests a single config or subscription URL.

**Usage:**
```bash
./v2collector test-manual <config_or_url>
```

**Arguments:**
- `config_or_url` (required): Either a config string (e.g., `vless://...`) or a subscription URL

**What it does:**
1. If the input is a URL, fetches configs from it
2. If the input is a config, tests it directly
3. Tests against all target sites
4. Generates reports
5. Prints detailed results to console

**Example with config:**
```bash
./v2collector test-manual "vless://user@server:443?type=ws&path=/vless"
```

**Example with subscription URL:**
```bash
./v2collector test-manual "https://example.com/sub.txt"
```

**Example Output:**
```
Testing single config: vless://user@server:443?type=ws&path=/vless

=== Test Results ===

Config: vless://user@server:443?type=ws&path=/vless
Type: VLESS
Valid: true
Success: 18/20 sites
  ✅ Google (120ms)
  ✅ YouTube (150ms)
  ❌ ChatGPT (timeout)
  ...

Reports saved to: reports/
```

---

## 📁 File Structure

```
GO_V2rayCollector_V2/
├── cmd/
│   └── v2collector/
│       └── main.go          # Main entry point
├── config/
│   ├── channels.csv         # Telegram channel configuration
│   ├── collector.json        # Collector settings
│   ├── github.json          # GitHub discovery settings
│   ├── sources.json         # Subscription source configuration
│   └── target_sites.json     # Target sites for testing
├── data/
│   └── state/
│       ├── configs.json     # All collected configs
│       ├── candidates.json   # Candidate configs for discovery
│       └── health.json       # Health status of channels/sources
├── output/
│   └── temporary/            # Current day's snapshot
├── archive/
│   ├── daily/                # Daily snapshots
│   │   └── YYYY-MM-DD/       # Snapshot for specific date
│   └── all/                 # Rolling snapshot
├── reports/
│   ├── *.md                  # Various reports
│   ├── *.json                # JSON reports
│   ├── subscriptions/        # Individual subscription test reports
│   │   └── <name>/
│   │       ├── *.json        # Timestamped reports
│   │       ├── *.md          # Markdown reports
│   │       ├── latest.json   # Symlink to latest JSON
│   │       └── latest.md     # Symlink to latest Markdown
│   └── individual/           # Individual config reports
│       └── *.json
└── internal/
    ├── tester/               # Testing system
    │   ├── config_test.go    # Core testing functionality
    │   ├── target_sites.go    # Target site management
    │   ├── report.go         # Report generation
    │   └── watcher.go        # Subscription monitoring
    └── ...                  # Other internal packages
```

---

## 🚀 Shell Scripts

For convenience, several shell scripts are provided:

### test_subscriptions.sh

Tests all collected configs.

**Usage:**
```bash
./test_subscriptions.sh [max_configs]
```

**Example:**
```bash
# Test with default limit (50)
./test_subscriptions.sh

# Test up to 100 configs
./test_subscriptions.sh 100
```

---

### test_single_sub.sh

Tests a single subscription URL.

**Usage:**
```bash
./test_single_sub.sh <subscription_url> [output_dir]
```

**Example:**
```bash
./test_single_sub.sh "https://example.com/sub.txt"
./test_single_sub.sh "https://example.com/sub.txt" custom/output
```

---

### test_all_subs.sh

Tests all subscriptions from `config/sources.json` individually.

**Usage:**
```bash
./test_all_subs.sh
```

**What it does:**
- Extracts all enabled subscription URLs from `config/sources.json`
- Tests each one individually
- Saves reports in `reports/subscriptions/<sanitized_url>/`
- Provides summary of results

---

## 📊 Common Workflows

### Daily Collection and Testing

```bash
# Collect new configs
./v2collector collect

# Test all collected configs
./v2collector test-configs

# Check health of sources
./v2collector check-sources
```

### Testing New Configurations

```bash
# Test a new subscription URL
./v2collector test-subscription "https://new-source.com/sub.txt"

# If it works well, add it to sources.json
```

### Monitoring Subscriptions

```bash
# Test all subscriptions
./test_all_subs.sh

# View latest reports
ls -la reports/subscriptions/*/latest.md
```

### Validating Configuration

```bash
# Check if configuration is valid
./v2collector check-config

# Fix any issues reported
```

---

## 🎯 Target Sites

The testing system uses a configurable list of target sites from `config/target_sites.json`. By default, it includes:

### Search Engines
- Google
- Bing
- DuckDuckGo
- Yahoo

### Video & Streaming
- YouTube
- Netflix
- Twitch

### Code & Development
- GitHub
- GitLab
- Stack Overflow

### Social Media
- Twitter/X
- Instagram
- Facebook
- Reddit
- LinkedIn

### News & Information
- BBC News
- CNN
- Reuters
- Wikipedia

### AI & Machine Learning
- Gemini AI
- Hugging Face
- Kaggle

### Cloud Services
- AWS
- Google Cloud
- Microsoft Azure

### Shopping
- Amazon
- eBay

### Speed Tests
- Fast.com
- Speedtest.net

You can customize this list by editing `config/target_sites.json`.

---

## 🔧 Customization

### Adding New Target Sites

Edit `config/target_sites.json`:

```json
{
  "version": 1,
  "sites": [
    {
      "name": "My Custom Site",
      "url": "https://mycustomsite.com",
      "category": "custom",
      "expected_status": 200,
      "timeout_seconds": 10
    }
  ],
  "test_settings": {
    "max_concurrent_tests": 10,
    "request_timeout": 15,
    "retry_count": 2,
    "user_agent": "Custom User Agent"
  }
}
```

### Adjusting Test Parameters

Modify the `test_settings` section:

- **max_concurrent_tests**: Number of configs to test simultaneously (default: 10)
- **request_timeout**: Timeout in seconds for each request (default: 15)
- **retry_count**: Number of times to retry failed requests (default: 2)
- **user_agent**: User agent string for HTTP requests

---

## 📈 Understanding Results

### Config Status

- **Valid**: The config has a valid format
- **Invalid**: The config format is incorrect
- **Working**: The config can successfully access at least one target site
- **Not Working**: The config cannot access any target sites

### Site Accessibility

- **✅ Success**: HTTP status code 200-299
- **❌ Failed**: HTTP status code 400-599 or connection error
- **⏳ Timeout**: Request timed out

### Reports

All reports include:
- Summary statistics
- Detailed config results
- Site accessibility statistics
- Invalid configs list
- Top working configs

---

## 🛠️ Troubleshooting

### "No configs found"

**Solution:** Run `collect` first to gather configs:
```bash
./v2collector collect
```

### "Configuration invalid"

**Solution:** Check the specific error message and fix the configuration file:
```bash
./v2collector check-config
```

### "Permission denied"

**Solution:** Make sure the reports directory is writable:
```bash
mkdir -p reports
chmod 755 reports
```

### Slow Testing

**Solution:** Reduce the number of concurrent tests or test fewer configs:
```bash
./v2collector test-configs 20  # Test only 20 configs
```

### All Configs Failing

**Solution:**
- Check your internet connection
- Verify that target sites are accessible without a proxy
- Some sites may be blocked in your region

---

## 📚 Additional Documentation

- [TESTING.md](TESTING.md) - Detailed testing system documentation
- [FIXES.md](FIXES.md) - List of fixes applied to the project
- [README.md](README.md) - Main project documentation

---

## 🎨 Command Summary Cheat Sheet

```bash
# === Collection ===
./v2collector collect                    # Collect all configs
./v2collector check-config              # Validate config files

# === Health Checks ===
./v2collector scan-channels              # Check Telegram channels
./v2collector revive-channels            # Revive inactive channels
./v2collector check-sources              # Check subscription sources

# === Testing ===
./v2collector test-configs              # Test all configs
./v2collector test-configs 100          # Test up to 100 configs
./v2collector test-subscription <url>   # Test specific subscription
./v2collector test-file <path>          # Test configs from file
./v2collector test-manual <config>      # Test single config/URL

# === Shell Scripts ===
./test_subscriptions.sh                 # Test all collected configs
./test_subscriptions.sh 100             # Test with limit
./test_single_sub.sh <url>             # Test single subscription
./test_all_subs.sh                      # Test all subscriptions
```
