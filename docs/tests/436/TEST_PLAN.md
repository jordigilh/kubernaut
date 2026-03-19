# Test Plan: LLM Observability Metrics (Issue #436)

**Feature**: Wire LiteLLM success/failure callbacks to emit LLM call, duration, and token usage metrics
**Version**: 1.0
**Created**: 2026-03-18
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/436-llm-metrics-callback`

**Authority**:
- [BR-HAPI-301]: LLM Observability Metrics
- [DD-005 v3.0]: Observability Standards — Metric Name Constants

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../development/business-requirements/TESTING_GUIDELINES.md)
- [Metrics Anti-Pattern](../development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-metrics-method-calls-in-integration-tests)

---

## 1. Scope

### In Scope

- **LiteLLM callback wiring** (`src/metrics/litellm_callback.py`): A `litellm.Callbacks` implementation that extracts provider, model, status, duration, and token usage from each LLM completion and calls `HAMetrics.record_llm_call()`
- **Callback registration** (`src/main.py`): Registering the callback with `litellm.callbacks` at HAPI startup so every LiteLLM call (including those inside HolmesGPT SDK's `investigate_issues`) is instrumented
- **Business outcome validation**: After a real investigation completes, the `/metrics` endpoint reports non-zero values for `aiagent_api_llm_calls_total`, `aiagent_api_llm_call_duration_seconds`, and `aiagent_api_llm_token_usage_total`

### Out of Scope

- **Prometheus client library correctness**: Counter/Histogram `.inc()` and `.observe()` are the library's responsibility
- **`HAMetrics` class internals**: Already tested — metric registration and `record_llm_call()` method are in place and correct
- **Dashboard queries and alerting**: Downstream consumers of the metrics
- **BR-HAPI-302 (HTTP metrics)**: Deferred per GitHub #294

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use `litellm.callbacks` (global callback list) rather than per-call hooks | HolmesGPT SDK calls LiteLLM internally; HAPI has no per-call hook point. `litellm.callbacks` is the only way to intercept all LLM completions including SDK-internal ones. |
| Extract token counts from `response.usage` | LiteLLM normalizes all provider responses into an OpenAI-compatible `Usage` object with `prompt_tokens` and `completion_tokens`. |
| Single callback class covering success + failure | Keeps instrumentation cohesive; `log_success_event` and `log_failure_event` both route to `record_llm_call()` with different `status` labels. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of the callback module (pure logic: label extraction, token parsing, duration calculation, edge cases)
- **Integration**: >=80% of the wiring path (callback registered at startup, metrics emitted after a mock LLM investigation)

### 2-Tier Minimum

Both Unit and Integration tiers are required:
- **Unit tests** validate label extraction logic, token parsing from various response shapes, and graceful handling of missing/malformed `usage` data
- **Integration tests** validate that an end-to-end investigation flow (using Mock LLM) produces non-zero metric samples on the `/metrics` endpoint — testing the full wiring from LiteLLM callback → `HAMetrics` → Prometheus registry

### Business Outcome Quality Bar

Each test validates an **operator-visible outcome**: "After an LLM investigation completes, can I query Prometheus and see accurate token usage, call counts, and latency data for cost tracking and SLO monitoring?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `src/metrics/litellm_callback.py` (new) | `_extract_provider()`, `_extract_model()`, `_extract_tokens()`, `_compute_duration()`, `log_success_event()`, `log_failure_event()` | ~80 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `src/main.py` | Callback registration in `_inject_runtime_env()` or startup | ~5 |
| `src/extensions/incident/llm_integration.py` | `analyze_incident()` — metrics flow through investigation | ~150 |
| `src/metrics/instrumentation.py` | `HAMetrics.record_llm_call()` — called by callback | ~30 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-301 | LLM call counter incremented per LLM call | P0 | Unit | UT-HAPI-301-001 | Pending |
| BR-HAPI-301 | LLM call duration recorded per LLM call | P0 | Unit | UT-HAPI-301-002 | Pending |
| BR-HAPI-301 | Token usage (prompt + completion) extracted from response | P0 | Unit | UT-HAPI-301-003 | Pending |
| BR-HAPI-301 | Provider and model labels correctly extracted | P0 | Unit | UT-HAPI-301-004 | Pending |
| BR-HAPI-301 | Missing/zero token usage handled gracefully | P0 | Unit | UT-HAPI-301-005 | Pending |
| BR-HAPI-301 | Callback errors do not crash the LLM call | P0 | Unit | UT-HAPI-301-006 | Pending |
| BR-HAPI-301 | Failed LLM call records error status metric | P0 | Unit | UT-HAPI-301-007 | Pending |
| BR-HAPI-301 | After investigation, `/metrics` shows non-zero LLM call count | P0 | Integration | IT-HAPI-301-001 | Pending |
| BR-HAPI-301 | After investigation, `/metrics` shows non-zero token usage | P0 | Integration | IT-HAPI-301-002 | Pending |
| BR-HAPI-301 | After investigation, `/metrics` shows LLM call duration histogram | P0 | Integration | IT-HAPI-301-003 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-HAPI-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `HAPI` (HolmesGPT API)
- **BR_NUMBER**: 301 (BR-HAPI-301)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `src/metrics/litellm_callback.py` — 100% of label extraction, token parsing, and error handling logic

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-301-001` | Operator sees `llm_calls_total` incremented with correct provider/model/status labels after each LLM call | Pending |
| `UT-HAPI-301-002` | Operator sees `llm_call_duration_seconds` histogram populated with accurate call latency | Pending |
| `UT-HAPI-301-003` | Operator sees `llm_token_usage_total` with accurate prompt and completion token counts for cost tracking | Pending |
| `UT-HAPI-301-004` | Operator sees correct provider (`vertex_ai`, `openai`, `anthropic`) and model labels regardless of LiteLLM's internal model name formatting | Pending |
| `UT-HAPI-301-005` | Operator sees `llm_calls_total` incremented even when LLM response contains no token usage data (streaming, some providers) — zero tokens recorded, call still counted | Pending |
| `UT-HAPI-301-006` | LLM investigation is not disrupted if the metrics callback raises an unexpected error — callback errors are swallowed with a warning log | Pending |
| `UT-HAPI-301-007` | Operator sees `llm_calls_total{status="error"}` incremented when an LLM call fails (timeout, auth error, rate limit) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Full callback → metrics pipeline wired through a Mock LLM investigation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-HAPI-301-001` | After a complete Mock LLM investigation, operator queries `/metrics` and sees `llm_calls_total >= 1` with correct provider/model labels | Pending |
| `IT-HAPI-301-002` | After a complete Mock LLM investigation, operator queries `/metrics` and sees `llm_token_usage_total` with prompt and completion token counts > 0 | Pending |
| `IT-HAPI-301-003` | After a complete Mock LLM investigation, operator queries `/metrics` and sees `llm_call_duration_seconds` histogram with at least one observation | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. Integration tests with Mock LLM provide sufficient coverage for callback wiring. E2E would require a live LLM provider and adds cost/flakiness without meaningfully different coverage for the metrics plumbing.

---

## 6. Test Cases (Detail)

### UT-HAPI-301-001: LLM call counter incremented with correct labels

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_litellm_metrics_callback.py`

