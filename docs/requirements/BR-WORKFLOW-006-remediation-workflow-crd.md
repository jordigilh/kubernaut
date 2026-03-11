# BR-WORKFLOW-006: Kubernetes-Native Workflow Registration via RemediationWorkflow CRD

**Business Requirement ID**: BR-WORKFLOW-006
**Category**: Workflow Catalog Service
**Priority**: P0
**Target Version**: V1.0
**Status**: Active
**Date**: March 4, 2026
**Version**: 1.0

**Authority**: This is the authoritative specification for the RemediationWorkflow CRD lifecycle, including CRD format, AuthWebhook behavior, DS integration, and status subresource semantics.

**Related**:
- [ADR-058](../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) -- Webhook-driven registration architecture
- [BR-WORKFLOW-004](./BR-WORKFLOW-004-workflow-schema-format.md) -- Workflow schema format specification
- [DD-WEBHOOK-001](../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md) -- CRD Webhook Requirements Matrix
- [DD-WEBHOOK-003](../architecture/decisions/DD-WEBHOOK-003-webhook-complete-audit-pattern.md) -- Webhook audit pattern
- [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) -- Workflow lifecycle interactions
- [DD-WORKFLOW-012](../architecture/decisions/DD-WORKFLOW-012-workflow-immutability-constraints.md) -- Immutability constraints
- [ADR-034](../architecture/decisions/ADR-034-unified-audit-table-design.md) -- Unified audit table

**GitHub Issue**: [#299](https://github.com/jordigilh/kubernaut/issues/299)

---

## Changelog

### Version 1.0 (2026-03-04)

- Initial specification for RemediationWorkflow CRD lifecycle
- Defines CRD format (apiVersion/kind/metadata/spec/status), validation rules, AW behavior
- Establishes CRD as THE registration mechanism; DS REST API is internal only

---

## Business Need

### Problem Statement

Workflow registration in Kubernaut must be Kubernetes-native to support GitOps workflows, declarative cluster management, and standard RBAC controls. Operators should register workflows using `kubectl apply` rather than direct REST API calls to an internal service.

### Impact Without This BR

- Workflow registration requires knowledge of internal DS API endpoints
- No GitOps support: workflows cannot be managed via Flux, ArgoCD, or Git-based pipelines
- No Kubernetes RBAC for workflow management (relies on DS-level auth)
- No declarative desired-state management for the workflow catalog

### Business Objective

**Workflow registration and lifecycle management SHALL be performed exclusively through the RemediationWorkflow CRD. The Data Storage REST API for workflow registration is an internal implementation detail consumed only by the AuthWebhook.**

---

## CRD Specification

### API Identity

| Field | Value |
|-------|-------|
| **apiVersion** | `kubernaut.ai/v1alpha1` |
| **kind** | `RemediationWorkflow` |
| **shortName** | `rw` |
| **scope** | Namespaced (`kubernaut-system`) |
| **subresources** | `status` (`+kubebuilder:subresource:status`) |

### Spec Fields

The `.spec` maps to the workflow-schema.yaml content per BR-WORKFLOW-004, structured as a Kubernetes resource. The workflow name is provided by `metadata.name`; the DS-assigned UUID is in `status.workflowId`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spec.version` | string | Yes | Semantic version (e.g., "1.0.0") |
| `spec.description` | object | Yes | Structured description: `what`, `whenToUse`, `whenNotToUse`, `preconditions` |
| `spec.maintainers` | array | No | Maintainer name/email pairs |
| `spec.actionType` | string | Yes | Action type from taxonomy (PascalCase) |
| `spec.labels` | object | Yes | `severity`, `environment`, `component`, `priority` |
| `spec.customLabels` | map | No | Operator-defined key-value labels |
| `spec.detectedLabels` | JSON | No | Author-declared infrastructure requirements |
| `spec.execution` | object | Yes | Engine config: `engine`, `bundle`, `bundleDigest`, `engineConfig` |
| `spec.parameters` | array | Yes | Workflow input parameters (min 1) |
| `spec.rollbackParameters` | array | No | Rollback parameter definitions |
| `spec.dependencies` | object | No | Secrets and ConfigMaps required by the workflow |

### Status Fields

The `.status` subresource is populated asynchronously by the AuthWebhook after successful registration:

| Field | Type | Description |
|-------|------|-------------|
| `status.workflowId` | string | UUID assigned by Data Storage |
| `status.catalogStatus` | string | DS catalog state: `active`, `disabled`, `deprecated`, `archived` |
| `status.registeredBy` | string | Authenticated user who created the CRD |
| `status.registeredAt` | Time | Timestamp of registration in DS |
| `status.previouslyExisted` | bool | `true` if this workflow was re-enabled from a previously disabled state |

### Printer Columns

```
NAME       ACTION           ENGINE   VERSION   STATUS   AGE
my-wf      RestartPod       tekton   1.0.0     active   5m
```

---

## CRD Lifecycle

### CREATE: Registration

1. Operator applies `RemediationWorkflow` CR via `kubectl apply`
2. K8s API server sends `AdmissionReview` (CREATE) to AuthWebhook
3. AW extracts authenticated user from `req.UserInfo`
4. AW marshals CRD spec to JSON, calls DS internal API `POST /api/v1/workflows` with `content`, `source: "crd"`, and `registeredBy`
5. DS parses schema, validates, inserts into catalog, returns `workflowId`
6. AW emits `remediationworkflow.admitted.create` audit event
7. AW returns `Allowed` -- CRD is created in etcd
8. AW asynchronously updates `.status` with `workflowId`, `catalogStatus`, `registeredBy`, `registeredAt`

**Failure modes**:
- DS unreachable: AW returns `Denied` with error message; CRD is NOT created
- Schema validation failure: AW returns `Denied` with DS error; CRD is NOT created
- Status update failure: CRD exists, workflow IS registered in DS; `.status` remains empty (informational only)

### DELETE: Disable

1. Operator deletes `RemediationWorkflow` CR via `kubectl delete`
2. K8s API server sends `AdmissionReview` (DELETE) to AuthWebhook
3. AW extracts `status.workflowId` from the old object
4. AW calls DS `PATCH /api/v1/workflows/{id}/disable` (best-effort)
5. AW emits `remediationworkflow.admitted.delete` audit event
6. AW returns `Allowed` -- CRD is deleted from etcd

**Best-effort DELETE**: DELETE is ALWAYS allowed regardless of DS response. This prevents GitOps drift where a CRD cannot be removed because DS is unreachable.

### UPDATE: Passthrough

UPDATE operations pass through without DS interaction. Per DD-WORKFLOW-012, workflow specs are immutable -- the only mutable fields are Kubernetes metadata (labels, annotations). Spec changes require creating a new version.

---

## Audit Events

Per DD-WEBHOOK-003 and ADR-034:

| Event Type | Category | Action | Outcome | Trigger |
|------------|----------|--------|---------|---------|
| `remediationworkflow.admitted.create` | `webhook` | `admitted` | `success` | CREATE allowed after DS registration |
| `remediationworkflow.admitted.delete` | `webhook` | `admitted` | `success` | DELETE allowed (with or without DS disable) |
| `remediationworkflow.admitted.denied` | `webhook` | `denied` | `failure` | CREATE denied (auth failure, DS error, unmarshal error) |

All events include: authenticated user, resource name/ID, correlation ID (admission UID), namespace.

---

## RBAC

### AuthWebhook Service Account

```yaml
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationworkflows"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationworkflows/status"]
  verbs: ["update", "patch"]
