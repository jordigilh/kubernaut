# ADR-056: Post-RCA Label Computation (Target-Scoped, Not Signal-Scoped)

**Status**: ACCEPTED
**Decision Date**: 2026-02-12
**Version**: 2.0 (Go rewrite)
**Confidence**: 94%
**Applies To**: SignalProcessing, Kubernaut Agent (KA), AIAnalysis Controller, Data Storage

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0–1.7 | 2026-02-12 to 2026-03-25 | Architecture Team | Historical evolution under the Python-era implementation: relocated label computation from SignalProcessing to a `HAPI`-internal step, added a read-only `cluster_context` surfaced in the `list_available_actions` tool response, added a one-shot `detected_infrastructure` field on the resource-context tool response for RCA reassessment, then moved detection into a `HAPI`-internal `EnrichmentService` (Issue #529). Superseded by v2.0 below; see git history for the original entries. |
| 2.0 | 2026-08-02 | — | Rewritten against the Go KA implementation as part of [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806). **Key correction**: the decision this ADR records — compute infrastructure labels for the RCA-identified target's root owner, not the pre-RCA signal source — is unchanged and confirmed implemented (`internal/kubernautagent/enrichment/label_detector.go`). But the delivery mechanism the Python-era design built (a read-only `cluster_context` tool-response field plus a one-shot `detected_infrastructure` reassessment field) was **not carried into the Go rewrite** — neither string appears anywhere in `internal/kubernautagent/`. The actual Go mechanism is simpler: labels are (1) rendered directly into the investigation prompt (`internal/kubernautagent/prompt/builder.go`) and (2) propagated via `SignalContext.DetectedLabelsJSON` on `context.Context` to filter workflow-discovery queries (`internal/kubernautagent/tools/custom/tools.go`) — not a shared mutable `session_state` dict. Replaced the Python "Changes Required" migration-plan tables with a pointer to the current Go implementation and to [DD-KA-018](DD-KA-018-detected-labels-detection-specification.md) (label count corrected from 7–9 to the current 12 characteristics) and [DD-KA-017](DD-KA-017-three-step-workflow-discovery-integration.md). Renamed `HAPI` → Kubernaut Agent (KA) throughout. |

---

## Context & Problem

### Background: ADR-055 Exposed a Deeper Gap

[ADR-055](ADR-055-llm-driven-context-enrichment.md) moved owner-chain resolution, spec hash, and remediation history from pre-RCA signal-source computation to post-RCA target-scoped computation. That correctly addressed the "wrong resource context" problem for those three data points.

It did not, at the time, address **DetectedLabels**, which suffer from the same flaw: they were computed at **signal time** for the **signal source resource** by SignalProcessing, then propagated downstream for workflow discovery and LLM prompt context.

### The Stale Labels Problem

When the LLM's RCA identifies a resource different from the signal source, DetectedLabels computed for the signal source are **stale and potentially misleading**:

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

This is not an edge case — RCA routinely identifies a resource outside the signal source's owner chain (Pod crash → Node memory pressure; Pod eviction → PVC storage class issue; Service latency → upstream dependency failure).

### Business Requirements Affected

- **BR-SP-101**: DetectedLabels auto-detection — scope is SP-internal (signal classification only)
- **BR-SP-103**: FailedDetections tracking — stays within SP
- **BR-KA-264/265**: Post-RCA label detection and use in workflow discovery

---

## Decision

### Relocate Label Computation from SignalProcessing to KA (Post-RCA, Automatic)

Compute infrastructure labels for the **resolved root owner of the RCA-identified target**, not the signal source — as part of the same automatic enrichment pass ADR-055 introduced, not a separate LLM-triggered step.

### Foundational Principle: Impact vs. Target (unchanged)

- **Impact-scoped (SignalProcessing, pre-RCA)**: Business classification — environment, priority, severity, business unit. These describe the **business impact of the signal** and remain correct regardless of what RCA identifies as the root cause.
- **Target-scoped (KA, post-RCA)**: DetectedLabels — stateful, hpaManaged, helmManaged, pdbProtected, GitOps-managed, service mesh, CNV/KubeVirt characteristics, etc. These describe **operational properties of the remediation target** and can only be answered correctly for the actual target RCA identifies.

SignalProcessing owns impact classification. KA owns target properties. Neither crosses into the other's domain.

### Current Implementation (Go KA)

```
SignalProcessing              AIAnalysis           Kubernaut Agent (KA)
+----------------------+      +----------+         +--------------------------+
| Business classif.:   |      |          |         | 1. LLM performs RCA      |
|  - environment       |      | Signal   |-------->| 2. KA resolves enrichment|
|  - priority          |      | context  | request |    target (ADR-055)      |
|  - severity          |      | only     |         | 3. Enricher.Enrich runs  |
|  - business unit     |      | (no      |         |    automatically:         |
|                      |      |  labels  |         |    - owner chain, spec    |
| DetectedLabels:      |      |  or      |         |      hash, history        |
|  (SP-internal only)  |      |  owner   |         |    - LabelDetector:       |
|  for its own audit/  |      |  chain)  |         |      12 characteristics   |
|  classification      |      |          |         |      for the root owner   |
+----------------------+      +----------+         | 4. Labels rendered into   |
                                                    |    investigation prompt   |
                                                    | 5. Labels propagated via  |
                                                    |    SignalContext.         |
                                                    |    DetectedLabelsJSON to  |
                                                    |    filter list_workflows/ |
                                                    |    list_available_actions |
                                                    +--------------------------+
```

### Key Design Principles

1. **Impact vs. target separation** (unchanged). Business classification (SignalProcessing) describes signal impact and is stable across RCA outcomes. DetectedLabels (KA) describe the remediation target and are computed post-RCA for the correct resource.

2. **DetectedLabels are KA-computed and delivered two ways, both read-only to the LLM.** Labels are computed once per investigation, during the same automatic enrichment pass that resolves owner chain/spec hash/history (ADR-055) — not via a separate LLM tool call, and not stored in a shared mutable dict. They reach the LLM through: (a) direct rendering into the investigation prompt (`internal/kubernautagent/prompt/builder.go`, e.g. "Detected labels: hpaManaged=true, helmManaged=true"), giving the LLM infrastructure context during RCA and workflow-selection reasoning; and (b) transparent injection as filter criteria into `list_available_actions`/`list_workflows` catalog queries via `SignalContext.DetectedLabelsJSON` (`internal/kubernautagent/tools/custom/tools.go`). The LLM never passes labels as a tool parameter and cannot override them. **Simplified from the original Python-era design**: the earlier read-only `cluster_context` tool-response field and the one-shot `detected_infrastructure` RCA-reassessment field were not carried into the Go rewrite — direct prompt rendering replaced both.

3. **Workflow discovery tools take no label parameter.** The LLM calls `list_workflows(action_type)` with no `detected_labels` argument; KA applies the stored labels as filter criteria internally. This is unchanged from the original decision.

4. **SignalProcessing keeps its own labels for internal purposes.** SP still computes labels for signal classification and its own audit events. These do not leave SP.

5. **KA's enrichment pass (ADR-055) is the single source of truth for target context, including labels.** Owner chain, spec hash, remediation history, and DetectedLabels are all resolved together, for the same RCA-identified target, in the same automatic pass — not four independently-triggered lookups.

6. **No backwards compatibility required.** Carried forward from the original decision; still true.

---

## Consequences

### Positive

1. **Accurate workflow discovery**: Labels always describe the resource being remediated, not the signal source.
2. **Simpler LLM interface**: `list_workflows` takes no label parameter — fewer parameters, less hallucination risk.
3. **Simpler pipeline**: Removes DetectedLabels propagation across CRD boundaries (SP → RO → AIAnalysis → KA); KA computes them directly against the K8s API for its resolved target.
4. **Clean separation of concerns**: SignalProcessing owns business classification (impact-scoped, stable); KA owns target properties (resource-scoped, post-RCA).
5. **Consistent with ADR-055**: Completes the architectural shift — all target-specific context (owner chain, spec hash, history, labels) is computed together, post-RCA, for the actual target.
6. **Simpler than originally designed**: The Go implementation dropped the Python-era `cluster_context`/`detected_infrastructure` two-field delivery split in favor of straightforward prompt rendering, with no loss of the underlying guarantee (labels always describe the correct target).

### Negative

1. **Additional K8s API calls at RCA time**: Label detection (PDB, HPA, NetworkPolicy, ResourceQuota, CNV/KubeVirt lookups) runs as part of every investigation's enrichment pass. Mitigated by K8s client caching within `Enricher`.
2. **Increased KA RBAC surface**: KA's ServiceAccount needs read access for PDBs, HPAs, NetworkPolicies, ResourceQuotas, and (for CNV/KubeVirt detections) VirtualMachine-related resources.
3. **Single Go implementation, single point of specification drift risk**: `internal/kubernautagent/enrichment/label_detector.go` is the sole implementation (SignalProcessing's original Go label detector was retired per this ADR). Mitigated by [DD-KA-018](DD-KA-018-detected-labels-detection-specification.md), the authoritative detection specification kept in sync with the Go source.

---

## Alternatives Considered

### Alternative A: Guard-Based Exclusion

Keep SignalProcessing-computed labels but exclude them whenever RCA diverges from the signal source.

**Rejected because**: solves "wrong labels" only by removing labels entirely for ~30-40% of investigations where RCA diverges from the signal source — workflow discovery would then operate with no label context at all for those cases.

### Alternative B: LLM-Driven Natural-Language Filtering (No Structured Labels)

Remove structured label filtering; let the LLM describe infrastructure constraints in natural language.

**Deferred because**: less deterministic and harder to test; Rego-based safety guardrails benefit from structured label data (e.g., "require approval for stateful workload operations"). May be revisited as the architecture matures, but not adopted for V1.0.

### Alternative C: SignalProcessing Re-enriches After RCA

Trigger a second SignalProcessing enrichment pass for the RCA-identified target.

**Rejected because**: adds a round-trip (KA → Controller → SP → Controller → KA) for something KA can compute directly against the K8s API in one pass, as part of the same enrichment step that already resolves owner chain and spec hash.

---

## Related Decisions

- **[ADR-055](ADR-055-llm-driven-context-enrichment.md)**: Post-RCA Context Enrichment — prerequisite; established the automatic, target-scoped enrichment pass this decision's label computation is now part of.
- **[DD-KA-018](DD-KA-018-detected-labels-detection-specification.md) v2.0**: DetectedLabels Detection Specification — authoritative detection contract, 12 characteristics, ground truth for `label_detector.go`.
- **[DD-KA-017](DD-KA-017-three-step-workflow-discovery-integration.md) v2.0**: Three-Step Workflow Discovery Integration — authoritative for how labels reach workflow-discovery filtering via `SignalContext.DetectedLabelsJSON`.
- **DD-WORKFLOW-001**: DetectedLabels schema and validation framework.
- **BR-SP-101**: DetectedLabels auto-detection — scope is SP-internal.
- **BR-KA-264/265**: DetectedLabels detection and use in workflow discovery.

---

## Confidence Assessment

**Confidence: 94%**

| Risk | Assessment |
|------|-----------|
| Label detection specification drift | Low. Single Go implementation (`label_detector.go`); DD-KA-018 is the formal specification kept in sync with source, both dated 2026-08-01. |
| RBAC configuration gaps | Low-Medium. KA's ServiceAccount needs PDB/HPA/NetworkPolicy/ResourceQuota/CNV-VM RBAC; missing grants surface as `FailedDetections` entries rather than silent wrong data (fail-observable, not fail-silent). |

**Residual gap (6%)**: The read-only `cluster_context`/`detected_infrastructure` reassessment mechanism the Python-era design specified was not reimplemented in Go (see v2.0 changelog). This ADR treats that as an intentional simplification confirmed by the current implementation, not a gap to close — but it is called out here in case a future iteration wants the one-shot RCA-reassessment capability back.
