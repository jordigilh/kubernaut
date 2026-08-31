# DD-FLEET-008: Workflow-Declared Execution Cluster

## Status

**✅ Implemented** (2026-08-31)
**Approved**: 2026-08-31
**Confidence**: 92%
**Milestone**: v1.6
**Related Issue**: #2326

## Context & Problem

Today, `RemediationRequest.Spec.ClusterID` (the signal's origin cluster) does double duty:
it identifies which cluster the triggering signal/incident came from, **and** it is
propagated verbatim onto `WorkflowExecution.Spec.ClusterID` by the RemediationOrchestrator
(RO) creator, which determines which cluster actually runs the workflow's Job/PipelineRun.
The implicit assumption is that these are always the same cluster.

Two real fleet topologies break that assumption:

1. **GitOps-hub remediation**: the fix is a Git commit (e.g. bumping a Helm value or
   Kustomize overlay) that a centralized ArgoCD/Flux instance on a hub cluster will
   reconcile onto the signal's origin cluster. The Job that performs the commit needs to
   run wherever that GitOps control plane's credentials/tooling live — not on the
   (possibly unrelated) cluster where the incident occurred.
2. **Edge/aggregator remediation**: a resource-constrained edge device (e.g. a minimal
   K3s node with no spare capacity to run a Job) reports the signal, but a separate,
   capable aggregator cluster with network reach to that device performs the actual
   remediation (device management API call, SSH/Redfish action, etc.).

In both cases, the cluster that should run the workflow's execution resource is a property
of the **workflow itself** — it knows how to reach the GitOps control plane or the device
management API — not something the signal or the orchestration layer can infer generically.

**Key constraint** (direct user guidance, 2026-08-31): the execution cluster must be
declared statically on the workflow's catalog definition, the same place
`ExecutionEngine`/`ExecutionBundle`/`ServiceAccountName` already live — not resolved
dynamically at runtime by the system. A `RemediationWorkflow`'s schema already encodes
everything needed to run it (image, secrets, service account); declaring *where* it runs
belongs in that same place.

**Explicitly out of scope** (confirmed via `AskQuestion`, 2026-08-31): a native
GitOps execution engine (git commit+push, wait-for-reconcile) and edge devices becoming
execution-capable fleet members themselves. This DD is strictly the execution-cluster
decoupling primitive; GitOps/edge-device logic is the workflow author's own
Job/Ansible-engine implementation, routed via this field to whichever capable
intermediary cluster it needs.

## Alternatives Considered

### Alternative A: Decoupled `execution.clusterId` on the workflow catalog schema (Approved)

**Approach**: Add an optional `ClusterID` field to `RemediationWorkflow.spec.execution`
(alongside `ServiceAccountName`), flowing through the existing catalog-authoritative
propagation chain into `WorkflowExecution.Spec.ClusterID`, taking precedence over
`RemediationRequest.Spec.ClusterID` when set.

**Pros**:
- Mirrors an already-proven pattern: `ExecutionEngine`/`ExecutionBundle`/
  `ServiceAccountName` are already static, catalog-declared execution properties
- Zero new propagation infrastructure — reuses the exact chain `ServiceAccountName`
  already traverses (CRD → KA cache → `WorkflowMeta` → `InvestigationResult` →
  `WorkflowSnapshot` → RO creator)
- Fully backward compatible: unset (the default) falls back to
  `RemediationRequest.Spec.ClusterID`, identical to today's behavior
- Reuses the existing fleet MCP Gateway dispatch path (`ClientFactory.ClientFor`) that
  `JobExecutor`/`TektonExecutor` already have — no new authorization surface
- Naturally supports per-region/per-topology variation the same way the catalog already
  handles per-environment `ServiceAccountName`/`EngineConfig` variation: author distinct
  workflow versions/variants, each with its own declared `clusterId`

**Cons**:
- A workflow that needs a *different* execution cluster per invocation (not per catalog
  entry) is not supported — must be modeled as distinct catalog entries instead
- Naming collision risk with the pre-existing `RemediationWorkflowLabels.Cluster`
  ([DD-FLEET-002](DD-FLEET-002-cluster-scoped-workflow-targeting.md)), a same-named but
  semantically unrelated concept (signal-cluster *eligibility filter*, not execution
  target) — mitigated by placing the new field under `spec.execution` and naming it
  `ClusterID`, not `Cluster`, to read unambiguously alongside `ServiceAccountName`

**Confidence**: 92% (approved)

---

### Alternative B: Dynamic execution-cluster resolution at RR/AIAnalysis time (Rejected)

**Approach**: Let the system (RO, or a new resolution step) compute the execution cluster
at runtime — e.g. via a lookup table, a Rego policy, or LLM-suggested override — similar
to how [DD-FLEET-002](DD-FLEET-002-cluster-scoped-workflow-targeting.md) resolves
`ClusterClassification` dynamically from signal context.

**Cons**:
- Rejected directly by the user (2026-08-31): "the execution cluster ID should be part
  of the workflow itself, not something that the system should know" — the workflow
  schema already encodes secrets/images/other execution-specific knowledge, so execution
  location belongs there, not in a separately-resolved runtime concept
- Introduces a second, LLM- or policy-influenced source of cluster identity for dispatch,
  when `ExecutionClusterID` is deliberately meant to be catalog-authoritative and never
  LLM-suppliable (same posture as `ActionType`/`WorkflowName`)
- No concrete runtime-customization use case exists yet to justify the added complexity
  ("we haven't yet reached the point where we customize runtimes" — user, 2026-08-31)

**Confidence in rejecting**: high — directly contradicts explicit user guidance and adds
complexity with no current business need.

---

### Alternative C: New first-class GitOps execution engine (Rejected, out of scope)

**Approach**: Add a fourth `ExecutionEngine` value (alongside `tekton`/`job`/`ansible`)
that natively performs a git commit+push and polls for reconciliation, rather than routing
through the existing Job/Ansible engines.

**Cons**:
- Confirmed out of scope via `AskQuestion` (2026-08-31): the user selected "workflow
  authors handle GitOps/edge-device logic inside their existing Job/Ansible engine" over
  "also design a new native GitOps engine type"
- A GitOps-aware `job` workflow can already push commits today (existing precedent:
  `cert-failure-gitops`-style workflows auto-discover `GIT_REPO_URL`/`GIT_BRANCH`) — the
  only missing primitive was *where* that Job runs, which Alternative A supplies

**Confidence in rejecting**: high — explicitly descoped by the user, and the gap it would
close (execution location) is already closed by Alternative A alone.

## Decision

**APPROVED: Alternative A** — add `ClusterID` to `RemediationWorkflow.spec.execution`,
propagate it catalog-authoritatively through the existing chain, and have the RO creator
prefer it over `RemediationRequest.Spec.ClusterID` when set.

**Rationale**:
1. **Matches user-specified design intent directly**: execution location is workflow
   knowledge, declared statically alongside the workflow's other execution properties
   (engine, bundle, service account) — not system-inferred.
2. **Zero new propagation infrastructure**: `ServiceAccountName` already proves this exact
   chain (CRD → KA in-memory cache → `WorkflowMeta` → `InvestigationResult` →
   `WorkflowSnapshot` → RO creator) end to end; `ExecutionClusterID` is an additive field
   riding the same rails.
3. **Backward compatible by construction**: empty (the default) means "same cluster as the
   signal," so every existing workflow's behavior is completely unchanged.
4. **No new authorization surface**: dispatch still goes through the same
   `ClientFactory.ClientFor(ctx, wfe.Spec.ClusterID)` → fleet MCP Gateway path that
   `RemediationRequest.Spec.ClusterID` already uses today. A workflow can only route
   execution to a cluster already registered with the fleet MCP Gateway — the identical
   trust boundary an operator-supplied `ClusterID` typo already fails against today
   (dispatch-time failure, not a new admission-time validation gap).

## Data Flow

```
RemediationWorkflow CRD (spec.execution.clusterId, camelCase per K8s convention)
  -> AuthWebhook admission (content-hash covers the whole spec; no special-casing needed)
  -> KA's own informer cache: cache_convert.go (crdWorkflowToModel)
       -> models.RemediationWorkflow.ExecutionClusterID (in-memory DTO only,
          DD-WORKFLOW-019: DataStorage no longer hosts the catalog)
  -> cmd/kubernautagent/toolregistry.go: buildWorkflowMeta
       -> parser.WorkflowMeta.ClusterID
  -> internal/kubernautagent/investigator/investigator_gates.go: enrichFromCatalog
       -> pkg/kubernautagent/types.InvestigationResult.ExecutionClusterID
          (json:"execution_cluster_id", snake_case: KA's external MCP tool-response
          wire format, not a CRD field -- see Naming Conventions below)
       -- catalog-authoritative: always overwritten, same treatment as
          ActionType/WorkflowName, never LLM-suppliable
  -> pkg/aianalysis/handlers/response_processor.go
       -> pkg/shared/types.WorkflowSnapshot.ExecutionClusterID (camelCase CRD field,
          inline-embedded in both AIAnalysis.Status.SelectedWorkflow and
          WorkflowExecution.Spec.WorkflowRef)
  -> pkg/remediationorchestrator/creator/workflowexecution.go: resolveExecutionClusterID
       -> WorkflowExecution.Spec.ClusterID
          (SelectedWorkflow.ExecutionClusterID if set, else rr.Spec.ClusterID)
  -> pkg/workflowexecution/executor: JobExecutor/TektonExecutor dispatch via
     ClientFactory.ClientFor(ctx, wfe.Spec.ClusterID) -- unchanged, existing fleet
     MCP Gateway routing
```

### Naming Conventions

Two layers, two conventions, both already established and unchanged by this DD:

| Layer | Convention | This field's name |
|---|---|---|
| CRD spec/status fields (`RemediationWorkflow`, `WorkflowExecution`, shared `WorkflowSnapshot`) | camelCase (K8s convention) | `clusterId` / `ExecutionClusterID` (Go) |
| KA's external MCP tool-response wire JSON (`InvestigationResult`, mirrors `api/openapi.json`'s `IncidentResponse` schema) | snake_case | `execution_cluster_id` |

