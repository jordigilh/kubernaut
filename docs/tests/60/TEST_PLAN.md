# Test Plan: PagerDuty Delivery Channel

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-060-v1
**Feature**: PagerDuty delivery channel for the Notification service via Events API v2
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feature/v1.0-remaining-bugs-demos`

---

## 1. Introduction

### 1.1 Purpose

This test plan provides behavioral assurance that the PagerDuty delivery channel correctly implements the `Service` interface, constructs valid Events API v2 payloads, handles errors with proper retryability classification, enforces the 512KB payload size limit, and integrates with the per-receiver credential resolver pattern (BR-NOT-104).

### 1.2 Objectives

1. **Payload correctness**: PagerDuty Events API v2 payloads are correctly constructed from NotificationRequest CRD fields (routing_key, severity, summary, source, component, dedup_key)
2. **Error classification**: HTTP status codes are classified correctly — 5xx/429 = retryable, 4xx = permanent, TLS = permanent
3. **Payload size guard**: Notifications exceeding 512KB are truncated with a correlation ID reference
4. **Credential integration**: Routing key is resolved via `CredentialRef` and the projected-volume credential resolver
5. **Orchestrator registration**: PagerDuty service registers per-receiver with qualified channel names (`pagerduty:<receiver>`)
6. **Dedup strategy**: `dedup_key` is set to `notificationRequest.metadata.name` for incident merging

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/notification/delivery/...` |
| Integration test pass rate | 100% | `go test ./test/integration/notification/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/notification/delivery/pagerduty*.go` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on routing handler rebuild + credential wiring |
| Backward compatibility | 0 regressions | Existing notification tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-NOT-104**: Per-receiver credentials via projected-volume CredentialRef
- **BR-NOT-053**: At-Least-Once Delivery
- **BR-NOT-055**: Retry logic with permanent error classification
- **BR-NOT-058**: TLS/security error handling (permanent, no retry)
- **DD-NOT-002**: Delivery service interface (Service.Deliver)
- **DD-NOT-007**: Delivery orchestrator registration pattern
- **Issue #60**: PagerDuty delivery channel

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Slack delivery implementation](../../../pkg/notification/delivery/slack.go) (reference pattern)
- [Orchestrator registration tests](../../../test/unit/notification/delivery/orchestrator_registration_test.go)
- [Credential resolver tests](../../../test/integration/notification/credential_resolver_test.go)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Events API v2 payload schema mismatch | PD rejects events; incidents not created | Medium | UT-NOT-060-001..004 | Validate against official PD Events v2 schema; mock server validates structure |
| R2 | 512KB limit miscalculated (JSON encoding overhead) | Payload silently rejected by PD | Medium | UT-NOT-060-009, UT-NOT-060-010 | Test with payloads near boundary; measure JSON-encoded size, not raw string |
| R3 | Credential file missing at runtime | Delivery service not registered; silent no-op | High | IT-NOT-060-001, UT-NOT-060-012 | ValidateCredentialRefs() rejects config with missing refs; credential resolver validates files on reload |
| R4 | dedup_key collision across namespaces | Incidents merged incorrectly | Low | UT-NOT-060-005 | Include namespace in dedup_key if NR names are not globally unique |
| R5 | Context cancellation not respected | Goroutine leak, resource exhaustion | Medium | UT-NOT-060-008 | Test with cancelled context; verify immediate return |

### 3.1 Risk-to-Test Traceability

- **R1 (CRITICAL)**: Mitigated by UT-NOT-060-001 through UT-NOT-060-004 (payload construction for all notification types)
- **R2 (MEDIUM)**: Mitigated by UT-NOT-060-009 and UT-NOT-060-010 (size guard threshold and truncation behavior)
- **R3 (HIGH)**: Mitigated by IT-NOT-060-001 (credential resolution flow) and UT-NOT-060-012 (missing credential error)
- **R5 (MEDIUM)**: Mitigated by UT-NOT-060-008 (context cancellation)

