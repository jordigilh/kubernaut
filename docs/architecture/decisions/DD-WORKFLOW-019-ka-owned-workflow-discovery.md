# DD-WORKFLOW-019: Relocating Workflow Discovery/Selection Ownership from Data Storage to KubernautAgent

**Date**: July 17, 2026
**Status**: Proposed — not yet approved for implementation; gated on [Issue #1661](https://github.com/jordigilh/kubernaut/issues/1661) completing
**Decision Maker**: Kubernaut Architecture Team (pending final approval)
**Version**: 1.0
**Authority**: PROPOSAL — captures a validated architectural direction for future planning; does not authorize implementation
**Affects**: Data Storage Service (DS), KubernautAgent (KA), APIFrontend (AF)
**Related**: [DD-WORKFLOW-018](./DD-WORKFLOW-018-etcd-single-source-of-truth.md) (Etcd single source of truth — this decision's predecessor and prerequisite), [DD-WORKFLOW-016](./DD-WORKFLOW-016-action-type-workflow-indexing.md) (Three-step discovery protocol, Change 2 of DD-WORKFLOW-018 ports this into DS's in-memory cache — this decision proposes moving that cache/logic again, from DS to KA), [DD-HAPI-017](./DD-HAPI-017-three-step-workflow-discovery-integration.md) (Three-Step Workflow Discovery Integration), [DD-KA-001](./DD-KA-001-workflow-response-validation-architecture.md) (KA's existing schema-fetch/validation responsibility)
**Supersedes**: nothing yet — this is a proposal, not an approved change
**Business Requirement**: BR-WORKFLOW-007 (ActionType/workflow discovery), BR-AUDIT-023 (workflow discovery audit trail)

---

## Changelog

### Version 1.0 (2026-07-17)

- Initial version. Captures an evidence-based analysis performed during Issue #1661 Phase 52, in response to the
  question: "now that DS's workflow catalog is an informer-backed cache with in-memory discovery/scoring logic
  (DD-WORKFLOW-018 Change 1-2), and KA is functionally the sole consumer of the sophisticated discovery protocol,
  does DS still need to host that logic — or should it move into KA?"
- Explicitly deferred: implementation does not start until Issue #1661's full phased rollout lands. This document
  exists so the analysis and evidence are not lost in the interim, and so the eventual follow-up issue has a properly
  scoped starting point rather than re-deriving the same evidence from scratch.

---

## Problem Statement

DD-WORKFLOW-018 correctly relocates workflow/action-type catalog *storage* from PostgreSQL to etcd, and moves the
discovery/scoring *logic* into an in-memory Go implementation running inside Data Storage (DS), backed by a
`controller-runtime` informer cache. This closes the divergence/availability problem DD-WORKFLOW-018 identifies.

It does not, however, ask a follow-up question: once that in-memory cache and logic exist, does DS remain the right
*service* to host them? DS's original justification for owning this logic was that it owned the PostgreSQL catalog
data — SQL filtering/scoring naturally lived next to the SQL storage. That justification is gone after
DD-WORKFLOW-018: the informer cache is just Go code watching two CRD types, runnable by any process with the right
RBAC. Concretely:

- The sophisticated three-step discovery protocol (`ListActions` → `ListWorkflowsByActionType` →
  `GetWorkflowWithContextFilters`/`GetByID`, DD-WORKFLOW-016/DD-HAPI-017) has exactly **one** production consumer:
  KubernautAgent (KA), via `internal/kubernautagent/tools/custom/tools.go`. No other service calls these endpoints in
  production.
- KA already imports `controller-runtime/pkg/client` (`cmd/kubernautagent/routes.go`) for other K8s API access, so
  adding `List`/`Watch` on `RemediationWorkflow`/`ActionType` is incremental RBAC, not a new capability class.
- KA already has a proven, buffered audit-write path to DS
  (`internal/kubernautagent/audit/ds_buffered_store.go`, `BufferedDSAuditStore`) used for other event types — the
  same path can carry the `workflow.catalog.actions_listed`-equivalent event (BR-AUDIT-023, DD-WORKFLOW-014 v3.0)
  that DS's handler currently emits as a side effect of serving the query.
- Every call from KA to DS for this protocol is a same-cluster network hop with no caching or batching benefit,
  since KA already re-derives its own decision context per investigation.

The one apparent counter-argument — "DS can't retire this logic, another consumer depends on it" — does not survive
inspection. APIFrontend (AF)'s `kubernaut_list_workflows` MCP tool is a second consumer of DS's workflow-catalog
repository, but AF's *sibling* tool for the sophisticated protocol, `kubernaut_discover_workflows`, is **already**
wired to call KA, not DS:

```251:270:pkg/apifrontend/handler/mcp_bridge.go
func registerKAMCPTools(srv *mcp.Server, cfg *MCPBridgeConfig, sem *semaphore.Weighted, shouldRegister toolGate) {
	...
	if shouldRegister("kubernaut_discover_workflows") {
		registerTool(srv, cfg, sem, "kubernaut_discover_workflows", "Discover available workflows with parameter schemas",
			func(ctx context.Context, args tools.DiscoverWorkflowsArgs) (any, error) {
				result, err := tools.HandleDiscoverWorkflows(ctx, cfg.KAMCPClient, args)
```

`kubernaut_list_workflows` (the simpler, non-scored catalog browse) is the only AF tool still calling `cfg.DSClient`
directly. Rewiring it to `cfg.KAMCPClient` is the same one-line pattern already proven for its sibling — it does not
require DS to remain the discovery-logic host, only that KA expose an equivalent read-only "list" capability
alongside its existing `discover`/`select` tools.

A second apparent counter-argument — "moving the cache into KA duplicates it across KA's replicas" — is not actually
an argument for keeping it in DS: any horizontally-scaled process running an informer cache duplicates that cache
per replica, regardless of which service the code lives in. If DS itself scales beyond one replica (a reasonable
production assumption), it duplicates the exact same cache the exact same number of times. This consideration is
symmetric and does not favor either placement on its own; it only matters insofar as KA's replica count differs
materially from DS's, which is an operational question for the eventual implementation, not an architectural
objection to the direction.

---

## Decision (Proposed)

**After [Issue #1661](https://github.com/jordigilh/kubernaut/issues/1661)'s phased rollout completes and stabilizes**,
relocate ownership of the workflow/action-type discovery, search, and scoring logic — and the informer-backed cache
that backs it — from Data Storage into KubernautAgent. Data Storage's role in the workflow domain becomes strictly
what DD-WORKFLOW-018 already establishes for PostgreSQL: an audit trail and on-demand aggregation source, with no
remaining read/write responsibility for catalog *definitions* of any kind.

This decision is **not yet approved for implementation**. It requires:

1. A dedicated CHECKPOINT DD review once Issue #1661 lands, re-validating the evidence above against the
   then-current codebase (call sites, replica counts, RBAC posture may have shifted).
2. A Wiring Manifest and phased TDD plan, following the same rigor as DD-WORKFLOW-018's own Issue #1661 execution —
   this is not a small change; it relocates cache-management, K8s RBAC, discovery/scoring logic, and audit-emission
   responsibility across two services simultaneously.
3. Explicit user approval before any code changes, per this project's CHECKPOINT DD governance.

### Anticipated scope (subject to revision at implementation time)

- Relocate the discovery/scoring logic (`pkg/datastorage/repository/workflow/{discovery,cache_filter,discovery_cache,cache_convert}.go`
  and the relevant `pkg/datastorage/workflowcache` pieces, all built during Issue #1661 Phases 28-33) into KA,
  adapting them to KA's own informer cache over `RemediationWorkflow`/`ActionType`.
- Add the incremental RBAC for KA to `list`/`watch` those two CRDs.
- Rewire APIFrontend's `kubernaut_list_workflows` to `cfg.KAMCPClient`, mirroring the existing
  `kubernaut_discover_workflows` pattern; add an equivalent read-only "list" tool/handler in KA if one does not
  already exist in a suitable form.
- Add discovery/selection audit-event emission in KA via the existing `BufferedDSAuditStore` path, preserving
  BR-AUDIT-023/DD-WORKFLOW-014 v3.0 coverage without DS's handler-side emission.
- Determine and remove whatever remains of DS's `/api/v1/workflows`, `/api/v1/workflows/actions` REST surface once
  both consumers (KA-internal, APIFrontend) no longer call it directly.

### Alternatives considered

| Alternative | Assessment |
|---|---|
| **Status quo** — leave discovery/scoring in DS (current DD-WORKFLOW-018 design) | Valid, lower-risk, but leaves an unnecessary network hop on KA's hot path and keeps DS hosting logic with a single real caller once AF's `list_workflows` is also considered a KA-shaped concern. |
| **Partial move** — relocate only the sophisticated three-step protocol to KA, leave `ListWorkflows` (AF's simpler browse) in DS | Avoids touching AF, but leaves DS maintaining a second, smaller read path for one caller — not clearly less total complexity than finishing the consolidation. |
| **Full move (this proposal)** | Consolidates all workflow-catalog *reads* into KA; DS becomes purely audit/ledger, matching its already-stated end-state role in DD-WORKFLOW-018. Highest consistency, but the largest single change. |

---

## FedRAMP / SOC2 Control Mapping (Anticipated)

| Concern | Control Objective | How It Would Be Satisfied |
|---|---|---|
| Discovery/selection audit trail (currently DS-handler-emitted) | CC7.2 (decision audit trails, workflow selection auditing) | KA emits the equivalent event via its existing `BufferedDSAuditStore` path — same control objective, different emitter |
| RBAC expansion for KA | AC-6 (least privilege) | Read-only `list`/`watch` on two CRD types; no write access; scoped identically to DS's existing informer RBAC today |

---

## Confidence Assessment

**Confidence: ~90% on direction, not yet spiked on mechanics.**

**Justification**: every claim in the Problem Statement was verified against actual source (call-site greps, reading
`mcp_bridge.go`, `ds_buffered_store.go`, and `routes.go` directly) rather than assumed — this is a validated
direction, not a speculative one. The two objections raised during initial review (second consumer, replica
duplication) were both checked and found to not hold, or to be symmetric rather than DS-favoring. What remains
unvalidated is implementation mechanics: exact RBAC scoping, whether KA's cache should be a shared component or
per-replica, and the precise migration sequencing for AF's tool rewiring — appropriately left to the future
CHECKPOINT DD review and spike this document defers to.

**Next Review**: after Issue #1661's full phased rollout completes; do not begin implementation planning before
then.
