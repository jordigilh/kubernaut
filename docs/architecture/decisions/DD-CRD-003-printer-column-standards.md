# DD-CRD-003: CRD Printer Column Standards

**Status**: ✅ APPROVED
**Decision Date**: 2026-02-05
**Version**: 1.0
**Authority Level**: FOUNDATIONAL
**Applies To**: All CRD controllers (AA, WE, RO, SP, Notification, RAR)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-05 | AI Assistant | Initial standard: per-CRD column registry, RR/SP additions, Reason column gap analysis |

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
| **Reason** | Why it's in this state | SHOULD (pending Ready condition, see Future Work) |

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

## Future Work: Ready Condition + Reason Column

### Gap Analysis (2026-02-05)

An audit of all 5 controllers revealed that **no CRD has a unified `Ready` condition**. Each uses phase-specific conditions instead (e.g., `InvestigationComplete`, `ExecutionCreated`, `RecoveryComplete`). This means a `Reason` printer column via `.status.conditions[?(@.type=="Ready")].reason` is not possible today.

### Conditions Audit Findings

| Controller | Condition Count | Has Ready? | Gaps |
|------------|----------------|------------|------|
| AIAnalysis | 5 | No | `InvestigationComplete=False` never set on failure |
| WorkflowExecution | 5 | No | `ResourceLocked` defined but never used |
| RemediationOrchestrator | 8 | No | `NotificationDelivered` uses inline strings; `RemediationExecuted` documented but not implemented |
| SignalProcessing | 4 | No | `ClassificationComplete=False` and `EnrichmentComplete=False` never set on failure paths |
| Notification | 1 | No | Fallback routing uses wrong reason constant; manual slice manipulation instead of `meta.SetStatusCondition` |

### Tracked In

- **GitHub Issue**: (see issue created for this effort)
- **DD-CRD-002**: `DD-CRD-002-kubernetes-conditions-standard.md` (existing conditions standard)
- **DD-EVENT-001**: `DD-EVENT-001-controller-event-registry.md` (event reason alignment)

### Implementation Path

1. Add a `Ready` condition type to each CRD's conditions helper
2. Fix consistency gaps (set conditions on failure paths)
3. Ensure `Ready` is updated on every meaningful state change
4. Add `Reason` printer column: `+kubebuilder:printcolumn:name="Reason",type=string,JSONPath='.status.conditions[?(@.type=="Ready")].reason'`
5. Regenerate CRD manifests

---

## Consequences

### Positive

- RemediationRequest is now usable in `kubectl get` for operational triage
- SignalProcessing shows the primary triage dimension (Severity)
- Cross-CRD standard prevents ad-hoc column proliferation
- Clear path to unified Reason column via Ready condition

### Negative

- Reason column deferred until conditions infrastructure is fixed (separate effort)
- Adding columns to existing CRDs changes `kubectl get` output format (minor disruption)

---

## Related Decisions

- **DD-CRD-001**: API group domain selection
- **DD-CRD-002**: Kubernetes conditions standard (conditions infrastructure)
- **DD-EVENT-001**: Controller Kubernetes Event Registry (event reason constants)
- **DD-CONTROLLER-001**: Observed generation idempotency pattern
