# WorkflowExecution Service - Business Requirements

**Service**: WorkflowExecution Controller
**Service Type**: CRD Controller
**CRD**: WorkflowExecution
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Controller**: WorkflowExecutionReconciler
**Version**: 3.0 (Standardized BR-WE-* + API Group)
**Last Updated**: December 2, 2025
**Status**: Ready for Implementation

---

## 📋 Overview

The **WorkflowExecution Service** is a Kubernetes CRD controller that delegates workflow execution to specialized engines (Tekton, Argo, etc.) via OCI bundle references. It creates execution resources (e.g., Tekton PipelineRun) and monitors their completion status.

### Architecture Principle: Engine-Agnostic Execution

Kubernaut is **NOT** a workflow execution engine. We:
- ✅ **Store** workflow OCI bundles (via Data Storage / Workflow Catalog)
- ✅ **Reference** workflows from AI recommendations
- ✅ **Delegate** execution to specialized engines
- ✅ **Monitor** execution status and outcomes
- ❌ **DO NOT** orchestrate steps, handle rollback, or transform workflows

### Service Responsibilities

1. **PipelineRun Creation**: Create Tekton PipelineRun from OCI bundle reference
2. **Parameter Passing**: Pass LLM-selected parameters to execution engine
3. **Status Monitoring**: Watch execution status and update CRD
4. **Audit Trail**: Emit events and metrics for execution lifecycle

---

## 🎯 Business Requirements

### Category 1: Execution Delegation

#### BR-WE-001: Create PipelineRun from OCI Bundle

**Description**: WorkflowExecution Controller MUST create a Tekton PipelineRun using the bundle resolver to reference the OCI image specified in `spec.workflowRef.containerImage`.

**Priority**: P0 (CRITICAL)

**Rationale**: Engine-agnostic execution requires delegating to specialized engines. Tekton's bundle resolver allows direct execution from OCI bundles without transformation.

**Implementation**:
- Use Tekton bundle resolver: `resolver: bundles`
- Pass container image directly: `params: [{name: bundle, value: <containerImage>}]`
- No transformation or parsing of workflow contents
- Set owner reference for cascade deletion

**Acceptance Criteria**:
- ✅ PipelineRun created with bundle resolver
- ✅ Container image passed directly from spec
- ✅ Owner reference set to WorkflowExecution
- ✅ PipelineRun created within 5 seconds of CRD creation

**Test Coverage**:
- Unit: PipelineRun building logic
- Integration: PipelineRun creation with EnvTest
- E2E: Full execution with Kind + Tekton

**Related DDs**: DD-CONTRACT-001 (Contract Alignment)

---

#### BR-WE-002: Pass Parameters to Execution Engine

**Description**: WorkflowExecution Controller MUST pass all parameters from `spec.parameters` to the Tekton PipelineRun params, preserving UPPER_SNAKE_CASE naming per DD-WORKFLOW-003.

**Priority**: P0 (CRITICAL)

**Rationale**: Parameters from LLM selection (via AIAnalysis) must be passed unchanged to the execution engine. The workflow definition in the OCI bundle expects these parameters.

**Implementation**:
- Convert `map[string]string` to `[]tektonv1.Param`
- Preserve parameter names exactly (UPPER_SNAKE_CASE)
- No validation of parameter values (engine validates)
- Log parameters for audit trail

**Acceptance Criteria**:
- ✅ All parameters from spec present in PipelineRun
- ✅ Parameter names preserved exactly
- ✅ Parameter values passed as strings
- ✅ Empty parameters map handled gracefully

**Test Coverage**:
- Unit: Parameter conversion logic
- Integration: Parameters in created PipelineRun
- E2E: Parameters received by workflow tasks

**Related DDs**: DD-WORKFLOW-003 (Parameterized Actions)

---

### Category 2: Status Management

#### BR-WE-003: Monitor Execution Status

**Description**: WorkflowExecution Controller MUST watch the created PipelineRun status and update WorkflowExecution status accordingly (Pending → Running → Completed/Failed).

**Priority**: P0 (CRITICAL)

**Rationale**: The controller's primary responsibility is to track execution status for the RemediationOrchestrator and audit trail. Status must accurately reflect execution engine's state.

**Implementation**:
- Watch PipelineRun via owner reference
- Map Tekton conditions to WorkflowExecution phase:
  - PipelineRun running → Phase: Running
  - PipelineRun succeeded → Phase: Completed, Outcome: Success
  - PipelineRun failed → Phase: Failed, Outcome: Failed
- Update status within 10 seconds of PipelineRun completion

