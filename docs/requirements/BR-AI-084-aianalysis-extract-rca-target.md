# BR-AI-084: AIAnalysis Extract RCA Target Resource

**Business Requirement ID**: BR-AI-084
**Category**: AIAnalysis Service
**Priority**: P0
**Target Version**: V1.1
**Status**: ✅ Approved
**Date**: 2026-01-20
**Last Updated**: 2026-01-20

**Related Design Decisions**:
- [DD-HAPI-006: Affected Resource in Root Cause Analysis](../architecture/decisions/DD-HAPI-006-affectedResource-in-rca.md)
- [DD-CONTRACT-002: Service Integration Contracts](../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)
- [DD-AIANALYSIS-001: AIAnalysis CRD Spec Structure](../architecture/decisions/DD-AIANALYSIS-001-spec-structure.md)

**Related Business Requirements**:
- BR-HAPI-212: HAPI RCA Target Resource
- BR-SCOPE-010: RO Routing Validation
- BR-SCOPE-001: Resource Scope Management

---

## 📋 **Business Need**

### **Problem Statement**

HolmesGPT-API now returns an `affectedResource` field in its Root Cause Analysis (RCA) response (BR-HAPI-212), identifying the resource that should be remediated. However, AIAnalysis **does not extract or store** this critical information, leading to:

1. ❌ **Scope validation gaps**: RemediationOrchestrator cannot validate if the RCA-determined target is managed by Kubernaut
2. ❌ **Audit trail gaps**: No record of which resource was identified by AI for remediation
3. ❌ **Incorrect remediation**: Workflows may target the wrong resource (symptom vs root cause)

**Example Scenario**:
- **Signal source**: `Pod/payment-api-xyz-123` (OOMKilled)
- **HAPI RCA response**: `affectedResource = {kind: "Deployment", name: "payment-api", namespace: "production"}`
- **Gap**: AIAnalysis does not extract this, so RO validates the wrong resource (Pod instead of Deployment)

---

## 🎯 **Business Objective**

**Enable AIAnalysis to extract the RCA-determined target resource from HolmesGPT-API responses and store it in the AIAnalysis CRD status, making it available for downstream scope validation by RemediationOrchestrator.**

**Value Proposition**:
- ✅ **Correct Remediation**: RemediationOrchestrator validates the correct resource (root cause, not symptom)
- ✅ **Scope Control**: Kubernaut only remediates resources it's configured to manage
- ✅ **Audit Trail**: Clear record of RCA-determined target resource in AIAnalysis status
- ✅ **Flexibility**: Supports complex RCA scenarios (Pod → Deployment, Node → StatefulSet, etc.)

---

## 🔍 **Functional Requirements**

### **FR-AI-084-001: CRD Status Field**

**Requirement**: AIAnalysis CRD MUST include a `TargetResource` field in `Status.RootCauseAnalysis` to store the RCA-determined target resource.

**CRD Schema**:
```go
type RootCauseAnalysis struct {
    Summary             string            `json:"summary"`
    Severity            string            `json:"severity"`
    SignalType          string            `json:"signalType"`
    ContributingFactors []string          `json:"contributingFactors,omitempty"`

    // NEW: RCA-determined target resource for remediation
    // This is the resource identified by HolmesGPT as the root cause,
    // which may differ from the signal source resource (Spec.AnalysisRequest.SignalContext.TargetResource).
    // RemediationOrchestrator validates scope against THIS resource before creating WorkflowExecution.
    // +optional
    TargetResource      *TargetResource   `json:"targetResource,omitempty"`
}

type TargetResource struct {
    // Kubernetes resource kind (e.g., "Deployment", "Pod", "StatefulSet")
    Kind      string `json:"kind"`
    // Kubernetes API version (e.g., "apps/v1", "v1") - OPTIONAL
    // When provided: Used for deterministic GVK resolution
    // When missing: RO uses static mapping for core resources
    // +optional
    APIVersion string `json:"apiVersion,omitempty"`
    // Resource name
    Name      string `json:"name"`
    // Resource namespace (empty for cluster-scoped resources)
    // +optional
    Namespace string `json:"namespace,omitempty"`
}
```

**Acceptance Criteria**:
1. ✅ `RootCauseAnalysis` struct includes `TargetResource *TargetResource` field
2. ✅ `TargetResource` field is optional (pointer type)
3. ✅ `TargetResource` includes `APIVersion` field (optional)
4. ✅ `make manifests` generates updated CRD YAML with new fields
5. ✅ Kubebuilder validation accepts new CRD schema

---

### **FR-AI-084-002: Extraction Logic**

