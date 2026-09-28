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

# ========================================
# UT-MG-2037-004: DataStorage URL built from configurable namespace (Issue #2037)
# ========================================

@test "UT-MG-2037-004: DataStorage URL targets the configured RELEASE_NAMESPACE, not a hardcoded literal" {
    # Business Outcome: BR-PLATFORM-001.6a -- collection must reach DataStorage
    # even when the chart is installed into a non-default release namespace.
    create_mock_datastorage_workflows
    mock_curl "${TEST_TEMP_DIR}/workflows.json"

    run env RELEASE_NAMESPACE="custom-kubernaut-ns" bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_contains "${TEST_TEMP_DIR}/curl-calls.log" "data-storage-service.custom-kubernaut-ns.svc.cluster.local"
}

@test "UT-MG-2037-004: DataStorage URL defaults to kubernaut-system when RELEASE_NAMESPACE is unset" {
    # Business Outcome: standalone invocation (e.g. direct script debugging)
    # still targets the common default install namespace.
    create_mock_datastorage_workflows
    mock_curl "${TEST_TEMP_DIR}/workflows.json"

    run env -u RELEASE_NAMESPACE bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_contains "${TEST_TEMP_DIR}/curl-calls.log" "data-storage-service.kubernaut-system.svc.cluster.local"
}

@test "UT-MG-2463-003: DataStorage collection retries through the Kubernetes API proxy after Service DNS failure" {
    # BR-PLATFORM-001.6a / issue #2463: the local must-gather container shares
    # the Kind network but not cluster DNS. curl exit 6 must therefore switch
    # to the authenticated Kubernetes Service proxy so the audit trail remains
    # collectible while the Agent pod is still alive.
    create_mock_datastorage_workflows

    cat > "${TEST_TEMP_DIR}/bin/kubectl" <<'EOF'
#!/bin/bash
echo "$*" >> "${TEST_TEMP_DIR}/kubectl-proxy-calls.log"
if [[ "$1" == "proxy" ]]; then
    trap 'exit 0' TERM INT
    while :; do
        read -r -t 1 _ || true
    done
fi
exit 0
EOF
    chmod +x "${TEST_TEMP_DIR}/bin/kubectl"

    cat > "${TEST_TEMP_DIR}/bin/curl" <<'EOF'
#!/bin/bash
count_file="${TEST_TEMP_DIR}/curl-count"
count=0
if [ -f "${count_file}" ]; then
    count=$(cat "${count_file}")
fi
count=$((count + 1))
echo "${count}" > "${TEST_TEMP_DIR}/curl-count"
url="${@: -1}"
echo "${url}" >> "${TEST_TEMP_DIR}/curl-calls.log"
if [ "${count}" -eq 1 ]; then
    exit 6
fi
if [[ "${url}" == "http://127.0.0.1:8001/version" ]]; then
    echo '{"gitVersion":"v1.36.2"}'
    exit 0
fi
cat "${TEST_TEMP_DIR}/workflows.json"
EOF
    chmod +x "${TEST_TEMP_DIR}/bin/curl"
    export PATH="${TEST_TEMP_DIR}/bin:${PATH}"

    run bash "${COLLECTORS_DIR}/datastorage.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json"
    assert_file_contains "${TEST_TEMP_DIR}/kubectl-proxy-calls.log" "proxy"
    assert_file_contains "${TEST_TEMP_DIR}/curl-calls.log" "/api/v1/namespaces/kubernaut-system/services/http:data-storage-service:8080/proxy/api/v1/audit/events"
}

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
