# ADR-058: Webhook-Driven Workflow Registration

**Date**: March 4, 2026
**Status**: Approved
**Version**: 1.0
**Authority**: AUTHORITATIVE for RemediationWorkflow CRD admission and DS bridge architecture
**Related**: DD-WEBHOOK-001 (CRD Webhook Matrix), DD-WEBHOOK-003 (Audit Pattern), DD-WORKFLOW-017 (Lifecycle), BR-WORKFLOW-006 (CRD Spec), ADR-051 (Webhook Scaffolding)
**Supersedes**: DD-WORKFLOW-005 (OCI Schema Extraction), DD-WORKFLOW-007 (Manual Registration) for the registration path
**GitHub Issue**: [#299](https://github.com/jordigilh/kubernaut/issues/299)

---

## Changelog

### Version 1.0 (2026-03-04)

- Initial ADR documenting the webhook-driven workflow registration architecture
- ValidatingWebhook chosen over MutatingWebhook for RemediationWorkflow CRD
- Async `.status` update via `client.Status().Update()` goroutine
- DS bridge: AW forwards inline CRD content to DS REST API (internal only)

---

## Context

### Problem Statement

Kubernaut needs a Kubernetes-native mechanism for workflow schema registration. The previous architecture used a user-facing REST API (`POST /api/v1/workflows`) with OCI image pullspecs, requiring operators to interact directly with the Data Storage service. This creates several problems:

1. **Not GitOps-compatible**: REST API calls cannot be declaratively managed via Git repositories
2. **No Kubernetes-native lifecycle**: Workflow registration is disconnected from the cluster's declarative state
3. **Operator friction**: Requires knowledge of internal DS API endpoints and authentication
4. **No reconciliation**: If DS state diverges from intended state, there is no automatic correction path

### Decision Drivers

- **GitOps alignment**: Workflow registration must work with `kubectl apply` and Flux/ArgoCD
- **CRD-native**: Leverage Kubernetes API for access control, audit, and lifecycle management
- **SOC2 CC8.1**: Capture authenticated user identity for all workflow registration/deletion actions
- **Minimal complexity**: Avoid introducing a full operator/controller for V1.0

---

## Decision

**RemediationWorkflow CRD lifecycle events (CREATE/DELETE) are intercepted by a ValidatingWebhookConfiguration in the AuthWebhook service, which bridges CRD operations to the Data Storage catalog via its internal REST API.**

### Key Architectural Choices

#### 1. ValidatingWebhook, Not MutatingWebhook

A **ValidatingWebhookConfiguration** is used rather than a MutatingWebhookConfiguration because:

- The CRD uses `+kubebuilder:subresource:status`, which means `.status` updates require a separate API call to the `/status` subresource -- they cannot be set via JSON patches in a mutating admission response
- The webhook's role is to **gate** admission (allow/deny based on DS registration success), not to mutate the object
- Simpler mental model: the webhook either allows or denies; `.status` is updated asynchronously after admission

#### 2. Asynchronous Status Update via Goroutine

After a successful CREATE admission, the webhook spawns a goroutine that:

1. Creates a fresh `context.Context` with a 10-second timeout (the admission context is cancelled after the response)
2. Fetches the newly-created CRD via `k8sClient.Get()`
3. Populates `.status` fields (`workflowId`, `catalogStatus`, `registeredBy`, `registeredAt`, `previouslyExisted`)
4. Calls `k8sClient.Status().Update()` to persist the status

**Trade-off**: This introduces eventual consistency -- there is a brief window where the CRD exists but `.status` is empty. This is acceptable because:

- Status is informational, not load-bearing for the registration flow
- The DS catalog is the source of truth for workflow state
- Operators can check `.status.catalogStatus` to confirm registration
- If the goroutine fails, the workflow IS registered in DS (admission succeeded); only the CRD status reflection is missing

#### 3. DS REST API as Internal-Only Bridge

The Data Storage `POST /api/v1/workflows` endpoint with inline `content` is an **internal API** consumed exclusively by the AuthWebhook. It is NOT a user-facing management endpoint.

- **User-facing interface**: `kubectl apply -f remediationworkflow.yaml` (CRD)
- **Internal interface**: AW calls `POST /api/v1/workflows` with JSON-marshaled CRD spec as `content`
- The `source` field is set to `"crd"` to distinguish from any future registration paths
- `registeredBy` is populated from the authenticated Kubernetes user identity

---

## Architecture

### Component Interaction

```
Operator                  K8s API Server              AuthWebhook (AW)           Data Storage (DS)
   |                           |                           |                          |
   |-- kubectl apply RW CR --->|                           |                          |
   |                           |-- AdmissionReview ------->|                          |
   |                           |   (CREATE)                |                          |
   |                           |                           |-- POST /api/v1/workflows |
   |                           |                           |   {content, source, by}   |
   |                           |                           |<-- 201 {workflowId} ------|
   |                           |                           |                          |
   |                           |<-- Allowed ---------------|                          |
   |                           |                           |                          |
   |                           |                           |-- (goroutine) ---------->|
   |                           |                           |   GET RW, Status.Update()|
   |<-- CR created ------------|                           |                          |
   |                           |                           |                          |
   |-- kubectl delete RW CR -->|                           |                          |
   |                           |-- AdmissionReview ------->|                          |
   |                           |   (DELETE)                |                          |
   |                           |                           |-- PATCH /disable -------->|
   |                           |                           |   (best-effort)           |
   |                           |<-- Allowed ---------------|                          |
   |<-- CR deleted ------------|                           |                          |
```

### Webhook Configuration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: authwebhook-validating
webhooks:
  - name: remediationworkflow.validate.kubernaut.ai
    rules:
      - apiGroups: ["kubernaut.ai"]
        apiVersions: ["v1alpha1"]
        operations: ["CREATE", "DELETE"]
        resources: ["remediationworkflows"]
    failurePolicy: Fail
    sideEffects: NoneOnDryRun
    timeoutSeconds: 15
    namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: kubernaut-system
```

Key configuration choices:

- **`failurePolicy: Fail`**: If the webhook is unavailable, CRD creation is blocked. This prevents unregistered workflows from existing in the cluster.
- **`sideEffects: NoneOnDryRun`**: The webhook has side effects (DS registration, audit events) on non-dry-run requests, but none on dry-run.
- **`timeoutSeconds: 15`**: Allows for DS network latency. DS registration includes schema parsing and database insertion.
- **`namespaceSelector`**: Restricts to `kubernaut-system` namespace only.

### Handler Implementation

The `RemediationWorkflowHandler` in `pkg/authwebhook/remediationworkflow_handler.go`:

- **CREATE flow**: Unmarshal CRD -> Extract authenticated user -> Marshal CRD as JSON content -> Call `dsClient.CreateWorkflowInline()` -> Emit audit event -> Spawn async status update -> Return `Allowed`
- **DELETE flow**: Unmarshal old CRD -> Extract `status.workflowId` -> Call `dsClient.DisableWorkflow()` (best-effort) -> Emit audit event -> Return `Allowed` (always, to prevent GitOps drift)
- **UPDATE**: Passes through without DS interaction (CRD spec is immutable per DD-WORKFLOW-012)

---

## Alternatives Considered

| Approach | Pros | Cons | Decision |
|----------|------|------|----------|
| **A. ValidatingWebhook + async status** | Simple, compatible with `+kubebuilder:subresource:status`, clear allow/deny semantics | Eventual consistency for status | **Selected** |
| **B. MutatingWebhook + JSON patch** | Synchronous status update | Incompatible with status subresource; patches applied before object creation | Rejected |
| **C. Full CRD controller (operator)** | Standard operator pattern, reconciliation loop | Over-engineered for V1.0; introduces controller complexity and potential drift between CRD spec and DS state | Rejected (deferred to V1.1+) |
| **D. Keep REST API as user-facing** | No webhook needed | Not GitOps-compatible, not Kubernetes-native | Rejected |

**Confidence**: 90%

---

## Consequences

### Positive

- **GitOps-compatible**: `RemediationWorkflow` CRDs can be managed declaratively via Git
- **Kubernetes-native**: Uses standard `kubectl` workflow, no custom CLI or API knowledge needed
- **SOC2 compliant**: Webhook captures authenticated user from `req.UserInfo` for audit trail
- **Consistent with existing handlers**: Follows the same `pkg/authwebhook` patterns as WorkflowExecution, RemediationApprovalRequest, and NotificationRequest handlers

### Negative

- **Eventual consistency**: Brief window where CRD `.status` is empty after creation
- **Goroutine failure**: If status update fails, CRD status does not reflect DS state (workflow IS registered in DS regardless)
- **Webhook availability**: If AuthWebhook is down, CRD creation is blocked (`failurePolicy: Fail`)
- **No reconciliation**: If DS state and CRD state diverge, there is no automatic correction (deferred to V1.1 controller)

### Risks

| Risk | Mitigation | Severity |
|------|------------|----------|
| Goroutine status update fails | Workflow registered in DS (source of truth); status is informational only | Low |
| AW unavailable blocks CRD creation | Standard K8s HA patterns (replicas, PDB); same risk as other webhook handlers | Medium |
| DS unreachable during admission | Webhook returns `Denied` with clear error message; operator retries | Medium |
| Namespace drift (CRD in wrong namespace) | `namespaceSelector` restricts to `kubernaut-system` | Low |

---

## RBAC Requirements

The AuthWebhook service account requires:

```yaml
# Read RemediationWorkflow CRDs (for status update GET)
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationworkflows"]
  verbs: ["get", "list", "watch"]

# Update RemediationWorkflow status subresource
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationworkflows/status"]
  verbs: ["update", "patch"]
```

---

## Audit Events

Per DD-WEBHOOK-003, the handler emits complete audit events:

| Event Type | Trigger | Outcome |
|------------|---------|---------|
| `remediationworkflow.admitted.create` | CREATE admitted | success |
| `remediationworkflow.admitted.delete` | DELETE admitted | success |
| `remediationworkflow.admitted.denied` | CREATE denied (DS error, auth failure, unmarshal error) | failure |

All events include: `event_category: "webhook"`, authenticated user, resource name, correlation ID (admission UID), and namespace.

---

## Implementation Files

| File | Purpose |
|------|---------|
| `pkg/authwebhook/remediationworkflow_handler.go` | Handler: `Handle()`, `handleCreate()`, `handleDelete()`, `updateCRDStatus()` |
| `pkg/authwebhook/remediationworkflow_audit.go` | Audit helpers: `emitAdmitAudit()`, `emitDeniedAudit()` |
| `pkg/authwebhook/ds_client.go` | `DSClientAdapter`: wraps ogen client for `CreateWorkflowInline()`, `DisableWorkflow()` |
| `pkg/authwebhook/types.go` | Event type constants: `EventTypeRWAdmittedCreate`, etc. |
| `api/remediationworkflow/v1alpha1/remediationworkflow_types.go` | CRD Go types: `RemediationWorkflowSpec`, `RemediationWorkflowStatus` |
| `cmd/authwebhook/main.go` | Wiring: registers handler at `/validate-remediationworkflow` |
| `charts/kubernaut/templates/authwebhook/webhooks.yaml` | `ValidatingWebhookConfiguration` for CREATE/DELETE |
| `charts/kubernaut/templates/authwebhook/authwebhook.yaml` | ClusterRole with RW and RW/status permissions |

---

## References

- [DD-WEBHOOK-001](./DD-WEBHOOK-001-crd-webhook-requirements-matrix.md) - CRD Webhook Requirements Matrix
- [DD-WEBHOOK-003](./DD-WEBHOOK-003-webhook-complete-audit-pattern.md) - Webhook-Complete Audit Pattern
- [DD-WORKFLOW-017](./DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) - Workflow Lifecycle
- [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md) - RemediationWorkflow CRD Specification
- [ADR-051](./ADR-051-operator-sdk-webhook-scaffolding.md) - Webhook Scaffolding Pattern
- [ADR-034](./ADR-034-unified-audit-table-design.md) - Unified Audit Table Design

---

**Document Status**: Approved
**Version**: 1.0
**Next Review**: 2026-09-04 (6 months)
