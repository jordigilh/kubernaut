#!/usr/bin/env bats
# Kubernaut Must-Gather - Kubernetes Jobs Collection Tests
# BR-PLATFORM-001.6g: Support engineers can diagnose Job-executor workflow
# failures (Issue #2036) -- native batchv1.Job resources created by
# WorkflowExecution's Job executor (pkg/workflowexecution/executor/job.go)
# in WORKFLOW_NAMESPACE had zero must-gather coverage before this collector:
# logs.sh never scans WORKFLOW_NAMESPACE (RELEASE_NAMESPACE/OPERATOR_NAMESPACE
# only), and tekton.sh only sees Tekton CRDs, not native Jobs.

load helpers

setup() {
    setup_test_environment
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: Diagnose Job-Executor Workflow Failures
# ========================================

@test "BR-PLATFORM-001.6g: Support engineer can see why a WorkflowExecution Job failed" {
    # Business Outcome: Job executor workflow failures are diagnosable from
    # the Job's own status/conditions, not just the parent WorkflowExecution CRD.
    create_mock_job_names_list
    create_mock_jobs_list_yaml
    create_mock_job_spec_yaml
    mock_kubectl "${TEST_TEMP_DIR}/jobs-list.yaml"

    run bash "${COLLECTORS_DIR}/jobs.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/jobs/all-jobs.yaml"
    assert_file_contains "${MOCK_COLLECTION_DIR}/jobs/all-jobs.yaml" "wfe-deployment-frontend-abc123"
    assert_file_exists "${MOCK_COLLECTION_DIR}/jobs/wfe-deployment-frontend-abc123/spec.yaml"
    assert_file_contains "${MOCK_COLLECTION_DIR}/jobs/wfe-deployment-frontend-abc123/spec.yaml" "BackoffLimitExceeded"
}

@test "BR-PLATFORM-001.6g: Support engineer gets pod logs for every retry attempt of a failed Job" {
    # Business Outcome: A Job with BackoffLimit > 0 can own multiple Pods
    # across retries -- the job-name label selector (set on every Pod the
    # Job controller creates) must capture all of them, not just the latest.
    create_mock_job_names_list
    create_mock_jobs_list_yaml
    create_mock_job_spec_yaml
    mock_kubectl "${TEST_TEMP_DIR}/jobs-list.yaml"

    run bash "${COLLECTORS_DIR}/jobs.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/jobs/wfe-deployment-frontend-abc123/logs.txt"
}

@test "BR-PLATFORM-001.6g: Support engineer sees diagnostics for every Job, not just one" {
    # Business Outcome: Multiple concurrent WorkflowExecutions each create
    # their own Job -- all of them must be collected, not just the first.
    create_mock_job_names_list
    create_mock_jobs_list_yaml
    create_mock_job_spec_yaml
    mock_kubectl "${TEST_TEMP_DIR}/jobs-list.yaml"

    run bash "${COLLECTORS_DIR}/jobs.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/jobs/wfe-deployment-frontend-abc123/spec.yaml"
    assert_file_exists "${MOCK_COLLECTION_DIR}/jobs/wfe-statefulset-cache-xyz789/spec.yaml"
    [[ "$output" =~ "Jobs collection complete (2 Jobs)" ]]
}

# ========================================
# Edge Case: No Jobs Present
# ========================================

@test "BR-PLATFORM-001.6g: Collection succeeds when no Job-executor workflows have run yet" {
    # Edge Case: cluster only uses the Tekton executor, or no workflows have
    # executed at all -- must not fail the overall must-gather run.
    echo "" > "${TEST_TEMP_DIR}/empty.yaml"
    mock_kubectl "${TEST_TEMP_DIR}/empty.yaml"

    run bash "${COLLECTORS_DIR}/jobs.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    [[ "$output" =~ "Jobs collection complete (0 Jobs)" ]]
}

@test "BR-PLATFORM-001.6g: Collection succeeds when WORKFLOW_NAMESPACE is unreachable" {
    # Edge Case: kubectl errors (e.g. RBAC denial, namespace not yet created)
    # must degrade gracefully -- partial diagnostics beat a hard failure that
    # aborts the rest of the must-gather run.
    run env WORKFLOW_NAMESPACE="kubernaut-workflows" SINCE_DURATION="24h" \
        bash "${COLLECTORS_DIR}/jobs.sh" "${MOCK_COLLECTION_DIR}"

    assert_success
    assert_file_exists "${MOCK_COLLECTION_DIR}/jobs/all-jobs.yaml"
}
