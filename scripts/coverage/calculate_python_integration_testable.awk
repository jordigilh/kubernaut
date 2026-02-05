#!/usr/bin/awk -f
# Calculate integration-testable coverage from Python pytest-cov report
# Usage: awk -f calculate_python_integration_testable.awk coverage_integration_holmesgpt-api_python.txt
#
# Python integration-testable packages:
#   - src/extensions/
#   - src/middleware/
#   - src/auth/
#   - src/clients/
#   - src/main.py
#   - src/audit/events.py, factory.py
#   - src/metrics/instrumentation.py
#
# Output: Coverage percentage (e.g., "43.5%")

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
    
    # Match integration-testable packages
    if (file ~ /src\/(extensions|middleware|auth|clients)\//) {
        total += total_stmts
        covered += covered_stmts
    }
    else if (file ~ /src\/main\.py/) {
        total += total_stmts
        covered += covered_stmts
    }
    else if (file ~ /src\/audit\/(events|factory)\.py/) {
        total += total_stmts
        covered += covered_stmts
    }
    else if (file ~ /src\/metrics\/instrumentation\.py/) {
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
