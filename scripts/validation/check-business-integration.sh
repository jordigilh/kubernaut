#!/bin/bash
# Business Code Integration Check Script
# Authority: .cursor/rules/00-core-development-methodology.mdc

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PKG_DIR="${PKG_DIR:-pkg/}"
CMD_DIR="${CMD_DIR:-cmd/}"
VERBOSE="${1:-}"

echo "🔍 Checking business code integration..."
echo ""

# Counters
ORPHANED_TYPES=0
ORPHANED_FILES=()

# Find sophisticated business types (indicators of business logic)
SOPHISTICATED_PATTERNS=(
    "type.*Engine.*struct"
    "type.*Optimizer.*struct"
    "type.*Analyzer.*struct"
    "type.*Builder.*struct"
    "type.*Manager.*struct"
    "type.*Controller.*struct"
    "type.*Processor.*struct"
    "type.*Generator.*struct"
)

echo "📋 Searching for sophisticated business types..."
echo ""

for pattern in "${SOPHISTICATED_PATTERNS[@]}"; do
    while IFS= read -r line; do
        if [ -n "$line" ]; then
            file=$(echo "$line" | cut -d: -f1)
            lineno=$(echo "$line" | cut -d: -f2)
            code=$(echo "$line" | cut -d: -f3-)

            # Skip test files
            if [[ "$file" == *"_test.go" ]]; then
                continue
            fi

            # Extract type name
            type_name=$(echo "$code" | grep -o "type [A-Za-z]*" | cut -d' ' -f2 || true)

            if [ -z "$type_name" ]; then
                continue
            fi

            # Check if type is used in main applications (cmd/)
            MAIN_USAGE=$(grep -r "${type_name}" "${CMD_DIR}" --include="*.go" | wc -l)

            if [ "$MAIN_USAGE" -eq 0 ]; then
                ORPHANED_TYPES=$((ORPHANED_TYPES + 1))
                ORPHANED_FILES+=("${file}:${lineno} - ${type_name}")

                if [ "$VERBOSE" == "--verbose" ]; then
                    echo -e "${RED}❌ ORPHANED: ${type_name} (${file}:${lineno})${NC}"
                    echo "   Not found in any main application (cmd/)"
                    echo ""
                fi
            else
                if [ "$VERBOSE" == "--verbose" ]; then
                    echo -e "${GREEN}✅ INTEGRATED: ${type_name} (found in ${MAIN_USAGE} cmd/ files)${NC}"
                fi
            fi
        fi
    done < <(grep -rn "${pattern}" "${PKG_DIR}" --include="*.go" 2>/dev/null || true)
done

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "📊 BUSINESS INTEGRATION SUMMARY"
echo "═══════════════════════════════════════════════════════════"
echo ""

if [ $ORPHANED_TYPES -gt 0 ]; then
    echo -e "${RED}❌ ORPHANED BUSINESS TYPES: ${ORPHANED_TYPES}${NC}"
    echo ""
    echo "The following business types are not integrated in main applications:"
    echo ""
    for item in "${ORPHANED_FILES[@]}"; do
        echo "   - ${item}"
    done
    echo ""
    echo "═══════════════════════════════════════════════════════════"
    echo ""
    echo "🚨 VIOLATION: All sophisticated business code MUST be integrated in main applications (cmd/)"
    echo ""
    echo "📖 Remediation steps:"
    echo "   1. Verify the type is actually business logic (not a helper/util)"
    echo "   2. Add instantiation in appropriate cmd/*/main.go file"
    echo "   3. Wire dependencies through dependency injection"
    echo "   4. Verify integration with: grep -r \"TypeName\" cmd/ --include=\"*.go\""
    echo ""
    echo "💡 If the type is NOT business logic:"
    echo "   - Move to internal/ directory (implementation details)"
    echo "   - Or rename to avoid \"Engine\"/\"Optimizer\" patterns"
    echo ""
    exit 1
else
    echo -e "${GREEN}✅ ALL BUSINESS TYPES INTEGRATED${NC}"
    echo ""
    echo "All sophisticated business types are properly integrated in main applications."
    echo ""
    exit 0
fi
