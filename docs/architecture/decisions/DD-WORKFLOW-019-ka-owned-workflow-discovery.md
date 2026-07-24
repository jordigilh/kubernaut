# DD-WORKFLOW-019: Relocating Workflow Discovery/Selection Ownership from Data Storage to KubernautAgent

**Date**: July 17, 2026
**Status**: ✅ **APPROVED — Implemented** (Issue [#1677](https://github.com/jordigilh/kubernaut/issues/1677), Phases 1-2g)
**Decision Maker**: Kubernaut Architecture Team
**Version**: 2.0
**Authority**: APPROVED — implementation complete
**Affects**: Data Storage Service (DS), KubernautAgent (KA), APIFrontend (AF), Notification
**Related**: [DD-WORKFLOW-018](./DD-WORKFLOW-018-etcd-single-source-of-truth.md) (Etcd single source of truth — this decision's predecessor and prerequisite), [DD-WORKFLOW-016](./DD-WORKFLOW-016-action-type-workflow-indexing.md) (Three-step discovery protocol, Change 2 of DD-WORKFLOW-018 ports this into DS's in-memory cache — this decision moves that cache/logic again, from DS to KA), [DD-HAPI-017](./DD-HAPI-017-three-step-workflow-discovery-integration.md) (Three-Step Workflow Discovery Integration), [DD-KA-001](./DD-KA-001-workflow-response-validation-architecture.md) (KA's existing schema-fetch/validation responsibility)
**Supersedes**: DS as the discovery/scoring/audit-emission owner established by DD-WORKFLOW-018 Change 1-2 (Issue #1661)
**Business Requirement**: BR-WORKFLOW-007 (ActionType/workflow discovery), BR-AUDIT-023 v2.1 (workflow discovery audit trail — "who generates" amended to KA)

---

## Changelog

### Version 2.0 (2026-07-23) — Implemented

- Issue #1661 landed and stabilized; CHECKPOINT DD re-validated the evidence below against the then-current codebase
  (sole-consumer status, cache mechanics, cutover behavior parity, audit-event schema/governance) with no material
  changes to the original analysis. User approved the resulting Wiring Manifest and phased TDD plan.
- Implemented across Phases 2a-2g (Issue #1677): KA's own informer-backed cache + RBAC (2a), ported
  discovery/scoring logic (2b), KA audit infrastructure + BR-AUDIT-023/DD-WORKFLOW-014 "who generates" amendment
  (2c), the 3 custom MCP tools cut over to `workflowcatalog.Catalog` (2d), `select_workflow`/`investigate_discovery`
  rewired (2e), APIFrontend's `kubernaut_list_workflows` rewired to `cfg.KAMCPClient` (2f), and DS's
  `/api/v1/workflows*` REST surface + success-metrics machinery retired as dead code (2g).
- One deviation from the "Anticipated scope": `TotalExecutions`/`SuccessfulExecutions`/`ActualSuccessRate`
  ("success-metrics overlay") were dropped entirely rather than migrated — confirmed (via code and grep) to be
  dead, best-effort display telemetry from an abandoned success-rate-weighted-selection design, with zero use in
  the actual scoring logic. The fields were first left on `models.RemediationWorkflow` as always-zero-valued
  wire-compat stubs, then deleted outright once a repo-wide sweep confirmed zero remaining Go references
  anywhere (production or test) — matching this bullet's own "dropped entirely" wording. `PaginationMetadata`/
  `MandatoryLabels` were evaluated for the same treatment and found to still be live: both are actively used by
  KA's own `workflowcatalog` package (`cache_convert.go`, `cache_filter.go`) and
  `internal/kubernautagent/tools/custom/tools.go`, so only their OpenAPI-schema representations (not the Go
  types) were pruned below.
- `api/openapi/data-storage-v1.yaml` has since been pruned to remove the retired `/api/v1/workflows*` and
  `/api/v1/action-types/{name}/workflow-count` routes and their associated schemas (`WorkflowListResponse`,
  `PaginationMetadata`, `ActionTypeEntry`, `MandatoryLabels`, `ActionTypeWorkflowCountResponse`, etc. — as DS
  REST contract types; the same-named Go types in `pkg/datastorage/models` remain, per above), and the
  generated `pkg/datastorage/ogen-client` regenerated to match. Production behavior was already correct
  (`test/e2e/datastorage/04_workflow_endpoints_retired_test.go`); this closes the contract-vs-behavior gap the
  v2.0 changelog originally flagged here as a residual follow-up. Regenerating the client surfaced a real missed
  production consumer (`cmd/kubernautagent/toolregistry.go`'s workflow validator fetcher was still calling DS's
  REST endpoint directly), fixed as part of this same cleanup.
- KA cache-restart resilience: `test/e2e/datastorage/27_ds_restart_cache_recovery_test.go` (deleted in Phase 2g)
  proved DD-WORKFLOW-018's "disposable, etcd-backed cache survives a restart with zero data loss" property E2E,
  against a real pod kill, for DS's now-retired catalog. That property now lives at the IT tier instead —
  `IT-KA-1677-CACHE-007` (`test/integration/kubernautagent/workflowcatalog/cache_test.go`) builds a brand-new
  `workflowcatalog.Cache` against the same envtest API server (simulating what a replacement KA pod does on
  startup) and proves it re-derives an identical catalog with zero manual reseeding. A full E2E-tier equivalent
  (real pod kill against an isolated KA deployment) was deliberately not built: every read here goes through
  controller-runtime's own List/Watch/WaitForCacheSync machinery, not bespoke cache logic, so what's actually at
  risk on a real KA restart is KA's own construction/wiring of that informer — exactly what the IT test exercises
  — not the informer implementation itself or generic Kubernetes pod-restart/scheduling behavior.
- Clarified Phase 1's "RemediationOrchestrator/Notification read workflow display metadata from AIAnalysis
  instead of live DataStorage lookups" bullet: Notification never reads the `AIAnalysis` CRD directly.
  RemediationOrchestrator (RO) is the only reader — it copies `WorkflowName`/`ActionType` from
  `AIAnalysis.Status.SelectedWorkflow` (a `sharedtypes.WorkflowSnapshot`, `+kubebuilder:validation:Required` on
  both fields, sourced from `RemediationWorkflow.metadata.name`) into `NotificationRequest.Spec.Context.Workflow`
  when it creates the `NotificationRequest`. Notification only ever consumes the already-denormalized CRD field.
- Follow-up dead-code removal, prompted by the above: `WorkflowName` being `+kubebuilder:validation:Required` on
  `WorkflowSnapshot` means "workflow ID present, `WorkflowName` absent" cannot occur via either of RO's two
  notification-creation call sites (`pkg/remediationorchestrator/creator/notification.go`) — confirmed these are
  the only two production call sites that construct a `WorkflowContext`. That made
  `pkg/notification/enrichment`'s live-DataStorage-lookup fallback (`WorkflowNameResolver` interface, and the
  branch in `Enricher.EnrichNotification` that called it) structurally unreachable, not merely unused: its own
  backing implementation (`DataStorageResolver`) was already deleted in Phase 2g, leaving an interface with zero
  implementations. Removed outright: `pkg/notification/enrichment/resolver.go` deleted,
  `Enricher`/`NewEnricher` simplified to drop the resolver parameter, and `cmd/notification/main.go`'s wiring
  comment updated. Rewrote the ~13 dependent specs across `pkg/notification/enrichment_test.go` (9 of 14 kept,
  adapted to pre-populate `WorkflowName` like real callers do; 5 removed as redundant with the resolver gone —
  "resolver failure" and "empty resolved name" collapse into a single "name not populated" case, and "prefers
  pre-populated name over resolver" is no longer expressible once there's no resolver to prefer over) and
  `test/integration/notification/enrichment_delivery_test.go` (all 3 specs kept, adapted the same way).

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

## Decision (Implemented)

Ownership of the workflow/action-type discovery, search, and scoring logic — and the informer-backed cache that
backs it — has been relocated from Data Storage into KubernautAgent. Data Storage's role in the workflow domain is
now strictly what DD-WORKFLOW-018 already established for PostgreSQL: an audit trail and on-demand aggregation
source, with no remaining read/write responsibility for catalog *definitions* of any kind.

This decision was approved and executed as Issue #1677:

1. CHECKPOINT DD was re-run after Issue #1661 landed, re-validating the evidence below against the then-current
   codebase (call sites, replica counts, RBAC posture) — no material change to the original analysis.
2. A Wiring Manifest and phased TDD plan (Phases 2a-2g) were produced and approved, following the same rigor as
   DD-WORKFLOW-018's own Issue #1661 execution.
3. Explicit user approval was obtained before implementation began, per this project's CHECKPOINT DD governance.

### Implemented scope

- Relocated the discovery/scoring logic (`pkg/datastorage/repository/workflow/{discovery,cache_filter,discovery_cache,cache_convert}.go`
  and the relevant `pkg/datastorage/workflowcache` pieces, originally built during Issue #1661 Phases 28-33) into
  `internal/kubernautagent/workflowcatalog`, adapted to KA's own informer cache over
  `RemediationWorkflow`/`ActionType` (Phases 2a-2b).
- Added the incremental RBAC for KA to `list`/`watch` those two CRDs (Phase 2a).
- Rewired APIFrontend's `kubernaut_list_workflows` to `cfg.KAMCPClient`, mirroring the existing
  `kubernaut_discover_workflows` pattern, with a new `ka.MCPClient.ListWorkflows` + `HandleListWorkflowsKA` (Phase 2f).
- Added discovery/selection audit-event emission in KA via the existing `BufferedDSAuditStore` path, preserving
  BR-AUDIT-023/DD-WORKFLOW-014 coverage without DS's handler-side emission (Phase 2c-2d).
- Removed DS's `/api/v1/workflows*`, `/api/v1/action-types/{name}/workflow-count` REST surface and all now-dead
  repository/cache/handler code once both consumers (KA-internal, APIFrontend) no longer called it directly
  (Phase 2g). The OpenAPI *contract* for these routes has not yet been pruned to match — see the v2.0 changelog
  entry above.

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

**Confidence: ~95% — implemented and verified.**

**Justification**: every claim in the Problem Statement was verified against actual source before implementation
began (call-site greps, reading `mcp_bridge.go`, `ds_buffered_store.go`, and `routes.go` directly), and the
resulting Wiring Manifest was executed phase-by-phase with build/lint/test verification at each step. Post-
implementation verification (Phase 2g) confirmed zero remaining production callers of every retired DS symbol via
both a full-repo `go build`/`go vet`/`golangci-lint` pass and gopls-based (`go_symbol_references`) reference checks
on the still-live replacement symbols (`ka.MCPClient.ListWorkflows`, `workflowcatalog.Catalog`). The residual 5% is
the known OpenAPI-contract-drift gap noted in the v2.0 changelog entry, not a risk to the implemented behavior.

**Next Review**: N/A — implementation complete. Re-open only if the deferred OpenAPI spec cleanup or a future
architectural change (e.g. KA horizontal scaling requiring a shared cache) requires revisiting this decision.