---

## 4. Scope

### 4.1 Features to be Tested

- **PagerDuty delivery service** (`pkg/notification/delivery/pagerduty.go`): Events API v2 payload construction, HTTP POST, error classification, dedup_key, payload size guard
- **PagerDuty payload formatting** (`pkg/notification/delivery/pagerduty_payload.go`): NR-to-PD field mapping, severity mapping, kubectl command in custom_details
- **Routing handler rebuild** (`internal/controller/notification/routing_handler.go`): `rebuildPagerDutyDeliveryServices()`, per-receiver credential wiring
- **Routing config validation** (`pkg/notification/routing/config.go`): `PagerDutyConfig.CredentialRef` validation in `ValidateCredentialRefs()`, `QualifiedChannels()` for PD

### 4.2 Features Not to be Tested

- **PagerDuty auto-resolve lifecycle**: Deferred to v1.5 (incident resolve events when RR reaches terminal phase)
- **PagerDuty REST API (non-Events)**: Only Events API v2 is in scope
- **Cross-channel bridging**: PD→Slack or PD→Teams bridging is deferred to v1.5
- **Circuit breaker for PD**: Will reuse existing `circuitbreaker.Manager` — covered by existing CB tests

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Raw HTTP instead of `go-pagerduty` SDK | Fewer dependencies; Events API v2 is a single POST endpoint; consistent with Slack's raw HTTP pattern |
| dedup_key = NR metadata.name | Deterministic, unique per NR; PD merges incidents with same dedup_key |
| JSON-encoded size for 512KB check | PD measures payload size post-encoding; raw string length undercounts |
| Reuse `RetryableError` from `delivery/errors.go` | Consistent error classification across all channels |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `pkg/notification/delivery/pagerduty*.go` (payload construction, error classification, size guard, dedup_key)
- **Integration**: >=80% of `rebuildPagerDutyDeliveryServices()` in routing handler + credential wiring via httptest mock
- **E2E**: Deferred to Phase 3 implementation plan — mock PD server in Kind cluster

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Payload correctness, error classification, size guard, dedup_key strategy
- **Integration tests**: End-to-end delivery via httptest mock server, per-receiver credential resolution, routing config reload

### 5.3 Business Outcome Quality Bar

Tests validate observable business outcomes:
- "PagerDuty incident is created with correct severity and summary"
- "Retryable errors are classified as retryable; permanent errors are not"
- "Large payloads are truncated with audit trail reference"

### 5.4 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage >=80%
4. No regressions in existing notification test suites
5. PagerDuty Events API v2 payload validates against official schema

