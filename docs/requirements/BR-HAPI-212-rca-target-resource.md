# BR-HAPI-212: RCA Target Resource in Root Cause Analysis

**Business Requirement ID**: BR-HAPI-212
**Category**: HolmesGPT API Service
**Priority**: P0
**Target Version**: V1.1
**Status**: ✅ Approved
**Date**: 2026-01-20
**Last Updated**: 2026-03-24 (Issue #524: resource-context tool split; conditional `TARGET_RESOURCE_*` injection)

**Related Design Decisions**:
- [DD-HAPI-006 v1.3: Affected Resource in Root Cause Analysis](../architecture/decisions/DD-HAPI-006-affectedResource-in-rca.md)
- [DD-CONTRACT-002: Service Integration Contracts](../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)
- [DD-WORKFLOW-001 v1.7: OwnerChain Validation](../architecture/decisions/DD-WORKFLOW-001-v1.7-ownerchain-validation.md)

**Related Business Requirements**:
- **BR-HAPI-197**: Human Review Required Flag (BASE REQUIREMENT - this BR extends it)
- BR-AI-084: AIAnalysis Extract RCA Target Resource
- BR-SCOPE-001: Resource Scope Management
- BR-SCOPE-010: RO Routing Validation

---

## 🔗 **Relationship to BR-HAPI-197**

**This BR extends BR-HAPI-197 (Human Review Required Flag)**:

| Document | Purpose | Relationship |
|----------|---------|--------------|
| **BR-HAPI-197** | Defines `needs_human_review` flag and 6 existing scenarios | BASE REQUIREMENT (approved Dec 6, 2025) |
| **BR-HAPI-212** (this BR) | Adds scenario #7: Missing `affectedResource` when workflow selected | EXTENSION (adds new trigger) |

**Existing BR-HAPI-197 Scenarios** (6 scenarios):
1. Workflow validation failures (workflow not found, image mismatch, parameter validation failed)
2. No suitable workflows matched
3. LLM parsing error
4. (Note: Low confidence threshold is AIAnalysis's responsibility, not HAPI's)

**NEW BR-HAPI-212 Scenario** (7th scenario):
- **Scenario 7**: `root_owner` missing from `session_state` when workflow is selected → `rca_incomplete` (BR-496 v2: HAPI owns target resource identity; `_inject_target_resource` cannot populate `affectedResource` or `TARGET_RESOURCE_*` without `root_owner`)

---

## 📋 **Business Need**

### **Problem Statement**

HolmesGPT-API performs Root Cause Analysis (RCA) on Kubernetes signals and identifies the **resource that should be remediated**. However, this RCA-determined target resource is **not clearly exposed** in the HAPI API contract, leading to:

1. ❌ **Scope validation gaps**: RemediationOrchestrator cannot validate if the RCA-determined target resource is managed by Kubernaut
2. ❌ **Audit trail gaps**: No clear record of which resource was identified for remediation by AI
3. ❌ **Incorrect remediation**: Workflows may target the wrong resource (symptom vs root cause)

**Example Scenario**:
- **Signal source**: `Pod/payment-api-xyz-123` (OOMKilled)
- **RCA analysis determines**: Root cause is insufficient memory limits on `Deployment/payment-api`
- **Remediation target**: `Deployment/payment-api` (not the Pod)

**Current Gap**: HAPI extracts `affectedResource` from LLM responses (line 218 of `result_parser.py`) but **does not expose it** in the OpenAPI spec or API contract, so AIAnalysis cannot consume it.

---

## 🎯 **Business Objective**

**Enable HolmesGPT-API to return the RCA-determined target resource in its `/incident/analyze` response, allowing AIAnalysis to extract and store this information for scope validation by RemediationOrchestrator.**

**Value Proposition**:
- ✅ **Correct Remediation**: Workflows target root cause, not symptom
- ✅ **Scope Control**: Kubernaut only remediates resources it's configured to manage
- ✅ **Audit Trail**: Clear record of which resource was identified by AI for remediation
- ✅ **Flexibility**: Supports complex RCA scenarios (Pod → Deployment, Node → StatefulSet, etc.)

---

## 🔍 **Functional Requirements**

### **FR-HAPI-212-001: RCA Target Resource Structure**

**Requirement**: HolmesGPT-API MUST include an `affectedResource` object in the `root_cause_analysis` response field.

**API Contract**:
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

**Field Specifications**:
- **`affectedResource`**: CONDITIONALLY REQUIRED object
  - **REQUIRED** when `selected_workflow` is present (workflow matched → remediation planned)
  - **OPTIONAL** when `selected_workflow` is null (no workflow match → no remediation possible)
  - **OPTIONAL** when `investigation_outcome=problem_resolved` (BR-HAPI-200 → no remediation needed)
  - **OPTIONAL** when `needs_human_review=true` (investigation inconclusive/failed)
- **Required fields within `affectedResource`** when present:
  - **`kind`**: REQUIRED string - Kubernetes resource kind (e.g., "Deployment", "Pod", "StatefulSet")
  - **`name`**: REQUIRED string - Resource name
  - **`namespace`**: CONDITIONALLY REQUIRED string
    - REQUIRED for namespace-scoped resources (Deployment, Pod, ConfigMap, etc.)
    - OMIT for cluster-scoped resources (Node, PersistentVolume, ClusterRole, etc.)
- **Optional fields within `affectedResource`**:
  - **`apiVersion`**: OPTIONAL string - Kubernetes API version (e.g., "apps/v1", "v1")
    - When provided: Used for deterministic GVK resolution (prevents ambiguity with custom resources)
    - When missing: RemediationOrchestrator uses static mapping for core Kubernetes resources
    - Recommended for signal sources that provide it (Kubernetes Events, Prometheus)
    - Rationale: Core resources are primary remediation targets; CRDs are configuration-related

**Validation** (DD-HAPI-002 v1.2 - 3-Attempt Self-Correction Loop):

**What HAPI DOES Validate**:
- **Workflow Selection Validation** (conditionally required):
  - If `selected_workflow` present AND `affectedResource` missing → Validation error → LLM self-correction attempt
  - If `selected_workflow` is null → Skip `affectedResource` validation (no remediation planned)
  - If `investigation_outcome=problem_resolved` → Skip `affectedResource` validation (problem already resolved)
  - If `needs_human_review=true` (other reason) → Skip `affectedResource` validation (already requires review)
  - After 3 failed attempts → Set `needs_human_review=true`, `human_review_reason=rca_incomplete`
- **Field Completeness Validation** (when `affectedResource` is provided):
  - `kind` and `name` MUST be non-empty strings → Validation error if missing
  - `apiVersion` is OPTIONAL → No validation error if missing (RO uses static mapping for core resources)
  - `namespace` validated based on resource scope (omitted for cluster-scoped resources is valid)
- **OwnerChain Validation** (when `affectedResource` is provided):
  - HAPI validates `affectedResource` is in the OwnerChain (already implemented in line 218 of `result_parser.py`)
  - If not in OwnerChain → Warning logged (not blocking)

**What HAPI does NOT Validate**:
- ❌ **Resource Existence**: HAPI does NOT validate that the resource exists in the Kubernetes cluster
  - **Rationale**: Temporal drift - resource may be deleted/recreated between HAPI analysis and WorkflowExecution
  - **Responsibility**: WorkflowExecution validates resource existence at execution time (latest state)
  - **Future Enhancement**: Could add optional existence check while LLM context is available (wait-and-see approach)
  - **Failure Mode**: If resource doesn't exist, WorkflowExecution will fail with clear error message

**Acceptance Criteria**:
1. ✅ `/incident/analyze` response includes `affectedResource` when LLM provides it
2. ✅ OpenAPI spec documents `affectedResource` structure
3. ✅ Python client generation includes `affectedResource` field
4. ✅ HAPI validates `affectedResource` is in OwnerChain before returning

---

### **FR-HAPI-212-002: LLM Prompt Engineering (Updated for BR-496 v2)**

**Requirement (v1.5)**: HolmesGPT-API MUST **NOT** instruct the LLM to provide `affectedResource`. Instead, prompts instruct the LLM to call the appropriate `resource_context` tool: `get_namespaced_resource_context` for namespace-scoped resources, or `get_cluster_resource_context` for cluster-scoped resources (e.g., Node, PV).

**Prompt Guidelines (v1.5)**:
```markdown
Phase 3b: Call get_namespaced_resource_context or get_cluster_resource_context
(whichever matches your RCA target's scope) with the resource identified in your RCA
to obtain the root managing resource when an owner chain applies (e.g., Pod → ReplicaSet → Deployment).
The root_owner returned by this tool identifies the actual remediation target.
```

**Rationale**: BR-496 v2 shifts target resource identity to HAPI. The LLM's role is to call the matching resource-context tool (which populates `root_owner` and `resource_scope` in `session_state`). HAPI then uses `root_owner` to inject `affectedResource`. **`TARGET_RESOURCE_NAME` / `TARGET_RESOURCE_KIND` / `TARGET_RESOURCE_NAMESPACE` are injected into `selected_workflow.parameters` only when those keys are declared in the workflow schema** (Issue #524 — no longer unconditionally mandatory).

**Acceptance Criteria**:
1. ✅ LLM prompts do **NOT** include guidance on providing `affectedResource`
2. ✅ LLM prompts instruct the LLM to call `get_namespaced_resource_context` or `get_cluster_resource_context` as appropriate
3. ✅ JSON examples in prompts do **NOT** contain `affectedResource`

---

### **FR-HAPI-212-003: OpenAPI Specification**

**Requirement**: HolmesGPT-API MUST update the OpenAPI spec to document the `affectedResource` field in `root_cause_analysis`.

**OpenAPI Schema**:
```yaml
root_cause_analysis:
  type: object
  required: [summary, severity]
  properties:
    summary:
      type: string
      description: Brief summary of root cause
    severity:
      type: string
      enum: [critical, high, medium, low, unknown]
      description: Severity determined by RCA
    contributing_factors:
      type: array
      items:
        type: string
      description: List of contributing factors
    affectedResource:
      type: object
      description: RCA-determined target resource for remediation (may differ from signal source)
      required: [kind, name, namespace]
      properties:
        kind:
          type: string
          description: Kubernetes resource kind (e.g., Deployment, Pod, StatefulSet)
        name:
          type: string
          description: Resource name
        namespace:
          type: string
          description: Resource namespace
```

**Acceptance Criteria**:
1. ✅ OpenAPI spec (`holmesgpt-api/api/openapi.json`) includes `affectedResource` schema
2. ✅ OpenAPI spec marks `affectedResource` as optional
3. ✅ OpenAPI spec includes descriptions for all fields
4. ✅ Python client generation reflects updated schema

---

### **FR-HAPI-212-004: Human Review Escalation (Extends BR-HAPI-197)**

**Requirement**: When the LLM does not provide `affectedResource` AND a workflow is selected, HolmesGPT-API MUST set `needs_human_review=true` to prevent remediating the wrong resource.

**This FR adds scenario #7 to BR-HAPI-197's existing 6 scenarios.**

**CRITICAL DISTINCTION**:
- **`needs_human_review`** (HAPI decision - BR-HAPI-197) = "AI **can't** answer" (RCA incomplete/unreliable)
- **`needs_approval`** (AIAnalysis Rego decision) = "AI **has** answer, but policy requires approval" (high-risk)

**Escalation Logic (v1.4 — BR-496 v2: HAPI-Owned Identity)**:

HAPI no longer relies on the LLM to provide `affectedResource`. Instead, `_inject_target_resource` derives it from `root_owner`:

1. **`root_owner` present in `session_state`** → HAPI injects `affectedResource` (from `root_owner`) into `root_cause_analysis`, and injects `TARGET_RESOURCE_NAME` / `TARGET_RESOURCE_KIND` / `TARGET_RESOURCE_NAMESPACE` into `selected_workflow.parameters` **only for parameters declared on the workflow schema** (Issue #524). Any LLM-provided `affectedResource` is unconditionally overwritten.
2. **`root_owner` missing from `session_state`** (tool not called or failed) → Set `needs_human_review=true`, `human_review_reason=rca_incomplete`. RO creates **NotificationRequest** (no remediation plan).
3. **No workflow selected** → `affectedResource` still injected if `root_owner` present (for audit completeness), but `TARGET_RESOURCE_*` params not injected (no workflow to inject into).

**Why NO Fallback to Signal Source**:
- ❌ Signal source = **Symptom** (e.g., OOMKilled Pod)
- ✅ RCA target = **Root Cause** (e.g., Deployment with insufficient memory)
- **Remediating the symptom without identifying root cause is dangerous**
- Missing `affectedResource` means RCA is incomplete → escalate to human

**All BR-HAPI-197 Escalation Scenarios** (7 total scenarios that set `needs_human_review=true`):

| # | Scenario | `human_review_reason` | BR Reference |
|---|----------|----------------------|--------------|
| 1 | Workflow Not Found | `workflow_not_found` | BR-HAPI-197 |
| 2 | Container Image Mismatch | `image_mismatch` | BR-HAPI-197 |
| 3 | Parameter Validation Failed | `parameter_validation_failed` | BR-HAPI-197 |
| 4 | No Workflows Matched | `no_matching_workflows` | BR-HAPI-197 |
| 5 | LLM Parsing Error | `llm_parsing_error` | BR-HAPI-197 |
| 6 | Investigation Inconclusive | `investigation_inconclusive` | BR-HAPI-200 |
| 7 | **`root_owner` missing from session_state** | `rca_incomplete` | **BR-HAPI-212 / BR-496 v2** |

> **Note (v1.4)**: Scenarios 8 (`affectedResource_mismatch`) and 9 (`unverified_target_resource`) were removed in BR-496 v2. HAPI now unconditionally derives `affectedResource` from `root_owner`, eliminating the mismatch and unverified scenarios. The only failure mode is `root_owner` being absent (scenario 7).

**NOT HAPI's Responsibility** (AIAnalysis makes these decisions):
- ❌ Low confidence threshold (AIAnalysis applies threshold, not HAPI - per BR-HAPI-197.2 note)
- ❌ High-risk remediation policy (production namespace, database resource)
  - → This is **AIAnalysis Rego** decision → sets `needs_approval=true`
  - → RO creates **RemediationApprovalRequest** (has remediation plan, awaiting approval)

**Acceptance Criteria**:
1. ✅ HAPI sets `needs_human_review=true` if workflow selected but `affectedResource` missing (after 3 attempts per DD-HAPI-002 v1.2)
2. ✅ HAPI sets `human_review_reason=rca_incomplete` (per BR-HAPI-197.3 warning correlation)
3. ✅ HAPI does NOT use signal source as fallback for missing `affectedResource`
4. ✅ AIAnalysis propagates `needs_human_review` to CRD status (per BR-HAPI-197.6)
5. ✅ RO creates NotificationRequest (not WorkflowExecution) when `needs_human_review=true` (per BR-HAPI-197.6)

---

## 📊 **Non-Functional Requirements**

### **NFR-HAPI-212-001: Backward Compatibility**

**Requirement**: The addition of `affectedResource` MUST NOT break existing AIAnalysis consumers.

**Backward Compatibility**:
- ✅ `affectedResource` is **optional** field in API contract
- ✅ Existing consumers that don't read `affectedResource` will continue to work
- ✅ Missing `affectedResource` triggers `needs_human_review=true` (escalation, not fallback)

---

### **NFR-HAPI-212-002: Performance Impact**

**Requirement**: Adding `affectedResource` MUST NOT degrade HAPI response time or LLM token usage.

**Performance Analysis**:
- ✅ `affectedResource` extraction is already implemented (line 218 of `result_parser.py`)
- ✅ No additional LLM calls required
- ✅ Minimal JSON serialization overhead (~100 bytes)
- ✅ No database queries or external API calls

**Acceptance Criteria**:
1. ✅ HAPI response time increase < 1ms
2. ✅ LLM token usage increase < 50 tokens (prompt guidance)
3. ✅ No additional external API calls

---

## 🔗 **Integration Points**

### **Upstream: LLM Providers**

**Integration**: HolmesGPT-API receives `affectedResource` from LLM in its JSON response.

**Contract**:
- LLM response includes `affectedResource` in `root_cause_analysis` object
- HAPI extracts and validates `affectedResource` (already implemented)

---

### **Downstream: AIAnalysis Service**

**Integration**: AIAnalysis consumes `affectedResource` from HAPI response and stores it in CRD status.

**Contract** (BR-AI-084):
- AIAnalysis reads `affectedResource` from `IncidentResponse.root_cause_analysis`
- AIAnalysis stores it in `Status.RootCauseAnalysis.TargetResource`
- AIAnalysis stores `Status.NeedsHumanReview=true` if HAPI indicates incomplete RCA

---

### **Downstream: RemediationOrchestrator Service**

**Integration**: RemediationOrchestrator reads `AIAnalysis.Status.RootCauseAnalysis.TargetResource` for scope validation.

**Contract** (BR-SCOPE-010):
- RemediationOrchestrator prioritizes RCA target over signal source
- RemediationOrchestrator validates RCA target is managed by Kubernaut
- RemediationOrchestrator blocks remediation if RCA target is unmanaged

**Defense-in-Depth (BR-ORCH-036 v4.0)**: The RO has its own guard that validates AffectedResource presence before routing. If HAPI and AA both miss the issue, the RO catches it and produces the same Failed + ManualReviewRequired response. See DD-HAPI-006 v1.3 for the complete four-layer model (including BR-496 mismatch detection).

---

## ✅ **Success Criteria**

### **Business Success**
1. ✅ 100% of remediation workflows target the correct resource (root cause, not symptom)
2. ✅ 0% of remediations target unmanaged resources when RCA target differs from signal source
3. ✅ 100% of audit trails include RCA-determined target resource

### **Technical Success**
1. ✅ HAPI returns `affectedResource` in 80%+ of RCA responses (LLM provides it)
2. ✅ AIAnalysis correctly extracts `affectedResource` in 100% of cases when provided
3. ✅ RemediationOrchestrator correctly validates RCA target in 100% of cases
4. ✅ OpenAPI spec validation passes for `affectedResource` schema

### **Quality Success**
1. ✅ HAPI unit tests cover `affectedResource` extraction
2. ✅ HAPI integration tests validate `affectedResource` in OwnerChain
3. ✅ E2E tests validate end-to-end flow (HAPI → AIAnalysis → RO → scope validation)

---

## 📈 **Business Value & Metrics**

### **Before (Current State)**
- ❌ 10-20% of remediations target the wrong resource (symptom vs root cause)
- ❌ 5-10% of remediations fail scope validation (unmanaged resources)
- ❌ No audit trail of RCA-determined target resource

### **After (Target State)**
- ✅ 100% of remediations target the correct resource (RCA-determined)
- ✅ 0% of remediations fail scope validation due to incorrect target
- ✅ 100% of audit trails include RCA-determined target resource

### **KPIs**
| Metric | Baseline | Target | Measurement |
|--------|----------|--------|-------------|
| Remediation accuracy (correct resource targeted) | 80% | 100% | Audit event analysis |
| Scope validation failures (incorrect target) | 5-10% | 0% | Prometheus `orchestrator_routing_blocked_total` |
| Audit trail completeness (RCA target recorded) | 0% | 100% | DataStorage `audit_events` table |

---

## 🚀 **Implementation Plan**

### **Phase 1: HAPI OpenAPI Spec Update**
1. Update `holmesgpt-api/api/openapi.json` to add `affectedResource` schema
2. Update `holmesgpt-api/src/models/incident_models.py` docstring
3. Regenerate Python client
4. Update LLM prompt templates to include `affectedResource` guidance

**Deliverables**:
- Updated OpenAPI spec
- Updated Python client
- LLM prompt documentation

**Timeline**: 1 day

---

### **Phase 2: HAPI Testing**
1. Add unit tests for `affectedResource` extraction
2. Add integration tests for OwnerChain validation
3. Update E2E tests to validate `affectedResource` presence

**Deliverables**:
- HAPI unit tests (5 test cases)
- HAPI integration tests (3 test cases)
- E2E test updates

**Timeline**: 1 day

---

### **Phase 3: Documentation**
1. Create LLM response format guide (`holmesgpt-api/docs/LLM_RESPONSE_FORMAT.md`)
2. Update HAPI API documentation
3. Update integration contract documentation (DD-CONTRACT-002)

**Deliverables**:
- LLM response format guide
- Updated API documentation
- Updated DD-CONTRACT-002

**Timeline**: 0.5 day

---

## 📚 **Related Documents**

### **Design Decisions**
- **DD-HAPI-006**: Affected Resource in Root Cause Analysis (THIS REQUIREMENT IMPLEMENTS THIS DD)
- **DD-CONTRACT-002**: Service Integration Contracts (needs update for RCA target section)
- **DD-WORKFLOW-001 v1.7**: OwnerChain Validation (referenced - already validates `affectedResource`)

### **Business Requirements**
- **BR-AI-084**: AIAnalysis Extract RCA Target Resource (downstream consumer)
- **BR-SCOPE-001**: Resource Scope Management (context)
- **BR-SCOPE-010**: RO Routing Validation (downstream consumer)
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
- ✅ HAPI already extracts `affectedResource` from LLM responses (line 218 of `result_parser.py`)
- ✅ Clear use cases and examples (OOMKilled Pod → Deployment)
- ✅ Minimal code changes required (OpenAPI spec + prompt updates)
- ✅ Backward compatible (optional field)

**Risks**:
- ⚠️ **3% Gap**: The LLM may not call `get_namespaced_resource_context` / `get_cluster_resource_context`, leaving `root_owner` absent
  - **Mitigation**: `_inject_target_resource` detects missing `root_owner` and sets `rca_incomplete`
  - **Mitigation**: Prompt Phase 3b explicitly instructs the LLM to call the correct resource-context tool for the target's scope
  - **Mitigation**: Layer 3 (RO) catches any remaining gaps at routing level

> **Note (v1.5)**: The LLM reliability risk for `affectedResource` (previously 5%) is eliminated by BR-496 v2. HAPI derives the field from K8s-verified `root_owner`, not from LLM output. The remaining risk is limited to the resource-context tool not being called. Canonical `TARGET_RESOURCE_*` workflow parameters are optional at the platform level until declared on a given workflow schema (Issue #524).

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-03-24 (Issue #524: tool rename/split, conditional `TARGET_RESOURCE_*` injection — updated FR-002, FR-004, risks)
