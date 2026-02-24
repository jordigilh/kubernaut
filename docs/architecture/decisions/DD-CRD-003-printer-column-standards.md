# DD-CRD-003: CRD Printer Column Standards

**Status**: ✅ APPROVED
**Decision Date**: 2026-02-05
**Version**: 1.0
**Authority Level**: FOUNDATIONAL
**Applies To**: All CRD controllers (AA, WE, RO, SP, Notification, RAR). KubernetesExecution excluded (DEPRECATED - ADR-025).

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-05 | AI Assistant | Initial standard: per-CRD column registry, RR/SP additions, Reason column gap analysis |
| 1.1 | 2026-02-18 | AI Assistant | Issue #79: Ready + Reason column IMPLEMENTED; ResourceLocked/RemediationExecuted removed from gaps; NotificationDelivered uses centralized constants |

---

## Context & Problem

Kubernaut CRDs use `+kubebuilder:printcolumn` markers to define what operators see in `kubectl get <crd>` output. This is the primary operational interface for quick triage. However:

1. **RemediationRequest has zero printer columns**: The most important CRD in the system (the orchestrator) shows only NAME and AGE in `kubectl get rr`. Operators cannot see Phase or Outcome without `kubectl describe`.
2. **SignalProcessing omits Severity**: The primary triage dimension for signals is not visible in `kubectl get`.
3. **No Reason column**: None of the CRDs surface *why* they are in their current state. This requires a unified `Ready` condition (see Future Work).
4. **No standard**: Each CRD defines columns ad-hoc with no cross-cutting guideline.

### Ecosystem Comparison

Mature Kubernetes operators follow a consistent pattern:

| Operator | Columns |
|----------|---------|
| **Tekton PipelineRun** | Succeeded, Reason, StartTime, CompletionTime |
| **Argo Workflow** | Status, Age, Duration, Priority |
| **cert-manager Certificate** | Ready, Secret, Age |
| **Knative Service** | URL, Ready, Reason |

The common pattern is: **Phase/Status + Reason + Age**, with domain-specific columns added as needed.

---

## Decision

### 1. Standard Columns

Every Kubernaut CRD SHOULD include at minimum:

| Column | Purpose | Required? |
|--------|---------|-----------|
| **Phase** | Current lifecycle state | MUST (if CRD has phases) |
| **Age** | Time since creation | MUST |
| **Reason** | Why it's in this state | ✅ IMPLEMENTED (Ready condition + printer column on all 7 CRDs) |

Domain-specific columns (Confidence, WorkflowID, Severity, etc.) are added per-CRD as needed.

### 2. Column Width Guideline

`kubectl get` output should fit in a standard terminal (120 columns). Limit to 5-6 columns maximum per CRD. Prefer short scalar values; avoid columns that are frequently empty or excessively wide.

### 3. Priority for Column Selection

When choosing which fields to surface:

1. **Always populated, always useful** (Phase, Age) -- highest priority
2. **Primary triage dimension** (Severity, Outcome, Decision) -- high priority
3. **Populated only on specific states** (FailureReason, BlockReason) -- lower priority, better in `describe`

---

## Printer Column Registry

### AIAnalysis

| Column | JSONPath | Type | Status |
|--------|----------|------|--------|
| Phase | `.status.phase` | string | Existing |
| Confidence | `.status.selectedWorkflow.confidence` | number | Existing |
| ApprovalRequired | `.status.approvalRequired` | boolean | Existing |
| Age | `.metadata.creationTimestamp` | date | Existing |

Assessment: Good coverage. Confidence and ApprovalRequired are high-value domain columns.

### WorkflowExecution

| Column | JSONPath | Type | Status |
|--------|----------|------|--------|
| Phase | `.status.phase` | string | Existing |
| WorkflowID | `.spec.workflowRef.workflowId` | string | Existing |
| Target | `.spec.targetResource` | string | Existing |
| Duration | `.status.duration` | string | Existing |
| Age | `.metadata.creationTimestamp` | date | Existing |

Assessment: Good coverage. 5 columns with strong operational value.

### RemediationRequest

| Column | JSONPath | Type | Status |
|--------|----------|------|--------|
| Phase | `.status.overallPhase` | string | **Added (v1.0)** |
| Outcome | `.status.outcome` | string | **Added (v1.0)** |
| Age | `.metadata.creationTimestamp` | date | **Added (v1.0)** |

