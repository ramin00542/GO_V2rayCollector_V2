#!/bin/bash

# Script to test a single subscription URL and generate reports
# Usage: ./test_single_sub.sh <subscription_url> [output_dir]

set -e

if [ $# -lt 1 ]; then
    echo "Usage: $0 <subscription_url> [output_dir]"
    echo "Example: $0 https://example.com/sub.txt"
    exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

SUB_URL="$1"
OUTPUT_DIR="${2:-reports/subscriptions}"

echo "=== Testing Subscription: $SUB_URL ==="
echo "Output directory: $OUTPUT_DIR"
echo ""

# Check if the binary exists
if [ ! -f "v2collector" ]; then
    echo "Building v2collector..."
    go build ./cmd/v2collector
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Test the subscription
./v2collector -root . test-subscription "$SUB_URL"

echo ""
echo "=== Test Complete ==="
echo "Reports saved in: $OUTPUT_DIR/"

# Show the latest report
LATEST_MD=$(find "$OUTPUT_DIR" -name "*.md" -type f -printf '%T@ %p\n' | sort -n | tail -1 | cut -f2- -d" ")
if [ -n "$LATEST_MD" ]; then
    echo ""
    echo "=== Latest Report ==="
    cat "$LATEST_MD"
fi
