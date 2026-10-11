#!/usr/bin/env bats
# Kubernaut Must-Gather - DataStorage API Collection Tests
# BR-PLATFORM-001.6a: Support engineers can analyze workflow catalog and audit trail

load helpers

setup() {
    setup_test_environment
    create_mock_datastorage_workflows
    create_mock_datastorage_audit
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Workflow Catalog Analysis
# ========================================

@test "BR-PLATFORM-001.6a: Support engineer can identify which workflows were available during incident" {
    # Business Outcome: Determine if workflow catalog was complete when issue occurred
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-events.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Verify workflow catalog is accessible
    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/workflows.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/workflows.json" "workflow-1"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/workflows.json" "workflow-2"
}

@test "BR-PLATFORM-001.6a: Support engineer can trace remediation history from audit events" {
    # Business Outcome: Reconstruct timeline of what happened during incident
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-events.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Verify audit trail is complete for forensics
    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "remediation.created"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "workflow.executed"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "2026-01-04T12:00:00Z"
}

# ========================================
# Edge Case: API Unavailable (Partial Collection)
# ========================================

@test "BR-PLATFORM-001.6a: Unavailable DataStorage is explicit and preserves partial diagnostic evidence" {
    # Edge Case: DataStorage service crashed/unreachable
    # Business Outcome: Partial diagnostic data is better than failing entire collection
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-events.json" \
        "000" 7 "curl: (7) Failed to connect to datastorage:8080: Connection refused"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # The collector reports failure; the real gather entry point warns and
    # continues packaging other diagnostics (IT-MG-2493-002).
    assert_failure
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/error.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/error.json" "Failed to connect"
    jq -e '.exit_code == 7 and .http_status == "000"' "${MOCK_COLLECTION_DIR}/datastorage/error.json"
}

# ========================================
# Edge Case: Pagination and Limits
# ========================================

@test "BR-PLATFORM-001.6a: Support engineer gets last 50 workflows (most recent data)" {
    # Edge Case: Cluster has 200+ workflows, only collect most recent
    # Business Outcome: BR-PLATFORM-001.6a specifies limit=50 for performance
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-events.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Verify limit=50 is enforced (per BR-PLATFORM-001.6a)
    assert_success
    assert_file_contains "${TEST_TEMP_DIR}/curl-calls.log" "/api/v1/workflows?limit=50"
}

@test "BR-PLATFORM-001.6a: Support engineer gets last 24h of audit events (1000 max)" {
    # Edge Case: High-volume audit trail (10k+ events)
    # Business Outcome: BR-PLATFORM-001.6a specifies limit=1000, last 24h
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-events.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Verify limit=1000 and timeframe (per BR-PLATFORM-001.6a)
    assert_success
    assert_file_contains "${TEST_TEMP_DIR}/curl-calls.log" "/api/v1/audit/events?limit=1000&offset=0&since=24h&until="
}

# ========================================
# Edge Case: Malformed API Response
# ========================================

@test "BR-PLATFORM-001.6a: Support engineer can identify DataStorage API errors" {
    # Edge Case: API returns HTTP 500 or invalid JSON
    # Business Outcome: Error is documented for troubleshooting
    printf '%s\n' '{"error":"Internal Server Error","status":500}' > "${TEST_TEMP_DIR}/http-error.json"
    mock_curl "${TEST_TEMP_DIR}/http-error.json" "${TEST_TEMP_DIR}/http-error.json" 500

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Should capture the error response
    assert_failure
    # Error is preserved in collected data
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/error.json"
    jq -e '.exit_code == 0 and .http_status == "500" and (.message | contains("Internal Server Error"))' \
        "${MOCK_COLLECTION_DIR}/datastorage/error.json"
}

# ========================================
# Edge Case: Empty Results
# ========================================

@test "BR-PLATFORM-001.6a: Support engineer can identify when no workflows exist in catalog" {
    # Edge Case: Fresh deployment, no workflows created yet
    # Business Outcome: Empty catalog indicates misconfiguration
    cat > "${TEST_TEMP_DIR}/workflows-empty.json" <<'EOF'
{
  "workflows": [],
  "total": 0
}
EOF
    mock_curl "${TEST_TEMP_DIR}/workflows-empty.json" "${TEST_TEMP_DIR}/audit-events.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Empty catalog is captured (signals problem)
    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/workflows.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/workflows.json" '"total": 0'
}

@test "BR-PLATFORM-001.6a: Support engineer can identify when no audit events exist" {
    # Edge Case: Audit trail empty (DataStorage never received events)
    # Business Outcome: Missing audit trail indicates integration issue
    cat > "${TEST_TEMP_DIR}/audit-empty.json" <<'EOF'
{
  "data": [],
  "pagination": {"total": 0, "limit": 1000, "offset": 0}
}
EOF
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-empty.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Empty audit trail is captured (signals audit integration problem)
    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" '"total": 0'
}

# ========================================
# UT-MG-2037-004: DataStorage URL built from configurable namespace (Issue #2037)
# ========================================

@test "UT-MG-2037-004: DataStorage URL targets the configured RELEASE_NAMESPACE, not a hardcoded literal" {
    # Business Outcome: BR-PLATFORM-001.6a -- collection must reach DataStorage
    # even when the chart is installed into a non-default release namespace.
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-events.json"

    run env RELEASE_NAMESPACE="custom-kubernaut-ns" bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_contains "${TEST_TEMP_DIR}/curl-calls.log" "https://data-storage-service.custom-kubernaut-ns.svc:8080/"
}

@test "UT-MG-2037-004: DataStorage URL defaults to kubernaut-system when RELEASE_NAMESPACE is unset" {
    # Business Outcome: standalone invocation (e.g. direct script debugging)
    # still targets the common default install namespace.
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-events.json"

    run env -u RELEASE_NAMESPACE bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_contains "${TEST_TEMP_DIR}/curl-calls.log" "https://data-storage-service.kubernaut-system.svc:8080/"
}

@test "BR-PLATFORM-001.6a: Support engineer can identify DataStorage network timeouts" {
    # Edge Case: API request times out (slow network, overloaded service)
    # Business Outcome: Timeout is documented as diagnostic clue
    mock_curl "${TEST_TEMP_DIR}/workflows.json" "${TEST_TEMP_DIR}/audit-events.json" \
        "000" 28 "curl: (28) Operation timed out after 30000 milliseconds"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Timeout is documented
    assert_failure
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/error.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/error.json" "timed out"
    jq -e '.exit_code == 28 and .http_status == "000"' "${MOCK_COLLECTION_DIR}/datastorage/error.json"
}
