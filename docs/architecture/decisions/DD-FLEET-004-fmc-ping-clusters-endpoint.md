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
| `fmc.HTTPClient.Ping()` → `ClustersPath` | `readiness.ScopeCheckerProber.Probe()` (GW/RO's `readiness.Gate`) | `pkg/fleet/fmc/http_client.go` | `IT-FMC-1683-A-002` |
| `/healthz` health-port-only registration | `buildFMCServers` | `cmd/fleetmetadatacache/main.go` | `IT-FMC-1683-A-004` |

## Authority

Issue #1683, Issue #1553, ADR-068, BR-INTEGRATION-065.
