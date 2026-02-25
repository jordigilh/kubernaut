# ADR-057: CRD Namespace Consolidation

**Date**: 2026-02-24
**Status**: ACCEPTED
**Issue**: [#176](https://github.com/jordigilh/kubernaut/issues/176)
**Supersedes**: BR-GATEWAY-020 (CRD Namespace Handling), DD-GATEWAY-007 (Fallback Namespace Strategy)

## Context

Kubernaut CRDs (RemediationRequest, SignalProcessing, AIAnalysis, WorkflowExecution,
EffectivenessAssessment, NotificationRequest, RemediationApprovalRequest) were previously
created in the signal/workload namespace — the namespace where the target resource resides.

This design introduced two problems:

1. **Finalizer-blocked namespace deletion**: Deleting a workload namespace blocks on
   kubernaut CRD finalizers. The kubernaut controllers must process and remove finalizers
   before the namespace can terminate. If a controller is down or slow, the namespace
   hangs in `Terminating` state indefinitely.

2. **Unauthorized CRD creation**: Any user with CRD create permission in a managed
   namespace could create fake downstream CRDs (e.g., AIAnalysis, WorkflowExecution)
   to trigger expensive operations — LLM inference via HolmesGPT, Tekton pipeline
   execution — without going through the legitimate Gateway → RO pipeline.

## Decision

**All kubernaut CRDs are created in the controller namespace** (the namespace where
the kubernaut controllers are deployed, typically `kubernaut-system`).

Security is enforced through three layers:

1. **CRD creation**: Gateway creates RemediationRequest in the controller namespace.
   RO creates all downstream CRDs in the same namespace (they use `rr.Namespace`,
   which is now the controller namespace).

2. **CRD watch restriction**: Each controller's informer cache is configured via
   `Cache.ByObject` to watch kubernaut CRD types only in the controller namespace.
   A CRD created in any other namespace is invisible to the controllers.

3. **RBAC**: Only kubernaut service accounts have write access to CRDs in the
   controller namespace. Workload namespace users cannot create kubernaut CRDs
   in the controller namespace.

The signal/target resource namespace is preserved in `RR.Spec.TargetResource.Namespace`
and remains available to all controllers for cross-namespace workload reads
(enrichment, health checks, scope validation, spec hashing, etc.).

### Controller namespace discovery

Each controller discovers its own namespace at startup by reading the Kubernetes
service account namespace file:

```
/var/run/secrets/kubernetes.io/serviceaccount/namespace
```

This is the standard Kubernetes mechanism, available in every in-cluster pod.
The namespace is never hardcoded.

### Watch configuration

Controllers use `Cache.ByObject` (controller-runtime v0.22+) to restrict kubernaut
CRD watches to the controller namespace while keeping workload resource reads
cluster-wide:

| Controller | CRD types (controller NS only) | Other types (cluster-wide or specific NS) |
|---|---|---|
| SignalProcessing | SignalProcessing | Pods, Deployments, Nodes (K8sEnricher) |
| AIAnalysis | AIAnalysis | ConfigMaps (mounted policy file) |
| WorkflowExecution | WorkflowExecution | PipelineRun, Job, TaskRun (kubernaut-workflows) |
| EffectivenessMonitor | EffectivenessAssessment | Pods, Deployments (health, spec hash) |
| NotificationRequest | NotificationRequest | ConfigMaps (kubernaut-notifications) |
| RemediationOrchestrator | All 7 CRD types | Workload resources via APIReader |
| Gateway | RemediationRequest (dedup cache) | Namespace, Pod (scope validation), Lease |

## Consequences

### Positive

- Workload namespace lifecycle is fully decoupled from kubernaut CRD lifecycle
- RBAC provides a strong security boundary without per-controller scope validation logic
- Controllers cannot be tricked into processing fake CRDs in workload namespaces
- Simplified operational model: all kubernaut CRDs are in one namespace

### Negative

- Cross-namespace scope validation must use the target resource namespace from the
  CRD spec (`TargetResource.Namespace`), not the CRD's own namespace
- Existing tests that assert CRD namespace equals signal namespace require updating

### Neutral

- Non-CRD workload reads remain cluster-wide (unchanged behavior)
- RO pre-remediation hash capture continues to use uncached APIReader (DD-EM-002)
- Gateway scope validation continues to check target resources in workload namespaces

## Bug Fix: RO Scope Validation Namespace

During investigation, a latent bug was discovered in `CheckUnmanagedResource`
(`pkg/remediationorchestrator/routing/blocking.go`): it passes `rr.Namespace`
(the CRD namespace) instead of `rr.Spec.TargetResource.Namespace` (the actual
target namespace) to `IsManaged()`. With CRDs in the controller namespace,
this bug would cause every scope check to evaluate `kubernaut-system` instead
of the target resource's namespace. This is fixed as part of this ADR.

## References

- BR-SCOPE-001: Resource Scope Management
- ADR-053: Resource Scope Management Architecture
- DD-EM-002: Canonical Spec Hash
- 005: Owner Reference Architecture (flat sibling hierarchy, all CRDs owned by RR)
