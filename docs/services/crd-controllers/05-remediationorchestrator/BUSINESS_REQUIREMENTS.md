# RemediationOrchestrator Service - Business Requirements

**Service**: RemediationOrchestrator Controller
**Service Type**: CRD Controller
**CRD**: RemediationRequest
**Controller**: RemediationRequestReconciler
**Version**: 1.0
**Last Updated**: November 28, 2025
**Status**: In Development

---

## 📋 Overview

The **RemediationOrchestrator** is the central coordinator for the Kubernaut remediation lifecycle. It watches `RemediationRequest` CRDs and orchestrates the creation and monitoring of child CRDs (SignalProcessing, AIAnalysis, RemediationApprovalRequest, WorkflowExecution) to drive automated incident remediation.

### Service Responsibilities

1. **Lifecycle Orchestration**: Manage RemediationRequest phase transitions
2. **Child CRD Management**: Create and monitor SignalProcessing, AIAnalysis, WorkflowExecution
3. **Approval Orchestration**: Create RemediationApprovalRequest and NotificationRequest when needed
4. **Catalog Lookup**: Resolve workflow_id to container image via Data Storage API
5. **Timeout Enforcement**: Enforce global and per-phase timeouts
6. **Notification Triggering**: Create NotificationRequest CRDs for operator alerts

---

## 🎯 Business Requirements

### Category 1: Workflow Catalog Integration

#### BR-ORCH-025: Catalog Lookup Before WorkflowExecution

**Description**: RemediationOrchestrator MUST resolve `workflow_id` to `container_image` via Data Storage API before creating WorkflowExecution CRD, ensuring the workflow exists and capturing the image digest for audit.

**Priority**: P0 (CRITICAL)

**Rationale**: The AIAnalysis output contains `workflow_id` (catalog reference), not the actual container image. RemediationOrchestrator must resolve this reference before creating WorkflowExecution to ensure the workflow exists and capture immutable image reference for audit trail.

**Implementation**:
- Call Data Storage API: `GET /api/v1/workflows/{workflow_id}`
- Extract `container_image` and `container_digest` from response
- Populate WorkflowExecution.spec.workflowRef with resolved values
- Fail gracefully if workflow not found in catalog

**Acceptance Criteria**:
- ✅ Catalog lookup executed before WorkflowExecution creation
- ✅ `container_image` and `container_digest` populated in WorkflowExecution
- ✅ Workflow not found → RemediationRequest marked as Failed
- ✅ Catalog lookup errors retried with backoff

**Test Coverage**:
- Unit: Catalog client mock tests
- Integration: Data Storage API integration
- E2E: End-to-end workflow resolution

**Related DDs**: DD-CONTRACT-001 (AIAnalysis ↔ WorkflowExecution Alignment)
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
  - Add condition: `NotificationDelivered=False` with reason `UserCancelled`
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
  - `Sent` → `notificationStatus = "Sent"`, condition `NotificationDelivered=True`
  - `Failed` → `notificationStatus = "Failed"`, condition `NotificationDelivered=False` with reason `DeliveryFailed`
  - `Deleted` → `notificationStatus = "Cancelled"`, condition `NotificationDelivered=False` with reason `UserCancelled`
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
- [DD-RO-001: Notification Cancellation Handling](./DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md)
- [DD-NOT-005: NotificationRequest Spec Immutability](../06-notification/DD-NOT-005-SPEC-IMMUTABILITY.md)
- [ADR-017: NotificationRequest Creator](../../../architecture/decisions/ADR-017-notification-crd-creator.md)
- [ADR-040: RemediationApprovalRequest](../../../architecture/decisions/ADR-040-remediation-approval-request-architecture.md)

---

## 📝 Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-11-28 | Added BR-ORCH-029/030/031 for notification handling (cancellation, status tracking, cascade cleanup) |
| 1.0 | 2025-11-28 | Initial BR document with catalog integration and timeout requirements |

---

**Document Version**: 1.1
**Last Updated**: November 28, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: In Development

