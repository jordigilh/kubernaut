#!/usr/bin/awk -f
# Merge coverage from multiple Go coverage files (unit, integration, E2E)
# Usage: awk -f merge_go_coverage.awk -v pkg_pattern="/pkg/service/" coverage_unit.out coverage_integration.out coverage_e2e.out
#
# Inputs (via -v):
#   pkg_pattern: Regex to match package (e.g., "/pkg/aianalysis/")
#
# Algorithm: For each file:line, mark as covered if ANY input file shows count > 0
# Output: Coverage percentage (e.g., "76.9%")

BEGIN {
    mode = 1
}

# Skip mode line in all files
mode == 1 {
    mode = 0
    next
}

# Process coverage entries from all files
{
    # Skip if doesn't match package pattern
    if ($1 !~ pkg_pattern) next
    
    # Skip generated code
    if ($1 ~ /ogen-client/) next
    if ($1 ~ /mocks/) next
    if ($1 ~ /\/test\//) next
    
    key = $1       # file.go:lines (unique identifier)
    stmts = $2     # number of statements
    count = $3     # execution count
    
    # Track total statements for this key
    if (!(key in total_stmts)) {
        total_stmts[key] = stmts
    }
    
    # Mark as covered if ANY file shows count > 0
    if (count > 0) {
        covered[key] = stmts
    }
}

END {
    total = 0
    covered_count = 0
    
    for (k in total_stmts) {
        total += total_stmts[k]
        if (k in covered) {
            covered_count += covered[k]
        }
    }
    
    if (total > 0) {
        printf "%.1f%%", (covered_count/total)*100
    } else {
        print "0.0%"
    }
}
