#!/bin/bash

# Script to fix e2e constructor calls that need error handling and version parameter
set -e

echo "🔧 Fixing e2e constructor calls..."

# Find all e2e test files that were processed by the previous script
E2E_FILES=$(find test/e2e -name "*.go" -type f -exec grep -l "cluster.NewE2EClusterManager" {} \;)

for file in $E2E_FILES; do
    echo "Processing: $file"

    # Check if file needs constructor fixing
    if grep -q "testCluster = cluster.NewE2EClusterManager" "$file" 2>/dev/null; then
        echo "  🔧 Fixing constructor in: $file"

        # Create backup
        cp "$file" "$file.constructor.backup"

        # Use a more sophisticated approach with temporary file
        temp_file=$(mktemp)

        # Process the file line by line to handle the constructor pattern
        awk '
        /testCluster = cluster\.NewE2EClusterManager/ {
            # Replace the assignment with proper error handling
            gsub(/testCluster = cluster\.NewE2EClusterManager\("ocp", realLogger\)/, "var err error\n\t\ttestCluster, err = cluster.NewE2EClusterManager(\"ocp\", realLogger)\n\t\tExpect(err).ToNot(HaveOccurred(), \"Failed to create E2E cluster manager\")")
            print
            next
        }
        /testCluster\.InitializeCluster\(ctx\)/ {
            # Add version parameter and proper error handling
            gsub(/testCluster\.InitializeCluster\(ctx\)/, "testCluster.InitializeCluster(ctx, \"latest\")")
            print
            next
        }
        /err := testCluster\.InitializeCluster\(ctx\)/ {
            # Fix existing error handling pattern
            gsub(/err := testCluster\.InitializeCluster\(ctx\)/, "err = testCluster.InitializeCluster(ctx, \"latest\")")
            print
            next
        }
        {
            print
        }
        ' "$file" > "$temp_file"

        # Replace original file
        mv "$temp_file" "$file"

        echo "  ✅ Fixed constructor in: $file"
    else
        echo "  ⏭️  No constructor changes needed: $file"
    fi
done

echo "🎉 E2E constructor fixes completed!"
