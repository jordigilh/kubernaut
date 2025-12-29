#!/bin/bash
# fix-white-box-testing.sh
# Fixes test files to use white-box testing (same package name as production code)
# Per TEST_PACKAGE_NAMING_STANDARD.md (Version 1.1)

set -e

echo "🔧 Fixing white-box testing violations (43 files)"
echo "Authority: docs/testing/TEST_PACKAGE_NAMING_STANDARD.md"
echo ""

# Track statistics
total_fixed=0
failed_files=()

# Get list of files with _test suffix violations
violation_files=$(find test/ -name "*_test.go" -exec grep -l "^package.*_test$" {} \;)

for file in $violation_files; do
    # Extract current package name with _test suffix
    current_pkg=$(grep "^package " "$file" | head -1 | awk '{print $2}')

    # Remove _test suffix
    correct_pkg=$(echo "$current_pkg" | sed 's/_test$//')

    # Skip if already correct (shouldn't happen, but safety check)
    if [ "$current_pkg" = "$correct_pkg" ]; then
        echo "⚠️  Skipped (already correct): $file"
        continue
    fi

    # Create backup
    cp "$file" "$file.bak"

    # Replace package declaration
    sed -i.tmp "s/^package ${current_pkg}$/package ${correct_pkg}/" "$file"
    rm -f "$file.tmp"

    if grep -q "^package ${correct_pkg}$" "$file"; then
        echo "✅ Fixed: $file"
        echo "   $current_pkg → $correct_pkg"
        ((total_fixed++))
    else
        echo "❌ FAILED: $file"
        failed_files+=("$file")
        # Restore from backup
        mv "$file.bak" "$file"
    fi
done

echo ""
echo "📊 Summary:"
echo "   Total files fixed: $total_fixed"
echo "   Failed: ${#failed_files[@]}"

if [ ${#failed_files[@]} -gt 0 ]; then
    echo ""
    echo "❌ Failed files:"
    for file in "${failed_files[@]}"; do
        echo "   - $file"
    done
fi

echo ""
echo "⚠️  Next Steps:"
echo "   1. Review changes with: git diff test/"
echo "   2. Remove .bak files: find test/ -name '*.bak' -delete"
echo "   3. Run tests: make test && make test-integration && make test-e2e"
echo "   4. If tests pass, commit changes"
echo ""
echo "Note: You may need to manually remove imports and package prefixes"
echo "      in files that now use the same package as production code."

