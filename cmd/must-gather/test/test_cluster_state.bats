#!/usr/bin/env bats
# Kubernaut Must-Gather - Cluster State Collection Tests
# BR-PLATFORM-001.6: Collect RBAC, Storage, Network resources
#
# Issue #2196: cluster-state.sh's per-namespace RBAC/Storage/Network loops
# iterate over KUBERNAUT_NAMESPACES. gather.sh used to export that as a bash
# array, but cluster-state.sh runs as a separate `bash cluster-state.sh`
# subprocess -- bash cannot export array variables across that boundary
# (only scalar strings), so the loops silently iterated zero times in every
# real install. These tests invoke the collector via `run bash ...` (a
# genuine subprocess), so they fail against the old array-export contract.

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Diagnose RBAC/Storage/Network Issues Per Namespace
# ========================================

@test "UT-MG-2196-005: Support engineer gets per-namespace RBAC/storage/network files across a real subprocess boundary" {
    # Business Outcome: cluster-state.sh must actually collect each
    # configured namespace's ServiceAccounts, PVCs, Services, and
    # NetworkPolicies when run the way gather.sh runs it (as a subprocess).
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    run env KUBERNAUT_NAMESPACES_CSV="kubernaut-system,kubernaut-workflows" \
        bash "${COLLECTORS_DIR}/cluster-state.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/cluster-scoped/rbac/serviceaccounts-kubernaut-system.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/cluster-scoped/rbac/serviceaccounts-kubernaut-workflows.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/cluster-scoped/storage/pvc-kubernaut-system.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/cluster-scoped/storage/pvc-kubernaut-workflows.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/cluster-scoped/network/services-kubernaut-system.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/cluster-scoped/network/services-kubernaut-workflows.yaml"
}

@test "UT-MG-2196-006: Collection falls back to RELEASE_NAMESPACE/WORKFLOW_NAMESPACE when KUBERNAUT_NAMESPACES_CSV is unset" {
    # Edge Case: standalone/direct-debug invocation (no gather.sh CSV export)
    # must still collect the two default namespaces, not zero.
    mock_kubectl "${TEST_TEMP_DIR}/pod-list.yaml"

    run env RELEASE_NAMESPACE="kubernaut-system" WORKFLOW_NAMESPACE="kubernaut-workflows" \
        bash "${COLLECTORS_DIR}/cluster-state.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/cluster-scoped/rbac/serviceaccounts-kubernaut-system.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/cluster-scoped/rbac/serviceaccounts-kubernaut-workflows.yaml"
}
