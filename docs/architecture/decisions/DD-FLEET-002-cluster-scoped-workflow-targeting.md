# DD-FLEET-002: Cluster-Scoped Workflow Targeting via Rego Policy Classification

## Status

**Approved** (2026-06-28)
**Last Reviewed**: 2026-06-28
**Confidence**: 90%
**Milestone**: v1.6
**Related Issue**: #54 (Fleet Management)

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
- Backward compatible: when fleet is disabled or Rego doesn't output `cluster`, the field is empty (matches all)
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
    // Cluster restricts this workflow to signals originating from clusters
    // classified with these values by the SP Rego policy.
    // Empty means eligible for all clusters (backward compatible).
    // +optional
    Cluster []string `json:"cluster,omitempty"`
}
```

**Rego Policy Input** (new field in SP policy input context):
```rego
input.cluster_labels    # map[string]string from MCP Gateway CRD
```

**Rego Policy Output** (new field):
```rego
cluster := "production"   # operator-defined classification
```

### DS Query Change

DS adds `cluster` as a filter dimension in the workflow discovery query, identical
to how `severity`, `environment`, and `component` are matched today:

```sql
-- Existing pattern (severity example):
AND (w.severity IS NULL OR $severity = ANY(w.severity))
-- New cluster dimension (same pattern):
AND (w.cluster IS NULL OR $cluster = ANY(w.cluster))
```

### Primary Implementation Files

- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go` - Add `Cluster` field
- `internal/controller/signalprocessing/signalprocessing_controller.go` - Fetch cluster labels, pass to Rego
- `pkg/fleet/config.go` - SP fleet config (MCPGatewayType awareness)
- DS workflow discovery query - Add cluster filter dimension
- Rego policy schema - Add `cluster_labels` input and `cluster` output

## Consequences

**Positive**:
- No new architectural patterns -- purely additive to existing SP/Rego/DS flow
- Operators retain full semantic control over cluster taxonomy
- DS remains decoupled from fleet infrastructure
- Backward compatible: empty `cluster` field matches all clusters

**Negative**:
- SP gains a dependency on `MCPGatewayType` and `ClusterRegistry` - **Mitigation**: SP already
  participates in fleet operations (enrichment via MCP Gateway); this extends that role naturally
- Operators must write Rego rules for cluster classification - **Mitigation**: Provide example
  policies in documentation; many operators already write Rego for severity/environment

**Neutral**:
- DS schema migration required (add `cluster` column to workflow catalog table)
- Rego policy schema gains one new input field and one new output field

## Validation Results

**Confidence Assessment**: 90%

**Key Validation Points**:
- SP already fetches namespace and workload labels and passes them to Rego (proven pattern)
- DS already filters by severity/environment/component/priority (same query pattern)
- `ClusterRegistry` interface exists and is used by FMC Syncer (proven integration)
- `FleetConfig.MCPGatewayType` already selects the correct CRD type (proven configuration)

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
