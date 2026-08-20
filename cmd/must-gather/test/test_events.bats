#!/usr/bin/env bats
# Kubernaut Must-Gather - Kubernetes Events Collection Tests
# BR-PLATFORM-001.4: Collect Kubernetes events for troubleshooting context
#
# Issue #2196: events.sh's per-namespace loop iterates over
# KUBERNAUT_NAMESPACES. gather.sh used to export that as a bash array, but
# events.sh runs as a separate `bash events.sh` subprocess -- bash cannot
# export array variables across that boundary (only scalar strings), so the
# loop silently iterated zero times in every real install. These tests
# invoke the collector via `run bash ...` (a genuine subprocess, matching how
# gather.sh actually calls it), not a same-process function call, so they
# fail against the old array-export contract.

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Diagnose Failures via Per-Namespace Events
# ========================================

@test "UT-MG-2196-001: Support engineer gets per-namespace event files across a real subprocess boundary" {
    # Business Outcome: events.sh must actually iterate the configured
    # namespaces when run the way gather.sh runs it (as a subprocess), not
    # just when a test calls its loop logic directly in the same process.
    create_mock_events
    mock_kubectl "${TEST_TEMP_DIR}/events.yaml"

    run env KUBERNAUT_NAMESPACES_CSV="kubernaut-system,kubernaut-workflows" \
        bash "${COLLECTORS_DIR}/events.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/events/events-kubernaut-system.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/events/events-kubernaut-system.json"
    assert_file_exists "${MOCK_COLLECTION_DIR}/events/events-kubernaut-workflows.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/events/events-kubernaut-workflows.json"
}

@test "UT-MG-2196-002: Collection falls back to RELEASE_NAMESPACE/WORKFLOW_NAMESPACE when KUBERNAUT_NAMESPACES_CSV is unset" {
    # Edge Case: standalone/direct-debug invocation (no gather.sh CSV export)
    # must still collect the two default namespaces, not zero.
    create_mock_events
    mock_kubectl "${TEST_TEMP_DIR}/events.yaml"

    run env RELEASE_NAMESPACE="kubernaut-system" WORKFLOW_NAMESPACE="kubernaut-workflows" \
        bash "${COLLECTORS_DIR}/events.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/events/events-kubernaut-system.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/events/events-kubernaut-workflows.yaml"
}
