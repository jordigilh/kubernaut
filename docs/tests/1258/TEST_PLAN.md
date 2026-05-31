# Test Plan: A2A Progressive Streaming (Issue #1258)

> **Note:** `summarizeCreateRR` output format updated by [#1282](https://github.com/jordigilh/kubernaut/issues/1282).
> Expected TextPart for `af_create_rr` is now `"Remediation request created: {message}"` or
> `"Remediation request created."`, not `"Remediation created."`. See `UT-AF-1282-OUT-*`.

**IEEE 829-2008 Compliant**

| Field | Value |
|-------|-------|
| Plan ID | TP-1258 |
| Version | 1.0 |
| Author | AI Agent |
| Date | 2026-05-24 |
| Status | Draft |
| Business Requirements | BR-INT-029, BR-AI-056 |

---

## 1. Introduction

### 1.1 Purpose

Validate that the A2A progressive streaming feature enables external agents to receive real-time LLM reasoning and action status during the 4-phase interactive remediation journey via the `message/stream` A2A method.

### 1.2 Scope

- StreamingExecutor wrapper (`pkg/apifrontend/launcher/streaming_executor.go`)
- EventBridge context helpers (`pkg/apifrontend/launcher/event_bridge.go`)
- KA SSE → A2A bridge in `HandleStreamInvestigation` (`pkg/apifrontend/tools/ka_stream.go`)
- Part converter FunctionResponse suppression (`pkg/apifrontend/launcher/part_converter.go`)
- Agent card streaming capability (`pkg/apifrontend/handler/agentcard.go`)
- Audit method detection fix (`pkg/apifrontend/launcher/launcher.go`)

### 1.3 Out of Scope

- `token_delta` relay (deferred — only `reasoning_delta` relayed for GA)
- `TC-E2E-STREAM-03` client disconnect → session Disconnected (deferred to PR7)
- `tasks/resubscribe` E2E validation (library-tested, AF IT only)

### 1.4 References

- Issue: https://github.com/jordigilh/kubernaut/issues/1258
- PR #1193: 4-phase interactive journey (foundation)
- TP-1189: Interactive remediation test plan
- a2a-go v0.3.15 eventqueue documentation
- ADK v1.2.0 adka2a executor documentation

---

## 2. Test Strategy

### 2.1 Test Pyramid (Invariant)

| Tier | Target Coverage | Scope |
|------|-----------------|-------|
| **Unit (UT)** | ≥80% of unit-testable code | Executor wrapper, event bridge, part converter changes, KA stream bridge |
| **Integration (IT)** | ≥80% of integration-testable code | A2A message/stream through router, KA SSE mock → A2A artifact emission |
| **E2E** | Behavioral validation | Full Kind cluster, real AF pod with mock-LLM + KA SSE stream |

### 2.2 Testing Framework

- Ginkgo/Gomega BDD (all tiers)
- `httptest` for HTTP mocking (UT/IT)
- `a2aclient` for A2A JSON-RPC streaming (IT/E2E)
- Mock KA SSE server for controlled event emission

### 2.3 Mock Strategy

| Dependency | Strategy |
|------------|----------|
| KA SSE endpoint | `httptest` server emitting controlled SSE events |
| LLM (Gemini) | mock-llm service (existing E2E infrastructure) |
| eventqueue.Queue | Real `eventpipe` (test goroutine-safety) |
| A2A client | `a2aclient.Client.SendStreamingMessage()` |

---

## 3. Test Scenarios

### 3.1 Unit Tests — StreamingExecutor

| ID | Scenario | Input | Expected |
|----|----------|-------|----------|
| UT-AF-1258-001 | Execute delegates to inner executor | Valid reqCtx + queue | Inner Execute called, no error |
| UT-AF-1258-002 | Execute stores bridge in context | Valid reqCtx + queue | Bridge retrievable from ctx inside inner Execute |
| UT-AF-1258-003 | Cancel delegates to inner executor | Valid reqCtx + queue | Inner Cancel called |
| UT-AF-1258-004 | Cleanup delegates to inner (AgentExecutionCleaner) | Result + error | Inner Cleanup called |
| UT-AF-1258-005 | Cleanup nil-safe when inner lacks Cleaner | No cleaner | No panic |

### 3.2 Unit Tests — EventBridge

| ID | Scenario | Input | Expected |
|----|----------|-------|----------|
| UT-AF-1258-010 | EmitReasoning writes TaskArtifactUpdateEvent to queue | Text + bridge | Event written with TextPart |
| UT-AF-1258-011 | EmitReasoning nil-safe when bridge not in context | nil bridge | No-op, no panic |
| UT-AF-1258-012 | EmitReasoning returns error on closed queue | Closed queue | ErrQueueClosed returned |
| UT-AF-1258-013 | EmitReasoning respects context cancellation | Cancelled ctx | ctx.Err() returned |
| UT-AF-1258-014 | EventBridgeFromContext returns nil for non-streaming | No bridge in ctx | nil returned |
| UT-AF-1258-015 | EmitReasoning sets Append=true on artifact | Text emission | Artifact.Append == true |
| UT-AF-1258-016 | EmitReasoning truncates text > 512 chars | Long text | Truncated to 512 |

### 3.3 Unit Tests — KA Stream Bridge Integration

| ID | Scenario | Input | Expected |
|----|----------|-------|----------|
| UT-AF-1258-020 | reasoning_delta emitted via bridge during stream | KA SSE with reasoning_delta | Bridge.EmitReasoning called with content_preview |
| UT-AF-1258-021 | token_delta NOT emitted (GA policy) | KA SSE with token_delta | Bridge not called |
| UT-AF-1258-022 | tool_call NOT emitted via bridge | KA SSE with tool_call | Bridge not called |
| UT-AF-1258-023 | tool_result NOT emitted via bridge | KA SSE with tool_result | Bridge not called |
| UT-AF-1258-024 | complete event emits final summary | KA SSE with complete | Bridge emits summary text |
| UT-AF-1258-025 | Bridge nil-safe when not in A2A streaming context | No bridge in ctx | HandleStreamInvestigation works as before |
| UT-AF-1258-026 | Structured content_preview extracted from reasoning_delta | `{"content_preview":"text"}` | "text" extracted |
| UT-AF-1258-027 | Bridge write error logged but does not fail tool | queue.Write returns error | Tool continues, error logged |

### 3.4 Unit Tests — Part Converter Changes

| ID | Scenario | Input | Expected |
|----|----------|-------|----------|
| UT-AF-1258-030 | FunctionResponse for stream_investigation → nil when bridge active | FunctionResponse part | nil returned (suppressed) |
| UT-AF-1258-031 | FunctionResponse for discover_workflows → brief summary | FunctionResponse part | "Found N workflows." TextPart |
| UT-AF-1258-032 | FunctionResponse for select_workflow → brief status | FunctionResponse part | "Workflow selected." TextPart |
| UT-AF-1258-033 | FunctionResponse for af_create_rr → brief status | FunctionResponse part | "Remediation created." TextPart |
| UT-AF-1258-034 | FunctionResponse for kubectl_* → nil (unchanged) | FunctionResponse part | nil |
| UT-AF-1258-035 | FunctionCall parts unchanged (action status) | FunctionCall part | Existing toolStatusMessages behavior |
| UT-AF-1258-036 | Thought parts unchanged ("Analyzing...") | Thought part | "Analyzing..." |

### 3.5 Unit Tests — Agent Card + Audit

| ID | Scenario | Input | Expected |
|----|----------|-------|----------|
| UT-AF-1258-040 | Agent card advertises streaming: true | GET /.well-known/agent-card.json | capabilities.streaming == true |
| UT-AF-1258-041 | Audit detects message/stream method | Stream request metadata | audit.Detail["method"] == "message/stream" |
| UT-AF-1258-042 | Audit preserves message/send detection | Send request metadata | audit.Detail["method"] == "message/send" |

### 3.6 Integration Tests

| ID | Scenario | Input | Expected |
|----|----------|-------|----------|
| IT-AF-1258-001 | message/stream returns SSE content-type | POST /a2a/invoke method=message/stream | Content-Type: text/event-stream |
| IT-AF-1258-002 | message/stream yields TaskArtifactUpdateEvent with reasoning | Stream + mock LLM triggering stream_investigation | At least 1 KA-derived reasoning artifact |
| IT-AF-1258-003 | message/stream yields final TaskStatusUpdateEvent | Stream through full tool execution | final=true, state=completed |
| IT-AF-1258-004 | FunctionResponse suppressed in stream output | Stream investigation with bridge | No stream_investigation FunctionResponse artifact |
| IT-AF-1258-005 | Concurrent ADK + bridge writes don't deadlock | High-frequency KA + LLM events | Task completes within timeout |

### 3.7 E2E Tests

| ID | Scenario | Input | Expected |
|----|----------|-------|----------|
| E2E-AF-1258-001 | Progressive streaming in Kind cluster | message/stream "investigate the OOM" | SSE events include reasoning text before final completion |
| E2E-AF-1258-002 | Agent card reflects streaming capability | GET /.well-known/agent-card.json via NodePort | streaming: true |

---

## 4. Test Environment

### 4.1 Unit Tests

- Local Go test runner (`go test ./pkg/apifrontend/...`)
- No external dependencies (all mocked)
- `httptest` for KA SSE simulation

### 4.2 Integration Tests

- Local Go test runner
- Real HTTP router stack with test middleware
- Mock KA SSE server (`httptest`)
- Mock LLM (in-process or `httptest`)

### 4.3 E2E Tests

- Kind cluster (existing `apifrontend-e2e` infrastructure)
- Real AF pod with coverage instrumentation
- mock-llm service with keyword_scenarios
- Mock KA SSE endpoint (via mock-llm or dedicated fixture)

---

## 5. Risk Assessment

| ID | Risk | Probability | Impact | Mitigation |
|----|------|-------------|--------|------------|
| R1 | Queue backpressure blocks ADK on high KA volume | Low | Medium | Only relay reasoning_delta (low freq) |
| R2 | Sensitive data in reasoning text leaked to external agent | Medium | High | Basic redaction + no tool_result relay |
| R3 | Part converter suppression breaks existing E2E tests | Medium | Medium | Selective suppression, update affected tests |
| R4 | Non-deterministic event ordering in stream | Low | Low | Append-only artifacts, external agent assembles |
| R5 | extractTextFromData mismatch with prod KA JSON | High | High | Fix parser + add structured payload tests |

---

## 6. Entry/Exit Criteria

### 6.1 Entry Criteria

- GA readiness audit completed (Phase 0) ✓
- Test plan approved
- a2a-go v0.3.15 eventqueue API confirmed goroutine-safe ✓

### 6.2 Exit Criteria

- All UT/IT/E2E tests pass
- ≥80% coverage per tier for new/modified code
- `go build ./...` passes
- `golangci-lint run` passes
- No `XIt` or `Skip()` in new tests
- Agent card advertises streaming: true
- Audit events emitted for stream lifecycle

---

## 7. Deliverables

| Deliverable | Location |
|-------------|----------|
| StreamingExecutor | `pkg/apifrontend/launcher/streaming_executor.go` |
| EventBridge | `pkg/apifrontend/launcher/event_bridge.go` |
| Unit tests | `pkg/apifrontend/launcher/streaming_executor_test.go`, `event_bridge_test.go` |
| KA stream bridge tests | `pkg/apifrontend/tools/ka_stream_test.go` (additions) |
| Part converter tests | `pkg/apifrontend/launcher/part_converter_test.go` (additions) |
| Integration tests | `test/integration/apifrontend/a2a_streaming_test.go` |
| E2E tests | `test/e2e/apifrontend/streaming_test.go` (additions) |