**Given**: A `KubernautLiteLLMCallback` instance with a fresh `HAMetrics` (custom test registry)
**When**: `log_success_event` is called with a LiteLLM response containing `model="vertex_ai/claude-sonnet-4-20250514"`
**Then**: `llm_calls_total{provider="vertex_ai", model="claude-sonnet-4-20250514", status="success"}` equals 1.0

**Acceptance Criteria**:
- Counter incremented exactly once per call
- Provider label extracted correctly from model prefix (e.g. `vertex_ai/...` → `vertex_ai`)
- Model label is the bare model name without provider prefix
- Status label is `success`

---

### UT-HAPI-301-002: LLM call duration recorded accurately

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_litellm_metrics_callback.py`

**Given**: A `KubernautLiteLLMCallback` instance with a fresh `HAMetrics` (custom test registry)
**When**: `log_success_event` is called with `start_time` = T and `end_time` = T + 3.5s
**Then**: `llm_call_duration_seconds` histogram has exactly 1 observation, and `_sum` is approximately 3.5

**Acceptance Criteria**:
- Duration computed from `end_time - start_time` (LiteLLM provides both in kwargs)
- Histogram `_count` equals 1
- Histogram `_sum` is within 0.01 of expected duration

---

### UT-HAPI-301-003: Token usage extracted from response

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_litellm_metrics_callback.py`

