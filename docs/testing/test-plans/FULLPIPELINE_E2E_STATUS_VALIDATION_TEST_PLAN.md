# E2E Test Plan: Issue #118 (FullPipeline CRD Status Validation + RAR Approval Flow)

**Version**: 1.0.0
**Created**: 2026-02-17
**Status**: Active
**Authority**: [Issue #118](https://github.com/jordigilh/kubernaut/issues/118)
**Service**: FullPipeline (cross-cutting: SP, AA, RR, WE, NT, EA, RAR)
**Test Tier**: E2E (End-to-End)
**Template**: [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)

---

## Overview

The FullPipeline E2E test validates the complete remediation lifecycle but only asserts ~8 fields total. Across the 6 CRDs, there are ~80+ status fields that are never validated at the E2E tier, allowing silent regressions where fields stop being populated. Additionally, the RAR approval flow has no full-pipeline E2E coverage.

This test plan extends the existing FullPipeline E2E tests with comprehensive CRD status validation using a **collect-all-failures** pattern, and adds a new RAR approval flow scenario.

**Collect-All-Failures Pattern**: Each CRD gets a `ValidateXXXStatus()` function that checks every expected field and returns `[]string` of failures. Wrapped in `Eventually` for async tolerance:

```go
Eventually(func() []string {
    sp := &signalprocessingv1.SignalProcessing{}
    if err := k8sClient.Get(ctx, key, sp); err != nil {
        return []string{fmt.Sprintf("failed to get SP: %v", err)}
    }
    return ValidateSPStatus(sp)
}, 30*time.Second, 2*time.Second).Should(BeEmpty(),
    "SignalProcessing status validation failures")
```

When `Eventually` times out, the last `[]string` is printed -- showing every unpopulated/incorrect field at once.

**Test Environment**:
- Kind cluster (reuses existing FullPipeline infrastructure from `suite_test.go`)
- All 6 CRD controllers + Gateway + DataStorage + Mock LLM deployed
- Workflow seeding via DataStorage API
- Mock LLM with scenario UUID sync

**Test ID Convention**: `E2E-FP-118-{SEQUENCE}` per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md

---

## Testing Guidelines Compliance

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md):

### Business Outcome Focus

These tests validate that the **full remediation pipeline produces complete, correct CRD status records** -- the last line of defense against silent regressions where controllers stop populating fields without any test catching it. The business problem: if any CRD status field regresses (becomes nil, empty, or incorrect), downstream consumers (operators, audit, notifications, the RO orchestrator itself) receive incomplete data, potentially causing missed remediations, incorrect approvals, or incomplete audit trails.

### Anti-Pattern Compliance

- `time.Sleep()`: NOT USED. All async waits use `Eventually()` with explicit timeouts.
- `Skip()`: NOT USED. All tests either pass or fail.
- Direct audit infrastructure testing: NOT DONE. Audit trail validation (Step 12) verifies audit events as a **side effect** of the business flow (pipeline completion), not by calling audit store methods directly.
- HTTP testing: NOT APPLICABLE. E2E tier correctly uses full Kind cluster deployment.

### Validator Failure Messages

Validator failure messages describe the **business impact** of missing fields, not just technical absence. Example:

```go
// BAD: purely technical
"Status.EnvironmentClassification is nil"

// GOOD: describes downstream business impact
"SP: EnvironmentClassification not populated -- approval policy cannot evaluate environment, workflow matching may fail"
```

### Quality Gates

- [x] **Maps to documented business requirements**: BR-E2E-001, BR-ORCH-026, BR-ORCH-029, BR-AI-013
- [x] **Uses realistic data and scenarios**: Real OOMKill events, real controller reconciliation, real Rego policies
- [x] **Validates end-to-end outcomes**: Full pipeline from signal to effectiveness assessment
- [x] **Includes business success criteria**: All validators return zero failures

---

## Test Scenarios

### P0 -- Must Have (CRD Status Completeness)