**FAIL**:
1. Any P0 test fails
2. Per-tier coverage below 80%
3. Existing tests regress
4. Payload rejected by PD Events API v2 schema validation

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken, credential resolver unavailable, existing notification tests failing
**Resume**: Root cause fixed, build green, existing tests passing

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/delivery/pagerduty.go` | `NewPagerDutyDeliveryService`, `Deliver`, `buildPagerDutyPayload`, `mapSeverity` | ~120 |
| `pkg/notification/delivery/pagerduty_payload.go` | `FormatPagerDutyPayload`, `buildCustomDetails`, `truncateForSizeLimit` | ~80 |
| `pkg/notification/routing/config.go` | `ValidateCredentialRefs` (PD path), `QualifiedChannels` (PD path) | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/notification/routing_handler.go` | `rebuildPagerDutyDeliveryServices`, `ReloadRoutingFromContent` (PD path) | ~60 |
| `pkg/notification/delivery/pagerduty.go` | `Deliver` (HTTP POST path) | ~40 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/v1.0-remaining-bugs-demos` HEAD | Phase 0 shared infra landed |
| Dependency: credential resolver | Already implemented (#244) | No changes needed |
| Dependency: orchestrator | Already implemented (DD-NOT-007) | Register via `RegisterChannel` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-NOT-053 | At-Least-Once Delivery — PD event delivered | P0 | Unit | UT-NOT-060-001 | Pending |
| BR-NOT-053 | At-Least-Once Delivery — PD event delivered (integration) | P0 | Integration | IT-NOT-060-002 | Pending |
| BR-NOT-055 | Retry logic — 5xx/429 = retryable | P0 | Unit | UT-NOT-060-006 | Pending |
| BR-NOT-055 | Retry logic — 4xx = permanent | P0 | Unit | UT-NOT-060-007 | Pending |
| BR-NOT-058 | TLS error = permanent | P1 | Unit | UT-NOT-060-008 | Pending |
| BR-NOT-104 | Per-receiver credential resolution | P0 | Unit | UT-NOT-060-012 | Pending |
| BR-NOT-104 | Per-receiver credential resolution (integration) | P0 | Integration | IT-NOT-060-001 | Pending |
| BR-NOT-104 | QualifiedChannels PD per-receiver names | P0 | Unit | UT-NOT-060-013 | Pending |
| BR-NOT-104 | ValidateCredentialRefs for PD | P0 | Unit | UT-NOT-060-014 | Pending |
| Issue #60 | Dedup key = NR name | P0 | Unit | UT-NOT-060-005 | Pending |
| Issue #60 | Events API v2 payload structure | P0 | Unit | UT-NOT-060-001 | Pending |
| Issue #60 | Severity mapping (NR priority → PD severity) | P0 | Unit | UT-NOT-060-003 | Pending |
| Issue #60 | 512KB payload size guard | P1 | Unit | UT-NOT-060-009 | Pending |
| Issue #60 | Payload truncation with audit reference | P1 | Unit | UT-NOT-060-010 | Pending |
| Issue #60 | kubectl command in custom_details | P1 | Unit | UT-NOT-060-004 | Pending |
| Issue #60 | Context cancellation | P1 | Unit | UT-NOT-060-011 | Pending |
| Issue #60 | Full delivery flow via routing | P0 | Integration | IT-NOT-060-002 | Pending |
| Issue #60 | PD service rebuild on config reload | P1 | Integration | IT-NOT-060-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-NOT-060-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **NOT**: Notification service
- **060**: Issue number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `pkg/notification/delivery/pagerduty*.go`, `pkg/notification/routing/config.go` (PD paths) — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-060-001` | Events API v2 payload has correct structure (routing_key, event_action=trigger, payload with severity, summary, source, component) | Pending |
| `UT-NOT-060-002` | Payload includes RCA summary, confidence score, and affected resource in custom_details | Pending |
| `UT-NOT-060-003` | NR priority maps to PD severity: critical→critical, high→error, medium→warning, low→info | Pending |
| `UT-NOT-060-004` | custom_details includes kubectl command: `kubectl kubernaut chat rar/{name} -n {namespace}` | Pending |
| `UT-NOT-060-005` | dedup_key is set to NR metadata.name (deterministic, unique) | Pending |
| `UT-NOT-060-006` | HTTP 500, 502, 503, 429 responses produce RetryableError | Pending |
| `UT-NOT-060-007` | HTTP 400, 401, 403, 404 responses produce permanent (non-retryable) error | Pending |
| `UT-NOT-060-008` | TLS certificate error produces permanent error (BR-NOT-058) | Pending |
| `UT-NOT-060-009` | Payload exceeding 512KB triggers truncation | Pending |
| `UT-NOT-060-010` | Truncated payload includes "[truncated — full details in audit trail]" and correlation ID | Pending |
| `UT-NOT-060-011` | Cancelled context returns immediately with context error | Pending |
| `UT-NOT-060-012` | Constructor fails gracefully when webhook URL is empty | Pending |
| `UT-NOT-060-013` | QualifiedChannels returns `pagerduty:<receiver>` for PD with CredentialRef | Pending |
| `UT-NOT-060-014` | ValidateCredentialRefs fails when PagerDutyConfig has empty CredentialRef | Pending |
| `UT-NOT-060-015` | HTTP 202 (accepted) response is treated as success | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Routing handler PD rebuild, credential resolution wiring, end-to-end delivery via httptest — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-060-001` | Per-receiver PD service registered via credential resolver; delivery succeeds through orchestrator | Pending |
| `IT-NOT-060-002` | Full delivery flow: NR created → routing resolves PD receiver → PD delivery service called → mock server receives valid Events API v2 payload | Pending |
| `IT-NOT-060-003` | Config reload triggers PD service rebuild; stale keys unregistered, new keys registered | Pending |
| `IT-NOT-060-004` | dedup_key in delivered payload matches NR name; second delivery with same NR updates existing incident | Pending |

### Tier 3: E2E Tests

**Deferred to implementation Phase 3** — requires mock PD server deployed in Kind cluster following `test/infrastructure/notification_e2e.go` pattern.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-NOT-060-001` | Routing config with PD receiver → NR created → PD mock receives event → NR status transitions to Sent | Pending |

