# RemediationOrchestrator Service - Business Requirements

**Service**: RemediationOrchestrator Controller
**Service Type**: CRD Controller
**CRD**: RemediationRequest
**Controller**: RemediationRequestReconciler
**Version**: 1.3
**Last Updated**: December 1, 2025
**Status**: In Development

---

## 📋 Overview

The **RemediationOrchestrator** is the central coordinator for the Kubernaut remediation lifecycle. It watches `RemediationRequest` CRDs and orchestrates the creation and monitoring of child CRDs (SignalProcessing, AIAnalysis, RemediationApprovalRequest, WorkflowExecution) to drive automated incident remediation.

### Service Responsibilities

1. **Lifecycle Orchestration**: Manage RemediationRequest phase transitions
2. **Child CRD Management**: Create and monitor SignalProcessing, AIAnalysis, WorkflowExecution
3. **Approval Orchestration**: Create RemediationApprovalRequest and NotificationRequest when needed
4. **Data Pass-Through**: Pass workflow data from AIAnalysis.status to WorkflowExecution.spec (no catalog lookup)
5. **Timeout Enforcement**: Enforce global and per-phase timeouts
6. **Notification Triggering**: Create NotificationRequest CRDs for operator alerts

---

## 🎯 Business Requirements

### Category 0: V1.0 Core Requirements

#### BR-ORCH-001: Approval Notification Creation

**Description**: RemediationOrchestrator MUST create NotificationRequest CRDs when AIAnalysis enters the "Approving" phase (confidence between 60-79%), alerting operators that manual approval is required before workflow execution.

**Priority**: P0 (CRITICAL)

**Rationale**: Without push notifications, operators must manually poll for pending approvals (`kubectl get aiapprovalrequest --watch`), resulting in 40-60% approval miss rate and 30-40% timeout rate. Push notifications reduce approval miss rate to <5% and timeout rate to <10%.

**Implementation**:
- Watch AIAnalysis CRD status changes
- When `AIAnalysis.status.phase == "Approving"`:
  - Check `RemediationRequest.status.approvalNotificationSent == false` (idempotency)
  - Create NotificationRequest CRD with approval context
  - Set `RemediationRequest.status.approvalNotificationSent = true`
- NotificationRequest contains:
  - Investigation summary
  - Evidence collected
  - Recommended actions with rationales
  - Why approval is required
  - Links to approve/reject

**Acceptance Criteria**:
- ✅ NotificationRequest created when AIAnalysis enters "Approving" phase
- ✅ Idempotency: Only ONE notification per approval request (no duplicates on retries)
- ✅ Notification contains complete approval context
- ✅ OwnerReference set for cascade deletion
- ✅ Approval miss rate reduced from 40-60% to <5%
- ✅ End-to-end latency <5 seconds from AIAnalysis "Approving" to notification delivery

**Test Coverage**:
- Unit: Approval detection logic, idempotency flag handling
- Integration: AIAnalysis status → NotificationRequest creation
- E2E: Full approval notification workflow

**Related ADRs**: ADR-018 (Approval Notification V1.0 Integration), ADR-017 (NotificationRequest Creator)
**Related DDs**: None (V1.0 core feature)

---

### Category 1: Workflow Data Pass-Through

#### BR-ORCH-025: Workflow Data Pass-Through to WorkflowExecution

**Description**: RemediationOrchestrator MUST pass through workflow data (including `container_image` and `container_digest`) from AIAnalysis.status.selectedWorkflow to WorkflowExecution.spec.workflowRef without performing catalog lookups.

**Priority**: P0 (CRITICAL)

**Rationale**: HolmesGPT-API resolves `workflow_id → container_image` during MCP search (per DD-CONTRACT-001 v1.2). AIAnalysis.status.selectedWorkflow contains the fully resolved workflow reference including containerImage and containerDigest. RO's responsibility is to pass this data through to WorkflowExecution, not to perform additional catalog lookups.

**Implementation**:
- Read `AIAnalysis.status.selectedWorkflow` when phase = "Completed"
- Pass through all fields to `WorkflowExecution.spec.workflowRef`:
  - `workflowId` ← `selectedWorkflow.workflowId`
  - `version` ← `selectedWorkflow.version`
  - `containerImage` ← `selectedWorkflow.containerImage` (resolved by HolmesGPT-API)
  - `containerDigest` ← `selectedWorkflow.containerDigest` (resolved by HolmesGPT-API)
- Pass through `parameters` unchanged (UPPER_SNAKE_CASE keys)
- Pass through `confidence` and `rationale` for audit trail
- Fail if `selectedWorkflow` is nil or missing required fields

**Acceptance Criteria**:
- ✅ RO does NOT call Data Storage API for workflow resolution
- ✅ `container_image` and `container_digest` passed through from AIAnalysis.status
- ✅ All workflow parameters passed through unchanged
- ✅ Missing selectedWorkflow → RemediationRequest marked as Failed
- ✅ Audit trail contains complete workflow selection context

**Test Coverage**:
- Unit: Pass-through logic, field mapping
- Integration: AIAnalysis → WorkflowExecution data flow
- E2E: End-to-end workflow execution with resolved image

**Related DDs**: DD-CONTRACT-001 v1.2 (AIAnalysis ↔ WorkflowExecution Alignment - **AUTHORITATIVE**)
**Related ADRs**: ADR-043 (Workflow Schema Definition)

---

#### BR-ORCH-026: Approval Orchestration

**Description**: RemediationOrchestrator MUST create RemediationApprovalRequest CRD and NotificationRequest CRD when AIAnalysis signals approval is required, enabling operator review before workflow execution.

**Priority**: P0 (CRITICAL)

**Rationale**: When AIAnalysis confidence is below threshold (80%), human approval is required. RemediationOrchestrator must orchestrate both the approval request (for tracking) and notification (for alerting operators).

**Implementation**:
- Watch AIAnalysis.status.approvalRequired
- When true: Create RemediationApprovalRequest (per ADR-040)
- When true: Create NotificationRequest (per ADR-017/ADR-018)
- Wait for RemediationApprovalRequest.status.decision
- On Approved: Create WorkflowExecution
- On Rejected/Expired: Mark RemediationRequest as Rejected

**Acceptance Criteria**:
- ✅ RemediationApprovalRequest created when approvalRequired = true
- ✅ NotificationRequest created for operator alerting
- ✅ Approval decision watched and acted upon
- ✅ Timeout handled via RemediationApprovalRequest controller

**Test Coverage**:
- Unit: Approval orchestration logic
- Integration: CRD creation and watching
- E2E: Full approval workflow

**Related ADRs**: ADR-017 (NotificationRequest Creator), ADR-018 (Approval Notification), ADR-040 (RemediationApprovalRequest)

---

### Category 2: Timeout Management

#### BR-ORCH-027: Global Remediation Timeout

**Description**: RemediationOrchestrator MUST enforce a global timeout (default: 1 hour) for the entire remediation lifecycle, preventing stuck remediations from consuming resources indefinitely.

**Priority**: P0 (CRITICAL)

**Rationale**: Without global timeout, stuck remediations (due to hung HolmesGPT, unresponsive approvers, stuck Tekton pipelines) would never terminate. Global timeout ensures all remediations eventually reach a terminal state.

**Implementation**:
- Default global timeout: 1 hour (configurable)
- Check on every reconciliation: `time.Since(creationTimestamp) > globalTimeout`
- On timeout: Set phase = "Timeout", create notification
- Configurable per-remediation via spec.timeouts.global

**Acceptance Criteria**:
- ✅ Remediations exceeding global timeout marked as Timeout
- ✅ NotificationRequest created on timeout
- ✅ Default timeout configurable via ConfigMap
- ✅ Per-remediation override supported

**Test Coverage**:
- Unit: Timeout detection logic
- Integration: Timeout triggering with simulated delays
- E2E: End-to-end timeout behavior

**Related DDs**: DD-TIMEOUT-001 (Global Remediation Timeout Strategy)

---

#### BR-ORCH-028: Per-Phase Timeouts

**Description**: RemediationOrchestrator MUST enforce per-phase timeouts to detect stuck individual phases without waiting for global timeout.

**Priority**: P1 (HIGH)

