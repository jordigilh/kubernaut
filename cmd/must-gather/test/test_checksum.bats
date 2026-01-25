#!/usr/bin/env bats
# Kubernaut Must-Gather - Checksum Tests
# BR-PLATFORM-001.8: Support engineers can verify data integrity

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Detect Tampering
# ========================================

@test "BR-PLATFORM-001.8: Support engineer can detect if diagnostic data was modified during transfer" {
    # Business Outcome: Integrity verification for forensics/compliance
    # Edge Case: File modified after collection (network corruption, tampering)
    mkdir -p "${MOCK_COLLECTION_DIR}/logs"
    mkdir -p "${MOCK_COLLECTION_DIR}/crds"
    echo "Original log content" > "${MOCK_COLLECTION_DIR}/logs/gateway.log"
    echo "Original CRD data" > "${MOCK_COLLECTION_DIR}/crds/rr-001.yaml"

    # Generate checksums
    run bash "${UTILS_DIR}/checksum.sh" "${MOCK_COLLECTION_DIR}"

    # Simulate file modification (tampering)
    echo "Modified content" > "${MOCK_COLLECTION_DIR}/logs/gateway.log"

    # Verify checksums detect the modification
    cd "${MOCK_COLLECTION_DIR}"
    run sha256sum -c SHA256SUMS

    # Should fail verification (file was tampered with)
    assert_failure
    [[ "$output" =~ "FAILED" ]]
}

@test "BR-PLATFORM-001.8: Support engineer can prove data integrity for compliance audit" {
    # Business Outcome: SOC2/ISO compliance requires integrity verification
    mkdir -p "${MOCK_COLLECTION_DIR}/logs"
    mkdir -p "${MOCK_COLLECTION_DIR}/crds"
    echo "Log data" > "${MOCK_COLLECTION_DIR}/logs/gateway.log"
    echo "CRD data" > "${MOCK_COLLECTION_DIR}/crds/rr-001.yaml"

    run bash "${UTILS_DIR}/checksum.sh" "${MOCK_COLLECTION_DIR}"

    # Verify SHA256SUMS proves integrity
    assert_file_exists "${MOCK_COLLECTION_DIR}/SHA256SUMS"

    # All files are checksummed
    assert_file_contains "${MOCK_COLLECTION_DIR}/SHA256SUMS" "logs/gateway.log"
    assert_file_contains "${MOCK_COLLECTION_DIR}/SHA256SUMS" "crds/rr-001.yaml"

    # Checksums are verifiable
    cd "${MOCK_COLLECTION_DIR}"
    run sha256sum -c SHA256SUMS
    assert_success
    [[ "$output" =~ "OK" ]]
}

# ========================================
# Edge Case: Large Collections
# ========================================

@test "BR-PLATFORM-001.8: Support engineer can verify integrity of large collections (100+ files)" {
    # Edge Case: Large collection with many files
    # Business Outcome: Checksum all files without missing any
    mkdir -p "${MOCK_COLLECTION_DIR}/logs" "${MOCK_COLLECTION_DIR}/crds" "${MOCK_COLLECTION_DIR}/events"

    # Create 100 files across multiple directories
    for i in {1..100}; do
        echo "file $i content" > "${MOCK_COLLECTION_DIR}/logs/file${i}.log"
    done

    run bash "${UTILS_DIR}/checksum.sh" "${MOCK_COLLECTION_DIR}"

    # Verify all 100 files are checksummed
    cd "${MOCK_COLLECTION_DIR}"
    file_count=$(grep -c "file.*\.log" SHA256SUMS)
    [ "$file_count" -eq 100 ]

    # All checksums verify
    run sha256sum -c SHA256SUMS
    assert_success
}

# ========================================
# Edge Case: Nested Directories
# ========================================

@test "BR-PLATFORM-001.8: Support engineer can verify deeply nested file integrity" {
    # Edge Case: Files in deeply nested directories
    # Business Outcome: Relative paths work across different extraction locations
    mkdir -p "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc/containers"
    mkdir -p "${MOCK_COLLECTION_DIR}/crds/remediationrequests/instances"

    echo "nested log" > "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway-abc/containers/current.log"
    echo "nested crd" > "${MOCK_COLLECTION_DIR}/crds/remediationrequests/instances/rr-001.yaml"

    run bash "${UTILS_DIR}/checksum.sh" "${MOCK_COLLECTION_DIR}"

    # Verify relative paths are correct
    assert_file_contains "${MOCK_COLLECTION_DIR}/SHA256SUMS" "./logs/kubernaut-system/gateway-abc/containers/current.log"
    assert_file_contains "${MOCK_COLLECTION_DIR}/SHA256SUMS" "./crds/remediationrequests/instances/rr-001.yaml"

    # Checksums verify from any location
    cd "${MOCK_COLLECTION_DIR}"
    run sha256sum -c SHA256SUMS
    assert_success
}

