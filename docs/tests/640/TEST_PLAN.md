# Test Plan: NotificationRequest TYPE/PRIORITY PascalCase Migration

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-640-v1.0
**Feature**: Migrate NotificationType and NotificationPriority CRD enums from lowercase to PascalCase
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc7`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that the NotificationType and NotificationPriority CRD enum values
are correctly migrated from lowercase (`approval`, `low`) to PascalCase (`Approval`, `Low`),
ensuring consistency with existing PascalCase enums (NotificationPhase, NotificationStatusReason)
and standard Kubernetes conventions for enum-style printer column values.

### 1.2 Objectives

1. **Enum correctness**: All 6 NotificationType and 4 NotificationPriority constants use PascalCase values
2. **Validation alignment**: kubebuilder `+kubebuilder:validation:Enum` annotations match the new constant values
3. **Routing fidelity**: `RoutingAttributesFromSpec` extracts PascalCase values into routing attributes
4. **Audit data fidelity**: Audit payloads carry the new PascalCase values through `string(spec.Type)` casts
5. **Display formatting**: `kubectl get notificationrequest` shows PascalCase TYPE and PRIORITY columns
6. **OpenAPI alignment**: Data storage OpenAPI spec enum values match CRD values (including adding missing `Completion`)
7. **Zero regressions**: All existing notification, RO, and authwebhook tests pass after migration

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/notification/...` |
| RO unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | `go test -coverpkg=.../notification/... -coverprofile` |
| Backward compatibility | 0 regressions | All pre-existing tests pass |
| Enum completeness | 10/10 | All NotificationType (6) + NotificationPriority (4) use PascalCase |

---

## 2. References

### 2.1 Authority

- Issue #640: NotificationRequest TYPE column uses lowercase instead of PascalCase
- BR-NOT-064: Notification lineage and audit correlation
- DD-NOT-005: Spec immutability

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [NotificationRequest CRD types](../../api/notification/v1alpha1/notificationrequest_types.go)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Routing rules in user deployments break | NR routing stops matching on type/priority | Medium | UT-NOT-640-003, UT-NOT-640-004 | Alpha API allows breaking changes; document in release notes |
| R2 | Audit data storage rejects new values | Audit events fail OpenAPI validation | High if OpenAPI not updated | UT-NOT-640-005, UT-NOT-640-006 | Update OpenAPI spec + regenerate ogen-client in same change |
| R3 | Missed hardcoded string causes test failure | CI failure | Low | All tests | Comprehensive grep + `go test ./...` catches all |
| R4 | Existing NR resources in clusters show old values | Mixed casing in kubectl output | Low | N/A (runtime) | NRs are transient; alpha API; release note |

### 3.1 Risk-to-Test Traceability

- **R1** → UT-NOT-640-003 (routing extracts PascalCase), UT-NOT-640-004 (routing constants updated)
- **R2** → UT-NOT-640-005 (audit payload uses PascalCase), UT-NOT-640-006 (OpenAPI enum includes Completion)
- **R3** → Full regression suite (`go test ./...`)

---

## 4. Scope

### 4.1 Features to be Tested

- **CRD enum values** (`api/notification/v1alpha1/notificationrequest_types.go`): All 10 enum constants produce PascalCase
- **Routing attribute extraction** (`pkg/notification/routing/resolver.go`): `RoutingAttributesFromSpec` produces PascalCase values
- **Routing constants** (`pkg/notification/routing/attributes.go`): Constants that mirror CRD values are updated
- **Audit payload mapping** (`pkg/notification/audit/manager.go`): `string(spec.Type)` and `string(spec.Priority)` produce PascalCase
- **Display formatting** (`pkg/notification/formatting/console.go`, `pkg/notification/delivery/slack_blocks.go`): `priorityEmoji` map uses new keys

### 4.2 Features Not to be Tested

