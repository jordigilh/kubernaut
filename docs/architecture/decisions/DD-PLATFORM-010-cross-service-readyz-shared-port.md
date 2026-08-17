# DD-PLATFORM-010: Cross-Service Readyz Checks Use a Dedicated, Unauthenticated Route on the Already-Open Business Port

**Date**: August 16, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 96%
**Related**: Issue #1985 (BR-AUDIT-005 v2.0, DataStorage readiness gate), `DD-FLEET-004`
(FMC ping/clusters endpoint), Issue #1993 (ADR-068 gap closure, IA-2/AC-3),
`pkg/shared/auth` (DD-AUTH-014), `pkg/audit.DataStorageProber`,
`pkg/fleet/readiness.ScopeCheckerProber`

---

## 🎯 **DECISION**

**When Service A needs to gate its own readiness on Service B's reachability,
Service B SHALL expose a dedicated, purpose-built, side-effect-free `/readyz`
handler -- the same handler already backing its own kubelet probe -- as a
top-level, unauthenticated route on its main business API port. It SHALL
NOT: (a) widen the kubelet-only health port's NetworkPolicy to accept
cross-pod traffic, or (b) repurpose a real, authenticated business-data
endpoint as the health-check target.**

This is the canonical pattern going forward for any new cross-service
readiness dependency. It is applied retroactively to the two existing
instances in the codebase, each of which violated one half of the rule:

- **DataStorage (#1985)** violated (a): it widened its kubelet-only health
  port 8081's NetworkPolicy ingress+egress to accept traffic from all 10
  audit-writing services, so that `pkg/audit.DataStorageProber` could poll
  `/readyz` there. This exposes the exact port kubelet depends on to keep
  the pod alive to a materially larger blast radius (10 additional pods'
  worth of traffic and any bug in their handling of that port) than kubelet
  probing alone requires.
- **FleetMetadataCache (`DD-FLEET-004`)** violated (b): `fmc.HTTPClient.Ping()`
  targets `GET /api/v1/clusters` (`ClustersPath`) -- a real, legitimate
  business-data endpoint used for actual scope-check queries -- purely
  because it happened to be a cheap, already-reachable GET on the open API
  port. A later, independent decision (#1993/ADR-068) added DD-AUTH-014
  TokenReview+SAR authentication to that entire API mux, so `Ping()` now
  pays a live, uncached authn/authz round-trip to kube-apiserver on every
  15s poll from both Gateway and RemediationOrchestrator -- a cost
  `DD-FLEET-004` never accounted for and one that is fundamentally
  unnecessary for a reachability check.

---

## 📊 **Context & Problem**

Both DataStorage and FMC need another service (or several) to be able to ask
"are you reachable and functioning?" without becoming a full API client.
Each solved it differently, and neither solution is right:

**DataStorage's dedicated-port approach** treats the *cross-service*
reachability check as if it were the *kubelet* liveness/readiness check --
same port, same NetworkPolicy scope. But these are two different consumers
with two different trust/scope requirements: kubelet traffic originates from
the node and (depending on CNI) may or may not even be subject to
pod-selector NetworkPolicy enforcement, while cross-pod service-to-service
traffic is exactly what NetworkPolicy is meant to scope. Conflating them
means the kubelet-facing port's attack surface now scales with the number of
services that need a readiness signal from it, not just with "does kubelet
need to reach this pod."

**FMC's endpoint-reuse approach** treats "cheap and already reachable" as
sufficient justification for a health-check target, without accounting for
what else might later get bolted onto that same route. `ClustersPath` was a
reasonable choice under `DD-FLEET-004`'s original constraint (FMC's health
port 8081 had zero ingress at the time, and opening it was explicitly
rejected to preserve that boundary) -- but the decision was made purely on
NetworkPolicy topology grounds, with zero consideration of what
authentication that endpoint might carry, now or later. When #1993 added
authentication to the whole apiMux for defensible, unrelated reasons (IA-2/
AC-3), `Ping()` silently inherited a cost nobody evaluated for it.

Chi's (and Go's `net/http`'s) native support for per-route middleware scoping
makes both problems avoidable with the same mechanism: register a dedicated,
minimal `/readyz` handler as a top-level route on the router/mux that also
serves the main business API, but outside whatever middleware group enforces
authentication on the *actual* business routes. DataStorage already does
this internally -- its `/api/v1` routes are the only ones wrapped in
`auth.Middleware.Handler` (`pkg/datastorage/server/server_routes.go:68-71`);
anything registered at the router's top level is unauthenticated for free,
no changes to `pkg/shared/auth` required.

This is not a novel pattern for this codebase: `pkg/apifrontend/handler/router.go`
already documents and implements the identical two-tier shape ("Routes are
organized into two tiers: Public (no auth): `/healthz`, `/readyz`, `/metrics`
...; Authenticated: `/a2a/invoke`, `/mcp`"), and Gateway's `pkg/gateway/server.go`
likewise serves `/health`/`/healthz`/`/ready` unauthenticated alongside its
authenticated `/api/v1/signals/*` routes on the same chi router/port. This DD
extends that already-proven, in-repo shape to the two services that
currently deviate from it (DataStorage, FMC), rather than introducing a new
one.

---

## 🔍 **Alternatives Considered**

### **Option A: Dedicated, unauthenticated `/readyz` on the business port** ✅ **CHOSEN**

Reuse each service's existing, purpose-built readiness handler (DataStorage's
`s.ReadinessHandler()`; FMC's `fmc.ReadyzHandler(ready, pinger)`) by
registering it a second time, as a top-level route outside the
auth-enforcing group, on the port that's already open to legitimate callers
via NetworkPolicy.

- ✅ Zero new NetworkPolicy surface: the caller set already has ingress to
  the business port for its real traffic (DataStorage: audit writes; FMC:
  scope-check queries).
- ✅ Zero new auth cost: the route sits outside the authenticated group by
  construction, not via a new exemption mechanism that has to be built and
  trusted.
- ✅ Zero new logic: the handler function is identical to the one already
  serving kubelet; only its *registration* is duplicated, not its behavior.
- ✅ Kubelet's own health port is untouched and stays minimally exposed --
  the specific attack-surface concern that motivated this reconsideration.
- ➖ One extra route registration per service that needs this (accepted:
  trivial, and the alternative is strictly worse on every other axis).

### **Option B: Widen the kubelet-only health port's NetworkPolicy (status quo for DataStorage, #1985)** ❌ REJECTED

- ❌ Expands the blast radius of the port kubelet depends on to keep the pod
  alive to every service that needs a readiness signal from it -- a
  materially larger DoS/failure-cascade surface than kubelet-only traffic.
- ❌ Once #1985's widening exists, there is no NetworkPolicy benefit left to
  preserve by keeping it -- it is pure incremental risk versus Option A.
- ➖ Simpler at the time it was written (no route-registration work), but
  rejected once the shared-port alternative was identified as equally cheap
  and strictly safer.

### **Option C: Reuse a real, authenticated business-data endpoint as the health target (status quo for FMC, `DD-FLEET-004`)** ❌ REJECTED

- ❌ Silently inherits whatever authentication/authorization requirements are
  later added to that endpoint for unrelated reasons (exactly what happened
  when #1993 added DD-AUTH-014 to FMC's entire apiMux).
- ❌ Cannot be made unauthenticated without also weakening the real business
  endpoint's legitimate protection -- the two concerns (serve real data,
  answer "are you up") are fused together and can't be decoupled later
  without a breaking change.
- ➖ Avoided a new route at the time (`DD-FLEET-004`'s stated rationale), but
  rejected now that Option A is confirmed to cost the same (one route
  registration) while avoiding both failure modes.

---

## ✅ **Consequences**

- **DataStorage**: new unauthenticated `GET /readyz` on port 8080
  (`pkg/datastorage/server/server_routes.go`), reusing `s.ReadinessHandler()`
  verbatim. `kubernaut.datastorage.healthUrl` (`_helpers.tpl`) repointed from
  `:8081/readyz` to `:8080/readyz`. The #1985 NetworkPolicy ingress block
  opening port 8081 to the 10 caller pods is removed, and
  `kubernaut.np.datastorageEgress` reverts to port 8080 only. Port 8081
  reverts to kubelet-only, matching the pre-#1985 (and FMC's original)
  posture.
- **FleetMetadataCache**: the existing `fmc.ReadyzHandler(ready.Load,
  deps.cacheReader)` is additionally registered directly on `apiMux` in
  `cmd/fleetmetadatacache/main.go`'s `buildFMCServers`, before/outside
  `authMiddleware.Handler(apiMux)`'s wrapping. `pkg/fleet/scope_factory.go`'s
  FMC `remoteChecker` construction retargets `Ping()` from `ClustersPath` to
  `/readyz`. `ClustersPath` itself is unchanged -- still authenticated, still
  used for real scope-check data queries. `DD-FLEET-004` is amended (not
  superseded) to record this retarget and its rationale.
- `IT-FMC-1683-A-002` (`cmd/fleetmetadatacache/main_wiring_test.go`), which
  currently asserts "no liveness handler duplicated onto the API mux" as a
  feature, is updated to assert the new (intentional) duplication instead --
  this DD explicitly overrides that specific prior design point in favor of
  eliminating the auth cost.
- **Follow-up (not implemented by this DD)**: this repository's Helm chart
  is the development/E2E-test deployment path; `kubernaut-operator` is the
  production deployment path and defines its own resource manifests
  independently (same chart/operator parity split noted in
  `DD-PLATFORM-008`). Confirmed via code search that `kubernaut-operator`
  (`internal/resources/networkpolicies.go`) has its own
  `dataStorageNetworkPolicy(kn)` and `datastorageEgressRule()` -- but
  DataStorage's main API port there is **8443 (TLS)**, not 8080/8081 as in
  this chart, and `datastorageEgressRule()` currently only opens 8443 with
  no port-8081 equivalent visible -- suggesting the operator path has not
  (yet) applied an equivalent #1985 cross-service-readiness-gate
  NetworkPolicy widening at all. This needs its own audit against its
  actual current behavior (not assumed identical to this chart) before
  deciding whether it needs the DD-PLATFORM-010 pattern applied, a
  from-scratch #1985-equivalent implementation, or neither -- tracked as
  [jordigilh/kubernaut-operator#360](https://github.com/jordigilh/kubernaut-operator/issues/360)
  (v1.6 milestone), not actioned in this DD since that repository is not
  checked out in this workspace.

## 🔗 Related Decisions

- `DD-FLEET-004` (FMC Ping/ClustersPath endpoint): amended by this DD for
  the health-check use case specifically; its topology rationale (avoid
  opening FMC's zero-ingress health port) remains correct, only the chosen
  target endpoint changes.
- `DD-AUTH-014` (Middleware-Based SAR Authentication): unchanged. This DD
  does not modify `pkg/shared/auth`; it relies entirely on existing
  per-route-group middleware scoping already used by DataStorage's `/api/v1`
  routes.
- Issue #1993 (ADR-068 gap closure, IA-2/AC-3): the auth requirement this DD
  routes around for the health-check use case remains fully in force for
  FMC's real `/api/v1/scope/check` and `/api/v1/clusters` business routes.
