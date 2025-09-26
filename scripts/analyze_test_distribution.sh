#!/bin/bash
# analyze_test_distribution.sh - Pyramid Test Migration Helper

echo "Current Test Distribution:"
echo "========================="

UNIT_TESTS=$(find test/unit/ -name "*_test.go" | wc -l)
INTEGRATION_TESTS=$(find test/integration/ -name "*_test.go" | wc -l)
E2E_TESTS=$(find test/e2e/ -name "*_test.go" | wc -l)
TOTAL_TESTS=$((UNIT_TESTS + INTEGRATION_TESTS + E2E_TESTS))

if [ $TOTAL_TESTS -eq 0 ]; then
    echo "No test files found!"
    exit 1
fi

UNIT_PERCENT=$((UNIT_TESTS * 100 / TOTAL_TESTS))
INTEGRATION_PERCENT=$((INTEGRATION_TESTS * 100 / TOTAL_TESTS))
E2E_PERCENT=$((E2E_TESTS * 100 / TOTAL_TESTS))

echo "Unit Tests: $UNIT_TESTS ($UNIT_PERCENT%)"
echo "Integration Tests: $INTEGRATION_TESTS ($INTEGRATION_PERCENT%)"
echo "E2E Tests: $E2E_TESTS ($E2E_PERCENT%)"
echo "Total Tests: $TOTAL_TESTS"

echo ""
echo "Pyramid Target Distribution:"
echo "============================"
TARGET_UNIT=$((TOTAL_TESTS * 70 / 100))
TARGET_INTEGRATION=$((TOTAL_TESTS * 20 / 100))
TARGET_E2E=$((TOTAL_TESTS * 10 / 100))

echo "Unit Tests: $TARGET_UNIT (70%) - Need: $((TARGET_UNIT - UNIT_TESTS))"
echo "Integration Tests: $TARGET_INTEGRATION (20%) - Need: $((TARGET_INTEGRATION - INTEGRATION_TESTS))"
echo "E2E Tests: $TARGET_E2E (10%) - Need: $((TARGET_E2E - E2E_TESTS))"

echo ""
echo "Migration Status:"
echo "================="
if [ $UNIT_PERCENT -ge 70 ]; then
    echo "✅ Unit tests: Target achieved ($UNIT_PERCENT% >= 70%)"
else
    echo "❌ Unit tests: Need more ($UNIT_PERCENT% < 70%)"
fi

if [ $INTEGRATION_PERCENT -le 20 ]; then
    echo "✅ Integration tests: Target achieved ($INTEGRATION_PERCENT% <= 20%)"
else
    echo "❌ Integration tests: Need fewer ($INTEGRATION_PERCENT% > 20%)"
fi

if [ $E2E_PERCENT -le 10 ]; then
    echo "✅ E2E tests: Target achieved ($E2E_PERCENT% <= 10%)"
else
    echo "❌ E2E tests: Need fewer ($E2E_PERCENT% > 10%)"
fi