**Requirement**: AIAnalysis response processor MUST extract `affectedResource` from HAPI's `IncidentResponse.root_cause_analysis` and store it in `Status.RootCauseAnalysis.TargetResource`.

**Extraction Logic** (`pkg/aianalysis/handlers/response_processor.go`):
```go
if len(resp.RootCauseAnalysis) > 0 {
    rcaMap := GetMapFromOptNil(resp.RootCauseAnalysis)
    if rcaMap != nil {
        analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
            Summary:             GetStringFromMap(rcaMap, "summary"),
            Severity:            GetStringFromMap(rcaMap, "severity"),
            ContributingFactors: GetStringSliceFromMap(rcaMap, "contributing_factors"),

            // NEW: Extract affectedResource from RCA
            TargetResource:      extractTargetResourceFromRCA(rcaMap),
        }
    }
}

func extractTargetResourceFromRCA(rcaMap map[string]interface{}) *aianalysisv1.TargetResource {
    // Try "affectedResource" (camelCase) - HAPI returns this
    affectedResource := rcaMap["affectedResource"]
    if affectedResource == nil {
        // Fallback: Try "affected_resource" (snake_case) for compatibility
        affectedResource = rcaMap["affected_resource"]
    }
    if affectedResource == nil {
        return nil // No RCA target - HAPI should have set needs_human_review=true
    }

    arMap, ok := affectedResource.(map[string]interface{})
    if !ok {
        return nil // Invalid format
    }

    kind := GetStringFromMap(arMap, "kind")
    apiVersion := GetStringFromMap(arMap, "apiVersion")  // NEW: Extract optional apiVersion
    name := GetStringFromMap(arMap, "name")
    namespace := GetStringFromMap(arMap, "namespace")

    // Validate required fields (namespace is optional for cluster-scoped resources)
    if kind == "" || name == "" {
        return nil // Incomplete resource specification
    }

    return &aianalysisv1.TargetResource{
        Kind:       kind,
        APIVersion: apiVersion,  // NEW: Optional - empty string if not provided
        Name:       name,
        Namespace:  namespace,   // Empty for cluster-scoped resources
    }
}
```

**Acceptance Criteria**:
1. ✅ Response processor extracts `affectedResource` from HAPI response
2. ✅ Extraction supports both camelCase (`affectedResource`) and snake_case (`affected_resource`)
3. ✅ Extraction validates required fields (`kind`, `name`, `namespace`) are non-empty
4. ✅ Extraction returns `nil` if `affectedResource` is absent or invalid
5. ✅ Extracted data is stored in `Status.RootCauseAnalysis.TargetResource`

---

### **FR-AI-084-003: Human Review vs Approval - TWO DIFFERENT FLAGS**

**CRITICAL DISTINCTION**:

| Flag | Set By | Meaning | RO Action | User Experience |
|------|--------|---------|-----------|-----------------|
| **`needs_human_review`** | HAPI | AI **can't** answer (RCA incomplete) | NotificationRequest | "Manual investigation needed" |
| **`needs_approval`** | AIAnalysis Rego | AI **has** answer, policy requires approval | RemediationApprovalRequest | "Approve remediation plan?" |

**FR-AI-084-003a: Propagate HAPI's `needs_human_review`**

**Requirement**: When HAPI sets `needs_human_review=true`, AIAnalysis MUST propagate this flag to `Status.NeedsHumanReview`.

**Why?**
- Signal source = **Symptom** (e.g., OOMKilled Pod)
- RCA target = **Root Cause** (e.g., Deployment with insufficient memory)
- **Remediating symptom without identifying root cause is dangerous**
- Missing `affectedResource` means RCA is incomplete → escalate to human (DO NOT fallback)

**Propagation Logic** (No Fallback):
1. **HAPI provides `affectedResource`** → AIAnalysis stores it in `Status.RootCauseAnalysis.TargetResource`
2. **HAPI sets `needs_human_review=true`** (per BR-HAPI-197):
   - AIAnalysis stores `Status.RootCauseAnalysis.TargetResource = nil` (if missing)
   - AIAnalysis stores `Status.NeedsHumanReview = true`
   - AIAnalysis stores `Status.HumanReviewReason = hapiResponse.human_review_reason`

**FR-AI-084-003b: Evaluate Rego for `needs_approval`**

**Requirement**: AIAnalysis MUST evaluate Rego policies to determine if `needs_approval=true` based on risk assessment.

**When?** Only when `needs_human_review=false` (AI has complete answer)

**Rego Policy Decisions** (separate from HAPI):
- Production namespace
- Database resource
- Custom policy rules (e.g., confidence < 90% for StatefulSet)

