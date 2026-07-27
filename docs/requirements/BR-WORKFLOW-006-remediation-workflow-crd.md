# BR-WORKFLOW-006: Kubernetes-Native Workflow Registration via RemediationWorkflow CRD

**Business Requirement ID**: BR-WORKFLOW-006
**Category**: Workflow Catalog Service
**Priority**: P0
**Target Version**: V1.0
**Status**: Active
**Date**: March 4, 2026
**Version**: 1.2

**Authority**: This is the authoritative specification for the RemediationWorkflow CRD lifecycle, including CRD format, AuthWebhook behavior, DS integration, and status subresource semantics.

**Related**:
- [ADR-058](../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) -- Webhook-driven registration architecture
- [BR-WORKFLOW-004](./BR-WORKFLOW-004-workflow-schema-format.md) -- Workflow schema format specification
- [DD-WEBHOOK-001](../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md) -- CRD Webhook Requirements Matrix
- [DD-WEBHOOK-003](../architecture/decisions/DD-WEBHOOK-003-webhook-complete-audit-pattern.md) -- Webhook audit pattern
- [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) -- Workflow lifecycle interactions
- [DD-WORKFLOW-018](../architecture/decisions/DD-WORKFLOW-018-etcd-single-source-of-truth.md) -- Storage architecture (etcd single source of truth, supersedes the dual-storage model)
- [DD-WORKFLOW-012](../architecture/decisions/DD-WORKFLOW-012-workflow-immutability-constraints.md) -- Immutability constraints
- [ADR-034](../architecture/decisions/ADR-034-unified-audit-table-design.md) -- Unified audit table

