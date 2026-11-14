# Test Plan: FMC Resource Presence and TTL Recovery

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2593-v0.9
**Feature**: Fail-closed FMC resource presence, typed scope states, and Valkey recovery
**Version**: 0.9
**Created**: 2026-11-13
**Author**: Kubernaut Team
**Status**: Fast-path implementation baseline — Step 0 complete; RED pending
**Branch**: `work/2593-fmc-presence`

> This is the authoritative scenario and evidence-boundary artifact for the bounded
> #2593 implementation. The user-approved clean-worktree fast path authorizes RED and
> subsequent implementation only in `/private/var/folders/r7/gktmmltd1zq7wqhsjjslwsm80000gn/T/opencode/2593-fmc-implementation`;
> it authorizes no public v2 route/media, cluster activity, deployment, release, GitHub
> update, ACM Search qualification, or ACM Console inventory. The original dirty
> worktree and all held paths remain preserved. Functional scenarios, denominators,
> wiring rows, and public v1 boundaries are unchanged.

### Fast-path execution boundary (2026-11-13)

The historical review-only Step 0/three-clean-review sequencing below is superseded
for this clean worktree by the bounded plan's approved fast path. The carry/hold
classification is recorded in `evidence/FAST_PATH_CARRY_HOLD.md`; the original draft
worktree remains untouched. This is not a new functional plan or a public API approval:
RED starts from the carried foundation, GREEN still requires production wiring and
passing RED tests, and final qualification/evidence gates remain pending. Expected
implementation changes are reviewed with immutable RED/GREEN/REFACTOR pass captures;
no second pre-RED review cycle is introduced.

### Amendment note: user-approved pre-GA v1/no-migration and internal repair/Lease decision

> **v0.9 amendment / user-approved pre-GA v1/no-migration and internal repair/Lease decision (2026-11-13):** FMC v1
> has not reached GA. #2593 is an atomic pre-release replacement in the unreleased source
> tree, not a production migration. The public FMC contract remains the ADR-068 v1 route
> `GET /api/v1/scope/check`, ordinary `application/json`, and `ScopeCheckResponse.Managed`
> (`managed` on the wire). This plan defines no public v2 route or media token. Any typed
> unavailable/verified v1 response extension requires prior BR/ADR/API-owner approval; this
> plan does not invent field names. Public migration inventory, rollout/cutover,
> mixed-version, old-client, rollback-compatibility, data-backfill, and migration-evidence
> requirements are out of scope and receive no credit. R13 and v0.8 remain historical
> context; R13 is immutable and this amendment does not edit it.
>
> The internal repair/Lease contract in §5.4.3 is the user-approved replacement authority:
> `PartitionStateKey`, tokenized `BeginRepair`/`Repair`/`CommitRepair`, proof-bearing
> `ReadVerified`, and epoch-scoped `LeaseSession`. The superseded internal
> `StatusStore`/`LeaseCoordinator` signatures have no compatibility shim. This is an
> internal pre-release contract change, not a public FMC version change; current source
> remains a source gap until implemented and proven.
>
> The preserved original draft `pkg/fleet/fmc` contains v2 route/media symbols that do not
> establish the approved public contract. This held v2 source/test inventory is the RED
> input/acceptance list, not a pre-RED cleanup task. RED defines and tests the approved v1
> conversion; GREEN changes the unreleased source and all listed consumers atomically before any
> production-composition or wiring credit; REFACTOR cleans up the converted path. The
> fast-path boundary replaces the historical pre-RED plan-approval ceremony for this
> clean worktree; it does not accept the v2 symbols. This metadata amendment makes no
> public-contract change. RED, GREEN, and REFACTOR each require a separate immutable
> source-diff capture and review.

> **R12 amendment boundary retained:** invalid-label unavailable/non-inheriting behavior,
> separation of the public six-field canonical ID from internal registration fencing,
> expected registry revision in `WithCurrent`, and the `0/2` conditional-server subset of
> the aggregate `0/3` external gate remain plan requirements only. They add no production
> implementation, route, cluster, ACM, GitHub, or release evidence. The earlier R9–R11
> public migration wording is superseded historical context; the typed-scope, internal
> cache, denominator, and external-registration requirements are restated below.

---

## 1. Introduction

### 1.1 Purpose

Define the requirements-based unit, integration, and conditional control evidence
for FMC-backed resource scope. The test effort must prove that only a verified, present
resource can inherit namespace scope; explicit resource false remains unmanaged; cache,
source, lease, identity, GVK, or authorization uncertainty fails closed; and a safe
Valkey recovery cannot serve stale positive authorization or false-complete inventory.

This plan does **not** qualify ACM inventory. Direct ACM Gateway/RO scope remains a
separate regression path, and non-local API Frontend Console list/policy-scope reads
remain unavailable under DD-AF-018/#2584/#2600.

### 1.2 Objectives

1. Prove presence-before-inheritance and explicit-false precedence through FMC, Gateway,
   Remediation Orchestrator, and the approved AF tool scope path.
2. Prove that incomplete, missing, expired, uncertain, unsupported, invalid, or
   unauthorized state cannot become verified unmanaged, namespace fallback, an allow,
   an RR/WFE, or an inventory row.
3. Prove the internal Valkey record/status protocol, fenced single-writer lifecycle,
   authoritative registration incarnation, server-time TTL rules, and repair behavior.
4. Prove explicit GVK propagation and validated namespace/cluster-scope behavior across
   SignalProcessing, Gateway, RO, AF, ACM, local scope, and FMC identity paths.
5. Preserve the approved AF/ACM boundaries and provide reproducible evidence for every
   implementation wiring row before any GREEN decision.

### 1.3 Success metrics

| Metric | Target | Measurement |
|---|---:|---|
| UT business scenarios | 12/12 | Ginkgo/Gomega behavior scenarios; every UT scenario is critical |
| IT-owned business scenarios | 4/4 | Production-path source wiring evidence; no deployment qualification claim |
| Overall business scenarios | 16/16 and 15/15 critical | Requirements-based matrix; no duplicate-tier inflation |
| FMC implementation wiring | 19/19 | 14 existing production rows + 4 typed-scope seams + 1 AIAnalysis/GVK handoff |
| Conditional AF inventory/server rows | 0/2 separate external gates | Exactly `IT-AF-CACHE-006` + `IT-FMC-INVENTORY-012`; a subset of aggregate AF/FMC/Page `0/3`, whose third row is `IT-AF-PAGE-2584-012`; no source-slice or qualification credit |
| Authentication E2E control gate | 0/1 separate future gate | Real Kind TokenReview/SAR path; execution requires explicit authorization; no source-slice credit |
| RO audit reconstruction gate | 0/1 separate future gate | Persisted `RoutingBlockedData`, complete correlation reconstruction and owner/reviewer evidence; no source-slice credit |
| External AF/FMC/Page gates | 0/3 separate future gates | `IT-AF-CACHE-006`, `IT-FMC-INVENTORY-012`, and `IT-AF-PAGE-2584-012`; separate ownership and no FMC inventory credit |
| Applicable control objectives | Source-slice mappings complete; external controls pending | BR, FedRAMP AC/SC/SI mappings and separately owned audit/control evidence; no qualification claim |
| Fail-open scope outcomes | 0 | No unavailable/uncertain/error state produces an allow or verified false |
| Sensitive inventory leakage | 0 | Negative response and log assertions |

Raw line coverage is diagnostic. It cannot replace the scenario, wiring, control, or
review gates above.

---

## 2. References

### 2.1 Authority

- Issue [#2593](https://github.com/jordigilh/kubernaut/issues/2593), FMC resource
  presence and TTL recovery prerequisite work.
- [BR-SCOPE-001](../../requirements/BR-SCOPE-001-resource-scope-management.md).
- [BR-SCOPE-002](../../requirements/BR-SCOPE-002-gateway-signal-filtering.md).
- [BR-SCOPE-010](../../requirements/BR-SCOPE-010-ro-routing-validation.md).
- [BR-SECURITY-016](../../requirements/BR-SECURITY-016-kubernetes-rbac-enforcement.md)
  and
  [BR-SECURITY-017](../../requirements/BR-SECURITY-017-serviceaccount-token-authentication.md)
  for authenticated and RBAC-authorized service boundaries.
- [DD-AF-018](../../architecture/decisions/DD-AF-018-console-policy-scope-read-projection.md).
- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md).
- [ADR-053](../../architecture/decisions/ADR-053-resource-scope-management.md).
- [ADR-068](../../architecture/decisions/ADR-068-fleet-federation-architecture.md).
- [DD-FLEET-011](../../architecture/decisions/DD-FLEET-011-shared-target-read-policy.md),
  preserved and not modified by this implementation slice.
- [DD-AUTH-014](../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md)
  for the required 401/403/500 middleware boundary.