**Rationale**: Per-phase timeouts enable faster detection of phase-specific issues (e.g., hung AIAnalysis) without waiting for the full global timeout. This improves MTTR by failing fast.

**Implementation**:
- Default phase timeouts:
  - SignalProcessing: 5 minutes
  - AIAnalysis: 10 minutes
  - Approval: 15 minutes (per ADR-040)
  - WorkflowExecution: 30 minutes
- Track phase start time in status.phaseTransitions
- Check on reconciliation: `time.Since(phaseStart) > phaseTimeout`

**Acceptance Criteria**:
- ✅ Each phase has configurable timeout
- ✅ Phase timeout triggers before global timeout
- ✅ Phase start times tracked in status
- ✅ Timeout reason indicates which phase timed out

**Test Coverage**:
- Unit: Per-phase timeout logic
- Integration: Phase-specific timeout triggering
- E2E: Multi-phase timeout scenarios

**Related DDs**: DD-TIMEOUT-001 (Global Remediation Timeout Strategy)
**Related ADRs**: ADR-040 (Approval timeout: 15 minutes)

---

### Category 3: Notification Handling

#### BR-ORCH-029: User-Initiated Notification Cancellation

**Description**: RemediationOrchestrator MUST treat user deletion of NotificationRequest CRDs as intentional cancellation (not system failure), marking RemediationRequest as `Completed` with a cancellation condition rather than `Failed`.

**Priority**: P0 (CRITICAL)

**Rationale**: Per DD-NOT-005, NotificationRequest spec is immutable, so users can only cancel notifications by deleting the CRD. RO must distinguish user-initiated cancellation from system failures to prevent false positive escalations and provide accurate audit trail.

**Implementation**:
- Watch NotificationRequest CRDs via owner reference pattern
- Detect `NotFound` errors during reconciliation
- Distinguish cascade deletion (RemediationRequest being deleted) from user cancellation (NotificationRequest deleted independently)
- On user cancellation:
  - Set `status.phase = Completed` (NOT Failed)
  - Set `status.notificationStatus = "Cancelled"`
  - Add condition: `NotificationDelivered=False` with reason `ReasonUserCancelled` (from `pkg/remediationrequest/conditions.go`)
  - DO NOT trigger escalation workflows

**Acceptance Criteria**:
- ✅ User deletion of NotificationRequest detected via watch
- ✅ RemediationRequest marked as `Completed` (not `Failed`) on user cancellation
- ✅ Condition `NotificationDelivered=False` with reason `UserCancelled` set
- ✅ No automatic escalation triggered for user cancellations
- ✅ Cascade deletion (RR deleted) handled gracefully without warnings
- ✅ Audit trail clearly indicates user-initiated cancellation

**Test Coverage**:
- Unit: Cancellation detection logic, status update logic
- Integration: NotificationRequest deletion scenarios (user vs cascade)
- E2E: Full workflow with user cancellation

**Related DDs**: DD-NOT-005 (NotificationRequest Spec Immutability), DD-RO-001 (Notification Cancellation Handling)
**Related ADRs**: ADR-001 (CRD Microservices - Owner References), ADR-017 (NotificationRequest Creator)

---

#### BR-ORCH-030: Notification Status Tracking in Remediation Workflow

**Description**: RemediationOrchestrator MUST track NotificationRequest delivery status and propagate it to RemediationRequest status for observability, enabling SREs to query remediation status including notification outcomes.

**Priority**: P1 (HIGH)

**Rationale**: Notification delivery is a critical part of the remediation workflow. RO must track and expose notification status to provide complete workflow observability and enable querying by notification outcome.

**Implementation**:
- Watch NotificationRequest status updates
- Update `status.notificationStatus` based on NotificationRequest phase:
  - `Pending` → `notificationStatus = "Pending"`
  - `Sending` → `notificationStatus = "InProgress"`
  - `Sent` → `notificationStatus = "Sent"`, condition `NotificationDelivered=True` (reason `ReasonDeliverySucceeded`)
  - `Failed` → `notificationStatus = "Failed"`, condition `NotificationDelivered=False` with reason `ReasonDeliveryFailed`
  - `Deleted` → `notificationStatus = "Cancelled"`, condition `NotificationDelivered=False` with reason `ReasonUserCancelled`