| Test ID | Scenario | CRDs Validated | BRs |
|---------|----------|---------------|-----|
| E2E-FP-118-001 | Pipeline produces complete status records for all 6 CRDs (K8s event source) | SP, AA, RR, WE, NT, EA | BR-E2E-001 |
| E2E-FP-118-002 | Pipeline produces complete status records for all 6 CRDs (AlertManager source) | SP, AA, RR, WE, NT, EA | BR-E2E-001 |
| E2E-FP-118-003 | Production approval gate: pipeline pauses for operator approval and resumes correctly with complete status records across all 7 CRDs | SP, AA, RAR, RR, WE, NT, EA | BR-E2E-001, BR-ORCH-026, BR-AI-013 |

### P1 -- Should Have (Approval-Specific Validation)

| Test ID | Scenario | Focus | BRs |
|---------|----------|-------|-----|
| E2E-FP-118-004 | Operator receives sufficient context in the RAR to make an informed approval decision | RAR spec fields | BR-ORCH-026 |
| E2E-FP-118-005 | Operator is notified when approval is required so they can act before the timeout | NT (approval type) | BR-ORCH-029 |
| E2E-FP-118-006 | Approval decisions are recorded in the audit trail for compliance accountability | Audit events | BR-ORCH-026 |

Note: P1 scenarios E2E-FP-118-004 through 006 are validation steps within the E2E-FP-118-003 test, not separate `It` blocks.

---

## Test Scenario Details

### E2E-FP-118-001: Happy Path CRD Status Completeness (K8s Event)

**Business Outcome**: The full remediation pipeline produces complete, correct CRD status records at every stage, ensuring that downstream consumers (operators, audit systems, notification routing, orchestrator decisions) receive the data they depend on. Prevents silent regressions where a controller change causes fields to stop being populated without any test catching it.

**Approach**: Extends the existing K8s OOMKill event test in `01_full_remediation_lifecycle_test.go` by adding validator calls after each phase completion. Validators check all fields documented in the Per-CRD Field Inventory and return business-impact-aware failure messages.

**Preconditions**:
- Existing FullPipeline infrastructure deployed
- Workflow seeding and Mock LLM configured (existing BeforeEach)

**Steps (additions to existing test)**:
1. After SP Phase=Completed: call `ValidateSPStatus(sp)` -- verifies enrichment, classification, and categorization produced complete output for downstream AI analysis
2. After AA Phase=Completed: call `ValidateAAStatus(aa)` -- verifies investigation, RCA, workflow selection, and PostRCAContext produced complete output for workflow execution
3. After WE Phase=Completed: call `ValidateWEStatus(we)` -- verifies execution tracking fields are complete for audit trail and SLA monitoring
4. After NT created: call `ValidateNTStatus(nr)` -- verifies notification lifecycle fields are complete for delivery tracking
5. After RR OverallPhase=Completed: call `ValidateRRStatus(rr)` -- verifies all cross-references and outcome fields are set for audit reconstruction
6. After EA terminal phase: call `ValidateEAStatus(ea)` -- verifies assessment components produced results for effectiveness tracking

**Pass Criteria**: All validators return empty `[]string` (zero failures) for every CRD. A non-empty return indicates a field that downstream business logic depends on is not being populated.

---

### E2E-FP-118-002: Happy Path CRD Status Completeness (AlertManager)

**Business Outcome**: Same as E2E-FP-118-001 but for the AlertManager signal source path.

**Approach**: Extends the existing AlertManager alert test in `01_full_remediation_lifecycle_test.go` with the same validator calls.

**Steps**: Same as E2E-FP-118-001 but exercised via AlertManager signal injection.

**Pass Criteria**: All validators return empty `[]string` for every CRD.

---

### E2E-FP-118-003: RAR Approval Flow Full Pipeline

**Business Outcome**: Production environments require human approval before automated remediation executes (BR-AI-013). The full pipeline must correctly: (1) detect the production environment, (2) require approval via Rego policy, (3) create a RAR with sufficient context for the operator to decide, (4) send an approval notification, (5) resume the pipeline after operator approval, and (6) produce complete status records across all 7 CRDs. This validates that the approval gate does not corrupt or omit any CRD status data compared to the auto-approved path.

**Preconditions**:
- Existing FullPipeline infrastructure deployed
- Workflow seeding and Mock LLM configured (existing BeforeEach)