**Acceptance Criteria**:
- ✅ Phase transitions match PipelineRun status
- ✅ Outcome reflects success/failure accurately
- ✅ CompletionTime set when execution finishes
- ✅ Message populated from Tekton condition

**Test Coverage**:
- Unit: Status mapping logic
- Integration: Status updates during reconciliation
- E2E: Full status lifecycle

---

#### BR-WE-004: Owner Reference for Cascade Deletion

**Description**: WorkflowExecution Controller MUST set owner reference on created PipelineRun to enable cascade deletion when WorkflowExecution is deleted.

**Priority**: P0 (CRITICAL)

**Rationale**: Kubernetes garbage collection should automatically delete PipelineRun when WorkflowExecution is deleted, preventing orphaned resources.

**Implementation**:
- Set `ownerReferences` with controller: true
- Reference WorkflowExecution as owner
- Kubernetes GC handles deletion

**Acceptance Criteria**:
- ✅ Owner reference set on PipelineRun
- ✅ PipelineRun deleted when WorkflowExecution deleted
- ✅ No orphaned PipelineRuns

**Test Coverage**:
- Unit: Owner reference construction
- Integration: Cascade deletion verification
- E2E: Cleanup after test

---

### Category 3: Observability

#### BR-WE-005: Audit Events for Execution Lifecycle

**Description**: WorkflowExecution Controller MUST emit audit events for key lifecycle transitions (created, running, completed, failed) to support compliance and debugging.

**Priority**: P0 (CRITICAL)

**Rationale**: Complete audit trail enables compliance reporting, debugging, and analytics. Events correlate with RemediationRequest for end-to-end tracing.

**Implementation**:
- Emit Kubernetes events on phase transitions
- Write audit records to Data Storage (per ADR-034)
- Include correlation_id from RemediationRequest
- Include workflow_id and execution details

**Acceptance Criteria**:
- ✅ Events emitted for all phase transitions
- ✅ Audit records written to Data Storage
- ✅ Events correlatable via remediation_id
- ✅ Event details include workflow_id, outcome, duration

**Test Coverage**:
- Unit: Event emission logic
- Integration: Events recorded in K8s
- E2E: Audit trail verification

**Related ADRs**: ADR-034 (Unified Audit Table)

---

### Category 4: Error Handling

#### BR-WE-006: ServiceAccount Configuration

**Description**: WorkflowExecution Controller MUST support optional ServiceAccountName configuration for PipelineRun execution.

**Priority**: P1 (HIGH)

**Rationale**: Workflows may require specific RBAC permissions. ServiceAccount configuration enables secure execution with appropriate permissions.

**Implementation**:
- Read from `spec.executionConfig.serviceAccountName`
- Set in PipelineRun `taskRunTemplate.serviceAccountName`
- Default to "default" if not specified

**Acceptance Criteria**:
- ✅ ServiceAccountName propagated to PipelineRun
- ✅ Default used when not specified
- ✅ Workflow execution uses specified SA

**Test Coverage**:
- Unit: SA configuration logic
- Integration: SA in created PipelineRun

---

#### BR-WE-007: Handle Externally Deleted PipelineRun

**Description**: WorkflowExecution Controller MUST gracefully handle PipelineRun deletion by external actors (operators, garbage collection) and mark WorkflowExecution as Failed.

**Priority**: P1 (HIGH)

**Rationale**: External deletion is a valid scenario (operator intervention, timeout cleanup). Controller must detect and report this clearly.

**Implementation**:
- Check for NotFound error when getting PipelineRun
- Mark WorkflowExecution as Failed with clear message
- Include deletion timestamp if available

**Acceptance Criteria**:
- ✅ NotFound handled without panic
- ✅ WorkflowExecution marked Failed
- ✅ Message indicates external deletion
- ✅ No retry loop on deleted PipelineRun

**Test Coverage**:
- Unit: NotFound handling
- Integration: Simulated external deletion

---

#### BR-WE-008: Prometheus Metrics for Execution Outcomes

**Description**: WorkflowExecution Controller MUST expose Prometheus metrics for execution outcomes (success/failure counts, duration histograms) on port 9090.

**Priority**: P1 (HIGH)

**Rationale**: Metrics enable SLO tracking, alerting, and capacity planning. Essential for production observability.

**Implementation**:
- `workflowexecution_total{outcome}` - Counter for execution outcomes
- `workflowexecution_duration_seconds{outcome}` - Histogram for execution duration
- `workflowexecution_pipelinerun_creation_total` - Counter for PR creation
- `workflowexecution_skip_total{reason}` - Counter for skipped executions (DD-WE-001 visibility)
- Expose on `:9090/metrics`

**Note**: `workflow_id` label removed from core metrics to prevent high cardinality.
Label `reason` for skip_total: `ResourceBusy`, `RecentlyRemediated`.

**Acceptance Criteria**:
- ✅ Metrics exposed on /metrics endpoint
- ✅ Metrics updated on execution completion
- ✅ Labels include outcome and workflow_id
- ✅ Duration histogram with appropriate buckets

**Test Coverage**:
- Unit: Metrics recording logic
- Integration: Prometheus scrape validation

---

### Category 5: Resource Safety (V1.0)

#### BR-WE-009: Resource Locking - Prevent Parallel Execution

**Description**: WorkflowExecution Controller MUST prevent parallel workflow execution on the same target resource. Only ONE workflow can remediate a resource at any given time, regardless of workflow type.

**Priority**: P0 (CRITICAL)

**Rationale**: Parallel workflows on the same resource can cause conflicts, unpredictable state, and cascading failures. Two different workflows (e.g., `increase-memory` and `restart-pods`) targeting the same deployment could interfere with each other.

**Implementation**:
- Before creating PipelineRun, check for other Running/Pending WorkflowExecutions on same `targetResource`
- If found, set Phase=Skipped with Reason=ResourceBusy
- Include `conflictingWorkflow` details in `skipDetails`
- Emit audit event and notification
- No PipelineRun created for skipped executions

**Acceptance Criteria**:
- ✅ Only one workflow runs on a target resource at a time
- ✅ Second workflow is Skipped (not queued or failed)
- ✅ `skipDetails.conflictingWorkflow` populated with blocking workflow info
- ✅ Audit trail records skipped execution with reason
- ✅ Different targets can run in parallel

**Test Coverage**:
- Unit: Resource lock checking logic
- Integration: Concurrent WorkflowExecution creation
- E2E: Parallel signals targeting same resource

**Related DDs**: DD-CONTRACT-001 v1.4 (Resource Locking)

---

#### BR-WE-010: Cooldown - Prevent Redundant Sequential Execution

**Description**: WorkflowExecution Controller MUST prevent the same workflow from executing on the same target within a cooldown period (default: 5 minutes). This prevents redundant remediations from duplicate signals.

**Priority**: P0 (CRITICAL)

**Rationale**: Multiple signals can resolve to the same root cause and workflow (e.g., 10 pod evictions due to node DiskPressure all trigger `node-disk-cleanup`). Only one execution should occur; subsequent identical requests should be skipped.

**Implementation**:
- After resource lock check, check for recent Completed/Failed WorkflowExecutions with same `workflowId` + `targetResource`
- If found within cooldown period, set Phase=Skipped with Reason=RecentlyRemediated
- Include `recentRemediation` details in `skipDetails`
- Different workflows on same target ARE allowed (only same workflow+target blocked)
- Cooldown period configurable via controller config

**Acceptance Criteria**:
- ✅ Same workflow+target skipped within cooldown period
- ✅ Different workflow on same target is allowed
- ✅ `skipDetails.recentRemediation` populated with previous execution info
- ✅ `cooldownRemaining` indicates time until next execution allowed
- ✅ Audit trail records skipped execution with reason

**Test Coverage**:
- Unit: Cooldown checking logic
- Integration: Rapid sequential WorkflowExecution creation
- E2E: Storm scenario with duplicate signals

**Related DDs**: DD-CONTRACT-001 v1.4 (Resource Locking), DD-GATEWAY-008 (Storm Aggregation)

---

#### BR-WE-011: Target Resource Identification

**Description**: WorkflowExecution MUST include `spec.targetResource` field identifying the Kubernetes resource being remediated. Format: `namespace/kind/name` for namespaced resources, `kind/name` for cluster-scoped.

**Priority**: P0 (CRITICAL)

**Rationale**: Resource locking requires identifying the target resource. The `targetResource` field provides a canonical key for lock checking and audit trail.

**Implementation**:
- `spec.targetResource` is required field
- Format: `namespace/kind/name` (e.g., `payment/deployment/payment-api`)
- For cluster-scoped: `kind/name` (e.g., `node/worker-node-1`)
- Populated by RemediationOrchestrator from signal context
- Used as cache key for resource locking

**Acceptance Criteria**:
- ✅ `targetResource` is required in CRD validation
- ✅ Format enforced via validation webhook
- ✅ Used for resource lock comparisons
- ✅ Included in audit trail

**Test Coverage**:
- Unit: Target resource parsing and comparison
- Integration: CRD validation

**Related DDs**: DD-CONTRACT-001 v1.4

