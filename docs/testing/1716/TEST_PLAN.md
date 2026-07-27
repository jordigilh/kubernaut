# Test Plan: Issue #1716 — AF: Relay a Live Signal for Redacted Reasoning Turns

**Feature**: Emit a content-free live SSE signal (`metadata.redacted=true`, no text) on a redacted
reasoning turn, instead of AF's current full no-op, so Console can render a "reasoning hidden by
provider" placeholder.
**Version**: 1.1
**Created**: 2026-07-23
**Author**: AI Agent (triage + implementation planning)
**Status**: Implemented
**Branch**: `chore/253-drop-redundant-cert-manager-e2e` (implementation should land on its own branch)

**Authority**:
- [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md) AC10: live-streaming of KA's captured reasoning content
- [DD-LLM-009](../../architecture/decisions/DD-LLM-009-reasoning-content-live-stream-event-type.md): dedicated event type + deferred redaction sub-decision (this issue revisits that sub-decision)
- Issue #1716: AF: relay a live signal for redacted reasoning turns (BR-AI-086)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- Prior art in the same pipeline: `pkg/apifrontend/tools/reasoning_content_relay_1635_test.go` (#1635), `pkg/apifrontend/tools/pooled_session_event_relay_1637_test.go` (#1637)

---

## 1. Scope

### In Scope

- **`EventBridge.EmitReasoningContent`/`EmitReasoningContentSafe`** (`pkg/apifrontend/launcher/event_bridge.go`): signature gains a `redacted bool` parameter; no-op condition changes from `text == ""` to `text == "" && !redacted`; redacted turns emit `metadata.redacted=true` with empty text.
- **`emitEventToA2A`** (`pkg/apifrontend/tools/ka_investigate_bridge.go`): dedicated branch for `EventTypeReasoningContentDelta` that extracts `redacted` from `evt.Data` and calls the updated `EmitReasoningContentSafe`.
- **New helper `extractJSONBool`** (`pkg/apifrontend/tools/ka_investigate_bridge.go`): boolean counterpart to the existing `extractJSONField` string helper.
- **Existing regression test** `UT-AF-1635-003` (`pkg/apifrontend/tools/reasoning_content_relay_1635_test.go`): its current assertion (redacted event -> zero queue writes) is exactly the behavior this issue overturns; it must be updated in place.

### Out of Scope

- **KA producer side** (`internal/kubernautagent/investigator/investigator_tools.go`): already emits `{"text": ..., "redacted": ...}` on the wire — confirmed during triage, no change needed.
- **`katypes.ReasoningSummary`/audit trail** (`pkg/kubernautagent/types/types.go`, `internal/kubernautagent/audit/ds_response_mapping.go`): unaffected; remains the durable record of truth for redacted content (AC6).
- **`EmitReasoning`/`EmitOutput`/`emitWithLimit`'s existing no-op-on-empty-text contract**: unchanged for every other caller.
- **`FormatEventForUser`'s text-extraction behavior**: unchanged — still returns `""` for a redacted turn's text; the `redacted` flag is read directly from `evt.Data` inside `emitEventToA2A`, which already receives the full event.
- **Console-side rendering** (`kubernaut-console#32`): separate repo, out of scope here.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Thread `redacted bool` as a new parameter on `EmitReasoningContent`/`EmitReasoningContentSafe`, rather than a new event type or new field on `ka.InvestigationEvent` | The wire payload already carries `redacted` (KA producer confirmed unchanged); DD-LLM-009's deferred sub-decision explicitly anticipated this exact additive extension |
| `emitEventToA2A` extracts `redacted` itself (via a new `extractJSONBool` helper) rather than changing `FormatEventForUser`'s return type | `FormatEventForUser` is used by many other event types purely for display text; `emitEventToA2A` already receives the full `evt` (including `Data`), so no signature change ripples outward |
| Bypass `emitWithLimit`'s unconditional empty-text guard only inside `EmitReasoningContent`, not by changing `emitWithLimit` itself | `emitWithLimit` is shared with `EmitReasoning`/`EmitOutput`, whose no-op-on-empty-text contract must not change |

---