- [OWASP ASVS v5.0.0](https://github.com/OWASP/ASVS/tree/180622b1329ee7ed349c9f5ce5e14fa91f42c359/5.0/en);
  mapped controls use the pinned commit and exact chapter links in Section 7.
- `docs/testing/TEST_PLAN_TEMPLATE.md`.

### 2.2 Deferred approval and final-evidence documents

The following remain acceptance, final-evidence, or separately owned gates. The
user-approved clean-worktree fast path permits bounded RED from the recorded carry
boundary; it does not waive these gates for GREEN completion, qualification, or release:

- The [v0.9 amendment note](#amendment-note-user-approved-pre-ga-v1no-migration-and-internal-repairlease-decision)
  records the user-approved pre-GA v1/no-migration decision; the bounded OpenChamber plan
  named in Section 12 is the only active implementation-plan authority for this pre-release
  v1 slice. Its exact plan SHA-256 is captured externally in the Step 0 approved-inputs
  record/capture envelope and is not asserted in this mutable test plan.
- BR-SCOPE-001/002/010 and ADR-053 amendments for verified observation, typed
  unavailable state, explicit remote GVK, invalid-label behavior, and deferred
  cluster-scoped behavior.
- ADR-068/DD-FLEET-011 reconciliation: amend only the FMC observation/cache/scope
  clauses while preserving gateway-routed access, registration-name routing, and the
  accepted shared target-read/RBAC policy. No DD-FLEET-011 file is changed here.
- The user-approved internal Valkey record/status, `PartitionStateKey`, tokenized repair,
  proof-bearing fenced-read, full-fence LeaseSession, and epoch-scoped loss/rearm
  contract in §5.4.3. External design-decision recording and final evidence ownership
  remain pending.
- An explicit BR/ADR/API-owner disposition for any typed unavailable/verified extension of
  the public v1 scope response. Until approved, the only asserted public response member is
  `ScopeCheckResponse.Managed` / `managed` with ordinary `application/json`.
- The versioned target-kind/RBAC matrix and rendered deployment evidence.
- A scoped real-Kind E2E authorization run for `E2E-FMC-AUTH-018`, or an explicit
  approved risk acceptance that defers BR-SECURITY-016/017 E2E evidence.
- An approved #2584/#2600 external gate amendment naming `IT-AF-CACHE-006`,
  `IT-FMC-INVENTORY-012`, and `IT-AF-PAGE-2584-012` with the exact files, commands,
  owner, independent reviewer, wire artifacts, and result record specified in Section 9.2.

---

## 3. Risks and mitigations

| ID | Risk | Impact | Priority | Probability | Tests / mitigation |
|---|---|---|---|---|---|
| R1 | Cache miss, expiry, lost write, or stale index is treated as unmanaged or allowed | Unauthorized remediation or false inventory | P0 | High | UT-FMC-PRES-005/006, IT-FMC-CACHE-013/015; typed state and no-fallback assertions |
| R2 | Two sync writers or a stale writer clears a newer uncertainty fence | Stale positive authorization | P0 | High | IT-FMC-CACHE-004/005; Lease, generation, fence, takeover, and stale-writer tests |
| R3 | Deleted/recreated registration reuses a name-only cache partition | Cross-incarnation data leakage | P0 | High | IT-FMC-CACHE-004; registration UID propagation and same-ID recreation |
| R4 | GVK is dropped or inferred as core `v1` | Wrong resource or cross-kind identity decision | P0 | High | UT-FMC-PRES-008/UT-FMC-CACHE-014, IT-FMC-PRES-010; end-to-end handoff and collision tests |
| R5 | The public v1 route or response is changed without the required pre-GA approval | Client contract drift or unavailable state becomes an authorization decision | P0 | High | `IT-FMC-PRES-003`; public-contract approval gate; internal typed-state fail-closed assertions |
| R6 | Backend identity is confused with interactive-user authorization | Cross-user or cross-scope disclosure | P0 | Medium | IT-FMC-AUTH-008/011; TokenReview/SAR and no-browser-token tests |
| R7 | Incomplete inventory is serialized as empty/complete | False operational state or data omission | P1 | Medium | IT-FMC-CACHE-010/015 and external #2584 Page-contract gate |
| R8 | RO routing audit payload/schema cannot reconstruct blocked routing decisions | Incomplete audit trail and failed release/control evidence | P1 | Medium | External `IT-RO-AUDIT-2593-001`; RO OpenAPI/schema/codegen/producer ownership, persisted `RoutingBlockedData` and correlation reconstruction |

Every P0 risk requires at least one passing proving scenario at the appropriate tier;
logic risks require UT coverage and lifecycle/wiring/security risks require production
path IT coverage. A failed P0 scenario suspends the plan. A risk does not require an
artificial duplicate UT/IT case when the behavior is inherently deployment-only.

---

## 4. Scope

### 4.1 Features to be tested

- `pkg/fleet/fmc`: source collection, observed label state, sync error propagation,
  typed scope response, and no-row unavailable behavior.
- `pkg/fleet/scopecache`: internal record/index/status protocol, canonical identity, TTL,
  generation/fence, repair, and typed read results. The existing `fmc_v2_atomic.lua` name is
  an internal artifact name, not a public API version or media contract.
- `pkg/fleet/registry`: registration-object UID/incarnation extraction and event
  propagation for EAIGW and Kuadrant sources.
- `cmd/fleetmetadatacache`: Lease, status-store, readiness, repair, and production
  dependency wiring.
- `pkg/gateway`, `pkg/remediationorchestrator`, `pkg/apifrontend/tools`,
  `pkg/apifrontend/policyread`, `pkg/fleet/acm`, and `pkg/shared/scope`: typed state
  and explicit GVK propagation where the FMC scope contract is selected.
- `api/signalprocessing/v1alpha1`: explicit `APIVersion` schema update, generated CRD,
  and admission/serialization evidence.
- `pkg/fleet/inventory`: existing `Page` states, completeness, canonical ID, and exact
  GET invariants as a dependency gate; no new inventory vocabulary.

### 4.2 Features not tested or not authorized in this slice

- ACM Search inventory visibility, traversal, live source qualification, or Console
  inventory/display. It remains `unavailable` with no rows.
- AF non-local Console list/policy-scope route implementation or registration.
- Direct remote Kubernetes/MCP discovery for Console display.
- Policy editing, approval mutation, workflow execution, or new audit event types.
- Public v2 FMC routes or media, public API migration inventory, migration rollout or
  deployment cutover, mixed-version/old-client compatibility, rollback compatibility, data
  backfill, or a migration evidence packet. These are not applicable to this unreleased
  replacement and receive zero credit.
- A target-cluster UID claim when the gateway exposes only registration-object UID. If
  the deployment requires target UID rather than registration UID, the capability stays
  unavailable until that source is added and proven.
- Any live cluster, Valkey production, or GitHub issue action without separate explicit
  authorization.

### 4.3 Boundary decisions

| Decision | Selected contract | Approval state |
|---|---|---|
| Cache writer | Fenced single writer using a Kubernetes Lease and monotonic fence | Approved internal replacement contract; Step 0 external design record pending |
| Incarnation | Registered gateway object's `metadata.uid`; target UID is an additional field only if exposed | Approved internal replacement contract; external registry/architecture record pending |
| Expiry time | Valkey server `TIME`, `PEXPIREAT`, and `PTTL`; no application-clock authority | Approved internal replacement contract; Step 0 external design record pending |
| GVK | Explicit schema-propagated group/version; no remote `InferGVK` | Pending BR/ADR and schema approval |
| Invalid labels | Invalid resource/namespace label is typed invalid/unavailable; it is not absent and cannot inherit namespace scope | Pending ADR-053 amendment |
| Cluster-scoped target | No namespace inheritance; no `verified_managed` cluster-scoped remediation claim until FR-SCOPE-004 is separately approved | Pending BR-SCOPE-001 decision |
| Inventory vocabulary | Existing `pkg/fleet/inventory.Page` states plus `CompletenessComplete/Partial` | #2584/#2600 conditional; not implemented here |
| AF boundary | Local list/policy scope; existing exact/get; non-local Console list/policy scope unavailable | Approved by DD-AF-018 unless separately amended |
| Public FMC scope API | `GET /api/v1/scope/check`; ordinary `application/json`; `ScopeCheckResponse.Managed` / `managed` is the asserted response contract; no public v2 route or media token | ADR-068/current approved baseline; any typed unavailable/verified v1 extension requires BR/ADR/API-owner approval before RED |

---

## 5. Approach and pass/fail criteria

### 5.1 Test method

- Ginkgo/Gomega BDD for all new business behavior tests.
- Real internal business logic; fake only external Kubernetes, Search, Valkey, HTTP,
  and authorization dependencies where the test tier permits it.
- A controlled Valkey test dependency or approved fixture for TTL, scripts, races, restart,
  failover, ACL, and server-time behavior. No result from this source-only plan is a
  production, cluster, ACM, or Valkey qualification claim.
- `envtest` or an equivalent Kubernetes API harness for Lease, registration UID,
  TokenReview/SAR, CRD schema, and RBAC wiring.
- `httptest` plus the production router and middleware for FMC/GW/RO/AF HTTP paths.
- No `testing.T` business tests, `XIt`, `PIt`, or `Skip()` placeholders.

### 5.2 Pass criteria

PASS for this source-only amendment requires all of the following:

1. All 12 UT scenarios and all 4 in-scope IT-owned business scenarios pass.
2. All 19 in-scope implementation wiring rows pass through production dispatch.
3. Every in-scope P0 scenario passes; all 15 critical scenarios are proven.
4. The public contract remains the ADR-068 v1 route with ordinary `application/json` and
   `ScopeCheckResponse.Managed` / `managed`; no unapproved typed response extension or
   public v2 route/media is asserted.
5. Internal typed scope, MCP completeness, status/TTL/fence, Lease/readiness/repair,
   registry incarnation/revision, GVK, invalid-label, direct ACM and DD-AF-018 boundaries
   are proven through the applicable source and wiring tests.
6. The approved BR/ADR/API-owner package and source/wiring evidence are present, with the
   historical review state and per-pass immutable review captures recorded. Conditional E2E, external AF/FMC/Page, RO-audit, deployment,
   cluster, ACM, and Valkey qualification evidence remains separately owned and is not
   credited by this slice.
7. Required affected validation is run only after approval; this documentation review
   itself runs `git diff --check` and makes no build, test, deployment, or release claim.

FAIL includes any stale positive authorization, unavailable-to-unmanaged conversion,
namespace fallback after a miss/error, missing GVK, cross-incarnation record, route
boundary violation, sensitive-data leak, incomplete-page success, or unresolved P0/P1
review finding.

### 5.3 Suspension and resumption

Suspend RED/GREEN when the approved BR/ADR/design decision is absent, source or target
incarnation is unknown, the Valkey deployment matrix is incomplete, rendered identity or
RBAC is missing, the current origin baseline changes, or a shared infrastructure cause
produces cascading failures. Resume only after the evidence packet is refreshed and the
affected focused suite is rerun.

### 5.4 Requirements-to-test traceability policy

Every in-scope business or control requirement maps to a named scenario, production
wiring row, or conditional E2E/external gate in Sections 7–10. Each mapping records the
primary proof tier, exact test ID, file, command, owner, status and BR/control/ASVS
references. A scenario counts only when its assertions prove the stated business outcome;
execution without an outcome assertion does not count. A wiring row counts only when a
passing test traverses the production construction/router path. External AF/Page,
Kind-authentication, and RO-audit gates remain separate from the 16 business-scenario and
19 wiring denominators and cannot be silently counted as FMC GREEN or qualification
evidence.

Risk traceability is mandatory:

| Risk | Proving scenario or gate | Required tier |
|---|---|---|
| R1 cache/presence fail-open | `UT-FMC-PRES-005/006`, `IT-FMC-CACHE-010/013/015` | UT + IT |
| R2 stale writer/fence | `IT-FMC-CACHE-004/005` | Production IT |
| R3 registration recreation | `IT-FMC-CACHE-004`, `IT-FMC-PRES-009` | Production IT |
| R4 GVK/identity collision | `UT-FMC-PRES-008/UT-FMC-CACHE-014`, `IT-FMC-PRES-010`, `IT-FMC-GVK-022` | UT + production IT |
| R5 public v1 contract/typed-extension approval | `IT-FMC-PRES-003` and the public-contract approval gate | Source/wiring IT |
| R6 auth identity/RBAC | `IT-FMC-AUTH-008`, `E2E-FMC-AUTH-018` | Integration + E2E |
| R7 incomplete inventory/Page | `IT-FMC-CACHE-010/015`, `IT-AF-PAGE-2584-012` | Integration + external gate |
| R8 RO routing audit reconstruction | `IT-RO-AUDIT-2593-001` | External production audit gate |

### 5.4.1 R9 plan-executability amendments

The following five contracts close the R9 plan-level gaps without adding scenarios or
inflating the denominators.

**MCP continuation and typed-result bridge.** Every unfiltered `resources_list` call uses
the exact request semantics `apiVersion`, bounded `namespace`, `kind`, and `limit=50`; the
first request omits both `labelSelector` and `continue`, and each later request repeats the
same scope and sends exactly the previous non-empty continuation token in `continue`.
The reader allows at most 100 pages and 5,000 items per bounded source cycle. A repeated
token, a continuation token that is not a non-empty string, a page/item bound, a tool/error
response, missing or malformed `structuredContent`, or an unrecognized page makes the
observation incomplete. An empty `items` page with a continuation is not a complete empty
partition; only the final empty page with no continuation can establish verified emptiness.

`structuredContent` must be an object containing an `items` array. Each item must be a
Kubernetes object with matching `apiVersion` and `kind`, `metadata.name`, and the exact
qualified-GVK namespace rule. `metadata.labels` is optional: an absent field is a valid
present resource with `LabelState=absent`; a present empty/object map is valid and also
preserves `LabelState=absent` when the managed key is missing; valid string `true`/`false`
values produce the corresponding states. A null/non-object labels field or invalid managed
label is typed invalid/unavailable, while malformed metadata is incomplete. Table rows,
text-only output and mismatched GVK are rejected as incomplete; they are never normalized
into objects or treated as source absence. The typed bridge returns item label presence and
state, `Items`, `Complete`, `PageCount`, `ItemCount`, presence-aware continuation semantics
and a typed incomplete/backend failure reason. `ObservedReaderFactory` and `Syncer` must
receive that complete-scan state; a bare object list with nil error is not a successful
source observation. `SyncCluster` commits or refreshes TTL only after `Complete=true`,
scope/duplicate validation and verified writes. A complete empty scan is distinct from a
selector returning zero managed-label matches. These assertions are owned by
`IT-FMC-PRES-001`, `IT-FMC-CACHE-010`, and `IT-FMC-CACHE-015`.

**Durable Valkey fence and measurable expiry.** The exact writer identity is
`WriterFence{HolderPodUID, HolderSessionUID, LeaseEpoch, Fence}`, where `LeaseEpoch` is
the exact Kubernetes Lease `spec.leaseTransitions` value and `Fence` is exactly
`uint64(LeaseEpoch)`, not a second counter. The holder Pod UID and a new opaque
holder-session UID are both required; a renewal keeps them unchanged, while every
takeover increments `spec.leaseTransitions` under resource-version CAS and creates a new
session UID. Valkey status stores `holder_pod_uid`, `holder_session_uid`, `lease_epoch`
and `fence`; `lease_epoch` and `fence` are equal decimal encodings of that Lease value.
All status, record, index and commit guards compare registration UID, generation and all
four writer-fence values. A Valkey restart, data loss, missing status hash,
lower/mismatched fence, holder/session mismatch, or inability to advance the Lease leaves
the partition `repairing`/`uncertain` and unavailable. No writer may reuse a prior fence or
mark `clean` until the Lease-derived fence, registration UID, generation, status, records,
and index reconcile successfully.

The selected internal atomic protocol is one server-side `fmc_v2_atomic.lua` script invoked only
by `EVALSHA`; there is no `MULTI/EXEC`, client-side sequence, or alternate transaction
implementation. `SCRIPT LOAD` is bootstrap only. The script obtains Valkey `TIME`,
calculates `expires_at_ms=server_now_ms+ttl_ms`, sets `PEXPIREAT`, writes the record and
index score, checks registration UID/generation/full-fence guards, and returns
`server_now_ms`, `expires_at_ms`, and `pttl_ms` from the same operation. Its read, repair
and cleanup verbs obtain server time and PTTL together and require `pttl_ms > 0`,
`expires_at_ms > server_now_ms`, and
`abs((expires_at_ms-server_now_ms)-pttl_ms) <= 2ms`; any mismatch, missing TIME, negative
or overflow value, wrong type, or connection ambiguity is `uncertain` and cannot
authorize. A timeout, reset, or lost reply after EVALSHA is `ErrAmbiguousWrite`; it is
never blindly retried and always enters repair. `IT-FMC-CACHE-002`, `IT-FMC-CACHE-004`,
`IT-FMC-CACHE-005`, `IT-FMC-CACHE-013`, and `IT-FMC-CACHE-016` must assert this single
internal script contract in the approved test dependency. The `fmc_v2_atomic.lua` filename
does not create a public v2 API or media contract, and no deployment or Valkey
qualification claim is made by this plan.

**Public FMC v1 contract and approval gate.** The authoritative public scope route is
`GET /api/v1/scope/check` from ADR-068. Its ordinary `application/json` response is
represented by `ScopeCheckResponse.Managed` / the `managed` member. This amendment does
not require a public v2 route, a custom media token, a migration inventory, mixed-version
or old-client compatibility, rollback compatibility, cutover, backfill, or a migration
evidence packet. If the pre-GA implementation needs a typed unavailable/verified extension
on the v1 response, the BR/ADR/API owner must approve the exact schema before RED; this
plan names no additional public fields. `IT-FMC-MIGRATION-017` is retained below only as
an explicit N/A/out-of-scope identifier with zero credit; its prior v0.7 migration meaning
is historical and superseded.

**AF `policyread` typed-scope wiring.** `IT-AF-ROUTE-010` includes a dedicated production
consumer test for `pkg/apifrontend/policyread/service.go:333-347` in
`test/integration/apifrontend/fleet/af_policyread_typed_scope_test.go` (planned), in
addition to the existing route-boundary file. For a non-local exact inventory read,
`ScopeUnavailable`, `ScopeUnsupported`, `ScopeInvalid`, authorization/backend error, or a
non-verified result returns a state-only unavailable response with no resource; it must
not map to `NotFound`, `managed:false`, an allow, or a local-reader fallback. Only a
verified managed result proceeds, and only a verified unmanaged result may map to the
existing not-found behavior. Non-local list/policy-scope remains unavailable/no rows under
DD-AF-018. The test must traverse `Service.GetInventory` and the configured scope checker,
not call the helper in isolation.

**External gate registration and ownership.** `IT-AF-CACHE-006`,
`IT-FMC-INVENTORY-012`, and `IT-AF-PAGE-2584-012` remain separately owned external gates
at 0/3 for any future related claim; they are not source-slice or qualification credit.
This bounded source slice intentionally assigns no human/service identity; role-only
labels such as “AF owner” or “reviewer” are not execution evidence. Before any external gate
can be counted or any external test can run, the approved #2584/#2600 plan must register
each ID with: accountable owner identity, independent reviewer identity (different from the
owner), exact approved plan path and revision, exact test file and source revision,
command, artifact path, authorization reference, UTC start/end, exit status, redaction
status, expected/actual result, and current plan/test-plan hashes. Missing registration,
identity, reviewer independence, or wire artifact keeps the gate unavailable and preserves
AF non-local unavailable/no rows and FMC inventory no rows.

### 5.4.2 R10 plan-executability amendments

These four contracts supersede any less-specific R9 wording. They are documentation-only,
do not add scenarios or wiring rows, and do not authorize RED/GREEN, route registration,
cluster/E2E execution, ACM inventory, or release claims.

**Concrete MCP typed bridge.** The neutral observation contract is owned by
`pkg/fleet/observation`; it imports no `pkg/fleet/fmc` or general parent `pkg/fleet` package.
`pkg/fleet/mcpclient` owns MCP transport, structured-content parsing, pagination, and the
concrete implementation satisfying `observation.ObservedReader`. `pkg/fleet/fmc` owns the
FMC adapter, `ObservedReaderFactory`, and Syncer composition and consumes
`observation.ObservedReader`. The general `pkg/fleet.ReaderFactory` remains a separate GW/SP
adapter and is not accepted by FMC. The current source does not already satisfy these package
boundaries; they are implementation targets, not completion evidence.

The MCP success response is a `CallToolResult` with
`IsError=false` and `StructuredContent` decoded as the object
`{"items": []any, "continue": <absent|null|non-empty string>}`. `items` is present even
when empty. `continue` absent or JSON `null` means final page; a non-empty string is the
opaque token for the next request; a present empty string or any other type is malformed.
Text/table content is never a substitute for `structuredContent`. Every item must retain
matching `apiVersion`/`kind`, `metadata.name`, and the exact qualified-GVK namespace rule.
`metadata.labels` is optional: an absent field is a valid present resource with
`LabelState=absent`; a present empty/object map is valid and also preserves
`LabelState=absent` when the managed key is missing; valid string `true`/`false` values
produce the corresponding states. A null/non-object labels field is malformed/incomplete;
an invalid managed-label value is typed `LabelInvalid` and unavailable/non-inheriting, not
`LabelAbsent`, verified unmanaged or a namespace allow. Malformed metadata is incomplete.

The exact Go bridge for the FMC path is split by package:

```go
// pkg/fleet/observation owns all neutral request/result types and the interface.
type ListRequest struct {
    APIVersion    string
    Kind          string
    Namespace     string
    Limit         int
    Continue      *string // nil omits the request field; non-nil must be non-empty
    LabelSelector *string // nil for the FMC unfiltered observation
}
type ListObservation struct {
    Items        []ObservedItem
    Complete     bool
    PageCount    int
    ItemCount    int
    Continuation ContinuationField
    Failure      ObservationFailure
}
type ObservationFailure string
const (
    FailureNone             ObservationFailure = ""
    FailureTool             ObservationFailure = "tool_error"
    FailureTransport        ObservationFailure = "transport_error"
    FailureMalformedContent ObservationFailure = "malformed_content"
    FailureContinuation     ObservationFailure = "invalid_continuation"
    FailureBounds           ObservationFailure = "bounds_exceeded"
    FailureItem             ObservationFailure = "invalid_item"
    FailureScope            ObservationFailure = "invalid_scope"
    FailureDuplicate        ObservationFailure = "duplicate_identity"
    FailureConflict         ObservationFailure = "conflicting_identity"
)
type ContinuationField struct {
    Present bool
    Null    bool
    Value   string
}
type ObservedItem struct {
    Object        unstructured.Unstructured
    LabelsPresent bool
    LabelState    scope.LabelState // true, false, absent or invalid
}
type ObservedReader interface {
    ListObserved(context.Context, ListRequest) (ListObservation, error)
}

// pkg/fleet/mcpclient owns the concrete transport/parser implementation.
var _ observation.ObservedReader = (*Client)(nil)
func (c *Client) ListObserved(context.Context, observation.ListRequest) (observation.ListObservation, error)
func (c *Client) listResources(context.Context, observation.ListRequest) (observation.ListObservation, error)

// pkg/fleet/fmc owns the FMC adapter/factory and Syncer composition.
type ObservedReaderFactory interface {
    ReaderFor(context.Context, registry.ClusterInfo) (observation.ObservedReader, error)
}
func (s *Syncer) SyncCluster(context.Context, registry.ClusterInfo) error
func (s *Syncer) syncKind(context.Context, registry.ClusterInfo, string) (observation.ListObservation, error)
```

The existing controller-runtime compatibility method
`Client.List(context.Context, client.ObjectList, ...client.ListOption) error` may remain
for non-FMC callers, but it must delegate to the typed observation and return an error for
any incomplete result; it is not the FMC completeness boundary. `listResources` sends the
bounded requests and returns `Complete=false`, a typed `ObservationFailure`, and a
non-nil error for tool errors, malformed content, invalid/repeated continuation, page or
item bounds, scope validation, duplicate/conflicting canonical items, or transport failure.
It returns `Complete=true` and nil error only after the final page and every item validate.
`Continuation` preserves whether the final response omitted the field or returned JSON
null; either is a final page, while a present empty string is malformed. Every valid source
item preserves whether the labels field was absent versus present as an empty map.
`fmc.ObservedReaderFactory` passes `registry.ClusterInfo`, including the current
`RegistrationUID`, and returns `observation.ObservedReader`; `Syncer` receives the typed observation directly; it never accepts
a bare `client.Reader` as proof of a complete source. No write, commit or TTL refresh
occurs unless `Complete=true`, scope/duplicate validation passes and every write is
verified. `IT-FMC-PRES-001`, `IT-FMC-CACHE-010` and `IT-FMC-CACHE-015` must exercise this
production bridge.

**Consistent Lease fence.** The exact shared value is:

```go
type WriterFence struct {
    HolderPodUID     string
    HolderSessionUID string
    LeaseEpoch       int64  // exact Lease.spec.leaseTransitions value
    Fence            uint64 // exact Valkey value; Fence == uint64(LeaseEpoch)
}
```

`LeaseEpoch` is the only monotonic source. `Fence` is its non-negative Valkey encoding,
not an independent counter. `spec.holderIdentity` is the holder Pod UID. Each successful
acquire creates a new opaque holder-session UID and increments `spec.leaseTransitions`
under resource-version CAS; renewal changes neither the epoch nor either identity. The
status hash stores `holder_pod_uid`, `holder_session_uid`, `lease_epoch` and `fence`, with
the two numeric fields equal. `CycleToken` carries the complete `WriterFence`, and every
status, record, index and commit guard compares registration UID, generation and all four
fence fields. A stale holder/session cannot write, clean, repair or clear state.

**One atomic expiry protocol.** All record/index/status/generation mutation and all
guarded read/repair/cleanup operations use the single server-side
`fmc_v2_atomic.lua` script by `EVALSHA`. `SCRIPT LOAD` is bootstrap only; `EVAL`,
`MULTI/EXEC`, pipelines and client-side command sequences are not allowed alternatives.
The approved operation verbs are `begin`, `put_observed`, `commit`, `abort`,
`mark_uncertain`, `read_verified`, `repair_reconcile`, `repair_list_members` and
`cleanup_member`. The script
checks registration UID, generation and the complete fence before touching co-located
keys. `put_observed` obtains `TIME`, computes `expires_at_ms=server_now_ms+ttl_ms`, calls
`PEXPIREAT`, writes the record and index score, and returns
`server_now_ms`, `expires_at_ms` and `pttl_ms` from the same script. Read/repair/cleanup
also obtain server time and PTTL together and require `pttl_ms > 0`,
`expires_at_ms > server_now_ms`, and
`abs((expires_at_ms-server_now_ms)-pttl_ms) <= 2ms`. A wrong type, failed TIME, overflow,
or timeout/connection reset/lost reply after EVALSHA produces `ErrAmbiguousWrite`, leaves
the partition `uncertain`, and is never blindly retried; repair is required.

**Deterministic repair reconciliation.** Authority is ordered as: (1) current Kubernetes
registry entry and Lease for registration incarnation and writer ownership; (2) a complete
unfiltered MCP observation for the live resource set and label states; (3) Valkey status,
index and records as derived materialized state only. Status never authorizes from records,
and an index never authorizes without its record/status predicates.

`Repair` executes exactly: (1) resolve the current registry UID, acquire/verify the
current `WriterFence`, and atomically set status `repairing` with a new generation; (2)
run every qualified kind through `ListObserved` and require complete results; (3) sort and
validate the complete source set by canonical resource ID and re-check registry UID and
full Lease fence; (4) use `repair_reconcile` EVALSHA for each source item to create or
replace the current-generation record, server-time TTL and index member; (5) enumerate
the prior index in lexical order and use guarded `cleanup_member` EVALSHA to remove
expired, malformed, wrong-incarnation, wrong-generation, wrong-fence or source-absent
pointers/records; (6) verify every expected record, index member, status field, TTL/PTTL
delta and observed count with `read_verified` EVALSHA; and (7) only after all predicates
pass, use `commit` EVALSHA to set `clean`, clear the repair reason and publish the current
generation/fence. Missing/inconsistent status, index or record is rebuilt from the source
or remains unavailable. An incomplete source, lost reply, Lease/registry change, wrong
type or failed predicate leaves `repairing`/`uncertain` and restarts under a new fence;
Valkey contents alone can never clear uncertainty.

### 5.4.3 R11 plan-executability amendments (v0.9 internal contract replacement)

These contracts supersede any conflicting R9/R10 wording. They are documentation-only,
do not add scenarios or wiring rows, and do not authorize RED/GREEN, route registration,
cluster/E2E execution, ACM inventory, or release claims.

**Presence-aware item schema and pre-write validation.** `metadata` and `metadata.name`
are required objects/strings. The `metadata.labels` field is optional, and its presence is
separate from the managed-label state: an absent field is valid with
`LabelsPresent=false` and `LabelState=absent`; a present JSON object, including `{}`, is
valid with `LabelsPresent=true` and `LabelState=absent` when the managed key is missing;
string values `"true"` and `"false"` produce the corresponding states. A null/non-object
labels field or a non-string/invalid managed-label value produces typed `LabelInvalid`,
which is retained as unavailable and cannot inherit namespace state; malformed metadata or
labels shape makes the complete source observation fail before any cache write. Empty and
absent label maps therefore both preserve a verified present resource without turning
absence of the managed key into source absence.

Before any `PutObserved`, the complete aggregate is validated against the qualified GVK
scope table. Namespaced kinds require a non-empty valid namespace; cluster-scoped kinds
require an empty namespace; an unknown/unsupported GVK is unavailable and is never
inferred. Each item receives the public `CanonicalResourceID` from the existing six-field
`(cluster_id, group, version, kind, namespace, name)` tuple. `registration_uid` is not a
public-ID field: it is carried separately in the internal partition key, status,
generation and full writer-fence guards. Items are sorted by public canonical ID and
checked for uniqueness within the current registration partition. Any duplicate public ID
is rejected even if the payloads match; a duplicate with different labels is additionally
a conflict. A scope violation, duplicate, conflict or invalid item fails the entire source
cycle, writes no record/index member and refreshes no TTL. This validation is part of
`IT-FMC-PRES-001`, `IT-FMC-CACHE-010` and `IT-FMC-CACHE-015`.

**Package-safe source and lifecycle composition.** Ownership is fixed so the typed bridge cannot introduce an import cycle:

```go
// pkg/fleet/observation: neutral contract; no import of fleet/fmc or parent fleet.
type ObservedReader interface {
    ListObserved(context.Context, ListRequest) (ListObservation, error)
}

// pkg/fleet/mcpclient: transport/parser/pagination and concrete implementation;
// imports observation, never fleet/fmc or the general fleet.ReaderFactory.
var _ observation.ObservedReader = (*Client)(nil)
func (c *Client) ListObserved(context.Context, observation.ListRequest) (observation.ListObservation, error)
func (c *Client) listResources(context.Context, observation.ListRequest) (observation.ListObservation, error)

// pkg/fleet/scopecache owns the internal partition, repair, read, and Lease
// contracts. The normal observation cycle is not a repair compatibility API.
type StatusStore interface {
    BeginRepair(context.Context, BeginRepairRequest) (RepairStartResult, error)
    MarkRepairUncertain(context.Context, RepairStartResult, RepairFailure) error
    Repair(context.Context, RepairRequest) (RepairResult, error)
    CommitRepair(context.Context, RepairCommitRequest) (RepairCommitResult, error)
    ReadVerified(context.Context, FencedReadRequest) (FencedReadResult, error)
}
type LeaseSession struct {
    Fence WriterFence
    Loss  <-chan struct{}
}
type LeaseCoordinator interface {
    Acquire(context.Context) (LeaseSession, error)
    Verify(context.Context, WriterFence) error
    Renew(context.Context, WriterFence) error
    Release(context.Context, WriterFence) error
}

// pkg/fleet/fmc owns the adapter, factory, and Syncer composition.
type ObservedReaderFactory interface {
    ReaderFor(context.Context, registry.ClusterInfo) (observation.ObservedReader, error)
}
type Dependencies struct {
	Registry    RegistryView
	Readers     ObservedReaderFactory
	StatusStore scopecache.StatusStore
	Lease       scopecache.LeaseCoordinator
	Readiness   *scopecache.FMCReadinessSnapshot
}
func NewSyncer(Dependencies, Config, logr.Logger, *Metrics) *Syncer
func (s *Syncer) syncKind(context.Context, registry.ClusterInfo, string) (observation.ListObservation, error)
```

The `fmc` factory may construct `mcpclient.Client`, but its public contract is the neutral
`observation.ObservedReader`; it must not reuse or embed the general `pkg/fleet.ReaderFactory`
used by GW/SP callers. The current source's package ownership and signatures remain source gaps.

Readiness ownership is exact and future-facing: `wireFMCDependencies` constructs exactly
one `*scopecache.FMCReadinessSnapshot`; `fmc.NewSyncer` receives that same `Readiness`
pointer that `NewClientWithStatusStore` and both FMC `/readyz` handlers receive.
`NewClientWithStatusStore` accepts only the replacement `StatusStore`, the exact
`registry.CurrentRegistrationGuard`, and that shared readiness snapshot; it must not accept
or reach through the legacy `CacheReader`/Valkey reader. The old reader constructor may
remain only for explicitly approved tests or non-FMC callers without implementation credit.
`Syncer`
publishes lifecycle state; `checkFencedStatus` consumes the proof/readiness view; and
`ReadyzHandler` consumes `ServiceReady`. No independent `ready.Load` or ping-only
readiness authority is permitted. A backend probe may publish `ValkeyReady`/global state
into the snapshot, but it is not a second readiness decision. The current source does
not yet define this field or shared flow; it remains a future implementation gap, not
completion evidence. After successful MCP and registry initial sync, an empty registry
may make `ServiceReady` true when global prerequisites and the configured process-wide
Lease policy are satisfied; zero partitions do not require a per-partition Lease, and an
uncompleted registry sync is not an empty registry.
Unknown targets/partitions remain typed unavailable and never become `managed:false`, a
namespace fallback, an allow, or an inventory row.

The request-bearing methods above are the exact approved internal Go contract. A
concrete implementation keeps `context.Context` as its cancellation parameter and
accepts exactly one request object for each repair/read operation; it must not add an
overload, adapter, or compatibility shim for the superseded `PartitionKey`,
mark-only repair, old `ReadVerified`, terminal `Lost`, or repair-through-cycle APIs.
The source currently has those superseded forms and is therefore a source gap, not
completion evidence.

The `wireFMCDependencies` production seam must construct the concrete Lease
coordinator from the rendered Kubernetes Lease and Pod UID, the status store from the
qualified Valkey client and script manager, an `ObservedReaderFactory` that receives
full `registry.ClusterInfo` including `RegistrationUID`, and pass the complete
`Dependencies` contract to `fmc.NewSyncer`, including the shared `Readiness` pointer.
The callback may create
`mcpclient.NewFromSessionProvider` with the
registration's cluster ID/tool prefix, but it must never return a bare `client.Reader`
or a local fallback for a fleet cluster. The existing `pkg/fleet.ReaderFactory` remains
a separate adapter for GW/SP callers and is not accepted by the FMC constructor.
`Syncer` acquires the Lease, begins repair before MCP collection, obtains one typed
complete observation per configured kind, validates the aggregate before writes,
propagates every source/write error to the typed repair failure path, and publishes
clean only through the current registry/revision and full-fence guard. These are the
existing `IT-FMC-CACHE-004`, `IT-FMC-CACHE-005`, `IT-FMC-PRES-001`, and wiring rows,
not new rows.

**Approved internal repair, fenced-read, and Lease contract.** The public six-field
canonical resource ID remains unchanged. Internal state uses a registration-scoped
partition key:

```go
type PartitionStateKey struct {
    ClusterID       string
    RegistrationUID string // registration UID/incarnation; never empty
}
type RepairToken struct { Value string }
type RepairObservation struct {
    Identity      ResourceIdentity
    APIVersion    string
    LabelsPresent bool
    LabelState    scope.LabelState
    TTL           time.Duration
}
type ValidatedObservationSet struct {
    Items     []RepairObservation
    Complete  bool
    PageCount int
    ItemCount int
}
type BeginRepairRequest struct {
    Partition               PartitionStateKey
    RegistrationUID         string
    ExpectedRegistryRevision uint64
    WriterFence             WriterFence
}
type RepairStartResult struct {
    Partition               PartitionStateKey
    RegistrationUID         string
    ExpectedRegistryRevision uint64
    WriterFence              WriterFence
    Generation               uint64
    Token                   RepairToken
}
type RepairRequest struct {
    Partition               PartitionStateKey
    RegistrationUID         string
    ExpectedRegistryRevision uint64
    WriterFence              WriterFence
    Generation               uint64
    Token                   RepairToken
    Observations             ValidatedObservationSet
}
type TTLPTTLValidation struct {
    ServerNowMillis int64
    ExpiresAtMillis int64
    PTTLMillis      int64
    DeltaMillis     int64
    Valid           bool
}
type RepairResult struct {
    Partition               PartitionStateKey
    RegistrationUID         string
    ExpectedRegistryRevision uint64
    ProvenRegistryRevision  uint64
    WriterFence              WriterFence
    ProvenGeneration        uint64
    ProvenWriterFence       WriterFence
    Token                   RepairToken
    ExpectedCount            int
    ObservedCount            int
    CleanupCount             int
    SourceComplete           bool
    RecordValid              bool
    IndexValid               bool
    StatusValid              bool
    RecordIndexChecksValid   bool
    RecordTTL                TTLPTTLValidation
    IndexTTL                 TTLPTTLValidation
    StatusTTL                TTLPTTLValidation
    CleanCommitEligible      bool
    Failure                  RepairFailure
}
type RepairCommitRequest struct {
    Partition               PartitionStateKey
    RegistrationUID         string
    ExpectedRegistryRevision uint64
    WriterFence              WriterFence
    Generation               uint64
    Token                   RepairToken
    ExpectedCount            int
    Proof                   RepairResult
}
type RepairCommitResult struct {
    Partition               PartitionStateKey
    RegistrationUID         string
    ExpectedRegistryRevision uint64
    ProvenRegistryRevision  uint64
    WriterFence              WriterFence
    ProvenGeneration         uint64
    ProvenWriterFence        WriterFence
    Token                   RepairToken
    Committed                bool
    CleanCommit              bool
    Failure                  RepairFailure
}
type FencedReadRequest struct {
    Partition               PartitionStateKey
    RegistrationUID         string
    ExpectedRegistryRevision uint64
    WriterFence              WriterFence
    Generation               uint64
    Identity                 ResourceIdentity
}
type FencedReadResult struct {
    Record                 ResourceRecord
    State                  ReadState
    ProvenRegistryRevision uint64
    ProvenGeneration       uint64
    ProvenWriterFence      WriterFence
    RecordValid             bool
    IndexValid              bool
    StatusValid             bool
    RecordTTL               TTLPTTLValidation
    IndexTTL                TTLPTTLValidation
    StatusTTL               TTLPTTLValidation
}
```

`BeginRepair(BeginRepairRequest)` is the only start contract: it atomically invokes
`begin`, transitions the registration-scoped partition to `repairing`, allocates a
strictly new generation and opaque token, persists the registration UID and every
`WriterFence` field, and returns `RepairStartResult` **before any MCP collection**.
`Repair(RepairRequest)` accepts only that token/generation/partition/registration UID,
the expected registry revision, the complete `WriterFence` (Pod UID, session UID,
Lease epoch, and Valkey fence), and a complete validated observation aggregate from
every configured kind. `CommitRepair(RepairCommitRequest)` is the only repair commit;
it is invoked inside `CurrentRegistrationGuard.WithCurrent` with the exact expected
revision, registration UID, generation, token, and full fence. `ReadVerified(FencedReadRequest)`
requires the same partition, registration UID, expected revision, generation, identity,
and full fence and returns proof of the revision/generation/fence used for the read.

Typed failures are mandatory and never collapse to managed-false or success:

```go
type RepairFailureCode string
const (
    RepairFailureNone             RepairFailureCode = ""
    RepairFailureInvalidRequest    RepairFailureCode = "invalid_request"
    RepairFailureSourceIncomplete  RepairFailureCode = "source_incomplete"
    RepairFailureBoundsExceeded    RepairFailureCode = "bounds_exceeded"
    RepairFailureLeaseLost         RepairFailureCode = "lease_lost"
    RepairFailureStaleRegistration RepairFailureCode = "stale_registration"
    RepairFailureStaleFence         RepairFailureCode = "stale_fence"
    RepairFailureGenerationConflict RepairFailureCode = "generation_conflict"
    RepairFailureScriptUnavailable  RepairFailureCode = "script_unavailable"
    RepairFailureAmbiguousWrite     RepairFailureCode = "ambiguous_write"
    RepairFailureCountMismatch      RepairFailureCode = "count_mismatch"
    RepairFailureRecordInvalid      RepairFailureCode = "record_invalid"
    RepairFailureIndexInvalid       RepairFailureCode = "index_invalid"
    RepairFailureStatusInvalid      RepairFailureCode = "status_invalid"
    RepairFailureTTLInvalid         RepairFailureCode = "ttl_invalid"
    RepairFailureCommitRejected     RepairFailureCode = "commit_rejected"
)
type RepairFailure struct {
    Code      RepairFailureCode
    Message   string
    Retryable bool
}
type RepairFailureError struct { Failure RepairFailure }
```

All repair and read mutations use the bounded `EVALSHA` verbs `begin`,
`repair_reconcile`, `repair_list_members`, `cleanup_member`, `read_verified`,
`commit`, and `mark_uncertain`; `SCRIPT LOAD` is bootstrap/recovery only. Every verb
checks the `PartitionStateKey` registration UID, generation, and all four
`WriterFence` values. `WithCurrent` additionally checks the exact registry revision.
No `EVAL`, `MULTI/EXEC`, pipeline, client-side mutation sequence, terminal loss
channel, old read signature, old repair signature, or repair-through-normal-cycle-commit
compatibility path is allowed.

The complete repair ordering is: (1) obtain the current registry snapshot and Lease
session/fence; (2) call `BeginRepair` and publish `repairing` before MCP; (3) collect
complete unfiltered observations for every qualified kind; (4) validate GVK, scope,
labels, duplicate identities, bounds, and counts; (5) recheck the same registration
UID/revision and full Lease fence; (6) call `Repair`, using `repair_reconcile` for
source items; (7) enumerate the prior index in bounded lexical batches and use guarded
`cleanup_member`; (8) run `read_verified` for every expected record, index member,
status field, count, and server-time TTL/PTTL predicate; (9) call `CommitRepair` only
inside `CurrentRegistrationGuard.WithCurrent`; and (10) publish clean only when the
commit result is `Committed && CleanCommit`. Any incomplete source, Lease/registry
change, lost EVALSHA reply, wrong type, count mismatch, invalid record/index/status,
or TTL/PTTL failure remains `repairing`/`uncertain` and never authorizes a read.

The current source remains non-compliant until these replacement APIs and callers are
implemented and proven. This documentation amendment makes no source completion
claim and does not authorize RED/GREEN, deployment, cluster, ACM, Valkey
  qualification, or release evidence.

**EVALSHA artifact, restart recovery and time sampling.** The script artifact is owned at
`pkg/fleet/scopecache/scripts/fmc_v2_atomic.lua`, embedded by
`pkg/fleet/scopecache/atomic_script.go`, and its SHA-1 is generated/checked by the same
package. `ScriptManager.Load` uses `SCRIPT LOAD` only during startup or recovery, verifies
the returned SHA-1 equals the embedded digest, and publishes readiness only after the
script is loaded for every Valkey node/slot used by the declared topology. Every data
operation then uses only `EVALSHA` with that fixed digest; the artifact path, digest and
topology are recorded in the Valkey evidence packet.

If `EVALSHA` returns the exact `NOSCRIPT` error, the server has definitively not executed
that invocation. A per-node single-flight recovery performs one `SCRIPT LOAD`, verifies the
digest, and retries the same operation once with the same cycle/fence token. A load error,
digest mismatch, second `NOSCRIPT` or topology ambiguity yields `ErrScriptUnavailable`,
marks the partition `uncertain`, and enters repair. A timeout, connection reset or lost
reply is not treated as `NOSCRIPT` and is never retried; it yields `ErrAmbiguousWrite` and
enters repair. Recovery never uses `EVAL`, `MULTI/EXEC`, pipelines or a client-side
mutation sequence.

`put_observed`, `read_verified`, `repair_reconcile` and `cleanup_member` each call Valkey
`TIME` once inside the script. `put_observed` calls `PEXPIREAT`, then samples `PTTL` on the
record key before returning `server_now_ms`, `expires_at_ms`, `pttl_ms` and the script
digest. Read/repair/cleanup sample `PTTL` and `expires_at_ms` in that same invocation;
no client-side wall-clock value is used for authorization. A sample is valid only when
`pttl_ms > 0`, `expires_at_ms > server_now_ms`, and
`abs((expires_at_ms-server_now_ms)-pttl_ms) <= 2ms`; `PTTL=-1`, `PTTL=-2`, a missing
expiry, overflow, wrong type or mismatch is unavailable. The returned sample is the
linearization evidence for that operation and is not reinterpreted after the script
returns. `IT-FMC-CACHE-002`, `IT-FMC-CACHE-013` and `IT-FMC-CACHE-016` must cover
startup/restart `NOSCRIPT`, digest mismatch, lost reply and these exact sampling rules.

**Bounded repair and registry linearization.** Repair limits are fixed at 100 source pages,
5,000 source items, 10,000 prior index members and 256 members per index batch. At the
start, it resolves the current `RegistrationUID`, acquires/verifies the `WriterFence`,
and atomically marks the partition `repairing` with a new generation. It obtains complete
unfiltered observations for every qualified kind, applies the scope/label/duplicate
validation above, and sorts the complete source set by canonical ID before any record
mutation. A source or index count above its bound fails closed without publishing `clean`.

The single script additionally exposes the approved read-only `repair_list_members` verb.
It returns at most one 256-member batch from a bounded index snapshot, with members sorted
by canonical lexical ID and a presence-aware cursor; a cursor that cannot advance or a
changed generation/fence is an error. Repair verifies the current registry snapshot and
Lease fence before each mutation batch, uses `repair_reconcile` for each source item,
enumerates the prior index through `repair_list_members`, and uses guarded
`cleanup_member` for stale/expired/malformed/wrong-incarnation/wrong-generation/
wrong-fence/source-absent members. It then runs `read_verified` for every expected record,
index pointer, status field, TTL/PTTL sample and observed count.

The final `commit` EVALSHA is invoked only inside
`registry.WithCurrent(ctx, clusterID, registrationUID, expectedRevision, callback)`, where
`expectedRevision` came from the snapshot used to start the repair. The callback first
  verifies the same full `WriterFence` and all read predicates before calling
  `StatusStore.CommitRepair(RepairCommitRequest)`. A UID or revision change cannot pass that guard; a Lease/fence
change, lost reply, incomplete source, wrong type or failed predicate leaves
`repairing`/`uncertain` and the next attempt starts with a new generation/fence. Partial
repair is never clean, and Valkey contents alone can never clear uncertainty. This is a
bounded per-partition algorithm rather than an unbounded `ZRANGE`/cleanup loop.

### 5.4.4 R12 plan-executability amendments

These four contracts supersede any conflicting R11 wording. They are documentation-only,
do not add scenarios or wiring rows, and do not authorize RED/GREEN, route registration,
cluster/E2E execution, ACM inventory, or release claims.

**Invalid labels are unavailable and never inherit.** A valid observed resource may have an
absent labels field or a present empty labels object; both yield `LabelAbsent` and preserve
verified resource presence. A managed-label value of the exact strings `"true"` or
`"false"` yields the corresponding state. A non-string, unsupported or otherwise invalid
managed-label value yields `LabelInvalid`, which is an unavailable scope result: it is not
converted to `LabelAbsent`, `LabelFalse`, verified unmanaged or a namespace-inheriting
state. An invalid namespace label has the same unavailable/non-inheriting behavior. A
null/non-object labels field or malformed metadata is a malformed source item and makes the
complete observation unavailable before any record/index write or TTL refresh. The RED
update must replace the existing `pkg/shared/scope/provenance_2584_test.go` expectation
that invalid `"enabled"` inherits namespace `true`; no implementation is authorized by
this text.

**Public canonical ID and internal incarnation identity are separate.** The public
`pkg/fleet/inventory.CanonicalResourceID` contract remains the stable six-field tuple
`(cluster_id, group, version, kind, namespace, name)` with the existing `r-` SHA-256
encoding. `registration_uid` is never included in or exposed as that public ID, and no new
public ID format is introduced by this slice. The registration UID remains mandatory internal
identity in `PartitionStateKey`, Valkey status/record guards, generation and `WriterFence`, and
the current registry binding; therefore the same public target name in a new registration
incarnation is isolated by a different internal partition/fence even though its public
canonical ID format stays stable. Duplicate public IDs are rejected within a complete
current-registration source aggregate before any write.

**Registry revision is part of the commit guard.** The exact guard signature is:

```go
type RegistrationSnapshot struct {
    Info     ClusterInfo
    Revision uint64
}
type CurrentRegistrationGuard interface {
    Snapshot(clusterID string) (RegistrationSnapshot, bool)
    WithCurrent(
        context.Context,
        clusterID string,
        registrationUID string,
        expectedRevision uint64,
        callback func(ClusterInfo) error,
    ) error
}
```

The repair/commit caller obtains the snapshot and passes its exact `Revision`. The guard
holds the registry update exclusion, verifies both registration UID and revision, then runs
the callback. Every published add/update/delete, including a same-UID endpoint or metadata
update, increments the revision; any UID or revision mismatch rejects the commit and leaves
the partition `repairing`/`uncertain`. Source collection remains outside this critical
section.

**Conditional denominator relationship.** The conditional server rows are exactly
`IT-AF-CACHE-006` and `IT-FMC-INVENTORY-012`, reported as `0/2` in this FMC slice. They are
a strict subset of the aggregate external AF/FMC/Page gate `0/3`, whose third row is
`IT-AF-PAGE-2584-012`. Thus `0/2` means neither conditional server row has passed and does
not replace, reduce or imply the aggregate `0/3`; the aggregate remains zero until all
three independently registered external gates pass. The two server rows remain outside the
19-row FMC implementation denominator, and the Page gate remains separately owned by
`#2584/#2600`.

### 5.5 Requirements-based coverage policy

The authoritative source-slice denominators are 12 UT scenarios, 4 IT-owned business
scenarios, 16 overall business scenarios, 19 implementation wiring rows, and 15 critical
business scenarios. The separate conditional authentication E2E gate remains 0/1, the
external AF/FMC/Page gates remain 0/3, and the external RO-audit gate remains 0/1; none is
source-slice or qualification credit. Required source results are 12/12 UT, 4/4 IT,
16/16 overall, and 19/19 implementation wiring. All 15 critical business scenarios and
every P0 risk must pass. Structural line/branch coverage is diagnostic and must not replace
requirements-based coverage. `IT-FMC-MIGRATION-017` is explicitly N/A/out-of-scope with
zero credit and is not part of any active denominator.

---

## 6. Test items and version identification

### 6.1 Unit-testable items

| Package/file | Behavior |
|---|---|
| `pkg/fleet/scopecache` | Identity validation, canonical ID/key vectors, typed states, TTL predicates, generation/fence transitions, cleanup guards |
| `pkg/fleet/fmc` | Label precedence, source-cycle classification, typed HTTP mapping, response validation |
| `pkg/fleet/observation` | Neutral request/result and `ObservedReader` contract for complete typed observations |
| `pkg/fleet/mcpclient` | Concrete MCP transport, structured-content parsing, pagination, label preservation and incomplete-scan propagation |
| `pkg/fleet/registry` | Registration UID extraction, update/delete/recreate events |
| `pkg/shared/scope` | GVK validation, namespaced/cluster-scoped derivation, typed result mapping |
| `pkg/fleet/inventory` | Page state/completeness invariants and canonical ID recomputation |

### 6.2 Integration-testable items

| Production path | Required proof |
|---|---|
| `cmd/fleetmetadatacache` wiring | Lease, status store, registry, Valkey, readiness, repair, and syncer lifecycle |
| `pkg/fleet/fmc/handler.go` | ADR-068 v1 scope HTTP, ordinary JSON, `ScopeCheckResponse.Managed`, auth middleware, and the approval gate for any typed response extension |
| `pkg/gateway` | Scope result propagation and no RR on unavailable |
| `pkg/remediationorchestrator` | Scope result propagation, block/retry distinction, direct ACM regression |
| `pkg/apifrontend/tools` | Explicit APIVersion before scope check; no unavailable-to-unmanaged conversion |
| `pkg/apifrontend/policyread` | `Service.GetInventory` consumes typed remote scope results; unavailable/unsupported/error is state-only with no `NotFound`, allow or local fallback; proof runs through `IT-AF-ROUTE-010` |
| SignalProcessing CRD path | Schema update and GVK survives serialization and downstream reads |
| `pkg/aianalysis/handlers` and policy/history readers | AIAnalysis target GVK survives analysis/history persistence; remote records without GVK are unavailable and never use `InferGVK` |
| Typed scope seams | `scope.Manager`, `fleet.FederatedScopeChecker`, `fleet/acm.Client`, and `fleet.NewScopeChecker` return and route typed results |
| Authentication E2E | Real Kind `TokenReview`/`SubjectAccessReview` through the production middleware path |

### 6.3 Baseline

| Item | Revision | Notes |
|---|---|---|
| Checkout | `7a4ed304580298802520e292cad0a7c4efccd17b` | Review-only working tree; preserve existing DD-FLEET-011 changes |
| `origin/main` | `18870e92d58b83c1c061841130bc73fde62abc70` | Observed 2026-11-11; baseline for the clean fast path |
| Plan revision | SHA-256 recorded in the external plan ledger | Recompute after every amendment |
| Test-plan artifact | `TP-2593-v0.9`, Fast-path implementation baseline | `docs/tests/2593/TEST_PLAN.md`; functional scenarios are frozen; pass captures record later source changes |
| Valkey fixtures | Not yet qualified | Do not merge `redis:7-alpine`, unpinned 8.1, pinned 8.0.2, or 9.1.1 evidence without a deployment contract |

---

## 7. Business requirement and control coverage

| Requirement/control | Primary proof | Secondary/wiring proof | Owner |
|---|---|---|---|
| BR-SCOPE-001 / ADR-053 | UT-FMC-PRES-001–008 | IT-FMC-PRES-001/003/009; IT-GW-PRES-004; IT-RO-PRES-005 | Scope/FMC owner |
| BR-SCOPE-002 | UT-FMC-PRES-001–006 | IT-GW-PRES-004 | Gateway owner |
| BR-SCOPE-010 | UT-FMC-PRES-005–008 | IT-RO-PRES-005 and direct ACM regression | RO owner |
| BR-SECURITY-016/017 | `UT-FMC-CACHE-014` covers canonical identity-tuple isolation only; no separate auth-state UT is counted; authentication proof is supplied only by `IT-FMC-AUTH-008`/`IT-FMC-AUTH-011` and authorized `E2E-FMC-AUTH-018` gates; this row remains Pending until those gates pass | `IT-FMC-AUTH-008`/`IT-FMC-AUTH-011` plus `E2E-FMC-AUTH-018`; 401/403/500, identity/header and security-event assertions through real Kind TokenReview/SAR | Security/operator owner |
| FedRAMP AC-4/AC-6 | UT state/identity cases | IT-FMC-CACHE-004/005, GW/RO/AF dispatch, rendered RBAC; ASVS V8.2/V8.3/V8.4 pinned chapter | Security/platform owner |
| FedRAMP SC-8 | HTTP/Valkey TLS/auth negative cases | IT-FMC-CACHE-003 and rendered transport config; ASVS V12.3.1/V12.3.2 pinned chapter | Platform owner |
| FedRAMP SI-10 | Canonical identity and ordinary `application/json` contract validation | IT-FMC-PRES-009/010; ASVS V2.2.1/V2.2.2 pinned chapter | Scope/API owner |
| DD-AUDIT-003 / AU-2/AU-3 / CC7.2 | No FMC audit claim | AF `IT-AF-2584-010`; RO `IT-RO-AUDIT-2593-001` executable external evidence with persisted routing fields and correlation reconstruction | AF/RO owners |
| SOC2 CC8.1 | This amendment's change-control, authorization, testing, and review record only; not an audit-event completeness claim | Review archive, approval disposition, and clean-review counter | #2593 coordinator |
| FedRAMP AU-9/AU-11 | Inherited or pending outside this FMC source-only slice; no audit-storage or retention claim | DataStorage/compliance evidence when separately authorized | DataStorage/compliance owner |
| Typed scope contract | `UT-FMC-PRES-005/006/008` and typed-state unit cases | `IT-SCOPE-TYPED-018`–`IT-SCOPE-TYPED-021`; local, FMC, ACM, unknown-cluster, unavailable and verified-unmanaged routing | Scope/API owner |
| AIAnalysis GVK handoff | `UT-FMC-PRES-008` identity validation | `IT-FMC-GVK-022`; analyzing handler, AI history, policy reader and remote `InferGVK` rejection | SignalProcessing/AI owner |
| External AF/FMC/Page contract | No FMC inventory credit | `IT-AF-CACHE-006`, `IT-FMC-INVENTORY-012`, and `IT-AF-PAGE-2584-012`; exact files, commands, wire JSON, owner and independent reviewer in Section 9.2 | #2584/#2600 AF/FMC owners |
| Authentication E2E control | No claim from unit/MockAuthorizer tests alone | `E2E-FMC-AUTH-018`; real Kind TokenReview/SAR, 401/403/500 and security-event proof | Security/operator owner |

FMC is a no-audit service under DD-AUDIT-003. The AF event strings are the persisted
`apifrontend.console.policy_scope_read`, `apifrontend.console.managed_resource_list_read`,
and `apifrontend.console.managed_resource_get_read` discriminators; the shorthand
`console.*` names are not sufficient evidence. The RO `orchestrator.routing.blocked`
payload/correlation proof is a separate RO-owned gate. `IT-RO-AUDIT-2593-001` must prove
that the persisted event retains `RoutingBlockedData` rather than only its discriminator,
RR name and namespace, and must reconstruct the event through its correlation ID. This
plan does not claim complete BR-AUDIT-005 reconstruction.

Pinned ASVS sources: [V2](https://github.com/OWASP/ASVS/blob/180622b1329ee7ed349c9f5ce5e14fa91f42c359/5.0/en/0x11-V2-Validation-and-Business-Logic.md),
[V8](https://github.com/OWASP/ASVS/blob/180622b1329ee7ed349c9f5ce5e14fa91f42c359/5.0/en/0x17-V8-Authorization.md),
[V12](https://github.com/OWASP/ASVS/blob/180622b1329ee7ed349c9f5ce5e14fa91f42c359/5.0/en/0x21-V12-Secure-Communication.md),
and [V16](https://github.com/OWASP/ASVS/blob/180622b1329ee7ed349c9f5ce5e14fa91f42c359/5.0/en/0x25-V16-Security-Logging-and-Error-Handling.md).

---

## 8. Requirements-based scenario matrix

### 8.1 Unit scenarios — denominator 12, required 12/12

| ID | Business outcome | Priority | BR/control mapping | Status |
|---|---|---|---|---|
| `UT-FMC-PRES-001` | Present explicit true is managed even when namespace false | P0 | BR-SCOPE-001/002/010; AC-4/AC-6 | Pending |
| `UT-FMC-PRES-002` | Present explicit false is unmanaged even when namespace true | P0 | BR-SCOPE-001/002/010; AC-4/AC-6/SI-10 | Pending |
| `UT-FMC-PRES-003` | Present unlabeled resource inherits confirmed managed namespace | P0 | BR-SCOPE-001/002/010; AC-4/AC-6 | Pending |
| `UT-FMC-PRES-004` | Invalid resource or namespace label is typed invalid/unavailable, not absent; it cannot inherit or allow | P0 | BR-SCOPE-001/002/010; pending ADR-053 amendment; AC-4/AC-6/SI-10 | Pending |
| `UT-FMC-PRES-005` | Missing/unconfirmed target never allows a ghost signal | P0 | BR-SCOPE-001/002/010; AC-4/AC-6 | Pending |
| `UT-FMC-PRES-006` | Presence/backend error never falls through to namespace allow | P0 | BR-SCOPE-001/002/010; AC-4/AC-6/SI-10 | Pending |
| `UT-FMC-PRES-007` | Cluster-scoped input never inherits namespace; deferred/unsupported cluster-scope authorization is unavailable | P0 | BR-SCOPE-001 FR-SCOPE-004 deferred; SI-10 | Pending |
| `UT-FMC-PRES-008` | Unsupported/uncollected/unknown GVK is unavailable, not false | P0 | BR-SCOPE-001/002/010; SI-10 | Pending |
| `UT-FMC-CACHE-009` | Only observed entries refresh TTL; unobserved entries expire | P0 | BR-SCOPE-001; AC-4/AC-6 | Pending |
| `UT-FMC-CACHE-011` | Expired record and stale index are not returned | P0 | BR-SCOPE-001; AC-4/AC-6/SI-10 | Pending |
| `UT-FMC-CACHE-012` | Concurrent refresh survives stale cleanup | P0 | BR-SCOPE-001; AC-4 | Pending |
| `UT-FMC-CACHE-014` | Wrong cluster/GVK/identity cannot return another target | P0 | BR-SCOPE-001/BR-SECURITY-016; AC-4/AC-6/SI-10 | Pending |

### 8.2 Integration-owned business scenarios — denominator 4, required 4/4

| ID | Business outcome | Priority | BR/control mapping | Status |
|---|---|---|---|---|
| `IT-FMC-CACHE-010` | Incomplete scan returns unavailable/no rows and does not refresh unobserved records | P0 | BR-SCOPE-001; AC-4/AC-6/SI-10 | Pending |
| `IT-FMC-CACHE-013` | Partial/lost/wrong-type mutation cannot serve trusted data before repair | P0 | BR-SCOPE-001/BR-SECURITY-016; AC-4/AC-6/SI-10 | Pending |
| `IT-FMC-CACHE-015` | Sparse scan/cursor failure cannot masquerade as complete/absent | P0 | BR-SCOPE-001; AC-4/AC-6/SI-10 | Pending |
| `IT-FMC-CACHE-016` | Verified recovery makes the partition available again | P1 | BR-SCOPE-001; AC-4/AC-6 | Pending |
| `IT-FMC-MIGRATION-017` | N/A — the superseded v0.7 migration/cutover premise is out of scope for this unreleased v1 replacement; no public v2, rollout, compatibility, rollback, backfill, or migration evidence requirement | N/A | N/A; zero credit; owner `#2593` coordinator; FMC/API-owner disposition required before RED | N/A / out of scope |

### 8.3 Test-case specifications for highest-risk scenarios

| ID | Given | When | Then |
|---|---|---|---|
| `UT-FMC-PRES-005` | Resource record is missing/expired and namespace is verified managed | Scope is evaluated | Result is `unavailable`; no namespace fallback, no verified false, no allow |
| `UT-FMC-PRES-006` | Exact resource lookup returns backend error and namespace is managed | Scope is evaluated | Typed unavailable/error is returned; no unmanaged message and no allow |
| `IT-FMC-CACHE-013` | A true record is followed by an ambiguous true-to-false mutation | Writer loses reply or detects partial state | Partition becomes uncertain; scope/inventory return unavailable/no rows until repair |
| `IT-FMC-CACHE-004` | Old holder, new holder, registry loss, and empty registry are exercised | Lease `spec.leaseTransitions` advances under CAS, or renewal is lost or registration UID changes | Old holder/session fence cannot write/clear state; affected target is unavailable; takeover obtains the Lease-derived fence and repairs before clean |
| `IT-FMC-CACHE-005` | Two syncer candidates use the same Lease | Both attempt a cycle with distinct holder-session UIDs | Only the holder with the current Lease `leaseTransitions` fence can write; the stale holder receives a typed stale-fence outcome |
| `IT-FMC-PRES-010` | Same Kind exists in two groups and APIVersion is explicit | Signal passes Gateway, SP, RO, AF, and FMC scope paths | Exact group/version survives every handoff; missing version is rejected before lookup |
| `IT-FMC-PRES-009` | Fleet mode receives an empty or mismatched `ClusterID` while a local hub exists | GW/RO/AF scope dispatch is attempted | Missing/mismatched attribution is typed invalid/unavailable; no local fallback, lookup, allow, RR or WFE; explicit non-fleet local mode is tested separately |
| `IT-FMC-PRES-001` | Unfiltered MCP source returns multiple pages with `limit=50`/`continue`, missing/malformed structured content, a table row, or a label-bearing item | Production reader factory calls `mcpclient.listResources` and `fmc.SyncCluster` | Exact continuation is followed at most 100 pages/5,000 items; metadata/label-less rows are rejected; all labels are preserved; incomplete/malformed content is unavailable and never an empty success or TTL refresh |
| `IT-FMC-AUTH-008` | TokenReview or SAR service is unavailable after a valid bearer reaches FMC | Auth middleware handles the failure | FMC HTTP boundary returns `500 Internal Server Error`; internal callers may preserve typed unavailable after receiving that boundary result; no operation or fallback |
| `E2E-FMC-AUTH-018` | Real Kind ServiceAccount token, denial, invalid token and API outage are exercised | Production FMC auth middleware handles requests | Real TokenReview/SAR path proves 401/403/500, verified identity/header stripping, no token forwarding, and structured security events for invalid/denied requests, auth-service failure and successful grant |

### 8.3.1 Complete P0 test specifications

The following table is the complete detailed Given/When/Then specification for the 15
critical P0 business scenarios: 12 UT rows and three active IT rows. It does not add scenarios or
inflate the denominator; it makes the IEEE 829 acceptance assertions explicit.

| ID | Given | When | Then |
|---|---|---|---|
| `UT-FMC-PRES-001` | Resource is verified present with explicit `managed=true` and namespace is false | Scope is evaluated | Result is verified managed; resource label wins over namespace |
| `UT-FMC-PRES-002` | Resource is verified present with explicit `managed=false` and namespace is true | Scope is evaluated | Result is verified unmanaged; explicit false wins and no allow occurs |
| `UT-FMC-PRES-003` | Resource is verified present with no resource label and namespace is verified managed | Scope is evaluated | Result is verified managed from namespace provenance |
| `UT-FMC-PRES-004` | Resource or namespace label is malformed | Scope is evaluated | Result is invalid/unavailable; malformed input neither inherits nor authorizes |
| `UT-FMC-PRES-005` | Resource record is missing or expired while namespace is managed | Scope is evaluated | Result is unavailable; no namespace fallback, verified false or allow |
| `UT-FMC-PRES-006` | Exact resource observation or backend returns an error while namespace is managed | Scope is evaluated | Result is unavailable/error; no unmanaged message and no allow |
| `UT-FMC-PRES-007` | Cluster-scoped input has a namespace, or lacks an approved supported-GVK contract | Scope is evaluated | Result is invalid/unsupported/unavailable; namespace inheritance is impossible |
| `UT-FMC-PRES-008` | Kind is unsupported, uncollected, ambiguous or missing explicit GVK | Scope is evaluated | Result is unsupported/unavailable; no core-v1 inference or false decision |
| `UT-FMC-CACHE-009` | A sync observes only a subset of records and an unobserved record exists in cache | Sync completes | Only observed records refresh TTL; unobserved record expires and cannot authorize |
| `UT-FMC-CACHE-011` | Record is expired or index member is stale | Scope/cache read runs | Expired/stale data is rejected; no stale positive is returned |
| `UT-FMC-CACHE-012` | Refresh and stale-index cleanup race on the same identity | Cleanup and refresh overlap | New valid generation survives; stale cleanup cannot delete or authorize the new record |
| `UT-FMC-CACHE-014` | Same name/kind exists across clusters or groups, or identity tuple is malformed | Canonical lookup runs | Only the exact validated tuple/partition matches; collision and malformed IDs fail closed |
| `IT-FMC-CACHE-010` | Source pagination, structured content, reader or write cycle is incomplete, including a table row without metadata/labels | Production sync/read path runs | Typed complete-scan state is false; cycle is unavailable/no rows, unobserved records do not refresh, and no allow is served |
| `IT-FMC-CACHE-013` | Valkey write/reply is partial, wrong-type or ambiguous, or `abs((expires_at_ms-server_now_ms)-pttl_ms)>2ms` | Recovery/read path runs | Partition is uncertain and unavailable until Lease-fenced repair proves clean state; no stale positive or false absence is served |
| `IT-FMC-CACHE-015` | A bounded source scan, repeated/invalid continuation cursor, or page bound fails after some pages have been observed | Production inventory/scope read completes | The result is incomplete/unavailable with no rows or false absence; only a verified complete scan may refresh or establish emptiness |
| `IT-FMC-MIGRATION-017` | Superseded v0.7 migration/cutover premise (historical only) | No public v2, rollout, compatibility, rollback, backfill, or migration evidence is authorized for this unreleased replacement | N/A / out of scope with zero credit; `#2593` coordinator owns disposition and FMC/API-owner approval is required before RED |

### 8.4 Authoritative scenario execution matrix

This is the auditable scenario denominator. Planned new files are named before RED;
existing files are extended rather than duplicated. Commands are scoped so each result
can be attached to its matching evidence artifact.

| ID | Exact test file | Exact command | Owner | Denominator | Status |
|---|---|---|---|---|---|
| `UT-FMC-PRES-001`–`UT-FMC-PRES-008` | `pkg/fleet/fmc/presence_scope_test.go` (planned) | `go test ./pkg/fleet/fmc/... -ginkgo.v -ginkgo.focus='UT-FMC-PRES-'` | FMC/scope owner | UT 1–8 of 12 | Pending |
| `UT-FMC-CACHE-009` | `pkg/fleet/scopecache/recovery_test.go` (planned) | `go test ./pkg/fleet/scopecache/... -ginkgo.v -ginkgo.focus='UT-FMC-CACHE-009'` | Cache owner | UT 9 of 12 | Pending |
| `UT-FMC-CACHE-011`–`UT-FMC-CACHE-012` | `pkg/fleet/scopecache/recovery_test.go` (planned) | `go test ./pkg/fleet/scopecache/... -ginkgo.v -ginkgo.focus='UT-FMC-CACHE-01[12]'` | Cache owner | UT 10–11 of 12 | Pending |
| `UT-FMC-CACHE-014` | `pkg/fleet/scopecache/recovery_test.go` (planned) | `go test ./pkg/fleet/scopecache/... -ginkgo.v -ginkgo.focus='UT-FMC-CACHE-014'` | Cache/security owner | UT 12 of 12 | Pending |
| `IT-FMC-CACHE-010` | `test/integration/fleetmetadatacache/fmc_presence_recovery_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-CACHE-010'` | FMC owner | IT 1 of 4 | Pending |
| `IT-FMC-CACHE-013` | `test/integration/fleetmetadatacache/fmc_valkey_recovery_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-CACHE-013'` | FMC/platform owner | IT 2 of 4 | Pending |
| `IT-FMC-CACHE-015` | `test/integration/fleetmetadatacache/fmc_presence_recovery_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-CACHE-015'` | FMC owner | IT 3 of 4 | Pending |
| `IT-FMC-CACHE-016` | `test/integration/fleetmetadatacache/fmc_valkey_recovery_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-CACHE-016'` | FMC/platform owner | IT 4 of 4 | Pending |
| `IT-FMC-MIGRATION-017` | N/A (no test file or command planned; identifier retained for disposition traceability) | N/A | `#2593` coordinator; FMC/API-owner disposition required before RED | N/A / out of scope; zero credit |

### 8.5 Conditional E2E control gate — denominator 1, required 1/1

This gate is separate from the 16 business scenarios and 19 implementation wiring rows.
It is mandatory for claiming BR-SECURITY-016/017 control assessment. This draft does not
authorize a cluster run; explicit user/architecture authorization or an approved risk
acceptance is required before execution.

| ID | Exact test file | Exact command | Owner | Evidence | Status |
|---|---|---|---|---|---|
| `E2E-FMC-AUTH-018` | `test/e2e/fleet/20_fmc_auth_rbac_e2e_test.go` (planned) | `go test ./test/e2e/fleet -ginkgo.v -ginkgo.focus='E2E-FMC-AUTH-018'` | Security/operator owner | `docs/tests/2593/evidence/rbac/e2e/` with Kind revision, rendered identity/RBAC, TokenReview/SAR output, status/log assertions and redaction record | Pending authorization |

---

## 9. Production wiring manifest

The following 19 rows are the implementation wiring denominator. Each requires a passing
IT through production construction/dispatch before GREEN. The two conditional server rows,
exactly `IT-AF-CACHE-006` and `IT-FMC-INVENTORY-012`, are owned by #2584/#2600 and remain
unavailable/no rows in this slice; they are tracked as `0/2` external rows and are not
counted as passed implementation wiring. They are a strict subset of the aggregate
AF/FMC/Page qualification denominator `0/3`, whose third row is
`IT-AF-PAGE-2584-012`; `0/2` does not replace or imply `0/3`. The 19 rows are 14 existing
FMC/GW/RO/AF rows, four typed-scope contract seams, and one AIAnalysis/GVK handoff row.

| ID | Production wiring point | Owner | Required evidence |
|---|---|---|---|
| `IT-FMC-PRES-001` | `cmd/fleetmetadatacache` → `pkg/fleet/fmc.ObservedReaderFactory` → `observation.ObservedReader` → `pkg/fleet/mcpclient.Client.ListObserved/listResources` → syncer | FMC owner | Multi-page unfiltered identity/label observation, structured-content presence, pagination failure and error propagation; incomplete source never refreshes TTL |
| `IT-FMC-CACHE-002` | FMC writer/reader → controlled Valkey test dependency | FMC/platform owner | TTL, index, repair, restart, race behavior; no qualification claim |
| `IT-FMC-CACHE-003` | Valkey test configuration → FMC clients | Platform owner | Internal protocol, TLS/ACL boundary and failure behavior; no deployment qualification claim |
| `IT-FMC-CACHE-004` | Typed status store → syncer → handler/readers | FMC owner | Lifecycle, registry loss, repair, stale fence, readiness |
| `IT-FMC-CACHE-005` | `wireFMCDependencies` → Lease client → `syncer.Run` | FMC/operator owner | Lease RBAC, holder Pod UID/session UID, Lease epoch-to-Valkey fence equality, renewal/loss/takeover |
| `IT-FMC-PRES-003` | `fmc.Handler.RegisterRoutes` → v1 scope client | FMC owner | `/api/v1/scope/check`, ordinary `application/json`, `ScopeCheckResponse.Managed` / `managed`; any typed unavailable/verified extension is gated by BR/ADR/API-owner approval and no public v2 route/media is allowed |
| `IT-FMC-PRES-009` | HTTP validation → canonical identity → cache | Scope/API owner | Invalid, duplicate, unsupported, collision rejection |
| `IT-FMC-PRES-010` | NormalizedSignal → RR → SP → RO/AF → FMC | SP/API owner | Schema, generated CRD, full GVK propagation |
| `IT-FMC-AUTH-011` | Rendered target roles/config → collector and MCP target caller | Operator/security owner | Fixed/dynamic/admin kind matrix, wildcard/subresource tests |
| `IT-GW-PRES-004` | Gateway `validateScope` → typed `ResourceIdentity` → FMC HTTP client → RR decision | Gateway owner | Group/version are copied before the pre-RR scope check; managed, unmanaged and unavailable are distinct |
| `IT-RO-PRES-005` | RO scope factory → routing check → WFE decision | RO owner | Block/retry distinction; direct ACM regression |
| `IT-AF-PRES-007` | AF tool scope helper → configured FMC dispatcher | AF owner | Explicit GVK and no unavailable-to-unmanaged conversion |
| `IT-FMC-AUTH-008` | TokenReview/SAR and actual caller identities | Security/operator owner | FMC, Gateway, RO, MCP, Search, AF identity matrix |
| `IT-FMC-MIGRATION-017` | N/A — superseded v0.7 migration identifier retained for disposition traceability only | `#2593` coordinator; FMC/API-owner approval required before RED | Zero credit; no migration inventory, rollout/cutover, mixed-version, old-client, rollback-compatibility, backfill, or migration evidence requirement |
| `IT-AF-ROUTE-010` | AF local/exact/tool routes, `policyread.Service.authorizeInventoryTarget`, and FMC/ACM separation | AF owner | Typed remote scope unavailable/unsupported/error is state-only and never `NotFound`, `managed:false`, allow, or local fallback; no remote Console list route; direct ACM remains separate |
| `IT-SCOPE-TYPED-018` | `pkg/shared/scope/manager.go:Manager.Resolve/IsManagedResource` → local typed scope result | Scope/API owner | Explicit true/false/absent/invalid, namespaced/cluster scope, unavailable and no fallback |
| `IT-SCOPE-TYPED-019` | `pkg/fleet/federated_checker.go:NewFederatedScopeChecker/Remote/IsManagedResource` → local/remote typed routing | Fleet owner | Empty `ClusterID` is local only outside fleet mode; unknown cluster is unavailable; remote states preserve typed outcomes |
| `IT-SCOPE-TYPED-020` | `pkg/fleet/acm/client.go:Client.IsManagedResource` → direct ACM typed adapter | ACM/RO owner | Direct ACM remains separate; authenticated/unauthorized/unavailable/verified states are not collapsed |
| `IT-SCOPE-TYPED-021` | `pkg/fleet/scope_factory.go:NewScopeChecker` → backend selection and caller contract | Fleet/GW/RO owner | FMC, ACM, local and unsupported backend routes return the agreed typed result |
| `IT-FMC-GVK-022` | `pkg/aianalysis/handlers/analyzing.go` → AI history/policy reader → explicit GVK identity | AI/SignalProcessing owner | AIAnalysis and SignalProcessing paths retain GVK; remote missing GVK is unavailable; no `InferGVK` fallback |

### 9.1 Wiring execution matrix

The 19 rows below are the complete implementation wiring denominator. Conditional
inventory rows are listed separately in Section 9.2 and cannot be marked passed here.

| ID | Exact test file | Exact command | Owner | Wiring denominator |
|---|---|---|---|---:|
| `IT-FMC-PRES-001` | `pkg/fleet/mcpclient/client_test.go` (extend concrete reader) + `test/integration/fleetmetadatacache/fmc_presence_wiring_test.go` (planned; FMC factory/observation seam) | `go test ./pkg/fleet/mcpclient/... ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-PRES-001'` | FMC owner | 1/19 |
| `IT-FMC-CACHE-002` | `test/integration/fleetmetadatacache/fmc_valkey_recovery_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-CACHE-002'` | FMC/platform owner | 2/19 |
| `IT-FMC-CACHE-003` | `test/infrastructure/fmc_valkey_contract_test.go` (planned) plus controlled test-dependency configuration | `go test ./test/infrastructure -ginkgo.v -ginkgo.focus='IT-FMC-CACHE-003'` | Platform owner | 3/19 |
| `IT-FMC-CACHE-004` | `test/integration/fleetmetadatacache/fmc_lifecycle_wiring_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-CACHE-004'` | FMC owner | 4/19 |
| `IT-FMC-CACHE-005` | `test/integration/fleetmetadatacache/fmc_lease_wiring_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-CACHE-005'` | FMC/operator owner | 5/19 |
| `IT-FMC-PRES-003` | `test/integration/fleetmetadatacache/fmc_http_api_test.go` (extend) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-PRES-003'` | FMC owner | 6/19 |
| `IT-FMC-PRES-009` | `test/integration/fleetmetadatacache/fmc_http_api_test.go` (extend) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-PRES-009'` | Scope/API owner | 7/19 |
| `IT-FMC-PRES-010` | `test/integration/signalprocessing/fleet/sp_fleet_gvk_test.go` (planned) | `go test ./test/integration/signalprocessing/fleet -ginkgo.v -ginkgo.focus='IT-FMC-PRES-010'` | SignalProcessing API owner | 8/19 |
| `IT-FMC-AUTH-011` | `test/integration/fleetmetadatacache/fmc_target_rbac_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-AUTH-011'` | Operator/security owner | 9/19 |
| `IT-GW-PRES-004` | `test/integration/gateway/fleet/gw_fleet_owner_chain_test.go` (extend) | `go test ./test/integration/gateway/fleet -ginkgo.v -ginkgo.focus='IT-GW-PRES-004'` | Gateway owner | 10/19 |
| `IT-RO-PRES-005` | `test/integration/remediationorchestrator/ro_fleet_scope_test.go` (extend) | `go test ./test/integration/remediationorchestrator -ginkgo.v -ginkgo.focus='IT-RO-PRES-005'` | RO owner | 11/19 |
| `IT-AF-PRES-007` | `test/integration/apifrontend/fleet/af_remediate_fleet_wiring_test.go` (extend) | `go test ./test/integration/apifrontend/fleet -ginkgo.v -ginkgo.focus='IT-AF-PRES-007'` | AF owner | 12/19 |
| `IT-FMC-AUTH-008` | `test/integration/fleetmetadatacache/fmc_auth_test.go` (planned) | `go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-AUTH-008'` | Security/operator owner | 13/19 |
| `IT-FMC-MIGRATION-017` | N/A (identifier retained for disposition traceability; no test file or command planned) | N/A | `#2593` coordinator; FMC/API-owner disposition required before RED | N/A / out of scope; zero credit |
| `IT-AF-ROUTE-010` | `test/integration/apifrontend/fleet/af_fleet_routing_test.go` (extend) + `test/integration/apifrontend/fleet/af_policyread_typed_scope_test.go` (planned) | `go test ./test/integration/apifrontend/fleet -ginkgo.v -ginkgo.focus='IT-AF-ROUTE-010'` | AF owner | 14/19 |
| `IT-SCOPE-TYPED-018` | `test/integration/scope/manager_typed_scope_test.go` (planned) | `go test ./test/integration/scope -ginkgo.v -ginkgo.focus='IT-SCOPE-TYPED-018'` | Scope/API owner | 15/19 |
| `IT-SCOPE-TYPED-019` | `test/integration/fleet/scope_federated_typed_test.go` (planned) | `go test ./test/integration/fleet -ginkgo.v -ginkgo.focus='IT-SCOPE-TYPED-019'` | Fleet owner | 16/19 |
| `IT-SCOPE-TYPED-020` | `test/integration/fleet/acm_scope_typed_test.go` (planned) | `go test ./test/integration/fleet -ginkgo.v -ginkgo.focus='IT-SCOPE-TYPED-020'` | ACM/RO owner | 17/19 |
| `IT-SCOPE-TYPED-021` | `test/integration/fleet/scope_factory_typed_test.go` (planned) | `go test ./test/integration/fleet -ginkgo.v -ginkgo.focus='IT-SCOPE-TYPED-021'` | Fleet/GW/RO owner | 18/19 |
| `IT-FMC-GVK-022` | `test/integration/aianalysis/fleet/ai_gvk_propagation_test.go` (planned) | `go test ./test/integration/aianalysis/fleet -ginkgo.v -ginkgo.focus='IT-FMC-GVK-022'` | AI/SignalProcessing owner | 19/19 |

### 9.2 External conditional gates

These gates are owned outside the FMC implementation denominator. They are required only
for a separately authorized related claim and must carry exact owner, command, artifact and
result evidence; they are not source-slice or qualification credit here.

| ID | Exact test file | Exact command | Accountable owner | Independent reviewer | Artifact | Status |
|---|---|---|---|---|---|---|
| `IT-AF-CACHE-006` | `test/integration/apifrontend/fleet/af_fleet_inventory_gate_test.go` (planned in the approved #2584/#2600 external plan) | `go test ./test/integration/apifrontend/fleet -count=1 -ginkgo.v -ginkgo.focus='IT-AF-CACHE-006'` | **Unassigned in this slice; named identity required in approved #2584/#2600 plan** | **Unassigned; independent named reviewer required** | `docs/tests/2593/evidence/af-fmc/it-af-cache-006.json` plus wire JSON, stdout/stderr and revision/hash metadata | Blocked: external registration absent |
| `IT-FMC-INVENTORY-012` | `test/integration/fleetmetadatacache/fmc_inventory_contract_2584_gate_test.go` (planned in the approved #2584/#2600 external plan) | `go test ./test/integration/fleetmetadatacache -count=1 -ginkgo.v -ginkgo.focus='IT-FMC-INVENTORY-012'` | **Unassigned in this slice; named identity required in approved #2584/#2600 plan** | **Unassigned; independent named reviewer required** | `docs/tests/2593/evidence/fmc-inventory/it-fmc-inventory-012.json` plus wire JSON, stdout/stderr and revision/hash metadata | Blocked: external registration absent |
| `IT-AF-PAGE-2584-012` | `pkg/fleet/inventory/types_2584_test.go` (extend and register in the approved #2584/#2600 external plan) | `go test ./pkg/fleet/inventory -count=1 -ginkgo.v -ginkgo.focus='IT-AF-PAGE-2584-012'` | **Unassigned in this slice; named identity required in approved #2584/#2600 plan** | **Unassigned; independent named reviewer required** | `docs/tests/2593/evidence/af-page/it-af-page-2584-012-wire-json.json` plus stdout/stderr and revision/hash metadata | Blocked: external registration absent |
| `IT-RO-AUDIT-2593-001` | `test/integration/remediationorchestrator/scope_blocking_integration_test.go` (extend) | `go test ./test/integration/remediationorchestrator -count=1 -ginkgo.v -ginkgo.focus='IT-RO-AUDIT-2593-001'` | Named RO audit/API owner | Named #2593 coordinator, independent of owner | `docs/tests/2593/evidence/ro-audit/it-ro-audit-2593-001.json` plus stdout/stderr, producer-field assertions, DataStorage reconstruction and revision/hash metadata | Pending external approval |

The two conditional server rows are a `0/2` subset of the three AF/FMC/Page external
blocking gates: `IT-AF-CACHE-006` and `IT-FMC-INVENTORY-012` are the server rows, and
`IT-AF-PAGE-2584-012` is the third Page row. The aggregate remains `0/3` until all three
independently registered gates pass. These rows are external blocking gates, not
implementation credit. The external plan owner must register the exact IDs/files/commands above before execution. A
gate artifact is complete only when its JSON record names the accountable owner and
independent reviewer, records the external plan revision and both current artifact hashes,
source revision, UTC start/end, command, exit status, redaction status, expected and actual
result, and links the captured wire JSON and stdout/stderr. `IT-AF-CACHE-006` must prove
that non-local AF inventory remains state-only unavailable/no rows unless the approved
remote projection is explicitly qualified; it must never use direct MCP/Kubernetes
discovery or a local fallback. `IT-FMC-INVENTORY-012` must prove the authenticated FMC
inventory response matrix: complete `success`/`empty` only, state-only `503` unavailable,
`400` invalid, `401/403` unauthorized and `422` unsupported, with no rows/count/cursor on
failure. `IT-AF-PAGE-2584-012` must prove `Page.Validate`, complete/empty invariants,
state-only failure pages, bounded cursor behavior, strict `r-[0-9a-f]{64}` IDs and exact
recomputation from the validated cluster/GVK/namespace/name tuple. A missing external plan
entry, owner, reviewer, wire artifact or result keeps all three gates unavailable and
preserves the zero inventory rows.

### 9.3 External gate registration record

The three AF/FMC/Page gates are not executable from role labels alone. Before execution,
the approved #2584/#2600 external plan must replace each `Unassigned` field above with an
immutable accountable owner identity and a different independent reviewer identity, then
record the approved plan path/revision, source revision, exact test file, command,
authorization reference and evidence artifact path. The result record must include UTC
start/end, exit status, redaction status, expected/actual result, captured wire JSON and
stdout/stderr, and both current plan/test-plan hashes. Until that registration is present,
the denominator remains 0/3; no FMC inventory rows, AF non-local rows or route qualification
credit may be claimed.

---

## 10. Environmental needs and evidence ownership

### 10.1 Unit-test environment

- Go `1.26.10` from `go.mod`; Ginkgo `v2.33.1`; Gomega `v1.44.0`.
- Ginkgo/Gomega BDD only; external Valkey/Kubernetes/HTTP dependencies may be faked in
  unit tests, but internal cache, scope, identity and state logic is real.
- Unit packages: `pkg/fleet/fmc/...`, `pkg/fleet/scopecache/...`,
  `pkg/fleet/registry/...`, `pkg/shared/scope/...`, and `pkg/fleet/inventory/...`.

### 10.2 Integration environment

- `httptest` with production FMC router/middleware for HTTP contract tests.
- `envtest` or the repository's supported Kubernetes harness for registration UID,
  Lease, CRD schema and TokenReview/SAR wiring.
- A controlled Valkey test dependency or approved fixture for internal TTL, EVALSHA,
  status, fence, repair, TLS and ACL behavior. No live deployment, cluster, or Valkey
  qualification claim is made by this source-only slice.
- No shared fixed ports; use `:0` and per-test clients. No business-logic mocks.

### 10.3 Conditional E2E environment

- `E2E-FMC-AUTH-018` remains a separately owned conditional control gate for any future
  BR-SECURITY-016/017 E2E assessment. This draft authorizes no E2E or live-cluster run;
  it is not source-slice credit. If separately authorized, use an exclusively owned Kind
  cluster, real ServiceAccount tokens, real TokenReview/SAR calls, and the exact rendered
  operator/auth artifacts. ACM inventory remains outside this plan and unqualified.

### 10.4 Evidence ownership and paths

| Evidence | Owner | Required path |
|---|---|---|
| Internal Valkey TTL/status/fence/repair source evidence | Platform/FMC owner | `docs/tests/2593/evidence/valkey/` |
| Registration UID/target incarnation | Fleet registry/Gateway owner | `docs/tests/2593/evidence/cluster-incarnation/` |
| Rendered identities, RBAC, SAR/TokenReview | Operator/security owner | `docs/tests/2593/evidence/rbac/` |
| Target-kind/GVK matrix | Scope/API owner | `docs/tests/2593/evidence/kinds/` |
| SignalProcessing schema and GVK handoff | SignalProcessing API owner | `docs/tests/2593/evidence/gvk/` |
| Cache lifecycle, readiness, Lease, repair, and source wiring | FMC owner | `docs/tests/2593/evidence/fmc/` |
| Review responses, exact hashes, Engram references | #2593 review coordinator | `docs/tests/2593/evidence/reviews/` |
| AF audit and conditional route evidence | #2584/#2600 owner | Existing #2584 plan/evidence; no FMC credit |
| RO routing audit evidence | RO audit owner | `docs/tests/2593/evidence/ro-audit/it-ro-audit-2593-001.json` plus command/output, persisted `RoutingBlockedData`, correlation reconstruction, owner/reviewer and both artifact hashes |
| Authentication E2E control evidence | Security/operator owner | `docs/tests/2593/evidence/rbac/e2e/` for `E2E-FMC-AUTH-018` |
| External AF/FMC inventory gate evidence | Unassigned in this slice; named #2584/#2600 owner and independent reviewer required before execution | `docs/tests/2593/evidence/af-fmc/` and `docs/tests/2593/evidence/fmc-inventory/` for `IT-AF-CACHE-006` and `IT-FMC-INVENTORY-012` |
| External Page contract | Unassigned in this slice; named #2584/#2600 owner and independent reviewer required before execution | `docs/tests/2593/evidence/af-page/` for `IT-AF-PAGE-2584-012` |

The role owner and independent reviewer must be named before RED. Missing or ambiguous
source/wiring evidence keeps the affected capability unavailable. Deployment, cluster,
ACM, Valkey qualification, and external audit artifacts remain separately owned and do not
create a claim in this source-only plan.

Every evidence artifact must record: owner, independent reviewer, UTC timestamp, source
revision, command or test command, redaction status, expected result, actual result, and
the plan/test-plan SHA-256. Directory-only evidence is not complete. Any future external
deployment artifact may add its image/config digest and environment metadata, but that is
not required or credited for this source-only amendment. Required source-level evidence
templates include:

- Valkey: controlled-test evidence for `EVALSHA`, `TIME`, `PTTL`, `PEXPIREAT`, status,
  full writer fence, restart/lost-reply repair, and the absolute expiry/PTTL delta; the
  delta must fail when it exceeds 2ms. No live-cluster command is required here.
- Source: an **unfiltered** `resources_list` MCP `tools/call` test fixture with explicit
  `apiVersion`, bounded namespace/kind scope, `limit=50`, no `labelSelector`,
  `--list-output=yaml`, and captured `structuredContent.items[]`. The item evidence records
  whether `metadata.labels` is absent or present (including `{}`), and distinguishes
  true, false, absent and invalid managed-label states. Namespaced items must have a
  non-empty namespace; cluster-scoped items must have an empty namespace. The first
  request omits `continue`; later requests send the exact prior non-empty token, preserving
  absent-versus-null final-field presence, with a 100-page and 5,000-item bound and
  repeated-token detection. Duplicate/conflicting canonical IDs are rejected before any
  write. If
  the qualified backend cannot provide an unfiltered list, an approved paired design
  must use an exact identity-bearing `resources_get` for the target plus an independent
  unfiltered observation; a positive `labelSelector=kubernaut.ai/managed=true` query is
  never sufficient by itself. Evidence must distinguish zero resources from zero
  managed-label matches and include true, false, absent, invalid and incomplete-scan
  cases.
 - Generated GVK: `make manifests`, `make sync-embed`, `make generate`, `make gen-diff` when
   source generation changes require them; these are future source-evidence commands only.

These commands are future evidence requirements; none are executed as deployment or
qualification evidence during this source-only phase.

---

## 11. Dependencies, schedule and TDD/review gate

### 11.1 Blocking dependencies

| Dependency | Owner | Status | Impact if unavailable | Required resolution |
|---|---|---|---|---|
| BR-SCOPE-001/002/010 and ADR-053 amendments | Architecture/scope owner | Pending final disposition | Final semantics/qualification remain blocked; RED records the exact fail-closed contract | Approve/reject the exact reconciliation in the external plan |
| ADR-068/DD-FLEET-011 reconciliation and new fleet DD | Architecture owner | Pending final disposition | Final cache/typed-scope evidence remains blocked | Approve a new DD-FLEET record; preserve DD-FLEET-011 files |
| Internal Valkey test dependency and semantics | Platform/Valkey owner | Missing | TTL/recovery GREEN blocked | Approve the controlled test dependency and internal TTL/status/fence/repair evidence; no qualification claim |
| Rendered identity/RBAC and target-kind matrix | Operator/security owner | Missing | Source collection and auth ITs blocked | Attach rendered manifests, SAR/TokenReview and MCP label-bearing evidence |
| SignalProcessing GVK schema/generated artifacts | SignalProcessing API owner | Missing | Remote exact scope/history unavailable | Update schema, generated CRDs/assets, and pass generation diff |
| AIAnalysis GVK handoff and remote-history contract | AI/SignalProcessing owner | Missing | AI history/remote exact scope may infer the wrong kind | Update `analyzing.go` and policy/history readers; add explicit AI GVK IT; reject remote `InferGVK` |
| Public FMC v1 contract and typed-extension approval | FMC/API owner | v1 baseline fixed; typed extension pending | Route/media/schema drift or invented fields block GREEN | Reconcile the source draft to ADR-068 `/api/v1/scope/check`, ordinary `application/json`, and `ScopeCheckResponse.Managed`; approve any typed extension before use |
| Typed scope seam wiring | Scope/Fleet/ACM owners | Missing | Boolean implementations can collapse unavailable or backend identity states | Add and pass `IT-SCOPE-TYPED-018`–`IT-SCOPE-TYPED-021` |
| #2584/#2600 Page contract and AF boundary | AF owner | External/conditional | No FMC inventory rows or AF remote list/policy scope | Keep unavailable/no rows; attach separate approved evidence |
| Kind authentication E2E | Security/operator owner | Pending authorization | BR-SECURITY-016/017 E2E assessment is incomplete | Authorize and run `E2E-FMC-AUTH-018`, or attach approved risk acceptance |
| Historical clean-review counter | #2593 coordinator | 0/3 historical | No new pre-RED review cycle; per-pass review remains required | Preserve the counter and record immutable RED/GREEN/REFACTOR captures |

### 11.2 Authoritative execution sequence

1. **DISCOVERY**: use the approved fast-path boundary, carry/hold manifest, public v1
   contract and bounded plan. Record unresolved final-evidence dependencies without
   starting another planning or three-review cycle.
2. **RED**: add failing Ginkgo/Gomega UTs and production-path ITs for every selected
   wiring row, including all 19 implementation rows. Capture current positive-only,
   swallowed-error, GVK-loss, AI `InferGVK`, boolean-HTTP, and namespace-fallback failures.
   RED also defines/tests the conversion of every listed dirty v2 route/media/test consumer;
   no source conversion is required before RED.
3. **GREEN**: after the RED contracts pass; wire the minimal internal
   Valkey protocol, Lease, typed result, GVK schema, typed scope seams and FMC v1 scope
   path, and atomically convert the unreleased source plus every listed consumer before any
   production-composition or wiring credit. Do not add a public v2 route/media or
   migration/compatibility machinery. Do not register conditional AF inventory routes. Do
   not run the conditional E2E without explicit authorization.
4. **REFACTOR**: simplify state/label precedence and cleanup/recovery paths; run build,
   lint, focused tests, and required regressions as safety validation.
5. **SOURCE-ONLY EXIT**: attach final unreleased v1 source and wiring evidence, verify
   Checkpoint W, direct ACM scope regression, typed-scope/GVK/fail-closed boundaries, and
   the separate audit-owner disposition. This step makes no production, cluster, ACM,
   Valkey qualification, deployment, or release claim.

Before implementation approval, three separate independent read-only CodeRabbit-style
reviews must run against one immutable plan-approval baseline. Each must report no
actionable P0–P2 finding; the current unreleased source is context, not accepted v2 behavior.
Preserve complete responses, session IDs, exact plan/source hashes, Engram references, and
disposition diffs in `docs/tests/2593/evidence/reviews/`. After approval, expected source
changes are captured and reviewed per RED/GREEN/REFACTOR pass and do not reset this counter.
Any plan/TP/architecture/approval drift, pre-approval source drift, or new actionable finding
resets the count to zero.

---

## 12. Test deliverables

| Deliverable | Location | Required content |
|---|---|---|
| This test plan | `docs/tests/2593/TEST_PLAN.md` | Frozen IEEE 829 strategy, denominators, mappings and execution matrix |
| Active bounded implementation plan | OpenChamber knowledge plan `44585f5b-5673-4cd9-ad59-7b0ba98dff66` at `/Users/jgil/.config/openchamber/projects/path_L1VzZXJzL2pnaWwvZ28vc3JjL2dpdGh1Yi5jb20vam9yZGlnaWxoL2t1YmVybmF1dA/plans/1794533840933-fmc-bounded-implementation-and-review.md` | **Only active implementation-plan authority** for this pre-release v1 slice: approval gates, internal protocol, public v1 contract, and review gate; exact current SHA-256 is external capture data |
| Historical external implementation plan (non-authoritative) | `1794436520127-fmc-resource-presence-and-ttl-recovery-tdd.md` | Historical context only; do not read, hash, require, compare, or use for approval/readiness |
| Unit test suite | `pkg/fleet/fmc/`, `pkg/fleet/scopecache/`, `pkg/fleet/registry/`, `pkg/shared/scope/`, `pkg/fleet/inventory/` | Ginkgo/Gomega behavior proof for 12 UT rows |
| Integration suite | `test/integration/fleetmetadatacache/`, Gateway, RO, AF, SP, AI and typed-scope fleet paths | Source dispatch proof for 4 IT rows and 19 wiring rows |
| Valkey/source evidence | `docs/tests/2593/evidence/valkey/` and `.../source/mcp-label-bearing/` | Reproducible internal TTL/status/fence behavior and label-bearing structured MCP test output; no qualification claim |
| Identity/GVK evidence | `docs/tests/2593/evidence/rbac/`, `.../kinds/`, `.../gvk/` | Rendered RBAC, target-kind matrix, generated CRD and handoff proof |
| Authentication E2E evidence | `docs/tests/2593/evidence/rbac/e2e/` | Real Kind TokenReview/SAR, 401/403/500 and identity/security-event proof, or approved risk acceptance |
| External Page evidence | `docs/tests/2593/evidence/af-page/` | #2584/#2600 Page invariants, exact wire JSON, command and owner/reviewer record |
| Review archive | `docs/tests/2593/evidence/reviews/` | Complete responses, hashes, Engram/source references and dispositions |
| Coverage/evidence report | CI or manually attached artifact | 16/16 source scenarios, 19/19 wiring, 15/15 critical, separate 0/1 E2E, 0/3 AF/FMC/Page, and 0/1 RO-audit gates, controls and no sensitive leakage; no qualification claim |

## 13. Execution commands

These commands are not run while the plan is Draft. After approval and implementation,
the owner must attach stdout/stderr and exit status for each affected command:

```bash
# Static/build gates
go build ./...
go test ./... -run=^$ -timeout=30s
golangci-lint run --timeout=5m
git diff --check

# Unit scenario denominator
go test ./pkg/fleet/fmc/... ./pkg/fleet/scopecache/... ./pkg/fleet/registry/... ./pkg/shared/scope/... ./pkg/fleet/inventory/... -ginkgo.v

# FMC integration and wiring
make test-integration-fleetmetadatacache
go test ./test/integration/fleetmetadatacache -ginkgo.v -ginkgo.focus='IT-FMC-(CACHE|PRES|AUTH)-'

# Cross-service wiring
go test ./test/integration/gateway/fleet -ginkgo.v -ginkgo.focus='IT-GW-PRES-004'
go test ./test/integration/remediationorchestrator -ginkgo.v -ginkgo.focus='IT-RO-PRES-005'
go test ./test/integration/apifrontend/fleet -ginkgo.v -ginkgo.focus='IT-(AF-PRES-007|AF-ROUTE-010)'
go test ./test/integration/signalprocessing/fleet -ginkgo.v -ginkgo.focus='IT-FMC-PRES-010'
go test ./test/integration/aianalysis/fleet -ginkgo.v -ginkgo.focus='IT-FMC-GVK-022'
go test ./test/integration/scope ./test/integration/fleet -ginkgo.v -ginkgo.focus='IT-SCOPE-TYPED-01(8|9|20|21)'

# Conditional control gate; requires explicit authorization and a real Kind cluster
go test ./test/e2e/fleet -ginkgo.v -ginkgo.focus='E2E-FMC-AUTH-018'

# Generated GVK artifacts
make manifests
make sync-embed
make generate
make gen-diff
```

The exact per-scenario commands and files are authoritative in Section 8.4; the exact
per-wiring commands and files are authoritative in Section 9.1.

## 14. Wiring verification

Checkpoint W passes only when all 19 implementation rows in Section 9.1 have a real
production caller and a passing IT that traverses the production construction/router
path. Unit tests do not count as wiring proof. Conditional AF/FMC inventory rows remain
explicitly unavailable and are not silently marked complete.

The reviewer must record, for each row: production entry symbol, implementation symbol,
test symbol/file, command, passing result, BR/control mapping, and artifact path. The four
typed-scope rows and AI/GVK row are required production seams, not duplicate credit for
the route rows. Any new component without a `cmd/` or handler production caller is a hard
failure.

## 15. Existing source/test consumers requiring updates

The following inventory was produced by exact fixed-string searches for
`/api/v2/scope/check`, `application/vnd.kubernaut.fmc.scope.v2+json`, `ScopeCheckPath`,
and `ScopeCheckMediaType`. The current dirty source/test consumers are **unaccepted v2
route/media assertions or wiring**, not v1 evidence. They are the RED input/acceptance list:
RED defines and tests the conversion against the approved public v1 contract (`GET
/api/v1/scope/check`, ordinary `application/json`, and `ScopeCheckResponse.Managed` /
`managed`), GREEN converts the unreleased source and all listed consumers atomically before
production-composition credit, and REFACTOR cleans up. Any typed
unavailable/verified response extension remains approval-gated. This documentation
amendment makes no source/test change and grants no v1 coverage credit to these rows
before conversion.

| Existing location | Current assertion | Required RED/GREEN update | Reason |
|---|---|---|---|
| `pkg/fleet/fmc/handler.go:34-35,76,91,209,228` | Current production route registration, v2 media constant, typed response, and media negotiation | Replace the dirty v2 route/media implementation with the approved v1 route, ordinary `application/json`, and `Managed` / `managed`; any typed extension requires prior approval | Public contract and fail-closed boundary |
| `pkg/fleet/fmc/http_client.go:164,173,284` | Current client builds `ScopeCheckPath`, sends `ScopeCheckMediaType`, and validates v2 media | Convert the client to the approved v1 route and ordinary JSON; preserve typed internal fail-closed handling without inventing a wire schema | Public client contract |
| `pkg/fleet/fmc/handler_test.go:104-105,150-258` | Literal v2 paths and v2 media through the shared request helper | Convert every route/media assertion to the approved v1 contract; retain fail-closed unknown/invalid behavior and do not claim current v1 coverage | Handler route/media proof |
| `pkg/fleet/fmc/http_client_test.go:64-70,83-299` | Literal v2 path and repeated v2 media response/Accept fixtures | Convert all fixtures and assertions to ordinary v1 JSON; any typed extension follows the approved schema only | Client contract and fail-closed safety |
| `pkg/fleet/fmc/r13_scope_test.go:31-75` | Current untracked R13-labeled source test uses literal v2 route/media and typed response assertions | RED defines/tests the approved v1 conversion; GREEN reconciles this dirty source test atomically with the source consumers before production-composition credit; do not count it as v1 evidence; preserve `R13.md` as immutable historical evidence | Historical test label does not authorize v2 |
| `pkg/fleet/scope_factory_test.go:238-376` | Current FMC factory tests return the v2 media type in five HTTP fixtures | Convert fixtures to the approved ordinary v1 JSON response; preserve transport/auth/TLS assertions and do not add a public media token | Factory/client wiring |
| `pkg/fleet/mcpclient/client_test.go` | One-page list, missing structured content treated as empty | Add multi-page continuation, label retention, malformed/missing structured-content and pagination-error cases; incomplete observation must be unavailable, never empty | Presence evidence and TTL safety |
| `pkg/fleet/fmc/syncer_test.go` | Positive-only `managed=true` filtering and namespace fallback assumptions | Assert full observed label states, incomplete cycle, no unobserved TTL refresh and label-bearing source evidence | Presence-before-inheritance |
| `pkg/fleet/scopecache/scopecache_test.go` | `InferGVK` fallback and miss-to-namespace fallback | Replace with explicit validated GVK, typed unavailable and no fallback on miss/error | GVK and fail-closed safety |
| `pkg/fleet/inventory/types_2584_test.go` | Permissive opaque IDs/Page states | External #2584 owner must add strict `r-` validation and `Page.Validate` invariant cases | Shared contract dependency |
| `pkg/fleet/registry/*_test.go` | Name-only registry update/delete behavior | Add UID mismatch/stale-event and same-name delete/recreate cases | Incarnation fencing |
| `test/integration/fleetmetadatacache/fmc_http_api_test.go:127-139` | Current constant-based FMC path/media caller and typed response decoder | Convert the dirty v2 path/media caller to the approved v1 ordinary-JSON route/body; retain only approved internal fail-closed behavior and no public v2/migration matrix | Production HTTP proof and v1 contract |
| `test/integration/gateway/fleet/gw_fleet_owner_chain_test.go` | Scope validation omits group/version before RR creation | Assert `NormalizedSignal.Resource.APIVersion` reaches the pre-RR `ResourceIdentity`; missing/ambiguous GVK blocks before lookup/RR | GVK propagation safety |
| `test/integration/remediationorchestrator/ro_fleet_scope_test.go:92-95` | Current FMC handler/HTTP integration fixture is documented as serving `fmc.ScopeCheckPath` (v2 typed scope) | Convert the fixture and its client path/media expectations to the approved v1 ordinary-JSON contract; no public v2 or migration evidence | RO-to-FMC public contract |
| `test/integration/remediationorchestrator/scope_blocking_integration_test.go` | Routing-block event assertion checks only discriminator/RR name | Assert persisted `RoutingBlockedData`, actor/action/outcome, correlation ID and complete routing reconstruction | External RO audit gate |
| `test/infrastructure/fleet_api_diagnostics_test.go:250-263` | Current diagnostic assertion resolves `fmc.ScopeCheckPath` and checks v2-shaped responses | Convert the endpoint path/media and response assertions to the approved v1 ordinary-JSON contract; no public v2 negotiation or migration evidence | Public v1 contract |
| `test/infrastructure/fleet_e2e.go:3920-3925` | Current E2E probe builds `fmc.ScopeCheckPath` and sends `fmc.ScopeCheckMediaType` | Convert the probe to the approved v1 route and ordinary JSON; E2E remains separately authorized and receives no migration credit | Public v1 contract; E2E authorization |
| `test/e2e/fleetmetadatacache/shared/helpers.go:38-63` | Shared helper calls the current constant route/media and decodes typed `ScopeCheckResponse` | Convert the serving-path helper to approved v1 ordinary JSON and `Managed` / `managed`; no v2 helper or old-route rejection requirement | Public v1 contract |
| `pkg/apifrontend/policyread/service.go:333-347` | Remote boolean scope result maps `false` to `NotFound` | Consume typed `ScopeResult`; only verified unmanaged maps to existing not-found behavior, while unavailable/unsupported/error is state-only with no row/allow/local fallback; prove through the planned AF production-path IT | AF typed-scope wiring |
| `pkg/aianalysis/handlers/analyzing.go` and `pkg/apifrontend/policyread/service.go` | AI target/history loses or infers GVK | Add explicit GVK serialization and remote missing-GVK unavailable tests | AIAnalysis GVK handoff |

The exact literal search found 20 current `/api/v2/scope/check` matches in four dirty
source/test files (`handler.go`, `handler_test.go`, `http_client_test.go`, and
`r13_scope_test.go`) and 15 current `application/vnd.kubernaut.fmc.scope.v2+json`
matches in three dirty source/test files (`handler.go`, `http_client_test.go`, and
`r13_scope_test.go`). The symbol search additionally found the constant-based consumers
listed above, including the FMC HTTP integration, factory, RO, diagnostics, E2E, and
infrastructure paths. Comment-only symbol mentions in
`test/e2e/fleetmetadatacache/{suite_test.go,eaigw/suite_test.go}` and
`docs/architecture/decisions/DD-FLEET-004-fmc-ping-clusters-endpoint.md` are not v1
evidence; the literal mention in historical `docs/tests/2593/evidence/reviews/R5.md`
is archive context only and is not edited.

No existing test is deleted to improve a denominator. Tests are amended only when the
approved contract changes; all scenario IDs remain visible.

## 16. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-11-11 | Initial Draft IEEE 829 artifact; historical review-only boundaries and 16-row matrix |
| 0.2 | 2026-11-12 | **Historical**: added the superseded 17/17 denominator and migration row, exact execution/wiring matrices, IEEE sections, UID/GVK/API/auth/Page gates, pinned links and evidence commands |
| 0.3 | 2026-11-12 | Frozen after R8 amendments: executable MCP pagination/structured-content contract, pre-RR Gateway GVK proof, RO schema/codegen/producer ownership, complete P0 specifications, deterministic 404/503 mapping, dual-hash review rule, and exact external AF/FMC/Page gates |
| 0.4 | 2026-11-12 | **Historical**: R9 amendment added exact MCP continuation and typed-result bridge, durable Lease-derived Valkey fence with measurable server-time expiry, and a superseded five-caller v1 migration inventory; AF policyread typed-scope wiring and external AF/FMC/Page registration contract remain only where restated in v0.8 |
| 0.5 | 2026-11-12 | R10 amendment: concrete MCP response/result and Go bridge signatures, consistent Pod/session/Lease/fence identity, one Lua/EVALSHA expiry protocol with lost-reply handling, and deterministic repair authority/order; denominators and fail-closed boundaries unchanged |
| 0.6 | 2026-11-12 | R11 amendment: presence-aware labels, duplicate/scope rejection, package-safe MCP/Syncer/Lease/status composition, EVALSHA artifact/NOSCRIPT/TTL recovery, and bounded repair linearization; denominators and fail-closed boundaries unchanged |
| 0.7 | 2026-11-12 | R12 amendment: invalid labels are unavailable/non-inheriting, public six-field canonical IDs are separated from internal registration fencing, `WithCurrent` requires expected registry revision, and conditional server `0/2` is explicitly a subset of aggregate AF/FMC/Page `0/3`; denominators and fail-closed boundaries unchanged |
| 0.8 | 2026-11-13 | **Historical draft amendment**: user-approved pre-GA v1/no-migration contract; public `/api/v1/scope/check`, ordinary `application/json`, and `ScopeCheckResponse.Managed`; typed v1 extension approval gate; `IT-FMC-MIGRATION-017` retained as N/A/out-of-scope with zero credit; active denominators are 12 UT, 4 IT, 16 overall, 15 critical, and 19 wiring; R13 remains immutable historical evidence |
| 0.9 | 2026-11-13 | **Fast-path implementation baseline**: user-approved internal `PartitionStateKey`/tokenized repair/epoch-scoped `LeaseSession` contract; `BeginRepair` precedes MCP collection; `Repair`/`CommitRepair`/proof-bearing `ReadVerified` replace old internal repair/read signatures with no compatibility shim; bounded EVALSHA guards and complete repair ordering are authoritative; public FMC remains v1/no-migration; denominators unchanged; R13 remains immutable; clean-worktree carry/hold boundary recorded |

## 17. Current status

- RED: **Not started**
- GREEN: **Pending RED and production-wiring proof**
- ACM inventory: **Unavailable / no rows**
- AF non-local Console list/policy scope: **Unavailable**
- Latest review round R13: **Non-clean** (7 revalidated P0 and 5 revalidated P1; no fresh v0.7 contradiction); see immutable `docs/tests/2593/evidence/reviews/R13.md`
- This v0.9 amendment invalidated the historical dirty-worktree clean-review eligibility; the fast-path carry/hold boundary is recorded in `docs/tests/2593/evidence/FAST_PATH_CARRY_HOLD.md` and the review README. No new three-review cycle is claimed.
- Historical three-clean-review counter: **0/3**
- Implementation readiness: **RED authorized in the clean worktree; final qualification remains pending**

No public v2 route/media, deployment, cluster, ACM, Valkey qualification, GitHub, or release
changes are authorized. Source changes are limited to the clean worktree and the bounded
TDD steps in the active OpenChamber plan.
