# DD-FLEET-004: FMC's Readiness Ping Targets `ClustersPath`, Not a Duplicated `/healthz`

**Status**: ✅ Approved & Implemented
**Date**: 2026-07-24
**Author**: AI Assistant (reviewed and revised per user feedback)
**Related**: Issue #1683, Issue #1553, ADR-068, BR-INTEGRATION-065

---

## Context

Issue #1683 aligns FMC's HTTP interface with every other Kubernaut HTTP-API service: TLS on
the API port and the 3-port standard (API `:8080`, health `:8081`, metrics `:9090`). Before
this split, FMC's liveness (`/healthz`) and readiness (`/readyz`) handlers lived on the same
port as the business API.

`fmc.HTTPClient.Ping()` is the backing `Pinger` for `readiness.ScopeCheckerProber`
(`pkg/fleet/readiness/probers.go`), which GW/RO wire into their own fail-closed
`readiness.Gate` (Issue #1553/ADR-068): when Fleet is enabled and the scope-checker backend is
unreachable, the *entire* GW/RO pod is marked `NotReady` and removed from Service endpoints.
`Ping()` calls `c.baseURL + HealthzPath`, where `baseURL` is FMC's **API** base URL (the same
one used for real scope-check calls) — so splitting `/healthz` off to a separate port would
break it unless `Ping()`'s target moved too.

### Original design (superseded)

