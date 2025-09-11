#!/bin/bash

# Simple batch conversion for remaining test files
echo "Converting remaining test files to external packages..."

# Function to convert a single file
convert_to_external() {
    local file="$1"
    local old_package=$(grep "^package " "$file" | cut -d' ' -f2)
    local new_package="${old_package}_test"

    # Derive import path from file path
    local import_path=$(echo "$file" | sed 's|/[^/]*\.go$||' | sed 's|^/Users/jgil/go/src/||')

    echo "Converting $file: $old_package -> $new_package"

    # Change package declaration
    sed -i '' "s/^package $old_package$/package $new_package/" "$file"

    # Add import line after existing imports (before closing import parenthesis)
    if grep -q "^import (" "$file"; then
        # Find the line number of the last import line and add after it
        sed -i '' "/^import ($/,/^)$/ { /^)$/ i\\
	\"$import_path\"
}" "$file"
    else
        echo "WARNING: No import block in $file, manual fix needed"
    fi
}

# Convert files one by one
convert_to_external "/Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/ai/insights/effectiveness_suite_test.go"
convert_to_external "/Users/jgil/go/src/github.com/jordigilh/kubernaut/pkg/ai/insights/model_trainer_test.go"
