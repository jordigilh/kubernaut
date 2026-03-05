# Test Plan: Fix #276 NT Audit Delivery Channels

**Feature**: Replace dead `recipients` field with `delivery_channels` in NotificationRequest audit payload; remove `Recipient` struct from CRD
**Version**: 1.0
**Created**: 2026-03-04
**Author**: Release Team
**Status**: Active
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- [BR-AUTH-001]: SOC2 CC8.1 Operator Attribution -- audit events must record WHERE notifications were delivered
- [BR-NOT-065]: Config-based routing is authoritative -- `spec.recipients` is dead, delivery channels come from `status.deliveryAttempts`
- [DD-WEBHOOK-003]: Webhook-Complete Audit Pattern -- business context in event_data, attribution in structured columns
- [DD-AUDIT-004]: Zero unstructured data in audit events

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `pkg/authwebhook/notificationrequest_validator.go`: Audit mapping logic in `ValidateDelete` -- replacing `spec.recipients` with `status.deliveryAttempts` channel extraction
- `api/notification/v1alpha1/notificationrequest_types.go`: Removal of `Recipient` struct and `spec.recipients` field
- `api/openapi/data-storage-v1.yaml`: Schema change from `recipients` (object array) to `delivery_channels` (string array)
- Channel extraction helper: new pure-logic function `extractDeliveryChannels`

### Out of Scope

