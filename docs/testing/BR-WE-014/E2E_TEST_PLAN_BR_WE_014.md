# E2E Test Plan: BR-WE-014 (Kubernetes Job Execution Backend)

**Version**: 1.1.0
**Created**: 2026-02-05
**Status**: Active
**Authority**: BR-WE-014 (Kubernetes Job Execution Backend)
**Service**: WorkflowExecution (WE)
**Test Tier**: E2E (End-to-End)
**Template**: [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)

---

## Overview

This test plan validates the Kubernetes Job execution backend for the WorkflowExecution controller.
E2E tests run against a Kind cluster with the full controller stack deployed, exercising real
Kubernetes API interactions including Job creation, status monitoring, and cleanup.

**Test Environment**:
- Kind cluster (2 nodes: control-plane + worker)
- Tekton Pipelines v1.7.0 (for regression)
- WorkflowExecution Controller with ExecutorRegistry (Tekton + Job)
- DataStorage + AuthWebhook infrastructure
- Pre-built Job container images from quay.io/kubernaut-cicd/test-workflows

**Test ID Convention**: `E2E-WE-014-{SEQUENCE}` per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md

---

## Test Scenarios

### P0 — Must Have (Core Job Backend Functionality)

| Test ID | Scenario | Gherkin AC | BRs |
|---------|----------|------------|-----|
| E2E-WE-014-001 | Job lifecycle: success path to completion | AC Scenario 1 | BR-WE-014, BR-WE-001 |
| E2E-WE-014-002 | Job lifecycle: failure with actionable details | AC Scenario 2 | BR-WE-014, BR-WE-004 |
| E2E-WE-014-003 | Job status sync: ExecutionRef and timing fields | AC Scenario 1 (partial) | BR-WE-014, BR-WE-003 |
| E2E-WE-014-004 | Job spec correctness: labels, env vars, image | AC Scenario 1 (partial) | BR-WE-014 |

### P1 — Should Have (Edge Cases and Cross-Cutting Concerns)

| Test ID | Scenario | Gherkin AC | BRs |
|---------|----------|------------|-----|
| E2E-WE-014-005 | Invalid executionEngine rejected by API server | AC Scenario 5 | BR-WE-014 |
| E2E-WE-014-006 | Deterministic Job naming for resource locking | AC Scenario 6 | BR-WE-014, BR-WE-009 |
| E2E-WE-014-007 | External Job deletion marks WFE as Failed | BR-WE-007 equivalent | BR-WE-014, BR-WE-007 |

### Out of Scope (This Iteration)

| Scenario | Gherkin AC | Reason |
|----------|------------|--------|
| Full platform E2E: OOMKill with Job backend | AC Scenario 8 | Requires all services deployed; deferred to platform E2E suite |
| RO propagates execution_engine from catalog | AC Scenario 7 | Cross-service; belongs to RO E2E suite |
| Default executionEngine applied when omitted | AC Scenario 4 | CRD has `+kubebuilder:default=tekton`; covered by existing Tekton tests |

---

## Test Scenario Details

### E2E-WE-014-001: Job Lifecycle Success Path

**Business Outcome**: Job-based remediations complete successfully within SLA (BR-WE-001).

**Preconditions**:
- WorkflowExecution controller deployed with Job executor registered
- `quay.io/kubernaut-cicd/test-workflows/job-hello-world:v1.0.0` pullable from cluster

**Steps**:
1. Create WFE with `executionEngine: "job"` and hello-world container image
2. Wait for transition to Running (Job created)
3. Wait for terminal phase (Completed or Failed)
4. Assert phase is Completed
5. Assert CompletionTime is set
6. Assert ExecutionCreated condition is True
7. Assert TektonPipelineComplete condition exists (terminal condition)

**Pass Criteria**: WFE reaches PhaseCompleted with conditions set.

---

### E2E-WE-014-002: Job Lifecycle Failure Path

**Business Outcome**: Job failures produce actionable failure details (BR-WE-004).

**Preconditions**:
- `quay.io/kubernaut-cicd/test-workflows/job-failing:v1.0.0` pullable from cluster

**Steps**:
1. Create WFE with `executionEngine: "job"` and intentionally failing container image
2. Wait for transition to Failed
3. Assert FailureDetails is populated with non-empty Message
4. Assert TektonPipelineComplete condition is False