- **ogen-client generated code**: Validated by regeneration, not manual tests (generated code excluded from coverage)
- **CRD YAML manifests**: Validated by `make manifests` regeneration
- **Channel and DeliveryAttemptStatus enums**: Not in printcolumns, out of scope per issue #640
- **Severity field**: Plain string, not a typed enum, not affected
- **EventCategoryApproval**: RAR audit domain, same word but different semantics

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Change enum values at source (Option A) | v1alpha1 allows breaking changes; clean long-term fix vs display-field patch |
| PascalCase with no hyphens | `status-update` → `StatusUpdate`, matching `PartiallySent` convention |
| Fix OpenAPI `completion` gap | Pre-existing drift; fix while touching the same enums |
| Do NOT change `Severity` constants | Different semantic domain; `Severity` is a plain string, not `NotificationPriority` |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code in `api/notification/v1alpha1/` and `pkg/notification/routing/`
- **Integration**: Existing integration tests updated to pass (no new integration tests needed — value change is compile-time)
- **E2E**: Deferred — enum value change is validated at unit/integration level; E2E tests updated for compatibility

### 5.2 Two-Tier Minimum

- **Unit tests**: Validate enum values, routing extraction, audit mapping, display formatting
- **Integration tests**: Updated assertions in existing tests validate end-to-end wiring

### 5.3 Business Outcome Quality Bar

Tests validate that operators see PascalCase values in `kubectl get notificationrequest` output,
routing rules match on PascalCase, and audit payloads carry PascalCase.

### 5.4 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. All 10 enum constants produce PascalCase values
3. `go test ./...` shows zero regressions
4. `go build ./...` succeeds
5. OpenAPI spec includes `Completion` in type enum

**FAIL**:
1. Any P0 test fails
2. Any pre-existing test regresses
3. Build fails

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken, cascading test failures from unrelated changes
**Resume**: Build fixed, root cause identified

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `api/notification/v1alpha1/notificationrequest_types.go` | Const definitions (NotificationType, NotificationPriority) | ~30 |
| `pkg/notification/routing/attributes.go` | Const definitions (routing type/severity) | ~25 |
| `pkg/notification/routing/resolver.go` | `RoutingAttributesFromSpec` | ~35 |
| `pkg/notification/formatting/console.go` | `ConsoleFormatter.Format` (priorityEmoji map) | ~25 |
| `pkg/notification/delivery/slack_blocks.go` | `FormatSlackBlocks` (priorityEmoji map) | ~50 |

### 6.2 Integration-Testable Code (I/O, wiring)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/audit/manager.go` | `EmitMessageSent`, `EmitDeliveryFailed` | ~80 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc7` HEAD | Branch |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-NOT-064 | Notification lineage and audit correlation | P0 | Unit | UT-NOT-640-001 | Pending |
| BR-NOT-064 | NotificationType enum PascalCase | P0 | Unit | UT-NOT-640-002 | Pending |
| BR-NOT-064 | Routing attribute extraction with PascalCase | P0 | Unit | UT-NOT-640-003 | Pending |
| BR-NOT-064 | Routing constants alignment | P1 | Unit | UT-NOT-640-004 | Pending |
| BR-NOT-064 | Audit payload PascalCase fidelity | P0 | Unit | UT-NOT-640-005 | Pending |
| BR-NOT-064 | OpenAPI enum completeness (Completion) | P1 | Unit | UT-NOT-640-006 | Pending |
| BR-NOT-064 | Display formatting with PascalCase priority | P0 | Unit | UT-NOT-640-007 | Pending |
| BR-NOT-064 | priorityEmoji map uses PascalCase keys | P1 | Unit | UT-NOT-640-008 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-NOT-640-NNN`

### Tier 1: Unit Tests

**Testable code scope**: `api/notification/v1alpha1/`, `pkg/notification/routing/`, `pkg/notification/formatting/`, `pkg/notification/delivery/`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-640-001` | All 4 NotificationPriority constants use PascalCase (Critical, High, Medium, Low) | Pending |
| `UT-NOT-640-002` | All 6 NotificationType constants use PascalCase (Escalation, Simple, StatusUpdate, Approval, ManualReview, Completion) | Pending |
| `UT-NOT-640-003` | `RoutingAttributesFromSpec` extracts PascalCase type and priority into attrs map | Pending |
| `UT-NOT-640-004` | Routing constants `NotificationTypeEscalation` and `NotificationTypeManualReview` match CRD PascalCase | Pending |
| `UT-NOT-640-005` | `string(NotificationType)` cast produces PascalCase for audit payload mapping | Pending |
| `UT-NOT-640-006` | All 6 NotificationType values are distinct and non-empty (enum completeness) | Pending |
| `UT-NOT-640-007` | Console formatter's priorityEmoji map resolves all 4 PascalCase priority values to correct emojis | Pending |
| `UT-NOT-640-008` | Slack block formatter's priorityEmoji map resolves all 4 PascalCase priority values to correct emojis | Pending |