**Constants**: Use centralized constants from `pkg/remediationrequest/conditions.go`: `ReasonDeliverySucceeded`, `ReasonDeliveryFailed`, `ReasonUserCancelled`.
- Set `NotificationDelivered` condition with appropriate status and reason
- Store NotificationRequest name in `status.notificationRequestName` for tracking

**Acceptance Criteria**:
- ✅ RO watches NotificationRequest status updates
- ✅ `status.notificationStatus` updated based on NotificationRequest phase
- ✅ `NotificationDelivered` condition set with accurate reason
- ✅ SREs can query RemediationRequests by notification status
- ✅ Metrics expose notification status distribution

**Test Coverage**:
- Unit: Status mapping logic (NotificationRequest → RemediationRequest)
- Integration: NotificationRequest status propagation
- E2E: Full workflow with notification status tracking

**Related DDs**: DD-RO-001 (Notification Cancellation Handling)
**Related ADRs**: ADR-017 (NotificationRequest Creator)

---

#### BR-ORCH-031: Cascade Cleanup for Child NotificationRequest CRDs

**Description**: RemediationOrchestrator MUST set owner references on NotificationRequest CRDs to enable automatic cascade deletion when RemediationRequest is deleted, preventing orphaned notification CRDs.

**Priority**: P1 (HIGH)

**Rationale**: Kubernetes owner references provide automatic cleanup of child resources when parent is deleted. This prevents orphaned NotificationRequest CRDs and ensures consistent resource lifecycle management.

**Implementation**:
- Set `ownerReferences` on NotificationRequest during creation:
  - `apiVersion`: RemediationRequest API version
  - `kind`: "RemediationRequest"
  - `name`: RemediationRequest name
  - `uid`: RemediationRequest UID
  - `controller: true`: RO is the controlling owner
  - `blockOwnerDeletion: false`: Allow independent NotificationRequest deletion (for user cancellation)
- Kubernetes automatically deletes NotificationRequest when RemediationRequest is deleted
- RO detects cascade deletion (RemediationRequest has `deletionTimestamp`) vs user cancellation (no `deletionTimestamp`)

**Acceptance Criteria**:
- ✅ NotificationRequest has ownerReference to RemediationRequest
- ✅ `blockOwnerDeletion = false` allows independent user deletion
- ✅ Deleting RemediationRequest automatically deletes NotificationRequest
- ✅ No orphaned NotificationRequest CRDs remain after RemediationRequest deletion
- ✅ RO distinguishes cascade deletion from user cancellation

**Test Coverage**:
- Unit: Owner reference creation logic
- Integration: Cascade deletion behavior, orphan detection
- E2E: Full cleanup scenarios

**Related DDs**: DD-RO-001 (Notification Cancellation Handling)
**Related ADRs**: ADR-001 (CRD Microservices - Owner References)

---

### Category 4: Resource Lock Deduplication (DD-RO-001)

#### BR-ORCH-032: Handle WE Skipped Phase

**Description**: RemediationOrchestrator MUST watch WorkflowExecution status and handle the `Skipped` phase when WE's resource locking mechanism prevents execution due to `ResourceBusy` or `RecentlyRemediated` reasons.

**Priority**: P0 (CRITICAL)

**Rationale**: WorkflowExecution implements resource-level locking (DD-WE-001) to prevent parallel and redundant workflow executions on the same Kubernetes resource. When WE skips execution, RO must update RemediationRequest status accordingly and track the relationship with the active remediation.

**Implementation**:
- Watch WorkflowExecution.status.phase for `Skipped` value
- Extract skip reason from `status.skipDetails.reason` (`ResourceBusy` or `RecentlyRemediated`)
- Extract parent RR reference from:
  - `ResourceBusy`: `status.skipDetails.conflictingWorkflow.remediationRequestRef`
  - `RecentlyRemediated`: `status.skipDetails.recentRemediation.remediationRequestRef`
- Update RemediationRequest:
  - `status.phase = "Skipped"`
  - `status.skipReason = reason`
  - `status.duplicateOf = parentRRName`
  - `status.message = "Skipped: {reason} - see {parentRRName}"`