**Approval Trigger Mechanism**:
- Namespace label `kubernaut.ai/environment: production` causes SP to classify environment as `"production"` via Rego policy
- RO propagates `environment: "production"` to AA's SignalContext
- AA's Rego approval policy: `input.environment == "production"` triggers `require_approval = true` (BR-AI-013)
- Mock LLM matches workflows by `workflow_name` only (environment-agnostic), so existing workflow seeding works

**Steps**:

```
Step 1: Create namespace "fp-rar-{timestamp}" with labels:
          kubernaut.ai/managed: "true"
          kubernaut.ai/environment: "production"
Step 2: Deploy memory-eater pod (OOMKill trigger)
Step 2b: Wait for OOMKill detected
Step 3: Wait for RemediationRequest created in namespace
Step 4: Wait for SP Phase=Completed
Step 4b: ValidateSPStatus(sp) -- environment="production", source="namespace-labels"
Step 5: Wait for AA Phase=Completed
Step 5b: ValidateAAStatus(aa, WithApprovalFlow()) -- ApprovalRequired=true
Step 6: Wait for RR OverallPhase=AwaitingApproval
Step 7: Wait for RAR created (name: rar-{rr.Name})
Step 7b: ValidateRARSpec(rar) [E2E-FP-118-004]
Step 7c: Wait for approval NR (name: nr-approval-{rr.Name}) [E2E-FP-118-005]
Step 7d: ValidateNTStatus(approvalNR) -- verify approval notification lifecycle fields
Step 8: Approve RAR via k8sClient.Status().Update()
Step 8b: ValidateRARStatus(rar) -- verify controller-set fields (CreatedAt, Expired, Conditions) + test-set fields (Decision, DecidedBy, DecidedAt)
Step 9: Wait for WE created + Phase=Completed
Step 9b: ValidateWEStatus(we)
Step 10: Wait for completion NR (Type: Completion)
Step 10b: ValidateNTStatus(completionNR)
Step 11: Wait for RR OverallPhase=Completed
Step 11b: ValidateRRStatus(rr, WithApprovalFlow())
Step 12: Audit trail validation [E2E-FP-118-006]
         Required events: orchestrator.approval.requested, orchestrator.approval.approved + standard pipeline events
Step 13: Wait for EA terminal phase
Step 13b: ValidateEAStatus(ea)
```

**RAR Approval Code**:

```go
rar.Status.Decision = remediationv1.ApprovalDecisionApproved
rar.Status.DecidedBy = "e2e-test-operator@example.com"
now := metav1.Now()
rar.Status.DecidedAt = &now
rar.Status.DecisionMessage = "E2E full-pipeline approval test"
Expect(k8sClient.Status().Update(testCtx, rar)).To(Succeed())
```

**Pass Criteria**:
- All 7 CRD validators return empty `[]string`
- RAR spec fields match expected values (from AA status)
- Approval NR created with correct type and metadata
- Audit trail includes approval-specific events
- Pipeline completes successfully after approval

---

### E2E-FP-118-004: Operator Approval Context (sub-step of E2E-FP-118-003)

**Business Outcome**: When a production remediation requires approval, the operator must receive sufficient context to make an informed decision -- the recommended workflow, confidence level, investigation summary, and why approval was required. Without these fields, the operator is forced to approve or reject blindly, defeating the purpose of the approval gate (BR-ORCH-026).

**Validated within**: Step 7b of E2E-FP-118-003

**Pass Criteria**: `ValidateRARSpec(rar)` returns empty `[]string`. All spec fields contain meaningful data derived from the AIAnalysis output.

---

### E2E-FP-118-005: Approval Notification Delivery (sub-step of E2E-FP-118-003)

**Business Outcome**: When approval is required, the operator must be notified so they can act before the approval timeout expires. If the notification is not created, the RAR expires silently and the remediation fails without the operator ever knowing it needed attention (BR-ORCH-029).

**Validated within**: Step 7c of E2E-FP-118-003

**Pass Criteria**: NotificationRequest with name `nr-approval-{rr.Name}` and type `NotificationTypeApproval` exists in the namespace.

