# Test Plan: Workflow Name Resolution in Notifications

**Feature**: Resolve workflow UUID to human-readable name in notification body before delivery
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.1.0-rc13`

**Authority**:
- [#553](https://github.com/jordigilh/kubernaut/issues/553): RO notification: workflow UUID shown instead of human-readable workflow name

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **`pkg/notification/enrichment/` (new)**: Body enrichment component that resolves workflow UUIDs to human-readable names via DataStorage catalog API before delivery
- **`pkg/notification/delivery/orchestrator.go`**: Integration of enrichment step into the delivery pipeline (between sanitization and channel delivery)
- **Completion notifications**: "Workflow Executed: {uuid}" → "Workflow Executed: {name}"
- **Approval notifications**: "Proposed Workflow: {uuid}" → "Proposed Workflow: {name}"
- **Graceful degradation**: When catalog lookup fails, the UUID is preserved (no delivery failure)

### Out of Scope

- **RO notification creation** (`pkg/remediationorchestrator/creator/notification.go`): Body construction is unchanged — RO continues to embed the UUID. Rendering is NT's responsibility.
- **AIAnalysis CRD changes**: No new fields on `SelectedWorkflow`
- **HAPI response processing**: No changes to `response_processor.go`
- **Notification CRD schema**: No spec changes — enrichment happens at delivery time, not CRD authoring time

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Enrichment in NT, not RO | Rendering/presentation is NT's responsibility. RO passes data; NT decides how to display it. |
| String replacement of UUID in body | Simple, deterministic. The UUID is a unique string unlikely to collide with other body content. |
| Graceful degradation on lookup failure | Notification delivery must never fail because of a cosmetic enrichment. UUID fallback is acceptable. |
| Metadata-driven resolution | NT reads `workflowId` (completion) or `selectedWorkflow` (approval) from `Spec.Metadata` to identify the UUID to resolve. No body parsing needed. |
| `WorkflowNameResolver` interface | Decouples enrichment logic from DataStorage HTTP transport. Enables unit testing with mock resolvers. |
| Enrichment in `DeliverToChannels` before channel loop | Single enrichment call shared by all channels. Applied before per-channel sanitization, so the resolved name passes through the existing sanitizer in `doDelivery`. |
| DeepCopy before enrichment | Enrichment operates on a copy of the notification to avoid mutating the controller's cached object. Same pattern as `sanitizeNotification`. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (enrichment logic, metadata extraction, string replacement, fallback paths)
- **Integration**: >=80% of **integration-testable** code (full delivery pipeline with enrichment wired into reconciler, DataStorage HTTP mock)

### 2-Tier Minimum

Every business requirement gap is covered by at least 2 test tiers (UT + IT):
- **Unit tests** validate enrichment logic in isolation (mock resolver)
- **Integration tests** validate the full pipeline: notification CRD → reconciler → enrichment → delivery

### Business Outcome Quality Bar

Tests validate **business outcomes** — "the operator sees a human-readable workflow name in the notification" — not internal implementation details.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/enrichment/enricher.go` (new) | `NewEnricher`, `EnrichNotification`, `extractWorkflowID`, `replaceWorkflowIDInBody` | ~60 |
| `pkg/notification/enrichment/resolver.go` (new) | `WorkflowNameResolver` interface | ~10 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/notification/delivery/orchestrator.go` | `DeliverToChannels` (enrichment call site, before channel loop) | ~10 (delta) |
| `pkg/notification/enrichment/ds_resolver.go` (new) | `DataStorageResolver.ResolveWorkflowName` (HTTP call to DS) | ~40 |
| `cmd/notification/main.go` | Startup wiring (resolver + enricher injection) | ~10 (delta) |

---

## 4. BR Coverage Matrix

| BR/Issue | Description | Priority | Tier | Test ID | Status |
|----------|-------------|----------|------|---------|--------|
| #553 | Completion notification shows workflow name instead of UUID | P0 | Unit | UT-NOT-553-001 | Pending |
| #553 | Approval notification shows workflow name instead of UUID | P0 | Unit | UT-NOT-553-002 | Pending |
| #553 | Approval with ActionType: "ActionType (name)" format preserved | P1 | Unit | UT-NOT-553-003 | Pending |
| #553 | Graceful degradation: resolver failure keeps UUID | P0 | Unit | UT-NOT-553-004 | Pending |
| #553 | Graceful degradation: workflow not found keeps UUID | P1 | Unit | UT-NOT-553-005 | Pending |
| #553 | No-op when no workflow metadata present | P1 | Unit | UT-NOT-553-006 | Pending |
| #553 | Metadata key `workflowId` extracted for completion | P1 | Unit | UT-NOT-553-007 | Pending |
| #553 | Metadata key `selectedWorkflow` extracted for approval | P1 | Unit | UT-NOT-553-008 | Pending |
| #553 | Subject line unchanged (enrichment applies to body only) | P1 | Unit | UT-NOT-553-009 | Pending |
| #553 | Metadata preserved after enrichment | P1 | Unit | UT-NOT-553-010 | Pending |
| #553 | Nil resolver is no-op (defensive) | P2 | Unit | UT-NOT-553-011 | Pending |
| #553 | Enrichment operates on a copy, not the original notification | P1 | Unit | UT-NOT-553-012 | Pending |
| #553 | End-to-end delivery with resolved workflow name | P0 | Integration | IT-NOT-553-001 | Pending |
| #553 | Delivery succeeds when DataStorage is unavailable | P0 | Integration | IT-NOT-553-002 | Pending |
| #553 | All channels receive enriched body | P1 | Integration | IT-NOT-553-003 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `NOT` (Notification)
- **ISSUE**: `553`
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/notification/enrichment/` — >=80% coverage of enrichment logic

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-553-001` | Operator sees workflow name (not UUID) in completion notification body | Pending |
| `UT-NOT-553-002` | Operator sees workflow name (not UUID) in approval notification body | Pending |
| `UT-NOT-553-003` | Approval notification preserves "ActionType (name)" format when ActionType is present | Pending |
| `UT-NOT-553-004` | Notification delivery is not impacted when workflow catalog is unavailable | Pending |
| `UT-NOT-553-005` | UUID preserved when workflow is not found in catalog (deleted/expired) | Pending |
| `UT-NOT-553-006` | Notifications without workflow metadata (e.g., bulk duplicate) are unmodified | Pending |
| `UT-NOT-553-007` | Completion notification metadata key `workflowId` correctly drives resolution | Pending |
| `UT-NOT-553-008` | Approval notification metadata key `selectedWorkflow` correctly drives resolution | Pending |
| `UT-NOT-553-009` | Subject line is never modified by enrichment | Pending |
| `UT-NOT-553-010` | All metadata fields preserved after enrichment | Pending |
| `UT-NOT-553-011` | Nil resolver results in no-op enrichment (defensive) | Pending |
| `UT-NOT-553-012` | Enrichment operates on a copy — original notification is not mutated | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Delivery pipeline with enrichment wired into reconciler — >=80% coverage of integration-testable enrichment I/O

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-553-001` | Full delivery pipeline resolves UUID to workflow name and delivers enriched body | Pending |
| `IT-NOT-553-002` | Delivery completes successfully when DataStorage is down (graceful degradation) | Pending |
| `IT-NOT-553-003` | Multiple channels all receive body with resolved workflow name | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. E2E tests require a running DataStorage with a populated catalog and a Kind cluster. The enrichment logic is fully covered at UT + IT tiers. E2E coverage for the notification body content can be added in a future iteration when the full pipeline test harness is exercised.

