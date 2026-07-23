# DD-WORKFLOW-018: Etcd as Single Source of Truth for Workflow/ActionType Catalog

**Date**: July 14, 2026
**Status**: Approved — Phased Rollout In Progress (tracked in [Issue #1661](https://github.com/jordigilh/kubernaut/issues/1661))
**Decision Maker**: Kubernaut Architecture Team
**Version**: 1.1
**Authority**: AUTHORITATIVE — governs the storage architecture for `RemediationWorkflow`/`ActionType` catalog data
**Affects**: Data Storage Service (DS), AuthWebhook (AW), Workflow Execution (WE), AIAnalysis (AA), Remediation Orchestrator (RO), HolmesGPT-API/KubernautAgent (KA)
**Related**: [DD-WORKFLOW-012](./DD-WORKFLOW-012-workflow-immutability-constraints.md) (Immutability), [DD-WORKFLOW-016](./DD-WORKFLOW-016-action-type-workflow-indexing.md) (Indexing/three-step discovery), [DD-AUDIT-003](./DD-AUDIT-003-service-audit-trace-requirements.md), [DD-AUDIT-004](./DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md), [ADR-034](./ADR-034-unified-audit-table-design.md) (Unified Audit Table), [ADR-058](./ADR-058-webhook-driven-workflow-registration.md) (Webhook-Driven Registration), [DD-KA-001](./DD-KA-001-workflow-response-validation-architecture.md) (Workflow Response Validation)
**Supersedes**: [DD-WORKFLOW-009](./DD-WORKFLOW-009-catalog-storage.md) (Workflow Catalog Storage) — in full
**Revises**: [DD-WORKFLOW-017](./DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) v1.2 — "Dual Storage Model", DS-Level validation rules, and Operational Management sections (see changelog entries in that document)
**Business Requirement**: [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md) — "Dual Storage Model" section rewritten to match this decision

---

## Changelog

### Version 1.1 (2026-07-23)

- **CRD-embedded execution snapshot deduplicated into a shared type.** `AIAnalysis.Status.SelectedWorkflow` and
  `WorkflowExecution.Spec.WorkflowRef` (item 9 above) each independently hand-copied the same 11-12 field list, and
  that duplication had already caused one drift bug in practice: `ActionType` was added to `SelectedWorkflow` but
  initially left off `WorkflowRef`. Both types now inline-embed a single `sharedtypes.WorkflowSnapshot` (Go anonymous
  struct embedding + JSON `,inline`), making a field added to one CRD's schema structurally impossible to omit from
  the other.
- **`WorkflowName` propagation gap closed.** Auditing this deduplication surfaced that KubernautAgent (KA) never
  actually populated `action_type`/`workflow_name` in its `select_workflow` wire response — `enrichFromCatalog`
  (autonomous path) and `applySelectedWorkflow`/`WorkflowCatalogAdapter.GetWorkflowByID` (interactive path) both
  omitted them, so `ActionType` silently stayed empty end-to-end despite item 11's audit-enrichment intent above, and
  `WorkflowName` was never wired at all. Both are now copied catalog-authoritatively (never LLM-suppliable) at every
  KA/AA/RO call site that constructs a `SelectedWorkflow`/`WorkflowRef`.
- No architectural change to the decision itself — this is a REFACTOR-phase consolidation and GREEN-phase bug fix
  within the already-approved "CRD-embedded workflow snapshot chain" (item 9), not a new design decision.

### Version 1.0 (2026-07-14)

- Initial version. Formally supersedes DD-WORKFLOW-009's "PostgreSQL + pgvector is the workflow catalog storage
  backend" decision. Establishes etcd (via the `RemediationWorkflow`/`ActionType` CRDs) as the sole source of truth
  for workflow/action-type *definitions*, with PostgreSQL relegated strictly to audit traces and on-demand
  aggregation queries.
- Rollout is phased; see [Issue #1661](https://github.com/jordigilh/kubernaut/issues/1661) for current implementation
  status against the Wiring Manifest in this document.

---

## Problem Statement

Since [DD-WORKFLOW-009](./DD-WORKFLOW-009-catalog-storage.md) (November 2025), Kubernaut has run a **dual-storage
model**: `RemediationWorkflow`/`ActionType` CRDs live in etcd (the Kubernetes-native, GitOps-friendly interface an
operator interacts with), while AuthWebhook (AW) bridges every CRD CREATE/UPDATE/DELETE to a synchronous HTTP call
against Data Storage (DS), which persists the authoritative catalog row in PostgreSQL. Discovery, execution-time
metadata resolution, and success-rate scoring all read from that PostgreSQL catalog.

This design has a structural availability/consistency problem: **DS's PostgreSQL catalog is a second, independently
mutable copy of state that etcd already owns.** Two copies of the same state that are not transactionally linked
will diverge whenever one changes without the other observing it — and there is no reconciliation controller to
detect or repair that divergence (an explicitly acknowledged gap in DD-WORKFLOW-017 v1.2's own confidence
assessment). Concretely:

- If DS's PostgreSQL is restored from a backup, migrated, or has its data volume wiped **without AW being
  restarted**, AW keeps believing CRDs are registered (their `.status.workflowId` is already populated) while DS's
  catalog no longer has a matching row — silent, permanent divergence with no automated detection.
- Every CRD admission (CREATE/UPDATE/DELETE) requires a **synchronous HTTP round-trip from AW to DS** before the
  Kubernetes API server can admit the request. If DS is briefly unavailable, workflow registration — a control-plane
  operation — is blocked on a data-plane dependency it has no architectural reason to need.
- A `WorkflowExecution` created against a `workflow_id` that is later superseded (version bump) or deleted has no
  guaranteed way to resolve its own execution-time metadata (`executionEngine`, `serviceAccountName`, dependencies,
  declared parameter schema) once that `workflow_id` stops resolving in DS's *live* catalog — the lookup happens at
  *execution* time, which can be arbitrarily later than *selection* time, across the `RemediationApprovalRequest`
  human-approval gate.

None of this is required by the actual business need. The LLM discovery protocol (DD-WORKFLOW-016) and general
catalog search are purely **label-based, no vector embeddings** — despite DD-WORKFLOW-009's original title citing
"PostgreSQL + pgvector," pgvector was never actually used for workflow search. There is no technical requirement for
a second, independently-writable copy of catalog *definitions*. The only genuine need for PostgreSQL in this domain
is what it already does well elsewhere in Kubernaut: an **immutable, queryable audit trail** (ADR-034).

---

## Decision

**Etcd (via the `RemediationWorkflow` and `ActionType` CRDs) becomes the sole source of truth for workflow/action-type
catalog *definitions*.** PostgreSQL is relegated strictly to what [ADR-034](./ADR-034-unified-audit-table-design.md)
already established it for: an append-only audit trail, plus on-demand aggregate queries computed *from* that trail.
There is no second mutable copy of catalog state anywhere in the system.

This is implemented as eleven coordinated changes, tracked end-to-end in
[Issue #1661](https://github.com/jordigilh/kubernaut/issues/1661):

### 1. DataStorage replaces its PostgreSQL catalog with an informer-backed in-memory cache

DS stops owning `remediation_workflow_catalog`/`action_type_taxonomy` as mutable PostgreSQL tables. Instead, DS runs
its own `controller-runtime` informer against the `RemediationWorkflow`/`ActionType` CRDs in etcd, maintaining an
in-memory cache that is rebuilt from a fresh `List` + continuous `Watch` on every DS startup. DS never accepts writes
into this cache from any other component — it is a **pure, read-only reflection of etcd**, always internally
consistent with the one source of truth.

**Effect**: a DS restart, a wiped PostgreSQL volume, or a PostgreSQL migration can no longer cause catalog
divergence, because there is no longer a second copy for the CRDs to diverge *from*. The cache simply rebuilds
itself from etcd, every time, deterministically.

### 2. Filtering, search, and scoring move from PostgreSQL SQL to in-memory Go

The three-step discovery protocol (DD-WORKFLOW-016) and general catalog search are ported from SQL `WHERE`/`ORDER
BY` clauses to equivalent in-memory Go predicates and comparators evaluated over the Change 1 cache. This was
confirmed safe because the existing filtering logic is label-based (no `pgvector` embedding search is actually
performed today, despite DD-WORKFLOW-009's original design intent).

### 3. Success-rate metrics become on-demand aggregates, not stored columns

`total_executions`, `successful_executions`, and `actual_success_rate` are no longer columns updated in place on a
catalog row. They are computed on demand, at query time, by aggregating the `audit_events` table (ADR-034) for the
relevant `workflow_id`. `UpdateSuccessMetrics` (confirmed to have zero production callers) is removed as dead code.
This keeps PostgreSQL strictly audit-only — there is nothing in the workflow domain it holds that isn't either raw
audit history or a computed aggregate of that history.

### 4. Deterministic workflow IDs are reused, not re-derived

DS's existing pure functions — `DeterministicUUID` (`uuid.NewSHA1` over the canonicalized spec) and
`computeContentHash` (SHA-256 over the same canonicalized spec) — already make `workflow_id` a deterministic
function of CRD content, with zero database dependency. These functions are shared (not reimplemented) so that
AuthWebhook can compute the identical `workflow_id`/content hash locally, guaranteeing continuity of existing IDs
across the migration with zero risk of divergent hashing logic between AW and DS.

### 5. AuthWebhook fully decouples from DataStorage for CRD admission

AW no longer makes **any synchronous HTTP call to DS** on the CRD admission path (CREATE/UPDATE/DELETE). Three
checks that today require a DS round-trip move to run locally in AW, against AW's own cache-backed
`client.Client` (the same watch-cache every controller-runtime client already maintains):

- **ActionType existence**: validated via a `.spec.name` field indexer against `ActionType` CRDs in etcd, checking
  `Status.CatalogStatus == Active`.
- **Content-hash / workflow-ID computation**: computed locally using the Change 4 shared pure functions.
- **Content-integrity / version-conflict detection**: AW lists existing `RemediationWorkflow` CRDs directly (its own
  etcd-backed client) to detect a same-version-different-content conflict, instead of relying on a PostgreSQL
  unique-constraint violation surfaced back from DS.

AW patches the CRD's own `.status` subresource directly after a local decision — there is no DS response to relay
status from anymore. **This is the concrete fix for the divergence problem in the Problem Statement**: there is no
longer any DS-side mutable state for AW to diverge from, because AW's admission decision never depended on DS being
reachable, correct, or even running.

### 6. DataStorage's mutation REST endpoints are removed entirely

`CreateWorkflowInline`, `DisableWorkflow`, `EnableWorkflow`, and the PostgreSQL-backed `ActionTypeExists` check that
existed only to serve them, are removed. AW's etcd-native check (Change 5) becomes the **sole** gate for
ActionType/RemediationWorkflow consistency — there is no remaining non-etcd path into the system for any caller,
production or test. Direct-API test callers construct real CRDs via the Kubernetes client instead, exercising the
same path production traffic uses.

### 7. Workflow deletion has no blast radius on in-flight or historical remediations

Deleting a `RemediationWorkflow` CRD removes it from etcd; DS's cache (Change 1) naturally stops seeing it — this is
correct behavior, not a data-loss regression. Historical/audit reconstruction (SOC2 CC8.1) never depends on DS's
live cache retaining a "disabled" row; it is fully satisfied by the enriched `audit_events` snapshot (see Change 9
below). There is no soft-delete state at the CRD level. AW's only remaining responsibility on DELETE is emitting the
audit event recording who deleted the workflow and when — the deletion itself always succeeds against etcd,
regardless of DS's state (preserving GitOps drift-safety from ADR-058).

### 8. In-flight executions resolve their metadata from the CRD chain, not a live catalog lookup

A `WorkflowExecution` created against a `workflow_id` that is later superseded or deleted must still be able to
resolve its own execution-time metadata (`executionEngine`, `serviceAccountName`, dependencies, declared parameter
names) — this can happen arbitrarily later than selection, across the `RemediationApprovalRequest` human-approval
gate. The chosen design is the standard Kubernetes pattern: **capture everything the executor needs directly in the
CRD chain at selection time**, rather than re-resolving it via a live lookup at execution time.

- KA (already independently fetching and parsing the workflow schema for its own parameter validation, see
  [DD-KA-001](./DD-KA-001-workflow-response-validation-architecture.md)) surfaces the schema-derived fields it
  already computes — `Dependencies`, `Resources`, declared parameter names — into
  `AIAnalysis.Status.SelectedWorkflow`. This field is made write-once immutable via a CEL `XValidation` rule, so it
  cannot be tampered with after KA's validation populates it.
- `RemediationOrchestrator` copies the full snapshot into `WorkflowExecution.Spec.WorkflowRef` at WFE-creation time,
  including `ExecutionEngine`/`ServiceAccountName` — reversing issues #518/#650's exclusion of those fields from the
  WFE spec. Their original "no silent default" property is preserved (and arguably strengthened): RO's existing
  `validateSelectedWorkflow` fail-closed check now runs at CRD-creation time instead of at WE-runtime, meaning a
  missing `ExecutionEngine` fails before any child CRD is even created.
- WE's `WorkflowQuerier`/DS dependency is removed entirely. WE executes purely from its own `Spec.WorkflowRef` —
  **zero live calls to DS, the audit trail, or even etcd** at execution time.
- A **rejected alternative** was a DS-side fallback that would query `audit_events` on a cache miss. This was
  rejected because it reintroduces a PostgreSQL dependency on WE's hot execution path and adds a second query path
  for the same lookup, working against this decision's own goal of removing PostgreSQL from every read path in the
  workflow domain.

### 9. WE's independent parameter-validation re-check is retired (one validation layer, not two)

WE previously performed its own independent re-fetch of the workflow's declared-parameter schema from DS
specifically so it would not have to trust KA's self-reported validation (Issue #243, defense-in-depth). Now that
`DeclaredParameterNames` travels with the CRD-embedded snapshot from the *same* KA validation pass (Change 8), a
second independent DS fetch would validate against the identical data KA already validated — providing no additional
security. This is a deliberate, informed trade-off, documented in
[DD-KA-001](./DD-KA-001-workflow-response-validation-architecture.md)'s changelog, not an oversight.

### 10. CRD schema format hardening

Pre-existing format-validation gaps are closed with `kubebuilder:validation:Pattern` constraints:

| Field | Constraint | Rationale |
|---|---|---|
| `RemediationWorkflow.spec.version` | Semver (`^[0-9]+\.[0-9]+\.[0-9]+...$`) | Version comparison/supersession logic assumes semver |
| `RemediationWorkflow.spec.actionType` | PascalCase (`^[A-Z][A-Za-z0-9]*$`) | Must match `ActionType.spec.name`'s own contract |
| `RemediationWorkflow.spec.maintainers[].email` | Permissive email shape | Contact-hint field, not an auth credential |
| `ActionType.spec.name` | PascalCase (`^[A-Z][A-Za-z0-9]*$`) | DD-WORKFLOW-016 taxonomy naming contract |

These are enforced by the Kubernetes API server itself, before any admission webhook is invoked — a stronger
guarantee than the equivalent validation previously living only in DS's request-body parsing.

### 11. Audit events become the append-only historical ledger

`remediationworkflow.admitted.create`/`.update` (and, best-effort, `.denied` when the CRD unmarshaled successfully)
are enriched with a fully structured `workflow_content` payload (mirroring `RemediationWorkflowSpec` field-for-field)
plus `content_hash`. `workflowexecution.execution.started`/`.completed`/`.failed` are enriched with `ActionType` and
`WorkflowName`. No new PostgreSQL ledger table is introduced — `audit_events` (ADR-034), which already exists for
exactly this purpose, becomes sufficient to reconstruct the exact workflow definition that was ever admitted,
independent of etcd or DS's cache (SOC2 CC8.1 full reconstruction), and to correlate execution events back to their
originating admission without an additional catalog lookup.

---

## Architecture

### Before (DD-WORKFLOW-009 / DD-WORKFLOW-017 v1.2)

```
┌──────────┐  kubectl apply   ┌─────────────┐   HTTP (sync)   ┌──────────────┐
│ Operator │ ───────────────▶ │ AuthWebhook │ ───────────────▶│ Data Storage │
└──────────┘                  └─────────────┘                 └──────┬───────┘
                                     ▲                                │ SQL
                                     │ .status patch                  ▼
                                     │                         ┌──────────────┐
                                     └─────────────────────────│  PostgreSQL  │◀── discovery, execution
                                          (async goroutine)     │ (catalog +   │    metadata, scoring
                                                                 │  pgvector)   │    all read from here
                                                                 └──────────────┘
        etcd: CRD spec + status              PostgreSQL: catalog row (2nd copy, independently mutable)
```

### After (this decision)

```
┌──────────┐  kubectl apply   ┌─────────────┐   .status patch (local)
│ Operator │ ───────────────▶ │ AuthWebhook │ ───────┐
└──────────┘                  └──────┬──────┘        │
                                      │ List/Watch    ▼
                                      │ (etcd cache)  etcd (RemediationWorkflow/ActionType CRDs)
                                      │                     ▲
                                      ▼                     │ List + Watch (informer)
                              local validation:      ┌──────┴───────┐
                              - ActionType exists     │ Data Storage │──▶ in-memory cache
                              - content-hash/ID        └──────┬───────┘    (discovery, scoring,
                              - content-integrity              │            execution metadata)
                                                                │ audit events only
                                                                ▼
                                                        ┌──────────────┐
                                                        │  PostgreSQL  │  audit_events (ADR-034):
                                                        │ (audit-only) │  append-only ledger +
                                                        └──────────────┘  on-demand aggregates
```

No synchronous HTTP call exists between AuthWebhook and Data Storage on the CRD-admission path. Data Storage's
in-memory cache and AuthWebhook's admission decisions are both independently derived from etcd — never from each
other.

---

## Component Impact Summary

| Component | Before | After |
|---|---|---|
| **AuthWebhook** | Calls DS synchronously (`CreateWorkflowInline`/`DisableWorkflow`) for every CRD admission; relays DS's response into `.status` | Computes content hash/workflow ID locally; validates ActionType existence and content-integrity against its own etcd-backed client; patches `.status` directly; zero DS calls |
| **Data Storage** | Owns `remediation_workflow_catalog`/`action_type_taxonomy` as mutable PostgreSQL tables; exposes mutation REST endpoints | Read-only informer cache over etcd CRDs; mutation REST endpoints removed; discovery/scoring reimplemented as in-memory Go; success-rate metrics computed on demand from `audit_events` |
| **Workflow Execution** | Resolves `executionEngine`/`serviceAccountName`/dependencies/parameter schema via a live `WorkflowQuerier` call to DS at *execution* time | Reads the full execution snapshot from its own `Spec.WorkflowRef`, populated once at WFE-creation time; zero live calls to DS, audit, or etcd |
| **AIAnalysis** | `Status.SelectedWorkflow` carries `WorkflowID`/parameters only | `Status.SelectedWorkflow` also carries `Dependencies`/`Resources`/`DeclaredParameterNames`, write-once immutable via CEL |
| **Remediation Orchestrator** | Creates WFE without `ExecutionEngine`/`ServiceAccountName` (Issues #518/#650) | Copies the full snapshot from `AIAnalysis.Status.SelectedWorkflow` into `WorkflowExecution.Spec.WorkflowRef`; fails closed if `ExecutionEngine` is missing |
| **KubernautAgent (KA)** | Validates parameters against its own fetched schema, discards `Dependencies`/`Resources`/declared-names after validation | Surfaces those already-computed fields into its response so AA can persist them; remains sole parameter-validation authority ([DD-KA-001](./DD-KA-001-workflow-response-validation-architecture.md)) |

---

## Deletion Semantics

Deleting a `RemediationWorkflow` CRD is a **true deletion** from etcd — there is no soft-delete/disabled state
retained at the CRD or cache level. This is a deliberate decision, not an oversight:

- Any remediation that already selected the deleted workflow retains full execution capability, because its
  `WorkflowExecution.Spec.WorkflowRef` already carries the complete execution snapshot (Change 8) — it never needs
  to re-resolve the now-deleted CRD.
- Full auditability (SOC2 CC8.1/CC7.2) is preserved because the `remediationworkflow.admitted.create`/`.update`
  audit events already captured the complete `workflow_content` at every point the workflow was ever admitted
  (Change 11) — "what did workflow X look like when remediation Y used it?" is answered from `audit_events`, not
  from a live CRD or cache lookup.
- AuthWebhook's only obligation on DELETE is to emit an audit event recording who deleted the workflow and when.

---

## Migration / Upgrade Considerations

- **Pre-existing workflows lack enriched historical audit content.** Workflows registered before this migration
  ships will not have a `workflow_content`-enriched admission event in their history — only new CREATE/UPDATE/DENIED
  events after rollout carry it. This is flagged as a **documentation task for the `kubernaut-docs` 1.6 upgrade
  guide**, alongside the existing fleet `cluster: ["*"]` label-backfill guidance tracked in
  [Issue #204](https://github.com/jordigilh/kubernaut/issues/204) — both are "what changes for existing workflows
  when you upgrade to 1.6" concerns and belong in the same upgrade-guide section. No reconciler or migration code
  backfills historical audit content; this is an explicit, documented scope decision, not a gap.
- **No database migration deletes `remediation_workflow_catalog`/`action_type_taxonomy` until every caller — direct
  API test callers included — has migrated to the CRD-based path.** The removal (Change 6) is the last phase of the
  rollout, gated on a zero-remaining-caller verification (`gopls`/`grep`), never executed speculatively ahead of
  that verification.

---

## Cross-References

This decision does **not** supersede or duplicate:

| Document | Remains Authoritative For |
|---|---|
| [DD-WORKFLOW-012](./DD-WORKFLOW-012-workflow-immutability-constraints.md) | Immutability constraints (mutable vs. immutable fields) |
| [DD-WORKFLOW-016](./DD-WORKFLOW-016-action-type-workflow-indexing.md) | Action-type indexing, three-step discovery protocol, filter semantics |
| [ADR-058](./ADR-058-webhook-driven-workflow-registration.md) | Webhook-driven registration architecture (ValidatingWebhook, async status pattern) |
| [ADR-034](./ADR-034-unified-audit-table-design.md) | Unified audit table design (event-sourcing, hash-chain integrity, retention) |
| [DD-AUDIT-003](./DD-AUDIT-003-service-audit-trace-requirements.md) | Per-service audit trace requirements |
| [DD-AUDIT-004](./DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md) | `workflow_content` field mapping and cross-event join query (Change 11) |
| [DD-KA-001](./DD-KA-001-workflow-response-validation-architecture.md) | Parameter validation architecture; sole-validator decision (Change 9) |

**Proposed follow-up** (not yet approved, gated on this decision's rollout completing):
[DD-WORKFLOW-019](./DD-WORKFLOW-019-ka-owned-workflow-discovery.md) captures a validated (but not-yet-implemented)
direction to further relocate the discovery/scoring logic and cache this decision's Change 1-2 place in DS into
KubernautAgent, once KA's status as the sole real consumer is reconfirmed post-rollout. Tracked in
[Issue #1677](https://github.com/jordigilh/kubernaut/issues/1677).

---

## FedRAMP / SOC2 Control Mapping

| Change | Control Objective | How It's Satisfied |
|---|---|---|
| 1 (informer cache) | CC7.2 (system operations) | DS availability no longer depends on PostgreSQL for reads; cache rebuilds deterministically from etcd |
| 5 (AW decoupling) | AC-4 (information flow), CC7.2 | Removes the exact async-divergence failure mode that motivated this decision |
| 7 (deletion semantics) | CC8.1 (audit completeness) | Historical reconstruction relies on `audit_events`, not live/cached state that a deletion could remove |
| 8 (CRD-embedded snapshot) | SI-10 (input validation), SI-7 (integrity) | RO's fail-closed check + CEL write-once immutability on `SelectedWorkflow` |
| 10 (schema hardening) | SI-10 (input validation) | API-server-level `Pattern` enforcement, before any webhook runs |
| 11 (audit enrichment) | AU-3 (content of audit records), CC8.1 | Full workflow content and cross-event identifiers captured in the immutable audit trail |

---

## Confidence Assessment

**Confidence: 95%** (see [Issue #1661](https://github.com/jordigilh/kubernaut/issues/1661) for the full preflight
evidence trail and phased TDD execution log).

**Justification**: every mechanism this decision depends on was verified against actual source before being adopted
— the deterministic UUID/content-hash functions already exist and are pure; the discovery/search logic was confirmed
label-based with no `pgvector` usage; `UpdateSuccessMetrics` was confirmed to have zero production callers before
being marked for removal. The primary residual risk is migration sequencing (ensuring zero remaining direct-API
callers before the PostgreSQL catalog tables are actually dropped) — mitigated by gating that removal on an explicit
verification step rather than a fixed phase count.

**Next Review**: after Issue #1661's full phased rollout completes and the PostgreSQL catalog tables are dropped.
