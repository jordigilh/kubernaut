#!/bin/bash
# integration-check-constructors.sh - Verify constructors used in main app
CONSTRUCTOR_PATTERN="$1"  # e.g., "NewContextOptimizer"

if [ -z "$CONSTRUCTOR_PATTERN" ]; then
    echo "❌ ERROR: Constructor pattern required"
    echo "Usage: $0 <NewComponentName>"
    exit 1
fi

echo "🔍 Checking constructor integration for $CONSTRUCTOR_PATTERN..."
CONSTRUCTOR_USAGE=$(grep -r "$CONSTRUCTOR_PATTERN" cmd/ --include="*.go" | wc -l)

if [ "$CONSTRUCTOR_USAGE" -eq 0 ]; then
    echo "❌ CONSTRUCTOR INTEGRATION FAILURE: $CONSTRUCTOR_PATTERN not called in main applications"
    echo "🔧 Required: Add $CONSTRUCTOR_PATTERN() call in main application startup"
    echo ""
    echo "Example integration:"
    echo "// In main.go:"
    echo "component := $CONSTRUCTOR_PATTERN(config, logger, dependencies...)"
    echo "app.RegisterComponent(component)"
    exit 1
fi

echo "✅ Constructor integration verified: $CONSTRUCTOR_PATTERN called in main applications"
