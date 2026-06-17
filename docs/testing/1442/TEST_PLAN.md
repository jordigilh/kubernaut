# Test Plan: Fix #1442 MCP Session State Lost

**Issue**: [#1442](https://github.com/jordigilh/kubernaut/issues/1442)
**Service Type**: [x] Stateless HTTP API (API Frontend + Kubernaut Agent)
**Date**: 2026-06-16
**Status**: Active

---

## Business Requirements

| BR ID | Description |
|---|---|
| BR-INTERACTIVE-001 | Interactive MCP sessions MUST persist across the full investigation lifecycle |
| BR-INTERACTIVE-008 | Session reconstruction MUST succeed after unplanned disconnect |
| BR-OPS-013 | Session lifecycle events MUST be auditable |

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-1442-{SEQUENCE}`

- `KA` = Kubernaut Agent
- `AF` = API Frontend

---

## Component 1: GracefulSessionClosedHandler (KA)

### Unit Tests

| ID | Component | Scenario | Expected Outcome |
|---|---|---|---|
| UT-KA-1442-001 | GracefulSessionClosedHandler | MCP disconnect starts grace timer instead of immediate release | onClose callback is NOT invoked during grace period; timer is stored in pendingRelease map |
| UT-KA-1442-002 | GracefulSessionClosedHandler | CancelPendingRelease cancels timer before expiry | Timer is stopped; onClose callback is never invoked; pending entry is removed |
| UT-KA-1442-003 | GracefulSessionClosedHandler | Timer expiry triggers release and reconstruction | After gracePeriod elapses, onClose callback is invoked with the MCP session ID |
| UT-KA-1442-004 | LeaseSessionManager | ReattachMCPSession maps new MCP session ID to existing interactive session | New MCP session ID is registered in the event store for the existing rrID's interactive session |
| UT-KA-1442-005 | GracefulSessionClosedHandler | Context cancellation stops Run loop and cancels pending timers | All pending timers are cancelled; Run returns cleanly |
| UT-KA-1442-006 | GracefulSessionClosedHandler | Multiple disconnects for different sessions are tracked independently | Each session gets its own timer; cancelling one does not affect others |

### Integration Tests

| ID | Component | Scenario | Expected Outcome |
|---|---|---|---|
| IT-KA-1442-001 | KA disconnect grace | MCP disconnect within grace period followed by reconnect preserves lease | EventStore.SessionClosed fires; GracefulHandler defers release; new session re-registers same rrID; lease remains active |
| IT-KA-1442-002 | KA reattach | ReattachMCPSession via production dispatch maps new MCP session | DelegatingEventStore shows new MCP session ID mapped to existing interactive session ID |
| IT-KA-1442-003 | KA grace expiry | Grace period expires without reconnect triggers full release | After grace period, onClose callback fires; interactive session is released |

---

## Component 2: InjectVerified (AF Session Pool)

### Unit Tests

| ID | Component | Scenario | Expected Outcome |
|---|---|---|---|
| UT-AF-1442-002 | KASessionPool.InjectVerified | Dead session rejected on inject (Ping fails) | InjectVerified returns error; session is closed; pool size unchanged |
| UT-AF-1442-003 | KASessionPool.InjectVerified | Live session accepted on inject (Ping succeeds) | InjectVerified returns nil; session is added to pool; Acquire returns injected session |

### Integration Tests

| ID | Component | Scenario | Expected Outcome |
|---|---|---|---|
| IT-AF-1442-001 | AF InjectVerified | Investigation handoff uses InjectVerified | Pool.Inject call site in ka_investigate_mcp.go replaced with InjectVerified; dead sessions are rejected at handoff |

---

## Component 3: AF Console-Facing MCP Session Audit

### Unit Tests

| ID | Component | Scenario | Expected Outcome |
|---|---|---|---|
| UT-AF-1442-001 | AF MCP handler audit | Console-facing MCP session close emits audit event | When EventStore.SessionClosed fires, audit.EventMCPSessionClosed is emitted with session ID |

---

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| GracefulSessionClosedHandler | `go disconnectHandler.Run(ctx)` | cmd/kubernautagent/main.go | IT-KA-1442-001 |
| ReattachMCPSession | `leaseMgr.ReattachMCPSession()` on reconnect | internal/kubernautagent/mcp/session_manager.go | IT-KA-1442-002 |
| CancelPendingRelease | `gracefulHandler.CancelPendingRelease()` on new session | cmd/kubernautagent/main.go | IT-KA-1442-003 |
| InjectVerified | `pool.InjectVerified()` in investigation handoff | pkg/apifrontend/tools/ka_investigate_mcp.go | IT-AF-1442-001 |
| AF MCP session close audit | `EventStore.SessionClosed` on console handler | pkg/apifrontend/handler/mcp.go | UT-AF-1442-001 |

---

## Test Execution Summary

| Test Category | Tests | Status |
|---|---|---|
| KA Unit Tests (UT-KA-1442-*) | 6 | Pending |
| KA Integration Tests (IT-KA-1442-*) | 3 | Pending |
| AF Unit Tests (UT-AF-1442-*) | 3 | Pending |
| AF Integration Tests (IT-AF-1442-*) | 1 | Pending |
| **Total** | **13** | **Pending** |
