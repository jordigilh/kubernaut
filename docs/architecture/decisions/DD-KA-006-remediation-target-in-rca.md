# DD-KA-006: Remediation Target in Root Cause Analysis

**Status**: ✅ Approved
**Version**: 2.0 (Go rewrite)
**Date**: 2026-02-24
**Last Updated**: 2026-08-01 (Rewritten against the Go Kubernaut Agent (KA) implementation; superseded field name `affectedResource` → `remediationTarget`, superseded "session_state" model → direct enrichment-result plumbing)
**Confidence**: 95%
**Authority**: Authoritative (Approved)

---

## Context

Kubernaut Agent (KA) returns `root_cause_analysis` in its investigation response. This RCA includes a `summary`, `severity`, and `contributing_factors`, plus a structured **`remediationTarget`** field for the **RCA-determined target resource** for remediation, which may differ from the signal source resource.

### Problem Statement

**Scenario**: A Pod crashes due to OOMKilled, but the remediation should target the parent Deployment (to increase memory limits), not the Pod itself.

- **Signal source**: `Pod/payment-api-xyz-123` (OOMKilled)
- **RCA target**: `Deployment/payment-api` (should increase memory limits)

**Gap** (pre-KA): Without a structured target-resource field in the RCA response, AIAnalysis had no way to extract and store the RCA-determined target resource, leading to:
1. ❌ **Scope validation gaps** in RemediationOrchestrator (BR-SCOPE-001, BR-SCOPE-010)
2. ❌ **Audit trail gaps** — no clear record of which resource was remediated
3. ❌ **Incorrect remediation** — workflows could target the wrong resource
4. ❌ **Resource ambiguity** — multiple resources with same Kind/Name but different APIVersions

### Current State (Go KA)

**KA Code** (`internal/kubernautagent/investigator/investigator_phases.go`, `InjectRemediationTarget`):

KA resolves the authoritative remediation target from K8s-verified data, then reconciles it against whatever target the LLM proposed. Root resolution priority:
1. **Owner-chain last entry** (most common: Pod → ReplicaSet → Deployment)
2. **Enrichment source identity** — used after re-enrichment, when the owner chain is already empty because the enriched resource *is* the root (#694)
3. **Signal identity** — fallback when no enrichment data is available at all

LLM-target reconciliation:
- If the LLM's proposed `Kind` is empty or matches the resolved root's `Kind` → the K8s-verified root identity **overrides** the LLM's target (the LLM's `apiVersion` is preserved when the kind matches, since same kind implies same API group, #1040)
- If the LLM's proposed `Kind` is a **descendant** in the owner chain (e.g., it names the Pod when the resolved root is the Deployment) → KA resolves **upward** to the K8s-verified root owner
- Only when the LLM's `Kind` is genuinely **cross-type** — not the signal's kind and not present anywhere in the owner chain (e.g., `Node` when the root is `Deployment`) — is the LLM's proposed target preserved as-is

`InjectTargetResourceParameters` (same file) then unconditionally derives `TARGET_RESOURCE_NAME`, `TARGET_RESOURCE_KIND`, `TARGET_RESOURCE_NAMESPACE`, and (when present) `TARGET_RESOURCE_API_VERSION` from the authoritative `RemediationTarget` into the workflow's `Parameters` map. These four parameters are registered as `kaManagedParams` in `internal/kubernautagent/parser/validator.go` — they are excluded from schema validation and are **never stripped** during undeclared-parameter stripping, regardless of whether the selected workflow's schema declares them.

> **Note**: this supersedes the pre-Go design, under which target-resource injection was conditional on the workflow schema declaring the canonical parameter slots. The Go implementation always injects and always preserves these four parameters.

**Escalation**: `HumanReviewReasonRCAIncomplete` (wire value `rca_incomplete`) is set when:
- Owner-chain re-enrichment **hard-fails** after retry exhaustion (`internal/kubernautagent/enrichment/enricher.go`, `EnrichmentResult.HardFail`; NotFound errors are exempt as they indicate a deleted resource, #1039), handled in `internal/kubernautagent/investigator/investigator_discovery.go` (`reEnrichForRCATargetShift`)
- The RCA phase exhausts its turn budget before producing a usable result (`internal/kubernautagent/investigator/investigator_gates.go`)

---

## Decision

### **CRITICAL: Two Different Escalation Flags**

**This decision involves `needs_human_review` — DO NOT CONFUSE with `needs_approval`:**

| Flag | Set By | Meaning | RO Action | User Experience |
|------|--------|---------|-----------|-----------------|
| **`needs_human_review`** | KA (this DD) | AI **can't** answer (RCA incomplete) | NotificationRequest | "Manual investigation needed" |
| **`needs_approval`** | AIAnalysis Rego | AI **has** answer, policy requires approval | RemediationApprovalRequest | "Approve remediation plan?" |

**Scenarios**:
- **KA**: Owner-chain resolution hard-fails or RCA turn budget exhausted → `needs_human_review=true` (`rca_incomplete`) → NotificationRequest
- **AIAnalysis**: Complete RCA + production namespace → `needs_approval=true` → RemediationApprovalRequest

---

### 1. KA Contract Enhancement (BR-KA-212, BR-496 v2)

KA's investigation response **MUST** return a `remediationTarget` object within `root_cause_analysis`, **derived by KA from the K8s-verified root owner** (not taken verbatim from the LLM):

```json
{
  "root_cause_analysis": {
    "summary": "Deployment has insufficient memory limits",
    "severity": "high",
    "contributing_factors": ["OOMKilled events recurring", "No HPA configured"],
    "remediationTarget": {
      "kind": "Deployment",
      "name": "payment-api",
      "namespace": "production",
      "api_version": "apps/v1"
    }
  },
  "selected_workflow": {
    "workflow_id": "oomkill-increase-memory-v1",
    "parameters": {
      "TARGET_RESOURCE_NAME": "payment-api",
      "TARGET_RESOURCE_KIND": "Deployment",
      "TARGET_RESOURCE_NAMESPACE": "production",
      "TARGET_RESOURCE_API_VERSION": "apps/v1",
      "MEMORY_LIMIT_NEW": "256Mi"
    }
  }
}
```

**Contract Guarantees (Go KA)**:
- `remediationTarget` is **KA-injected**, not LLM-verbatim: `InjectRemediationTarget` reconciles the LLM's proposed target against the K8s-verified root owner (see resolution priority above) before the response is built
- `InjectTargetResourceParameters` copies the resolved `RemediationTarget` into the four `TARGET_RESOURCE_*` workflow parameters, which are always preserved (`kaManagedParams`) regardless of workflow schema declarations
- If owner-chain resolution hard-fails → `needs_human_review=true`, `human_review_reason=rca_incomplete`
- **Fields within `remediationTarget`**:
  - **`kind`**: REQUIRED string — Kubernetes resource kind (e.g., "Deployment", "StatefulSet")
  - **`name`**: REQUIRED string — Resource name
  - **`namespace`**: CONDITIONALLY REQUIRED — Present for namespace-scoped resources, omitted for cluster-scoped resources (e.g., Node, PersistentVolume)
  - **`api_version`** (wire, snake_case): OPTIONAL — disambiguates the resource's API group when the Kind exists in multiple groups (e.g. `Route` in `route.openshift.io` vs `serving.knative.dev`), #1040

> **Known gap**: `internal/kubernautagent/api/openapi.json` does not yet document the `remediationTarget` schema. Tracked as a follow-up alongside the other OpenAPI spec accuracy gaps noted in [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806).

### 2. AIAnalysis CRD Enhancement (BR-AI-084)

AIAnalysis extracts and stores the RCA target in `Status.RootCauseAnalysis.RemediationTarget`:

```go
// api/aianalysis/v1alpha1/aianalysis_types.go
type RootCauseAnalysis struct {
    Summary             string   `json:"summary"`
    Severity            string   `json:"severity,omitempty"`
    SignalType          string   `json:"signalType"`
    ContributingFactors []string `json:"contributingFactors,omitempty"`

    // RemediationTarget identifies the actual resource the LLM/KA determined
    // should be remediated. May differ from the signal source resource.
    // RemediationOrchestrator prefers this over the RR's own TargetResource
    // when available (BR-KA-212).
    // +optional
    RemediationTarget *RemediationTarget `json:"remediationTarget,omitempty"`
}

type RemediationTarget struct {
    Kind       string `json:"kind"`
    Name       string `json:"name"`
    Namespace  string `json:"namespace,omitempty"`
    APIVersion string `json:"apiVersion,omitempty"` // disambiguates API group, #1040
}
```

**Extraction Logic** (`pkg/aianalysis/handlers/response_processor.go`, `ExtractRootCauseAnalysis`):
```go
// #542: KA emits "remediationTarget" in JSON; CRD stores it as RemediationTarget.
if arRaw, ok := rcaMap["remediationTarget"]; ok {
    if arMap, ok := arRaw.(map[string]interface{}); ok {
        kind, _ := arMap["kind"].(string)
        name, _ := arMap["name"].(string)
        ns, _ := arMap["namespace"].(string)
        apiVersion, _ := arMap["api_version"].(string) // #1040
        if kind != "" && name != "" {
            rca.RemediationTarget = &aianalysisv1.RemediationTarget{
                Kind:       kind,
                Name:       name,
                Namespace:  ns,
                APIVersion: apiVersion,
            }
        }
    }
}
```

RemediationRequest's own `Status.RemediationTarget` is in turn populated from `AIAnalysis.Status.RootCauseAnalysis.RemediationTarget` (see `internal/controller/remediationorchestrator/reconcile_phases_test.go`, `UT-RR-387-002`/`UT-RR-387-003`), so downstream WorkflowExecution creation reads a single, RO-owned field regardless of which upstream layer produced it.

### 3. RemediationOrchestrator Scope Validation (BR-SCOPE-010)

RemediationOrchestrator prefers `AIAnalysis.Status.RootCauseAnalysis.RemediationTarget` over the RemediationRequest's signal-derived target when present (`pkg/remediationorchestrator/handler/aianalysis.go`), using it for scope validation and for populating `RemediationRequest.Status.RemediationTarget` before WorkflowExecution creation. The `apiVersion` (when present) drives GVK resolution for the scope check; when absent, RO falls back to a static Kind→Group mapping for core resource types.

### 4. Rego Policy Enhancement (ADR-055)

Rego policies receive `remediation_target` for workflow approval decisions:

```rego
# Example: Require approval if RCA targets production Deployment
package kubernaut.approval

require_approval if {
    # Check RCA-determined target (not signal source)
    input.remediation_target.kind == "Deployment"
    input.remediation_target.namespace == "production"
    input.severity_level == "critical"
}
```

**PolicyInput Struct** (`pkg/aianalysis/rego/evaluator.go`):
```go
type PolicyInput struct {
    // ... existing fields ...

    // ADR-055: Remediation target identified during RCA.
    // Replaces the older target_in_owner_chain boolean with structured
    // resource data, enabling granular per-kind approval policies.
    RemediationTarget *RemediationTargetInput `json:"remediation_target,omitempty"`
}

type RemediationTargetInput struct {
    Kind      string `json:"kind"`
    Name      string `json:"name"`
    Namespace string `json:"namespace"` // Empty for cluster-scoped
}
```

> `RemediationTargetInput` does not currently carry `apiVersion` — Rego policies key on `kind`/`name`/`namespace` only. If GVK-level disambiguation is ever needed in policy, this struct will need to grow an `ApiVersion` field.

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
- KA investigation: `kubectl get deployment payment-api` → **Ambiguous!**
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
- If missing: No warning needed — optional field
- Pass through to KA for RCA

**KA Handling** (BR-KA-212):
- Accept `apiVersion` from the LLM if provided (no validation required — KA's own owner-chain resolution is authoritative regardless)
- If missing: No error — RO will use static mapping
- No self-correction loop needed for `apiVersion`

### Why This Matters

1. **Correctness**: Remediation should target the **root cause**, not just the **symptom**.
   - Example: OOMKilled Pod → remediate Deployment, not Pod

2. **Scope Control**: BR-SCOPE-001 requires validation of the **remediation target**, not the signal source.
   - Prevents remediating resources outside of Kubernaut's managed scope

3. **Audit Trail**: Clear traceability of which resource was remediated and why.
   - `AIAnalysis.Status.RootCauseAnalysis.RemediationTarget` provides audit evidence

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
⏳ **Deferred**: Current workflows support only one target.
- Future enhancement for storm scenarios (100 pods → 1 Deployment)
- Future enhancement for cascading failures (ConfigMap → 5 Deployments)

---

## Defense-in-Depth: Three-Layer RemediationTarget Validation

Three independent layers ensure `remediationTarget` is always populated correctly, producing a consistent operator experience regardless of which layer catches an issue:

### Layer 1: KA Injection (Authoritative Source — BR-496 v2)
- **File**: `internal/kubernautagent/investigator/investigator_phases.go` (`InjectRemediationTarget`, `InjectTargetResourceParameters`)
- **Mechanism**: Resolves the root owner from enrichment data (owner chain, or post-re-enrichment source identity, or signal identity as a last resort), reconciles the LLM's proposed target against it, and injects the four `TARGET_RESOURCE_*` workflow parameters as `kaManagedParams` (never stripped).
- **Owner-chain hard-fail**: If owner-chain re-enrichment hard-fails after retry exhaustion → `needs_human_review=true`, `human_review_reason=rca_incomplete` (`internal/kubernautagent/investigator/investigator_discovery.go`).
- **Reference**: BR-496 v2, ADR-056

### Layer 2: AIAnalysis (Extraction Level)
- **File**: `pkg/aianalysis/handlers/response_processor.go` (`ExtractRootCauseAnalysis`)
- **Check**: Only stores `RemediationTarget` when `kind != ""` AND `name != ""`. Otherwise stays nil.
- **Reference**: DD-KA-006 Section 2

### Layer 3: RemediationOrchestrator (Routing Level)
- **File**: `pkg/remediationorchestrator/handler/aianalysis.go` (`HandleRemediationTargetMissing`)
- **Check**: If `RemediationTarget` is nil or has empty Kind/Name on a completed AIAnalysis with a selected workflow → manual review notification (`RemediationTargetMissing` / `rca_resource_missing`)
- **Reference**: BR-ORCH-036 v4.0

**Operator Experience**: All three layers produce the same response when the target resource cannot be determined:
- RR transitions to `Failed`
- `RequiresManualReview = true`
- `NotificationRequest` created with `type=manual-review`
- K8s Warning event emitted

**`human_review_reason` value**:

| Reason | Layer | When |
|--------|-------|------|
| `rca_incomplete` | 1 | Owner-chain re-enrichment hard-failed, or the RCA phase exhausted its turn budget |

---

## Future Enhancements

### Multiple Target Resources
Support multiple remediation targets in a single RCA:

```go
type RootCauseAnalysis struct {
    // ... existing fields ...

    // Future: Multiple targets
    // +optional
    RemediationTargets []RemediationTarget `json:"remediationTargets,omitempty"`
}
```

**Use Cases**:
- **Storm scenarios**: 100 pods crashing → scale 1 Deployment
- **Cascading failures**: ConfigMap missing → affects 5 Deployments
- **Cluster-wide issues**: Node failure → affects all pods on node

### Rego GVK Disambiguation
Plumb `apiVersion` through to `RemediationTargetInput` if policy authors need to gate on API group, not just Kind/Name/Namespace.

---

## Confidence Assessment

**Confidence Level**: 92%

**Strengths**:
- ✅ KA derives `remediationTarget` from K8s-verified data — eliminates reliance on LLM accuracy alone
- ✅ Clear use cases and examples (OOMKilled Pod → Deployment)
- ✅ Aligns with existing BR-SCOPE-001 and BR-SCOPE-010
- ✅ `TARGET_RESOURCE_*` params are unconditionally preserved (`kaManagedParams`), so workflow jobs always receive correct resource identity
- ✅ Escalation to human review when owner-chain resolution hard-fails (safe default)
- ✅ Three-layer defense-in-depth (KA → AIAnalysis → RO) verified against current source in all three services

**Risks**:
- ⚠️ **OpenAPI spec gap**: `internal/kubernautagent/api/openapi.json` does not yet document `remediationTarget` — tracked as a follow-up
- ⚠️ **Rego GVK gap**: `RemediationTargetInput` lacks `apiVersion`, so policies cannot currently disambiguate by API group
- ⚠️ Owner-chain resolution depends on enrichment succeeding at least once; repeated hard-fails correctly escalate to human review rather than guessing

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.1–1.6 | 2026-01-20 to 2026-03-25 | Historical evolution under the Python-era implementation: introduced `apiVersion`, three-layer defense-in-depth, LLM-provided-then-KA-resolved hybrid model, and the three-phase RCA flow. Superseded by v2.0 below; see git history for the original entries if archaeology is needed. |
| 2.0 | 2026-08-01 | Rewritten against the Go KA implementation as part of [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806). Field renamed `affectedResource` → `remediationTarget` throughout (KA wire contract, AIAnalysis CRD, Rego `PolicyInput`). Corrected the canonical-parameter-injection model: `TARGET_RESOURCE_*` are unconditionally injected and unconditionally preserved (`kaManagedParams`), not conditional on workflow schema declaration. Corrected root-resolution priority to match `InjectRemediationTarget` (owner chain → enrichment source identity → signal identity). Corrected Layer 3 file/function to `pkg/remediationorchestrator/handler/aianalysis.go` (`HandleRemediationTargetMissing`). Documented the RemediationRequest-level `Status.RemediationTarget` propagation step. Flagged the OpenAPI spec and Rego `apiVersion` gaps rather than asserting false completeness. |

---

**Document Control**:
- **Created**: 2026-01-20
- **Last Updated**: 2026-08-01 (v2.0 — Go rewrite)
- **Version**: 2.0
- **Status**: ✅ Approved
- **Next Review**: When the OpenAPI spec gap or Rego `apiVersion` gap is closed
