#!/bin/bash

# Find all functions with ctx parameters that don't use ctx.Done/ctx.Err/ctx.Value
find . -name "*.go" -not -path "./vendor/*" | while read file; do
    echo "Processing $file"
    
    # Get line numbers of functions with ctx parameters
    grep -n "func.*ctx context\.Context" "$file" 2>/dev/null | while IFS=: read lineno line; do
        # Check if function uses ctx in next 10 lines
        if ! sed -n "${lineno},$(($lineno+10))p" "$file" | grep -q "ctx\.Done\|ctx\.Err\|ctx\.Value\|select.*ctx"; then
            echo "  Found unused ctx at line $lineno: $line"
            
            # Create a backup
            cp "$file" "$file.bak"
            
            # Add context cancellation check after function signature
            awk -v target_line="$lineno" '
                NR == target_line {
                    print $0
                    if (match($0, /func.*ctx context\.Context.*{/)) {
                        getline
                        print "\t// Check for context cancellation"
                        print "\tselect {"
                        print "\tcase <-ctx.Done():"
                        if (match($0, /return.*error/)) {
                            print "\t\treturn ctx.Err()"
                        } else if (match($0, /return/)) {
                            print "\t\treturn nil, ctx.Err()"
                        } else {
                            print "\t\treturn ctx.Err()"
                        }
                        print "\tdefault:"
                        print "\t}"
                        print ""
                    }
                    print $0
                }
                NR != target_line { print }
            ' "$file.bak" > "$file"
        fi
    done
done
