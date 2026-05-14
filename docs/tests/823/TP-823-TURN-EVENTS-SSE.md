# Test Plan: Turn-Level Event Emission + SSE Stream

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-823-SSE-v1.0
**Feature**: Turn-level event emission from runLLMLoop, SSE stream handler, auto-flush middleware, session observed audit event
**Version**: 1.0
**Created**: 2026-04-24
**Author**: AI Assistant + Jordi Gil
**Status**: Active
**Branch**: `feature/pr4-turn-events-sse`

---

## 1. Introduction

### 1.1 Purpose

Validate that turn-level investigation events are emitted from `runLLMLoop` to the
session event channel, that the SSE stream endpoint delivers these events to HTTP
clients in real time with correct framing and flush semantics, and that stream
subscription is audited for SOC2 compliance. This PR completes the "Observer mode"
milestone: operators can watch an autonomous investigation in real time and cancel at
any point.

### 1.2 Objectives

1. **Event emission**: `runLLMLoop` emits structured `InvestigationEvent`s to the
   context-carried event sink at each turn boundary and tool execution point.
2. **Non-blocking**: Event emission never blocks the investigation, even with a slow
   consumer (non-blocking send with `select`/`default`).
3. **SSE delivery**: The stream endpoint delivers events as `text/event-stream` with
   correct SSE framing (`id: {seq}\nevent: {type}\ndata: {json}\n\n`).
4. **Flush**: Each SSE event is flushed to the client immediately (no buffering).
5. **Lifecycle**: The stream ends cleanly when the investigation completes (channel
   closed) or the client disconnects (`ctx.Done()`).
6. **Audit**: Stream subscription emits `aiagent.session.observed` audit event.
7. **Regression**: Nil event sink = zero events emitted = identical v1.4 behavior.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on new event emission code |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority

- BR-SESSION-003: Operator can observe autonomous investigation progress in real time (turn-level)
- BR-SESSION-005: All session control actions are audited (cancel, observe)
- BR-SESSION-007: Streaming event types are runtime-agnostic (Goose compatibility)
- ADR-038: Async buffered audit ingestion (fire-and-forget)
- Issue #823: Session streaming and cancellation

### 2.2 Cross-References

- [TP-823-CANCELLED-RESULT.md](TP-823-CANCELLED-RESULT.md) — PR3 test plan (cancellation propagation)
- [TP-823-AUDIT.md](TP-823-AUDIT.md) — PR1.5 test plan (session audit trail)
- [conversation/sse.go](../../../internal/kubernautagent/conversation/sse.go) — Existing SSE pattern
- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) — Audit event registry

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | ogen `io.Copy` blocks until reader EOF — no per-event delivery | SSE events buffered until stream ends, defeating purpose | High | UT-KA-823-S01, IT-KA-823-S01 | Use `io.Pipe()`: PipeWriter writes SSE frames, PipeReader is passed as ogen response Data. Auto-flush middleware wraps ResponseWriter. |
| R2 | Non-blocking send drops events under load | Observer misses investigation events | Medium | UT-KA-823-S04 | Buffer of 64 events. Non-blocking send with `select`/`default`. Document in API: events are best-effort for observer mode. |
| R3 | Event sink not wired into investigation context | No events emitted even with subscriber | High | IT-KA-823-S01, IT-KA-823-S02 | Wire `WithEventSink(bgCtx, eventChan)` in `StartInvestigation` goroutine. |
| R4 | Reverse proxies (nginx, envoy) buffer SSE stream | Client receives delayed events | Medium | UT-KA-823-S05 | Set `Cache-Control: no-cache`, `X-Accel-Buffering: no` via middleware, matching conversation handler pattern. |
| R5 | Client disconnect not detected — goroutine leak | PipeWriter goroutine runs forever | High | UT-KA-823-S06 | PipeWriter goroutine selects on both event channel and `ctx.Done()`. On disconnect, close pipe. |
| R6 | Race between channel close and SSE write | Panic writing to closed pipe | Medium | UT-KA-823-S06 | PipeWriter goroutine owns the pipe lifecycle. Channel range-loop exits cleanly on close. |
| R7 | Existing test UT-KA-823-OAS-008 expects 501 for stream | Test fails when real implementation lands | Low | UT-KA-823-OAS-008 | Update test to expect 200 with SSE content type for running sessions. |
| R8 | Event emission adds latency to investigation loop | Investigation slower with observer | Low | UT-KA-823-S04 | Non-blocking send with `select`/`default` adds ~nanoseconds. No measurable impact. |

---