---

#### BR-WE-012: Exponential Backoff Cooldown (Pre-Execution Failures Only)

**Description**: WorkflowExecution Controller MUST implement exponential backoff for the cooldown period after consecutive **pre-execution failures** (`wasExecutionFailure: false`). This prevents remediation storms when infrastructure issues cause repeated failures. **Execution failures** (`wasExecutionFailure: true`) are NOT retried - they require manual intervention.

**Priority**: P0 (CRITICAL)

**Rationale**:
- **Pre-execution failures** (validation, image pull, quota, Tekton unavailable): Transient infrastructure issues that may resolve with time. Exponential backoff prevents storms while allowing recovery.
- **Execution failures** (workflow ran and failed): Non-idempotent actions may have occurred. Retrying is dangerous and could worsen the situation.

**Implementation**:
- Track `ConsecutiveFailures` count per target resource in WFE status (pre-execution failures only)
- Calculate cooldown: `min(BaseCooldown × 2^(failures-1), MaxCooldown)`
- Default: Base=1min, Max=10min, MaxConsecutiveFailures=5
- After `MaxConsecutiveFailures` reached → WFE marked Skipped with `ExhaustedRetries`
- Reset counter to 0 on successful completion
- Store `NextAllowedExecution` timestamp in status for persistence
- **Execution failures** (`wasExecutionFailure: true`) → Skipped with `PreviousExecutionFailed`, no retry

**Acceptance Criteria**:
- ✅ First pre-execution failure triggers 1-minute cooldown
- ✅ Consecutive pre-execution failures double cooldown (capped at 10 min)
- ✅ After 5 consecutive pre-execution failures, WFE marked Skipped with `ExhaustedRetries`
- ✅ Success resets failure counter to 0
- ✅ **Execution failures block ALL future retries** with `PreviousExecutionFailed`
- ✅ Backoff state survives controller restart (stored in CRD status)
- ✅ `workflowexecution_backoff_skip_total` metric exposed
- ✅ `workflowexecution_consecutive_failures` gauge exposed

**Test Coverage**:
- Unit: Backoff calculation, counter increment/reset, wasExecutionFailure distinction
- Integration: Multi-failure progression, reset on success, execution failure blocking
- E2E: Full backoff sequence with Tekton

**Related DDs**: DD-WE-004 (Exponential Backoff Cooldown), DD-WE-001 (Resource Locking)
**Cross-Team Agreements**: WE→RO-003 (QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md)

---

## 🔮 v1.1 Planned Requirements

### Category: Operational Safety Enhancements

#### BR-WE-013: Audit-Tracked Execution Block Clearing (v1.1)

**Description**: WorkflowExecution Controller MUST provide a mechanism for operators to clear `PreviousExecutionFailed` blocks that tracks WHO cleared the block and records the action in the audit trail.

**Priority**: P1 (HIGH)
**Target Version**: v1.1

**Rationale**: When a workflow execution fails (`wasExecutionFailure: true`), subsequent executions are blocked to prevent cascading failures from non-idempotent operations. Operators need a way to clear this block after manual investigation, but the clearing action must be auditable for accountability.

**v1.0 Limitation**: Operators must delete the failed WorkflowExecution CRD to clear the block. This loses the audit trail of the failed execution.

**v1.1 Requirements**:
- ✅ Clearing mechanism tracks operator identity (user who performed the action)
- ✅ Clearing action is recorded in audit trail with timestamp and reason
- ✅ Failed WFE is preserved for historical audit
- ✅ Clear action requires explicit acknowledgment (not accidental)

**Possible Implementations** (to be finalized in v1.1 design):
1. **Dedicated API endpoint**: POST `/api/v1/workflowexecutions/{name}/clear-block`
2. **Admission webhook**: Validates annotation with identity from request context
3. **CRD field**: `status.blockCleared` with `clearedBy`, `clearedAt`, `clearReason`

**Why NOT Annotations (v1.0 Decision)**:
- Annotations cannot capture WHO made the change in the audit trail
- Kubernetes annotations lack identity context
- No admission control to enforce required fields

**Acceptance Criteria** (v1.1):
- ⬜ Operator can clear `PreviousExecutionFailed` block without deleting WFE
- ⬜ Clearing action includes operator identity (from request context)
- ⬜ Clearing action recorded in audit trail with reason
- ⬜ Failed WFE preserved in cluster for audit
- ⬜ Clear action requires explicit acknowledgment

**Test Coverage** (v1.1):
- Unit: Block clearing logic, identity extraction
- Integration: Audit trail recording, WFE preservation
- E2E: Full clear workflow with audit validation

