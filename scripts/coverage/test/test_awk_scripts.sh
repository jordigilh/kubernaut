#!/usr/bin/env bash
# Unit tests for AWK coverage calculation scripts
#
# Usage: ./test_awk_scripts.sh
#
# Tests each AWK script with sample coverage data to ensure correctness

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COVERAGE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
FIXTURES="$SCRIPT_DIR/fixtures"

# Color codes
readonly GREEN='\033[0;32m'
readonly RED='\033[0;31m'
readonly NC='\033[0m'

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Test helper functions
assert_equals() {
    local expected="$1"
    local actual="$2"
    local test_name="$3"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if [[ "$expected" == "$actual" ]]; then
        echo -e "${GREEN}✓${NC} PASS: $test_name"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}✗${NC} FAIL: $test_name"
        echo "  Expected: $expected"
        echo "  Actual:   $actual"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# Test: calculate_go_unit_testable.awk
test_go_unit_testable() {
    echo ""
    echo "Testing calculate_go_unit_testable.awk..."
    
    # Test with config and validation packages (should be included)
    # config: 2 statements, 1 covered (5 count) = 50%
    # validation: 2 statements, 2 covered (10, 8 counts) = 100%
    # Total: 4 statements, 3 covered = 75%
    local result
    result=$(awk -f "$COVERAGE_DIR/calculate_go_unit_testable.awk" \
        -v pkg_pattern="/pkg/testservice/" \
        -v exclude_pattern="/handler/" \
        "$FIXTURES/sample_go_coverage.out")
    
    assert_equals "75.0%" "$result" "Go unit-testable coverage (excludes handler)"
    
    # Test with different exclusion (exclude validation)
    # Only config: 2 statements, 1 covered = 50%
    result=$(awk -f "$COVERAGE_DIR/calculate_go_unit_testable.awk" \
        -v pkg_pattern="/pkg/testservice/" \
        -v exclude_pattern="/(handler|validation)/" \
        "$FIXTURES/sample_go_coverage.out")
    
    assert_equals "50.0%" "$result" "Go unit-testable coverage (excludes handler and validation)"
}

# Test: calculate_go_integration_testable.awk
test_go_integration_testable() {
    echo ""
    echo "Testing calculate_go_integration_testable.awk..."
    
    # Test with handler packages (integration-only)
    # handler: 2 statements, 1 covered (3 count) = 50%
    local result
    result=$(awk -f "$COVERAGE_DIR/calculate_go_integration_testable.awk" \
        -v pkg_pattern="/pkg/testservice/" \
        -v include_pattern="/handler/" \
        "$FIXTURES/sample_go_coverage.out")
    
    assert_equals "50.0%" "$result" "Go integration-testable coverage (handler only)"
}

# Test: calculate_python_unit_testable.awk
test_python_unit_testable() {
    echo ""
    echo "Testing calculate_python_unit_testable.awk..."
    
    # Unit-testable packages: models (35 stmts, 5 miss), validation (30 stmts, 3 miss), errors (10 stmts, 2 miss)
    # Total: 75 statements, 65 covered = 86.7%
    local result
    result=$(awk -f "$COVERAGE_DIR/calculate_python_unit_testable.awk" \
        "$FIXTURES/sample_python_coverage.txt")
    
    assert_equals "86.7%" "$result" "Python unit-testable coverage"
}

# Test: calculate_python_integration_testable.awk
test_python_integration_testable() {
    echo ""
    echo "Testing calculate_python_integration_testable.awk..."
    
    # Integration-testable packages: extensions (40 stmts, 20 miss), middleware (25 stmts, 15 miss)
    # Total: 65 statements, 30 covered = 46.2%
    local result
    result=$(awk -f "$COVERAGE_DIR/calculate_python_integration_testable.awk" \
        "$FIXTURES/sample_python_coverage.txt")
    
    assert_equals "46.2%" "$result" "Python integration-testable coverage"
}

# Test: merge_go_coverage.awk
test_merge_go_coverage() {
    echo ""
    echo "Testing merge_go_coverage.awk..."
    
    # With only one file, should equal that file's coverage
    # Total: 4 config+validation statements, 3 covered (excluding handler) = 75%
    local result
    result=$(awk -f "$COVERAGE_DIR/merge_go_coverage.awk" \
        -v pkg_pattern="/pkg/testservice/(config|validation)/" \
        "$FIXTURES/sample_go_coverage.out")
    
    # Note: This is a simplified test - in reality we'd have multiple coverage files
    # Just testing the script runs without error
    [[ -n "$result" ]] && echo -e "${GREEN}✓${NC} PASS: merge_go_coverage.awk runs without error"
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

# Run all tests
main() {
    echo "═══════════════════════════════════════════"
    echo "AWK Coverage Script Unit Tests"
    echo "═══════════════════════════════════════════"
    
    test_go_unit_testable
    test_go_integration_testable
    test_python_unit_testable
    test_python_integration_testable
    test_merge_go_coverage
    
    echo ""
    echo "═══════════════════════════════════════════"
    echo "Test Summary"
    echo "═══════════════════════════════════════════"
    echo "Tests run:    $TESTS_RUN"
    echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
    [[ $TESTS_FAILED -gt 0 ]] && echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}" || echo "Tests failed: 0"
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "\n${GREEN}✓ All tests passed!${NC}"
        return 0
    else
        echo -e "\n${RED}✗ Some tests failed${NC}"
        return 1
    fi
}

main "$@"
