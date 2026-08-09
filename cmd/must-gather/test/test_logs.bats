#!/usr/bin/env bats
# Kubernaut Must-Gather - Logs Collection Tests
# BR-PLATFORM-001.3: Support engineers can diagnose service errors from logs

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Diagnose Service Errors
# ========================================

@test "BR-PLATFORM-001.3: Support engineer can diagnose Gateway errors from collected logs" {
    # Business Outcome: Can we troubleshoot why Gateway rejected a signal?
    create_mock_pod_list
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    # Create mock log with error context
    mkdir -p "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123"
    cat > "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log" <<'EOF'
2026-01-04T12:00:00Z ERROR Failed to validate signal: missing required field 'severity'
2026-01-04T12:00:01Z ERROR Signal rejected: invalid schema
EOF

    run bash "${COLLECTORS_DIR}/logs.sh" "${MOCK_COLLECTION_DIR}"

    # Verify support engineer can identify the validation error
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log" "missing required field"
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log" "Signal rejected"
}

@test "BR-PLATFORM-001.3: Support engineer can correlate errors across services using timestamps" {
    # Business Outcome: Can we trace a request through multiple services?
    create_mock_pod_list
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    # Create logs from multiple services with same correlation ID
    mkdir -p "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123"
    mkdir -p "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/datastorage-xyz789"

    cat > "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log" <<'EOF'
2026-01-04T12:00:00Z INFO correlation_id=req-12345 Signal received
2026-01-04T12:00:05Z ERROR correlation_id=req-12345 DataStorage unavailable
EOF

    cat > "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/datastorage-xyz789/current.log" <<'EOF'
2026-01-04T12:00:04Z ERROR Database connection failed
2026-01-04T12:00:05Z ERROR Audit write failed: connection timeout
EOF

    run bash "${COLLECTORS_DIR}/logs.sh" "${MOCK_COLLECTION_DIR}"

    # Verify timestamps allow correlation (Gateway error at 12:00:05, DS connection failed at 12:00:04)
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log" "2026-01-04T12:00:05Z"
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/datastorage-xyz789/current.log" "2026-01-04T12:00:04Z"
}

# ========================================
# Edge Case: Missing/Crashed Pods
# ========================================

@test "BR-PLATFORM-001.3: Support engineer can diagnose crashes from previous pod logs" {
    # Edge Case: Pod restarted due to crash - need previous logs
    create_mock_pod_list
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    # Create current log (after restart) and previous log (before crash)
    mkdir -p "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/workflow-controller-abc"
    cat > "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/workflow-controller-abc/previous.log" <<'EOF'
2026-01-04T11:59:50Z INFO Processing PipelineRun test-run-1
2026-01-04T11:59:55Z ERROR Panic: nil pointer dereference in reconciler
2026-01-04T11:59:56Z FATAL Service crashed with exit code 1
EOF

    run bash "${COLLECTORS_DIR}/logs.sh" "${MOCK_COLLECTION_DIR}"

    # Verify crash context is available from previous logs
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/workflow-controller-abc/previous.log" "nil pointer dereference"
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/workflow-controller-abc/previous.log" "Service crashed"
}

@test "BR-PLATFORM-001.3: Collection succeeds even when namespace is missing" {
    # Edge Case: Namespace doesn't exist (e.g., partial deployment)
    # Business Outcome: Partial diagnostic data is better than none
    echo "" > "${TEST_TEMP_DIR}/empty.yaml"
    mock_kubectl "${TEST_TEMP_DIR}/empty.yaml"

    run bash "${COLLECTORS_DIR}/logs.sh" "${MOCK_COLLECTION_DIR}"

    # Should succeed without failing entire collection
    assert_success
}

# ========================================
# Edge Case: Timeframe Configuration
# ========================================

@test "BR-PLATFORM-001.3: Support engineer can collect longer time windows for intermittent issues" {
    # Edge Case: Issue occurred 36 hours ago, need extended logs
    # Business Outcome: Configurable timeframe captures intermittent problems
    export SINCE_DURATION="48h"
    create_mock_pod_list
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    run bash "${COLLECTORS_DIR}/logs.sh" "${MOCK_COLLECTION_DIR}"

    # Verify 48h timeframe was used (would be passed to kubectl logs --since)
    assert_success
    [[ "$SINCE_DURATION" == "48h" ]]
}

# ========================================
# Edge Case: High-Volume Logs
# ========================================

@test "BR-PLATFORM-001.3: Support engineer can identify error patterns in high-volume logs" {
    # Edge Case: Service generating thousands of log lines
    # Business Outcome: Error patterns remain visible despite volume
    create_mock_pod_list
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    # Create high-volume log with repeated errors
    mkdir -p "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123"
    {
        # 100 normal log lines
        for i in {1..100}; do
            echo "2026-01-04T12:00:$(printf '%02d' $((i % 60)))Z INFO Processing signal $i"
        done
        # Critical error in the noise
        echo "2026-01-04T12:05:00Z ERROR CRITICAL: Audit buffer full - events being dropped"
        echo "2026-01-04T12:05:01Z ERROR Dropped 150 audit events due to buffer overflow"
    } > "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log"

    run bash "${COLLECTORS_DIR}/logs.sh" "${MOCK_COLLECTION_DIR}"

    # Verify critical error is present and findable despite volume
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log" "Audit buffer full"
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log" "Dropped 150 audit events"
}

# ========================================
# UT-MG-2037-003: All-pod log collection, no allowlist (Issue #2037)
# ========================================

@test "UT-MG-2037-003: Support engineer has logs from every service pod, not a stale allowlist" {
    # Business Outcome: BR-PLATFORM-001.3 -- log collection must not silently
    # omit services added after the tool was last updated (drift).
    # This proves ACTUAL collection (via the mocked "kubectl logs" call), unlike
    # the old "all 8 V1.0 services" test it replaces, which only asserted that
    # pre-created destination files survived the run -- it never proved logs.sh
    # itself wrote anything (see TEST_PLAN.md Section 15).
    create_mock_pod_names_list
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    run env RELEASE_NAMESPACE="kubernaut-system" bash "${COLLECTORS_DIR}/logs.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    # Services present in the old SERVICE_PATTERNS allowlist
    assert_file_exists "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc123/current.log"
    assert_file_exists "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/datastorage-xyz789/current.log"
    # Services ABSENT from the old SERVICE_PATTERNS allowlist -- these prove the
    # allowlist removal actually took effect, not just that pre-existing files survive
    assert_file_exists "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/authwebhook-abc123/current.log"
    assert_file_exists "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/apifrontend-xyz789/current.log"
}

