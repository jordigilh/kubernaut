#!/bin/bash
# TDD Compliance Check Script
# Authority: .cursor/rules/00-core-development-methodology.mdc

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
TEST_DIR="${TEST_DIR:-test/}"
PKG_DIR="${PKG_DIR:-pkg/}"
VERBOSE="${1:-}"

echo "🔍 Checking TDD compliance..."
echo ""

# Counters
NON_BDD_TESTS=0
TESTS_WITHOUT_BR=0
DIRECT_MOCKS=0
VIOLATIONS=0

# 1. Check for non-BDD test framework (standard Go testing instead of Ginkgo/Gomega)
echo "📋 Checking for BDD framework compliance (Ginkgo/Gomega)..."
while IFS= read -r line; do
    if [ -n "$line" ]; then
        file=$(echo "$line" | cut -d: -f1)

        # Skip if file also has Ginkgo (mixed file)
        if grep -q "Describe\|Context\|It" "$file" 2>/dev/null; then
            continue
        fi

        NON_BDD_TESTS=$((NON_BDD_TESTS + 1))

        if [ "$VERBOSE" == "--verbose" ]; then
            echo -e "${RED}❌ NON-BDD: ${file}${NC}"
            echo "   Using standard Go testing instead of Ginkgo/Gomega"
        fi
    fi
done < <(grep -rl "func Test.*testing\.T" "${TEST_DIR}" --include="*_test.go" 2>/dev/null || true)

# 2. Check for tests without business requirement references
echo "📋 Checking for business requirement references (BR-XXX-XXX)..."
while IFS= read -r file; do
    if [ -n "$file" ]; then
        TESTS_WITHOUT_BR=$((TESTS_WITHOUT_BR + 1))

        if [ "$VERBOSE" == "--verbose" ]; then
            echo -e "${YELLOW}⚠️  MISSING BR: ${file}${NC}"
            echo "   Test file lacks BR-XXX-XXX business requirement mapping"
        fi
    fi
done < <(find "${TEST_DIR}" -name "*_test.go" -exec grep -L "BR-[A-Z]*-[0-9]*" {} \; 2>/dev/null || true)

# 3. Check for direct mock instantiation (should use mock factory)
echo "📋 Checking for mock factory usage..."
while IFS= read -r line; do
    if [ -n "$line" ]; then
        file=$(echo "$line" | cut -d: -f1)
        lineno=$(echo "$line" | cut -d: -f2)
        code=$(echo "$line" | cut -d: -f3-)

        # Skip if line contains "Factory" (using factory pattern)
        if echo "$code" | grep -q "Factory"; then
            continue
        fi

        # Skip if it's a mock definition, not usage
        if echo "$code" | grep -q "type Mock"; then
            continue
        fi

        DIRECT_MOCKS=$((DIRECT_MOCKS + 1))

        if [ "$VERBOSE" == "--verbose" ]; then
            echo -e "${YELLOW}⚠️  DIRECT MOCK: ${file}:${lineno}${NC}"
            echo "   ${code}"
            echo "   Consider using mock factory pattern"
        fi
    fi
done < <(grep -rn "mock.*:=.*\.New" "${TEST_DIR}" --include="*_test.go" 2>/dev/null || true)

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "📊 TDD COMPLIANCE SUMMARY"
echo "═══════════════════════════════════════════════════════════"
echo ""

# Print summary
if [ $NON_BDD_TESTS -gt 0 ]; then
    echo -e "${RED}❌ Non-BDD test files: ${NON_BDD_TESTS}${NC}"
    echo "   MANDATORY: All tests must use Ginkgo/Gomega BDD framework"
else
    echo -e "${GREEN}✅ BDD framework compliance: All tests use Ginkgo/Gomega${NC}"
fi

if [ $TESTS_WITHOUT_BR -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Tests without BR references: ${TESTS_WITHOUT_BR}${NC}"
    echo "   RECOMMENDED: Map all tests to business requirements (BR-XXX-XXX)"
else
    echo -e "${GREEN}✅ Business requirement mapping: All tests reference BRs${NC}"
fi

if [ $DIRECT_MOCKS -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Direct mock instantiation: ${DIRECT_MOCKS}${NC}"
    echo "   RECOMMENDED: Use mock factory pattern for consistency"
else
    echo -e "${GREEN}✅ Mock factory usage: All mocks use factory pattern${NC}"
fi

echo ""
echo "═══════════════════════════════════════════════════════════"

TOTAL_ISSUES=$((NON_BDD_TESTS + TESTS_WITHOUT_BR + DIRECT_MOCKS))

if [ $TOTAL_ISSUES -gt 0 ]; then
    # Only fail on critical violations (non-BDD tests)
    if [ $NON_BDD_TESTS -gt 0 ]; then
        echo -e "${RED}❌ CRITICAL TDD VIOLATIONS: ${NON_BDD_TESTS}${NC}"
        echo ""
        echo "🚨 MANDATORY: Convert tests to Ginkgo/Gomega BDD framework"
        echo ""
        echo "📖 For migration guidance, see:"
        echo "   docs/testing/PYRAMID_TEST_MIGRATION_GUIDE.md"
        echo ""
        exit 1
    else
        echo -e "${YELLOW}⚠️  WARNINGS: ${TOTAL_ISSUES}${NC}"
        echo ""
        echo "Non-critical issues found. Consider addressing for better compliance."
        echo ""
        echo "💡 Run with --verbose flag for detailed locations:"
        echo "   $0 --verbose"
        exit 0
    fi
else
    echo -e "${GREEN}✅ ALL TDD COMPLIANCE CHECKS PASSED!${NC}"
    exit 0
fi