**Related BRs**: BR-WE-012 (Exponential Backoff), BR-WE-005 (Audit Trail)
**Origin**: DD-WE-004 Q3 response (NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md)

---

## 📊 Test Coverage Summary

### Target Coverage

| Test Tier | Target | Focus |
|-----------|--------|-------|
| **Unit** | 70%+ | PipelineRun building, status mapping, edge cases |
| **Integration** | 50%+ | EnvTest with Tekton CRDs, reconciliation cycle |
| **E2E** | 15% | Kind + Tekton, full execution |

### BR Coverage Matrix

**Last Updated**: 2025-12-07 (Day 12 Production Readiness)

| BR ID | Unit | Integration | E2E | Total |
|-------|------|-------------|-----|-------|
| BR-WE-001 | ✅ | ✅ | ✅ | 100% |
| BR-WE-002 | ✅ | ✅ | ✅ | 100% |
| BR-WE-003 | ✅ | ✅ | ✅ | 100% |
| BR-WE-004 | ✅ | ✅ | ✅ | 100% |
| BR-WE-005 | ✅ | ✅ | ⬜ | 90% |
| BR-WE-006 | ✅ | ✅ | ⬜ | 80% |
| BR-WE-007 | ✅ | ✅ | ⬜ | 70% |
| BR-WE-008 | ✅ | ✅ | ⬜ | 80% |
| BR-WE-009 | ✅ | ✅ | ✅ | 100% |
| BR-WE-010 | ✅ | ✅ | ✅ | 100% |
| BR-WE-011 | ✅ | ✅ | ✅ | 90% |
| BR-WE-012 | ✅ | ✅ | ⬜ | 67% |
| BR-WE-013 | - | - | - | v1.1 |

**Summary**: 12/12 BRs have Unit + Integration coverage. 8/12 BRs have E2E coverage. Overall: **94%**

---

## 🔗 Related Documentation

- [WorkflowExecution Overview](./overview.md)
- [CRD Schema](./crd-schema.md)
- [Implementation Plan v3.0](./implementation/IMPLEMENTATION_PLAN_V3.0.md)
- [DD-CONTRACT-001: AIAnalysis ↔ WorkflowExecution Alignment](../../../architecture/decisions/DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md)
- [ADR-043: Workflow Schema Definition](../../../architecture/decisions/ADR-043-workflow-schema-definition-standard.md)
- [DD-WORKFLOW-003: Parameterized Actions](../../../architecture/decisions/DD-WORKFLOW-003-parameterized-actions.md)

---

## 📝 Version History

| Version | Date | Changes |
|---------|------|---------|
| 2.1 | 2025-12-01 | **Resource Locking Safety**: Added BR-WE-009 (parallel prevention), BR-WE-010 (cooldown), BR-WE-011 (target identification). New `targetResource` spec field. New `Skipped` phase. See DD-WE-001 for design decision. |
| 2.0 | 2025-11-28 | **SIMPLIFIED**: Engine-agnostic architecture. Reduced from 38 BRs to 8 BRs. Removed step orchestration, validation framework, rollback handling. |
| 1.0 | 2025-10-13 | ❌ SUPERSEDED - Complex step orchestration with 38 BRs |

---

**Document Version**: 3.2
**Last Updated**: December 6, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: Ready for Implementation (v1.0), Planned (v1.1)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 3.2 | 2025-12-06 | **v1.1 Planning**: Added BR-WE-013 (Audit-Tracked Block Clearing) for v1.1. Deferred from v1.0 - annotations lack audit trail for identity tracking. |
| 3.1 | 2025-12-06 | **Exponential Backoff**: Added BR-WE-012 for exponential backoff cooldown. New `ConsecutiveFailures` and `NextAllowedExecution` status fields. New `ExhaustedRetries` skip reason. New metrics. See DD-WE-004. |
| 3.0 | 2025-12-02 | **Standardization**: Changed BR prefix from `BR-WF-*` to `BR-WE-*` per [00-core-development-methodology.mdc](../../../.cursor/rules/00-core-development-methodology.mdc). API group updated to `workflowexecution.kubernaut.ai/v1alpha1`. |
| 2.1 | 2025-12-01 | **Resource Locking Safety**: Added BR-WE-009 to BR-WE-011. New `targetResource` spec field. New `Skipped` phase. |
| 2.0 | 2025-11-28 | **SIMPLIFIED**: Engine-agnostic architecture. Reduced from 38 BRs to 8 BRs. |
| 1.0 | 2025-10-13 | ❌ SUPERSEDED - Complex step orchestration with 38 BRs |

