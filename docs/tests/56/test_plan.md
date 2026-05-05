# Test Plan: CRD-backed Session Service, State Machine, TTL Controller, Re-invocation, SSE

**Test Plan Identifier:** TP-AF-056
**Issue:** PR4
**Version:** 1.0
**Date:** 2026-05-04
**Status:** Draft

---

## 1. Introduction

This test plan validates the CRD-backed `session.Service` implementation (delegate pattern wrapping ADK `InMemoryService`), the `InvestigationSession` state machine, the TTL cleanup controller, the re-invocation fallback, and SSE event formatting.

### 1.1 Scope

- CRD-backed `session.Service` (Create, Get, List, Delete, AppendEvent)
- Delegate pattern: `CRDSessionService` wraps `session.InMemoryService()`
- FunctionResponse trimming for etcd safety (4KB threshold)
- InvestigationSession phase transitions and state machine validation
- TTL cleanup controller (controller-runtime reconciler)
- Re-invocation fallback detection and synthetic message generation
- SSE event formatting (frame serialization, heartbeat, event type mapping)

### 1.2 References

- ADR-005: Session persistence via InvestigationSession CRD
- ADR-017: CRD PII classification and data retention
- ARCHITECTURE.md Section 4: CRD Design (InvestigationSession)
- ARCHITECTURE.md Section 7: Observability (af_sessions_active metric)
- `google.golang.org/adk@v1.2.0/session` -- Service and Session interfaces

---

## 2. Test Items

| Item | Package | Version |
|------|---------|---------|
| `CRDSessionService` | `internal/session` | PR4 |
| `TrimToolResult` | `internal/session` | PR4 |
| `ValidateTransition` | `internal/session` | PR4 |
| `UpdatePhase` | `internal/session` | PR4 |
| `GetSessionPhase` | `internal/session` | PR4 |
| `SessionCleanupReconciler` | `internal/controller` | PR4 |
| `NeedsReinvocation` | `internal/session` | PR4 |
| `SyntheticMessage` | `internal/session` | PR4 |
| `FormatSSEFrame` | `internal/streaming` | PR4 |
| `EventTypeFromEvent` | `internal/streaming` | PR4 |
| `HeartbeatFrame` | `internal/streaming` | PR4 |

---

## 3. Test Cases

### 3.1 CRD-backed session.Service (22 tests)

| ID | Description | Priority |
|----|-------------|----------|
| UT-AF-200-001 | Create returns session with generated ID | P0 |
| UT-AF-200-002 | Create creates InvestigationSession CRD with ownerRef | P0 |
| UT-AF-200-003 | Create sets standard labels | P0 |
| UT-AF-200-004 | Create with client-provided SessionID uses it | P1 |
| UT-AF-200-005 | Create populates CRD spec from request state | P0 |
| UT-AF-200-006 | Create rolls back CRD if delegate fails | P1 |
| UT-AF-201-001 | Get returns session by AppName+UserID+SessionID | P0 |
| UT-AF-201-002 | Get returns error when session not found | P0 |
| UT-AF-201-003 | Get with NumRecentEvents returns filtered events | P1 |
| UT-AF-201-004 | Get after service restart returns error | P1 |
| UT-AF-202-001 | List returns all sessions for user | P0 |
| UT-AF-202-002 | List returns empty when no sessions exist | P1 |
| UT-AF-202-003 | List filters by AppName and UserID | P1 |
| UT-AF-203-001 | Delete removes CRD and delegate state | P0 |
| UT-AF-203-002 | Delete returns error when session not found | P1 |
| UT-AF-203-003 | Delete removes CRD even if delegate has no state | P1 |
| UT-AF-204-001 | AppendEvent stores event in delegate | P0 |
| UT-AF-204-002 | AppendEvent skips partial events | P0 |
| UT-AF-204-003 | AppendEvent strips temp: keys | P0 |
| UT-AF-204-004 | AppendEvent trims large FunctionResponse | P0 |
| UT-AF-204-005 | AppendEvent updates CRD status lastUpdateTime | P1 |
| UT-AF-204-006 | AppendEvent preserves user messages and final responses | P0 |

### 3.2 State Machine (10 tests)

| ID | Description | Priority |
|----|-------------|----------|
| UT-AF-210-001 | Active -> Completed | P0 |
| UT-AF-210-002 | Active -> Cancelled | P0 |
| UT-AF-210-003 | Active -> Failed | P0 |
| UT-AF-210-004 | Active -> Disconnected | P0 |
| UT-AF-210-005 | Disconnected -> Active | P0 |
| UT-AF-210-006 | Terminal phase rejects transition | P0 |
| UT-AF-210-007 | Disconnected -> Cancelled | P0 |
| UT-AF-210-008 | UpdatePhase updates CRD status + timestamp | P0 |
| UT-AF-210-009 | UpdatePhase sets completedAt on terminal | P0 |
| UT-AF-210-010 | UpdatePhase sets disconnectedAt/reconnectedAt | P0 |

### 3.3 TTL Controller (10 tests)

| ID | Description | Priority |
|----|-------------|----------|
| UT-AF-220-001 | Disconnected -> Cancelled after TTL | P0 |
| UT-AF-220-002 | Completed deleted after retention | P0 |
| UT-AF-220-003 | Cancelled deleted after retention | P0 |
| UT-AF-220-004 | Failed deleted after retention | P0 |
| UT-AF-220-005 | Active session not touched | P0 |
| UT-AF-220-006 | Recent terminal not deleted | P0 |
| UT-AF-220-007 | Requeues with correct delay | P1 |
| UT-AF-220-008 | Updates af_sessions_active metric (deferred: gauge accuracy verified in session pkg tests) | P2 |
| UT-AF-220-009 | Handles CRD not found | P1 |
| UT-AF-220-010 | Zero TTL does not delete prematurely | P1 |

### 3.4 Re-invocation Fallback (8 tests)

| ID | Description | Priority |
|----|-------------|----------|
| UT-AF-230-001 | Detects text-only turn end during active investigation | P0 |
| UT-AF-230-002 | Does not trigger with tool calls | P0 |
| UT-AF-230-003 | Does not trigger when terminal | P0 |
| UT-AF-230-004 | Correct synthetic message | P0 |
| UT-AF-230-005 | Tracks reinvocation count | P0 |
| UT-AF-230-006 | Stops after max reinvocations | P0 |
| UT-AF-230-007 | Does not trigger when Disconnected | P1 |
| UT-AF-230-008 | Does not trigger with empty events | P1 |

### 3.5 SSE Event Formatting (8 tests)

| ID | Description | Priority |
|----|-------------|----------|
| UT-AF-240-001 | Formats ADK event as SSE frame | P0 |
| UT-AF-240-002 | Maps author to SSE event type | P0 |
| UT-AF-240-003 | Heartbeat comment frame | P0 |
| UT-AF-240-004 | Closes on terminal event | P0 |
| UT-AF-240-005 | Skips partial events | P0 |
| UT-AF-240-006 | LongRunningToolIDs as input-required | P0 |
| UT-AF-240-007 | Data field is valid JSON | P1 |
| UT-AF-240-008 | Event type has no newlines | P1 |

---

## 4. Pass/Fail Criteria

- All 58 tests pass with `-race` flag
- Coverage >= 80% per package
- `golangci-lint run` reports 0 errors
- No `panic()` in production code
- `go mod tidy` clean

## 5. Test Environment

- Go 1.25.6
- `sigs.k8s.io/controller-runtime/pkg/client/fake` for K8s API simulation
- `google.golang.org/adk/session.InMemoryService()` as delegate
- Ginkgo v2 + Gomega test framework (ADR-015)
