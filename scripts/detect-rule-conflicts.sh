#!/bin/bash
# detect-rule-conflicts.sh - Comprehensive rule conflict detection

echo "🔍 Comprehensive Rule Conflict Detection"
echo "======================================="

EXIT_CODE=0

# CONFLICT DETECTOR 1: Integration vs Development Speed
echo "📋 Checking integration vs development speed conflicts..."
SOPHISTICATED_COMPONENTS=$(grep -r "type.*Optimizer\|type.*Engine\|type.*Analyzer" pkg/ --include="*.go" 2>/dev/null | grep -v "_test.go")
INTEGRATED_COMPONENTS=$(grep -r "Optimizer\|Engine\|Analyzer" cmd/ --include="*.go" 2>/dev/null)

if [ ! -z "$SOPHISTICATED_COMPONENTS" ] && [ -z "$INTEGRATED_COMPONENTS" ]; then
    echo "❌ CONFLICT: Sophisticated components without integration"
    echo "Components found:"
    echo "$SOPHISTICATED_COMPONENTS"
    echo "🔧 RESOLUTION: Integrate components in cmd/ before proceeding"
    EXIT_CODE=1
else
    echo "✅ No integration conflicts detected"
fi

# CONFLICT DETECTOR 2: Mock Usage Contradictions
echo ""
echo "📋 Checking mock usage contradictions..."
BUSINESS_MOCKS=$(grep -r "Mock.*Engine\|Mock.*Calculator\|Mock.*Analyzer" test/ --include="*_test.go" 2>/dev/null | wc -l)
BUSINESS_REAL=$(grep -r "engine\.New\|calculator\.New\|analyzer\.New" test/ --include="*_test.go" 2>/dev/null | wc -l)

if [ "$BUSINESS_MOCKS" -gt 0 ] && [ "$BUSINESS_REAL" -eq 0 ]; then
    echo "❌ CONFLICT: Over-mocking business logic vs real component preference"
    echo "Business mocks: $BUSINESS_MOCKS"
    echo "Business real: $BUSINESS_REAL"
    echo "🔧 RESOLUTION: Use real business components, mock external services only"
    EXIT_CODE=1
elif [ "$BUSINESS_MOCKS" -gt "$BUSINESS_REAL" ]; then
    echo "⚠️  WARNING: More business mocks than real components"
    echo "Business mocks: $BUSINESS_MOCKS"
    echo "Business real: $BUSINESS_REAL"
    echo "💡 RECOMMENDATION: Prefer real business logic where possible"
else
    echo "✅ No mock usage conflicts detected"
fi

# CONFLICT DETECTOR 3: TDD Phase vs Component Creation
echo ""
echo "📋 Checking TDD phase vs component creation conflicts..."
if git rev-parse --git-dir > /dev/null 2>&1; then
    NEW_TYPES=$(git diff HEAD~1 2>/dev/null | grep "^+type.*struct" | wc -l)
    NEW_INTERFACES=$(git diff HEAD~1 2>/dev/null | grep "^+type.*interface" | wc -l)

    if [ "$NEW_TYPES" -gt 2 ] || [ "$NEW_INTERFACES" -gt 1 ]; then
        echo "⚠️  WARNING: Multiple new types/interfaces created"
        echo "New types: $NEW_TYPES"
        echo "New interfaces: $NEW_INTERFACES"
        echo "💡 RECOMMENDATION: Consider if this is GREEN (basic) or REFACTOR (enhancement)"
    else
        echo "✅ No TDD phase conflicts detected"
    fi
else
    echo "ℹ️  Not a git repository - skipping TDD phase conflict detection"
fi

# CONFLICT DETECTOR 4: AI Component Development
echo ""
echo "📋 Checking AI component development conflicts..."
AI_FILES=$(find pkg/ai/ -name "*.go" -not -name "*_test.go" 2>/dev/null | wc -l)
AI_MAIN_USAGE=$(grep -r "AI\|LLM\|Holmes" cmd/ --include="*.go" 2>/dev/null | wc -l)

if [ "$AI_FILES" -gt 0 ] && [ "$AI_MAIN_USAGE" -eq 0 ]; then
    echo "❌ CONFLICT: AI components without main application integration"
    echo "AI component files: $AI_FILES"
    echo "Main app AI usage: $AI_MAIN_USAGE"
    echo "🔧 RESOLUTION: Follow Rule 12 AI/ML development methodology"
    EXIT_CODE=1
else
    echo "✅ No AI component conflicts detected"
fi

# CONFLICT DETECTOR 5: Configuration Conflicts
echo ""
echo "📋 Checking configuration conflicts..."
HARDCODED_IPS=$(grep -r "192\.168\|127\.0\.0\.1" . --include="*.go" --include="*.md" 2>/dev/null | grep -v ".git" | wc -l)
CONFIG_FILES=$(find config/ -name "*.yaml" -o -name "*.yml" 2>/dev/null | wc -l)

if [ "$HARDCODED_IPS" -gt 5 ] && [ "$CONFIG_FILES" -gt 0 ]; then
    echo "⚠️  WARNING: Hardcoded IPs found despite configuration files"
    echo "Hardcoded IPs: $HARDCODED_IPS occurrences"
    echo "Config files: $CONFIG_FILES"
    echo "💡 RECOMMENDATION: Use configuration variables instead of hardcoded values"
else
    echo "✅ No configuration conflicts detected"
fi

echo ""
echo "======================================="
if [ $EXIT_CODE -eq 0 ]; then
    echo "✅ All rule conflict checks passed"
else
    echo "❌ Rule conflicts detected - see details above"
    echo ""
    echo "🔧 RESOLUTION GUIDANCE:"
    echo "1. Integration conflicts: Apply Rule Priority Level 1"
    echo "2. TDD conflicts: Apply Rule Priority Level 2"
    echo "3. Component conflicts: Apply Rule Priority Level 3"
    echo "4. Use ./scripts/resolve-rule-conflict.sh for automated resolution"
fi

exit $EXIT_CODE
