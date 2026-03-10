# ADR-059: ActionType CRD Lifecycle via Admission Webhook

**Status**: Approved
**Date**: 2026-03-09
**Deciders**: Architecture Team
**Related**: BR-WORKFLOW-007, ADR-058, BR-WORKFLOW-006, DD-ACTIONTYPE-001, ADR-034
**Version**: 1.0

---

## Changelog

### Version 1.0 (2026-03-09)

- Initial decision: migrate ActionType provisioning from SQL seeds to CRD + Admission Webhook

---

## Context

### Problem Statement

ActionType taxonomy entries in Kubernaut are currently provisioned through SQL migration scripts that insert rows directly into the `action_type_taxonomy` table. This is inconsistent with the RemediationWorkflow pattern established in ADR-058/BR-WORKFLOW-006, where CRDs + Admission Webhooks provide Kubernetes-native lifecycle management with audit, RBAC, and GitOps support.

The discrepancy means:
1. Action types cannot be managed via `kubectl` or GitOps pipelines
2. No lifecycle management (disable, re-enable, description updates)
3. No audit trail for taxonomy changes (SOC2 gap)
4. No dependency visibility between action types and workflows

### Requirements

- Kubernetes-native provisioning via `kubectl apply`
- Lifecycle management: idempotent CREATE, description UPDATE, soft-DELETE
- Dependency guard: prevent disabling action types with active workflows
- SOC2-compliant audit trail for all changes
- Operator visibility via printer columns

---

## Options Evaluated

### Option A: Enhanced SQL Migrations (Rejected)

Add lifecycle columns to the SQL migration and manage via direct DB access.

**Pros**: Simple, no new CRD/webhook infrastructure
**Cons**: No Kubernetes-native management, no RBAC, no GitOps, no audit trail, inconsistent with RW pattern

### Option B: CRD + Admission Webhook (Approved)

Follow the RemediationWorkflow pattern: ActionType CRD with ValidatingWebhookConfiguration that bridges to DS REST API.

**Pros**: Kubernetes-native, GitOps-compatible, RBAC-controlled, audited, consistent pattern
**Cons**: Additional infrastructure (CRD, webhook handler, DS endpoints)

### Option C: ConfigMap-Based Provisioning (Rejected)

Store action types in a ConfigMap and reconcile via a controller.

**Pros**: Simple Kubernetes-native storage
**Cons**: No per-entry lifecycle, no admission validation, no status subresource, poor audit trail

---

## Decision

**APPROVED: Option B -- CRD + Admission Webhook**

ActionType provisioning migrates to a Kubernetes CRD (`ActionType`) with a ValidatingWebhookConfiguration that intercepts CREATE, UPDATE, and DELETE operations. The webhook bridges to Data Storage REST API endpoints for catalog management.

This follows the exact pattern established by ADR-058 for RemediationWorkflow CRDs.

---

## Rationale

1. **Consistency**: Same pattern as RemediationWorkflow (ADR-058), reducing cognitive load
2. **Kubernetes-native**: `kubectl apply/edit/delete`, GitOps, RBAC
3. **Audit trail**: Per-operation audit events satisfy SOC2 CC8.1/CC7.3/CC7.4
4. **Dependency tracking**: `status.activeWorkflowCount` provides operator visibility; DS-level dependency guard prevents unsafe deletion
5. **Soft-delete**: Preserves referential integrity and enables re-enablement

### Why Soft-Delete, Not Hard-Delete

- Existing RemediationWorkflow records reference action types by name
- Hard-delete would orphan those references
- SOC2 requires audit trail continuity (disabled_at, disabled_by)
- Re-enablement is a valid business operation

---

## Lifecycle Specification

| Operation | Behavior |
|-----------|----------|
| **CREATE** | Idempotent: NOOP if active, re-enable if disabled, create if new |
| **UPDATE** | Only `spec.description` mutable; audit trail with old+new values |
| **DELETE** | Soft-disable; denied with 409 if active workflows reference it |

---

## Component Interactions

```
kubectl apply -f actiontype.yaml
        |
        v
  [K8s API Server]
        |
        v  (ValidatingWebhookConfiguration)
  [AuthWebhook /validate-actiontype]
        |
        v  (REST API call)
  [Data Storage POST/PATCH /api/v1/action-types]
        |
        v  (DB operation)
  [PostgreSQL action_type_taxonomy]
        |
  (async) <-- AW patches CRD status
        v
  [ActionType CRD .status updated]
```

---

## Consequences

### Positive

- Consistent CRD pattern across the platform
- Full audit trail for SOC2 compliance
- GitOps and RBAC support for action type management
- Operator visibility via `kubectl get at` printer columns

### Negative

- Additional DS REST endpoints to maintain
- Webhook handler adds processing to API server admission path
- Cross-update (activeWorkflowCount) is eventually consistent

### Mitigations

- DS endpoints follow established patterns from workflow CRUD
- Webhook handler mirrors RemediationWorkflow handler structure
- Cross-update failures are logged but non-blocking (best-effort)

---

## Related Documents

- [BR-WORKFLOW-007](../../requirements/BR-WORKFLOW-007-actiontype-crd.md) -- Business requirement
- [DD-ACTIONTYPE-001](DD-ACTIONTYPE-001-crd-lifecycle-design.md) -- Detailed design
- [ADR-058](ADR-058-webhook-driven-workflow-registration.md) -- RemediationWorkflow webhook pattern
- [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md) -- RemediationWorkflow CRD
- [ADR-034](ADR-034-unified-audit-table-design.md) -- Unified audit table

---

## Approval

| Role | Name | Date |
|------|------|------|
| Architecture | AI Assistant | 2026-03-09 |
