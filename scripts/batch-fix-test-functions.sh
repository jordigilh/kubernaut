#!/bin/bash

# Batch fix script for missing TestXxx functions
# Addresses "No packages found for open file" errors efficiently

set -e

echo "🔧 Batch fixing missing TestXxx functions..."

# Function to add TestXxx function to a test file
fix_test_file() {
    local file="$1"
    local base_name=$(basename "$file" _test.go)
    local package_name=$(basename "$(dirname "$file")")

    # Generate proper test function name
    test_func_name="Test$(echo "$base_name" | sed 's/_\([a-z]\)/\U\1/g' | sed 's/^./\U&/')"

    echo "  📝 Fixing: $file -> $test_func_name"

    # Check if file already has testing import
    if ! grep -q '"testing"' "$file"; then
        # Add testing import after existing imports
        sed -i '' '/^import (/,/^)/ {
            /^)/ i\
	"testing"
        }' "$file"
    fi

    # Add TestXxx function after imports, before first var _ = Describe
    sed -i '' '/^)/a\
\
// '"$test_func_name"' runs the test suite\
// Business Requirement: Auto-generated test runner for Go testing framework\
func '"$test_func_name"'(t *testing.T) {\
	RegisterFailHandler(Fail)\
	RunSpecs(t, "'"$(echo "$base_name" | sed 's/_/ /g' | sed 's/\b\w/\u&/g')"' Unit Tests Suite")\
}
' "$file"
}

# Get all files missing TestXxx functions
missing_files=$(find ./test/unit -name "*_test.go" -exec sh -c 'if ! grep -q "func Test.*testing\\.T" "$1"; then echo "$1"; fi' _ {} \;)

if [ -z "$missing_files" ]; then
    echo "✅ No files missing TestXxx functions found."
    exit 0
fi

total_files=$(echo "$missing_files" | wc -l | tr -d ' ')
echo "📊 Found $total_files files missing TestXxx functions"

# Process files in batches for efficiency
batch_size=10
current_batch=0

echo "$missing_files" | while read -r file; do
    if [ -n "$file" ] && [ -f "$file" ]; then
        fix_test_file "$file"
        current_batch=$((current_batch + 1))

        # Show progress every batch
        if [ $((current_batch % batch_size)) -eq 0 ]; then
            echo "  ✅ Processed $current_batch/$total_files files..."
        fi
    fi
done

echo "🎉 Batch fix complete! All test files should now be discoverable by Go testing framework."
echo "💡 Tip: Restart your IDE/language server to refresh package discovery."
