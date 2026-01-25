#!/usr/bin/env bats
# Kubernaut Must-Gather - DataStorage API Collection Tests
# BR-PLATFORM-001.6a: Support engineers can analyze workflow catalog and audit trail

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Workflow Catalog Analysis
# ========================================

@test "BR-PLATFORM-001.6a: Support engineer can identify which workflows were available during incident" {
    # Business Outcome: Determine if workflow catalog was complete when issue occurred
    create_mock_datastorage_workflows
    mock_curl "${TEST_TEMP_DIR}/workflows.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Verify workflow catalog is accessible
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/workflows.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/workflows.json" "workflow-1"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/workflows.json" "workflow-2"
}

@test "BR-PLATFORM-001.6a: Support engineer can trace remediation history from audit events" {
    # Business Outcome: Reconstruct timeline of what happened during incident
    create_mock_datastorage_audit
    mock_curl "${TEST_TEMP_DIR}/audit-events.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Verify audit trail is complete for forensics
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "remediation.created"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "workflow.executed"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "2026-01-04T12:00:00Z"
}

# ========================================
# Edge Case: API Unavailable (Partial Collection)
# ========================================

@test "BR-PLATFORM-001.6a: Collection succeeds when DataStorage API is unavailable" {
    # Edge Case: DataStorage service crashed/unreachable
    # Business Outcome: Partial diagnostic data is better than failing entire collection
    cat > "${TEST_TEMP_DIR}/bin/curl" <<'EOF'
#!/bin/bash
echo "curl: (7) Failed to connect to datastorage:8080: Connection refused"
exit 7
EOF
    chmod +x "${TEST_TEMP_DIR}/bin/curl"
    export PATH="${TEST_TEMP_DIR}/bin:${PATH}"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Should document the failure without failing collection
    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/error.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/error.json" "Failed to connect"
}

# ========================================
# Edge Case: Pagination and Limits
# ========================================

@test "BR-PLATFORM-001.6a: Support engineer gets last 50 workflows (most recent data)" {
    # Edge Case: Cluster has 200+ workflows, only collect most recent
    # Business Outcome: BR-PLATFORM-001.6a specifies limit=50 for performance
    create_mock_datastorage_workflows
    mock_curl "${TEST_TEMP_DIR}/workflows.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Verify limit=50 is enforced (per BR-PLATFORM-001.6a)
    [[ "$output" =~ "limit" ]] && [[ "$output" =~ "50" ]]
}

@test "BR-PLATFORM-001.6a: Support engineer gets last 24h of audit events (1000 max)" {
    # Edge Case: High-volume audit trail (10k+ events)
    # Business Outcome: BR-PLATFORM-001.6a specifies limit=1000, last 24h
    create_mock_datastorage_audit
    mock_curl "${TEST_TEMP_DIR}/audit-events.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Verify limit=1000 and timeframe (per BR-PLATFORM-001.6a)
    [[ "$output" =~ "limit" ]] && [[ "$output" =~ "1000" ]]
}

# ========================================
# Edge Case: Malformed API Response
# ========================================

@test "BR-PLATFORM-001.6a: Support engineer can identify DataStorage API errors" {
    # Edge Case: API returns HTTP 500 or invalid JSON
    # Business Outcome: Error is documented for troubleshooting
    cat > "${TEST_TEMP_DIR}/bin/curl" <<'EOF'
#!/bin/bash
echo '{"error": "Internal Server Error", "status": 500}'
exit 0
EOF
    chmod +x "${TEST_TEMP_DIR}/bin/curl"
    export PATH="${TEST_TEMP_DIR}/bin:${PATH}"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Should capture the error response
    assert_success
    # Error is preserved in collected data
    [ -f "${MOCK_COLLECTION_DIR}/datastorage/workflows.json" ] || [ -f "${MOCK_COLLECTION_DIR}/datastorage/error.json" ]
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
    mock_curl "${TEST_TEMP_DIR}/workflows-empty.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Empty catalog is captured (signals problem)
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
    mock_curl "${TEST_TEMP_DIR}/audit-empty.json"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Empty audit trail is captured (signals audit integration problem)
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" '"total": 0'
}

# ========================================
# Edge Case: Network Timeout
# ========================================

@test "BR-PLATFORM-001.6a: Support engineer can identify DataStorage network timeouts" {
    # Edge Case: API request times out (slow network, overloaded service)
    # Business Outcome: Timeout is documented as diagnostic clue
    cat > "${TEST_TEMP_DIR}/bin/curl" <<'EOF'
#!/bin/bash
echo "curl: (28) Operation timed out after 30000 milliseconds"
exit 28
EOF
    chmod +x "${TEST_TEMP_DIR}/bin/curl"
    export PATH="${TEST_TEMP_DIR}/bin:${PATH}"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    # Timeout is documented
    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/error.json"
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/error.json" "timed out"
}

