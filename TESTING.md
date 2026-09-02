# Config Testing System

This document describes the config testing system that allows you to test V2Ray/Nexus configurations against various target websites to determine which configs are working and which sites are accessible.

## Features

✅ **Automatic Config Testing** - Test all collected configs against multiple target sites  
✅ **Manual Testing** - Test individual configs or subscription URLs manually  
✅ **Subscription Monitoring** - Monitor subscription URLs and automatically re-test when updated  
✅ **Detailed Reports** - Generate comprehensive JSON and Markdown reports  
✅ **Individual Config Reports** - Save detailed results for each config  
✅ **Category Filtering** - Test against specific categories (AI, Social, Search, etc.)  

## Quick Start

### 1. Build the Project

```bash
cd GO_V2rayCollector_V2
go build ./cmd/v2collector
```

### 2. Test All Collected Configs

```bash
# Test all configs (default: up to 50 configs)
./v2collector test-configs

# Test with a specific limit
./v2collector test-configs 100  # Test up to 100 configs
```

### 3. Test a Specific Subscription URL

```bash
./v2collector test-subscription "https://example.com/subscription.txt"
```

### 4. Test Configs from a Local File

```bash
./v2collector test-file /path/to/configs.txt
```

### 5. Manual Testing

```bash
# Test a single config
./v2collector test-manual "vless://..."

# Test a subscription URL
./v2collector test-manual "https://example.com/sub.txt"
```

## Configuration

### Target Sites Configuration

The system tests configs against a list of target websites. You can customize this list in `config/target_sites.json`:

```json
{
  "version": 1,
  "sites": [
    {
      "name": "Google",
      "url": "https://www.google.com",
      "category": "search",
      "expected_status": 200,
      "timeout_seconds": 10
    },
    {
      "name": "YouTube",
      "url": "https://www.youtube.com",
      "category": "video",
      "expected_status": 200,
      "timeout_seconds": 10
    }
  ],
  "test_settings": {
    "max_concurrent_tests": 10,
    "request_timeout": 15,
    "retry_count": 2,
    "user_agent": "Mozilla/5.0 ..."
  }
}
```

### Test Settings

- **max_concurrent_tests**: Maximum number of configs to test simultaneously (default: 10)
- **request_timeout**: Timeout in seconds for each HTTP request (default: 15)
- **retry_count**: Number of retries for failed requests (default: 2)
- **user_agent**: User agent string for HTTP requests

## Output Files

All test reports are saved in the `reports/` directory:

```
reports/
├── config_test_YYYYMMDD_HHMMSS.json    # JSON report for all configs
├── config_test_YYYYMMDD_HHMMSS.md      # Markdown report for all configs
├── manual_test_YYYYMMDD_HHMMSS.json     # Manual test reports
├── manual_test_YYYYMMDD_HHMMSS.md
├── subscriptions/                       # Individual subscription test reports
│   └── <subscription_name>/
│       ├── YYYYMMDD_HHMMSS.json
│       ├── YYYYMMDD_HHMMSS.md
│       ├── latest.json -> YYYYMMDD_HHMMSS.json  # Symlink to latest
│       └── latest.md -> YYYYMMDD_HHMMSS.md
└── individual/                         # Individual config reports
    └── <config_fingerprint>.json
```

## Report Contents

### JSON Report

Contains detailed information about each config test:

```json
{
  "generated_at": "2024-01-01T12:00:00Z",
  "total_configs": 150,
  "valid_configs": 120,
  "tested_configs": 120,
  "working_configs": 85,
  "config_results": [
    {
      "config_value": "vless://...",
      "config_type": "VLESS",
      "is_valid": true,
      "total_success": 15,
      "total_failed": 5,
      "total_tested": 20,
      "site_results": {
        "Google": {
          "status_code": 200,
          "success": true,
          "latency_ms": 120000000,
          "tested_at": "2024-01-01T12:00:00Z"
        }
      }
    }
  ],
  "site_statistics": {
    "Google": {
      "total_tested": 85,
      "total_success": 60,
      "success_rate": 70.588
    }
  }
}
```

### Markdown Report

Human-readable report with tables and statistics:

```markdown
# Config Test Report

Generated: 2024-01-01T12:00:00Z

## Summary

- Total configs: 150
- Valid configs: 120
- Tested configs: 120
- Working configs: 85
- Valid rate: 80.0%
- Working rate: 70.8%

## Top Working Configs

| Rank | Config (short) | Type | Success Rate | Working Sites | Avg Latency |
|------|----------------|------|--------------|---------------|-------------|
| 1 | vless://abc... | VLESS | 95.0% | Google, YouTube | 120ms |

## Site Accessibility

| Site | Category | Tested | Success | Success Rate |
|------|----------|--------|---------|--------------|
| Google | search | 85 | 60 | 70.6% |

## Invalid Configs

| Config (short) | Error |
|----------------|-------|
| invalid://... | unknown protocol |
```

