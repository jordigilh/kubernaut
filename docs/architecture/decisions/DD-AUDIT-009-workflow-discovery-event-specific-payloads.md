# DD-AUDIT-009: Event-Specific Typed Workflow Discovery Payloads

**Status**: ✅ Approved and implemented for #2459/#2466; focused unit tests, live Data Storage ITs, live etcd-Catalog IT, and the post-fix focused Kind E2E pass. The E2E verifies remediation-ID reconstruction and the raw audit minimization boundary.
**Decision Date**: 2026-09-26
**Owner**: Kubernaut Agent / Data Storage
**Scope**: KA workflow discovery audit events and the Data Storage OpenAPI event-data union
**Related**: Issue #2459, BR-AUDIT-005, BR-AUDIT-023/025/026/030, DD-AUDIT-004, DD-KA-017, DD-WORKFLOW-016, DD-WORKFLOW-019, ADR-034

## Context

KubernautAgent (KA) emits the four `workflow.catalog.*` discovery events; Data Storage validates, persists, and queries them. The OpenAPI contract currently maps all four event types to one `WorkflowDiscoveryAuditPayload`. Step 1 (`actions_listed`) returns action-taxonomy entries, while Step 2 (`workflows_listed`) returns workflow candidates for a requested `action_type`. These different business results currently share a generic shape, and the producer persists counts while dropping the actual choices/candidates. Step 2's actual `final_score` is computed for deterministic ordering in KA but is discarded before audit serialization.

This prevents remediation-scoped audit queries from reconstructing what the catalog exposed and why a workflow candidate ranked where it did. It also makes the common payload increasingly dependent on event-specific optional properties.

## Decision

Use distinct, typed OpenAPI payload variants for the two result-bearing events:

- `workflow.catalog.actions_listed` maps to a Step 1 payload containing the returned action-type entries and page/result metadata.
- `workflow.catalog.workflows_listed` maps to a Step 2 payload containing the requested `action_type`, returned workflow candidates, stable identity/version, rank, and the actual `final_score` used for ordering.
- Reuse common typed components for signal filters, detected labels, pagination, timing, and shared audit context. Keep `workflow.catalog.workflow_retrieved` and `workflow.catalog.selection_validated` on their current payload contract; they are outside this issue's result-capture change.
- Preserve the four existing event type strings and the outer ADR-034 audit envelope. No database migration or new event type is introduced.
- Carry `final_score` from the cache-backed Catalog through a discovery-only result path into the Step 2 audit payload. Do not recompute it in the audit builder and do not add it to the LLM-facing workflow result.
- Follow-up #2466 clarifies the separate Step 3 boundary: the LLM may receive the allowed operational parameter schema, but generic audit storage must not persist the `get_workflow` result. The persisted `aiagent.llm.tool_call` result and the in-memory history field assembled for later `aiagent.llm.request` audit events use an omission marker; the current Data Storage LLM-request schema does not persist message history. The tool arguments retain the selected workflow ID; Step 1/2 typed audit events remain the reconstruction source for discovery candidates.
- Use an explicit `final_score` property and preserve the current audit result's nested `scoring.confidence` for compatibility. For new Step 2 events, both fields carry the same cache-computed label-ranking score, following the existing `WorkflowSearchResult` schema precedent; this is not the LLM's decision confidence. `final_score` remains optional in the wire schema so historical payloads without it remain readable; the current KA producer must populate it.
- Clarify the `ScoringV1Audit.confidence` description for these workflow result records to identify it as the legacy catalog-ranking-score field, avoiding confusion with LLM decision confidence.
- Keep newly introduced result fields optional in the wire schema for mixed-version compatibility. The current KA producer must populate them. Generated decoders and Data Storage query results must continue to handle historical payloads.

## Alternatives considered

### A. Persist identities and rank, omit score — rejected

This is the narrowest change and would prove which candidates were returned. It loses the actual ranking signal already computed by KA and leaves BR-AUDIT-026 incomplete.

### B. Extend one shared discovery payload — rejected

This would minimize OpenAPI discriminator changes and could preserve score-complete results. However, it makes one shared schema accept action entries and workflow candidates together, permits cross-step field combinations, and accumulates event-specific optional fields. It is less precise for generated types and future evolution.

### C. Event-specific typed result payloads — approved

Separate Step 1 and Step 2 contracts, while sharing common nested schemas. This best expresses the domain and lets each event validate and evolve its own results. The additional generated union/client work is accepted and will be guarded by compatibility tests.