---

### E2E-FP-118-006: Approval Audit Compliance (sub-step of E2E-FP-118-003)

**Business Outcome**: Every approval decision must be recorded in the audit trail for compliance accountability and post-incident review. Without these audit events, there is no record of who approved a production remediation, when, or why -- a compliance gap (BR-ORCH-026).

**Validated within**: Step 12 of E2E-FP-118-003

**Pass Criteria**: Audit trail contains `orchestrator.approval.requested` and `orchestrator.approval.approved` event types linked to the remediation's correlation ID.

---

## Per-CRD Status Field Inventory

### SignalProcessing (SP)

| # | Field | Check | Happy Path | Approval Flow |
|---|-------|-------|:----------:|:-------------:|
| 1 | `Status.Phase` | == `"Completed"` | Y | Y |
| 2 | `Status.StartTime` | non-nil | Y | Y |
| 3 | `Status.CompletionTime` | non-nil | Y | Y |
| 4 | `Status.ObservedGeneration` | > 0 | Y | Y |
| 5 | `Status.KubernetesContext` | non-nil | Y | Y |
| 6 | `Status.KubernetesContext.Namespace` | non-nil | Y | Y |
| 7 | `Status.KubernetesContext.Workload` | non-nil | Y | Y |
| 8 | `Status.KubernetesContext.Workload.Kind` | non-empty | Y | Y |
| 9 | `Status.KubernetesContext.Workload.Name` | non-empty | Y | Y |
| 10 | `Status.EnvironmentClassification` | non-nil | Y | Y |
| 11 | `Status.EnvironmentClassification.Environment` | non-empty | Y | Y |
| 12 | `Status.EnvironmentClassification.Source` | non-empty | Y | Y |
| 13 | `Status.EnvironmentClassification.ClassifiedAt` | non-zero time | Y | Y |
| 14 | `Status.PriorityAssignment` | non-nil | Y | Y |
| 15 | `Status.PriorityAssignment.Priority` | non-empty | Y | Y |
| 16 | `Status.PriorityAssignment.Source` | non-empty | Y | Y |
| 17 | `Status.PriorityAssignment.AssignedAt` | non-zero time | Y | Y |
| 18 | `Status.BusinessClassification` | non-nil | Y | Y |
| 19 | `Status.Severity` | non-empty | Y | Y |
| 20 | `Status.SignalMode` | non-empty | Y | Y |
| 21 | `Status.SignalType` | non-empty | Y | Y |
| 22 | `Status.PolicyHash` | non-empty | Y | Y |
| 23 | `Status.Conditions` | len > 0 | Y | Y |

**Skipped (enrichment-depth dependent)**: `KubernetesContext.OwnerChain` (depends on resource type and cluster state), `KubernetesContext.CustomLabels` (depends on Rego policy output), `KubernetesContext.DegradedMode` (false for normal enrichment, not worth asserting)

**Skipped (error/recovery path only)**: `Error`, `ConsecutiveFailures`, `LastFailureTime`, `RecoveryContext` (nil for first attempt)

**Skipped (conditional)**: `OriginalSignalType` -- only populated for predictive signals (`SignalMode == "predictive"`); empty for reactive signals like OOMKill. All E2E tests use reactive signals.

---

### AIAnalysis (AA)

