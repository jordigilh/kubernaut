# Test Plan — #1301/#1302: Paragraph Breaks & Full KA Event Bridging

**IEEE 829 Compliant** | **Issues**: [#1301](https://github.com/jordigilh/kubernaut/issues/1301), [#1302](https://github.com/jordigilh/kubernaut/issues/1302)

| Field | Value |
|-------|-------|
| Plan ID | TP-1301-1302 |
| Version | 1.0 |
| Author | AI Agent |
| Date | 2026-05-27 |
| Status | Active |
| Business Requirements | BR-INT-029 (progressive streaming), BR-AI-056 (investigation visibility) |
| Supersedes | TP-1258 §3.3 rows UT-AF-1258-021, UT-AF-1258-022 (GA bridge policy changed) |

---

## 1. Introduction

### 1.1 Purpose

Validate that:
1. **#1301**: All A2A streaming text parts use `\n\n` paragraph breaks so clients
   (kagenti) render consecutive chunks as separate paragraphs, not concatenated blocks.
2. **#1302**: All KA SSE event types (`token_delta`, `reasoning_delta`, `tool_call`)
   are bridged to the A2A stream via `EventBridge`, and the agent prompt mandates
   `kubernaut_stream_investigation` after every investigation start.

### 1.2 Scope

| Component | File | Change |
|-----------|------|--------|
| Part converter | `pkg/apifrontend/launcher/part_converter.go` | `\n` → `\n\n` paragraph breaks; rename `ensureTrailingNewline` → `ensureTrailingParagraphBreak` |
| KA stream bridge | `pkg/apifrontend/tools/ka_stream.go` | Bridge `token_delta` + `tool_call` via `emitViaBridge` |
| Agent prompt | `pkg/apifrontend/agent/prompt.txt` | Mandate `stream_investigation` in full journey |
| Wiring | `cmd/apifrontend/main.go` | No change (existing `WithKeepAlive` + `StreamingExecutor`) |

### 1.3 Out of Scope

- KA-side SSE emission (KA's own test suite)
- kagenti client rendering logic (client team)
- `tool_result` bridging (intentionally omitted to bound queue volume — see TP-1258 R1)

### 1.4 References

- TP-1258 (A2A Progressive Streaming) — bridge policy updated by this plan
- TP-1306 (PooledMCPClient) — session persistence for MCP calls
- TP-1307 (Phase Guard) — tool ordering enforcement

---

## 2. FedRAMP Control Mapping

| Control | Title | Relevance | Test IDs |
|---------|-------|-----------|----------|
| **AU-2** | Audit Events | Bridged `token_delta` / `tool_call` events must produce audit trail via existing `afterAudit` callback | UT-AF-1302-010, IT-AF-1302-001 |
| **AU-3** | Content of Audit Records | Audit events must include `tool_name`, `result_type` for bridged tool calls | UT-AF-1302-010 |
| **AU-12** | Audit Generation | All event types flowing through `emitViaBridge` are recorded by the bridge metrics counter (`af_a2a_bridge_events_total`) | UT-AF-1302-011 |
| **SI-4** | Information System Monitoring | Progressive streaming enables real-time investigation monitoring; paragraph breaks ensure readability | UT-AF-1301-010, IT-AF-1301-001 |
| **SC-4** | Information in Shared Resources | SSE stream integrity — paragraph breaks prevent content merging across concurrent artifact frames | UT-AF-1301-011 |
| **SC-7** | Boundary Protection | Bridged `token_delta` text passes through `sanitizeBridgeText` (JWT/secret redaction) before reaching A2A stream | UT-AF-1302-012 |
| **CM-3** | Configuration Change Control | Prompt change mandating `stream_investigation` is validated by structural wiring test | WT-AF-1302-001 |

---

## 3. Test Strategy — Pyramid Invariant

> **UT proves logic. IT proves wiring. E2E proves the journey.**

| Tier | Proves | Coverage Target |
|------|--------|-----------------|
| **UT** | `ensureTrailingParagraphBreak` logic; bridge relay for all event types; sanitization of bridged text | ≥80% of modified lines in `part_converter.go`, `ka_stream.go` |
| **IT** | A2A handler SSE frames contain paragraph breaks; bridged events reach audit trail | ≥80% of integration-testable wiring |
| **WT** | Prompt contains mandatory `stream_investigation` sequence; `emitViaBridge` called in production code path | Structural compliance |
| **E2E** | Existing TC-E2E-STREAM-05 validates progressive artifacts (no new E2E needed — see §3.5) |  |

### 3.5 E2E Justification

TC-E2E-STREAM-05 in `test/e2e/apifrontend/streaming_test.go` already validates:
- `message/stream` yields `artifact-update` SSE frames with non-empty text
- Terminal `status-update` with completed state

The `\n\n` vs `\n` distinction is a text content detail that E2E cannot reliably assert
(mock-LLM output varies). The IT tier is the correct boundary for content assertions.
New E2E tests would add execution cost without proving anything the IT tier doesn't.

---

## 4. Test Scenarios

### 4.1 Unit Tests — Paragraph Break Helper (#1301)

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| UT-AF-1301-010 | Text without trailing newline gets `\n\n` | `"hello"` | `"hello\n\n"` | SI-4 |
| UT-AF-1301-011 | Text with single `\n` gets upgraded to `\n\n` | `"hello\n"` | `"hello\n\n"` | SC-4 |
| UT-AF-1301-012 | Text already ending with `\n\n` is unchanged | `"hello\n\n"` | `"hello\n\n"` | SC-4 |
| UT-AF-1301-013 | Empty string is unchanged | `""` | `""` | — |
| UT-AF-1301-014 | Text with trailing `\n\n\n` is normalized to `\n\n` | `"hello\n\n\n"` | `"hello\n\n"` | SC-4 |

### 4.2 Unit Tests — Bridge Relay Policy (#1302)

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| UT-AF-1302-010 | `token_delta` emitted via bridge | KA SSE with `token_delta` | Bridge queue contains text | AU-2 |
| UT-AF-1302-011 | `tool_call` emitted via bridge with `[Tool: ...]` format | KA SSE with `tool_call` | Bridge queue contains `[Tool: kubectl get pods]` | AU-12 |
| UT-AF-1302-012 | Bridged `token_delta` passes through sanitization | KA SSE with JWT in `token_delta` | Bridge text redacted | SC-7 |

### 4.3 Integration Tests — SSE Artifact Content (#1301)

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| IT-AF-1301-001 | A2A `message/stream` SSE artifacts contain paragraph breaks | POST with text prompt | At least 1 SSE artifact `TextPart` ending with `\n\n` | SI-4 |

### 4.4 Integration Tests — Bridge Audit Trail (#1302)

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| IT-AF-1302-001 | A2A `message/stream` produces audit trail with task lifecycle | POST streaming request | `task_started` + terminal audit event emitted | AU-2 |

### 4.5 Wiring Tests — Prompt Compliance (#1302)

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| WT-AF-1302-001 | prompt.txt contains mandatory `kubernaut_stream_investigation` in full journey | Read prompt file | Contains `MANDATORY` + `kubernaut_stream_investigation` + `CRITICAL` | CM-3 |
| WT-AF-1302-002 | prompt.txt lists `af_create_rr` as first step in fix journey | Read prompt file | "Fix something" section starts with `af_create_rr` | CM-3 |

---

## 5. Pass/Fail Criteria

- All UT/IT/WT tests pass
- ≥80% coverage per tier for modified code
- `go build ./...` passes
- `golangci-lint run` passes
- No `XIt` or `Skip()` in new tests
- Zero data races under `go test -race`
- TP-1258 updated to reflect policy change (§1.3 "Out of Scope" → "Superseded")

---

## 6. Risk Assessment

| ID | Risk | Probability | Impact | Mitigation |
|----|------|-------------|--------|------------|
| R1 | `\n\n` creates excessive whitespace in non-markdown clients | Low | Low | `\n\n` is standard markdown; non-markdown clients already handle it |
| R2 | `token_delta` bridging increases queue volume | Medium | Low | `token_delta` is low-frequency (LLM token-by-token); `tool_result` still not bridged |
| R3 | Prompt change doesn't guarantee LLM compliance | Medium | Medium | Phase guard (#1307) enforces tool ordering at runtime |
| R4 | Bridged `token_delta` leaks sensitive KA reasoning | Low | High | `sanitizeBridgeText` redacts JWTs/secrets (tested by UT-AF-1258-030/031) |

---

## 7. Deliverables

| Deliverable | Location |
|-------------|----------|
| Paragraph break helper tests | `pkg/apifrontend/launcher/part_converter_test.go` |
| Bridge relay policy tests | `pkg/apifrontend/tools/ka_stream_test.go` |
| IT streaming content test | `test/integration/apifrontend/a2a_streaming_test.go` |
| Wiring prompt compliance test | `cmd/apifrontend/main_wiring_test.go` |
| This test plan | `docs/tests/1301-1302/TEST_PLAN.md` |
