# Test Plan — #1472: Validate RR Existence Before Session Reactivation

**IEEE 829 Compliant** | **Issue**: [#1472](https://github.com/jordigilh/kubernaut/issues/1472) | **Milestone**: v1.5

## 1. Test Plan Identifier

TP-1472-STALE-SESSION-VALIDATION

## 2. Introduction

### 2.1 Purpose

After an AF pod restart, the in-memory session store is empty (session hydration deferred to #1451). When a browser client sends a message with a stale `context_id` from the previous pod lifetime, the `SessionInterceptor` passes it through unconditionally. ADK's `AutoCreateSession: true` then creates an empty session with that ID, causing the LLM agent to attempt reconnection to a non-existent investigation — producing confusing "reconnecting to your investigation" messages.

### 2.2 Objectives

1. **Stale context detection**: Identify when an explicit `context_id` refers to a session that no longer exists in memory (post-restart staleness).
2. **Context clearing**: Clear the stale `context_id` so ADK generates a fresh session ID, resulting in a clean new conversation.
3. **Fail-open safety**: If the session existence check encounters unexpected errors, pass through (availability over correctness).
4. **Observability**: Log stale context clearing for SRE troubleshooting.
5. **No regression**: Existing session continuity (BR-SESS-020) and idle expiry (#1446) remain functional.

### 2.3 Business Requirements

- BR-SESS-025: Stale session invalidation — clear `context_id` when no matching in-memory session exists
- BR-SESS-020: Session continuity — active sessions continue to route correctly (no regression)
- BR-SESS-024: Boundary protection — extended with stale context validation

## 3. Features to be Tested

- F-1: `StaleSessionValidator.IsContextValid()` returns `false` when session does not exist in memory
- F-2: `StaleSessionValidator.IsContextValid()` returns `true` when session exists in memory
- F-3: `StaleSessionValidator.IsContextValid()` returns `true` (fail-open) on unexpected errors
- F-4: `SessionInterceptor.Before()` clears `msg.ContextID` when validator returns `false`
- F-5: `SessionInterceptor.Before()` preserves `msg.ContextID` when validator returns `true`
- F-6: `SessionInterceptor.Before()` logs stale context clearing with user identity and original context_id
- F-7: Existing empty-context routing (registry override) still functions
- F-8: Production wiring — validator injected into `SessionInterceptor` via `NewSessionInterceptor()`

## 4. Features Not to be Tested

- IS CRD lookup by context_id (spike S-2 determined this is unnecessary)
- Full session hydration from CRDs (deferred to #1451)
- LLM agent behavior after context clearing (LLM tests are separate)
- ADK `AutoCreateSession` internals (third-party library)

## 5. Approach

### Test Pyramid

| Tier | Scope | Count |
|---|---|---|
| Unit | Validator logic, interceptor branching with mocked validator | 6 |
| Integration | Full interceptor + real session service (InMemoryService) through production dispatch | 2 |
| E2E | Full journey: session → AF pod restart → stale context_id → fresh conversation | 1 |

### FedRAMP Control Mapping

| Control | Objective | Behavioral Assurance | Test IDs |
|---|---|---|---|
| SC-7 | Boundary protection | Stale context_id from previous pod lifetime is rejected at session boundary | UT-AF-1472-001, IT-AF-1472-001 |
| SC-10 | Network disconnect | Post-restart stale sessions are cleanly invalidated rather than silently reactivated | UT-AF-1472-001, UT-AF-1472-004 |
| SI-10 | Information input validation | Invalid context_id (no backing session) is rejected before reaching the agent | UT-AF-1472-001, IT-AF-1472-001 |
| AU-3 | Content of audit records | Stale context clearing logged with username and original context_id | UT-AF-1472-004 |

## 6. Test Cases

### 6.1 Unit Tests — Validator Logic

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| UT-AF-1472-001 | Validator returns false for non-existent session | `IsContextValid(ctx, "stale-id", "user1")` returns `false` when session service returns "not found" | SC-7, SI-10 |
| UT-AF-1472-002 | Validator returns true for existing session | `IsContextValid(ctx, "active-id", "user1")` returns `true` when session service returns valid session | — |
| UT-AF-1472-003 | Validator returns true (fail-open) on unexpected error | `IsContextValid(ctx, "any-id", "user1")` returns `true` when session service returns non-"not found" error | SC-5 |

### 6.2 Unit Tests — Interceptor Integration with Validator

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| UT-AF-1472-004 | Interceptor clears stale context_id | When validator returns `false`, `msg.ContextID` is set to `""` after `Before()` returns | SC-10, AU-3 |
| UT-AF-1472-005 | Interceptor preserves valid context_id | When validator returns `true`, `msg.ContextID` remains unchanged after `Before()` | — |
| UT-AF-1472-006 | Interceptor skips validation for empty context_id | When `msg.ContextID == ""`, validator is NOT called (existing registry logic runs) | — |

### 6.3 Integration Tests — Production Dispatch Path

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| IT-AF-1472-001 | Stale context cleared through full interceptor stack | A2A `MessageSendParams` with stale context_id arrives → interceptor clears it → response uses different (fresh) context_id | SC-7, SI-10 |
| IT-AF-1472-002 | Active session passes through full interceptor stack | A2A message with active context_id (session previously created) → interceptor preserves it → response uses same context_id | — |

### 6.4 E2E Tests — Full User Journey

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| E2E-AF-1472-001 | Stale context_id after pod restart yields fresh conversation | Session established → AF pod killed → pod restarts → same context_id sent → valid response (fresh conversation, no "reconnecting") | SC-7, SC-10, SI-10 |

## 7. Test Environment

### Unit Tests
- Mock `StaleSessionValidator` interface for interceptor tests
- Mock session service (or use a stub returning canned errors) for validator tests
- Ginkgo/Gomega BDD framework

### Integration Tests
- Real `adksession.InMemoryService` (or `CRDSessionService` with in-memory delegate)
- Real `SessionInterceptor` with real validator wired to real session service
- `httptest.NewServer` with same router/middleware as production (no port conflicts)
- No external dependencies (no K8s API, no LLM)

## 8. Pass/Fail Criteria

- All 9 test cases pass (6 UT + 2 IT + 1 E2E)
- Zero regressions in existing `session_interceptor_test.go` and `session_interceptor_it_test.go`
- `go build ./...` clean
- `golangci-lint run --timeout=5m` clean

## 9. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Session service `Get()` signature changes in ADK upgrade | Low | Medium | Interface abstracts the call; adapter pattern isolates ADK types |
| Validator adds latency to every request with explicit context_id | Low | Low | Check is purely in-memory (no I/O, no K8s API call) |
| Fail-open policy masks persistent errors | Low | Medium | Structured logging + metrics on fail-open events for SRE alerting |
