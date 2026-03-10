# DD-ACTIONTYPE-001: ActionType CRD Lifecycle Design

**Version**: 1.0
**Date**: 2026-03-09
**Status**: APPROVED
**Author**: Architecture Team
**Reviewers**: Platform Team, Catalog Team

---

## Context

This design document details the implementation of ActionType CRD lifecycle management as specified in BR-WORKFLOW-007 and decided in ADR-059. It covers the CRD schema, DS API endpoints, AW handler flows, RW cross-update mechanism, DB schema changes, and Helm integration.

---

## CRD Schema

### ActionType CRD

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: ActionType
metadata:
  name: restart-pod
  namespace: kubernaut-system
spec:
  name: RestartPod
  description:
    what: "Deletes and restarts a failing pod to clear transient errors"
    whenToUse: "When a pod is in CrashLoopBackOff or unresponsive state"
    whenNotToUse: "When the issue is caused by misconfiguration that persists"
    preconditions: "Verify the pod is not handling in-flight requests"
status:
  registered: true
  registeredAt: "2026-03-09T10:30:00Z"
  registeredBy: "system:serviceaccount:kubernaut-system:authwebhook"
  previouslyExisted: false
  activeWorkflowCount: 3
  catalogStatus: active
```

### Printer Columns

```
$ kubectl get at
NAME           ACTION TYPE      WORKFLOWS   REGISTERED   AGE
restart-pod    RestartPod       3           true         5d
scale-replicas ScaleReplicas    1           true         5d
drain-node     DrainNode        0           true         2d

$ kubectl get at -o wide
NAME           ACTION TYPE      WORKFLOWS   REGISTERED   DESCRIPTION                                    AGE
restart-pod    RestartPod       3           true         Kill and recreate one or more pods.             5d
```

### Selectable Fields

`.spec.name` is a selectable field, enabling filtering:

```bash
kubectl get at --field-selector spec.name=RestartPod
```

---

## Database Schema Changes

### Migration: `migrations/004_action_type_lifecycle.sql`

Add lifecycle columns to the existing `action_type_taxonomy` table:

```sql
ALTER TABLE action_type_taxonomy
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active',
  ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN IF NOT EXISTS disabled_by TEXT;
```

These columns support:
- `status`: `'active'` or `'disabled'` (soft-delete state)
- `disabled_at`: Timestamp when the action type was disabled
- `disabled_by`: Identity (K8s SA or user) who disabled it

---

## Data Storage REST API

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/action-types` | Create or re-enable an action type |
| PATCH | `/api/v1/action-types/{name}` | Update description fields |
| PATCH | `/api/v1/action-types/{name}/disable` | Soft-disable (with dependency guard) |

### POST /api/v1/action-types (Idempotent Create)

**Request**:
```json
{
  "name": "RestartPod",
  "description": {
    "what": "Deletes and restarts a failing pod",
    "whenToUse": "Pod is in CrashLoopBackOff",
    "whenNotToUse": "Misconfiguration that persists",
    "preconditions": "No in-flight requests"
  },
  "registeredBy": "system:serviceaccount:kubernaut-system:authwebhook"
}
```

**Behavior**:
1. If no matching entry: INSERT new row, return 201 with `{ status: "created", wasReenabled: false }`
2. If matching active entry: return 200 with `{ status: "exists", wasReenabled: false }`
3. If matching disabled entry: UPDATE to active, clear disabled_at/disabled_by, return 200 with `{ status: "reenabled", wasReenabled: true }`

### PATCH /api/v1/action-types/{name} (Update Description)

**Request**:
```json
{
  "description": {
    "what": "Updated description",
    "whenToUse": "Updated usage guidance"
  },
  "updatedBy": "admin@example.com"
}
```

**Response**: 200 with updated record + `updatedFields` list + old/new descriptions for audit.

**Constraint**: Returns 404 if the action type does not exist or is disabled.

### PATCH /api/v1/action-types/{name}/disable (Soft-Disable)

**Request**:
```json
{
  "disabledBy": "system:serviceaccount:kubernaut-system:authwebhook"
}
```

**Behavior**:
1. Query count of active RemediationWorkflows with this action type
2. If count > 0: return 409 Conflict with `{ dependentWorkflowCount: N, dependentWorkflows: ["name1", "name2"] }`
3. If count == 0: UPDATE status to `'disabled'`, set `disabled_at`, `disabled_by`, return 200

---

## AuthWebhook Handler

### Webhook Path: `/validate-actiontype`

Intercepts: CREATE, UPDATE, DELETE operations on `actiontypes.kubernaut.ai`.

### CREATE Flow

