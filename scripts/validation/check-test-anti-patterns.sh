#!/bin/bash
# Test Anti-Pattern Detection Script
# Authority: docs/testing/ANTI_PATTERN_DETECTION.md

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
TEST_DIR="${TEST_DIR:-test/}"
VERBOSE="${1:-}"
REPORT_MODE="${2:-}"

# Counters
NULL_TESTS=0
STATIC_DATA=0
LIBRARY_TESTS=0
MISSING_BR=0
VIOLATIONS=0

echo "🔍 Checking for test anti-patterns in ${TEST_DIR}..."
echo ""

# Function to print violation
print_violation() {
    local pattern=$1
    local file=$2
    local line=$3
    local code=$4

    VIOLATIONS=$((VIOLATIONS + 1))

    if [ "$VERBOSE" == "--verbose" ]; then
        echo -e "${RED}❌ ${pattern}: ${file}:${line}${NC}"
        if [ -n "$code" ]; then
            echo "   ${code}"
        fi
    fi
}

# 1. NULL-TESTING DETECTION
echo "📋 Checking for NULL-TESTING violations..."
while IFS= read -r line; do
    if [ -n "$line" ]; then
        NULL_TESTS=$((NULL_TESTS + 1))
        file=$(echo "$line" | cut -d: -f1)
        lineno=$(echo "$line" | cut -d: -f2)
        code=$(echo "$line" | cut -d: -f3-)
        print_violation "NULL-TESTING" "$file" "$lineno" "$code"
    fi
done < <(grep -rn "ToNot(BeNil())\|ToNot(BeEmpty())" "${TEST_DIR}" --include="*_test.go" 2>/dev/null || true)

# 2. STATIC DATA TESTING DETECTION
echo "📋 Checking for STATIC DATA violations..."
while IFS= read -r line; do
    if [ -n "$line" ]; then
        # Only count if line doesn't have BR reference (indicating business context)
        if ! echo "$line" | grep -q "BR-"; then
            STATIC_DATA=$((STATIC_DATA + 1))
            file=$(echo "$line" | cut -d: -f1)
            lineno=$(echo "$line" | cut -d: -f2)
            code=$(echo "$line" | cut -d: -f3-)
            print_violation "STATIC DATA" "$file" "$lineno" "$code"
        fi
    fi
done < <(grep -rn "Equal(\"[^\"]*\")" "${TEST_DIR}" --include="*_test.go" 2>/dev/null || true)

# 3. LIBRARY TESTING DETECTION
echo "📋 Checking for LIBRARY TESTING violations..."
while IFS= read -r line; do
    if [ -n "$line" ]; then
        LIBRARY_TESTS=$((LIBRARY_TESTS + 1))
        file=$(echo "$line" | cut -d: -f1)
        lineno=$(echo "$line" | cut -d: -f2)
        code=$(echo "$line" | cut -d: -f3-)
        print_violation "LIBRARY TESTING" "$file" "$lineno" "$code"
    fi
done < <(grep -rn "logrus\.New()\|context\.WithValue.*Expect\|os\.Setenv.*Expect" "${TEST_DIR}" --include="*_test.go" 2>/dev/null || true)

# 4. MISSING BUSINESS REQUIREMENT REFERENCES
echo "📋 Checking for missing BR references..."
while IFS= read -r file; do
    if [ -n "$file" ]; then
        MISSING_BR=$((MISSING_BR + 1))
        if [ "$VERBOSE" == "--verbose" ]; then
            echo -e "${YELLOW}⚠️  MISSING BR: ${file}${NC}"
        fi
    fi
done < <(find "${TEST_DIR}" -name "*_test.go" -exec grep -L "BR-" {} \; 2>/dev/null || true)

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "📊 ANTI-PATTERN DETECTION SUMMARY"
echo "═══════════════════════════════════════════════════════════"
echo ""

# Print summary
if [ $NULL_TESTS -gt 0 ]; then
    echo -e "${RED}❌ NULL-TESTING violations: ${NULL_TESTS}${NC}"
else
    echo -e "${GREEN}✅ NULL-TESTING violations: 0${NC}"
fi

if [ $STATIC_DATA -gt 0 ]; then
    echo -e "${RED}❌ STATIC DATA violations: ${STATIC_DATA}${NC}"
else
    echo -e "${GREEN}✅ STATIC DATA violations: 0${NC}"
fi

if [ $LIBRARY_TESTS -gt 0 ]; then
    echo -e "${RED}❌ LIBRARY TESTING violations: ${LIBRARY_TESTS}${NC}"
else
    echo -e "${GREEN}✅ LIBRARY TESTING violations: 0${NC}"
fi

if [ $MISSING_BR -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Tests missing BR references: ${MISSING_BR}${NC}"
else
    echo -e "${GREEN}✅ All tests have BR references${NC}"
fi

echo ""
echo "═══════════════════════════════════════════════════════════"

TOTAL_ISSUES=$((NULL_TESTS + STATIC_DATA + LIBRARY_TESTS + MISSING_BR))

if [ $TOTAL_ISSUES -gt 0 ]; then
    echo -e "${RED}❌ TOTAL ISSUES FOUND: ${TOTAL_ISSUES}${NC}"
    echo ""
    echo "📖 For remediation guidance, see:"
    echo "   docs/testing/ANTI_PATTERN_DETECTION.md"
    echo ""
    echo "💡 Run with --verbose flag for detailed violation locations:"
    echo "   $0 --verbose"
    exit 1
else
    echo -e "${GREEN}✅ ALL CHECKS PASSED - No anti-patterns detected!${NC}"
    exit 0
fi