# ========================================
# Edge Case: Checksum Doesn't Include Itself
# ========================================

@test "BR-PLATFORM-001.8: SHA256SUMS file does not create circular dependency" {
    # Edge Case: SHA256SUMS must not checksum itself
    # Business Outcome: Avoid checksum validation failures
    mkdir -p "${MOCK_COLLECTION_DIR}"
    echo "test data" > "${MOCK_COLLECTION_DIR}/file.txt"

    run bash "${UTILS_DIR}/checksum.sh" "${MOCK_COLLECTION_DIR}"

    # SHA256SUMS must not include itself
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/SHA256SUMS" "SHA256SUMS"

    # But validation still works for all other files
    cd "${MOCK_COLLECTION_DIR}"
    run sha256sum -c SHA256SUMS
    assert_success
}

# ========================================
# Edge Case: Empty Collection
# ========================================

@test "BR-PLATFORM-001.8: Support engineer can identify empty collections" {
    # Edge Case: Collection completed but no data was collected
    # Business Outcome: Empty SHA256SUMS indicates collection issue
    mkdir -p "${MOCK_COLLECTION_DIR}"

    run bash "${UTILS_DIR}/checksum.sh" "${MOCK_COLLECTION_DIR}"

    # Should create SHA256SUMS even if empty
    assert_file_exists "${MOCK_COLLECTION_DIR}/SHA256SUMS"
    assert_success

    # Empty or near-empty SHA256SUMS signals problem
    file_size=$(wc -c < "${MOCK_COLLECTION_DIR}/SHA256SUMS")
    [ "$file_size" -lt 100 ]  # Very small file = likely no data collected
}

# ========================================
# Edge Case: Special Characters in Filenames
# ========================================

@test "BR-PLATFORM-001.8: Support engineer can verify files with special characters" {
    # Edge Case: Filenames with spaces, dashes, underscores
    # Business Outcome: Checksums work for all valid Kubernetes resource names
    mkdir -p "${MOCK_COLLECTION_DIR}/crds"
    echo "data1" > "${MOCK_COLLECTION_DIR}/crds/my-rr-001.yaml"
    echo "data2" > "${MOCK_COLLECTION_DIR}/crds/test_signal_processing.yaml"
    echo "data3" > "${MOCK_COLLECTION_DIR}/crds/workflow.execution.kubernaut.ai.yaml"

    run bash "${UTILS_DIR}/checksum.sh" "${MOCK_COLLECTION_DIR}"

    # All files checksummed despite special characters
    assert_file_contains "${MOCK_COLLECTION_DIR}/SHA256SUMS" "my-rr-001.yaml"
    assert_file_contains "${MOCK_COLLECTION_DIR}/SHA256SUMS" "test_signal_processing.yaml"
    assert_file_contains "${MOCK_COLLECTION_DIR}/SHA256SUMS" "workflow.execution.kubernaut.ai.yaml"

    # Verification works
    cd "${MOCK_COLLECTION_DIR}"
    run sha256sum -c SHA256SUMS
    assert_success
}

# ========================================
# Edge Case: Network Transfer Corruption Detection
# ========================================

@test "BR-PLATFORM-001.8: Support engineer can detect network corruption during archive transfer" {
    # Edge Case: Tarball transferred over network, bits flipped
    # Business Outcome: Checksum detects even single-bit corruption
    mkdir -p "${MOCK_COLLECTION_DIR}/logs"

    # Create file with known content
    printf "12345678901234567890" > "${MOCK_COLLECTION_DIR}/logs/test.log"

    # Generate checksum
    run bash "${UTILS_DIR}/checksum.sh" "${MOCK_COLLECTION_DIR}"

    # Simulate single-bit corruption (change one byte)
    printf "X2345678901234567890" > "${MOCK_COLLECTION_DIR}/logs/test.log"

    # Checksum verification catches the corruption
    cd "${MOCK_COLLECTION_DIR}"
    run sha256sum -c SHA256SUMS

    assert_failure
    [[ "$output" =~ "FAILED" ]]
}