**Given**: A `KubernautLiteLLMCallback` instance with a fresh `HAMetrics` (custom test registry)
**When**: `log_success_event` is called with a response containing `usage={"prompt_tokens": 1500, "completion_tokens": 350}`
**Then**: `llm_token_usage_total{type="prompt"}` equals 1500.0 and `llm_token_usage_total{type="completion"}` equals 350.0

**Acceptance Criteria**:
- Prompt tokens extracted from `response.usage.prompt_tokens`
- Completion tokens extracted from `response.usage.completion_tokens`
- Metric values match exactly (counters, not averages)

---

### UT-HAPI-301-004: Provider and model label extraction from various formats

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_litellm_metrics_callback.py`

**Given**: A `KubernautLiteLLMCallback` instance
**When**: Callback receives models in different LiteLLM formats
**Then**: Provider and model labels are normalized correctly

**Acceptance Criteria** (table-driven):

| LiteLLM model string | Expected provider | Expected model |
|---|---|---|
| `vertex_ai/claude-sonnet-4-20250514` | `vertex_ai` | `claude-sonnet-4-20250514` |
| `gpt-4` | `openai` | `gpt-4` |
| `claude-sonnet-4-20250514` | `anthropic` | `claude-sonnet-4-20250514` |
| `openai/llama2` | `openai` | `llama2` |

---

### UT-HAPI-301-005: Missing token usage handled gracefully

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_litellm_metrics_callback.py`

**Given**: A `KubernautLiteLLMCallback` instance with a fresh `HAMetrics` (custom test registry)
**When**: `log_success_event` is called with a response where `usage` is `None` or missing
**Then**: `llm_calls_total{status="success"}` is incremented (call counted), but `llm_token_usage_total` has no samples (zero tokens, not recorded)

**Acceptance Criteria**:
- Call counter still incremented (the LLM call happened)
- Duration still recorded
- Token usage counter NOT incremented (no data to report)
- No exception raised

---

### UT-HAPI-301-006: Callback error does not disrupt LLM call

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_litellm_metrics_callback.py`

**Given**: A `KubernautLiteLLMCallback` instance where the `HAMetrics` instance raises an exception on `.record_llm_call()`
**When**: `log_success_event` is called
**Then**: No exception propagates (callback swallows the error), and a warning is logged

**Acceptance Criteria**:
- Callback catches all exceptions internally
- Warning log emitted with error details
- LiteLLM's completion flow is not interrupted

---

### UT-HAPI-301-007: Failed LLM call records error status

**BR**: BR-HAPI-301
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_litellm_metrics_callback.py`

**Given**: A `KubernautLiteLLMCallback` instance with a fresh `HAMetrics` (custom test registry)
**When**: `log_failure_event` is called with an `AuthenticationError` exception
**Then**: `llm_calls_total{status="error"}` equals 1.0

**Acceptance Criteria**:
- Error status label is `error` (not the exception type)
- Duration still recorded if `start_time` is available
- Token usage NOT recorded (no successful response)

---

### IT-HAPI-301-001: Investigation produces non-zero LLM call metrics

**BR**: BR-HAPI-301
**Type**: Integration
**File**: `holmesgpt-api/tests/integration/test_llm_metrics_integration.py`