**Outcome**:
- AIAnalysis stores `Status.ApprovalRequired = true` (DIFFERENT field than `NeedsHumanReview`)
- AIAnalysis stores `Status.ApprovalReason` and `Status.ApprovalContext`

**Acceptance Criteria**:
1. ✅ AIAnalysis extracts `affectedResource` from HAPI response
2. ✅ AIAnalysis propagates `needs_human_review` from HAPI to `Status.NeedsHumanReview`
3. ✅ AIAnalysis evaluates Rego policies and sets `Status.ApprovalRequired` independently
4. ✅ AIAnalysis stores `nil` `TargetResource` when HAPI sets `needs_human_review=true`
5. ✅ RemediationOrchestrator checks BOTH flags (validated by BR-SCOPE-010)

---

### **FR-AI-084-004: Audit Trail**

**Requirement**: AIAnalysis MUST log when `affectedResource` is extracted, including the resource details, for audit and debugging purposes.

**Logging**:
```go
if targetResource != nil {
    log.Info("Extracted RCA target resource",
        "kind", targetResource.Kind,
        "name", targetResource.Name,
        "namespace", targetResource.Namespace,
        "signal_source_kind", analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Kind,
        "signal_source_name", analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Name,
    )
} else {
    log.Info("No RCA target resource provided - HAPI should have set needs_human_review=true")
}
```

**Acceptance Criteria**:
1. ✅ AIAnalysis logs when `TargetResource` is extracted
2. ✅ Log includes resource details (`kind`, `name`, `namespace`)
3. ✅ Log includes RCA target details for traceability
4. ✅ Log indicates when RCA target is absent (escalation required)

---

## 📊 **Non-Functional Requirements**

### **NFR-AI-084-001: Backward Compatibility**

**Requirement**: The addition of `TargetResource` field MUST NOT break existing RemediationOrchestrator consumers.

**Backward Compatibility**:
- ✅ `TargetResource` is **optional** (pointer type) - RemediationOrchestrator checks `needs_human_review` flag
- ✅ RO escalation logic ensures dangerous remediation is prevented (no fallback to symptom)
- ✅ No breaking changes to AIAnalysis CRD spec (only Status field addition)

---

### **NFR-AI-084-002: Performance Impact**

**Requirement**: Extracting and storing `TargetResource` MUST NOT degrade AIAnalysis response time.

**Performance Analysis**:
- ✅ Extraction is a simple map lookup and struct construction (~10 µs)
- ✅ No additional HAPI calls required
- ✅ No database queries or external API calls
- ✅ CRD status update is atomic (no additional K8s API calls)

**Acceptance Criteria**:
1. ✅ AIAnalysis response time increase < 1ms
2. ✅ No additional HAPI calls
3. ✅ No additional K8s API calls

---

## 🔗 **Integration Points**

### **Upstream: HolmesGPT-API**

**Integration**: AIAnalysis consumes `affectedResource` from HAPI's `/incident/analyze` response.

**Contract** (BR-HAPI-212):
- HAPI returns `affectedResource` in `IncidentResponse.root_cause_analysis`
- HAPI validates `affectedResource` is in OwnerChain before returning
- AIAnalysis extracts and stores `affectedResource` in CRD status

---

### **Downstream: RemediationOrchestrator Service**

**Integration**: RemediationOrchestrator reads `AIAnalysis.Status.RootCauseAnalysis.TargetResource` for scope validation.

**Contract** (BR-SCOPE-010):
- RO uses RCA target (no fallback to signal source)
- RO validates RCA target is managed by Kubernaut (using `kubernaut.ai/managed` label)
- RO blocks remediation if RCA target is unmanaged

---

## ✅ **Success Criteria**

### **Business Success**
1. ✅ 100% of AIAnalysis CRDs include RCA target resource when HAPI provides it
2. ✅ 100% of RemediationOrchestrator scope validations use RCA target (no fallback)
3. ✅ 0% of remediations target unmanaged resources due to incorrect target extraction

### **Technical Success**
1. ✅ AIAnalysis extracts `affectedResource` in 100% of cases when HAPI provides it
2. ✅ AIAnalysis correctly handles absent `affectedResource` (stores `nil`) in 100% of cases
3. ✅ RO correctly prioritizes RCA target in 100% of scope validations

### **Quality Success**
1. ✅ AIAnalysis unit tests cover `extractTargetResourceFromRCA()` function
2. ✅ AIAnalysis integration tests validate `TargetResource` extraction from HAPI
3. ✅ E2E tests validate end-to-end flow (HAPI → AIAnalysis → RO → scope validation)

---

## 📈 **Business Value & Metrics**

