# BR-WORKFLOW-008: WorkflowExecution Runtime Dependency-Failure Observability

**Business Requirement ID**: BR-WORKFLOW-008
**Category**: Workflow Execution Service — Reliability & Observability
**Priority**: **P1 (HIGH)** — Closes a fail-open-safety gap introduced by Issue #1481
**Target Version**: **V1.5**
**Status**: ✅ Implemented
**Date**: July 3, 2026
**Related ADRs**: None
**Related BRs**: BR-PLATFORM-054 (Dependency Validator Removal), BR-WORKFLOW-004 (Workflow Schema Format), BR-WE-014 (Kubernetes Job Execution Backend), BR-ORCH-028 (RemediationOrchestrator Executing-Phase Timeout)
**Related Design Decisions**: DD-WE-006 (Schema-Declared Infrastructure Dependencies — partially superseded)
**GitHub Issues**: [#1481](https://github.com/jordigilh/kubernaut/issues/1481)

---

## Business Need

### Problem Statement

Issue #1481 removed the `K8sDependencyValidator` pre-flight check that Data Storage and the WorkflowExecution (WE) controller previously ran against declared `dependencies.secrets` / `dependencies.configMaps` before a workflow could be registered or executed. The pre-flight check was removed because it duplicated — and could drift out of sync with — Kubernetes' own native enforcement: a Job's Pod simply cannot start if a referenced Secret or ConfigMap volume does not exist.

Removing the pre-flight check without a replacement created a fail-open-safety gap: `JobExecutor.buildJob()` (`pkg/workflowexecution/executor/job.go`) did not set `batchv1.JobSpec.ActiveDeadlineSeconds`, and `reconcileRunning` (`internal/controller/workflowexecution/workflowexecution_lifecycle.go`) simply requeues every 10 seconds while a Job is active. A Pod that cannot mount a missing Secret/ConfigMap sits in `Pending` indefinitely — the Job never reaches a terminal `.status.conditions` entry, so the WFE never transitions out of `Running`.

### Impact Without This BR

- **Detection latency measured in tens of minutes, not seconds.** `RemediationOrchestrator`'s existing 30-minute `Executing`-phase safety net (BR-ORCH-028, `internal/controller/remediationorchestrator/timeout_management.go`) eventually marks the parent `RemediationRequest` `TimedOut`, so the system does not hang *forever* — but detection can take up to 30 minutes, far too slow for an operator debugging a live remediation.
- **Generic, unhelpful failure messages.** The RO timeout message (`"Phase Executing exceeded timeout of 30m0s"`) gives no indication that the root cause is a missing Secret or ConfigMap, forcing an operator to manually inspect the Job's Pods and Events.
- **Resource leakage.** The child WFE and its Job are not proactively cleaned up on this path; they linger until the RemediationRequest's retention TTL cascades deletion.
- **SOC2 CC7.2 / CC8.1 gap**: incomplete audit/observability trail for a class of execution failure that is expected to become more common now that dependency existence is no longer checked before a Job is created.

---

## Decision: Job-Level Deadline + Pod-Event Enrichment + Event Message Enrichment

This BR is a superset of Issue #1481's own "Add" checklist items ("WE reconciler logic to extract failure reasons from Job `.status.conditions`", "Updated WFE status error messages") — formalizing it as its own BR makes the fail-open-safety guarantee explicit and independently testable.

### 1. `ActiveDeadlineSeconds` on the Job

`JobExecutor.buildJob()` sets `batchv1.JobSpec.ActiveDeadlineSeconds`, sourced from `wfe.Spec.ExecutionConfig.Timeout` when declared, falling back to a 30-minute default (`defaultJobActiveDeadline`) matching the semantics already used for Tekton executions and RemediationOrchestrator's Executing-phase safety net. This bounds how long a Pod can remain unable to start before the Job reaches a terminal `JobFailed` condition (`DeadlineExceeded` reason), instead of hanging in `Pending` forever.

### 2. Pod-Event Message Enrichment

When `JobExecutor.GetStatus()` observes a `JobFailed` condition, it inspects the Job's Pods (`ExecutorClient.List` with `job-name` label selector) for a `FailedMount` or `CreateContainerConfigError` Event (the kubelet Event reasons that indicate a missing/misconfigured Secret or ConfigMap dependency) and, if found, uses that Event's message in place of the generic Job condition message — e.g. `MountVolume.SetUp failed for volume "secret-my-creds" : secret "my-creds" not found` instead of `Job was active longer than specified deadline`. This is best-effort enrichment: any list error, absent Pods, or absent matching Event falls back to the original condition message rather than failing status reporting.

The enriched message is propagated into both `ExecutionResult.Message` and `ExecutionResult.Summary.Message`, since `WorkflowExecutionReconciler.MarkFailed` persists `WorkflowExecution.Status.FailureDetails.Message` from the executor's `Summary`, not the top-level result message.

### 3. Enriched `WorkflowFailed` K8s Event

`MarkFailed` (`internal/controller/workflowexecution/workflowexecution_status_marking.go`) includes `FailureDetails.Message` alongside `FailureDetails.Reason` in the emitted `WorkflowFailed` Kubernetes Event, so the specific missing-dependency detail is visible via `kubectl get events` / `kubectl describe wfe`, not only in the audit trace or WFE status YAML. The sibling `MarkFailedWithReason` path already interpolated both reason and message; only the `MarkFailed` terminal-execution path required this change.

### Multi-Cluster Coverage

`JobExecutor.buildJob()` / `GetStatus()` is the single shared code path for both local and remote (MCP-backed multi-cluster) execution via the `ClientFactory` abstraction (`j.factory.ClientFor(ctx, wfe.Spec.ClusterID)`). The `ExecutorClient` interface was extended with a `List` method (delegating to the existing `mcpclient.Client.List` for remote clusters), so this fix automatically covers both local and MCP-federated clusters without additional scope.

---

## Success Criteria

1. A WFE whose Job cannot mount a missing Secret/ConfigMap reaches `Failed` within the configured `ActiveDeadlineSeconds` (default 30m) instead of hanging indefinitely.
2. `WorkflowExecution.Status.FailureDetails.Message` names the specific missing resource (e.g. contains the Secret/ConfigMap name) when a `FailedMount`/`CreateContainerConfigError` Pod Event is present.
3. The emitted `WorkflowFailed` Kubernetes Event includes both the failure reason and the enriched message.
4. Unit tests cover `buildJob()` `ActiveDeadlineSeconds` wiring (default and `ExecutionConfig.Timeout`-sourced) and `GetStatus()` Pod-event message enrichment (match found, and fallback when no match).
5. Integration test (`IT-WE-1481-001`, envtest) proves the full pipeline: Job created despite a missing Secret dependency, `ActiveDeadlineSeconds` set, and — after simulating the kubelet-driven `FailedMount` Event + terminal `JobFailed` condition envtest cannot produce natively — the WFE reaches `Failed` with the enriched message.
6. Zero regression in RemediationOrchestrator's existing 30-minute Executing-phase safety net (BR-ORCH-027/028): it remains the outer backstop if this BR's mechanism is ever bypassed (e.g. a non-Job/Tekton executor).

---

## Related Documents

- [DD-WE-006: Schema-Declared Infrastructure Dependencies (partially superseded)](../architecture/decisions/DD-WE-006-schema-declared-dependencies.md)
- [BR-WE-014: Kubernetes Job Execution Backend](./BR-WE-014-kubernetes-job-execution-backend.md)
- [docs/testing/DD-WE-006/TEST_PLAN.md](../testing/DD-WE-006/TEST_PLAN.md) (historical — pre-flight validation scenarios superseded)
- [docs/tests/659/TEST_PLAN.md](../tests/659/TEST_PLAN.md) (historical — `DependencyValidator` scenario superseded)

---

**Document Version**: 1.0
**Last Updated**: July 3, 2026
**Maintained By**: Kubernaut Architecture Team
