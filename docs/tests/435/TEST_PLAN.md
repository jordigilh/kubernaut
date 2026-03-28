# Test Plan: Wire LLM Token Usage into Audit Traces

**Feature**: Enrich HAPI audit events with per-session LLM token usage
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `development/v1.2`

**Authority**:
- DD-AUDIT-003: Service audit trace requirements (HAPI = MUST)
- DD-AUDIT-005: Hybrid provider data capture
- ADR-034: Unified audit table design
- BR-HAPI-301: LLM observability metrics
- BR-HAPI-195: Cost tracking metrics (token counts; USD deferred to v2.0)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- GitHub Issue: [#435](https://github.com/jordigilh/kubernaut/issues/435)

---

## 1. Scope

### In Scope

- **TokenAccumulator** (`src/metrics/token_accumulator.py`): Session-scoped accumulator that sums prompt and completion tokens across all LiteLLM round-trips within one investigation
- **Audit event enrichment** (`src/audit/events.py`): `create_aiagent_response_complete_event` gains `total_prompt_tokens` / `total_completion_tokens`; `create_llm_response_event` wires existing `tokens_used` field
- **LiteLLM callback integration** (`src/metrics/litellm_callback.py`): `log_success_event` reads ContextVar and accumulates tokens
- **Investigation wiring** (`src/extensions/incident/endpoint.py`, `src/extensions/investigation_helpers.py`): ContextVar lifecycle and token data flow to audit events

### Out of Scope

- AA controller changes (tokens are HAPI-internal)
- CRD schema changes (`.status` reflects reconciliation state, not analytics)
- HAPI HTTP API response changes (tokens are audit-only data)
- USD cost calculation (deferred to v2.0 per BR-HAPI-195)

### Design Decisions

- Session-scoped `ContextVar` chosen over threading accumulator through `analyze_incident` to avoid modifying the SDK integration signature
- Token fields added to `AIAgentResponsePayload` (audit envelope), not `IncidentResponseData` (API response), because tokens are audit metadata
- `tokens_used` on `aiagent.llm.response` populated alongside session totals on `aiagent.response.complete` for dual-granularity audit trail

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (TokenAccumulator logic, audit event factories, callback accumulator path)
- **Integration**: >=80% of integration-testable code (endpoint wiring, investigation helpers token passing)

### 2-Tier Minimum

Every business requirement gap is covered by Unit + Integration tiers:
- **Unit tests** validate accumulator correctness, audit event structure, callback behavior in isolation
- **Integration tests** validate end-to-end wiring from investigation through audit event emission

### Business Outcome Quality Bar

Tests validate business outcomes -- "operator sees token usage in audit trail for cost attribution" -- not just "function is called." Each test asserts measurable data accuracy (exact token counts, correct field names, ADR-034 compliance).

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `src/metrics/token_accumulator.py` (NEW) | `add()`, `prompt_tokens`, `completion_tokens`, `total()` | ~30 |
| `src/audit/events.py` | `create_aiagent_response_complete_event`, `create_llm_response_event` | ~100 |
| `src/metrics/litellm_callback.py` | `log_success_event` (accumulator code path) | ~25 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `src/extensions/incident/endpoint.py` | `_run_incident_investigation` (ContextVar lifecycle) | ~30 |
| `src/extensions/investigation_helpers.py` | `audit_llm_response_and_tools` (tokens_used param) | ~20 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| DD-AUDIT-003 | Token accumulation correctness | P0 | Unit | UT-HAPI-435-001 | Pending |
| DD-AUDIT-003 | Multi-call token accumulation | P0 | Unit | UT-HAPI-435-002 | Pending |
| DD-AUDIT-003 | Zero-token call handling | P1 | Unit | UT-HAPI-435-003 | Pending |
| DD-AUDIT-003 | Total token count accuracy | P0 | Unit | UT-HAPI-435-004 | Pending |
| DD-AUDIT-003 | Session token fields in response.complete event | P0 | Unit | UT-HAPI-435-005 | Pending |
| ADR-034 | Backward compat when tokens not provided | P0 | Unit | UT-HAPI-435-006 | Pending |
| DD-AUDIT-003 | Per-call tokens_used in llm.response event | P0 | Unit | UT-HAPI-435-007 | Pending |
| ADR-034 | Existing tokens_used=None behavior preserved | P1 | Unit | UT-HAPI-435-008 | Pending |
| BR-HAPI-301 | Callback accumulates via ContextVar | P0 | Unit | UT-HAPI-435-009 | Pending |
| BR-HAPI-301 | Callback works without accumulator | P0 | Unit | UT-HAPI-435-010 | Pending |
| BR-HAPI-301 | Accumulator error does not disrupt metrics | P0 | Unit | UT-HAPI-435-011 | Pending |
| DD-AUDIT-003 | Per-call delta across self-correction retry loop | P0 | Unit | UT-HAPI-435-012 | Pending |
| DD-AUDIT-003 | Full flow: investigation emits token-enriched audit | P0 | Integration | IT-HAPI-435-001 | Pending |
| DD-AUDIT-003 | Full flow: llm.response contains tokens_used | P0 | Integration | IT-HAPI-435-002 | Pending |
| DD-AUDIT-003 | Multi-attempt: per-call deltas + session totals correct | P0 | Integration | IT-HAPI-435-003 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-HAPI-435-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: HAPI (HolmesGPT API)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `token_accumulator.py` (100%), `events.py` token paths (~80%), `litellm_callback.py` accumulator path (~80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-435-001` | Operator querying audit sees zero token counts for investigations with no LLM calls | Pending |
| `UT-HAPI-435-002` | Operator sees accurate cumulative token counts across multiple LLM round-trips in one investigation | Pending |
| `UT-HAPI-435-003` | Investigations with LLM calls returning zero tokens do not produce errors or corrupt counts | Pending |
| `UT-HAPI-435-004` | Operator can compute total tokens (prompt + completion) from audit data for chargeback | Pending |
| `UT-HAPI-435-005` | Audit event `aiagent.response.complete` includes `total_prompt_tokens` and `total_completion_tokens` for cost attribution | Pending |
| `UT-HAPI-435-006` | Existing audit events without token data remain valid and ADR-034 compliant (no regression) | Pending |
| `UT-HAPI-435-007` | Audit event `aiagent.llm.response` includes `tokens_used` for per-call visibility | Pending |
| `UT-HAPI-435-008` | Existing `aiagent.llm.response` events with `tokens_used=None` continue to work (no regression) | Pending |
| `UT-HAPI-435-009` | LLM callback automatically accumulates token usage into session-scoped accumulator | Pending |
| `UT-HAPI-435-010` | LLM callback operates normally for legacy code paths without a session accumulator | Pending |
| `UT-HAPI-435-011` | Token accumulation failure does not disrupt LLM metrics recording or investigation flow | Pending |
| `UT-HAPI-435-012` | Snapshot/delta approach produces correct per-call tokens_used across self-correction retry loop | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `endpoint.py` ContextVar lifecycle (~80%), `investigation_helpers.py` token wiring (~80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-HAPI-435-001` | End-to-end investigation emits `aiagent.response.complete` audit event with accurate accumulated token totals | Pending |
| `IT-HAPI-435-002` | End-to-end investigation emits `aiagent.llm.response` audit event with `tokens_used` populated | Pending |
| `IT-HAPI-435-003` | Multi-attempt investigation produces correct per-call deltas and correct session totals | Pending |

### Tier Skip Rationale

- **E2E**: Token usage enrichment is a data-passthrough feature with no K8s/cluster interactions. Unit + Integration tiers provide sufficient coverage. E2E would require Kind + DataStorage + Mock LLM infrastructure without adding meaningful coverage beyond integration for this pure data-enrichment feature.

---

## 6. Test Cases (Detail)

### UT-HAPI-435-001: Zero totals when no calls

**BR**: DD-AUDIT-003
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: A freshly created TokenAccumulator with no `add()` calls
**When**: `prompt_tokens`, `completion_tokens`, and `total()` are read
**Then**: All return 0

**Acceptance Criteria**:
- `accumulator.prompt_tokens == 0`
- `accumulator.completion_tokens == 0`
- `accumulator.total() == 0`

### UT-HAPI-435-002: Multi-call accumulation

**BR**: DD-AUDIT-003
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: A TokenAccumulator
**When**: `add(150, 80)` then `add(200, 120)` are called
**Then**: Totals reflect the sum of all calls

**Acceptance Criteria**:
- `accumulator.prompt_tokens == 350`
- `accumulator.completion_tokens == 200`
- `accumulator.total() == 550`

### UT-HAPI-435-003: Zero-token calls handled

**BR**: DD-AUDIT-003
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: A TokenAccumulator with existing totals from `add(100, 50)`
**When**: `add(0, 0)` is called
**Then**: Totals remain unchanged, no error raised

**Acceptance Criteria**:
- `accumulator.prompt_tokens == 100`
- `accumulator.completion_tokens == 50`
- No exception raised

### UT-HAPI-435-004: Total returns combined count

**BR**: DD-AUDIT-003
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: A TokenAccumulator after `add(500, 200)`
**When**: `total()` is called
**Then**: Returns 700 (prompt + completion)

**Acceptance Criteria**:
- `accumulator.total() == 700`
- `accumulator.total() == accumulator.prompt_tokens + accumulator.completion_tokens`

### UT-HAPI-435-005: Response complete event with tokens

**BR**: DD-AUDIT-003
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: Valid incident_id, remediation_id, response_data, and token counts
**When**: `create_aiagent_response_complete_event(..., total_prompt_tokens=350, total_completion_tokens=200)` is called
**Then**: Audit event contains token fields in event_data

**Acceptance Criteria**:
- `event.event_data.actual_instance.total_prompt_tokens == 350`
- `event.event_data.actual_instance.total_completion_tokens == 200`
- ADR-034 envelope fields present (version, event_category, event_type, etc.)

### UT-HAPI-435-006: Response complete event backward compatibility

**BR**: ADR-034
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: Valid incident_id, remediation_id, response_data, NO token counts
**When**: `create_aiagent_response_complete_event(...)` is called without token params
**Then**: Event is valid, token fields are None/absent

**Acceptance Criteria**:
- Event passes ADR-034 envelope validation
- `event.event_data.actual_instance.total_prompt_tokens is None`
- `event.event_data.actual_instance.total_completion_tokens is None`

### UT-HAPI-435-007: LLM response event with tokens_used

**BR**: DD-AUDIT-003
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: Valid LLM response data and token count
**When**: `create_llm_response_event(..., tokens_used=550)` is called (note: tokens_used is an existing unused param)
**Then**: Event contains `tokens_used` in event_data

**Acceptance Criteria**:
- `event.event_data.actual_instance.tokens_used == 550`

### UT-HAPI-435-008: LLM response event tokens_used=None preserved

**BR**: ADR-034
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: Valid LLM response data, no token count
**When**: `create_llm_response_event(...)` is called without tokens_used (default None)
**Then**: Event is valid, tokens_used is None

**Acceptance Criteria**:
- `event.event_data.actual_instance.tokens_used is None`

### UT-HAPI-435-009: Callback accumulates via ContextVar

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: A TokenAccumulator set in the ContextVar, and a KubernautLiteLLMCallback
**When**: `log_success_event` is called with usage(prompt=150, completion=80)
**Then**: Accumulator reflects the tokens

**Acceptance Criteria**:
- `accumulator.prompt_tokens == 150`
- `accumulator.completion_tokens == 80`
- Prometheus metrics also recorded (existing behavior unchanged)

### UT-HAPI-435-010: Callback works without accumulator

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: No TokenAccumulator in ContextVar (ContextVar is empty/None)
**When**: `log_success_event` is called with usage(prompt=100, completion=50)
**Then**: No error raised, Prometheus metrics still recorded

**Acceptance Criteria**:
- No exception raised
- Prometheus `llm_calls_total` incremented
- Prometheus `llm_token_usage_total` incremented

### UT-HAPI-435-011: Accumulator error swallowed

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: A broken accumulator (add() raises RuntimeError) set in ContextVar
**When**: `log_success_event` is called
**Then**: No exception propagated, Prometheus metrics still recorded

**Acceptance Criteria**:
- No exception raised to caller
- Prometheus `llm_calls_total` incremented (metrics not disrupted)

### IT-HAPI-435-001: Full flow token-enriched response.complete

**BR**: DD-AUDIT-003
**Type**: Integration
**File**: `holmesgpt-api/tests/integration/test_token_usage_integration.py`

**Given**: Mock LLM returning canned response with usage(prompt=500, completion=200), mock audit store capturing events
**When**: `_run_incident_investigation` completes a full investigation
**Then**: Captured `aiagent.response.complete` audit event has token totals

**Acceptance Criteria**:
- Audit event with `event_type == "aiagent.response.complete"` captured
- `event.event_data.actual_instance.total_prompt_tokens == 500`
- `event.event_data.actual_instance.total_completion_tokens == 200`

### IT-HAPI-435-002: Full flow tokens_used on llm.response

**BR**: DD-AUDIT-003
**Type**: Integration
**File**: `holmesgpt-api/tests/integration/test_token_usage_integration.py`

**Given**: Mock LLM returning canned response with usage(prompt=500, completion=200), mock audit store capturing events
**When**: `audit_llm_response_and_tools` is called with accumulated token total
**Then**: Captured `aiagent.llm.response` audit event has tokens_used

**Acceptance Criteria**:
- Audit event with `event_type == "aiagent.llm.response"` captured
- `event.event_data.actual_instance.tokens_used == 700`

### UT-HAPI-435-012: Per-call delta across retries

**BR**: DD-AUDIT-003
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

**Given**: A TokenAccumulator, a KubernautLiteLLMCallback, and 3 LLM calls with usage (300,150), (200,80), (100,70)
**When**: For each call, snapshot `acc.total()` before, fire callback, compute `acc.total() - snapshot`
**Then**: Per-call deltas are [450, 280, 170] and session total is 900

**Acceptance Criteria**:
- Per-call deltas match expected values
- `accumulator.prompt_tokens == 600`
- `accumulator.completion_tokens == 300`
- `accumulator.total() == 900`

### IT-HAPI-435-003: Multi-attempt per-call deltas and session totals

**BR**: DD-AUDIT-003
**Type**: Integration
**File**: `holmesgpt-api/tests/integration/test_token_usage_integration.py`

**Given**: 2-attempt investigation with usage (300,150) and (200,100)
**When**: Each attempt emits `aiagent.llm.response` with per-call delta, then `aiagent.response.complete` with session totals
**Then**: Per-call deltas are 450 and 300; session totals are prompt=500, completion=250

**Acceptance Criteria**:
- `llm_response_events[0].tokens_used == 450`
- `llm_response_events[1].tokens_used == 300`
- `complete_event.total_prompt_tokens == 500`
- `complete_event.total_completion_tokens == 250`

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: pytest (HAPI Python convention)
- **Mocks**: `unittest.mock.MagicMock` for LiteLLM response objects, `CollectorRegistry` for isolated Prometheus metrics
- **Location**: `holmesgpt-api/tests/unit/test_token_usage_audit.py`

### Integration Tests

- **Framework**: pytest
- **Mocks**: Mock LLM (always mocked per testing guidelines), mock audit store (capture emitted events)
- **Infrastructure**: No external services required
- **Location**: `holmesgpt-api/tests/integration/test_token_usage_integration.py`

---

## 8. Execution

```bash
# Unit tests
cd holmesgpt-api && python -m pytest tests/unit/test_token_usage_audit.py -v

# Integration tests
cd holmesgpt-api && python -m pytest tests/integration/test_token_usage_integration.py -v

# All HAPI tests
cd holmesgpt-api && python -m pytest tests/unit/ tests/integration/ -v

# Specific test by ID
cd holmesgpt-api && python -m pytest tests/unit/test_token_usage_audit.py -v -k "UT_HAPI_435_001"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
