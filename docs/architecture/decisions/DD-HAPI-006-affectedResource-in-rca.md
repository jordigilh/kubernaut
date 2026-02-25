# DD-HAPI-006: Affected Resource in Root Cause Analysis

**Status**: ✅ Approved
**Version**: 1.1
**Date**: 2026-01-20
**Last Updated**: 2026-01-20 (Added apiVersion requirement)
**Confidence**: 95%
**Authority**: Authoritative (Approved)

---

## Context

HolmesGPT-API returns `root_cause_analysis` in its `/incident/analyze` response. This RCA includes a `summary`, `severity`, and `contributing_factors`. However, it lacked a clear, **structured field** for the **RCA-determined target resource** for remediation, which may differ from the signal source resource.

### Problem Statement

**Scenario**: A Pod crashes due to OOMKilled, but the remediation should target the parent Deployment (to increase memory limits), not the Pod itself.

- **Signal source**: `Pod/payment-api-xyz-123` (OOMKilled)
- **RCA target**: `Deployment/payment-api` (should increase memory limits)

**Gap**: Without a structured `affectedResource` field in the HAPI RCA response, AIAnalysis had no way to extract and store the RCA-determined target resource, leading to:
1. ❌ **Scope validation gaps** in RemediationOrchestrator (BR-SCOPE-001, BR-SCOPE-010)
2. ❌ **Audit trail gaps** - no clear record of which resource was remediated
3. ❌ **Incorrect remediation** - workflows could target the wrong resource
4. ❌ **Resource ambiguity** - multiple resources with same Kind/Name but different APIVersions

### Current State

**HAPI Code** (`holmesgpt-api/src/extensions/incident/result_parser.py` line 218):
```python
# Check if RCA-identified target is in OwnerChain
rca_target = rca.get("affectedResource") or rca.get("affected_resource")
if rca_target and owner_chain:
    target_in_owner_chain = is_target_in_owner_chain(rca_target, owner_chain, request_data)
```

**Observation**: HAPI **already extracts** `affectedResource` from the LLM response and validates it, but:
- ❌ It's **not documented** in the HAPI OpenAPI spec
- ❌ It's **not extracted** by AIAnalysis response processor
- ❌ It's **not stored** in AIAnalysis CRD status
- ❌ It's **not used** by RemediationOrchestrator for scope validation
- ❌ **apiVersion** not required → ambiguous resource identification

---

## Decision

### **CRITICAL: Two Different Escalation Flags**

**This decision involves `needs_human_review` - DO NOT CONFUSE with `needs_approval`:**

| Flag | Set By | Meaning | RO Action | User Experience |
|------|--------|---------|-----------|-----------------|
| **`needs_human_review`** | HAPI (this DD) | AI **can't** answer (RCA incomplete) | NotificationRequest | "Manual investigation needed" |
| **`needs_approval`** | AIAnalysis Rego | AI **has** answer, policy requires approval | RemediationApprovalRequest | "Approve remediation plan?" |

**Scenarios**:
- **HAPI**: Missing `affectedResource` + workflow selected → `needs_human_review=true` → NotificationRequest
- **AIAnalysis**: Complete RCA + production namespace → `needs_approval=true` → RemediationApprovalRequest

---

### 1. HAPI Contract Enhancement (BR-HAPI-212)

HolmesGPT-API's `/incident/analyze` endpoint **MUST** return an `affectedResource` object within `root_cause_analysis`:

```json
{
  "root_cause_analysis": {
    "summary": "Deployment has insufficient memory limits",
    "severity": "high",
    "contributing_factors": ["OOMKilled events recurring", "No HPA configured"],
    "affectedResource": {
      "kind": "Deployment",
      "apiVersion": "apps/v1",
      "name": "payment-api",
      "namespace": "production"
    }
  }
}
```

**Contract Guarantees**:
- `affectedResource` is **CONDITIONALLY REQUIRED**:
  - **REQUIRED** when `selected_workflow` is present (workflow matched → remediation planned)
  - **OPTIONAL** when `selected_workflow` is null (no workflow match → no remediation possible)
  - **OPTIONAL** when `investigation_outcome=problem_resolved` (BR-HAPI-200 → no remediation needed)
  - **OPTIONAL** when `needs_human_review=true` (investigation inconclusive/failed)
