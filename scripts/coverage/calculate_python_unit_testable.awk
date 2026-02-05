#!/usr/bin/awk -f
# Calculate unit-testable coverage from Python pytest-cov report
# Usage: awk -f calculate_python_unit_testable.awk coverage_unit_holmesgpt-api.txt
#
# Python unit-testable packages:
#   - src/models/
#   - src/validation/
#   - src/sanitization/
#   - src/toolsets/
#   - src/config/
#   - src/audit/buffered_store.py
#   - src/errors.py
#
# Output: Coverage percentage (e.g., "76.0%")

BEGIN {
    total = 0
    covered = 0
}

# Skip header and summary lines
/^Name/ || /^---/ || /^==/ || /^TOTAL/ {
    next
}

# Process coverage lines (format: filename    total missed pct)
/^src\// {
    file = $1
    total_stmts = $2
    missed_stmts = $3
    covered_stmts = total_stmts - missed_stmts
    
    # Match unit-testable packages
    if (file ~ /src\/(models|validation|sanitization|toolsets|config)\//) {
        total += total_stmts
        covered += covered_stmts
    }
    else if (file ~ /src\/audit\/buffered_store\.py/ || file ~ /src\/errors\.py/) {
        total += total_stmts
        covered += covered_stmts
    }
}

END {
    if (total > 0) {
        printf "%.1f%%", (covered/total)*100
    } else {
        print "0.0%"
    }
}