---

## 6. Test Cases (Detail)

### UT-NOT-553-001: Completion notification body shows workflow name

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A `NotificationRequest` with:
- `Spec.Body` containing `"**Workflow Executed**: 53c7c5d3-ee13-42e5-a920-43f3df75ec6d"`
- `Spec.Metadata["workflowId"]` = `"53c7c5d3-ee13-42e5-a920-43f3df75ec6d"`
- A mock `WorkflowNameResolver` that returns `"oom-recovery"` for that UUID

**When**: `EnrichNotification` is called

**Then**: `Spec.Body` contains `"**Workflow Executed**: oom-recovery"` and does not contain the UUID

**Acceptance Criteria**:
- Body string replacement is exact (UUID → name)
- No other body content is modified

---

### UT-NOT-553-002: Approval notification body shows workflow name

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A `NotificationRequest` with:
- `Spec.Body` containing `"**Proposed Workflow**: 53c7c5d3-ee13-42e5-a920-43f3df75ec6d"`
- `Spec.Metadata["selectedWorkflow"]` = `"53c7c5d3-ee13-42e5-a920-43f3df75ec6d"`
- A mock resolver returning `"crashloop-config-fix"`

**When**: `EnrichNotification` is called

**Then**: `Spec.Body` contains `"**Proposed Workflow**: crashloop-config-fix"` and not the UUID