### D. Untyped generic JSON/map — rejected

This avoids generated schema changes but bypasses DD-AUDIT-004's typed-payload mandate and weakens validation and consumer contracts.

## Consequences

### Positive

- The event discriminator selects a payload type that matches that discovery step's business result.
- Audit queries can reconstruct the action options, selected action category, returned candidates, and their actual order/score under the remediation correlation ID.
- Shared filter/timing definitions stay consistent without coupling unrelated event result shapes.
- Scores remain audit-only and cannot alter the LLM's selection input or response contract.

### Trade-offs and mitigations

- Ogen discriminator mappings and generated union getters change; regenerate with repository targets and test each event-specific type end-to-end.
- Historical events and mixed-version producers must remain readable. Keep new fields optional, preserve existing common fields and legacy scoring representation, and add old/new payload compatibility tests.
- The cache Catalog currently discards scores after sorting. Carry the original value through a focused discovery result type and assert score fidelity; do not infer score from rank.
- The audit payload adds only the missing `final_score` while retaining the legacy nested score field; both values must come from the same score computation. Generated-schema and Data Storage integration tests prove this mapping.

## Schema precedent

The Data Storage OpenAPI document has an existing `WorkflowSearchResult` component requiring both `confidence` and `finalScore`. It describes `confidence` as the normalized label score and `finalScore` as “same as confidence”; `pkg/datastorage/models/workflow.go` mirrors that representation. DD-WORKFLOW-013 identifies this as the label-only catalog ranking model. This gives direct precedent for the pair of score fields and their equality.

This precedent is not an active endpoint contract: the OpenAPI document states the old search endpoint/types were removed when discovery ownership moved into KA, and the workflow-search components remain as deprecated schemas. The new audit result derives only the explicit final-score naming/equality semantics, not the deprecated DTO wholesale. In particular, the old DTO includes parameter and execution-bundle properties excluded from audit under data minimization. DD-KA-004 also distinguishes this catalog ranking score from the LLM's separate workflow-selection confidence; the audit payload must keep that distinction explicit.

## Feasibility spike

An isolated prototype on 2026-09-26 copied the current Data Storage OpenAPI document, mapped Step 1 and Step 2 to distinct payload schemas, retained the shared schema for Step 3, and generated with the repository-pinned Ogen v1.20.1. Ogen produced distinct request and response union variants. Scratch Go tests decoded and validated the pre-change common Step 1/Step 2 payload shape through both unions. The discriminator experiment requires narrowing the legacy shared schema's event-type enum to Step 3 so `oneOf` has exactly one matching schema. This prototype did not exercise the final workflow-result score schema or a live Data Storage validator; those remain in implementation/IT scope. The prototype and generated files remained outside the repository; no production schema/client was modified.

The score path was traced separately: the current cache scorer sorts `scoredWorkflow` values then drops the score while projecting to workflow models. The existing focused ordering scenario passes. Implementation must preserve that score through the Step 2 Catalog/tool path while retaining Step 1's use of the scorer for match counts and keeping the LLM projection score-free.

## Requirements and control objectives

- BR-AUDIT-023 and FedRAMP AU-2: emit/persist the step-specific discovery event.
- BR-AUDIT-025/026 and FedRAMP AU-3: preserve the action/filter context and actual per-candidate score/result content.
- BR-AUDIT-005/030 and SOC 2 CC7.2: reconstruct discovery decisions from audit events queried by remediation ID.
- OWASP ASVS v5.0.0 V16.2.1 and V16.2.4: retain necessary when/where/who/what investigation metadata and machine-readable, correlatable logs. V16.2.5 requires protection-level handling for sensitive data that is logged; this feature verifies the approved boundary by excluding workflow parameter/execution content and testing that a synthetic sensitive-content sentinel is absent from persisted audit records.

## Verification gate

The user authorized execution of [TP-2459-v1.1](../../tests/2459/TEST_PLAN.md) and its companion [implementation plan](../../tests/2459/IMPLEMENTATION_PLAN.md). Focused UTs, the live-Data-Storage audit IT, the live-Catalog projection IT, and the post-#2466 AgentSession E2E pass. The E2E queries real Data Storage by remediation ID and verifies both typed discovery evidence and the generic `get_workflow` audit omission marker. See [TP-2466](../../tests/2466/TEST_PLAN.md) for the LLM/audit boundary contract.
