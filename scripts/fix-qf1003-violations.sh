#!/bin/bash

# Fix QF1003 gocritic violations (ifElseChain - simplify if-else chains with switch)
# This script detects and suggests fixes for if-else chains that can be simplified

set -e

echo "🔍 Detecting and fixing QF1003 violations (ifElseChain - if-else to switch)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

FIXES_APPLIED=0
FILES_CHECKED=0

# Function to check if a file contains QF1003 patterns
check_qf1003_patterns() {
    local file="$1"

    echo "   Checking: $file"

    # Check for status comparison chains (common pattern)
    if grep -q "if.*Status.*==" "$file" && grep -A 5 "if.*Status.*==" "$file" | grep -q "} else if.*Status.*=="; then
        echo -e "   ${YELLOW}⚠️  Status comparison chain detected in $file${NC}"
        echo "      Pattern: if status == A { ... } else if status == B { ... }"
        echo "      Recommendation: Use switch statement for status comparisons"
        return 0
    fi

    # Check for type assertion chains
    if grep -q "if.*ok.*:=.*\.(.*); ok" "$file" && grep -A 3 "if.*ok.*:=.*\.(.*); ok" "$file" | grep -q "} else if.*ok.*:=.*\.(.*); ok"; then
        echo -e "   ${YELLOW}⚠️  Type assertion chain detected in $file${NC}"
        echo "      Pattern: if val, ok := x.(Type1); ok { ... } else if val, ok := x.(Type2); ok { ... }"
        echo "      Recommendation: Use switch val := x.(type) pattern"
        return 0
    fi

    # Check for string comparison chains (QF1003 violations)
    if grep -q "if.*== \".*\"" "$file" && grep -A 3 "if.*== \".*\"" "$file" | grep -q "} else if.*== \".*\""; then
        echo -e "   ${YELLOW}⚠️  String comparison chain detected in $file${NC}"
        echo "      Pattern: if str == \"value1\" { ... } else if str == \"value2\" { ... }"
        echo "      Fix: Use switch statement for string comparisons"
        return 0
    fi

    # Check for numeric threshold chains (but these might be acceptable)
    if grep -q "if.*>=.*{" "$file" && grep -A 3 "if.*>=.*{" "$file" | grep -q "} else if.*>=.*{"; then
        echo "   ${YELLOW}ℹ️  Numeric threshold chain detected in $file${NC}"
        echo "      Note: Numeric thresholds may be better as if-else for readability"
        echo "      Manual review recommended"
    fi

    return 1
}

# Function to suggest fixes for common patterns
suggest_fixes() {
    echo
    echo "🛠️  Common QF1003 Fix Patterns:"
    echo "================================"
    echo
    echo "1. Status Comparisons:"
    echo "   ❌ if status == A { ... } else if status == B { ... }"
    echo "   ✅ switch status { case A: ... case B: ... }"
    echo
    echo "2. Type Assertions:"
    echo "   ❌ if val, ok := x.(Type1); ok { ... } else if val, ok := x.(Type2); ok { ... }"
    echo "   ✅ switch val := x.(type) { case Type1: ... case Type2: ... }"
    echo
    echo "3. String Comparisons:"
    echo "   ❌ if str == \"A\" { ... } else if str == \"B\" { ... }"
    echo "   ✅ switch str { case \"A\": ... case \"B\": ... }"
    echo
    echo "4. Impact Level Comparisons (Common Pattern):"
    echo "   ❌ if baseImpact == \"low\" { baseImpact = \"medium\" } else if baseImpact == \"medium\" { baseImpact = \"high\" }"
    echo "   ✅ switch baseImpact { case \"low\": baseImpact = \"medium\" case \"medium\": baseImpact = \"high\" }"
    echo
}

echo "1. Scanning for Go files..."

# Find all Go files and check for QF1003 patterns
find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | while read -r file; do
    FILES_CHECKED=$((FILES_CHECKED + 1))

    if check_qf1003_patterns "$file"; then
        FIXES_APPLIED=$((FIXES_APPLIED + 1))
    fi
done

echo
echo "2. Running verification..."

# Run gocritic to check for QF1003 violations if available
if command -v golangci-lint >/dev/null 2>&1; then
    echo "   Running golangci-lint to check QF1003 violations..."

    # Try to run gocritic check specifically
    QF1003_OUTPUT=$(golangci-lint run --enable=gocritic --disable-all 2>/dev/null | grep "ifElseChain" || echo "")

    if [ -n "$QF1003_OUTPUT" ]; then
        echo -e "   ${YELLOW}⚠️  QF1003 violations detected:${NC}"
        echo "$QF1003_OUTPUT" | head -10 | sed 's/^/     /'
        if [ $(echo "$QF1003_OUTPUT" | wc -l) -gt 10 ]; then
            echo "     ... (showing first 10 violations)"
        fi
    else
        echo -e "   ${GREEN}✅ No QF1003 violations detected by golangci-lint${NC}"
    fi
else
    echo "   golangci-lint not available - skipping automated check"
fi

suggest_fixes

echo
echo "📊 Summary"
echo "=========="
echo "Files checked: $FILES_CHECKED"
echo "Potential QF1003 patterns found: $FIXES_APPLIED"

echo
echo -e "${GREEN}🔧 QF1003 Pattern Analysis Complete${NC}"
echo
echo "Next steps:"
echo "1. Review flagged files manually"
echo "2. Convert appropriate if-else chains to switch statements"
echo "3. Run: golangci-lint run --enable=gocritic"
echo "4. Test changes: make test"
echo "5. Commit improvements if tests pass"

echo
echo "📖 Guidelines:"
echo "- Status/enum comparisons: Use switch"
echo "- Type assertions: Use switch with type assertion"
echo "- Numeric thresholds: Consider if-else for clarity"
echo "- String constants: Use switch for multiple values"
