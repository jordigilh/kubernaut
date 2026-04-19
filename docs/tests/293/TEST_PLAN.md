# Test Plan: Rename Prometheus Metrics Prefix holmesgpt_api to aiagent_api

**Feature**: Rename all HAPI Prometheus metric names from `holmesgpt_api_*` to `aiagent_api_*` so dashboards and alerts remain stable if the AI agent backend is replaced.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/cve-remediation-kubernaut-agent`

**Authority**:
- Issue [#293](https://github.com/jordigilh/kubernaut/issues/293): Rename HolmesGPT API metrics prefix from holmesgpt_api to aiagent_api
- DD-005: Observability Standards (metric name constants)

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 1. Scope

### In Scope

- **`kubernaut-agent/src/metrics/constants.py`**: Rename 5 metric name string constants from `holmesgpt_api_*` to `aiagent_api_*`
- **`kubernaut-agent/src/metrics/litellm_callback.py`**: No code changes (uses constants, not string literals)
- **`kubernaut-agent/src/metrics/instrumentation.py`**: No code changes (uses constants)
- **Unit tests**: Update hardcoded metric name strings in assertions
- **Integration tests**: Update hardcoded metric name strings in assertions
- **E2E tests**: Update hardcoded metric name strings in assertions
- **Documentation**: Update metric name references in architecture docs and BRs

### Out of Scope

- **OpenAPI client package name** (`holmesgpt_api_client`): Separate concern, not a Prometheus metric
- **Generated client files** (`tests/clients/`): Auto-generated, package name not metrics-related
- **Service rename** (kubernaut-agent directory/module): Separate issue
- **Grafana dashboards / PrometheusRules**: None exist referencing these metrics (verified)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Single constant file change | DD-005 mandates metric names as constants; changing 5 constants in one file propagates everywhere via imports |
| No code changes in instrumentation.py or litellm_callback.py | Both use constants from constants.py, not string literals |
| Tests updated by string replacement | Test assertions reference metric names directly for Prometheus registry queries |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (constants.py at 100%, instrumentation.py at 86%, litellm_callback.py at 93%)
- **Integration**: >=80% of integration-testable code (test_llm_metrics_integration.py covers metrics pipeline)
- **E2E**: Existing E2E tests in `test/e2e/aianalysis/hapi/` and `kubernaut-agent/tests/e2e/`

### 2-Tier Minimum

Every renamed metric is validated at minimum 2 tiers (UT + IT). E2E provides a third defense layer.

### Business Outcome Quality Bar

Tests validate that **operators see correctly named metrics on the /metrics endpoint** and that metric values are accurate after business operations. Tests do NOT test Prometheus counter/histogram infrastructure directly (anti-pattern per TESTING_GUIDELINES.md).

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `kubernaut-agent/src/metrics/constants.py` | Metric name constants | ~20 |
| `kubernaut-agent/src/metrics/litellm_callback.py` | `_extract_provider_model`, `log_success_event`, `log_failure_event` | ~80 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `kubernaut-agent/src/metrics/instrumentation.py` | `HAMetrics.__init__`, `record_llm_call`, `record_investigation_complete` | ~80 |
| `kubernaut-agent/src/main.py` | `startup_event` (callback registration) | ~10 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #293 | Metric prefix is implementation-agnostic (aiagent_api_) | P1 | Unit | UT-HAPI-293-001 | Pending |
| #293 | Metric prefix is implementation-agnostic (aiagent_api_) | P1 | Unit | UT-HAPI-293-002 | Pending |
| #293 | Metric prefix is implementation-agnostic (aiagent_api_) | P1 | Unit | UT-HAPI-293-003 | Pending |
| #293 | Metric prefix is implementation-agnostic (aiagent_api_) | P1 | Unit | UT-HAPI-293-004 | Pending |
| #293 | Metric prefix is implementation-agnostic (aiagent_api_) | P1 | Integration | IT-HAPI-293-001 | Pending |
| #293 | Metric prefix is implementation-agnostic (aiagent_api_) | P1 | Integration | IT-HAPI-293-002 | Pending |
| #293 | Metric prefix is implementation-agnostic (aiagent_api_) | P1 | E2E | E2E-HAPI-293-001 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Blast Radius Analysis

### Source Code (5 metric constants in 1 file)

The rename is concentrated in `kubernaut-agent/src/metrics/constants.py`, which defines all 5 active metric names:

| Current Name | New Name |
|-------------|----------|
| `holmesgpt_api_investigations_total` | `aiagent_api_investigations_total` |
| `holmesgpt_api_investigations_duration_seconds` | `aiagent_api_investigations_duration_seconds` |
| `holmesgpt_api_llm_calls_total` | `aiagent_api_llm_calls_total` |
| `holmesgpt_api_llm_call_duration_seconds` | `aiagent_api_llm_call_duration_seconds` |
| `holmesgpt_api_llm_token_usage_total` | `aiagent_api_llm_token_usage_total` |

No other source files need changes because `instrumentation.py` and `litellm_callback.py` reference the constants, not string literals.

### Test Files (string literal updates)

Tests query the Prometheus registry using raw metric name strings. These must be updated:

| File | Matches | Category |
|------|---------|----------|
| `kubernaut-agent/tests/unit/test_litellm_metrics_callback.py` | 11 | Unit test assertions |
| `kubernaut-agent/tests/integration/test_llm_metrics_integration.py` | 7 | Integration test assertions |
| `kubernaut-agent/tests/e2e/test_audit_pipeline_e2e.py` | 3 | E2E test assertions |
| `kubernaut-agent/tests/e2e/test_workflow_selection_e2e.py` | 7 | E2E test assertions |
| `kubernaut-agent/tests/e2e/test_mock_llm_edge_cases_e2e.py` | 3 | E2E test assertions |
| `test/e2e/aianalysis/hapi/test_mock_llm_mode_e2e.py` | 4 | E2E test assertions |
| `test/e2e/aianalysis/hapi/test_custom_labels_e2e.py` | 4 | E2E test assertions |

### Documentation (reference updates)

| File | Matches | Category |
|------|---------|----------|
| `docs/architecture/HOLMESGPT_REST_API_ARCHITECTURE.md` | 12 | Architecture doc |
| `docs/architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md` | 2 | Design decision |
| `docs/services/stateless/kubernaut-agent/BUSINESS_REQUIREMENTS.md` | 10 | Business requirements |
| `docs/requirements/BR-HAPI-189-PHASE2-IMPLEMENTATION-TEMPLATES.md` | 2 | Implementation templates |
| `docs/tests/436/TEST_PLAN.md` | 1 | Test plan |
| `kubernaut-agent/tests/load/README.md` | 6 | Load test documentation |
| `test/e2e/aianalysis/hapi/README.md` | 1 | E2E README |

### NOT Affected (verified)

- `kubernaut-agent/src/metrics/instrumentation.py`: Uses constant imports, no string literals
- `kubernaut-agent/src/metrics/litellm_callback.py`: Uses HAMetrics methods, no metric name strings
- `kubernaut-agent/src/main.py`: No metric name strings
- `kubernaut-agent/src/middleware/metrics.py`: All business metrics already removed (GitHub #294)
- Grafana dashboards: None exist
- PrometheusRule YAML: None reference these metrics
- OpenAPI client (`tests/clients/`): Package name `holmesgpt_api_client`, not metric names

### Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Missed string literal in test file | Low | Global `rg` search + full test suite run catches any mismatch |
| Breaking existing monitoring | None | No dashboards or alerting rules exist for these metrics |
| Pre-production metric consumers | None | Service is pre-production (v1.1.0-rc) |

---

## 6. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-HAPI-293-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `constants.py` (100%), `litellm_callback.py` (93%), `instrumentation.py` (86%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-293-001` | Operator sees `aiagent_api_llm_calls_total` (not `holmesgpt_api_`) on /metrics after a successful LLM call, ensuring metric names are implementation-agnostic | Pending |
| `UT-HAPI-293-002` | Operator sees `aiagent_api_llm_token_usage_total` with correct prompt/completion token counts, proving cost tracking metrics use the new prefix | Pending |
| `UT-HAPI-293-003` | Operator sees `aiagent_api_llm_call_duration_seconds` histogram populated, confirming latency metrics use the new prefix | Pending |
| `UT-HAPI-293-004` | Operator sees `aiagent_api_llm_calls_total{status='error'}` when an LLM call fails, confirming error metrics use the new prefix | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `instrumentation.py` HAMetrics through `analyze_incident()` pipeline

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-HAPI-293-001` | Operator sees `aiagent_api_investigations_total{status='success'}` after a complete investigation, confirming the full pipeline emits investigation metrics with the new prefix | Pending |
| `IT-HAPI-293-002` | Operator sees `aiagent_api_investigations_duration_seconds` histogram populated after investigation, confirming duration metrics use the new prefix | Pending |

### Tier 3: E2E Tests (existing, updated)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-HAPI-293-001` | Existing E2E tests that assert metric names continue to pass with the new `aiagent_api_` prefix | Pending |

