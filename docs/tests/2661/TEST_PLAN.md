# Test Plan: FMC fleet inventory projection with ACM Search source adapter

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2661-v2.0
**Feature**: FMC-served, bounded fleet inventory projection; ACM Search as
one FMC source adapter; removal of the ACM Console inventory release gate
**Version**: 2.0 (supersedes TP-2661-v1.0, AF→ACM direct)
**Created**: 2026-11-18
**Author**: Kubernaut Team
**Status**: 📝 Draft v4.7 (round-24 pre-implementation review: §10's preamble
matches RED/GREEN c and §12 — the DD-FLEET-017 §8 amendment already shipped;
RED/GREEN c verifies the identity, it does not re-amend). **Live rows proven
(2026-11-22):** E2E-FMC-INV-2661-001 ran live on helios08 against the recorded
ACM 2.17.4 build — 4/4 Its, evidence verbatim in
[LIVE_EVIDENCE_2026-11-22.md](LIVE_EVIDENCE_2026-11-22.md); E2E-FMC-INV-2661-002
is shipped as shared scenario `-017` in both FMC E2E lanes (CI-runnable) and
was run focused on the Kuadrant lane: 4/4 Its green, `SUCCESS! -- 4 Passed |
0 Failed` in 821.6 s (verbatim record in
[LIVE_EVIDENCE_2026-11-22.md](LIVE_EVIDENCE_2026-11-22.md)). All Tier 1-3 rows are now ✅. Draft v4.6 (round-23 pre-implementation review: ADR-068's separate
2026-11-03 qualification note — "ACM Console inventory remains disabled until …
qualified" — joins the amendment set and §12(a)'s revert list; phase c
discharged the condition, enforcement being FMC's proof-gated 503 plus the
fail-closed `fleet.fmc.endpoint` wiring). Draft v4.5 (round-22 pre-implementation review: the DD-FLEET-017
amendment extends beyond row 4a — its §1.3 state rows (dispatch no longer
blocked, inventory no longer gated, peer-model count reduced) and D6.2's
qualification mechanism are falsified by phase c and get a dated note
recording that enforcement is FMC's proof-gated 503 plus the fail-closed
`fleet.fmc.endpoint` wiring; DD-FLEET-011's "keep the inventory gate closed
until qualified" directive joins the amendment set; the REMOTE-LIST-003
rationale no longer calls the group-ful service-lane fixture "core-only";
§4's OpenAPI clause covers response-contract entries, matching RED/GREEN c).
Draft v4.4 (round-21 pre-implementation review: §12(a)'s OpenAPI
revert clause enumerates the structural additions (the exact-read 403 entry,
the 404 correction, the 503 rewordings), not just "state descriptions";
DD-FLEET-015's 2026-11-18 amendment ("closed until … qualified") joins the
amendment set and revert list; the backend_deps.go DD-AF-018 site is named
as the log-only gate site it is, not a "skip site"). Draft v4.3 (round-20 pre-implementation review: ADR-068's
in-force 2026-11-18 amendment — the "ACM Console inventory stays closed" bullet
and the "four places" peer-model consequence — is falsified by the gate removal
and joins the RED/GREEN c amendment set and §12(a)'s revert list). Draft v4.2
(round-19 pre-implementation review: the OpenAPI work
extends beyond description rewrites where the contract itself is falsified —
the exact-read route gains its previously undocumented wire-real `403`
response entry and a corrected 404 description ("absent or *local* target
access denied"), and the list/policy-scope routes' 503 descriptions drop
"local" ("Required source…", matching the exact-read route) since a non-local
503 now arises from the remote FMC source). Draft v4.1 (round-18 pre-implementation review: the
`pkg/apifrontend/policyread/service.go` doc comments that the shared
`authorize()` change falsifies — `ListInventory`'s "capability and local
target-scope authorization" and `GetInventory`'s local-denial→not_found note —
join the RED/GREEN c re-expression list and §12(a)'s restore list). Draft v4.0
(round-17 pre-implementation review: the OpenAPI
rewrite set gains its fourth surface — the policy-scope route's "non-local
policy-scope requests return a state-only `unavailable` response" is
falsified by the same shared `authorize()` SAR, which changes a denied
non-local `ListPolicyScope` to `403 forbidden` while the listing itself stays
unsupported). Draft v3.9 (round-16 pre-implementation review: the
REMOTE-LIST-002 partial-case rationale corrected — the 503 is wire-realistic
because FMC's `inventoryWireStatus` maps partial→503 server-side; a 200
state-only partial passes `Page.Validate` and would surface as `partial`,
which is why the fixture must use the 503). Draft v3.8 (round-15 pre-implementation review: the OpenAPI
rewrite set gains the exact-read route description — the shared `authorize()`
SAR falsifies its local-vs-remote contrast; the REMOTE-LIST-002 partial case
names its wire-realistic mechanism (stub serves 503 state-only `partial`
mirroring `inventoryWireStatus`; a 200 partial is rejected by the FMC client's
own `Page.Validate`) and its terminal Console state; the DD-FLEET-017 row-4a
amendment is marked already-shipped (#2691) — RED/GREEN c verifies it, and
§12(a) restores it by a separate documentation edit outside the atomic code
unit; the Tier 1c `fleet.backend: fmc` wording corrected to
`fleetmetadatacache`). Draft v3.7 (round-14 pre-implementation review: RED/GREEN c now
amends DD-AF-018 with a dated inventory-lane-gate-removal note and updates the
decisions README index row, and rewrites the now-false OpenAPI state
descriptions (`apifrontend-v1.yaml` list route and `ConsoleClusterID`) — §4's
"only if a documented state description changes" condition fires for exactly
these; §12(a) reverts all three documentation surfaces with the code unit).
Draft v3.6 (round-13 pre-implementation review: §7 carries the
remaining three §9b re-scope IDs — `IT-AF-2584-008`, the `IT-AF-2584-005`
probe, `IT-AF-ROUTE-010`/`IT-AF-PRES-007`; the REMOTE-LIST-002 row names both
of its re-expressed cases (success dispatch and a `partial` state-only page →
503, keeping the no-leak assertions in spirit); the 005 probe row requires
the canonical ID regenerated for the prod-east identity; a stale
`service.go:234` citation corrected to `:235`). Draft v3.5 (round-12 pre-implementation review: the
`UT-AF-2592-REMOTE-FAIL-CLOSED-001` row now requires a fully contract-valid
served page — ≥1 item, cluster attribution, `scope.included=true`, canonical
ID — the current fixture is a zero-item success page that `validateSuccess`
rejects; `UT-AF-2661-004` gains the non-local `ListPolicyScope` denial arm so
every arm of the shared `authorize()` branch has a proving scenario; §12(a)
reverts the DD-FLEET-017 §8 row-4a wording; §7 now carries the §9b re-scoped
spec IDs). Draft v3.4 (round-11 pre-implementation review: DD-FLEET-017 §8 row
4a re-synced to the plan's wording and its amendment added to RED/GREEN c; the
REMOTE-LIST-001/-003 rows name their terminal states and -003's audit
assertion is re-expressed to `result_state=success` rather than deleted; every
re-scoped FMC-wire fixture row requires `scope.included=true` on served items;
the GREEN slice's CA-reloader error attribution uses the configured backend
name). Draft v3.3 (round-10 pre-implementation review: the
freshness-envelope requirement extends to all three REMOTE-LIST rows; §9b adds
the `IT-AF-2584-008` re-scope — it pins the AF→ACM direct exact-read lane this
plan's fail criteria forbid, and only stays green because it wires the ACM
client by hand; `UT-AF-CFG-2661-001` now proves endpoint *selection* at the UT
tier, driving one read onto a request-capture FMC fixture with the scope
adapter's endpoint receiving nothing; a malformed-`fleet.fmc.endpoint`
validation scenario is recorded as deferred in §13 rather than expanding the
Tier 1c slice). Draft v3.2 (round-9 pre-implementation review: §7 now covers the
eleven previously unmapped §8 IDs (UT-FMC-SRC-2661-014/019/020/024/025/026,
IT-FMC-SRC-2661-005/006/007, both E2E rows); the REFACTOR cross-package
page-assembly dedup is dropped — a helper shared by two packages would be a
new component, forbidden by the phase's own rule — leaving only the in-package
`projectionStatus`/`projectionMessage` collapse; IT-AF-2661-004's
misspelled-key arm names its mechanism (the production loader's
`yaml.Unmarshal`, not struct literals); UT-AF-2661-004's exact-get rationale
states the real pre-phase-c baseline (dispatch already happens today); the
ROUTE-010 row no longer claims non-inheritance proof for a same-URL fixture).
Draft v3.1 (round-8 pre-implementation review: the
`IT-AF-2584-ACM-FAIL-CLOSED` row names which config drives the nil-reader read
assertions — only the absent-key one — closing a wrong-reason-pass path; the
stale `buildPolicyReadDeps` doc comment joins the RED/GREEN c re-expression and
§12's restore list; the fleet-mode deployment-config renderer gap is carved out
to #2695 and recorded in §13, keeping this plan inside its original
requirements). Draft v3.0 (round-7 pre-implementation review: the
`UT-AF-2592-REMOTE-FAIL-CLOSED-001` re-scope names its terminal state and
requires a contract-valid freshness envelope on the re-scoped stub page
(`Page.Validate` would otherwise keep the old `unavailable` alive for the
wrong reason); wiring row 4a locates the `fleet.fmc.endpoint` consumption in
`pkg/fleet/inventory_reader.go` (backend_deps.go is the unchanged production
caller and owns the logs); `UT-AF-2661-004` also proves the SAR denial on a
non-local exact get — `authorize()` is shared, so the branch changes that path
from `unavailable` to `forbidden`). Draft v2.9 (round-6 pre-implementation review: §12(a) deletes
IT-AF-2661-001/003 (and -002 if its forbidden case rides the SAR branch) with
the code unit — they are red against reverted code; the IT-AF-2584-005 probe
re-scope names its terminal state (a prod-east-attributed success page → 200,
not an unchanged 503 surviving via cross-cluster item mismatch) and the three
REMOTE-LIST rows pin cluster attribution on served items; §7 now covers
UT-FMC-INV-2661-011..015 and IT-FMC-INV-2661-001/001W). Draft v2.8 (round-5 pre-implementation review: `IT-AF-2661-004`
added to BR-SAFETY-001 in §7; §12(a) now removes the `authorize()` non-local
SAR branch — it is shared by list, exact-get and policy-scope reads — and
names the re-expressed startup logs and the deletion of the new phase-c specs
with the code unit). Draft v2.7 (round-4 pre-implementation review: `UT-AF-2661-004`
added for the non-local namespace-list SAR (the `authorize()` change is now
named in RED/GREEN c — the AC-6 control had IT-only proof); `IT-AF-2661-004`
added for fail-closed inventory startup observability, including the
misspelled-`fleet.fmc`-key shape, and the nil-reader startup log required; the
three REMOTE-LIST §9b rows now require the re-scoped fixture to serve the FMC
page route shape with a group-ful kind; §12(a) reverts the §9b re-scoped specs
with the code; `UT-AF-CFG-2661-002/003` added to BR-SAFETY-001 in §7). Draft
v2.6 (round-3 pre-implementation review: §12 rollback (a)
rewritten as a full revert of the phase-c change set — the two DD-AF-018 sites
alone would silently break fmc-backend inventory; the IT-AF-ROUTE-010/007 row
now keeps both endpoint keys in the fixture; the stale backend_deps "list
remains unavailable" log added to RED/GREEN c; `IT-FMC-INV-2661-001W` documented
in §8 Tier 2 (it exists, `cmd/fleetmetadatacache/main_wiring_test.go:638`);
`IT-FMC-INV-2661-002` added to BR-SAFETY-001; §9b preamble count fixed, its
default-arm requirement added to REFACTOR, and the `UT-FLEET-CFG-029` re-scope
row added). Draft v2.5 (round-2 pre-implementation review: §9b extended with
the four missed re-scopes — `IT-AF-2584-REMOTE-LIST-002`, the cross-cluster
probe inside `IT-AF-2584-005`, `UT-AF-2584-FMC-READER` and the
`IT-AF-ROUTE-010 / IT-AF-PRES-007` table; DD-FLEET-017 §8 amended with wiring
rows 1a/4a so the single-authority claim holds; the REFACTOR status-table
clause reworded to name the real per-package dedup). Draft v2.4 (phase-c
planning hardened by a grounded pre-implementation
review: `fleet.fmc.endpoint` config slice added as Tier 1c + wiring row 4a, §9b
extended with the three 2584 re-scopes, DD-AF-018 gate sites named in phase c
and rollback, REFACTOR narrowed to in-package deduplication, AC-6/AU-3 tagged on
the SAR audit journey). Draft v2.3: DD-FLEET-017 D1–D6 and the ADR-068 amendment were
approved 2026-11-18; increment 2 (this plan's ACM source adapter) was authorised
for execution on 2026-11-20. **Implemented code slices:** the `sources.acm`
configuration surface (`UT-FMC-SRC-2661-001..010`), the ACM collector
(`UT-FMC-SRC-2661-011..019`, `021..026`; `020` lives in
`pkg/fleet/scopecache/inventory_page_test.go`), and the Tier 2 integration
specs `IT-FMC-SRC-2661-001..007` (commits `73e66c805`, `06a017fe9` and the
#2691 review fixes, `work/2661-inc2-acm-adapter`), which prove wiring rows 3–4 through production
construction (`buildFMCSyncer`/`buildACMSource`/`NewACMHTTPSearch` over
configs from the production loader; the fenced commit path against miniredis
with pinned server time). Known limitation: `IT-FMC-SRC-2661-004` proves the
one-deadline-per-repair sharing semantics (D4.1a rule 7b) through the
production context seam with a shortened staging lifetime — the repair path's
own 55 s budget-setting is proven by `UT-FMC-SRC-2661-024` (§11b).
**Not yet proven:** the Live tier — E2E-FMC-INV-2661-001 (ACM lane on
helios08, one live run against the recorded ACM 2.17.4 build with the
least-privilege hub principal, evidence recorded verbatim) and
E2E-FMC-INV-2661-002 (GitOps lane, CI-runnable). **Scope decision
(2026-11-22):** the Live row is executed in a follow-up, not the phase-c PR;
merging phase c leaves #2661 open on the Live row alone. Increment 1 carries
its own evidence and is merged (`#2681`).
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
| Wiring rows proven through production dispatch | 9/9 |
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
`pkg/fleet/config.go` (the new `fleet.fmc.endpoint` key, phase c) and
`cmd/apifrontend/backend_deps.go` (its wiring into the AF inventory reader),
`api/apifrontend/openapi/apifrontend-v1.yaml` only where a documented state
description or response-contract entry is falsified (the surfaces enumerated
in RED/GREEN c).

Not tested here: GW/RO/AF scope rerouting and `NewScopeChecker`'s ACM branch,
Rancher or further sources, per-user visibility/token delegation (#2600),
remote policy-scope listing, Console rendering, new caller-facing routes, totals,
snapshots. FMC's internal inventory route is in scope (wiring rows 1–2).

## 5. Approach

Ginkgo/Gomega. Real `pkg/` logic. Fake only external dependencies: ACM Search
over `httptest`, Valkey via the approved test dependency or the real instance in
IT, Kubernetes via `envtest`/fake clients, MCP readers via fixtures. No
`testing.T` business tests, no `XIt`/`PIt`/`Skip()`.

**Pass criteria.** All listed scenarios green; 9/9 wiring rows through production
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
| AF fleet config | `pkg/fleet/config.go` (new `fleet.fmc.endpoint` key, phase c), `cmd/apifrontend/backend_deps.go` |
| AF dispatch | `pkg/apifrontend/policyread/service.go`, `types.go` |
| AF HTTP/audit | `pkg/apifrontend/handler/console_policyread.go`, `router.go` |

**Version identification**: ACM 2.17.4 / `acm-search-v2-api-rhel9@sha256:85c53e64…`
as recorded in the spike; FMC chart version and Valkey version captured with the
live run.

## 7. BR coverage matrix

| BR | Contract | Scenarios |
|---|---|---|
| BR-SCOPE-001 | Managed-only inventory with ADR-053 precedence | UT-FMC-SRC-2661-011/012/014, UT-FMC-INV-2661-011, UT-AF-2592-REMOTE-FAIL-CLOSED-001, IT-AF-2584-REMOTE-LIST-001/003, IT-AF-ROUTE-010 / IT-AF-PRES-007 |
| BR-SCOPE-002 | Exact GVK, canonical identity, no inference | UT-FMC-SRC-2661-013/018, UT-FMC-INV-2661-008, IT-AF-2584-008 |
| BR-API-001 | Typed, sanitized page, states and freshness | UT-FMC-INV-2661-001..015, IT-AF-2661-001/002, IT-FMC-INV-2661-001/001W, E2E-FMC-INV-2661-001 |
| BR-SECURITY-016 | Least privilege, user gate, no privileged widening (AC-6, AU-3 on the SAR audit journey) | UT-FMC-INV-2661-010, UT-FMC-SRC-2661-023/025/026, UT-AF-2661-004, IT-AF-2661-003, IT-FMC-SRC-2661-003, E2E-FMC-INV-2661-001, IT-AF-2584-REMOTE-LIST-002, IT-AF-2584-ACM-FAIL-CLOSED, UT-AF-2584-FMC-READER, IT-AF-2584-008 |
| BR-SECURITY-017 | Metadata-only egress | UT-FMC-INV-2661-010, IT-FMC-INV-2661-001 |
| BR-INTEGRATION-065 | Source abstraction and dispatch | UT-FMC-SRC-2661-001..010/024..026, UT-AF-CFG-2661-001..003, IT-FMC-SRC-2661-001/002/006, IT-AF-2661-001, E2E-FMC-INV-2661-002 |
| BR-SAFETY-001 | Fail-closed proof, no fallback | UT-FMC-INV-2661-003/004/014/015, UT-FMC-SRC-2661-004..009/015..017/019..022/024..026, UT-AF-CFG-2661-002/003, IT-AF-2661-002, IT-AF-2661-004, IT-FMC-INV-2661-002, IT-FMC-SRC-2661-004/005/007, UT-FLEET-CFG-029, IT-AF-2584-005 (cross-cluster probe) |

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
| UT-FMC-SRC-2661-001 | `sources` absent → `Sources.ACM` is nil, validation passes, collector stays `SourceMCP`: the rendered config behaves exactly as today (D4.1a rule 1) | ✅ |
| UT-FMC-SRC-2661-002 | A complete `sources.acm` block parses `endpoint`/`tokenPath`/`caFile` and selects `SourceACM` (D4.1a rules 2, 4) | ✅ |
| UT-FMC-SRC-2661-003 | `caFile` absent is valid and means system trust; it never fails startup (D4.1a) | ✅ |
| UT-FMC-SRC-2661-004 | Fail-closed endpoint: plain `http`, relative, host-less or empty → `Validate()` rejects, naming `sources.acm.endpoint` (D4.1a rule 3, SC-8/SI-10) | ✅ |
| UT-FMC-SRC-2661-005 | Fail-closed token: relative, empty, missing or directory `tokenPath` → rejected, naming the key; the file is opened, not merely stat'ed, and no token material appears in the error (D4.1a rules 3, 6; IA-5) | ✅ |
| UT-FMC-SRC-2661-006 | A relative `caFile` is rejected, naming the key (SC-8) | ✅ |
| UT-FMC-SRC-2661-007 | Strict decoding: an unknown key under `sources.acm` (e.g. `endpointURL`) fails loading, so an operator typo cannot silently disable a guard (D4.1a rule 3, CM-6) | ✅ |
| UT-FMC-SRC-2661-008 | A second or unknown source (`sources.rancher`) is a configuration error, never ignored (D4.1a rule 2) | ✅ |
| UT-FMC-SRC-2661-009 | Presence selects a source (D4.1a rule 2): a present `sources` block that is empty, null, or names only a null `acm` fails loading, and `acm: {}` loads as ACM then fails validation for its missing endpoint — a named source never resolves back to MCP | ✅ |
| UT-FMC-SRC-2661-010 | A merged `sources` block is validated, not skipped, and flattening reproduces YAML merge precedence: a direct key wins over a merge, and the earlier merge target wins over a later one (CM-6) | ✅ |
| UT-FMC-SRC-2661-011 | Observation shape: every object of a configured kind is written with its label state (`true`/`false`/`absent`/`invalid`), matching the MCP collector's cache shape so both sources project identically | ✅ |
| UT-FMC-SRC-2661-012 | Collector-side precedence: the label state is written per object, and a corrupt projection is invalid, never absent. The read-time precedence clauses — explicit `false` overrides namespace `true`; cluster-scoped never inherits — are owned by `UT-FMC-INV-2661-011` (the shared ADR-053 resolver, proven in `pkg/fleet/fmc/inventory_reader_test.go`). **Review-hardened shapes (rounds 2-3):** a present-but-unusable projection — JSON `null`, an empty managed value, a duplicate key, an entry without `=`, or a space-adjacent separator — is invalid, never absent, never resolved; legacy per-key projections are held to the same rule as the semicolon `label` field; more than one managed-label source at once is corruption, never a precedence choice (parity with the Check lane) | ✅ |
| UT-FMC-SRC-2661-013 | Identity: missing `apiversion`, cross-cluster item, or two items mapping to one canonical ID fails the kind (not just the item). **Wire shape (#2691 review):** group and version arrive as separate `apigroup`/`apiversion` properties (absent group = core, the live #2592 shape our request filters mirror); a foreign `apigroup` on the requested group fails the kind. **Review-hardened:** an item with no or non-string `cluster` attribution fails the kind — Search visibility is not membership — and a present-but-non-string `namespace` field is a failure, never a silent cluster-scoped reclassification | ✅ |
| UT-FMC-SRC-2661-014 | Traversal: explicit `orderBy uid asc`, offset chunking, strictly increasing `_uid`, stable `count` → complete collection | ✅ |
| UT-FMC-SRC-2661-015 | `count` changes mid-walk → exactly one restart from offset 0, then complete; a second change → incomplete | ✅ |
| UT-FMC-SRC-2661-016 | Duplicate/non-increasing `_uid`, non-final short page, or `sum(items) != count` → incomplete | ✅ |
| UT-FMC-SRC-2661-017 | A per-kind error aborts before any write: no staging write, no commit, the previous proven generation stays readable, and no smaller clean set is published | ✅ |
| UT-FMC-SRC-2661-018 | `_uid` is used only for ordering/dedupe: absent from the cache projection, the page and the canonical ID | ✅ |
| UT-FMC-SRC-2661-019 | A successfully traversed zero-item collection is recorded as an **applied observation** (never `unavailable`, never the failure envelope) and commits through `empty_verified`; the page layer, not the adapter, decides `empty` vs `partial` (G10) | ✅ |
| UT-FMC-SRC-2661-020 | One atomic commit writes **both** indexes from the same staged generation: member counts must agree (`LEX_COUNT_MISMATCH` otherwise), the next `begin` clears an abandoned staging's lex index, both enforce the 10 000-member bound, and an empty commit removes both | ✅ |
| UT-FMC-SRC-2661-021 | **Derived request budget** (D4.1a 7a): each request takes `min(10 s, remaining − 5 s commit reserve)`; when remaining is below the 2 s start floor the collector abandons staging and starts no request, so the previous proof ages out instead of being half-replaced | ✅ |
| UT-FMC-SRC-2661-022 | **Walk-level page budget**: the 100-page budget is enforced *before* the request — an exhausted budget is reported without sending page 101 — while a walk that completes on the final allowed page still succeeds; the 5000-item ceiling remains defense in depth inside `addPage`. The **aggregate bounds across kinds** (two kinds individually inside every ceiling whose summed pages or items exceed the partition-wide bounds → no `Repair`, no staged write, no `CommitRepair`) are enforced by the repair contract (`ValidatedObservationSet.Validate`) and proven by `IT-FMC-SRC-2661-007` | ✅ |
| UT-FMC-SRC-2661-023 | `NewACMHTTPSearch` sends `Authorization` from `tokenPath` and validates TLS against `caFile`; replacing the token or CA material changes the next request **WITHOUT rebuilding the client**; no secret appears in the URL, a log line or a status | ✅ |
| UT-FMC-SRC-2661-024 | **Deadline ownership (D4.1a rule 7b)**: `repairPartition` sets exactly one collection deadline per repair, at repair start, from `RepairCollectionBudget` — every configured kind's collection sees the identical value — and never overwrites a deadline already present on the context | ✅ |
| UT-FMC-SRC-2661-025 | **Transport hardening (review round 1)**: one Search reply is byte-bounded before decoding, so the 50-item logical guard is never the only defense against a hostile or broken endpoint; chunk failures are classified transport/tool/malformed and mapped onto the matching observation failure — an HTTP or GraphQL answer is never reported as a network outage, and a TLS failure as a tool failure | ✅ |
| UT-FMC-SRC-2661-026 | **Transport reply integrity (#2691 review)**: redirect answers are refused outright (the bearer can never be routed to a different target); the reply is exactly one JSON value under the byte bound (trailing data is malformed); the searchResult count is explicitly provided and nonnegative — a missing, null or negative count is a tool failure, never a proven empty collection, while an explicit zero with empty items remains the well-formed empty answer; the endpoint is parsed once and the route joined to its path (no query or fragment) | ✅ |

### Tier 1c — AF fleet endpoint config + non-local SAR (`pkg/fleet/config.go`, `pkg/apifrontend/policyread`, phase c)

| ID | Scenario | Critical |
|---|---|---|
| UT-AF-CFG-2661-001 | The new `fleet.fmc.endpoint` key parses and selects the FMC inventory endpoint; the AF inventory reader is built from it, never from `fleet.endpoint` (`cfg.Fleet.EffectiveEndpoint()` stays the scope adapter's endpoint, D2.1a) | ✅ |
| UT-AF-CFG-2661-002 | `fleet.fmc.endpoint` absent under `fleet.backend: fleetmetadatacache` → the inventory path fails closed `unavailable`; it never inherits the scope adapter's endpoint or the ACM Search URL | ✅ |
| UT-AF-CFG-2661-003 | `fleet.backend: acm` with `fleet.fmc.endpoint` present → the FMC reader is still built from the new key (DD-AF-018 gate removed, both sites); with it absent → `unavailable`, never a direct ACM Search client | ✅ |
| UT-AF-2661-004 | Non-local namespace-list SAR (the `policyread.Service` authorize path's new non-local branch): a stub authorizer denying `list/core/v1/namespaces` → non-local list is `forbidden`, the remote reader's `listCalls == 0`, exactly one typed audit event with `result_state=forbidden`; granting it proceeds to dispatch. `authorize()` is shared by list, exact-get and policy-scope reads, so the same denial is asserted on a non-local exact get (`forbidden`, reader `getCalls == 0`, one typed audit event) and on a non-local `ListPolicyScope` (`forbidden`, reader untouched, one typed audit event — the branch changes that path from `unavailable` to `forbidden` before the surviving non-local policy-scope rejection) — every arm of the shared branch needs its own proving scenario | ✅ |

### Tier 2 — Integration

| ID | Scenario | Critical |
|---|---|---|
| IT-FMC-INV-2661-001 | Production mux serves list and exact read; response is the shared sanitized schema; unknown route/method behaviour unchanged | ✅ |
| IT-FMC-INV-2661-001W | Production assembly (`buildFMCServers`, `cmd/fleetmetadatacache/main_wiring_test.go`): the authenticated production mux serves the inventory routes — 503 state-only for an unproven partition, the exact route registered, an unknown route stays 404. Proves wiring row 1a (implemented with increment 2) | ✅ |
| IT-FMC-INV-2661-002 | End-to-end over real cache semantics: proven partition → `success`/`empty`; broken/staged/incomplete partition → `partial`/`unavailable`, with no staging key ever served | ✅ |
| IT-FMC-SRC-2661-001 | From `buildFMCSyncer`→sync, an ACM-configured deployment makes Search calls and reaches `BeginRepair`→`Repair`→`CommitRepair`; a direct Valkey write, or a missing Search call, fails the spec | ✅ |
| IT-FMC-SRC-2661-002 | Build two production syncers — one from an absent-`sources` config, one from `sources.acm` — and assert MCP-only versus ACM-only call surfaces; an invalid `sources` block fails at load/validate, the steps that precede syncer construction in the startup order | ✅ |
| IT-FMC-SRC-2661-003 | Through the production `ACMHTTPSearch` construction: bearer header present, TLS validated against `caFile`, then token rotation and CA rotation both take effect on the same client | ✅ |
| IT-FMC-SRC-2661-004 | Two configured kinds under one repair context share a single deadline (rule 7b): the start floor refuses a request, no clean commit is published, and the previous proof ages to `stale` then `unavailable` | ✅ |
| IT-FMC-SRC-2661-005 | A `sources.acm` block that fails production construction (e.g. an absent `caFile` that passes path-shape validation) exits through main's dependency-wiring choreography — the "Failed to construct FMC dependencies" log and the bootstrap health-server shutdown — never a bare `os.Exit` inside `wireFMCDependencies` (D4.1a rule 3, fail closed observably) | ✅ |
| IT-FMC-SRC-2661-006 | **MCP readiness scoping (DD-FLEET-017 D1)**: an ACM-selected deployment stays ready when the MCP gateway is down (membership comes from the registration CRs; the ACM collector never calls MCP) **and on a cold start with zero registrations** (nothing ever runs a cycle, so the wiring layer owns the flag), while an MCP-selected deployment stays unready for the same outage — the protocol gate applies only to the selected source | ✅ |
| IT-FMC-SRC-2661-007 | **Aggregate bounds across kinds (D2.7)**: two configured kinds, each individually inside the 50-per-page and 100-page walk ceilings, whose summed items and pages exceed the partition-wide ceilings — the repair contract refuses the sync ("out of bounds"), no clean commit is published, and each kind's walk completed individually legally | ✅ |
| IT-AF-2661-001 | AF list dispatch reaches the endpoint from `fleet.fmc.endpoint` (asserted by the request the fixture receives, including with `fleet.backend: acm`) and returns the FMC page — the inventory path never inherits the scope adapter's endpoint | ✅ |
| IT-AF-2661-002 | AF maps `partial`/`unavailable` → `503` state-only, `forbidden` → `403`, `not_found` → `404`; no rows or cursors leak | ✅ |
| IT-AF-2661-003 [AC-6, AU-3] | Namespace-list SAR denial → `403` state-only with zero requests to FMC; one typed audit event per read with `result_state`, no records | ✅ |
| IT-AF-2661-004 | Startup observability of the fail-closed inventory state (GA dimension 12, no silent failures): with `fleet.enabled` and the FMC reader nil — `fleet.fmc.endpoint` absent, or misspelled under AF's non-strict decode (e.g. `fmc.endpointURL`, loaded through the production config loader's `yaml.Unmarshal` into `config.Config`, mirroring `pkg/apifrontend/config/config.go` — the wiring-test convention of struct literals cannot express a misspelled YAML key) — AF logs an explicit inventory-unavailable line at startup; the reader stays nil and reads fail closed. The non-nil branch logs list availability and `cfg.Fleet.FMC.Endpoint` (replacing the stale "list remains unavailable" line) | ✅ |

### Tier 3 — Live / E2E

| ID | Scenario |
|---|---|
| E2E-FMC-INV-2661-001 | ACM lane on helios08: FMC collects from ACM Search with the least-privilege hub principal, AF lists/pages/reads one exact resource, `freshness=fresh` from proof, sanitized payload, no fallback. Assesses AC-4, AC-6, SI-10 and ASVS V5/V8 for this path. ✅ Spec: `test/e2e/fleetmetadatacache/live` (live-only package, no CI target). Proof: one live run 2026-11-22 against the recorded ACM 2.17.4 build (`acm-search-v2-api-rhel9@sha256:85c53e64…`, digest re-verified from the running pod) — 4/4 Its green; evidence verbatim in [LIVE_EVIDENCE_2026-11-22.md](LIVE_EVIDENCE_2026-11-22.md) (output, versions, principal, fixture). |
| E2E-FMC-INV-2661-002 | GitOps lane: an MCP-sourced cluster produces the same inventory surface with no ACM present, proving the source abstraction and that `empty` is only emitted from a proven partition. ✅ Spec: `test/e2e/fleetmetadatacache/shared/inventory_projection.go` (`{prefix}-017`), registered verbatim in both the Kuadrant (`test/e2e/fleetmetadatacache/inventory_projection_test.go`) and EAIGW (`…/eaigw/inventory_projection_test.go`) lanes — CI-runnable via `make test-e2e-fleetmetadatacache-kuadrant` / `-eaigw`; asserts the deployed FMC config carries no `sources` key (no ACM present), success/fresh/sanitized list over the real Keycloak→gateway→kube-mcp-server pipeline, cursor paging, exact read, a zero-item namespace-filtered read as `empty`+`fresh` from the proven partition, and an unregistered cluster as 503 state-only `unavailable`. |

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

**IT-FMC-SRC-2661-003 (construction split).** The bearer, TLS-validation and
token-rotation assertions run through `buildACMSource` (the production
composition `buildFMCSyncer` uses). The CA-rotation half constructs
`NewACMHTTPSearch` directly with the loaded config's values — the identical
call `buildACMSource` makes — because `ACMSource` does not expose its transport
for `ReloadCA`. Both halves use the production constructor; only the
composition root differs.

## 9b. Tests retired or re-scoped by this plan

The existing specs below assert behaviour this plan intentionally changes. They are
**task items, not regressions**:

| Test | Today | Required change |
|---|---|---|
| `UT-AF-2592-REMOTE-FAIL-CLOSED-001` (`pkg/apifrontend/policyread/interim_contract_2592_red_test.go`) | Pins "non-local read rejected, no remote reader called" | Re-scope to the new boundary with the terminal state named: non-local dispatch reaches the remote stub (`remote.listCalls ≥ 1`) and never falls back to local (`local.listCalls == 0` kept verbatim); the response is `success` with the stub's served page. The re-scoped stub page must be fully contract-valid: a freshness envelope (`Freshness{State: fresh, AsOf: …}`), **at least one item** (`validateSuccess` rejects a zero-item success page — the current fixture is exactly that), and each item carrying the requested cluster's ID, `scope.included=true` and a canonical ID matching its identity — `Page.Validate` and `readInventoryPage` enforce all three, and any failure would keep the old `unavailable` outcome alive for the wrong reason (validation, not the boundary). The old expectation is replaced, not weakened. |
| `IT-AF-2584-REMOTE-LIST-003` (`pkg/apifrontend/policyread/service_2584_test.go`) | Asserts remote list rejection with zero requests to a wired ACM client; one audit event with `result_state=unavailable` | Re-scope: dispatch now reaches `RemoteInventory`; the "never contacts ACM Search" half is preserved by asserting the wired reader is the FMC client (fixture receives the request), not by asserting zero dispatch. Terminal state named: the response is a 200-equivalent `success` page with the served items. The audit assertion is re-expressed, not deleted: exactly one event, same attribution fields (user, cluster), `result_state=success` — the AC-6/AU-3 proof moves to the dispatched outcome. The re-scoped stub serves the FMC page route shape — verified against `fmc.NewHTTPClient`'s request (GET `/api/v1/inventory/managed-resources`, page JSON), with at least one group-ful kind (apps/v1) among the served items, every served item carrying the requested cluster's ID and `scope.included=true` (`readInventoryPage` rejects an excluded item by failing the whole page, and `Page.Validate` in `readInventoryPage` (`pkg/apifrontend/policyread/service.go:235`) rejects a page without a contract-valid freshness envelope — either would keep the state assertions passing for the wrong reason). The current GraphQL fixture (group-ful, but a Search envelope rather than the FMC page JSON) would make the state assertions pass for the wrong reason (decode failure, not mapping). |
| `IT-AF-2584-REMOTE-LIST-001` (`pkg/apifrontend/handler/console_policyread_2584_test.go`) | Handler-level pin of "non-local list rejected, no remote reader contacted" (503 + `state:unavailable`) | Re-scope to the dispatch boundary with the terminal state named: non-local list reaches FMC when configured — 200 with the served `success` page — and never falls back to local (`local.listCalls == 0` kept verbatim). The re-scoped stub serves the FMC page route shape (verified against `fmc.NewHTTPClient`'s request, at least one group-ful apps/v1 kind, every served item carrying the requested cluster's ID and `scope.included=true`, contract-valid freshness envelope on every served page) — not the current GraphQL core-only fixture. |
| `IT-AF-2584-REMOTE-LIST-002` (`pkg/apifrontend/handler/console_policyread_2584_test.go`) | Handler-level pin of "a remote ACM list stays unavailable instead of exposing a partial Search page" — allow authorizer, wired ACM client, zero requests asserted | Re-scope with `-001`: non-local list now dispatches to the wired remote reader (the fixture receives the request). Both halves are re-expressed with named cases: the success case (FMC stub serves a contract-valid success page → 200 with the served items) and the no-partial-rows case (a second request where the FMC stub serves HTTP 503 with a state-only `partial` body, mirroring FMC's own `inventoryWireStatus` server-side mapping — partial→503 — which is the wire-realistic way to reach the handler's error arm; a 200 with a well-formed state-only partial body **passes** `Page.Validate` and would surface at the Console as `partial`, not `unavailable`, which is exactly why the fixture must use the 503) → the Console response is 503 state-only `unavailable`, no items, no cursors — the current no-leak assertions, `sensitive-namespace` and `"items"`, are kept in spirit on this case. The never-ACM half is preserved by making the wired fixture the FMC client — serving the FMC page route shape (verified against `fmc.NewHTTPClient`'s request, at least one group-ful apps/v1 kind, every served item carrying the requested cluster's ID and `scope.included=true`, contract-valid freshness envelope on every served page), not the current GraphQL core-only fixture. |
| cross-cluster probe inside `IT-AF-2584-005` (`pkg/apifrontend/handler/console_policyread_2584_test.go`) | Asserts `remote.listCalls == 0` — "a non-local list must be refused before remote dispatch" — with both backends wired and an allow authorizer | Re-express the probe with its terminal state named: the remote stub serves a **prod-east-attributed** success page (the current `successfulPage()` items are all `ClusterID: "local"`, and `validateResource` rejects cross-cluster items — keeping them would make the 503 survive for the wrong reason, via item mismatch instead of dispatch refusal), with each item's **canonical ID regenerated for the prod-east identity** (a re-attributed item whose ID still encodes the local identity is wire-invalid; the current fixture builds IDs via `CanonicalResourceID` of the local identity), so the response becomes 200 `success` with `remote.listCalls ≥ 1`; `local.listCalls == 0` is kept verbatim. The guessed-opaque-ID half of the spec is unchanged. |
| `IT-AF-2584-ACM-FAIL-CLOSED` (`cmd/apifrontend/policyread_wiring_2584_test.go`) | Asserts `deps.RemoteInventory` is nil for `Backend: fleet.BackendACM`, then drives `ListInventory`/`ListPolicyScope`/`GetInventory` → `unavailable` off the same config | Re-scope: with `fleet.fmc.endpoint` configured the FMC reader is wired under `backend: acm` (DD-AF-018 gate removed) — that half asserts only `deps.RemoteInventory` non-nil. With it absent, `RemoteInventory` stays nil and reads fail closed `unavailable` — the nil-reader/read assertions are driven **only** by the absent-key config (with `fleet.endpoint` still set, proving non-inheritance); adding the key to that config would make the read assertions pass for the wrong reason (transport failure against an unreachable wired endpoint, and the surviving policy-scope rejection). |
| `UT-AF-2584-FMC-READER` (`cmd/apifrontend/policyread_wiring_2584_test.go`) | Asserts the FMC Console reader is wired from `fleet.endpoint` (`Backend: fleet.BackendFMC`, `Endpoint` set → `RemoteInventory` non-nil) | Re-scope to the new key: the reader is wired when `fleet.fmc.endpoint` is set (keeping `fleet.endpoint` present proves non-inheritance), and stays nil when only `fleet.endpoint` is set. The old expectation is replaced, not weakened: the inventory path no longer reads the scope adapter's endpoint (D2.1a, `UT-AF-CFG-2661-001`). |
| `IT-AF-ROUTE-010 / IT-AF-PRES-007` DescribeTable (`cmd/apifrontend/policyread_wiring_2584_test.go`) | Drives `deps.RemoteInventory` through exact reads with `Backend: fleet.BackendFMC, Endpoint: server.URL`; the "verified managed" entry expects success through the wired reader | Re-scope the fixture config: keep `fleet.endpoint = server.URL` (the scope adapter's key, which the table's scope-check entries still read through `EffectiveEndpoint()`) **and** set `fleet.fmc.endpoint = server.URL` — this keeps the table's scope-check and exact-read lanes green through production wiring. Key non-inheritance itself is proven by `UT-AF-CFG-2661-001/002` (distinct/absent `fleet.fmc.endpoint`), not here: both keys carry the same fixture URL, which cannot discriminate selection. No assertion weakens — only the config key that selects the reader changes. |
| `UT-FLEET-2584-ACM-EMPTY-001` (`pkg/fleet/acm/inventory_2584_red_test.go`) | **Already narrowed by its author (2026-11-18)**: asserts only "an applied zero-item Search is an empty observation, not unavailable"; no `Page.Validate()`/completeness claim, no AF read boundary | No re-scope needed. This plan adds the adapter-side shape assertion in `UT-FMC-SRC-2661-019` (applied zero-item observation, never a failure envelope) and keeps the state decision in `UT-FMC-INV-2661-002/003` (page assembly). |
| `IT-AF-2584-008` (`pkg/apifrontend/policyread/service_2584_test.go`) | Pins the AF→ACM direct exact-read lane as correct behavior: wires `acm.NewClient` as `RemoteInventory`, asserts a GraphQL request body scoped by cluster, and a successful non-local `GetInventory` | Re-scope the remote fixture to the FMC exact-read wire shape: `fmc.NewHTTPClient` against a stub serving `GET /api/v1/inventory/managed-resources/{id}?cluster_id=…` with the resource JSON (group-ful apps/v1 kind, prod-east attribution, canonical ID matching the identity, `scope.included=true` — an excluded item maps to `not_found`, which would silently reproduce a failure outcome). The cluster-scoping assertion moves from the GraphQL request body to the stub's received path/query; the managed-scope check half is kept unchanged. The old expectation is replaced, not weakened: AF contacting ACM Search for inventory is a fail criterion of this plan (objective 4, R4), and the spec only stays green today because it wires the ACM client by hand — a green spec contradicting the boundary. |
| `UT-FLEET-CFG-029` (`pkg/fleet/fleet_test.go`) | Asserts the facade default builds the FMC inventory reader from the auto-derived `EffectiveEndpoint()` with an absent backend | Re-express: the reader is built only from `fleet.fmc.endpoint`; with it absent the inventory path fails closed to a nil reader (unavailable). The derivation itself is unchanged and still serves the scope adapter — `UT-FLEET-CFG-024/025` keep pinning that lane. The old expectation is replaced, not weakened: auto-derivation on the inventory path would inherit an endpoint the caller never vetted for inventory (D2.1a). |

## 10. Wiring Manifest

Identical to DD-FLEET-017 §8 (single authority; rows 1a/4a and the row 2/4
wording already shipped with #2691's and this branch's documentation commits —
RED/GREEN c verifies the identity, it does not re-amend).

| # | Component | Production entry point | Wiring location | IT ID |
|---|---|---|---|---|
| 1 | FMC inventory route | `GET /api/v1/inventory/managed-resources` | `pkg/fleet/fmc/handler.go` `RegisterRoutes` | IT-FMC-INV-2661-001 |
| 1a | FMC inventory production assembly | same route, real mux | `buildFMCServers` in `cmd/fleetmetadatacache/main.go` — done only when the authenticated production mux returns 503 state-only for an unproven partition, the exact route is registered, and an unknown route stays 404 | IT-FMC-INV-2661-001W |
| 2 | Cache-backed page/cursor | same route | FMC page reader over index + proof gate | IT-FMC-INV-2661-002 |
| 3 | ACM Search source adapter | FMC sync cycle | new adapter under `pkg/fleet/fmc` | IT-FMC-SRC-2661-001 |
| 4 | Source selection (per deployment) | FMC sync cycle | `cmd/fleetmetadatacache/main.go` selects the collector from `cfg.SourceKind()` (MCP / ACM) | IT-FMC-SRC-2661-002 |
| 4a | AF→FMC endpoint config | `fleet.fmc.endpoint` | `pkg/fleet/config.go` defines the key; `pkg/fleet/inventory_reader.go` consumes it (replacing `EffectiveEndpoint()` inside `NewInventoryReader`); `cmd/apifrontend/backend_deps.go` remains the production caller via `fleet.NewInventoryReader` and owns the re-expressed startup logs | IT-AF-2661-001 |
| 5 | AF→FMC dispatch + qualification | `GET /api/v1/console/inventory/managed-resources` | `policyread.Service.ListInventory` / `readerFor` | IT-AF-2661-001 |
| 6 | AF partial/unavailable mapping | both inventory routes | handler `projectionStatus` | IT-AF-2661-002 |
| 7 | Namespace-list SAR + typed audit | both non-local reads | `policyread` authorizer / `emitRead` | IT-AF-2661-003 |

## 11. Implementation sequence and TDD budget

| Phase | Work | Evidence / IDs | Done only when (completion condition) |
|---|---|---|---|
| 0 | ADR-068/DD-FLEET amendment drafted and approved; SDL provenance recorded; paginated Search document added as a **new** constant | — | 60–90 min (excluding approval latency) |
| RED a | UT-FMC-INV-2661-001..014 (page contract, proof gate, managed projection, cursor drift, lex ordering/prefix/missing index) | — | 130 min |
| GREEN a | FMC inventory route + page reader + `Page.Validate()` freshness checks | — | 120 min |
| RED b1 — collector contracts | `pkg/fleet/fmc/acm_source.go`, `acm_search.go` | UT-FMC-SRC-2661-011..019, 021..022 | Done only when every collector UT passes; no production caller is claimed |
| GREEN b1 — collector package | `ACMSource`, `ACMHTTPSearch`, traversal guards | same UTs | Done only when the collector is independently testable and still unwired |
| RED b2 — source composition | `cmd/fleetmetadatacache/main.go` `buildFMCSyncer` | IT-FMC-SRC-2661-002 | Fails while `buildFMCSyncer` always builds the MCP reader |
| GREEN b2 — deployment selection | `buildFMCSyncer` reads `cfg.SourceKind()` | IT-FMC-SRC-2661-002 | Done only when absent `sources` selects MCP and `sources.acm` selects ACM **through production construction** |
| RED b3 — repair publication and budget | `pkg/fleet/fmc/syncer.go` repair path | IT-FMC-SRC-2661-001, -004; UT-FMC-SRC-2661-020/022 | Fails without fenced repair, one repair-wide deadline and the commit reserve |
| GREEN b3 — fenced publication | same repair path | same | Done only when Search observations reach BeginRepair→Repair→CommitRepair, with the shared deadline set once per repair and no direct Valkey write |
| RED b4 — transport credentials | `NewACMHTTPSearch` production construction | UT-FMC-SRC-2661-023; IT-FMC-SRC-2661-003 | Fails without bearer, CA and rotation assertions |
| GREEN b4 — transport credentials | `NewACMHTTPSearch` | same | Done only when request-level auth, trust and rotation pass without client recreation |
| RED/GREEN c | IT-FMC-* and IT-AF-2661-001..004; AF dispatch + the non-local namespace-list SAR (`policyread.Service`'s authorize path, UT-AF-2661-004 first) + status mapping; the `fleet.fmc.endpoint` key (UT-AF-CFG-2661-001..003 first, wiring row 4a) and removal of both DD-AF-018 gate sites — `pkg/fleet/inventory_reader.go` (the `BackendACM → nil, nil` branch) and the `cmd/apifrontend/backend_deps.go` DD-AF-018 ACM-gate log site (`backend_deps.go:256-258`, log-only — the skip half of the gate was the inventory_reader branch) — plus re-expression of the adjacent startup logs and stale documentation (`cmd/apifrontend/backend_deps.go`): the "list remains unavailable" line that also logs `EffectiveEndpoint()` is replaced by list availability + `cfg.Fleet.FMC.Endpoint`, a new explicit inventory-unavailable line fires when `fleet.enabled` leaves the FMC reader nil (IT-AF-2661-004), the `buildPolicyReadDeps` doc comment ("remote inventory list reads remain unavailable … exact/get only") is rewritten to match the dispatch boundary, and the `pkg/apifrontend/policyread/service.go` doc comments that the shared `authorize()` change falsifies are updated (`ListInventory`'s "capability and local target-scope authorization" gains the non-local namespace-list SAR arm; `GetInventory`'s local-denial→not_found note gains the non-local `forbidden` arm); the same change amends DD-FLEET-017 §8 row 4a to this plan's wording (consumption in `pkg/fleet/inventory_reader.go`; `backend_deps.go` production caller owning the re-expressed logs), keeping the single-authority claim true — **already shipped**: the row carries this wording as of #2691/this branch's documentation commits, so RED/GREEN c only verifies it, not re-amends it; adds a dated DD-AF-018 amendment (inventory-lane gate removed by #2661 phase c; FMC is the sole Console inventory contact; the scope lane is unchanged) with the `docs/architecture/decisions/README.md` index row updated to match, and a dated note in ADR-068's 2026-11-18 amendment recording that phase c removed the inventory-lane gate and the inventory-lane peer branches (the "ACM Console inventory stays closed" bullet and the "four places" peer-model consequence; scope lane unchanged), plus a dated note in ADR-068's 2026-11-03 qualification block ("ACM Console inventory remains disabled until … qualified") recording that phase c discharged the condition — enforcement is FMC's proof-gated 503 plus the fail-closed `fleet.fmc.endpoint` wiring, and the same for DD-FLEET-015's 2026-11-18 amendment (its "closed until … qualified" reading of the DD-AF-018 gate; scope lane unchanged) and DD-FLEET-011's "keep the inventory gate closed until qualified" directive (discharged by this qualification; the target-read/scope content unchanged); the DD-FLEET-017 amendment is extended beyond row 4a with a dated note covering its §1.3 state rows (AF→FMC dispatch no longer blocked; ACM Console inventory no longer gated; the peer-model count reduced by the inventory-lane branches) and D6.2's qualification mechanism — qualification enforcement is FMC's server-side proof gate (state-only 503 for an unproven partition) plus the fail-closed `fleet.fmc.endpoint` wiring, superseding the reader-declared-capability wording; scope lane unchanged; and rewrites the now-false OpenAPI state descriptions in `api/apifrontend/openapi/apifrontend-v1.yaml` (the list route's "non-local list requests return a state-only unavailable response", `ConsoleClusterID`'s "non-local identities remain supported for exact reads only", the exact-read route's "remote exact reads additionally require the existing managed-scope checker" — the shared `authorize()` SAR changes non-local exact gets to `forbidden` before the reader, falsifying the local-vs-remote contrast — and the policy-scope route's "non-local policy-scope requests return a state-only `unavailable` response", which the same shared SAR changes to `403 forbidden` on denial while non-local policy-scope listing itself stays unsupported); the same rewrite adds a `403` response entry (`ConsoleStateEnvelope`, matching the list/policy-scope routes' 403 shape) to the exact-read route — a denied non-local exact get is now wire-real `403` on a route whose contract cannot express it — and corrects the exact-read route's 404 description ("absent or *local* target access denied") plus the list and policy-scope routes' 503 descriptions ("Required local source…" → "Required source…", matching the exact-read route's existing wording, since a non-local 503 now arises from the remote FMC source) — §4's "only if a documented state description changes" condition fires for exactly these surfaces | — | 120 min + the config slice |
| REFACTOR | Executed as the refactor step of each RED→GREEN cycle (user rule, 2026-11-22 — never a deferred trailing batch); this row is the budget/sequencing note naming the dedup targets: (a) the parallel per-state switches in `pkg/apifrontend/handler/console_policyread.go` — `projectionStatus` and `projectionMessage` — collapsed into one state→(status, message) table with the fail-safe default arm (503 / "Console projection unavailable"; a map without a default would zero-value `partial`/`stale`/future states) — the cycle owning that file; (b) the shared page-assembly tail `inventory.AssemblePage` in `pkg/fleet/inventory` (the package that owns `Page`), consuming the duplicated success/empty envelope + last-item-ID cursor construction of `pkg/apifrontend/policyread/local_inventory.go` `list` and `pkg/fleet/fmc/inventory_reader.go` `assemble` — the page-paths cycle. The v3.2 round's drop of (b) is superseded by the user rule. A merge of `projectionStatus` with `inventoryWireStatus` (`pkg/fleet/fmc/handler.go`) remains out of scope: different packages, differing 2xx semantics (the AF switch has no success arm; FMC maps success/empty→200) — merging would change behavior | — | 45 min |
| Validate | affected UT/IT with `-race`, build, repo compile, full lint, local/FMC/scope regressions | — | 60–90 min |
| Live | E2E-FMC-INV-2661-001/002, evidence captured | E2E-FMC-INV-2661-001 run live on helios08 2026-11-22 (4/4, [LIVE_EVIDENCE_2026-11-22.md](LIVE_EVIDENCE_2026-11-22.md)); E2E-FMC-INV-2661-002 spec shipped in both FMC E2E lanes, CI-runnable and run focused on the Kuadrant lane: 4/4 Its green (`SUCCESS! -- 4 Passed | 0 Failed`, 821.6 s) | Done: both rows have proving journeys; the ACM row's proof is the recorded single live run against the recorded build |

Total ≈ 12–15 h plus CI. The lex index adds ~1.5–2.5 h of code and, critically,
**re-qualification of the DD-FLEET-016 atomic swap** (~0.5–1 day of IT/live
evidence) because it changes the committed keyspace.

### 11a. Budget ownership (D4.1a rules 7a–7b) — one deadline per repair

The collector reads its deadline from the context, so exactly one component may
set it. That component is the repair path, not the collector:

- `repairPartition` computes **one** deadline immediately after a successful
  `BeginRepair`, from that generation's staging lifetime, and wraps the context
  with `fmc.WithCollectionDeadline`.
- The same context is passed through every kind's `ReaderFor`/`ListObserved`, so
  N kinds share one window instead of each receiving a fresh fallback. The
  per-kind path must never derive a new budget.
- Within the window the collector reserves the commit cost (5 s), refuses to
  start a request below the 2 s floor, and abandons the repair when the deadline
  is exhausted — no clean commit, the previous proof ages out.

Proving this is `IT-FMC-SRC-2661-004` (two kinds, one context) and
`UT-FMC-SRC-2661-021` (start floor). Without this rule the 55 s fallback is a
per-kind constant and a seven-kind sync can outlive a 60 s staging generation.

### 11b. TDD compliance record (methodology enforcement)

The project methodology is RED-first without exception. This section records
a breach found in increment 2 and its remediation, and binds the remaining
phases.

**Breach.** The b2 wiring (source selection, `7d905be27`) and the b3 deadline
wiring (inside the review-fix commit `457c37f9c`) landed **before** their RED
specs existed — the IT specs were written afterwards against committed code,
which inverts this plan's own §11 ordering ("Fails while `buildFMCSyncer`
always builds the MCP reader").

**Remediation (executed 2026-11-20).** The RED phases were recovered against
the true pre-wiring baselines rather than simulated:

- **b3 / rule 7b ownership**: `UT-FMC-SRC-2661-024` (internal to `pkg/fleet/fmc`
  because `deadlineKey` is deliberately unexported) was written first, then run
  against `c72b0503e` (the last commit without the deadline wiring): it fails
  with the deadline unset — the right reason. Green at HEAD.
- **b2 / source selection**: the four IT specs do not compile against
  `8ec562999` (the pre-wiring `buildFMCSyncer` returns one value, no error) —
  the authentic first RED for the wiring. Green at HEAD; behavioral failure
  modes are additionally pinned by the mutation-control runs recorded with
  `73e66c805` (severed catalog, severed sources, removed rotation, removed
  shared deadline).
- `IT-FMC-SRC-2661-004`'s sharing semantics remain proven through the
  production context seam (Status note above); `UT-FMC-SRC-2661-024` now
  proves the repair path's own deadline-setting that the IT could not observe.

**b1 (config and collector unit slices).** RED evidence was recovered by a
**verification replay** (2026-11-20): the committed specs were run against the
pre-implementation states, which modifies nothing that survives, so the final
tree is bit-identical by construction:

- *Greenfield RED*: with `config/sources.go` absent, the config package does
  not compile (`SourcesConfig`, `decodeSourcesStrict`, `validateACM`
  undefined); with `acm_source.go`/`acm_search.go` absent, the fmc package
  does not compile — the specs' contracts do not exist without the
  implementation.
- *Review-fix RED, round 2* (`fa4655087`): its three per-key-projection specs
  fail against `457c37f9c`'s pre-fix code (null/numeric/empty values laundered
  into decisions instead of invalid).
- *Review-fix RED, round 3* (`6f065422f`): its three ambiguity specs fail
  against `fa4655087`'s pre-fix code (two managed-label sources and a
  space-adjacent separator read as clean states instead of corruption).

A **re-derivation** replay (delete the code and re-implement from the specs)
was considered and rejected: the outcome cannot be assured identical — the
review-fix refinements and incidental structure could diverge, and it would
invalidate the SHAs this record cites. The verification replay provides the
RED evidence the methodology requires without that risk.

**c (config and dispatch slices) — breach recorded 2026-11-22, remediated same day.**
The Tier 1c config slice (`FleetFMCConfig` + the `NewInventoryReader` rewrite)
started before the plan review the user required, and its GREEN was left
incomplete: the sibling spec `UT-FLEET-CFG-029` (`pkg/fleet/fleet_test.go:751-759`)
was red at the end of GREEN, and no REFACTOR step was performed. RED itself
was shown failing for the right reason (the contract did not exist:
`FleetConfig has no field or method FMC`).

Remediation, in order:
- The plan-fix loop ran first (rounds 3–25, plan v2.5→v4.7, zero findings).
- `UT-FLEET-CFG-029` was re-expressed per its §9b row (absent
  `fleet.fmc.endpoint` → nil reader; the auto-derivation lane asserted
  unchanged), and the full `pkg/fleet` suite went green with `-race`.
- The cycle's REFACTOR was assessed: **N/A** — the gate removal collapsed
  `NewInventoryReader` to flat, single-purpose construction, leaving no
  in-package duplication or awkward structure to improve.
- The two dedup targets named in §11's REFACTOR row were executed as the
  refactor steps of the cycles owning those files: the
  `projectionStatus`‖`projectionMessage` table (the `console_policyread.go`
  cycle — it landed inside that cycle's commit rather than its own, a
  deviation from the own-commit rule noted here) and the shared
  `inventory.AssemblePage` page-assembly tail (the page-paths cycle — own
  commit `7be71d1f4`).

**Standing rule for the remaining phases** (c, REFACTOR, Live): strict
RED-first — write the failing spec, show it failing for the right reason,
then implement. Implement-first is not an option; when RED-first is genuinely
impossible, escalate for approval before writing any implementation code.
Every cycle completes RED → GREEN (validated against the whole affected
suite) → REFACTOR (its own commit, concrete improvement named; N/A only with
a one-line reason) before the next cycle begins.

## 12. Rollback

Two independent, atomic reverts: (a) revert the phase-c change set as one
unit — restore the `pkg/fleet/inventory_reader.go` `BackendACM → nil, nil`
branch, the `cmd/apifrontend/backend_deps.go` DD-AF-018 ACM-gate log site
(`backend_deps.go:256-258`, log-only), the
`policyread.Service.ListInventory` non-local rejection, and the
`EffectiveEndpoint()`-based reader construction (the `fleet.fmc.endpoint` key
reverts with it), so pre-phase-c behavior — including the fmc-backend reader
built from the scope adapter's endpoint or its auto-derivation — returns
exactly. Restoring only the two DD-AF-018 sites is not a rollback: it would
leave `fleet.fmc.endpoint` mandatory for fmc-backend inventory, silently
unavailable to deployments that never set it. The revert also removes the
`policyread.Service` `authorize()` non-local namespace-list SAR branch
(UT-AF-2661-004/IT-AF-2661-003) — `authorize()` is shared by list, exact-get
and policy-scope reads, so leaving the branch would keep a denial surface on
non-local exact gets that did not exist pre-phase-c — and restores the re-expressed `backend_deps.go` startup log lines and the
rewritten `buildPolicyReadDeps` doc comment to their pre-phase-c form.
The §9b re-scoped/re-expressed specs revert to their pre-phase-c form in the
same unit, the new phase-c specs (UT-AF-CFG-2661-001..003,
UT-AF-2661-004, IT-AF-2661-001, IT-AF-2661-003, IT-AF-2661-004, and
IT-AF-2661-002 if its forbidden case is driven through the non-local SAR
branch) are deleted with the code unit, the DD-AF-018 amendment (+ its
`decisions/README.md` index row, the ADR-068 amendment notes (2026-11-18 amendment and 2026-11-03 qualification block) and the
DD-FLEET-015/DD-FLEET-011 amendment notes), the OpenAPI changes in
`api/apifrontend/openapi/apifrontend-v1.yaml` — the four rewritten
route/parameter descriptions, the new exact-read 403 response entry, the
corrected 404 description, and the list/policy-scope 503 description
changes — and the re-expressed
`pkg/apifrontend/policyread/service.go` doc comments revert to
their pre-phase-c forms, the DD-FLEET-017 amendment's state-table/D6.2 note
likewise reverts, and the DD-FLEET-017 §8 row-4a wording is restored by a separate
documentation edit outside the atomic code unit — its phase-c wording already
shipped with #2691's documentation commits, so a git revert of the phase-c
change set cannot reach it; its pre-phase-c form is the #2691-era wording
naming `backend_deps.go` as the builder — an executor
who reverts only the code sites leaves a red suite; (b) disable the ACM source
adapter so the partition falls back to its previous proven state or
`unavailable`. No compatibility mode, no data migration.

## 13. Evidence boundary (not claimed)

No certification, no claim about ACM builds other than the recorded one, no
per-user remote visibility (#2600), no offset-snapshot guarantee, no Rancher or
further sources, no remote policy-scope, no Console rendering evidence, no live
result before the run. The peer model in current code and ADR-068 is not
migration-tested by this plan; the amendment is a prerequisite, not a test item.
CA rotation (`ReloadCA` on the live client) is proven as a capability of the
production constructor with no production watcher wired yet; it is not claimed
as operational rotation behavior. Rendering `fleet.fmc.endpoint` in fleet-mode
AF deployment configs (E2E infrastructure and charts) is deferred to
[#2695](https://github.com/jordigilh/kubernaut/issues/2695) — deployment
plumbing, not this plan's contract. Validation of a present-but-malformed
`fleet.fmc.endpoint` (no scheme, plain `http`, host-less) is likewise deferred:
`NewInventoryReader` checks only for an empty value, so a reachable plain-`http`
endpoint is **not** rejected and can serve successful (unencrypted) inventory
reads — only an unreachable or absent endpoint fails closed per-request
(`unavailable`). Adding `Validate()`-level rejection with CM-6/SI-10 parity to
`sources.acm.endpoint` would expand the phase-c config slice beyond its
approved Tier 1c shape — it needs its own decision if wanted, and until then
the SC-8 posture of a plain-`http` value is an accepted, documented risk of
that deferral.
