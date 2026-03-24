# DD-HAPI-006: Affected Resource in Root Cause Analysis

**Status**: ✅ Approved
**Version**: 1.4
**Date**: 2026-02-24
**Last Updated**: 2026-03-04 (BR-496 v2: HAPI-owned target resource identity)
**Confidence**: 97%
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

### Current State (v1.4 — BR-496 v2)

**HAPI Code** (`holmesgpt-api/src/extensions/incident/llm_integration.py`):
```python
# BR-496 v2: HAPI owns the target resource identity.
# _inject_target_resource derives affectedResource from K8s-verified root_owner
# stored in session_state by get_resource_context.
_inject_target_resource(result, session_state, remediation_id)
```

**Architecture (v1.4)**: HAPI **owns** target resource identity — the LLM never provides `affectedResource`. Instead:
- ✅ `get_resource_context` resolves the K8s owner chain and stores `root_owner` in `session_state`
- ✅ `_inject_target_resource` derives `affectedResource` from `root_owner` (K8s-verified)
- ✅ `_inject_target_resource` injects `TARGET_RESOURCE_NAME/KIND/NAMESPACE` into workflow params
- ✅ Prompts do **not** instruct the LLM to provide `affectedResource`
- ✅ If `root_owner` is missing → `needs_human_review=true`, `human_review_reason=rca_incomplete`

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

### 1. HAPI Contract Enhancement (BR-HAPI-212, BR-496 v2)

HolmesGPT-API's `/incident/analyze` endpoint **MUST** return an `affectedResource` object within `root_cause_analysis`, **derived by HAPI from `root_owner`** (not from the LLM):

```json
{
  "root_cause_analysis": {
    "summary": "Deployment has insufficient memory limits",
    "severity": "high",
    "contributing_factors": ["OOMKilled events recurring", "No HPA configured"],
    "affectedResource": {
      "kind": "Deployment",
      "name": "payment-api",
      "namespace": "production"
    }
  },
  "selected_workflow": {
    "workflow_id": "oomkill-increase-memory-v1",
    "parameters": {
      "TARGET_RESOURCE_NAME": "payment-api",
      "TARGET_RESOURCE_KIND": "Deployment",
      "TARGET_RESOURCE_NAMESPACE": "production",
      "MEMORY_LIMIT_NEW": "256Mi"
    }
  }
}
```

**Contract Guarantees (v1.4 — HAPI-Owned Identity)**:
- `affectedResource` is **HAPI-injected** (not LLM-provided):
  - `get_resource_context` resolves the K8s owner chain (Pod → ReplicaSet → Deployment) and stores `root_owner` in `session_state`
  - `_inject_target_resource` copies `root_owner` into `root_cause_analysis.affectedResource` for Go backward compat
  - `_inject_target_resource` injects `TARGET_RESOURCE_NAME`, `TARGET_RESOURCE_KIND`, `TARGET_RESOURCE_NAMESPACE` into `selected_workflow.parameters`
  - If `root_owner` is missing from `session_state` → Set `needs_human_review=true`, `human_review_reason=rca_incomplete`
- **Fields within `affectedResource`** (derived from `root_owner`):
  - **`kind`**: REQUIRED string — Kubernetes resource kind (e.g., "Deployment", "StatefulSet")
  - **`name`**: REQUIRED string — Resource name
  - **`namespace`**: CONDITIONALLY REQUIRED — Present for namespace-scoped resources, omitted for cluster-scoped resources (e.g., Node, PersistentVolume). The CRD schema marks this field as `+optional` (Issue #192).
- **Canonical workflow parameters** (HAPI-managed, not LLM-provided):
  - `TARGET_RESOURCE_NAME`: Root owner name (mandatory)
  - `TARGET_RESOURCE_KIND`: Root owner kind (mandatory)
  - `TARGET_RESOURCE_NAMESPACE`: Root owner namespace (present for namespaced resources)
- **Workflow schema contract**: All workflow schemas **MUST** declare the three canonical parameters (`TARGET_RESOURCE_NAME`, `TARGET_RESOURCE_KIND`, `TARGET_RESOURCE_NAMESPACE`). The `WorkflowResponseValidator` Step 0 rejects schemas that don't declare them.
- **Schema stripping**: `get_workflow` strips canonical parameters from the schema before returning it to the LLM, preventing the LLM from providing values that HAPI will overwrite.
- **LLM prompt**: Does **NOT** instruct the LLM to provide `affectedResource`. The LLM focuses on calling `get_resource_context` to identify the root owner.

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
1. **HAPI LLM Integration** (`holmesgpt-api/src/extensions/incident/llm_integration.py`): v1.4 — `_inject_target_resource()` replaces `_check_affected_resource_mismatch()`. Derives `affectedResource` and `TARGET_RESOURCE_*` from `session_state["root_owner"]`.
2. **HAPI Resource Context Tool** (`holmesgpt-api/src/toolsets/resource_context.py`): v1.3 — Store K8s-verified `root_owner` in `session_state`
3. **HAPI Prompt Builder** (`holmesgpt-api/src/extensions/incident/prompt_builder.py`): v1.4 — Removed all `affectedResource` instructions. LLM focuses on `get_resource_context`.
4. **HAPI Result Parser** (`holmesgpt-api/src/extensions/incident/result_parser.py`): v1.4 — Removed BR-HAPI-212 `rca_incomplete` check for missing `affectedResource` (superseded by `_inject_target_resource`).
5. **HAPI Validation** (`holmesgpt-api/src/validation/workflow_response_validator.py`): v1.4 — Added `HAPI_MANAGED_PARAMS` constant, `_validate_canonical_params` (Step 0), and skip required-check for HAPI-managed params.
6. **HAPI Workflow Discovery** (`holmesgpt-api/src/toolsets/workflow_discovery.py`): v1.4 — `strip_hapi_managed_params()` removes `TARGET_RESOURCE_*` from schema before returning to LLM.
7. **AIAnalysis Response Processor** (`pkg/aianalysis/handlers/response_processor.go`): `ExtractRootCauseAnalysis` maps `affectedResource` from HAPI response into CRD.
8. **AIAnalysis Rego Evaluator** (`pkg/aianalysis/rego/evaluator.go`): `affected_resource` in `PolicyInput`.
9. **RemediationOrchestrator Scope Validator** (`pkg/remediationorchestrator/routing/scope_validator.go`): Uses `AffectedResource` for scope validation.

### Documentation (Created/Updated)
1. **LLM Response Format Guide** (`holmesgpt-api/docs/LLM_RESPONSE_FORMAT.md`): Document `affectedResource` structure with `apiVersion` examples
2. **Scope Management Handoff**: Reference RCA target with `apiVersion` in RO validation (internal development reference, removed in v1.0)

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

## Defense-in-Depth: Three-Layer AffectedResource Validation (v1.4)

Three independent layers ensure `affectedResource` is always populated correctly, producing a consistent operator experience regardless of which layer catches an issue:

### Layer 1: HAPI Injection (Authoritative Source — BR-496 v2)
- **File**: `holmesgpt-api/src/extensions/incident/llm_integration.py` (`_inject_target_resource`)
- **Mechanism**: `get_resource_context` resolves the K8s owner chain and stores `root_owner` in `session_state`. After the self-correction loop, `_inject_target_resource` unconditionally sets `affectedResource` from `root_owner` and injects `TARGET_RESOURCE_*` into workflow parameters.
- **Missing root_owner**: If `root_owner` is absent from `session_state` (tool never called or failed) → `needs_human_review=true`, `human_review_reason=rca_incomplete`.
- **LLM overwrite**: Any `affectedResource` the LLM might include is unconditionally overwritten by the K8s-verified `root_owner`.
- **Schema stripping**: `get_workflow` strips `TARGET_RESOURCE_*` from the schema before returning it to the LLM, so the LLM never sees or populates these params.
- **Reference**: BR-496 v2, ADR-056 v1.4 (session_state)

### Layer 2: AIAnalysis (Extraction Level)
- **File**: `pkg/aianalysis/handlers/response_processor.go` (`ExtractRootCauseAnalysis`)
- **Check**: Only stores `AffectedResource` when `kind != ""` AND `name != ""`. Otherwise stays nil.
- **Reference**: DD-HAPI-006 Section 2

### Layer 3: RemediationOrchestrator (Routing Level)
- **File**: `internal/controller/remediationorchestrator/reconciler.go`
- **Check**: If `AffectedResource` is nil or has empty Kind/Name → `HandleAffectedResourceMissing` → Failed + ManualReviewRequired + NotificationRequest
- **Preconditions**: `WorkflowNotNeeded` and `ApprovalRequired` are already checked before this guard runs. Reaching the guard means a genuine data integrity issue.
- **Reference**: BR-ORCH-036 v4.0

**Operator Experience**: All three layers produce the same response when target resource cannot be determined:
- RR transitions to `Failed`
- `RequiresManualReview = true`
- `NotificationRequest` created with `type=manual-review`
- K8s Warning event emitted

**`human_review_reason` value**:

| Reason | Layer | When |
|--------|-------|------|
| `rca_incomplete` | 1 | `root_owner` absent from `session_state` (get_resource_context not called or failed) |

> **Note (v1.4)**: The `affectedResource_mismatch` and `unverified_target_resource` enum values were removed in BR-496 v2. HAPI now unconditionally derives `affectedResource` from `root_owner`, eliminating the possibility of LLM/K8s mismatch. The only failure mode is `root_owner` being absent, which produces `rca_incomplete`.

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

**Confidence Level**: 97%

**Strengths**:
- ✅ HAPI derives `affectedResource` from K8s-verified `root_owner` — eliminates LLM reliability dependency
- ✅ Clear use cases and examples (OOMKilled Pod → Deployment)
- ✅ Aligns with existing BR-SCOPE-001 and BR-SCOPE-010
- ✅ Canonical `TARGET_RESOURCE_*` params ensure workflow jobs get correct resource identity
- ✅ Schema stripping prevents LLM from providing incorrect values
- ✅ Escalation to human review when `root_owner` unavailable (safe default)
- ✅ 21 unit tests + 3 E2E tests covering all injection, validation, stripping, prompt, and parser scenarios

**Risks**:
- ⚠️ **3% Gap**: `get_resource_context` may fail or not be called by the LLM
  - **Mitigation**: `_inject_target_resource` detects missing `root_owner` and sets `rca_incomplete`
  - **Mitigation**: Prompt Phase 3b instructs LLM to call `get_resource_context` first
  - **Mitigation**: Layer 3 (RO) catches any remaining gaps

**Validation**:
- 21 Python unit tests covering injection, schema validation, schema stripping, prompt/parser, and enum cleanup
- 3 E2E tests (E2E-FP-496-001/002/003) validating full pipeline affectedResource and TARGET_RESOURCE_* injection
- Go build validation confirms CRD enum cleanup compiles cleanly

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2026-01-20 | Added apiVersion requirement |
| 1.2 | 2026-02-24 | Added defense-in-depth documentation. Section 5 describes the three-layer model (HAPI → AA → RO) for handling missing AffectedResource. RO guard produces same seamless response as HAPI/AA layers: Failed + ManualReviewRequired + NotificationRequest (BR-ORCH-036 v4.0). |
| 1.3 | 2026-03-04 | BR-496 v1: Added Layer 1b — affectedResource mismatch detection. `get_resource_context` stores K8s-verified `root_owner` in `session_state`; post-loop check compares LLM's `affectedResource` against `root_owner`. Two new `human_review_reason` values: `affectedResource_mismatch`, `unverified_target_resource`. |
| 1.4 | 2026-03-04 | BR-496 v2: HAPI-owned target resource identity. Replaced Layer 1b mismatch detection with `_inject_target_resource` — HAPI unconditionally derives `affectedResource` from `root_owner`. Removed `affectedResource_mismatch` and `unverified_target_resource` enum values. Added canonical `TARGET_RESOURCE_*` param injection and schema stripping. Simplified defense-in-depth from 4 layers to 3. Prompt no longer instructs LLM to provide `affectedResource`. Validator Step 0 enforces canonical params in workflow schemas. |

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-03-04 (v1.4 - BR-496 v2 HAPI-owned target resource identity)
- **Version**: 1.4
- **Status**: ✅ Approved
- **Next Review**: After v1.1.1 release validation
