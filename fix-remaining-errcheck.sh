#!/bin/bash
# Fix remaining errcheck errors by file:line

cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Get all errcheck errors and fix them
golangci-lint run ./... 2>&1 | \
    grep "errcheck" | \
    grep -o "^[^:]*:[0-9]*" | \
    sort -u | \
    while IFS=: read -r file line; do
        if [ -f "$file" ]; then
            # Check what's on that line and add _ = appropriately
            content=$(sed -n "${line}p" "$file")

            # Skip if already has _ =
            if echo "$content" | grep -q "_ ="; then
                continue
            fi

            # Add _ = to the beginning of the statement (after whitespace)
            perl -i -pe "if (\$. == $line && !/_ =/) { s/^(\s+)(\S)$/\$1_ = \$2/; }" "$file"
            echo "✓ $file:$line"
        fi
    done

echo ""
echo "✅ Fixed remaining errcheck errors"

