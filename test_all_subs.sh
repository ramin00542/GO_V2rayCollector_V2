#!/bin/bash

# Script to test all subscriptions from sources.json individually
# Each subscription gets its own report directory
# Usage: ./test_all_subs.sh

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

# Check if the binary exists
if [ ! -f "v2collector" ]; then
    echo "Building v2collector..."
    go build ./cmd/v2collector
fi

# Create reports directory
mkdir -p reports/subscriptions

# Extract subscription URLs from config/sources.json
echo "Extracting subscription URLs from config/sources.json..."
SUB_URLS=$(jq -r '.sources[] | select(.kind == "subscription" and .enabled == true) | .url' config/sources.json 2>/dev/null || echo "")

if [ -z "$SUB_URLS" ]; then
    echo "No subscriptions found in config/sources.json"
    echo "Trying to extract all URLs..."
    SUB_URLS=$(jq -r '.sources[] | select(.enabled == true) | .url' config/sources.json 2>/dev/null || echo "")
fi

if [ -z "$SUB_URLS" ]; then
    echo "Error: No subscriptions found or jq not installed"
    echo "Install jq with: sudo apt-get install jq (Debian/Ubuntu) or brew install jq (macOS)"
    exit 1
fi

TOTAL=0
SUCCESS=0
FAILED=0

echo ""
echo "=== Testing All Subscriptions ==="
echo ""

for SUB_URL in $SUB_URLS; do
    TOTAL=$((TOTAL + 1))
    echo "[$TOTAL] Testing: $SUB_URL"
    
    if ./v2collector -root . test-subscription "$SUB_URL" 2>&1; then
        SUCCESS=$((SUCCESS + 1))
        echo "✅ Success"
    else
        FAILED=$((FAILED + 1))
        echo "❌ Failed"
    fi
    echo ""
done

echo "=== Summary ==="
echo "Total: $TOTAL"
echo "Success: $SUCCESS"
echo "Failed: $FAILED"
echo ""
echo "Reports saved in: $ROOT_DIR/reports/subscriptions/"