```
1. Extract ActionType from admission request
2. Extract user identity via authenticator
3. Call DS POST /api/v1/action-types
4. If DS returns success:
   a. Async: Patch CRD status (registered=true, registeredAt, registeredBy, catalogStatus=active)
   b. Emit AW audit: actiontype.admitted.create
   c. Return Allowed
5. If DS returns error:
   a. Emit AW audit: actiontype.denied.create
   b. Return Denied with error message
```

### UPDATE Flow

```
1. Extract old and new ActionType from admission request
2. Reject if spec.name changed (immutable)
3. Compare descriptions, build diff
4. If no changes: Return Allowed (NOOP)
5. Call DS PATCH /api/v1/action-types/{name}
6. Emit AW audit: actiontype.admitted.update
7. Return Allowed
```

### DELETE Flow

```
1. Extract ActionType from admission request (old object)
2. Call DS PATCH /api/v1/action-types/{name}/disable
3. If DS returns 200 (no dependencies):
   a. Async: Patch CRD status (catalogStatus=disabled)
   b. Emit AW audit: actiontype.admitted.delete
   c. Return Allowed
4. If DS returns 409 (has dependencies):
   a. Emit AW audit: actiontype.denied.delete
   b. Return Denied with message: "Cannot delete ActionType 'X': Y active workflows depend on it (workflow1, workflow2)"
```

---

## RW Handler Cross-Update (activeWorkflowCount)

When the existing RemediationWorkflow admission webhook handler processes CREATE or DELETE:

1. After DS registration/deregistration succeeds, extract `spec.actionType` from the RW
2. Best-effort async: Look up `ActionType` CRD by `.spec.name == <actionType>`
3. Call DS to get current count of active workflows for that action type
4. Patch `ActionType` CRD `status.activeWorkflowCount` with refreshed count
5. Failures logged but do not block RW admission (eventual consistency)

---

## Audit Events

### Data Storage Events

| Event Type | Payload | When |
|-----------|---------|------|
| `datastorage.actiontype.created` | name, description, registeredBy, wasReenabled | New entry or re-enable |
| `datastorage.actiontype.updated` | name, oldDescription, newDescription, updatedBy, updatedFields | Description change |
| `datastorage.actiontype.disabled` | name, disabledBy, disabledAt | Successful disable |
| `datastorage.actiontype.disable_denied` | name, dependentWorkflowCount, dependentWorkflows, requestedBy | Denied due to dependencies |
| `datastorage.actiontype.reenabled` | name, reenabledBy, prevDisabledAt, prevDisabledBy | Re-enable from disabled |

### AuthWebhook Events

| Event Type | Payload | When |
|-----------|---------|------|
| `actiontype.admitted.create` | ActionType spec, user identity, DS result | CREATE allowed |
| `actiontype.admitted.update` | Changed fields, user identity | UPDATE allowed |
| `actiontype.admitted.delete` | ActionType name, user identity | DELETE allowed |
| `actiontype.denied.delete` | ActionType name, reason, dependent count/names | DELETE denied |

---

## Helm Integration

### ValidatingWebhookConfiguration

Add `actiontype.validate.kubernaut.ai` webhook entry for CREATE + UPDATE + DELETE operations on `actiontypes.kubernaut.ai`.

### RBAC

Grant to AW service account:
- `get`, `list`, `watch` on `actiontypes`
- `update`, `patch` on `actiontypes/status`

### CRD Manifest

`charts/kubernaut/crds/kubernaut.ai_actiontypes.yaml` synced from `config/crd/bases/`.

### Seed Job Ordering

ActionType CRDs applied BEFORE RemediationWorkflow CRDs to ensure action types exist when workflows reference them.

---

## Affected Components

| Component | Change |
|-----------|--------|
| `api/actiontype/v1alpha1/` | New CRD types |
| `pkg/datastorage/repository/actiontype/` | New DB repository |
| `pkg/datastorage/server/` | New HTTP handlers |
| `api/openapi/data-storage-v1.yaml` | New endpoints |
| `pkg/authwebhook/` | New webhook handler |
| `cmd/authwebhook/main.go` | Register handler |
| `charts/kubernaut/` | Webhook config, RBAC, CRD |
| `deploy/action-types/` | 24 ActionType CRD YAMLs |
| `migrations/` | DB schema migration |

---

## Related Documents

- [BR-WORKFLOW-007](../../requirements/BR-WORKFLOW-007-actiontype-crd.md) -- Business requirement
- [ADR-059](ADR-059-actiontype-crd-lifecycle.md) -- Architecture decision
- [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md) -- RemediationWorkflow CRD pattern
- [ADR-058](ADR-058-webhook-driven-workflow-registration.md) -- Webhook-driven registration
- [ADR-034](ADR-034-unified-audit-table-design.md) -- Unified audit table

---

## Document Maintenance

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-09 | Initial design covering CRD schema, DS API, AW handler, RW cross-update, Helm |
