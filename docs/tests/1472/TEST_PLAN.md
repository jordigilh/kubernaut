# Test Plan — #1472: Validate RR Existence Before Session Reactivation

**IEEE 829 Compliant** | **Issue**: [#1472](https://github.com/jordigilh/kubernaut/issues/1472) | **Milestone**: v1.5.1

## 1. Test Plan Identifier

TP-1472-STALE-SESSION-VALIDATION

## 2. Introduction

### 2.1 Purpose

After an AF pod restart, the in-memory session store is empty (session hydration deferred to #1451). When a browser client sends a message with a stale `context_id` from the previous pod lifetime, ADK's `AutoCreateSession: true` creates an empty session, causing the LLM agent to attempt reconnection to a non-existent investigation — producing confusing "reconnecting to your investigation" messages.

The fix validates RR existence and phase at the tool level (`kubernaut_reconnect`) before allowing reconnection. If the RR does not exist or has reached a terminal phase, the tool returns `session_expired` — the client can then start a fresh conversation.

### 2.2 Objectives

1. **RR existence validation**: Reject reconnection when the referenced RemediationRequest does not exist in the cluster.
2. **Terminal phase rejection**: Reject reconnection when the RR exists but has completed or failed.
3. **Fail-open safety**: If the K8s client is unavailable or namespace is empty, skip validation (availability over correctness).
4. **No regression**: Existing reconnection to active RRs remains functional.

### 2.3 Business Requirements

- BR-SESS-025: Stale session invalidation — reject reconnect to non-existent or terminal RRs
- BR-SESS-020: Session continuity — active RR reconnection continues to work (no regression)
- BR-SESS-024: Boundary protection — extended with RR lifecycle validation

## 3. Features to be Tested

- F-1: `HandleReconnect` returns `session_expired` when RR does not exist
- F-2: `HandleReconnect` returns `session_expired` when RR is in a terminal phase (Completed, Failed)
- F-3: `HandleReconnect` proceeds normally when RR exists and is active
- F-4: `HandleReconnect` skips validation when `k8sClient == nil` (fail-open)
- F-5: `HandleReconnect` skips validation when `namespace == ""` (fail-open)
- F-6: Full user journey — stale context after pod restart yields fresh conversation

## 4. Features Not to be Tested

- SessionInterceptor behavior (not modified in final implementation)
- ADK `AutoCreateSession` internals (third-party library)
- LLM agent behavior after session_expired response (LLM tests are separate)
- Session hydration from CRDs (deferred to #1451)

## 5. Approach

### Test Pyramid

| Tier | Scope | Count |
|---|---|---|
| Unit | `HandleReconnect` RR validation logic with fake K8s client | 6 |
| E2E | Full journey: session → AF pod restart → stale context_id → fresh conversation | 1 |

### FedRAMP Control Mapping

| Control | Objective | Behavioral Assurance | Test IDs |
|---|---|---|---|
| SC-7 | Boundary protection | Reconnect to non-existent RR is rejected at tool boundary | UT-AF-1472-001 |
| SC-10 | Network disconnect | Post-restart stale sessions produce clean rejection rather than silent reactivation | UT-AF-1472-001, UT-AF-1472-002 |
| SI-10 | Information input validation | Invalid RRID (non-existent or terminal) is rejected before reaching agent logic | UT-AF-1472-001, UT-AF-1472-002, UT-AF-1472-006 |
| SC-5 | DoS protection | Nil client or empty namespace skips validation (fail-open, availability preserved) | UT-AF-1472-004, UT-AF-1472-005 |
| AU-3 | Content of audit records | Rejection logged with RRID and phase for SRE observability | UT-AF-1472-001, UT-AF-1472-002 |

## 6. Test Cases

### 6.1 Unit Tests — HandleReconnect RR Validation

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| UT-AF-1472-001 | Non-existent RR returns session_expired | `HandleReconnect` with fake client returning NotFound → result has `Status: "session_expired"` | SC-7, SI-10, AU-3 |
| UT-AF-1472-002 | Terminal RR (Completed) returns session_expired | `HandleReconnect` with fake client returning RR in `Completed` phase → result has `Status: "session_expired"` | SC-10, SI-10, AU-3 |
| UT-AF-1472-003 | Active RR proceeds normally | `HandleReconnect` with fake client returning RR in `Investigating` phase → result does NOT have `Status: "session_expired"` | — |
| UT-AF-1472-004 | Nil K8s client skips validation (fail-open) | `HandleReconnect` with `k8sClient == nil` → validation skipped, no panic, proceeds to reconnection logic | SC-5 |
| UT-AF-1472-005 | Empty namespace skips validation (fail-open) | `HandleReconnect` with `namespace == ""` → validation skipped, proceeds to reconnection logic | SC-5 |
| UT-AF-1472-006 | Failed phase RR returns session_expired | `HandleReconnect` with fake client returning RR in `Failed` phase → result has `Status: "session_expired"` | SI-10 |

### 6.2 E2E Tests — Full User Journey

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| E2E-AF-1472-001 | Stale context_id after pod restart yields fresh conversation | Session established → AF pod killed → pod restarts → same context_id sent → valid response (fresh conversation, no "reconnecting") | SC-7, SC-10, SI-10 |

## 7. Test Environment

### Unit Tests
- Fake K8s client (`fake.NewClientBuilder().WithObjects(...)`) from controller-runtime
- Existing test helpers from `pkg/apifrontend/tools/kubernaut_list_remediations_test.go` (`newTypedFakeClient`, `newTypedRR`)
- Ginkgo/Gomega BDD framework

### E2E Tests
- Kind cluster with full Kubernaut deployment
- Real AF pod restart (pod deletion + wait for ready)
- Real A2A client sending messages with stale context_id

## 8. Pass/Fail Criteria

- All 7 test cases pass (6 UT + 1 E2E)
- Zero regressions in existing `ka_interactive_test.go` and `mcp_bridge_integration_test.go`
- `go build ./...` clean
- `golangci-lint run --timeout=5m` clean

## 9. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| K8s API call adds latency to reconnect path | Low | Low | Only triggered on explicit reconnect tool calls (not every message); K8s Get is fast for single objects |
| Fail-open policy masks persistent K8s connectivity issues | Low | Medium | Structured logging on skip; SRE alerting on repeated fail-opens |
| RR phase transitions during reconnect attempt | Very Low | Low | Eventual consistency acceptable — user can retry; no data corruption |
