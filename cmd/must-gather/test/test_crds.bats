#!/usr/bin/env bats
# Kubernaut Must-Gather - CRD Collection Tests
# BR-PLATFORM-001.2: Support engineers can troubleshoot CRD state

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

@test "BR-PLATFORM-001.2: Support engineer can extract RemediationRequest state for analysis" {
    # Business Outcome: Can we troubleshoot a specific RemediationRequest?
    create_mock_crd_response
    mock_kubectl "${TEST_TEMP_DIR}/crd-response.yaml"

    run env PATH="${TEST_TEMP_DIR}/bin:${PATH}" bash "${COLLECTORS_DIR}/crds.sh" "${MOCK_COLLECTION_DIR}"

    # Verify the collected data contains troubleshooting information
    assert_file_contains "${MOCK_COLLECTION_DIR}/crds/remediationrequests/all-instances.yaml" "test-rr-001"
    assert_file_contains "${MOCK_COLLECTION_DIR}/crds/remediationrequests/all-instances.yaml" "signal_id: \"test-signal\""
    assert_file_contains "${MOCK_COLLECTION_DIR}/crds/remediationrequests/all-instances.yaml" "phase: \"Completed\""
}

@test "BR-PLATFORM-001.2: Collection succeeds even when CRDs are not installed" {
    # Business Outcome: Partial collection is better than total failure
    echo "---" > "${TEST_TEMP_DIR}/empty.yaml"
    mock_kubectl "${TEST_TEMP_DIR}/empty.yaml"

    run bash "${COLLECTORS_DIR}/crds.sh" "${MOCK_COLLECTION_DIR}"

    # Business validation: Collection completes without failing
    assert_success
}

@test "BR-PLATFORM-001.2: Support engineer can inspect CRD schema for version compatibility" {
    # Business Outcome: Can support determine what version of CRD is deployed?
    create_mock_crd_response
    mock_kubectl "${TEST_TEMP_DIR}/crd-response.yaml"

    run env PATH="${TEST_TEMP_DIR}/bin:${PATH}" bash "${COLLECTORS_DIR}/crds.sh" "${MOCK_COLLECTION_DIR}"

    # Verify CRD schema is available for version analysis
    assert_file_contains "${MOCK_COLLECTION_DIR}/crds/remediationrequests/crd-definition.yaml" "kind: CustomResourceDefinition"
    assert_file_contains "${MOCK_COLLECTION_DIR}/crds/remediationrequests/crd-definition.yaml" "group: kubernaut.ai"
}

# ========================================
# UT-MG-2037-001: Dynamic CRD discovery (Issue #2037)
# ========================================

@test "UT-MG-2037-001: Support engineer gets ALL registered kubernaut.ai CRD types, not a stale allowlist" {
    # Business Outcome: BR-PLATFORM-001.2 -- collection must not silently omit
    # CRD types added after the tool was last updated (drift).
    create_mock_crd_response
    create_mock_crd_list
    mock_kubectl "${TEST_TEMP_DIR}/crd-response.yaml"

    run env PATH="${TEST_TEMP_DIR}/bin:${PATH}" bash "${COLLECTORS_DIR}/crds.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    # Types absent from the old static 6-entry CRD_TYPES array must still be collected
    assert_directory_exists "${MOCK_COLLECTION_DIR}/crds/actiontypes"
    assert_directory_exists "${MOCK_COLLECTION_DIR}/crds/effectivenessassessments"
    # A type already in the old list must still be collected (no regression)
    assert_directory_exists "${MOCK_COLLECTION_DIR}/crds/aianalyses"
}

@test "UT-MG-2037-001: Support engineer's collection is not polluted by non-kubernaut CRDs" {
    # Business Outcome: Dynamic discovery must not sweep in unrelated CRDs
    # installed by other operators sharing the cluster (R3).
    create_mock_crd_response
    create_mock_crd_list
    mock_kubectl "${TEST_TEMP_DIR}/crd-response.yaml"

    run env PATH="${TEST_TEMP_DIR}/bin:${PATH}" bash "${COLLECTORS_DIR}/crds.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    [ ! -d "${MOCK_COLLECTION_DIR}/crds/widgets" ]
}

