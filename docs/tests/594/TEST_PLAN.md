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

### 1.2 Objectives

1. **CRD types**: `WorkflowOverride` struct on RAR status serializes/deserializes correctly.
2. **Authwebhook validation**: Override references a valid, Ready RW CRD; override only allowed with Approved decision.
3. **RO merge logic**: RAR overrides take precedence over AIA defaults; WE gets correct merged spec.
4. **Audit trail**: WE annotated with override source; K8s event emitted on RR.
5. **Error handling**: Invalid workflow name, non-Ready RW, override on Rejected — all rejected cleanly.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `ginkgo ./test/unit/remediationorchestrator/...` |
| Integration test pass rate | 100% | `ginkgo ./test/integration/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | Override types, webhook validation, merge logic |
| Integration-testable code coverage | >=80% | Full approve-with-override flow |

---

## 2. References

### 2.1 Authority

- Issue #594: Operator workflow/parameter override via RAR approval
- ADR-040: RAR immutable spec, CSR-like pattern
- ADR-001: Spec immutability
- DD-CONTRACT-002: AIA → WE output format
- BR-ORCH-025/026: Workflow approval orchestration

### 2.2 Cross-References

- Issue #592: Conversational RAR (conversation-mode override advisory depends on these types)
- Issue #632: OCP Console Plugin (override UI depends on these types)
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Operator references non-existent RW CRD | WE created with invalid workflow → execution failure | Medium | UT-OV-594-003, UT-OV-594-004 | Authwebhook validates RW exists and is Ready |
| R2 | Override on Rejected decision | Inconsistent state — rejected but with override | Low | UT-OV-594-005 | Webhook rejects override when decision != Approved |
| R3 | Parameters full-replacement semantics confusion | Operator passes empty map intending "no change" but it means "no params" | Medium | UT-OV-594-010 | Document clearly; nil vs empty-map distinction |
| R4 | RW CRD deleted between webhook validation and RO merge | Race condition: webhook validates, RW removed, RO fails | Low | UT-OV-594-008 | RO GET with retry; fail gracefully with event |
| R5 | Schema regression on RAR CRD | Controller-gen changes break existing RAR clients | Medium | UT-OV-594-001, UT-OV-594-002 | CRD validation via make manifests; backward-compat test |

### 3.1 Risk-to-Test Traceability

- **R1** (non-existent RW): UT-OV-594-003, UT-OV-594-004
- **R2** (override on reject): UT-OV-594-005
- **R3** (params semantics): UT-OV-594-010
- **R4** (race condition): UT-OV-594-008

---

## 4. Scope

### 4.1 Features to be Tested

**CRD Types**:
- `WorkflowOverride` struct (`workflowName`, `parameters`, `rationale`)
- Addition to `RemediationApprovalRequestStatus`
- Serialization/deserialization (JSON round-trip)

**Authwebhook Validation** (`pkg/authwebhook/remediationapprovalrequest_handler.go`):
- Override present + Approved → validate RW CRD exists and is `Ready`
- Override present + Rejected/Expired → reject
- Override with empty workflowName (only params) → allow
- RW CRD not found → reject with descriptive error
- RW CRD not Ready → reject with descriptive error

**RO Merge Logic** (`internal/controller/remediationorchestrator/reconciler.go`):
- Override with workflowName → resolve RW CRD → build merged `SelectedWorkflow`
- Override with only parameters → use AIA workflow but override params
- Override with both → override everything
- No override → existing behavior unchanged
- WE annotation `kubernaut.ai/override-source: rar/{rar-name}`
- K8s event emitted on RR

**WE Creator Integration** (`pkg/remediationorchestrator/creator/workflowexecution.go`):
- Accepts pre-merged workflow spec from RO (no change to creator itself)

### 4.2 Features Not to be Tested

- **Conversation-mode override advisory** (#592): Separate issue, advisory only
- **OCP Console Plugin override UI** (#632): Separate issue, consumes these types
- **Parameter schema validation**: Freeform — full operator freedom (per issue design)
- **RBAC distinction approve vs approve-with-override**: Future enhancement

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% — type serialization, webhook validation, merge logic, annotation, events
- **Integration**: >=80% — full approve-with-override flow through RO reconciler
- **E2E**: Deferred — requires stable CI/CD. Will validate with Kind when available.

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass; per-tier >=80%; webhook rejects invalid overrides; merge produces correct WE spec; annotation present

**FAIL**: Any P0 fails; webhook allows invalid RW; WE spec incorrect after merge

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-030 | WorkflowOverride JSON round-trip | P0 | Unit | UT-OV-594-001 | Pending |
| BR-ORCH-030 | WorkflowOverride nil (absent) is preserved | P0 | Unit | UT-OV-594-002 | Pending |
| BR-ORCH-031 | Webhook: override + Approved + valid RW → allow | P0 | Unit | UT-OV-594-003 | Pending |
| BR-ORCH-031 | Webhook: override + Approved + RW not found → reject | P0 | Unit | UT-OV-594-004 | Pending |
| BR-ORCH-031 | Webhook: override + Rejected → reject | P0 | Unit | UT-OV-594-005 | Pending |
| BR-ORCH-031 | Webhook: override + RW not Ready → reject | P0 | Unit | UT-OV-594-006 | Pending |
| BR-ORCH-031 | Webhook: override with only parameters (no workflowName) → allow | P1 | Unit | UT-OV-594-007 | Pending |
| BR-ORCH-032 | RO: override workflowName → resolve RW → merged WE spec | P0 | Unit | UT-OV-594-008 | Pending |
| BR-ORCH-032 | RO: override params only → AIA workflow + override params in WE | P0 | Unit | UT-OV-594-009 | Pending |
| BR-ORCH-032 | RO: override params nil → AIA params used; empty map → no params | P0 | Unit | UT-OV-594-010 | Pending |
| BR-ORCH-032 | RO: no override → existing behavior unchanged | P0 | Unit | UT-OV-594-011 | Pending |
| BR-ORCH-033 | WE annotation `kubernaut.ai/override-source` present when override applied | P0 | Unit | UT-OV-594-012 | Pending |
| BR-ORCH-033 | K8s event emitted on RR when override applied | P1 | Unit | UT-OV-594-013 | Pending |
| BR-ORCH-032 | RO: RW CRD deleted after webhook → graceful failure with event | P1 | Unit | UT-OV-594-014 | Pending |
| BR-ORCH-030 | Full override flow: approve with override → WE created with merged spec | P0 | Integration | IT-OV-594-001 | Pending |
| BR-ORCH-031 | Full flow: approve without override → existing behavior | P0 | Integration | IT-OV-594-002 | Pending |
| BR-ORCH-032 | Full flow: params-only override → AIA workflow + new params | P0 | Integration | IT-OV-594-003 | Pending |
| BR-ORCH-033 | Full flow: override annotation present on WE after creation | P0 | Integration | IT-OV-594-004 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests (14 tests)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-OV-594-001` | `WorkflowOverride{WorkflowName: "drain-restart", Parameters: {"timeout": "30s"}, Rationale: "prefer safe"}` serializes and deserializes correctly | Pending |
| `UT-OV-594-002` | RAR status with nil `WorkflowOverride` → JSON omits field entirely | Pending |
| `UT-OV-594-003` | Webhook: RAR with Approved + override referencing existing Ready RW → admission allowed + DecidedBy set | Pending |
| `UT-OV-594-004` | Webhook: RAR with Approved + override referencing non-existent RW → admission denied with "not found" | Pending |
| `UT-OV-594-005` | Webhook: RAR with Rejected + override → admission denied with "override only valid with Approved" | Pending |
| `UT-OV-594-006` | Webhook: RAR with Approved + override referencing RW in `Pending` catalog status → admission denied | Pending |
| `UT-OV-594-007` | Webhook: RAR with Approved + override with empty workflowName but with parameters → allowed | Pending |
| `UT-OV-594-008` | RO merge: override.workflowName set → GET RW CRD → WE spec uses RW bundle/version/engine | Pending |
| `UT-OV-594-009` | RO merge: override.parameters set, no workflowName → WE uses AIA workflow + override params | Pending |
| `UT-OV-594-010` | RO merge: override.parameters is `{}` (empty) → WE params empty; override.parameters nil → WE uses AIA params | Pending |
| `UT-OV-594-011` | RO merge: no override (nil `WorkflowOverride`) → WE spec matches AIAnalysis exactly | Pending |
| `UT-OV-594-012` | WE created with override → annotation `kubernaut.ai/override-source: rar/{name}` present | Pending |
| `UT-OV-594-013` | Override applied → K8s event on RR with reason "OperatorOverride" and descriptive message | Pending |
| `UT-OV-594-014` | RO merge: override.workflowName references deleted RW → fail with event, RR not stuck | Pending |

