# Test Plan: Gemini Tool-Call Delta Streaming

**Test Plan Identifier**: TP-2439-v1
**Feature**: Preserve streamed Gemini tool-call argument deltas from the KA provider adapter through investigator session events and the AF A2A boundary.
**Version**: 1.0
**Created**: 2026-09-18
**Author**: Kubernaut maintainers
**Status**: Active (GREEN)
**Branch**: `fix/fleet-e2e-logger-init`

## 1. Introduction

### 1.1 Purpose

This plan verifies that Gemini function-call fragments remain observable while the
complete final `ChatResponse.ToolCalls` remains the sole authority for tool
execution and audit events. It covers native Gemini and the shared Vertex/native
adapter path, KA session delivery, retry safety, and AF structured relay.

### 1.2 Objectives

1. Verify fragmented tool-call deltas preserve index, identifier, name, and argument fragments.
2. Verify observers receive non-blocking `tool_call_delta` events with turn and phase metadata.
3. Verify partial deltas do not execute tools or emit tool-execution audit events.
4. Verify an attempt that emits a tool-call delta is not retried as if it produced no output.
5. Verify AF relays the structured payload without silently dropping fields.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Focused test pass rate | 100% | Affected package Ginkgo suites |
| Wiring coverage | 100% | Adapter -> investigator -> session -> AF scenarios |
| Regression rate | 0 | Existing affected package tests |
| Live provider dependency | 0 in mandatory CI | httptest/native adapter and mocks only |

## 2. References

### 2.1 Authority

- BR-AI-087: KA native Gemini provider support
- BR-SESSION-003: operator-visible streaming investigation output
- BR-AUDIT-005: complete remediation lifecycle reconstruction
- Issue #2439: Gemini streamed tool-call delta propagation
- DD-2439: Gemini tool-call delta propagation boundary

### 2.2 Cross-References

- `AGENTS.md`
- `docs/architecture/decisions/DD-KA-019-go-rewrite-design/DD-KA-019-go-rewrite-design.md`
- `docs/architecture/decisions/DD-LLM-010-eino-agenticgemini-for-ka-gemini-client.md`

## 3. Risks and Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Provider fragments are discarded by the adapter | Operators cannot observe or diagnose tool-call progress | High | UT-GM-2439-001 | Native Gemini httptest stream with multiple fragments |
| R2 | Partial fragments are treated as executable tool calls | Duplicate, malformed, or premature tool execution | High | UT-KA-2439-002 | Final response has no complete tool calls; assert no execution/audit |
| R3 | Mid-stream errors are retried after output was emitted | Duplicate/interleaved observer output | High | UT-KA-2439-003 | Retryable failure after a tool-call delta; assert one attempt |
| R4 | AF drops structured event data | A2A consumers cannot reconstruct the streamed call | High | IT-AF-2439-001 | Inspect queued A2A event metadata and payload |

## 4. Scope

### 4.1 Features to be Tested

- `pkg/kubernautagent/llm/geminifamily`: native/Vertex-neutral stream conversion.
- `internal/kubernautagent/investigator`: callback propagation, execution boundary, and retry policy.
- `internal/kubernautagent/session`: stable event type and non-blocking sink behavior.
- `pkg/apifrontend/tools` and `pkg/apifrontend/launcher`: structured A2A relay.
- `cmd/kubernautagent/llm_builder.go`: production native and Vertex construction paths.

### 4.2 Features Not to be Tested

- Live Gemini or Vertex calls in mandatory CI. Those remain manual/on-demand only.
- OpenAI adapter changes already present in the worktree; they are unrelated to #2439.
- Tool execution semantics when a complete `ChatResponse.ToolCalls` is present; existing tests cover that authority boundary.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Emit a separate `tool_call_delta` event | Partial function-call fragments are not complete tool calls and must not reuse execution events. |
| Preserve structured fields in AF metadata/payload | Consumers need index and argument fragments, not only user-readable text. |
| Mark any emitted delta as unsafe to retry | A retry would duplicate an already observed partial stream. |

## 5. Approach

### 5.1 Coverage Policy

- Unit: adapter and investigator behavior with deterministic mocks/httptest.
- Integration: KA-to-session and session-to-AF wiring with an in-memory A2A queue.
- E2E: deferred; no live provider is required for this transport contract.

### 5.2 Pass/Fail Criteria

PASS requires all P0 scenarios to pass, no affected-package regressions, no live
provider dependency, and `git diff --check` to pass.

FAIL includes any dropped fragment, early execution/audit event, retry after a
delivered fragment, or AF event missing required structured fields.

## 6. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|------------------------|----------------------|------------|
| Gemini tool-call delta conversion | `llm.Client.StreamChat` | `pkg/kubernautagent/llm/geminifamily/client.go` | UT-GM-2439-001 |
| Investigator delta emission | `Investigator.Investigate` / Phase 3 | `internal/kubernautagent/investigator/investigator_loop.go` | IT-KA-2439-002 |
| Session event relay | A2A investigation bridge | `pkg/apifrontend/tools/ka_investigate_bridge.go` | IT-AF-2439-001 |

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AI-087 | Gemini function-call stream fidelity | P0 | Unit | UT-GM-2439-001 | GREEN |
| BR-SESSION-003 | Observer receives structured deltas | P0 | Integration | IT-KA-2439-002 | GREEN |
| BR-AUDIT-005 | Partial calls do not become execution/audit authority | P0 | Unit | UT-KA-2439-002 | GREEN |
| BR-SESSION-003 | No duplicate output after partial-stream failure | P0 | Unit | UT-KA-2439-003 | GREEN |
| BR-SESSION-003 | AF preserves structured event payload | P0 | Integration | IT-AF-2439-001 | GREEN |

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|-----------------------------|-------|
| `UT-GM-2439-001` | Native Gemini fragmented function-call chunks produce ordered partial tool-call events and a complete final tool call. | GREEN |
| `UT-KA-2439-002` | Partial fragments alone never execute a tool or emit an LLM tool-call audit event. | GREEN |
| `UT-KA-2439-003` | A retryable failure after a partial fragment does not trigger a second LLM attempt. | GREEN |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|-----------------------------|-------|
| `IT-KA-2439-002` | The production investigator stream path carries the structured event through the session sink. | GREEN |
| `IT-AF-2439-001` | AF emits an A2A event retaining event type, turn, phase, index, name, ID, and argument fragment. | GREEN |

## 9. Environment and Data

- Go tests use Ginkgo/Gomega.
- Gemini HTTP tests use `httptest.Server`; no external credentials or network calls.
- Investigator tests use deterministic `llm.Client` stubs and in-memory audit/event sinks.
- Vertex Gemini production construction honors `VertexLocation`; the manual live validation configuration uses location `global` and is excluded from CI.

## 10. TDD Phase Tracking

- RED: test plan, DD-2439, and failing propagation/relay tests.
- GREEN: add the minimal event constants, adapter callback propagation, retry guard, and AF relay.
- REFACTOR: improve naming/comments and validate build, lint, focused tests, and full affected packages.
