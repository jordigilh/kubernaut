# BR-AI-084: AIAnalysis Extract RCA Target Resource

**Business Requirement ID**: BR-AI-084
**Category**: AIAnalysis Service
**Priority**: P0
**Target Version**: V1.1
**Status**: ✅ Approved
**Date**: 2026-01-20
**Last Updated**: 2026-08-01 (Rewritten against the Go implementation, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806); field renamed `affectedResource` → `remediationTarget` per ADR-055-ADDENDUM-001)

**Related Design Decisions**:
- [DD-KA-006: Remediation Target in Root Cause Analysis](../architecture/decisions/DD-KA-006-remediation-target-in-rca.md)
- [ADR-055-ADDENDUM-001: RemediationTarget Rename](../architecture/decisions/ADR-055-ADDENDUM-001-remediation-target-rename.md)
- [DD-CONTRACT-002: Service Integration Contracts](../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)
- [DD-AIANALYSIS-001: AIAnalysis CRD Spec Structure](../architecture/decisions/DD-AIANALYSIS-001-spec-structure.md)

**Related Business Requirements**:
- BR-KA-212: KA RCA Target Resource
- BR-SCOPE-010: RO Routing Validation
- BR-SCOPE-001: Resource Scope Management

---

## 📋 **Business Need**

### **Problem Statement**

Kubernaut Agent (KA) returns a `remediationTarget` field in its Root Cause Analysis (RCA) response (BR-KA-212), identifying the resource that should be remediated. AIAnalysis must extract and store this critical information so that:

1. RemediationOrchestrator can validate whether the RCA-determined target is managed by Kubernaut
2. There is a clear audit record of which resource was identified by AI for remediation
3. Workflows target the root cause, not just the symptom

**Example Scenario**:
- **Signal source**: `Pod/payment-api-xyz-123` (OOMKilled)
- **KA RCA response**: `remediationTarget = {kind: "Deployment", name: "payment-api", namespace: "production"}`
- **Extraction**: AIAnalysis stores this in `Status.RootCauseAnalysis.RemediationTarget`, so RO validates the Deployment, not the Pod

---

## 🎯 **Business Objective**

**AIAnalysis extracts the RCA-determined target resource from KA's investigation response and stores it in the AIAnalysis CRD status, making it available for downstream scope validation by RemediationOrchestrator.**

**Value Proposition**:
- ✅ **Correct Remediation**: RemediationOrchestrator validates the correct resource (root cause, not symptom)
- ✅ **Scope Control**: Kubernaut only remediates resources it's configured to manage
- ✅ **Audit Trail**: Clear record of RCA-determined target resource in AIAnalysis status
- ✅ **Flexibility**: Supports complex RCA scenarios (Pod → Deployment, Node → StatefulSet, etc.)

---

## 🔍 **Functional Requirements**

### **FR-AI-084-001: CRD Status Field**

**Requirement**: AIAnalysis CRD MUST include a `RemediationTarget` field in `Status.RootCauseAnalysis` to store the RCA-determined target resource.

**CRD Schema** (`api/aianalysis/v1alpha1/aianalysis_types.go`):
```go
type RootCauseAnalysis struct {
    Summary             string            `json:"summary"`
    Severity            string            `json:"severity"`
    SignalType          string            `json:"signalType"`
    ContributingFactors []string          `json:"contributingFactors,omitempty"`

    // RCA-determined target resource for remediation, K8s-verified by KA
    // (InjectRemediationTarget), which may differ from the signal source
    // resource (Spec.AnalysisRequest.SignalContext.TargetResource).
    // RemediationOrchestrator validates scope against THIS resource before
    // creating WorkflowExecution.
    // +optional
    RemediationTarget   *RemediationTarget `json:"remediationTarget,omitempty"`
}

type RemediationTarget struct {
    Kind       string `json:"kind"`
    Name       string `json:"name"`
    // +optional
    Namespace  string `json:"namespace,omitempty"`
    // APIVersion disambiguates the resource's API group when the Kind exists
    // in multiple groups (e.g. Route in route.openshift.io vs serving.knative.dev). Issue #1040.
    // +optional
    APIVersion string `json:"apiVersion,omitempty"`
}
```

**Acceptance Criteria**:
1. ✅ `RootCauseAnalysis` struct includes `RemediationTarget *RemediationTarget` field
2. ✅ `RemediationTarget` field is optional (pointer type)
3. ✅ `RemediationTarget` includes `APIVersion` field (optional)
4. ✅ `make manifests` generates CRD YAML with these fields

---

### **FR-AI-084-002: Extraction Logic**

**Requirement**: AIAnalysis response processor MUST extract `remediationTarget` from KA's investigation response `root_cause_analysis` and store it in `Status.RootCauseAnalysis.RemediationTarget`.

**Extraction Logic** (`pkg/aianalysis/handlers/response_processor.go`, `ExtractRootCauseAnalysis` — Issue #97 centralized this; was previously duplicated across 5 handler functions):
```go
// #542: KA emits "remediationTarget" in JSON; CRD stores it as RemediationTarget.
func ExtractRootCauseAnalysis(rcaData interface{}) *aianalysisv1.RootCauseAnalysis {
    rcaMap := GetMapFromOptNil(rcaData)
    if rcaMap == nil {
        return nil
    }
    rca := &aianalysisv1.RootCauseAnalysis{
        Summary:             GetStringFromMap(rcaMap, "summary"),
        Severity:            severityOrUnknown(rcaMap),
        SignalType:          GetStringFromMap(rcaMap, "signal_name"),
        ContributingFactors: GetStringSliceFromMap(rcaMap, "contributing_factors"),
    }

    if arRaw, ok := rcaMap["remediationTarget"]; ok {
        if arMap, ok := arRaw.(map[string]interface{}); ok {
            kind, _ := arMap["kind"].(string)
            name, _ := arMap["name"].(string)
            ns, _ := arMap["namespace"].(string)
            apiVersion, _ := arMap["api_version"].(string) // #1040
            if kind != "" && name != "" {
                rca.RemediationTarget = &aianalysisv1.RemediationTarget{
                    Kind: kind, Name: name, Namespace: ns, APIVersion: apiVersion,
                }
            }
        }
    }
    return rca
}
```

**Acceptance Criteria**:
1. ✅ Response processor extracts `remediationTarget` from KA's response
2. ✅ Extraction validates required fields (`kind`, `name`) are non-empty
3. ✅ Extraction returns a `nil` `RemediationTarget` if the field is absent or invalid (whole `RootCauseAnalysis` is still populated)
4. ✅ Extracted data is stored in `Status.RootCauseAnalysis.RemediationTarget`

---

### **FR-AI-084-003: Human Review vs Approval - TWO DIFFERENT FLAGS**

**CRITICAL DISTINCTION**:

| Flag | Set By | Meaning | RO Action | User Experience |
|------|--------|---------|-----------|-----------------|
| **`needs_human_review`** | KA | AI **can't** answer (RCA incomplete) | NotificationRequest | "Manual investigation needed" |
| **`needs_approval`** | AIAnalysis Rego | AI **has** answer, policy requires approval | RemediationApprovalRequest | "Approve remediation plan?" |

**FR-AI-084-003a: Propagate KA's `needs_human_review`**

**Requirement**: When KA sets `needs_human_review=true`, AIAnalysis MUST propagate this flag to `Status.NeedsHumanReview`.

**Why?**
- Signal source = **Symptom** (e.g., OOMKilled Pod)
- RCA target = **Root Cause** (e.g., Deployment with insufficient memory)
- **Remediating symptom without identifying root cause is dangerous**
- Missing `remediationTarget` means RCA is incomplete → escalate to human (DO NOT fallback)

**Propagation Logic** (No Fallback):
1. **KA provides `remediationTarget`** → AIAnalysis stores it in `Status.RootCauseAnalysis.RemediationTarget`
2. **KA sets `needs_human_review=true`** (per BR-KA-197 / BR-KA-212, `rca_incomplete`):
   - AIAnalysis stores `Status.RootCauseAnalysis.RemediationTarget = nil` (if missing)
   - AIAnalysis stores `Status.NeedsHumanReview = true`
   - AIAnalysis stores `Status.HumanReviewReason = kaResponse.human_review_reason`

**FR-AI-084-003b: Evaluate Rego for `needs_approval`**

**Requirement**: AIAnalysis MUST evaluate Rego policies to determine if `needs_approval=true` based on risk assessment.

**When?** Only when `needs_human_review=false` (AI has complete answer)

**Rego Policy Decisions** (separate from KA):
- Production namespace
- Database resource
- Custom policy rules (e.g., confidence < 90% for StatefulSet)

**Outcome**:
- AIAnalysis stores `Status.ApprovalRequired = true` (DIFFERENT field than `NeedsHumanReview`)
- AIAnalysis stores `Status.ApprovalReason` and `Status.ApprovalContext`

**Acceptance Criteria**:
1. ✅ AIAnalysis extracts `remediationTarget` from KA's response
2. ✅ AIAnalysis propagates `needs_human_review` from KA to `Status.NeedsHumanReview`
3. ✅ AIAnalysis evaluates Rego policies and sets `Status.ApprovalRequired` independently
4. ✅ AIAnalysis stores `nil` `RemediationTarget` when KA sets `needs_human_review=true`
5. ✅ RemediationOrchestrator checks BOTH flags (validated by BR-SCOPE-010)

---

## 📊 **Non-Functional Requirements**

### **NFR-AI-084-001: Backward Compatibility**

`RemediationTarget` is an optional (pointer) field. RO's escalation logic (checking `needs_human_review`) prevents dangerous remediation regardless of whether `RemediationTarget` is populated.

### **NFR-AI-084-002: Performance Impact**

Extraction is a simple map lookup and struct construction (~10µs); no additional KA calls, database queries, or external API calls are required.

---

## 🔗 **Integration Points**

### **Upstream: Kubernaut Agent (KA)**

**Contract** (BR-KA-212): KA returns `remediationTarget` in its `root_cause_analysis` response, K8s-verified against the resolved owner chain (`InjectRemediationTarget`). AIAnalysis extracts and stores it in CRD status.

### **Downstream: RemediationOrchestrator Service**

**Contract** (BR-SCOPE-010): RO uses the RCA target (no fallback to signal source), validates it is managed by Kubernaut (`kubernaut.ai/managed` label), and blocks remediation if unmanaged.

---

## ✅ **Success Criteria**

### **Business Success**
1. ✅ AIAnalysis CRDs include the RCA target resource whenever KA provides one
2. ✅ RemediationOrchestrator scope validations use the RCA target (no fallback)
3. ✅ Remediations never target unmanaged resources due to incorrect target extraction

### **Technical Success**
1. ✅ AIAnalysis extracts `remediationTarget` in 100% of cases when KA provides it
2. ✅ AIAnalysis correctly handles an absent `remediationTarget` (stores `nil`) in 100% of cases
3. ✅ RO correctly prioritizes the RCA target in 100% of scope validations

---

## 📚 **Related Documents**

### **Design Decisions**
- **DD-KA-006**: Remediation Target in Root Cause Analysis (THIS REQUIREMENT IMPLEMENTS THIS DD)
- **ADR-055-ADDENDUM-001**: RemediationTarget Rename (`affectedResource` → `remediationTarget`)
- **DD-CONTRACT-002**: Service Integration Contracts
- **DD-AIANALYSIS-001**: AIAnalysis CRD Spec Structure

### **Business Requirements**
- **BR-KA-212**: KA RCA Target Resource (upstream provider)
- **BR-SCOPE-010**: RO Routing Validation (downstream consumer)
- **BR-SCOPE-001**: Resource Scope Management (context)
- **BR-AI-080**: Recovery Analysis Support (related — recovery also needs RCA target)

### **Architecture Decisions**
- **ADR-053**: Resource Scope Management (impacted — RO uses RCA target)
- **ADR-001**: CRD-based Microservices Architecture (referenced — no changes)

---

## 🔒 **Confidence Assessment**

**Confidence Level**: 92%

**Strengths**:
- ✅ Extraction logic is verified against the current implementation (`ExtractRootCauseAnalysis`)
- ✅ Simple extraction logic (map lookup + struct construction)
- ✅ Clear escalation strategy (`nil` + `needs_human_review=true` → no remediation)
- ✅ Backward compatible (optional field)

**Risks**:
- ⚠️ **8% Gap**: KA's OpenAPI spec does not yet document the `remediationTarget` schema (see DD-KA-006, BR-KA-212 FR-KA-212-003), so this contract is currently verified only by direct code inspection, not by generated-client type safety
  - **Mitigation**: Robust map-based parsing with validation already in place
  - **Mitigation**: Unit tests cover extraction edge cases

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-08-01 (v2.0 — rewritten against Go implementation, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))
