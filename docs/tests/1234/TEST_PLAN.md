# Test Plan: AF 4-Phase Interactive Journey Gap Assessment

**Identifier:** TP-1234-v1.0
**Template Version:** 2.0 -- Hybrid IEEE 829-2008 + Kubernaut
**Issue:** [#1234](https://github.com/jordigilh/kubernaut/issues/1234)
**Parent Issue:** [#1189](https://github.com/jordigilh/kubernaut/issues/1189)
**Status:** Active
**Author:** AI Agent
**Date:** 2026-05-22

---

## 1. Introduction

### 1.1 Purpose

Validate the end-to-end 4-phase interactive investigation journey in AF (investigate -> discover -> select -> watch) across 22 gaps (G1--G22) spanning functional wiring, security hardening, resilience, observability, and test coverage.

### 1.2 Feature Description

AF has tools registered for most phases but the interactive session lifecycle is not wired end-to-end. This plan covers:

- G1--G8: Functional gaps (interactive actions, session pool, args, stream bridge, CRD lifecycle, timeouts, naming)
- G9--G17: GA readiness (security, resilience, observability)
- G18--G22: Metrics, disconnect detection, multi-replica, doc drift, test coverage

### 1.3 Objectives

1. All 8 KA interactive actions callable via AF MCP bridge
2. Persistent KA MCP session pool with user isolation
3. Deferred IS CRD materialization with correct spec from birth
4. Input validation, audit enrichment, per-tool timeouts on all paths
5. KA-side SAR defense-in-depth
6. >=80% per-tier testable code coverage on modified files

### 1.4 Success Metrics

| Metric | Target |
|--------|--------|
| Unit test scenarios | 121 |
| Integration test scenarios | 36 |
| E2E test scenarios | 12 |
| Per-tier coverage (UT) | >=80% on modified files |
| Per-tier coverage (IT) | >=80% on modified files |
| Build / lint | Zero new errors |
| Race detector | Clean under `-race` |

---

## 2. References

### 2.1 Authoritative Documents

- [ADR-022: AF SA Unified Security Model](../../services/apifrontend/adr/ADR-022-af-sa-unified-security-model.md)
- [ADR-007: Spec Immutability](../../adr/ADR-007-spec-immutability.md)
- [Gap Assessment Plan](../../../.cursor/plans/af_interactive_journey_gap_a2eed676.plan.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)

### 2.2 Implementation Files

| File | Gap |
|------|-----|
| `pkg/apifrontend/ka/mcp_client.go` | G1, G3 |
| `pkg/apifrontend/ka/mcp_sdk_client.go` | G3, G10, G18 |
| `pkg/apifrontend/ka/session_pool.go` (new) | G2, G9, G14, G15 |
| `pkg/apifrontend/handler/mcp_bridge.go` | G1, G5, G7, G11, G12 |
| `pkg/apifrontend/handler/mcptools.go` | G1, G5 |
| `pkg/apifrontend/tools/ka_tools.go` | G1, G8, G13 |
| `pkg/apifrontend/tools/ka_stream.go` | G5 |
| `pkg/apifrontend/session/service.go` | G6 |
| `pkg/apifrontend/agent/root.go` | G6 |
| `internal/kubernautagent/mcp/tools/investigate.go` | G17 |
| `deploy/apifrontend/base/02-rbac.yaml` | G16 |
| `charts/kubernaut/values.yaml` | G16 |

### 2.3 Existing Related Tests

| File | Scenarios | Impact |
|------|-----------|--------|
| `pkg/apifrontend/ka/mcp_sdk_client_test.go` | 12 | G3: arg schema change breaks all |
| `pkg/apifrontend/session/service_test.go` | 38 | G6: 14 hard breaks |
| `pkg/apifrontend/session/statemachine_test.go` | 10 | G6: 3 breaks |
| `pkg/apifrontend/handler/mcp_bridge_test.go` | 74 | G1: add 7 new tool dispatch tests |
| `test/e2e/apifrontend/session_lifecycle_test.go` | 4 | G6: 1 break (JOIN-02) |
| `test/e2e/apifrontend/streaming_test.go` | 4 | G6: 1 break (STREAM-02) |

### 2.4 Proven Codebase Patterns

- `ka.MockMCPClient` with `*Fn` callbacks for MCP tool mocking
- `setupStackWithKAHandler` in `mcp_bridge_integration_test.go` for bridge ITs
- `fetchDEXTokenForPersona` + `initMCPSession` for E2E MCP testing
- `dynamicfake.NewSimpleDynamicClient` for K8s tool UTs
- `spyEmitter` / `fakeAuditor` for audit assertions

---

## 3. Risks & Mitigations

| ID | Risk | Probability | Impact | Affected Tests | Mitigation |
|----|------|------------|--------|----------------|------------|
| R1 | MCP SDK doesn't support session reuse | Low | High | UT-AF-1234-011..028 | PF-1: Confirmed reuse via FP-MCP-001/005 evidence |
| R2 | Session service test breakage cascade (22 tests) | High | Medium | UT-AF-200/210/250 series | PF-2: `materializeSession` helper; systematic update in C3 RED |
| R3 | E2E flakiness from CRD timing change | Medium | Medium | E2E-AF-1234-003/004 | PF-3: Only 2 tests affected; increase Eventually timeouts |
| R4 | wrapTool changes break 14 existing tools | Low | High | UT-AF-B-001..014 | Per-tool timeout is additive (map lookup with fallback) |
| R5 | Pool idle eviction goroutine leak under -race | Medium | Medium | UT-AF-1234-019..022 | Use context.WithCancel + time.AfterFunc, not background goroutine |

---

## 4. Scope

### 4.1 In Scope

- G1--G22 functional, security, resilience, observability, and test gaps
- All AF packages: `ka/`, `handler/`, `tools/`, `session/`, `agent/`, `audit/`
- KA package: `internal/kubernautagent/mcp/tools/` (G17 SAR only)
- RBAC manifests: `deploy/`, `charts/`
- Documentation: `AUDIT_EVENT_CATALOG.md`, `ARCHITECTURE.md`, SLO, runbooks

### 4.2 Out of Scope

- ADR-013 supersession / JWT delegation removal (separate issue/PR)
- KA MCP NetworkPolicy (kubernaut-operator#122)
- Production persona ClusterRoles (kubernaut-operator#122)
- DS schema changes
- KA tool decomposition (breaking up kubernaut_investigate)

### 4.3 Design Decisions

- **G6 Option B:** Deferred CRD materialization. CRDSessionService stays in chain for metrics/audit. K8s CRD created only after af_create_rr via MaterializeCRD.
- **G17:** KA SAR checks caller identity against 3 KA tool names, not AF per-action names.
- **G2:** Pool keyed by (rr_id, username) with AF SA token transport (not per-user JWT).

---

## 5. Approach

### 5.1 Coverage Policy

>=80% per-tier testable code coverage on all modified files. Measured via `scripts/coverage/coverage_report.py`.

### 5.2 TDD Phases

| Phase | Description | Gate |
|-------|-------------|------|
| Phase 0 | IEEE 829 test plan | This document |
| C1 RED | Failing tests: G3+G2+G9+G10 | CP-1 |
| C1 GREEN | Minimal implementation | CP-2 |
| C1 REFACTOR | Quality + 100 Go Mistakes | CP-3 |
| C2 RED | Failing tests: G1+G5+G7+G8 | CP-4 |
| C2 GREEN | Minimal implementation | CP-5 |
| C2 REFACTOR | Quality + 100 Go Mistakes | CP-6 |
| C3 RED | Failing tests: G6+G19 | CP-7 |
| C3 GREEN | Minimal implementation | CP-8 |
| C3 REFACTOR | Quality + 100 Go Mistakes | CP-9 |
| C4 RED | Failing tests: G13+G16+G17 | CP-10 |
| C4 GREEN | Minimal implementation | CP-11 |
| C4 REFACTOR | Quality + 100 Go Mistakes | CP-12 |
| C5 RED | Failing tests: G11+G12+G18 | CP-13 |
| C5 GREEN | Minimal implementation | CP-14 |
| C5 REFACTOR | Quality + 100 Go Mistakes | CP-15 |
| C6 RED | Failing tests: G14+G15+G20 | CP-16 |
| C6 GREEN | Minimal implementation | CP-17 |
| C6 REFACTOR | Quality + 100 Go Mistakes | CP-18 |
| C7 | Docs + E2E + final audit | CP-19 |

### 5.3 Anti-Pattern Compliance

- No `time.Sleep()` (use `Eventually`/`Consistently` or `time.AfterFunc`)
- No `Skip()` / `XIt` / pending tests
- No standard Go `testing` (Ginkgo/Gomega only)
- No mocking internal business logic (mock only external deps)
- Test IDs in all `Describe()` blocks

---

## 6. Test Design Specification

### 6.1 Cycle 1: Foundation (G3 + G2 + G9 + G10) -- 38 scenarios

#### 6.1.1 Unit Tests -- InvokeAction (G3)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-001 | InvokeAction sends `{rr_id, action: "start"}` for start action | G3 | Happy Path |
| UT-AF-1234-002 | InvokeAction sends `{rr_id, action: "investigate"}` with takeover intent (consolidated per #1332) | G3 | Happy Path |
| UT-AF-1234-003 | InvokeAction sends `{rr_id, action: "message", message: "..."}` | G3 | Happy Path |
| UT-AF-1234-004 | InvokeAction sends `{rr_id, action: "discover_workflows"}` | G3 | Happy Path |
| UT-AF-1234-005 | InvokeAction sends `{rr_id, action: "complete"}` | G3 | Happy Path |
| UT-AF-1234-006 | acting_user injected from UserIdentityFromContext | G3 | Happy Path |
| UT-AF-1234-007 | acting_user_groups injected as string slice | G3 | Happy Path |
| UT-AF-1234-008 | Nil UserIdentity returns error (fail-closed) | G3 | Security |
| UT-AF-1234-009 | KA unavailable returns user-friendly error | G3 | Error |
| UT-AF-1234-010 | KA IsError result wrapped as kubernaut agent error | G3 | Error |

#### 6.1.2 Unit Tests -- KASessionPool (G2 + G9)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-011 | Acquire creates new session via client.Connect | G2 | Happy Path |
| UT-AF-1234-012 | Acquire reuses existing session for same (rr_id, user) | G2 | Happy Path |
| UT-AF-1234-013 | Acquire creates separate sessions for different rr_ids | G2 | Happy Path |
| UT-AF-1234-014 | Acquire creates separate sessions for different users same rr_id | G9 | Security |
| UT-AF-1234-015 | Acquire returns error when client.Connect fails | G2 | Error |
| UT-AF-1234-016 | Release closes session and removes pool entry | G2 | Happy Path |
| UT-AF-1234-017 | Release non-existent key is no-op (no panic) | G2 | Edge Case |
| UT-AF-1234-018 | Release then Acquire creates fresh session | G2 | Happy Path |
| UT-AF-1234-019 | Idle session evicted after configurable TTL | G2 | Resilience |
| UT-AF-1234-020 | Max pool size cap rejects new Acquire with error | G15 | Resilience |
| UT-AF-1234-021 | ErrSessionMissing from CallTool triggers reconnect | G2 | Resilience |
| UT-AF-1234-022 | ErrConnectionClosed evicts entry and reconnects | G2 | Resilience |
| UT-AF-1234-023 | Parallel Acquire for same key serializes (no double-connect) | G2 | Concurrency |
| UT-AF-1234-024 | Parallel Acquire for different keys succeeds concurrently | G2 | Concurrency |
| UT-AF-1234-025 | RWMutex safety under -race with mixed read/write | G2 | Concurrency |
| UT-AF-1234-026 | DrainAll closes all sessions | G14 | Happy Path |
| UT-AF-1234-027 | User A cannot reuse User B session (composite key) | G9 | Security |
| UT-AF-1234-028 | RR ownership check validates IS CRD spec.userIdentity | G9 | Security |

#### 6.1.3 Unit Tests -- Retry/CB (G10)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-029 | CB transitions closed->open after N consecutive failures | G10 | Happy Path |
| UT-AF-1234-030 | Retry with backoff on 503 response | G10 | Resilience |

#### 6.1.4 Unit Tests -- Foundation (httptest)

NOTE: These tests use httptest servers but run via `make test-unit-apifrontend`, so they are classified as UT.

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-201 | SDKMCPClient.InvokeAction wire format against httptest MCP | G3 | Happy Path |
| UT-AF-1234-202 | InvokeAction forwards acting_user in args to KA | G3 | Happy Path |
| UT-AF-1234-203 | InvokeAction error mapping from httptest 500 | G3 | Error |
| UT-AF-1234-204 | Pool acquire/release with httptest endpoint | G2 | Happy Path |
| UT-AF-1234-205 | Pool session reuse verified via server session count | G2 | Happy Path |
| UT-AF-1234-206 | CB trips after 5 failures on httptest | G10 | Resilience |
| UT-AF-1234-207 | Retry on 503 from httptest succeeds on 2nd attempt | G10 | Resilience |
| UT-AF-1234-208 | af_circuit_breaker_state{dependency="ka-mcp"} metric emitted | G10 | Observability |

---

### 6.2 Cycle 2: Interactive Actions + Bridge (G1 + G5 + G7 + G8) -- 49 scenarios

#### 6.2.1 Unit Tests -- Interactive Action Handlers (G1)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-031 | HandleTakeover happy path returns session_id + status | G1 | Happy Path |
| UT-AF-1234-032 | HandleTakeover KA error returns user-friendly message | G1 | Error |
| UT-AF-1234-033 | HandleTakeover nil MCPClient returns ErrMCPUnavailable | G1 | Error |
| UT-AF-1234-034 | HandleMessage happy path with message text | G1 | Happy Path |
| UT-AF-1234-035 | HandleMessage empty message rejected | G1 | Validation |
| UT-AF-1234-036 | HandleMessage KA error returns user-friendly message | G1 | Error |
| UT-AF-1234-037 | HandleComplete happy path returns completed status | G1 | Happy Path |
| UT-AF-1234-038 | HandleComplete KA error | G1 | Error |
| UT-AF-1234-039 | HandleCancel happy path returns cancelled status | G1 | Happy Path |
| UT-AF-1234-040 | HandleCancel KA error | G1 | Error |
| UT-AF-1234-041 | HandleStatus happy path returns session state | G1 | Happy Path |
| UT-AF-1234-042 | HandleStatus KA error | G1 | Error |
| UT-AF-1234-043 | HandleReconnect happy path returns reconnected status | G1 | Happy Path |
| UT-AF-1234-044 | HandleReconnect KA error | G1 | Error |

#### 6.2.2 Unit Tests -- Stream Investigation (G5)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-045 | HandleStreamInvestigation empty session_id rejected | G5 | Validation |
| UT-AF-1234-046 | SSE reasoning_delta appended to narrative | G5 | Happy Path |
| UT-AF-1234-047 | SSE token_delta appended to narrative | G5 | Happy Path |
| UT-AF-1234-048 | SSE tool_call_start adds tool marker to narrative | G5 | Happy Path |
| UT-AF-1234-049 | SSE tool_call records in event log | G5 | Happy Path |
| UT-AF-1234-050 | SSE tool_result truncated to 500 chars | G5 | Happy Path |
| UT-AF-1234-051 | SSE complete extracts summary, returns completed status | G5 | Happy Path |
| UT-AF-1234-052 | SSE cancelled returns cancelled status | G5 | Happy Path |
| UT-AF-1234-053 | SSE error returns failed status | G5 | Error |
| UT-AF-1234-054 | Context cancel mid-stream returns cancelled with partial | G5 | Edge Case |

#### 6.2.3 Unit Tests -- Per-tool Timeout (G7)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-055 | Timeout lookup by name returns configured override | G7 | Happy Path |
| UT-AF-1234-056 | Missing tool name falls back to default 30s | G7 | Happy Path |
| UT-AF-1234-057 | Hard cap 30m enforced (configured value >30m clamped) | G7 | Validation |
| UT-AF-1234-058 | Zero ToolTimeouts config uses default 30s | G7 | Edge Case |

#### 6.2.4 Unit Tests -- Rename + Constructors (G8)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-059 | present_decision ADK tool name is kubernaut_present_decision | G8 | Happy Path |
| UT-AF-1234-060 | NewStreamInvestigationTool constructor returns valid tool | G5 | Happy Path |
| UT-AF-1234-061 | New interactive tool constructors (6) return valid tools | G1 | Happy Path |
| UT-AF-1234-062 | Constructor with nil KAClient returns error | G1 | Error |

#### 6.2.5 Unit Tests -- Audit for New Handlers

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-063 | HandleTakeover emits EventKADelegated audit | G1 | Observability |
| UT-AF-1234-064 | HandleMessage emits EventToolExecuted audit | G1 | Observability |
| UT-AF-1234-065 | HandleStreamInvestigation complete emits EventKAResultReceived | G5 | Observability |

#### 6.2.6 Unit Tests -- Bridge Dispatch (G1 + G5)

NOTE: Bridge dispatch tests use httptest + MockMCPClient and run via `make test-unit-apifrontend`, so they are classified as UT.

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-210 | Bridge dispatch kubernaut_investigate (takeover intent) via MCP protocol (#1332: consolidated) | G1 | Happy Path |
| UT-AF-1234-211 | Bridge dispatch kubernaut_message via MCP protocol | G1 | Happy Path |
| UT-AF-1234-212 | Bridge dispatch kubernaut_complete via MCP protocol | G1 | Happy Path |
| UT-AF-1234-213 | Bridge dispatch kubernaut_cancel via MCP protocol | G1 | Happy Path |
| UT-AF-1234-214 | Bridge dispatch kubernaut_status via MCP protocol | G1 | Happy Path |
| UT-AF-1234-215 | Bridge dispatch kubernaut_reconnect via MCP protocol | G1 | Happy Path |
| UT-AF-1234-216 | Bridge RBAC denial: viewer denied kubernaut_message | G1 | Security |
| UT-AF-1234-217 | Bridge RBAC denial: ai-orchestrator denied kubernaut_cancel | G1 | Security |
| UT-AF-1234-218 | Interactive tool timeout is 5m not 30s | G7 | Happy Path |
| UT-AF-1234-219 | Stream tool timeout is 30m not 30s | G7 | Happy Path |
| UT-AF-1234-220 | kubernaut_stream_investigation registered and callable | G5 | Happy Path |
| UT-AF-1234-221 | kubernaut_present_decision dispatches correctly | G8 | Happy Path |

#### 6.2.7 E2E Tests -- Interactive Journey

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| E2E-AF-1234-001 | MCP chain: start -> discover -> select -> watch (sre persona) | G1 | Happy Path |
| E2E-AF-1234-002 | RBAC: viewer can status, denied on investigate/message | G1 | Security |

---

### 6.3 Cycle 3: Session Lifecycle (G6 + G19) -- 26 scenarios

#### 6.3.1 Unit Tests -- Deferred CRD (G6)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-070 | Create does NOT call K8s API (no CRD in fake client) | G6 | Happy Path |
| UT-AF-1234-071 | Create creates in-memory delegate session | G6 | Happy Path |
| UT-AF-1234-072 | Create increments af_sessions_active metric | G6 | Observability |
| UT-AF-1234-073 | Create stores CreateConfig in crdIndex for later | G6 | Happy Path |
| UT-AF-1234-074 | MaterializeCRD creates IS CRD with correct remediationRequestRef | G6 | Happy Path |
| UT-AF-1234-075 | MaterializeCRD sets correct labels (rr-name, phase, managed-by) | G6 | Happy Path |
| UT-AF-1234-076 | MaterializeCRD sets correct userIdentity from stored config | G6 | Happy Path |
| UT-AF-1234-077 | MaterializeCRD returns error if session not in crdIndex | G6 | Error |
| UT-AF-1234-078 | MaterializeCRD K8s create failure returns error, keeps crdIndex | G6 | Error |
| UT-AF-1234-079 | MaterializeCRD idempotent (already materialized is no-op) | G6 | Edge Case |
| UT-AF-1234-080 | af_create_rr callback calls MaterializeCRD on success | G6 | Happy Path |
| UT-AF-1234-081 | af_create_rr callback does NOT call MaterializeCRD on error | G6 | Error |
| UT-AF-1234-082 | af_create_rr populates CreateContext.RRName/RRNamespace | G6 | Happy Path |

#### 6.3.2 Unit Tests -- Disconnect Detection (G19)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-083 | SSE close triggers UpdatePhase(Disconnected) | G19 | Happy Path |
| UT-AF-1234-084 | TTL reconciler: Active CRD stale heartbeat -> Disconnected | G19 | Happy Path |
| UT-AF-1234-085 | TTL reconciler: Disconnected expired TTL -> Cancelled | G19 | Happy Path |

#### 6.3.3 Unit Tests -- Edge Cases

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-086 | Create then Delete before MaterializeCRD cleans up | G6 | Edge Case |
| UT-AF-1234-087 | Concurrent MaterializeCRD for same session is safe | G6 | Concurrency |

#### 6.3.4 Integration Tests -- Deferred CRD (G6)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| IT-AF-1234-030 | Create -> no CRD in envtest -> MaterializeCRD -> CRD exists | G6 | Happy Path |
| IT-AF-1234-031 | MaterializeCRD spec.remediationRequestRef matches real RR ref | G6 | Happy Path |
| IT-AF-1234-032 | MaterializeCRD labels correct (rr-name = real name) | G6 | Happy Path |
| IT-AF-1234-033 | TTL reconciler: Active stale -> Disconnected in envtest | G19 | Happy Path |
| IT-AF-1234-034 | TTL reconciler: Disconnected -> Cancelled in envtest | G19 | Happy Path |
| IT-AF-1234-035 | A2A launcher flow: send -> af_create_rr -> IS CRD appears | G6 | Happy Path |

#### 6.3.5 E2E Tests -- Session Lifecycle

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| E2E-AF-1234-003 | IS CRD created only after af_create_rr (not on session start) | G6 | Happy Path |
| E2E-AF-1234-004 | SSE client disconnect transitions IS CRD to Disconnected | G19 | Happy Path |

---

### 6.4 Cycle 4: Security Hardening (G13 + G16 + G17) -- 22 scenarios

#### 6.4.1 Unit Tests -- Input Validation (G13)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-090 | ParseRRID rejects malformed rr_id (no slash) | G13 | Validation |
| UT-AF-1234-091 | ParseRRID rejects empty rr_id | G13 | Validation |
| UT-AF-1234-092 | ParseRRID rejects path traversal (../etc/passwd) | G13 | Security |
| UT-AF-1234-093 | ParseRRID applied on start_investigation, discover, select, stream | G13 | Happy Path |
| UT-AF-1234-094 | validate.Namespace rejects path traversal on KA tool args | G13 | Security |
| UT-AF-1234-095 | validate.ResourceName rejects empty name | G13 | Validation |
| UT-AF-1234-096 | Message length >10KB rejected | G13 | Validation |
| UT-AF-1234-097 | Invalid action string rejected at AF (not forwarded to KA) | G13 | Security |
| UT-AF-1234-098 | ValidateWorkflowParameters: missing required param rejected | G13 | Validation |

#### 6.4.2 Unit Tests -- Persona/RBAC (G16)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-099 | sre persona has all interactive tool names | G16 | Happy Path |
| UT-AF-1234-100 | ai-orchestrator has start/discover/select/complete | G16 | Happy Path |
| UT-AF-1234-101 | viewer has status only | G16 | Happy Path |
| UT-AF-1234-102 | AF SA ClusterRole includes kubernaut_investigate + select + complete_no_action | G16 | Happy Path |

#### 6.4.3 Unit Tests -- KA-side SAR (G17)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-KA-1234-110 | AF SA allowed for kubernaut_investigate | G17 | Happy Path |
| UT-KA-1234-111 | AF SA allowed for kubernaut_select_workflow | G17 | Happy Path |
| UT-KA-1234-112 | AF SA allowed for kubernaut_complete_no_action | G17 | Happy Path |
| UT-KA-1234-113 | Unauthorized SA denied for kubernaut_investigate | G17 | Security |
| UT-KA-1234-114 | acting_user in payload NOT used for authorization | G17 | Security |
| UT-KA-1234-115 | Missing acting_user allowed if SA has role | G17 | Edge Case |

#### 6.4.4 Unit Tests -- Security (Bridge Dispatch)

NOTE: Bridge dispatch validation tests run via `make test-unit-apifrontend`, so they are classified as UT.

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-240 | Bridge rejects malformed rr_id with user-friendly error | G13 | Validation |
| UT-AF-1234-241 | Bridge rejects invalid action with user-friendly error | G13 | Validation |
| IT-KA-1234-042 | KA MCP rejects unauthorized SA | G17 | Security |
| IT-KA-1234-043 | KA MCP allows AF SA for kubernaut_investigate | G17 | Happy Path |

#### 6.4.5 E2E Tests -- Security

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| E2E-AF-1234-005 | Viewer persona: list_remediations OK, investigate denied | G16 | Security |
| E2E-AF-1234-006 | Malformed rr_id returns clean error via MCP bridge | G13 | Validation |

---

### 6.5 Cycle 5: Observability (G11 + G12 + G18) -- 18 scenarios

#### 6.5.1 Unit Tests -- Audit Enrichment (G11)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-120 | wrapTool emits execution_duration_ms in audit detail | G11 | Observability |
| UT-AF-1234-121 | wrapTool emits request_id from requestid.FromContext | G11 | Observability |
| UT-AF-1234-122 | wrapTool emits session_id from tool args | G11 | Observability |
| UT-AF-1234-123 | wrapTool emits rr_id from tool args | G11 | Observability |
| UT-AF-1234-124 | EventMCPSessionInit includes mcp_session_id | G11 | Observability |
| UT-AF-1234-125 | EventWorkflowDiscovery payload built correctly | G11 | Observability |
| UT-AF-1234-126 | Audit enrichment absent when args have no session_id | G11 | Edge Case |
| UT-AF-1234-127 | Audit enrichment absent when args have no rr_id | G11 | Edge Case |

#### 6.5.2 Unit Tests -- Success Logging (G12)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-128 | Success log has fields: tool, user, duration_ms, request_id | G12 | Observability |
| UT-AF-1234-129 | Success log is Info level | G12 | Observability |

#### 6.5.3 Unit Tests -- KA MCP Metrics (G18)

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-130 | MCP transport records duration with dependency=ka-mcp | G18 | Observability |
| UT-AF-1234-131 | Timer not started on error before call (no phantom metric) | G18 | Edge Case |

#### 6.5.4 Unit Tests -- Observability (Bridge Dispatch + httptest)

NOTE: Bridge dispatch and httptest-based metric tests run via `make test-unit-apifrontend`, so they are classified as UT.

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-250 | Bridge start_investigation emits enriched audit to DS | G11 | Happy Path |
| UT-AF-1234-251 | Enriched audit event queryable with session_id field | G11 | Happy Path |
| UT-AF-1234-252 | KA MCP histogram has observation after success | G18 | Happy Path |
| UT-AF-1234-253 | KA MCP histogram has observation after failure | G18 | Error |

#### 6.5.5 E2E Tests -- Audit Trace

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| E2E-AF-1234-007 | Interactive audit trace: 3 events correlated by session_id | G11 | Happy Path |
| E2E-AF-1234-008 | Audit event has execution_duration_ms > 0 | G11 | Observability |

---

### 6.6 Cycle 6: Resilience (G14 + G15 + G20) -- 12 scenarios

#### 6.6.1 Unit Tests -- Resilience

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-140 | DrainAll closes all pool sessions | G14 | Happy Path |
| UT-AF-1234-141 | DrainAll respects context deadline | G14 | Edge Case |
| UT-AF-1234-142 | DrainAll on empty pool is no-op | G14 | Edge Case |
| UT-AF-1234-143 | IS CRD best-effort Disconnected on shutdown | G14 | Happy Path |
| UT-AF-1234-144 | IS CRD transition failure logged but not fatal | G14 | Error |
| UT-AF-1234-145 | AcquireSession rejects when at max sessions | G15 | Validation |
| UT-AF-1234-146 | Pool Acquire rejects when at maxEntries | G15 | Validation |
| UT-AF-1234-147 | Idle eviction triggered at configurable interval | G15 | Happy Path |
| UT-AF-1234-148 | ErrLeaseHeld returns "investigation active on another session" | G20 | Error |
| UT-AF-1234-149 | Pool entry not cached on KA rejection | G20 | Edge Case |

#### 6.6.2 Unit Tests -- Resilience (httptest)

NOTE: httptest-based resilience tests run via `make test-unit-apifrontend`, so they are classified as UT.

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| UT-AF-1234-260 | DrainAll with httptest: all server sessions closed | G14 | Happy Path |
| UT-AF-1234-261 | Session cap enforcement through A2A launcher | G15 | Happy Path |

---

### 6.7 Cycle 7: Docs + E2E (G21 + G22) -- 4 E2E scenarios

| ID | Description | AC | Category |
|----|-------------|-----|----------|
| E2E-AF-1234-009 | Full A2A interactive journey via mock-LLM keywords | G22 | Happy Path |
| E2E-AF-1234-010 | MCP session persistence: two tools share KA session | G22 | Happy Path |
| E2E-AF-1234-011 | IS CRD lifecycle: Active -> Completed after select | G22 | Happy Path |
| E2E-AF-1234-012 | KA unavailable: user-friendly error + CB metric | G22 | Resilience |

---

## 7. BR Coverage Matrix

| Gap | AC | UT IDs | IT IDs | E2E IDs | Status |
|-----|-----|--------|--------|---------|--------|
| G1 | Interactive actions | 031-044, 060-065, 210-217 | - | 001, 002 | Pending |
| G2 | Session pool | 011-026, 204-205 | - | - | Pending |
| G3 | Args + identity | 001-010, 201-203 | - | - | Pending |
| G5 | Stream on bridge | 045-054, 060, 220 | - | - | Pending |
| G6 | Deferred CRD | 070-087 | 030-035 | 003, 004 | Pending |
| G7 | Per-tool timeouts | 055-058, 218-219 | - | - | Pending |
| G8 | Rename | 059, 221 | - | - | Pending |
| G9 | Pool isolation | 014, 027-028 | - | - | Pending |
| G10 | Retry/CB | 029-030, 206-208 | - | - | Pending |
| G11 | Audit enrichment | 120-127, 250-251 | - | 007, 008 | Pending |
| G12 | Success logging | 128-129 | - | - | Pending |
| G13 | Input validation | 090-098, 240-241 | - | 006 | Pending |
| G14 | Graceful shutdown | 026, 140-144, 260 | - | - | Pending |
| G15 | Session cap | 020, 145-147, 261 | - | - | Pending |
| G16 | Persona/RBAC | 099-102 | - | 005 | Pending |
| G17 | KA SAR | 110-115 | 042-043 | - | Pending |
| G18 | KA MCP metrics | 130-131, 252-253 | - | - | Pending |
| G19 | Disconnect detection | 083-085 | 033-034 | 004 | Pending |
| G20 | Multi-replica | 148-149 | - | - | Pending |
| G21 | Doc drift | - | - | - | Pending |
| G22 | E2E coverage | - | - | 009-012 | Pending |

---

## 8. Test Case Specifications (P0 Representative Cases)

### TP-1234-001: InvokeAction Sends Correct Args

**AC:** G3 | **Type:** Unit | **Priority:** P0

**Preconditions:** httptest MCP server expecting `kubernaut_investigate` tool call

**Steps:**
- Given: SDKMCPClient configured with httptest endpoint
- When: InvokeAction(ctx, "ns/rr-001", "investigate", "", identity) called (takeover consolidated into investigate per #1332)
- Then: Server receives args `{"rr_id": "ns/rr-001", "acting_user": "alice", "acting_user_groups": ["sre"]}`

### TP-1234-002: Pool User Isolation

**AC:** G9 | **Type:** Unit | **Priority:** P0

**Preconditions:** Pool with one session for (rr-001, alice)

**Steps:**
- Given: Pool entry exists for key (rr-001, alice)
- When: Acquire(ctx, "rr-001", "bob") called
- Then: New session created (not alice's session), pool has 2 entries

### TP-1234-003: Deferred CRD Materialization

**AC:** G6 | **Type:** Unit | **Priority:** P0

**Preconditions:** CRDSessionService with fake K8s client

**Steps:**
- Given: CRDSessionService.Create(ctx, req) returns successfully
- When: K8s client List IS CRDs
- Then: Zero CRDs exist
- When: MaterializeCRD(sessionID, ObjectRef{Name: "rr-pod-crash-123", Namespace: "prod"}) called
- Then: Exactly one IS CRD exists with spec.remediationRequestRef.name = "rr-pod-crash-123"

---

## 9. Checkpoint Specifications

### CHECKPOINT 0 (after Phase 0)

**Gate:** Test plan document exists, all scenario IDs assigned, BR coverage matrix complete.

### CHECKPOINT 1 (CP-1, after C1 RED)

**Gate:** All C1 tests compile and fail for behavioral reasons (not compile errors).

**Preflight checklist:**
- [ ] `go build ./...` passes
- [ ] No `Skip()` or `XIt` in new tests
- [ ] All test IDs in `Describe()` blocks
- [ ] Tests fail with assertion errors, not panics
- [ ] Confidence >=95%

### CHECKPOINT 2 (CP-2, after C1 GREEN)

**Gate:** All C1 tests pass. No existing tests broken.

**Preflight checklist:**
- [ ] `go test ./pkg/apifrontend/ka/... -race` passes
- [ ] `go test ./pkg/apifrontend/handler/... -race` passes
- [ ] `go vet ./...` clean
- [ ] Coverage >=80% on new files

### CHECKPOINT 3 (CP-3, after C1 REFACTOR)

**Gate:** Code quality audit passed.

**9-Category Audit:**
1. Observability: metrics registered, audit events emitted
2. Adversarial inputs: fuzz-like edge cases in tests
3. Resource bounds: pool max size, timeout caps
4. Concurrency: -race clean, no goroutine leaks
5. Nil/zero: nil identity, nil client, empty args handled
6. Error-path observability: errors logged with context
7. Cross-phase integration: pool wired in main.go
8. Spec compliance: ADR-022 alignment verified
9. API surface hygiene: no exported types without tests

**100 Go Mistakes Audit:**
- [ ] #1 Variable shadowing: no shadowed err in pool ops
- [ ] #2 Unnecessary nesting: happy path aligned left
- [ ] #5 Interface pollution: MCPClient interface minimal
- [ ] #21 Slice init: pre-allocate where length known
- [ ] #26 Map init: pool map initialized with capacity hint
- [ ] #48 Context: all pool methods accept context
- [ ] #53 Goroutine leaks: eviction timer uses context.WithCancel
- [ ] #54 Channel misuse: no unbuffered channels without goroutine
- [ ] #66 Defer in loops: no defer in pool iteration
- [ ] #78 Race conditions: -race clean
- [ ] #85 Error wrapping: fmt.Errorf with %w

**GA Readiness:**
- [ ] Security: pool isolation tested
- [ ] Resilience: CB/retry/reconnect tested
- [ ] Observability: metrics + audit wired
- [ ] Coverage: >=80% per tier
- [ ] Lint: `golangci-lint run --timeout=5m` clean

**Confidence >=95% to proceed to C2.**

### CHECKPOINT 4-18

Follow same template as CP-1/2/3 for each cycle's RED/GREEN/REFACTOR phases.

### CHECKPOINT 19 (CP-19, Final)

**Gate:** All 169 tests pass. Full GA readiness audit across all dimensions.

**Final checklist:**
- [ ] `go build ./...` passes
- [ ] `golangci-lint run --timeout=5m` clean
- [ ] `go test ./pkg/apifrontend/... -race` passes
- [ ] `go test ./internal/kubernautagent/... -race` passes
- [ ] Coverage >=80% per tier on all modified files
- [ ] No new `Skip()` or pending tests
- [ ] All doc drift fixes in G21 applied
- [ ] ADR-022 alignment verified
- [ ] Confidence >=95%

---

## 10. Environmental Needs

### Unit Tests

- Ginkgo/Gomega framework
- `dynamicfake.NewSimpleDynamicClient` for K8s
- `ka.MockMCPClient` for MCP
- `httptest.NewServer` for HTTP backends
- `spyEmitter` / `fakeAuditor` for audit
- `mapAuthorizer` for RBAC

### Integration Tests

- envtest (KUBEBUILDER_ASSETS)
- httptest for KA/DS backends
- `mcp_bridge_integration_test.go` helpers
- Podman for DS/KA containers (suite-level)

### E2E Tests

- Kind cluster via `make test-e2e-apifrontend`
- AF+KA+DS+MockLLM+DEX real deployments
- 6 DEX personas
- MCP session helpers

---

## 11. Dependencies & Schedule

### Dependency Chain

```
C1 (G3+G2+G9+G10) -> C2 (G1+G5+G7+G8) -> C4 (G13+G16+G17)
                                          -> C5 (G11+G12+G18)
                                          -> C6 (G14+G15+G20)
C3 (G6+G19) independent of C1/C2 but should follow C2 for test helper reuse
C7 (G21+G22) last
```

### Blockers

None identified. All dependencies are internal to this PR.

---

## 12. Test Deliverables

| Artifact | Location |
|----------|----------|
| Test plan | `docs/tests/1234/TEST_PLAN.md` |
| Pool UT | `pkg/apifrontend/ka/session_pool_test.go` |
| MCP client UT | `pkg/apifrontend/ka/mcp_sdk_client_test.go` (updated) |
| Stream UT | `pkg/apifrontend/tools/ka_stream_test.go` (new) |
| Interactive tools UT | `pkg/apifrontend/tools/ka_interactive_test.go` (new) |
| Bridge UT | `pkg/apifrontend/handler/mcp_bridge_test.go` (updated) |
| Bridge IT | `pkg/apifrontend/handler/mcp_bridge_integration_test.go` (updated) |
| Session UT | `pkg/apifrontend/session/service_test.go` (updated) |
| Session IT | `test/integration/apifrontend/session_crd_test.go` (updated) |
| KA SAR UT | `internal/kubernautagent/mcp/tools/investigate_sar_test.go` (new) |
| E2E interactive | `test/e2e/apifrontend/interactive_journey_test.go` (new) |

---

## 13. Execution Commands

```bash
# Unit tests (includes bridge ITs)
make test-unit-apifrontend

# Integration tests (envtest + containers)
make test-integration-apifrontend

# E2E tests (Kind cluster)
make test-e2e-apifrontend

# Focused runs
go test ./pkg/apifrontend/ka/... -run UT-AF-1234 -race -v
go test ./pkg/apifrontend/handler/... -run UT-AF-1234 -race -v
go test ./pkg/apifrontend/tools/... -run UT-AF-1234 -race -v

# Coverage
make coverage-report
```

---

## 14. Existing Tests Requiring Updates

| File | Tests Affected | Change | Reason |
|------|---------------|--------|--------|
| `mcp_sdk_client_test.go` | 12 | Args schema update | G3 |
| `service_test.go` | 14 hard + 2 vacuous | Add materializeSession helper | G6 |
| `statemachine_test.go` | 3 (210-008/009/010) | Materialize before UpdatePhase | G6 |
| `concurrent_test.go` | 1 (250-002) | Materialize before races | G6 |
| `audit_emission_test.go` | 2 (1156-057/058) | Materialize before audit | G6 |
| `session_crd_test.go` (IT) | 1 (1195-043) | Materialize before UpdatePhase | G6 |
| `session_lifecycle_test.go` (E2E) | 1 (JOIN-02) | Change prompt or manual IS | G6 |
| `streaming_test.go` (E2E) | 1 (STREAM-02) | Multi-turn prompt or split | G6 |
| `constructors_test.go` | Add 7 | New tool constructors | G1+G5 |
| `audit_emission_test.go` (tools) | Add 3 | New handler audit | G11 |
| `mcp_bridge_test.go` | Add 7 | New tool dispatch | G1+G5 |

---

## 15. Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-22 | AI Agent | Initial test plan with 169 scenarios across 7 TDD cycles |
