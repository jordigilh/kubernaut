#!/bin/bash
# validate-tdd-completeness.sh - Validate TDD covers ALL targeted business requirements

TARGETED_BRS="$1"  # Comma-separated list: "BR-AI-001,BR-AI-002,BR-AI-003"

if [ -z "$TARGETED_BRS" ]; then
    echo "❌ ERROR: Targeted business requirements required"
    echo "Usage: $0 'BR-AI-001,BR-AI-002,BR-AI-003'"
    echo ""
    echo "RULE: Specify ONLY the business requirements you're implementing in THIS TDD cycle"
    echo "NOT all business requirements in the project"
    exit 1
fi

echo "🎯 Validating TDD completeness for targeted business requirements"
echo "Targeted BRs for this implementation: $TARGETED_BRS"
echo ""

# Convert comma-separated to array
IFS=',' read -ra BR_ARRAY <<< "$TARGETED_BRS"

# Check each targeted BR has corresponding tests
MISSING_BRS=()
COVERED_BRS=()

for br in "${BR_ARRAY[@]}"; do
    br=$(echo "$br" | xargs)  # Trim whitespace
    echo "🔍 Checking coverage for: $br"

    # Look for BR in test files
    TEST_COVERAGE=$(grep -r "$br" test/ --include="*_test.go" | wc -l)

    if [ "$TEST_COVERAGE" -eq 0 ]; then
        echo "  ❌ NO TEST COVERAGE: $br"
        MISSING_BRS+=("$br")
    else
        echo "  ✅ COVERED: $br ($TEST_COVERAGE test references)"
        COVERED_BRS+=("$br")

        # Show which test files cover this BR
        echo "     Test files:"
        grep -r "$br" test/ --include="*_test.go" | cut -d: -f1 | sort -u | sed 's/^/       /'
    fi
    echo ""
done

echo "========================================="
echo "📊 TDD COMPLETENESS SUMMARY"
echo "========================================="
echo "Targeted BRs: ${#BR_ARRAY[@]}"
echo "Covered BRs:  ${#COVERED_BRS[@]}"
echo "Missing BRs:  ${#MISSING_BRS[@]}"
echo ""

if [ ${#MISSING_BRS[@]} -eq 0 ]; then
    echo "🎉 TDD COMPLETENESS: PASSED"
    echo "✅ ALL targeted business requirements have corresponding tests"
    echo ""
    echo "Covered business requirements:"
    for br in "${COVERED_BRS[@]}"; do
        echo "  ✅ $br"
    done
    echo ""
    echo "RULE COMPLIANCE: TDD covers all business requirements targeted for this implementation"
    exit 0
else
    echo "❌ TDD COMPLETENESS: FAILED"
    echo "🚨 Missing test coverage for business requirements:"
    for br in "${MISSING_BRS[@]}"; do
        echo "  ❌ $br"
    done
    echo ""
    echo "🔧 REQUIRED ACTIONS:"
    echo "1. Add tests for each missing business requirement"
    echo "2. Ensure test names/comments reference the BR (e.g., 'per BR-AI-001')"
    echo "3. Validate business logic implementation for each BR"
    echo "4. Re-run this validation script"
    echo ""
    echo "RULE VIOLATION: TDD must cover ALL business requirements targeted for implementation"
    echo "SCOPE: Only the BRs you're implementing THIS cycle, not all BRs in the project"
    exit 1
fi