**Pass Criteria**: WFE reaches PhaseFailed with failure details populated.

---

### E2E-WE-014-003: Job Status Sync

**Business Outcome**: WFE status accurately reflects Job execution state (BR-WE-003).

**Steps**:
1. Create WFE with `executionEngine: "job"`
2. Wait for Running phase
3. Assert ExecutionRef is set (tracks Job name)
4. Wait for terminal phase
5. Assert StartTime, CompletionTime, and Duration are populated

**Pass Criteria**: Timing fields populated for SLA tracking.

---

### E2E-WE-014-004: Job Spec Correctness

**Business Outcome**: Created Jobs have correct labels, env vars, and container image per BR-WE-014.

**Steps**:
1. Create WFE with `executionEngine: "job"` and known parameters
2. Wait for Running phase (Job created)
3. Fetch the created Job from `kubernaut-workflows` namespace
4. Assert Job labels include `kubernaut.ai/workflow-execution`, `kubernaut.ai/workflow-id`, `kubernaut.ai/execution-engine=job`
5. Assert Job container image matches WFE spec
6. Assert environment variables include TARGET_RESOURCE and custom parameters
7. Assert Job backoff limit is 0 (no retries)
8. Assert Job restart policy is Never

**Pass Criteria**: Job resource matches expected spec.

---

### E2E-WE-014-005: Invalid ExecutionEngine CRD Validation

**Business Outcome**: Invalid executionEngine values are rejected at API level, preventing misconfigured WFEs.

**Steps**:
1. Attempt to create WFE with `executionEngine: "ansible"` (invalid enum value)
2. Assert creation fails with a validation error
3. Assert no WFE is persisted in the API server

**Pass Criteria**: API server rejects invalid enum value.

---

### E2E-WE-014-006: Deterministic Job Naming for Resource Locking

**Business Outcome**: Deterministic Job naming enables resource locking via AlreadyExists errors (DD-WE-003, BR-WE-009).

**Note**: The full concurrent resource locking scenario (two WFEs competing for the same Job name)
is validated in integration tests (IT-WE-014-xxx) where timing can be controlled. The E2E test
validates the deterministic naming mechanism that enables locking, because the hello-world Job
image completes near-instantly, making concurrent collision unreliable in E2E.

**Steps**:
1. Create WFE with `executionEngine: "job"` for a known target resource
2. Wait for Running phase (Job created)
3. Fetch the created Job from the execution namespace
4. Assert Job name follows `wfe-<sha256(targetResource)[:16]>` format (20 chars total)
5. Assert Job name has `wfe-` prefix
6. Assert WFE `ExecutionRef.Name` matches the deterministic Job name

**Pass Criteria**: Job uses deterministic name derived from targetResource; ExecutionRef tracks it.

---

### E2E-WE-014-007: External Job Deletion Handling

**Business Outcome**: WFE detects externally deleted Jobs and fails gracefully (BR-WE-007 equivalent).

**Steps**:
1. Create WFE with `executionEngine: "job"`
2. Wait for Running phase (Job created)
3. Find and delete the Job externally via kubectl/client
4. Wait for WFE to reach Failed phase
5. Assert failure details indicate deletion/not-found

**Pass Criteria**: WFE marks as Failed when its Job is externally deleted.

---

## Infrastructure Requirements

### RBAC
The E2E ClusterRole must include `batch/v1 jobs` permissions:
```yaml
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### Scheme Registration
E2E suite must register `batchv1` scheme for Job listing in tests.

### Container Images
| Image | Purpose | Registry |
|-------|---------|----------|
| `job-hello-world:v1.0.0` | Success path testing | quay.io/kubernaut-cicd/test-workflows |
| `job-failing:v1.0.0` | Failure path testing | quay.io/kubernaut-cicd/test-workflows |

---

## Test File Location

```
test/e2e/workflowexecution/03_job_backend_test.go
```

---

## Dependencies

- [x] BR-WE-014 controller implementation (executor pattern)
- [x] Job container images pushed to quay.io
- [x] RBAC updated for batch/v1 Jobs
- [x] batchv1 scheme registered via k8s.io/client-go/kubernetes/scheme (built-in)
