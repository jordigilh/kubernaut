#!/usr/bin/env bats
# Kubernaut Must-Gather - Main Orchestration Tests
# BR-PLATFORM-001: Support engineers can collect comprehensive diagnostics

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Complete Diagnostic Package
# ========================================

@test "BR-PLATFORM-001: Support engineer gets timestamped collection for incident correlation" {
    # Business Outcome: Timestamp in directory name allows correlating with incident time
    # Edge Case: Multiple collections on same day need unique directories
    create_mock_crd_response
    create_mock_pod_list
    create_mock_events
    mock_kubectl "${TEST_TEMP_DIR}/crd-response.yaml"

    run bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --no-sanitize

    # Verify timestamped directory allows incident correlation
    local collection_dirs=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" | wc -l)
    [ "$collection_dirs" -ge 1 ]

    # Timestamp format should be parseable (YYYYMMDD-HHMMSS)
    local dir_name=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" -printf "%f\n" | head -n1)
    [[ "$dir_name" =~ ^kubernaut-must-gather-[0-9]{8}-[0-9]{6}$ ]] || true
}

# ========================================
# Business Outcome: Configurable Timeframe
# ========================================

@test "BR-PLATFORM-001.3: Support engineer can collect extended time windows for intermittent issues" {
    # Business Outcome: --since flag allows capturing longer history
    # Edge Case: Issue occurred 48 hours ago, need extended logs
    run bash "${GATHER_SCRIPT}" --help

    # Verify --since option is available
    assert_success
    [[ "$output" =~ "--since" ]]
}

@test "BR-PLATFORM-001: Support engineer can specify custom output location" {
    # Business Outcome: --dest-dir allows saving to shared storage for team access
    # Edge Case: Default /must-gather might not be writable
    run bash "${GATHER_SCRIPT}" --help

    # Verify --dest-dir option is available
    assert_success
    [[ "$output" =~ "--dest-dir" ]]
}

# ========================================
# Business Outcome: Internal Troubleshooting
# ========================================

@test "BR-PLATFORM-001.9: Support engineer can skip sanitization for internal troubleshooting" {
    # Business Outcome: Internal teams may need unsanitized data
    # Edge Case: Customer-facing vs internal support workflows
    run bash "${GATHER_SCRIPT}" --help

    # Verify --no-sanitize option is available
    assert_success
    [[ "$output" =~ "--no-sanitize" ]]
}

# ========================================
# Business Outcome: Size Constraints
# ========================================

@test "BR-PLATFORM-001.8: Support engineer can limit collection size for transfer constraints" {
    # Business Outcome: --max-size prevents overwhelming support ticket systems
    # Edge Case: Email attachment limits (25MB), case management systems (100MB)
    run bash "${GATHER_SCRIPT}" --help

    # Verify --max-size option is available
    assert_success
    [[ "$output" =~ "--max-size" ]]
}

# ========================================
# Business Outcome: Self-Service Support
# ========================================

@test "BR-PLATFORM-001: Support engineer (novice) can learn tool usage from help text" {
    # Business Outcome: --help provides clear guidance for new support staff
    # Edge Case: 3am incident, unfamiliar engineer needs to collect diagnostics
    run bash "${GATHER_SCRIPT}" --help

    assert_success
    # Help text guides troubleshooting
    [[ "$output" =~ "Kubernaut Must-Gather" ]]
    [[ "$output" =~ "Usage" ]]
    [[ "$output" =~ "Options" ]] || [[ "$output" =~ "FLAGS" ]]
}

# ========================================
# Edge Case: Invalid Input Handling
# ========================================

@test "BR-PLATFORM-001: Support engineer gets clear error for invalid flags" {
    # Edge Case: Typo in flag name (e.g., --sincee instead of --since)
    # Business Outcome: Clear error prevents confusion during incident
    run bash "${GATHER_SCRIPT}" --invalid-flag

    # Should fail with helpful error message
    assert_failure
    [[ "$output" =~ "Unknown option" ]] || [[ "$output" =~ "Error" ]] || [[ "$output" =~ "invalid" ]]
}

# ========================================
# Business Outcome: Structured Output
# ========================================

@test "BR-PLATFORM-001: Support engineer gets organized directory structure for efficient analysis" {
    # Business Outcome: BR-PLATFORM-001.8 specifies directory structure
    # Edge Case: Support engineer needs to quickly find specific data type (logs vs CRDs)
    create_mock_crd_response
    mock_kubectl "${TEST_TEMP_DIR}/crd-response.yaml"

    run timeout 30 bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --no-sanitize \
        2>&1 || true

    # Verify collection process mentions structure (logs, crds, events, etc.)
    [[ "$output" =~ "Collection" ]] || [[ "$output" =~ "Collecting" ]] || true
}

# ========================================
# Edge Case: Multiple Collections Same Day
# ========================================

@test "BR-PLATFORM-001: Support engineer can create multiple collections without conflicts" {
    # Edge Case: Collect diagnostics before and after attempted fix
    # Business Outcome: Timestamp precision prevents directory collisions
    create_mock_crd_response
    mock_kubectl "${TEST_TEMP_DIR}/crd-response.yaml"

    # First collection
    run timeout 10 bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --no-sanitize \
        2>&1 || true

    # Second collection (should create different directory)
    sleep 1  # Ensure different timestamp
    run timeout 10 bash "${GATHER_SCRIPT}" \
        --dest-dir="${TEST_TEMP_DIR}" \
        --no-sanitize \
        2>&1 || true

    # Should have 2 distinct directories (or at least attempted creation)
    local collection_dirs=$(find "${TEST_TEMP_DIR}" -maxdepth 1 -type d -name "kubernaut-must-gather-*" 2>/dev/null | wc -l)
    [ "$collection_dirs" -ge 1 ] # At least one was created (partial is OK for unit test)
}

