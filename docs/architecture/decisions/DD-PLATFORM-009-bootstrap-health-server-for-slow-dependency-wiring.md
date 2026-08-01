# DD-PLATFORM-009: Bootstrap Health Server for Slow Dependency Wiring

**Date**: July 28, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 88%
**Last Reviewed**: July 28, 2026
**Related**: DD-PLATFORM-008 (`startupProbe` for fleet-aware services), Issue #54,
DD-TEST-015 (Fleet E2E "deploy correctly the first time"), `pkg/shared/health`,
`cmd/fleetmetadatacache/main.go`, Issue #1729 (`cmd/kubernautagent/main.go`)

---

## 🎯 **DECISION**

**A service whose `main()` builds a blocking dependency (an MCP Gateway
connection, an OAuth2 token source, a cluster registry cache sync) *before*
starting any HTTP listener SHALL bind a minimal "bootstrap" health server
(new `sharedhealth.NewBootstrapServer` helper) at the very top of `main()`,
answering `/healthz` truthfully (200, the process is alive) and `/readyz`
honestly (503, dependencies are still wiring) for the duration of that
blocking work, then hand off to the service's real health server once
dependency wiring completes.**

This complements, and does not replace, DD-PLATFORM-008's `startupProbe`:
`startupProbe` buys a slow-starting pod extra time before kubelet evaluates
liveness/readiness at all, but it can only help if the health **port is
already bound and answering something** during that window. A service that
doesn't start listening until its blocking dependency wiring finishes gives
`startupProbe` nothing to check but "connection refused" -- indistinguishable
from a genuinely hung process -- for the entire wiring duration.

## 📊 **Context & Problem**

DD-TEST-015's Fleet E2E validation run (the first live run against the
fully chart-native `global.fleet.enabled=true` deploy path) found FMC
crash-looping on first boot despite already having DD-PLATFORM-008's
`startupProbe`. Root cause: `cmd/fleetmetadatacache/main.go`'s `run()`
calls `wireFMCDependencies` (which blocks on `mcpclient.NewResilient`'s MCP
Gateway connection and `registry.ClusterRegistry`'s cache sync) *before*
`buildFMCServers` ever starts the health HTTP listener. For the full
duration of that blocking call, FMC's health port isn't bound at all --
every probe attempt (startup, liveness, or readiness) gets "connection
refused," which `startupProbe`'s generous `failureThreshold` cannot
distinguish from a genuinely dead process. kube-mcp-server's registration/
AuthPolicy convergence lag under Fleet E2E's Kind cluster made this window
long enough to exhaust even DD-PLATFORM-008's 150s startup grace.

Gateway showed the same shape during the same validation run (its
`wireFleetOwnerResolution`/`mcpclient.NewResilient` also blocks before its
HTTP server starts). A first pass judged DD-PLATFORM-008's `startupProbe`
alone sufficient there, since Fleet infrastructure was already converged
pre-`helm install` (DD-TEST-015). A subsequent Fleet E2E re-validation run
proved that judgment wrong: under Kind-cluster CPU contention from four
services restarting simultaneously (TLS cert reload), Gateway's MCP Gateway
connection attempts took ~40s each, and `kubectl rollout status` timed out
waiting on a `startupProbe` whose `failureThreshold` budget was being spent
against "connection refused" instead of an honest 503. Gateway now also
carries the `NewBootstrapServer` fix (see Consequences) -- the "Follow-up"
below is resolved, not merely anticipated.

## 🔍 **Alternatives Considered**

### **Option A: Bootstrap health server, started before blocking dependency wiring** ✅ **CHOSEN**

Bind a minimal `net/http` server answering `/healthz`=200 (`AlwaysReadyLiveness`)
and `/readyz`=503 (`NotYetReady`) immediately at the top of `main()`, in a
goroutine, on the same `HealthAddr` the real health server will later reuse.
Shut it down (bounded 5s timeout) right before constructing the real,
dependency-gated health server.

- ✅ Kubelet sees a live, honestly-503 process throughout dependency wiring
  instead of "connection refused" -- `startupProbe` (DD-PLATFORM-008) can now
  do its job: it correctly waits out the readyz=503 window instead of
  exhausting its failure budget against a closed port.
- ✅ Minimal, reusable: one `sharedhealth.NewBootstrapServer(addr)` call,
  matching the existing `pkg/shared/health` package's shape
  (`NewHealthServer`), not a bespoke per-service pattern.
- ✅ Correctly reports `NotReady` (503, not a fake 200) while genuinely not
  ready -- doesn't paper over real dependency problems, just gives kubelet
  an honest signal to wait on instead of an ambiguous connection failure.
- ➖ Two health servers exist briefly, sequentially, on the same address
  (bootstrap, then real) -- accepted because the handoff is a bounded,
  synchronous `Shutdown()` before the real server binds, so there's no
  window where both listen at once.

### **Option B: Move the real health server construction earlier, gate its handlers on a readiness flag** ❌ REJECTED

Restructure `main()` so the real health server (with real liveness/readiness
handlers) starts first, and have those handlers check an `atomic.Bool`
flipped once dependency wiring completes.

- ❌ Larger refactor of `buildFMCServers`/the real health server's
  construction, which today legitimately depends on already-wired
  dependencies (e.g., a `Pinger` handler needs a live client to ping) --
  splitting "server construction" from "handler construction" adds
  complexity for the same net behavior Option A achieves with a temporary,
  fully separate, disposable server.
- ➖ Would avoid the "two servers" mechanic, but the bootstrap server's
  lifecycle is trivial and well-isolated (`net/http.Server` + one
  `Shutdown()` call), so this simplicity gain wasn't judged worth the
  larger restructure.

### **Option C: Rely solely on a longer `startupProbe` `failureThreshold`** ❌ REJECTED

Just keep raising DD-PLATFORM-008's `failureThreshold`/`periodSeconds` until
FMC's window is covered.

- ❌ Doesn't fix the underlying signal quality problem: "connection refused"
  is genuinely indistinguishable from a hung/crashed process, so this only
  delays, rather than resolves, a false-positive kill under sufficiently
  bad node contention -- there's no threshold that's both "long enough" and
  "still catches a real hang promptly."
- ❌ A service that legitimately becomes even slower later (e.g. a busier
  MCP Gateway) would silently need this threshold raised again, whereas
  Option A's fix scales with the actual wiring time automatically (the
  bootstrap server answers `NotYetReady` for exactly as long as needed).

