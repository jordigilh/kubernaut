#!/bin/bash
# validate_mock_usage.sh - Pyramid Mock Usage Validator

echo "Validating Mock Usage in Unit Tests:"
echo "===================================="

VIOLATIONS_FOUND=0

# Find unit tests that mock internal business logic (anti-pattern)
echo "Checking for over-mocking of internal business logic..."
find test/unit/ -name "*_test.go" -exec grep -l "mock.*Engine\|mock.*Service\|mock.*Builder\|mock.*Framework\|mock.*Analytics" {} \; | \
while read file; do
    # Check if file has justification for mocking (external, infrastructure, etc.)
    if ! grep -q "external\|infrastructure\|database\|k8s\|llm\|api\|slow\|error.*simulation" "$file"; then
        echo "⚠️  WARNING: Potential over-mocking in $file"
        echo "   - Check if business logic components can be used instead of mocks"
        VIOLATIONS_FOUND=$((VIOLATIONS_FOUND + 1))
    fi
done

# Check for proper external dependency mocking
echo ""
echo "Checking for proper external dependency mocking..."
EXTERNAL_MOCKS=0
find test/unit/ -name "*_test.go" -exec grep -l "mock.*Database\|mock.*K8s\|mock.*LLM\|mock.*Vector\|mock.*Metrics" {} \; | \
while read file; do
    echo "✅ Good: External dependency mocking in $(basename $file)"
    EXTERNAL_MOCKS=$((EXTERNAL_MOCKS + 1))
done

# Check for real business logic usage
echo ""
echo "Checking for real business logic usage in unit tests..."
REAL_BUSINESS_LOGIC=0
find test/unit/ -name "*_test.go" -exec grep -l "New.*Engine\|New.*Service\|New.*Builder\|New.*Framework" {} \; | \
while read file; do
    if grep -q "github.com/jordigilh/kubernaut/pkg/" "$file"; then
        echo "✅ Good: Real business logic usage in $(basename $file)"
        REAL_BUSINESS_LOGIC=$((REAL_BUSINESS_LOGIC + 1))
    fi
done

echo ""
echo "Summary:"
echo "========"
echo "External dependency mocks: $EXTERNAL_MOCKS files"
echo "Real business logic usage: $REAL_BUSINESS_LOGIC files"
if [ $VIOLATIONS_FOUND -gt 0 ]; then
    echo "❌ Mock usage violations: $VIOLATIONS_FOUND files need review"
else
    echo "✅ No mock usage violations found"
fi

echo ""
echo "Pyramid Mock Strategy Guidelines:"
echo "================================"
echo "✅ ALWAYS mock in unit tests:"
echo "   - Databases (PostgreSQL, Redis, Vector DB)"
echo "   - External APIs (LLM services, monitoring, K8s API)"
echo "   - Infrastructure (file system, network calls)"
echo "   - Slow operations (>10ms execution time)"
echo ""
echo "✅ ALWAYS use REAL in unit tests:"
echo "   - Internal business logic (pkg/ components)"
echo "   - Business algorithms and validation"
echo "   - Service orchestration logic"
echo "   - Business rule enforcement"
