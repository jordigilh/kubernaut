# Test Plan: AuthWebhook system RAR audit enum contract

**Test Plan Identifier**: TP-2615-v1.0
**Feature**: Preserve AuthWebhook audit events for system `Expired` and `Cancelled` RAR decisions
**Version**: 1.0
**Created**: 2026-11-09
**Author**: Kubernaut Development Team
**Status**: Active
**Branch**: `fix/2615-authwebhook-audit-contract-final`

---

## 1. Introduction

### 1.1 Purpose

Issue #2615 closes the contract gap that causes AuthWebhook audit events for trusted
RemediationOrchestrator `Expired` and `Cancelled` decisions to fail OpenAPI validation.
The production handler already emits both decisions through the trusted RO path; this
plan proves that the structured payload accepts both values and that the real
AuthWebhook-to-DataStorage path persists them under the parent RemediationRequest
correlation ID.

### 1.2 Objectives

1. Prove `Expired` and `Cancelled` pass the real `BufferedAuditStore` OpenAPI validator.
2. Prove both events persist through the real AuthWebhook integration stack and are queryable by `correlation_id`.
3. Regenerate all Ogen and embedded OpenAPI artifacts from the single authoritative YAML enum.
4. Preserve existing human-decision, idempotency, authorization, and audit-validation behavior.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| RED unit regression | Fails before contract update | `ginkgo -focus='UT-RAR-AUDIT-AW-008' ./pkg/authwebhook/...` |
| Unit test pass rate | 100% | Focused AuthWebhook unit suite |
| Integration test pass rate | 100% when envtest/Podman are available | Focused AuthWebhook integration suite |
| Wiring coverage | 100% of manifest rows | Real webhook request through DataStorage query |
| Generated artifact drift | 0 differences | `make gen-diff` |
| Build/lint | No new failures | `go build ./...`, `golangci-lint run --timeout=5m` |

---

## 2. References

### 2.1 Authority

