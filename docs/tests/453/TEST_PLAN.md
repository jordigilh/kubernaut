# Test Plan: Refactor NotificationRequest CRD — Replace Unstructured String Fields with Typed Structs

**Feature**: Replace bare `string` and `map[string]string` fields in the NotificationRequest CRD with named types, kubebuilder enums, and a nested `NotificationContext` struct
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.2`

**Authority**:
- [BR-NOT-058]: CRD Validation and Data Sanitization — schema must enforce valid field values
- [BR-NOT-065]: Channel Routing Based on Spec Fields — routing reads `spec.context` (via `FlattenToMap`), `spec.extensions`, and top-level spec fields
- [BR-NOT-064]: Audit Event Correlation — audit payloads flatten `Spec.Context` + `Spec.Extensions` for correlation
- [BR-ORCH-001, BR-ORCH-034, BR-ORCH-036, BR-ORCH-045]: RO notification creation (approval, bulk duplicate, manual review, completion)
- Project coding standard: "AVOID using `any` or `interface{}` — ALWAYS use structured field values with specific types"

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Issue #453](https://github.com/jordigilh/kubernaut/issues/453)
- [Testing Guidelines — Anti-Patterns](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 1. Scope

### In Scope

This plan is split into two execution phases to reduce per-phase risk.

- **Phase A — Enum-only field typing**: Type 5 bare `string` fields with named types and kubebuilder enums where the value set is closed. No `Metadata` changes.
  - `spec.reviewSource` → `ReviewSourceType` enum
  - `status.deliveryAttempts[].status` → `DeliveryAttemptStatus` enum
  - `status.deliveryAttempts[].channel` → `DeliveryChannelName` alias
  - `status.reason` → `NotificationStatusReason` enum
  - `spec.actionLinks[].service` → `ActionLinkServiceType` alias

- **Phase B — Metadata decomposition**: Replace `Metadata map[string]string` with a nested `NotificationContext` struct and `Extensions map[string]string` for routing/custom data.
  - 7 sub-structs: Lineage, Workflow, Analysis, Review, Execution, Dedup, Target
  - `FlattenToMap()` method for routing and audit backward compatibility
  - Update 6 producer functions, 4 consumer sites, ~25 test files

### Out of Scope

- **`spec.severity`**: Kept as `string`. Signal-derived (open set from Prometheus alerts); a kubebuilder enum would reject valid severities. Typing as a named alias requires explicit casts at all 5 producer sites with minimal benefit.
- **`spec.phase`**: Kept as `string`. Cross-API reference to `RemediationPhase` (remediation API group); importing creates a dependency the notification API doesn't have today.
- **OpenAPI audit payload schema**: The `metadata` field in `NotificationMessageSentPayload` etc. remains `map[string]string`. `FlattenToMap()` bridges the gap.
- **Notification controller reconciliation logic**: No behavioral changes to phase transitions, retry, or delivery.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Two-phase execution (enum-only first, then Metadata) | Reduces per-commit blast radius; Phase A is low-risk (~8 files), Phase B is higher-risk (~30 files) |
| Nested sub-structs for NotificationContext | Better semantic organization; each sub-struct is optional (nil = not applicable for this notification type) |
| Keep `Extensions map[string]string` alongside `Context` | Routing rules need an open-ended map for arbitrary matching keys (e.g., `test-channel-set` in tests, future vendor extensions) |
| `FlattenToMap()` for backward compatibility | Audit payloads and log delivery expect `map[string]string`; flattening preserves identical key names |
| Drop `severity` and `source` from Context | Redundant with `spec.severity` and `spec.reviewSource`; routing resolver already reads spec fields first (these metadata keys never win precedence) |
| Keep `Severity`/`Phase` as `string` | Open-ended / cross-API; see Out of Scope |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of new unit-testable code (type constructors, FlattenToMap, enum constants, routing attribute extraction)
- **Integration**: >=80% of new integration-testable code (producer functions, routing resolver, audit manager, CRD validation via envtest)

### 2-Tier Minimum

Every business requirement gap is covered by at least 2 test tiers:
- **Unit tests**: Validate type safety, FlattenToMap correctness, enum constant completeness, nil-safety
- **Integration tests**: Validate CRD admission rejects invalid enums, routing produces identical channels, audit payloads preserve metadata, producer functions create valid typed CRDs

### Business Outcome Quality Bar

Tests validate **business outcomes** — behavior, correctness, and data accuracy — not implementation details:
- "Operator gets CRD validation that rejects unknown ReviewSource at admission time" (not "ReviewSourceType.String() works")
- "Routing system delivers to same channels after Metadata migration" (not "RoutingAttributesFromSpec returns a map")
- "Audit trail contains identical metadata for SOC2 compliance" (not "FlattenToMap key count matches")

### Anti-Pattern Avoidance

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md):

| Anti-Pattern | How We Avoid It |
|---|---|
| `time.Sleep()` before assertions | Use `Eventually()` for all async K8s operations in integration tests |
| `Skip()` | Absolutely forbidden. All tests implemented or removed from plan |
| Direct audit infrastructure testing | Test business logic (notification creation) that emits audit as side effect; do NOT test `audit.Manager.CreateMessageSentEvent` directly |
| Direct metrics method calls | Test business logic that increments metrics; do NOT call metrics methods directly |
| HTTP testing in integration tests | Use direct business logic calls (e.g., `creator.CreateApprovalNotification()`), not HTTP endpoints |

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `api/notification/v1alpha1/notificationrequest_types.go` | New type aliases, enum constants, `NotificationContext` struct, `FlattenToMap()` | ~120 |
| `pkg/notification/routing/resolver.go` | `RoutingAttributesFromSpec` (updated to read Context + Extensions) | ~40 |
| `pkg/notification/routing/attributes.go` | Routing attribute constants (unchanged but verified) | ~30 |
| `pkg/remediationorchestrator/creator/notification.go` | `buildManualReviewMetadata` → `buildManualReviewContext` (pure struct construction) | ~40 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/creator/notification.go` | `CreateApprovalNotification`, `CreateCompletionNotification`, `CreateBulkDuplicateNotification`, `CreateManualReviewNotification` (K8s client.Create) | ~300 |
| `internal/controller/remediationorchestrator/reconciler.go` | `handleGlobalTimeout`, `createPhaseTimeoutNotification` (K8s client.Create) | ~60 |
| `pkg/notification/audit/manager.go` | `CreateMessageSentEvent`, `CreateMessageFailedEvent`, `CreateMessageAcknowledgedEvent`, `CreateMessageEscalatedEvent` (payload construction from typed fields) | ~120 |
| `pkg/notification/delivery/log.go` | `LogDeliveryService.Deliver` (reads Context/Extensions for structured log) | ~30 |

