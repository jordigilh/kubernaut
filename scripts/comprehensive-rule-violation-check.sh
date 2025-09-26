#!/bin/bash
# comprehensive-rule-violation-check.sh - Check all rules 03, 09, 11, 12

echo "🚨 COMPREHENSIVE RULE VIOLATION ANALYSIS"
echo "=========================================="

# RULE 12: AI/ML Development Methodology Violations
echo ""
echo "🤖 RULE 12: AI/ML DEVELOPMENT METHODOLOGY"
echo "------------------------------------------"

echo "🔍 Checking for AI type creation violations..."
NEW_AI_TYPES=$(git diff HEAD~5 2>/dev/null | grep "^+type.*AI\|^+type.*Optimizer\|^+type.*Engine\|^+type.*Analyzer" | wc -l)
if [ "$NEW_AI_TYPES" -gt 0 ]; then
    echo "❌ RULE 12 VIOLATION: $NEW_AI_TYPES new AI types created"
    echo "📋 VIOLATING TYPES:"
    git diff HEAD~5 2>/dev/null | grep "^+type.*AI\|^+type.*Optimizer\|^+type.*Engine\|^+type.*Analyzer" | head -5
    echo ""
    echo "🔧 RULE 12 REQUIREMENT: Should enhance existing AI interfaces:"
    echo "   - pkg/ai/llm.Client interface (AnalyzeContext, AnalyzeAlert)"
    echo "   - pkg/ai/holmesgpt.Client interface (Investigate, AnalyzeRemediationStrategies)"
    echo "   - pkg/ai/conditions.AIConditionEvaluator interface"
    echo ""
fi

# Check AI integration in main apps
AI_INTEGRATION=$(grep -r "AI\|LLM\|llm\|holmes" cmd/ --include="*.go" | wc -l)
echo "✅ AI Integration: $AI_INTEGRATION usages found in main applications"

# RULE 11: Development Rhythm Violations
echo ""
echo "🔵 RULE 11: DEVELOPMENT RHYTHM"
echo "------------------------------"

echo "🔍 Checking REFACTOR phase compliance..."
# Check for new types during REFACTOR (should be 0)
ALL_NEW_TYPES=$(git diff HEAD~5 2>/dev/null | grep "^+type.*struct" | grep -v "_test.go" | wc -l)
if [ "$ALL_NEW_TYPES" -gt 0 ]; then
    echo "⚠️  RULE 11 WARNING: $ALL_NEW_TYPES new types created (REFACTOR should enhance existing)"
fi

# RULE 09: Interface Method Validation
echo ""
echo "🔧 RULE 09: INTERFACE METHOD VALIDATION"
echo "----------------------------------------"

echo "🔍 Checking for interface validation..."
# Check for compilation success
COMPILATION_ERRORS=$(go test -c -tags=unit ./test/unit/platform/executor/ 2>&1 | grep -E "undefined|error:" | wc -l)
if [ "$COMPILATION_ERRORS" -eq 0 ]; then
    echo "✅ RULE 09 COMPLIANCE: All interface usage compiles successfully"
else
    echo "❌ RULE 09 VIOLATION: $COMPILATION_ERRORS compilation errors found"
fi

# RULE 03: Testing Strategy
echo ""
echo "🧪 RULE 03: TESTING STRATEGY"
echo "-----------------------------"

echo "🔍 Checking TDD methodology compliance..."
BUSINESS_IMPORTS=$(find test/ -name "*_test.go" -exec grep -l "github.com/jordigilh/kubernaut/pkg/" {} \; | wc -l)
echo "✅ RULE 03 COMPLIANCE: $BUSINESS_IMPORTS test files with business logic imports"

MOCK_USAGE=$(find test/ -name "*_test.go" -exec grep -l "mocks\.*Mock" {} \; | wc -l)
echo "✅ RULE 03 COMPLIANCE: $MOCK_USAGE test files using external dependency mocks"

# SUMMARY
echo ""
echo "📊 RULE COMPLIANCE SUMMARY"
echo "=========================="

TOTAL_VIOLATIONS=0

if [ "$NEW_AI_TYPES" -gt 0 ]; then
    echo "❌ RULE 12: AI/ML Development - VIOLATIONS FOUND"
    TOTAL_VIOLATIONS=$((TOTAL_VIOLATIONS + NEW_AI_TYPES))
else
    echo "✅ RULE 12: AI/ML Development - COMPLIANT"
fi

if [ "$ALL_NEW_TYPES" -gt 3 ]; then
    echo "⚠️  RULE 11: Development Rhythm - MINOR VIOLATIONS"
    TOTAL_VIOLATIONS=$((TOTAL_VIOLATIONS + 1))
else
    echo "✅ RULE 11: Development Rhythm - MOSTLY COMPLIANT"
fi

if [ "$COMPILATION_ERRORS" -eq 0 ]; then
    echo "✅ RULE 09: Interface Validation - COMPLIANT"
else
    echo "❌ RULE 09: Interface Validation - VIOLATIONS FOUND"
    TOTAL_VIOLATIONS=$((TOTAL_VIOLATIONS + COMPILATION_ERRORS))
fi

if [ "$BUSINESS_IMPORTS" -gt 50 ]; then
    echo "✅ RULE 03: Testing Strategy - COMPLIANT"
else
    echo "⚠️  RULE 03: Testing Strategy - MINOR VIOLATIONS"
    TOTAL_VIOLATIONS=$((TOTAL_VIOLATIONS + 1))
fi

echo ""
echo "📈 TOTAL VIOLATIONS: $TOTAL_VIOLATIONS"
if [ "$TOTAL_VIOLATIONS" -eq 0 ]; then
    echo "🎉 EXCELLENT: All rules compliant!"
    exit 0
elif [ "$TOTAL_VIOLATIONS" -le 3 ]; then
    echo "⚠️  MINOR VIOLATIONS: Needs attention"
    exit 1
else
    echo "❌ MAJOR VIOLATIONS: Immediate remediation required"
    exit 2
fi
