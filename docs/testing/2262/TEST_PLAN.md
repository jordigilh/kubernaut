# Test Plan: MCP `server/discover` Probe Starves the Legacy `initialize` Fallback

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2262
**Feature**: Bound the go-sdk v1.7.0 SEP-2575 `server/discover` probe with
its own short sub-timeout inside `pkg/fleet/mcpclient`, so a gateway that
hangs (rather than erroring) on that probe cannot consume the entire
`ConnectTimeout` budget before the SDK's own legacy `initialize` fallback
gets a chance to run. Phase 2 (referenced, not detailed here) exposes the
resulting (and pre-existing) `ResilienceConfig` fields via the Helm chart
and Go config layer.
**Version**: 1.0
**Created**: 2026-08-23
**Author**: AI Agent
**Status**: Draft
**Branch**: `fix/2262-mcp-discover-probe-hang`

---

## 1. Introduction

### 1.1 Purpose

`go-sdk@v1.7.0`'s `Client.Connect()` (`mcp/client.go`) always attempts the
newer stateless `server/discover` RPC (SEP-2575) before falling back to the
legacy `initialize` handshake. Per that file's own comment, it "fall[s] back
to the legacy initialize handshake on any non-modern error from
server/discover" — i.e. **any** error from the discover step (JSON-RPC or
transport-level) triggers the fallback, and the fallback `initialize` call
reuses the *original* `ctx`, not a context derived from the discover
attempt.

Both steps, however, share that one `ctx` today, bounded only by
`ResilienceConfig.ConnectTimeout` (30s default, added by issue #1934/TP-1934).
Against the Kuadrant-fronted MCP Gateway reported in issue #2262,
`server/discover` never responds (hangs, does not error) and so consumes the
entire `ConnectTimeout` budget itself — by the time `Client.Connect` would
otherwise fall through to `initialize`, `ctx` is already expired, so every
attempt fails identically, even though the backend `kube-mcp-server`
(confirmed on the same `go-sdk@v1.7.0`) and the legacy handshake path both
work correctly in isolation.

### 1.2 Objectives

1. **Bounded discover probe**: the `server/discover` HTTP request is bounded
   by a new `ResilienceConfig.DiscoverProbeTimeout` (default 5s), independent
   of `ConnectTimeout`, so a hanging probe fails fast and leaves the rest of
   `ConnectTimeout`'s budget for the legacy `initialize` fallback.
2. **Behavioral proof, not implementation inspection**: tests exercise the
   real `New`/`NewResilient` production entry points against a fake MCP
   server whose `server/discover` handler hangs but whose `initialize`
   handler answers immediately — proving the fallback actually completes
   within a bounded wall-clock margin.
3. **Observability**: a timed-out discover probe is logged at `Info` level
   with `method`/`endpoint`/`discoverProbeTimeout`/`elapsed` fields (SOC2
   CC7.2 diagnostic signal), making a future recurrence immediately visible
   without a fresh investigation.
4. **No regression**: existing connect/backoff/reconnect behavior
   (`UT-FLEET-RES-001..009`) is unaffected when `server/discover` answers
   quickly (the existing fake server's default behavior), and when it is
   rejected outright (`-32601 method not found`, the pre-existing fixture
   default).

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/fleet/mcpclient/...` |
| Regression pass rate (pre-existing) | 100% | `UT-FLEET-RES-001..009` unchanged |
| Build/Lint | Clean | `go build ./...`, `golangci-lint run` |

---

## 2. References

### 2.1 Authority

- Issue #2262: this bug report.
- Issue #1934 / TP-1934 (`docs/tests/1934/TEST_PLAN.md`): prior fix adding
  `ResilienceConfig.ConnectTimeout`, whose budget this issue shows can be
  entirely consumed by one sub-step of a single connect attempt.
- `go-sdk@v1.7.0` `mcp/client.go` `Client.Connect`: SEP-2575 discover-first,
  fallback-on-any-error behavior this fix relies on and bounds.
- BR-INTEGRATION-065 (governing `pkg/fleet/mcpclient`, per TP-1934 and
  `docs/testing/BR-INTEGRATION-054/TEST_PLAN.md`).

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| CP-10 (Information System Recovery and Reconstitution) | The system must be able to retry and recover from failed dependencies within a bounded time | Bounding the discover sub-step is what allows the SDK's own fallback path to actually run within `ConnectTimeout`, instead of that budget being silently consumed by a hung probe | UT-FLEET-RES-010, UT-FLEET-RES-011 |
| CC7.2 (SOC2 — Monitoring for anomalies) | Anomalous/degraded dependency behavior must be observable for investigation | `Info`-level log on discover-probe timeout, with structured fields, gives operators a durable diagnostic signal the moment this recurs | UT-FLEET-RES-010 |

---

## 3. Scope

### 3.1 In Scope

| Area | Description |
|------|--------------|
| `ResilienceConfig` | Add `DiscoverProbeTimeout time.Duration` field, default 5s in `DefaultResilienceConfig()` |
| `discoverProbeRoundTripper` (new) | `http.RoundTripper` wrapper in `pkg/fleet/mcpclient` that peeks the outgoing JSON-RPC `method`; applies `DiscoverProbeTimeout` only to `server/discover` requests |
| `Option` (`options.go`) | New `WithDiscoverProbeTimeout(time.Duration)` and `WithDiscoverProbeLogger(logr.Logger)` options |
| `New()` (`client.go`) | Wraps `cfg.httpClient.Transport` with `discoverProbeRoundTripper` as the outermost layer when `discoverProbeTimeout > 0` |
| `connectWithBackoff` / `doReconnect` (`resilience.go`) | Automatically append the two new options (using `rc.config.DiscoverProbeTimeout` and `rc.logger`) to every `New()` call, so all 8 `cmd/*/main.go` call sites get the fix with zero call-site changes |

### 3.2 Out of Scope

- Any change to, or investigation of, the Kuadrant `mcp-gateway` or any
  other pluggable gateway implementation — flagged in a GitHub issue
  comment instead (75-80% confidence hypothesis, below the 90% bar for
  further investigation).
- Downgrading `go-sdk`.
- Exposing `DiscoverProbeTimeout` (or the rest of `ResilienceConfig`) via
  the Helm chart / Go service config layer — see Phase 2 below.
- Any change to the backoff schedule (`InitialInterval`/`MaxInterval`/`MaxElapsedTime`)
  itself.

---

## 4. Test Scenarios (Unit — behavioral, via production entry points)

All three scenarios reuse a new `newFakeMCPServerWithDiscover(onInitialize,
onDiscover)` fixture (backward-compatible superset of the existing
`newFakeMCPServer`, which becomes a thin wrapper passing `onDiscover=nil`)
and call the real `New`/`NewResilient` production entry points.

| ID | FedRAMP | Business Behavior Verified | Acceptance Criteria | Status |
|----|---------|------------------------------|----------------------|--------|
| UT-FLEET-RES-010 | CP-10, CC7.2 | `New()` + `WithDiscoverProbeTimeout` falls back to a successful `initialize` handshake when `server/discover` hangs past the sub-timeout; the timeout is logged | Call returns success within a bounded wall-clock margin (not the full test timeout); captured log output contains `server/discover` and the configured timeout | Pending |
| UT-FLEET-RES-011 | CP-10 | `NewResilient` (production entry point, via `connectWithBackoff`) succeeds against a fake gateway that hangs forever on `server/discover` but answers `initialize` immediately, using `DefaultResilienceConfig()`'s wiring (no direct `WithDiscoverProbeTimeout` call by the test) | `NewResilient` returns no error and `Ready()` is true, within a bounded wall-clock margin | Pending |
| UT-FLEET-RES-012 | CP-10 | Regression — when `server/discover` is rejected quickly (existing fixture default, `-32601`), `DiscoverProbeTimeout` has no observable effect on `NewResilient`'s existing behavior | Behavior identical to pre-fix; `UT-FLEET-RES-001..009` still pass unmodified | Pending |

**Test file**: `pkg/fleet/mcpclient/resilience_test.go`

---

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|-----------|------------------------|-----------------------|---------|
| `discoverProbeRoundTripper` | `New()` | `pkg/fleet/mcpclient/client.go` | UT-FLEET-RES-010 |
| `ResilienceConfig.DiscoverProbeTimeout` | `connectWithBackoff` / `doReconnect` | `pkg/fleet/mcpclient/resilience.go` | UT-FLEET-RES-011 / UT-FLEET-RES-012 |
| `WithDiscoverProbeTimeout` / `WithDiscoverProbeLogger` options | `New()` (folded into `clientConfig`) | `pkg/fleet/mcpclient/options.go` | UT-FLEET-RES-010 / UT-FLEET-RES-011 |

No new `cmd/` integration needed for Phase 1: all 8 existing `cmd/*/main.go`
call sites are unchanged; this alters `connectWithBackoff`/`doReconnect`'s
already-production-wired call to `New()`.

---

## 6. TDD Execution Phases

| Phase | Type | Scope | Tests |
|-------|------|-------|-------|
| 1 | RED | Extend fixture with `onDiscover` hook; add `UT-FLEET-RES-010/011/012` against unfixed code; confirm 010/011 fail (hang past a short deadline) | UT-FLEET-RES-010, UT-FLEET-RES-011, UT-FLEET-RES-012 |
| 2 | GREEN | Add `DiscoverProbeTimeout` to `ResilienceConfig`; add `discoverProbeRoundTripper`; add the two new `Option`s; wrap the transport in `New()`; append the options in `connectWithBackoff`/`doReconnect` | All three new tests pass; no regression in `UT-FLEET-RES-001..009` |
| 3 | REFACTOR | Dedupe body-peek logic if needed vs. the fixture's own peek; verify no goroutine/context leak; confirm `logr` key-value conventions; lint/build/race for `pkg/fleet/mcpclient` | All tests remain green |

---

## 7. Execution

```bash
go build ./...
go test ./pkg/fleet/mcpclient/... -run "UT-FLEET-RES-01[012]" -v -count=1
go test ./pkg/fleet/mcpclient/... -v -count=1
golangci-lint run --timeout=5m ./pkg/fleet/mcpclient/...
```

---

## 8. Phase 2 (referenced only — separate implementation section, same PR)

Phase 2 exposes the full `pkg/fleet/mcpclient.ResilienceConfig` (all 6
duration fields, including this issue's new `DiscoverProbeTimeout`) via the
Helm chart and each of the 8 services' Go config layer. It introduces a new
`fleet.FleetResilienceConfig` DTO, a `mcpclient.ResilienceConfigFromFleet`
conversion helper, and per-service chart/schema additions. It carries its
own BR (next available `BR-INTEGRATION-XXX`, assigned at implementation
time) and its own test scenarios (`UT-FLEET-CFG-00x`, IT wiring-manifest
extensions, and `helm-unittest` coverage) — tracked in the implementation
plan, not duplicated here, since it changes no business behavior of the
Phase 1 fix above, only its configurability.