- [BR-AUDIT-005 v2.0](../../requirements/11_SECURITY_ACCESS_CONTROL.md): audit events must be complete and reconstructable.
- [BR-AUDIT-006](../../requirements/BR-AUDIT-006-remediation-approval-audit-trail.md): RAR decisions require queryable audit evidence.
- [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md): unified audit schema and correlation requirements.
- [DD-WEBHOOK-003](../../architecture/decisions/DD-WEBHOOK-003-webhook-complete-audit-pattern.md): AuthWebhook attribution event pattern.
- Issue [#2615](https://github.com/jordigilh/kubernaut/issues/2615): AuthWebhook audit events must support system terminal decisions.

### 2.2 Applicable controls

- SOC 2 CC7.2: reconstruct the RAR lifecycle from audit traces.
- FedRAMP AU-2/AU-3: emit complete, structured audit events with actor and decision data.
- FedRAMP AU-9/AU-11: persist audit information through DataStorage for protected retention.

---

## 3. Risks and Mitigations

| ID | Risk | Impact | Affected Tests | Mitigation |
|----|------|--------|----------------|------------|
| R1 | Generated Ogen enum remains narrower than the YAML contract | System terminal audit events are rejected and lost | UT-RAR-AUDIT-AW-008 | Generate Ogen artifacts and run `make gen-diff`. |
| R2 | The unit test validates only a mock store and misses OpenAPI rejection | Regression reaches production undetected | UT-RAR-AUDIT-AW-008 | Use the real `audit.BufferedAuditStore`, whose `StoreAudit` invokes the embedded OpenAPI validator. |
| R3 | The webhook event is accepted but not persisted/queryable | Compliance reconstruction remains incomplete | IT-RAR-AUDIT-AW-2615-001 | Exercise envtest webhook admission, real buffered audit store, DataStorage API, and a `correlation_id` query. |
| R4 | Generated embedded copies drift from the authoritative spec | Runtime validation and server validation disagree | GEN-RAR-AUDIT-2615-001 | Run generation and `make gen-diff`; inspect only generated diffs. |

### 3.1 Risk-to-Test Traceability

- R1/R2: `UT-RAR-AUDIT-AW-008`
- R3: `IT-RAR-AUDIT-AW-2615-001`
- R4: `GEN-RAR-AUDIT-2615-001`

---

## 4. Scope

### 4.1 Features to be tested

- `api/openapi/data-storage-v1.yaml`: `RemediationApprovalAuditPayload.decision` accepts `Approved`, `Rejected`, `Expired`, and `Cancelled`.
- `pkg/authwebhook/remediationapprovalrequest_handler.go`: existing trusted RO system-decision path emits the structured audit payload.
- `pkg/audit/store.go` and `pkg/audit/openapi_validator.go`: the event passes real OpenAPI validation before buffering.
- `test/integration/authwebhook/remediationapprovalrequest_test.go`: persisted event is queryable by parent RR correlation ID.

### 4.2 Features not to be tested

- Human approval authorization or workflow override behavior: covered by existing AuthWebhook tests and unrelated to the enum-only change.
- RemediationOrchestrator decision production: the trusted RO request is the AuthWebhook integration boundary for this issue.
- New audit event types, schema fields, dependencies, or architectural changes: explicitly out of scope.
- Full E2E/Kind qualification: no new container journey is required for this contract-only fix.

### 4.3 Design decisions

| Decision | Rationale |
|----------|-----------|
| Change only the authoritative OpenAPI enum in production | The handler already emits both system decisions; generated code and embedded specs must derive from the YAML source. |
| Use `audit.BufferedAuditStore` in the unit regression | This exercises the same OpenAPI validator that rejects the production event, while mocking only the external DataStorage client. |
| Use the existing AuthWebhook envtest suite for integration | It already wires the real webhook, authenticated audit store, DataStorage, and correlation query helper. |

---

## 5. Approach

### 5.1 Coverage policy

- **Unit**: business scenarios for both system terminal decisions, with the critical OpenAPI contract covered 100%.
- **Integration**: 100% of the issue's wiring manifest rows, including persistence and correlation query.
- **E2E**: skipped; the integration journey covers the changed contract and no production container behavior changes.

### 5.2 TDD phases

1. **RED**: add the unit regression and run it against the pre-change generated contract; it must fail because `Expired`/`Cancelled` are rejected by OpenAPI validation. Add the integration scenario before changing the enum.
2. **GREEN**: update only `api/openapi/data-storage-v1.yaml`, regenerate Ogen and embedded copies, then run focused unit and integration tests.
3. **REFACTOR**: no new production types or logic; inspect generated output, keep the test table concise, run formatting and required validation.
4. **WIRING VERIFICATION**: prove the real handler admission path reaches the buffered store and DataStorage query for both decisions.

### 5.3 Pass/fail criteria

**PASS** requires:

1. `UT-RAR-AUDIT-AW-008` passes for both `Expired` and `Cancelled`.
2. `IT-RAR-AUDIT-AW-2615-001` persists and retrieves both events by the unique parent RR `correlation_id`.
3. Generated artifacts are current (`make gen-diff` passes).
4. Build, affected tests, lint, and `git diff --check` pass, with unavailable infrastructure explicitly recorded.

**FAIL** is any P0 regression, generated-artifact drift, missing persisted event, or silent validation rejection.

---

## 6. Test items

| File | Purpose |
|------|---------|
| `pkg/authwebhook/remediationapprovalrequest_audit_test.go` | Unit regression through real `BufferedAuditStore` validation |
| `test/integration/authwebhook/remediationapprovalrequest_test.go` | Real webhook/DataStorage persistence and correlation query |
| `api/openapi/data-storage-v1.yaml` | Authoritative enum source |
| Generated Ogen and embedded OpenAPI files | Runtime/client/server contract copies regenerated from YAML |

---

## 7. BR coverage matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-006 | Every RAR terminal decision has a structured AuthWebhook audit event | P0 | Unit | UT-RAR-AUDIT-AW-008 | RED |
| BR-AUDIT-006 | System `Expired` and `Cancelled` events persist and are queryable by parent RR correlation ID | P0 | Integration | IT-RAR-AUDIT-AW-2615-001 | Pending |
| BR-AUDIT-005 | Audit traces support lifecycle reconstruction across the service boundary | P0 | Integration | IT-RAR-AUDIT-AW-2615-001 | Pending |

---

## 8. Test scenarios

### Tier 1: Unit

| ID | Business outcome | Phase |
|----|------------------|-------|
| `UT-RAR-AUDIT-AW-008` | Trusted RO `Expired` and `Cancelled` decisions are accepted by real OpenAPI validation and reach the audit client with the correct structured decision value. | RED |

### Tier 2: Integration

| ID | Business outcome | Phase |
|----|------------------|-------|
| `IT-RAR-AUDIT-AW-2615-001` | A real AuthWebhook admission request for each system terminal decision results in one persisted approval audit event retrievable by the unique parent RR `correlation_id`. | Pending |

### Tier 3: E2E

Skipped. The issue changes an OpenAPI enum and generated validation artifacts only; the existing envtest plus real DataStorage integration path is the required boundary proof.

---

## 9. Test cases

### UT-RAR-AUDIT-AW-008: system terminal decisions pass OpenAPI audit validation

**BR**: BR-AUDIT-006
**Priority**: P0
**Type**: Unit
**File**: `pkg/authwebhook/remediationapprovalrequest_audit_test.go`

**Test steps**:

1. **Given** a pending RAR and the trusted RO service-account identity.
2. **When** the AuthWebhook handles a new `Expired` or `Cancelled` decision using a real `audit.BufferedAuditStore`.
3. **Then** OpenAPI validation accepts the event, the admission response is allowed, and the external audit client receives one structured event with the expected decision.

**RED expectation**: before the enum update, the handler logs an OpenAPI validation error and the recording DataStorage client receives zero events.

### IT-RAR-AUDIT-AW-2615-001: system terminal events persist by correlation ID

**BR**: BR-AUDIT-005 and BR-AUDIT-006
**Priority**: P0
**Type**: Integration
**File**: `test/integration/authwebhook/remediationapprovalrequest_test.go`

**Test steps**:

1. **Given** the existing envtest AuthWebhook server, authenticated real buffered audit store, PostgreSQL, Redis, and DataStorage service.
2. **When** a pending RAR is updated through the Kubernetes API with trusted RO `Expired` and `Cancelled` decisions.
3. **Then** DataStorage returns exactly one matching `webhook.remediationapprovalrequest.decided` approval event for each unique parent RR correlation ID, with the matching structured decision and the trusted RO service-account actor ID.

**Unavailable-environment rule**: if envtest assets or Podman are unavailable, record the command and blocker; do not replace this test with a mock and do not mark it passed.

### GEN-RAR-AUDIT-2615-001: generated contract is current

1. Run `make generate-datastorage-client` and the embedded-spec generation included by `make generate`.
2. Run `make gen-diff`.
3. Confirm no generated file differs after regeneration.

---

## 10. Environmental needs

### 10.1 Unit tests

- Ginkgo/Gomega BDD.
- Mock only the external `DataStorageClient`; use the real buffered store and OpenAPI validator.
- No network, envtest, PostgreSQL, Redis, or Podman required.

### 10.2 Integration tests

- Ginkgo/Gomega BDD with zero business-logic mocks.
- Kubernetes envtest assets via `KUBEBUILDER_ASSETS`.
- Podman for PostgreSQL, Redis, and DataStorage test infrastructure.
- Existing AuthWebhook integration credentials and authenticated OpenAPI client.

---

## 11. Execution order

1. **RED**: add plan, unit regression, and integration scenario; run focused unit test and capture the expected enum-validation failure.
2. **GREEN**: update the YAML enum and regenerate all artifacts.
3. **REFACTOR**: format, inspect the generated diff, and validate no unrelated production files changed.
4. **WIRING VERIFICATION**: run focused integration tests when envtest/Podman are available.
5. **Release gates**: run affected tests, build, lint, generated-artifact check, and `git diff --check`.

---

## 12. Wiring manifest

| Component | Production entry point | Wiring code location | IT test ID |
|-----------|------------------------|----------------------|------------|
| AuthWebhook RAR system-decision audit emission | Kubernetes API status update to `/mutate-remediationapprovalrequest` | `cmd/authwebhook/main.go` handler registration; `pkg/authwebhook/remediationapprovalrequest_handler.go:handleSystemDecision` | IT-RAR-AUDIT-AW-2615-001 |
| Buffered audit OpenAPI validation and DataStorage persistence | `audit.AuditStore.StoreAudit` from the handler | `pkg/audit/store.go:StoreAudit`; `test/integration/authwebhook/suite_test.go` real store setup | IT-RAR-AUDIT-AW-2615-001 |
| Correlation query readback | DataStorage `QueryAuditEvents` with parent RR name | `test/integration/authwebhook/helpers.go:queryAuditEvents` | IT-RAR-AUDIT-AW-2615-001 |

**Checkpoint W**: all three rows must pass through the real production dispatch path; unit tests do not count as wiring proof.

---

## 13. Required validation commands

```bash
gofmt -w pkg/authwebhook/remediationapprovalrequest_audit_test.go
go test ./pkg/authwebhook/... -run 'TestAuthWebhook|Test.*AuthWebhook' -count=1
make test-integration-authwebhook GINKGO_FOCUS='IT-RAR-AUDIT-AW-2615-001'
go build ./...
golangci-lint run --timeout=5m
make gen-diff
git diff --check
```

Record unavailable envtest/Podman checks separately from source-attributed failures.

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-11-09 | Initial issue #2615 TDD plan |
