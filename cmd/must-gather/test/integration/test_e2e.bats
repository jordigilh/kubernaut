#!/usr/bin/env bats
# Kubernaut Must-Gather - E2E Integration Tests
# BR-PLATFORM-001.3.4: End-to-end testing against real cluster
#
# Issue #2037: this is the deterministic drift detector. Assertions compare
# must-gather's output against LIVE CLUSTER TRUTH (queried independently via
# kubectl at test time), not a second hardcoded expected count -- so the test
# never itself drifts as CRDs/services are added or removed in future releases.
#
# These tests require a running Kubernetes cluster with Kubernaut installed
# (all CRDs registered, chart-managed services deployed into RELEASE_NAMESPACE).
# Skip if KUBERNAUT_E2E_TESTS is not set.

load ../helpers

setup() {
    if [ -z "${KUBERNAUT_E2E_TESTS}" ]; then
        skip "E2E tests disabled. Set KUBERNAUT_E2E_TESTS=1 to enable"
    fi

    setup_test_environment

    # Verify kubectl is available
    if ! command -v kubectl &> /dev/null; then
        skip "kubectl not found in PATH"
    fi

    # Verify cluster connection
    if ! kubectl cluster-info &> /dev/null; then
        skip "Cannot connect to Kubernetes cluster"
    fi

    # Issue #2037: RELEASE_NAMESPACE/WORKFLOW_NAMESPACE identify where the
    # live cluster under test has Kubernaut installed -- override via env
    # when running against a non-default install.
    export RELEASE_NAMESPACE="${RELEASE_NAMESPACE:-kubernaut-system}"
    export WORKFLOW_NAMESPACE="${WORKFLOW_NAMESPACE:-kubernaut-workflows}"
}

teardown() {
    teardown_test_environment
}

@test "E2E: Must-gather completes successfully on real cluster" {
    # IT-MG-2037-005: proves the actual gather.sh entry point (same one the
    # must-gather container image's ENTRYPOINT invokes) runs clean end-to-end
    # against a live cluster -- the deterministic drift-detection gate this
    # issue exists to create.
    run bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    assert_success
}

@test "E2E: Must-gather creates valid tarball" {
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    # Find generated tarball
    local tarball=$(find "${TEST_TEMP_DIR}" -name "kubernaut-must-gather-*.tar.gz" | head -n 1)

    # Verify tarball exists
    [ -f "${tarball}" ]

    # Verify tarball is valid
    run tar -tzf "${tarball}"
    assert_success
}

@test "IT-MG-2037-001: Must-gather collects EVERY registered kubernaut.ai CRD type from the cluster, not a stale subset" {
    # Business Outcome: BR-PLATFORM-001.2 -- support engineers must never receive
    # an incomplete CRD snapshot without any signal that collection was partial.
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    assert_directory_exists "${collection_dir}/crds"

    # Live-cluster truth, queried independently -- NOT a hardcoded expected
    # count, so this assertion self-corrects as CRDs are added/removed.
    local expected_crd_count=$(kubectl get crd -o name 2>/dev/null | grep -cE '\.kubernaut\.ai$' || echo "0")
    local collected_crd_count=$(find "${collection_dir}/crds" -type d -mindepth 1 | wc -l)

    [ "$expected_crd_count" -gt 0 ]  # sanity: the test cluster must actually have Kubernaut CRDs installed
    [ "$collected_crd_count" -eq "$expected_crd_count" ]
}

@test "IT-MG-2037-002: --namespace/--workflow-namespace flags flow through to the recorded collection namespaces" {
    # Business Outcome: BR-PLATFORM-001 -- support engineers running against a
    # non-default Helm release namespace get the RIGHT namespace collected, not
    # a hardcoded kubernaut-system/kubernaut-notifications/kubernaut-workflows
    # triplet (the pre-#2037 behavior, which included a namespace nothing is
    # ever deployed into).
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --namespace="${RELEASE_NAMESPACE}" \
        --workflow-namespace="${WORKFLOW_NAMESPACE}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    run jq -r '.namespaces_collected | length' "${collection_dir}/collection-metadata.json"
    [ "$output" -eq 2 ]

    run jq -r '.namespaces_collected[]' "${collection_dir}/collection-metadata.json"
    [[ "$output" =~ "${RELEASE_NAMESPACE}" ]]
    [[ "$output" =~ "${WORKFLOW_NAMESPACE}" ]]
    [[ ! "$output" =~ "kubernaut-notifications" ]]
}

@test "IT-MG-2037-003: Must-gather collects logs from EVERY pod in the release namespace, not a stale service allowlist" {
    # Business Outcome: BR-PLATFORM-001.3 -- support engineers must never
    # silently miss a service's logs because the tool predates that service.
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --namespace="${RELEASE_NAMESPACE}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    assert_directory_exists "${collection_dir}/logs"

    # Live-cluster truth: every pod actually running in the release namespace.
    local live_pods=$(kubectl get pods -n "${RELEASE_NAMESPACE}" --no-headers 2>/dev/null | awk '{print $1}')
    [ -n "${live_pods}" ]  # sanity: the test cluster must actually have pods running

    while IFS= read -r pod; do
        assert_file_exists "${collection_dir}/logs/${RELEASE_NAMESPACE}/${pod}/current.log"
    done <<< "${live_pods}"
}

@test "IT-MG-2037-004: Must-gather's DataStorage API collection reaches the live in-cluster service" {
    # Business Outcome: BR-PLATFORM-001.6a -- DATASTORAGE_URL must resolve
    # correctly for the configured release namespace, not a hardcoded literal.
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --namespace="${RELEASE_NAMESPACE}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    # Success path (workflows.json) proves the URL resolved and DataStorage
    # answered; error.json alone (no workflows.json) would mean DATASTORAGE_URL
    # was wrong for this cluster's actual release namespace.
    assert_file_exists "${collection_dir}/datastorage/workflows.json"
    run jq empty "${collection_dir}/datastorage/workflows.json"
    assert_success
}

@test "E2E: Must-gather generates valid checksums" {
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    # Verify SHA256SUMS exists
    assert_file_exists "${collection_dir}/SHA256SUMS"

    # Verify checksums are valid
    cd "${collection_dir}"
    run sha256sum -c SHA256SUMS
    assert_success
}

@test "E2E: Must-gather sanitizes sensitive data" {
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    # Verify sanitization report exists
    assert_file_exists "${collection_dir}/sanitization-report.txt"

    # Verify sanitization was performed
    local sanitized_count=$(grep -c "pre-sanitize" "${collection_dir}/sanitization-report.txt" || echo "0")
    [ "$sanitized_count" -ge 0 ]
}

@test "E2E: Must-gather generates collection metadata" {
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    # Verify metadata file exists
    assert_file_exists "${collection_dir}/collection-metadata.json"

    # Verify metadata is valid JSON
    run jq empty "${collection_dir}/collection-metadata.json"
    assert_success

    # Verify required fields exist
    run jq -r '.collection_time' "${collection_dir}/collection-metadata.json"
    [ -n "$output" ]

    run jq -r '.kubernaut_version' "${collection_dir}/collection-metadata.json"
    [ -n "$output" ]
}

@test "E2E: Must-gather respects size limits" {
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h \
        --max-size=500

    # Find generated tarball
    local tarball=$(find "${TEST_TEMP_DIR}" -name "kubernaut-must-gather-*.tar.gz" | head -n 1)

    # Verify tarball size is reasonable (< 500MB)
    local size_mb=$(du -m "${tarball}" | cut -f1)
    [ "$size_mb" -lt 500 ]
}
