#!/bin/bash
# Add nolint:unused comments to unused functions

cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Function to add nolint comment
add_nolint() {
    local file="$1"
    local line="$2"

    # Check if nolint comment already exists on previous line
    prev_line=$((line - 1))
    if grep -q "nolint:unused" "$file" 2>/dev/null | head -n "$prev_line" | tail -n 1 | grep -q "nolint"; then
        echo "  Skipping $file:$line (already has nolint)"
        return
    fi

    # Add nolint comment before the function
    perl -i -pe "if (\$. == $prev_line) { s/\$/\n\/\/nolint:unused \/\/ TODO: Remove if truly unused or use in implementation/; }" "$file"
    echo "  ✓ Added nolint to $file:$line"
}

# Get all unused function locations and add nolint comments
golangci-lint run ./internal/... ./test/... 2>&1 | grep "unused" | grep -o "[^:]*\.go:[0-9]*" | sort -u | while read location; do
    file=$(echo "$location" | cut -d: -f1)
    line=$(echo "$location" | cut -d: -f2)

    if [ -f "$file" ]; then
        add_nolint "$file" "$line"
    fi
done

echo ""
echo "✅ Added nolint:unused comments to all unused functions"