### Tier Skip Rationale

- **Integration**: No new integration tests needed. The change is compile-time (constant values). Existing integration tests will be updated in REFACTOR phase to assert on PascalCase strings, validating end-to-end wiring.
- **E2E**: Deferred — enum values are validated at unit level. E2E tests updated for compatibility only.

---

## 9. Test Cases

### UT-NOT-640-001: NotificationPriority PascalCase values

**BR**: BR-NOT-064
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/enum_casing_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: The NotificationPriority type constants
2. **When**: Accessing each constant value
3. **Then**: `NotificationPriorityCritical` == `"Critical"`, `NotificationPriorityHigh` == `"High"`, `NotificationPriorityMedium` == `"Medium"`, `NotificationPriorityLow` == `"Low"`

### UT-NOT-640-002: NotificationType PascalCase values

**BR**: BR-NOT-064
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/enum_casing_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: The NotificationType type constants
2. **When**: Accessing each constant value
3. **Then**: `NotificationTypeEscalation` == `"Escalation"`, `NotificationTypeSimple` == `"Simple"`, `NotificationTypeStatusUpdate` == `"StatusUpdate"`, `NotificationTypeApproval` == `"Approval"`, `NotificationTypeManualReview` == `"ManualReview"`, `NotificationTypeCompletion` == `"Completion"`

### UT-NOT-640-003: RoutingAttributesFromSpec PascalCase extraction

**BR**: BR-NOT-064
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/enum_casing_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: A NotificationRequest with `Type: NotificationTypeManualReview`, `Priority: NotificationPriorityHigh`
2. **When**: Calling `RoutingAttributesFromSpec(nr)`
3. **Then**: `attrs["type"]` == `"ManualReview"`, `attrs["priority"]` == `"High"`

### UT-NOT-640-004: Routing constants match CRD PascalCase

**BR**: BR-NOT-064
**Priority**: P1
**Type**: Unit
**File**: `test/unit/notification/enum_casing_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: The routing constants in `pkg/notification/routing/attributes.go`
2. **When**: Accessing `NotificationTypeEscalation` and `NotificationTypeManualReview`
3. **Then**: Values match CRD PascalCase: `"Escalation"` and `"ManualReview"`

### UT-NOT-640-005: Audit payload carries PascalCase via string cast

**BR**: BR-NOT-064
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/enum_casing_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: A NotificationRequest with `Type: NotificationTypeApproval`, `Priority: NotificationPriorityLow`
2. **When**: Casting `string(spec.Type)` and `string(spec.Priority)`
3. **Then**: Results are `"Approval"` and `"Low"`

### UT-NOT-640-006: Enum completeness and distinctness

**BR**: BR-NOT-064
**Priority**: P1
**Type**: Unit
**File**: `test/unit/notification/enum_casing_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: All 6 NotificationType constants
2. **When**: Collecting all values into a set
3. **Then**: Set has exactly 6 distinct, non-empty PascalCase entries

### UT-NOT-640-007: Console priorityEmoji resolves PascalCase priorities

