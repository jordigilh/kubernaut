# Test Plan — Cycle C: Handler / Config / API Hardening

| Field | Value |
|-------|-------|
| **Identifier** | `TP-CYCLE-C-001` |
| **Version** | 1.0 |
| **Status** | Draft |
| **Created** | 2026-05-13 |
| **Standard** | IEEE 829-2008 |

## 1. References

| Ref | Document |
|-----|----------|
| R-01 | GA Readiness Audit — findings HANDLER-01/02, CONFIG-01/03, API-01/03, WIRE-13/14/16/17 |
| R-02 | `internal/handler/router.go` — HTTP router and middleware chain |
| R-03 | `internal/handler/agentcard.go` — Agent Card handler |
| R-04 | `internal/launcher/launcher.go` — A2A protocol handler |
| R-05 | `internal/config/config.go` — configuration loading and defaults |
| R-06 | `internal/ratelimit/ratelimit.go` — IP and user rate limiters |
| R-07 | `internal/auth/replay_cache.go` — replay cache with Stop() |
| R-08 | `internal/severity/llm.go` — severity triage with audit events |
| R-09 | `internal/audit/audit.go` — audit event emitter |
| R-10 | `cmd/apifrontend/main.go` — wiring and shutdown sequence |
| R-11 | Cycle A Test Plan `TP-CYCLE-A-001` — cross-phase dependency |
| R-12 | Cycle B Test Plan `TP-CYCLE-B-001` — cross-phase dependency |

## 2. Introduction

This test plan covers **handler-level defense**, **configuration correctness**, and
**API contract compliance**. The findings fall into three groups:

1. **Handler defense** — HTTP panic recovery and write-deadline scoping
2. **Configuration** — startup guards and hot-reload consistency
3. **API/wiring** — A2A handler, AgentCard RBAC, session lifecycle, limiter cleanup, audit

**Objective:** Prove that the HTTP layer is resilient to panics, configuration changes are
applied consistently, and all API contracts (A2A, MCP, Agent Card) are correctly wired.

## 3. Test Items

| Item | Version | Source |
|------|---------|--------|
| `internal/handler/router.go` | HEAD | R-02 |
| `internal/handler/agentcard.go` | HEAD | R-03 |
| `internal/launcher/launcher.go` | HEAD | R-04 |
| `internal/config/config.go` | HEAD | R-05 |
| `internal/ratelimit/ratelimit.go` | HEAD | R-06 |
| `internal/severity/llm.go` | HEAD | R-08 |
| `cmd/apifrontend/main.go` | HEAD | R-10 |

## 4. Software Risk Issues

| Risk | Impact | Mitigation |
|------|--------|------------|
| Panic recovery swallows stack traces | Medium | Log full stack with `debug.Stack()` before responding |
| Write deadline conditional could miss edge cases | Medium | Test with `Accept: */*`, no Accept header, chunked |
| A2A handler wiring requires KA client | Medium | Guard nil KA with 503 response |
| Config watcher race with request serving | High | Use atomic swap; test under `-race` |

## 5. Features to be Tested

### 5.1 HANDLER-01 — Global HTTP Panic Recovery

**Current behavior:** No `recover()` middleware wraps the router mux. A panic in any
handler crashes the process.

**Required behavior:** `recoverMiddleware` wraps the entire mux. On panic:
1. Log stack trace with request context (method, path, request ID)
2. Return HTTP 500 with RFC 7807 `application/problem+json` body
3. Increment `af_http_panics_total` counter (new metric)
4. Do NOT re-panic (process stays alive)

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-01a | Handler that panics with string | HTTP request | 500, problem+json body, log with stack | Unit |
| TC-C-01b | Handler that panics with error | HTTP request | 500, problem+json body | Unit |
| TC-C-01c | Handler that panics with nil | HTTP request | 500, problem+json body (nil panic) | Unit |
| TC-C-01d | Handler that panics with runtime error (index OOB) | HTTP request | 500, recovered, process alive | Unit |
| TC-C-01e | Normal handler (no panic) | HTTP request | Normal response, no recovery log | Unit |
| TC-C-01f | Panic during response write (headers already sent) | HTTP request | Log error, no double-write panic | Unit |
| TC-C-01g | Concurrent panics from 10 goroutines | 10 parallel requests | All return 500, no process crash, counter == 10 | Unit |

### 5.2 HANDLER-02 — Write Deadline Conditional Clearing

**Current behavior:** `trackSSEConnection` unconditionally clears the write deadline via
`rc.SetWriteDeadline(time.Time{})`. This removes HTTP write timeouts for ALL requests
routed through that path, including non-SSE requests.

**Required behavior:** Write deadline cleared only when response is actually SSE
(`Content-Type: text/event-stream` or handler detects streaming context).

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-02a | SSE request (`Accept: text/event-stream`) | Request through tracked path | Write deadline cleared | Unit |
| TC-C-02b | Non-SSE request (`Accept: application/json`) | Same path | Write deadline NOT cleared | Unit |
| TC-C-02c | Request with no Accept header | Same path | Write deadline NOT cleared (default to non-SSE) | Unit |
| TC-C-02d | Request with `Accept: */*` | Same path | Document behavior: treat as SSE or not | Unit |

### 5.3 CONFIG-01 — Startup Guard: Empty Issuer + TLS Required

**Current behavior:** If `cfg.Auth.IssuerURL` is empty and `cfg.Server.TLS.Required` is
true, the service starts successfully but auth middleware becomes pass-through — all
requests are accepted without authentication.

**Required behavior:** `run()` returns error at startup when issuer is empty and TLS is
required. The service must not start in an insecure configuration.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-03a | `issuerURL: ""`, `tls.required: true` | Start service | Startup error, exit non-zero | Unit |
| TC-C-03b | `issuerURL: ""`, `tls.required: false` | Start service | Startup succeeds (dev mode) | Unit |
| TC-C-03c | `issuerURL: "https://dex.example.com"`, `tls.required: true` | Start service | Startup succeeds | Unit |
| TC-C-03d | `issuerURL: "https://dex.example.com"`, `tls.required: false` | Start service | Startup succeeds | Unit |
| TC-C-03e | Startup error | Error message | Contains "issuerURL" and "tls.required" for SRE diagnosis | Unit |

### 5.4 CONFIG-03 — Config Watcher Default Merge

**Current behavior:** On config file change, the watcher unmarshals the new file into a
fresh empty struct. Startup uses `config.Load()` which merges onto `DefaultConfig()`.
This inconsistency means a config field removed from the file on reload gets zero-value
instead of the default.

**Required behavior:** Watcher re-parse clones `config.DefaultConfig()` and unmarshals
the file onto the clone, matching startup behavior.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-04a | Config file has `shutdown.drainSeconds: 10` | Initial load | `cfg.Shutdown.DrainSeconds == 10` | Unit |
| TC-C-04b | Config file reload removes `shutdown.drainSeconds` | Reload callback | `cfg.Shutdown.DrainSeconds` == default (not 0) | Unit |
| TC-C-04c | Config file reload changes `rateLimit.requestsPerSecond` | Reload callback | New value applied; other fields retain defaults | Unit |
| TC-C-04d | Config file reload with invalid YAML | Reload callback | Error logged; previous config retained | Unit |
| TC-C-04e | Config file reload concurrent with request serving | 10 requests during reload | No race condition under `-race` | Unit |

### 5.5 WIRE-14 / API-03 — AgentCard RBAC and Group Mapping

**Current behavior:** `NewAgentCardHandler` is called without RBAC roles or group mapping.
The handler serves static JSON for all users regardless of their role.

**Required behavior:** Agent Card response is filtered by the authenticated user's role,
showing only the tools and capabilities the user's persona is authorized to use.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-05a | RBAC roles: `sre` has 14 tools, `viewer` has 4 | User with `sre` group | Agent card includes 14 tools | Unit |
| TC-C-05b | Same setup | User with `viewer` group | Agent card includes 4 tools | Unit |
| TC-C-05c | Same setup | User with unknown group | Agent card includes no tools (or default set) | Unit |
| TC-C-05d | No RBAC roles configured (nil) | Any user | Agent card includes all tools (fail-open for backwards compat) | Unit |
| TC-C-05e | Group mapping configured | User in mapped group | Mapped role's tools shown | Unit |

### 5.6 API-01 — A2A Handler Wiring

**Current behavior:** A2A endpoint returns HTTP 501 "Not Implemented" stub.

**Required behavior:** `launcher.NewA2AHandler` is wired with KA client. A2A requests
are processed and forwarded to KA.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-06a | A2A handler wired with mock KA | Valid A2A request | Non-501 response; request forwarded to KA | Unit |
| TC-C-06b | A2A handler with nil KA client | Valid A2A request | HTTP 503 "Service Unavailable" (not 501) | Unit |
| TC-C-06c | A2A handler wired | Malformed A2A request body | HTTP 400 with problem+json | Unit |
| TC-C-06d | A2A handler wired | Request body exceeds max size | HTTP 413 or 400 | Unit |

### 5.7 WIRE-13 — Session Acquire/Release Lifecycle

**Current behavior:** `AcquireSession` and `ReleaseSession` in `ratelimit.UserLimiter`
are never called from the MCP session lifecycle. `af_sessions_active` gauge is always 0.

**Required behavior:** MCP session init calls `AcquireSession`, session close calls
`ReleaseSession`. Gauge tracks active count.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-07a | UserLimiter configured: max 5 sessions | 5 sessions acquired | All succeed; gauge == 5 | Unit |
| TC-C-07b | Same | 6th session | Rate-limited; error returned | Unit |
| TC-C-07c | 5 sessions, then 3 released | Check gauge | Gauge == 2 | Unit |
| TC-C-07d | 50 acquire/release cycles | After all cycles | Gauge == 0 (resource bounds) | Unit |

