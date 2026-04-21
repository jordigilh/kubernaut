# Test Plan: DS CRD UPDATE Re-Sync, SOC2 Audit Compliance, and Content Integrity Enforcement

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-773-v1
**Feature**: CRD UPDATE webhook propagation to DS, distinct audit events, handleUpdate hardening, version-locked content integrity
**Version**: 1.0
**Created**: 2026-04-21
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/773-ds-crd-update-sync`

---

## 1. Introduction

### 1.1 Purpose

When a `RemediationWorkflow` CRD is updated via `kubectl apply`, the E2E `ValidatingWebhookConfiguration` does not include `UPDATE` in its operations list, so the webhook is never invoked and DS catalog becomes stale. Additionally, `handleUpdate` uses best-effort semantics (Allowed on every error) violating SOC2 CC8.1, audit events conflate UPDATE with CREATE, and DS incorrectly supersedes when it receives same (name, version) with different content instead of rejecting the request.

This test plan covers 4 workstreams:

1. **SOC2 Audit Compliance**: Distinct `remediationworkflow.admitted.update` audit event type
2. **Security Hardening**: `handleUpdate` strict like `handleCreate` (Denied + denied audit on all error paths)
3. **DS Content Integrity**: 409 Conflict when active workflow has same (name, version) but different content hash
4. **E2E Manifest + Tests**: Add UPDATE to E2E webhook manifest, E2E and unit test coverage

### 1.2 Objectives

1. UPDATE operations on RemediationWorkflow CRDs invoke the webhook in E2E (matching Helm chart)
2. Successful UPDATEs emit `remediationworkflow.admitted.update` audit events (not CREATE)
3. Failed UPDATEs emit `remediationworkflow.admitted.denied` audit events (not silently allowed)
4. `handleUpdate` denies on unmarshal, auth, marshal, and DS errors (parity with `handleCreate`)
5. DS returns 409 when active workflow with same (name, version) has a different content hash
6. Version bump UPDATEs propagate to DS catalog via cross-version supersession
7. Idempotent re-apply (same content) returns success without supersession

---

## 2. References

- Issue #773: DataStorage does not re-sync RemediationWorkflow CRD updates to its internal catalog
- BR-WORKFLOW-006: Kubernetes-Native Workflow Registration via RemediationWorkflow CRD
- DD-WORKFLOW-012: Workflow Immutability Constraints
- DD-WEBHOOK-001: CRD Webhook Requirements Matrix
- DD-WEBHOOK-003: Webhook Complete Audit Pattern
- ADR-058: Webhook-Driven Workflow Registration
- ADR-034: Unified Audit Table Design
- SOC2 CC8.1: Attribution of manual operational changes

---

## 3. Test Scenarios

### Tier 1: Unit Tests — AuthWebhook handleUpdate Hardening

| ID | Business Outcome Under Test |
|----|----------------------------|
| UT-AW-773-001 | UPDATE denies on unmarshal failure + emits `remediationworkflow.admitted.denied` |
| UT-AW-773-002 | UPDATE denies on auth failure + emits `remediationworkflow.admitted.denied` |
| UT-AW-773-003 | UPDATE denies on DS registration failure + emits `remediationworkflow.admitted.denied` |
| UT-AW-773-004 | UPDATE denies on marshal failure + emits `remediationworkflow.admitted.denied` |
| UT-AW-773-005 | UPDATE denies on DS 409 content integrity violation + denied audit contains "version" |

### Tier 1: Unit Tests — DS Content Integrity

| ID | Business Outcome Under Test |
|----|----------------------------|
| UT-DS-773-001 | Active + same (name,version) + different content hash → 409 RFC7807 `content-integrity-violation` |
| UT-DS-773-002 | `HandleCreateWorkflow` maps `contentIntegrityError` to 409 with correct RFC7807 fields |
| UT-DS-773-003 | Cross-version supersede (different version) still returns 201 (regression guard) |
| UT-DS-773-004 | Idempotent same-hash returns 200 (regression guard) |

### Tier 3: E2E Tests — CRD UPDATE Propagation

| ID | Business Outcome Under Test |
|----|----------------------------|
| E2E-AW-773-001 | Version bump UPDATE propagates to DS catalog, new workflowId, correct description |
| E2E-AW-773-002 | Same-version content change is rejected by webhook (admission Denied) |
| E2E-AW-773-003 | Idempotent re-apply (same content) succeeds, workflowId unchanged |

---

## 4. Existing Tests Requiring Updates

| Test ID | File | Change Required |
|---------|------|-----------------|
| UT-AW-299-009 | `remediationworkflow_handler_test.go:467` | Assert audit event is `EventTypeRWAdmittedUpdate` (not CREATE) |
| UT-AW-371-001 | `remediationworkflow_handler_test.go:500` | Same audit event assertion update |
| UT-AW-371-002 | `remediationworkflow_handler_test.go:529` | Same audit event assertion update |
| UT-DS-INTEGRITY-002 | `workflow_content_integrity_test.go:188` | Change expected status from 201 to 409 (active + diff hash now rejected) |

---

## 5. Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Ogen regeneration picks up unrelated changes | LOW | Clean working tree confirmed; additive-only enum changes |
| Existing tests break from handleUpdate hardening | ZERO | All 3 UPDATE tests are success-path only |
| E2E async timing flake | LOW | 30s/1s provides 2x headroom over 15s goroutine ceiling |
| DS 409 race in E2E | N/A | Single webhook replica, UUID names, serial execution |

---

## 6. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-21 | Initial test plan covering all 4 workstreams |