| # | Field | Check | Happy Path | Approval Flow |
|---|-------|-------|:----------:|:-------------:|
| 1 | `Status.Phase` | == `"Completed"` | Y | Y |
| 2 | `Status.StartedAt` | non-nil | Y | Y |
| 3 | `Status.CompletedAt` | non-nil | Y | Y |
| 4 | `Status.ObservedGeneration` | > 0 | Y | Y |
| 5 | `Status.RootCauseAnalysis` | non-nil | Y | Y |
| 6 | `Status.RootCauseAnalysis.Summary` | non-empty | Y | Y |
| 7 | `Status.RootCauseAnalysis.Severity` | non-empty | Y | Y |
| 8 | `Status.RootCauseAnalysis.SignalType` | non-empty | Y | Y |
| 9 | `Status.SelectedWorkflow` | non-nil | Y | Y |
| 10 | `Status.SelectedWorkflow.WorkflowID` | non-empty | Y | Y |
| 11 | `Status.SelectedWorkflow.ExecutionEngine` | non-empty | Y | Y |
| 12 | `Status.SelectedWorkflow.Confidence` | > 0 | Y | Y |
| 13 | `Status.SelectedWorkflow.Version` | non-empty | Y | Y |
| 14 | `Status.SelectedWorkflow.ExecutionBundle` | non-empty | Y | Y |
| 15 | `Status.SelectedWorkflow.Rationale` | non-empty | Y | Y |
| 16 | `Status.InvestigationID` | non-empty | Y | Y |
| 17 | `Status.TotalAnalysisTime` | > 0 | Y | Y |
| 18 | `Status.InvestigationSession` | non-nil | Y | Y |
| 19 | `Status.InvestigationSession.ID` | non-empty | Y | Y |
| 20 | `Status.PostRCAContext` | non-nil | Y | Y |
| 21 | `Status.PostRCAContext.DetectedLabels` | non-nil | Y | Y |
| 22 | `Status.PostRCAContext.SetAt` | non-nil | Y | Y |
| 23 | `Status.Conditions` | len > 0 | Y | Y |
| 24 | `Status.ApprovalRequired` | == true | N | Y |
| 25 | `Status.ApprovalReason` | non-empty | N | Y |
| 26 | `Status.ApprovalContext` | non-nil | N | Y |
| 27 | `Status.ApprovalContext.Reason` | non-empty | N | Y |
| 28 | `Status.ApprovalContext.WhyApprovalRequired` | non-empty | N | Y |
| 29 | `Status.ApprovalContext.ConfidenceScore` | > 0 | N | Y |
| 30 | `Status.ApprovalContext.ConfidenceLevel` | non-empty | N | Y |
| 31 | `Status.ApprovalContext.InvestigationSummary` | non-empty | N | Y |
| 32 | `Status.ApprovalContext.RecommendedActions` | len > 0 | N | Y |

**Skipped**: `RootCause` (legacy summary string, superseded by `RootCauseAnalysis.Summary`), `InvestigationTime` (HAPI-specific duration, may not be populated in all paths; `TotalAnalysisTime` covers the aggregate), `DegradedMode`, `RecoveryStatus`, `ValidationAttemptsHistory`, `Warnings` (optional), `AlternativeWorkflows` (optional), `Message`, `Reason`, `SubReason`, `NeedsHumanReview`, `HumanReviewReason`, `ConsecutiveFailures`

---

### RemediationRequest (RR)

| # | Field | Check | Happy Path | Approval Flow |
|---|-------|-------|:----------:|:-------------:|
| 1 | `Status.OverallPhase` | == `"Completed"` | Y | Y |
| 2 | `Status.StartTime` | non-nil | Y | Y |
| 3 | `Status.CompletedAt` | non-nil | Y | Y |
| 4 | `Status.ObservedGeneration` | > 0 | Y | Y |
| 5 | `Status.SignalProcessingRef` | non-nil | Y | Y |
| 6 | `Status.AIAnalysisRef` | non-nil | Y | Y |
| 7 | `Status.WorkflowExecutionRef` | non-nil | Y | Y |
| 8 | `Status.NotificationRequestRefs` | len >= 1 | Y | N |
| 9 | `Status.NotificationRequestRefs` | len >= 2 | N | Y |
| 10 | `Status.EffectivenessAssessmentRef` | non-nil | Y | Y |
| 11 | `Status.Outcome` | non-empty | Y | Y |
| 12 | `Status.SelectedWorkflowRef` | non-nil | Y | Y |
| 13 | `Status.SelectedWorkflowRef.WorkflowID` | non-empty | Y | Y |
| 14 | `Status.SelectedWorkflowRef.Version` | non-empty | Y | Y |
| 15 | `Status.SelectedWorkflowRef.ExecutionBundle` | non-empty | Y | Y |
| 16 | `Status.Conditions` | len > 0 | Y | Y |
| 17 | `Status.ApprovalNotificationSent` | == true | N | Y |