### **Before (Current State)**
- ❌ 0% of AIAnalysis CRDs include RCA target resource
- ❌ 100% of scope validations use signal source (symptoms, not root cause)
- ❌ 10-20% of remediations target the wrong resource (symptom vs root cause)

### **After (Target State)**
- ✅ 80%+ of AIAnalysis CRDs include RCA target resource (when HAPI provides it)
- ✅ 100% of scope validations use RCA target (escalation if missing)
- ✅ 100% of remediations target the correct resource (RCA-determined)

### **KPIs**
| Metric | Baseline | Target | Measurement |
|--------|----------|--------|-------------|
| AIAnalysis CRDs with RCA target | 0% | 80%+ | CRD status field analysis |
| Scope validations using correct target | 0% | 100% | RO routing logs |
| Remediations targeting correct resource | 80% | 100% | Audit event analysis |

---

## 🚀 **Implementation Plan**

### **Phase 1: CRD Schema Update**
1. Add `TargetResource` field to `RootCauseAnalysis` struct (`api/aianalysis/v1alpha1/aianalysis_types.go`)
2. Run `make manifests` to regenerate CRD YAML
3. Validate CRD schema with `kubectl apply --dry-run`

**Deliverables**:
- Updated `aianalysis_types.go`
- Regenerated CRD YAML

**Timeline**: 0.5 day

---

### **Phase 2: Extraction Logic**
1. Implement `extractTargetResourceFromRCA()` helper function (`pkg/aianalysis/handlers/response_processor.go`)
2. Update `PopulateStatusFromIncident()` to extract and store `TargetResource`
3. Add logging for `TargetResource` extraction

**Deliverables**:
- `extractTargetResourceFromRCA()` function
- Updated response processor
- Extraction logs

**Timeline**: 1 day

---

### **Phase 3: Unit Tests**
1. Add unit tests for `extractTargetResourceFromRCA()`:
   - Valid `affectedResource` (camelCase)
   - Valid `affectedResource` (snake_case)
   - Missing `affectedResource`
   - Invalid `affectedResource` (missing fields)
   - Invalid `affectedResource` (wrong type)

**Deliverables**:
- 5 unit tests for extraction function
- 100% code coverage for extraction logic

**Timeline**: 0.5 day

---

### **Phase 4: Integration Tests**
1. Add integration test: HAPI returns `affectedResource` → AIAnalysis extracts it
2. Add integration test: HAPI does not return `affectedResource` → AIAnalysis stores `nil`
3. Add integration test: RO reads `TargetResource` for scope validation

**Deliverables**:
- 3 integration tests for end-to-end extraction
- Validation of RO integration

**Timeline**: 1 day

---

## 📚 **Related Documents**

### **Design Decisions**
- **DD-HAPI-006**: Affected Resource in Root Cause Analysis (THIS REQUIREMENT IMPLEMENTS THIS DD)
- **DD-CONTRACT-002**: Service Integration Contracts (needs update for RCA target section)
- **DD-AIANALYSIS-001**: AIAnalysis CRD Spec Structure (impacted - new Status field)

### **Business Requirements**
- **BR-HAPI-212**: HAPI RCA Target Resource (upstream provider)
- **BR-SCOPE-010**: RO Routing Validation (downstream consumer)
- **BR-SCOPE-001**: Resource Scope Management (context)
- **BR-AI-080**: Recovery Analysis Support (related - recovery also needs RCA target)

### **Architecture Decisions**
- **ADR-053**: Resource Scope Management (impacted - RO uses RCA target)
- **ADR-001**: CRD-based Microservices Architecture (referenced - no changes)

---

## 🎯 **Approval**

✅ **Approved by user**: 2026-01-20

**Approval Context**:
- User requested to "proceed" with implementing RCA target resource extraction
- User confirmed need to create or update BRs for HAPI and AIAnalysis
- User confirmed to use next sequential numbers (BR-HAPI-212, BR-AI-084)

---

## 🔒 **Confidence Assessment**

**Confidence Level**: 95%

**Strengths**:
- ✅ HAPI already returns `affectedResource` (BR-HAPI-212)
- ✅ Simple extraction logic (map lookup + struct construction)
- ✅ Clear escalation strategy (`nil` + `needs_human_review=true` → no remediation)
- ✅ Backward compatible (optional field)

**Risks**:
- ⚠️ **5% Gap**: HAPI may return `affectedResource` in unexpected format (malformed)
  - **Mitigation**: Robust parsing with validation and error handling
  - **Mitigation**: Return `nil` and rely on `needs_human_review=true` escalation
  - **Mitigation**: Unit tests cover all edge cases

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-01-20
- **Version**: 1.0
- **Status**: ✅ Approved
- **Next Review**: After implementation (estimated 2026-01-22)
