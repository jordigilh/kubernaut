# Test Plan — #1351: Interactive Pipeline GA Readiness Bug Fixes

**IEEE 829 Compliant** | **Issue**: [#1351](https://github.com/jordigilh/kubernaut/issues/1351)

## 1. Test Plan Identifier

TP-1351-INTERACTIVE-PIPELINE-GA

## 2. Introduction

This test plan covers the 49 validated bugs across KA, AA, and AF services discovered during the interactive workflow pipeline GA readiness audit. The fixes restore correctness to the interactive session lifecycle, result propagation, timeout enforcement, error recovery, and access control.

### 2.1 Authoritative References for Grounding

| Document | Path | Relevance |
|----------|------|-----------|
| BR-INTERACTIVE-001..008 | `docs/requirements/BR-INTERACTIVE.md` | Session lifecycle, takeover, audit attribution, RBAC |
| BR-INTERACTIVE-010 | `docs/requirements/BR-INTERACTIVE-010.md` | IS CRD as interactive signal, AA poll handling, timeout edge cases |
| DD-INTERACTIVE-002 | `docs/architecture/decisions/DD-INTERACTIVE-002-dynamic-takeover-model.md` | Dynamic takeover model, cancel+reconstruct, timeout policy |
| BR-HAPI-197 | `docs/requirements/BR-HAPI-197-needs-human-review-field.md` | Confidence threshold enforcement, human review triggers |
| BR-HAPI-198 | `docs/requirements/BR-HAPI-198-configurable-confidence-thresholds.md` | Operator-configurable thresholds, V1.1 Rego integration |
| BR-ORCH-027/028 | `docs/requirements/BR-ORCH-027-028-timeout-management.md` | Global 1h cap, per-phase timeouts, timeout-phase recording |
| 11_SECURITY_ACCESS_CONTROL | `docs/requirements/11_SECURITY_ACCESS_CONTROL.md` | BR-RBAC-001..020, session management, audit |
| DD-AUTH-MCP-001 | `docs/architecture/decisions/DD-AUTH-MCP-001-mcp-endpoint-security.md` | MCP endpoint auth, user impersonation |
| DD-EVENT-001 | `docs/architecture/decisions/DD-EVENT-001-controller-event-registry.md` | K8s event emission policy |

### 2.2 FedRAMP Control Objectives

| Control | Title | Bugs Addressed | Test Coverage Strategy |
|---------|-------|----------------|------------------------|
| AC-2 | Account Management | AF-CRIT-1, AF-CRIT-2 | UT: auth config rejects pass-through; IT: session cap enforced |
| AC-6 | Least Privilege | AA-MED-4, AF-MED-8 | UT: Rego denies when identity absent; UT: rate limit applied |
| AU-2 | Audit Events | KA-MED-7, AA-MED-1, AA-MED-5 | UT: Error-level log on result loss; UT: Reason/SubReason set |
| AU-12 | Audit Generation | KA-CRIT-1/2 | IT: cancel/timeout emits completion audit |
| IA-2 | Identification & Authentication | AF-CRIT-1, AF-HIGH-3/4 | UT: startup fails without auth; UT: errors redacted |
| SC-8 | Transmission Confidentiality | AF-HIGH-3, AF-HIGH-4 | UT: RedactError applied to all client-facing errors |
| SC-13 | Cryptographic Protection | KA-MED-1 | UT: ExecutionBundleDigest propagated in interactive path |
| SI-2 | Flaw Remediation | All 49 bugs | Full TDD coverage per this plan |
| SI-10 | Information Input Validation | AF-MED-2 | UT: invalid rr_id rejected before reaching KA |
| CP-10 | System Recovery | AA-CRIT-2, AA-HIGH-1/2 | UT: transient errors don't permanently fail; IT: regeneration on 404 |

### 2.3 Referenced Acceptance Criteria from BRs

| BR | AC | Violated By |
|----|-----|------------|
| BR-INTERACTIVE-010 SC-7.3 | "On cancelled with IS deleted: PhaseFailed with ReasonInteractiveCancelled" | AA-HIGH-2 (IS check error → terminal without IS check) |
| BR-INTERACTIVE-010 SC-7.2 | "On cancelled with IS still Active: re-submit with interactive=true" | AA-HIGH-2 (API error blocks re-submit) |
| BR-INTERACTIVE-010 Edge Case | "Pending session abandoned: AA 25m cap fails investigation" | AA-CRIT-1 (cap missing for user_driving) |
| BR-HAPI-197 AC-4 | "Confidence < 0.7 → needs_human_review=true" | KA-HIGH-1 (confidence 0.00 propagated) |
| BR-ORCH-027 AC-1 | "All remediations MUST reach terminal state" | AA-CRIT-1 (unbounded user_driving) |
| BR-ORCH-028 AC-3 | "Per-phase timeout with phase recording" | AA-HIGH-3 (AA/RO timeout conflict) |
| DD-INTERACTIVE-002 §Timeout | "Global timeout 1h as hard cap. Dynamic extension bounded." | AA-MED-9 (maxInvestigationDuration not configurable) |
| BR-RBAC-001 | "MUST authenticate users before granting access" | AF-CRIT-1 (pass-through on misconfig) |
| BR-RBAC-005 | "MUST provide secure session management with configurable timeouts" | AF-CRIT-2 (MaxConcurrentSessions dead code) |
| BR-INTERACTIVE-004 §5 | "Only the active driver may terminate the session" (SEC-CRIT-01) | KA-HIGH-2 (unmutexed complete/cancel races) |

## 3. Test Items

### 3.1 KA — Kubernaut Agent

| Item | File | Bugs |
|------|------|------|
| `handleCancel` | `internal/kubernautagent/mcp/tools/investigate.go` | KA-CRIT-1 |
| TimeoutManager callback | `cmd/kubernautagent/main.go` | KA-CRIT-2 |
| SessionClosedHandler | `cmd/kubernautagent/main.go` | KA-CRIT-2 |
| `buildFinalResult` | `internal/kubernautagent/mcp/tools/select_workflow.go` | KA-HIGH-1, KA-MED-1/2/8 |
| `handleComplete` | `internal/kubernautagent/mcp/tools/investigate.go` | KA-HIGH-2, KA-MED-6 |
| `SelectWorkflowTool.Handle` goroutine | `internal/kubernautagent/mcp/tools/select_workflow.go` | KA-HIGH-3, KA-MED-7 |
| `SetResult` | `internal/kubernautagent/session/store.go` | KA-HIGH-4 |
| `GetLatestRCAResultByRemediationID` | `internal/kubernautagent/session/manager.go` | KA-HIGH-5 |
| `CompleteNoActionTool.Handle` | `internal/kubernautagent/mcp/tools/complete_no_action.go` | KA-MED-4/5 |

### 3.2 AA — AIAnalysis Controller

| Item | File | Bugs |
|------|------|------|
| `handleSessionPollUserDriving` | `pkg/aianalysis/handlers/investigating.go` | AA-CRIT-1, AA-MED-5 |
| `handleError` / poll success paths | `pkg/aianalysis/handlers/investigating.go` | AA-CRIT-2 |
| `handleSessionGetResultError` | `pkg/aianalysis/handlers/investigating.go` | AA-HIGH-1, AA-MED-3 |
| `handleSessionPollCancelled` | `pkg/aianalysis/handlers/investigating.go` | AA-HIGH-2 |
| `handleSessionPollPending` timeout | `pkg/aianalysis/handlers/investigating.go` | AA-HIGH-3, AA-MED-2/9 |
| `handleInvestigating` (idempotency) | `internal/controller/aianalysis/phase_handlers.go` | AA-HIGH-4 |
| `handleSessionPollFailed` | `pkg/aianalysis/handlers/investigating.go` | AA-MED-1 |
| `buildPolicyInput` | `pkg/aianalysis/handlers/analyzing.go` | AA-MED-4 |
| `handleDeletion` | `internal/controller/aianalysis/deletion_handler.go` | AA-MED-8 |

### 3.3 AF — API Frontend

| Item | File | Bugs |
|------|------|------|
| `buildAuthMiddleware` | `cmd/apifrontend/main.go` | AF-CRIT-1 |
| `UserLimiter.AcquireSession` wiring | `pkg/apifrontend/ratelimit/ratelimit.go` | AF-CRIT-2 |
| `kaMCPHTTPClient` construction | `cmd/apifrontend/main.go` | AF-HIGH-1 |
| `KASessionPool` lifecycle | `pkg/apifrontend/ka/session_pool.go` | AF-HIGH-2 |
| `SDKMCPClient.StartInvestigation` error | `pkg/apifrontend/ka/mcp_sdk_client.go` | AF-HIGH-3 |
| `FormatEventForUser` | `pkg/apifrontend/tools/ka_investigate_mcp.go` | AF-HIGH-4 |
| `SDKMCPClient.StartInvestigation` context | `pkg/apifrontend/ka/mcp_sdk_client.go` | AF-HIGH-5 |
| Router `/metrics` | `pkg/apifrontend/handler/router.go` | AF-MED-1 |
| `HandleDiscoverWorkflows` / `HandleSelectWorkflow` | `pkg/apifrontend/tools/ka_tools.go` | AF-MED-2 |

## 4. Pyramid Invariant

```
UT  proves logic   --> buildFinalResult merges Phase 3 fields; handleCancel bridges HTTP;
                       SetResult guards user_driving; auth rejects pass-through; error redaction
IT  proves wiring  --> main.go timeout callback → httpCompleter wired; AcquireSession called
                       from A2A handler; EvictIdle goroutine started; router denies /metrics
E2E proves journey --> Interactive takeover → select_workflow → AA poll → confidence passes
                       → WorkflowExecution created with correct fields
```

## 5. Features to Be Tested

### 5.1 KA Tier 1: Unit Tests — Prove Logic

#### Session Lifecycle Unification (KA-CRIT-1, KA-CRIT-2, KA-MED-3/4/5)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-KA-1351-001 | BR-INTERACTIVE-004 | AU-12 | `handleCancel` calls `httpCompleter.CompleteUserDriving` | HTTP session transitions to completed after MCP cancel |
| UT-KA-1351-002 | BR-INTERACTIVE-004 | AU-12 | `handleCancel` calls `ForceCompleteByRemediationID` as fallback | HTTP session resolved even without user_driving match |
| UT-KA-1351-003 | BR-INTERACTIVE-005 | CP-10 | TimeoutManager `onExpire` resolves HTTP session | Expired session has HTTP `StatusCompleted` with cancellation result |
| UT-KA-1351-004 | BR-INTERACTIVE-005 | CP-10 | SessionClosedHandler disconnect resolves HTTP session | Disconnected session has HTTP `StatusCompleted` |
| UT-KA-1351-005 | BR-INTERACTIVE-004 | AU-2 | `complete_no_action` calls `StopTracking` | TimeoutManager no longer fires after completion |
| UT-KA-1351-006 | BR-INTERACTIVE-004 | AU-2 | `complete_no_action` clears `sessionMu` and `reconHistory` | No stale state on next session for same rr_id |
| UT-KA-1351-007 | BR-INTERACTIVE-010 SC-7 | CP-10 | HTTP cancel on `user_driving` session also releases MCP lease | No split-brain between HTTP and MCP |

#### Phase 3 Result Propagation (KA-HIGH-1, KA-MED-1/2/8)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-KA-1351-010 | BR-HAPI-197 AC-4 | SI-2 | `buildFinalResult` propagates `discovery.FullResult.Confidence` | `result.Confidence == 0.82` when RCA has `0.0` and Phase 3 has `0.82` |
| UT-KA-1351-011 | BR-HAPI-197 AC-4 | SC-13 | `buildFinalResult` propagates `ExecutionBundleDigest` | Digest matches catalog entry |
| UT-KA-1351-012 | BR-HAPI-197 AC-4 | SI-2 | `buildFinalResult` propagates `AlternativeWorkflows` | Alternatives from Phase 3 present in final result |
| UT-KA-1351-013 | BR-HAPI-197 | SI-2 | `buildFinalResult` propagates `DetectedLabels`, `IsActionable`, `InvestigationOutcome`, `Warnings` | All Phase 3 fields merged into final result |
| UT-KA-1351-014 | BR-HAPI-197 | SI-2 | `buildFinalResult` with nil `discovery.FullResult` gracefully degrades | No panic; falls back to RCA-only fields |

#### Mutex and Race Fixes (KA-HIGH-2/3/4/5, KA-MED-6/7)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-KA-1351-020 | BR-INTERACTIVE-004 §5 | AC-6 | `handleComplete` acquires per-rrID mutex | Concurrent `handleMessage` blocked during completion |
| UT-KA-1351-021 | BR-INTERACTIVE-004 §5 | AC-6 | `handleCancel` acquires per-rrID mutex | Concurrent `handleDiscoverWorkflows` blocked during cancel |
| UT-KA-1351-022 | BR-INTERACTIVE-005 | SI-2 | `select_workflow` completion is synchronous (or mutex-held) | No TOCTOU between response and HTTP/lease cleanup |
| UT-KA-1351-023 | BR-INTERACTIVE-010 | SI-2 | `SetResult` rejects writes when `StatusUserDriving` | Cancelled goroutine cannot overwrite user-driven session result |
| UT-KA-1351-024 | BR-INTERACTIVE-010 | SI-2 | `GetLatestRCAResultByRemediationID` filters cancelled sessions | Returns only non-cancelled, non-stale results |
| UT-KA-1351-025 | BR-HAPI-197 | AU-2 | `handleComplete` builds minimal `InvestigationResult` instead of nil | HTTP session has non-nil result after `action=complete` |
| UT-KA-1351-026 | BR-HAPI-197 | AU-2 | Goroutine result-loss logged at Error level | Both-paths-fail scenario produces `logger.Error` (not `V(1).Info`) |

### 5.2 AA Tier 1: Unit Tests — Prove Logic

#### Timeout and Failure Handling (AA-CRIT-1/2, AA-HIGH-1/2/3/4, AA-MED-1/2/3/9)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AA-1351-001 | BR-ORCH-027 AC-1 | CP-10 | `handleSessionPollUserDriving` enforces `maxInvestigationDuration` | `PhaseFailed` after 25m even in `user_driving` |
| UT-AA-1351-002 | BR-ORCH-027 | CP-10 | `user_driving` timeout sets `Reason`, `SubReason`, and `SetInvestigationComplete` | All terminal fields populated consistently |
| UT-AA-1351-003 | BR-AI-009 | CP-10 | `ConsecutiveFailures` reset to 0 on successful poll | Intermittent errors don't accumulate across successes |
| UT-AA-1351-004 | BR-AA-HAPI-064.5 | CP-10 | `GetSessionResult` 404 triggers session regeneration (like poll 404) | `handleSessionLost` called, not `handleError` |
| UT-AA-1351-005 | BR-INTERACTIVE-010 SC-7.2 | SI-2 | `handleSessionPollCancelled` requeues on IS check error | Transient API error → `RequeueAfter` (not terminal) |
| UT-AA-1351-006 | BR-ORCH-028 | SI-2 | AA communicates timeout cap to RO-compatible boundary | `maxInvestigationDuration` wired from config |
| UT-AA-1351-007 | BR-AI-009 | SI-2 | `InvestigationTime > 0` with `Phase=Investigating` triggers recovery (not skip) | Handler requeues or re-evaluates instead of returning empty |
| UT-AA-1351-008 | BR-HAPI-197 | AU-2 | `handleSessionPollFailed` sets `Reason=InvestigationFailed` and `SubReason` | No stale retry metadata on terminal status |
| UT-AA-1351-009 | BR-ORCH-027 | SI-2 | Timeout path calls `SetInvestigationComplete(false, ...)` | `InvestigationComplete` condition consistent with `PhaseFailed` |
| UT-AA-1351-010 | BR-AA-HAPI-064 | CP-10 | 409 on `GetSessionResult` has bounded retry (cap at N attempts) | Does not re-poll indefinitely; fails after cap |
| UT-AA-1351-011 | DD-EVENT-001 | AU-2 | `user_driving` event emission rate-limited (not every 15s) | Max 1 event per 5-minute window (or similar) |

### 5.3 AF Tier 1: Unit Tests — Prove Logic

#### Auth Hardening (AF-CRIT-1, AF-CRIT-2, AF-MED-8)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1351-001 | BR-RBAC-001 | IA-2, AC-2 | `buildAuthMiddleware` returns deny-all when no JWT AND kubeconfig fails | Requests receive 503 (not pass-through) |
| UT-AF-1351-002 | BR-RBAC-001 | IA-2, AC-2 | `buildAuthMiddleware` returns deny-all when K8s client creation fails | Requests receive 503 |
| UT-AF-1351-003 | BR-RBAC-005 | AC-2 | `AcquireSession` called on investigation start | Session count incremented for user |
| UT-AF-1351-004 | BR-RBAC-005 | AC-2 | `ReleaseSession` called on investigation end/disconnect | Session count decremented |
| UT-AF-1351-005 | BR-RBAC-005 | AC-2 | Investigation rejected when `AcquireSession` returns false | 429 returned to client |

#### Resource Management (AF-HIGH-1/2/5)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1351-010 | BR-RBAC-005 | CP-10 | KA MCP HTTP client has configured Timeout | `http.Client.Timeout > 0` |
| UT-AF-1351-011 | BR-RBAC-005 | CP-10 | `EvictIdle` called periodically | Pool entries older than IdleTTL are removed |
| UT-AF-1351-012 | BR-INTERACTIVE-005 | CP-10 | Investigation session context tied to request lifecycle | Client disconnect cancels MCP session within timeout |

#### Error Redaction (AF-HIGH-3/4)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1351-020 | BR-RBAC-016 | SC-8 | `StartInvestigation` error path applies `security.RedactError` | Raw KA text never reaches client |
| UT-AF-1351-021 | BR-RBAC-016 | SC-8 | `FormatEventForUser` with `EventTypeError` applies `security.RedactError` | Internal error details stripped before SSE emission |

#### Input Validation (AF-MED-2)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1351-030 | BR-RBAC-016 | SI-10 | `HandleDiscoverWorkflows` validates `rr_id` format | Non-DNS-1123 rr_id returns validation error |
| UT-AF-1351-031 | BR-RBAC-016 | SI-10 | `HandleSelectWorkflow` validates `rr_id` format | Non-DNS-1123 rr_id returns validation error |
| UT-AF-1351-032 | BR-RBAC-016 | SI-10 | `HandleInvestigateMCP` validates `rr_id` format | Non-DNS-1123 rr_id returns validation error |

### 5.4 Tier 2: Integration Tests — Prove Wiring

#### KA Wiring (`test/integration/kubernautagent/`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-KA-1351-001 | BR-INTERACTIVE-004 | AU-12 | Cancel via MCP → HTTP session completed | Full stack: MCP tool call → httpCompleter wired in main.go → session store updated |
| IT-KA-1351-002 | BR-INTERACTIVE-005 | CP-10 | Inactivity timeout → HTTP session completed | TimeoutManager fires → httpCompleter called → poll returns `completed` |
| IT-KA-1351-003 | BR-INTERACTIVE-005 | CP-10 | MCP disconnect → HTTP session completed | SessionClosedHandler → httpCompleter → poll returns `completed` |
| IT-KA-1351-004 | BR-HAPI-197 AC-4 | SI-2 | `select_workflow` through full stack returns correct confidence | End-to-end: tool call → buildFinalResult → HTTP result has Phase 3 confidence |

#### AA Wiring (`test/integration/aianalysis/`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-AA-1351-001 | BR-ORCH-027 | CP-10 | `maxInvestigationDuration` wired from config in main.go | Handler constructed with config value (not default) |
| IT-AA-1351-002 | BR-AI-009 | CP-10 | Poll success resets `ConsecutiveFailures` on CR status | After transient error + success, failures = 0 |
| IT-AA-1351-003 | BR-INTERACTIVE-010 SC-7 | SI-2 | KA `cancelled` + IS check error → requeue (not terminal) | CR status remains `Investigating` after transient IS API error |

#### AF Wiring (`test/integration/apifrontend/`)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-AF-1351-001 | BR-RBAC-001 | IA-2 | AF startup without kubeconfig returns 503 (not pass-through) | `GET /a2a/invoke` returns 503 |
| IT-AF-1351-002 | BR-RBAC-005 | AC-2 | Concurrent session limit enforced at A2A layer | N+1th investigation returns 429 |
| IT-AF-1351-003 | BR-RBAC-005 | CP-10 | `EvictIdle` goroutine runs in production wiring | Pool size decreases after IdleTTL elapsed |

### 5.5 Tier 3: E2E Tests — Prove the Journey

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| E2E-1351-001 | BR-HAPI-197, BR-INTERACTIVE-010 | SI-2 | Interactive takeover → select_workflow → AA confidence ≥ 0.7 → WorkflowExecution created | Full pipeline: AF → KA → AA → RO → WE; confidence correctly propagated |
| E2E-1351-002 | BR-ORCH-027, BR-INTERACTIVE-005 | CP-10 | Interactive session exceeds 25m → AA fails with timeout | user_driving session has bounded lifetime |
| E2E-1351-003 | BR-INTERACTIVE-004, BR-INTERACTIVE-010 | AU-12 | MCP cancel during interactive → AA observes completed → no stuck RR | Pipeline reaches terminal state after cancel |

## 6. Features Not to Be Tested

| Feature | Reason |
|---------|--------|
| `maxUserDrivingDuration` (separate from investigation timeout) | Explicitly deferred to v1.6+ per BR-INTERACTIVE-010 "Out of Scope" |
| Multi-replica session affinity | Deferred per BR-INTERACTIVE-010 |
| RR cancellation propagation to KA session | Noted as "pre-existing gap" in BR-INTERACTIVE-010 |
| Rego policy correctness (approval.rego logic) | Operator-managed; test plan covers structural identity propagation only |

## 7. Approach

### 7.1 TDD Methodology

Each bug fix follows RED → GREEN → REFACTOR:

1. **RED**: Write the test(s) from this plan that expose the bug. Verify they fail against current code.
2. **GREEN**: Implement the minimal fix. Verify test passes.
3. **REFACTOR**: Consolidate, extract helpers, remove duplication.

### 7.2 Anti-Patterns to Avoid

| Anti-Pattern | Mitigation |
|--------------|-----------|
| Testing implementation details | Assert observable behavior (HTTP status, CR status, result fields) not internal state |
| Shared mutable test state | Each `It` block creates its own Manager/Store/Handler instances |
| Sleep-based synchronization | Use `Eventually`/`Consistently` with timeouts (Gomega async assertions) |
| Overly-specific mocks | Mock only external boundaries (K8s API, LLM, HTTP transport); use real business logic |
| Table-driven tests without context | Each entry has a descriptive name and documents the BR/AC it validates |
| Testing private functions directly | Test through public API; use `export_test.go` only for pure computational helpers |
| Ignoring error paths | Every error branch in scope has at least one test exercising it |
| Non-deterministic time | Inject `clock` interface or use `fakeclock` for timeout tests |

### 7.3 Coverage Targets (Per-Tier Testable Code)

| Tier | Target | Scope |
|------|--------|-------|
| Unit Tests | ≥80% of unit-testable code | Pure logic: buildFinalResult, SetResult guards, error classification, auth config |
| Integration Tests | ≥80% of integration-testable code | Wiring: main.go callbacks, handler→store paths, config injection |
| E2E Tests | ≥80% of full service code | Full journey: AF→KA→AA→RO pipeline for interactive flows |

## 8. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| httpCompleter in handleCancel | `InvestigateTool.handleCancel` | `cmd/kubernautagent/main.go` (tool construction) | IT-KA-1351-001 |
| httpCompleter in TimeoutManager | TimeoutManager `onExpire` callback | `cmd/kubernautagent/main.go:1272-1285` | IT-KA-1351-002 |
| httpCompleter in SessionClosedHandler | disconnect callback | `cmd/kubernautagent/main.go:1294-1335` | IT-KA-1351-003 |
| AcquireSession in A2A handler | `HandleInvestigateMCP` | `pkg/apifrontend/tools/ka_investigate_mcp.go` | IT-AF-1351-002 |
| EvictIdle goroutine | startup | `cmd/apifrontend/main.go` (pool lifecycle) | IT-AF-1351-003 |
| WithMaxInvestigationDuration(cfg) | handler construction | `cmd/aianalysis/main.go:281-286` | IT-AA-1351-001 |
| ConsecutiveFailures reset | poll success path | `pkg/aianalysis/handlers/investigating.go` | IT-AA-1351-002 |
| IS check error requeue | handleSessionPollCancelled | `pkg/aianalysis/handlers/investigating.go` | IT-AA-1351-003 |
| Auth deny-all on config failure | buildAuthMiddleware | `cmd/apifrontend/main.go:884-893` | IT-AF-1351-001 |

## 9. Pass/Fail Criteria

### 9.1 Individual Test

- **Pass**: Test assertion succeeds; no panics; no data races (`-race` flag)
- **Fail**: Any assertion failure, panic, or detected race condition

### 9.2 Test Plan

- **Pass**: ALL tests in sections 5.1–5.5 pass; coverage ≥80% per tier on changed files; `go build ./...` clean; `golangci-lint run --timeout=5m` clean
- **Fail**: Any test failure; coverage <80% on any tier's testable code; lint errors

## 10. Test Environment

| Tier | Environment | Dependencies |
|------|-------------|-------------|
| UT | Go test runner + httptest | fakeclock, mock interfaces (HTTPSessionCompleter, LeaseSessionManager) |
| IT | envtest (real kube-apiserver, etcd) | controller-runtime envtest, real session stores, real handlers |
| E2E | Kind cluster | DEX (auth), mock-LLM, TLS certs, full service deployment |

## 11. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| KA unit tests | `internal/kubernautagent/mcp/tools/*_test.go`, `internal/kubernautagent/session/*_test.go` |
| AA unit tests | `pkg/aianalysis/handlers/*_test.go`, `pkg/aianalysis/investigating_handler_test.go` |
| AF unit tests | `pkg/apifrontend/ka/*_test.go`, `pkg/apifrontend/tools/*_test.go`, `cmd/apifrontend/main_wiring_test.go` |
| KA integration tests | `test/integration/kubernautagent/session_lifecycle_test.go` |
| AA integration tests | `test/integration/aianalysis/investigating_handler_test.go` |
| AF integration tests | `test/integration/apifrontend/auth_session_test.go` |
| E2E tests | `test/e2e/interactive_pipeline_test.go` |

## 12. Schedule

| Phase | Commit Group | Bugs | Estimated Effort |
|-------|-------------|------|-----------------|
| 1 | KA session lifecycle unification | KA-CRIT-1/2, KA-MED-3/4/5 | 2 days |
| 2 | KA buildFinalResult Phase 3 merge | KA-HIGH-1, KA-MED-1/2/8 | 1 day |
| 3 | KA mutex and race fixes | KA-HIGH-2/3/4/5, KA-MED-6/7 | 2 days |
| 4 | AA timeout and failure handling | AA-CRIT-1/2, AA-HIGH-1/2/3/4, AA-MED-1/2/3/9 | 2 days |
| 5 | AF auth hardening and resources | AF-CRIT-1/2, AF-HIGH-1/2/3/4/5 | 2 days |
| 6 | AF input validation and lifecycle | AF-MED-1-8 | 1 day |
| 7 | E2E validation | E2E-1351-001/002/003 | 1 day |