### Tier Skip Rationale

- None skipped. All 3 tiers covered.

---

## 7. Test Cases (Detail)

### UT-HAPI-293-001: LLM call counter uses new prefix

**BR**: #293
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_litellm_metrics_callback.py`

**Given**: KubernautLiteLLMCallback wired to isolated HAMetrics
**When**: `log_success_event` is called with a model response
**Then**: `aiagent_api_llm_calls_total{provider=..., model=..., status='success'}` is incremented to 1.0

**Acceptance Criteria**:
- Counter value is exactly 1.0 for matching labels
- No metric with `holmesgpt_api_` prefix exists in the registry

---

### UT-HAPI-293-002: Token usage counter uses new prefix

**BR**: #293
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_litellm_metrics_callback.py`

**Given**: KubernautLiteLLMCallback wired to isolated HAMetrics
**When**: `log_success_event` is called with usage containing 1500 prompt + 350 completion tokens
**Then**: `aiagent_api_llm_token_usage_total{type='prompt'}` == 1500, `{type='completion'}` == 350

**Acceptance Criteria**:
- Prompt and completion token values match exactly
- Token type label is present and correct

---

### UT-HAPI-293-003: Duration histogram uses new prefix

**BR**: #293
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_litellm_metrics_callback.py`

**Given**: KubernautLiteLLMCallback with known start/end timestamps (3.5s apart)
**When**: `log_success_event` is called
**Then**: `aiagent_api_llm_call_duration_seconds_count` == 1.0 and `_sum` approx 3.5

**Acceptance Criteria**:
- Histogram count and sum values are accurate
- Duration within 0.01s tolerance

---

### UT-HAPI-293-004: Error counter uses new prefix

**BR**: #293
**Type**: Unit
**File**: `kubernaut-agent/tests/unit/test_litellm_metrics_callback.py`

**Given**: KubernautLiteLLMCallback wired to isolated HAMetrics
**When**: `log_failure_event` is called with an exception
**Then**: `aiagent_api_llm_calls_total{status='error'}` is incremented to 1.0

**Acceptance Criteria**:
- Error counter incremented
- Duration histogram also incremented (failures still record latency)

---

### IT-HAPI-293-001: Investigation success metric uses new prefix

**BR**: #293
**Type**: Integration
**File**: `kubernaut-agent/tests/integration/test_llm_metrics_integration.py`

**Given**: Isolated HAMetrics and mocked HolmesGPT SDK
**When**: `analyze_incident()` completes successfully
**Then**: `aiagent_api_investigations_total{status='success'}` >= 1.0

**Acceptance Criteria**:
- Investigation counter incremented through full business logic path
- Metrics recorded as side effect of business operation (not direct method call)

---

### IT-HAPI-293-002: Investigation duration metric uses new prefix

**BR**: #293
**Type**: Integration
**File**: `kubernaut-agent/tests/integration/test_llm_metrics_integration.py`

**Given**: Isolated HAMetrics and mocked HolmesGPT SDK
**When**: `analyze_incident()` completes
**Then**: `aiagent_api_investigations_duration_seconds_count` >= 1.0

**Acceptance Criteria**:
- Duration histogram populated through business logic
- Value is non-negative and represents wall-clock time

---

## 8. Test Infrastructure

### Unit Tests

- **Framework**: pytest (Python, mandatory for HAPI)
- **Mocks**: `prometheus_client.CollectorRegistry` per test for isolation
- **Location**: `kubernaut-agent/tests/unit/test_litellm_metrics_callback.py`

### Integration Tests

- **Framework**: pytest with `pytest-asyncio` (Python, mandatory for HAPI)
- **Mocks**: HolmesGPT SDK `investigate_issues` (external dependency), isolated CollectorRegistry
- **Infrastructure**: None required (direct business logic calls)
- **Location**: `kubernaut-agent/tests/integration/test_llm_metrics_integration.py`

---

## 9. Execution

```bash
# Unit tests (containerized)
make test-unit-kubernaut-agent