**Given**: HAPI running with Mock LLM provider, LiteLLM callback registered, and a custom Prometheus registry
**When**: A complete incident investigation is triggered via `analyze_incident()` and completes successfully
**Then**: Gathering metrics from the registry shows `llm_calls_total >= 1` with provider and model labels matching the Mock LLM configuration

**Acceptance Criteria**:
- Business operation (investigation) drives the metric emission — NOT a direct `record_llm_call()` call
- At least 1 LLM call counted (Mock LLM may produce 1+ round trips)
- Labels match the configured provider and model

---

### IT-HAPI-301-002: Investigation produces non-zero token usage metrics

**BR**: BR-HAPI-301
**Type**: Integration
**File**: `holmesgpt-api/tests/integration/test_llm_metrics_integration.py`

**Given**: HAPI running with Mock LLM provider that returns token usage in responses
**When**: A complete incident investigation completes successfully
**Then**: Gathering metrics shows `llm_token_usage_total{type="prompt"} > 0` and `llm_token_usage_total{type="completion"} > 0`

**Acceptance Criteria**:
- Token counts reflect what the Mock LLM actually returned in `usage`
- Both prompt and completion counters are populated
- Cost tracking query `sum(llm_token_usage_total) by (model, type)` would produce non-zero results

---

### IT-HAPI-301-003: Investigation produces LLM call duration histogram

**BR**: BR-HAPI-301
**Type**: Integration
**File**: `holmesgpt-api/tests/integration/test_llm_metrics_integration.py`

**Given**: HAPI running with Mock LLM provider
**When**: A complete incident investigation completes successfully
**Then**: Gathering metrics shows `llm_call_duration_seconds_count >= 1` and `llm_call_duration_seconds_sum > 0`

**Acceptance Criteria**:
- At least one duration observation recorded
- Duration sum is positive (Mock LLM has non-zero latency)
- Histogram buckets are populated according to the DD-005 bucket schema

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: pytest (HAPI is a Python service)
- **Mocks**: `HAMetrics` with custom `CollectorRegistry` for test isolation (already supported by constructor)
- **Assertions**: Direct registry gathering via `prometheus_client` to read counter/histogram values
- **Location**: `holmesgpt-api/tests/unit/test_litellm_metrics_callback.py`

### Integration Tests

- **Framework**: pytest
- **Mocks**: Mock LLM provider (HAPI's existing Mock LLM infrastructure)
- **Infrastructure**: HAPI app instance with Mock LLM, custom Prometheus registry
- **No direct `record_llm_call()` calls** — tests trigger `analyze_incident()` and verify metrics as a side effect (per [Metrics Anti-Pattern guidelines](../development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-metrics-method-calls-in-integration-tests))
- **Location**: `holmesgpt-api/tests/integration/test_llm_metrics_integration.py`

---

## 8. Execution

```bash
# Unit tests
cd holmesgpt-api && python -m pytest tests/unit/test_litellm_metrics_callback.py -v

# Integration tests
cd holmesgpt-api && python -m pytest tests/integration/test_llm_metrics_integration.py -v

# All HAPI tests
make test-unit-holmesgpt-api
```

---

## 9. Anti-Pattern Compliance Checklist

Per [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md):

| Anti-Pattern | Compliance |
|---|---|
| **Direct Metrics Method Calls** (§ ANTI-PATTERN) | ✅ Integration tests trigger business logic (`analyze_incident`), verify metrics as side effect |
| **Direct Audit Infrastructure Testing** (§ ANTI-PATTERN) | ✅ Not applicable — no audit assertions in these tests |
| **HTTP Testing in Integration** (§ ANTI-PATTERN) | ✅ Not applicable — no HTTP calls; direct function invocation |
| **`time.Sleep()` forbidden** (§ Version 2.0.0) | ✅ No `time.Sleep` — use `Eventually` or direct assertions |
| **`Skip()` forbidden** (§ Version 1.0.0) | ✅ No skipped tests |

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-18 | Initial test plan for Issue #436 |
