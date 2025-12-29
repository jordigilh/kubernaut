#!/bin/bash
# Compare current benchmark results against baseline
# Usage: ./scripts/compare-performance-baseline.sh <current_results.txt> <baseline.json>

set -e

CURRENT_FILE=$1
BASELINE_FILE=$2

if [[ ! -f "$CURRENT_FILE" ]]; then
    echo "❌ Error: Current results file not found: $CURRENT_FILE"
    exit 1
fi

if [[ ! -f "$BASELINE_FILE" ]]; then
    echo "❌ Error: Baseline file not found: $BASELINE_FILE"
    exit 1
fi

# Check for required tools
if ! command -v jq &> /dev/null; then
    echo "❌ Error: jq is required but not installed"
    exit 1
fi

if ! command -v bc &> /dev/null; then
    echo "❌ Error: bc is required but not installed"
    exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Performance Baseline Comparison"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Parse baseline values
BASELINE_P95=$(jq -r '.audit_write.p95_latency_ms' "$BASELINE_FILE")
BASELINE_P99=$(jq -r '.audit_write.p99_latency_ms' "$BASELINE_FILE")
BASELINE_QPS=$(jq -r '.audit_write.qps' "$BASELINE_FILE")
REGRESSION_TOLERANCE=$(jq -r '.thresholds.regression_tolerance' "$BASELINE_FILE")

# Parse current results (assuming Go benchmark format)
# Example line: "BenchmarkAuditWrite-8   	    1000	   1234567 ns/op"
CURRENT_P95=$(grep "p95" "$CURRENT_FILE" | awk '{print $2}' | sed 's/ms//')
CURRENT_P99=$(grep "p99" "$CURRENT_FILE" | awk '{print $2}' | sed 's/ms//')
CURRENT_QPS=$(grep "qps" "$CURRENT_FILE" | awk '{print $2}')

# Handle missing values
if [[ -z "$CURRENT_P95" ]]; then
    echo "⚠️  Warning: Could not parse p95 from current results"
    CURRENT_P95=0
fi

if [[ -z "$CURRENT_P99" ]]; then
    echo "⚠️  Warning: Could not parse p99 from current results"
    CURRENT_P99=0
fi

if [[ -z "$CURRENT_QPS" ]]; then
    echo "⚠️  Warning: Could not parse QPS from current results"
    CURRENT_QPS=0
fi

# Calculate thresholds
P95_THRESHOLD=$(echo "$BASELINE_P95 * $REGRESSION_TOLERANCE" | bc)
P99_THRESHOLD=$(echo "$BASELINE_P99 * $REGRESSION_TOLERANCE" | bc)
QPS_THRESHOLD=$(echo "$BASELINE_QPS / $REGRESSION_TOLERANCE" | bc)

# Initialize regression flag
REGRESSION_DETECTED=0

# Compare p95
echo "📊 p95 Latency:"
echo "   Baseline: ${BASELINE_P95}ms"
echo "   Current:  ${CURRENT_P95}ms"
echo "   Threshold: ${P95_THRESHOLD}ms (${REGRESSION_TOLERANCE}x baseline)"

if (( $(echo "$CURRENT_P95 > $P95_THRESHOLD" | bc -l) )); then
    echo "   ❌ REGRESSION: p95 latency exceeds threshold"
    REGRESSION_DETECTED=1
else
    CHANGE=$(echo "scale=1; (($CURRENT_P95 - $BASELINE_P95) / $BASELINE_P95) * 100" | bc)
    echo "   ✅ PASS: Within threshold (${CHANGE}% change)"
fi
echo ""

# Compare p99
echo "📊 p99 Latency:"
echo "   Baseline: ${BASELINE_P99}ms"
echo "   Current:  ${CURRENT_P99}ms"
echo "   Threshold: ${P99_THRESHOLD}ms (${REGRESSION_TOLERANCE}x baseline)"

if (( $(echo "$CURRENT_P99 > $P99_THRESHOLD" | bc -l) )); then
    echo "   ❌ REGRESSION: p99 latency exceeds threshold"
    REGRESSION_DETECTED=1
else
    CHANGE=$(echo "scale=1; (($CURRENT_P99 - $BASELINE_P99) / $BASELINE_P99) * 100" | bc)
    echo "   ✅ PASS: Within threshold (${CHANGE}% change)"
fi
echo ""

# Compare QPS
echo "📊 Queries Per Second (QPS):"
echo "   Baseline: ${BASELINE_QPS}"
echo "   Current:  ${CURRENT_QPS}"
echo "   Threshold: ${QPS_THRESHOLD} (baseline / ${REGRESSION_TOLERANCE})"

if (( $(echo "$CURRENT_QPS < $QPS_THRESHOLD" | bc -l) )); then
    echo "   ❌ REGRESSION: QPS below threshold"
    REGRESSION_DETECTED=1
else
    CHANGE=$(echo "scale=1; (($CURRENT_QPS - $BASELINE_QPS) / $BASELINE_QPS) * 100" | bc)
    echo "   ✅ PASS: Within threshold (${CHANGE}% change)"
fi
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [[ $REGRESSION_DETECTED -eq 1 ]]; then
    echo "❌ Performance regression detected!"
    echo ""
    echo "Action Required:"
    echo "  1. Review recent changes for performance impact"
    echo "  2. Profile the service to identify bottlenecks"
    echo "  3. If intentional, update baseline: .perf-baseline.json"
    exit 1
else
    echo "✅ Performance within baseline thresholds"
    exit 0
fi