# Specific test by pattern
cd kubernaut-agent && podman run --rm \
    -v $(pwd)/..:/workspace:z \
    -w /workspace/kubernaut-agent \
    registry.access.redhat.com/ubi10/python-312-minimal:latest \
    sh -c "pip install -q -r requirements.txt && pip install -q -r requirements-test.txt && \
           pytest tests/unit/test_litellm_metrics_callback.py -v -k 'hapi_293' -o addopts=''"
```

---

## 10. Execution Plan

### Step 1: RED -- Update metric name constants

Change the 5 constants in `kubernaut-agent/src/metrics/constants.py` from `holmesgpt_api_*` to `aiagent_api_*`. Existing tests will fail because their assertions use the old prefix string literals.

**Verify RED**: `make test-unit-kubernaut-agent` -- all metric-related tests fail.

### Step 2: GREEN -- Update test assertions

Replace `holmesgpt_api_` with `aiagent_api_` in all test files (unit, integration, E2E) that assert metric names via Prometheus registry queries.

**Verify GREEN**: `make test-unit-kubernaut-agent` -- all 630+ tests pass.

### Step 3: REFACTOR -- Update documentation

Replace `holmesgpt_api_` with `aiagent_api_` in all documentation files that reference metric names.

### Step 4: Commit and push

---

## 11. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