## 2. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | Changing `EmitReasoningContent`'s no-op condition regresses the pre-existing non-redacted empty-text case (e.g. a genuinely empty/error turn) | A spurious empty-text event would appear on the SSE stream for a case that must stay silent | Medium | `UT-AF-1716-EB-002`, `UT-AF-1716-002` | Explicit regression test asserting `redacted=false, text==""` still produces zero events |
| R2 | `UT-AF-1635-003` continues to assert the old (now-incorrect) behavior after GREEN | Stale test either fails (caught by CI) or is accidentally left contradicting the new contract | Low | `UT-AF-1716-001` (rewrite of `UT-AF-1635-003`) | Section 9 documents the exact required rewrite before GREEN begins |
| R3 | Console team's eventual rendering contract for `metadata.redacted` on `reasoning_content` diverges from this shape | Console-side follow-up work in `kubernaut-console#32` needs adjustment | Low | N/A (cross-repo) | Metadata key name (`redacted`) matches the audit trail's own `ReasoningSummary.Redacted` json tag for consistency; DD-LLM-009 pre-approved a boolean-only signal shape |

### 2.1 Risk-to-Test Traceability

R1 and R2 are High-value risks (they gate correctness of the core behavior change) and each has a direct test. R3 has no test (cross-repo, informational only).

---

## 3. Coverage Policy

- **Unit**: >=80% of unit-testable code (`EmitReasoningContent`/`EmitReasoningContentSafe` branch logic, `extractJSONBool`, `emitEventToA2A` routing logic)
- **Integration**: 100% of wiring points in this manifest (both production entry points that funnel through `emitEventToA2A` — initial investigation and pooled/interactive relay)
- **E2E**: None — this is a backend-only signal-shape change with no new user-facing K8s behavior; existing full-pipeline E2E suites are unaffected and do not need a new scenario (see Tier Skip Rationale)

### Two-Tier Minimum

Covered by UT + IT per BR-AI-086 AC10 refinement (see BR Coverage Matrix).

---

## 4. Wiring Manifest