Assessment: Previously had zero columns. Phase + Outcome + Age provides essential triage capability. Outcome is empty while in-progress (clean) and shows Success/Failure/Timeout/Skipped at terminal.

### RemediationApprovalRequest

| Column | JSONPath | Type | Status |
|--------|----------|------|--------|
| AIAnalysis | `.spec.aiAnalysisRef.name` | string | Existing |
| Confidence | `.spec.confidence` | number | Existing |
| Decision | `.status.decision` | string | Existing |
| Expired | `.status.expired` | boolean | Existing |
| RequiredBy | `.spec.requiredBy` | date | Existing |
| Age | `.metadata.creationTimestamp` | date | Existing |

Assessment: Excellent coverage. Decision effectively serves as the Reason column for this CRD.

### SignalProcessing

| Column | JSONPath | Type | Status |
|--------|----------|------|--------|
| Phase | `.status.phase` | string | Existing |
| Severity | `.status.severity` | string | **Added (v1.0)** |
| Environment | `.status.environmentClassification.environment` | string | Existing |
| Priority | `.status.priorityAssignment.priority` | string | Existing |
| Age | `.metadata.creationTimestamp` | date | Existing |

Assessment: Severity added as the primary triage dimension. Operators filter and sort by severity to prioritize response.

### NotificationRequest

| Column | JSONPath | Type | Status |
|--------|----------|------|--------|
| Type | `.spec.type` | string | Existing |
| Priority | `.spec.priority` | string | Existing |
| Phase | `.status.phase` | string | Existing |
| Attempts | `.status.totalAttempts` | integer | Existing |
| Age | `.metadata.creationTimestamp` | date | Existing |

Assessment: Good coverage. 5 columns with clear operational value.

---

## Ready Condition + Reason Column (IMPLEMENTED -- Issue #79)

### Implementation Status (2026-02-18)

Issue #79 implemented a unified `Ready` condition across all 7 active CRD types and added a `Reason` printer column using `.status.conditions[?(@.type=="Ready")].reason`. KubernetesExecution is deprecated and excluded.

### Conditions Audit (Post Issue #79)

| Controller | Condition Count | Has Ready? | Status |
|------------|----------------|------------|--------|
| AIAnalysis | 6 | Yes | `InvestigationComplete=False` wired on all failure paths; `WorkflowResolved`/`ApprovalRequired` gap fixed |
| WorkflowExecution | 5 | Yes | `ResourceLocked` dead code removed; `ObservedGeneration` fixed |
| RemediationOrchestrator (RR) | 9 | Yes | `NotificationDelivered` uses centralized constants; `RemediationExecuted` removed (never implemented); `RecoveryComplete` gap fixed on timeout/blocked-terminal paths |
| RemediationOrchestrator (RAR) | 4 | Yes | Ready wired on Approved/Rejected/Expired paths |
| SignalProcessing | 5 | Yes | `ClassificationComplete=False` and `EnrichmentComplete=False` wired on failure paths |
| Notification | 2 | Yes | `RoutingResolved` persistence bug fixed (conditions parameter); fallback reason bug fixed; `meta.SetStatusCondition` used |
| EffectivenessAssessment | 2 | Yes | Ready wired; `ObservedGeneration` fixed |

### Changes Made

- Added `ConditionReady`, `ReasonReady`, `ReasonNotReady`, and `SetReady()` to all 7 condition packages
- Fixed `ObservedGeneration` in all condition setters across 7 packages (was missing in 6)
- Added `Reason` printer column to all 7 CRDs (EA uses `ReadyReason` to avoid conflict with existing `assessmentReason` column)
- Fixed `phase.IsTerminal()` pre-existing bug (missing `PhaseCancelled`)
- Added terminal housekeeping safety net for externally-set terminal phases (e.g., `Cancelled`)

---

## Consequences

### Positive

- RemediationRequest is now usable in `kubectl get` for operational triage
- SignalProcessing shows the primary triage dimension (Severity)
- Cross-CRD standard prevents ad-hoc column proliferation
- Unified `Ready` condition and `Reason` printer column implemented across all 7 CRDs (Issue #79)

### Negative

- Adding columns to existing CRDs changes `kubectl get` output format (minor disruption)

---

## Related Decisions

- **DD-CRD-001**: API group domain selection
- **DD-CRD-002**: Kubernetes conditions standard (conditions infrastructure)
- **DD-EVENT-001**: Controller Kubernetes Event Registry (event reason constants)
- **DD-CONTROLLER-001**: Observed generation idempotency pattern
