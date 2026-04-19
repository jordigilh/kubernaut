# ADR-055: LLM-Driven Context Enrichment (Post-RCA)

**Status**: ACCEPTED
**Decision Date**: 2026-02-12
**Version**: 1.5
**Confidence**: 92%
**Applies To**: HolmesGPT API (HAPI), AIAnalysis Controller, SignalProcessing

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-12 | Architecture Team | Initial proposal: move context enrichment (owner chain, spec hash, remediation history) from pre-LLM computation to post-RCA tool-driven flow. |
| 1.1 | 2026-02-12 | Architecture Team | Address 8 triage gaps: replace `target_in_owner_chain` with `affected_resource` Rego input (§2), preserve `ExtractRootCauseAnalysis` (§3), enforce `affectedResource` as required LLM response field (§4), clarify CRD deprecation (§5), clarify `current_spec_hash` scope (§6), document new `resolve_owner_chain` function + RBAC expansion (§7), update latency estimate (§8). [Deprecated - Issue #180: Recovery flow reference removed] *(§4 "required LLM response field" for target identity superseded by BR-496 v2 / DD-HAPI-006 v1.4: HAPI owns `affectedResource` via `root_owner` injection.)* |
| 1.3 | 2026-02-24 | Architecture Team | **Issue #188 (DD-EM-003)**: Renamed `resolveEffectivenessTarget` to `resolveDualTargets` throughout. The function now returns `*creator.DualTarget{Signal, Remediation}` with explicit dual-target semantics. Updated compatibility table and data quality section. |
| 1.2 | 2026-02-12 | Architecture Team | Refine tool return contract: `get_resource_context` returns only `root_owner` and `remediation_history` to the LLM. Owner chain traversal and spec hash computation are internal implementation details not exposed in the tool response. Update prompt Phase 3b accordingly. See also ADR-056 for DetectedLabels relocation. |
| 1.4 | 2026-03-24 | Architecture Team | **Issue #524**: Namespaced tool renamed to `get_namespaced_resource_context`; added `get_cluster_resource_context` for cluster-scoped targets (Node, PV, etc.). Both tools registered in the `resource_context` toolset. `resource_scope` (`namespaced` / `cluster`) stored in `session_state`. Canonical `TARGET_RESOURCE_*` injection is conditional on workflow schema declarations; former validator Step 0 (mandatory canonical declarations) removed. |
| 1.5 | 2026-03-25 | Architecture Team | **Issue #529**: Three-phase RCA architecture. Context enrichment moves from LLM-driven tool call to HAPI-driven `EnrichmentService` (Phase 2). LLM provides `affectedResource` in Phase 1; HAPI resolves owner chain, detects labels, and fetches history for the resolved root owner. Enrichment context sent to LLM in Phase 3 for informed workflow selection. Resource context tools no longer write `root_owner` or `detected_labels` to `session_state`; `EnrichmentService` is the sole authoritative source. BR-HAPI-262 (history verification enforcement) dropped — HAPI always provides verified history. BR-HAPI-260 (dedicated history tool) dropped — existing resource context tools already return history; EnrichmentService provides authoritative history in Phase 2. Resource context tools remain available as optional Phase 1 informational tools (label detection still returns `detected_infrastructure` to LLM but no longer writes to `session_state`). See BR-HAPI-261, BR-HAPI-264. |

---

## Context & Problem

### Current Architecture (Pre-Computation Model)

The current pipeline pre-collects context **before** the LLM performs Root Cause Analysis (RCA):

```
Signal -> SP enriches with OwnerChain
  -> RO copies OwnerChain to AIAnalysis.Spec.EnrichmentResults
  -> AIAnalysis Controller passes OwnerChain to HAPI request (Issue #97)
  -> HAPI pre-computes BEFORE LLM invocation:
      1. resolve_root_owner(owner_chain) -> root owner
      2. compute_spec_hash(root_owner) -> SHA-256 hash
      3. fetch_remediation_history(root_owner, spec_hash) -> history context
  -> LLM receives all pre-computed context + signal -> performs RCA
  -> LLM selects workflow based on pre-computed context
```

This applies to the **incident flow** (`extensions/incident/llm_integration.py`). [Deprecated - Issue #180: Recovery flow reference removed]

### Problems

1. **Wrong resource context**: The owner chain and spec hash are computed from the **signal source** (e.g., the crashing Pod), not the **actual root cause target** (e.g., a misconfigured ConfigMap, an HPA with wrong thresholds, or a Deployment with missing resource limits). The LLM may identify a completely different resource as the root cause.

2. **Context pollution**: The LLM receives remediation history for the signal source resource, which may be irrelevant to the actual root cause. This consumes context window and can bias the LLM's reasoning.

3. **Wasted computation**: If the owner chain resolution or spec hash computation fails (e.g., the `ImportError: No module named 'utils.canonical_hash'` observed in CI), the data is empty anyway. The LLM proceeds without it, proving the pre-computation is not essential for RCA.

4. **Owner chain only for Pods**: SignalProcessing only computes owner chains for Pod signals. Deployment, StatefulSet, DaemonSet, Node, and Service signals have empty owner chains, making the entire propagation path a no-op for most signal types.

5. **Unnecessary data propagation**: The owner chain traverses three CRD boundaries (SP -> RO -> AIAnalysis -> HAPI) purely to enable a pre-computation that targets the wrong resource.

### Business Requirements Affected

- **BR-HAPI-016**: Remediation history context (enhanced, not broken)
- **BR-AI-023**: Investigation audit trail (unchanged)
- **Issue #97**: Owner chain / AffectedResource / SpecHash (superseded by this ADR)
- **DD-HAPI-017**: Three-step workflow discovery (enhanced, tools execute in correct order)

---

## Decision

### Move to LLM-Driven, Post-RCA Context Enrichment

Replace the pre-computation model with a three-phase, tool-driven flow where the LLM controls when and for which resource context is collected:

```
Signal -> AIAnalysis Controller passes signal context to HAPI
  -> HAPI invokes LLM with signal context only (no pre-computed enrichment)
  -> Phase 1 (RCA): LLM analyzes signal, identifies root cause and affected resource
  -> Phase 2 (Context): LLM calls get_namespaced_resource_context or get_cluster_resource_context (from the resource_context toolset) for the RCA target
      -> Tool internally:
         1. Traverses K8s ownerReferences for identified target
         2. Identifies root managing resource (e.g., Pod -> Deployment)
         3. Computes spec hash for root owner
         4. Queries DataStorage for remediation history (root owner + spec hash)
      -> Returns to LLM: root_owner identity + remediation_history only
         (owner chain and spec hash are internal, not exposed)
  -> Phase 3 (Workflow): LLM calls 3-step workflow discovery (DD-HAPI-017)
      -> list_available_actions(action_type)
      -> list_workflows(action_type, filters)
      -> get_workflow(workflow_id) with parameter mapping
  -> LLM returns complete result: RCA + affected resource + workflow recommendation
```

This flow applies to the incident path.

### Key Design Principles

1. **The LLM identifies the target resource, not pre-computation**. The `affectedResource` in the result is a required, first-class RCA output, not derived from a pre-computed owner chain.

2. **`affectedResource` is a required field in the LLM response contract**. The response validator (3-attempt self-correction loop) rejects responses that omit it, the same way it enforces `severity`, `summary`, and `selected_workflow`. If the LLM omits `affectedResource`, the validator returns an error and the LLM retries with the signal context (which includes the target resource) as fallback.

3. **Context is fetched for the right resource at the right time**. The spec hash and remediation history describe the resource that will actually be remediated.

4. **Tool-driven, not pipeline-driven**. The LLM requests what it needs through tool calls. If the RCA determines no context enrichment is needed (e.g., the root cause is a misconfiguration that doesn't need history), it can skip the tool call entirely.

5. **`affected_resource` replaces `target_in_owner_chain` in Rego policy input**. Instead of a boolean about chain membership, the Rego evaluator exposes the LLM-identified target resource (kind, name, namespace) as structured input. This enables granular, per-kind approval policies (e.g., "require approval for Node remediations in production") rather than the opaque `not target_validated` gate.

---

## Changes Required

### Phase 1: Revert Issue #97 Pre-Computation Path

#### HAPI (Python) -- Incident Flow

| File | Change | Rationale |
|------|--------|-----------|
| `kubernaut-agent/src/extensions/incident/llm_integration.py` | Remove pre-LLM root owner resolution, spec hash computation, remediation history fetch via root owner, and Phase C `affectedResource` population (~lines 227-278, 593-608) | Pre-computation targets wrong resource |
| `kubernaut-agent/src/extensions/incident/result_parser.py` | Remove `target_in_owner_chain` from `parse_and_validate_investigation_result`. Remove `is_target_in_owner_chain()` function. | Replaced by `affected_resource` Rego input |
| `kubernaut-agent/src/clients/k8s_client.py` | Remove `resolve_root_owner()` function. Keep `compute_spec_hash()` (reused by new tool). | `resolve_root_owner` was a trivial list[-1]; new tool traverses K8s API instead |
| `kubernaut-agent/tests/unit/test_k8s_client_owner_resolution.py` | Remove tests for `resolve_root_owner()` | Function removed |

#### AIAnalysis Controller (Go)

| File | Change | Rationale |
|------|--------|-----------|
| `pkg/aianalysis/handlers/request_builder.go` (lines 296-306) | Remove OwnerChain -> HAPI request mapping | OwnerChain no longer passed to HAPI |
| `pkg/aianalysis/handlers/response_processor.go` | **No changes**. `ExtractRootCauseAnalysis` stays as-is -- it correctly extracts `affectedResource` from the LLM's RCA JSON response. The centralization (dedup of 5 handler methods) is valuable. | Only the Python-side Phase C fallback is removed; the Go-side extraction of whatever the LLM returns is correct |
| `api/aianalysis/v1alpha1/aianalysis_types.go` | Remove `TargetInOwnerChain *bool` from AIAnalysis status. Add deprecation comment to `EnrichmentResults.OwnerChain` field: `// Deprecated: ADR-055 - no longer populated for HAPI. Scheduled for removal in v2 API.` | `target_in_owner_chain` concept removed; CRD OwnerChain field removal deferred to v2 |

#### Rego Policy and Evaluator

| File | Change | Rationale |
|------|--------|-----------|
| `pkg/aianalysis/rego/evaluator.go` | Remove `TargetInOwnerChain bool` from `RegoInput`. Add `AffectedResource` struct (Kind, Name, Namespace) to `RegoInput`. Populate from `analysis.Status.RootCauseAnalysis.AffectedResource`. | Replaces boolean with structured data |
| `pkg/aianalysis/handlers/analyzing.go` | Remove `TargetInOwnerChain` mapping (~lines 353-356). Add `AffectedResource` mapping from `analysis.Status.RootCauseAnalysis.AffectedResource`. | Feeds new Rego input |
| `config/rego/aianalysis/approval.rego` | Replace `target_validated` / `not target_validated` rules with `affected_resource`-based rules. See example below. | Enables granular per-kind policies |

**New Rego policy pattern:**

```rego
# Old (removed):
# target_validated if { input.target_in_owner_chain == true }
# require_approval if { is_production; not target_validated }

# New: Granular affected_resource policies
# Operators can write kind-specific rules

# Node remediations in production always require approval
require_approval if {
    is_production
    input.affected_resource.kind == "Node"
}

# StatefulSet remediations in production require approval
require_approval if {
    is_production
    input.affected_resource.kind == "StatefulSet"
}

# Risk factor: production + sensitive resource kinds
risk_factors contains {"score": 80, "reason": "Production environment with sensitive resource kind - requires manual approval"} if {
    is_production
    input.affected_resource.kind == "Node"
}

risk_factors contains {"score": 60, "reason": "Production environment with stateful resource - requires manual approval"} if {
    is_production
    input.affected_resource.kind == "StatefulSet"
}
```

#### RemediationOrchestrator (Go)

| File | Change | Rationale |
|------|--------|-----------|
| `pkg/remediationorchestrator/creator/aianalysis.go` | `buildEnrichmentResults()` can stop copying `OwnerChain` to AIAnalysis spec | No downstream consumer needs it for HAPI |

#### SignalProcessing (Go)

| File | Change | Rationale |
|------|--------|-----------|
| No changes | SP owner chain enrichment stays -- it serves SP's own purposes (label detection, HPA detection, Rego evaluation) | Owner chain computation in SP is not affected |

### Phase 2: Add HAPI Resource Context Tools (`resource_context` Toolset)

#### Tools: `get_namespaced_resource_context` and `get_cluster_resource_context`

```python
class GetNamespacedResourceContextTool:
    """Fetch remediation context for a namespace-scoped RCA target.

    Internally traverses K8s ownerReferences, computes spec hash,
    and queries remediation history. Returns only the root owner
    identity and history -- chain traversal and hash are internal.
    Stores resource_scope='namespaced' in session_state.

    Returns to LLM:
    - root_owner: The root managing resource (kind, name, namespace)
    - remediation_history: Past remediation attempts for that resource
    """

    def execute(self, kind: str, name: str, namespace: str) -> ResourceContext:
        # 1. (Internal) Traverse K8s ownerReferences to find root owner
        owner_chain = k8s_client.resolve_owner_chain(kind, name, namespace)

        # 2. (Internal) Determine root owner (last in chain, or resource itself)
        root_owner = owner_chain[-1] if owner_chain else {
            "kind": kind, "name": name, "namespace": namespace
        }

        # 3. (Internal) Compute spec hash of root owner
        spec_hash = k8s_client.compute_spec_hash(
            root_owner["kind"], root_owner["name"], namespace
        )

        # 4. (Internal) Fetch remediation history for root owner + spec hash
        history = remediation_history_client.fetch(root_owner, spec_hash)

        # Return only what the LLM needs
        return ResourceContext(
            root_owner=root_owner,
            remediation_history=history,
        )
```

`get_cluster_resource_context` is the sibling tool for **cluster-scoped** RCA targets (e.g. Node, PersistentVolume): it resolves context without a namespace, sets `resource_scope='cluster'` in `session_state`, and follows the same internal pattern (spec hash + remediation history for the resolved target).

#### New Function: `resolve_owner_chain`

This is a **new function** in `k8s_client.py`, not a refactor of `resolve_root_owner()`. It traverses K8s `ownerReferences` from scratch:

```python
async def resolve_owner_chain(
    self, kind: str, name: str, namespace: str, max_depth: int = 5
) -> List[Dict[str, str]]:
    """Traverse K8s ownerReferences to build the ownership chain.

    Starting from the given resource, follows metadata.ownerReferences
    (controller: true) upward until no more owners are found or max_depth
    is reached.

    Returns list of owner entries (excluding the source resource).
    Example for a Pod owned by a Deployment:
        [{"kind": "ReplicaSet", "name": "api-7d8f9c6b5", "namespace": "prod"},
         {"kind": "Deployment", "name": "api", "namespace": "prod"}]
    """
    chain = []
    current_kind, current_name = kind, name
    for _ in range(max_depth):
        resource = await self._get_resource(current_kind, current_name, namespace)
        if resource is None:
            break
        owner_refs = resource.get("metadata", {}).get("ownerReferences", [])
        controller_owner = next(
            (ref for ref in owner_refs if ref.get("controller") is True), None
        )
        if controller_owner is None:
            break
        entry = {
            "kind": controller_owner["kind"],
            "name": controller_owner["name"],
            "namespace": namespace,
        }
        chain.append(entry)
        current_kind = controller_owner["kind"]
        current_name = controller_owner["name"]
    return chain
```

#### RBAC Expansion

The resource context tools need to read `ownerReferences` on resources during chain traversal AND read `.spec` for hash computation (plus cluster-scoped `get` where applicable for `get_cluster_resource_context`). The RBAC manifest must be expanded:

```yaml
# deploy/kubernaut-agent/03-rbac.yaml
rules:
  # Existing: read events
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "list"]

  # ADR-055: Read ownerReferences for chain traversal + read spec for hash
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get"]
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
    verbs: ["get"]
```

Note: `replicasets` added (needed for Pod -> ReplicaSet traversal). `pods` added (needed for starting traversal from a Pod resource to read its `ownerReferences`).

#### Tool Registration

Both tools are registered on the **`resource_context` toolset** (`ResourceContextToolset` in `kubernaut-agent/src/toolsets/resource_context.py`), which is attached in the incident flow alongside the existing DD-HAPI-017 workflow discovery tools. The LLM sees `get_namespaced_resource_context` and `get_cluster_resource_context` as distinct tool names and must choose the one that matches the RCA target’s scope (Issue #524).

#### Updated Prompt Flow

> **Note (BR-496 v2, DD-HAPI-006 v1.4–v1.5):** Stored remediation target identity is derived by HAPI from `root_owner` (`_inject_target_resource`), not taken as an unconstrained required LLM field. **Issue #524**: Use `get_namespaced_resource_context` vs `get_cluster_resource_context` per target scope; canonical `TARGET_RESOURCE_*` workflow params are injected only when declared in the workflow schema. The numbered steps below reflect the original ADR-055 prompt contract where they still apply.

The HAPI system prompt instructs the LLM:

1. **First**: Analyze the signal context and perform root cause analysis. Identify the root cause and the affected resource. The `affectedResource` field is **required** in your response.
2. **Then**: Call `get_namespaced_resource_context` (namespace-scoped target) or `get_cluster_resource_context` (cluster-scoped target) with the appropriate arguments. The tool returns: (a) `root_owner` -- the root managing resource (e.g., a Pod's Deployment); use this as your `affectedResource`; (b) `remediation_history` -- past remediations for that resource. Owner chain traversal and spec hash computation are internal to the tool.
3. **Finally**: Use the three-step workflow discovery (DD-HAPI-017) to select the appropriate remediation workflow, informed by the remediation history.

#### Response Validation

> **Note (BR-496 v2, DD-HAPI-006 v1.4–v1.5):** Target resource identity for downstream consumers is injected from `root_owner`. **Issue #524** removed mandatory **Step 0** validation that required every workflow schema to declare `TARGET_RESOURCE_NAME` / `TARGET_RESOURCE_KIND` / `TARGET_RESOURCE_NAMESPACE`; HAPI injects only parameters that exist in the schema. The following still describes the original validator expectation for LLM RCA output shape where `affectedResource` was LLM-supplied.

The `WorkflowResponseValidator` (3-attempt self-correction loop) is updated to validate that `affectedResource` is present in the RCA output. If the LLM omits it, the validator returns:

```
"missing required field: root_cause_analysis.affectedResource (kind, name required; namespace required for namespace-scoped resources, omit for cluster-scoped)"
```

The LLM retries with the signal context (which includes the target resource kind/name) available to produce the field. This is the same validation pattern used for `severity`, `summary`, and `selected_workflow`.

### Phase 3: Clean Up Result Structure

#### Remove from HAPI Request

| Field | Action |
|-------|--------|
| `enrichment_results.ownerChain` | Remove from request schema -- HAPI resolves it via tool |
| `enrichment_results.currentSpecHash` | Remove -- computed by tool for correct resource |

#### Keep in HAPI Response

| Field | Source | Notes |
|-------|--------|-------|
| `affected_resource` | LLM RCA output (Phase 1) | Required field, enforced by response validator |
| `root_cause_analysis` | LLM RCA output (Phase 1) | Unchanged |
| `recommended_workflow` | LLM workflow selection (Phase 3) | Based on correct context |

#### `current_spec_hash` Scope

The `current_spec_hash` computed by the resource context tools (`get_namespaced_resource_context` / `get_cluster_resource_context`) is used **within the HAPI session only** -- for the remediation history lookup (DataStorage query). It is NOT surfaced in the HAPI response.

The Go-side `CapturePreRemediationHash` in the RO reconciler independently computes the spec hash from the K8s API at workflow execution time. This is correct: the RO captures the hash at the moment of remediation, not from a potentially stale HAPI response. No changes needed to the RO hash computation.

#### Remove `target_in_owner_chain`

Removed from:
- HAPI response (`result_parser.py` -- incident parse function)
- `is_target_in_owner_chain()` function in `result_parser.py`
- AIAnalysis CRD status (`TargetInOwnerChain *bool`)
- Rego evaluator input (`RegoInput` struct)
- Rego approval policy (`target_validated` rules)
- Analyzing handler (`TargetInOwnerChain` mapping)

Replaced by: `affected_resource` (kind, name, namespace) as Rego input for granular policy rules.

---

## Impact on Issue #97

Issue #97 introduced these capabilities:

| Capability | Issue #97 Implementation | ADR-055 Impact |
|------------|--------------------------|----------------|
| **Owner chain propagation** (SP -> AIAnalysis -> HAPI) | `request_builder.go` maps OwnerChain to HAPI request | **SUPERSEDED**: Remove mapping. HAPI resolves owner chain via tool call for the correct resource. |
| **Spec hash computation** | `k8s_client.py` computes hash pre-LLM from signal source's root owner | **SUPERSEDED**: `compute_spec_hash()` preserved, invoked by tool for the RCA-identified target. |
| **`affectedResource` population** | Derived from pre-computed root owner in `llm_integration.py` Phase C | **SUPERSEDED**: LLM identifies affected resource directly during RCA. Enforced as required field via response validator. |
| **`ExtractRootCauseAnalysis` centralization** | `response_processor.go` helper deduplicating 5 handler methods | **RETAINED**: Centralization is valuable. The Go-side extraction of `affectedResource` from the RCA JSON is correct regardless of how the LLM populates it. |
| **`resolveDualTargets`** | `reconciler.go` uses `AffectedResource` for EA targeting (DD-EM-003) | **RETAINED + RENAMED**: Renamed from `resolveEffectivenessTarget` in Issue #188. Now returns `*creator.DualTarget{Signal, Remediation}` with explicit dual-target semantics. The `AffectedResource` field is populated by the LLM directly rather than by HAPI's Phase C fallback. |
| **RBAC for apps/v1** | `03-rbac.yaml` grants read access for spec hash | **RETAINED + EXPANDED**: Still needed for the resource context tools. Expanded to include `replicasets` and `pods` for owner chain traversal (and cluster-scoped reads as required for `get_cluster_resource_context`). |
| **`target_in_owner_chain` validation** | `result_parser.py` checks RCA target against pre-computed chain | **REPLACED**: Removed. `affected_resource` exposed as structured Rego input for granular per-kind approval policies. |

---

## Advantages

1. **Accuracy**: Context is collected for the resource the LLM actually identified as the root cause, not the signal source.
2. **Efficiency**: No wasted computation when owner chain is empty (non-Pod signals) or when the LLM identifies a different target.
3. **Simpler data flow**: Eliminates the SP -> RO -> AIAnalysis -> HAPI propagation of owner chain data across three service boundaries.
4. **Cleaner LLM context**: RCA reasoning is not biased by pre-loaded remediation history for potentially the wrong resource.
5. **Agentic pattern**: Aligns with modern LLM tool-use patterns where the agent drives information gathering based on its analysis.
6. **Graceful degradation**: If `get_namespaced_resource_context` / `get_cluster_resource_context` fails (K8s API unavailable, RBAC issues), the LLM can still complete RCA and workflow selection without historical context, and it can reason about the failure explicitly.
7. **Better Rego policies**: `affected_resource` (kind, name, namespace) as Rego input enables granular, per-kind approval rules -- strictly more powerful than the previous boolean `target_in_owner_chain`.
8. **Enforced data quality** *(superseded for identity — BR-496 v2 / DD-HAPI-006 v1.4: HAPI injects `affectedResource` from `root_owner`)*: `affectedResource` as a required response field with validation was intended to ensure downstream consumers (`resolveDualTargets` (DD-EM-003), WFE creator, audit trail) always have the target resource.

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Additional latency from tool call round-trip | Medium | Low | Session-based async flow handles multi-turn interactions. Tool performs 3 sequential K8s/DS calls (~1-5s total). Spec hash + history fetch can be parallelized once root owner is known. |
| LLM may not call `get_namespaced_resource_context` / `get_cluster_resource_context` | Low | Medium | System prompt explicitly instructs the 3-phase flow and correct tool for target scope. Validation can check if tool was called. **BR-496 v2 (DD-HAPI-006 v1.4)**: If `selected_workflow` present but `root_owner` missing from `session_state`, HAPI flags `needs_human_review=true` with `human_review_reason=rca_incomplete`. **Issue #524**: Post-selection guard can flag node-scoped `action_type` vs namespaced resource-context mismatch for self-correction. |
| LLM omits `affectedResource` from RCA | Low | Low | `affectedResource` enforced as required field by response validator (3-attempt self-correction loop). Same pattern as `severity`, `summary`. |
| LLM identifies wrong target, fetches wrong context | Low | Low | Same risk exists today (pre-computed context may also be for wrong resource). The new flow is strictly better because the LLM can correct itself. **BR-496 v2 (DD-HAPI-006 v1.4)**: Stored target identity follows K8s-verified `root_owner` via HAPI injection, not a mismatch-driven human review path. |
| Rego policy breakage during migration | Medium | High | Rego input schema update (`target_in_owner_chain` → `affected_resource`) must be atomic. Test with existing E2E approval tests. See BR-AI-085-005 for default-deny safety pattern. |

---

## References

- **DD-HAPI-017**: Three-step workflow discovery integration
- **DD-HAPI-016**: Remediation history context via spec-hash matching
- **Issue #97**: Owner chain / AffectedResource / SpecHash (superseded)
- **DD-WORKFLOW-001 v1.7-1.8**: Owner chain schema and validation
- **DD-EM-002**: Canonical spec hash computation
- **BR-AI-085**: Rego Policy Input Schema for Approval Decisions (includes default-deny safety pattern)