## 4. Scope

### 4.1 Features to be Tested

- **Event emission** (`investigator/investigator.go`): `runLLMLoop` emits `InvestigationEvent` to context-carried event sink at turn boundaries and tool execution points
- **Event sink wiring** (`session/manager.go`): `StartInvestigation` attaches event channel to context via `WithEventSink`
- **SSE stream handler** (`server/handler.go`): Implements `SessionStreamAPIV1IncidentSessionSessionIDStreamGet` with `io.Pipe()` + SSE framing
- **Auto-flush middleware** (`server/sse_middleware.go`): Wraps `http.ResponseWriter` to flush after each write for SSE endpoints
- **Audit event** (`audit/emitter.go`): `EventTypeSessionObserved` emitted on stream subscription
- **Non-blocking send**: Event emission uses `select`/`default` to avoid blocking investigation

### 4.2 Features Not to be Tested

- **Token-level streaming**: Deferred to PR5/PR6 (`StreamChat` not yet on `llm.Client`)
- **Last-Event-ID reconnection**: Not implemented for investigation stream (events are transient turn-level data, not durable)
- **SSE ring buffer**: Conversation SSE uses `SSEWriter` with `ReplayFrom`; investigation stream does not need reconnection support

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `io.Pipe()` for ogen SSE response | ogen expects `io.Reader` for `text/event-stream`. `PipeReader` blocks on `Read()` until `PipeWriter` writes, providing natural backpressure. `io.Copy` in the encoder reads from the pipe and writes to `ResponseWriter`. |
| Auto-flush middleware instead of manual flush | ogen's encoder does `io.Copy` — we can't call `Flush()` between events from the handler. Middleware wraps `ResponseWriter` so every `Write()` is followed by `Flush()`. |
| Non-blocking send (`select`/`default`) | Investigation must never stall waiting for a slow SSE consumer. Events are best-effort for observers. Buffer of 64 provides headroom. |
| Proxy anti-buffering headers via middleware | Matches conversation handler pattern (lines 268-271). Applied to stream endpoint only to avoid overhead on non-SSE endpoints. |
| Event sink wired in `StartInvestigation`, not in handler | The event channel is created by the manager at investigation start. The sink should be attached to the context at the same point, ensuring every investigation has the plumbing even if no subscriber attaches immediately. |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of event emission logic, SSE framing, middleware
- **Integration**: >=80% of full stream flow (manager → investigator → event → pipe → HTTP)

### 5.2 Quality Bar

Tests validate **business outcomes**: "operator sees investigation events in real time" not "function X is called."

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass, >=80% coverage per tier, 0 regressions.
**FAIL**: Any P0 failure, coverage below 80%, or regression.

### 5.4 Suspension Criteria

