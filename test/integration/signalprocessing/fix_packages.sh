#!/bin/bash
# Fix package declarations: integration tests should use package signalprocessing (no _test suffix)

for file in *.go; do
    sed -i '' 's/^package signalprocessing_test$/package signalprocessing/g' "$file"
    echo "Fixed $file"
done

echo "All files now use: package signalprocessing"