- **Required fields within `affectedResource`** when present:
  - **`kind`**: REQUIRED string - Kubernetes resource kind (e.g., "Deployment", "Pod")
  - **`apiVersion`**: OPTIONAL string - Kubernetes API version (e.g., "apps/v1", "v1")
    - When provided: Used for deterministic GVK resolution (prevents ambiguity with custom resources)
    - When missing: RO uses static mapping for core Kubernetes resources (Pod, Deployment, Service, etc.)
    - Recommended for signal sources that provide it (Kubernetes Events, Prometheus)
  - **`name`**: REQUIRED string - Resource name
  - **`namespace`**: CONDITIONALLY REQUIRED - Required for namespace-scoped resources, omit for cluster-scoped resources (e.g., Node, PersistentVolume). The CRD schema marks this field as `+optional` (Issue #192) so cluster-scoped targets pass K8s validation.
- **LLM response format** MUST include `affectedResource` when selecting a workflow for remediation
- **HAPI validation logic** (DD-HAPI-002 v1.2 - 3-attempt self-correction loop):
  - If `selected_workflow` present AND `affectedResource` missing → Set `needs_human_review=true`, `human_review_reason=rca_incomplete`
  - If `selected_workflow` is null → `affectedResource` not validated (no remediation planned)
  - If `investigation_outcome=problem_resolved` → `affectedResource` not validated (problem already resolved)
  - If `needs_human_review=true` (other reason) → `affectedResource` not validated (already requires review)
  - `apiVersion` is optional - no validation error if missing (RO uses static mapping for core resources)
- **HAPI validates `affectedResource` is in OwnerChain** when present (already implemented in `result_parser.py` line 218)

### 2. AIAnalysis CRD Enhancement (BR-AI-084)

AIAnalysis extracts and stores the RCA target in `Status.RootCauseAnalysis.TargetResource`:

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
    Kind       string `json:"kind"`
    APIVersion string `json:"apiVersion"`  // NEW: Required for GVK resolution
    Name       string `json:"name"`
    Namespace  string `json:"namespace,omitempty"`  // Empty for cluster-scoped resources
}
```

**Extraction Logic** (`pkg/aianalysis/handlers/response_processor.go`):
```go
if len(resp.RootCauseAnalysis) > 0 {
    rcaMap := GetMapFromOptNil(resp.RootCauseAnalysis)
    if rcaMap != nil {
        analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
            Summary:             GetStringFromMap(rcaMap, "summary"),
            Severity:            GetStringFromMap(rcaMap, "severity"),
            ContributingFactors: GetStringSliceFromMap(rcaMap, "contributing_factors"),

            // NEW: Extract affectedResource from RCA (includes apiVersion)
            TargetResource:      extractTargetResourceFromRCA(rcaMap),
        }
    }
}

func extractTargetResourceFromRCA(rcaMap map[string]interface{}) *aianalysisv1.TargetResource {
    affectedResource := rcaMap["affectedResource"]
    if affectedResource == nil {
        affectedResource = rcaMap["affected_resource"] // Try snake_case
    }
    if affectedResource == nil {
        return nil
    }

    arMap, ok := affectedResource.(map[string]interface{})
    if !ok {
        return nil
    }

    return &aianalysisv1.TargetResource{
        Kind:       GetStringFromMap(arMap, "kind"),
        APIVersion: GetStringFromMap(arMap, "apiVersion"),  // NEW
        Name:       GetStringFromMap(arMap, "name"),
        Namespace:  GetStringFromMap(arMap, "namespace"),
    }
}
```

### 3. RemediationOrchestrator Scope Validation (BR-SCOPE-010)

RemediationOrchestrator uses `AIAnalysis.Status.RootCauseAnalysis.TargetResource` for scope validation (Check #6 in routing):

```go
// Priority: RCA target > Signal source
func (r *Reconciler) CheckManagedResource(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) (bool, error) {
    // Get AIAnalysis CRD
    aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
    err := r.Get(ctx, client.ObjectKey{
        Name:      rr.Status.ChildCRDs.AIAnalysis.Name,
        Namespace: rr.Status.ChildCRDs.AIAnalysis.Namespace,
    }, aiAnalysis)

    // Get RCA-determined target resource (required for routing)
    if aiAnalysis.Status.RootCauseAnalysis == nil || aiAnalysis.Status.RootCauseAnalysis.TargetResource == nil {
        // No RCA target: AIAnalysis should have set needs_human_review=true
        // RO should have created NotificationRequest instead of reaching routing
        return false, fmt.Errorf("RCA target missing - escalation required")
    }

    targetResource := aiAnalysis.Status.RootCauseAnalysis.TargetResource

    // Use apiVersion to determine GVK for scope validation
    gvk := schema.GroupVersionKind{
        Group:   extractGroupFromAPIVersion(targetResource.APIVersion),
        Version: extractVersionFromAPIVersion(targetResource.APIVersion),
        Kind:    targetResource.Kind,
    }

    // Validate scope using metadata-only cache (ADR-053)
    isManaged, err := r.scopeManager.IsManaged(ctx,
        targetResource.Namespace,
        gvk,
        targetResource.Name,
    )

    return isManaged, err
}
```

### 4. Rego Policy Enhancement (BR-AI-085)

Rego policies receive `affected_resource` for workflow approval decisions:

```rego
# Example: Require approval if RCA targets production Deployment
package kubernaut.approval