Both call sites already exist in production and already call `emitEventToA2A` — this issue changes `emitEventToA2A`'s internal routing for one event type, so no new wiring is introduced. Manifest below proves both existing production paths pick up the change automatically.

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|------------------------|------------------------|------------|
| `emitEventToA2A` (redacted branch) | `bridgeEventsCollectSummary` — the blocking MCP investigation path used by `kubernaut_investigate` | `pkg/apifrontend/tools/ka_investigate_bridge.go:181` (`processBridgeEvent` -> `emitEventToA2A`) | `IT-AF-1716-001` |
| `emitEventToA2A` (redacted branch) | `WatchTerminalEvents`/`relayLiveEvent` — the pooled/interactive-turn live relay path (#1637, DD-AF-009) used by `kubernaut_message` and other pooled calls | `pkg/apifrontend/tools/ka_investigate_bridge.go:410` (`relayLiveEvent` -> `emitEventToA2A`) | `IT-AF-1716-002` |

---

## 5. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|--------------------|------------------|
| `pkg/apifrontend/launcher/event_bridge.go` | `EmitReasoningContent`, `EmitReasoningContentSafe` | ~20 |
| `pkg/apifrontend/tools/ka_investigate_bridge.go` | `extractJSONBool` (new), `emitEventToA2A` routing branch | ~20 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|--------------------|------------------|
| `pkg/apifrontend/tools/ka_investigate_bridge.go` | `bridgeEventsCollectSummary`/`BridgeEventsCollectSummary` (initial investigation dispatch) | 0 (existing, exercised not modified) |
| `pkg/apifrontend/tools/ka_investigate_bridge.go` | `WatchTerminalEvents`/`relayLiveEvent` (pooled/interactive dispatch, #1637) | 0 (existing, exercised not modified) |

---

## 6. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AI-086 AC10 | Redacted reasoning turn emits `metadata.redacted=true` with empty text via `EmitReasoningContent` | P0 | Unit | UT-AF-1716-EB-001 | Pass |
| BR-AI-086 AC10 | Non-redacted empty-text turn still no-ops (regression guard, unchanged contract) | P0 | Unit | UT-AF-1716-EB-002 | Pass |
| BR-AI-086 AC10 | `emitEventToA2A` relays a redacted signal end-to-end via the initial-investigation dispatch path | P0 | Integration | IT-AF-1716-001 | Pass |
| BR-AI-086 AC10 | `emitEventToA2A` relays a redacted signal end-to-end via the pooled/interactive relay path (#1637) | P0 | Integration | IT-AF-1716-002 | Pass |
| BR-AI-086 AC10 | Non-redacted empty-text event produces zero queue writes (regression guard on the real dispatch path) | P0 | Integration | UT-AF-1716-002 | Pass |
| BR-AI-086 AC10 | `UT-AF-1635-003` rewritten in place: a redacted event now produces one queue write carrying `metadata.redacted=true`, superseding its old zero-writes assertion | P0 | Integration | UT-AF-1716-001 (rewrite of `UT-AF-1635-003`) | Pass |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 7. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-1716-{SEQUENCE}` (mirrors the existing `UT-AF-1635-*`/`IT-AF-1637-*` convention in this same pipeline)

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `AF` (API Frontend)
- **1716**: Issue number
- **SEQUENCE**: descriptive suffix where an existing sub-convention exists (`EB` for EventBridge-level tests), else zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `EmitReasoningContent`/`EmitReasoningContentSafe` (~20 LOC), `extractJSONBool` + `emitEventToA2A` routing (~20 LOC). Target: >=80%.

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-AF-1716-EB-001` | `EmitReasoningContent(ctx, "", redacted=true)` writes one `TaskStatusUpdateEvent` with `metadata.type=reasoning_content`, `metadata.redacted=true`, empty message text | Pass |
| `UT-AF-1716-EB-002` | `EmitReasoningContent(ctx, "", redacted=false)` still no-ops (zero events) — regression guard for the preserved case | Pass |

**File**: `pkg/apifrontend/launcher/event_bridge_test.go` (existing `Describe("EmitReasoningContent", ...)` block)

### Tier 2: Integration Tests

**Testable code scope**: both production dispatch paths that funnel through `emitEventToA2A` (~0 new LOC, existing wiring exercised end-to-end).

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-AF-1716-001` | A real `ka.InvestigationEvent{Type: EventTypeReasoningContentDelta, Data: {"text":"","redacted":true}}` driven through `BridgeEventsCollectSummary` (production dispatch path) reaches the A2A queue as one event with `metadata.redacted=true` | Pass |
| `IT-AF-1716-002` | The same redacted event driven through `WatchTerminalEvents`'s live-relay path (#1637 pooled/interactive calls) reaches the attached ctx's queue with `metadata.redacted=true` | Pass |
| `UT-AF-1716-002` | A non-redacted empty-text event (`{"text":"","redacted":false}`) driven through `BridgeEventsCollectSummary` still produces zero queue writes (regression guard, matches R1's mitigation) | Pass |

**Note (v1.1 correction)**: earlier drafts of this plan (Sections 6-8) inconsistently reused
`UT-AF-1716-001` for this regression-guard scenario, colliding with Section 9's use of the same
ID for the `UT-AF-1635-003` rewrite. Risk R1 (Section 2) already used `UT-AF-1716-002` for this
exact scenario; the implementation follows R1's numbering, so this regression guard is
`UT-AF-1716-002`, and `UT-AF-1716-001` refers only to the rewritten `UT-AF-1635-003` (Section 9).

**File**: new `pkg/apifrontend/tools/redacted_reasoning_signal_1716_test.go`

### Tier Skip Rationale

**E2E skipped**: this is a backend signal-shape refinement (one additional metadata field on an existing, already-E2E-covered SSE event type). No new user-facing K8s behavior or control-plane state change is introduced. Existing full-pipeline E2E suites already exercise the `reasoning_content` SSE channel's existence; the redaction-aware rendering itself is Console-side (`kubernaut-console#32`, separate repo/E2E surface).

---

## 8. Test Cases (Detail)

### UT-AF-1716-EB-001: Redacted turn emits content-free signal

**BR**: BR-AI-086 AC10
**Type**: Unit
**File**: `pkg/apifrontend/launcher/event_bridge_test.go`

**Given**: An `EventBridge` attached to a fake queue via `WithEventBridge`
**When**: `bridge.EmitReasoningContent(ctx, "", true)` is called
**Then**:
- Exactly one `TaskStatusUpdateEvent` is written
- `evt.Metadata["type"]` equals `"reasoning_content"`
- `evt.Metadata["redacted"]` equals `true`
- The message text part (if present) is empty

**Acceptance Criteria**:
- **Behavior**: redacted turns are no longer silently dropped
- **Correctness**: no visible text is ever synthesized for a redacted turn
- **Accuracy**: `metadata.redacted` is exactly `true`, not a truthy string or other type

### UT-AF-1716-EB-002: Non-redacted empty text still no-ops

**BR**: BR-AI-086 AC10
**Type**: Unit
**File**: `pkg/apifrontend/launcher/event_bridge_test.go`

**Given**: An `EventBridge` attached to a fake queue
**When**: `bridge.EmitReasoningContent(ctx, "", false)` is called
**Then**: Zero events are written (identical to pre-#1716 behavior for this case)

**Acceptance Criteria**:
- **Behavior**: the existing no-op contract for a genuinely-empty, non-redacted turn is preserved byte-for-byte

**Dependencies**: none

### IT-AF-1716-001: Redacted signal reaches the queue via the initial-investigation dispatch path

**BR**: BR-AI-086 AC10
**Type**: Integration
**File**: `pkg/apifrontend/tools/redacted_reasoning_signal_1716_test.go`

**Given**: A channel of `ka.InvestigationEvent` containing one `EventTypeReasoningContentDelta` event with `Data: {"text":"","redacted":true}`, followed by `EventTypeComplete`
**When**: The channel is driven through `tools.BridgeEventsCollectSummary` (the same production entry point used by `kubernaut_investigate`)
**Then**: The A2A queue contains exactly one event with `metadata.type=reasoning_content` and `metadata.redacted=true`

**Acceptance Criteria**:
- **Behavior**: the production dispatch path (not just the `EventBridge` unit) relays the signal
- **Correctness**: no other event carries `metadata.redacted=true`
- **Accuracy**: the redacted event's text remains empty

**Dependencies**: `UT-AF-1716-EB-001`

### IT-AF-1716-002: Redacted signal reaches the queue via the pooled/interactive relay path

**BR**: BR-AI-086 AC10
**Type**: Integration
**File**: `pkg/apifrontend/tools/redacted_reasoning_signal_1716_test.go`

**Given**: A `ka.EventRelay` attached to a live ctx (mirroring `IT-AF-1637-004`'s setup), and a redacted `EventTypeReasoningContentDelta` event delivered via `tools.WatchTerminalEvents`
**When**: The event is relayed live to the attached ctx
**Then**: The attached ctx's queue receives the event with `metadata.redacted=true`; the watcher's own detached ctx receives nothing

**Acceptance Criteria**:
- **Behavior**: the fix is live on the #1637 pooled-session code path without any additional wiring change
- **Correctness**: matches #1637's existing idle/live routing semantics exactly

**Dependencies**: `IT-AF-1716-001`

### UT-AF-1716-002: Non-redacted empty text produces zero queue writes (regression guard)

**BR**: BR-AI-086 AC10
**Type**: Integration (`UT` prefix retained per R1's own naming, to preserve grep-ability alongside the EventBridge-level `UT-AF-1716-EB-002`)
**File**: `pkg/apifrontend/tools/redacted_reasoning_signal_1716_test.go`

**Given**: A `ka.InvestigationEvent{Type: EventTypeReasoningContentDelta, Data: {"text":"","redacted":false}}`
**When**: Driven through `tools.BridgeEventsCollectSummary`
**Then**: The A2A queue is empty (this is the one case that must NOT change behavior)

**Acceptance Criteria**:
- **Behavior**: distinguishes "genuinely nothing to say" (`redacted=false`, empty text) from "reasoning occurred but was withheld" (`redacted=true`, empty text) — only the latter now produces an event

### UT-AF-1716-001 (rewrite of UT-AF-1635-003): Redacted turn relays a content-free signal

**BR**: BR-AI-086 AC10
**Type**: Integration (despite the `UT` prefix retained from its #1635 origin, to preserve grep-ability across the two issues)
**File**: `pkg/apifrontend/tools/reasoning_content_relay_1635_test.go`

**Given**: A `ka.InvestigationEvent{Type: EventTypeReasoningContentDelta, Data: {"text":"","redacted":true}}` (the exact payload `UT-AF-1635-003` originally used)
**When**: Driven through `tools.BridgeEventsCollectSummary`
**Then**: The A2A queue contains exactly one event with `metadata.type=reasoning_content` and `metadata.redacted=true` (supersedes the old zero-writes assertion — see Section 9)

**Acceptance Criteria**:
- **Behavior**: the pre-existing regression test that encoded the bug being fixed now encodes the corrected contract instead of silently going stale

---

## 9. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|---------------------|---------------------|--------------------|--------|
| `UT-AF-1635-003` in `pkg/apifrontend/tools/reasoning_content_relay_1635_test.go:110-127` | A redacted (`{"text":"","redacted":true}`) event produces **zero** queue writes | Rewrite to assert **one** queue write carrying `metadata.redacted=true` and empty text; re-tag as `UT-AF-1716-001` | This is exactly the behavior #1716 overturns — the old assertion is a description of the bug being fixed, not a contract to preserve |
| `UT-AF-1635-EB-004` in `pkg/apifrontend/launcher/event_bridge_test.go:456-465` | `bridge.EmitReasoningContent(ctx, "")` (1-arg) no-ops | Update call site to `bridge.EmitReasoningContent(ctx, "", false)` (signature gains `redacted` param); assertion (zero events) is unchanged since `redacted=false` here | Signature change only — behavior for this exact case is preserved |
| `UT-AF-1635-EB-001`, `UT-AF-1635-EB-002`, `UT-AF-1635-EB-003` in `pkg/apifrontend/launcher/event_bridge_test.go:414-454` | Call `EmitReasoningContent`/`EmitReasoningContentSafe` with text-only args | Update call sites to pass `redacted=false`; assertions unchanged | Signature change only, non-redacted paths are unaffected |
| `UT-AF-1635-001`, `UT-AF-1635-002`, `UT-AF-1635-004` in `pkg/apifrontend/tools/reasoning_content_relay_1635_test.go` | `FormatEventForUser`/`isStatusEvent`/summary-accumulation behavior | None | These test `FormatEventForUser` and `processBridgeEvent`'s summary logic directly, which #1716 does not touch |
| `IT-AF-1637-004` in `pkg/apifrontend/tools/pooled_session_event_relay_1637_test.go` | Non-redacted (`redacted:false`) event relays correctly through the pooled path | None | Uses a non-redacted event; unaffected by the new branch |

---

## 10. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — `fakeQueue`/`bridgeQueue` test doubles already exist in the package for `eventqueue.Interface`
- **Location**: `pkg/apifrontend/launcher/event_bridge_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks of internal business logic — drives real `ka.InvestigationEvent` channels through the real, unmodified-by-test production dispatch functions (`BridgeEventsCollectSummary`, `WatchTerminalEvents`)
- **Location**: `pkg/apifrontend/tools/redacted_reasoning_signal_1716_test.go` (new), `pkg/apifrontend/tools/reasoning_content_relay_1635_test.go` (updated)

---

## 11. Execution

```bash
# Unit tests (EventBridge)
go test ./pkg/apifrontend/launcher/... -ginkgo.focus="EmitReasoningContent"

# Integration tests (relay paths)
go test ./pkg/apifrontend/tools/... -ginkgo.focus="1716"

# Full AF package regression
go test ./pkg/apifrontend/...

# Full build + lint
go build ./...
golangci-lint run --timeout=5m
```

---

## 12. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-23 | Initial test plan: 2 UT + 2 IT + 1 rewritten regression test = 5 scenarios |
| 1.1 | 2026-07-23 | Implementation complete, all 6 scenarios Pass. Corrected a test ID collision introduced in v1.0: Sections 6-8 had reused `UT-AF-1716-001` for the non-redacted regression guard, colliding with Section 9's use of the same ID for the `UT-AF-1635-003` rewrite. Resolved per Risk R1's own (correct) numbering: the regression guard is `UT-AF-1716-002`; `UT-AF-1716-001` refers only to the rewritten `UT-AF-1635-003`. |