- `pkg/datastorage/models/notification_audit.go:Recipient`: Per-delivery-attempt record with actual recipient string (different concept, works correctly)
- Notification controller delivery logic (not affected by this change)
- E2E tests (deferred -- this fix is scoped to webhook audit correctness, validated at UT+IT tiers)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Replace `recipients` with `delivery_channels` (string array) | `spec.recipients` is never populated (config-based routing per #260). `status.deliveryAttempts` contains the actual channels used. A flat string array (e.g., `["slack", "console"]`) is simpler and matches the business question "WHERE was it sent?" |
| Extract `extractDeliveryChannels` as a named helper | Isolates pure logic from webhook wiring; enables direct unit testing without envtest overhead |
| Remove `Recipient` struct from CRD entirely | Dead code -- RO never populates it. Removing prevents future confusion and reduces CRD surface area |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of `extractDeliveryChannels` helper (pure logic: deduplication, sorting, nil handling)
- **Integration**: >=80% of `ValidateDelete` webhook flow (I/O: admission request -> CRD read -> audit store write)
- **E2E**: Deferred -- webhook audit correctness is fully validated at IT tier via envtest

### 2-Tier Minimum

Every business requirement gap is covered by UT + IT:
- **Unit tests** catch channel extraction logic errors (wrong dedup, sort order, nil crash)
- **Integration tests** catch wiring errors (wrong field read, payload serialization, audit store delivery)

### Business Outcome Quality Bar

Tests validate **business outcomes**:
- "Does the audit event record the actual channels the notification was delivered to?"
- "Does the audit event handle the case where no delivery occurred?"
- "Can NRs be created without the removed `spec.recipients` field?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/authwebhook/notificationrequest_validator.go` | `extractDeliveryChannels` | ~15 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/authwebhook/notificationrequest_validator.go` | `ValidateDelete` (full webhook flow) | ~85 |
| `test/integration/authwebhook/notificationrequest_test.go` | INT-NR-01, INT-NR-03 scenarios | ~150 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUTH-001 | SOC2 CC8.1 operator attribution -- actual delivery channels | P0 | IT | IT-AW-276-001 | Pending |
| BR-AUTH-001 | SOC2 CC8.1 operator attribution -- empty delivery channels | P0 | IT | IT-AW-276-002 | Pending |
| BR-AUTH-001 | SOC2 CC8.1 operator attribution -- sorted deduplicated channels | P0 | UT | UT-AW-276-001 | Pending |
| BR-AUTH-001 | SOC2 CC8.1 operator attribution -- nil/empty attempts | P0 | UT | UT-AW-276-002 | Pending |
| BR-AUTH-001 | SOC2 CC8.1 operator attribution -- dedup across retries | P0 | UT | UT-AW-276-003 | Pending |
| BR-NOT-065 | Config-based routing is authoritative -- CRD field removed | P1 | IT | IT-AW-276-003 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `AW` (AuthWebhook)
- **BR_NUMBER**: `276` (GitHub issue)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `extractDeliveryChannels` in `pkg/authwebhook/notificationrequest_validator.go` -- 100% coverage targeted

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AW-276-001` | Channel extraction produces sorted, deduplicated list from mixed deliveryAttempts | Pending |
| `UT-AW-276-002` | Channel extraction handles nil/empty deliveryAttempts without crash | Pending |
| `UT-AW-276-003` | Channel extraction deduplicates retries (5 slack attempts -> 1 "slack" entry) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `ValidateDelete` webhook flow in `pkg/authwebhook/notificationrequest_validator.go` -- >=80% coverage targeted

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-AW-276-001` | Audit event records actual delivery channels when NR with deliveryAttempts is deleted | Pending |
| `IT-AW-276-002` | Audit event records empty delivery_channels when NR has no deliveryAttempts | Pending |
| `IT-AW-276-003` | NR created without spec.recipients compiles and deploys (CRD field removed) | Pending |

### Tier Skip Rationale

- **E2E**: Deferred -- webhook audit correctness is fully validated at IT tier via envtest + real audit store. E2E would add Kind cluster overhead without additional coverage for this specific fix.

---

## 6. Test Cases (Detail)

### UT-AW-276-001: Sorted deduplicated channels from mixed attempts

**BR**: BR-AUTH-001
**Type**: Unit
**File**: `test/unit/authwebhook/notificationrequest_delivery_channels_test.go`

**Given**: NR with deliveryAttempts [{Channel:"slack", Status:"success"}, {Channel:"console", Status:"success"}, {Channel:"slack", Status:"failed"}]
**When**: `extractDeliveryChannels` is called
**Then**: Result is `["console", "slack"]` (sorted alphabetically, deduplicated)

**Acceptance Criteria**:
- Result contains exactly 2 entries
- Entries are sorted alphabetically: "console" before "slack"
- Duplicate "slack" entries from retries are collapsed to 1

---

### UT-AW-276-002: Empty deliveryAttempts returns nil

**BR**: BR-AUTH-001
**Type**: Unit
**File**: `test/unit/authwebhook/notificationrequest_delivery_channels_test.go`

**Given**: NR with nil deliveryAttempts
**When**: `extractDeliveryChannels` is called
**Then**: Result is nil (no crash, no spurious data)

**Acceptance Criteria**:
- No panic on nil input
- Returns nil (not empty slice) to distinguish "no delivery" from "delivered to zero channels"

---

### UT-AW-276-003: Deduplication across retries

**BR**: BR-AUTH-001
**Type**: Unit
**File**: `test/unit/authwebhook/notificationrequest_delivery_channels_test.go`

**Given**: NR with 5 deliveryAttempts all for channel "slack" (retries)
**When**: `extractDeliveryChannels` is called
**Then**: Result is `["slack"]` (single entry)

**Acceptance Criteria**:
- Result contains exactly 1 entry
- 5 retry attempts for same channel produce single channel string

---

### IT-AW-276-001: Audit captures actual delivery channels on DELETE

**BR**: BR-AUTH-001
**Type**: Integration
**File**: `test/integration/authwebhook/notificationrequest_test.go`

**Given**: NR with status.deliveryAttempts populated [{Channel:"slack", Status:"success"}, {Channel:"console", Status:"success"}]
**When**: Operator deletes the NR
**Then**: Audit event contains `delivery_channels: ["console", "slack"]` and notification_type/priority/final_status are correct

**Acceptance Criteria**:
- Audit event `event_data` contains `delivery_channels` key (not `recipients`)
- `delivery_channels` value is `["console", "slack"]` (sorted)
- Other business fields (notification_name, notification_type, priority, final_status) remain correct

---

### IT-AW-276-002: Empty delivery_channels when no attempts

**BR**: BR-AUTH-001
**Type**: Integration
**File**: `test/integration/authwebhook/notificationrequest_test.go`

**Given**: NR that was cancelled before any delivery (empty deliveryAttempts)
**When**: Operator deletes the NR
**Then**: Audit event contains `delivery_channels: []` (empty array, not null)

**Acceptance Criteria**:
- Audit event `event_data` contains `delivery_channels` key
- Value is empty array `[]` (not null/missing) to indicate "no delivery occurred"

---

### IT-AW-276-003: NR creation succeeds without spec.recipients

**BR**: BR-NOT-065
**Type**: Integration
**File**: `test/integration/authwebhook/notificationrequest_test.go`

**Given**: NR spec without recipients field (field removed from CRD)
**When**: NR is created via envtest K8s API
**Then**: Creation succeeds with no validation error

**Acceptance Criteria**:
- NR creation does not fail due to missing `recipients` field
- CRD validation passes without the removed field

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None needed -- `extractDeliveryChannels` is pure logic
- **Location**: `test/unit/authwebhook/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**: envtest K8s API + real audit store client
- **Location**: `test/integration/authwebhook/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/authwebhook/... -ginkgo.focus="UT-AW-276"

# Integration tests
make test-integration-authwebhook

# All authwebhook tests
go test ./test/unit/authwebhook/... ./test/integration/authwebhook/...
```

---

## 9. Anti-Pattern Compliance

Per TESTING_GUIDELINES.md:

- No `time.Sleep()` -- use `Eventually()` for async operations
- No `Skip()` -- all tests implemented or not written
- No direct audit store testing -- test through `ValidateDelete` business flow
- No `ToNot(BeNil())` as sole assertion -- assert specific business outcomes
- Ginkgo/Gomega BDD framework (mandatory)

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for #276 |