### Tier 2: Integration Tests (4 tests)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-OV-594-001` | RR → AIA → RAR (Approved + override) → WE created with merged spec from override RW | Pending |
| `IT-OV-594-002` | RR → AIA → RAR (Approved, no override) → WE created with AIA spec (existing behavior) | Pending |
| `IT-OV-594-003` | RR → AIA → RAR (Approved + params-only override) → WE with AIA workflow + override params | Pending |
| `IT-OV-594-004` | Override WE has `kubernaut.ai/override-source` annotation; RR has OperatorOverride event | Pending |

### Tier Skip Rationale

- **E2E**: Requires stable CI/CD with Kind, authwebhook + RO + RW catalog. Deferred.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Fake K8s client (for RW GET, SAR, RAR/WE CRUD), mock audit store
- **Location**: `test/unit/remediationorchestrator/controller/` (merge logic), `test/unit/authwebhook/` (webhook validation)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest with CRDs, authwebhook, RO reconciler, fake RW catalog
- **Location**: `test/integration/remediationorchestrator/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact | Workaround |
|------------|------|--------|--------|------------|
| RAR CRD types (`api/remediation/v1alpha1/`) | Code | Exists | Extend with WorkflowOverride | N/A |
| Authwebhook handler | Code | Exists | Extend validation for override | N/A |
| RO reconciler (`handleAwaitingApprovalPhase`) | Code | Exists | Add merge logic | N/A |
| RemediationWorkflow CRD (catalog) | Code | Exists | Used for override resolution | N/A |
| make manifests (controller-gen) | Tool | Available | Regenerate CRDs | N/A |

### 11.2 Execution Order

1. **Phase 1**: CRD types + serialization tests
2. **Phase 2**: Authwebhook validation tests + implementation
3. **Phase 3**: RO merge logic tests + implementation
4. **Phase 4**: Annotation + event tests + integration tests

---

## 12. Execution

```bash
ginkgo -v --focus="UT-OV-594" ./test/unit/remediationorchestrator/controller/...
ginkgo -v --focus="UT-OV-594" ./test/unit/authwebhook/...
ginkgo -v --focus="IT-OV-594" ./test/integration/remediationorchestrator/...
```

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
