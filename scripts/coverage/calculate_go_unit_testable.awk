#!/usr/bin/awk -f
# Calculate unit-testable coverage from Go coverage file
# Usage: awk -f calculate_go_unit_testable.awk -v pkg_pattern="/pkg/service/" -v exclude_pattern="/handler\.go/" coverage_unit_service.out
#
# Inputs (via -v):
#   pkg_pattern: Regex to match package (e.g., "/pkg/aianalysis/")
#   exclude_pattern: Regex for integration-only packages to exclude (e.g., "/(handler|audit)/")
#
# Output: Coverage percentage (e.g., "71.8%")

BEGIN {
    mode_line = 1
    total = 0
    covered = 0
}

# Skip mode line (first line: "mode: atomic")
mode_line == 1 {
    mode_line = 0
    next
}

# Process coverage entries: filename:lines num_stmts count
{
    # Skip if doesn't match package pattern
    if ($1 !~ pkg_pattern) next
    
    # Skip excluded patterns (integration-only code)
    if ($1 ~ exclude_pattern) next
    
    # Skip generated code
    if ($1 ~ /ogen-client/) next
    if ($1 ~ /mocks/) next
    if ($1 ~ /\/test\//) next
    
    num_stmts = $2
    count = $3
    
    total += num_stmts
    if (count > 0) covered += num_stmts
}

END {
    if (total > 0) {
        printf "%.1f%%", (covered/total)*100
    } else {
        print "0.0%"
    }
}