## Categories

The system categorizes target sites for easier filtering:

- **search**: Google, Bing, DuckDuckGo, Yahoo
- **video**: YouTube, Netflix, Twitch
- **code**: GitHub, GitLab, Stack Overflow
- **social**: Twitter/X, Instagram, Facebook, Reddit
- **news**: BBC, CNN, Reuters
- **ai**: Gemini AI, Hugging Face, Kaggle
- **cloud**: AWS, Google Cloud, Azure
- **shopping**: Amazon, eBay
- **speedtest**: Fast.com, Speedtest.net
- **communication**: Zoom, WhatsApp Web, Telegram Web

## Testing Specific Categories

You can filter sites by category by modifying the `target_sites.json` file or by creating custom site lists.

## Automatic Subscription Monitoring

The system can automatically monitor subscription URLs and re-test them periodically:

```go
// Example usage in your own code
ctx := context.Background()
sites := tester.DefaultTargetSites()
settings := tester.TestSettings{...}

watcher := tester.NewSubscriptionWatcher(
    "https://example.com/sub.txt",
    "reports/subscriptions",
    sites,
    settings,
    24 * time.Hour,  // Test every 24 hours
)

watcher.Start(ctx)
// ... later ...
watcher.Stop()
```

## Using the Shell Script

A convenience script is provided for testing all subscriptions:

```bash
# Make it executable
chmod +x test_subscriptions.sh

# Run with default settings (test 50 configs)
./test_subscriptions.sh

# Run with custom limit
./test_subscriptions.sh 100
```

## Understanding Results

### Success Criteria

A config is considered **working** if it can successfully access at least one target site with HTTP status code 200-299.

### Latency Measurement

The system measures the round-trip time for each request and calculates average latency for each config.

### Error Types

- **Validation Error**: The config format is invalid
- **Timeout**: The request timed out
- **HTTP Error**: The server returned an error status code (4xx, 5xx)
- **Connection Error**: Failed to establish a connection

## Customizing Tests

### Adding New Target Sites

Edit `config/target_sites.json` and add your sites:

```json
{
  "name": "My Site",
  "url": "https://mysite.com",
  "category": "custom",
  "expected_status": 200,
  "timeout_seconds": 10
}
```

### Adjusting Test Parameters

Modify the `test_settings` section in `config/target_sites.json`:

```json
"test_settings": {
  "max_concurrent_tests": 20,    // Test more configs simultaneously
  "request_timeout": 30,        // Wait longer for responses
  "retry_count": 3,             // Retry failed requests more times
  "user_agent": "Custom User Agent"
}
```

## Troubleshooting

### No Configs Found

Make sure you've run `collect` first to gather configs:

```bash
./v2collector collect
```

### All Configs Failing

- Check your internet connection
- Verify that the target sites are accessible without a proxy
- Some sites may block requests from certain regions

### Slow Testing

- Reduce `max_concurrent_tests` to lower system resource usage
- Increase `request_timeout` if you have a slow connection
- Test fewer configs at once

### Permission Errors

Make sure the `reports/` directory is writable:

```bash
mkdir -p reports
chmod 755 reports
```

## Integration with Collection

The testing system is designed to work seamlessly with the collection system:

1. **Collect** configs from various sources
2. **Test** the collected configs
3. **Analyze** the results to find working configs
4. **Monitor** subscriptions for updates

### Example Workflow

```bash
# Step 1: Collect configs
./v2collector collect

# Step 2: Test all collected configs
./v2collector test-configs

# Step 3: Test a specific subscription
./v2collector test-subscription "https://example.com/sub.txt"

# Step 4: View reports
cat reports/config_test_*.md
```

## Performance Considerations

- Testing many configs can take time (several minutes for hundreds of configs)
- The system uses concurrency to speed up testing
- Reduce `max_concurrent_tests` if you experience issues
- Testing against more sites increases accuracy but also increases time

## Security Notes

- All testing is done locally on your machine
- No data is sent to external servers (except for the HTTP requests to target sites)
- Config values are stored in reports but can be redacted if needed
- Use HTTPS URLs for all target sites to ensure encrypted connections

## Contributing

To add new features to the testing system:

1. Add new test types in `internal/tester/config_test.go`
2. Add new report formats in `internal/tester/report.go`
3. Update the CLI commands in `cmd/v2collector/main.go`
4. Add tests in `internal/tester/*_test.go`

## License

This testing system is part of the GO_V2rayCollector_V2 project and is licensed under the MIT License.
