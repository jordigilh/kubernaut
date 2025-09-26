#!/bin/bash
# validate-ai-development.sh - MANDATORY for AI components per Rule 12

PHASE="$1"  # red, green, refactor

echo "🤖 Validating AI development phase: $PHASE"

case $PHASE in
    "red")
        # Validate RED phase for AI
        AI_IMPORTS=$(grep -r "pkg/ai/.*Client" test/ --include="*_test.go" | wc -l)
        if [ "$AI_IMPORTS" -eq 0 ]; then
            echo "❌ AI RED VIOLATION: No existing AI interface imports"
            echo "🔧 Required: Import existing AI interfaces like pkg/ai/llm.Client"
            exit 1
        fi
        echo "✅ AI RED: Found $AI_IMPORTS AI interface imports in tests"
        ;;
    "green")
        # Validate GREEN phase for AI
        NEW_AI_FILES=$(git diff --name-only HEAD~1 2>/dev/null | grep "pkg/ai/.*\.go" | grep -v "_test.go" | wc -l)
        if [ "$NEW_AI_FILES" -gt 1 ]; then
            echo "❌ AI GREEN VIOLATION: Multiple new AI files created"
            echo "🔧 Required: Enhance existing AI client only"
            exit 1
        fi

        # Check AI integration
        AI_MAIN_USAGE=$(grep -r "AI\|LLM\|llm\|holmes" cmd/ --include="*.go" | wc -l)
        if [ "$AI_MAIN_USAGE" -eq 0 ]; then
            echo "❌ AI INTEGRATION VIOLATION: AI not integrated in main app"
            echo "🔧 Required: Integrate AI client in cmd/ applications"
            exit 1
        fi
        echo "✅ AI GREEN: Found $AI_MAIN_USAGE AI integrations in main applications"
        ;;
    "refactor")
        # Validate REFACTOR phase for AI
        NEW_AI_TYPES=$(git diff HEAD~1 2>/dev/null | grep "^+type.*AI\|^+type.*Optimizer" | wc -l)
        if [ "$NEW_AI_TYPES" -gt 0 ]; then
            echo "❌ AI REFACTOR VIOLATION: New AI types during REFACTOR"
            echo "🔧 Required: Enhance existing AI methods only"
            exit 1
        fi
        echo "✅ AI REFACTOR: No new AI types created during REFACTOR"
        ;;
    *)
        echo "ℹ️  Unknown phase: $PHASE"
        ;;
esac

echo "✅ AI development phase $PHASE validation passed"