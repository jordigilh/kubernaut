# Test Plan: FMC fleet inventory projection with ACM Search source adapter

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2661-v2.0
**Feature**: FMC-served, bounded fleet inventory projection; ACM Search as
one FMC source adapter; removal of the ACM Console inventory release gate
**Version**: 2.0 (supersedes TP-2661-v1.0, AF→ACM direct)
**Created**: 2026-11-18
**Author**: Kubernaut Team
**Status**: 📝 Draft v2. DD-FLEET-017 D1–D6 and the ADR-068 amendment were
approved 2026-11-18; this plan awaits its own approval. No test or implementation
result is claimed.
**Issue**: [#2661](https://github.com/jordigilh/kubernaut/issues/2661)
(parent [#2593](https://github.com/jordigilh/kubernaut/issues/2593),
investigation [#2599](https://github.com/jordigilh/kubernaut/issues/2599))
**Authority**: [DD-FLEET-017 v2](../../architecture/decisions/DD-FLEET-017-fleet-inventory-projection.md)
and [SPIKE_2026-11-18](SPIKE_2026-11-18.md)
**Refinement repaired (2026-11-18)**: v1.0 assumed `AF → ACM Search` directly.
The recorded 2026-11-11 #2593 decision requires `AF → FMC` only, with FMC owning
the source. All testing below follows that boundary. Corrected after independent
review: the cache is **not** managed-only (it holds every observed object with
its label state), per-kind completion is **not** in the proof today, and a
per-kind error aborts the whole collection.

---

## 1. Introduction

### 1.1 Purpose

Prove that the Console receives a bounded, sanitized fleet inventory from FMC
alone; that FMC's projection makes only claims its proof state supports; that
ACM Search collection happens at sync time under the recorded traversal and
identity rules; and that the release gate is removed only against a qualified
production path.

### 1.2 Objectives

1. FMC serves `inventory.Page`/`Resource` for list and exact read, with
   `fresh`/`stale` derived from proof state and `empty` only from a proven
   partition.
2. An incomplete or truncated collection is never published as a clean
   partition and never appears as a complete page.
3. The ACM source adapter observes **objects** (the same cache shape the MCP
   collector produces) with exact GVK, ADR-053 precedence resolved at read time,
   uid-ordered offset traversal with mutation guards and the 60 s staging
   window, and publishes only through the fenced/staged cache path — including
   the `empty_verified` path for a zero-item collection.
4. AF dispatch is backend-neutral and qualified: AF never contacts ACM Search,
   MCP or remote Kubernetes for inventory, and never falls back to local.
5. The GitOps (MCP-sourced) lane keeps working and gains the same inventory
   surface without any ACM dependency.

### 1.3 Success metrics

| Metric | Target |
|---|---|
| In-scope unit scenarios proven | ≥ 90%, critical 100% |
| Wiring rows proven through production dispatch | 7/7 |
| Live control objectives assessed (AC-4, AC-6, SI-10; ASVS V5/V8) | all, on the recorded clusters only |
| Regressions in local inventory, scope lane, FMC scope checks | zero |

## 2. References

- DD-FLEET-017 v2 (contract; D1–D6 and the ADR-068 amendment approved 2026-11-18),
  DD-AF-018 (Console read boundary and states),
  DD-FLEET-011 (target-read policy and identity), DD-FLEET-012/013/016 (writer
  coordination, proof lifecycle, staged commit), DD-FLEET-015 (no target-UID
  claim), ADR-053 (label precedence), ADR-068 with its approved peer→facade amendment.
- `docs/tests/2661/SPIKE_2026-11-18.md` (ACM traversal, mutation, SDL).
- `docs/tests/2593/TEST_PLAN.md` §4.2 (this slice was excluded there) and
  `docs/tests/2584/TEST_PLAN.md` §16.3/§16.4 (prior adapter and containment
  baseline).
- BR-SCOPE-001/002, BR-API-001, BR-SECURITY-016/017, BR-INTEGRATION-065,
  BR-SAFETY-001.

## 3. Risks and mitigations

| ID | Risk | Impact | Priority | Proving scenario |
|---|---|---|---|---|
| R1 | Unproven/incomplete collection served as a complete page | False inventory, missed managed state | P0 | UT-FMC-SRC-2661-014..017, IT-FMC-INV-2661-002 |
| R2 | `empty` from a stale or unproven partition | "Nothing managed" while resources exist | P0 | UT-FMC-INV-2661-002/003, E2E-FMC-INV-2661-002 |
| R3 | Freshness invented (TTL or query time treated as proof) | Misleading compliance metadata | P0 | UT-FMC-INV-2661-003/007 |
| R4 | AF contacts a source directly, or re-introduces source selection | Second federation path | P0 | IT-AF-2661-001, plus a negative assertion that no ACM/MCP client is constructed in AF for inventory |
| R5 | Truncated or reordered ACM walk publishes a smaller clean set | Silent inventory loss | P0 | UT-FMC-SRC-2661-015..017 |
| R6 | Wrong identity: inferred GVK, cross-cluster item, `_uid` leaked | Wrong decision surface | P0 | UT-FMC-SRC-2661-013/018 |
| R7 | Cache page reads bypass proof/fence (e.g. staging keys served) | Inconsistent or unproven reads | P0 | UT-FMC-INV-2661-003/004, IT-FMC-INV-2661-002 |
| R8 | GitOps lane regresses or gains an ACM dependency | Primary deployment broken | P0 | IT-FMC-SRC-2661-004, E2E-FMC-INV-2661-002 |
| R9 | Cursor drift reported as caller error | Read outage on ordinary churn | P1 | UT-FMC-INV-2661-005 |
| R10 | Denied identity reaches FMC or Search | Unauthorized read | P0 | IT-AF-2661-003 |

Each P0 needs at least one passing proving scenario at the listed tier.

## 4. Scope

Tested: `pkg/fleet/inventory` (page/identity/freshness validation),
`pkg/fleet/fmc` (inventory route, page reader, ACM source adapter, sync
selection), `pkg/fleet/scopecache` (proof-gated reads) as consumed,
`pkg/apifrontend/policyread` + `handler` (dispatch, qualification, SAR, audit),
`api/apifrontend/openapi/apifrontend-v1.yaml` only if a documented state
description changes.

Not tested here: GW/RO/AF scope rerouting and `NewScopeChecker`'s ACM branch,
Rancher or further sources, per-user visibility/token delegation (#2600),
remote policy-scope listing, Console rendering, new caller-facing routes, totals,
snapshots. FMC's internal inventory route is in scope (wiring rows 1–2).

## 5. Approach

Ginkgo/Gomega. Real `pkg/` logic. Fake only external dependencies: ACM Search
over `httptest`, Valkey via the approved test dependency or the real instance in
IT, Kubernetes via `envtest`/fake clients, MCP readers via fixtures. No
`testing.T` business tests, no `XIt`/`PIt`/`Skip()`.

**Pass criteria.** All listed scenarios green; 7/7 wiring rows through production
dispatch; every P0 passing; `go build ./...` and `golangci-lint run` clean; local
inventory, FMC scope and GW/RO scope suites unchanged; one live ACM journey and
one GitOps journey executed with results recorded verbatim.

**Fail criteria.** A clean proof published for an incomplete kind; `empty` from
an unproven partition; any freshness value not backed by proof; AF constructing
a source client; `_uid` on the wire; a staging key served as live; a denied SAR
reaching FMC; a live claim made from fixtures.

**Suspension.** Suspend if the deployed ACM build changes traversal or count
semantics, if a registered cluster's Search visibility cannot be established, or
if serving the page would require weakening the fenced proof lifecycle.
Membership is settled (registration CR only, DD-FLEET-017 D1.3a).

## 6. Test items

| Area | File under test |
|---|---|
| Shared contract | `pkg/fleet/inventory/types.go` |
| FMC route | `pkg/fleet/fmc/handler.go`, new inventory handler |
| FMC page reader | new reader over `pkg/fleet/scopecache` index/records + proof gate |
| ACM source adapter | new adapter under `pkg/fleet/fmc` |
| Sync selection | `pkg/fleet/fmc/syncer.go`, FMC startup wiring |
| AF dispatch | `pkg/apifrontend/policyread/service.go`, `types.go` |
| AF HTTP/audit | `pkg/apifrontend/handler/console_policyread.go`, `router.go` |

**Version identification**: ACM 2.17.4 / `acm-search-v2-api-rhel9@sha256:85c53e64…`
as recorded in the spike; FMC chart version and Valkey version captured with the
live run.

## 7. BR coverage matrix

| BR | Contract | Scenarios |
|---|---|---|
| BR-SCOPE-001 | Managed-only inventory with ADR-053 precedence | UT-FMC-SRC-2661-011/012, IT-FMC-SRC-2661-003 |
| BR-SCOPE-002 | Exact GVK, canonical identity, no inference | UT-FMC-SRC-2661-013/018, UT-FMC-INV-2661-008 |
| BR-API-001 | Typed, sanitized page, states and freshness | UT-FMC-INV-2661-001..010, IT-AF-2661-001/002 |
| BR-SECURITY-016 | Least privilege, user gate, no privileged widening | UT-FMC-INV-2661-010, IT-AF-2661-003 |
| BR-SECURITY-017 | Metadata-only egress | UT-FMC-INV-2661-010, IT-FMC-INV-2661-001 |
| BR-INTEGRATION-065 | Source abstraction and dispatch | IT-FMC-SRC-2661-001/002/004, IT-AF-2661-001 |
| BR-SAFETY-001 | Fail-closed proof, no fallback | UT-FMC-INV-2661-003/004, UT-FMC-SRC-2661-015..017, IT-AF-2661-002 |

## 8. Test scenarios

### Tier 1a — FMC page contract (`pkg/fleet/fmc`, `pkg/fleet/inventory`)

| ID | Scenario | Critical |
|---|---|---|
| UT-FMC-INV-2661-001 | Proven partition, items present → `success`, `freshness=fresh`, cursor when more remain | ✅ |
| UT-FMC-INV-2661-002 | Proven partition with `empty_verified=1`/`observed_count=0` and **no indexes at all** → `empty` + `fresh`; a missing index *without* that proof → `unavailable` (never "nothing exists") | ✅ |
| UT-FMC-INV-2661-003 | Proof ageing: with the staleness threshold strictly between the sync interval and the cache TTL (test config 30 s / 40 s / 45 s), a failed kind yields `stale` then `unavailable` — **not** `partial`, which needs per-kind state we do not have | ✅ |
| UT-FMC-INV-2661-004 | Proof missing, expired, staging-only, or source unqualified → `unavailable` state-only | ✅ |
| UT-FMC-INV-2661-005 | Cursor present → resume after it; cursor gone because the cache moved on → re-run the sorted projection from the start (200, no `400`) | ✅ |
| UT-FMC-INV-2661-012 | **Lex ordering**: pages follow the identity key (namespace → kind → group → version → name → ID) and are stable across requests; deterministic, deliberately not `SortResources` | ✅ |
| UT-FMC-INV-2661-013 | **Prefix filters**: `namespace`, `namespace+kind`, `namespace+kind+name` are range scans returning exactly the match set; a kind-only query (scan path) still returns the correct set | ✅ |
| UT-FMC-INV-2661-014 | A partition whose lex index is **missing** fails closed as `unavailable`, is never served as empty, and becomes readable after the next clean commit | ✅ |
| UT-FMC-INV-2661-015 | **Bounded proof re-resolve** (D2.5b): a commit superseding the page proof mid-read is invisible (one retry, then `success`); a second failure returns `unavailable` with exactly two attempts, never a loop | ✅ |
| UT-FMC-INV-2661-006 | Malformed cursor → invalid filter (the only `400`) | — |
| UT-FMC-INV-2661-007 | `Page.Validate()` rejects empty/unknown freshness state and a zero `as_of`; accepts `fresh`/`stale` | ✅ |
| UT-FMC-INV-2661-008 | Exact GET: canonical match; ID mismatch → `unavailable`; absent → `not_found`; cross-cluster ID → `not_found` | ✅ |
| UT-FMC-INV-2661-009 | Bounds: limit/cursor/filter validation, retained-projection saturation → `partial`; a partition above the proof ceiling (5000 items / 100 pages) can never publish clean and reads `unavailable` | ✅ |
| UT-FMC-INV-2661-010 | Payload is metadata-only: no labels beyond the scope projection, no annotations, object data, secrets or tokens | ✅ |
| UT-FMC-INV-2661-011 | **Managed projection**: a cache containing `true`, `false`, `absent` and `invalid` records yields exactly the `Included` set (asserted by identity); inheritance comes from the shared resolver — asserted by a namespace-inherited item appearing while a `false` override does not | ✅ |

### Tier 1b — ACM Search source adapter (`pkg/fleet/fmc`)

| ID | Scenario | Critical |
|---|---|---|
| UT-FMC-SRC-2661-011 | Observation shape: every object of a configured kind is written with its label state (`true`/`false`/`absent`/`invalid`), matching the MCP collector's cache shape so both sources project identically | ✅ |
| UT-FMC-SRC-2661-012 | Precedence: explicit `false` overrides namespace `true`; invalid label is invalid, never absent, never inherits; cluster-scoped never inherits | ✅ |
| UT-FMC-SRC-2661-013 | Identity: missing `apiversion`, cross-cluster item, or two items mapping to one canonical ID fails the kind (not just the item) | ✅ |
| UT-FMC-SRC-2661-014 | Traversal: explicit `orderBy uid asc`, offset chunking, strictly increasing `_uid`, stable `count` → complete collection | ✅ |
| UT-FMC-SRC-2661-015 | `count` changes mid-walk → exactly one restart from offset 0, then complete; a second change → incomplete | ✅ |
| UT-FMC-SRC-2661-016 | Duplicate/non-increasing `_uid`, non-final short page, or `sum(items) != count` → incomplete | ✅ |
| UT-FMC-SRC-2661-017 | A per-kind error aborts before any write: no staging write, no commit, the previous proven generation stays readable, and no smaller clean set is published | ✅ |
| UT-FMC-SRC-2661-018 | `_uid` is used only for ordering/dedupe: absent from the cache projection, the page and the canonical ID | ✅ |
| UT-FMC-SRC-2661-019 | A successfully traversed zero-item collection is recorded as an **applied observation** (never `unavailable`, never the failure envelope) and commits through `empty_verified`; the page layer, not the adapter, decides `empty` vs `partial` (G10) | ✅ |
| UT-FMC-SRC-2661-020 | One atomic commit writes **both** indexes from the same staged generation: member counts must agree (`LEX_COUNT_MISMATCH` otherwise), the next `begin` clears an abandoned staging's lex index, both enforce the 10 000-member bound, and an empty commit removes both | ✅ |

### Tier 2 — Integration

| ID | Scenario | Critical |
|---|---|---|
| IT-FMC-INV-2661-001 | Production mux serves list and exact read; response is the shared sanitized schema; unknown route/method behaviour unchanged | ✅ |
| IT-FMC-INV-2661-002 | End-to-end over real cache semantics: proven partition → `success`/`empty`; broken/staged/incomplete partition → `partial`/`unavailable`, with no staging key ever served | ✅ |
| IT-FMC-SRC-2661-001 | The ACM adapter runs inside the real sync cycle and writes through the fenced/staged path (no direct Valkey writes) | ✅ |
| IT-FMC-SRC-2661-002 | Source selection: an MCP-sourced cluster keeps its existing collector and never calls Search; an ACM-sourced cluster uses the adapter; an unknown source is `unavailable`, not guessed | ✅ |
| IT-AF-2661-001 | AF list dispatch reaches the endpoint from `fleet.fmc.endpoint` (asserted by the request the fixture receives, including with `fleet.backend: acm`) and returns the FMC page — the inventory path never inherits the scope adapter's endpoint | ✅ |
| IT-AF-2661-002 | AF maps `partial`/`unavailable` → `503` state-only, `forbidden` → `403`, `not_found` → `404`; no rows or cursors leak | ✅ |
| IT-AF-2661-003 | Namespace-list SAR denial → `403` state-only with zero requests to FMC; one typed audit event per read with `result_state`, no records | ✅ |

### Tier 3 — Live / E2E

| ID | Scenario |
|---|---|
| E2E-FMC-INV-2661-001 | ACM lane on helios08: FMC collects from ACM Search with the least-privilege hub principal, AF lists/pages/reads one exact resource, `freshness=fresh` from proof, sanitized payload, no fallback. Assesses AC-4, AC-6, SI-10 and ASVS V5/V8 for this path. |
| E2E-FMC-INV-2661-002 | GitOps lane: an MCP-sourced cluster produces the same inventory surface with no ACM present, proving the source abstraction and that `empty` is only emitted from a proven partition. |

## 9. Key test-case detail

**UT-FMC-INV-2661-003 (failed kind → no proof).** Cache where a `Pod` sync failed,
so no new clean generation was committed. Assertions: while the previous proof is
inside the budget the page reads `stale`; once it expires it reads `unavailable`
state-only. A `partial` page is **not** expected — that state requires per-kind
proof data the system does not have. No kind names or counts in any response.

**UT-FMC-SRC-2661-017 (abort, no smaller clean set).** Fixture returns a truncated
Pod kind. Assertions: the collection returns before any staging write; no
`commit_swap`; the previous proven generation still reads while inside the
budget; no page ever shows the reduced set as `complete`.

**IT-AF-2661-001 (no direct source).** Construct the AF dependency graph for a
`fleetmetadatacache` deployment and assert the constructed inventory reader is
the FMC client, with no ACM Search endpoint, no MCP reader and no remote
Kubernetes client reachable from the inventory path.

**IT-AF-2661-003 (SAR).** Authorizer grants `kubernaut_list_managed_resources`
but denies `list/core/v1/namespaces`. Assertions: `403`, `forbidden`, FMC
fixture sees zero requests, exactly one audit event with `result_state=forbidden`.

## 9b. Tests retired or re-scoped by this plan

Two existing specs assert behaviour this plan intentionally changes. They are
**task items, not regressions**:

| Test | Today | Required change |
|---|---|---|
| `UT-AF-2592-REMOTE-FAIL-CLOSED-001` (`pkg/apifrontend/policyread/interim_contract_2592_red_test.go`) | Pins "non-local read rejected, no remote reader called" | Re-scope to the new boundary: non-local dispatch reaches FMC when the endpoint is configured and never falls back to local. The old expectation is replaced, not weakened. |
| `UT-FLEET-2584-ACM-EMPTY-001` (`pkg/fleet/acm/inventory_2584_red_test.go`) | **Already narrowed by its author (2026-11-18)**: asserts only "an applied zero-item Search is an empty observation, not unavailable"; no `Page.Validate()`/completeness claim, no AF read boundary | No re-scope needed. This plan adds the adapter-side shape assertion in `UT-FMC-SRC-2661-019` (applied zero-item observation, never a failure envelope) and keeps the state decision in `UT-FMC-INV-2661-002/003` (page assembly). |

## 10. Wiring Manifest

Identical to DD-FLEET-017 §8 (single authority).

| # | Component | Production entry point | Wiring location | IT ID |
|---|---|---|---|---|
| 1 | FMC inventory route | `GET /api/v1/inventory/managed-resources` | `pkg/fleet/fmc/handler.go` `RegisterRoutes` | IT-FMC-INV-2661-001 |
| 2 | Cache-backed page/cursor | same route | FMC page reader over index + proof gate | IT-FMC-INV-2661-002 |
| 3 | ACM Search source adapter | FMC sync cycle | new adapter under `pkg/fleet/fmc` | IT-FMC-SRC-2661-001 |
| 4 | Source selection per cluster | FMC sync cycle | adapter selection table (MCP / ACM) | IT-FMC-SRC-2661-002 |
| 5 | AF→FMC dispatch + qualification | `GET /api/v1/console/inventory/managed-resources` | `policyread.Service.ListInventory` / `readerFor` | IT-AF-2661-001 |
| 6 | AF partial/unavailable mapping | both inventory routes | handler `projectionStatus` | IT-AF-2661-002 |
| 7 | Namespace-list SAR + typed audit | both non-local reads | `policyread` authorizer / `emitRead` | IT-AF-2661-003 |

## 11. Implementation sequence and TDD budget

| Phase | Work | Budget |
|---|---|---|
| 0 | ADR-068/DD-FLEET amendment drafted and approved; SDL provenance recorded; paginated Search document added as a **new** constant | 60–90 min (excluding approval latency) |
| RED a | UT-FMC-INV-2661-001..014 (page contract, proof gate, managed projection, cursor drift, lex ordering/prefix/missing index) | 130 min |
| GREEN a | FMC inventory route + page reader + `Page.Validate()` freshness checks | 120 min |
| RED b | UT-FMC-SRC-2661-011..019 (collector) | 90 min |
| GREEN b | ACM source adapter + sync selection + fenced publication incl. the lex identity key in the staged commit | 180 min |
| RED/GREEN c | IT-FMC-* and IT-AF-2661-001..003; AF dispatch + SAR + status mapping | 120 min |
| REFACTOR | Collapse the two page-assembly paths into one projection helper; single state/status table; no new types | 45 min |
| Validate | affected UT/IT with `-race`, build, repo compile, full lint, local/FMC/scope regressions | 60–90 min |
| Live | E2E-FMC-INV-2661-001/002, evidence captured | 90–120 min |

Total ≈ 12–15 h plus CI. The lex index adds ~1.5–2.5 h of code and, critically,
**re-qualification of the DD-FLEET-016 atomic swap** (~0.5–1 day of IT/live
evidence) because it changes the committed keyspace.

## 12. Rollback

Two independent, atomic reverts: (a) re-gate AF dispatch (the rejection returns,
FMC keeps serving internally); (b) disable the ACM source adapter so the
partition falls back to its previous proven state or `unavailable`. No
compatibility mode, no data migration.

## 13. Evidence boundary (not claimed)

No certification, no claim about ACM builds other than the recorded one, no
per-user remote visibility (#2600), no offset-snapshot guarantee, no Rancher or
further sources, no remote policy-scope, no Console rendering evidence, no live
result before the run. The peer model in current code and ADR-068 is not
migration-tested by this plan; the amendment is a prerequisite, not a test item.
