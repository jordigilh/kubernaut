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

# ========================================
# UT-MG-2187-*: RBAC allowlist completeness (Issue #2187)
# ========================================

@test "UT-MG-2187-001: ClusterRole's static kubernaut.ai allowlist has an entry for every CRD shipped in the Helm chart" {
    # Business Outcome: BR-PLATFORM-001.3.2 -- crds.sh discovers CRD types
    # dynamically (#2037), but RBAC is still a hand-maintained allowlist. A
    # type discoverable-but-not-readable is a silent partial-collection gap
    # (kubectl 403s on the instances/definition call even though the type was
    # enumerated). Source of truth is the CRD manifests actually shipped in
    # the chart (charts/kubernaut/crds/kubernaut.ai_*.yaml), not a
    # hand-maintained fixture list here -- so this test itself can't rot the
    # same way the allowlist it's checking did.
    local chart_crd_dir="${MUST_GATHER_ROOT}/../../charts/kubernaut/crds"
    local clusterrole="${MUST_GATHER_ROOT}/templates/clusterrole.yaml"

    [ -d "${chart_crd_dir}" ] || skip "chart CRD directory not found: ${chart_crd_dir}"

    local missing=""
    for crd_file in "${chart_crd_dir}"/kubernaut.ai_*.yaml; do
        local plural
        plural=$(basename "${crd_file}" .yaml | sed 's/^kubernaut\.ai_//')
        if ! grep -qE "^\s+- ${plural}\$" "${clusterrole}"; then
            missing="${missing} ${plural}"
        fi
    done

    if [ -n "${missing}" ]; then
        echo "ClusterRole's kubernaut.ai allowlist is missing:${missing}"
        return 1
    fi
}

@test "UT-MG-2187-002: Support engineer can extract AgentSession state for analysis" {
    # Business Outcome: BR-PLATFORM-001.2 -- once agentsessions.kubernaut.ai
    # is both discoverable (crds.sh, #2037) and readable (clusterrole.yaml,
    # #2187), a seeded AgentSession instance must actually show up in the
    # collected archive, end-to-end through the same collector RemediationRequest
    # already exercises above.
    cat > "${TEST_TEMP_DIR}/crd-list.txt" <<'EOF'
customresourcedefinition.apiextensions.k8s.io/agentsessions.kubernaut.ai
EOF
    create_mock_crd_response
    mock_kubectl "${TEST_TEMP_DIR}/crd-response.yaml"

    cat > "${TEST_TEMP_DIR}/crd-instances.yaml" <<'EOF'
apiVersion: v1
kind: List
items:
  - apiVersion: kubernaut.ai/v1alpha1
    kind: AgentSession
    metadata:
      name: test-as-001
      namespace: kubernaut-system
    spec:
      remediationRequestRef:
        name: test-rr-001
    status:
      phase: "Completed"
EOF

    run env PATH="${TEST_TEMP_DIR}/bin:${PATH}" bash "${COLLECTORS_DIR}/crds.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_contains "${MOCK_COLLECTION_DIR}/crds/agentsessions/all-instances.yaml" "test-as-001"
    assert_file_contains "${MOCK_COLLECTION_DIR}/crds/agentsessions/all-instances.yaml" "kind: AgentSession"
}

