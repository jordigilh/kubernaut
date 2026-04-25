# Test Plan: StreamChat on llm.Client Interface

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-823-STREAMCHAT-v1.0
**Feature**: Add StreamChat method to llm.Client interface and implement in all adapters/wrappers
**Version**: 1.0
**Created**: 2026-04-24
**Author**: AI Assistant + Jordi Gil
**Status**: Active
**Branch**: `feature/pr5-streamchat-interface`

---

## 1. Introduction

### 1.1 Purpose

Validate that the `StreamChat` method is correctly added to the `llm.Client` interface
and implemented in all adapters (LangChainGo, VertexAnthropic) and wrappers (Swappable,
Instrumented, LLMProxy). This is pure infrastructure — no changes to `runLLMLoop` yet.

### 1.2 Objectives

1. **Interface extension**: `StreamChat` added to `llm.Client` with `ChatStreamEvent` callback
2. **VertexAnthropic adapter**: Streaming via Anthropic SDK `Messages.NewStreaming`
3. **LangChainGo adapter**: Streaming via `llms.WithStreamingFunc`
4. **Wrappers**: SwappableClient, InstrumentedClient, LLMProxy delegate correctly
5. **Backward compatibility**: `Chat` is unchanged; all existing tests still pass
6. **Test mocks**: All test mocks updated with `StreamChat` stub

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/llm/...` |
| Backward compatibility | 0 regressions | All existing tests pass |
| Interface compliance | All 5 implementations satisfy Client | Compile-time checks |

---

## 2. References

### 2.1 Authority

- BR-SESSION-003: Real-time streaming of investigation to observer
- DD-HAPI-019: Framework Isolation Pattern (Client interface)
- Issue #823: Session streaming and cancellation

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| R1 | Interface change breaks all implementations and mocks | Compile errors across codebase | Certain | Update all 5 implementations + test mocks in a single atomic commit |
| R2 | LangChainGo `StreamingFunc` behavior varies by provider | Incorrect event mapping | Medium | Test with mock `llms.Model`; document per-provider behavior |
| R3 | Anthropic SDK streaming API changes | Compile error | Low | Pin SDK version; test with mock stream |
| R4 | Investigator snapshot pin doesn't include StreamChat | StreamChat panics at runtime | Medium | InstrumentedClient + SwappableClient both implement StreamChat |

---

## 4. Scope

### 4.1 Features to be Tested

- **`llm.Client` interface** (`pkg/kubernautagent/llm/types.go`): `StreamChat` method, `ChatStreamEvent`, `PartialToolCall` types
- **`vertexanthropic.Client`** (`pkg/kubernautagent/llm/vertexanthropic/client.go`): `StreamChat` implementation
- **`langchaingo.Adapter`** (`pkg/kubernautagent/llm/langchaingo/adapter.go`): `StreamChat` implementation
- **`SwappableClient`** (`pkg/kubernautagent/llm/swappable_client.go`): `StreamChat` delegation
- **`InstrumentedClient`** (`pkg/kubernautagent/llm/instrumented_client.go`): `StreamChat` with metrics
- **`alignment.LLMProxy`** (`internal/kubernautagent/alignment/llmproxy.go`): `StreamChat` delegation

### 4.2 Features Not to be Tested

- **`runLLMLoop` integration**: Deferred to PR6
- **Real LLM streaming over network**: Unit tests use mocks; real provider tests are E2E

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `StreamChat` returns final `ChatResponse` like `Chat` | Callers get the same aggregated result; streaming is additive, not replacing |
| Callback `func(ChatStreamEvent) error` | Allows caller to abort stream by returning error; matches LangChainGo pattern |
| `ChatStreamEvent.Done` field | Signals final event; mirrors SSE `complete` event type |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of new `StreamChat` code in all implementations
- Existing `Chat` coverage must not decrease

### 5.2 Pass/Fail Criteria

**PASS**: All tests pass, all implementations compile against `Client` interface, 0 regressions.
**FAIL**: Any compile error, test failure, or regression.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/llm/types.go` | `ChatStreamEvent`, `PartialToolCall` types | ~15 |
| `pkg/kubernautagent/llm/vertexanthropic/client.go` | `StreamChat` | ~35 |
| `pkg/kubernautagent/llm/langchaingo/adapter.go` | `StreamChat` | ~25 |
| `pkg/kubernautagent/llm/swappable_client.go` | `StreamChat` | ~5 |
| `pkg/kubernautagent/llm/instrumented_client.go` | `StreamChat` | ~15 |
| `internal/kubernautagent/alignment/llmproxy.go` | `StreamChat` | ~10 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SESSION-003 | StreamChat produces ChatStreamEvents with text deltas | P0 | Unit | UT-KA-823-SC01 | Pass |
| BR-SESSION-003 | StreamChat returns aggregated ChatResponse identical to Chat | P0 | Unit | UT-KA-823-SC02 | Pass |
| BR-SESSION-003 | StreamChat callback error aborts the stream | P0 | Unit | UT-KA-823-SC03 | Pass |
| BR-SESSION-003 | StreamChat context cancellation propagates | P0 | Unit | UT-KA-823-SC04 | Pass |
| BR-SESSION-003 | SwappableClient.StreamChat delegates to inner | P1 | Unit | UT-KA-823-SC05 | Pass |
| BR-SESSION-003 | InstrumentedClient.StreamChat records metrics | P1 | Unit | UT-KA-823-SC06 | Pass |
| BR-SESSION-003 | LLMProxy.StreamChat delegates and submits alignment | P1 | Unit | UT-KA-823-SC07 | Pass |
| BR-SESSION-003 | All 5 implementations satisfy Client interface | P0 | Unit | UT-KA-823-SC08 | Pass |
| BR-SESSION-003 | Existing Chat tests pass unchanged (regression guard) | P0 | Unit | UT-KA-823-SC09 | Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-823-SC01 | StreamChat produces ChatStreamEvents with text deltas for each chunk | Pending |
| UT-KA-823-SC02 | StreamChat returns aggregated ChatResponse with correct usage and tool calls | Pending |
| UT-KA-823-SC03 | Callback returning error aborts stream and propagates error | Pending |
| UT-KA-823-SC04 | Context cancellation mid-stream returns context error | Pending |
| UT-KA-823-SC05 | SwappableClient.StreamChat delegates to current inner under RLock | Pending |
| UT-KA-823-SC06 | InstrumentedClient.StreamChat records duration and token metrics | Pending |
| UT-KA-823-SC07 | LLMProxy.StreamChat delegates to inner and submits alignment step | Pending |
| UT-KA-823-SC08 | Compile-time interface satisfaction for all 5 implementations | Pending |
| UT-KA-823-SC09 | All existing Chat tests pass unchanged | Pending |

---

## 9. Execution

```bash
go test ./test/unit/kubernautagent/llm/... -ginkgo.v
go test ./test/unit/kubernautagent/investigator/ -ginkgo.v
go test -race ./test/unit/kubernautagent/llm/...
```

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan |
