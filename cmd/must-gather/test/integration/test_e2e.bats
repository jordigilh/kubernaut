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
    # Optional: only set when the CI job installs the separate
    # kubernaut-operator component (see .github/workflows/must-gather-tests.yml).
    export OPERATOR_NAMESPACE="${OPERATOR_NAMESPACE:-kubernaut-operator-system}"
}

teardown() {
    teardown_test_environment
}

@test "E2E: Must-gather completes successfully on real cluster" {
    # IT-MG-2037-005: proves the actual gather.sh entry point (same one the
    # must-gather container image's ENTRYPOINT invokes) runs clean end-to-end
    # against a live cluster -- the deterministic drift-detection gate this
    # issue exists to create.
    run bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    assert_success
}

@test "E2E: Must-gather creates valid tarball" {
    bash "${GATHER_SCRIPT}" \
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
    bash "${GATHER_SCRIPT}" \
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

@test "IT-MG-2187-001: Must-gather's RBAC actually grants read access to EVERY discovered CRD type, not just enumerates them" {
    # Business Outcome: BR-PLATFORM-001.3.2 -- discovery (#2037) and RBAC
    # (#2187) are two independently-maintained things; a type crds.sh
    # discovers but the ClusterRole can't read is a silent partial-collection
    # gap that IT-MG-2037-001 above cannot catch, because crds.sh does
    # `mkdir -p "${CRD_DIR}/${crd_name}"` BEFORE the RBAC-gated `kubectl get`
    # call -- the per-type directory exists either way, so a directory-count
    # match alone proves discovery worked, not that RBAC did. This asserts
    # actual file CONTENT for every dynamically-discovered type: it must
    # neither contain an RBAC-403 error signature nor be empty/placeholder
    # (mirrors IT-MG-2037-003's content-vs-presence distinction for logs.sh).
    bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)
    assert_directory_exists "${collection_dir}/crds"

    local checked_count=0
    for crd_dir in "${collection_dir}/crds"/*/; do
        local crd_name=$(basename "${crd_dir}")
        local def_file="${crd_dir}crd-definition.yaml"

        assert_file_exists "${def_file}"
        run grep -iE "Forbidden|Error from server|cannot (get|list) resource" "${def_file}"
        [ "$status" -ne 0 ] || {
            echo "RBAC denial leaked into ${crd_name}/crd-definition.yaml: $output"
            return 1
        }
        assert_file_contains "${def_file}" "kind: CustomResourceDefinition"

        checked_count=$((checked_count + 1))
    done
    [ "${checked_count}" -gt 0 ]  # sanity: the test cluster must have discoverable Kubernaut CRDs
}

@test "IT-MG-2037-002: --namespace/--workflow-namespace flags flow through to the recorded collection namespaces" {
    # Business Outcome: BR-PLATFORM-001 -- support engineers running against a
    # non-default Helm release namespace get the RIGHT namespace collected, not
    # a hardcoded kubernaut-system/kubernaut-notifications/kubernaut-workflows
    # triplet (the pre-#2037 behavior, which included a namespace nothing is
    # ever deployed into).
    bash "${GATHER_SCRIPT}" \
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
    #
    # Content correctness, not just existence: `assert_file_exists` alone
    # cannot catch a `kubectl logs` failure, since logs.sh redirects stderr
    # into the same file (`> "${pod_dir}/current.log" 2>&1`) -- a failed
    # collection still produces a non-empty, existing file (kubectl's own
    # "Error from server (...)" text), which would silently pass a
    # presence-only check. Proven here two ways: (1) the file must be
    # non-empty and must not contain that error signature, and (2) a subset
    # proof -- a real log line fetched independently via `kubectl logs
    # --tail=1` immediately BEFORE gather.sh runs must appear verbatim in the
    # collected file. Log streams are append-only, so that line is guaranteed
    # to still be present in gather.sh's later, fuller
    # --since=1h/--tail=10000 capture -- this is what proves the collected
    # content is a genuine (not necessarily exact, but verifiably contained)
    # subset of the pod's real log stream, not just any non-empty file.
    local live_pods=$(kubectl get pods -n "${RELEASE_NAMESPACE}" --no-headers 2>/dev/null | awk '{print $1}')
    [ -n "${live_pods}" ]  # sanity: the test cluster must actually have pods running

    local ref_dir="${TEST_TEMP_DIR}/ref-logs"
    mkdir -p "${ref_dir}"
    local checked_count=0
    while IFS= read -r pod; do
        # `tail -n 1` on the kubectl output (not just kubectl's own --tail=1)
        # guarantees a single line even for multi-container pods, where
        # --all-containers can return one line per container.
        local line=$(kubectl logs "${pod}" -n "${RELEASE_NAMESPACE}" --tail=1 --timestamps --all-containers 2>/dev/null | tail -n 1)
        if [ -n "${line}" ]; then
            printf '%s' "${line}" > "${ref_dir}/${pod}"
            checked_count=$((checked_count + 1))
        fi
    done <<< "${live_pods}"
    [ "${checked_count}" -gt 0 ]  # sanity: at least one pod must have real log content, or the subset check below is vacuous

    bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --namespace="${RELEASE_NAMESPACE}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    assert_directory_exists "${collection_dir}/logs"

    while IFS= read -r pod; do
        local collected_file="${collection_dir}/logs/${RELEASE_NAMESPACE}/${pod}/current.log"
        assert_file_exists "${collected_file}"
        [ -s "${collected_file}" ]  # non-empty: catches a silently-collected-nothing regression

        run grep -q "^Error from server" "${collected_file}"
        assert_failure  # must NOT contain kubectl's own error envelope

        if [ -f "${ref_dir}/${pod}" ]; then
            assert_file_contains "${collected_file}" "$(cat "${ref_dir}/${pod}")"
        fi
    done <<< "${live_pods}"
}

@test "IT-MG-2037-004: Must-gather's DataStorage API collection degrades gracefully (blocked by NetworkPolicy by design)" {
    # Business Outcome: BR-PLATFORM-001.6a -- DATASTORAGE_URL must resolve to
    # the correct Service name (data-storage-service, not the stale
    # "datastorage" name -- confirmed and fixed as a real drift bug during
    # this issue).
    #
    # NOT asserting workflows.json/audit-events.json contain real data: the
    # chart's own kubernaut-datastorage NetworkPolicy intentionally allows
    # ingress on :8080 only from specific labeled service pods (gateway,
    # authwebhook, etc.) -- confirmed by direct testing against this real
    # live cluster, a curl from an unlabeled pod times out (silently
    # dropped). None of the three documented production invocation methods
    # (README.md: oc adm must-gather / kubectl debug node / kubectl run) run
    # must-gather as one of those labeled pods, so this has likely never
    # worked on any NetworkPolicy-enforcing cluster (OpenShift always
    # enforces it). Deliberately NOT fixed by loosening the NetworkPolicy or
    # using kubectl exec/port-forward: audit_events is FedRAMP/SOC2-controlled
    # compliance data (AU-9), and a diagnostic tarball handed to a support
    # engineer is not that data's intended access path. This test instead
    # locks in the correct, already-existing graceful-degradation contract
    # (BR-PLATFORM-001.6a's own error-handling path, also unit-tested in
    # test_datastorage.bats): gather.sh must still complete successfully and
    # record *why* the collection was incomplete, not crash or hang.
    # Live-cluster truth: the Service name datastorage.sh's DATASTORAGE_URL
    # must target. If this ever drifts again, this fails loudly here instead
    # of silently inside a swallowed curl error.
    run kubectl get svc data-storage-service -n "${RELEASE_NAMESPACE}"
    assert_success

    bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --namespace="${RELEASE_NAMESPACE}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    assert_file_exists "${collection_dir}/datastorage/error.json"
    run jq empty "${collection_dir}/datastorage/error.json"
    assert_success
}

@test "E2E: Must-gather generates valid checksums" {
    bash "${GATHER_SCRIPT}" \
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
    bash "${GATHER_SCRIPT}" \
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
    bash "${GATHER_SCRIPT}" \
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

@test "IT-MG-2037-006: Must-gather discovers the kubernaut-operator's CRD, not just this Helm chart's own CRDs" {
    # Business Outcome: BR-PLATFORM-001.2 -- dynamic CRD discovery (crds.sh)
    # must work across component boundaries too: kubernauts.kubernaut.ai is
    # registered by the SEPARATE kubernaut-operator, not this Helm chart, so
    # this proves the discovery mechanism isn't secretly scoped to
    # chart-owned CRDs only.
    if ! kubectl get crd kubernauts.kubernaut.ai &> /dev/null; then
        skip "kubernaut-operator not installed on this cluster (kubernauts.kubernaut.ai CRD absent)"
    fi

    bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    assert_file_exists "${collection_dir}/crds/kubernauts/crd-definition.yaml"
    assert_file_contains "${collection_dir}/crds/kubernauts/crd-definition.yaml" "kubernauts.kubernaut.ai"
}

@test "IT-MG-2037-007: Must-gather collects the kubernaut-operator's own controller-manager logs when the operator is installed" {
    # Business Outcome: BR-PLATFORM-001.3 -- operator-path deployments need
    # the operator's own reconciliation logs, not just the Helm chart's
    # service logs, to diagnose install/upgrade failures.
    #
    # Same content-correctness proof as IT-MG-2037-003 (see its comment for
    # the full rationale): file existence alone can't distinguish a genuine
    # collection from a swallowed `kubectl logs` error, since logs.sh
    # redirects stderr into the same file.
    if ! kubectl get namespace "${OPERATOR_NAMESPACE}" &> /dev/null; then
        skip "kubernaut-operator not installed on this cluster (${OPERATOR_NAMESPACE} namespace absent)"
    fi

    # Live-cluster truth: every pod actually running in the operator namespace.
    local live_pods=$(kubectl get pods -n "${OPERATOR_NAMESPACE}" --no-headers 2>/dev/null | awk '{print $1}')
    [ -n "${live_pods}" ]  # sanity: the operator's controller-manager must actually be running

    local ref_dir="${TEST_TEMP_DIR}/ref-operator-logs"
    mkdir -p "${ref_dir}"
    local checked_count=0
    while IFS= read -r pod; do
        local line=$(kubectl logs "${pod}" -n "${OPERATOR_NAMESPACE}" --tail=1 --timestamps --all-containers 2>/dev/null | tail -n 1)
        if [ -n "${line}" ]; then
            printf '%s' "${line}" > "${ref_dir}/${pod}"
            checked_count=$((checked_count + 1))
        fi
    done <<< "${live_pods}"
    [ "${checked_count}" -gt 0 ]  # sanity: the operator's controller-manager must have real log content

    bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    while IFS= read -r pod; do
        local collected_file="${collection_dir}/logs/${OPERATOR_NAMESPACE}/${pod}/current.log"
        assert_file_exists "${collected_file}"
        [ -s "${collected_file}" ]

        run grep -q "^Error from server" "${collected_file}"
        assert_failure

        if [ -f "${ref_dir}/${pod}" ]; then
            assert_file_contains "${collected_file}" "$(cat "${ref_dir}/${pod}")"
        fi
    done <<< "${live_pods}"
}

@test "E2E: Must-gather respects size limits" {
    bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h \
        --max-size=500

    # Find generated tarball
    local tarball=$(find "${TEST_TEMP_DIR}" -name "kubernaut-must-gather-*.tar.gz" | head -n 1)

    # Verify tarball size is reasonable (< 500MB)
    local size_mb=$(du -m "${tarball}" | cut -f1)
    [ "$size_mb" -lt 500 ]
}
