# ADR-056: Post-RCA Label Computation Relocation

**Status**: PROPOSED
**Decision Date**: 2026-02-12
**Version**: 1.1
**Confidence**: 90%
**Applies To**: SignalProcessing, HolmesGPT API (HAPI), AIAnalysis Controller, Data Storage

---

## Context & Problem

### Background: ADR-055 Exposed a Deeper Gap

ADR-055 moved context enrichment (owner chain resolution, spec hash, remediation history) from pre-LLM computation to an LLM-driven post-RCA tool call (`get_resource_context`). This correctly addresses the "wrong resource context" problem for those three data points.

However, ADR-055 did not address **DetectedLabels**, which suffer from the same fundamental flaw: they are computed at **signal time** for the **signal source resource** by SignalProcessing, then propagated downstream to HAPI for workflow discovery and LLM prompt context.

### The Stale Labels Problem

When the LLM's RCA identifies a resource different from the signal source, the DetectedLabels computed by SP are **stale and potentially misleading**:

```
Signal: Pod "api-xyz" in namespace "prod" crashes
  |
  v
SP enriches "api-xyz":
  - Owner chain: Pod -> ReplicaSet -> Deployment
  - DetectedLabels: {stateful: false, hpaManaged: true, helmManaged: true, ...}
  |
  v
LLM performs RCA -> root cause is Node "worker-3" memory pressure
  |
  v
Workflow discovery receives SP's DetectedLabels:
  - hpaManaged: true (describes Deployment, NOT the Node)
  - helmManaged: true (describes Deployment, NOT the Node)
  - Result: Returns Deployment-oriented workflows instead of Node remediation workflows
```

This is not an edge case. RCA routinely pivots to a different resource:

| Signal Source | RCA Target | SP Labels Valid? |
|--------------|------------|-----------------|
| Pod crash | Deployment misconfiguration (in owner chain) | Yes |
| Pod OOMKill | Container resource limits on Deployment | Yes |
| Pod CrashLoopBackOff | Bad ConfigMap mount | No |
| Pod crash | Node memory pressure | No |
| Service latency | Upstream dependency failure | No |
| Pod eviction | PVC storage class issue | No |

Conservative estimate: ~30-40% of RCA results identify a resource outside the signal source's owner chain.

### Current Data Flow

```
SignalProcessing                  AIAnalysis              HAPI
+-----------------------+         +----------+           +----------------------+
| 1. Compute OwnerChain |         |          |           |                      |
| 2. Detect labels      |-------->| Copy to  |---------->| prompt_builder:      |
|    (signal source)    |  enrich | AIA spec |   request |   cluster context    |
| 3. Store in SP status |  results|          |           |   filter instructions|
+-----------------------+         +----------+           |                      |
                                                         | workflow_discovery:  |
                                                         |   detected_labels    |
                                                         |   query param        |
                                                         +----------------------+
```

SP computes labels for the **signal source** at signal time. These flow through three CRD boundaries (SP -> RO -> AIAnalysis -> HAPI) and are used for:

1. **LLM Prompt Context** (`prompt_builder.py`): `build_cluster_context_section()` converts labels to natural language (e.g., "This namespace is managed by GitOps (argocd)")
2. **Workflow Discovery Filtering** (`workflow_discovery.py`): Labels passed as `detected_labels` query parameter to Data Storage
3. **Workflow Search Scoring** (`search.go`): Labels used for boost/penalty scoring in POST search

### Evidence of the Gap in Existing Code

The gap was anticipated but never resolved:

- **`should_include_detected_labels()`** in `holmesgpt-api/src/toolsets/workflow_discovery.py` (line 152): A guard function that checks whether the RCA target matches the signal source before including DetectedLabels. It is **defined but never called** -- the exact guard for this gap was written but never wired into the workflow discovery flow.

- **Data Storage discovery SQL** in `pkg/datastorage/repository/workflow/discovery.go`: `detected_labels` are parsed from the request but **not used in the discovery SQL queries** -- they only participate in boost/penalty scoring in the separate POST search flow. This means the three-step discovery path (list actions -> list workflows -> get workflow) already operates without label filtering.

### Business Requirements Affected

