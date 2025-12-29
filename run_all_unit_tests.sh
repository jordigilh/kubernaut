#!/bin/bash

# Run all unit tests across all services
echo "=========================================="
echo "Running All Unit Tests Across All Services"
echo "=========================================="
echo ""

# Track results
TOTAL_SERVICES=0
PASSED_SERVICES=0
FAILED_SERVICES=0

# Service list
SERVICES=(
    "notification"
    "datastorage"
    "signalprocessing"
    "workflowexecution"
    "aianalysis"
    "remediationorchestrator"
    "shared"
)

# Run each service's unit tests
for service in "${SERVICES[@]}"; do
    TOTAL_SERVICES=$((TOTAL_SERVICES + 1))
    echo "=========================================="
    echo "Service: $service"
    echo "=========================================="
    
    if make test-unit-$service 2>&1 | tee "unit_test_${service}.log"; then
        echo "✅ $service unit tests PASSED"
        PASSED_SERVICES=$((PASSED_SERVICES + 1))
    else
        echo "❌ $service unit tests FAILED"
        FAILED_SERVICES=$((FAILED_SERVICES + 1))
    fi
    echo ""
done

# Summary
echo "=========================================="
echo "UNIT TEST SUMMARY"
echo "=========================================="
echo "Total Services:  $TOTAL_SERVICES"
echo "Passed:          $PASSED_SERVICES"
echo "Failed:          $FAILED_SERVICES"
echo ""

if [ $FAILED_SERVICES -eq 0 ]; then
    echo "✅ ALL UNIT TESTS PASSED!"
    exit 0
else
    echo "❌ SOME UNIT TESTS FAILED"
    exit 1
fi
