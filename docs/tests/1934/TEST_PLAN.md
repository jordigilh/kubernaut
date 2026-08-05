# Test Plan: Fleet MCP Client `connectWithBackoff`/`doReconnect` Block Forever on a Hung Handshake

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1934
**Feature**: Bound each MCP connect attempt inside `pkg/fleet/mcpclient`'s
backoff/reconnect loop with a per-attempt timeout, so a single hung handshake
cannot block startup or reconnection forever.
**Version**: 1.0
**Created**: 2026-08-04
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/1919-1934-console-gate-connect-timeout`

---

## 1. Introduction

### 1.1 Purpose

`connectWithBackoff` (`pkg/fleet/mcpclient/resilience.go`, inside
`wait.ExponentialBackoff`'s closure) and `doReconnect`
(`pkg/fleet/mcpclient/resilience.go`) both call `New(ctx, rc.endpoint,
rc.opts...)` using the caller's raw `ctx`. `New()`
(`pkg/fleet/mcpclient/client.go`) performs a single blocking
`mcpClient.Connect(ctx, transport, nil)` with no internal bound of its own.

Every production caller of `NewResilient`/`Reconnect` (8 `cmd/*/main.go`
sites) passes a signal-cancel-only context (`context.Background()` wrapped in
`signal.NotifyContext`/`ctrl.SetupSignalHandler`) — never a context with a
deadline. Issue #1933 reproduced a hung handshake against a non-conformant
fake server: a single such hang blocks the entire backoff loop, and thus
service startup (or a lazy reconnect), forever — silently defeating
`MaxElapsedTime`/`MaxInterval`, which exist specifically to bound total
retry duration.

### 1.2 Objectives

1. **Bounded connect attempts**: each individual `New()` call inside
   `connectWithBackoff` and `doReconnect` is bounded by a new
   `ResilienceConfig.ConnectTimeout`, independent of whether the caller's
   context carries a deadline.
2. **Behavioral proof, not implementation inspection**: tests exercise the
   real `NewResilient`/`Reconnect` entry points against a fake MCP server
   that never responds to `initialize`, with `context.Background()` (no
   caller deadline) — proving the bound is enforced internally, not merely
   inherited from the caller.
3. **No regression**: existing backoff/retry/reconnect behavior
   (`UT-FLEET-RES-002/003/007`) is unaffected when connections succeed
   within `ConnectTimeout`.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit` (`pkg/fleet/mcpclient/...`) |
| Regression pass rate (pre-existing) | 100% | `UT-FLEET-RES-001..005,007` unchanged |
| Build/Lint | Clean | `go build ./...`, `golangci-lint run` |

---

## 2. References

### 2.1 Authority

- Issue #1934: this bug report.
- Issue #1933: original hang reproduction that motivated this fix.
- `pkg/fleet/readiness/probers.go` (`MCPClientProber.Probe`): pre-existing
  `context.WithTimeout(ctx, timeout)` idiom this fix mirrors, one layer up.
- `pkg/fleet/fmc/http_client.go`: second pre-existing instance of the same
  idiom.
- BR-INTEGRATION-065 (governing `pkg/fleet/mcpclient`, per
  `docs/testing/BR-INTEGRATION-054/TEST_PLAN.md`).

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| CP-10 (Information System Recovery and Reconstitution) | The system must be able to retry and recover from failed dependencies within a bounded time | Bounding each connect attempt is what allows the backoff loop to actually recover/retry rather than being stuck indefinitely on one hung attempt | UT-FLEET-RES-008, UT-FLEET-RES-009 |

---

## 3. Scope

### 3.1 In Scope

| Area | Description |
|------|--------------|
| `ResilienceConfig` | Add `ConnectTimeout time.Duration` field, default 30s in `DefaultResilienceConfig()` |
| `connectWithBackoff` | Wrap the per-attempt `New(ctx, ...)` call with `context.WithTimeout(ctx, rc.config.ConnectTimeout)` |
| `doReconnect` | Same treatment around its single `New(ctx, ...)` call |

### 3.2 Out of Scope

- `ResilienceConfig.TokenRefreshTimeout` being dead/unwired (pre-existing,
  unrelated gap discovered during preflight) — not touched here.
- Any change to `New()`/`Client` in `client.go` itself — the fix bounds the
  *caller's* context passed into `New()`, not `New()`'s internals.
- Any change to the backoff schedule (`InitialInterval`/`MaxInterval`/`MaxElapsedTime`)
  itself.

---

## 4. Test Scenarios (Unit — behavioral, via production entry points)

Both scenarios reuse the existing `newFakeMCPServer(onInitialize func(...))`
hook (already used by `UT-FLEET-RES-003/007`) with an `onInitialize` that
never writes a response — simulating the exact #1933-class hang — and call
the real `NewResilient`/`Reconnect` production entry points with
`context.Background()` (no caller-supplied deadline), proving the bound is
enforced internally rather than inherited.

| ID | FedRAMP | Business Behavior Verified | Acceptance Criteria | Status |
|----|---------|------------------------------|----------------------|--------|
| UT-FLEET-RES-008 | CP-10 | `NewResilient` against a hanging fake server, with a short test `ConnectTimeout`/`MaxElapsedTime` and a caller context with no deadline, returns instead of hanging | Call returns (with an error) within a bounded wall-clock margin (not the full test timeout); error indicates a connect failure, not a test hang | Pending |
| UT-FLEET-RES-009 | CP-10 | `Reconnect` against a hanging fake server, with a caller context with no deadline, returns instead of hanging | `doReconnect`'s own bound fires even though the caller supplies no deadline (today it only avoids hanging by accident, because the sole caller `probers.go` happens to wrap its own timeout) | Pending |

**Test file**: `pkg/fleet/mcpclient/resilience_test.go`

---

## 5. Wiring Manifest

Not applicable — no new component/handler/entry point. `ResilienceConfig`,
`connectWithBackoff`, and `doReconnect` are already-wired, already
production-exercised code (8 `cmd/*/main.go` call sites via
`NewResilient`/`Reconnect`); only their internal per-attempt timeout
behavior changes.

---

## 6. TDD Execution Phases

| Phase | Type | Scope | Tests |
|-------|------|-------|-------|
| 1 | RED | Add `UT-FLEET-RES-008/009` against unfixed code; confirm they fail (hang/timeout at the test level) | UT-FLEET-RES-008, UT-FLEET-RES-009 |
| 2 | GREEN | Add `ConnectTimeout` to `ResilienceConfig`; wrap the `New()` calls in `connectWithBackoff` and `doReconnect` with `context.WithTimeout` | Both new tests pass; no regression in `UT-FLEET-RES-001..005,007` |
| 3 | REFACTOR | Lint/build/race for `pkg/fleet/mcpclient` | All tests remain green |

---

## 7. Execution

```bash
go build ./...
go test ./pkg/fleet/mcpclient/... -run "UT-FLEET-RES-00(8|9)" -v -count=1
make test-unit
golangci-lint run --timeout=5m ./pkg/fleet/mcpclient/...
```