**Acceptance Criteria**:
- UUID replaced in approval body format
- Metadata key `selectedWorkflow` correctly identifies the UUID

---

### UT-NOT-553-003: Approval with ActionType preserves format

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A `NotificationRequest` with:
- `Spec.Body` containing `"**Proposed Workflow**: ScaleReplicas (53c7c5d3-ee13-42e5-a920-43f3df75ec6d)"`
- `Spec.Metadata["selectedWorkflow"]` = `"53c7c5d3-ee13-42e5-a920-43f3df75ec6d"`
- A mock resolver returning `"scale-replicas-v1"`

**When**: `EnrichNotification` is called

**Then**: `Spec.Body` contains `"**Proposed Workflow**: ScaleReplicas (scale-replicas-v1)"`

**Acceptance Criteria**:
- UUID inside parentheses is replaced; surrounding format preserved
- ActionType label is untouched

---

### UT-NOT-553-004: Graceful degradation on resolver failure

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A `NotificationRequest` with UUID in body and metadata, and a mock resolver that returns an error

**When**: `EnrichNotification` is called

**Then**: `Spec.Body` is unchanged (still contains UUID), no error propagated

**Acceptance Criteria**:
- Original body content is byte-identical to input
- No panic or delivery failure
- Warning logged (verifiable via log capture in test)

---

### UT-NOT-553-005: Workflow not found preserves UUID

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A mock resolver returning empty string (workflow deleted or not found)

**When**: `EnrichNotification` is called

**Then**: Body unchanged

**Acceptance Criteria**:
- Empty resolved name is treated as "not found"
- UUID remains in body

---

### UT-NOT-553-006: No-op when no workflow metadata present

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A `NotificationRequest` with no `workflowId` or `selectedWorkflow` in metadata (e.g., bulk duplicate notification)

**When**: `EnrichNotification` is called

**Then**: Body and metadata unchanged, resolver never called

**Acceptance Criteria**:
- Mock resolver's `ResolveWorkflowName` is never invoked
- Body is byte-identical

---

### UT-NOT-553-007: Completion metadata key `workflowId`

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: Metadata with `workflowId` = `"abc-123"`

**When**: `extractWorkflowID` is called

**Then**: Returns `"abc-123"`

**Acceptance Criteria**:
- `workflowId` key takes precedence when both keys are present

---

### UT-NOT-553-008: Approval metadata key `selectedWorkflow`

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: Metadata with only `selectedWorkflow` = `"def-456"` (no `workflowId`)

**When**: `extractWorkflowID` is called

**Then**: Returns `"def-456"`

**Acceptance Criteria**:
- Falls back to `selectedWorkflow` when `workflowId` absent

---

### UT-NOT-553-009: Subject line unchanged

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A notification with `Subject = "Remediation Completed: high-memory-alert"` and UUID in body

**When**: `EnrichNotification` is called

**Then**: Subject is byte-identical to input

**Acceptance Criteria**:
- Enrichment only modifies `Spec.Body`

---

### UT-NOT-553-010: Metadata preserved after enrichment

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A notification with metadata `{"workflowId": "uuid", "executionEngine": "job", "rootCause": "OOM"}`

**When**: `EnrichNotification` is called with a resolver returning `"oom-recovery"`

**Then**: All metadata key-value pairs are preserved exactly

**Acceptance Criteria**:
- Metadata map has same length and content post-enrichment
- `workflowId` value in metadata is still the UUID (metadata is for audit, not display)

---

### UT-NOT-553-011: Nil resolver is no-op

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: Enricher constructed with `nil` resolver

**When**: `EnrichNotification` is called

**Then**: Notification returned unchanged, no panic

**Acceptance Criteria**:
- Nil-safe design — orchestrator can be created without a resolver (e.g., in tests)

---

### UT-NOT-553-012: Enrichment operates on a copy

**Issue**: #553
**Type**: Unit
**File**: `test/unit/notification/enrichment_test.go`

**Given**: A `NotificationRequest` with UUID in body and a mock resolver returning `"oom-recovery"`

