# BR-FLEET-004: Workflow-Declared Execution Cluster

**Document Version**: 1.0
**Date**: August 31, 2026
**Status**: ✅ Implemented
**Category**: Fleet Management / Workflow Execution Routing
**Priority**: P2 (Medium)
**Service**: KubernautAgent, AIAnalysis, RemediationOrchestrator, AuthWebhook
**GitHub Issue**: [#2326](https://github.com/jordigilh/kubernaut/issues/2326)
**Related**: DD-FLEET-008, DD-FLEET-002, DD-FLEET-007, DD-WORKFLOW-018, DD-WORKFLOW-019, ADR-068

---

## Business Context

### Problem Statement

`RemediationRequest.Spec.ClusterID` (the signal's origin cluster) is propagated verbatim
onto `WorkflowExecution.Spec.ClusterID`, which determines which cluster actually runs the
selected workflow's Job/PipelineRun. This assumes the signal's origin cluster and the
execution cluster are always the same, which breaks down for two real fleet topologies:

1. **GitOps-hub remediation**: the fix is a Git commit that a centralized ArgoCD/Flux
   instance reconciles onto the target cluster. The Job performing the commit needs to run
   wherever that GitOps control plane's tooling/credentials live, not on the cluster where
   the incident occurred.
2. **Edge/aggregator remediation**: a resource-constrained edge device cannot run a Job
   itself; a separate, capable aggregator cluster with network reach to that device must
   perform the remediation on its behalf.

### Business Value

1. **Enables GitOps-hub and edge/aggregator remediation patterns** using Kubernaut's
   existing Job/Tekton/Ansible execution engines and fleet MCP Gateway routing — no new
   engine types or gateway infrastructure required.
2. **Author-controlled, not system-inferred**: execution location is declared statically
   on the workflow catalog entry, alongside the execution properties (engine, bundle,
   service account) that already live there — consistent with how workflow authors already
   reason about what a workflow needs to run.
3. **Zero regression risk**: fully backward compatible — an unset `execution.clusterId`
   preserves today's behavior (execution follows the signal's origin cluster) exactly.
4. **No new authorization surface**: reuses the identical fleet MCP Gateway trust boundary
   that `RemediationRequest.Spec.ClusterID`-based dispatch already relies on today.

---

## Requirements

### R1: Catalog-Declared Execution Cluster Field

`RemediationWorkflow.spec.execution` MUST support an optional `clusterId` field (camelCase,
matching the existing `serviceAccountName` sibling field's convention), declaring the fleet
cluster this workflow's execution resource runs on. The field MUST remain optional at the
schema level; omitting it MUST preserve today's behavior exactly (execution follows the
signal's origin cluster).

### R2: Catalog-Authoritative Propagation

The declared cluster MUST propagate through KA's existing catalog-authoritative chain —
mirroring how `ActionType`/`WorkflowName`/`ServiceAccountName` already flow — from the
`RemediationWorkflow` CRD through KA's own informer cache
(`internal/kubernautagent/workflowcatalog/cache_convert.go`, per
[DD-WORKFLOW-019](../architecture/decisions/DD-WORKFLOW-019-ka-owned-workflow-discovery.md):
no DataStorage involvement), into `WorkflowMeta`, `InvestigationResult`, and the shared
`WorkflowSnapshot` embedded in both `AIAnalysis.Status.SelectedWorkflow` and
`WorkflowExecution.Spec.WorkflowRef`. It MUST NOT be LLM-suppliable — the LLM selects
*which* workflow runs, never *where* it runs.

### R3: RO Creator Resolution with Fallback

RemediationOrchestrator's `WorkflowExecutionCreator` MUST resolve
`WorkflowExecution.Spec.ClusterID` as: the selected workflow's declared
`ExecutionClusterID` when set, otherwise `RemediationRequest.Spec.ClusterID` (today's
default). This resolution MUST be unconditional across all execution engines.

### R4: Ansible Fail-Closed Regression Preserved

A workflow declaring `execution.clusterId` on the `ansible` engine MUST hit the existing
fail-closed guard from
[DD-FLEET-007](../architecture/decisions/DD-FLEET-007-ansible-engine-not-supported-for-remote-execution.md)
unchanged — no special-casing for the new field's origin (workflow-declared vs.
RR-derived).

### R5: No New Authorization Surface

Dispatch to a workflow-declared execution cluster MUST go through the identical
`ClientFactory.ClientFor` → fleet MCP Gateway registration-check path that
RR-derived `ClusterID` dispatch already uses. An unregistered or unreachable
workflow-declared cluster MUST fail at dispatch time — the same fail-closed behavior as an
RR-supplied `ClusterID` typo today — not a new admission-time cluster-registry validation.

---

## Clarification: `execution.clusterId` vs `labels.cluster`

These are two independent, same-word-but-unrelated fields on the same CRD and MUST NOT be
confused:

| Field | Purpose | Introduced By |
|---|---|---|
| `spec.labels.cluster` | **Eligibility filter** — which signal-origin cluster classifications make this workflow discoverable | [DD-FLEET-002](../architecture/decisions/DD-FLEET-002-cluster-scoped-workflow-targeting.md) / BR-FLEET-003 |
| `spec.execution.clusterId` | **Dispatch target** — which cluster the selected workflow's Job/PipelineRun actually runs on | This BR / DD-FLEET-008 |

---

## Acceptance Criteria

- [x] `RemediationWorkflow.spec.execution.clusterId` is optional; empty maps to a nil
      `ExecutionClusterID` in KA's cache (UT-KA-2326-001/002)
- [x] `WorkflowMeta.ClusterID` copies `ExecutionClusterID` verbatim (UT-KA-2326-003/004)
- [x] `enrichFromCatalog` always overwrites `InvestigationResult.ExecutionClusterID` from
      the catalog, never from LLM input (UT-KA-2326-005..008)
- [x] All three `SelectedWorkflow` population call sites in AA's response processor
      (`storeSelectedWorkflow`, `preserveLowConfidenceWorkflow`,
      `preservePartialSelectedWorkflow`) extract `ExecutionClusterID` correctly, including
      absence (UT-AA-2326-001..004)
- [x] RO creator prefers the workflow-declared cluster over `RemediationRequest.Spec.ClusterID`
      when set (UT-RO-2326-001)
- [x] RO creator falls back to `RemediationRequest.Spec.ClusterID` when the workflow
      declares no execution cluster — regression guard for unchanged default behavior
      (UT-RO-2326-002)
- [x] RO creator leaves `ClusterID` empty when neither the RR nor the workflow declare one
      (UT-RO-2326-003)
- [x] The Ansible engine's fail-closed guard (DD-FLEET-007) applies unchanged to a
      workflow-declared execution cluster (UT-RO-2326-004)
- [x] Full real-infrastructure chain: AuthWebhook admission → KA catalog cache →
      mock-llm-driven selection → AIAnalysis → RO creator produces a
      `WorkflowExecution.Spec.ClusterID` that follows the workflow's declared execution
      cluster rather than the signal's origin cluster, against a genuinely separate remote
      Kind cluster (E2E-FLEET-2326-001)

---

## Implementation Points

| Component | File(s) | Change |
|---|---|---|
| CRD schema | `api/remediationworkflow/v1alpha1/remediationworkflow_types.go` | Add optional `ClusterID` to `RemediationWorkflowExecution` |
| KA in-memory DTO | `pkg/datastorage/models/workflow.go` | Add `ExecutionClusterID *string` (no DB/migration — DD-WORKFLOW-019) |
| KA cache conversion | `internal/kubernautagent/workflowcatalog/cache_convert.go` | Map `spec.execution.clusterId` into the DTO |
| KA parser | `internal/kubernautagent/parser/validator.go`, `cmd/kubernautagent/toolregistry.go` | `WorkflowMeta.ClusterID`, `buildWorkflowMeta` mapping |
| KA investigator | `internal/kubernautagent/investigator/investigator_gates.go`, `pkg/kubernautagent/types/types.go` | `InvestigationResult.ExecutionClusterID` (catalog-authoritative overwrite) |
| Shared snapshot | `pkg/shared/types/workflow_snapshot.go` | `WorkflowSnapshot.ExecutionClusterID` |
| AA response mapping | `pkg/aianalysis/handlers/response_processor.go` | Extract `execution_cluster_id` at all 3 call sites |
| RO creator | `pkg/remediationorchestrator/creator/workflowexecution.go` | `resolveExecutionClusterID` helper |
| WFE doc comment | `api/workflowexecution/v1alpha1/workflowexecution_types.go` | Document new resolution precedence |
| E2E fixture | `test/fixtures/workflows/fleet-exec-cluster-override/workflow-schema.yaml` | Isolated workflow declaring `execution.clusterId: prod-east` |
| E2E mock-llm scenario | `test/services/mock-llm/scenarios/scenario_fleet_exec_cluster_override.go`, `registry_default.go` | Dedicated signal-matched scenario selecting the fixture |
| E2E test | `test/e2e/fleet/22_execution_cluster_override_test.go` | E2E-FLEET-2326-001 |

---

## Test Plan

Unit and integration-equivalent coverage lives alongside each wiring point (see
Implementation Points above); no separate formal test-plan document was created for this
BR given its narrow, single-field scope. Full pyramid: Unit (`UT-KA-2326-*`,
`UT-AA-2326-*`, `UT-RO-2326-*`) + E2E (`E2E-FLEET-2326-001`) — see
[DD-FLEET-008's Test Coverage section](../architecture/decisions/DD-FLEET-008-workflow-declared-execution-cluster.md#test-coverage)
for the full list.

---

## Security & Compliance

See [DD-FLEET-008's Security & Compliance Mapping](../architecture/decisions/DD-FLEET-008-workflow-declared-execution-cluster.md#security--compliance-mapping)
for the full FedRAMP AC-4/AC-6 and OWASP ASVS V4.1.1/V4.1.3 control mapping.

---

## References

- [DD-FLEET-008: Workflow-Declared Execution Cluster](../architecture/decisions/DD-FLEET-008-workflow-declared-execution-cluster.md)
- [DD-FLEET-002: Cluster-Scoped Workflow Targeting](../architecture/decisions/DD-FLEET-002-cluster-scoped-workflow-targeting.md) / BR-FLEET-003
- [DD-FLEET-007: Ansible Engine Not Supported for Remote Execution](../architecture/decisions/DD-FLEET-007-ansible-engine-not-supported-for-remote-execution.md)
- [DD-WORKFLOW-019: KA-Owned Workflow Discovery](../architecture/decisions/DD-WORKFLOW-019-ka-owned-workflow-discovery.md)
- [Issue #2326](https://github.com/jordigilh/kubernaut/issues/2326)
