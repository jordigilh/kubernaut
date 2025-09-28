#!/bin/bash

# Script to add missing test runner functions to Ginkgo test files
# This script fixes the issue where test files have Describe/It but no func Test*(t *testing.T)

set -e

echo "🔧 Fixing missing test runner functions..."

# Find all files that need test runners
FILES_TO_FIX=$(find ./test/unit -name "*_test.go" -type f -exec sh -c '
    if grep -q "Describe\|It(" "$1" && ! grep -q "func Test.*testing\.T\|RunSpecs" "$1"; then
        echo "$1"
    fi
' _ {} \;)

if [ -z "$FILES_TO_FIX" ]; then
    echo "✅ No files need test runner functions!"
    exit 0
fi

TOTAL_FILES=$(echo "$FILES_TO_FIX" | wc -l)
echo "📋 Found $TOTAL_FILES files that need test runner functions"
echo ""

COUNT=0
for file in $FILES_TO_FIX; do
    COUNT=$((COUNT + 1))
    echo "🔨 [$COUNT/$TOTAL_FILES] Fixing: $file"

    # Extract package name from the file
    PACKAGE_NAME=$(grep "^package " "$file" | head -1 | awk '{print $2}')

    # Generate test function name from file path
    FILENAME=$(basename "$file" .go)
    # Convert filename to CamelCase for test function name
    # Remove _test suffix and convert to proper CamelCase
    BASE_NAME=$(echo "$FILENAME" | sed 's/_test$//')
    TEST_FUNC_NAME=$(echo "$BASE_NAME" | sed -r 's/(^|_)([a-z])/\U\2/g')

    # Generate suite name from the base name with proper formatting
    SUITE_NAME=$(echo "$BASE_NAME" | sed 's/_/ /g' | sed 's/\b\w/\U&/g') Suite

    # Check if file needs testing import
    if ! grep -q '"testing"' "$file"; then
        # Add testing import if not present
        # Find the import block and add testing
        if grep -q 'import (' "$file"; then
            # Multi-line import block exists
            sed -i '' '/import (/a\
	"testing"\
' "$file"
        else
            # Single line import or no imports
            if grep -q '^import ' "$file"; then
                # Add after existing import
                sed -i '' '/^import /a\
import "testing"\
' "$file"
            else
                # Add import block after package
                sed -i '' '/^package /a\
\
import "testing"\
' "$file"
            fi
        fi
    fi

    # Add the test runner function at the end of file
    cat >> "$file" << EOF

// TestRunner bootstraps the Ginkgo test suite
func Test${TEST_FUNC_NAME}(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "${SUITE_NAME}")
}
EOF

    echo "   ✅ Added Test${TEST_FUNC_NAME} function"
done

echo ""
echo "🎉 Successfully fixed $TOTAL_FILES test files!"
echo "📊 All test files now have proper test runner functions"
echo ""
echo "🧪 Run 'make test' to verify all tests can now execute"