### 5.8 WIRE-16 — Limiter and Cache Stop() on Shutdown

**Current behavior:** `IPLimiter.Stop()`, `UserLimiter.Stop()`, and `ReplayCache.Stop()`
are never called during shutdown. Background goroutines (cleanup tickers) leak.

**Required behavior:** All three `Stop()` methods called during shutdown sequence.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-08a | IPLimiter, UserLimiter, ReplayCache constructed | Shutdown signal | All three `Stop()` called before exit | Unit |
| TC-C-08b | IPLimiter is nil (not configured) | Shutdown signal | No panic; other Stop() still called | Unit |
| TC-C-08c | Stop() called after Stop() (double-stop) | Second Stop() call | No panic, idempotent | Unit |

### 5.9 WIRE-17 — Severity Triage Audit Events

**Current behavior:** Severity triage results are not emitted as audit events. There is
no audit trail for automated severity decisions.

**Required behavior:** Each triage decision emits an audit event with: signal ID, input
severity, output severity, confidence, model used, timestamp.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-C-09a | Triager + mock auditor | Triage succeeds | Audit event emitted with severity result | Unit |
| TC-C-09b | Triager returns error | Triage call | Audit event emitted with error status | Unit |
| TC-C-09c | Noop triager (production default) | Triage call | No audit event (noop doesn't produce results) | Unit |
| TC-C-09d | Audit event fields | Inspect emitted event | Contains: signal_id, input_severity, output_severity, model, timestamp | Unit |

## 6. Features Not to be Tested

| Feature | Rationale |
|---------|-----------|
| Full A2A protocol compliance | A2A spec is evolving; test basic wiring only |
| Hot-reload of all config fields | Only `drainSeconds` and `rateLimit` tested; full hot-reload is v1.6 |
| MCP SDK session internals | SDK manages sessions; we test the lifecycle hooks |

## 7. Approach

### 7.1 Panic Recovery Testing

- Register a handler that calls `panic(value)` for each value type
- Use `httptest.NewRecorder` to capture response
- Assert response code, content-type, body structure
- Assert metric counter via `testutil.ToFloat64()`

### 7.2 Config Watcher Testing

- Use `os.CreateTemp` for config file
- Write initial config, call `Load()`
- Modify file, trigger watcher callback
- Assert resulting config fields match expected (defaults for removed fields)
- Run under `-race` with concurrent HTTP requests

### 7.3 Agent Card RBAC Testing

- Construct `AgentCardHandler` with mock RBAC roles and group mapping
- Create `UserIdentity` with specific groups
- Assert filtered capabilities in response JSON

## 8. Pass/Fail Criteria

### Plan-level

The plan **passes** when:
1. All TC-* pass with `-race`
2. `make test-unit` exits 0
3. `make lint` exits 0
4. No goroutine leaks detected (check with `goleak` if available)
5. 9-category checkpoint audit (Checkpoint C) satisfied

## 9. Suspension Criteria

| Condition | Action |
|-----------|--------|
| A2A spec changes | Reassess TC-C-06* scope |
| Config watcher race detected | Fix before advancing |
| Panic recovery test causes actual process crash | Debug in isolation |

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/test/cycle-c-handler-config-api/TEST_PLAN.md` |
| Panic recovery tests | `internal/handler/router_test.go` (extend) |
| Write deadline tests | `internal/handler/router_test.go` (extend) |
| Config guard tests | `cmd/apifrontend/main_test.go` (extend) |
| Config watcher tests | `internal/config/config_test.go` (extend) |
| AgentCard RBAC tests | `internal/handler/agentcard_test.go` (extend) |
| A2A handler tests | `internal/launcher/launcher_test.go` (extend) |
| Session lifecycle tests | `internal/ratelimit/ratelimit_test.go` (extend) |
| Limiter stop tests | `cmd/apifrontend/main_wiring_test.go` (extend) |
| Audit event tests | `internal/severity/llm_triager_test.go` (extend) |

## 11. Cross-Phase Integration (Cycles A+B → C)

| Integration point | Verification |
|-------------------|-------------|
| Panic recovery metric `af_http_panics_total` → Cycle A metrics scrape | Checkpoint C: trigger panic, scrape metrics, assert counter |
| Session gauge (Cycle A WIRE-09) → session lifecycle (Cycle C WIRE-13) | Checkpoint C: acquire/release session, assert gauge from Cycle A registry |
| Auth guard (CONFIG-01) → issuer validation (Cycle B SEC-02) | Checkpoint C: empty issuer with scheme validation catches both |
| Replay cache Stop() (WIRE-16) → replay sentinel (Cycle B SEC-05) | Checkpoint C: construct with replay, shutdown, verify clean stop |
