# BR-KA-212: RCA Target Resource in Root Cause Analysis

**Business Requirement ID**: BR-KA-212
**Category**: Kubernaut Agent (KA) Service
**Priority**: P0
**Target Version**: V1.1
**Status**: ✅ Approved
**Date**: 2026-01-20
**Last Updated**: 2026-08-01 (Rewritten against the Go KA implementation, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806); field renamed `affectedResource` → `remediationTarget`; corrected conditional-injection and escalation claims)

**Related Design Decisions**:
- [DD-KA-006 v2.0: Remediation Target in Root Cause Analysis](../architecture/decisions/DD-KA-006-remediation-target-in-rca.md)
- [DD-CONTRACT-002: Service Integration Contracts](../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)

**Related Business Requirements**:
- **BR-KA-197**: Human Review Required Flag (BASE REQUIREMENT — this BR extends it)
- BR-AI-084: AIAnalysis Extract RCA Target Resource
- BR-SCOPE-001: Resource Scope Management
- BR-SCOPE-010: RO Routing Validation

---

## 🔗 **Relationship to BR-KA-197**

**This BR extends BR-KA-197 (Human Review Required Flag)**:

| Document | Purpose | Relationship |
|----------|---------|--------------|
| **BR-KA-197** | Defines `needs_human_review` flag and its base scenarios | BASE REQUIREMENT |
| **BR-KA-212** (this BR) | Adds the `rca_incomplete` scenario for owner-chain resolution failure | EXTENSION |

**NEW BR-KA-212 Scenario**:
- Owner-chain re-enrichment hard-fails after retry exhaustion when a workflow is selected → `needs_human_review=true`, `human_review_reason=rca_incomplete` (`internal/kubernautagent/investigator/investigator_discovery.go`, `reEnrichForRCATargetShift`). See [DD-KA-006](../architecture/decisions/DD-KA-006-remediation-target-in-rca.md).

---

## 📋 **Business Need**

### **Problem Statement**

Kubernaut Agent (KA) performs Root Cause Analysis (RCA) on Kubernetes signals and identifies the **resource that should be remediated**. This RCA-determined target resource must be clearly exposed in KA's API contract so that:

1. RemediationOrchestrator can validate whether the RCA-determined target resource is managed by Kubernaut
2. There is a clear audit record of which resource was identified for remediation
3. Workflows target the root cause, not just the symptom

**Example Scenario**:
- **Signal source**: `Pod/payment-api-xyz-123` (OOMKilled)
- **RCA analysis determines**: Root cause is insufficient memory limits on `Deployment/payment-api`
- **Remediation target**: `Deployment/payment-api` (not the Pod)

---

## 🎯 **Business Objective**

**KA returns the RCA-determined target resource (`remediationTarget`) in its investigation response, allowing AIAnalysis to extract and store this information for scope validation by RemediationOrchestrator.**

**Value Proposition**:
- ✅ **Correct Remediation**: Workflows target root cause, not symptom
- ✅ **Scope Control**: Kubernaut only remediates resources it's configured to manage
- ✅ **Audit Trail**: Clear record of which resource was identified by AI for remediation
- ✅ **Flexibility**: Supports complex RCA scenarios (Pod → Deployment, Node → StatefulSet, etc.)

---

## 🔍 **Functional Requirements**

### **FR-KA-212-001: RCA Target Resource Structure**

**Requirement**: KA MUST include a `remediationTarget` object in the `root_cause_analysis` response field.

**API Contract**:
```json
{
  "root_cause_analysis": {
    "summary": "Deployment has insufficient memory limits",
    "severity": "high",
    "contributing_factors": ["OOMKilled events recurring", "No HPA configured"],
    "remediationTarget": {
      "kind": "Deployment",
      "api_version": "apps/v1",
      "name": "payment-api",
      "namespace": "production"
    }
  }
}
```

**Field Specifications**:
- **`remediationTarget`**: KA-injected, not LLM-verbatim (see DD-KA-006 — `InjectRemediationTarget` reconciles whatever the LLM proposes against the K8s-verified owner chain)
- **Required fields within `remediationTarget`** when present:
  - **`kind`**: REQUIRED string — Kubernetes resource kind
  - **`name`**: REQUIRED string — Resource name
  - **`namespace`**: CONDITIONALLY REQUIRED — required for namespace-scoped resources, omitted for cluster-scoped resources
- **Optional fields**:
  - **`api_version`**: OPTIONAL — disambiguates the API group when the Kind exists in multiple groups (#1040). When missing, RemediationOrchestrator falls back to a static Kind→Group mapping for core resources.

**Acceptance Criteria**:
1. ✅ Investigation response includes `remediationTarget` derived from the K8s-verified owner chain
2. ✅ `TARGET_RESOURCE_NAME` / `TARGET_RESOURCE_KIND` / `TARGET_RESOURCE_NAMESPACE` (+ `TARGET_RESOURCE_API_VERSION` when known) are injected into `selected_workflow.parameters` unconditionally, and are never stripped as undeclared parameters (`kaManagedParams`, `internal/kubernautagent/parser/validator.go`)
3. ⚠️ `internal/kubernautagent/api/openapi.json` does not yet document the `remediationTarget` schema — tracked as a follow-up (see DD-KA-006's "Known gap" note)

---

### **FR-KA-212-002: Target Resolution Is K8s-Verified, Not LLM-Trusted**

**Requirement**: KA MUST reconcile whatever remediation target the LLM proposes against the K8s-verified owner chain resolved during enrichment, rather than trusting the LLM's proposal verbatim.

**Resolution priority** (`InjectRemediationTarget`, `internal/kubernautagent/investigator/investigator_phases.go`):
1. Owner-chain last entry (e.g. Pod → ReplicaSet → Deployment)
2. Enrichment source identity (post re-enrichment, when the chain is already empty because the enriched resource is the root)
3. Signal identity (fallback when no enrichment data is available)

If the LLM's proposed kind matches the resolved root or is a descendant in the owner chain, KA overrides/resolves upward to the K8s-verified root. Only a genuinely cross-type proposal (not in the owner chain at all) is preserved as-is.

**Acceptance Criteria**:
1. ✅ LLM proposals that name a child resource of the resolved root are corrected upward
2. ✅ LLM proposals that are genuinely cross-type (e.g. Node vs Deployment) are preserved
3. ✅ `remediationTarget` in the response always reflects the resolved, K8s-verified target — never an uncorrected LLM guess

---

### **FR-KA-212-003: API Contract Documentation**

**Requirement**: KA's OpenAPI spec SHOULD document the `remediationTarget` field in `root_cause_analysis`.

**Status**: ⚠️ **Not yet done.** `internal/kubernautagent/api/openapi.json` does not currently include a `remediationTarget` schema, despite the field being emitted on the wire and consumed by AIAnalysis (`ExtractRootCauseAnalysis`). This is a documentation/spec-accuracy gap, not a functional gap — tracked as a follow-up in [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806).

**Acceptance Criteria** (pending):
1. ⬜ OpenAPI spec includes a `remediationTarget` schema under `root_cause_analysis`
2. ⬜ Schema documents `kind` (required), `name` (required), `namespace` (conditional), `api_version` (optional)

---

### **FR-KA-212-004: Human Review Escalation (Extends BR-KA-197)**

**Requirement**: When owner-chain re-enrichment hard-fails after retry exhaustion and a workflow would otherwise be selected, KA MUST set `needs_human_review=true` with `human_review_reason=rca_incomplete`, rather than guessing at a remediation target.

**CRITICAL DISTINCTION** (unchanged from BR-KA-197):
- **`needs_human_review`** (KA decision) = "AI **can't** answer" (RCA incomplete)
- **`needs_approval`** (AIAnalysis Rego decision) = "AI **has** answer, but policy requires approval"

**Why NO Fallback to Signal Source**:
- Signal source = **Symptom** (e.g., OOMKilled Pod)
- RCA target = **Root Cause** (e.g., Deployment with insufficient memory)
- Remediating the symptom without a resolved root cause is dangerous — escalate to human review instead of guessing.

**Acceptance Criteria**:
1. ✅ KA sets `needs_human_review=true`, `human_review_reason=rca_incomplete` when owner-chain hard-fails
2. ✅ KA does not fall back to the raw signal source as a remediation target on failure
3. ✅ AIAnalysis propagates `needs_human_review` to CRD status
4. ✅ RO creates `NotificationRequest` (not `WorkflowExecution`) when `needs_human_review=true`, per the three-layer defense-in-depth chain in DD-KA-006 (KA → AIAnalysis → RemediationOrchestrator, `HandleRemediationTargetMissing`)

---

## 📊 **Non-Functional Requirements**

### **NFR-KA-212-001: Backward Compatibility**

`remediationTarget` is an optional field in the wire contract; consumers that don't read it continue to function. A missing target triggers escalation (`needs_human_review=true`), not a silent fallback.

### **NFR-KA-212-002: Performance Impact**

Target resolution reuses owner-chain data already fetched during enrichment — no additional K8s API calls or LLM round-trips are required specifically for this field.

---

## 🔗 **Integration Points**

### **Downstream: AIAnalysis Service**

**Contract** (BR-AI-084): AIAnalysis reads `remediationTarget` from the investigation response (`ExtractRootCauseAnalysis`, `pkg/aianalysis/handlers/response_processor.go`) and stores it in `Status.RootCauseAnalysis.RemediationTarget`. AIAnalysis stores `Status.NeedsHumanReview=true` if KA indicates incomplete RCA.

### **Downstream: RemediationOrchestrator Service**

**Contract** (BR-SCOPE-010): RemediationOrchestrator prioritizes the RCA target over the signal source, validates it against Kubernaut's managed scope, and blocks remediation if the target is unmanaged.

**Defense-in-Depth (BR-ORCH-036 v4.0)**: RO has its own guard (`HandleRemediationTargetMissing`, `pkg/remediationorchestrator/handler/aianalysis.go`) that checks for a missing `RemediationTarget` before routing. If KA and AIAnalysis both miss the issue, RO catches it and produces the same Failed + ManualReviewRequired response. See [DD-KA-006](../architecture/decisions/DD-KA-006-remediation-target-in-rca.md) for the complete three-layer model.

---

## ✅ **Success Criteria**

### **Business Success**
1. ✅ Remediation workflows target the correct resource (root cause, not symptom)
2. ✅ Remediations never target unmanaged resources when the RCA target differs from the signal source
3. ✅ Audit trails include the RCA-determined target resource

### **Technical Success**
1. ✅ KA returns a K8s-verified `remediationTarget` on every completed RCA with a selected workflow
2. ✅ AIAnalysis correctly extracts `remediationTarget` in 100% of cases when provided
3. ✅ RemediationOrchestrator correctly validates the RCA target in 100% of cases

---

## 📚 **Related Documents**

### **Design Decisions**
- **DD-KA-006**: Remediation Target in Root Cause Analysis (THIS REQUIREMENT IMPLEMENTS THIS DD)
- **DD-CONTRACT-002**: Service Integration Contracts

### **Business Requirements**
- **BR-AI-084**: AIAnalysis Extract RCA Target Resource (downstream consumer)
- **BR-SCOPE-001**: Resource Scope Management (context)
- **BR-SCOPE-010**: RO Routing Validation (downstream consumer)

---

## 🔒 **Confidence Assessment**

**Confidence Level**: 90%

**Strengths**:
- ✅ Target resolution is K8s-verified (`InjectRemediationTarget`), not dependent on LLM reliability
- ✅ `TARGET_RESOURCE_*` parameters are unconditionally injected and preserved (`kaManagedParams`) — no silent parameter loss
- ✅ Clear escalation path (`rca_incomplete`) when owner-chain resolution cannot complete

**Risks**:
- ⚠️ OpenAPI spec does not yet document `remediationTarget` — a documentation gap, not a functional one (FR-KA-212-003)
- ⚠️ Owner-chain resolution depends on enrichment succeeding at least once; this BR's safety property is that failure escalates rather than guesses

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-08-01 (v2.0 — rewritten against Go KA implementation, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))