### Tier Skip Rationale

- **E2E**: Deferred to implementation Phase 3. Integration tests with httptest mock provide sufficient confidence for the delivery service itself. E2E validates the full Kind cluster pipeline (mock PD server + Helm config).

---

## 9. Test Cases

### UT-NOT-060-001: Events API v2 payload structure

**BR**: BR-NOT-053, Issue #60
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/delivery/pagerduty_test.go`

**Preconditions**:
- NotificationRequest with subject, body, priority=critical, context with lineage

**Test Steps**:
1. **Given**: A NotificationRequest with subject "OOM kill detected", body with RCA summary, priority "critical", and lineage context
2. **When**: `buildPagerDutyPayload(notification, "test-routing-key")` is called
3. **Then**: Returned JSON has `routing_key=test-routing-key`, `event_action=trigger`, `payload.severity=critical`, `payload.summary` contains subject, `payload.source` contains namespace/resource

**Expected Results**:
1. `routing_key` matches the provided routing key
2. `event_action` is "trigger"
3. `payload.severity` is "critical"
4. `payload.summary` contains the NR subject
5. `payload.custom_details` contains RCA body

**Acceptance Criteria**:
- **Behavior**: Valid Events API v2 JSON produced
- **Correctness**: All required fields present with correct types
- **Accuracy**: Field mapping matches PD Events API v2 specification

### UT-NOT-060-006: Retryable error classification

**BR**: BR-NOT-055
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/delivery/pagerduty_test.go`

**Preconditions**:
- httptest mock returning 5xx or 429 status codes

**Test Steps**:
1. **Given**: A PagerDuty delivery service with httptest mock returning HTTP 500
2. **When**: `Deliver(ctx, notification)` is called
3. **Then**: Error is returned and `IsRetryableError(err)` is true

**Expected Results**:
1. Error is non-nil
2. `IsRetryableError(err)` returns true for 500, 502, 503, 429
3. `IsRetryableError(err)` returns false for 400, 401, 403, 404

**Acceptance Criteria**:
- **Behavior**: Retryable errors trigger retry; permanent errors do not
- **Correctness**: Status code → retryability mapping is correct

### UT-NOT-060-009: Payload size guard (512KB)

**BR**: Issue #60
**Priority**: P1
**Type**: Unit
**File**: `test/unit/notification/delivery/pagerduty_test.go`

**Preconditions**:
- NotificationRequest with a body exceeding 512KB when JSON-encoded

**Test Steps**:
1. **Given**: A NotificationRequest with a 600KB body (RCA summary)
2. **When**: `buildPagerDutyPayload(notification, key)` is called
3. **Then**: Resulting JSON payload is <=512KB and body is truncated

