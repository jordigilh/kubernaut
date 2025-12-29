# BR-WE-009/010/011: Resource Locking Safety for Workflow Execution

**Service**: WorkflowExecution Controller
**Category**: Resource Safety
**Priority**: P0 (CRITICAL)
**Version**: 1.0
**Date**: 2025-12-01
**Status**: ✅ Approved
**Design Decision**: [DD-WE-001-resource-locking-safety.md](../architecture/decisions/DD-WE-001-resource-locking-safety.md)

---

## Overview

This document consolidates three related business requirements that implement resource locking safety for the WorkflowExecution service. These requirements prevent parallel and redundant workflow executions on the same Kubernetes resource.

**Design Decision**: These BRs were generated from DD-WE-001 (Resource Locking Safety).

---

## BR-WE-009: Resource Locking - Prevent Parallel Execution

### Description

WorkflowExecution Controller MUST prevent parallel workflow execution on the same target resource. Only ONE workflow can remediate a resource at any given time, regardless of workflow type.

### Priority

**P0 (CRITICAL)** - Safety-critical feature for V1.0

### Rationale

Parallel workflows on the same resource can cause:
- Conflicting state changes
- Unpredictable cluster state
- Cascading failures
- Race conditions

**Example**: If `increase-memory` and `restart-pods` workflows run simultaneously on the same deployment, the restart might interfere with the memory increase taking effect.

### Implementation

1. Before creating PipelineRun, query for other Running/Pending WorkflowExecutions
2. Filter by same `spec.targetResource`
3. If found, set `Phase=Skipped` with `Reason=ResourceBusy`
4. Populate `skipDetails.conflictingWorkflow` with blocking workflow info
5. Emit audit event and notification
6. Do NOT create PipelineRun for skipped executions

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-009-1 | Only one workflow runs on a target resource at a time | Integration, E2E |
| AC-009-2 | Second workflow is Skipped (not queued or failed) | Unit, Integration |
| AC-009-3 | `skipDetails.conflictingWorkflow` populated correctly | Unit |
| AC-009-4 | Audit trail records skipped execution with reason | Integration |
| AC-009-5 | Different targets can run in parallel | Integration, E2E |
| AC-009-6 | No PipelineRun created for skipped execution | Unit, Integration |

### Test Scenarios

```gherkin
Scenario: Parallel execution blocked
  Given WorkflowExecution "wfe-1" is Running on target "payment/deployment/api"
  When WorkflowExecution "wfe-2" is created for same target
  Then "wfe-2" status.phase should be "Skipped"
  And "wfe-2" status.skipDetails.reason should be "ResourceBusy"
  And "wfe-2" status.skipDetails.conflictingWorkflow.name should be "wfe-1"

Scenario: Different targets allowed in parallel
  Given WorkflowExecution "wfe-1" is Running on target "payment/deployment/api"
  When WorkflowExecution "wfe-2" is created for target "staging/deployment/api"
  Then "wfe-2" status.phase should be "Running"
  And PipelineRun should be created for "wfe-2"
```

---

## BR-WE-010: Cooldown - Prevent Redundant Sequential Execution

### Description

WorkflowExecution Controller MUST prevent the same workflow from executing on the same target within a cooldown period (default: 5 minutes). This prevents redundant remediations from duplicate signals.

### Priority

**P0 (CRITICAL)** - Safety-critical feature for V1.0

### Rationale

Multiple signals can resolve to the same root cause:
- 10 pod evictions due to node DiskPressure
- All resolve to `node-disk-cleanup` workflow
- Only ONE execution should occur
- Subsequent identical requests should be skipped

**Key Distinction**: Different workflows on the same target ARE allowed. Only same workflow+target is blocked.

### Implementation

1. After resource lock check (BR-WE-009)
2. Query for recent Completed/Failed WorkflowExecutions
3. Filter by same `spec.workflowRef.workflowId` AND same `spec.targetResource`
4. If found within cooldown period, set `Phase=Skipped` with `Reason=RecentlyRemediated`
5. Populate `skipDetails.recentRemediation` with previous execution info
6. Include `cooldownRemaining` time

### Cooldown Configuration

```yaml
# Controller configuration
workflowExecution:
  resourceLocking:
    cooldownPeriod: 5m  # Default: 5 minutes
```

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-010-1 | Same workflow+target skipped within cooldown | Unit, Integration |
| AC-010-2 | Different workflow on same target is allowed | Unit, Integration |
| AC-010-3 | `skipDetails.recentRemediation` populated correctly | Unit |
| AC-010-4 | `cooldownRemaining` indicates time until next allowed | Unit |
| AC-010-5 | Cooldown period is configurable | Unit |
| AC-010-6 | Execution allowed after cooldown expires | Integration |

### Test Scenarios