**Skipped (error/edge-case paths)**: `Deduplication`, `Message`, `ProcessingStartTime`, `AnalyzingStartTime`, `ExecutingStartTime`, `RemediationProcessingRef`, `PreRemediationSpecHash`, `SkipReason`, `SkipMessage`, `BlockingWorkflowExecution`, `DuplicateOf`, `DuplicateCount`, `DuplicateRefs`, `BlockReason`, `BlockMessage`, `BlockedUntil`, `NextAllowedExecution`, `ConsecutiveFailureCount`, `FailurePhase`, `FailureReason`, `RequiresManualReview`, `TimeoutPhase`, `TimeoutTime`, `RetentionExpiryTime`, `NotificationStatus`, `TimeoutConfig`, `LastModifiedBy`, `LastModifiedAt`, `RecoveryAttempts`, `CurrentProcessingRef`, `ExecutionRef`

---

### WorkflowExecution (WE)

| # | Field | Check | Happy Path | Approval Flow |
|---|-------|-------|:----------:|:-------------:|
| 1 | `Status.Phase` | == `"Completed"` | Y | Y |
| 2 | `Status.StartTime` | non-nil | Y | Y |
| 3 | `Status.CompletionTime` | non-nil | Y | Y |
| 4 | `Status.ObservedGeneration` | > 0 | Y | Y |
| 5 | `Status.Duration` | non-empty | Y | Y |
| 6 | `Status.ExecutionRef` | non-nil | Y | Y |
| 7 | `Status.ExecutionStatus` | non-nil | Y | Y |
| 8 | `Status.ExecutionStatus.Status` | non-empty | Y | Y |
| 9 | `Status.Conditions` | len > 0 | Y | Y |

**Skipped (failure/block paths)**: `FailureReason` (deprecated), `FailureDetails`, `BlockClearance`

---

### NotificationRequest (NT)

| # | Field | Check | Happy Path | Approval Flow |
|---|-------|-------|:----------:|:-------------:|
| 1 | `Status.Phase` | in terminal state (Sent, PartiallySent, Failed) | Y | Y |
| 2 | `Status.QueuedAt` | non-nil | Y | Y |
| 3 | `Status.ProcessingStartedAt` | non-nil | Y | Y |
| 4 | `Status.CompletionTime` | non-nil | Y | Y |
| 5 | `Status.ObservedGeneration` | > 0 | Y | Y |
| 6 | `Status.TotalAttempts` | >= 1 | Y | Y |
| 7 | `Status.DeliveryAttempts` | len >= 1 | Y | Y |
| 8 | `Status.Conditions` | len > 0 | Y | Y |

**Skipped**: `SuccessfulDeliveries` (may be 0 if channels not configured), `FailedDeliveries`, `Reason`, `Message`

---

### EffectivenessAssessment (EA)

| # | Field | Check | Happy Path | Approval Flow |
|---|-------|-------|:----------:|:-------------:|
| 1 | `Status.Phase` | in terminal state (Completed, Failed) | Y | Y |
| 2 | `Status.CompletedAt` | non-nil | Y | Y |
| 3 | `Status.Components.HealthAssessed` | == true | Y | Y |
| 4 | `Status.Components.HashComputed` | == true | Y | Y |
| 5 | `Status.Components.PostRemediationSpecHash` | non-empty | Y | Y |
| 6 | `Status.Components.CurrentSpecHash` | non-empty | Y | Y |
| 7 | `Status.Components.HealthScore` | non-nil | Y | Y |
| 8 | `Status.Conditions` | len > 0 | Y | Y |

**Skipped (depend on Prometheus/AlertManager configuration)**: `ValidityDeadline`, `PrometheusCheckAfter`, `AlertManagerCheckAfter`, `Components.AlertAssessed`, `Components.AlertScore`, `Components.MetricsAssessed`, `Components.MetricsScore`, `AssessmentReason`, `Message`

---

### RemediationApprovalRequest (RAR) -- Approval Flow Only

#### RAR Status (after approval)

