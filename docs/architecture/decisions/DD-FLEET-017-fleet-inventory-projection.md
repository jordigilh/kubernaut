# DD-FLEET-017: FMC fleet inventory projection with ACM Search source adapter

**Status**: 📝 Draft v2 — **re-scoped 2026-11-18** after corpus triage.
Supersedes the ACM-direct draft (`AF → ACM Search`), which targeted an
architecture decision already replaced on 2026-11-11. **D1–D6 approved
2026-11-18.** Nothing is implemented; implementation waits on the amendment's
approval.
**Prerequisite**: the ADR-068 / DD-FLEET amendment removing the peer-adapter
model — **approved 2026-11-18** (ADR-068 "One federation-facing contact
(peer → facade)", with cross-references in DD-FLEET-011, DD-FLEET-015 and
DD-AF-018). Decisions D1–D6 were approved the same day. Prerequisite satisfied;
implementation begins after the fresh review of this set reports.
**Issue**: [#2661](https://github.com/jordigilh/kubernaut/issues/2661)
(parent [#2593](https://github.com/jordigilh/kubernaut/issues/2593),
sibling investigation [#2599](https://github.com/jordigilh/kubernaut/issues/2599))
**Spike evidence**: [docs/tests/2661/SPIKE_2026-11-18.md](../../tests/2661/SPIKE_2026-11-18.md)
(ACM Search traversal, visibility, mutation behaviour)
**Test plan**: [docs/tests/2661/TEST_PLAN.md](../../tests/2661/TEST_PLAN.md)
**Requirements**: BR-SCOPE-001/002, BR-API-001, BR-SECURITY-016/017,
BR-INTEGRATION-065, BR-SAFETY-001.
**Controls**: FedRAMP AC-4, AC-6, SI-10; SOC 2 CC7.2 (audit only);
OWASP ASVS V5 (validation), V8 (data protection). No certification claim.

---

## 1. Problem, correction history, and what is actually approved

### 1.1 What #2661 originally asked

#2593 requires the bounded, sanitized ACM/FMC Console inventory projection.
PR #2637 delivered the FMC proof lifecycle, the typed scope contract and a
fail-closed AF gate: `pkg/fleet/inventory_reader.go` returns `nil` for
`BackendACM`, `policyread.Service.ListInventory` rejects every non-local
cluster, and `interim_contract_2592_red_test.go` pins that refusal. #2661 was
written against the then-current peer-backend model, so it asks for an
"ACM Search Console inventory projection" and its first task is "codify the
ACM Search inventory contract DD".

### 1.2 The correction (recorded 2026-11-11, before this DD existed)

The federation boundary was decided for #2593:

> "AF federated inventory must use **FMC as the single federation-facing
> contact**; AF must not list directly from ACM Search, MCP, or remote
> Kubernetes. Include the FMC inventory API and AF→FMC production dispatch in
> the architecture/wiring plan, while retaining the ACM Console inventory
> release gate as unavailable/no rows until the FMC ACM source, configured hub
> Search identity, completeness and production journey are qualified."
> — recorded #2593 decision, 2026-11-11

Supporting decisions from the same period: FMC "sources via MCP for GitOps/no
control plane and queries ACM Search directly for ACM; AF uses the same FMC
inventory API" (no MCP dependency for the ACM source); callers "must not know
or select whether the underlying adapter is ACM Search, MCP-backed collection,
or another fleet backend"; and the peer-adapter model "where GW/RO selected FMC
versus ACM directly" is **superseded**, requiring an ADR-068/DD-FLEET amendment
plus bounded AF/Console coverage.

The draft v1 of this DD (AF→ACM direct) therefore described the superseded
model. That draft is withdrawn; its analysis is retained where it still applies
(§3, D4).

**Scope of the supersession, precisely:** it covers the *fleet-control-plane
query* surface. GW/RO direct ACM `ScopeChecker` behaviour "remains separate
unless explicitly rerouted" and is **not** changed by this DD. Kubernetes MCP
tools remain the path for live Kubernetes object reads (KA, owner chains) —
those are not fleet-control-plane queries.

### 1.3 What is approved, what is implemented, what is gated

| Piece | State |
|---|---|
| FMC as the single federation-facing contact for federated inventory | **Decided** (2026-11-11); ADR-068/DD-FLEET amendment **approved 2026-11-18** |
| FMC inventory API (`/api/v1/inventory/managed-resources`) | **Client only**; FMC serves only `/api/v1/scope/check` and `/api/v1/clusters` (`pkg/fleet/fmc/handler.go:74-75`) |
| FMC ACM Search source adapter | **Not implemented** — no ACM code in `pkg/fleet/fmc` or `cmd/fleetmetadatacache` |
| AF→FMC dispatch | Code exists (`fmc.HTTPClient` returns `inventory.Page`), blocked by the non-local rejection (`pkg/apifrontend/policyread/service.go:77`) |
| ACM Console inventory | **Gated**: `unavailable`, no rows |
| Code model | Still the **peer model**, in four places: `pkg/fleet/config.go:33-42` (`BackendFMC` / `BackendACM`), `fleet.NewInventoryReader`, `fleet.NewScopeChecker`, and AF's two `fleet.backend` branches in `cmd/apifrontend/backend_deps.go` |

## 2. Scope

In scope:

1. **FMC inventory API** — a typed, bounded, sanitized `inventory.Page`/
   `Resource` projection served from the existing cache.
2. **FMC ACM Search source adapter** — collection from ACM Search into that
   cache, under the recorded visibility/identity rules.
3. **AF→FMC dispatch and qualification** — removing the backend-neutral
   non-local rejection in favour of a capability-qualified dispatch, with the
   ACM-sourced qualification tracked separately (D6.1).
4. The **conditions** under which the ACM Console inventory release gate may be
   removed.
5. **Increment 1d — the config half of the facade migration** (added after the
   operator contract sync, 2026-11-19): `fleet.effectiveBackend()` defaults an
   **absent** backend to `fleetmetadatacache`; `Validate`/`ValidateFullFederation`
   accept a fleet config with no backend and validate the derived FMC endpoint;
   tests/fixtures updated. Explicit `backend: acm` and
   `backend: fleetmetadatacache` keep their current behaviour, so this is a
   widening of accepted config, not a behaviour change.
   Two existing specs encoded the superseded peer model and were **re-expressed,
   not deleted**: `UT-FLEET-CFG-042` (a config with only `mcpGatewayEndpoint`
   now derives the FMC scope endpoint instead of failing dual-capability
   validation) and `UT-FLEET-FAC-007` (an absent backend now builds the FMC
   checker instead of an "unsupported backend" error).
   **Superseded by 1e once increment 2 lands (1.6 is not deployed, so no
   compatibility window is required):** increment **1e** deletes the peer
   scaffolding outright — `FleetConfig.Backend`, `BackendACM`,
   `supportedBackends`, the ACM branches in `NewInventoryReader` and
   `NewScopeChecker`, AF's two `fleet.backend` branches, and the direct-ACM
   **scope** lane — as one atomic change with no dual path, no deprecation and no
   shim.

   `FleetConfig.TokenPath` is deleted as well. It is **not** ACM-only (operator
   correction, 2026-11-19): the FMC branch of `NewScopeChecker` also consults it
   as a test / non-default-mount seam (`pkg/fleet/scope_factory.go:106-126`,
   alongside the ACM use at `:148-152`). 1e therefore removes the field and
   replaces that seam with a **non-config** injection on the factory, so no dead
   config surface lingers. Consequence for the operator: no `fleet.tokenPath`
   rendering and no GW/RO/AF/EM token mount at all; FMC's own credential is
   `sources.acm.tokenPath`, in a different ConfigMap and mount.

   The ordering constraint is **data, not compatibility**: until increment 2
   supplies ACM observations, deleting the ACM scope branch would make every ACM
   scope check fail closed on `main`, because FMC would have no ACM-sourced
   partitions to read. So: 1d keeps `main` coherent now (absent backend resolves
   to FMC, explicit `acm` still works), increment 2 makes FMC the actual ACM
   source, then 1e removes the peer surface. The operator drops its hardcoded
   `backend` in the same window as 1e.

   **Release rule.** The operator is the only supported deployment path and 1.6
   is unreleased, so 1e takes no deprecation window and must land **before the
   1.6 cut**. 1d is an intermediate that keeps `main` coherent between increments
   and is then **subsumed** by 1e (the field disappears; the absent-backend
   assertions stay valid because that becomes the only mode). No release may
   contain both the peer surface and the facade default — "clean deployment"
   means one config model, one scope lane, no dead branches and no operator-side
   transitional constant at release.

Out of scope (unchanged or separately owned): GW/RO/AF scope rerouting to FMC,
`NewScopeChecker`'s ACM branch, Rancher or any further source adapter, per-user
remote visibility and token delegation (#2600), remote policy-scope listing,
new caller-facing routes or media types (FMC's internal inventory route is in
scope by D2.1), totals/counts/snapshot identifiers, audit schema changes,
AF-side caching, policy editing/approval/writes.

## 3. Established inputs (not re-spiked)

From #2592/#2593 and the [spike record](../../tests/2661/SPIKE_2026-11-18.md); these now govern
the **collector**, not a per-request path:

1. **Traversal**: `orderBy uid asc` is accepted and honored (`desc` reverses);
   an invalid order value is **silently ignored**; a keyset predicate on
   `uid`/`_uid` is accepted and matches **nothing**; `offset` continuation is
   exact (88 namespaces/9 pages, 232 pods/5 pages, stable counts, strictly
   increasing `_uid`); `count` is the total for item queries on 2.17.4.
2. **Mutation**: deleting already-passed identities mid-walk skipped four
   unrelated identities while `sum(items) == count` still held — the count
   transition is the detector, not the total. Index lag observed 0.4–6.5 s.
3. **Visibility**: a scoped Search ServiceAccount can receive a successful
   **empty** result for a known resource; empty is never proof of absence.
4. **Labels**: semicolon `label` projection; explicit `false` beats a namespace
   `true`; cluster-scoped objects never inherit; invalid ≠ absent.
5. **No watermark**: `searchSchema` exposes no index-freshness signal, and
   deployed introspection is disabled.
6. **Cache + TTL are not proof** (#2599): scope-cache entries and a successful
   sync do not prove complete *enumerable* inventory or current source state.
   Correction to #2599's wording: the syncer does **not** continue after a
   per-kind error — `syncLegacyCluster` and `collectRepairObservations` return on
   the first kind error, before any write, and no commit happens unless it is
   clean. What it *does* tolerate is a **per-cluster** error in `syncAll`. The
   real risk is therefore a source that reports "complete" while being
   incomplete, not a partial set published by FMC.
7. **Label inheritance is resolved at read time**, not at sync: the collector
   writes per-object label state (including namespace objects), and the verified
   read applies the 2-level resource/namespace hierarchy
   (`pkg/fleet/scopecache/client.go` verified read). Any inventory projection
   must reuse that resolver rather than reimplementing precedence.

## 4. Observed gaps (preflight against origin/main 0e086e065)

| ID | Observation | Consequence |
|---|---|---|
| G1 | FMC serves no inventory route; the `/api/v1/inventory/managed-resources` client has no server (`pkg/fleet/fmc/handler.go:74-75`). | The AF client is unreachable; D2 is greenfield. |
| G2 | No ACM code exists in FMC; the syncer is MCP-reader based. | D4 is a new adapter, not a rewire. |
| G3 | `inventory.Page.Freshness` is never set by any FMC-facing path and `Page.Validate()` never inspects it; `FreshnessState` is a local enum type with no generated Console counterpart and no validation. | A new page path can serialize `""`/zero `as_of` and nothing rejects it. |
| G4 | ACM inventory cursor miss returns `ErrInvalidFilter`, which the handler tests **before** state mapping → today it would surface as **400**, while the local reader restarts its sorted projection (`policyread.cursorStart`). | Drift is misreported as caller error. |
| G5 | The ACM adapter discovers only `managed=true` namespaces and caps each query at 1000, re-walking everything per request (`2 + m + k` round-trips). | Wrong place and wrong cost — collection must move to sync time. |
| G6 | `count` semantics differ by ACM version (SDL note vs 2.17.4 observation). | Completeness cannot rest on count arithmetic alone. |
| G7 | The vendored SDL has no `orderBy`/`offset` and no recorded provenance; `UT-ACM-054-009` validates `SearchQuery` against it, and `acm.SearchQuery` is shared by `Check` and `Ping`. | Paginated collection needs a **new** query constant and a provenance-stamped SDL. |
| G8 | Cache records are managed-only and keyed by full identity (`scopecache.BuildKey`); per-partition indexes exist, but there is no page/cursor contract over them. | D2 must define bounded enumeration over the index. |
| G9 | The cache TTL and a clean commit are not proof of *enumerable* completeness (#2599): a source can report complete while missing records. (Correction: the syncer does **not** continue after a per-kind error — it aborts before writing.) | D2/D3 must not equate "clean commit" with "complete fleet". |
| G10 | `pageFromSearchResponse` returned `StateOnly(StateEmpty)` — a **failure helper** (empty + `CompletenessPartial`) — for a *successful* zero-result query, which `Page.validateEmpty` rejects. Latent, because `pkg/fleet/acm` is a collection adapter with no read-surface caller. | **Fixed:** a completed zero-item traversal now returns a complete empty collection result validated by `Page.ValidateCollection()` (shape only), while `Page.Validate()` keeps the strict read-surface freshness rule. Empty-vs-partial stays FMC page assembly's decision (D2.4). |

## 5. Decisions

Each decision states the proposal, the alternatives with why they lose, and the
consequence. Numbers marked *provisional* belong to the implementation plan.

### D1 — FMC is the only federated inventory contact; it selects the source

**Proposed (B-A).**

1. AF calls FMC and nothing else for federated inventory. AF does not know,
   select, or contact ACM Search, MCP, or remote Kubernetes.
2. FMC owns source selection: MCP-backed collection for GitOps/no-control-plane
   clusters, ACM Search for ACM-managed clusters (today), with further sources
   added behind the same interface.
3. A source is qualified separately and explicitly: implementing the source
   adapter is not the same as declaring its inventory contract proven.
3a. **Membership is not an open question: the gateway registration CR decides
   it.** The authoritative #2593 decision is that the MCP Gateway cluster
   registration CR (EAIGW `Backend`/`MCPRoute`, Kuadrant
   `MCPServerRegistration`) is the **source of truth for whether a cluster is
   managed by Kubernaut**; FMC abstracts that registry, and **ACM Search cannot
   override it**. Unregistered clusters stay fail-closed/`unavailable`. This
   matches the code (partition key carries the registration `metadata.uid`; sync
   verifies against the registry snapshot) and ADR-068's "Gateway CRDs as source
   of truth".

   Consequences:

   - **An ACM source collects for *registered* clusters only.** "No MCP
     dependency for the ACM source" means the *collector* does not call MCP for
     metadata — it calls Search — not that membership comes from ACM. ACM
     deployments still register their spokes, because Kubernaut needs the MCP
     path for live object access (GW/RO owner chains, KA reads).
   - **A `ManagedCluster` that is not registered must not become a partition.**
     Deriving membership from ACM (`ManagedCluster.metadata.uid`) would let ACM
     override the registration source of truth and is **rejected**.
   - **Source selection is per deployment** (`fleet.backend` selects the
     collector FMC runs: MCP collection for GitOps, ACM Search for ACM), which
     is what the recorded decision describes. Per-cluster classification
     (registration label/annotation) is unnecessary for a homogeneous fleet and
     is deferred until a genuinely mixed fleet is approved; a static ACM cluster
     list is rejected as unauditable.
   - A registered cluster that is **not visible in Search** is a collection
     failure (incomplete → no clean commit → `unavailable`), never empty.
   - This removes the earlier "membership blocker": no new FMC API grant, no
     `ManagedCluster` RBAC, no synthetic registration identity.
4. The peer model in code (`pkg/fleet/config.go` backend selection,
   `fleet.NewInventoryReader`/`NewScopeChecker` ACM branches) is removed for the
   *inventory* lane only; the scope lane is untouched here.

**Alternatives.** (i) Keep the peer model and let AF pick a backend per
deployment — the recorded 2026-11-11 supersession, and it makes every caller
responsible for source semantics. (ii) Let AF contact multiple sources
concurrently and merge — duplicates visibility, freshness and authorization
policy in the least suitable place. (iii) Route Console inventory through the
Kubernetes MCP tools — those are live object reads, not the fleet inventory
abstraction, and they would not carry cache/proof semantics.

### D2 — The inventory page is a managed-only projection over the proven cache

**Proposed (P-A).**

1. FMC serves `GET /api/v1/inventory/managed-resources` (list) and
   `/…/{resourceID}` (exact), returning the shared `inventory.Page`/
   `inventory.Resource` contract unchanged (canonical six-field ID, no totals,
   no snapshot identifier).
1a. AF's inventory endpoint is `fleet.fmc.endpoint`, **independent of
   `fleet.backend` and of `fleet.endpoint`**. Today `fleet.endpoint` is shared by
   the scope adapter and the inventory reader, so an ACM-configured deployment
   would point the FMC inventory client at the Search URL. The inventory reader
   must never inherit the scope adapter's endpoint, and its absence is
   `unavailable`, not a fallback.
2. **Managed-only by projection, over a complete observation set**: the cache
   holds **every** observed object of the configured kinds, including explicit
   `false`, `absent` and `invalid` label states — the syncer passes no label
   selector (`pkg/fleet/fmc/syncer.go` `ListObserved`, `observation.ListRequest`
   leaves `LabelSelector` nil). The page is therefore "resources Kubernaut
   manages" only because the projection **filters on the resolved
   `Scope.Included`**; that filter is a critical contract, not a property of the
   cache. Namespace inheritance is resolved at **read** time
   (`pkg/fleet/scopecache/client.go` verified read: resource → namespace record),
   not written by the syncer.
3. List requires `cluster_id` and is scoped to that cluster's partition; exact
   reads resolve the canonical ID and require the same partition.
4. **`empty` requires proof, and a proven-empty partition has no indexes.** An
   empty commit deletes both indexes, so the page derives `empty` from the proof
   (`empty_verified=1` and `observed_count=0`) — **never** from an index being
   present or absent. A missing index without that proof is `unavailable`.
5. Cursor is the canonical resource ID of the last item over a deterministic
   order; a cursor missing because the cache moved on **re-runs the sorted
   projection from the start**, exactly like the local reader
   (`policyread.cursorStart`) — never `400`, never `unavailable`.
5a. **Order and filtering are served by a lexicographic identity index**
   (decided 2026-11-18; "sort canonical IDs in memory" was rejected — measured
   at 284 ms / 858 KiB per page at the 10 000-item cap). The commit path writes
   a second per-partition ZSET with identity members and `score = 0`, so
   ordering and prefix filtering become range scans instead of full
   enumerations:
   - member encoding, in order: `namespace \x00 kind \x00 group \x00 version
     \x00 name \x00 publicID`. `\x00` cannot appear in those values, so byte
     order is unambiguous; cluster is omitted because it is fixed per partition.
   - **deliberate divergence**: order is namespace-first, not
     `SortResources`' group/version-first, because Console filters narrow
     namespace → kind → name. Determinism is the contract; ordering parity with
     the local reader is not. Local readers keep `SortResources`.
   - fast prefixes: `namespace`, `namespace+kind`, `namespace+kind+name`. A
     kind-only or name-only query without a namespace still scans the partition
     and filters — documented, not hidden.
   - a partition **without** the lex index (committed before this change) fails
     closed as `unavailable` until its next clean commit creates it (≤ one sync
     interval), logged and metric'd — never served as empty.
   - both indexes share the 10 000-member bound; the lex index adds one ZSET per
     partition (~1 MB at the cap).
   - **bounded proof re-resolve (D2.5b).** A page is read under one proof across
     two calls (header, then records), so a commit landing between them
     supersedes that proof. The reader re-resolves the fence **once**, after a
     2 ms pause, and repeats the page read; a second failure is `unavailable`
     state-only. Two attempts maximum — a genuinely stale fence still fails
     closed. Requalification measured the race at ≈0.01% of reads at the
     production 30 s cadence (§ evidence: `REQUAL_1B.md`).
   - the **wire cursor stays the canonical resource ID**. The reader recovers the
     page position by reading that ID's record inside the header script (O(1));
     if the record is absent from the current generation, that is the drift
     signal and the projection restarts from the beginning. No reverse index and
     no wire-format change.
   - a **scan budget** (provisional: 20 lexicographic batches) bounds one page's
     projection; exceeding it returns `partial` state-only rather than a
     truncated page.
   - consistency is enforced at the writer: one script swaps both indexes, and a
     commit whose lex member count disagrees with the data index is rejected
     (`LEX_COUNT_MISMATCH`) instead of publishing a page that cannot page
     correctly. `begin` also clears an abandoned staging's lex index, so an
     abandoned generation can never leave members behind.
6. Bounded by the existing contract (`DefaultLimit`, `MaxLimit`, cursor length,
   filter validation) plus a retained-projection budget; a saturated read is
   `partial`, not a truncated `complete`.
7. **Inherited ceilings, stated explicitly (partition-wide, not per kind):**
   `ValidatedObservationSet.Validate` rejects more than **5000** items or
   **100** pages for the aggregated observation set, the commit script bounds the
   staged index at **10 000** members, and `observation.ObservationPageLimit` is
   50 per page. These already govern the scope lane. A cluster above them can
   never publish a clean generation, so its inventory sits at `unavailable`
   permanently. Raising the bounds or sharding partitions is decided before this
   lane is qualified — and applies to both lanes.

**Alternatives.** (i) Enumerate the source per request (the v1 draft) — user-
visible latency, per-page O(N) fan-out and an unverifiable completeness claim.
(ii) Serve every cached identity including unmanaged ones — the cache *does*
hold them (explicit `false`/absent observations), and "exists" is not what the
Console route promises, so the projection must filter. (iii) Serve
`empty` from TTL alone — refuted by #2599.

### D3 — Freshness and completeness come from proof state, not from a query time

**Proposed (F-A).**

1. `freshness.state=fresh` when the partition has a **fenced-clean committed
   generation** whose age is inside the freshness budget; `stale` when that proof
   is valid but older than the budget. Those are the only two values the proof
   can support today.
2. `freshness.as_of` is the **projection completion time**, matching the local
   reader's existing semantics (`local_inventory.go` sets `AsOf` to the read
   time). It is not a commit timestamp: the committed status carries no
   timestamp today, and inventing one would change the fenced atomic contract
   (DD-FLEET-016). `observed_at` is the same instant. Neither claims source
   currency beyond the proof.
2a. `stale` is derived from **remaining proof life**, not from a commit time:
   `fresh` while the proof is valid and its remaining TTL is at or above the
   staleness threshold, `stale` while valid but below it, `unavailable` once the
   proof is gone. The threshold must be strictly smaller than the cache TTL
   (45 s default) and larger than the sync interval (30 s default); otherwise a
   single missed cycle jumps straight from `fresh` to `unavailable` and `stale`
   is unreachable. The numbers are an open decision (§9.4).
3. **Per-kind completion is not represented in the proof today**: a commit is
   whole-partition and all-or-nothing, and a per-kind failure aborts before any
   write. A failed kind therefore manifests as *no new clean generation* — an
   ageing or invalid proof, i.e. `stale` then `unavailable` — not as
   `partial`. Introducing per-kind state is a **separate, explicitly gated
   change** with its own RED tests; this DD does not assume it.
4. `partial` is reserved for **projection-level** truncation (retained-workingset
   or page bound reached) and for any future per-kind state. `unavailable` covers
   a missing/expired proof, a transport failure, or an unqualified source.
5. **The `unknown` freshness value is not introduced in v1.6.** If AF only ever
   sees FMC pages, the freshness question is answered by proof state. `unknown`
   remains available as a future source-adapter capability for a source that
   genuinely reports no watermark (e.g. direct ACM), and would need its own
   public-contract amendment then.
6. ACM index lag is a source property, not a page property: the collector
   records the observation time it read, and the page reports the proof, not the
   lag.

**Alternatives.** (i) TTL-only freshness, ignoring proof identity — refuted:
a TTL does not distinguish "freshly proven" from "proven long ago and never
re-proven", and #2599's caveat (a source can be incomplete without failing)
means the proof is necessary but not sufficient evidence of enumerable
completeness. (ii) `observed_at`-only, no freshness field — hides the absence of
proof. (iii) Introduce `unknown` now — a public enum plus Console coordination
for a value the FMC path would never emit. (iv) Derive `partial` from a failed
kind — requires per-kind proof state that does not exist (D3.3).

### D4 — ACM Search source adapter: collect at sync time, publish only proven state

**Proposed (S-A).** `pkg/fleet/acm` is a **collection adapter, not a read
surface**: `ListManagedResources`/`GetManagedResource` have no production
caller, and AF's remote inventory reader is `fmc.HTTPClient`. Nothing here turns
that package into a Console-facing API — its job is to produce observations for
the cache.

1. **Identity and visibility.** The collector runs as the configured hub Search
   principal. Search visibility defines the collection surface, and the
   adapter never claims the surface is complete: a successful empty result is
   not evidence of absence. Exact GVK is required; `Kind` is never used to
   infer group/version, and no discovery call fills a missing GVK.
2. **Query shape.** Label projection is evidence-backed: the 2026-11-18 probe
   showed `label` present and complete for sampled labeled Pods and Namespaces,
   absent entirely when the object has no labels, and never truncated. The
   collector observes **objects**, not pre-filtered managed sets: the existing MCP collector writes every object of a configured
   kind with its label state, and inheritance is applied at read time
   (§3.7). The ACM source must do the same so the two sources produce the same
   cache shape; it may additionally filter the managed query for cost, but the
   projection's `Scope.Included` filter remains the authority. A dedicated
   paginated GraphQL document is added; `acm.SearchQuery` is **not** modified
   because `Check` and `Ping` share it.
3. **Traversal.** `orderBy: "uid asc"` requested explicitly, internal chunk
   independent of any caller limit, `offset` continuation, per-chunk checks:
   `_uid` present, strictly increasing, no duplicates, stable `count`. Any
   deviation, saturation, page cap or `sum(items) != count` marks the
   collection **incomplete** for that kind; one restart from offset 0 is allowed
   before marking incomplete. **Wall clock is part of the contract**: a repair
   stages for at most 60 s (`created_at + 60s` cap in the commit script), and
   collection happens after `begin` and before the first staged write, so
   begin → collect (including the restart) → commit must fit that window. A
   collection that cannot finish inside it must abandon staging and let the
   previous proof age out.
4. **Publication.** Results are written only through the existing
   fenced/staged cache path (DD-FLEET-012/013/016), including the
   **`empty_verified`** path for a zero-item collection: the adapter records an
   applied, fully-traversed zero-item observation and the commit path marks it
   verified-empty (the script already requires `empty_verified=1` for an empty
   staged set). The adapter never decides `empty` vs `partial` and never emits a
   failure envelope for a successful traversal (G10); that is FMC page
   assembly's decision, from the partition proof (D2.4/D3). An incomplete or truncated
   kind **must not** publish a clean proof: the existing code already returns on
   the first kind error before any write and commits only a clean set, so the
   previous proven generation stays readable and the partition ages into
   `stale` then `unavailable`. The residual risk is a source that reports
   complete while being incomplete (§3.6), which is why the guards in D4.3 are
   mandatory rather than advisory.
5. **Identity projection.** Canonical six-field ID, `_uid` used only for
   ordering and duplicate detection, never serialized, never part of the public
   ID (DD-FLEET-015: no target-UID claim). Cross-cluster, missing-`apiversion`
   or ambiguous items fail the kind, not just the item. The collector emits a
   validated canonical identity and nothing index-shaped: the repair writer
   passes identity plus `public_id` to `repair_reconcile`, which composes the
   lexicographic member and atomically stages and swaps **both** indexes for one
   staged generation (D2.5a). Index ownership lives in the writer, not in the
   source adapter.
6. **Label precedence** stays ADR-053, evaluated by the shared resolver; an
   invalid label is invalid, never absent, and never falls through to
   inheritance.

### D4.1a — FMC ACM source configuration (cross-repo contract)

The operator renders this block into FMC's `config.yaml`
(`fleetmetadatacache-config`) and FMC parses exactly these keys. The rendered
values are **resolved artifacts (paths), never policy**: the operator resolves
`FleetTrustSpec` to a CA path and mounts the token secret; FMC receives paths
only.

```yaml
sources:
  acm:
    endpoint: https://search-api.example:4010   # required, absolute HTTPS
    tokenPath: /etc/fleet-token/token           # required, absolute path
    caFile: /etc/fleet-tls/acm-source/ca.crt    # optional; absent = system trust
```

Go shape it mirrors (`pkg/fleet/fmc/config`):

```go
type SourcesConfig struct {
	ACM *ACMSourceConfig `yaml:"acm,omitempty"`
}

type ACMSourceConfig struct {
	Endpoint  string `yaml:"endpoint"`
	TokenPath string `yaml:"tokenPath"`
	CAFile    string `yaml:"caFile,omitempty"`
}
```

Key naming follows existing conventions on purpose: `caFile` matches
`valkey.tls.caFile`, `tokenPath` matches `fleet.TokenPath`. There is **no nested
`tls:` struct** in FMC's config — trust resolution stays in the operator.

Rules:

1. **Absence means today.** No `sources` key → the MCP/GitOps collector runs and
   the rendered config is byte-identical to the current one.
2. **Presence selects ACM** for this FMC deployment. At most one source may be
   declared; an unknown or additional `sources.*` entry is a configuration error.
3. **Fail closed at startup**, not at first sync: `endpoint` must be an absolute
   `https` URL with a host; `tokenPath` must be absolute and readable;
   `caFile`, when set, must be absolute. Unknown keys under `sources.acm` are
   rejected (strict decoding), so an operator typo cannot silently disable a
   guard.
4. **No fallback.** An ACM collection failure for a registered cluster yields an
   incomplete observation → no clean commit → `unavailable`. It never degrades to
   the MCP collector and never becomes `empty`.
5. **Membership is unchanged** (D1.3a): the gateway registration CR decides which
   clusters exist. `sources.acm` selects the collector, not the cluster set; a
   registered cluster invisible in Search is a collection failure.
6. **Rotation without restart**: the token is read through the existing file token
   source and the CA through the existing CA reloader — no new mechanism, and no
   token ever appears in config, logs or status.
7. **Deliberately not configurable**: traversal page size and guards (correctness
   parameters, not policy), timeouts, per-cluster source lists, and visibility
   settings. Adding any of these needs its own decision, not an operator knob.
7a. **Timeouts are derived, not configured.** Each Search request takes the
   smaller of a fixed per-request cap (provisional 10 s, the value the shared ACM
   client already uses) and the **remaining staging budget** minus a commit
   reserve (provisional 5 s). If a viable request cannot start — remaining budget
   below a floor (provisional 2 s) — the collector abandons staging instead of
   starting a request that cannot finish, so the previous proven generation
   simply ages out. An operator-set timeout cannot be expressed safely: a value
   above the 60 s staged-lifetime cap makes every collection fail permanently,
   and a value below normal Search latency makes it fail under load. Tuning
   latency means changing the staging cap or the chunking (DD-FLEET-016
   territory), not a per-deployment knob.
7b. **One deadline per repair, owned by the repair path.** The collector reads its
   deadline from the context; exactly one component may put it there. The FMC
   repair path sets it once, immediately after a successful `BeginRepair`, from
   that generation's staging lifetime, and passes the same context through every
   kind's collection. N kinds share one window. A per-kind fallback would let a
   seven-kind sync outlive the 60 s staged generation, which is why the collector
   must never derive its own budget in production.

**Cross-repo status (2026-11-20).** The operator half of D4.1a is implemented and
merged: kubernaut-operator#649 (`feat/647-fmc-acm-source-config`, merge commit
`fb8a3f0c`, closes kubernaut-operator#647). It renders exactly
`sources.acm.{endpoint, tokenPath, caFile}` and, for consumers, the key-less
fleet block that increment 1d accepts. Two facts from that landing bind this
side:

1. **Our strict decoding is now load-bearing.** The operator relies on FMC
   rejecting a mismatch rather than silently ignoring it; an operand without
   increment 2 decodes `sources` leniently and would drop the block while
   running the MCP collector.
2. **A present-but-empty `sources` block is a startup failure here.** The
   operator therefore rejects an empty declaration at admission
   (`UT-FLEET-647-VALIDATE-005`) and its renderer cannot emit one
   (`UT-FLEET-647-SRC-009`); rules 1-2 of this section are the contract both
   sides now test.

**Alternatives.** (i) Reuse the v1 per-request adapter (rejected in D2).
(ii) Keyset continuation — unsupported (spike: silent `count=0`).
(iii) Single-shot bounded query with fail-closed saturation — keeps 1000-item
ceilings; retained as a fallback if the guarded offset walk cannot be
implemented safely. (iv) Let the source adapter decide publication without the
fenced path — bypasses the proof lifecycle that every scope consumer relies on.

### D5 — AF authorization for federated inventory reads

**Proposed (A-A).**

1. Existing named capabilities (`kubernaut_list_managed_resources`,
   `kubernaut_get_managed_resource`) stay required; service-account identities
   stay rejected on the Console surface.
2. A non-local read additionally requires a `list` on `namespaces` (`core/v1`,
   no name) in the **hub** cluster through `auth.TargetAuthorizer.CheckTarget`.
   This is defense in depth, not a mitigation: a `view` user can list
   namespaces, so it does not make the projection per-user.
3. Per-user remote visibility remains #2600. The residual — every authorised
   Console user sees the collector principal's surface — is stated in the
   Console documentation.
4. Typed audit is unchanged: one event per authenticated read with the bounded
   `result_state`.

**Alternatives.** (i) No extra guard — fewer moving parts, no defense in depth.
(ii) A per-source capability check (ACM-only) — reintroduces the peer model into
AF. (iii) Block the surface until per-user visibility exists — correct but
punts the whole feature to post-1.6.

### D6 — Gate removal happens only on qualification, and reverts atomically

**Proposed (G-A).**

1. **Two gates, two conditions.** The gate AF applies is backend-neutral, not
   ACM-specific: `ListInventory` today rejects *every* non-local cluster, so the
   GitOps/MCP lane is blocked by the same line. They are separated:
   - **a. Non-local enablement (GitOps lane).** Remove the rejection once FMC
     serves the inventory route, AF dispatches to `fleet.fmc.endpoint` (D2.1a),
     and the GitOps journey passes. Requires no ACM anything.
   - **b. ACM-sourced qualification.** ACM-sourced deployments additionally
     require the source adapter, the configured hub Search identity, and a live
     journey on real ACM with a least-privilege principal. Membership is
     unchanged: registered clusters only (D1.3a).
   Order matters: (a) can ship without (b); (b) never ships without (a).
2. Qualification is declared by the reader (an optional capability the FMC
   client implements), not by a configuration flag and not by "whatever URL
   answers".
3. Rollback is an atomic reviewed revert: the rejection and the gate return; no
   compatibility mode, no data migration.

**Alternatives.** (i) Configuration flag — a knob that can be set wrong and
drifts from the code that was qualified. (ii) Remove the gate as soon as the
API exists — ships an unproven completeness claim. (iii) Keep the gate forever —
abandons #2593's goal.

## 6. Page assembly and fail-closed mapping

Wire status follows the existing handler mapping and is **not** changed by this
DD: `partial`/`unavailable` → `503` state-only, `forbidden` → `403`,
`not_found` → `404`, only a malformed cursor → `400`.

| Condition | Result |
|---|---|
| Proven partition, all kinds complete, ≥1 item | `success` (200), `freshness=fresh`, cursor when more remain |
| Proven partition, all kinds complete, no item | `empty` (200), `freshness=fresh` — "no managed resource", never "nothing exists" |
| A kind failed or truncated (no new clean commit) | The previous proof ages into `stale` — **200 with rows**, `freshness.state=stale` (D3.2a) — then `unavailable` (503) state-only once it is gone. **Not** `partial`: per-kind state does not exist (D3.3) |
| Projection-level truncation (working set / page bound) | `partial` (503) state-only |
| Proof missing, expired, or source unqualified | `unavailable` (503) state-only |
| Capability or namespace-list SAR denied | `forbidden` (403) state-only |
| Cursor missing because the cache moved on | Re-run the sorted projection, return its first page (200) |
| Cursor malformed | `400` |
| Exact read: identity mismatch / absent / invisible | `unavailable` (503) / `not_found` (404) |

Invariants: explicit cluster identity, no fallback to local or another cluster,
no records/cursors on state-only responses, metadata-only payloads (no object
data, secrets, tokens, arbitrary labels or annotations). `empty` + `partial` is
an **invalid envelope** that `Page.Validate()` rejects; it must never be
constructed by a source adapter or a page assembler (G10).

## 7. Consequences

**Wins.** One federation boundary instead of two; AF no longer carries source
semantics; the per-page O(N) walk disappears (cache read + bounded projection);
ACM index lag and offset-skip residue become collector concerns handled by
reconcile and the proof lifecycle, not user-visible per-request behaviour;
freshness becomes evidence-backed (`fresh`/`stale`) instead of `unknown`; the
GitOps-validated FMC path ships inventory first, and ACM becomes one more
source.

**Costs and risks.**

- Greenfield work moves into FMC: a route, a cache-backed page/cursor contract
  and a new source adapter. The v1 draft's AF-side adapter is dropped, so its
  effort is not lost but relocated.
- The ADR-068/DD-FLEET amendment is a prerequisite; until it lands, code and
  documents disagree (this DD states the model it implements).
- The projection is managed-only, so Console inventory means "resources
  Kubernaut manages"; unmanaged browsing is explicitly not this feature. The
  cache itself holds every observed object (D2.2), so that filter is a code
  obligation, not a storage property.
- An incomplete or failed kind shows up as an ageing proof (`stale`, then
  `unavailable`), not as `partial` — the proof has no per-kind state (D3.3).
  `partial` therefore means "this page was truncated", not "part of the fleet
  failed".
- Repair cycles are bounded by a 60 s staging window (D4.3), so a large cluster
  can fail collection repeatedly and stay `unavailable`; the ceilings in D2.7
  make that a design limit to settle, not an incident.
- ACM collection still depends on the hub Search principal's visibility; a
  restricted principal shrinks the inventory and cannot be distinguished from a
  small fleet — the documented limitation, unchanged by this DD.
- Rancher and any further source is deferred; the seam admits it, the contract
  does not cover it yet.

## 8. Verification gates (planned, not passes)

**Wiring Manifest (planned; IDs are proposals):**

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

**Critical contracts:** proven-vs-partial inventory; `empty` only from a proven
partition; freshness from proof state (never invented); collector truncation
never publishes clean; label precedence and exact GVK in the collector; canonical
identity and `_uid` non-disclosure; cursor drift re-runs the projection; AF never
contacts a source directly; qualification is declared, not inferred; denied SAR
never reaches FMC; metadata-only egress; state-only failures at their real wire
statuses; empty is only ever a verified-empty generation, never empty+partial.

**Evidence tiers.** Unit ≥90% of in-scope scenarios and 100% of critical
scenarios (Ginkgo). Integration: every wiring row through production dispatch,
including a real FMC HTTP path and HTTP fixtures for Search. Live: one journey
on real ACM (helios08) with a least-privilege principal, plus the GitOps lane on
the existing kind/real-MCP path — this is the production-journey precondition in
§1.2.

## 9. Open questions (each needs its own gate)

1. ~~**ADR-068 / DD-FLEET amendment**~~ — **approved 2026-11-18** (peer → facade,
   source contract, preserved boundaries). No longer blocks implementation.
2. ~~**Membership source for ACM clusters**~~ — **resolved**: the gateway
   registration CR is authoritative (D1.3a); ACM cannot override it; source
   selection is per deployment. A registered cluster invisible in Search is a
   collection failure, not an empty fleet.
3. ~~**`partial` persistence**~~ — dissolved: a failed kind produces no clean
   commit, and the proof has no per-kind state, so the partition ages `stale` →
   `unavailable` (D3.3). `partial` is page truncation only.
4. **Freshness threshold** — the boundary between `fresh` and `stale`, derived
   from remaining proof life; must be `<` cache TTL (45 s) and `>` sync interval
   (30 s) or `stale` is unreachable (D3.2a).
5. **Rancher / further sources** — contract shape only; not this release.
6. **Per-user visibility (#2600)** — the residual every authorised user sees the
   collector's surface.

## 10. Approval record

| Decision | Proposed | Alternatives presented |
|---|---|---|
| D1 boundary | FMC only; FMC selects the source; qualification declared per source | peer model (superseded); AF contacts several sources; MCP tools as inventory |
| D2 page | managed-only projection over the proven cache; `empty` requires proof; local drift rule | per-request source walk; include unmanaged; `empty` from TTL |
| D3 freshness | proof-derived `fresh`/`stale`; no `unknown` in v1.6 | TTL-only; `observed_at` only; add `unknown` now |
| D4 ACM source | sync-time collection, uid-offset with mutation guards, publish only proven partitions | per-request adapter; keyset; single-shot cap; unguarded publication |
| D5 authorization | named capability + hub namespace-list SAR (defense in depth) | no guard; per-source check; block until #2600 |
| D6 gate removal | on qualification, declared by the reader, atomic revert | config flag; remove on API availability; keep gate |

**Approved 2026-11-18:** D1–D6 as proposed, and the ADR-068/DD-FLEET amendment.

**Review history.** Two independent reviews ran the same day. The first found the
superseded peer model and the TTL-as-proof overclaims. The second found four gaps,
three of which stand: `as_of` had no evidence source and is now the projection
time with `stale` from remaining proof life (D3.2/2a), AF's inventory endpoint
must not inherit `fleet.endpoint` (D2.1a), and the "ACM gate" is backend-neutral
so removal splits into two conditions (D6.1). Its membership finding was
**reassessed against the authoritative #2593 decision**: the gateway registration
CR is the source of truth and ACM cannot override it, so there is no membership
choice to make — D1.3a now states that, and the "blocker" is withdrawn. The
review also prompted the label-projection probe confirming Search projects labels
completely for sampled workloads.

**Cross-session correction (2026-11-18):** `pkg/fleet/acm` is a *collection*
adapter, not a read surface (`ListManagedResources` has no production caller; AF
reads `fmc.HTTPClient`). The earlier "empty observation must be proven-complete at
the adapter" framing was therefore wrong; D4.4/G10 now place that decision in FMC
page assembly, and the owning test was narrowed by its author to "applied
zero-item Search is an empty observation, not unavailable".

**Correction history.** Draft v1 (2026-11-18, earlier the same day) specified
`AF → ACM Search` with `unknown` freshness and an ACM-only qualification. It was
withdrawn after the corpus triage showed the 2026-11-11 decision already
required `AF → FMC` and that cache TTL is not a completeness proof (#2599). Those
four original decisions survive with relocated ownership: visibility → D4.1,
traversal → D4.3, identity → D4.5, freshness → D3.