require_approval if {
    # Check RCA-determined target (not signal source)
    input.affected_resource.kind == "Deployment"
    input.affected_resource.apiVersion == "apps/v1"
    input.affected_resource.namespace == "production"
    input.severity_level == "critical"
}
```

**PolicyInput Struct** (`pkg/aianalysis/rego/evaluator.go`):
```go
type PolicyInput struct {
    // ... existing fields ...

    // NEW: RCA-determined target resource (BR-AI-085)
    // This is the resource that WILL BE REMEDIATED (from HolmesGPT RCA)
    // May differ from target_resource (signal source)
    // Example: OOMKilled Pod → affected_resource = Deployment
    // +optional (nil if HAPI didn't determine different target)
    AffectedResource *TargetResourceInput `json:"affected_resource,omitempty"`
}

type TargetResourceInput struct {
    Kind       string `json:"kind"`
    APIVersion string `json:"api_version"`  // NEW: snake_case for Rego
    Name       string `json:"name"`
    Namespace  string `json:"namespace"`  // Empty for cluster-scoped
}
```

---

## Rationale

### Why apiVersion is Optional (Best-Effort)

**Architectural Decision: Optional apiVersion with Static Mapping Fallback**

**Rationale**:
- ✅ **Core resources** (Pod, Deployment, Service, Node, etc.) are the primary remediation targets
- ✅ **Static mapping** works reliably for core Kubernetes resources (apps/v1, v1, batch/v1)
- ✅ **CRDs** (custom resources) are configuration-related and less likely to be remediation targets
- ✅ **Pragmatic approach**: Start optional, make required later if custom resource remediation becomes common

**Potential Issue: Resource Ambiguity Without apiVersion**

```yaml
# Custom CRD in cluster
apiVersion: mycompany.io/v1
kind: Deployment
metadata:
  name: payment-api  # Cluster-scoped CRD

# Standard Kubernetes Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payment-api
  namespace: production
