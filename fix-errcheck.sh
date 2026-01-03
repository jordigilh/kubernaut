#!/bin/bash
# Script to fix errcheck errors in E2E test files

set -e

cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Function to fix resp.Body.Close() errors
fix_resp_body_close() {
    local file="$1"
    echo "Fixing resp.Body.Close() in $file..."
    
    # Use perl for in-place editing with multi-line regex
    perl -i -pe 's/defer\s+resp\.Body\.Close\(\)/defer func() { _ = resp.Body.Close() }()/g' "$file"
    perl -i -pe 's/defer\s+resp([0-9]+)\.Body\.Close\(\)/defer func() { _ = resp$1.Body.Close() }()/g' "$file"
    perl -i -pe 's/defer\s+healthResp\.Body\.Close\(\)/defer func() { _ = healthResp.Body.Close() }()/g' "$file"
    perl -i -pe 's/defer\s+metricsResp\.Body\.Close\(\)/defer func() { _ = metricsResp.Body.Close() }()/g' "$file"
}

# Function to fix db.Close() errors
fix_db_close() {
    local file="$1"
    echo "Fixing db.Close() in $file..."
    
    perl -i -pe 's/defer\s+testDB\.Close\(\)/defer func() { _ = testDB.Close() }()/g' "$file"
    perl -i -pe 's/defer\s+db\.Close\(\)/defer func() { _ = db.Close() }()/g' "$file"
}

# Function to fix conn.Close() errors
fix_conn_close() {
    local file="$1"
    echo "Fixing conn.Close() in $file..."
    
    perl -i -pe 's/defer\s+conn\.Close\(\)/defer func() { _ = conn.Close() }()/g' "$file"
}

# Function to fix rows.Close() errors
fix_rows_close() {
    local file="$1"
    echo "Fixing rows.Close() in $file..."
    
    perl -i -pe 's/defer\s+rows\.Close\(\)/defer func() { _ = rows.Close() }()/g' "$file"
}

# Function to fix json.Decoder.Decode() errors - these need manual review
fix_decoder_decode() {
    local file="$1"
    echo "Note: $file has json.Decoder.Decode() error - needs manual review"
}

# Process all files
FILES=(
    "test/e2e/aianalysis/02_metrics_test.go"
    "test/e2e/aianalysis/suite_test.go"
    "test/e2e/datastorage/01_happy_path_test.go"
    "test/e2e/datastorage/02_dlq_fallback_test.go"
    "test/e2e/datastorage/04_workflow_search_test.go"
    "test/e2e/datastorage/06_workflow_search_audit_test.go"
    "test/e2e/datastorage/07_workflow_version_management_test.go"
    "test/e2e/datastorage/08_workflow_search_edge_cases_test.go"
    "test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go"
    "test/e2e/datastorage/10_malformed_event_rejection_test.go"
    "test/e2e/datastorage/11_connection_pool_exhaustion_test.go"
    "test/e2e/datastorage/datastorage_e2e_suite_test.go"
    "test/e2e/datastorage/helpers.go"
    "test/e2e/gateway/02_state_based_deduplication_test.go"
    "test/e2e/gateway/03_k8s_api_rate_limit_test.go"
    "test/e2e/gateway/04_metrics_endpoint_test.go"
    "test/e2e/gateway/05_multi_namespace_isolation_test.go"
    "test/e2e/gateway/06_concurrent_alerts_test.go"
    "test/e2e/gateway/07_health_readiness_test.go"
    "test/e2e/gateway/08_k8s_event_ingestion_test.go"
    "test/e2e/gateway/09_signal_validation_test.go"
    "test/e2e/gateway/10_crd_creation_lifecycle_test.go"
    "test/e2e/gateway/11_fingerprint_stability_test.go"
    "test/e2e/gateway/12_gateway_restart_recovery_test.go"
    "test/e2e/gateway/13_redis_failure_graceful_degradation_test.go"
    "test/e2e/gateway/14_deduplication_ttl_expiration_test.go"
    "test/e2e/gateway/15_audit_trace_validation_test.go"
    "test/e2e/gateway/16_structured_logging_test.go"
    "test/e2e/gateway/17_error_response_codes_test.go"
    "test/e2e/gateway/18_cors_enforcement_test.go"
    "test/e2e/gateway/19_replay_attack_prevention_test.go"
    "test/e2e/gateway/20_security_headers_test.go"
    "test/e2e/gateway/21_crd_lifecycle_test.go"
    "test/e2e/gateway/deduplication_helpers.go"
    "test/e2e/gateway/gateway_e2e_suite_test.go"
    "test/e2e/holmesgpt-api/holmesgpt_api_e2e_suite_test.go"
    "test/e2e/notification/01_notification_lifecycle_audit_test.go"
    "test/e2e/notification/02_audit_correlation_test.go"
    "test/e2e/notification/04_failed_delivery_audit_test.go"
    "test/e2e/notification/04_metrics_validation_test.go"
    "test/e2e/notification/05_retry_exponential_backoff_test.go"
    "test/e2e/notification/notification_e2e_suite_test.go"
    "test/e2e/remediationorchestrator/approval_e2e_test.go"
    "test/e2e/remediationorchestrator/audit_wiring_e2e_test.go"
    "test/e2e/remediationorchestrator/blocking_e2e_test.go"
    "test/e2e/remediationorchestrator/lifecycle_e2e_test.go"
    "test/e2e/remediationorchestrator/metrics_e2e_test.go"
    "test/e2e/remediationorchestrator/notification_cascade_e2e_test.go"
    "test/e2e/remediationorchestrator/operational_e2e_test.go"
    "test/e2e/remediationorchestrator/routing_cooldown_e2e_test.go"
    "test/e2e/remediationorchestrator/suite_test.go"
    "test/e2e/signalprocessing/business_requirements_test.go"
    "test/e2e/signalprocessing/suite_test.go"
    "test/e2e/workflowexecution/02_observability_test.go"
    "test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go"
)

for file in "${FILES[@]}"; do
    if [ -f "$file" ]; then
        fix_resp_body_close "$file"
        fix_db_close "$file"
        fix_conn_close "$file"
        fix_rows_close "$file"
        echo "✓ Fixed $file"
    else
        echo "⚠ File not found: $file"
    fi
done

echo ""
echo "✓ All automatic fixes applied!"
echo "Note: Files with json.Decoder.Decode() errors need manual review:"
echo "  - test/e2e/datastorage/08_workflow_search_edge_cases_test.go (lines 445, 474)"