```gherkin
Scenario: Same workflow blocked within cooldown
  Given WorkflowExecution "wfe-1" completed 2 minutes ago
    And "wfe-1" workflow was "node-disk-cleanup" on target "node/worker-1"
  When WorkflowExecution "wfe-2" is created for "node-disk-cleanup" on "node/worker-1"
  Then "wfe-2" status.phase should be "Skipped"
  And "wfe-2" status.skipDetails.reason should be "RecentlyRemediated"
  And "wfe-2" status.skipDetails.cooldownRemaining should be "3m"

Scenario: Different workflow allowed on same target
  Given WorkflowExecution "wfe-1" completed 2 minutes ago
    And "wfe-1" workflow was "node-disk-cleanup" on target "node/worker-1"
  When WorkflowExecution "wfe-2" is created for "node-memory-reclaim" on "node/worker-1"
  Then "wfe-2" status.phase should be "Running"
  And PipelineRun should be created for "wfe-2"

Scenario: Same workflow allowed after cooldown
  Given WorkflowExecution "wfe-1" completed 6 minutes ago
    And "wfe-1" workflow was "node-disk-cleanup" on target "node/worker-1"
  When WorkflowExecution "wfe-2" is created for "node-disk-cleanup" on "node/worker-1"
  Then "wfe-2" status.phase should be "Running"
```

---

## BR-WE-011: Target Resource Identification

### Description

WorkflowExecution MUST include `spec.targetResource` field identifying the Kubernetes resource being remediated. This field is required for resource locking (BR-WE-009, BR-WE-010).

### Priority

**P0 (CRITICAL)** - Required for resource locking

### Format

```
namespace/kind/name    # For namespaced resources
kind/name              # For cluster-scoped resources

Examples:
- payment/deployment/payment-api
- staging/statefulset/postgres
- node/worker-node-1
- clusterrole/admin
```

### Implementation

1. `spec.targetResource` is a required field
2. CRD validation enforces format
3. RemediationOrchestrator populates from signal context
4. Used as cache key for resource locking comparisons
5. Included in audit trail

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-011-1 | `targetResource` is required in CRD validation | Unit |
| AC-011-2 | Format enforced: `[namespace/]kind/name` | Unit |
| AC-011-3 | Used for resource lock comparisons | Unit, Integration |
| AC-011-4 | Included in audit trail | Integration |
| AC-011-5 | RO populates from signal context | Integration |

### Validation Rules

```go
// Validation regex for targetResource
var targetResourceRegex = regexp.MustCompile(`^([a-z0-9-]+/)?[a-z]+/[a-z0-9-]+$`)

// Examples of valid targetResource values:
// - "payment/deployment/payment-api" ✓
// - "node/worker-node-1" ✓
// - "kube-system/configmap/coredns" ✓

// Examples of invalid values:
// - "payment-api" ✗ (missing kind)
// - "deployment" ✗ (missing name)
// - "Payment/Deployment/API" ✗ (uppercase not allowed)
```

---

## CRD Schema Impact

### New Spec Field

```yaml
spec:
  targetResource: "payment/deployment/payment-api"  # REQUIRED
```

### New Status Phase

```yaml
status:
  phase: Skipped  # New valid phase
```

### New Status Fields

```yaml
status:
  skipDetails:
    reason: "ResourceBusy"  # or "RecentlyRemediated"
    message: "Another workflow is currently remediating this resource"
    skippedAt: "2025-12-01T10:16:00Z"
    conflictingWorkflow:  # For ResourceBusy
      name: "workflow-payment-oom-001"
      workflowId: "oomkill-increase-memory"
      startedAt: "2025-12-01T10:15:00Z"
      targetResource: "payment/deployment/payment-api"
    recentRemediation:    # For RecentlyRemediated
      name: "workflow-node-disk-001"
      workflowId: "node-disk-cleanup"
      completedAt: "2025-12-01T10:18:00Z"
      outcome: "Completed"
      targetResource: "node/worker-node-1"
      cooldownRemaining: "4m30s"
```

---

## Test Coverage Summary

| BR ID | Unit | Integration | E2E | Total |
|-------|------|-------------|-----|-------|
| BR-WE-009 | ✅ | ✅ | ✅ | 100% |
| BR-WE-010 | ✅ | ✅ | ✅ | 100% |
| BR-WE-011 | ✅ | ✅ | - | 90% |

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **DD-WE-001** | Design decision that generated these BRs |
| **DD-CONTRACT-001 v1.4** | Contract alignment with resource locking |
| **DD-GATEWAY-008** | Storm aggregation at Gateway level |
| **DD-GATEWAY-009** | Fingerprint deduplication at Gateway level |
| **CRD Schema v3.1** | Schema changes for resource locking |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-01 | Initial BR: Resource locking safety (BR-WE-009, BR-WE-010, BR-WE-011) |

---

**Document Version**: 1.0
**Last Updated**: December 1, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: ✅ Approved



