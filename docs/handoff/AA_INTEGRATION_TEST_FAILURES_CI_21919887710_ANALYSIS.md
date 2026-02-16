# AA Integration Test Failures - CI Run 21919887710 Analysis

**Date**: February 11, 2026  
**CI Run**: 21919887710, Job 63297335399  
**Status**: Root cause identified for all 6 failing tests

---

## Executive Summary

| Test | Type | Root Cause |
|------|------|------------|
| IT-AA-064-01b | NEW (RED) | Session lifecycle logic - needs GREEN phase |
| IT-AA-064-01c | NEW (RED) | Session lifecycle logic - needs GREEN phase |
| BR-AUDIT-001 | EXISTING | Session mode uses audit stubs - no `holmesgpt.call` events |
| BR-AI-023 | EXISTING | Same - expects `holmesgpt.call` with HTTP 200 |
| BR-AI-050 | EXISTING | Same - expects `holmesgpt.call` (error scenario) |
| IT-AA-095-02 | EXISTING | Mock HAPI has no `mock_rca_permanent_error` scenario |

---

## 1. Test Architecture Overview

### Flow
- All tests go through the **AA reconciler** (create AIAnalysis CR → controller reconciles)
- Controller uses `WithSessionMode()` → async submit/poll/result flow (BR-AA-HAPI-064)
- HAPI returns **202 Accepted** on submit; HolmesGPTClient handles async internally
- `suite_test.go:670-672` wires the handler with `WithSessionMode()` and `WithRecorder()`

### Key Files
- `test/integration/aianalysis/audit_flow_integration_test.go` - BR-AUDIT-001, BR-AI-023, BR-AI-050
- `test/integration/aianalysis/events_test.go` - IT-AA-095-02, IT-AA-064-01b, IT-AA-064-01c
- `pkg/aianalysis/handlers/investigating.go` - session mode flow
- `pkg/aianalysis/audit/audit.go` - RecordHolmesGPTSubmit, RecordHolmesGPTResult (stubs)

---

## 2. Root Cause: Tests #3–#6 (Existing Tests That Broke)

### 2.1 BR-AUDIT-001, BR-AI-023, BR-AI-050: Audit Event Gap

**Root Cause**: Session mode uses `RecordHolmesGPTSubmit` and `RecordHolmesGPTResult`, which are **stubs** that only log—they do **not** persist audit events to DataStorage.

**Evidence** (`pkg/aianalysis/audit/audit.go:448-460`):
```go
// RecordHolmesGPTSubmit records an async HAPI submit event with session ID.
func (c *AuditClient) RecordHolmesGPTSubmit(...) {
    c.log.V(1).Info("RecordHolmesGPTSubmit stub called", ...)
    // TODO: Implement in GREEN phase - emit EventTypeHolmesGPTSubmit audit event
}

// RecordHolmesGPTResult records an async HAPI result retrieval with investigation time.
func (c *AuditClient) RecordHolmesGPTResult(...) {
    c.log.V(1).Info("RecordHolmesGPTResult stub called", ...)
    // TODO: Implement in GREEN phase - emit EventTypeHolmesGPTResult audit event
}
```

**Legacy flow** (when `useSessionMode=false`) called `RecordHolmesGPTCall` with endpoint, statusCode, duration. The session flow never calls `RecordHolmesGPTCall`.

**Impact**:
- Tests query for `event_type=aianalysis.holmesgpt.call` (EventTypeHolmesGPTCall)
- No such events are emitted in session mode
- BR-AUDIT-001: expects 7 events including 1 holmesgpt.call → gets 6
- BR-AI-023: expects holmesgpt.call with HTTPStatusCode 200 → never gets event
- BR-AI-050: expects holmesgpt.call (even on error) → never gets event

**DD-AA-HAPI-064 design** (`docs/architecture/decisions/DD-AA-HAPI-064-session-based-pull-design.md:222-225`):
- `holmesgpt.submit`: when investigation is submitted (replaces `holmesgpt.call` for new flow)
- `holmesgpt.result`: when result is retrieved, includes `investigationTime`
- `holmesgpt.session_lost`: when session regeneration occurs