Suspend if: build broken, CI red on `development/v1.5`, or blocking dependency unresolved.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | Event emission in `runLLMLoop`: `emitToSink` helper, turn-start/response/tool-start/tool-result/cancelled events | ~40 |
| `internal/kubernautagent/server/sse_middleware.go` (new) | `autoFlushWriter`, `SSEHeadersMiddleware` | ~30 |
| `internal/kubernautagent/audit/emitter.go` | `EventTypeSessionObserved`, `ActionSessionObserved` | ~5 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/server/handler.go` | `SessionStreamAPIV1IncidentSessionSessionIDStreamGet`: pipe creation, goroutine, SSE framing, client disconnect | ~60 |
| `internal/kubernautagent/session/manager.go` | `StartInvestigation`: `WithEventSink(bgCtx, eventChan)` wiring | ~5 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/pr4-turn-events-sse` HEAD | Branched from `development/v1.5` |
| Dependency: PR1-3 | Merged to `development/v1.5` | Session infra, audit, OAS, cancellation |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SESSION-003 | Turn-level events emitted from runLLMLoop to event sink | P0 | Unit | UT-KA-823-S01 | Pass |
| BR-SESSION-003 | LLM response event includes turn, phase, content preview | P0 | Unit | UT-KA-823-S02 | Pass |
| BR-SESSION-003 | Tool call events emitted before and after execution | P0 | Unit | UT-KA-823-S03 | Pass |
| BR-SESSION-003 | Non-blocking send: full channel does not block investigation | P0 | Unit | UT-KA-823-S04 | Pass |
| BR-SESSION-003 | SSE proxy headers set by middleware | P1 | Unit | UT-KA-823-S05 | Pass |
| BR-SESSION-003 | Auto-flush middleware flushes after each write | P1 | Unit | UT-KA-823-S06 | Pass |
| BR-SESSION-003 | Nil event sink = zero events emitted (regression guard) | P0 | Unit | UT-KA-823-S07 | Pass |
| BR-SESSION-003 | Cancelled event emitted on cancellation | P0 | Unit | UT-KA-823-S08 | Pass |
| BR-SESSION-005 | Stream subscription emits audit event | P1 | Unit | UT-KA-823-S09 | Deferred (SSE handler impl) |
| BR-SESSION-003 | SSE stream handler delivers events via io.Pipe | P0 | Integration | IT-KA-823-S01 | Pass |
| BR-SESSION-003 | Stream ends when investigation completes (channel closed) | P0 | Integration | IT-KA-823-S02 | Pass |
| BR-SESSION-003 | Stream ends when client disconnects | P0 | Integration | IT-KA-823-S03 | Deferred (SSE handler impl) |
| BR-SESSION-003 | Event sink wired in StartInvestigation context | P0 | Integration | IT-KA-823-S04 | Pass |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-823-S{SEQUENCE}` (S for Stream/SSE)

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-823-S01 | Operator subscribes → investigation emits turn-level events (response received, phase, turn number) to the event sink during normal multi-turn investigation | Pass |
| UT-KA-823-S02 | Each LLM response event carries structured data: turn number, phase, content preview, tool call count | Pass |
| UT-KA-823-S03 | Tool execution produces two events per tool call: tool_call_start (name, args) and tool_call_result (name, result preview) | Pass |
| UT-KA-823-S04 | Full event channel (buffer exhausted) does NOT block investigation — events are dropped silently | Pass |
| UT-KA-823-S05 | SSE middleware sets Cache-Control, Connection, X-Accel-Buffering headers on response | Pass |
| UT-KA-823-S06 | Auto-flush middleware calls Flush() after each Write() | Pass |
| UT-KA-823-S07 | No event sink on context → zero events emitted, investigation completes identically to v1.4 | Pass |
| UT-KA-823-S08 | Context cancelled → EventTypeCancelled emitted to sink before CancelledResult is returned | Pass |
| UT-KA-823-S09 | Stream subscription emits `aiagent.session.observed` audit event with session ID and user identity | Deferred |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-KA-823-S01 | Full flow: start investigation with subscriber → events arrive via event channel → investigation completes | Pass |
| IT-KA-823-S02 | Investigation completes → event channel closed → subscriber detects closure | Pass |
| IT-KA-823-S03 | Client disconnects mid-stream → handler detects via ctx.Done() → pipe closed cleanly → no goroutine leak | Deferred (SSE handler impl) |
| IT-KA-823-S04 | Event sink is wired into investigation context by StartInvestigation → investigator receives events via EventSinkFromContext | Pass |

### Tier Skip Rationale

- **E2E**: SSE delivery over real HTTP requires Kind cluster or similar infrastructure. Deferred to E2E suite. Integration tests with `httptest.Server` provide equivalent confidence.

---

## 9. Environmental Needs

### 9.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Mock LLM client (existing `cancelAwareMockClient` pattern), mock `http.ResponseWriter` with `Flusher` interface
- **Location**: `test/unit/kubernautagent/investigator/`, `test/unit/kubernautagent/server/`

### 9.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: `httptest.Server` for real HTTP SSE delivery, `session.Manager` with real store
- **Location**: `test/integration/kubernautagent/session/`, `test/integration/kubernautagent/server/`

---

## 10. Execution Order (TDD Phases)

### Phase 1: RED — Failing Tests

1. Unit tests for event emission (UT-KA-823-S01 through S04, S07, S08)
2. Unit tests for middleware (UT-KA-823-S05, S06)
3. Unit test for audit (UT-KA-823-S09)
4. Integration tests for SSE pipe flow (IT-KA-823-S01 through S04)

### Phase 2: GREEN — Minimal Implementation

1. `emitToSink` helper in `investigator.go`
2. Event emission at 5 points in `runLLMLoop`
3. `WithEventSink` wiring in `StartInvestigation`
4. `autoFlushWriter` + `SSEHeadersMiddleware` in `server/sse_middleware.go`
5. `SessionStreamAPIV1IncidentSessionSessionIDStreamGet` with `io.Pipe()` in `handler.go`
6. `EventTypeSessionObserved` audit event in `emitter.go`
7. Update `UT-KA-823-OAS-008` (stream stub test)

### Phase 3: REFACTOR — Code Quality

1. GoDoc on all new functions
2. Extract SSE framing constants
3. Verify code duplication with conversation SSE pattern

---

## 11. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan |
