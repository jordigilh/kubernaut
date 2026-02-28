# DD-HAPI-003: Affected Resource in Root Cause Analysis

**Status**: ✅ Approved
**Version**: 1.1
**Date**: 2026-02-25
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

---

## Decision

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
      "name": "payment-api",
      "namespace": "production"
    }
  }
}
```

**Contract Guarantees**:
- `affectedResource` is **OPTIONAL** (defaults to signal source if not provided)
- When provided, `kind` and `name` are **REQUIRED**; `namespace` is optional for cluster-scoped resources (e.g., Node, PersistentVolume)
- LLM response format includes `affectedResource` in the RCA structure
- HAPI validates `affectedResource` is in OwnerChain (already implemented)

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
            
            // NEW: Extract affectedResource from RCA
            TargetResource:      extractTargetResourceFromRCA(rcaMap),
        }
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
    
    // Determine target resource for scope validation
    var targetResource *aianalysisv1alpha1.TargetResource
    if aiAnalysis.Status.RootCauseAnalysis != nil && aiAnalysis.Status.RootCauseAnalysis.TargetResource != nil {
        // Use RCA-determined target (priority)
        targetResource = aiAnalysis.Status.RootCauseAnalysis.TargetResource
    } else {
        // Fallback: Use signal source if RCA didn't identify a different target
        targetResource = &aianalysisv1alpha1.TargetResource{
            Kind:      rr.Spec.TargetResource.Kind,
            Name:      rr.Spec.TargetResource.Name,
            Namespace: rr.Spec.TargetResource.Namespace,
        }
    }
    
    // Validate scope
    isManaged, err := r.scopeManager.IsManaged(ctx, 
        targetResource.Namespace, 
        targetResource.Kind, 
        targetResource.Name,
    )
    
    return isManaged, err
}
```

---

## Rationale

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

### Why Not Alternatives?

#### Alternative 1: Use Signal Source Only
❌ **Rejected**: Doesn't handle cases where RCA target differs from signal source.
- Would remediate Pods instead of Deployments
- Would fail scope validation for unmanaged Pods

#### Alternative 2: Multiple Target Resources (List)
⏳ **Deferred to V2.0**: Current workflows support only one target.
- Future enhancement for storm scenarios (100 pods → 1 Deployment)
- Future enhancement for cascading failures (ConfigMap → 5 Deployments)

#### Alternative 3: Infer from Workflow Parameters
❌ **Rejected**: Workflows are selected AFTER RCA. Scope validation must happen BEFORE workflow selection.
- Creates circular dependency
- Doesn't support scope filtering at signal ingestion (Gateway)

---

## Implementation Checklist

### Phase 1: Documentation (This Document)
- [x] Create DD-HAPI-003
- [ ] Create BR-HAPI-212 (HAPI responsibility)
- [ ] Create BR-AI-084 (AIAnalysis responsibility)
- [ ] Update BR-SCOPE-010 (reference RCA target)
- [ ] Update DD-CONTRACT-002 (add RCA section)
- [ ] Update ADR-053 (resource scope management)

### Phase 2: HAPI Updates
- [ ] Update HAPI OpenAPI spec (`holmesgpt-api/api/openapi.json`)
- [ ] Update HAPI models docstring (`holmesgpt-api/src/models/incident_models.py`)
- [ ] Create LLM response format guide (`holmesgpt-api/docs/LLM_RESPONSE_FORMAT.md`)
- [ ] Regenerate HAPI Python client

### Phase 3: AIAnalysis CRD Updates
- [ ] Add `TargetResource` field to `RootCauseAnalysis` struct (`api/aianalysis/v1alpha1/aianalysis_types.go`)
- [ ] Run `make manifests` to regenerate CRD YAML
- [ ] Update response processor to extract `affectedResource` (`pkg/aianalysis/handlers/response_processor.go`)
- [ ] Add `extractTargetResourceFromRCA()` helper function
- [ ] Add unit tests for RCA target extraction

### Phase 4: RemediationOrchestrator Updates
- [ ] Create `pkg/remediationorchestrator/routing/scope_validator.go`
- [ ] Implement `CheckManagedResource()` with RCA target priority
- [ ] Integrate scope validator into routing engine (Check #6)
- [ ] Add unit tests for scope validation with RCA target
- [ ] Add integration tests for RO scope validation

### Phase 5: Validation
- [ ] Run integration tests (Gateway → AIAnalysis → RO)
- [ ] Run E2E tests (signal ingestion → scope validation → workflow execution)
- [ ] Verify audit trail includes RCA target resource
- [ ] Verify scope validation uses RCA target

---

## Impacted Documents

### Business Requirements (Created/Updated)
1. **BR-HAPI-212** (NEW): HAPI must return RCA-determined target resource in `root_cause_analysis.affectedResource`
2. **BR-AI-084** (NEW): AIAnalysis must extract and store `affectedResource` from HAPI response
3. **BR-SCOPE-010** (UPDATE): RemediationOrchestrator must validate scope using `AIAnalysis.Status.RootCauseAnalysis.TargetResource`
4. **BR-SCOPE-001** (REFERENCE): Resource scope management (no changes needed)

### Design Decisions (Created/Updated)
1. **DD-HAPI-003** (THIS DOCUMENT): Affected Resource in Root Cause Analysis
2. **DD-CONTRACT-002** (UPDATE): Service Integration Contracts - add RCA target section
3. **DD-WORKFLOW-001 v1.7** (REFERENCE): OwnerChain validation (already implemented)
4. **DD-AUDIT-CORRELATION-001** (REFERENCE): Audit correlation standards (no changes)

### Architecture Decisions (Updated)
1. **ADR-053** (UPDATE): Resource Scope Management - update RO validation section to reference `AIAnalysis.Status.RootCauseAnalysis.TargetResource`
2. **ADR-001** (REFERENCE): CRD Spec Immutability (no changes - RCA target is in Status)

### API Specifications (Updated)
1. **HAPI OpenAPI Spec** (`holmesgpt-api/api/openapi.json`): Add `affectedResource` schema to `root_cause_analysis`
2. **HAPI Python Models** (`holmesgpt-api/src/models/incident_models.py`): Update docstring for `IncidentResponse.root_cause_analysis`
3. **AIAnalysis CRD** (`api/aianalysis/v1alpha1/aianalysis_types.go`): Add `TargetResource` field to `RootCauseAnalysis`

### Implementation Files (Updated)
1. **HAPI Result Parser** (`holmesgpt-api/src/extensions/incident/result_parser.py`): Already extracts `affectedResource` (line 218) - no changes needed
2. **AIAnalysis Response Processor** (`pkg/aianalysis/handlers/response_processor.go`): Add extraction logic for `affectedResource`
3. **RemediationOrchestrator Scope Validator** (`pkg/remediationorchestrator/routing/scope_validator.go`): NEW FILE - implement `CheckManagedResource()`
4. **RemediationOrchestrator Routing** (`pkg/remediationorchestrator/routing/blocking.go`): Integrate scope validator as Check #6

### Documentation (Created/Updated)
1. **LLM Response Format Guide** (`holmesgpt-api/docs/LLM_RESPONSE_FORMAT.md`): NEW FILE - document `affectedResource` structure and examples
2. **Scope Management Handoff** (`docs/handoff/BR_SCOPE_COMPLETE_DEFINITION_JAN20_2026.md`): UPDATE - reference RCA target in RO validation

### Test Files (Created)
1. **AIAnalysis Response Processor Unit Tests** (`pkg/aianalysis/handlers/response_processor_test.go`): Add tests for `extractTargetResourceFromRCA()`
2. **RO Scope Validator Unit Tests** (`pkg/remediationorchestrator/routing/scope_validator_test.go`): NEW FILE - test `CheckManagedResource()`
3. **RO Scope Validation Integration Tests** (`test/integration/remediationorchestrator/scope_validation_test.go`): NEW FILE - test end-to-end scope validation

---

## Related Documents

### Design Decisions
- **DD-CONTRACT-002**: Service Integration Contracts (impacted - needs RCA section)
- **DD-WORKFLOW-001 v1.7**: OwnerChain validation (referenced - no changes)
- **DD-AIANALYSIS-001**: AIAnalysis CRD Spec Structure (referenced - no changes)
- **DD-SEVERITY-001**: Severity Determination Refactoring (referenced - no changes)

### Business Requirements
- **BR-SCOPE-001**: Resource Scope Management (referenced - no changes)
- **BR-SCOPE-002**: Gateway Signal Filtering (referenced - no changes)
- **BR-SCOPE-010**: RO Routing Validation (impacted - needs update)
### Architecture Decisions
- **ADR-053**: Resource Scope Management (impacted - needs update)
- **ADR-001**: CRD-based Microservices Architecture (referenced - no changes)

---

## Success Criteria

### Functional Success
1. ✅ HAPI returns `affectedResource` in RCA response (validated by integration tests)
2. ✅ AIAnalysis extracts and stores RCA target in Status (validated by unit tests)
3. ✅ RemediationOrchestrator uses RCA target for scope validation (validated by integration tests)
4. ✅ Audit trail includes RCA target resource (validated by E2E tests)

### Quality Success
1. ✅ 100% unit test coverage for RCA target extraction
2. ✅ 100% unit test coverage for scope validation logic
3. ✅ Integration tests pass for Gateway → AIAnalysis → RO flow
4. ✅ E2E tests pass for signal ingestion → scope validation → workflow execution

### Documentation Success
1. ✅ HAPI OpenAPI spec documents `affectedResource`
2. ✅ LLM response format guide includes `affectedResource` examples
3. ✅ BR-HAPI-212 and BR-AI-084 created and approved
4. ✅ BR-SCOPE-010 updated to reference RCA target
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

### Resource Hierarchy Support
Support hierarchical resource relationships:

```go
type TargetResource struct {
    Kind      string   `json:"kind"`
    Name      string   `json:"name"`
    // +optional — empty for cluster-scoped resources (e.g., Node, PersistentVolume)
    Namespace string   `json:"namespace,omitempty"`
    
    // V2.0: Resource hierarchy (future)
    // +optional
    Parents   []ObjectReference `json:"parents,omitempty"`
    Children  []ObjectReference `json:"children,omitempty"`
}
```

**Use Cases**:
- **Ownership tracking**: Pod → ReplicaSet → Deployment
- **Dependency tracking**: Service → Deployment → ConfigMap
- **Impact analysis**: Node → Pods → Deployments

---

## Approval

✅ **Approved by user**: 2026-01-20

**Approval Context**:
- User requested to "proceed" with implementing RCA target resource extraction
- User confirmed need to update authoritative documentation in HAPI and AIAnalysis
- User requested to include references to impacted documents and new DDs
- User confirmed need to create or update BRs for AA and HAPI

---

## Confidence Assessment

**Confidence Level**: 95%

**Strengths**:
- ✅ HAPI already extracts `affectedResource` from LLM responses (line 218 of `result_parser.py`)
- ✅ Clear use cases and examples (OOMKilled Pod → Deployment)
- ✅ Aligns with existing BR-SCOPE-001 and BR-SCOPE-010
- ✅ Minimal breaking changes (new optional field)
- ✅ Clear fallback strategy (use signal source if RCA target not provided)

**Risks**:
- ⚠️ **5% Gap**: LLM may not always provide `affectedResource` consistently
  - **Mitigation**: Make field optional, fall back to signal source
  - **Mitigation**: Add LLM prompt engineering to encourage `affectedResource` output

**Validation**:
- Unit tests for RCA target extraction
- Integration tests for scope validation
- E2E tests for end-to-end flow
- HAPI contract documentation in OpenAPI spec

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-02-25
- **Version**: 1.1
- **Status**: ✅ Approved
- **Next Review**: After implementation (estimated 2026-01-22)

**Changelog**:
| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-01-20 | Initial document |
| 1.1 | 2026-02-25 | Issue #192: `AffectedResource.Namespace` changed from required to optional for cluster-scoped resources (Node, PersistentVolume). Updated contract guarantees and TargetResource struct. |
