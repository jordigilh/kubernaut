#!/bin/bash
# ai-component-discovery.sh - AI Component Discovery per Rule 12
AI_COMPONENT="$1"

echo "🤖 AI COMPONENT DISCOVERY: $AI_COMPONENT"

# Check existing AI interfaces
EXISTING_AI=$(grep -r "$AI_COMPONENT\|interface.*AI\|LLM.*interface" pkg/ai/ --include="*.go" | grep -v "_test.go" | wc -l)
MAIN_AI_USAGE=$(grep -r "$AI_COMPONENT\|AI\|LLM\|llm\|holmes" cmd/ --include="*.go" | wc -l)

echo "Existing AI interfaces: $EXISTING_AI"
echo "Main application AI usage: $MAIN_AI_USAGE"

if [ "$EXISTING_AI" -gt 0 ] && [ "$MAIN_AI_USAGE" -eq 0 ]; then
    echo "⚠️  WARNING: Existing AI interface but no main app usage"
    echo "❓ QUESTION: Should you enhance existing AI client instead?"
fi

# Show existing AI interfaces
echo ""
echo "📋 EXISTING AI INTERFACES:"
find pkg/ai/ -name "*.go" -exec grep -l "interface" {} \; | head -5

echo ""
echo "📋 MAIN APP AI USAGE:"
grep -r "AI\|LLM\|llm\|holmes" cmd/ --include="*.go" | head -5

echo ""
echo "✅ AI component discovery complete"