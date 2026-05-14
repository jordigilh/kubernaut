# Test Plan: Token-Level Streaming in runLLMLoop

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-823-TOKEN-v1.0
**Feature**: Use StreamChat in runLLMLoop when event sink is present, Chat when not
**Version**: 1.1
**Created**: 2026-04-24
**Author**: AI Assistant + Jordi Gil
**Status**: Active
**Branch**: `feature/pr6-token-streaming`

---

## 1. Introduction

### 1.1 Purpose

Validate that operators observing an active investigation receive token-by-token
LLM reasoning in real time, while autonomous investigations without observers
behave identically to v1.4. This is the capstone PR that completes the SSE
streaming pipeline.

### 1.2 Objectives

1. **Real-time observation** (BR-SESSION-003): When an operator is observing, they receive each LLM text fragment as it is generated — faithfully reproducing the LLM output in order
2. **Ordering guarantee** (BR-SESSION-003): Token-level deltas arrive BEFORE the turn-level reasoning summary for the same turn
3. **Autonomous regression** (BR-SESSION-003): Without an observer, the investigation produces identical results to v1.4
4. **Resilience** (BR-SESSION-003): A slow or disconnected observer cannot block the autonomous investigation
5. **Cancellation parity** (BR-SESSION-001): Cancellation during streaming yields the same outcome as non-streaming cancellation
6. **Runtime agnosticism** (BR-SESSION-007): EventTypeTokenDelta has a stable wire value mapped to Goose ACP

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/investigator/ --ginkgo.focus="PR6"` |
| Backward compatibility | 0 regressions | All existing tests pass |
| Coverage | >=80% of `chatOrStream` | `go test -coverprofile` |

---

## 2. References

### 2.1 Authority

- **BR-SESSION-003**: Real-time streaming of investigation to observer — operator can observe autonomous investigation progress token-by-token
- **BR-SESSION-001**: Operator can cancel an autonomous investigation; cancellation behavior identical with and without streaming
- **BR-SESSION-007**: Event types are runtime-agnostic, providing a stable SSE contract across runtime migrations (LangChainGo → Goose ACP)
- TP-823-TURN-EVENTS-SSE.md (PR4), TP-823-STREAMCHAT.md (PR5)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| R1 | StreamChat callback error disrupts investigation | Investigation stops mid-turn | Medium | Callback always returns nil (non-blocking send); StreamChat errors handled same as Chat errors |
| R2 | Token deltas overwhelm event channel buffer | Events dropped for slow consumers | Low | Non-blocking send (select/default); documented as acceptable degradation |
| R3 | StreamChat changes response structure vs Chat | Audit events record different data | Low | Both return identical ChatResponse; same downstream code path |
| R4 | Token delta ordering broken by concurrent events | Observer sees garbled output | Low | Events emitted sequentially within single-goroutine runLLMLoop |

---

## 4. Scope

### 4.1 Features to be Tested

- **Operator observation fidelity**: Token deltas faithfully reproduce LLM output fragments in order
- **Autonomous regression**: No behavioral change when no observer is present
- **Observation resilience**: Slow consumer does not block investigation
- **Cancellation parity**: Streaming cancellation equivalent to non-streaming
- **Event ordering**: Token deltas precede turn-level reasoning summaries
- **Runtime agnosticism**: Wire value stability for Goose migration

### 4.2 Features Not to be Tested

- StreamChat adapter implementations (covered in PR5 / TP-823-STREAMCHAT)
- SSE pipe delivery to HTTP clients (covered in PR4 deferred items)
- Turn-level events (covered in PR4 / TP-823-TURN-EVENTS-SSE)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Helper function `chatOrStream` encapsulates the conditional | Single change point; easy to test; clear separation of concerns |
| `EventTypeTokenDelta` distinct from `EventTypeReasoningDelta` | Token-level = character fragments for real-time display; reasoning = turn summary for UI state. Different consumers filter differently. |
| StreamChat callback always returns nil | Observation path must never disrupt investigation — fire-and-forget semantics per ADR-038 |

---

## 5. Test Scenarios

### 5.1 Business Requirement Traceability

| BR ID | Business Outcome | Priority | Tier | Test ID | Status |
|-------|------------------|----------|------|---------|--------|
| BR-SESSION-003 | Operator receives exact LLM text fragments in order | P0 | Unit | UT-KA-823-T01 | Pass |
| BR-SESSION-003 | Autonomous investigation without observer produces identical results | P0 | Unit | UT-KA-823-T02 | Pass |
| BR-SESSION-003 | Slow observer cannot block autonomous investigation | P0 | Unit | UT-KA-823-T03 | Pass |
| BR-SESSION-001, BR-SESSION-003 | Cancellation during streaming yields same outcome | P0 | Unit | UT-KA-823-T04 | Pass |
| BR-SESSION-003 | Token deltas arrive before turn summary (real-time ordering) | P0 | Unit | UT-KA-823-T05 | Pass |
| BR-SESSION-007 | EventTypeTokenDelta wire value is runtime-agnostic | P1 | Unit | UT-KA-823-T06 | Pass |

### 5.2 Business Outcome Quality Bar

Each test asserts an operator-visible behavior or business invariant, not an
implementation detail. Tests verify:
- **What the operator sees** (event content, ordering, fidelity)
- **What the operator's action causes** (cancellation outcome)
- **What remains unchanged** (autonomous mode regression)

---

## 6. Execution

```bash
go test ./test/unit/kubernautagent/investigator/ --ginkgo.focus="PR6" -v
go test -race ./test/unit/kubernautagent/investigator/ ./test/unit/kubernautagent/session/
```

---

## 7. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan |
| 1.1 | 2026-04-24 | Rewritten tests for business-outcome behavioral assurance: added content fidelity (T01), ordering guarantee (T05), cancellation parity (T04), wire value stability (T06). Removed implementation-detail assertions (call counters). |