- **BR-SP-101**: DetectedLabels auto-detection (8 characteristics) -- scope changes from pipeline-wide to SP-internal
- **BR-SP-103**: FailedDetections tracking -- stays within SP
- **BR-HAPI-194**: Honor `failedDetections` in workflow filtering -- moves to HAPI-computed labels
- **BR-HAPI-250/252**: DetectedLabels integration with Data Storage -- labels now computed by HAPI
- **DD-WORKFLOW-001 v1.7/v2.1/v2.2**: DetectedLabels schema and validation -- architectural relocation

---

## Decision

### Relocate Label Computation from SignalProcessing to HAPI (Post-RCA, Internal)

Move label detection from signal time (SP) to post-RCA time (HAPI), aligned with the actual remediation target resource identified by the LLM. Labels are computed and used **internally by HAPI** -- the LLM never sees or manages them.

### Foundational Principle: Impact vs. Target

The architecture separates two fundamentally different classification scopes:

- **Impact-scoped (SP, pre-RCA)**: Business classification -- environment, priority, severity, business unit. These describe the **business impact of the signal**: which service is affected, in which environment, how urgent is it. These are valid at signal time and remain correct regardless of what the RCA identifies as the root cause. A Pod crash in production is P0 whether the fix targets a Pod, a Deployment, or a Node.

- **Target-scoped (HAPI, post-RCA)**: DetectedLabels -- stateful, hpaManaged, helmManaged, pdbProtected, etc. These describe **operational properties of the remediation target**: what constraints apply to the resource we are about to fix. These can only be answered correctly for the actual target identified by RCA.

SP owns impact classification. HAPI owns target properties. Neither crosses into the other's domain.

### Proposed Architecture

```
SignalProcessing                  AIAnalysis              HAPI
+-----------------------+         +----------+           +---------------------------+
| Business classif.:    |         |          |           | 1. LLM performs RCA       |
|  - environment        |         | Signal   |           | 2. LLM calls              |
|  - priority           |         | context  |---------->|    get_resource_context()  |
|  - severity           |         | only     |  request  |    for RCA target          |
|  - business unit      |         | (no      |           | 3. Tool internally:        |
|                       |         |  labels  |           |    - Resolves owner chain  |
| DetectedLabels:       |         |  or      |           |    - Computes spec hash    |
|  (SP-internal only)   |         |  owner   |           |    - Fetches history       |
|  for audit/classify   |         |  chain)  |           |    - Computes labels (NEW) |
+-----------------------+         +----------+           |    - Stores labels in      |
                                                         |      session state         |
                                                         |    Returns to LLM:         |
                                                         |      root_owner + history  |
                                                         |      (labels NOT returned) |
                                                         |                            |
                                                         | 4. LLM calls list_workflows|
                                                         |    HAPI transparently      |
                                                         |    injects stored labels   |
                                                         |    into DataStorage query  |
                                                         +---------------------------+
```

### Key Design Principles

1. **Impact vs. target separation.** Business classification (SP) describes signal impact and is stable across RCA outcomes. DetectedLabels (HAPI) describe the remediation target and must be computed post-RCA for the correct resource.

2. **DetectedLabels are HAPI-internal.** Labels are computed by `get_resource_context` and stored in session state. They are **not returned to the LLM** and **not passed as tool parameters**. HAPI transparently injects them into workflow discovery queries. The LLM's tool interface is simpler -- fewer parameters, no risk of label hallucination.

3. **Workflow discovery `list_workflows` drops the `detected_labels` parameter.** The LLM calls `list_workflows(action_type)` and HAPI internally applies the stored labels as filter criteria. This is completely transparent from the LLM's perspective.

4. **SP keeps its own labels for internal purposes.** SP still computes labels for signal classification, audit events (`HasOwnerChain`, `OwnerChainLength`), and internal detection. These do not leave SP.

5. **`get_resource_context` becomes the single source of truth for target context.** ADR-055 already made it the source for owner chain, spec hash, and remediation history. Adding label detection completes the picture.

6. **CustomLabels (Rego-extracted) stay as-is.** Customer-defined Rego policies extract labels from the signal source resource. These are a customer extension point and should be designed for impact/signal-scoped properties (team, cost-center, service-tier) rather than resource-specific mechanics. Guidance will be documented. Moving customer Rego execution to HAPI post-RCA is a potential future iteration but out of scope for this decision.

7. **No backwards compatibility required.** The system has not been released. All changes are forward-only.

---

## Changes Required

### Phase 1: Extend `get_resource_context` with Internal Label Detection

| File | Change | Rationale |
|------|--------|-----------|
| `holmesgpt-api/src/toolsets/resource_context.py` | Add label detection logic to `GetResourceContextTool._invoke_async()`. After resolving owner chain, detect: gitOpsManaged, pdbProtected, hpaEnabled, stateful, helmManaged, networkIsolated, serviceMesh. **Store labels in session state, do NOT return them in tool result.** Tool still returns only `root_owner` + `remediation_history` to LLM. | Labels computed for the actual RCA target but kept internal to HAPI |
| `holmesgpt-api/src/clients/k8s_client.py` | Add functions for label detection K8s API calls: PDB lookup, HPA lookup, NetworkPolicy lookup, annotation/label inspection | Mirrors SP's detection logic in Python for the target resource |
| `deploy/holmesgpt-api/03-rbac.yaml` | Add RBAC for: `poddisruptionbudgets`, `horizontalpodautoscalers`, `networkpolicies` (GET/LIST) | Required for label detection API calls |
| `holmesgpt-api/tests/unit/test_resource_context_tool.py` | Add tests for label detection in resource context tool | TDD: test each label detection path |

### Phase 2: Remove Label and OwnerChain Propagation from Pipeline

| File | Change | Rationale |
|------|--------|-----------|
| `pkg/shared/types/enrichment.go` | Remove `OwnerChain []OwnerChainEntry` field from `EnrichmentResults`. Keep `OwnerChainEntry` type (used by SP internally). | OwnerChain no longer propagated; SP uses its own CRD type |
| `pkg/shared/types/enrichment.go` | Remove `DetectedLabels *DetectedLabels` field from `EnrichmentResults`. Keep `DetectedLabels` type (used by SP internally). | Labels no longer propagated; SP uses them internally |
| `pkg/remediationorchestrator/creator/aianalysis.go` | Remove `buildEnrichmentResults` OwnerChain copy logic and DetectedLabels copy logic | No longer propagated to AIAnalysis |
| `pkg/aianalysis/handlers/request_builder.go` | Remove DetectedLabels mapping to HAPI request (OwnerChain mapping already removed by ADR-055) | HAPI computes its own labels |
| `api/signalprocessing/v1alpha1/signalprocessing_types.go` | Keep `OwnerChain` in `KubernetesContext` (SP-internal) | SP still needs it for its own label detection |
| `pkg/shared/types/zz_generated.deepcopy.go` | Regenerate with `controller-gen` after type changes | DeepCopy must match updated types |

### Phase 3: Transparent Label Injection in Workflow Discovery

| File | Change | Rationale |
|------|--------|-----------|
| `holmesgpt-api/src/toolsets/workflow_discovery.py` | Remove `should_include_detected_labels()` function. Remove `detected_labels` from `list_workflows` tool parameter schema. Update toolset to read labels from session state (stored by `get_resource_context`) and inject them transparently into DataStorage queries. LLM calls `list_workflows(action_type)` with no label parameters. | Labels are HAPI-internal; LLM never sees or manages them. Guard function no longer needed -- labels always match target. |
| `holmesgpt-api/src/extensions/incident/prompt_builder.py` | Remove `build_cluster_context_section()` DetectedLabels rendering from LLM prompt. Labels are no longer provided as LLM context -- they are used internally for filtering only. | LLM does not need to reason about DetectedLabels |
| `holmesgpt-api/src/extensions/llm_config.py` | Update `register_workflow_discovery_toolset()` to not pass `detected_labels` from enrichment results. Toolset reads from session state instead. | Labels come from internal state, not request |

### Phase 4: Update SP Internal Label Flow

| File | Change | Rationale |
|------|--------|-----------|
| `internal/controller/signalprocessing/signalprocessing_controller.go` | Keep `detectLabels()` but store result in SP-specific status field instead of shared `EnrichmentResults` | Labels stay internal to SP |
| `pkg/signalprocessing/audit/client.go` | Update audit to read labels from SP's internal field | Audit still captures label detection results |
| `pkg/signalprocessing/detection/labels.go` | No changes -- label detection logic stays in SP for internal use | SP still detects labels for signal classification |

### Phase 5: Cleanup

| File | Change | Rationale |
|------|--------|-----------|
| `pkg/shared/types/enrichment.go` | Evaluate whether `EnrichmentResults` struct is still needed. If only `KubernetesContext` and `CustomLabels` remain, consider simplifying. | Reduce unnecessary abstraction |
| `api/openapi/data-storage-v1.yaml` | Update OpenAPI spec if `detected_labels` parameter semantics change | API contract alignment |
| Test files across `test/unit/`, `test/integration/`, `test/e2e/` | Update all tests that reference propagated OwnerChain or DetectedLabels | Test alignment with new architecture |

---

## Consequences

### Positive

1. **Accurate workflow discovery**: Labels always describe the resource being remediated, not the signal source. Workflow selection matches the actual target.

2. **Simpler LLM interface**: The LLM never sees DetectedLabels. `list_workflows` drops the `detected_labels` parameter. Fewer tool parameters means less hallucination risk and simpler prompt engineering.

3. **Simpler pipeline**: Removes OwnerChain and DetectedLabels propagation across three CRD boundaries (SP -> RO -> AIAnalysis -> HAPI). Fewer moving parts, fewer conversion points.

4. **Clean separation of concerns**: SP owns business classification (impact-scoped, stable). HAPI owns target properties (resource-scoped, post-RCA). Each component classifies what it has visibility into.

5. **Eliminates dead code**: `should_include_detected_labels()` and the unused discovery SQL label filtering become unnecessary.

6. **Consistent with ADR-055**: Completes the architectural shift started by ADR-055. All target-specific context (owner chain, spec hash, history, labels) is now computed post-RCA for the actual target.

7. **SP simplification**: SP's enrichment results become lighter. SP still performs label detection for its own audit and classification needs, but the results don't need serialization into shared types.

### Negative

1. **Additional K8s API calls at RCA time**: Label detection requires API calls (PDB lookup, HPA lookup, NetworkPolicy list, annotation inspection) during HAPI tool execution. Estimated ~5-8 additional API calls per investigation. Mitigated by K8s client caching.

2. **Label detection logic duplication**: SP has Go label detection in `pkg/signalprocessing/detection/labels.go`. HAPI will need Python equivalents. The implementations must stay aligned. Consider extracting shared documentation of detection criteria.

3. **Increased HAPI RBAC surface**: HAPI's ServiceAccount needs additional RBAC permissions for PDB, HPA, and NetworkPolicy resources.

4. **Larger refactor scope**: Changes touch SP, AIAnalysis, RO, HAPI, Data Storage, and shared types. Requires careful phased execution.

---

## Alternatives Considered

### Alternative A: Wire in `should_include_detected_labels()` Guard

Keep SP-computed labels but activate the existing guard function to exclude them when RCA diverges from signal source.

**Rejected because**:
- Only solves the "wrong labels" problem by exclusion (no labels at all), not by providing correct labels for the target
- Workflow discovery would operate with no label context for ~30-40% of investigations
- The guard function's owner chain dependency creates a circular problem (needs the chain to validate labels, but the chain describes the wrong resource)

### Alternative B: LLM-Driven Natural Language Filtering (No Labels)

Remove structured label filtering entirely. Let the LLM describe what it needs in natural language when calling workflow discovery.

**Deferred because**:
- Less deterministic -- harder to test and validate
- Safety guardrails (Rego) benefit from structured label data (e.g., "require approval for stateful workload operations")
- Could be revisited as the LLM-driven architecture matures
- May be the right long-term direction but premature for v1.0

### Alternative C: SP Re-enriches After RCA

After the LLM identifies the root cause, trigger a second SP enrichment pass for the RCA target resource.

**Rejected because**:
- Adds round-trip latency (HAPI -> Controller -> SP -> Controller -> HAPI)
- Architecturally backwards -- moves more work into the pipeline instead of simplifying it
- The LLM already has direct K8s API access via `get_resource_context`

---

## Related Decisions

- **[ADR-055](ADR-055-llm-driven-context-enrichment.md)**: LLM-Driven Context Enrichment (Post-RCA) -- prerequisite; established `get_resource_context` tool and post-RCA pattern
- **DD-WORKFLOW-001 v1.7/v2.1/v2.2**: DetectedLabels schema and validation framework
- **DD-CONTRACT-002**: Enrichment results schema -- will be updated to remove propagated fields
- **BR-SP-101**: DetectedLabels auto-detection -- scope narrows to SP-internal
- **BR-HAPI-194**: Honor failedDetections -- relocates to HAPI-computed labels
- **BR-HAPI-250/252**: DetectedLabels in workflow search -- source changes from SP to HAPI
