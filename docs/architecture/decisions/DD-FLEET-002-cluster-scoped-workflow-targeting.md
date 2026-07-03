# DD-FLEET-002: Cluster-Scoped Workflow Targeting via Rego Policy Classification

## Status

**✅ Implemented** (2026-07-03)
**Approved**: 2026-06-28
**Last Reviewed**: 2026-07-03 (implementation complete, see Phase 7 confidence reassessment below)
**Confidence**: 97%
**Milestone**: v1.6
**Related Issue**: #54 (Fleet Management), #1511 (implementation, closed)

## Amendments (2026-07-02, pre-implementation due diligence)

The following corrections were made after tracing the actual data flow and existing
DataStorage (DS) implementation. They do not change the approved decision (Alternative 3),
only its implementation details:

1. **Storage is JSONB, not a new SQL column.** DS already stores `severity`, `environment`,
   `component`, and `priority` inside the existing `labels JSONB` column
   (`migrations/001_v1_schema.sql`). `cluster` follows the identical pattern -- **no DB
   migration is required**. The original "Neutral" consequence below describing a schema
   migration was inaccurate and is superseded by [Matching Semantics](#matching-semantics).
2. **`cluster` is optional at the schema level, not mandatory.** For non-fleet (single
   -cluster) deployments the field is omitted entirely: Kubernaut Agent (KA) does not send a
   `cluster` discovery parameter, and DS does not apply a cluster filter at all (identical to
   today's behavior). There is no `MinItems=1` validation and no catalog backfill/migration
   requirement. See [Matching Semantics](#matching-semantics) for the fleet-mode behavior.
3. **`ClusterID` and `ClusterClassification` are two distinct concepts** -- see
   [Clarification: ClusterID vs ClusterClassification](#clarification-clusterid-vs-clusterclassification).
   This issue introduces `ClusterClassification`; it does not repurpose the existing
   `ClusterID`/`ClusterName` fields.
4. **`MCPGatewayConfig` is promoted to a shared type.** FMC's previously-private
   `MCPGatewayConfig` (`pkg/fleet/fmc/config`) is promoted to `pkg/fleet.MCPGatewayConfig` and
   reused by both FMC and SP, avoiding duplicated gateway-type/endpoint config structs.
   SP's local `FleetConfig` (`pkg/signalprocessing/config/config.go`) is extended additively
   with `MCPGatewayType` + `Namespace` fields rather than being replaced by the shared
   `pkg/fleet.FleetConfig` (no breaking YAML changes to existing SP deployments).
5. **A wiring gap was found and is now in scope.** `ClusterClassification` does not
   automatically propagate end-to-end today. It must be explicitly threaded through
   `SignalProcessing.Status` &rarr; RemediationOrchestrator's `buildSignalContext()` &rarr;
   `AIAnalysis.Spec.SignalContext` &rarr; KA's `SignalContext` &rarr; KA's workflow-discovery
   tool-call parameters &rarr; DS discovery filter. See
   [RO/KA Propagation](#roka-propagation-new-wiring).

## Context & Problem

With multi-cluster federation (issue #54), RemediationWorkflows need a mechanism to
restrict which clusters they can execute on. An operator may define workflows that
should only run in staging clusters and never in production, or vice versa.

The existing `RemediationWorkflowLabels` struct provides multi-dimensional filtering
(severity, environment, component, priority) but lacks a cluster dimension. Without
this, all workflows are eligible for all clusters once fleet is enabled.

**Key Requirements**:
- Operators must be able to restrict workflow eligibility by cluster characteristics
- The solution must not couple DataStorage (DS) to MCP Gateway CRDs or the fleet registry
- Cluster names are often cryptic (e.g., `k8s-fin-eu-03p`) -- raw name matching is fragile
- Operators need semantic control over cluster classification (not hardcoded conventions)
- Backward compatible: existing single-cluster deployments must work without changes

**Constraint**: SP must know the MCPGatewayType (EAIGW or Kuadrant) since it needs to
read cluster labels from the Backend/MCPServerRegistration CRDs that define the MCP
connection.

## Alternatives Considered

### Alternative 1: Glob Pattern Matching on Cluster Name

**Approach**: Add `ClusterSelector []string` to `RemediationWorkflowLabels` with glob
patterns (e.g., `prod-*`, `staging-*`) matched against `ClusterInfo.ID`.

**Pros**:
- Simple implementation (`path.Match()` in Go stdlib)
- No new dependencies for DS -- cluster name already on the RR
- Familiar to operators who use naming conventions

**Cons**:
- Fragile: assumes operators use predictable naming conventions
- Cluster names are often cryptic (`k8s-fin-eu-03p`) -- glob patterns become unreadable
- Operators encode semantics in single characters (e.g., `d`=dev, `p`=prod) that are error-prone
- No semantic clarity -- `*p` matching "production" is not self-documenting
- Renames break all workflow targeting

**Confidence**: 70% (rejected -- too fragile for enterprise fleet naming conventions)

---

### Alternative 2: Kubernetes LabelSelector on Cluster Metadata

**Approach**: Add `ClusterSelector *metav1.LabelSelector` to `RemediationWorkflowLabels`
that matches against labels on the MCP Gateway Backend/MCPServerRegistration CRDs.

**Pros**:
- Kubernetes-native pattern (operators know label selectors)
- Supports complex expressions (In, NotIn, Exists, DoesNotExist)
- Decoupled from cluster naming -- resilient to renames
- Labels provide semantic clarity (`env=production` vs `*p`)

**Cons**:
- DS would need access to cluster labels to evaluate the selector -- awkward dependency
- Either DS imports the fleet registry (layer violation) or labels must be propagated through the pipeline
- JSONB containment queries (`<@`) are more complex than string matching
- Adds a data propagation chain that doesn't exist today (GW/SP → RR → AA → KA → DS)

**Confidence**: 75% (rejected -- DS dependency on fleet registry is architecturally unclean)

---

### Alternative 3: Rego Policy Classification with `cluster` Output Field (Approved)

**Approach**: SP fetches cluster labels from the MCP Gateway CRD (Backend or
MCPServerRegistration, depending on `MCPGatewayType`), passes them to the Rego
policy engine as input context. Operators write Rego rules that output a `cluster`
business value (e.g., "production", "staging-eu"). This value flows through the
pipeline exactly like `severity`/`environment`/`priority` and is used by DS as
another filter dimension on workflow discovery.

**Pros**:
- Zero new architectural concepts -- follows the existing SP → Rego → business values pattern
- Operator-controlled taxonomy -- they define what "production" or "staging-eu" means
- DS stays simple -- `cluster` is just another `[]string` dimension (same query pattern as severity/environment)
- No JSONB containment, no label selectors, no glob matching in DS
- No coupling between DS and fleet registry
- Backward compatible: when fleet is disabled, KA sends no `cluster` discovery parameter and DS
  applies no cluster filter at all (identical to pre-#1511 behavior)
- Decoupled from cluster naming -- Rego logic maps labels to meaningful categories

**Cons**:
- SP gains a dependency on `MCPGatewayType` to know which CRD holds cluster labels
- Requires operators to write Rego rules for cluster classification (small learning curve)
- Two-hop resolution: SP reads CRD labels → Rego transforms → business value

**Confidence**: 90% (approved)

---

## Decision

**APPROVED: Alternative 3** - Rego Policy Classification with `cluster` Output Field

**Rationale**:
1. **Architectural consistency**: Follows the identical pattern used for severity, environment,
   and priority. No new concepts, no new query patterns, no new dependencies for DS.
2. **Operator empowerment**: Operators control the cluster taxonomy through Rego policies
   they already write. They map cryptic cluster labels to meaningful business categories.
3. **Clean separation of concerns**: SP handles metadata resolution (including fleet labels),
   Rego handles business classification, DS handles catalog filtering. Each component does
   its existing job with one more dimension.

**Key Insight**: The problem of "which cluster is this?" is structurally identical to
"what severity is this?" -- both require transforming raw infrastructure metadata into
business-meaningful categories. SP + Rego already solves this; adding `cluster` is additive.

## Implementation

### Clarification: ClusterID vs ClusterClassification

These are two independent fields with different sources and purposes. They must not be
confused or merged:

| Field | Source | Purpose | Example |
|---|---|---|---|
| `ClusterID` / `ClusterName` | Raw, alert-source-supplied identifier (e.g., federated Prometheus's `webhook.CommonLabels[types.ClusterLabelKey]` in `pkg/gateway/adapters/prometheus_adapter.go`; propagated onto `KubernetesContext.ClusterID` in `pkg/shared/types/enrichment.go`) | **Lookup key** into `ClusterRegistry.Get(clusterID)` to resolve which cluster a signal came from | `"k8s-fin-eu-03p"` |
| `ClusterClassification` (new, this issue) | Rego-derived business value, computed from `ClusterInfo.Labels` (`pkg/fleet/registry/types.go`), which SP fetches from the MCP Gateway CRD (Backend/MCPServerRegistration) via `ClusterRegistry` | **Filter value** used by DS to match against `RemediationWorkflowLabels.Cluster` | `"production"`, `"staging-eu"` |

`ClusterID` is never itself used as the DS filter value -- it is cryptic and
operator-uncontrolled (see Alternative 1, rejected). `ClusterClassification` is the
Rego-produced, human-meaningful category derived *from* the cluster identified by
`ClusterID`. Both fields coexist on `SignalProcessing.Status`; code and doc comments
introduced by this issue MUST reference `ClusterClassification` explicitly and must not
alias or repurpose `ClusterID`/`ClusterName`.

### Matching Semantics

Two independent conditions determine whether the `cluster` dimension is applied at all,
and how it matches once applied:

1. **Fleet disabled, or fleet enabled but SP produced no `ClusterClassification`
   (Rego didn't set it)**: KA omits the `cluster` discovery parameter entirely. DS applies
   no cluster filter -- behaves exactly as it did before this issue. This is what makes the
   feature backward compatible and keeps non-fleet deployments unaffected.
2. **Fleet enabled and SP produced a concrete `ClusterClassification` value**: KA passes it
   to DS as the `cluster` filter parameter. DS then applies the **same mandatory-field
   idiom already used for severity/environment/priority**: a workflow matches only if
   `labels->'cluster'` contains the classification value (case-insensitive) OR contains the
   literal wildcard `"*"`. A workflow with **no `cluster` entry at all is excluded** once a
   concrete filter value is supplied -- it is *not* treated as "eligible for all clusters".

   Rationale (per due-diligence discussion): treating "unset" as a wildcard risks the LLM
   selecting a workflow that was authored for local/single-cluster use in a fleet context by
   accident. Operators who want a workflow to be discoverable across all cluster
   classifications must say so explicitly with `cluster: ["*"]`.

   This is a **soft, self-enforcing consequence of the query pattern**, not a schema-level
   validation rule -- there is no `MinItems=1` on `Cluster`, no admission-webhook rejection,
   and no required backfill migration. Existing workflow authors are unaffected unless/until
   fleet mode is enabled for their deployment; at that point, any workflow they want to keep
   universally discoverable needs `cluster: ["*"]` added (operational guidance, not a
   breaking schema change).

### RO/KA Propagation (new wiring)

`ClusterClassification` must be threaded through every hop between SP and DS, mirroring how
`severity`/`environment`/`component` already flow:

```
SignalProcessing.Status.ClusterClassification
  -> RemediationOrchestrator: buildSignalContext() (pkg/remediationorchestrator/creator/aianalysis.go)
  -> AIAnalysis.Spec.SignalContext.Cluster (api/aianalysis/v1alpha1/aianalysis_types.go: SignalContextInput)
  -> KA: katypes.SignalContext.ClusterClassification (pkg/kubernautagent/types/types.go)
  -> KA discovery tool-call params (internal/kubernautagent/tools/custom/tools.go)
  -> DS discovery filter (cluster dimension)
```

None of these hops exist today; `SignalContextInput` and `katypes.SignalContext` have no
cluster-classification field. This was not identified in the original issue scope and is a
required, non-optional part of the implementation -- without it, the DS filter dimension
would be unreachable dead code (SP would compute `ClusterClassification` but nothing would
ever pass it to DS).

### Data Flow

```
1. Signal arrives at GW with cluster identity (from federated Prometheus label or RR metadata)
2. SP controller receives the signal, identifies the source cluster ID
3. SP reads cluster labels from the MCP Gateway CRD (Backend for EAIGW, MCPServerRegistration for Kuadrant)
4. SP passes cluster labels as input to the Rego policy engine (alongside namespace/workload labels)
5. Rego policy outputs `cluster` classification (operator-defined mapping logic)
6. `cluster` value stored on SP output (alongside severity/environment/priority/component)
7. Workflow discovery query includes `cluster` as a filter dimension
8. DS matches `cluster` against RemediationWorkflow's `labels.cluster []string` field
```

### SP Dependency on MCPGatewayType

SP must know which CRD type to read for cluster labels:
- **EAIGW**: `gateway.envoyproxy.io/v1alpha1 Backend` (labels on `.metadata.labels`)
- **Kuadrant**: `mcp.kuadrant.io MCPServerRegistration` (labels on `.metadata.labels`)

This is configured via the existing `FleetConfig.MCPGatewayType` field. SP will import
`pkg/fleet/registry` to resolve cluster labels from the appropriate CRD source, using
the same `ClusterRegistry` interface that FMC already uses.

### CRD Changes

**RemediationWorkflow** (`api/remediationworkflow/v1alpha1/remediationworkflow_types.go`):
```go
type RemediationWorkflowLabels struct {
    Severity    []string `json:"severity"`
    Environment []string `json:"environment"`
    Component   []string `json:"component"`
    Priority    string   `json:"priority"`
    // Cluster restricts this workflow to signals whose SP-derived
    // ClusterClassification matches one of these values.
    // Empty/omitted is valid: in non-fleet deployments (no ClusterClassification
    // produced) this dimension is never evaluated. In fleet-enabled deployments,
    // once a concrete classification is supplied by KA, workflows with no
    // `cluster` entries are excluded unless they explicitly opt in with "*"
    // (see DD-FLEET-002 Matching Semantics -- this is a query-time exclusion,
    // not schema-level validation).
    // +optional
    Cluster []string `json:"cluster,omitempty"`
}
```

**Rego Policy Input** (new field on `evaluator.PolicyInput`, `pkg/signalprocessing/evaluator/types.go`):
```rego
input.cluster.labels    # map[string]string, sourced from ClusterInfo.Labels (pkg/fleet/registry)
```

**Rego Policy Output** (new field, alongside `severity`/`environment`/`priority`):
```rego
cluster := "production"   # operator-defined classification
```

### DS Query Change

DS adds `cluster` as a filter dimension in the workflow discovery query using the
**identical JSONB pattern already used for `severity`/`environment`** (see
`buildContextFilterSQL` in `pkg/datastorage/repository/workflow/discovery.go`) -- no new
column, no new index:

```sql
-- Existing pattern (severity, from buildContextFilterSQL):
(EXISTS (SELECT 1 FROM jsonb_array_elements_text(labels->'severity') elem
         WHERE LOWER(elem) = LOWER($1)) OR labels->'severity' ? '*')

-- New cluster dimension (identical pattern; only applied when KA supplies a
-- non-empty ClusterClassification -- see Matching Semantics):
(EXISTS (SELECT 1 FROM jsonb_array_elements_text(labels->'cluster') elem
         WHERE LOWER(elem) = LOWER($2)) OR labels->'cluster' ? '*')
```

### Primary Implementation Files

- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go` - Add `Cluster` field to `RemediationWorkflowLabels`
- `pkg/workflowschema/converter.go` - Map `Cluster` to DS models
- `internal/controller/signalprocessing/signalprocessing_controller.go` - Orchestrate cluster label fetch + Rego evaluation
- `internal/controller/signalprocessing/interfaces.go` - `PolicyEvaluator.EvaluateCluster`
- `pkg/signalprocessing/enricher/k8s_enricher.go` - Populate `KubernetesContext.Cluster` via `ClusterRegistry`
- `pkg/shared/types/enrichment.go` - Add `ClusterContext` type (labels for Rego input, distinct from `ClusterID`)
- `pkg/signalprocessing/evaluator/{types,evaluator}.go` - `PolicyInput.Cluster`, `EvaluateCluster()`
- `api/signalprocessing/v1alpha1/signalprocessing_types.go` - `Status.ClusterClassification`
- `pkg/signalprocessing/config/config.go` - Extend local `FleetConfig` with `MCPGatewayType` + `Namespace`
- `pkg/fleet/config.go`, `pkg/fleet/fmc/config/config.go` - Promote `MCPGatewayConfig` to shared `pkg/fleet` package
- `cmd/signalprocessing/main.go` - Wire `ClusterRegistry` into SP controller manager
- `pkg/datastorage/repository/workflow/discovery.go` - `buildContextFilterSQL` cluster dimension
- `pkg/datastorage/models/workflow_discovery.go`, `pkg/datastorage/server/workflow_handlers.go` - `Cluster` filter plumbing
- `api/openapi/data-storage-v1.yaml` - Optional `cluster` query parameter
- `api/aianalysis/v1alpha1/aianalysis_types.go` - `SignalContextInput.Cluster` (RO -> AA hop)
- `pkg/remediationorchestrator/creator/aianalysis.go` - Populate `SignalContextInput.Cluster` in `buildSignalContext()`
- `pkg/kubernautagent/types/types.go` - `SignalContext.ClusterClassification` (AA -> KA hop)
- `internal/kubernautagent/tools/custom/tools.go` - Pass `Cluster` into discovery tool-call params (KA -> DS hop)
- Rego policy schema (`charts/kubernaut/examples/signalprocessing-policy.rego`) - Add `cluster` rule group

## Consequences

**Positive**:
- No new architectural patterns -- purely additive to existing SP/Rego/DS flow
- Operators retain full semantic control over cluster taxonomy
- DS remains decoupled from fleet infrastructure
- Backward compatible: non-fleet deployments never send a `cluster` filter, so the
  dimension is never evaluated (see [Matching Semantics](#matching-semantics))

**Negative**:
- SP gains a dependency on `MCPGatewayType` and `ClusterRegistry` - **Mitigation**: SP already
  participates in fleet operations (enrichment via MCP Gateway); this extends that role naturally
- Operators must write Rego rules for cluster classification - **Mitigation**: Provide example
  policies in documentation; many operators already write Rego for severity/environment

**Neutral**:
- No DS schema/DB migration required -- `cluster` reuses the existing `labels JSONB`
  column and the same case-insensitive `EXISTS(jsonb_array_elements_text(...))` query
  pattern as its sibling dimensions
- Rego policy schema gains one new input field (`cluster.labels`) and one new output
  field (`cluster`)
- Legacy standalone Rego files under `deploy/signalprocessing/policies/` are already
  deprecated in favor of the unified `signalprocessing-policy.rego` (ADR-060) and are not
  updated by this issue; a documentation note flagging this as a pre-existing gap is a
  tracked follow-up, not a blocker for #1511

## Validation Results

**Pre-Implementation Confidence Assessment**: 90%

**Key Validation Points**:
- SP already fetches namespace and workload labels and passes them to Rego (proven pattern)
- DS already filters by severity/environment/component/priority (same query pattern)
- `ClusterRegistry` interface exists and is used by FMC Syncer (proven integration)
- `FleetConfig.MCPGatewayType` already selects the correct CRD type (proven configuration)

## Post-Implementation Confidence Reassessment (Phase 7, 2026-07-03)

**Final Confidence**: 97%

All phases (0-6) of the implementation plan completed via strict TDD (RED->GREEN->REFACTOR)
with a formal Wiring Audit Protocol executed across every row of the Wiring Manifest:

- Every production wiring point (`cmd/signalprocessing/main.go`, `k8s_enricher.go`,
  `signalprocessing_classifying.go`, `aianalysis.go`, `tools.go`, `discovery.go`,
  `parser.go`, shared `pkg/fleet.MCPGatewayConfig`) has a verified production caller
  (grep evidence) and a passing IT/E2E test -- no "built but not wired" gaps.
- `go build ./...` passes cleanly across the full repository post-implementation.
- No deferred wiring markers (`TODO: wire later`) and no orphaned `pkg/` files
  (new files with zero `cmd/`/`handler/` references) were found.
- `E2E-FLEET-1511-001` validates the complete SP -> RO -> AIAnalysis -> KA -> DS chain
  in a real Kind cluster: cluster classification via Rego, propagation through every
  hop, DS filter exclusion on mismatch, inclusion on match, and backward-compatible
  inclusion of both workflows when no `cluster` filter is supplied (fleet-disabled path).
- All canonical Test IDs from `docs/tests/1511/TEST_PLAN.md` pass: UT-SP-1511-001/002,
  IT-SP-1511-001/002(a/b/c), IT-RO-1511-001, UT-KA-1511-001, IT-KA-1511-001,
  UT-DS-1511-004..007, IT-DS-1511-001..003, IT-AW-1511-001, E2E-FLEET-1511-001.

**Residual 3%**: ordinary long-tail risk inherent to any cross-service feature spanning
5 services (edge-case Rego authoring errors by operators, and MCP Gateway CRD label
drift at scale) -- not a gap in this implementation's test coverage or wiring.

## Related Decisions

- **Builds On**: DD-FLEET-001 (Fleet Hierarchical Scope Checking)
- **Builds On**: ADR-068 (Fleet Federation Architecture)
- **Supports**: BR-INTEGRATION-065 (Multi-Cluster Federation)
- **Supports**: Issue #54 (Fleet Management)

## Review & Evolution

**When to Revisit**:
- If operators need more complex cluster matching than single-value equality (e.g., "all clusters
  in region X AND tier Y") -- may need multi-field cluster classification
- If DS performance degrades with the additional filter dimension at scale (unlikely -- same pattern)
- If a dedicated cluster metadata service emerges that supersedes reading MCP Gateway CRDs directly

**Success Metrics**:
- Operators can restrict workflows to specific cluster categories via Rego policy
- DS query latency unchanged (< 5ms p99) with additional cluster dimension
- Zero coupling between DS and fleet/registry packages
