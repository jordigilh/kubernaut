#!/bin/bash
# Run all unit tests for all services (Go + Python)
# This script runs unit tests for:
# - All Go services in cmd/
# - authwebhook (separate test target)
# - holmesgpt-api (Python service)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=========================================="
echo "Running ALL Unit Tests"
echo "=========================================="
echo ""

# Track failures
FAILED_SERVICES=()
TOTAL_SERVICES=0

# Function to run test and track result
run_test() {
    local service=$1
    TOTAL_SERVICES=$((TOTAL_SERVICES + 1))

    echo "=========================================="
    echo "[$TOTAL_SERVICES] Testing: $service"
    echo "=========================================="

    if make test-unit-$service; then
        echo "✅ PASSED: $service"
    else
        echo "❌ FAILED: $service"
        FAILED_SERVICES+=("$service")
    fi
    echo ""
}

# Go services from cmd/
echo "=== Go Services from cmd/ ==="
for service in aianalysis datastorage gateway notification remediationorchestrator signalprocessing webhooks workflowexecution; do
    run_test "$service"
done

# Special services
echo "=== Special Services ==="
run_test "authwebhook"
run_test "holmesgpt-api"

# Summary
echo "=========================================="
echo "UNIT TEST SUMMARY"
echo "=========================================="
echo "Total services tested: $TOTAL_SERVICES"
echo "Passed: $((TOTAL_SERVICES - ${#FAILED_SERVICES[@]}))"
echo "Failed: ${#FAILED_SERVICES[@]}"
echo ""

if [ ${#FAILED_SERVICES[@]} -gt 0 ]; then
    echo "❌ FAILED SERVICES:"
    for service in "${FAILED_SERVICES[@]}"; do
        echo "  - $service"
    done
    echo ""
    exit 1
else
    echo "✅ ALL UNIT TESTS PASSED"
    exit 0
fi