The design uses new event types; current tests still expect `holmesgpt.call`.

---

### 2.2 IT-AA-095-02: Investigation Failure Event Trail

**Root Cause**: The test expects `Failed` phase and `AnalysisFailed` event, but **Mock HAPI has no `mock_rca_permanent_error` scenario** to force a permanent failure.

**Evidence** (`events_test.go:154-156`):
```go
// NOTE: This test requires Mock HAPI configured with a scenario that returns
// permanent error (e.g., mock_rca_permanent_error). Without that config,
// the test may reach Completed instead of Failed.
```

**Grep result**: `mock_rca_permanent_error` does **not** exist in the codebase:
- Mock LLM (`test/services/mock-llm/src/server.py`) has scenarios like `mock_no_workflow_found`, `mock_rca_incomplete`, `mock_max_retries_exhausted`
- No scenario returns a permanent error that would cause HAPI to fail the session
- With default Mock LLM behavior (e.g. CrashLoopBackOff → crashloop scenario → success), the analysis reaches `Completed`, not `Failed`
- The test times out waiting for `Failed` (2 min) or fails the phase assertion

---

## 3. Root Cause: Tests #1–#2 (RED-Phase Session Tests)

### IT-AA-064-01b: SessionLost on Stale Session (404)

**Design**: Create AIAnalysis → wait for real session ID → inject fake UUID → next poll gets 404 → handler runs `handleSessionLost` → regenerates → should emit `SessionLost` + `SessionCreated` and reach `Completed`.

**Possible issues**:
1. Status update race: `k8sClient.Status().Update()` may conflict with controller updates
2. Poll timing: controller may not poll before test timeout
3. Mock HAPI 404: Mock HAPI may not return 404 for unknown session IDs (HAPI/OpenAPI behavior not verified)

### IT-AA-064-01c: SessionRegenerationExceeded

**Design**: Same setup as 01b, but set `Generation = 4` (MaxSessionRegenerations - 1) before injecting stale ID → next 404 triggers regeneration cap → should transition to `Failed` with `SessionRegenerationExceeded`.

**Possible issues**: Same as 01b, plus correctness of `MaxSessionRegenerations` and generation handling in `handleSessionLost`.

**Recommendation**: Treat both as RED-phase until:
1. Mock HAPI session endpoints are confirmed to return 404 for unknown session IDs
2. Handler logic is validated with unit tests
3. Integration environment is verified for status-update races

---

## 4. Session Mode and HAPI 202 Handling

### Handler Flow (`investigating.go`)

- `useSessionMode` set via `WithSessionMode()` option (line 70-74)
- `Handle()` → `handleSessionBased()` → `handleSessionSubmit` / `handleSessionPoll`
- HolmesGPTClient `SubmitInvestigation` expects **202 Accepted** (`holmesgpt.go:298`)
- Session flow is built for async (submit → poll → result), so 202 is handled correctly

### Audit in Session Flow

| Step | Method | Status |
|------|--------|--------|
| Submit | `RecordHolmesGPTSubmit` | Stub – no persistence |
| Result | `RecordHolmesGPTResult` | Stub – no persistence |
| Session lost | `RecordHolmesGPTSessionLost` | Stub – no persistence |

The legacy flow used `RecordHolmesGPTCall`; the session flow does not.

---

## 5. Recommended Fixes

### Fix 1: Implement Session Audit Stubs (Tests #3–#5)

**Option A – Align with design (preferred)**  
Implement `RecordHolmesGPTSubmit` and `RecordHolmesGPTResult` to emit `aianalysis.holmesgpt.submit` and `aianalysis.holmesgpt.result`, then update tests to assert on these event types instead of `holmesgpt.call`.

**Option B – Backward compatibility**  
Have `RecordHolmesGPTResult` also call `RecordHolmesGPTCall` with endpoint, 200, and investigation time. Tests stay unchanged but design diverges from DD-AA-HAPI-064.

