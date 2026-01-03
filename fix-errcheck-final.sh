#!/bin/bash
# Final comprehensive errcheck fix script

set -e

cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Function to fix all Body.Close() patterns (any variable name ending with Resp or resp)
fix_all_resp_body_close() {
    local file="$1"
    echo "Fixing all resp*.Body.Close() in $file..."
    
    # Match any variable name ending with Resp or resp followed by digits
    perl -i -pe 's/^(\s*)(\w*[Rr]esp[0-9]*)\.Body\.Close\(\)/$1_ = $2.Body.Close()/g' "$file"
}

# Function to fix k8sClient.Delete() patterns
fix_k8s_client_delete() {
    local file="$1"
    echo "Fixing k8sClient.Delete() in $file..."
    
    perl -i -pe 's/^(\s*)k8sClient\.Delete\(/$1_ = k8sClient.Delete(/g' "$file"
}

# Function to fix os.RemoveAll() and os.MkdirAll() patterns
fix_os_file_ops() {
    local file="$1"
    echo "Fixing os.RemoveAll/MkdirAll in $file..."
    
    perl -i -pe 's/^(\s*)os\.RemoveAll\(/$1_ = os.RemoveAll(/g' "$file"
    perl -i -pe 's/^(\s*)os\.MkdirAll\(/$1_ = os.MkdirAll(/g' "$file"
}

# Files with remaining errors (from golangci-lint output)
FILES=(
    "test/e2e/gateway/03_k8s_api_rate_limit_test.go"
    "test/e2e/gateway/04_metrics_endpoint_test.go"
    "test/e2e/gateway/05_multi_namespace_isolation_test.go"
    "test/e2e/gateway/06_concurrent_alerts_test.go"
    "test/e2e/gateway/07_health_readiness_test.go"
    "test/e2e/gateway/08_k8s_event_ingestion_test.go"
    "test/e2e/gateway/09_signal_validation_test.go"
    "test/e2e/gateway/10_crd_creation_lifecycle_test.go"
    "test/e2e/gateway/13_redis_failure_graceful_degradation_test.go"
    "test/e2e/notification/01_notification_lifecycle_audit_test.go"
    "test/e2e/notification/02_audit_correlation_test.go"
    "test/e2e/notification/04_failed_delivery_audit_test.go"
    "test/e2e/remediationorchestrator/metrics_e2e_test.go"
    "test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go"
)

for file in "${FILES[@]}"; do
    if [ -f "$file" ]; then
        fix_all_resp_body_close "$file"
        fix_k8s_client_delete "$file"
        fix_os_file_ops "$file"
        echo "✓ Fixed $file"
    else
        echo "⚠ File not found: $file"
    fi
done

echo ""
echo "✓ Final errcheck fixes applied!"