```

### Operator RBAC

Operators creating/deleting `RemediationWorkflow` CRDs need standard Kubernetes RBAC:

```yaml
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationworkflows"]
  verbs: ["create", "get", "list", "watch", "delete"]
```

---

## Validation Rules

### CRD-Level (Kubebuilder Markers)

- `spec.actionType`: Required
- `spec.labels.severity`: MinItems=1
- `spec.labels.environment`: MinItems=1
- `spec.labels.component`: Required
- `spec.labels.priority`: Required
- `spec.parameters`: MinItems=1
- `spec.execution.engine`: Enum `tekton|job|ansible`
- `metadata.name`: Required, MaxLength=255 (workflow name)
- `spec.version`: Required, MaxLength=50

### DS-Level (Server-Side)

DS performs additional validation when the AW forwards the content:
- Schema parsing and structural validation (BR-WORKFLOW-004)
- Action type FK validation against taxonomy
- Unique constraint on `(workflow_name, version)` -- returns 409 on conflict (triggers re-enable)
- Content hash computation for deduplication

---

## Dual Storage Model

Workflow data exists in two stores:

| Store | Contains | Source of Truth For |
|-------|----------|---------------------|
| **etcd** (Kubernetes) | CRD spec + status | Desired state, GitOps reconciliation |
| **PostgreSQL** (Data Storage) | Catalog entry with embeddings, search index, content hash | Workflow discovery, execution, audit |

The AuthWebhook bridges these stores: CRD CREATE triggers DS catalog insert; CRD DELETE triggers DS catalog disable. There is no reconciliation controller in V1.0 -- if stores diverge, manual intervention is required.

---

## Acceptance Criteria

1. `RemediationWorkflow` CRD can be created via `kubectl apply` in `kubernaut-system` namespace
2. CREATE triggers DS registration; CRD is rejected if DS registration fails
3. DELETE triggers DS disable (best-effort); CRD deletion always succeeds
4. `.status` reflects DS registration state (workflowId, catalogStatus, registeredBy, registeredAt)
5. Audit events emitted for CREATE (admitted/denied) and DELETE (admitted)
6. Authenticated user captured from Kubernetes admission context (SOC2 CC8.1)
7. RBAC enforced via standard Kubernetes mechanisms
8. Existing workflows can be re-enabled by re-applying a previously deleted CRD (`.status.previouslyExisted: true`)

---

## References

- [ADR-058](../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) -- Architecture decision
- [BR-WORKFLOW-004](./BR-WORKFLOW-004-workflow-schema-format.md) -- Schema format
- [DD-WORKFLOW-012](../architecture/decisions/DD-WORKFLOW-012-workflow-immutability-constraints.md) -- Immutability
- [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) -- Lifecycle
- [Test Plan](../testing/299/TEST_PLAN.md) -- Phase 2 test plan

---

**Document Status**: Active
**Version**: 1.0
