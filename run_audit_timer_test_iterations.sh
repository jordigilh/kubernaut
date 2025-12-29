#!/bin/bash

# Audit Timer Intermittency Test Script
# Runs RO integration tests 10 times to check for timer bugs

echo "=========================================="
echo "Audit Timer Intermittency Test"
echo "Starting 10 test iterations..."
echo "=========================================="
echo ""

# Create results directory
mkdir -p ro_audit_timer_test_results
cd ro_audit_timer_test_results

# Track statistics
TOTAL_RUNS=10
PASS_COUNT=0
FAIL_COUNT=0
BUG_DETECTED_COUNT=0

# Run tests 10 times
for i in $(seq 1 $TOTAL_RUNS); do
    echo "=========================================="
    echo "TEST RUN $i of $TOTAL_RUNS"
    echo "=========================================="
    
    START_TIME=$(date +%s)
    
    # Run test and capture output
    timeout 300 make -C .. test-integration-remediationorchestrator > "run_${i}.log" 2>&1
    EXIT_CODE=$?
    
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    # Check for timer bug
    if grep -q "TIMER BUG DETECTED" "run_${i}.log"; then
        echo "🚨 TIMER BUG DETECTED in run $i!"
        BUG_DETECTED_COUNT=$((BUG_DETECTED_COUNT + 1))
        
        # Extract timer tick logs around the bug
        echo "Extracting timer logs for analysis..."
        grep -A 5 -B 5 "TIMER BUG DETECTED" "run_${i}.log" > "run_${i}_bug_context.log"
    fi
    
    # Check test result
    if grep -q "SUCCESS!" "run_${i}.log"; then
        echo "✅ Run $i: PASSED (${DURATION}s)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ Run $i: FAILED (${DURATION}s)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    
    # Extract test summary
    grep -A 2 "Ran.*Specs" "run_${i}.log" | tail -3 > "run_${i}_summary.txt"
    
    # Extract timer tick samples (first 10 ticks)
    grep "Timer tick received" "run_${i}.log" | head -10 > "run_${i}_timer_ticks.txt"
    
    echo ""
done

# Generate summary report
echo "=========================================="
echo "INTERMITTENCY TEST SUMMARY"
echo "=========================================="
echo ""
echo "Total Runs:      $TOTAL_RUNS"
echo "Passed:          $PASS_COUNT"
echo "Failed:          $FAIL_COUNT"
echo "Timer Bugs:      $BUG_DETECTED_COUNT"
echo ""

if [ $BUG_DETECTED_COUNT -eq 0 ]; then
    echo "✅ RESULT: No timer bugs detected in $TOTAL_RUNS runs"
    echo "   Recommendation: Enable AE-INT-3 and AE-INT-5 tests"
else
    echo "🚨 RESULT: Timer bug detected in $BUG_DETECTED_COUNT of $TOTAL_RUNS runs"
    echo "   Intermittency Rate: $(echo "scale=1; $BUG_DETECTED_COUNT * 100 / $TOTAL_RUNS" | bc)%"
    echo "   Recommendation: Share logs with DS Team for further investigation"
fi

echo ""
echo "Log files saved in: ro_audit_timer_test_results/"
echo "=========================================="

# Save summary to file
{
    echo "Audit Timer Intermittency Test Results"
    echo "Date: $(date)"
    echo ""
    echo "Total Runs:      $TOTAL_RUNS"
    echo "Passed:          $PASS_COUNT"
    echo "Failed:          $FAIL_COUNT"
    echo "Timer Bugs:      $BUG_DETECTED_COUNT"
    echo ""
    if [ $BUG_DETECTED_COUNT -eq 0 ]; then
        echo "✅ No timer bugs detected"
    else
        echo "🚨 Timer bug intermittency rate: $(echo "scale=1; $BUG_DETECTED_COUNT * 100 / $TOTAL_RUNS" | bc)%"
    fi
} > summary.txt

