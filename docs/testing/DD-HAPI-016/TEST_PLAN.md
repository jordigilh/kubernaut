# DD-HAPI-016 Test Plan: Remediation History Context

**Version**: 1.1.0
**Created**: 2026-02-14
**Updated**: 2026-02-14
**Status**: Active
**BR**: BR-HAPI-016 (Remediation History Context for LLM Prompt Enrichment)
**DD**: DD-HAPI-016 v1.1 (Two-step query with EM scoring), DD-EM-002 v1.1 (spec_drift)

---

## Overview

This test plan defines the integration and E2E test scenarios for the remediation
history feature across DataStorage (DS) and HolmesGPT API (HAPI). Unit tests are
already in place (21 Ginkgo DS + 28 pytest HAPI). This plan targets the remaining
integration and E2E tiers to reach >=80% per-tier coverage.

**Anti-Pattern Compliance** (TESTING_GUIDELINES.md v2.7.0):
- No `time.Sleep()` -- all async waits use `Eventually()`
- No `Skip()` -- tests fail or pass, never skip
- No direct audit/metrics infrastructure testing
- No HTTP in integration tests -- integration tests use direct business logic calls;
  HTTP testing is E2E-only (per anti-pattern: HTTP Testing in Integration Tests)
- No UT/IT duplication -- integration tests validate component coordination,
  not the same mock patterns as unit tests

