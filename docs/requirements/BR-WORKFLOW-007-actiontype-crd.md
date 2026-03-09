# BR-WORKFLOW-007: Kubernetes-Native ActionType Taxonomy via ActionType CRD

**Business Requirement ID**: BR-WORKFLOW-007
**Category**: Workflow Catalog Service
**Priority**: P0
**Target Version**: V1.0
**Status**: Active
**Date**: March 9, 2026
**Version**: 1.0

**Authority**: This is the authoritative specification for the ActionType CRD lifecycle, including CRD format, AuthWebhook behavior, DS integration, and status subresource semantics.

**Related**:
- [ADR-059](../architecture/decisions/ADR-059-actiontype-crd-lifecycle.md) -- ActionType CRD lifecycle architecture
- [DD-ACTIONTYPE-001](../architecture/decisions/DD-ACTIONTYPE-001-crd-lifecycle-design.md) -- Detailed design
- [BR-WORKFLOW-006](./BR-WORKFLOW-006-remediation-workflow-crd.md) -- RemediationWorkflow CRD (blueprint pattern)
- [ADR-058](../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) -- Webhook-driven registration architecture
- [ADR-034](../architecture/decisions/ADR-034-unified-audit-table-design.md) -- Unified audit table
- [DD-WEBHOOK-003](../architecture/decisions/DD-WEBHOOK-003-webhook-complete-audit-pattern.md) -- Webhook audit pattern

**GitHub Issue**: [#300](https://github.com/jordigilh/kubernaut/issues/300)

---

## Changelog

### Version 1.0 (2026-03-09)

- Initial specification for ActionType CRD lifecycle
- Defines CRD format, lifecycle operations (CREATE/UPDATE/DELETE), audit trail requirements
- Establishes CRD as THE provisioning mechanism for action types; SQL seed data migration

---

## Business Need

### Problem Statement

Action type provisioning in Kubernaut is currently performed through SQL migration scripts that insert rows directly into the `action_type_taxonomy` database table. This approach:

1. Bypasses Kubernetes RBAC and audit controls
2. Cannot be managed via GitOps workflows (Flux, ArgoCD)
3. Provides no declarative desired-state management
4. Offers no lifecycle management (disable, re-enable, description update)
5. Lacks SOC2-compliant audit trail for taxonomy changes

### Impact Without This BR

- ActionType taxonomy changes require SQL migrations with database access
- No visibility into action type lifecycle via `kubectl`
- No dependency tracking between action types and active workflows
- No audit trail for who changed what and when

### Business Objective

**ActionType taxonomy management SHALL be performed exclusively through the ActionType CRD. Operators use `kubectl apply` to register, `kubectl edit` to update descriptions, and `kubectl delete` to soft-disable action types. The Data Storage REST API for action type management is an internal implementation detail consumed only by the AuthWebhook.**

---

## CRD Specification

### API Identity

| Field | Value |
|-------|-------|
| **apiVersion** | `kubernaut.ai/v1alpha1` |
| **kind** | `ActionType` |
| **shortName** | `at` |
| **scope** | Namespaced (`kubernaut-system`) |
| **subresources** | `status` (`+kubebuilder:subresource:status`) |
| **selectableFields** | `.spec.name` |

### Spec Fields

| Field | Type | Required | Mutable | Description |
|-------|------|----------|---------|-------------|
| `spec.name` | string | Yes | No | PascalCase action type identifier (e.g., `RestartPod`) |
| `spec.description.what` | string | Yes | Yes | What this action type concretely does |
| `spec.description.whenToUse` | string | Yes | Yes | When this action type is appropriate |
| `spec.description.whenNotToUse` | string | No | Yes | Exclusion conditions |
| `spec.description.preconditions` | string | No | Yes | Conditions to verify before use |

### Status Fields (Webhook-Managed)

| Field | Type | Description |
|-------|------|-------------|
| `status.registered` | bool | Whether registered in DS catalog |
| `status.registeredAt` | timestamp | When first registered |
| `status.registeredBy` | string | K8s SA or user identity |
| `status.previouslyExisted` | bool | true if re-enabled after disable |
| `status.activeWorkflowCount` | int | Active RemediationWorkflows referencing this type |
| `status.catalogStatus` | string | DS catalog state: `active` or `disabled` |

### Printer Columns

Default (`kubectl get at`):

| Column | JSONPath | Type |
|--------|----------|------|
| ACTION TYPE | `.spec.name` | string |
| WORKFLOWS | `.status.activeWorkflowCount` | integer |
| REGISTERED | `.status.registered` | boolean |
| AGE | `.metadata.creationTimestamp` | date |

Wide (`kubectl get at -o wide`):

| Column | JSONPath | Type | Priority |
|--------|----------|------|----------|
| DESCRIPTION | `.spec.description.what` | string | 1 |

---

## Lifecycle Requirements

### BR-WORKFLOW-007.1: CREATE (Idempotent Registration)

**MUST**: When an ActionType CRD is created:

1. If no matching action type exists in DS: create new entry with `status = 'active'`, populate CRD status
2. If matching action type exists and is active: NOOP, populate CRD status from existing record
3. If matching action type exists and is disabled: re-enable (set `status = 'active'`, clear `disabled_at`/`disabled_by`), set `status.previouslyExisted = true`

**MUST**: Admission webhook returns `Allowed` for all three cases.

### BR-WORKFLOW-007.2: UPDATE (Description Only)

**MUST**: Only `spec.description` fields are mutable. Changes to `spec.name` SHALL be denied.

**MUST**: Description updates generate an audit trail containing:
- Old description values (before change)
- New description values (after change)
- List of changed fields
- Identity of who made the change

### BR-WORKFLOW-007.3: DELETE (Soft-Disable with Dependency Guard)

**MUST**: DELETE sets the action type to `disabled` status (soft-delete). Hard-delete is not supported.

**MUST**: DELETE is denied (409 Conflict) if active RemediationWorkflows reference the action type. The denial message includes:
- Count of dependent workflows
- Names of dependent workflows

**MUST**: When no active workflows reference the type, the action type is soft-disabled with `disabled_at` and `disabled_by` recorded.

### BR-WORKFLOW-007.4: Audit Trail (SOC2 Compliance)

**MUST**: All lifecycle operations emit audit events to the unified audit table per ADR-034:

| Operation | DS Event Type | AW Event Type |
|-----------|--------------|---------------|
| CREATE | `datastorage.actiontype.created` | `actiontype.admitted.create` |
| UPDATE | `datastorage.actiontype.updated` | `actiontype.admitted.update` |
| DELETE (allowed) | `datastorage.actiontype.disabled` | `actiontype.admitted.delete` |
| DELETE (denied) | `datastorage.actiontype.disable_denied` | `actiontype.denied.delete` |

### BR-WORKFLOW-007.5: Cross-Update (Workflow Count)

**SHOULD**: When a RemediationWorkflow CRD is created or deleted, the corresponding ActionType CRD's `status.activeWorkflowCount` is updated asynchronously. This is best-effort and non-blocking.

---

## Non-Requirements

- Hard-delete of action types (preserves referential integrity, audit continuity)
- Versioning of action types (action types are unversioned taxonomy entries)
- Direct DS REST API access for action type management (internal only)

---

## Acceptance Criteria

1. `kubectl apply -f actiontype.yaml` registers the action type in DS and populates CRD status
2. `kubectl get at` shows ACTION TYPE, WORKFLOWS, REGISTERED, AGE columns
3. `kubectl get at -o wide` additionally shows DESCRIPTION column
4. `kubectl edit at restart-pod` allows updating description fields only
5. `kubectl delete at restart-pod` soft-disables when no active workflows reference it
6. `kubectl delete at restart-pod` is denied with workflow count+names when dependencies exist
7. All operations emit audit events with actor attribution
8. Re-applying a previously disabled action type re-enables it

---

## Test Coverage

See [Test Plan for #300](../../testing/300/TEST_PLAN.md) for complete test scenarios.

---

## Implementation Status

| Phase | Component | Status |
|-------|-----------|--------|
| Phase 1 | CRD Types + Manifests | Complete |
| Phase 2a | DB Migration + Repository | Complete |
| Phase 2b | DS HTTP Handlers | Complete |
| Phase 3a | DS Client Adapter | Complete |
| Phase 3b | AW Webhook Handler | Complete |
| Phase 3c | RW Cross-Update | Complete |
| Phase 4 | Helm Charts | Complete |
| Phase 5 | Seed Data Migration | Complete |
| Phase 6 | Audit Events (DS + AW) | Complete |
| Phase 7a | Unit Tests (15 specs) | Complete |
| Phase 7b | Integration Tests (12 specs) | Complete |
| Phase 7c | E2E Tests (9 specs) | Complete |
