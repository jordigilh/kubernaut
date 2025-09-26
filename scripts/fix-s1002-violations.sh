#!/bin/bash

# Fix S1002 staticcheck violations (redundant nil checks)
# This script detects and fixes the pattern: "x != nil && len(x) > n"

set -e

echo "🔍 Detecting and fixing S1002 violations (redundant nil checks)"

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

# Function to fix S1002 violations in a file
fix_s1002_in_file() {
    local file="$1"
    local temp_file=$(mktemp)
    local fixed=false

    echo "   Checking: $file"

    # Use sed to fix the pattern: "!= nil && len(" -> "len("
    # This handles various spacing patterns
    sed -E 's/([a-zA-Z_][a-zA-Z0-9_.]*)\s*!=\s*nil\s*&&\s*len\(\1\)/len(\1)/g' "$file" > "$temp_file"

    # Check if any changes were made
    if ! cmp -s "$file" "$temp_file"; then
        echo -e "   ${GREEN}✅ Fixed S1002 violations in $file${NC}"
        cp "$temp_file" "$file"
        fixed=true
        FIXES_APPLIED=$((FIXES_APPLIED + 1))
    fi

    rm "$temp_file"

    if [ "$fixed" = true ]; then
        # Run gofmt to ensure proper formatting
        gofmt -w "$file"

        # Verify the fix didn't break compilation
        if ! go build -o /dev/null "$file" 2>/dev/null; then
            echo -e "   ${RED}⚠️  Warning: Fix in $file may have introduced compilation issues${NC}"
            echo "   Please review manually"
        fi
    fi
}

# Find all Go files and process them
echo "1. Scanning for Go files..."

find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | while read -r file; do
    FILES_CHECKED=$((FILES_CHECKED + 1))

    # Check if file contains the problematic pattern
    if grep -q "!= nil && len(" "$file"; then
        fix_s1002_in_file "$file"
    fi
done

echo
echo "2. Running verification..."

# Run staticcheck to verify fixes
if command -v staticcheck >/dev/null 2>&1; then
    echo "   Running staticcheck to verify fixes..."

    S1002_VIOLATIONS=$(staticcheck ./... 2>/dev/null | grep "S1002:" | wc -l || echo 0)

    if [ "$S1002_VIOLATIONS" -eq 0 ]; then
        echo -e "   ${GREEN}✅ No S1002 violations detected${NC}"
    else
        echo -e "   ${YELLOW}⚠️  $S1002_VIOLATIONS S1002 violations still remain${NC}"
        echo "   Manual review may be required for complex cases"

        # Show remaining violations
        echo "   Remaining violations:"
        staticcheck ./... 2>/dev/null | grep "S1002:" | head -5 | sed 's/^/     /'
    fi
else
    echo "   Staticcheck not available - skipping verification"
fi

echo
echo "📊 Summary"
echo "=========="
echo "Files checked: $FILES_CHECKED"
echo "Fixes applied: $FIXES_APPLIED"

if [ "$FIXES_APPLIED" -gt 0 ]; then
    echo
    echo -e "${GREEN}✅ S1002 violations fixed successfully${NC}"
    echo
    echo "Next steps:"
    echo "1. Review the changes: git diff"
    echo "2. Run tests: make test"
    echo "3. Run linting: make lint"
    echo "4. Commit changes if everything looks good"
else
    echo
    echo -e "${GREEN}✅ No S1002 violations found${NC}"
fi