**Reference Documents**:
- [DD-HAPI-016 v1.1](../../architecture/decisions/DD-HAPI-016-remediation-history-context.md)
- [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-016-{SEQUENCE}`

- `IT-DS-016-NNN` -- DataStorage integration tests (real PostgreSQL, direct business logic)
- `E2E-DS-016-NNN` -- DataStorage E2E tests (Kind cluster, HTTP)
- `IT-HAPI-016-NNN` -- HolmesGPT API integration tests (direct function calls)

---

## Code Surface by Tier

### DS (Go)

| Tier | Files | LOC | Coverage Before | Target |
|------|-------|-----|-----------------|--------|
| Unit | `remediation_history_logic.go`, `effectiveness_handler.go` | ~380 | ~95% (21 tests) | >=80% MET |
| Integration | `remediation_history_repository.go`, `remediation_history_adapter.go`, correlation pipeline | ~340 | 0% | >=80% |
| E2E | Full stack (server wiring, routing, PostgreSQL, HTTP) | ~230 handler + wiring | 0% | >=80% |

### HAPI (Python)

| Tier | Files | Functions | Coverage Before | Target |
|------|-------|-----------|-----------------|--------|
| Unit | `remediation_history_prompt.py`, prompt builders | 13 | ~95% (28 tests) | >=80% MET |
| Integration | `remediation_history_client.py`, incident/recovery wiring | 5 | ~40% | >=80% |

---

## 1. DS Integration Tests

**File**: `test/integration/datastorage/remediation_history_integration_test.go`
**Infrastructure**: PostgreSQL container via `suite_test.go` (`db`, `logger`)
**Pattern**: Direct business logic calls with real DB (NO httptest, NO HTTP)

### 1.1 Repository Layer

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-DS-016-001 | Insert RO audit events for a target, query via `QueryROEventsByTarget`, verify rows filtered by `target_resource` JSONB field and `since` timestamp | DS retrieves correct remediation chain for a target resource | `repo.QueryROEventsByTarget` (~40 LOC) |
| IT-DS-016-002 | Insert EM audit events for 3 correlation IDs, query via `QueryEffectivenessEventsBatch`, verify correct grouping by `correlation_id` | DS batches EM component events for scoring | `repo.QueryEffectivenessEventsBatch` (~50 LOC) |
| IT-DS-016-003 | Insert RO events with `pre_remediation_spec_hash`, query via `QueryROEventsBySpecHash` with hash + time range, verify Tier 2 widening results | DS supports historical hash lookup for broader context | `repo.QueryROEventsBySpecHash` (~40 LOC) |

### 1.2 Adapter Layer

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-DS-016-004 | Insert EM events, query through adapter, verify all 5 EM component events available with complete hash data (pre + post) | EM scoring data (health, metrics, hash) is complete after repository-to-server adapter boundary | `adapter.QueryEffectivenessEventsBatch` (~20 LOC) |

### 1.3 Orchestration Pipeline (Direct Business Logic + Real DB)

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-DS-016-005 | Insert RO + full EM events (reason=full), call `QueryROEventsByTarget` -> `QueryEffectivenessEventsBatch` -> `CorrelateTier1Chain`, verify `assessmentReason=full` and positive weighted score on entry struct | **LLM receives accurate effectiveness data from real DB** | Adapter + repo + correlation pipeline (~120 LOC) |
| IT-DS-016-006 | Same pipeline with EM events having `reason=spec_drift`, verify `assessmentReason=spec_drift` and `effectivenessScore=0.0` on entry struct | **LLM correctly informed that spec_drift != failure** | spec_drift code path (~15 LOC) |
| IT-DS-016-007 | Insert events where `current_spec_hash` matches `pre_remediation_spec_hash`, call `CorrelateTier1Chain` + `DetectRegression`, verify `regressionDetected=true` and `hashMatch=preRemediation` | **LLM warned about configuration regression** | Regression detection path (~15 LOC) |
| IT-DS-016-008 | Query for non-existent target via adapter, call `CorrelateTier1Chain`, verify empty entries and no regression | Empty history gracefully handled (no errors, no false positives) | Empty result path (~10 LOC) |

**Coverage estimate**: 8 tests cover ~310 of ~340 integration-testable LOC = **~91%**.

---

## 2. DS E2E Tests

**File**: `test/e2e/datastorage/25_remediation_history_api_test.go`
**Infrastructure**: Kind cluster with DS service running
**Pattern**: HTTP API calls via `AuthHTTPClient` (HTTP is correct in E2E tier)

| ID | Scenario | Business Outcome |
|----|----------|------------------|
| E2E-DS-016-001 | Write RO + EM audit events via direct DB, query `GET /remediation-history/context`, verify response has `tier1.chain` with `assessmentReason`, `effectivenessScore`, `healthChecks`, `metricDeltas` | Complete remediation context flows through real service |
| E2E-DS-016-002 | Write EM events with `reason=spec_drift`, query, verify `assessmentReason=spec_drift` and `effectivenessScore=0.0` | spec_drift semantics survive full service stack |
| E2E-DS-016-003 | Query for non-existent target, verify 200 OK with empty chains | Graceful empty handling in production-like environment |
| E2E-DS-016-004 | Write events for multiple correlation IDs with mixed reasons (full, spec_drift, partial), verify all appear with correct `assessmentReason` | Multi-entry mixed-reason correctness |
| E2E-DS-016-005 | Query with `tier1Window=not-a-duration`, verify 400 Bad Request | Malformed parameters rejected cleanly by deployed service |

---

## 3. HAPI Integration Tests

**File**: `holmesgpt-api/tests/integration/test_remediation_history_integration.py`
**Infrastructure**: Direct function calls, env patching, mock pool manager
**Pattern**: Same as `test_llm_prompt_business_logic.py`

### 3.1 Client Factory

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-HAPI-016-001 | `create_remediation_history_api` with `DATA_STORAGE_URL` env var + mocked pool manager: returns configured API instance | HAPI connects to DS when configured | `create_remediation_history_api` happy path (~25 LOC) |
| IT-HAPI-016-002 | `create_remediation_history_api` with no DS URL: returns None | Graceful skip when DS not configured | None path (~5 LOC) |
| IT-HAPI-016-003 | `create_remediation_history_api` with pool manager import error: returns None | Resilience when auth pool unavailable | Exception path (~10 LOC) |

### 3.2 End-to-End Wiring (fetch -> build prompt)

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-HAPI-016-004 | Full pipeline: `fetch_remediation_history_for_request` (mock API returning spec_drift) -> `create_incident_investigation_prompt`: verify prompt contains INCONCLUSIVE and suppresses 0.0 score | **spec_drift flows from DS query through to LLM prompt as INCONCLUSIVE** | Client + prompt builder coordination (~45 LOC) |
| IT-HAPI-016-005 | Full pipeline: `fetch_remediation_history_for_request` (ConnectionError) -> `create_incident_investigation_prompt`: verify prompt is valid without history section | **LLM analysis continues even if DS is down (graceful degradation)** | Client error path + prompt fallback (~20 LOC) |

### 3.3 Prompt Wiring

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-HAPI-016-006 | `create_incident_investigation_prompt` with remediation history containing spec_drift: prompt includes "INCONCLUSIVE" and spec drift guidance | **LLM correctly distinguishes spec_drift from failure** | Incident prompt wiring (~15 LOC) |
| IT-HAPI-016-007 | `_create_recovery_investigation_prompt` with remediation history: prompt includes remediation section | Recovery flow receives same enrichment as incident | Recovery prompt wiring (~15 LOC) |

**Coverage estimate**: 7 tests cover ~135 of ~135 integration-testable LOC = **~100%**.

---

## 4. Compliance Summary

| Tier | Service | Tests | Est. Coverage | Target | Status |
|------|---------|-------|---------------|--------|--------|
| Unit | DS | 21 Ginkgo | ~95% | >=80% | PASS |
| Unit | HAPI | 28 pytest | ~95% | >=80% | PASS |
| Integration | DS | 8 Ginkgo | ~91% | >=80% | IMPLEMENTED |
| Integration | HAPI | 7 pytest | ~100% | >=80% | IMPLEMENTED |
| E2E | DS | 5 Ginkgo | >=80% | >=80% | IMPLEMENTED |

---

## 5. Anti-Pattern Compliance Audit

| Anti-Pattern (TESTING_GUIDELINES.md) | DS Integration | DS E2E | HAPI Integration |
|--------------------------------------|----------------|--------|------------------|
| `time.Sleep()` FORBIDDEN | Clean | Clean | Clean |
| `Skip()` FORBIDDEN | Clean | Clean | Clean |
| Direct Audit Infrastructure | Clean | Clean | Clean |
| Direct Metrics Method Calls | Clean | Clean | Clean |
| HTTP in Integration Tests | Clean (direct calls only) | N/A (HTTP correct in E2E) | Clean |
| UT/IT Duplication | Clean | N/A | Clean (IT-004/005 test coordination, not mock patterns) |

---

## References

- [DD-HAPI-016 v1.1](../../architecture/decisions/DD-HAPI-016-remediation-history-context.md)
- [DD-EM-002 v1.1](../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)
- [DD-TEST-006](../../architecture/decisions/DD-TEST-006-test-plan-policy.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)