---

## 4. BR Coverage Matrix

### Phase A: Enum-Only Fields

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-NOT-058 | CRD rejects unknown ReviewSource values at admission | P0 | Unit | UT-NOT-453A-001 | Implemented |
| BR-NOT-058 | CRD rejects unknown DeliveryAttemptStatus values at admission | P0 | Unit | UT-NOT-453A-002 | Implemented |
| BR-NOT-058 | NotificationStatusReason constants cover all known status reasons | P1 | Unit | UT-NOT-453A-003 | Implemented |
| BR-NOT-065 | Routing attribute map is identical when ReviewSource uses typed enum | P0 | Unit | UT-NOT-453A-004 | Implemented |
| BR-ORCH-036 | Manual review notification created with typed ReviewSource field | P0 | Integration | IT-NOT-453A-001 | Implemented |
| BR-NOT-058 | CRD round-trips typed enum fields through K8s API without data loss | P0 | Integration | IT-NOT-453A-002 | Implemented |

### Phase B: Metadata Decomposition

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-001 | Approval notification context contains lineage, workflow, and analysis data | P0 | Unit | UT-NOT-453B-001 | Implemented |
| BR-ORCH-045 | Completion notification context contains lineage, workflow, analysis, and outcome | P0 | Unit | UT-NOT-453B-002 | Implemented |
| BR-ORCH-034 | Bulk duplicate notification context contains lineage and dedup count | P0 | Unit | UT-NOT-453B-003 | Implemented |
| BR-ORCH-036 | Manual review notification context contains review and execution data | P0 | Unit | UT-NOT-453B-004 | Implemented |
| BR-NOT-065 | FlattenToMap produces identical routing attribute keys as old Metadata map | P0 | Unit | UT-NOT-453B-005 | Implemented |
| BR-NOT-064 | FlattenToMap preserves all metadata keys for audit correlation | P0 | Unit | UT-NOT-453B-006 | Implemented |
| BR-NOT-065 | Extensions map preserves arbitrary routing keys not in typed schema | P0 | Unit | UT-NOT-453B-007 | Implemented |
| BR-NOT-058 | Nil sub-structs in NotificationContext are safely handled (no panics) | P0 | Unit | UT-NOT-453B-008 | Implemented |
| BR-NOT-058 | Redundant keys (severity, source) are dropped without data loss | P1 | Unit | UT-NOT-453B-009 | Implemented |
| BR-NOT-065 | Routing rules produce identical channel selection with Context+Extensions | P0 | Integration | IT-NOT-453B-001 | Implemented |
| BR-NOT-064 | Audit payloads contain identical metadata keys after Context migration | P0 | Integration | IT-NOT-453B-002 | Implemented |
| BR-ORCH-001 | Approval notification created with valid typed Context via K8s API | P0 | Integration | IT-NOT-453B-003 | Implemented |
| BR-ORCH-036 | Manual review notification (AI source) created with typed Context via K8s API | P0 | Integration | IT-NOT-453B-004 | Implemented |
| BR-ORCH-036 | Manual review notification (WE source) created with typed Context including retry data | P0 | Integration | IT-NOT-453B-005 | Implemented |
| BR-ORCH-045 | Completion notification created with typed Context via K8s API | P0 | Integration | IT-NOT-453B-006 | Implemented |
| BR-ORCH-034 | Bulk duplicate notification created with typed Context via K8s API | P0 | Integration | IT-NOT-453B-007 | Implemented |
| BR-NOT-058 | Timeout notifications (global + phase) created with typed Context including Target and Execution sub-structs | P0 | Integration | IT-NOT-453B-008 | Implemented |
| BR-NOT-058 | Log delivery service outputs structured Context and Extensions in log payload | P1 | Integration | IT-NOT-453B-009 | Implemented |