---

## ✅ **Consequences**

- `pkg/shared/health/server.go`: three new exports --
  `AlwaysReadyLiveness`, `NotYetReady` (unconditional handlers), and
  `NewBootstrapServer(addr)` (wraps `NewHealthServer` with those two
  handlers, `enablePprof=false`).
- `cmd/fleetmetadatacache/main.go`: `run()` starts `NewBootstrapServer` in a
  goroutine immediately before `wireFMCDependencies`, then `Shutdown()`s it
  (bounded 5s context) immediately after, before `buildFMCServers` binds
  the real health server on the same address.
- `cmd/gateway/main.go`: `run()` starts `NewBootstrapServer` in a goroutine
  immediately before `buildAPIRegistry`/`registerAdapters` (the latter
  performs the blocking `wireFleetOwnerResolution`/`mcpclient.NewResilient`
  connection), then `Shutdown()`s it (bounded 5s context) right after
  `registerAdapters` returns and before `srv.Start()` binds the real health
  server on the same `HealthAddr`.
- `pkg/shared/health/server_test.go` (new): unit-tests
  `NewBootstrapServer`/`AlwaysReadyLiveness`/`NotYetReady` directly --
  `/healthz`=200, `/readyz`=503, and that `Shutdown()` fully releases the
  port so a subsequent `NewHealthServer` can bind the same address without
  conflict (the exact handoff both `cmd/fleetmetadatacache` and
  `cmd/gateway` rely on). This package had no prior test coverage.
- Validated: `go build ./...` clean; `go test ./pkg/shared/health/...
  ./cmd/gateway/... ./cmd/fleetmetadatacache/...` passing; DD-TEST-015's
  Fleet E2E re-run (first full-chart, fleet-enabled-from-first-install
  validation) is the proving journey for both fixes -- FMC's and Gateway's
  health ports now answer throughout their dependency-wiring windows
  instead of refusing connections.
- **Third application (Issue #1729, `cmd/kubernautagent/main.go`)**:
  `kubernaut-agent`'s `initializeAgent()` builds `registerFleetTools()`'s
  fleet MCP Gateway connection (same `mcpclient.NewResilient` shape as
  Gateway/FMC) *before* `startHealthAndMetricsServers()` binds the health
  port -- previously masked because, before #1729's Helm-wiring fix, KA's
  fleet client was always a no-op (`gatewayType`/`endpoint` unset -> immediate
  `nil, nil` return, zero blocking) since Helm never actually rendered
  `kubernautAgent.fleet`. Once #1729 wires real fleet config through, this
  same crash-loop shape reproduced in CI (`E2E (fleet)`:
  "kubernaut-agent not ready after 6m0s"). Fixed identically: `run()`'s
  `main()` now calls `startBootstrapHealthServer`/`stopBootstrapHealthServer`
  (mirroring `cmd/gateway/main.go`'s helpers of the same name) around
  `initializeAgent()`. Validated: `go build ./...` clean; `go test
  ./pkg/shared/health/... ./cmd/kubernautagent/...` passing; `golangci-lint
  run ./cmd/kubernautagent/...` clean; the Fleet E2E re-run (PR #1820) is the
  proving journey.

## 🔗 Related Decisions

- DD-PLATFORM-008 (`startupProbe` for Fleet-Aware Services with Slow Cold
  Starts): this DD is the necessary complement -- `startupProbe` only helps
  once the health port is actually bound and answering; this DD ensures
  that's true from the first instant of `main()`.
- DD-TEST-015 (Fleet E2E "Deploy Correctly the First Time"): the live
  validation run against DD-TEST-015's chart-native fleet-enabled deploy
  path is what surfaced this gap.
- Issue #54: umbrella Fleet E2E infrastructure this fix was discovered
  during.
