#!/bin/bash
# integration-check-main-usage.sh - Run after ANY business code creation
COMPONENT_NAME="$1"  # e.g., "ContextOptimizer"

if [ -z "$COMPONENT_NAME" ]; then
    echo "❌ ERROR: Component name required"
    echo "Usage: $0 <ComponentName>"
    exit 1
fi

echo "🔍 Checking main application integration for $COMPONENT_NAME..."
MAIN_USAGE=$(grep -r "$COMPONENT_NAME" cmd/ --include="*.go" | wc -l)

if [ "$MAIN_USAGE" -eq 0 ]; then
    echo "❌ INTEGRATION FAILURE: $COMPONENT_NAME not found in main applications"
    echo "📁 Checked directories: cmd/"
    echo "🔧 Required: Add instantiation in cmd/kubernaut/main.go or cmd/kubernaut/main.go"
    echo ""
    echo "Example integration:"
    echo "// In main.go:"
    echo "$COMPONENT_NAME := New$COMPONENT_NAME(dependencies...)"
    echo "processor.Set$COMPONENT_NAME($COMPONENT_NAME)"
    exit 1
fi

echo "✅ Integration verified: $COMPONENT_NAME found in $MAIN_USAGE main application files"