### Status Legend

- Implemented: Test code exists and passes
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-NOT-453{PHASE}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `NOT` (Notification)
- **453**: Issue number
- **PHASE**: `A` (enum-only) or `B` (Metadata decomposition)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Phase A — Tier 1: Unit Tests

**Testable code scope**: `api/notification/v1alpha1/notificationrequest_types.go` (new types/enums), `pkg/notification/routing/resolver.go` (attribute extraction). Target >=80% of new unit-testable code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-453A-001` | Operator deploying a NotificationRequest with `reviewSource: "InvalidSource"` gets a CRD validation error at admission time, preventing invalid data from entering the system | RED |
| `UT-NOT-453A-002` | Operator deploying a NotificationRequest where delivery attempts record `status: "unknown"` gets a CRD validation error, ensuring only `success`/`failed`/`timeout`/`invalid` are persisted | RED |
| `UT-NOT-453A-003` | All known notification status reasons (`AllDeliveriesSucceeded`, `PartialDeliverySuccess`, etc.) have compile-time constants, preventing typo-based bugs in controller code | RED |
| `UT-NOT-453A-004` | When a notification has `ReviewSource: ReviewSourceAIAnalysis`, the routing attribute map contains `review-source: "AIAnalysis"` — identical to the old `string` behavior | RED |

### Phase A — Tier 2: Integration Tests

**Testable code scope**: `pkg/remediationorchestrator/creator/notification.go` (manual review creator), CRD validation via envtest. Target >=80% of new integration-testable code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-453A-001` | When AIAnalysis fails with WorkflowResolutionFailed, the RO creates a manual review notification with typed `ReviewSource: "AIAnalysis"` that persists correctly in the K8s API and routes to the expected channel | RED |
| `IT-NOT-453A-002` | A NotificationRequest with typed enum fields (ReviewSource, DeliveryAttemptStatus) round-trips through K8s API (Create → Get) without data loss or type coercion issues | RED |

### Phase B — Tier 1: Unit Tests

