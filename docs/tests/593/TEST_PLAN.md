# Test Plan: Microsoft Teams Delivery Channel

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-593-v1
**Feature**: Microsoft Teams delivery channel for the Notification service via Power Automate Workflows webhooks with Adaptive Cards
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feature/v1.0-remaining-bugs-demos`

---

## 1. Introduction

### 1.1 Purpose

This test plan provides behavioral assurance that the Microsoft Teams delivery channel correctly implements the `Service` interface, constructs valid Adaptive Card payloads in the Power Automate Workflows format, handles errors with proper retryability classification, enforces the 28KB payload size limit, and integrates with the per-receiver credential resolver pattern (BR-NOT-104).

### 1.2 Objectives

1. **Payload correctness**: Teams Workflows webhook payloads use the correct `type: "message"` + Adaptive Card attachment format (NOT legacy MessageCard/connector format)
2. **Adaptive Card formatting**: Card body maps NotificationRequest fields to structured sections (RCA summary, affected resource, confidence, status, kubectl command)
3. **Error classification**: HTTP status codes are classified correctly — 5xx/429 = retryable, 4xx = permanent, TLS = permanent
4. **Payload size guard**: Notifications exceeding 28KB are truncated with a correlation ID reference
5. **Credential integration**: Webhook URL is resolved via `CredentialRef` and the projected-volume credential resolver
6. **Orchestrator registration**: Teams service registers per-receiver with qualified channel names (`teams:<receiver>`)
7. **Notification type layouts**: Distinct Adaptive Card layouts for approval, status-update, escalation, and completion notification types

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/notification/delivery/...` |
| Integration test pass rate | 100% | `go test ./test/integration/notification/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/notification/delivery/teams*.go` |
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
- **Issue #593**: Microsoft Teams delivery channel

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Slack delivery implementation](../../../pkg/notification/delivery/slack.go) (reference pattern)
- [Microsoft Adaptive Cards schema](https://adaptivecards.io/explorer/)
- [Power Automate Workflows webhook format](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Adaptive Card schema invalid | Teams rejects payload; message not posted | Medium | UT-NOT-593-001..003 | Validate against Adaptive Card v1.x schema; mock server validates `contentType` |
| R2 | Legacy connector format used instead of Workflows | Breaks after April 2026 deprecation | High | UT-NOT-593-001 | Test asserts `type: "message"` + `attachments[].contentType = "application/vnd.microsoft.card.adaptive"` — NOT `@type: MessageCard` |
| R3 | 28KB limit exceeded silently | Webhook returns 413 or silently drops | High | UT-NOT-593-009, UT-NOT-593-010 | Size guard checks JSON-encoded payload; truncation includes audit trail reference |
| R4 | Credential file missing at runtime | Delivery service not registered; silent no-op | High | IT-NOT-593-001, UT-NOT-593-012 | ValidateCredentialRefs() rejects config; credential resolver validates files on reload |
| R5 | kubectl command formatting error | Operators cannot launch chat session from Teams card | Medium | UT-NOT-593-004 | Test asserts exact format: `kubectl kubernaut chat rar/{name} -n {namespace}` |

### 3.1 Risk-to-Test Traceability

- **R1 (MEDIUM)**: Mitigated by UT-NOT-593-001 through UT-NOT-593-003 (Adaptive Card construction for all notification types)
- **R2 (HIGH)**: Mitigated by UT-NOT-593-001 (asserts Workflows format, not legacy MessageCard)
- **R3 (HIGH)**: Mitigated by UT-NOT-593-009 and UT-NOT-593-010 (size guard threshold and truncation)
- **R4 (HIGH)**: Mitigated by IT-NOT-593-001 (credential resolution flow) and UT-NOT-593-012
- **R5 (MEDIUM)**: Mitigated by UT-NOT-593-004 (kubectl command format)

---

## 4. Scope

### 4.1 Features to be Tested

- **Teams delivery service** (`pkg/notification/delivery/teams.go`): Adaptive Card payload construction, HTTP POST to Workflows webhook, error classification, payload size guard
- **Adaptive Card formatting** (`pkg/notification/delivery/teams_cards.go`): NR-to-card field mapping, distinct layouts per notification type (approval, status-update, escalation, completion), kubectl command inclusion
- **Routing handler rebuild** (`internal/controller/notification/routing_handler.go`): `rebuildTeamsDeliveryServices()`, per-receiver credential wiring
- **Routing config validation** (`pkg/notification/routing/config.go`): `TeamsConfig.CredentialRef` validation in `ValidateCredentialRefs()`, `QualifiedChannels()` for Teams

### 4.2 Features Not to be Tested

- **Teams Bot Framework integration**: Requires paid M365 subscription; deferred beyond v1.4
- **Teams thread replies / dedup**: Teams Workflows webhooks are fire-and-forget with no dedup mechanism
- **Cross-channel bridging**: Teams↔Slack or Teams↔PD bridging deferred to v1.5
- **Circuit breaker for Teams**: Will reuse existing `circuitbreaker.Manager` — covered by existing CB tests

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Power Automate Workflows format from day one | Office 365 Connectors deprecated April 2026; Workflows is the replacement; no legacy support needed |
| Adaptive Cards v1.x (not v1.5+) | Broadest Teams client compatibility; v1.5 features not needed for notification content |
| Raw HTTP instead of Teams SDK | Single POST endpoint; no SDK dependency; consistent with Slack/PD raw HTTP pattern |
| Per-type card layouts (not generic) | Approval cards need kubectl command + confidence; escalation cards need urgency indicators; generic layout loses context |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `pkg/notification/delivery/teams*.go` (card construction, error classification, size guard)
- **Integration**: >=80% of `rebuildTeamsDeliveryServices()` in routing handler + credential wiring via httptest mock
- **E2E**: Deferred to Phase 3 implementation plan — mock Teams webhook in Kind cluster

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Card correctness, error classification, size guard, Workflows format validation
- **Integration tests**: End-to-end delivery via httptest mock server, per-receiver credential resolution, routing config reload

### 5.3 Business Outcome Quality Bar

Tests validate observable business outcomes:
- "Teams channel receives a rich Adaptive Card with RCA summary and kubectl command"
- "Retryable errors are classified as retryable; permanent errors are not"
- "Large payloads are truncated with audit trail reference, not silently dropped"

### 5.4 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage >=80%
4. No regressions in existing notification test suites
5. Adaptive Card payload validates against v1.x schema

**FAIL**:
1. Any P0 test fails
2. Per-tier coverage below 80%
3. Existing tests regress
4. Payload uses legacy MessageCard format instead of Adaptive Card

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken, credential resolver unavailable, existing notification tests failing
**Resume**: Root cause fixed, build green, existing tests passing

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/delivery/teams.go` | `NewTeamsDeliveryService`, `Deliver`, `buildTeamsPayload` | ~100 |
| `pkg/notification/delivery/teams_cards.go` | `FormatApprovalCard`, `FormatStatusUpdateCard`, `FormatEscalationCard`, `FormatCompletionCard`, `truncateForTeamsSizeLimit` | ~150 |
| `pkg/notification/routing/config.go` | `ValidateCredentialRefs` (Teams path), `QualifiedChannels` (Teams path) | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/notification/routing_handler.go` | `rebuildTeamsDeliveryServices`, `ReloadRoutingFromContent` (Teams path) | ~60 |
| `pkg/notification/delivery/teams.go` | `Deliver` (HTTP POST path) | ~40 |

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
| BR-NOT-053 | At-Least-Once Delivery — Teams message delivered | P0 | Unit | UT-NOT-593-001 | Pending |
| BR-NOT-053 | At-Least-Once Delivery — Teams message delivered (integration) | P0 | Integration | IT-NOT-593-002 | Pending |
| BR-NOT-055 | Retry logic — 5xx/429 = retryable | P0 | Unit | UT-NOT-593-006 | Pending |
| BR-NOT-055 | Retry logic — 4xx = permanent | P0 | Unit | UT-NOT-593-007 | Pending |
| BR-NOT-058 | TLS error = permanent | P1 | Unit | UT-NOT-593-008 | Pending |
| BR-NOT-104 | Per-receiver credential resolution | P0 | Unit | UT-NOT-593-012 | Pending |
| BR-NOT-104 | Per-receiver credential resolution (integration) | P0 | Integration | IT-NOT-593-001 | Pending |
| BR-NOT-104 | QualifiedChannels Teams per-receiver names | P0 | Unit | UT-NOT-593-013 | Pending |
| BR-NOT-104 | ValidateCredentialRefs for Teams | P0 | Unit | UT-NOT-593-014 | Pending |
| Issue #593 | Power Automate Workflows format (not legacy connector) | P0 | Unit | UT-NOT-593-001 | Pending |
| Issue #593 | Adaptive Card with RCA summary | P0 | Unit | UT-NOT-593-002 | Pending |
| Issue #593 | Distinct card for approval (with kubectl cmd) | P0 | Unit | UT-NOT-593-003 | Pending |
| Issue #593 | kubectl command format in card | P1 | Unit | UT-NOT-593-004 | Pending |
| Issue #593 | Distinct card for escalation (urgency indicators) | P1 | Unit | UT-NOT-593-005 | Pending |
| Issue #593 | 28KB payload size guard | P0 | Unit | UT-NOT-593-009 | Pending |
| Issue #593 | Payload truncation with audit reference | P0 | Unit | UT-NOT-593-010 | Pending |
| Issue #593 | Context cancellation | P1 | Unit | UT-NOT-593-011 | Pending |
| Issue #593 | Full delivery flow via routing | P0 | Integration | IT-NOT-593-002 | Pending |
| Issue #593 | Teams service rebuild on config reload | P1 | Integration | IT-NOT-593-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-NOT-593-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **NOT**: Notification service
- **593**: Issue number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `pkg/notification/delivery/teams*.go`, `pkg/notification/routing/config.go` (Teams paths) — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-593-001` | Payload uses Workflows format: outer `type: "message"`, `attachments[0].contentType = "application/vnd.microsoft.card.adaptive"`, inner Adaptive Card with `$schema`, `type: "AdaptiveCard"`, `version: "1.0"` | Pending |
| `UT-NOT-593-002` | Adaptive Card body contains TextBlocks with subject, RCA summary, affected resource, and confidence score | Pending |
| `UT-NOT-593-003` | Approval notification card includes kubectl command action: `kubectl kubernaut chat rar/{name} -n {namespace}` | Pending |
| `UT-NOT-593-004` | Status-update notification card includes phase transition details and verification context | Pending |
| `UT-NOT-593-005` | Escalation notification card includes urgency indicators (priority emoji, severity color) | Pending |
| `UT-NOT-593-006` | HTTP 500, 502, 503, 429 responses produce RetryableError | Pending |
| `UT-NOT-593-007` | HTTP 400, 401, 403, 404 responses produce permanent (non-retryable) error | Pending |
| `UT-NOT-593-008` | TLS certificate error produces permanent error (BR-NOT-058) | Pending |
| `UT-NOT-593-009` | Payload exceeding 28KB triggers truncation | Pending |
| `UT-NOT-593-010` | Truncated payload includes "[truncated — full details in audit trail]" and correlation ID; total size <=28KB | Pending |
| `UT-NOT-593-011` | Cancelled context returns immediately with context error | Pending |
| `UT-NOT-593-012` | Constructor fails gracefully when webhook URL is empty | Pending |
| `UT-NOT-593-013` | QualifiedChannels returns `teams:<receiver>` for Teams with CredentialRef | Pending |
| `UT-NOT-593-014` | ValidateCredentialRefs fails when TeamsConfig has empty CredentialRef | Pending |
| `UT-NOT-593-015` | Completion notification card includes verification outcome when present | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Routing handler Teams rebuild, credential resolution wiring, end-to-end delivery via httptest — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-593-001` | Per-receiver Teams service registered via credential resolver; delivery succeeds through orchestrator | Pending |
| `IT-NOT-593-002` | Full delivery flow: NR created → routing resolves Teams receiver → Teams delivery service called → mock server receives valid Workflows webhook payload with Adaptive Card | Pending |
| `IT-NOT-593-003` | Config reload triggers Teams service rebuild; stale keys unregistered, new keys registered | Pending |
| `IT-NOT-593-004` | Mock server validates `Content-Type: application/json` and Workflows payload structure (not legacy MessageCard) | Pending |

### Tier 3: E2E Tests

**Deferred to implementation Phase 3** — requires mock Teams webhook server deployed in Kind cluster following `test/infrastructure/notification_e2e.go` pattern.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-NOT-593-001` | Routing config with Teams receiver → NR created → Teams mock receives Adaptive Card → NR status transitions to Sent | Pending |

### Tier Skip Rationale

- **E2E**: Deferred to implementation Phase 3. Integration tests with httptest mock provide sufficient confidence for the delivery service itself. E2E validates the full Kind cluster pipeline (mock Teams webhook + Helm config).

---

## 9. Test Cases

### UT-NOT-593-001: Workflows format compliance (NOT legacy MessageCard)

**BR**: BR-NOT-053, Issue #593
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/delivery/teams_test.go`

**Preconditions**:
- NotificationRequest with subject, body, priority=high, type=approval

**Test Steps**:
1. **Given**: A NotificationRequest with subject "CrashLoopBackOff detected", body with RCA, priority "high", type "approval"
2. **When**: `buildTeamsPayload(notification)` is called
3. **Then**: JSON has `type: "message"`, `attachments` array with one element, `attachments[0].contentType = "application/vnd.microsoft.card.adaptive"`, inner `content` has `type: "AdaptiveCard"` and `version: "1.0"`

**Expected Results**:
1. Outer wrapper has `type: "message"` (Workflows format)
2. `attachments[0].contentType` is `"application/vnd.microsoft.card.adaptive"`
3. Inner card has `$schema: "http://adaptivecards.io/schemas/adaptive-card.json"`
4. Inner card has `type: "AdaptiveCard"` and `version: "1.0"`
5. Card body contains TextBlock elements with notification content

**Acceptance Criteria**:
- **Behavior**: Valid Power Automate Workflows JSON produced
- **Correctness**: NOT using legacy `@type: MessageCard` format
- **Accuracy**: Adaptive Card schema reference and version are correct

### UT-NOT-593-009: Payload size guard (28KB)

**BR**: Issue #593
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/delivery/teams_test.go`

**Preconditions**:
- NotificationRequest with body exceeding 28KB when JSON-encoded as Adaptive Card

**Test Steps**:
1. **Given**: A NotificationRequest with a 40KB body (large RCA summary)
2. **When**: `buildTeamsPayload(notification)` is called
3. **Then**: Resulting JSON payload is <=28KB (28*1024 bytes) and body is truncated

**Expected Results**:
1. JSON-encoded payload size <= 28*1024 bytes
2. Truncated body contains "[truncated — full details in audit trail]"
3. Card body includes correlation ID (NR name) for audit trail lookup

### IT-NOT-593-002: Full delivery flow with Adaptive Card validation

**BR**: BR-NOT-053, Issue #593
**Priority**: P0
**Type**: Integration
**File**: `test/integration/notification/teams_delivery_test.go`

**Preconditions**:
- httptest mock Teams webhook server running
- Credential file in temp dir with mock server URL
- Routing config with Teams receiver referencing the credential

**Test Steps**:
1. **Given**: A routing config with `teamsConfigs: [{credentialRef: "teams-webhook"}]` and a credential file containing the mock server URL
2. **When**: `ReloadRoutingFromContent` is called, then a delivery is triggered through the orchestrator for the Teams channel
3. **Then**: Mock server receives HTTP POST with `Content-Type: application/json`, body containing `type: "message"` and Adaptive Card attachment

**Expected Results**:
1. Mock server receives exactly one HTTP POST request
2. Request body parses as valid JSON with Workflows structure
3. Adaptive Card content contains the notification subject and body
4. Request came through the per-receiver qualified channel key `teams:<receiver>`

**Acceptance Criteria**:
- **Behavior**: Full routing → credential resolution → delivery pipeline works end-to-end
- **Correctness**: Payload uses Workflows format, not legacy connector
- **Accuracy**: Content-Type header is `application/json`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `httptest.Server` for HTTP responses (external dependency)
- **Location**: `test/unit/notification/delivery/`
- **Resources**: Minimal (no external services)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO business logic mocks; `httptest.Server` for Teams endpoint (external service boundary)
- **Infrastructure**: Credential resolver with temp directory, routing config reload
- **Location**: `test/integration/notification/`
- **Resources**: Minimal

### 10.3 E2E Tests (deferred to Phase 3)

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Kind cluster, mock Teams nginx webhook (following `deployNotificationMockSlack` pattern, validating Workflows format)
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
| Phase 0 shared infra | Code | Merged | UT-NOT-593-013/014 blocked | Already completed |
| Credential resolver (#244) | Code | Merged | IT-NOT-593-001 blocked | Already implemented |
| Delivery orchestrator (DD-NOT-007) | Code | Merged | IT-NOT-593-002 blocked | Already implemented |

### 11.2 Execution Order

1. **Phase 1**: Unit tests for Adaptive Card construction and Workflows format (UT-NOT-593-001..005)
2. **Phase 2**: Unit tests for error classification (UT-NOT-593-006..008)
3. **Phase 3**: Unit tests for size guard (UT-NOT-593-009..010)
4. **Phase 4**: Unit tests for routing config (UT-NOT-593-013..014)
5. **Phase 5**: Integration tests (IT-NOT-593-001..004)
6. **Phase 6**: E2E test (E2E-NOT-593-001) — deferred

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/593/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/notification/delivery/teams_test.go` | Ginkgo BDD test file |
| Integration test suite | `test/integration/notification/teams_delivery_test.go` | Ginkgo BDD test file |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/notification/delivery/... -ginkgo.v -ginkgo.focus="Teams"

# Integration tests
go test ./test/integration/notification/... -ginkgo.v -ginkgo.focus="Teams"

# Specific test by ID
go test ./test/unit/notification/delivery/... -ginkgo.focus="UT-NOT-593-001"

# Coverage
go test ./test/unit/notification/delivery/... -coverprofile=coverage_teams_ut.out -coverpkg=github.com/jordigilh/kubernaut/pkg/notification/delivery/...
go tool cover -func=coverage_teams_ut.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | N/A | TeamsConfig is new; no existing tests reference it. Phase 0 already updated ValidateCredentialRefs and QualifiedChannels for Teams. |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