A repo-wide audit during this DD's planning confirmed all `api/**/*.go` CRD types are
already camelCase with zero snake_case drift — no separate migration issue was needed.

### Interaction with DD-FLEET-007 (Ansible fail-closed)

`AnsibleExecutor.Create` already fails closed for any `wfe.Spec.ClusterID != ""`
([DD-FLEET-007](DD-FLEET-007-ansible-engine-not-supported-for-remote-execution.md)). Since
`ExecutionClusterID` flows into that same `wfe.Spec.ClusterID` field, a workflow declaring
`execution.clusterId` on the `ansible` engine hits the identical fail-closed guard — no
special-casing required, and the regression is pinned by a dedicated unit test
(`UT-RO-2326-004`).

### Distinction from DD-FLEET-002's `Labels.Cluster`

`RemediationWorkflowLabels.Cluster` ([DD-FLEET-002](DD-FLEET-002-cluster-scoped-workflow-targeting.md))
and `RemediationWorkflowExecution.ClusterID` (this DD) are unrelated, same-word concepts
living in different parts of the same CRD:

| Field | Purpose |
|---|---|
| `spec.labels.cluster` | **Eligibility filter** — which signal-origin cluster classifications make this workflow discoverable at all |
| `spec.execution.clusterId` | **Dispatch target** — which cluster the selected workflow's Job/PipelineRun actually runs on |

