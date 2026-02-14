# DD-HAPI-016 Test Plan: Remediation History Context

**Version**: 1.0.0
**Created**: 2026-02-14
**Status**: Active
**BR**: BR-HAPI-016 (Remediation History Context for LLM Prompt Enrichment)
**DD**: DD-HAPI-016 v1.1 (Two-step query with EM scoring), DD-EM-002 v1.1 (spec_drift)

---

## Overview

This test plan defines the integration and E2E test scenarios for the remediation
history feature across DataStorage (DS) and HolmesGPT API (HAPI). Unit tests are
already in place (21 Ginkgo DS + 28 pytest HAPI). This plan targets the remaining
integration and E2E tiers to reach >=80% per-tier coverage.

**Reference Documents**:
- [DD-HAPI-016 v1.1](../../architecture/decisions/DD-HAPI-016-remediation-history-context.md)
- [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-016-{SEQUENCE}`

- `IT-DS-016-NNN` -- DataStorage integration tests (real PostgreSQL)
- `E2E-DS-016-NNN` -- DataStorage E2E tests (Kind cluster)
- `IT-HAPI-016-NNN` -- HolmesGPT API integration tests (direct function calls)

---

## Code Surface by Tier

### DS (Go)

| Tier | Files | LOC | Coverage Before | Target |
|------|-------|-----|-----------------|--------|
| Unit | `remediation_history_logic.go`, `effectiveness_handler.go` | ~380 | ~95% (21 tests) | >=80% MET |
| Integration | `remediation_history_repository.go`, `remediation_history_adapter.go`, `remediation_history_handler.go` | ~340 | 0% | >=80% |
| E2E | Full stack (server wiring, routing, PostgreSQL, HTTP) | ~15 wiring | 0% | >=80% |

### HAPI (Python)

| Tier | Files | Functions | Coverage Before | Target |
|------|-------|-----------|-----------------|--------|
| Unit | `remediation_history_prompt.py`, prompt builders | 13 | ~95% (28 tests) | >=80% MET |
| Integration | `remediation_history_client.py`, incident/recovery wiring | 5 | ~40% | >=80% |

---

## 1. DS Integration Tests

**File**: `test/integration/datastorage/remediation_history_integration_test.go`
**Infrastructure**: PostgreSQL container via `suite_test.go` (`db`, `auditRepo`, `logger`)
**Pattern**: Same as `hash_chain_db_round_trip_test.go`

### 1.1 Repository Layer

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-DS-016-001 | Insert RO audit events for a target, query via `QueryROEventsByTarget`, verify rows filtered by `target_resource` JSONB field and `since` timestamp | DS retrieves correct remediation chain for a target resource | `repo.QueryROEventsByTarget` (~40 LOC) |
| IT-DS-016-002 | Insert EM audit events for 3 correlation IDs, query via `QueryEffectivenessEventsBatch`, verify correct grouping by `correlation_id` | DS batches EM component events for scoring | `repo.QueryEffectivenessEventsBatch` (~50 LOC) |
| IT-DS-016-003 | Insert RO events with `pre_remediation_spec_hash`, query via `QueryROEventsBySpecHash` with hash + time range, verify Tier 2 widening results | DS supports historical hash lookup for broader context | `repo.QueryROEventsBySpecHash` (~40 LOC) |

### 1.2 Adapter Layer

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-DS-016-004 | Insert EM events, create adapter via `NewRemediationHistoryRepoAdapter`, call `QueryEffectivenessEventsBatch` through adapter, verify `EffectivenessEventRow` -> `EffectivenessEvent` conversion preserves `event_data` map | Adapter type conversion is lossless -- no data lost between repo and server packages | `adapter.QueryEffectivenessEventsBatch` (~20 LOC) |

### 1.3 Handler Layer (HTTP + Real DB)

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-DS-016-005 | Full pipeline: insert RO + full EM events (reason=full), start `httptest` server with handler backed by real DB, `GET /remediation-history/context`, verify JSON has `assessmentReason=full` and computed weighted score | **LLM receives accurate effectiveness data from real DB** | Handler + adapter + repo pipeline (~100 LOC) |
| IT-DS-016-006 | Same pipeline with EM events having `reason=spec_drift`, verify JSON has `assessmentReason=spec_drift` and `effectivenessScore=0.0` | **LLM correctly informed that spec_drift != failure** | spec_drift code path (~15 LOC) |
| IT-DS-016-007 | Insert events where `current_spec_hash` matches a `pre_remediation_spec_hash`, verify `regressionDetected: true` in response | **LLM warned about configuration regression** | Regression detection path (~15 LOC) |
| IT-DS-016-008 | Query with `tier1Window=invalid` duration string, verify 400 error | Malformed parameters rejected cleanly | Handler error path (~10 LOC) |
| IT-DS-016-009 | Query for non-existent target, verify 200 with empty `tier1.chain` and `tier2.chain` | Graceful empty response (not 404) | Empty result path (~10 LOC) |

**Coverage estimate**: 9 tests cover ~290 of ~340 LOC = **~85%**.

---

## 2. DS E2E Tests

**File**: `test/e2e/datastorage/25_remediation_history_api_test.go`
**Infrastructure**: Kind cluster with DS service running
**Pattern**: Same as `01_happy_path_test.go`

| ID | Scenario | Business Outcome |
|----|----------|------------------|
| E2E-DS-016-001 | Write RO + EM audit events via audit API, query `GET /remediation-history/context`, verify response has `tier1.chain` with `assessmentReason`, `effectivenessScore`, `healthChecks`, `metricDeltas` | Complete remediation context flows through real service |
| E2E-DS-016-002 | Write EM events with `reason=spec_drift`, query, verify `assessmentReason=spec_drift` and `effectivenessScore=0.0` | spec_drift semantics survive full service stack |
| E2E-DS-016-003 | Query for non-existent target, verify 200 OK with empty chains | Graceful empty handling in production-like environment |
| E2E-DS-016-004 | Write events for multiple correlation IDs with mixed reasons (full, spec_drift, partial), verify all appear with correct `assessmentReason` | Multi-entry mixed-reason correctness |

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

### 3.2 Client Query

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-HAPI-016-004 | `query_remediation_history` with mocked API returning spec_drift entry: verify `assessmentReason` preserved in dict | spec_drift data flows through Python client | `query_remediation_history` (~30 LOC) |
| IT-HAPI-016-005 | `fetch_remediation_history_for_request` with `ConnectionError` from API: returns None, no exception | **LLM analysis continues even if DS is down** | Graceful degradation (~10 LOC) |

### 3.3 Prompt Wiring

| ID | Scenario | Business Outcome | Code Covered |
|----|----------|------------------|--------------|
| IT-HAPI-016-006 | `create_incident_investigation_prompt` with remediation history containing spec_drift: prompt includes "INCONCLUSIVE" and spec drift guidance | **LLM correctly distinguishes spec_drift from failure** | Incident prompt wiring (~15 LOC) |
| IT-HAPI-016-007 | `_create_recovery_investigation_prompt` with remediation history: prompt includes remediation section | Recovery flow receives same enrichment as incident | Recovery prompt wiring (~15 LOC) |
| IT-HAPI-016-008 | `fetch_remediation_history_for_request` with empty `current_spec_hash`: returns None without calling DS | No unnecessary DS calls when spec hash unavailable | Early return path (~5 LOC) |

**Coverage estimate**: 8 tests cover ~115 of ~135 integration-testable LOC = **~85%**.

---

## 4. Compliance Summary

| Tier | Service | Tests | Est. Coverage | Target | Status |
|------|---------|-------|---------------|--------|--------|
| Unit | DS | 21 Ginkgo | ~95% | >=80% | PASS |
| Unit | HAPI | 28 pytest | ~95% | >=80% | PASS |
| Integration | DS | 9 Ginkgo | ~85% | >=80% | PLANNED |
| Integration | HAPI | 8 pytest | ~85% | >=80% | PLANNED |
| E2E | DS | 4 Ginkgo | >=80% | >=80% | PLANNED |

---

## References

- [DD-HAPI-016 v1.1](../../architecture/decisions/DD-HAPI-016-remediation-history-context.md)
- [DD-EM-002 v1.1](../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)
- [DD-TEST-006](../../architecture/decisions/DD-TEST-006-test-plan-policy.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)