**Testable code scope**: `NotificationContext` struct construction, `FlattenToMap()`, routing attribute extraction. Target >=80% of new unit-testable code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-453B-001` | When a remediation requires approval, the notification context correctly captures lineage (RR name, AIAnalysis name), workflow details (ID, confidence), and analysis data (approval reason) as typed fields — enabling operators to inspect structured data instead of parsing a flat key-value bag | RED |
| `UT-NOT-453B-002` | When a remediation completes successfully, the notification context correctly captures lineage, workflow ID, execution engine, root cause analysis, and outcome as typed fields | RED |
| `UT-NOT-453B-003` | When duplicate signals are resolved, the notification context correctly captures lineage and duplicate count | RED |
| `UT-NOT-453B-004` | When manual review is required (AIAnalysis or WorkflowExecution source), the notification context correctly captures review details (reason, subReason, humanReviewReason) and execution data (retry count, exit code) as typed fields | RED |
| `UT-NOT-453B-005` | `FlattenToMap()` produces a `map[string]string` with identical key names and values as the old `Metadata` map for each notification type (approval, completion, bulk, manual review, global timeout, phase timeout), ensuring routing compatibility | RED |
| `UT-NOT-453B-006` | `FlattenToMap()` output preserves all keys needed for audit correlation (`remediationRequest`, `aiAnalysis`, etc.) and SOC2 compliance | RED |
| `UT-NOT-453B-007` | Arbitrary routing keys in `Extensions` (e.g., `test-channel-set`, `environment`, `skip-reason`) appear in `FlattenToMap()` output and in routing attribute extraction, enabling configurable routing rules | RED |
| `UT-NOT-453B-008` | When a notification type only populates some sub-structs (e.g., bulk duplicate has no Workflow/Analysis), nil sub-structs are safely skipped in `FlattenToMap()` without panics and produce no keys in the output map | RED |
| `UT-NOT-453B-009` | The redundant `severity` and `source` keys (previously duplicated from `spec.severity` and `spec.reviewSource`) are NOT present in `FlattenToMap()` output, since routing already reads the spec fields directly | RED |

### Phase B — Tier 2: Integration Tests

**Testable code scope**: All 6 producer functions (4 in `creator/notification.go` + 2 in `reconciler.go`), routing resolver, audit manager (4 methods — 1 tested explicitly, 3 structurally identical), log delivery. Target >=80% of new integration-testable code (~215 lines, 9 tests cover ~195 lines = ~91%).

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-453B-001` | Given routing rules that match on `type`, `severity`, `review-source`, and `environment` (from Extensions), the routing system selects identical channels as before the Metadata migration — zero behavioral change for operators | RED |
| `IT-NOT-453B-002` | When a notification is delivered, the audit event's `metadata` field (in DataStorage) contains the same key-value pairs as before the migration, ensuring SOC2 audit trail continuity | RED |
| `IT-NOT-453B-003` | The RO creates an approval notification with a valid typed Context (Lineage + Workflow + Analysis sub-structs populated) that persists in the K8s API and passes CRD schema validation | RED |
| `IT-NOT-453B-004` | The RO creates a manual review notification (AI source) with typed Context (Lineage + Review sub-structs) that persists correctly and includes `humanReviewReason` when HAPI reports `needs_human_review=true` | RED |
| `IT-NOT-453B-005` | The RO creates a manual review notification (WE source) with typed Context (Lineage + Review + Execution sub-structs) that persists correctly and includes retry count, max retries, and last exit code | RED |
| `IT-NOT-453B-006` | The RO creates a completion notification with typed Context (Lineage + Workflow + Analysis sub-structs) that persists correctly and includes outcome and root cause | RED |
| `IT-NOT-453B-007` | The RO creates a bulk duplicate notification with typed Context (Lineage + Dedup sub-structs) that persists correctly and includes duplicate count | RED |
| `IT-NOT-453B-008` | The RO reconciler creates timeout notifications (global and phase) with typed Context (Lineage + Execution + Target sub-structs) that persist correctly and include timeoutPhase, phaseTimeout, and targetResource | RED |
| `IT-NOT-453B-009` | The log delivery service receives a notification with typed Context and Extensions and outputs a structured log entry containing all context fields and extension keys — enabling log-based debugging and alerting | RED |

### Tier Skip Rationale

- **E2E**: Not included in this plan. The refactoring is schema-level with no behavioral changes. Existing E2E tests (notification lifecycle, audit correlation, file delivery) exercise the full production code path and will detect any regression when run against the updated CRD. A separate E2E pass after both phases validates end-to-end.

---

## 6. Test Cases (Detail)

### Phase A

#### UT-NOT-453A-001: ReviewSourceType enum validation

**BR**: BR-NOT-058
**Type**: Unit
**File**: `test/unit/notification/crd_typed_fields_test.go`

**Given**: A `ReviewSourceType` named string type with kubebuilder enum `+kubebuilder:validation:Enum=AIAnalysis;WorkflowExecution`
**When**: Constants `ReviewSourceAIAnalysis` and `ReviewSourceWorkflowExecution` are referenced
**Then**: Both constants are defined and their string values match the enum declaration

**Acceptance Criteria**:
- `ReviewSourceAIAnalysis` == `"AIAnalysis"`
- `ReviewSourceWorkflowExecution` == `"WorkflowExecution"`
- No other `ReviewSourceType` constants exist (enum is closed)
- Type is assignable from string literals in struct initializers

---

#### UT-NOT-453A-002: DeliveryAttemptStatus enum validation

**BR**: BR-NOT-058
**Type**: Unit
**File**: `test/unit/notification/crd_typed_fields_test.go`

**Given**: A `DeliveryAttemptStatus` named string type with kubebuilder enum
**When**: Constants for all 4 statuses are referenced
**Then**: Constants match documented values: `success`, `failed`, `timeout`, `invalid`

**Acceptance Criteria**:
- `DeliveryAttemptStatusSuccess` == `"success"`
- `DeliveryAttemptStatusFailed` == `"failed"`
- `DeliveryAttemptStatusTimeout` == `"timeout"`
- `DeliveryAttemptStatusInvalid` == `"invalid"`

