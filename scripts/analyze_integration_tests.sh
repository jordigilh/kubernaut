#!/bin/bash
# analyze_integration_tests.sh - Systematic Integration Test Analysis for Conversion

echo "🔍 Systematic Integration Test Analysis for Pyramid Conversion"
echo "=============================================================="
echo "Date: $(date)"
echo ""

# Categories for conversion analysis
BUSINESS_LOGIC_TESTS=()
TRUE_INTEGRATION_TESTS=()
HYBRID_TESTS=()

echo "📊 Integration Test Categorization:"
echo "=================================="

# Analyze each integration test file
find test/integration/ -name "*_test.go" -not -name "*_suite_test.go" | while read file; do
    echo "Analyzing: $(basename $file)"

    # Check for business logic indicators
    BUSINESS_LOGIC_SCORE=0
    TRUE_INTEGRATION_SCORE=0

    # Business logic indicators (suggest conversion to unit tests)
    if grep -q "Business Requirement\|BR-.*-.*\|business logic\|algorithm\|calculation" "$file"; then
        BUSINESS_LOGIC_SCORE=$((BUSINESS_LOGIC_SCORE + 2))
    fi

    if grep -q "mock.*Engine\|mock.*Service\|mock.*Builder" "$file"; then
        BUSINESS_LOGIC_SCORE=$((BUSINESS_LOGIC_SCORE + 1))
    fi

    if grep -q "NewMock\|SetError\|SetResponse" "$file"; then
        BUSINESS_LOGIC_SCORE=$((BUSINESS_LOGIC_SCORE + 1))
    fi

    # True integration indicators (keep as integration tests)
    if grep -q "real.*database\|actual.*k8s\|live.*service\|cross.*component" "$file"; then
        TRUE_INTEGRATION_SCORE=$((TRUE_INTEGRATION_SCORE + 2))
    fi

    if grep -q "TestLifecycleHooks\|SetupAIIntegrationTest\|WithRealVectorDB" "$file"; then
        TRUE_INTEGRATION_SCORE=$((TRUE_INTEGRATION_SCORE + 2))
    fi

    if grep -q "network\|http\|api.*call\|database.*connection" "$file"; then
        TRUE_INTEGRATION_SCORE=$((TRUE_INTEGRATION_SCORE + 1))
    fi

    # Categorize based on scores
    if [ $BUSINESS_LOGIC_SCORE -gt $TRUE_INTEGRATION_SCORE ] && [ $BUSINESS_LOGIC_SCORE -ge 2 ]; then
        echo "  ✅ CONVERT TO UNIT TEST (Business Logic Score: $BUSINESS_LOGIC_SCORE)"
        echo "$file" >> /tmp/business_logic_tests.txt
    elif [ $TRUE_INTEGRATION_SCORE -gt $BUSINESS_LOGIC_SCORE ] && [ $TRUE_INTEGRATION_SCORE -ge 2 ]; then
        echo "  🔗 KEEP AS INTEGRATION TEST (Integration Score: $TRUE_INTEGRATION_SCORE)"
        echo "$file" >> /tmp/true_integration_tests.txt
    else
        echo "  🔄 HYBRID - NEEDS ANALYSIS (BL: $BUSINESS_LOGIC_SCORE, INT: $TRUE_INTEGRATION_SCORE)"
        echo "$file" >> /tmp/hybrid_tests.txt
    fi
done

echo ""
echo "📈 Conversion Analysis Summary:"
echo "==============================="

if [ -f /tmp/business_logic_tests.txt ]; then
    BUSINESS_COUNT=$(wc -l < /tmp/business_logic_tests.txt)
    echo "✅ Business Logic Tests (Convert to Unit): $BUSINESS_COUNT"
    echo "   Top candidates for conversion:"
    head -10 /tmp/business_logic_tests.txt | while read file; do
        echo "   - $(basename $file)"
    done
else
    BUSINESS_COUNT=0
    echo "✅ Business Logic Tests (Convert to Unit): 0"
fi

echo ""
if [ -f /tmp/true_integration_tests.txt ]; then
    INTEGRATION_COUNT=$(wc -l < /tmp/true_integration_tests.txt)
    echo "🔗 True Integration Tests (Keep): $INTEGRATION_COUNT"
    echo "   Examples of true integration tests:"
    head -5 /tmp/true_integration_tests.txt | while read file; do
        echo "   - $(basename $file)"
    done
else
    INTEGRATION_COUNT=0
    echo "🔗 True Integration Tests (Keep): 0"
fi

echo ""
if [ -f /tmp/hybrid_tests.txt ]; then
    HYBRID_COUNT=$(wc -l < /tmp/hybrid_tests.txt)
    echo "🔄 Hybrid Tests (Need Manual Analysis): $HYBRID_COUNT"
    echo "   Tests requiring manual review:"
    head -5 /tmp/hybrid_tests.txt | while read file; do
        echo "   - $(basename $file)"
    done
else
    HYBRID_COUNT=0
    echo "🔄 Hybrid Tests (Need Manual Analysis): 0"
fi

echo ""
echo "🎯 Conversion Plan for Option B (Systematic):"
echo "=============================================="
echo "Target: Convert 20-30 integration tests to unit tests"
echo "Available business logic tests for conversion: $BUSINESS_COUNT"

if [ $BUSINESS_COUNT -ge 20 ]; then
    echo "✅ SUFFICIENT: Can convert $BUSINESS_COUNT business logic tests to unit tests"
    echo "📋 Recommended conversion order:"
    echo "   1. AI Enhancement and TDD verification tests"
    echo "   2. Workflow optimization business logic tests"
    echo "   3. Core business logic integration tests"
    echo "   4. Platform operations business logic tests"
else
    echo "⚠️  LIMITED: Only $BUSINESS_COUNT clear business logic tests found"
    echo "📋 Recommended approach:"
    echo "   1. Convert all $BUSINESS_COUNT business logic tests"
    echo "   2. Analyze $HYBRID_COUNT hybrid tests for additional conversions"
    echo "   3. Focus on comprehensive unit test coverage"
fi

echo ""
echo "🚀 Next Steps:"
echo "=============="
echo "1. Start with top 10 business logic tests for conversion"
echo "2. Create comprehensive unit tests following pyramid principles"
echo "3. Validate each conversion maintains business requirement coverage"
echo "4. Update test distribution metrics after each batch"

# Cleanup temp files
rm -f /tmp/business_logic_tests.txt /tmp/true_integration_tests.txt /tmp/hybrid_tests.txt

echo ""
echo "Analysis completed at $(date)"