**Suggested implementation** (Option A):

1. **`pkg/aianalysis/audit/audit.go`**:
   - Implement `RecordHolmesGPTSubmit`: create and persist event with `EventTypeHolmesGPTSubmit`, session ID in `event_data`
   - Implement `RecordHolmesGPTResult`: create and persist event with `EventTypeHolmesGPTResult`, `investigationTimeMs` in `event_data`
   - Add OpenAPI payload types if missing in data-storage schema

2. **`test/integration/aianalysis/audit_flow_integration_test.go`**:
   - BR-AUDIT-001: accept either `holmesgpt.call` OR (`holmesgpt.submit` + `holmesgpt.result`), adjust total event count
   - BR-AI-023: assert on `holmesgpt.result` (or `holmesgpt.call` if Option B), validate `investigationTime`/duration
   - BR-AI-050: add a scenario that triggers HAPI error and assert on the corresponding audit event (submit or result with failure)

### Fix 2: IT-AA-095-02 – Permanent Error Scenario (Test #6)

1. **Add Mock LLM scenario** in `test/services/mock-llm/src/server.py`:
   - e.g. `mock_rca_permanent_error` or `mock_permanent_error`
   - Return response that causes HAPI to mark the session as failed (e.g. error payload or parsing failure)

2. **Wire scenario in tests**:
   - Use a `SignalType` or other trigger that matches this scenario
   - Confirm HAPI returns session status `failed` and that AA transitions to `Failed`

3. **Alternative**: Use an existing failure scenario (e.g. `mock_max_retries_exhausted`) if it produces `Failed` phase and `AnalysisFailed` event.

### Fix 3: IT-AA-064-01b and 01c (Tests #1–#2)

1. **Verify Mock HAPI**:
   - Ensure session endpoints return 404 for unknown session IDs
   - Confirm behavior matches `holmesgpt.go` and `investigating.go` expectations

2. **Fix test flakiness**:
   - Add retries or more robust waits when updating status
   - Ensure status updates are applied before the controller reconciles with the injected stale ID

3. **Unit tests**:
   - Cover `handleSessionLost` and regeneration cap logic in unit tests before relying on integration tests

---

## 6. File References

| File | Lines | Purpose |
|------|-------|---------|
| `pkg/aianalysis/audit/audit.go` | 40-48, 448-467 | Event types, stubs |
| `pkg/aianalysis/handlers/investigating.go` | 67-74, 125-127, 379-468, 458, 566, 594 | Session mode, submit/result audit calls |
| `test/integration/aianalysis/suite_test.go` | 669-672 | WithSessionMode, WithRecorder |
| `test/integration/aianalysis/audit_flow_integration_test.go` | 133-391, 392-401, 407-447, 704-780 | BR-AUDIT-001, BR-AI-023, BR-AI-050 |
| `test/integration/aianalysis/events_test.go` | 153-218, 358-416, 418-423 | IT-AA-095-02, IT-AA-064-01b, IT-AA-064-01c |
| `pkg/holmesgpt/client/holmesgpt.go` | 278-311, 359-376 | 202 handling, SubmitInvestigation |

---

## 7. Summary

| Test | Root Cause | Fix Complexity |
|------|------------|----------------|
| BR-AUDIT-001 | Audit stubs in session mode | Medium – implement stubs + test updates |
| BR-AI-023 | Same | Medium |
| BR-AI-050 | Same + error-path audit | Medium |
| IT-AA-095-02 | Missing `mock_rca_permanent_error` | Medium – add scenario + wire |
| IT-AA-064-01b | RED – session logic/Mock HAPI | Keep RED until GREEN implementation |
| IT-AA-064-01c | RED – session logic/Mock HAPI | Keep RED until GREEN implementation |

The main driver of failures for the previously passing tests is the **session-mode audit gap**: `RecordHolmesGPTSubmit` and `RecordHolmesGPTResult` are stubs, so no HolmesGPT-related audit events are persisted, and tests expecting `holmesgpt.call` fail.
