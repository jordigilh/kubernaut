# ADR-055: Post-RCA Context Enrichment (Target-Scoped, Not Signal-Scoped)

**Status**: ACCEPTED
**Decision Date**: 2026-02-12
**Version**: 2.0 (Go rewrite)
**Confidence**: 90%
**Applies To**: Kubernaut Agent (KA), AIAnalysis Controller, SignalProcessing

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0–1.5 | 2026-02-12 to 2026-03-25 | Architecture Team | Historical evolution under the Python-era implementation: proposed moving context enrichment from pre-LLM computation to an LLM-driven tool call (`get_namespaced_resource_context` / `get_cluster_resource_context`), then to a `HAPI`-internal `EnrichmentService` (Issue #529). Superseded by v2.0 below; see git history for the original entries. |
| 2.0 | 2026-08-02 | — | Rewritten against the Go KA implementation as part of [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806). **Key correction**: the decision this ADR records — compute owner-chain/spec-hash/history context for the RCA-identified target, not the pre-RCA signal source — is unchanged and confirmed implemented. But the *mechanism* is not what v1.0–v1.4 decided: KA does not wait for the LLM to call a resource-context tool. `Investigator.resolveEnrichment` (`internal/kubernautagent/investigator/investigator.go`) runs automatically once the RCA target is resolved (`ResolveEnrichmentTarget`), calling `enrichment.Enricher.Enrich` (`internal/kubernautagent/enrichment/enricher.go`) unconditionally, for every investigation. The `get_namespaced_resource_context`/`get_cluster_resource_context` LLM tools (`internal/kubernautagent/tools/custom/resource_context.go`) still exist and remain registered, but are not the primary or sole trigger — enrichment happens whether or not the LLM calls them. Replaced the Python "Changes Required" migration-plan tables (file paths under `kubernaut-agent/src/...py`, already executed and merged long ago) with a pointer to the current Go implementation and to [DD-KA-016](DD-KA-016-remediation-history-context.md)/[DD-KA-017](DD-KA-017-three-step-workflow-discovery-integration.md), which are authoritative for current mechanics. Corrected the Rego input field name from the originally-proposed `affected_resource` to the actual implemented `remediation_target` (`pkg/aianalysis/rego/evaluator.go`). Renamed `HAPI` → Kubernaut Agent (KA) throughout. |

---

## Context & Problem

### Original Architecture (Pre-Computation Model, Retired)

Before this decision, the pipeline pre-collected context **before** the LLM performed Root Cause Analysis (RCA), computed from the **signal source** resource:

```
Signal -> SignalProcessing enriches with OwnerChain (of the signal source)
  -> RemediationOrchestrator copies OwnerChain to AIAnalysis.Spec.EnrichmentResults
  -> AIAnalysis Controller passes OwnerChain to KA's predecessor
  -> Pre-computes BEFORE LLM invocation:
      1. resolve_root_owner(owner_chain) -> root owner
      2. compute_spec_hash(root_owner) -> SHA-256 hash
      3. fetch_remediation_history(root_owner, spec_hash) -> history context
  -> LLM receives all pre-computed context + signal -> performs RCA
  -> LLM selects workflow based on pre-computed context
```

### Problems That Motivated the Change (still the reason this decision holds)

1. **Wrong resource context**: The owner chain and spec hash were computed from the **signal source** (e.g., the crashing Pod), not the **actual root cause target** (e.g., a misconfigured ConfigMap, an HPA with wrong thresholds, or a Deployment with missing resource limits). The LLM may identify a completely different resource as the root cause.
2. **Context pollution**: Pre-computed history for the signal source resource may be irrelevant to the actual root cause, consuming context window and biasing the LLM's reasoning.
3. **Wasted computation**: If the pre-computed owner chain or spec hash was empty or wrong, the LLM proceeded without it anyway — the pre-computation was not essential for RCA to begin with.
4. **Owner chain only for Pods**: SignalProcessing only computed owner chains for Pod signals; Deployment, StatefulSet, DaemonSet, Node, and Service signals had empty owner chains.
5. **Unnecessary data propagation**: The owner chain traversed three CRD boundaries (SP → RO → AIAnalysis → KA's predecessor) purely to enable a pre-computation that targeted the wrong resource.

### Business Requirements Affected

- **DD-KA-016**: Remediation history context (enhanced, not broken, by this decision)
- **BR-AI-023**: Investigation audit trail (unchanged)
- **DD-KA-017**: Three-step workflow discovery (enhanced — label/history context now scoped to the correct target)

---

## Decision

**Compute owner-chain, spec-hash, and remediation-history context for the RCA-identified target, not the pre-RCA signal source — automatically, once KA resolves that target.**

### Current Implementation (Go KA)

```
Signal -> AIAnalysis Controller passes signal context to KA
  -> KA invokes LLM with signal context only (no pre-computed enrichment)
  -> Phase 1 (RCA): LLM analyzes the signal and returns a structured result
     including a remediation target (kind/name/namespace), parsed by
     internal/kubernautagent/parser
  -> KA resolves the enrichment target: ResolveEnrichmentTarget prefers the
     LLM's RCA-identified target, falling back to the signal resource only
     if the LLM didn't provide one (internal/kubernautagent/investigator/investigator.go)
  -> KA automatically enriches for that target (no LLM tool call required):
     Investigator.resolveEnrichment -> enrichment.Enricher.Enrich
       1. Traverses K8s ownerReferences for the resolved target
       2. Identifies the root managing resource (e.g., Pod -> Deployment)
       3. Computes the spec hash for the root owner (DD-EM-002)
       4. Queries DataStorage for target-scoped remediation history (DD-KA-016)
       5. Detects 12 infrastructure characteristics for the root owner (DD-KA-018)
  -> Phase 2 (Workflow): LLM uses the three-step workflow discovery protocol
     (DD-KA-017), informed by the enrichment result rendered into the prompt
  -> KA returns the complete result: RCA + remediation target + workflow
     recommendation
```

This is KA's single, unified investigation flow (there is no separate incident/recovery split in Go KA).

**A secondary path also exists**: `get_namespaced_resource_context` / `get_cluster_resource_context` are still registered as LLM-callable tools (used by the interactive MCP mode's `select_workflow` pre-selection hook, and available to the autonomous flow's LLM). Calling them re-runs the same `Enricher.Enrich` logic for a target the LLM specifies. They are not a prerequisite for enrichment — the automatic path above always runs regardless of whether the LLM calls them.

### Key Design Principles

1. **The LLM identifies the target resource, not pre-computation.** The RCA-time remediation target is a first-class output of RCA, not derived from a pre-computed owner chain of the signal source.

2. **Target identity is K8s-verified, not blindly trusted from the LLM.** Per [DD-KA-006](DD-KA-006-remediation-target-in-rca.md) v2.0, KA injects `TARGET_RESOURCE_*` workflow parameters from a K8s-verified owner chain (`InjectRemediationTarget`) as `kaManagedParams`, which are never stripped regardless of workflow schema declaration. This superseded the original v1.0–v1.4 design of a strictly-required, unconstrained LLM response field enforced by a hard validator failure — the LLM's stated target is a strong input, but KA's own K8s lookups are the final authority for what gets injected into the workflow.

3. **Context is fetched for the right resource at the right time.** The spec hash and remediation history describe the resource that will actually be remediated, computed automatically once that resource is known — not the signal source, and not gated on an explicit LLM request.

4. **Automatic, not solely tool-driven.** Enrichment runs unconditionally for every investigation once the RCA target is resolved. This is a deliberate correction from the original v1.0–v1.4 decision (LLM must call a resource-context tool to trigger enrichment): making it automatic guarantees every investigation has correct target-scoped context, rather than depending on the LLM choosing to call a tool. The LLM-callable tools remain available as a secondary path (e.g., for the interactive MCP mode, or mid-investigation re-scoping).

5. **`remediation_target` is structured Rego policy input.** Instead of a boolean about owner-chain membership, the Rego evaluator (`pkg/aianalysis/rego/evaluator.go`) exposes the RCA-identified/K8s-verified target resource (kind, name, namespace) as `remediation_target` — enabling granular, per-kind approval policies (e.g., "require approval for Node remediations in production"). The field name is `remediation_target`, not the originally-proposed `affected_resource` — it was renamed during implementation to align with DD-KA-006's `remediationTarget` terminology.

---

## Advantages

1. **Accuracy**: Context is collected for the resource actually identified as the root cause, not the signal source.
2. **Efficiency**: No wasted computation when the owner chain is empty (non-Pod signals) or when the LLM identifies a different target.
3. **Simpler data flow**: Eliminates the SP → RO → AIAnalysis → KA propagation of owner chain data across three service boundaries.
4. **Cleaner LLM context**: RCA reasoning is not biased by pre-loaded remediation history for potentially the wrong resource.
5. **Guaranteed correctness over agentic optionality**: Making enrichment automatic (rather than purely LLM-tool-driven) removes the risk of an investigation completing without target-scoped context because the LLM didn't think to ask for it.
6. **Better Rego policies**: `remediation_target` (kind, name, namespace) as Rego input enables granular, per-kind approval rules — strictly more powerful than a boolean owner-chain-membership flag.

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Additional latency from automatic enrichment on every investigation | Low | Low | `Enricher.Enrich` performs a small number of sequential K8s/DataStorage calls; K8s client caching and DD-KA-016's indexed queries keep this bounded. |
| Owner-chain re-enrichment fails (RBAC, timeout, API unavailable) | Low | Medium | KA retries owner-chain resolution before giving up. Per BR-496 v2 / DD-KA-006 v2.0, if it hard-fails, KA sets `needs_human_review=true` with `human_review_reason=rca_incomplete` (`internal/kubernautagent/investigator/investigator_discovery.go`). |
| LLM identifies the wrong target, so KA enriches the wrong resource | Low | Low | Same risk existed under the pre-computation model. KA's automatic enrichment is strictly better because it targets the LLM's RCA output rather than a fixed pre-RCA guess, and target identity is ultimately K8s-verified (DD-KA-006) before being injected into workflow parameters. |
| Rego policy input schema drift | Low | Medium | `remediation_target` is a stable, tested field on `RegoInput` (`pkg/aianalysis/rego/evaluator.go`); covered by existing approval-policy tests. |

---

## Related Decisions

- **[DD-KA-006](DD-KA-006-remediation-target-in-rca.md)**: Remediation target identity — K8s-verified owner-chain injection, the current authority for how target identity flows into workflow execution.
- **[DD-KA-016](DD-KA-016-remediation-history-context.md)**: Remediation history context via target-scoped, spec-hash-matched queries — the current authority for the history-fetch mechanics referenced above.
- **[DD-KA-017](DD-KA-017-three-step-workflow-discovery-integration.md)**: Three-step workflow discovery — the current authority for how enrichment results (labels, history) reach the LLM during workflow selection.
- **[DD-KA-018](DD-KA-018-detected-labels-detection-specification.md)**: DetectedLabels detection specification — the current authority for the 12 infrastructure characteristics computed during enrichment.
- **[ADR-056](ADR-056-post-rca-label-computation.md)**: Post-RCA label computation — extends this decision's target-scoping principle to DetectedLabels.
- **[DD-EM-002](DD-EM-002-canonical-spec-hash.md)**: Canonical spec hash computation.
- **BR-AI-085**: Rego Policy Input Schema for Approval Decisions (includes default-deny safety pattern).
