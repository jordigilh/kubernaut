#!/bin/bash

# Script to fix duplicate err := declarations in e2e tests
set -e

echo "🔧 Fixing duplicate err declarations..."

# Find all e2e test files that have the duplicate pattern
E2E_FILES=$(find test/e2e -name "*.go" -type f -exec grep -l "err := testCluster.InitializeCluster" {} \; 2>/dev/null || true)

for file in $E2E_FILES; do
    echo "Processing: $file"

    # Check if the file has the duplicate pattern
    if grep -q "var err error" "$file" && grep -q "err := testCluster.InitializeCluster" "$file"; then
        echo "  🔧 Fixing duplicate err in: $file"

        # Create backup
        cp "$file" "$file.err.backup"

        # Fix the duplicate err declaration
        sed -i '' 's/err := testCluster\.InitializeCluster/err = testCluster.InitializeCluster/g' "$file"

        echo "  ✅ Fixed duplicate err in: $file"
    else
        echo "  ⏭️  No duplicate err issues: $file"
    fi
done

echo "🎉 Duplicate err fixes completed!"