**Acceptance Criteria**:
- ✅ RO watches WorkflowExecution status changes
- ✅ `Skipped` phase detected and handled
- ✅ Skip reason (`ResourceBusy`, `RecentlyRemediated`) extracted and stored
- ✅ Parent RR reference stored in `status.duplicateOf`
- ✅ RemediationRequest phase set to `Skipped` (not `Failed`)
- ✅ Audit trail clearly indicates skip reason

**Test Coverage**:
- Unit: Skip detection logic, status update logic
- Integration: WE Skipped → RO handling flow
- E2E: Full workflow with resource lock skip

**Related DDs**: DD-RO-001 (Resource Lock Deduplication Handling), DD-WE-001 (Resource Locking Safety)

---

#### BR-ORCH-033: Track Duplicate Remediations

**Description**: RemediationOrchestrator MUST track the relationship between skipped (duplicate) RemediationRequests and their parent (active) RemediationRequest, enabling audit trail and consolidated reporting.

**Priority**: P1 (HIGH)

**Rationale**: When multiple signals with different fingerprints target the same resource, Gateway creates separate RemediationRequests. WE's resource locking causes all but one to be skipped. RO must track these relationships for audit, metrics, and consolidated notifications.

**Implementation**:
- When handling Skipped phase (BR-ORCH-032):
  - Update parent RR's `status.duplicateCount++`
  - Append to parent RR's `status.duplicateRefs[]`
- Handle race conditions with optimistic concurrency (resourceVersion)
- Non-blocking: Continue even if parent tracking fails (log warning)

**Acceptance Criteria**:
- ✅ Parent RR tracks count of skipped duplicates
- ✅ Parent RR tracks list of duplicate RR names
- ✅ Duplicate tracking survives RO restarts (persisted in status)
- ✅ Race conditions handled gracefully
- ✅ Tracking failure does not block remediation workflow

**Test Coverage**:
- Unit: Duplicate tracking logic, race condition handling
- Integration: Multiple RRs → one parent tracking
- E2E: Full storm scenario with duplicate tracking

**Related DDs**: DD-RO-001 (Resource Lock Deduplication Handling)

---

#### BR-ORCH-034: Bulk Notification for Duplicates

**Description**: RemediationOrchestrator MUST send ONE consolidated notification when a parent RemediationRequest completes (success or failure), including summary of all skipped duplicates, to avoid notification spam.

**Priority**: P1 (HIGH)

**Rationale**: Without consolidated notifications, 10 skipped RRs would generate 10 separate notifications, overwhelming operators. Bulk notification provides complete context (result + duplicate count) in a single message.

**Implementation**:
- When parent RR completes (WorkflowExecution Completed/Failed):
  - Check `status.duplicateCount > 0`
  - Build notification body with:
    - Workflow result (success/failure)
    - Target resource
    - Duration
    - Duplicate count with breakdown by skip reason
    - First/last signal timestamps
  - Create single NotificationRequest with consolidated content
- Notification triggered on parent completion (not on each skip)

**Acceptance Criteria**:
- ✅ ONE notification sent when parent completes (not per-skip)
- ✅ Notification includes duplicate count and skip reasons
- ✅ Notification sent for both success AND failure outcomes
- ✅ Duplicate RR names included in notification metadata
- ✅ No notification spam (10 duplicates = 1 notification)

**Test Coverage**:
- Unit: Notification content building, trigger logic
- Integration: Parent completion → bulk notification
- E2E: Full storm scenario with consolidated notification

**Related DDs**: DD-RO-001 (Resource Lock Deduplication Handling)
**Related ADRs**: ADR-017 (NotificationRequest Creator)

---

### Category 5: Operational Awareness

#### BR-ORCH-046: Policy-Driven Operational Awareness Notification

**Description**: RemediationOrchestrator MUST evaluate a configurable Rego policy after SignalProcessing completes (at the `processing → analyzing` transition) to determine whether operators should be proactively notified that a remediation is underway, using normalized signal data and remediation history.

**Priority**: P1 (HIGH)