A workflow can filter on one and declare the other independently, or use neither, or both.

## Primary Implementation Files

- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go` — `RemediationWorkflowExecution.ClusterID`
- `pkg/datastorage/models/workflow.go` — `RemediationWorkflow.ExecutionClusterID` (in-memory DTO)
- `internal/kubernautagent/workflowcatalog/cache_convert.go` — CRD → DTO mapping
- `internal/kubernautagent/parser/validator.go` — `WorkflowMeta.ClusterID`
- `cmd/kubernautagent/toolregistry.go` — `buildWorkflowMeta`
- `pkg/kubernautagent/types/types.go` — `InvestigationResult.ExecutionClusterID`
- `internal/kubernautagent/investigator/investigator_gates.go` — `enrichFromCatalog`
- `pkg/shared/types/workflow_snapshot.go` — `WorkflowSnapshot.ExecutionClusterID`
- `pkg/aianalysis/handlers/response_processor.go` — response mapping (3 call sites)
- `pkg/remediationorchestrator/creator/workflowexecution.go` — `resolveExecutionClusterID`
- `api/workflowexecution/v1alpha1/workflowexecution_types.go` — `ClusterID` doc comment update

## Consequences

**Positive**:
- Closes a real gap for GitOps-hub and edge/aggregator remediation patterns using
  existing execution engines, with zero new engine types or gateway plumbing
- Fully backward compatible — every pre-existing workflow's behavior is unchanged
- No new authorization surface — reuses the exact fleet MCP Gateway trust boundary
  `RemediationRequest.Spec.ClusterID` already relies on

**Negative**:
- A single catalog entry cannot target a different execution cluster per invocation;
  per-region/per-topology variation requires authoring distinct workflow versions
- An unregistered or unreachable `execution.clusterId` fails at dispatch time (same
  fail-closed behavior as an RR-supplied `ClusterID` typo today), not at admission time —
  operators get a delayed rather than immediate signal for a catalog authoring mistake

**Neutral**:
- No DataStorage schema/migration involved — `models.RemediationWorkflow` is a
  KA-internal in-memory DTO only (DD-WORKFLOW-019)
- No CEL immutability rule changes were needed beyond the field additions themselves —
  `WorkflowSnapshot`'s existing pairwise-comparison rule generalizes automatically

## Security & Compliance Mapping

- **FedRAMP AC-4** (information flow enforcement): execution dispatch remains gated by the
  same fleet MCP Gateway cluster-registration boundary regardless of whether the target
  cluster identity originates from `RemediationRequest.Spec.ClusterID` or a workflow's
  declared `ExecutionClusterID` — no new information-flow path is introduced.
- **FedRAMP AC-6** (least privilege): a workflow's declared execution cluster is
  catalog-authoritative (author-controlled, admission-time content-hashed, never
  LLM-suppliable) — the LLM can select *which* workflow runs, never override *where* it
  runs.
- **OWASP ASVS 4.0.3 V4.1.1** (verify that access controls are enforced at a trusted
  service layer): cluster-dispatch authorization is enforced by `ClientFactory.ClientFor`
  against the fleet MCP Gateway's own registered-cluster set, not by any value asserted in
  the request/workflow selection path.
- **OWASP ASVS 4.0.3 V4.1.3** (principle of least privilege): `ExecutionClusterID` cannot
  grant access to any cluster the fleet MCP Gateway hasn't already independently
  registered — it selects among an existing, operator-provisioned trust set, never
  expands it.

## Test Coverage

- **Unit**: `UT-KA-2326-001..008` (CRD → cache → `WorkflowMeta` → `enrichFromCatalog`),
  `UT-AA-2326-001..004` (response processor mapping, all three `SelectedWorkflow`
  population call sites), `UT-RO-2326-001..004` (RO creator resolution: override,
  fallback, unset default, Ansible fail-closed regression pin)
- **E2E**: `E2E-FLEET-2326-001` — real AuthWebhook admission → KA catalog cache →
  mock-llm-driven selection → AIAnalysis → RO creator chain, proving
  `WorkflowExecution.Spec.ClusterID` follows the workflow's declared execution cluster
  (`prod-east`) rather than the signal's origin cluster (`prod-west`), against a
  genuinely separate remote Kind cluster (DD-TEST-013)

## Related Decisions

- **Builds On**: [DD-WORKFLOW-018](DD-WORKFLOW-018-etcd-single-source-of-truth.md) (etcd
  single source of truth), [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md)
  (KA-owned catalog, no DataStorage involvement)
- **Interacts With**: [DD-FLEET-007](DD-FLEET-007-ansible-engine-not-supported-for-remote-execution.md)
  (Ansible fail-closed guard applies unchanged)
- **Distinct From**: [DD-FLEET-002](DD-FLEET-002-cluster-scoped-workflow-targeting.md)
  (`Labels.Cluster` is an eligibility filter, not an execution target — see
  [Distinction](#distinction-from-dd-fleet-002s-labelscluster) above)
- **Supports**: BR-FLEET-004, Issue #2326

## Review & Evolution

**When to Revisit**:
- If a genuine need emerges for per-invocation (not per-catalog-entry) execution-cluster
  selection — Alternative B's dynamic-resolution approach would need re-evaluation
- If GitOps-native or edge-device-native execution engines become a prioritized business
  requirement — Alternative C's scope would need to be revisited

**Success Metrics**:
- `WorkflowExecution.Spec.ClusterID` follows a workflow's declared
  `execution.clusterId` when set, and falls back to `RemediationRequest.Spec.ClusterID`
  when unset, with zero behavior change for existing workflows
- Zero new authorization bypass: every execution-cluster value (whether RR-derived or
  workflow-declared) resolves through the same fleet MCP Gateway registration check
