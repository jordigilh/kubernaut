#!/bin/bash
# validate-refactor-compliance.sh - Validate REFACTOR phase compliance
set -e

echo "🔄 Validating REFACTOR phase compliance..."

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "⚠️  WARNING: Not in a git repository, skipping git-based validation"
    exit 0
fi

# Detect new types created during REFACTOR
echo "📋 Checking for new types during REFACTOR..."
NEW_TYPES=$(git diff --name-only HEAD~1 2>/dev/null | xargs grep -l "type.*struct" 2>/dev/null | grep -v "_test.go" || echo "")

if [ ! -z "$NEW_TYPES" ]; then
    echo "❌ REFACTOR VIOLATION: New types created during REFACTOR phase"
    echo "Files with new types: $NEW_TYPES"
    echo "🔧 RULE: REFACTOR enhances existing code, never creates new types"
    echo ""
    echo "REFACTOR should enhance existing methods like:"
    echo "// BEFORE: func (c *ClientImpl) Method() { simple logic }"
    echo "// AFTER:  func (c *ClientImpl) Method() { enhanced logic with caching, etc. }"
    exit 1
fi

# Detect new methods added during REFACTOR
echo "📋 Checking for new methods during REFACTOR..."
NEW_METHODS=$(git diff HEAD~1 2>/dev/null | grep "^+func " | grep -v "_test.go" || echo "")

if [ ! -z "$NEW_METHODS" ]; then
    echo "❌ REFACTOR VIOLATION: New methods created during REFACTOR phase"
    echo "New methods detected:"
    echo "$NEW_METHODS"
    echo "🔧 RULE: REFACTOR enhances existing methods, never creates new ones"
    echo ""
    echo "Instead of adding new methods, enhance existing ones with:"
    echo "- Better algorithms"
    echo "- Caching mechanisms"
    echo "- Error handling improvements"
    echo "- Performance optimizations"
    exit 1
fi

# Detect new files created during REFACTOR
echo "📋 Checking for new files during REFACTOR..."
NEW_FILES=$(git diff --name-only --diff-filter=A HEAD~1 2>/dev/null | grep "\.go$" | grep -v "_test.go" || echo "")

if [ ! -z "$NEW_FILES" ]; then
    echo "❌ REFACTOR VIOLATION: New files created during REFACTOR phase"
    echo "New files: $NEW_FILES"
    echo "🔧 RULE: REFACTOR works within existing files, never creates new ones"
    exit 1
fi

echo "✅ REFACTOR phase compliance verified - no violations detected"