**Rationale**: During early adoption, SREs need real-time awareness of automated remediation activity. Beyond early adoption, policy-driven notifications detect operational patterns (remediation loops, critical production incidents) that require attention even when individual remediations succeed. No existing notification mechanism covers the window between signal classification and workflow outcome.

**Implementation**:
- Evaluate Rego policy at `processing → analyzing` transition
- Policy input: normalized severity, environment, priority, fingerprint, remediation history
- Default rules: critical/high + production, frequency ≥ 2/1h, first-time remediation
- Create `NotificationRequest` with `type=operational-awareness` when policy returns `notify: true`
- Idempotency via `operationalNotificationSent` status flag
- Non-blocking: policy failure does not delay remediation pipeline

**Acceptance Criteria**:
- ✅ Rego policy evaluated after SP completion, before AA creation
- ✅ Default policy covers critical/high production, frequency escalation, first-time
- ✅ Configurable via ConfigMap (hot-reloadable)
- ✅ Non-blocking: policy errors logged but don't stop remediation
- ✅ Idempotent: one notification per RR

**Test Coverage**:
- Unit: Policy evaluation, history computation, idempotency
- Integration: SP completion → policy → NotificationRequest flow
- E2E: Full pipeline with operational notification (file sink)

**Full BR**: [BR-ORCH-046: Policy-Driven Operational Awareness Notification](../../../requirements/BR-ORCH-046-operational-awareness-notification.md)
**Related BRs**: BR-ORCH-042 (field index reuse), BR-ORCH-001 (notification pattern), DD-AIANALYSIS-001 (Rego pattern)

---

## 📊 Test Coverage Summary

### Unit Tests
- **Status**: Planned
- **Target Coverage**: 70%+

### Integration Tests
- **Status**: Planned
- **Target Coverage**: 50%+

### E2E Tests
- **Status**: Planned
- **Target Coverage**: 10-15%

---

## 🔗 Related Documentation

- [RemediationOrchestrator Overview](./overview.md)
- [CRD Schema](./crd-schema.md)
- [Controller Implementation](./controller-implementation.md)
- [DD-TIMEOUT-001: Global Remediation Timeout](../../../architecture/decisions/DD-TIMEOUT-001-global-remediation-timeout.md)
- [DD-CONTRACT-001: AIAnalysis ↔ WorkflowExecution Alignment](../../../architecture/decisions/DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md)
- [DD-RO-001: Resource Lock Deduplication Handling](../../../architecture/decisions/DD-RO-001-resource-lock-deduplication-handling.md)
- [DD-NOT-005: NotificationRequest Spec Immutability](../06-notification/DD-NOT-005-SPEC-IMMUTABILITY.md)
- [ADR-017: NotificationRequest Creator](../../../architecture/decisions/ADR-017-notification-crd-creator.md)
- [ADR-040: RemediationApprovalRequest](../../../architecture/decisions/ADR-040-remediation-approval-request-architecture.md)

---

## 📝 Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.5 | 2026-02-12 | Added BR-ORCH-046 (Policy-Driven Operational Awareness Notification) - Rego-based notification at SP completion |
| 1.4 | 2025-12-02 | Added BR-ORCH-001 (Approval Notification Creation) - formalized from existing usage; Deprecated BR-ORCH-015 to BR-ORCH-021 as implementation details |
| 1.3 | 2025-12-01 | Added BR-ORCH-032/033/034 for resource lock deduplication handling (DD-RO-001) |
| 1.2 | 2025-12-01 | **BREAKING**: BR-ORCH-025 updated - RO does NOT call Data Storage API. HolmesGPT-API resolves workflow_id → containerImage during MCP search. RO passes through from AIAnalysis.status. Aligned with DD-CONTRACT-001 v1.2 (authoritative). |
| 1.1 | 2025-11-28 | Added BR-ORCH-029/030/031 for notification handling (cancellation, status tracking, cascade cleanup) |
| 1.0 | 2025-11-28 | Initial BR document with catalog integration and timeout requirements |

---

**Document Version**: 1.5
**Last Updated**: February 12, 2026
**Maintained By**: Kubernaut Architecture Team
**Status**: In Development

