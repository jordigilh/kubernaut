#!/bin/bash

# Fix integration test warnings by adding test suite runners to files missing them
# This script identifies files with Ginkgo specs but missing test suite runners

cd "$(dirname "$0")"

# List of files that have Ginkgo specs but are missing test suite runners
FILES_MISSING_RUNNERS=(
    "test/integration/advanced_analytics_tdd_verification_test.go"
    "test/integration/advanced_orchestration_tdd_verification_test.go"
    "test/integration/advanced_scheduling_tdd_verification_test.go"
    "test/integration/ai_enhancement_tdd_verification_test.go"
    "test/integration/analytics_tdd_verification_test.go"
    "test/integration/business_integration_automation_test.go"
    "test/integration/comprehensive_test_suite.go"
    "test/integration/dynamic_toolset_integration_test.go"
    "test/integration/environment_adaptation_tdd_verification_test.go"
    "test/integration/execution_monitoring_tdd_verification_test.go"
    "test/integration/objective_analysis_tdd_verification_test.go"
    "test/integration/pattern_discovery_enhanced_filtering_test.go"
    "test/integration/pattern_discovery_tdd_verification_test.go"
    "test/integration/pattern_management_tdd_verification_test.go"
    "test/integration/performance_monitoring_tdd_verification_test.go"
    "test/integration/race_condition_stress_test.go"
    "test/integration/resource_optimization_tdd_verification_test.go"
    "test/integration/security_enhancement_tdd_verification_test.go"
    "test/integration/template_generation_tdd_verification_test.go"
    "test/integration/validation_enhancement_tdd_verification_test.go"
)

echo "🔧 Fixing integration test warnings by adding test suite runners..."

for file in "${FILES_MISSING_RUNNERS[@]}"; do
    if [ -f "$file" ]; then
        echo "Processing: $file"

        # Extract test suite name from the last Describe block
        LAST_DESCRIBE=$(grep -n "var _ = Describe" "$file" | tail -1)
        if [ -n "$LAST_DESCRIBE" ]; then
            # Extract the suite name from the Describe block
            SUITE_NAME=$(echo "$LAST_DESCRIBE" | sed -n 's/.*Describe("\([^"]*\)".*/\1/p')

            # Create a function name from the suite name (remove spaces, make camelCase)
            FUNC_NAME=$(echo "$SUITE_NAME" | sed 's/ //g')

            # Add the test suite runner function at the end of the file
            cat >> "$file" << EOF

// Test suite runner for $SUITE_NAME
func Test${FUNC_NAME}(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "$SUITE_NAME Suite")
}
EOF
            echo "  ✅ Added Test${FUNC_NAME}() function"
        else
            echo "  ⚠️  No Describe blocks found in $file"
        fi
    else
        echo "  ❌ File not found: $file"
    fi
done

echo ""
echo "🧪 Testing the fix..."

# Test that the integration tests now run without warnings
echo "Running integration tests to verify fix..."
go test -tags=integration -list=. ./test/integration/... | grep -E "Test|?" | head -10

echo ""
echo "✅ Integration test warning fix completed!"
echo "📝 Files processed: ${#FILES_MISSING_RUNNERS[@]}"
echo ""
echo "To verify the fix worked:"
echo "  make test-integration-kind"
echo "  or"
echo "  go test -tags=integration ./test/integration/..."
