#!/bin/bash
# run-integration-validation.sh - MANDATORY after any business code changes
set -e

echo "🚨 Running MANDATORY integration validation..."

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "⚠️  WARNING: Not in a git repository, skipping git-based validation"
    exit 0
fi

# Step 1: Check all new components
echo "📋 Step 1: Validating new components..."
NEW_COMPONENTS=$(git diff --name-only HEAD~1 2>/dev/null | xargs grep -l "^type.*struct" 2>/dev/null | grep -v "_test.go" | xargs grep -o "type [A-Za-z]*" 2>/dev/null | cut -d' ' -f2 || echo "")

if [ ! -z "$NEW_COMPONENTS" ]; then
    for component in $NEW_COMPONENTS; do
        echo "Validating component: $component"
        if ! ./scripts/integration-check-main-usage.sh "$component"; then
            echo "❌ Component integration validation failed for: $component"
            exit 1
        fi
        if ! ./scripts/integration-check-constructors.sh "New$component"; then
            echo "❌ Constructor integration validation failed for: New$component"
            exit 1
        fi
    done
else
    echo "ℹ️  No new components detected"
fi

# Step 2: Check all new interface methods
echo "📋 Step 2: Validating new interface methods..."
NEW_METHODS=$(git diff HEAD~1 2>/dev/null | grep "^+.*func.*(" | grep -o "func [A-Za-z]*" | cut -d' ' -f2 || echo "")

if [ ! -z "$NEW_METHODS" ]; then
    for method in $NEW_METHODS; do
        echo "Validating method: $method"
        if ! ./scripts/integration-check-runtime-path.sh "$method"; then
            echo "❌ Method runtime path validation failed for: $method"
            exit 1
        fi
    done
else
    echo "ℹ️  No new methods detected"
fi

# Step 3: Check for orphaned sophisticated business code
echo "📋 Step 3: Checking for orphaned sophisticated business code..."
SOPHISTICATED_TYPES=$(grep -r "type.*Optimizer\|type.*Engine\|type.*Analyzer" pkg/ --include="*.go" 2>/dev/null | grep -v "_test.go" || echo "")

if [ ! -z "$SOPHISTICATED_TYPES" ]; then
    while IFS= read -r type_def; do
        TYPE_NAME=$(echo "$type_def" | grep -o "type [A-Za-z]*" | cut -d' ' -f2)
        if [ ! -z "$TYPE_NAME" ]; then
            MAIN_USAGE=$(grep -r "$TYPE_NAME" cmd/ --include="*.go" 2>/dev/null | wc -l)
            if [ "$MAIN_USAGE" -eq 0 ]; then
                echo "❌ ORPHANED CODE: Sophisticated type $TYPE_NAME not integrated in main applications"
                echo "🔧 Required: Integrate $TYPE_NAME in cmd/ directories"
                exit 1
            fi
        fi
    done <<< "$SOPHISTICATED_TYPES"
fi

echo "✅ All integration validation passed"