**GitHub Issue**: [#299](https://github.com/jordigilh/kubernaut/issues/299)

---

## Changelog

### Version 1.2 (2026-07-14) — DD-WORKFLOW-018 / Issue #1661

- Rewrote "Dual Storage Model" section as "Storage Model": etcd is now the sole source of truth; PostgreSQL is
  audit-only. Removed the now-inaccurate "no reconciliation controller... manual intervention required" caveat --
  a single source of truth has nothing to reconcile against.
- Cross-references [DD-WORKFLOW-018](../architecture/decisions/DD-WORKFLOW-018-etcd-single-source-of-truth.md) as
  authoritative for the storage architecture; this document remains authoritative for the CRD spec/lifecycle/RBAC.

### Version 1.1 (2026-04-21) — Issue #773

- UPDATE operations now intercepted by AuthWebhook (not passthrough)
- UPDATE re-registers CRD with DS, following same strict error-handling as CREATE
- Added `remediationworkflow.admitted.update` audit event type (SOC2 CC8.1)
- Version-locked content immutability: same version + different content → 409 Conflict
- Cross-version supersession: version bump → old workflow disabled, new created
- Idempotent re-apply: same version + same content → 200 OK (no DB writes)
- Updated acceptance criteria (3–6) for UPDATE behavior

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

The `.spec` is the declarative specification of the `RemediationWorkflow` resource. Field names and types originate from BR-WORKFLOW-004. The workflow name is provided by `metadata.name`; the DS-assigned UUID is in `status.workflowId`.

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

### UPDATE: Re-Registration with Content Integrity

UPDATE operations are intercepted by the AuthWebhook and re-registered with DS, following the same strict error-handling semantics as CREATE (Issue #773):

1. Operator updates `RemediationWorkflow` CR (e.g., version bump, idempotent re-apply)
2. K8s API server sends `AdmissionReview` (UPDATE) to AuthWebhook
3. AW extracts authenticated user, marshals clean CRD content, calls DS `POST /api/v1/workflows`
4. DS content integrity logic determines the outcome:
   - **Same version + same content hash**: Idempotent (200 OK, no DB writes)
   - **Same version + different content hash**: Rejected (409 Conflict, `content-integrity-violation`)
   - **Different version**: Cross-version supersession (old workflow disabled, new created)
5. On success: AW emits `remediationworkflow.admitted.update` audit event, returns `Allowed`
6. On failure: AW emits `remediationworkflow.admitted.denied` audit event, returns `Denied`

**Version-locked content immutability**: Once a workflow version is active, its content cannot be changed via UPDATE. Operators must bump the version to register new content. This enforces reproducibility and prevents silent spec drift.

---

## Audit Events

Per DD-WEBHOOK-003 and ADR-034:

| Event Type | Category | Action | Outcome | Trigger |
|------------|----------|--------|---------|---------|
| `remediationworkflow.admitted.create` | `webhook` | `admitted` | `success` | CREATE allowed after DS registration |
| `remediationworkflow.admitted.update` | `webhook` | `admitted` | `success` | UPDATE allowed after DS re-registration (Issue #773) |
| `remediationworkflow.admitted.delete` | `webhook` | `admitted` | `success` | DELETE allowed (with or without DS disable) |
| `remediationworkflow.admitted.denied` | `webhook` | `denied` | `failure` | CREATE or UPDATE denied (auth failure, DS error, unmarshal error, content integrity violation) |

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
- `spec.labels.component`: MinItems=1 (Issue #790: now an array like severity/environment)
- `spec.labels.priority`: Required
- `spec.parameters`: MinItems=1
- `spec.execution.engine`: Enum `tekton|job|ansible`
- `metadata.name`: Required, MaxLength=255 (workflow name)
- `spec.version`: Required, MaxLength=50

### AuthWebhook-Level (Server-Side)

> **Revised by DD-WORKFLOW-018**: this validation now runs locally in AuthWebhook against etcd, not in DS against
> PostgreSQL -- there is no DS round-trip on the admission path anymore. Renamed from "DS-Level" accordingly.

AuthWebhook performs additional validation locally, against its own etcd-backed client:
- Action type existence validation against `ActionType` CRDs (`.spec.name` field indexer, `CatalogStatus == Active`)
- Content-hash/workflow-ID computation (deterministic, shared pure functions -- also used for deduplication)
- Content-integrity/version-conflict check on `(workflow_name, version)` against existing CRDs -- returns 409 on
  same-version-different-content conflict; version bump triggers cross-version supersession

---

## Storage Model

> **Revised 2026-07-14 by [DD-WORKFLOW-018](../architecture/decisions/DD-WORKFLOW-018-etcd-single-source-of-truth.md)** (v1.2 changelog below). This section previously described a dual-storage model where PostgreSQL held an independently mutable catalog copy. That model is superseded.

**etcd is the sole source of truth** for `RemediationWorkflow` CRD data. There is only one store:

| Store | Contains | Source of Truth For |
|-------|----------|---------------------|
| **etcd** (Kubernetes) | CRD spec + status | Everything: desired state, discovery, execution metadata, GitOps reconciliation |

Data Storage (DS) maintains an **informer-backed, read-only in-memory cache** mirroring the `RemediationWorkflow`/`ActionType` CRDs in etcd, used for discovery/search/scoring. This cache is never written to by AuthWebhook or any other component -- it is rebuilt from a fresh `List` + continuous `Watch` against etcd on every DS startup, so it is always internally consistent with etcd by construction. PostgreSQL is used exclusively for the audit trail (`audit_events`, ADR-034) and on-demand aggregate queries (e.g., success-rate metrics) computed from that trail -- it holds no independently mutable copy of catalog state.

The AuthWebhook no longer bridges to DS at all on the CRD admission path: content-hash/workflow-ID computation, ActionType-existence validation, and content-integrity/version-conflict checks all run locally in AW against its own etcd-backed client. `.status` is patched directly from that local decision. **There is no reconciliation controller because there is nothing to reconcile** -- a single source of truth cannot diverge from itself. See DD-WORKFLOW-018 for the full architecture and rationale (this is the resolution of the original "if stores diverge, manual intervention is required" risk called out in v1.0 of this document).

---

## Acceptance Criteria

> **Note (2026-07-14)**: items 2, 7, 8 below describe the DS-bridge behavior being phased out by
> [DD-WORKFLOW-018](../architecture/decisions/DD-WORKFLOW-018-etcd-single-source-of-truth.md) (Issue #1661,
> Changes 5/6). They remain accurate for the currently deployed behavior; they will be updated to reflect AW's
> local etcd-native validation (no DS registration call) once that phase of the rollout ships.

1. `RemediationWorkflow` CRD can be created via `kubectl apply` in `kubernaut-system` namespace
2. CREATE triggers DS registration; CRD is rejected if DS registration fails
3. UPDATE triggers DS re-registration with content integrity enforcement (Issue #773)
4. UPDATE with same version + different content is rejected (409 content-integrity-violation)
5. UPDATE with version bump triggers cross-version supersession (new workflow ID)
6. Idempotent re-apply (same version + same content) succeeds without DB writes
7. DELETE triggers DS disable (best-effort); CRD deletion always succeeds
8. `.status` reflects DS registration state (workflowId, catalogStatus, registeredBy, registeredAt)
9. Audit events emitted for CREATE (admitted/denied), UPDATE (admitted/denied), and DELETE (admitted)
10. Authenticated user captured from Kubernetes admission context (SOC2 CC8.1)
11. RBAC enforced via standard Kubernetes mechanisms
12. Existing workflows can be re-enabled by re-applying a previously deleted CRD (`.status.previouslyExisted: true`)

---

## References

- [ADR-058](../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) -- Architecture decision
- [BR-WORKFLOW-004](./BR-WORKFLOW-004-workflow-schema-format.md) -- Schema format
- [DD-WORKFLOW-012](../architecture/decisions/DD-WORKFLOW-012-workflow-immutability-constraints.md) -- Immutability
- [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) -- Lifecycle
- [DD-WORKFLOW-018](../architecture/decisions/DD-WORKFLOW-018-etcd-single-source-of-truth.md) -- Storage architecture (etcd single source of truth)
- [Test Plan](../testing/299/TEST_PLAN.md) -- Phase 2 test plan

---

**Document Status**: Active
**Version**: 1.2
