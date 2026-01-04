#!/bin/bash
# Script to fix remaining errcheck errors (non-defer patterns)

set -e

cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Function to fix non-defer resp.Body.Close() patterns
fix_non_defer_resp_close() {
    local file="$1"
    echo "Fixing non-defer resp.Body.Close() in $file..."

    # Handle patterns like: resp1.Body.Close() → _ = resp1.Body.Close()
    # This handles resp, resp1, resp2, resp3 variants
    perl -i -pe 's/^(\s*)resp([0-9]*)\.Body\.Close\(\)/$1_ = resp$2.Body.Close()/g' "$file"
}

# Function to fix non-defer db.Close() patterns
fix_non_defer_db_close() {
    local file="$1"
    echo "Fixing non-defer db.Close() in $file..."

    perl -i -pe 's/^(\s*)db\.Close\(\)/$1_ = db.Close()/g' "$file"
    perl -i -pe 's/^(\s*)testDB\.Close\(\)/$1_ = testDB.Close()/g' "$file"
}

# Function to fix non-defer conn.Close() patterns
fix_non_defer_conn_close() {
    local file="$1"
    echo "Fixing non-defer conn.Close() in $file..."

    perl -i -pe 's/^(\s*)conn\.Close\(\)/$1_ = conn.Close()/g' "$file"
}

# Files with remaining errors (from golangci-lint output)
FILES=(
    "test/e2e/aianalysis/02_metrics_test.go"
    "test/e2e/datastorage/06_workflow_search_audit_test.go"
    "test/e2e/datastorage/07_workflow_version_management_test.go"
    "test/e2e/datastorage/08_workflow_search_edge_cases_test.go"
    "test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go"
    "test/e2e/datastorage/10_malformed_event_rejection_test.go"
    "test/e2e/datastorage/11_connection_pool_exhaustion_test.go"
    "test/e2e/datastorage/datastorage_e2e_suite_test.go"
    "test/e2e/datastorage/helpers.go"
    "test/e2e/gateway/02_state_based_deduplication_test.go"
)

for file in "${FILES[@]}"; do
    if [ -f "$file" ]; then
        fix_non_defer_resp_close "$file"
        fix_non_defer_db_close "$file"
        fix_non_defer_conn_close "$file"
        echo "✓ Fixed $file"
    else
        echo "⚠ File not found: $file"
    fi
done

echo ""
echo "✓ All remaining errcheck fixes applied!"

