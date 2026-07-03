# BR-FLEET-003: Cluster-Scoped Workflow Targeting via SP Rego Classification

**Document Version**: 1.1
**Date**: July 3, 2026
**Status**: ✅ Implemented
**Category**: Fleet Management / Workflow Discovery
**Priority**: P1 (High)
**Service**: SignalProcessing, RemediationOrchestrator, AIAnalysis, KubernautAgent, DataStorage
**GitHub Issue**: [#1511](https://github.com/jordigilh/kubernaut/issues/1511)
**Related**: DD-FLEET-002, DD-FLEET-001, ADR-068, BR-FLEET-002, BR-FLEET-054, BR-WORKFLOW-001, BR-WORKFLOW-006

---

## Business Context

### Problem Statement

With multi-cluster federation (issue #54), `RemediationWorkflow`s have no mechanism to
restrict which clusters they are eligible to execute on. Today, `RemediationWorkflowLabels`
supports filtering by `severity`, `environment`, `component`, and `priority`, but not by
cluster. Once fleet mode is enabled, every workflow in the catalog becomes eligible for
selection regardless of which cluster the triggering signal originated from — including
workflows an operator intended to be local/single-cluster-only.

Cluster names are frequently cryptic (e.g. `k8s-fin-eu-03p`) and operator-uncontrolled,
so raw name matching is not a viable targeting mechanism. Operators need a
semantically-controlled way to classify clusters (e.g. "production", "staging-eu") and
restrict workflow eligibility by that classification.

### Business Value

1. **Safety**: Prevents the LLM from selecting a workflow authored for one cluster context
   (e.g. a local/single-cluster remediation) for execution against a mismatched or
   unintended cluster in a fleet deployment.
2. **Operator control**: Cluster classification taxonomy is defined entirely by
   operator-authored Rego policy — no hardcoded naming conventions, no DS coupling to the
   fleet registry.
3. **Backward compatibility**: Non-fleet (single-cluster) deployments are entirely
   unaffected — the `cluster` dimension is never evaluated unless fleet mode is active and
   a concrete classification exists.
4. **Architectural consistency**: Extends the existing SP → Rego → business-value pattern
   already used for `severity`/`environment`/`priority`, requiring no new query mechanisms
   in DataStorage.

---

## Requirements

### R1: Cluster Label Resolution in SignalProcessing

SP MUST resolve the signal's cluster labels via the existing `ClusterRegistry`
(`pkg/fleet/registry`), keyed by the signal's `ClusterID` (the raw, alert-source-supplied
identifier — see [Clarification](#clarification-clusterid-vs-clusterclassification)
below). When the `ClusterID` has no fleet registration (non-fleet signal, or a cluster not
yet synced by FMC), SP MUST degrade gracefully (no classification produced) rather than
failing the reconcile.

### R2: Cluster Classification via Rego Policy

SP MUST feed the resolved cluster labels into the unified Rego policy engine
(`input.cluster.labels`), alongside the existing namespace/workload context. The policy
MAY output a `cluster` business classification (e.g. `"production"`). This evaluation
MUST be non-fatal: unlike severity (a correctness gate), a missing or failed cluster
classification MUST NOT transition the `SignalProcessing` resource to `PhaseFailed` — it
is an optional targeting dimension, not a correctness gate.

### R3: Status Persistence

The resolved classification MUST be persisted to `SignalProcessing.Status.ClusterClassification`,
using the same atomic status-update pattern already used for
`EnvironmentClassification`/`PriorityAssignment`/`Severity`.

### R4: End-to-End Propagation

`ClusterClassification` MUST propagate through every hop between SP and DataStorage
discovery, mirroring how `severity`/`environment`/`component` already flow:

```
SignalProcessing.Status.ClusterClassification
  -> RemediationOrchestrator: buildSignalContext()
  -> AIAnalysis.Spec.SignalContext.Cluster
  -> KubernautAgent: SignalContext.ClusterClassification
  -> KubernautAgent discovery tool-call parameters
  -> DataStorage discovery filter (cluster dimension)
```

None of these hops exist prior to this issue. Without this propagation, the DS filter
dimension introduced by R5 would be unreachable dead code.

### R5: DataStorage Filter Dimension

DataStorage MUST support an optional `cluster` filter dimension in workflow discovery,
using the identical JSONB query pattern already used for `severity`/`environment`
(`EXISTS(jsonb_array_elements_text(labels->'cluster')...) OR labels->'cluster' ? '*'`).
Storage MUST reuse the existing `labels JSONB` column — **no DB migration**.

### R6: Matching Semantics

Two independent conditions govern whether — and how — the `cluster` dimension applies:

1. **Fleet disabled, or fleet enabled but SP produced no `ClusterClassification`**: KA
   omits the `cluster` discovery parameter entirely; DS applies no cluster filter and
   behaves exactly as it did before this issue.
2. **Fleet enabled and SP produced a concrete `ClusterClassification`**: KA passes it to
   DS. DS then applies the same mandatory-field idiom used for
   severity/environment/priority: a workflow matches only if `labels->'cluster'` contains
   the classification value (case-insensitive) OR the literal wildcard `"*"`. A workflow
   with **no `cluster` entry at all is excluded** once a concrete filter value is
   supplied. Operators who want a workflow to remain discoverable across all cluster
   classifications must say so explicitly with `cluster: ["*"]`.

This is a **query-time behavior**, not a schema-level validation rule: `Cluster` remains
`+optional` with no `MinItems=1`, and no catalog backfill/migration is required.

### R7: Schema Optionality

The `Cluster []string` field on `RemediationWorkflowLabels` MUST remain optional at the
schema level in all deployment modes (fleet and non-fleet). It MUST NOT become a required
field, since single-cluster deployments never populate it.

---

## Clarification: ClusterID vs ClusterClassification

These are two independent fields with different sources and purposes and MUST NOT be
confused or merged:

| Field | Source | Purpose |
|---|---|---|
| `ClusterID` / `ClusterName` (existing) | Raw, alert-source-supplied identifier (e.g. federated Prometheus's `webhook.CommonLabels`), propagated onto `KubernetesContext.ClusterID` | **Lookup key** into `ClusterRegistry.Get(clusterID)` |
| `ClusterClassification` (new, this issue) | Rego-derived business value computed from `ClusterInfo.Labels`, which SP fetches from the MCP Gateway CRD via `ClusterRegistry` | **Filter value** used by DS to match against `RemediationWorkflowLabels.Cluster` |

`ClusterID` is never itself used as the DS filter value. Code and doc comments introduced
by this issue MUST reference `ClusterClassification` explicitly and must not alias or
repurpose `ClusterID`/`ClusterName`.

---

## Fleet Onboarding Prerequisite

`input.cluster.labels` (the Rego policy input for this feature) are the Kubernetes
`metadata.labels` on the MCP Gateway's own cluster-registration CRD — EAIGW's `Backend`
(`gateway.envoyproxy.io/v1alpha1`) or Kuadrant's `MCPServerRegistration`
(`mcp.kuadrant.io/v1alpha1`) — read via `ClusterInfo.Labels` in `pkg/fleet/registry`. They
describe the cluster **as a fleet member** (e.g. `environment: production`,
`tier: gold`, `region: us-east-1`), set by the fleet operator/administrator at
cluster-onboarding time. This is the same conceptual role that namespace/workload labels
play for `environment`/`severity`/`priority` classification today, one layer up
(cluster-level metadata instead of in-cluster resource metadata).

**Operational prerequisite**: for `cluster` classification to produce anything other than
"no classification," operators MUST apply meaningful labels to the Backend/
`MCPServerRegistration` CR when registering each cluster into the fleet. An unlabeled
cluster registration degrades gracefully to "no classification" (per R2) — the feature is
then inert for that cluster, not broken. This is an onboarding-time operational
requirement to document, not a runtime failure mode.

---

## Acceptance Criteria

- [x] `EvaluateCluster()` produces a classification when cluster labels are present and the
      Rego policy has a matching rule (UT-SP-1511-001)
- [x] `K8sEnricher` populates `KubernetesContext.Cluster` from `ClusterRegistry`, degrading
      gracefully (nil, no error) when the cluster is not registered (UT-SP-1511-002, IT-SP-1511-001)
- [x] A cluster-classification evaluation error does not transition `SignalProcessing` to
      `PhaseFailed` (non-fatal, unlike severity) (IT-SP-1511-002b)
- [x] `SignalProcessing.Status.ClusterClassification` is set end-to-end via a real Rego
      policy fixture through the production reconcile loop (IT-SP-1511-002a)
- [x] `ClusterClassification` propagates through `AIAnalysis.Spec.SignalContext.Cluster`
      and `katypes.SignalContext.ClusterClassification` into the KA discovery tool call
      (IT-RO-1511-001, UT-KA-1511-001, IT-KA-1511-001)
- [x] DS SQL generation includes the `cluster` dimension with exact + `*` wildcard fallback,
      mirroring severity/environment, and omits the condition entirely when the filter
      value is empty (UT-DS-1511-004..007, IT-DS-1511-001..003)
- [x] A workflow with no `cluster` entries is excluded once a concrete filter is active;
      `cluster: ["*"]` matches any concrete filter value (IT-DS-1511-001..003)
- [x] CRD `cluster` labels round-trip through the unmodified Authwebhook admission handler
      to DS storage with zero AW code changes (IT-AW-1511-001)
- [x] Full SP → RO → AA → KA → DS chain: workflow excluded for mismatched classification,
      included for match, included when fleet is disabled (E2E-FLEET-1511-001)
- [x] No DB migration introduced; no breaking changes to existing `RemediationWorkflow`
      CRDs or catalog entries (verified: `labels JSONB` column reused, `Cluster` remains
      `+optional`)

---

## Implementation Points

| Component | File(s) | Change |
|---|---|---|
| CRD schema | `api/remediationworkflow/v1alpha1/remediationworkflow_types.go` | Add optional `Cluster []string` to `RemediationWorkflowLabels` |
| Schema conversion | `pkg/workflowschema/converter.go`, `pkg/datastorage/models/workflow_schema.go`, `pkg/datastorage/models/workflow_labels.go`, `pkg/datastorage/schema/parser.go` | Add `Cluster` mapping/extraction |
| Shared fleet config | `pkg/fleet/config.go`, `pkg/fleet/fmc/config/config.go`, `pkg/signalprocessing/config/config.go` | Promote shared `MCPGatewayConfig`; extend SP's `FleetConfig` |
| SP startup wiring | `cmd/signalprocessing/main.go` | Construct `ClusterRegistry` |
| SP enrichment | `pkg/shared/types/enrichment.go`, `pkg/signalprocessing/enricher/k8s_enricher.go` | `ClusterContext` type; populate `KubernetesContext.Cluster` |
| SP Rego evaluation | `pkg/signalprocessing/evaluator/{types,evaluator}.go`, `internal/controller/signalprocessing/interfaces.go`, `internal/controller/signalprocessing/signalprocessing_classifying.go`, `api/signalprocessing/v1alpha1/signalprocessing_types.go` | `PolicyInput.Cluster`, `EvaluateCluster()`, `Status.ClusterClassification` |
| Rego policy example | `charts/kubernaut/examples/signalprocessing-policy.rego` | Example `cluster` rule group |
| RO propagation | `api/aianalysis/v1alpha1/aianalysis_types.go`, `pkg/remediationorchestrator/creator/aianalysis.go` | `SignalContextInput.Cluster` |
| KA propagation | `pkg/kubernautagent/types/types.go`, `internal/kubernautagent/tools/custom/tools.go` | `SignalContext.ClusterClassification`; discovery tool-call param |
| DS filter dimension | `pkg/datastorage/repository/workflow/discovery.go`, `pkg/datastorage/models/workflow_discovery.go`, `pkg/datastorage/server/workflow_discovery_handlers.go`, `api/openapi/data-storage-v1.yaml` | `cluster` SQL branch, filter model, param parsing, OpenAPI spec |

---

## Test Plan

Full test plan with BR Coverage Matrix, canonical Test IDs, FedRAMP control mapping, and
Given/When/Then detail: [docs/tests/1511/TEST_PLAN.md](../tests/1511/TEST_PLAN.md).

---

## References

- [DD-FLEET-002: Cluster-Scoped Workflow Targeting via Rego Policy Classification](../architecture/decisions/DD-FLEET-002-cluster-scoped-workflow-targeting.md)
- [DD-FLEET-001: Fleet Hierarchical Scope Checking](../architecture/decisions/)
- [ADR-068: Fleet Federation Architecture](../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [Issue #1511](https://github.com/jordigilh/kubernaut/issues/1511)
- [Issue #54: Fleet Management](https://github.com/jordigilh/kubernaut/issues/54)