---

#### UT-NOT-453A-003: NotificationStatusReason constants completeness

**BR**: BR-NOT-058
**Type**: Unit
**File**: `test/unit/notification/crd_typed_fields_test.go`

**Given**: A `NotificationStatusReason` type covering all known status reasons
**When**: The notification controller sets a reason on status
**Then**: Every known reason has a compile-time constant, preventing string-literal typos

**Acceptance Criteria**:
- Constants exist for at least: `AllDeliveriesSucceeded`, `PartialDeliverySuccess`, `AllDeliveriesFailed`, `DeliveryTimeout`, `ValidationFailed`
- Each constant value matches the string used by the notification controller today

---

#### UT-NOT-453A-004: Routing attribute map with typed ReviewSource

**BR**: BR-NOT-065
**Type**: Unit
**File**: `test/unit/notification/crd_typed_fields_test.go`

**Given**: A NotificationRequest with `ReviewSource: ReviewSourceAIAnalysis` (typed enum)
**When**: `RoutingAttributesFromSpec` extracts routing attributes
**Then**: The attribute map contains `review-source: "AIAnalysis"` — identical to the previous `string` behavior

**Acceptance Criteria**:
- `attrs["review-source"]` == `"AIAnalysis"`
- `attrs["type"]`, `attrs["severity"]`, `attrs["priority"]` unchanged
- Map key set is identical to pre-refactor output for the same input

---

#### IT-NOT-453A-001: Manual review notification with typed ReviewSource via K8s API

**BR**: BR-ORCH-036
**Type**: Integration
**File**: `test/integration/notification/crd_typed_fields_integration_test.go`

**Given**: A RemediationRequest and an AIAnalysis that triggers WorkflowResolutionFailed
**When**: `CreateManualReviewNotification` creates a NotificationRequest with typed `ReviewSource`
**Then**: The created CRD persists in the K8s API with `spec.reviewSource: "AIAnalysis"` and the notification routes to the expected channel per BR-NOT-065

**Acceptance Criteria**:
- CRD `client.Create` succeeds without validation errors
- `client.Get` returns the CRD with `spec.reviewSource == "AIAnalysis"`
- Routing resolver produces the correct channel for the `manual-review` + `AIAnalysis` attribute combination

---

#### IT-NOT-453A-002: Typed enum fields round-trip through K8s API

**BR**: BR-NOT-058
**Type**: Integration
**File**: `test/integration/notification/crd_typed_fields_integration_test.go`

**Given**: A NotificationRequest with all typed enum fields populated (ReviewSource, typed delivery attempt status)
**When**: The CRD is created and then retrieved via the K8s API
**Then**: All typed field values are preserved exactly — no type coercion, no data loss

**Acceptance Criteria**:
- `spec.reviewSource` value preserved through Create → Get
- `status.deliveryAttempts[0].status` value preserved when set via status subresource
- `status.reason` value preserved when set via status subresource
- Invalid enum values are rejected at admission (not at read time)

---

### Phase B

#### UT-NOT-453B-001: Approval notification context structure

**BR**: BR-ORCH-001
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: A RemediationRequest with severity "critical" and an AIAnalysis with SelectedWorkflow (WorkflowID="restart-pod", Confidence=0.95, ApprovalReason="LowConfidence")
**When**: The approval notification context is constructed
**Then**: The typed struct contains:
- `Lineage.RemediationRequest` = RR name
- `Lineage.AIAnalysis` = AIAnalysis name
- `Workflow.SelectedWorkflow` = "restart-pod"
- `Analysis.Confidence` = "0.95"
- `Review.ApprovalReason` = "LowConfidence"
- Execution, Dedup, Target sub-structs are nil (not applicable for approval)

**Acceptance Criteria**:
- All populated fields match the values that the old `Metadata` map would have contained
- Nil sub-structs do not appear in JSON serialization (omitempty)

---

#### UT-NOT-453B-002: Completion notification context structure

**BR**: BR-ORCH-045
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: A completed RemediationRequest with outcome "Success" and an AIAnalysis with SelectedWorkflow and RootCauseAnalysis
**When**: The completion notification context is constructed
**Then**: The typed struct contains:
- `Lineage.RemediationRequest` = RR name
- `Lineage.AIAnalysis` = AIAnalysis name
- `Workflow.WorkflowID` = workflow ID
- `Workflow.ExecutionEngine` = engine name
- `Analysis.RootCause` = root cause summary
- `Analysis.Outcome` = "Success"

**Acceptance Criteria**:
- All populated fields match the values from the old `Metadata` map
- Review, Execution, Dedup sub-structs are nil

---

#### UT-NOT-453B-003: Bulk duplicate notification context structure

**BR**: BR-ORCH-034
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: A RemediationRequest with DuplicateCount=5
**When**: The bulk duplicate notification context is constructed
**Then**: The typed struct contains:
- `Lineage.RemediationRequest` = RR name
- `Dedup.DuplicateCount` = "5"
- All other sub-structs are nil

**Acceptance Criteria**:
- `FlattenToMap()` produces `{"remediationRequest": "rr-name", "duplicateCount": "5"}`
- This matches the exact old `Metadata` output for `CreateBulkDuplicateNotification`

---

#### UT-NOT-453B-004: Manual review notification context (both sources)

**BR**: BR-ORCH-036
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: (a) An AIAnalysis ManualReviewContext with SubReason="WorkflowNotFound" and HumanReviewReason="workflow_not_found", (b) A WorkflowExecution ManualReviewContext with RetryCount=3, MaxRetries=5, LastExitCode=1
**When**: The manual review notification context is constructed for each source
**Then**:
- (a) `Review.Reason`, `Review.SubReason`, `Review.HumanReviewReason` populated; `Execution` is nil
- (b) `Review.Reason` populated; `Execution.RetryCount`, `Execution.MaxRetries`, `Execution.LastExitCode` populated

**Acceptance Criteria**:
- `FlattenToMap()` output matches exact keys/values from old `buildManualReviewMetadata`
- The redundant `source` key is NOT present (it duplicates `spec.reviewSource`)

---

#### UT-NOT-453B-005: FlattenToMap routing compatibility

**BR**: BR-NOT-065
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: A `NotificationContext` populated for each of the 6 notification types (approval, completion, bulk, manual review AI, manual review WE, phase timeout)
**When**: `FlattenToMap()` is called for each
**Then**: The resulting `map[string]string` has identical key names and values as the old `Metadata` map for that notification type

**Acceptance Criteria**:
- Table-driven test with 6 entries (one per notification type)
- Each entry compares FlattenToMap output against the expected old Metadata map
- Key ordering is irrelevant; key presence and values must match exactly

---

#### UT-NOT-453B-006: FlattenToMap audit correlation preservation

**BR**: BR-NOT-064
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: A NotificationContext with Lineage populated (RemediationRequest, AIAnalysis)
**When**: `FlattenToMap()` is called
**Then**: The output contains `remediationRequest` and `aiAnalysis` keys with correct values, ensuring audit events can correlate notifications to their parent remediation

**Acceptance Criteria**:
- `result["remediationRequest"]` matches input `Lineage.RemediationRequest`
- `result["aiAnalysis"]` matches input `Lineage.AIAnalysis`
- These are the exact keys used by `audit.Manager.CreateMessageSentEvent` for `payload.Metadata.SetTo()`

---

#### UT-NOT-453B-007: Extensions map in routing attributes

**BR**: BR-NOT-065
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: A NotificationRequest with `Extensions: map[string]string{"environment": "production", "skip-reason": "RecentlyRemediated"}`
**When**: `RoutingAttributesFromSpec` extracts attributes
**Then**: The routing attribute map contains `environment: "production"` and `skip-reason: "RecentlyRemediated"` alongside spec-derived attributes

**Acceptance Criteria**:
- Extensions keys appear in routing attributes
- Spec-derived keys (type, severity, priority) take precedence over Extensions keys with the same name
- Identical behavior to old `Metadata` map iteration

---

#### UT-NOT-453B-008: Nil sub-struct safety

**BR**: BR-NOT-058
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: A `NotificationContext` where all sub-structs are nil (zero-value context)
**When**: `FlattenToMap()` is called
**Then**: Returns an empty `map[string]string{}` without panics

**Acceptance Criteria**:
- No nil pointer dereference
- Empty context produces empty map
- Partially populated context (e.g., only Lineage set) produces only Lineage keys

---

#### UT-NOT-453B-009: Redundant key elimination

**BR**: BR-NOT-058
**Type**: Unit
**File**: `test/unit/notification/notification_context_test.go`

**Given**: A NotificationContext with `Lineage.RemediationRequest = "test-rr"` and `Target.TargetResource = "Deployment/nginx"` (replicating old global timeout Metadata)
**When**: `FlattenToMap()` is called
**Then**: The output does NOT contain a `severity` key or `source` key (previously duplicated from `spec.severity` and `spec.reviewSource`)

**Acceptance Criteria**:
- `_, exists := result["severity"]` → `exists == false`
- `_, exists := result["source"]` → `exists == false`
- `spec.severity` and `spec.reviewSource` remain the authoritative sources for routing

---

#### IT-NOT-453B-001: Routing channel selection equivalence

**BR**: BR-NOT-065
**Type**: Integration
**File**: `test/integration/notification/routing_context_integration_test.go`

**Given**: A routing config with rules matching on `type: manual-review`, `review-source: AIAnalysis`, and `environment: production` (from Extensions)
**When**: A NotificationRequest is created with typed Context (Review sub-struct) + Extensions containing `environment: "production"`
**Then**: `ResolveChannelsForNotification` returns the same channel list as it would have with the old Metadata-based notification

**Acceptance Criteria**:
- Channel selection is identical for all 6 notification types
- Extensions keys are accessible in routing rules
- No routing config YAML changes required

---

#### IT-NOT-453B-002: Audit payload metadata preservation

**BR**: BR-NOT-064
**Type**: Integration
**File**: `test/integration/notification/audit_context_integration_test.go`

**Given**: A NotificationRequest with typed Context (Lineage + Workflow + Analysis populated) and Extensions
**When**: `audit.Manager.CreateMessageSentEvent` is called
**Then**: The audit event's `event_data.metadata` field contains the same key-value pairs as before the migration

**Acceptance Criteria**:
- `payload.Metadata` contains `remediationRequest`, `aiAnalysis`, `workflowId`, `executionEngine`, `rootCause`, `outcome` (same as old Metadata)
- Extensions keys are also included in the flattened metadata
- Audit event structure (correlation_id, resource, namespace) is unchanged

---

#### IT-NOT-453B-003: Approval notification with typed Context via K8s API

**BR**: BR-ORCH-001
**Type**: Integration
**File**: `test/integration/remediationorchestrator/notification_context_integration_test.go`

**Given**: A RemediationRequest and AIAnalysis with SelectedWorkflow in envtest
**When**: `CreateApprovalNotification` is called
**Then**: The created NotificationRequest CRD has `spec.context.lineage.remediationRequest`, `spec.context.workflow.selectedWorkflow`, and `spec.context.analysis.confidence` populated correctly