| # | Field | Check | Approval Flow |
|---|-------|-------|:-------------:|
| 1 | `Status.Decision` | == `"Approved"` | Y |
| 2 | `Status.DecidedBy` | non-empty | Y |
| 3 | `Status.DecidedAt` | non-nil | Y |
| 4 | `Status.CreatedAt` | non-nil | Y |
| 5 | `Status.Expired` | == false | Y |
| 6 | `Status.Conditions` | len > 0 | Y |

**Skipped**: `DecisionMessage` (optional, set by approver -- we set it in the test but it's not a controller-populated field), `TimeRemaining` (periodically updated, may be empty after decision), `ObservedGeneration` (RAR reconciler is audit-only, may not update this), `Reason` (machine-readable status reason), `Message` (human-readable status message)

#### RAR Spec (validated at creation)

| # | Field | Check | Approval Flow |
|---|-------|-------|:-------------:|
| 1 | `Spec.RemediationRequestRef.Name` | non-empty | Y |
| 2 | `Spec.AIAnalysisRef.Name` | non-empty | Y |
| 3 | `Spec.Confidence` | > 0 | Y |
| 4 | `Spec.ConfidenceLevel` | non-empty (low, medium, high) | Y |
| 5 | `Spec.Reason` | non-empty | Y |
| 6 | `Spec.RecommendedWorkflow.WorkflowID` | non-empty | Y |
| 7 | `Spec.RecommendedWorkflow.Version` | non-empty | Y |
| 8 | `Spec.RecommendedWorkflow.ExecutionBundle` | non-empty | Y |
| 9 | `Spec.RecommendedWorkflow.Rationale` | non-empty | Y |
| 10 | `Spec.InvestigationSummary` | non-empty | Y |
| 11 | `Spec.RecommendedActions` | len > 0 | Y |
| 12 | `Spec.WhyApprovalRequired` | non-empty | Y |
| 13 | `Spec.RequiredBy` | non-zero time | Y |

**Skipped (optional)**: `EvidenceCollected` (optional slice, may be empty depending on investigation depth), `AlternativesConsidered` (optional slice, populated only when multiple approaches were evaluated), `PolicyEvaluation` (optional pointer, populated only when Rego policy evaluation details are available)

---

## Validator Framework

### Implementation

**File**: `test/shared/validators/crd_status.go`

**Unit Tests**: `test/unit/validators/crd_status_test.go`

### Options Pattern

```go
type ValidationOption func(*validationConfig)
type validationConfig struct { approvalFlow bool }
func WithApprovalFlow() ValidationOption { ... }
```

### Common Helpers

Helpers accept a `field` name and a `impact` string describing the downstream business consequence of an unpopulated field. The impact string appears in the failure message.

```go
func checkNonNil(field, impact string, val interface{}) string
func checkNonEmpty(field, impact string, val string) string
func checkTimeSet(field, impact string, t *metav1.Time) string
func checkNonZeroTime(field, impact string, t metav1.Time) string
func checkConditions(field, impact string, c []metav1.Condition) string
```

`checkTimeSet` handles `*metav1.Time` pointer fields (e.g., `StartTime`, `CompletedAt`). `checkNonZeroTime` handles non-pointer `metav1.Time` fields (e.g., `ClassifiedAt`, `AssignedAt`, `RequiredBy`) where the zero value is `time.Time{}`.

Numeric, boolean, phase, and slice-length checks use inline logic within each validator function rather than shared helpers, since their patterns vary per field (e.g., `> 0`, `>= 1`, `== true`, `== false`, `in [Sent, PartiallySent, Failed]`).

Example usage:
```go
checkNonNil("SP: EnvironmentClassification",
    "approval policy cannot evaluate environment, workflow matching may fail",
    sp.Status.EnvironmentClassification)
// Failure: "SP: EnvironmentClassification not populated -- approval policy cannot evaluate environment, workflow matching may fail"
```

### Functions

| Function | CRD | Options |
|----------|-----|---------|
| `ValidateSPStatus(sp)` | SignalProcessing | none |
| `ValidateAAStatus(aa, opts...)` | AIAnalysis | `WithApprovalFlow()` |
| `ValidateRRStatus(rr, opts...)` | RemediationRequest | `WithApprovalFlow()` |
| `ValidateWEStatus(we)` | WorkflowExecution | none |
| `ValidateNTStatus(nr)` | NotificationRequest | none |
| `ValidateEAStatus(ea)` | EffectivenessAssessment | none |
| `ValidateRARStatus(rar)` | RemediationApprovalRequest | none |
| `ValidateRARSpec(rar)` | RemediationApprovalRequest | none |

### Validators vs Scenario-Specific Assertions

Validators check that fields are **populated** (non-nil, non-empty, non-zero). They do NOT check specific values. Scenario-specific value assertions are explicit `Expect` calls in the test, placed after the validator call:

```go
// Generic validator: checks all fields are populated
Eventually(func() []string { return ValidateSPStatus(sp) }, ...).Should(BeEmpty())

// Scenario-specific: approval flow expects production environment
Expect(sp.Status.EnvironmentClassification.Environment).To(Equal("production"),
    "approval flow requires production environment classification")
Expect(sp.Status.EnvironmentClassification.Source).To(Equal("namespace-labels"),
    "environment should be classified from namespace labels")
```

This keeps validators reusable across all scenarios while allowing each scenario to assert the values it cares about.

---

## Ginkgo Test Description Guidance

`It` block descriptions must describe the business behavior being validated, not the technical mechanism. Include the test scenario ID.

```go
// GOOD: describes business outcome
It("should produce complete status records across all pipeline stages for downstream consumers [E2E-FP-118-001]", func() { ... })
It("should pause the pipeline for operator approval in production and resume correctly after approval [E2E-FP-118-003]", func() { ... })

// BAD: describes technical mechanism
It("should validate all SP status fields after completion", func() { ... })
It("should check that RAR spec fields are non-nil", func() { ... })
```

---

## Infrastructure Requirements

### Existing (no changes needed)

- Kind cluster with FullPipeline infrastructure
- All 6 CRD controllers deployed
- Gateway, DataStorage, Mock LLM deployed
- Workflow seeding via DataStorage API
- CRD scheme registration (RAR is in `remediationv1` package, already registered)

### New

- Test namespace for RAR flow: `fp-rar-{timestamp}` with label `kubernaut.ai/environment: production`
- No additional container images or RBAC changes needed

---

## Test File Locations

| File | Purpose |
|------|---------|
| `test/shared/validators/crd_status.go` | Validator functions (all 8) |
| `test/unit/validators/crd_status_test.go` | Unit tests for validators |
| `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` | Extended with validator calls (E2E-FP-118-001, E2E-FP-118-002) |
| `test/e2e/fullpipeline/02_approval_lifecycle_test.go` | RAR approval flow test (E2E-FP-118-003 through E2E-FP-118-006) |

---

## Dependencies

- [x] FullPipeline E2E infrastructure (existing)
- [x] All CRD controllers implemented (existing)
- [x] Mock LLM with ADR-056 4-step discovery (existing)
- [x] Workflow seeding infrastructure (existing)
- [x] RAR scheme registration via `remediationv1.AddToScheme` (existing)
- [x] Validator framework (`test/shared/validators/crd_status.go`)
- [x] RAR approval flow test (`02_approval_lifecycle_test.go`)
- [x] **Issue #113** -- SP KubernetesContext schema finalized (merged). Assertions now use `sharedtypes.KubernetesContext` with `Namespace`, `Workload` (generic `WorkloadDetails`), and `OwnerChain`.

---

## References

- **Issue**: [#118 - Extend FullPipeline E2E tests to validate CRD status fields](https://github.com/jordigilh/kubernaut/issues/118)
- **Dependency**: [#113 - Eliminate CRD-to-shared-type duplication](https://github.com/jordigilh/kubernaut/issues/113) (SP KubernetesContext schema change)
- **Current test**: `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go`
- **CRD definitions**: `api/*/v1alpha1/*_types.go`
- **Rego approval policy**: `config/rego/aianalysis/approval.rego`
- **Rego environment policy**: `test/infrastructure/signalprocessing_e2e_hybrid.go`
- **RAR approval pattern**: `test/e2e/remediationorchestrator/approval_e2e_test.go`
- **ADR-056**: PostRCAContext/DetectedLabels flow