The first implementation (committed in PR #1727's initial push) dual-registered a liveness-only
`/healthz` handler on **both** the API mux (now TLS) and the new health mux (plain HTTP),
leaving `Ping()`'s URL completely unchanged. This preserved `Ping()`'s contract with zero
plumbing changes to `fmc.HTTPClient`/`FleetConfig`, but was flagged on review for duplicating a
liveness handler across two ports — a real, if minor, "why do we need two of these" smell.

### The constraint that actually matters: `NetworkPolicy`

Two alternatives were on the table when this was reopened:

1. **Keep the dual registration** (as originally committed).
2. **Widen `NetworkPolicy` to expose the health port to GW/RO**, and add a second FMC base URL
   (health endpoint) so `Ping()` could target the *real*, single-registration `/healthz` there.

Inspecting FMC's rendered `NetworkPolicy`
(`charts/kubernaut/templates/fleetmetadatacache/fleetmetadatacache.yaml`) settled this: ingress
is scoped to **port 8080 only**, from GW/RO pods specifically. Port 8081 (health) has **no**
ingress rule at all — pod-to-pod traffic to it is default-denied by `NetworkPolicy` semantics;
the only traffic that reaches it is the node-local kubelet probe, which every common CNI
(Calico, Cilium, etc.) exempts from `NetworkPolicy` enforcement since it doesn't originate from
a pod's network namespace. The health port is *deliberately* kubelet-only. Option 2 would trade
that boundary away just to give `Ping()` a "purer" liveness target — not a good trade.

## Decision

`fmc.HTTPClient.Ping()` targets `ClustersPath` (`GET /api/v1/clusters`) instead of `HealthzPath`.
`/healthz` is removed from the API mux entirely and lives exclusively on the dedicated
(kubelet-only) health port, matching `/readyz` and matching Gateway's pattern.

```go
// Ping checks connectivity to FMC's API by calling ClustersPath on the same
// base URL used for scope checks. ... DD-FLEET-004: Ping deliberately
// targets ClustersPath, not HealthzPath, which is kubelet-only.
func (c *HTTPClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+ClustersPath, nil)
	// ...
}
```

`ClustersPath`'s handler (`handleListClusters`) only reads FMC's in-memory `ClusterRegistry` —
no Valkey round-trip — preserving the same "shallow liveness, not deep readiness" property that
motivated choosing `/healthz` over `/readyz` for `Ping()` in the first place (Issue #1553): a
transient Valkey hiccup on FMC's side does not cascade into GW/RO's own readiness gate.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Dual-register `/healthz` on both the API mux and health mux (original #1683 design) | Duplicates a liveness handler across two ports for no benefit beyond URL-preservation, once a same-port alternative (`ClustersPath`) exists |
| Widen `NetworkPolicy` to allow GW/RO → FMC:8081, retarget `Ping()` at a new FMC health base URL | Trades away the health port's deliberate kubelet-only network boundary; adds a second FMC address to `fmc.HTTPClient`/`FleetConfig`'s config surface across every Fleet-dependent service (GW, RO, and any future caller) |
| Relax ADR-068/#1553's fail-closed policy for FMC specifically (tolerate transient unavailability) | Out of scope: a pre-existing, deliberate cross-service decision (7 services), not something #1683's port-layout work should silently reinterpret. Revisiting it is a separate, larger discussion the user explicitly declined to fold into this fix. |
| Have `Ping()` call `ScopeCheckPath` with a synthetic/sentinel resource | Works, but is a less honest signal than `ClustersPath` (implies scope-check semantics are being tested, when only reachability is) and requires picking parameters that can't collide with a real cluster/resource |

## Consequences

- **Positive**: The health port (8081) remains kubelet-only with no `NetworkPolicy` change —
  its network boundary is unaffected by this issue.
- **Positive**: No duplicated liveness handler; `/healthz` has exactly one registration site.
- **Positive**: Zero new config surface — `Ping()` still uses FMC's single existing API base
  URL and the CA-verified transport already wired for scope-check calls (Issue #1683 Unit B).
- **Neutral**: `Ping()`'s exact request path changes (`/healthz` → `/api/v1/clusters`); its
  call signature and nil/err success contract do not.
- **Out of scope, deliberately**: ADR-068/#1553's fail-closed policy itself (should GW/RO go
  `NotReady` pod-wide when Fleet is enabled and unreachable?) is unchanged by this decision.

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| `fmc.HTTPClient.Ping()` → `ReadyzPath` (amended, see below) | `readiness.ScopeCheckerProber.Probe()` (GW/RO's `readiness.Gate`) | `pkg/fleet/fmc/http_client.go` | `IT-FMC-1683-A-002` |
| `/healthz` health-port-only registration | `buildFMCServers` | `cmd/fleetmetadatacache/main.go` | `IT-FMC-1683-A-004` |

## Authority

Issue #1683, Issue #1553, ADR-068, BR-INTEGRATION-065.

---

## Amendment (2026-08-18, Issue #2169 / `DD-PLATFORM-010`)

**Amended, not superseded**: `Ping()`'s target moves from `ClustersPath` to `ReadyzPath`
(`/readyz`). This section records why, and explicitly calls out a semantic change to the
readiness signal that the original decision above did not anticipate.

### What changed and why

`DD-PLATFORM-010` established a fleet-wide standard: cross-service readiness checks must use a
dedicated, unauthenticated `/readyz` route on the already-open business port, never a real
business-data endpoint. Issue #1993/`DD-AUTH-014` had, after this DD was originally written,
added TokenReview+SAR authentication to FMC's entire `apiMux` -- including `ClustersPath` -- so
every 15s `Ping()` poll from GW/RO was silently paying a live, uncached authn/authz round-trip
to `kube-apiserver`, a cost this DD's original NetworkPolicy-only analysis never accounted for.
`ClustersPath` itself is unchanged and remains the correct target for real scope-check/cluster-list
data queries -- only the health-check *target* moves.

### Semantic consequence: shallow liveness becomes deep readiness (deliberate)

The original decision above justified `ClustersPath` partly because `handleListClusters` "only
reads FMC's in-memory `ClusterRegistry` -- no Valkey round-trip", preserving a **shallow**
liveness signal that would not cascade a transient FMC-side Valkey hiccup into GW/RO's
fail-closed readiness gate (Issue #1553/ADR-068). `fmc.ReadyzHandler` -- the handler `Ping()` now
targets -- pings `deps.cacheReader` (a real Valkey backend), making this a **deep** check: a
Valkey outage inside FMC will now correctly flip GW/RO's readiness to `NotReady`.

This is intentional, not an overlooked regression, confirmed by precedent already shipped under
`DD-PLATFORM-010` for DataStorage: DataStorage's own `/readyz` (`pkg/datastorage/server/handlers.go`'s
`handleReadiness`) pings both Postgres and Redis/DLQ, and that exact deep check is what
`pkg/audit.DataStorageProber` polls to gate all 10 audit-writing services' own readiness
(`BR-AUDIT-005 v2.0`) -- explicitly to close an audit-loss window, i.e. cascading `NotReady` on a
backend outage is the intended fail-closed behavior, not a side effect to avoid. Retargeting
FMC's `Ping()` to its own deep `/readyz` makes FMC consistent with that already-accepted pattern
rather than an outlier: a scope-check backend that cannot reach its own cache genuinely cannot
answer scope checks correctly, so signaling `NotReady` to GW/RO is the correct, not merely
tolerable, outcome.

### Updated consequences

- **Positive**: `Ping()` no longer pays a TokenReview/SAR round-trip per poll (the problem this
  amendment fixes).
- **Positive**: FMC's cross-service readiness signal is now consistent with the DataStorage
  precedent under the same fleet-wide standard (`DD-PLATFORM-010`).
- **Changed (intentional)**: `Ping()`'s failure mode now includes FMC's own Valkey reachability,
  not just FMC's process liveness/registry state. GW/RO's readiness gate (Issue #1553/ADR-068)
  will cascade to `NotReady` on an FMC-side Valkey outage where it previously would not have.
- **Unchanged**: `ClustersPath` remains authenticated and unchanged for real data queries;
  `Ping()`'s call signature and nil/err success contract are unchanged; the health port (8081)
  and NetworkPolicy topology are unchanged (`/readyz` rides the already-open API port 8080).

### Updated Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| `fmc.HTTPClient.Ping()` → `ReadyzPath` | `readiness.ScopeCheckerProber.Probe()` (GW/RO's `readiness.Gate`) | `pkg/fleet/fmc/http_client.go` | `IT-FMC-1683-A-002` (updated) |
| FMC unauthenticated `/readyz` on API port 8080 | `buildFMCServers`'s `topMux` | `cmd/fleetmetadatacache/main.go` | `IT-FMC-1683-A-003` (updated), `IT-FMC-2169-001/002` |

### Authority (amendment)

Issue #2169, `DD-PLATFORM-010`, `pkg/audit.DataStorageProber` (DataStorage precedent),
`BR-AUDIT-005 v2.0`.
