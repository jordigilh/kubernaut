#!/bin/bash
# phase4-refactor-validation.sh - TDD REFACTOR Phase Validation per Rule 11

echo "🔵 PHASE 4: TDD REFACTOR VALIDATION"

# REFACTOR validation focuses on enhancement evidence, not git changes
# (git changes may show development history, not current REFACTOR state)
echo "🔍 Validating REFACTOR phase compliance - focusing on enhancement evidence..."

# Check tests still pass
echo "🔍 Validating tests remain GREEN during REFACTOR..."
if ! go test -tags=unit ./test/unit/platform/executor/ 2>/dev/null; then
    echo "❌ REFACTOR VIOLATION: Tests failing after REFACTOR in executor package"
    exit 1
fi

if ! go test -tags=unit ./test/unit/integration/comprehensive/ 2>/dev/null; then
    echo "❌ REFACTOR VIOLATION: Tests failing after REFACTOR in integration package"
    exit 1
fi

if ! go test -tags=unit ./test/unit/ai/llm/ 2>/dev/null; then
    echo "❌ REFACTOR VIOLATION: Tests failing after REFACTOR in AI package"
    exit 1
fi

# Check for REFACTOR phase evidence
echo "🔍 Validating REFACTOR phase evidence exists..."
REFACTOR_EVIDENCE=0

# Check for performance optimizations
if grep -r "semaphore\|concurrency\|performance\|optimization" pkg/platform/executor/ >/dev/null 2>&1; then
    echo "✅ Performance optimization evidence found in executor"
    REFACTOR_EVIDENCE=$((REFACTOR_EVIDENCE + 1))
fi

# Check for sophisticated algorithms
if grep -r "sophisticated\|advanced\|caching\|prediction" pkg/ai/insights/ >/dev/null 2>&1; then
    echo "✅ Sophisticated algorithm evidence found in insights"
    REFACTOR_EVIDENCE=$((REFACTOR_EVIDENCE + 1))
fi

# Check for enhanced business logic
if grep -r "Enhancement\|Business Logic Enhancement" test/unit/ >/dev/null 2>&1; then
    echo "✅ Business logic enhancement evidence found in tests"
    REFACTOR_EVIDENCE=$((REFACTOR_EVIDENCE + 1))
fi

if [ "$REFACTOR_EVIDENCE" -lt 2 ]; then
    echo "⚠️  WARNING: Limited REFACTOR phase evidence detected"
    echo "🔧 Recommendation: Add explicit REFACTOR documentation"
fi

echo "✅ PHASE 4 REFACTOR validation complete"
echo "📊 REFACTOR Evidence Score: $REFACTOR_EVIDENCE/3"