**BR**: BR-NOT-064
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/enum_casing_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: A NotificationRequest with `Priority: NotificationPriorityCritical`
2. **When**: Formatting with ConsoleFormatter
3. **Then**: Output contains priority emoji prefix (not the fallback "📢")

### UT-NOT-640-008: Slack priorityEmoji resolves PascalCase priorities

**BR**: BR-NOT-064
**Priority**: P1
**Type**: Unit
**File**: `test/unit/notification/enum_casing_test.go`

**Preconditions**: None

**Test Steps**:
1. **Given**: A NotificationRequest with each of the 4 priority levels
2. **When**: Calling `FormatSlackBlocks(nr)`
3. **Then**: Output blocks contain correct emoji for each priority (not fallback)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None required (pure value assertions)
- **Location**: `test/unit/notification/`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-gen | v0.16+ | CRD regeneration |
| ogen | v1.18.0 | OpenAPI client regeneration |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — all changes are self-contained.

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write failing tests for all 8 scenarios
2. **Checkpoint A**: Due diligence — verify tests fail for the right reasons
3. **Phase 2 (TDD GREEN)**: Change enum values, update validation annotations, update routing constants, update OpenAPI spec, regenerate CRD + ogen-client
4. **Checkpoint B**: Due diligence — verify all new tests pass, `go build ./...` succeeds
5. **Phase 3 (TDD REFACTOR)**: Update hardcoded strings in existing tests, update sample configs, update docs, verify zero regressions
6. **Checkpoint C**: Final due diligence — full regression suite, anti-pattern scan, coverage check

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/640/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/notification/enum_casing_test.go` | Ginkgo BDD test file |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (focused)
go test ./test/unit/notification/... -ginkgo.focus="UT-NOT-640" -v

# Full notification unit tests
go test ./test/unit/notification/... -count=1

# Full regression
go test ./test/unit/... -count=1
go test ./test/integration/... -count=1

# Coverage
go test ./test/unit/notification/... -coverpkg=github.com/jordigilh/kubernaut/api/notification/v1alpha1 -coverprofile=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/notification/crd_typed_fields_test.go:104` | `Equal("manual-review")` | `Equal("ManualReview")` | Type value changed |
| `test/unit/notification/crd_typed_fields_test.go:106` | `Equal("high")` | `Equal("High")` | Priority value changed |
| `test/integration/notification/routing_context_integration_test.go:58` | `Equal("manual-review")` | `Equal("ManualReview")` | Type value changed |
| `test/integration/notification/routing_context_integration_test.go:60` | `Equal("high")` | `Equal("High")` | Priority value changed |
| `test/integration/notification/crd_typed_fields_integration_test.go:65` | `Equal("manual-review")` | `Equal("ManualReview")` | Type value changed |
| `test/integration/notification/crd_typed_fields_integration_test.go:67` | `Equal("high")` | `Equal("High")` | Priority value changed |
| `test/unit/notification/controller_events_test.go:133,204` | `Priority: "critical"` | `Priority: "Critical"` | Priority value changed |
| `test/integration/authwebhook/notificationrequest_test.go:141` | `"notification_type": "escalation"` | `"notification_type": "Escalation"` | Type value changed |
| `test/integration/authwebhook/notificationrequest_test.go:142` | `"priority": "high"` | `"priority": "High"` | Priority value changed |
| `test/integration/authwebhook/notificationrequest_test.go:298` | `"notification_type": "status-update"` | `"notification_type": "StatusUpdate"` | Type value changed |
| `test/integration/authwebhook/notificationrequest_test.go:299` | `"priority": "low"` | `"priority": "Low"` | Priority value changed |
| `test/e2e/notification/02_audit_correlation_test.go:126` | `{"low","medium","high"}` | `{"Low","Medium","High"}` | Priority values changed |
| `test/e2e/notification/04_failed_delivery_audit_test.go:317,464,486` | `Equal("critical")` | `Equal("Critical")` | Priority in event_data changed |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
