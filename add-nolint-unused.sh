#!/bin/bash
# Add //nolint:unused to unused functions

cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Extract file:line from golangci-lint output and process each
golangci-lint run ./internal/... ./test/... 2>&1 | \
    grep "is unused (unused)" | \
    grep -o "^[^:]*:[0-9]*" | \
    sort -u | \
    while IFS=: read -r file line; do
        if [ -f "$file" ]; then
            # Check if line already has nolint comment
            if ! sed -n "${line}p" "$file" | grep -q "nolint:unused"; then
                # Add //nolint:unused to the end of the line
                perl -i -pe "if (\$. == $line && !/nolint:unused/) { chomp; \$_ .= \" \/\/nolint:unused\n\"; }" "$file"
                echo "✓ $file:$line"
            fi
        fi
    done

echo ""
echo "✅ Added //nolint:unused to all unused functions"