**When**: `EnrichNotification` is called

**Then**: The returned notification has the enriched body, but the original notification object is unmodified (body still contains UUID)

**Acceptance Criteria**:
- Original `Spec.Body` is byte-identical before and after the call
- Returned notification is a distinct object (not same pointer)
- Prevents mutation of controller's cached CRD objects

---

### IT-NOT-553-001: Full delivery pipeline with resolved name

**Issue**: #553
**Type**: Integration
**File**: `test/integration/notification/enrichment_delivery_test.go`

**Given**: A `NotificationRequest` CRD with UUID in body, metadata with `workflowId`, and a mock DataStorage HTTP server returning `{"workflowName": "oom-recovery"}`

**When**: The notification reconciler processes the CRD through to delivery

**Then**: The delivered notification body contains `"oom-recovery"` instead of the UUID

**Acceptance Criteria**:
- DataStorage HTTP endpoint is called with correct workflow ID
- Delivered body verified via mock channel capture
- Audit trail reflects the enriched body

---

### IT-NOT-553-002: Graceful degradation with DataStorage unavailable

**Issue**: #553
**Type**: Integration
**File**: `test/integration/notification/enrichment_delivery_test.go`

**Given**: A `NotificationRequest` CRD with UUID in body, and DataStorage HTTP endpoint returning 503

**When**: The notification reconciler processes the CRD

**Then**: Delivery completes successfully with the UUID in the body (graceful degradation)

**Acceptance Criteria**:
- Notification transitions to Delivered phase
- Body contains original UUID (not enriched)
- No error or retry triggered by enrichment failure

---

### IT-NOT-553-003: All channels receive enriched body

**Issue**: #553
**Type**: Integration
**File**: `test/integration/notification/enrichment_delivery_test.go`

**Given**: A notification routed to console and log channels, resolver returns `"crashloop-config-fix"`

**When**: Both channels deliver the notification

**Then**: Both channels receive the enriched body with `"crashloop-config-fix"`

**Acceptance Criteria**:
- Enrichment is applied once, before per-channel delivery
- Both mock channel captures show the resolved name

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `WorkflowNameResolver` mock (in-test, not external dependency)
- **Location**: `test/unit/notification/enrichment_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for notification delivery pipeline. DataStorage simulated via `httptest` server (real HTTP, mock response).
- **Infrastructure**: envtest for K8s CRDs, httptest for DataStorage API
- **Location**: `test/integration/notification/enrichment_delivery_test.go`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/notification/... -ginkgo.focus="553"

# Integration tests
make test-integration-notification

# Specific test by ID
go test ./test/unit/notification/... -ginkgo.focus="UT-NOT-553-001"
go test ./test/integration/notification/... -ginkgo.focus="IT-NOT-553-001"
```

---

## 9. Risk Analysis

### R1: UUID collision in body text

**Risk**: A UUID string could theoretically appear in other body content (e.g., a root cause analysis quoting a resource UUID).
**Likelihood**: Very low — UUIDs are 36-character strings with specific format; root cause text rarely contains raw UUIDs.
**Mitigation**: The replacement is targeted (only the UUID from metadata is replaced). If this becomes an issue, a future iteration can use positional markers.

### R2: DataStorage latency adds to delivery time

**Risk**: The catalog lookup adds an HTTP round-trip to each notification delivery.
**Likelihood**: Medium — DataStorage is typically co-located in the cluster.
**Mitigation**: Graceful degradation ensures delivery is never blocked. Future optimization: add an in-memory cache with short TTL (workflow names rarely change).

### R3: Resolved name bypasses sanitization

**Risk**: If enrichment happened after sanitization, the resolved workflow name could contain content that should be sanitized (XSS, injection).
**Likelihood**: Very low — workflow names are simple slugs. But defense-in-depth matters.
**Mitigation**: Enrichment happens in `DeliverToChannels` (before channel loop), and sanitization happens in `doDelivery` (per channel). The resolved name passes through the existing sanitizer. No code change needed — the pipeline order handles this naturally.

### R4: Metadata key inconsistency between notification types

**Risk**: Completion uses `workflowId`, approval uses `selectedWorkflow`. If new notification types are added with different keys, the enricher won't resolve them.
**Likelihood**: Low — new notification types would follow the existing pattern.
**Mitigation**: `extractWorkflowID` checks both keys. Documented in design decisions.

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
