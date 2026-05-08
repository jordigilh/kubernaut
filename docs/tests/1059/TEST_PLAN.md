# Test Plan: Shadow Agent LLM Token Audit Events

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1059-v1
**Feature**: Shadow agent (alignment evaluator) emits LLM token usage to audit_events
**Version**: 1.0
**Created**: 2026-05-07
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/1059-shadow-llm-token-audit`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the implementation of Issue #1059, which adds dedicated `aiagent.shadow.llm.request` and `aiagent.shadow.llm.response` audit events from the shadow evaluator, mirroring the primary investigation LLM audit path. Token usage data from shadow LLM calls is currently discarded, making golden transcript capture and per-incident cost attribution incomplete.

### 1.2 Objectives

1. **Per-call audit emission**: Each successful shadow `Chat()` call emits `aiagent.shadow.llm.request` (before) and `aiagent.shadow.llm.response` (after, with token usage).
2. **CorrelationID propagation**: Shadow audit events carry the same `correlation_id` as the investigation (derived from `signal.RemediationID` or `signal.Name`), automatically stamped by the Observer.
3. **Canary exclusion**: Canary checks (synthetic, non-billable) do NOT emit shadow audit events (empty CorrelationID guard).
4. **Token accumulation on verdict**: The `aiagent.alignment.verdict` event includes aggregate `shadow_prompt_tokens`, `shadow_completion_tokens`, `shadow_total_tokens`.
5. **DS store mapping**: New event types are correctly mapped to ogen payload types in `buildEventData`.
6. **OpenAPI compliance**: New schemas registered in the discriminator union.
7. **Backward compatibility**: Existing alignment tests pass without modification to their behavioral assertions.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/alignment/... ./test/unit/kubernautagent/audit/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on changed unit-testable files |
| Backward compatibility | 0 regressions | All pre-existing alignment + audit tests pass |
| Build success | 0 errors | `go build ./...` after all changes |
| Lint compliance | 0 new issues | `golangci-lint run` on changed files |

---

## 2. References

### 2.1 Authority

- **BR-AI-601**: Shadow agent alignment checking for LLM investigation safety
- **Issue #1059**: Shadow agent (alignment evaluator) should emit LLM token usage to audit_events
- **DD-AUDIT-005**: Structured audit event type schemas

### 2.2 Cross-References

- **Issue #1055/#1056**: SA token refresh and audit 401 handling (precedent for audit store patterns)
- Primary LLM audit emission: `internal/kubernautagent/investigator/investigator.go` (lines 865-874)
- Token accumulator pattern: `internal/kubernautagent/investigator/token_accumulator.go`

---

## 3. Test Items

### 3.1 Components Under Test

| Component | File | Change Type |
|-----------|------|-------------|
| Evaluator | `internal/kubernautagent/alignment/evaluator.go` | Modified (WithAuditStore option, emit per-call events) |
| Observer | `internal/kubernautagent/alignment/observer.go` | Modified (correlationID field, stamp on SubmitAsync) |
| Types | `internal/kubernautagent/alignment/types.go` | Modified (Usage on Observation, CorrelationID on Step) |
| InvestigatorWrapper | `internal/kubernautagent/alignment/investigator_wrapper.go` | Modified (sum tokens, pass correlationID) |
| DS Store | `internal/kubernautagent/audit/ds_store.go` | Modified (new buildEventData cases) |
| Emitter constants | `internal/kubernautagent/audit/emitter.go` | Modified (new event type constants) |
| OpenAPI spec | `api/openapi/data-storage-v1.yaml` | Modified (new schemas) |
| Main wiring | `cmd/kubernautagent/main.go` | Modified (pass auditStore to evaluator) |

---

## 4. Test Scenarios

### 4.1 Unit Tests — Evaluator Audit Emission

| ID | Description | Assertion |
|----|-------------|-----------|
| UT-SA-1059-001 | Evaluator with WithAuditStore emits shadow.llm.request before Chat | mockAuditStore receives event with EventType == EventTypeShadowLLMRequest, correct CorrelationID, prompt_length > 0 |
| UT-SA-1059-002 | Evaluator with WithAuditStore emits shadow.llm.response on successful Chat | mockAuditStore receives event with EventType == EventTypeShadowLLMResponse, token counts matching resp.Usage |
| UT-SA-1059-003 | Evaluator does NOT emit when CorrelationID is empty (canary path) | mockAuditStore receives 0 events when step.CorrelationID == "" |
| UT-SA-1059-004 | Evaluator does NOT emit shadow.llm.response on failed Chat (retry exhaustion) | mockAuditStore receives only 1 request event, 0 response events |
| UT-SA-1059-005 | Observation carries Usage from successful response | obs.Usage.TotalTokens == resp.Usage.TotalTokens |
| UT-SA-1059-006 | Zero-usage path: emit response event with tokens_used=0 | Event emitted with total_tokens=0 |
| UT-SA-1059-007 | Evaluator without WithAuditStore emits no events | No audit store interaction; behavioral assertions unchanged |

### 4.2 Unit Tests — Observer CorrelationID Stamping

| ID | Description | Assertion |
|----|-------------|-----------|
| UT-SA-1059-008 | Observer stamps CorrelationID on steps in SubmitAsync | After WaitForCompletion, obs.Step.CorrelationID == observer's correlationID |
| UT-SA-1059-009 | Observer with empty correlationID stamps empty string | obs.Step.CorrelationID == "" |

### 4.3 Unit Tests — DS Store Mapping

| ID | Description | Assertion |
|----|-------------|-----------|
| UT-SA-1059-010 | buildEventData for EventTypeShadowLLMRequest produces ShadowLLMRequestPayload | Ogen payload has correct event_type, step_index, prompt_length |
| UT-SA-1059-011 | buildEventData for EventTypeShadowLLMResponse produces ShadowLLMResponsePayload | Ogen payload has correct event_type, token fields |
| UT-SA-1059-012 | AlignmentVerdict payload includes shadow token totals when present | Ogen payload has shadow_prompt_tokens, shadow_completion_tokens, shadow_total_tokens set |
| UT-SA-1059-013 | AlignmentVerdict payload omits shadow tokens when zero | Ogen optional fields are unset when values are 0 |

### 4.4 Unit Tests — Wrapper Token Accumulation

| ID | Description | Assertion |
|----|-------------|-----------|
| UT-SA-1059-014 | Verdict event includes summed shadow tokens from all observations | shadow_total_tokens == sum of all obs.Usage.TotalTokens |
| UT-SA-1059-015 | correlationID passed to NewObserver matches signal identity | Observer's stamped CorrelationID matches signal.RemediationID (or signal.Name fallback) |

### 4.5 Integration Tests

| ID | Description | Assertion |
|----|-------------|-----------|
| IT-SA-1059-001 | Full wrapper flow emits shadow LLM events with correct correlation ID | recordingAuditStore captures shadow.llm.request + shadow.llm.response events per step, with matching correlationID |
| IT-SA-1059-002 | Verdict event in full flow includes aggregate shadow token totals | shadow_total_tokens on verdict == sum of per-step response tokens |

---

## 5. TDD Phase Mapping

### Phase 1: RED (Failing Tests)

Write all test scenarios (UT-SA-1059-001 through UT-SA-1059-015, IT-SA-1059-001/002) referencing types and functions that do not yet exist. Verify compilation fails (`go test -c`).

### Phase 2: GREEN (Minimal Implementation)

Implement production code in dependency order:
1. OpenAPI schemas + ogen regeneration
2. Event type constants (emitter.go)
3. Type changes (types.go: Usage, CorrelationID)
4. Observer changes (correlationID stamping)
5. DS store mapping (buildEventData cases)
6. Evaluator changes (WithAuditStore, per-call emission)
7. Wrapper changes (token accumulation, correlationID propagation)
8. Main wiring (pass auditStore)

Verify all tests pass.

### Phase 3: REFACTOR

- Audit against 100 Go Mistakes
- Run golangci-lint
- Remove dead code
- Verify no regressions

---

## 6. Pass/Fail Criteria

### 6.1 Pass Criteria

- All 17 test scenarios pass (15 unit + 2 integration)
- `go build ./...` succeeds
- `golangci-lint run` reports 0 new issues on changed files
- Pre-existing alignment and audit tests pass without modification
- No NULL-TESTING anti-patterns in new test code

### 6.2 Fail Criteria

- Any test scenario fails
- Build errors in changed files
- New lint violations
- Pre-existing test regressions

---

## 7. Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| NewObserver signature change breaks 25 test call sites | 15% | Medium | Mechanical update; pass empty correlationID for tests that don't exercise audit |
| ogen discriminator ordering sensitivity | 10% | Low | Run `go build ./...` immediately after regeneration |
| Pre-commit hook rejects NULL-TESTING patterns | 20% | Low | Assert behavioral outcomes in all new tests |
| DS must deploy schema before KA | 5% | Medium | Document deployment ordering in PR description |
| Audit volume increase (~30 events/investigation) | N/A | Low | Document for capacity planning; BufferedAuditStore handles batching |
