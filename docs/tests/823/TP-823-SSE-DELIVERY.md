# Test Plan: SSE Delivery Path (PR7)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-823-SSE-DELIVERY-v1.0
**Feature**: Wire SSE stream handler, lazy event sink, panic recovery, event completeness
**Version**: 1.0
**Created**: 2026-04-24
**Author**: AI Assistant + Jordi Gil
**Status**: Draft
**Branch**: `feature/pr7-sse-delivery`

---

## 1. Introduction

### 1.1 Purpose

Close the critical delivery gap: the v1.5 streaming pipeline (PRs 1-6) built
infrastructure but never connected it to HTTP. This PR wires the SSE stream
handler, makes the event sink lazy (restoring v1.4 autonomous behavior), adds
panic recovery in the investigation goroutine, and emits missing lifecycle events.

### 1.2 Objectives

1. **SSE delivery** (GAP-1/2/3): Operators can GET `/api/v1/incident/session/{id}/stream` and receive SSE frames with investigation events
2. **Lazy event sink** (GAP-4): Autonomous investigations without observers use `Chat` (v1.4 parity); sink attached only on `Subscribe`
3. **Panic recovery** (COR-1): Investigation goroutine recovers from panics, transitions session to `StatusFailed`
4. **Event completeness** (GAP-5/6): `EventTypeComplete` emitted at end; `EventTypeSessionObserved` emitted on subscribe
5. **Regression**: All existing tests pass unchanged

---

## 2. References

### 2.1 Authority

- BR-SESSION-003: Real-time streaming of investigation to observer
- BR-SESSION-005: All session control actions are audited
- BR-SESSION-007: Runtime-agnostic event types
- SOC2 CC8.1: Operator attribution for observation
- Gap audit: v1.5_gap_remediation_5a29c0a8.plan.md

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Mitigation |
|----|------|--------|------------|
| R1 | `io.Pipe` blocks if consumer disconnects | Investigation goroutine blocked | Write to pipe in separate goroutine; close pipe on context cancel |
| R2 | Lazy sink changes Subscribe semantics | Existing integration tests break | Subscribe triggers sink attachment atomically |
| R3 | Panic recovery masks bugs | Errors hidden | Log at Error level with stack trace; transition to StatusFailed |
| R4 | ogen encoder sets Content-Type but not proxy headers | Buffered by nginx/envoy | SSEHeadersMiddleware mounted on stream route |

---

## 4. Scope

### 4.1 Features to be Tested

- **SSE stream handler** (`handler.go`): `SessionStream...Get` using `io.Pipe` + `Manager.Subscribe`
- **Lazy event sink** (`manager.go`): Sink attached only on first `Subscribe`, not on `StartInvestigation`
- **Panic recovery** (`manager.go`): `recover()` in investigation goroutine
- **EventTypeComplete emission** (`investigator.go`): At investigation end
- **EventTypeSessionObserved emission** (`manager.go`): On `Subscribe`

### 4.2 Features Not to be Tested

- StreamChat adapters (PR5)
- Token-level streaming in runLLMLoop (PR6)
- Cancel/snapshot handlers (PR2)
- DataStorage typed payloads (PR8)

---

## 5. Test Scenarios

### 5.1 Business Requirement Traceability

| BR ID | Business Outcome | Priority | Tier | Test ID | Status |
|-------|------------------|----------|------|---------|--------|
| BR-SESSION-003 | SSE stream delivers investigation events to HTTP client | P0 | Unit | UT-KA-823-D01 | Pending |
| BR-SESSION-003 | Stream ends when investigation completes (pipe closed) | P0 | Unit | UT-KA-823-D02 | Pending |
| BR-SESSION-003 | Stream for unknown session returns 404 | P0 | Unit | UT-KA-823-D03 | Pending |
| BR-SESSION-003 | Stream for terminal session returns 409 | P0 | Unit | UT-KA-823-D04 | Pending |
| BR-SESSION-003 | Lazy sink: autonomous investigation uses Chat (no sink) | P0 | Unit | UT-KA-823-D05 | Pending |
| BR-SESSION-003 | Lazy sink: Subscribe triggers sink attachment, events flow | P0 | Integration | IT-KA-823-D01 | Pending |
| COR-1 | Panic in investigation: session transitions to Failed, goroutine exits | P0 | Unit | UT-KA-823-D06 | Pending |
| BR-SESSION-007 | EventTypeComplete emitted when investigation ends | P1 | Unit | UT-KA-823-D07 | Pending |
| BR-SESSION-005 | EventTypeSessionObserved emitted on Subscribe | P1 | Unit | UT-KA-823-D08 | Pending |
| BR-SESSION-003 | Client disconnect closes pipe without blocking investigation | P0 | Integration | IT-KA-823-D02 | Pending |

---

## 6. Execution

```bash
go test ./test/unit/kubernautagent/server/ --ginkgo.focus="PR7" -v
go test ./test/unit/kubernautagent/session/ --ginkgo.focus="PR7" -v
go test -race ./test/unit/kubernautagent/... ./test/integration/kubernautagent/session/
```

---

## 7. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan |
