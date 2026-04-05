# Test Plan: Operator Workflow/Parameter Override via RAR Approval

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-594-v1.0
**Feature**: Allow operators to override the AI-recommended workflow and/or parameters when approving a RemediationApprovalRequest (RAR)
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the operator workflow/parameter override capability introduced by Issue #594. Today RAR approval is binary (Approved/Rejected) — the WorkflowExecution (WE) always uses exactly what AIAnalysis recommended. This feature adds a `WorkflowOverride` struct to RAR status, allowing operators to redirect execution to a different `RemediationWorkflow` (RW) CRD and/or override parameters.

The WE creator remains agnostic to the override source. The RO is the single decision point: it resolves the final workflow spec from either the RAR override (if present) or the AIA (default), then passes the result downstream.

### 1.2 Objectives

1. **CRD types**: `WorkflowOverride` struct on RAR status serializes/deserializes correctly, nil is omitted.
2. **Authwebhook validation**: Override references a valid, Active RW CRD; override only allowed with Approved decision.
3. **RO merge logic**: RAR override takes precedence over AIA defaults; WE receives the final resolved spec without knowing the source.
4. **Audit trail**: K8s `OperatorOverride` event emitted on RR when override applied. `rr.Status.SelectedWorkflowRef` reflects what was actually used.
5. **Error handling**: Invalid workflow name, non-Active RW, override on Rejected — all rejected cleanly.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `ginkgo ./test/unit/authwebhook/... ./test/unit/remediationorchestrator/controller/...` |
| Integration test pass rate | 100% | `ginkgo ./test/integration/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | Override types, webhook validation, merge logic |
| Integration-testable code coverage | >=80% | Full approve-with-override RO reconciler flow |
| Backward compatibility | 0 regressions | Existing RAR tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #594: Operator workflow/parameter override via RAR approval
- ADR-040: RAR immutable spec, CSR-like pattern
- ADR-001: Spec immutability
- DD-CONTRACT-002: AIA → WE output format
- DD-EVENT-001: Controller Kubernetes Event Registry
- BR-ORCH-025/026: Workflow approval orchestration

### 2.2 Cross-References

- Issue #592: Conversational RAR (conversation-mode override advisory depends on these types)
- Issue #632: OCP Console Plugin (override UI depends on these types)
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Operator references non-existent RW CRD | WE created with invalid workflow → execution failure | Medium | UT-AW-594-003, UT-AW-594-004 | Authwebhook validates RW exists and is Active |
| R2 | Override on non-Approved decision | Inconsistent state — rejected but with override data | Low | UT-AW-594-005 | Webhook rejects override when decision != Approved |
| R3 | Parameters full-replacement semantics confusion | Operator passes empty map intending "no change" but it means "no params" | Medium | UT-RO-594-006 | Documented nil vs empty-map distinction |
| R4 | RW CRD deleted between webhook validation and RO merge | Race condition: webhook validates, RW removed, RO fails | Low | UT-RO-594-010 | RO GET with retry; fail gracefully with event |
| R5 | CatalogStatus mismatch — issue says "Ready" but enum has "Active" | Webhook validation never matches | High | UT-AW-594-003 | Due diligence F1: use `Active` (from `sharedtypes.CatalogStatusActive`) |
| R6 | Authwebhook constructor change breaks wiring | Existing webhook tests and cmd/ wiring fail to compile | Medium | All AW tests | Add `client.Reader` to handler; update constructor call sites |

### 3.1 Risk-to-Test Traceability

- **R1** (non-existent RW): UT-AW-594-003, UT-AW-594-004
- **R2** (override on reject): UT-AW-594-005
- **R3** (params semantics): UT-RO-594-006
- **R4** (race condition): UT-RO-594-010
- **R5** (CatalogStatus): UT-AW-594-003 (validates against `Active`)
- **R6** (constructor): compilation gate — all tests must compile after constructor change

---

## 4. Scope

### 4.1 Features to be Tested

- **WorkflowOverride CRD types** (`api/remediation/v1alpha1/remediationapprovalrequest_types.go`): New `WorkflowOverride` struct added to RAR status. Validates JSON serialization, nil omission, and field semantics.

- **Authwebhook override validation** (`pkg/authwebhook/remediationapprovalrequest_handler.go`): When `workflowOverride` present on Approved decision, webhook validates the referenced RW CRD exists and has `catalogStatus == Active`. Rejects override on non-Approved decisions.

- **RO merge logic** (`internal/controller/remediationorchestrator/reconciler.go` — `handleAwaitingApprovalPhase`): RO checks `rar.Status.WorkflowOverride`. If present, resolves RW CRD and builds the final `SelectedWorkflow`. If absent, uses AIA as today. Passes resolved spec to WE creator (unchanged) and `rr.Status.SelectedWorkflowRef`.

- **K8s event** (`pkg/shared/events/reasons.go`): New `EventReasonOperatorOverride` constant. RO emits event on RR when override applied.

### 4.2 Features Not to be Tested

- **WE creator**: Unchanged — agnostic to override source. Existing WE creator tests provide coverage.
- **Conversation-mode override advisory** (#592): Separate issue, advisory only.
- **OCP Console Plugin override UI** (#632): Separate issue, consumes these types.
- **Parameter schema validation**: Freeform — full operator freedom (per issue design).
- **RBAC distinction approve vs approve-with-override**: Future enhancement.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Override on RAR **status** (not spec) | ADR-040: spec is immutable. Status is operator-writable. |
| Workflow reference by RW CRD `.metadata.name` | More K8s-native than DS UUID. RO resolves name → full spec. |
| Parameters: full replacement semantics | Present (even empty map) replaces AIA params. Nil → AIA params. Simplest mental model. |
| Validate against `CatalogStatusActive` (not "Ready") | Due diligence F1: "Ready" doesn't exist in the enum. `Active` is the correct operational state. |
| WE creator unchanged | Due diligence F3/F4: RO is the single decision point. WE is agnostic to override source. |
| No annotation on WE | WE should not know where its spec came from. Traceability via K8s event + RAR status. |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (type serialization, webhook validation logic, merge logic, event constant)
- **Integration**: >=80% of **integration-testable** code (full RO reconciler approve-with-override flow via envtest)
- **E2E**: Deferred — requires stable CI/CD with Kind + authwebhook + RO + RW catalog

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- Unit tests catch logic and correctness (fast feedback, isolated)
- Integration tests catch wiring and cross-component behavior

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes** — "when the operator overrides, the WE receives the operator's workflow spec" — not implementation details like "function X is called with argument Y".

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:
1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing RAR webhook or RO reconciler tests
5. Approve-without-override flow produces identical results to pre-change behavior

**FAIL** — any of the following:
1. Any P0 test fails
2. Per-tier coverage falls below 80%
3. Existing tests that were passing before now fail (regression)
4. Webhook allows override referencing non-existent or non-Active RW

### 5.5 Suspension & Resumption Criteria

**Suspend**: v1.3 KA CI/CD instability blocks build; `make manifests` broken after CRD change.
**Resume**: Build green; CRD regeneration succeeds.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `api/remediation/v1alpha1/remediationapprovalrequest_types.go` | `WorkflowOverride` struct, field on `RemediationApprovalRequestStatus` | ~25 new |
| `pkg/authwebhook/remediationapprovalrequest_handler.go` | Override validation block in `Handle` | ~40 new |
| `pkg/authwebhook/decision_validator.go` | No change (existing) | — |
| `pkg/shared/events/reasons.go` | `EventReasonOperatorOverride` constant | ~5 new |
| `pkg/remediationorchestrator/override/merge.go` (new) | `ResolveWorkflow(ctx, reader, rar, ai)` | ~60 new |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | Override branch in `handleAwaitingApprovalPhase` | ~30 new |
| `pkg/authwebhook/remediationapprovalrequest_handler.go` | RW CRD GET via `client.Reader` | ~15 new |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.4` HEAD | Post-rebase on `development/v1.3` |
| CRD regeneration | `make manifests` | Must succeed after type change |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-030 | WorkflowOverride JSON round-trip preserves all fields | P0 | Unit | UT-OV-594-001 | Pending |
| BR-ORCH-030 | WorkflowOverride nil (absent) is omitted from JSON | P0 | Unit | UT-OV-594-002 | Pending |
| BR-ORCH-031 | Webhook: override + Approved + valid Active RW → allow | P0 | Unit | UT-AW-594-003 | Pending |
| BR-ORCH-031 | Webhook: override + Approved + RW not found → reject | P0 | Unit | UT-AW-594-004 | Pending |
| BR-ORCH-031 | Webhook: override + Rejected → reject | P0 | Unit | UT-AW-594-005 | Pending |
| BR-ORCH-031 | Webhook: override + RW not Active (Pending) → reject | P0 | Unit | UT-AW-594-006 | Pending |
| BR-ORCH-031 | Webhook: override with only params (no workflowName) → allow | P1 | Unit | UT-AW-594-007 | Pending |
| BR-ORCH-032 | RO: override workflowName → resolve RW → WE spec uses RW data | P0 | Unit | UT-RO-594-001 | Pending |
| BR-ORCH-032 | RO: override params only → WE uses AIA workflow + override params | P0 | Unit | UT-RO-594-002 | Pending |
| BR-ORCH-032 | RO: no override (nil) → WE spec matches AIA exactly (regression) | P0 | Unit | UT-RO-594-003 | Pending |
| BR-ORCH-032 | RO: override with both workflowName + params → both overridden | P0 | Unit | UT-RO-594-004 | Pending |
| BR-ORCH-032 | RO: override rationale preserved in resolved spec context | P1 | Unit | UT-RO-594-005 | Pending |
| BR-ORCH-032 | RO: override params `{}` → WE params empty; params nil → AIA params | P0 | Unit | UT-RO-594-006 | Pending |
| BR-ORCH-033 | RO: K8s event on RR with reason OperatorOverride when override applied | P0 | Unit | UT-RO-594-007 | Pending |
| BR-ORCH-033 | RO: no event when no override (existing behavior unchanged) | P0 | Unit | UT-RO-594-008 | Pending |
| BR-ORCH-032 | RO: rr.Status.SelectedWorkflowRef reflects overridden workflow | P0 | Unit | UT-RO-594-009 | Pending |
| BR-ORCH-032 | RO: override RW deleted after webhook → graceful failure with event | P1 | Unit | UT-RO-594-010 | Pending |
| BR-ORCH-030 | Full flow: approve + override → WE with overridden spec | P0 | Integration | IT-RO-594-001 | Pending |
| BR-ORCH-030 | Full flow: approve without override → WE matches AIA (regression) | P0 | Integration | IT-RO-594-002 | Pending |
| BR-ORCH-032 | Full flow: params-only override → AIA workflow + new params in WE | P0 | Integration | IT-RO-594-003 | Pending |
| BR-ORCH-033 | Full flow: override applied → OperatorOverride event on RR | P0 | Integration | IT-RO-594-004 | Pending |
| BR-ORCH-031 | Full flow: webhook rejects override referencing non-existent RW | P0 | Integration | IT-RO-594-005 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

- `UT-OV-594-NNN` — Unit: Override type serialization
- `UT-AW-594-NNN` — Unit: Authwebhook override validation
- `UT-RO-594-NNN` — Unit: RO merge logic
- `IT-RO-594-NNN` — Integration: Full RO reconciler override flow

### Tier 1: Unit Tests (17 tests)

**Testable code scope**: `WorkflowOverride` types, webhook validation logic, `ResolveWorkflow` merge function, event constant. >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-OV-594-001` | Operator sets `WorkflowOverride{WorkflowName: "drain-restart", Parameters: {"TIMEOUT": "30s"}, Rationale: "prefer safe"}` → JSON round-trip preserves all fields | Pending |
| `UT-OV-594-002` | RAR status with nil `WorkflowOverride` → JSON output omits field entirely (backward compat) | Pending |
| `UT-AW-594-003` | Operator approves with override referencing Active RW → webhook allows and preserves override | Pending |
| `UT-AW-594-004` | Operator approves with override referencing non-existent RW → webhook denies with "RemediationWorkflow not found" | Pending |
| `UT-AW-594-005` | Operator rejects with override → webhook denies with "override only valid with Approved decision" | Pending |
| `UT-AW-594-006` | Operator approves with override referencing Pending-status RW → webhook denies with "not in Active status" | Pending |
| `UT-AW-594-007` | Operator approves with override containing only params (no workflowName) → webhook allows | Pending |
| `UT-RO-594-001` | Override with workflowName "drain-restart" → `ResolveWorkflow` returns spec with RW's bundle, version, engine, serviceAccount | Pending |
| `UT-RO-594-002` | Override with only params → `ResolveWorkflow` returns AIA workflow with overridden params | Pending |
| `UT-RO-594-003` | No override (nil WorkflowOverride) → `ResolveWorkflow` returns AIA SelectedWorkflow unmodified | Pending |
| `UT-RO-594-004` | Override with both workflowName + params → resolved spec has RW's workflow data + override params | Pending |
| `UT-RO-594-005` | Override with rationale → rationale accessible on resolved context for audit/event | Pending |
| `UT-RO-594-006` | Override params is `{}` (empty map) → resolved params empty. Override params nil → AIA params used | Pending |
| `UT-RO-594-007` | Override present → `OperatorOverride` event emitted on RR with override details in message | Pending |
| `UT-RO-594-008` | No override → no `OperatorOverride` event emitted (existing behavior) | Pending |
| `UT-RO-594-009` | Override applied → `rr.Status.SelectedWorkflowRef` reflects the overridden workflow (not AIA) | Pending |
| `UT-RO-594-010` | Override references RW that was deleted after webhook validation → RO fails gracefully, emits warning event | Pending |

### Tier 2: Integration Tests (5 tests)

**Testable code scope**: Full `handleAwaitingApprovalPhase` with envtest, CRD CRUD, reconciler. >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-594-001` | RR → AIA → RAR (Approved + workflow override) → RO resolves RW → WE created with overridden spec | Pending |
| `IT-RO-594-002` | RR → AIA → RAR (Approved, no override) → WE created with AIA spec (regression guard) | Pending |
| `IT-RO-594-003` | RR → AIA → RAR (Approved + params-only override) → WE with AIA workflow + override params | Pending |
| `IT-RO-594-004` | Override applied → K8s events on RR include `OperatorOverride` reason | Pending |
| `IT-RO-594-005` | RAR with override referencing non-existent RW → webhook admission denied (envtest webhook) | Pending |

### Tier Skip Rationale

- **E2E**: Requires stable Kind cluster with authwebhook + RO + RW catalog CRDs + DS. Deferred until v1.4 CI/CD available.

---

## 9. Test Cases

### UT-OV-594-001: WorkflowOverride JSON round-trip

**BR**: BR-ORCH-030
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/override_types_test.go`

**Preconditions**: None (pure type test)

**Test Steps**:
1. **Given**: A `RemediationApprovalRequestStatus` with `WorkflowOverride{WorkflowName: "drain-restart", Parameters: map[string]string{"TIMEOUT": "30s", "FORCE": "true"}, Rationale: "prefer safe restart"}`
2. **When**: Marshal to JSON and unmarshal back
3. **Then**: All fields are preserved exactly

**Acceptance Criteria**:
- **Behavior**: `WorkflowOverride` survives JSON round-trip
- **Correctness**: `WorkflowName`, `Parameters` (all entries), and `Rationale` are identical
- **Accuracy**: No field loss, no default injection

### UT-AW-594-003: Webhook allows valid override

**BR**: BR-ORCH-031
**Priority**: P0
**Type**: Unit
**File**: `test/unit/authwebhook/rar_override_validation_test.go`

**Preconditions**: Fake `client.Reader` returns a `RemediationWorkflow` with `status.catalogStatus = Active`

**Test Steps**:
1. **Given**: RAR with `status.decision = Approved`, `status.workflowOverride.workflowName = "drain-restart"`
2. **When**: Webhook `Handle` is called with the admission request
3. **Then**: Response is `Allowed`; override data preserved on patched RAR

**Acceptance Criteria**:
- **Behavior**: Webhook allows the admission request
- **Correctness**: `DecidedBy` populated from authenticated user; `WorkflowOverride` preserved
- **Accuracy**: RW GET executed with correct name and namespace

### UT-RO-594-001: RO merge with workflowName override

**BR**: BR-ORCH-032
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/override_merge_test.go`

**Preconditions**: Fake `client.Reader` returns RW CRD with known bundle/version/engine

**Test Steps**:
1. **Given**: AIA with `SelectedWorkflow{WorkflowID: "wf-001", Version: "1.0", ExecutionBundle: "old-bundle"}`. RAR with `WorkflowOverride{WorkflowName: "drain-restart"}`. RW CRD "drain-restart" with `status.workflowId = "wf-002"`, `spec.version = "2.0"`, `spec.execution.bundle = "new-bundle"`
2. **When**: `ResolveWorkflow(ctx, reader, rar, ai)` is called
3. **Then**: Returned `SelectedWorkflow` has `WorkflowID = "wf-002"`, `Version = "2.0"`, `ExecutionBundle = "new-bundle"`, and parameters from AIA (since override.Parameters is nil)

**Acceptance Criteria**:
- **Behavior**: Override workflow takes precedence over AIA workflow
- **Correctness**: All RW fields (workflowId, version, bundle, bundleDigest, engine, engineConfig, serviceAccountName) mapped correctly
- **Accuracy**: AIA parameters preserved when override.Parameters is nil

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake `client.Reader` (for RW GET in webhook + merge logic). Mock `audit.AuditStore` (existing pattern for webhook tests). Fake `record.EventRecorder` (for K8s events).
- **Location**: `test/unit/authwebhook/rar_override_validation_test.go`, `test/unit/remediationorchestrator/controller/override_merge_test.go`, `test/unit/remediationorchestrator/controller/override_types_test.go`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (per No-Mocks Policy)
- **Infrastructure**: envtest with CRDs (RR, RAR, AIA, WE, RW) registered. RO reconciler running. RW catalog seeded with test workflows.
- **Location**: `test/integration/remediationorchestrator/override_flow_test.go`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-gen | latest | CRD regeneration |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| RAR CRD types | Code | Exists | Extend with WorkflowOverride | N/A |
| Authwebhook handler | Code | Exists | Add `client.Reader` + validation | N/A |
| RO reconciler (`handleAwaitingApprovalPhase`) | Code | Exists | Add merge logic | N/A |
| RW CRD (catalog) | Code | Exists | Used for override resolution | N/A |
| `make manifests` | Tool | Available | Regenerate CRDs after type change | N/A |
| `sharedtypes.CatalogStatusActive` | Code | Exists | Webhook validates against this | N/A |

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Failing tests for CRD types + webhook validation + merge logic + events
2. **Phase 2 (TDD GREEN)**: Minimal implementation to pass all unit tests
3. **Phase 3 (TDD REFACTOR)**: Extract reusable override helper, structured logging, metrics
4. **Checkpoint 1**: Due diligence — verify all unit tests pass, build succeeds, no regressions
5. **Phase 4 (TDD RED)**: Failing integration tests for full RO flow
6. **Phase 5 (TDD GREEN)**: Wire override into reconciler, integration tests pass
7. **Phase 6 (TDD REFACTOR)**: Code quality, error messages, documentation
8. **Checkpoint 2**: Due diligence — verify all tests pass, coverage >=80% per tier, no regressions

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/594/TEST_PLAN.md` | Strategy and test design |
| Override types unit tests | `test/unit/remediationorchestrator/controller/override_types_test.go` | Type serialization |
| Webhook validation unit tests | `test/unit/authwebhook/rar_override_validation_test.go` | Webhook override logic |
| Merge logic unit tests | `test/unit/remediationorchestrator/controller/override_merge_test.go` | RO merge / resolve |
| Integration test suite | `test/integration/remediationorchestrator/override_flow_test.go` | Full RO flow |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests — types
ginkgo -v --focus="UT-OV-594" ./test/unit/remediationorchestrator/controller/...

# Unit tests — webhook
ginkgo -v --focus="UT-AW-594" ./test/unit/authwebhook/...

# Unit tests — merge logic
ginkgo -v --focus="UT-RO-594" ./test/unit/remediationorchestrator/controller/...

# Integration tests
ginkgo -v --focus="IT-RO-594" ./test/integration/remediationorchestrator/...

# All #594 tests
ginkgo -v --focus="594" ./test/unit/... ./test/integration/...

# Coverage
go test ./test/unit/remediationorchestrator/controller/... -coverprofile=ut_coverage.out
go test ./test/integration/remediationorchestrator/... -coverprofile=it_coverage.out
go tool cover -func=ut_coverage.out
go tool cover -func=it_coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/authwebhook/remediationapprovalrequest_audit_test.go` | Constructs handler via `NewRemediationApprovalRequestAuthHandler(auditStore)` | Update to `NewRemediationApprovalRequestAuthHandler(auditStore, reader)` | F2: constructor gains `client.Reader` parameter |
| `test/integration/authwebhook/remediationapprovalrequest_test.go` | Same constructor | Same update | Same |
| `cmd/` wiring for authwebhook | Same constructor | Same update (pass the manager's client) | Same |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan. Due diligence findings F1-F7 incorporated. |
