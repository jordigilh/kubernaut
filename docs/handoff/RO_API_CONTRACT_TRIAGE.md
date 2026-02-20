# Remediation Orchestrator API Contract Triage

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed: DetectedLabels is now computed by HAPI post-RCA (ADR-056), and OwnerChain is resolved via get_resource_context (ADR-055).

**Date**: December 1, 2025
**Version**: 1.9
**Status**: ✅ **ALL EXTERNAL GAPS RESOLVED** - RO implementation requirements identified

## Executive Summary

This document triages all API contracts between the Remediation Orchestrator (RO) and the CRD services it interacts with. The analysis identified **12 gaps/inconsistencies** requiring resolution before implementation.

| Severity | Count | Description |
|----------|-------|-------------|
| 🔴 Critical | 3 | Breaking gaps that prevent correct operation |
| 🟠 High | 4 | Significant type mismatches requiring coordination |
| 🟡 Medium | 3 | Minor inconsistencies to address |
| 🟢 Low | 2 | Cosmetic or documentation-only issues |

---

## Contract Overview

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                      REMEDIATION ORCHESTRATOR CONTRACTS                           │
│                                                                                  │
│  Legend: ──► Write (RO creates CRD)    ◄── Read (RO reads status)               │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌─────────────────────┐                                                         │
│  │  RemediationRequest │ ◄── Gateway creates (spec is RO's input)               │
│  │  (Parent CRD)       │                                                         │
│  └──────────┬──────────┘                                                         │
│             │                                                                    │
│  ┌──────────┼───────────────┬─────────────────────┬──────────────────┐          │
│  │          │               │                     │                  │          │
│  ▼          ▼               ▼                     ▼                  ▼          │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  ┌───────────────┐│
│ │ SignalProcessing│  │   AIAnalysis    │  │ Remediation     │  │ Workflow      ││
│ │                 │  │                 │  │ ApprovalRequest │  │ Execution     ││
│ └────────┬────────┘  └────────┬────────┘  └─────────────────┘  └───────────────┘│
│          │                    │                                                  │
│  C1: ──► │ RO creates spec    │ C3: ──► RO creates spec                         │
│  C2: ◄── │ RO reads status    │ C4: ◄── RO reads status      C7: ──► RO creates │
│          │                    │                               C5: ──► RO creates │
│          │                    │                                                  │
│          │                    ▼                                                  │
│          │            ┌─────────────────┐                                        │
│          │            │ Notification    │                                        │
│          │            │ Request         │                                        │
│          │            └─────────────────┘                                        │
│          │             C6: ──► RO creates (approval notifications)               │
│          │                                                                       │
└──────────┴───────────────────────────────────────────────────────────────────────┘

CONTRACT SUMMARY:
┌────────────┬─────────────────────────────────┬───────────┬─────────────────────┐
│ Contract   │ Direction                       │ Type      │ Status              │
├────────────┼─────────────────────────────────┼───────────┼─────────────────────┤
│ Contract 1 │ RO → SignalProcessing.spec      │ Write     │ ✅ Resolved (SP+GW) │
│ Contract 2 │ SignalProcessing.status → RO    │ Read      │ ✅ No gaps          │
│ Contract 3 │ RO → AIAnalysis.spec            │ Write     │ ✅ Resolved (AI)    │
│ Contract 4 │ AIAnalysis.status → RO          │ Read      │ ✅ No gaps          │
│ Contract 5 │ RO → WorkflowExecution.spec     │ Write     │ ✅ Resolved (v3.1)  │
│ Contract 6 │ RO → NotificationRequest.spec   │ Write     │ 🟡 1 gap (mapping)  │
│ Contract 7 │ RO → RemediationApprovalReq     │ Write     │ ✅ Implemented      │
└────────────┴─────────────────────────────────┴───────────┴─────────────────────┘
```

---

## Contract 1: RO → SignalProcessing

**Direction**: RO creates SignalProcessing.spec from RemediationRequest.spec
**Owner**: SignalProcessing team (authoritative for SP schema)

### Field Mapping Analysis

| RemediationRequest.spec | SignalProcessing.spec | Status |
|-------------------------|----------------------|--------|
| `signalFingerprint` | `signalFingerprint` | ✅ Match |
| `signalName` | `signalName` | ✅ Match |
| `severity` | `severity` | ✅ Match (same enum) |
| ~~`environment`~~ | `environment` | ✅ **REMOVED** (SP owns) |
| ~~`priority`~~ | `priority` | ✅ **REMOVED** (SP owns) |
| `signalType` | `signalType` | ✅ Match |
| `signalSource` | `signalSource` | ✅ Match |
| `targetType` | `targetType` | ✅ Match (same enum) |
| `targetResource` | `targetResource` | 🟠 **GAP-C1-03** |
| `signalLabels` | `signalLabels` | ✅ Match |
| `signalAnnotations` | `signalAnnotations` | ✅ Match |
| `firingTime` | `firingTime` | ✅ Match |
| `receivedTime` | `receivedTime` | ✅ Match |
| `deduplication` | `deduplication` | 🟠 **GAP-C1-04** |
| `providerData` | `providerData` | ✅ Match |
| `originalPayload` | `originalPayload` | ✅ Match |
| `isStorm` | `isStorm` | ✅ Match |
| `stormAlertCount` | `stormAlertCount` | ✅ Match |
| `stormType` | ❌ missing | 🟡 **GAP-C1-05** |
| `stormWindow` | ❌ missing | 🟡 **GAP-C1-06** |
| N/A | `enrichmentConfig` | ✅ SP-specific field |

### Gap Details

#### ✅ GAP-C1-01: Environment Validation Mismatch - RESOLVED

**Resolution**: SP team changed to free-text validation (December 1, 2025).

**Before**: `Enum=prod;staging;dev`
**After**: `MinLength=1, MaxLength=63` (free-text)

---

#### ✅ GAP-C1-02: Priority Enum Mismatch - RESOLVED (Field Removed)

**SP Resolution**: SP team changed to free-text validation (December 1, 2025).

**Gateway Resolution (2025-12-06)**: Gateway removed `environment` and `priority` fields entirely per DD-CATEGORIZATION-001.
- See: `docs/handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md`
- Classification is now owned by Signal Processing service
- RO will remove `Environment`/`Priority` from `RemediationRequestSpec` (acknowledged in `NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md`)

**Status**: ✅ **RESOLVED** - Field will be removed from CRD schema (RO action pending)

---

#### 🟠 GAP-C1-03: TargetResource Optionality Mismatch (HIGH) - BUG FIX REQUIRED

**Location**:
- `api/remediation/v1alpha1/remediationrequest_types.go:88`
- `api/signalprocessing/v1alpha1/signalprocessing_types.go:93`

**RemediationRequest** (BUG - needs fix):
```go
TargetResource *ResourceIdentifier `json:"targetResource,omitempty"`  // ❌ WRONG - should be required
```

**SignalProcessing** (CORRECT):
```go
TargetResource ResourceIdentifier `json:"targetResource"`  // ✅ Required - correct
```

**Problem**: RemediationRequest incorrectly marks TargetResource as optional. Gateway service is responsible for populating this field when creating the RemediationRequest. For V1, we only support Kubernetes signals, so TargetResource is always present. Non-Kubernetes signals (AWS, Datadog, etc.) will be addressed in V2.

**Impact**: RO cannot reliably pass TargetResource to SP if RR allows nil.

**Resolution**: **BUG FIX** - Change RR.spec.targetResource to required (value type):
```go
// TargetResource identifies the Kubernetes resource that triggered this signal.
// Populated by Gateway from NormalizedSignal.Resource - REQUIRED.
// +kubebuilder:validation:Required
TargetResource ResourceIdentifier `json:"targetResource"`
```

**Action**: Gateway team to fix - make TargetResource required in RemediationRequest.

---

#### ✅ GAP-C1-04: Deduplication Type Mismatch (HIGH) - RESOLVED

**Location**:
- `api/remediation/v1alpha1/remediationrequest_types.go:162-177`
- `api/signalprocessing/v1alpha1/signalprocessing_types.go:148-161`

**RemediationRequest.DeduplicationInfo**:
```go
type DeduplicationInfo struct {
    IsDuplicate                   bool        // ← Not in SP
    FirstSeen                     metav1.Time // ← Named differently
    LastSeen                      metav1.Time // ← Named differently
    OccurrenceCount               int
    PreviousRemediationRequestRef string      // ← Not in SP
}
```

**SignalProcessing.DeduplicationContext**:
```go
type DeduplicationContext struct {
    FirstOccurrence metav1.Time  // ← Different name
    LastOccurrence  metav1.Time  // ← Different name
    OccurrenceCount int
    CorrelationID   string       // ← Not in RR
}
```

**Problem**: Field names differ, and each has unique fields not in the other.

**Impact**: RO must manually map fields, increasing code complexity and bug risk.

**Decision**: Create shared type in `pkg/shared/types/` - no fields need to be hidden between RR and SP.

**Resolution**: Create unified type in `pkg/shared/types/deduplication.go`:
```go
// DeduplicationInfo tracks duplicate signal suppression
// Shared between RemediationRequest and SignalProcessing CRDs
type DeduplicationInfo struct {
    // True if this signal is a duplicate of an active remediation
    IsDuplicate bool `json:"isDuplicate,omitempty"`

    // Timestamp when this signal fingerprint was first seen
    FirstOccurrence metav1.Time `json:"firstOccurrence"`

    // Timestamp when this signal fingerprint was last seen
    LastOccurrence metav1.Time `json:"lastOccurrence"`

    // Total count of occurrences of this signal
    OccurrenceCount int `json:"occurrenceCount"`

    // Optional correlation ID for grouping related signals
    CorrelationID string `json:"correlationId,omitempty"`

    // Reference to previous RemediationRequest CRD (if duplicate)
    PreviousRemediationRequestRef string `json:"previousRemediationRequestRef,omitempty"`
}
```

**Action**:
1. Create `pkg/shared/types/deduplication.go` with unified type
2. Update RR to import shared type (rename FirstSeen→FirstOccurrence, LastSeen→LastOccurrence)
3. Update SP to import shared type (rename DeduplicationContext→DeduplicationInfo)

**Status**: ✅ **RESOLVED** (December 1, 2025)
- SignalProcessing team completed update
- Shared type created in `pkg/shared/types/`
- DeepCopy methods added for controller-runtime compatibility
- Gateway team completed update (using `sharedtypes.DeduplicationInfo`)

---

#### ✅ GAP-C1-05 & GAP-C1-06: Storm Fields - RESOLVED

**Resolution**: SP team added both fields (December 1, 2025).

**Added Fields**:
```go
StormType string `json:"stormType,omitempty"`    // "rate" or "pattern"
StormWindow string `json:"stormWindow,omitempty"` // e.g., "5m"
```

**Rationale from SP team**:
- Self-contained CRD pattern (all data from RR)
- LLM context (storm type affects AI analysis)
- Future enrichment possibilities

---

## Contract 2: SignalProcessing.status → RO

**Direction**: RO reads SP.status to check completion and extract enrichment results
**Owner**: SignalProcessing team

### Field Mapping Analysis

| SignalProcessing.status | RO Usage | Status |
|------------------------|----------|--------|
| `phase` | Check completion | ✅ OK |
| `enrichmentResults` | Pass to AIAnalysis | ✅ OK (see Contract 3) |
| `startTime` | Audit/metrics | ✅ OK |
| `completedAt` | Audit/metrics | ✅ OK |

**No gaps identified** - RO only reads status fields for routing decisions.

---

## Contract 3: RO → AIAnalysis

**Direction**: RO creates AIAnalysis.spec from RemediationRequest.spec + SignalProcessing.status
**Owner**: AIAnalysis team (authoritative for AIAnalysis schema)

### ✅ ALL GAPS RESOLVED (AI Team Response - December 1, 2025)

| Source | AIAnalysis.spec.analysisRequest.signalContext | Status |
|--------|----------------------------------------------|--------|
| RR.signalFingerprint | `fingerprint` | ✅ Match |
| RR.severity | `severity` | ✅ Match |
| RR.signalType | `signalType` | ✅ Match |
| RR.environment | `environment` | ✅ **Resolved** (free-text) |
| RR.priority | `businessPriority` | ✅ **Resolved** (free-text) |
| ~~(derived)~~ | ~~`riskTolerance`~~ | ✅ **Removed** (in CustomLabels) |
| ~~(derived)~~ | ~~`businessCategory`~~ | ✅ **Removed** (in CustomLabels) |
| RR.targetResource | `targetResource` | ✅ Match (same type) |
| SP.status.enrichmentResults | `enrichmentResults` | ✅ **Resolved** (shared package) |

### Gap Details - ✅ ALL RESOLVED

#### ✅ GAP-C3-01: Environment Enum Mismatch - RESOLVED

**Resolution**: AI team changed to free-text validation (December 1, 2025).

**Before**: `Enum=production;staging;development`
**After**: `MinLength=1, MaxLength=63` (free-text)

**Also fixed**: `BusinessPriority` changed from enum to free-text for consistency.

---

#### ✅ GAP-C3-02: RiskTolerance - RESOLVED (REMOVED)

**Resolution**: AI team removed field (December 1, 2025).

**Rationale**: Per DD-WORKFLOW-001 v1.4, `risk_tolerance` is now customer-derived via Rego policies. It exists in `CustomLabels` (e.g., `{"constraint": ["risk-tolerance=low"]}`).

---

#### ✅ GAP-C3-03: BusinessCategory - RESOLVED (REMOVED)

**Resolution**: AI team removed field (December 1, 2025).

**Rationale**: Per DD-WORKFLOW-001 v1.4, `business_category` is now customer-derived via Rego policies. It exists in `CustomLabels` (e.g., `{"business": ["category=payments"]}`).

---

#### ✅ GAP-C3-04: EnrichmentResults Type Mismatch - RESOLVED

**Resolution**: AI team created shared package `pkg/shared/types/enrichment.go` (December 1, 2025).

**Types moved to shared package**:
- `EnrichmentResults`, `OwnerChainEntry`, `DetectedLabels`
- `KubernetesContext`, `PodDetails` (with Annotations, Containers), `ContainerStatus`
- `DeploymentDetails`, `NodeDetails` (with Capacity, Allocatable, Conditions)
- `ServiceSummary`, `IngressSummary`, `ConfigMapSummary`

**Both AIAnalysis and SignalProcessing now import from `pkg/shared/types`.**

---

## Contract 4: AIAnalysis.status → RO

**Direction**: RO reads AIAnalysis.status to check completion and get workflow recommendation
**Owner**: AIAnalysis team

### Field Mapping Analysis

| AIAnalysis.status | RO Usage | Status |
|-------------------|----------|--------|
| `phase` | Route to next step | ✅ OK |
| `approvalRequired` | Create RemediationApprovalRequest + NotificationRequest | ✅ OK |
| `approvalReason` | Include in notification | ✅ OK |
| `approvalContext` | Include in RemediationApprovalRequest | ✅ OK |
| `selectedWorkflow.workflowId` | Pass to WorkflowExecution | ✅ OK |
| `selectedWorkflow.version` | Pass to WorkflowExecution | ✅ OK |
| `selectedWorkflow.containerImage` | Pass to WorkflowExecution | ✅ OK |
| `selectedWorkflow.containerDigest` | Pass to WorkflowExecution | ✅ OK |
| `selectedWorkflow.confidence` | Audit/metrics | ✅ OK |
| `selectedWorkflow.parameters` | Pass to WorkflowExecution | ✅ OK |
| `selectedWorkflow.rationale` | Audit | ✅ OK |

**No gaps identified** - AIAnalysis.status provides all data RO needs.

---

## Contract 5: RO → WorkflowExecution

**Direction**: RO creates WorkflowExecution.spec from AIAnalysis.status.selectedWorkflow
**Owner**: WorkflowExecution team (authoritative for WE schema)

### ✅ ALL GAPS RESOLVED (WE Team Response - December 1, 2025)

The WE team confirmed all gaps were already resolved in **CRD Schema v3.1**:

| AIAnalysis.status.selectedWorkflow | WorkflowExecution.spec (v3.1) | Status |
|-----------------------------------|-------------------------------|--------|
| `workflowId` | `workflowRef.workflowId` | ✅ **Resolved** |
| `version` | `workflowRef.version` | ✅ Match |
| `containerImage` | `workflowRef.containerImage` | ✅ **Resolved** |
| `containerDigest` | `workflowRef.containerDigest` | ✅ **Resolved** |
| `parameters` | `parameters` (top-level) | ✅ **Resolved** |
| `confidence` | `confidence` | ✅ **Resolved** |
| `rationale` | `rationale` | ✅ **Resolved** |
| N/A | ~~`steps`~~ | ✅ **Removed** (ADR-044) |
| N/A | `executionConfig` (simplified) | ✅ **Resolved** |

### NEW Requirements for RO (from WE v3.1)

| Requirement | Description |
|-------------|-------------|
| `spec.targetResource` | **REQUIRED** - Format: `namespace/kind/name` for resource locking |
| Handle `Skipped` phase | **REQUIRED** - Reasons: `ResourceBusy`, `RecentlyRemediated` |

### Gap Details - ✅ ALL RESOLVED

**WE Team confirmed (December 1, 2025)**: All gaps were resolved in CRD Schema v3.1 per ADR-044 (Engine Delegation).

#### ✅ GAP-C5-01: WorkflowId vs Name - RESOLVED

**Resolution**: `WorkflowDefinition` replaced with `WorkflowRef` which has explicit `workflowId` field.

---

#### ✅ GAP-C5-02: Container Image - RESOLVED

**Resolution**: Fields added to `WorkflowRef`:
- `containerImage` - OCI bundle for Tekton
- `containerDigest` - for audit trail
- `parameters` added as top-level spec field

---

#### ✅ GAP-C5-03: Steps - RESOLVED

**Resolution**: `steps` field **completely removed** per ADR-044. Tekton handles step orchestration. Steps live inside the OCI bundle.

---

#### ✅ GAP-C5-04: ExecutionStrategy - RESOLVED

**Resolution**: Simplified to `ExecutionConfig` with only:
- `timeout` (default: 30m)
- `serviceAccountName` (default: "kubernaut-workflow-runner")

Removed fields (Tekton handles or not needed):
- `ApprovalRequired` → RO handles approval before WE creation
- `DryRunFirst` → Not in V1.0
- `RollbackStrategy` → Tekton `finally` tasks
- `MaxRetries` → Tekton handles task retries
- `SafetyChecks` → V2.0 feature

---

### 🔴 NEW: v3.1 Requirements for RO

The WE team identified NEW requirements that RO must implement:

#### NEW-C5-01: Populate `targetResource` (REQUIRED)

**Purpose**: Resource locking - prevents parallel workflows on same target.

**Format**: `namespace/kind/name` for namespaced, `kind/name` for cluster-scoped

**RO Implementation**:
```go
func buildTargetResource(rr *v1alpha1.RemediationRequest) string {
    tr := rr.Spec.TargetResource
    if tr.Namespace != "" {
        return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
    }
    return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
}
```

#### NEW-C5-02: Handle `Skipped` Phase (REQUIRED)

**New Phase**: WE can now return `Phase: Skipped` with `SkipDetails`.

| Reason | Meaning | RO Action |
|--------|---------|-----------|
| `ResourceBusy` | Another workflow running on same target | Log and wait, or mark RR as Skipped |
| `RecentlyRemediated` | Same workflow+target ran <5min ago | Mark RR as Skipped (dedup) |

**RO Implementation**:
```go
switch we.Status.Phase {
case "Completed":
    return r.handleCompleted(ctx, rr, we)
case "Failed":
    return r.handleFailed(ctx, rr, we)
case "Skipped":  // NEW v3.1
    log.Info("WorkflowExecution skipped",
        "reason", we.Status.SkipDetails.Reason,
        "target", we.Spec.TargetResource)
    return r.handleSkipped(ctx, rr, we)
}
```

---

## Contract 6: RO → NotificationRequest

**Direction**: RO creates NotificationRequest.spec when AIAnalysis.status.approvalRequired=true
**Owner**: Notification team

### Field Mapping Analysis

| Source | NotificationRequest.spec | Status |
|--------|-------------------------|--------|
| "escalation" (constant) | `type` | ✅ OK |
| AIAnalysis.status.approvalContext.confidenceLevel | `priority` mapping | 🟡 **GAP-C6-01** |
| (from policy/config) | `recipients` | ✅ OK (RO config) |
| (composed) | `subject` | ✅ OK |
| AIAnalysis.status.approvalContext | `body` | ✅ OK |
| (from policy/config) | `channels` | ✅ OK |
| (from context) | `metadata` | ✅ OK |
| (composed) | `actionLinks` | ✅ OK |
| (default) | `retryPolicy` | ✅ OK |
| (default) | `retentionDays` | ✅ OK |

### Gap Details

#### 🟡 GAP-C6-01: Priority Mapping (MEDIUM)

**Problem**: AIAnalysis provides `confidenceLevel` (low/medium/high), NotificationRequest expects `priority` (critical/high/medium/low).

**Mapping Required**:
```go
switch aiAnalysis.Status.ApprovalContext.ConfidenceLevel {
case "low":
    priority = NotificationPriorityCritical  // Low confidence = critical notification
case "medium":
    priority = NotificationPriorityHigh
case "high":
    priority = NotificationPriorityMedium  // High confidence = medium notification
}
```

**Resolution**: Document mapping in RO implementation. No schema change needed.

---

## Contract 7: RO → RemediationApprovalRequest

**Direction**: RO creates RemediationApprovalRequest.spec when AIAnalysis.status.approvalRequired=true
**Owner**: RemediationApprovalRequest team (per ADR-040)

### Status: ✅ CRD IMPLEMENTED

**Per ADR-040**, the CRD is at: `api/remediation/v1alpha1/remediationapprovalrequest_types.go`

**Implemented**: December 1, 2025

### Required Schema (per ADR-040)

```go
type RemediationApprovalRequestSpec struct {
    // Reference to parent RemediationRequest
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // Reference to AIAnalysis that requires approval
    AIAnalysisRef ObjectRef `json:"aiAnalysisRef"`

    // Approval context from AIAnalysis
    ApprovalContext ApprovalContext `json:"approvalContext"`

    // Deadline for approval decision
    RequiredBy metav1.Time `json:"requiredBy"`
}

type RemediationApprovalRequestStatus struct {
    // Decision: "", "Approved", "Rejected", "Expired"
    Decision string `json:"decision,omitempty"`

    // Standard conditions
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Who made the decision (for audit)
    DecidedBy string `json:"decidedBy,omitempty"`

    // When the decision was made
    DecidedAt *metav1.Time `json:"decidedAt,omitempty"`
}
```

**Completed**: `remediationapprovalrequest_types.go` created per ADR-040 design.

**CRD Location**: `config/crd/bases/remediation.kubernaut.io_remediationapprovalrequests.yaml`

---

## Summary of Required Actions

### 🔴 Critical (Block Implementation)

| ID | Gap | Owner | Action |
|----|-----|-------|--------|
| ~~GAP-C1-01~~ | ~~Environment enum mismatch (SP)~~ | SP Team | ✅ Resolved |
| GAP-C3-01 | Environment enum mismatch (AI) | AI Team | Change to free-text |
| GAP-C3-04 | EnrichmentResults type mismatch | AI Team | Import SP types |
| ~~GAP-C5-02~~ | ~~ContainerImage missing in WE~~ | WE Team | ✅ Resolved (v3.1) |
| ~~GAP-C5-03~~ | ~~Steps source unclear~~ | WE Team | ✅ Resolved (v3.1) |
| ~~GAP-C7-01~~ | ~~RemediationApprovalRequest missing~~ | RO Team | ✅ Created |

### 🟠 High (Address Before V1) - ✅ ALL RESOLVED

| ID | Gap | Owner | Action |
|----|-----|-------|--------|
| ~~GAP-C1-02~~ | ~~Priority enum mismatch~~ | Gateway | ✅ Resolved (free-text) |
| ~~GAP-C1-03~~ | ~~TargetResource optionality~~ | Gateway | ✅ Resolved (required) |
| ~~GAP-C1-04~~ | ~~Deduplication type mismatch~~ | SP/Gateway | ✅ Resolved (shared type) |
| ~~GAP-C5-01~~ | ~~WorkflowId vs Name naming~~ | WE Team | ✅ Resolved (v3.1) |
| ~~GAP-C5-04~~ | ~~ExecutionStrategy source~~ | WE Team | ✅ Resolved (v3.1) |

### 🟡 Medium/Low - ✅ ALL RESOLVED

| ID | Gap | Owner | Action |
|----|-----|-------|--------|
| ~~GAP-C1-05~~ | ~~StormType missing in SP~~ | SP Team | ✅ Resolved |
| ~~GAP-C1-06~~ | ~~StormWindow missing in SP~~ | SP Team | ✅ Resolved |
| ~~GAP-C3-02~~ | ~~RiskTolerance source~~ | AI Team | ✅ Removed (in CustomLabels) |
| ~~GAP-C3-03~~ | ~~BusinessCategory source~~ | AI Team | ✅ Removed (in CustomLabels) |
| GAP-C6-01 | Priority mapping | RO Team | Document mapping (internal) |

### 🟢 RO Requirements from WE v3.1 - ✅ ALL ACKNOWLEDGED

| ID | Requirement | Owner | Status |
|----|-------------|-------|--------|
| ~~REQ-WE-01~~ | ~~Populate `spec.targetResource`~~ | RO Team | ✅ Acknowledged (BR-ORCH-032) |
| ~~REQ-WE-02~~ | ~~Handle `Skipped` phase~~ | RO Team | ✅ Acknowledged (BR-ORCH-032/033/034) |
| ~~REQ-WE-03~~ | ~~Review v3.1 schema docs~~ | RO Team | ✅ Reviewed |
| ~~REQ-WE-04~~ | ~~Acknowledge contract changes~~ | RO Team | ✅ Acknowledged in shared doc |

**Design Decision**: DD-RO-001 (Resource Lock Deduplication Handling)
**Business Requirements**: BR-ORCH-032, BR-ORCH-033, BR-ORCH-034

---

## Next Steps

### ✅ Completed
1. ~~Create questions for each team based on gaps above~~ → Done (ephemeral docs)
2. ~~Collect team responses~~ → ALL TEAMS RESPONDED
3. ~~Implement schema changes~~ → All teams completed their changes

### ✅ RO Team Acknowledgment Complete
1. ~~**Acknowledge** WE team contract changes (REQ-WE-04)~~ → ✅ Done
2. ~~**Review** WE v3.1 documentation (REQ-WE-03)~~ → ✅ Done
3. **Design Decision** created: DD-RO-001 (Resource Lock Deduplication Handling)
4. **Business Requirements** defined: BR-ORCH-032, BR-ORCH-033, BR-ORCH-034

### 🟡 RO Team Implementation Pending
1. **Implement** `targetResource` population when creating WorkflowExecution (BR-ORCH-032)
2. **Implement** `Skipped` phase handler for ResourceBusy/RecentlyRemediated (BR-ORCH-032)
3. **Implement** duplicate tracking and bulk notification (BR-ORCH-033, BR-ORCH-034)
4. **Document** priority mapping for NotificationRequest (GAP-C6-01)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.11 | 2025-12-06 | **GAP-C1-02 RESOLVED**: Gateway removed environment/priority fields entirely (DD-CATEGORIZATION-001). RO to remove from RR schema. |
| 1.10 | 2025-12-01 | **ALL CONTRACTS COMPLETE**: Gateway confirmed Q3 (namespace empty for cluster-scoped). All team questions resolved. |
| 1.9 | 2025-12-01 | **RO ACK COMPLETE**: REQ-WE-01/02/03/04 acknowledged. DD-RO-001, BR-ORCH-032/033/034 created. WE Q1/Q2 answered. |
| 1.8 | 2025-12-01 | **ALL EXTERNAL GAPS RESOLVED!** Gateway team completed C1 gaps. WE team added REQ-WE-01/02/03/04 for RO. |
| 1.7 | 2025-12-01 | AI team response: All C3 gaps resolved (environment, removed riskTolerance/businessCategory, shared enrichment pkg) |
| 1.6 | 2025-12-01 | SP team response: All C1 gaps resolved (environment, priority, storm fields) |
| 1.5 | 2025-12-01 | WE team response: All C5 gaps resolved in v3.1. NEW reqs: targetResource, Skipped phase |
| 1.4 | 2025-12-01 | Created ephemeral RO_CONTRACT_GAPS.md docs for Gateway, SP, AI, WE teams |
| 1.3 | 2025-12-01 | GAP-C1-04 resolved - Shared DeduplicationInfo type created, SP team completed update |
| 1.2 | 2025-12-01 | Updated diagram to show all 7 contracts including read contracts (C2, C4) |
| 1.1 | 2025-12-01 | GAP-C7-01 resolved - RemediationApprovalRequest CRD implemented |
| 1.0 | 2025-12-01 | Initial triage of all 7 contracts |