**Expected Results**:
1. JSON-encoded payload size <= 512*1024 bytes
2. Truncated body contains "[truncated — full details in audit trail]"
3. `custom_details.correlation_id` is present for audit lookup

### IT-NOT-060-001: Per-receiver credential resolution and delivery

**BR**: BR-NOT-104
**Priority**: P0
**Type**: Integration
**File**: `test/integration/notification/pagerduty_delivery_test.go`

**Preconditions**:
- httptest mock PD server running
- Credential file created in temp dir with mock server URL
- Routing config with PD receiver referencing the credential

**Test Steps**:
1. **Given**: A routing config with `pagerdutyConfigs: [{credentialRef: "pd-routing-key"}]` and a credential file `pd-routing-key` containing the mock server URL
2. **When**: `ReloadRoutingFromContent` is called, then a PD delivery is triggered through the orchestrator
3. **Then**: Mock server receives a valid Events API v2 POST with the routing key from the credential file

**Expected Results**:
1. Credential resolver successfully resolves the routing key from the projected volume file
2. PD delivery service is registered with qualified key `pagerduty:<receiver>`
3. Mock server receives HTTP POST with correct `routing_key` and valid payload

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `httptest.Server` for HTTP responses (external dependency)
- **Location**: `test/unit/notification/delivery/`
- **Resources**: Minimal (no external services)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO business logic mocks; `httptest.Server` for PD endpoint (external service boundary)
- **Infrastructure**: Credential resolver with temp directory, routing config reload
- **Location**: `test/integration/notification/`
- **Resources**: Minimal

### 10.3 E2E Tests (deferred to Phase 3)

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Kind cluster, mock PD nginx server (following `deployNotificationMockSlack` pattern)
- **Location**: `test/e2e/notification/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Phase 0 shared infra | Code | Merged | UT-NOT-060-013/014 blocked | Already completed |
| Credential resolver (#244) | Code | Merged | IT-NOT-060-001 blocked | Already implemented |
| Delivery orchestrator (DD-NOT-007) | Code | Merged | IT-NOT-060-002 blocked | Already implemented |

### 11.2 Execution Order

1. **Phase 1**: Unit tests for payload construction (UT-NOT-060-001..005)
2. **Phase 2**: Unit tests for error classification (UT-NOT-060-006..008)
3. **Phase 3**: Unit tests for size guard (UT-NOT-060-009..010)
4. **Phase 4**: Unit tests for routing config (UT-NOT-060-013..014)
5. **Phase 5**: Integration tests (IT-NOT-060-001..004)
6. **Phase 6**: E2E test (E2E-NOT-060-001) — deferred

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/60/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/notification/delivery/pagerduty_test.go` | Ginkgo BDD test file |
| Integration test suite | `test/integration/notification/pagerduty_delivery_test.go` | Ginkgo BDD test file |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/notification/delivery/... -ginkgo.v -ginkgo.focus="PagerDuty"

# Integration tests
go test ./test/integration/notification/... -ginkgo.v -ginkgo.focus="PagerDuty"

# Specific test by ID
go test ./test/unit/notification/delivery/... -ginkgo.focus="UT-NOT-060-001"

# Coverage
go test ./test/unit/notification/delivery/... -coverprofile=coverage_pd_ut.out -coverpkg=github.com/jordigilh/kubernaut/pkg/notification/delivery/...
go tool cover -func=coverage_pd_ut.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `routing_config_test.go` (multiple) | PagerDutyConfig uses `serviceKey` | Updated to `credentialRef` | F-2: PagerDutyConfig restructured (already done in Phase 0) |
| `routing_hotreload_test.go` | PagerDutyConfig uses `serviceKey` | Updated to `credentialRef` | F-2: Already done in Phase 0 |
| `routing_integration_test.go` | PagerDutyConfig uses `serviceKey` | Updated to `credentialRef` | F-2: Already done in Phase 0 |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