**Acceptance Criteria**:
- CRD passes K8s schema validation
- `spec.context` is populated (not nil)
- `spec.extensions` is nil or empty (approval doesn't use routing extensions)
- `spec.metadata` field does NOT exist (removed from schema)

---

#### IT-NOT-453B-004: Manual review notification (AI source) with typed Context

**BR**: BR-ORCH-036
**Type**: Integration
**File**: `test/integration/remediationorchestrator/notification_context_integration_test.go`

**Given**: A RemediationRequest and ManualReviewContext with Source=AIAnalysis, SubReason="WorkflowNotFound", HumanReviewReason="workflow_not_found"
**When**: `CreateManualReviewNotification` is called
**Then**: The CRD has `spec.context.lineage`, `spec.context.review` populated with correct values

**Acceptance Criteria**:
- `spec.context.review.reason` == ManualReviewContext.Reason
- `spec.context.review.subReason` == "WorkflowNotFound"
- `spec.context.review.humanReviewReason` == "workflow_not_found"
- `spec.context.execution` is nil (AI source has no retry info)

---

#### IT-NOT-453B-005: Manual review notification (WE source) with typed Context including retry data

**BR**: BR-ORCH-036
**Type**: Integration
**File**: `test/integration/remediationorchestrator/notification_context_integration_test.go`

**Given**: A RemediationRequest and ManualReviewContext with Source=WorkflowExecution, RetryCount=3, MaxRetries=5, LastExitCode=1, PreviousExecution="we-prev-001"
**When**: `CreateManualReviewNotification` is called
**Then**: The CRD has `spec.context.execution` populated with retry data

**Acceptance Criteria**:
- `spec.context.execution.retryCount` == "3"
- `spec.context.execution.maxRetries` == "5"
- `spec.context.execution.lastExitCode` == "1"
- `spec.context.execution.previousExecution` == "we-prev-001"

---

#### IT-NOT-453B-006: Completion notification with typed Context

**BR**: BR-ORCH-045
**Type**: Integration
**File**: `test/integration/remediationorchestrator/notification_context_integration_test.go`

**Given**: A completed RemediationRequest (Outcome=Success), AIAnalysis with SelectedWorkflow and RootCauseAnalysis
**When**: `CreateCompletionNotification` is called
**Then**: The CRD has `spec.context.lineage`, `spec.context.workflow`, `spec.context.analysis` populated

**Acceptance Criteria**:
- `spec.context.analysis.outcome` == "Success"
- `spec.context.analysis.rootCause` == RCA summary
- `spec.context.workflow.workflowId` == SelectedWorkflow.WorkflowID
- `spec.context.workflow.executionEngine` == SelectedWorkflow.ExecutionEngine

---

#### IT-NOT-453B-007: Bulk duplicate notification with typed Context via K8s API

**BR**: BR-ORCH-034
**Type**: Integration
**File**: `test/integration/remediationorchestrator/notification_context_integration_test.go`

**Given**: A RemediationRequest with `Status.DuplicateCount = 5` and `Status.OverallPhase = Completed`
**When**: `CreateBulkDuplicateNotification` is called
**Then**: The created NotificationRequest CRD has `spec.context.lineage.remediationRequest` and `spec.context.dedup.duplicateCount` populated correctly

**Acceptance Criteria**:
- CRD passes K8s schema validation
- `spec.context.lineage.remediationRequest` == RR name
- `spec.context.dedup.duplicateCount` == "5"
- `spec.context.workflow`, `spec.context.analysis`, `spec.context.review`, `spec.context.execution` are nil
- `spec.type` == "simple" and `spec.severity` == "low" (matching existing `CreateBulkDuplicateNotification` behavior)

---

#### IT-NOT-453B-008: Timeout notifications with typed Context including Target and Execution sub-structs

**BR**: BR-NOT-058
**Type**: Integration
**File**: `test/integration/remediationorchestrator/notification_context_integration_test.go`

**Given**: (a) A RemediationRequest in `Processing` phase that has exceeded global timeout, (b) A RemediationRequest in `Analyzing` phase that has exceeded phase-specific timeout
**When**: (a) `handleGlobalTimeout` creates a timeout notification, (b) `createPhaseTimeoutNotification` creates a phase timeout notification
**Then**: Both CRDs have `spec.context.lineage`, `spec.context.execution`, and `spec.context.target` populated correctly

**Acceptance Criteria**:
- Global timeout: `spec.context.execution.timeoutPhase` == `string(rr.Status.OverallPhase)` at time of timeout
- Global timeout: `spec.context.target.targetResource` == `"Kind/Name"` format
- Phase timeout: `spec.context.execution.timeoutPhase` == `string(phase)` and `spec.context.execution.phaseTimeout` == Go duration string
- Phase timeout: `spec.phase` == `string(phase)` (the spec-level field, for routing)
- Both: `spec.type` == "escalation", `spec.context.lineage.remediationRequest` populated
- Both: redundant `severity` key NOT present in Context (read from `spec.severity` instead)

---

#### IT-NOT-453B-009: Log delivery with typed Context and Extensions

**BR**: BR-NOT-058
**Type**: Integration
**File**: `test/integration/notification/log_delivery_context_integration_test.go`

**Given**: A NotificationRequest with `Context` containing Lineage and Workflow sub-structs, and `Extensions` containing `{"environment": "production", "test-key": "test-value"}`
**When**: `LogDeliveryService.Deliver` is called
**Then**: The structured log output contains all Context fields and Extension keys, enabling operators to search and filter notification logs by structured fields

**Acceptance Criteria**:
- Log entry contains `context` or `metadata` field with flattened key-value data
- `remediationRequest`, `workflowId` keys appear in the log output
- `environment`, `test-key` keys from Extensions appear in the log output
- Log format is valid JSON (when `format=json`) or structured text (when `format=text`)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None for Phase A (pure type validation). None for Phase B unit tests (pure struct construction and FlattenToMap logic).
- **Location**: `test/unit/notification/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**: envtest (K8s API server) for CRD validation and round-trip tests; `fake.NewClientBuilder()` for RO creator tests that need a K8s client
- **Location**: `test/integration/notification/`, `test/integration/remediationorchestrator/`

---

## 8. Execution

```bash
# Phase A: Unit tests
go test ./test/unit/notification/... -ginkgo.focus="UT-NOT-453A"

# Phase A: Integration tests
go test ./test/integration/notification/... -ginkgo.focus="IT-NOT-453A"

# Phase B: Unit tests
go test ./test/unit/notification/... -ginkgo.focus="UT-NOT-453B"

# Phase B: Integration tests
go test ./test/integration/notification/... -ginkgo.focus="IT-NOT-453B"
go test ./test/integration/remediationorchestrator/... -ginkgo.focus="IT-NOT-453B"

# Full validation
go build ./...
go test ./test/unit/notification/... ./test/integration/notification/... ./test/integration/remediationorchestrator/...
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2026-03-04 | Coverage audit: added IT-NOT-453B-007 (bulk duplicate), IT-NOT-453B-008 (timeout notifications), IT-NOT-453B-009 (log delivery) to reach >=80% integration-testable code coverage (~91%). Total: 25 test scenarios. |
| 1.0 | 2026-03-04 | Initial test plan: 22 test scenarios (10 UT + 6 IT for Phase A+B combined), 2-phase execution strategy |
