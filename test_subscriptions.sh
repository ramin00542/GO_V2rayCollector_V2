#!/bin/bash

# Script to test all subscriptions and generate reports
# Usage: ./test_subscriptions.sh [max_configs]

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

MAX_CONFIGS="${1:-50}"  # Default to 50 configs, or use first argument

echo "=== Subscription Config Tester ==="
echo "Root directory: $ROOT_DIR"
echo "Max configs to test: $MAX_CONFIGS"
echo ""

# Check if the binary exists
if [ ! -f "v2collector" ]; then
    echo "Building v2collector..."
    go build ./cmd/v2collector
fi

# Create reports directory if it doesn't exist
mkdir -p reports/subscriptions

# Test all subscriptions from config/sources.json
echo "Testing subscriptions from config/sources.json..."
./v2collector -root . test-configs "$MAX_CONFIGS"

echo ""
echo "=== Test Complete ==="
echo "Reports saved in: $ROOT_DIR/reports/"
echo "Subscription reports saved in: $ROOT_DIR/reports/subscriptions/"
