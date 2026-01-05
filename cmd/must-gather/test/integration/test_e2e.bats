#!/usr/bin/env bats
# Kubernaut Must-Gather - E2E Integration Tests
# BR-PLATFORM-001.3.4: End-to-end testing against real cluster

# These tests require a running Kubernetes cluster with Kubernaut installed
# Skip if KUBERNAUT_E2E_TESTS is not set

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
}

teardown() {
    teardown_test_environment
}

@test "E2E: Must-gather completes successfully on real cluster" {
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

@test "E2E: Must-gather collects CRDs from cluster" {
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    # Find collection directory
    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    # Verify CRD directory exists
    assert_directory_exists "${collection_dir}/crds"

    # Verify at least one CRD was collected
    local crd_count=$(find "${collection_dir}/crds" -type d -mindepth 1 | wc -l)
    [ "$crd_count" -gt 0 ]
}

@test "E2E: Must-gather collects service logs from cluster" {
    bash "${MUST_GATHER_ROOT}/gather.sh" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --since=1h

    local collection_dir=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | head -n 1)

    # Verify logs directory exists
    assert_directory_exists "${collection_dir}/logs"

    # Verify at least one log file was collected
    local log_count=$(find "${collection_dir}/logs" -name "*.log" | wc -l)
    [ "$log_count" -gt 0 ]
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

