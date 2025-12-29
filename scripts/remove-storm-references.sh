#!/bin/bash
# Gateway Storm Reference Cleanup Script
# Per DD-GATEWAY-015: Storm detection removed, clean up test references
#
# This script performs systematic cleanup of storm-related references
# across Gateway integration tests, focusing on:
# 1. Comments mentioning "storm aggregation may reduce count"
# 2. Variable names like "stormPayload" → "persistentAlertPayload"
# 3. Comments like "storm detection" → "high occurrence tracking"
# 4. Test labels referencing "storm pattern"

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_DIR="$PROJECT_ROOT/test/integration/gateway"

echo "🧹 Gateway Storm Reference Cleanup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Target: $TEST_DIR"
echo ""

# Backup test directory
BACKUP_DIR="/tmp/gateway-tests-backup-$(date +%Y%m%d-%H%M%S)"
echo "📦 Creating backup: $BACKUP_DIR"
cp -r "$TEST_DIR" "$BACKUP_DIR"
echo "   ✅ Backup created"
echo ""

# Counter for changes
CHANGES=0

# Pattern 1: "storm aggregation may reduce count" → "deduplication may reduce count"
echo "🔄 Pattern 1: Updating 'storm aggregation may reduce count' comments..."
if grep -l "storm aggregation may reduce count" "$TEST_DIR"/*.go 2>/dev/null; then
    for file in "$TEST_DIR"/*.go; do
        if grep -q "storm aggregation may reduce count" "$file" 2>/dev/null; then
            sed -i.bak 's/storm aggregation may reduce count/deduplication may reduce count/g' "$file"
            rm -f "${file}.bak"
            ((CHANGES++))
            echo "   ✅ Updated: $(basename "$file")"
        fi
    done
fi
echo ""

# Pattern 2: "storm detection" → "high occurrence tracking" in comments only
echo "🔄 Pattern 2: Updating 'storm detection' → 'high occurrence tracking' in comments..."
if grep -l "// .*storm detection" "$TEST_DIR"/*.go 2>/dev/null; then
    for file in "$TEST_DIR"/*.go; do
        if grep -q "// .*storm detection" "$file" 2>/dev/null; then
            # Only replace in comment lines (starting with //)
            sed -i.bak 's/\/\/ \(.*\)storm detection/\/\/ \1high occurrence tracking/g' "$file"
            rm -f "${file}.bak"
            ((CHANGES++))
            echo "   ✅ Updated: $(basename "$file")"
        fi
    done
fi
echo ""

# Pattern 3: "storm pattern" → "persistent alert pattern"
echo "🔄 Pattern 3: Updating 'storm pattern' → 'persistent alert pattern'..."
if grep -l "storm pattern" "$TEST_DIR"/*.go 2>/dev/null; then
    for file in "$TEST_DIR"/*.go; do
        if grep -q "storm pattern" "$file" 2>/dev/null; then
            sed -i.bak 's/storm pattern/persistent alert pattern/g' "$file"
            rm -f "${file}.bak"
            ((CHANGES++))
            echo "   ✅ Updated: $(basename "$file")"
        fi
    done
fi
echo ""

# Pattern 4: "stormPayload" → "persistentAlertPayload"
echo "🔄 Pattern 4: Renaming variable 'stormPayload' → 'persistentAlertPayload'..."
if grep -l "stormPayload" "$TEST_DIR"/*.go 2>/dev/null; then
    for file in "$TEST_DIR"/*.go; do
        if grep -q "stormPayload" "$file" 2>/dev/null; then
            sed -i.bak 's/stormPayload/persistentAlertPayload/g' "$file"
            rm -f "${file}.bak"
            ((CHANGES++))
            echo "   ✅ Updated: $(basename "$file")"
        fi
    done
fi
echo ""

# Pattern 5: "storm threshold" → "occurrence threshold"
echo "🔄 Pattern 5: Updating 'storm threshold' → 'occurrence threshold'..."
if grep -l "storm threshold" "$TEST_DIR"/*.go 2>/dev/null; then
    for file in "$TEST_DIR"/*.go; do
        if grep -q "storm threshold" "$file" 2>/dev/null; then
            sed -i.bak 's/storm threshold/occurrence threshold/g' "$file"
            rm -f "${file}.bak"
            ((CHANGES++))
            echo "   ✅ Updated: $(basename "$file")"
        fi
    done
fi
echo ""

# Pattern 6: Remove "reduce Redis contention and storm aggregation" comments
echo "🔄 Pattern 6: Updating Redis/storm aggregation delay comments..."
if grep -l "reduce Redis contention and storm aggregation" "$TEST_DIR"/*.go 2>/dev/null; then
    for file in "$TEST_DIR"/*.go; do
        if grep -q "reduce Redis contention and storm aggregation" "$file" 2>/dev/null; then
            sed -i.bak 's/reduce Redis contention and storm aggregation/parallel test stability/g' "$file"
            rm -f "${file}.bak"
            ((CHANGES++))
            echo "   ✅ Updated: $(basename "$file")"
        fi
    done
fi
echo ""

# Pattern 7: "storm indicator" → "persistent issue indicator"
echo "🔄 Pattern 7: Updating 'storm indicator' → 'persistent issue indicator'..."
if grep -l "storm indicator" "$TEST_DIR"/*.go 2>/dev/null; then
    for file in "$TEST_DIR"/*.go; do
        if grep -q "storm indicator" "$file" 2>/dev/null; then
            sed -i.bak 's/storm indicator/persistent issue indicator/g' "$file"
            rm -f "${file}.bak"
            ((CHANGES++))
            echo "   ✅ Updated: $(basename "$file")"
        fi
    done
fi
echo ""

# Summary
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Cleanup Complete"
echo ""
echo "📊 Summary:"
echo "   Files modified: $CHANGES"
echo "   Backup location: $BACKUP_DIR"
echo ""
echo "🔍 Next Steps:"
echo "   1. Review changes: git diff test/integration/gateway/"
echo "   2. Run tests: make test-gateway"
echo "   3. If successful, commit changes"
echo "   4. If issues, restore from: $BACKUP_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

