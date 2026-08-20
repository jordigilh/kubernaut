#!/usr/bin/env bats
# Kubernaut Must-Gather - Metrics Collection Tests
# BR-PLATFORM-001.6c: Collect Prometheus metrics snapshots
#
# Issue #2196: metrics.sh's ServiceMonitor/`kubectl top pods` per-namespace
# loops iterate over KUBERNAUT_NAMESPACES. gather.sh used to export that as a
# bash array, but metrics.sh runs as a separate `bash metrics.sh` subprocess
# -- bash cannot export array variables across that boundary (only scalar
# strings), so the loops silently iterated zero times in every real install.
# These tests invoke the collector via `run bash ...` (a genuine subprocess),
# so they fail against the old array-export contract.

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Diagnose Resource/Metrics Issues Per Namespace
# ========================================

@test "UT-MG-2196-003: Support engineer gets per-namespace ServiceMonitor + resource usage files across a real subprocess boundary" {
    # Business Outcome: metrics.sh must actually query each configured
    # namespace's ServiceMonitors and pod resource usage when run the way
    # gather.sh runs it (as a subprocess).
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    run env KUBERNAUT_NAMESPACES_CSV="kubernaut-system,kubernaut-workflows" \
        bash "${COLLECTORS_DIR}/metrics.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/metrics/servicemonitor-kubernaut-system.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/metrics/servicemonitor-kubernaut-workflows.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/metrics/pods-resource-usage-kubernaut-system.txt"
    assert_file_exists "${MOCK_COLLECTION_DIR}/metrics/pods-resource-usage-kubernaut-workflows.txt"
}

@test "UT-MG-2196-004: Collection falls back to RELEASE_NAMESPACE/WORKFLOW_NAMESPACE when KUBERNAUT_NAMESPACES_CSV is unset" {
    # Edge Case: standalone/direct-debug invocation (no gather.sh CSV export)
    # must still collect the two default namespaces, not zero.
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    run env RELEASE_NAMESPACE="kubernaut-system" WORKFLOW_NAMESPACE="kubernaut-workflows" \
        bash "${COLLECTORS_DIR}/metrics.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/metrics/servicemonitor-kubernaut-system.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/metrics/servicemonitor-kubernaut-workflows.yaml"
}
