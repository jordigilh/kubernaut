#!/bin/bash

# Script to fix e2e test compilation issues
# Fixes: enhanced.TestClusterManager -> cluster.E2EClusterManager
#        BeOneOf -> BeElementOf

set -e

echo "🔧 Fixing e2e test compilation issues..."

# Find all e2e test files
E2E_FILES=$(find test/e2e -name "*.go" -type f)

for file in $E2E_FILES; do
    echo "Processing: $file"

    # Skip if already processed (contains cluster.E2EClusterManager)
    if grep -q "cluster.E2EClusterManager" "$file" 2>/dev/null; then
        echo "  ✅ Already fixed: $file"
        continue
    fi

    # Check if file needs fixing
    if grep -q "enhanced.TestClusterManager\|enhanced.NewTestClusterManager\|BeOneOf" "$file" 2>/dev/null; then
        echo "  🔧 Fixing: $file"

        # Create backup
        cp "$file" "$file.backup"

        # Fix import
        sed -i '' 's|"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"|"github.com/jordigilh/kubernaut/pkg/e2e/cluster"|g' "$file"

        # Fix type declarations
        sed -i '' 's|enhanced\.TestClusterManager|cluster.E2EClusterManager|g' "$file"

        # Fix constructor calls - more complex replacement
        sed -i '' 's|enhanced\.NewTestClusterManager()|cluster.NewE2EClusterManager("ocp", realLogger)|g' "$file"

        # Fix method calls
        sed -i '' 's|SetupTestCluster|InitializeCluster|g' "$file"
        sed -i '' 's|CleanupTestCluster|Cleanup|g' "$file"

        # Fix BeOneOf matcher
        sed -i '' 's|BeOneOf|BeElementOf|g' "$file"

        echo "  ✅ Fixed: $file"
    else
        echo "  ⏭️  No changes needed: $file"
    fi
done

echo "🎉 E2E test fixes completed!"