```

**Without apiVersion**:
- Signal says: `kind=Deployment, name=payment-api`
- **Which one?** Cannot determine!
- HAPI investigation: `kubectl get deployment payment-api` → **Ambiguous!**
- RCA determination: **Non-deterministic!**
- Scope validation: **Wrong resource checked!**

**With apiVersion (when provided)**:
- Signal says: `kind=Deployment, apiVersion=apps/v1, name=payment-api, namespace=production`
- ✅ Deterministic resource identification
- ✅ Correct GVK resolution via RESTMapper
- ✅ Accurate scope validation

**Without apiVersion (fallback to static mapping)**:
- Signal says: `kind=Deployment, name=payment-api, namespace=production`
- ✅ RO uses static mapping: `Deployment → apps/v1`
- ✅ Works for all core Kubernetes resources
- ⚠️ May be ambiguous if custom `Deployment` CRD exists (rare in practice)

### Gateway Best-Effort apiVersion Extraction

**Gateway Responsibility** (BR-SCOPE-002):
- Extract `apiVersion` from signal source if available (Kubernetes Events, Prometheus)
- If missing: No warning needed - optional field
- Pass through to HAPI for RCA

**HAPI Handling** (BR-HAPI-212):
- Accept `apiVersion` from LLM if provided (no validation required)
- If missing: No error - RO will use static mapping
- No self-correction loop needed for `apiVersion`

### Why This Matters

1. **Correctness**: Remediation should target the **root cause**, not just the **symptom**.
   - Example: OOMKilled Pod → remediate Deployment, not Pod

2. **Scope Control**: BR-SCOPE-001 requires validation of the **remediation target**, not the signal source.
   - Prevents remediating resources outside of Kubernaut's managed scope

3. **Audit Trail**: Clear traceability of which resource was remediated and why.
   - `AIAnalysis.Status.RootCauseAnalysis.TargetResource` provides audit evidence

4. **Flexibility**: Supports complex RCA scenarios:
   - Pod → Deployment
   - Node event → StatefulSet
   - ConfigMap → Deployment
   - Service → Ingress

5. **Deterministic Resource Identification**: Full GVK prevents ambiguity with custom resources.

### Why Not Alternatives?

#### Alternative 1: Use Signal Source Only
❌ **Rejected**: Doesn't handle cases where RCA target differs from signal source.
- Would remediate Pods instead of Deployments
- Would fail scope validation for unmanaged Pods

#### Alternative 2: Static Kind-to-Group Mapping Only
✅ **ADOPTED**: Works for core resources, optional apiVersion for edge cases.
- `kind=Deployment` → static mapping to `apps/v1` (works 99% of time)
- If `apiVersion` provided → use it (deterministic)
- Pragmatic: Start with optional, evaluate need for mandatory later

#### Alternative 3: Multiple Target Resources (List)
⏳ **Deferred to V2.0**: Current workflows support only one target.
- Future enhancement for storm scenarios (100 pods → 1 Deployment)
- Future enhancement for cascading failures (ConfigMap → 5 Deployments)

---

## Impacted Documents

### Business Requirements (Created/Updated)
1. **BR-HAPI-212** (NEW): HAPI must return RCA-determined target resource in `root_cause_analysis.affectedResource`
2. **BR-AI-084** (NEW): AIAnalysis must extract and store `affectedResource` from HAPI response
3. **BR-AI-085** (NEW): Rego Policy Input Schema - expose `affected_resource` to approval policies
4. **BR-SCOPE-010** (UPDATE): RemediationOrchestrator must validate scope using `AIAnalysis.Status.RootCauseAnalysis.TargetResource`
5. **BR-SCOPE-002** (REFERENCE): Gateway Signal Filtering (extract `apiVersion` from signal)

### Design Decisions (Created/Updated)
1. **DD-HAPI-006** (THIS DOCUMENT): Affected Resource in Root Cause Analysis
2. **DD-CONTRACT-002** (UPDATE): Service Integration Contracts - add RCA target section
3. **DD-HAPI-002 v1.2** (REFERENCE): Workflow Response Validation (3-attempt self-correction)
4. **DD-WORKFLOW-001 v1.7** (REFERENCE): OwnerChain validation (already implemented)

### Architecture Decisions (Updated)
1. **ADR-053** (UPDATE): Resource Scope Management - update RO validation section to use GVK from `apiVersion`
2. **ADR-001** (REFERENCE): CRD Spec Immutability (no changes - RCA target is in Status)

### API Specifications (Updated)
1. **HAPI OpenAPI Spec** (`holmesgpt-api/api/openapi.json`): Add `affectedResource` schema with `apiVersion` to `root_cause_analysis`
2. **HAPI Python Models** (`holmesgpt-api/src/models/incident_models.py`): Update docstring for `IncidentResponse.root_cause_analysis`
3. **AIAnalysis CRD** (`api/aianalysis/v1alpha1/aianalysis_types.go`): Add `TargetResource` field with `APIVersion` to `RootCauseAnalysis`
4. **RemediationRequest CRD** (`api/remediationrequest/v1alpha1/types.go`): Add `APIVersion` to `TargetResource`
5. **SignalProcessing CRD** (`api/signalprocessing/v1alpha1/types.go`): Add `APIVersion` to `TargetResource`

### Implementation Files (Updated)
1. **HAPI Result Parser** (`holmesgpt-api/src/extensions/incident/result_parser.py`): Update validation to require `apiVersion`
2. **HAPI Validation** (`holmesgpt-api/src/validation/workflow_response_validator.py`): Add `apiVersion` validation
3. **AIAnalysis Response Processor** (`pkg/aianalysis/handlers/response_processor.go`): Add extraction logic for `apiVersion`
4. **AIAnalysis Rego Evaluator** (`pkg/aianalysis/rego/evaluator.go`): Add `affected_resource` to `PolicyInput`
5. **Gateway Adapters** (`pkg/gateway/adapters/*.go`): Extract `apiVersion` from signal sources
6. **RemediationOrchestrator Scope Validator** (`pkg/remediationorchestrator/routing/scope_validator.go`): Use `apiVersion` for GVK resolution

### Documentation (Created/Updated)
1. **LLM Response Format Guide** (`holmesgpt-api/docs/LLM_RESPONSE_FORMAT.md`): Document `affectedResource` structure with `apiVersion` examples
2. **Scope Management Handoff** (`docs/handoff/BR_SCOPE_COMPLETE_DEFINITION_JAN20_2026.md`): Reference RCA target with `apiVersion` in RO validation

---

## Success Criteria

### Functional Success
1. ✅ HAPI returns `affectedResource` with `apiVersion` in RCA response (validated by unit tests with mockLLM)
2. ✅ AIAnalysis extracts and stores RCA target with `apiVersion` in Status (validated by unit tests)
3. ✅ RemediationOrchestrator uses RCA target `apiVersion` for GVK resolution (validated by unit tests)
4. ✅ Rego policies receive `affected_resource` with `api_version` (validated by unit tests)

### Quality Success
1. ✅ 100% unit test coverage for RCA target extraction with `apiVersion`
2. ✅ 100% unit test coverage for scope validation with GVK resolution
3. ✅ Unit tests cover cluster-scoped resources (empty `namespace`)
4. ✅ Unit tests cover custom resources (non-standard `apiVersion`)

### Documentation Success
1. ✅ HAPI OpenAPI spec documents `affectedResource` with `apiVersion`
2. ✅ LLM response format guide includes `affectedResource` examples with `apiVersion`
3. ✅ BR-HAPI-212, BR-AI-084, BR-AI-085 created and approved
4. ✅ BR-SCOPE-010 updated to reference RCA target with `apiVersion`
5. ✅ DD-CONTRACT-002 updated with RCA section

---

## Future Enhancements (V2.0)

### Multiple Target Resources
Support multiple affected resources in a single RCA:

```go
type RootCauseAnalysis struct {
    // ... existing fields ...

    // V2.0: Multiple targets (future)
    // +optional
    AffectedResources []TargetResource `json:"affectedResources,omitempty"`
}
```

**Use Cases**:
- **Storm scenarios**: 100 pods crashing → scale 1 Deployment
- **Cascading failures**: ConfigMap missing → affects 5 Deployments
- **Cluster-wide issues**: Node failure → affects all pods on node

---

## Approval

✅ **Approved by user**: 2026-01-20

**Approval Context**:
- User confirmed `apiVersion` is REQUIRED for deterministic resource identification
- User confirmed Gateway best-effort extraction with warning logs
- User confirmed HAPI validation with 3-attempt self-correction loop
- User confirmed Rego policies receive `affected_resource` with `api_version`
- User confirmed no metrics needed (existing validation metrics sufficient)
- User confirmed test plan using mockLLM for unit tests

---

## Confidence Assessment

**Confidence Level**: 95%

**Strengths**:
- ✅ HAPI already extracts `affectedResource` from LLM responses (line 218 of `result_parser.py`)
- ✅ Clear use cases and examples (OOMKilled Pod → Deployment)
- ✅ Aligns with existing BR-SCOPE-001 and BR-SCOPE-010
- ✅ apiVersion eliminates resource ambiguity with custom resources
- ✅ Escalation to human review when RCA target unavailable (safe default)

**Risks**:
- ⚠️ **5% Gap**: LLM may not always provide `apiVersion` consistently
  - **Mitigation**: HAPI 3-attempt self-correction loop (DD-HAPI-002 v1.2)
  - **Mitigation**: Escalates to `needs_human_review=true` after 3 attempts (no fallback)
  - **Mitigation**: Gateway provides `apiVersion` from signal source when available

**Validation**:
- Unit tests for RCA target extraction with `apiVersion`
- Unit tests for scope validation with GVK resolution
- Unit tests for Rego policy input with `affected_resource`
- HAPI contract documentation in OpenAPI spec

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-01-20 (v1.1 - Added apiVersion requirement)
- **Version**: 1.1
- **Status**: ✅ Approved
- **Next Review**: After implementation (estimated 2026-01-22)
