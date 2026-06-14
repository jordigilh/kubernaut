# DD-INT-020: ServiceNow Signal Target Type

**Status**: Proposed
**Decision Date**: 2026-06-03
**Version**: 1.5
**Confidence**: 95%
**Deciders**: Architecture Team
**Applies To**: API Frontend, Signal Processing, Remediation Orchestrator, AI Analysis, Kubernaut Agent, Workflow Execution

**Related Business Requirements**:
- BR-INT-004: ServiceNow ticket creation/tracking
- BR-INT-020: ServiceNow as signal target type

**Related Design Decisions**:
- DD-HAPI-019: KA Go rewrite design (prompt builder, parser, tool registry)
- DD-WORKFLOW-001: Mandatory label schema (workflow contract description)
- DD-RO-002: Centralized routing responsibility (scope, blocking conditions)

**Related ADRs**:
- ADR-063: ServiceNow Signal Integration Architecture
- ADR-064: Multi-Cluster Investigation via OCP MCP Server
- ADR-041: LLM Prompt Response Contract

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.5 | 2026-06-05 | Architecture Team | Part E updated: CMDB scope limited to clusters and nodes only. CMDB CI names map directly to Prometheus `cluster` and `node` labels (no mapping table needed). Added E2a (CMDB CI scope and label mapping). Confidence raised to 95%. |
| 1.4 | 2026-06-05 | Architecture Team | Part E revised: v1.5 MVP uses Thanos for multi-cluster resource status (existing Prometheus tools, zero new code). MCP Gateway deferred to v1.6+ (spike validated, prototype ready). Added KA RBAC requirement for Thanos access (`cluster-monitoring-view`). |
| 1.3 | 2026-06-04 | Architecture Team | Added Part E: Multi-Cluster Investigation via MCP Gateway. KA becomes an MCP client to the MCP Gateway (Kuadrant/Connectivity Link), which federates per-cluster OCP MCP servers. Added CMDB CI -> cluster prefix mapping via ClusterResolver. Spike validated: OCP MCP server covers 82% of KA's investigation tools, MCP Bridge prototype working (14 tests). |
| 1.2 | 2026-06-04 | Architecture Team | B2 clarified: KA injects lightweight list (numbers + titles) of related tickets, LLM uses ServiceNow tools to drill into details (mirrors K8s tool-driven investigation pattern). Workflow selection is mandatory unless ticket is already closed. |
| 1.1 | 2026-06-04 | Architecture Team | Dropped Part D (EM verification) and B6 (KA verification endpoint). ServiceNow is its own audit trail -- WFE success/failure is sufficient. Removed C3 (ProviderData to EA) since EM no longer consumes it. Simplified scope and file count. |
| 1.0 | 2026-06-03 | Architecture Team | Initial design: pipeline plumbing, KA enrichment/prompt/tools, RO guards, EM contract-driven verification, ProviderData schema |

---

## Context & Problem

### Current State

Kubernaut processes signals exclusively from Kubernetes-originated sources (AlertManager, proactive predictors). The pipeline assumes K8s semantics throughout: enrichment queries the K8s API, prompts reference K8s resources, EM verifies K8s resource state via spec hashing and pod health checks.

### Problem Statement

Customers need to investigate ServiceNow tickets that reference cloud objects (cluster status, workloads). The system must:

1. Accept signals derived from ServiceNow tickets
2. Correlate maintenance tickets with reported issues (false alarm detection)
3. Choose between closing the alert (maintenance-caused) or escalating with a new ticket

### Constraints

- `TargetType` and `ProviderData` are dropped at the AA boundary (9-hop plumbing gap)
- KA enrichment assumes K8s API access (would attempt `Pod/<ticket-number>` lookup without gating)
- Scope is customer evaluation readiness (POC/demo), not full production GA
- CMDB CIs are limited to two types: `cmdb_ci_kubernetes_cluster` (cluster) and `cmdb_ci_linux_server` (node). No application-level CIs. CI names map directly to Prometheus `cluster` and `node` labels (no mapping table required).

---

## Decision Drivers

1. **Customer demand**: Evaluation deployment requires ServiceNow integration end-to-end
2. **Pipeline reuse**: Maximize reuse of existing RR -> SP -> AA -> KA -> WFE pipeline
3. **Isolation**: ServiceNow-specific logic must not pollute K8s signal processing
4. **Minimal blast radius**: Prefer composition over modification of existing components

---

## Alternatives Considered

### Alternative A: New SignalSource="servicenow" -- REJECTED

**Approach**: Use `SignalSource` to distinguish ServiceNow signals.

**Cons**:
- `SignalSource` indicates origin (AlertManager, a2a-agent), not target system
- AF-originated ServiceNow signals should be `SignalSource="a2a-agent"`
- Would conflate two orthogonal dimensions

**Confidence**: 0% (rejected)

### Alternative B: TargetType="servicenow" with pipeline plumbing -- CHOSEN

**Approach**: Add `servicenow` to the existing `TargetType` enum. Propagate `TargetType` and `ProviderData` through the full pipeline. Gate K8s-specific logic on `TargetType`.

**Pros**:
- Reuses existing discriminator field
- Clean separation: `SignalSource` = origin, `TargetType` = target system
- K8s remains default behavior, ServiceNow is additive

**Cons**:
- 9-file plumbing change to close the AA boundary gap

**Confidence**: 95%

### Alternative C: LLM client in EM -- REJECTED

**Approach**: Embed an LLM client directly in the EM controller for ServiceNow verification.

**Cons**:
- Duplicates LLM infrastructure (client, prompt builder, parser) already in KA
- Makes EM non-deterministic for all target types
- Large blast radius in a controller that is currently purely deterministic

**Confidence**: 0% (rejected)

### Alternative D: KA verification endpoint for EM -- DEFERRED

**Approach**: Expose `POST /verify-effectiveness` in KA. EM calls it as an HTTP client. KA reuses existing LLM infrastructure for a completely independent reasoning session.

**Rationale for deferral**: ServiceNow is its own audit trail. Every ticket state change is recorded with timestamps, actors, and reasons. Unlike K8s resources (where we have no visibility into what changed and must verify post-remediation state), ServiceNow verification would be validating that a ticket did what it was supposed to do -- which the ticket history already confirms. WFE success/failure is sufficient for the POC. If a future need arises (e.g., complex multi-step ServiceNow workflows where partial success is possible), the verification endpoint can be revisited as a new requirement.

**Confidence**: N/A (deferred)

---

## Decision

### Chosen: Alternative B (TargetType) + No EM verification for ServiceNow (POC)

### Architecture

```
                         ┌──────────────────────────────────┐
                         │          ServiceNow API           │
                         └──────┬───────────────┬───────────┘
                                │               │
                           AF fetches      KA queries
                           originating     related
                           ticket          tickets
                                │               │
  ┌─────┐   ┌────┐   ┌────┐   ▼   ┌────┐      ▼   ┌─────┐        ┌─────┐
  │  AF │──▶│ RR │──▶│ SP │──────▶│ AA │─────────▶│ KA  │───────▶│ WFE │
  └─────┘   └────┘   └────┘       └────┘          └─────┘        └─────┘
              │                                                      │
              │  targetType + ProviderData                           │
              │  propagated through pipeline                  WFE success/failure
              │                                              is sufficient for EM
              ▼
           ┌────┐
           │ EA │  EM skips ServiceNow signals (no K8s checks apply,
           └────┘  ServiceNow audit trail is self-documenting)
```

**EM note**: For the POC, EM does not perform ServiceNow-specific verification. ServiceNow is its own audit trail -- every ticket state change is recorded with timestamps, actors, and reasons. WFE success/failure provides sufficient signal. Contract-driven verification via a KA endpoint could be added as a future requirement if complex multi-step workflows require partial-success detection.

### Part A: Pipeline Plumbing (9 files + codegen)

`TargetType` and `ProviderData` flow from RR through SP but are dropped at the AA boundary. The fix propagates both fields through 9 hops:

| Hop | File | Change |
|-----|------|--------|
| A1 | `api/aianalysis/v1alpha1/aianalysis_types.go` | Add `TargetType` and `ProviderData` to `SignalContextInput` |
| A2 | `pkg/remediationorchestrator/creator/aianalysis.go` | Copy from `rr.Spec` / `sp.Spec.Signal` in `buildSignalContext()` |
| A3 | `internal/kubernautagent/api/openapi.json` | Add `target_type` and `provider_data` to `IncidentRequest` (nullable optional string, `anyOf: [{type: string}, {type: null}]` pattern) |
| A3b | `pkg/agentclient/` | Regenerate with `make generate-agentclient` (produces `OptNilString` fields) |
| A4 | `pkg/aianalysis/handlers/request_builder.go` | Map AA CRD fields to `IncidentRequest` via `.SetTo()` |
| A5 | `pkg/kubernautagent/types/types.go` | Add `TargetType` and `ProviderData` to `SignalContext` |
| A6 | `internal/kubernautagent/server/handler.go` | Map `IncidentRequest` to `SignalContext` via `.Get()` pattern |
| A7 | `internal/kubernautagent/mcp/adapters/signal_resolver.go` | Read `rr.Spec.TargetType` and `rr.Spec.ProviderData` in RR fallback path |
| A8 | `internal/kubernautagent/prompt/builder.go` | Add to `SignalData`, `investigationTemplateData` |

### Part B: KA ServiceNow-Aware Logic

#### B1: Enrichment Gating

Gate K8s enrichment in `Investigate()` before `resolveEnrichmentCached()`:

```go
if signal.TargetType != "" && signal.TargetType != "kubernetes" {
    // skip K8s enrichment -- enrichData stays nil
} else {
    enrichData = inv.resolveEnrichmentCached(...)
}
```

Also skip re-enrichment (post-RCA target resolution). Downstream consumers handle nil enrichment gracefully (validated by spike).

#### B2: Prompt Templates

**Phase 1** (`incident_investigation.tmpl`): Add `{{ if eq .TargetType "servicenow" }}` conditional block (mirrors existing `{{ if eq .SignalMode "proactive" }}` pattern) containing:

- **Original ticket context** from ProviderData (number, description, state, priority, CMDB CI, timestamps, assignment details)
- **Lightweight list of related open tickets** for the same CMDB CI (fetched by KA pre-enrichment) -- ticket numbers and titles only, not full details
- **Tool-driven investigation instructions** directing the LLM to use ServiceNow tools (`servicenow_get_ticket`, etc.) to drill into the related tickets, extract summaries and relevant details, and triage them against the state of the resource to determine whether the symptoms in the original ticket are explained by the related tickets (scheduled maintenance, known changes, etc.) or whether the problem is independent
- **`submit_result` schema guidance** including `is_false_alarm`, `explained_by_ticket`, `correlation_reasoning` fields

The key design principle mirrors the K8s investigation pattern: KA provides the signal context (original ticket + lightweight list of related ticket numbers/titles) and the LLM uses tools to investigate. The LLM decides which related tickets to drill into, fetches their details via ServiceNow tools, and does the triage itself. KA does not pre-load full ticket data into the prompt -- the LLM drives the investigation through tool calls, just as it uses `kubectl_describe` and `kubectl_logs` for K8s signals.

**Phase 3** (`phase3_workflow_selection.tmpl`): Add ServiceNow action-type selection rules (CloseAlert vs EscalateTicket) alongside existing K8s/GitOps rules.

**Workflow selection is mandatory** unless the ticket is already closed at investigation time (early exit with "no action needed"). The RCA outcome (explained by maintenance or not) informs which workflow is selected but never skips selection. This mirrors the K8s flow:

| RCA Outcome | Workflow Selection |
|---|---|
| Explained by related tickets (maintenance) | CloseServiceNowAlert |
| Not explained by related tickets | EscalateServiceNowTicket |
| Ticket already closed at investigation time | No workflow -- early exit ("nothing to do") |

#### B3: Dynamic Tool Gating

Thread `phaseTools` as a parameter through the call chain (1 function reads `inv.phaseTools`, minimal refactor):

1. `toolDefinitionsForPhase(phase, phaseTools)` -- replace `inv.phaseTools` with parameter
2. `runLLMLoop(..., phaseTools)` -- pass through
3. 4 call sites pass the appropriate map based on `signal.TargetType`

Entry points select between `DefaultPhaseToolMap()` (K8s) and `ServiceNowPhaseToolMap()` (ServiceNow).

#### B4: Workflow Selection Component Key

`listActionsTool.Execute()` uses `signal.TargetType` to query the DS catalog with ServiceNow-specific component keys.

#### B5: Parser + InvestigationResult for Correlation Fields

Add to `InvestigationResult`:
- `IsFalseAlarm *bool` -- maintenance explains the incident
- `ExplainedByTicket string` -- ServiceNow ticket number that explains the issue
- `CorrelationReasoning string` -- LLM's reasoning about maintenance correlation

Add corresponding fields to `llmResponse` in parser, JSON schema in `schema.go`, and response mapper in `handler.go`.

#### B6: KA Verification Endpoint -- DEFERRED (not in POC)

Dropped from POC scope. ServiceNow's native audit trail makes contract-driven verification redundant for the demo. See "Decision Drivers" and "Alternatives Considered > Alternative D" for full rationale.

If revisited for production GA, the design would expose a `POST /verify-effectiveness` endpoint in KA for EM to call as an HTTP client. The endpoint would use the workflow's `RemediationWorkflowDescription.What` field as the agentic contract and evaluate each claim against post-remediation ServiceNow state.

### Part C: RO Guardrails

#### C1-C2: CapturePreRemediationHash Guard

Guard at both call sites in `analyzing_handler.go` and `awaiting_approval_handler.go` where `rr` is in scope:

```go
if rr.Spec.TargetType != "" && rr.Spec.TargetType != "kubernetes" {
    // skip hash capture -- preHash stays ""
} else {
    preHash, degradedReason, hashErr = h.callbacks.CapturePreRemediationHash(...)
}
```

Skip semantics: `preHash = ""` causes downstream EA creation to have empty `PreRemediationSpecHash`, which EM handles gracefully (hash comparison returns "no pre-hash for comparison").

#### C3: ProviderData into EA Spec -- DEFERRED (not in POC)

Originally planned so EM could read the pre-remediation ticket snapshot. With EM verification dropped, there is no consumer of `ProviderData` on the EA. Deferred to GA if/when EM verification is introduced.

#### CheckUnmanagedResource (no change needed for POC)

`scope.Manager.IsManaged` skips resource-level label checks for unknown K8s kinds and falls back to namespace label. ServiceNow signals in a managed namespace pass scope checks without code changes.

### Part D: EM ServiceNow Verification -- DROPPED FROM POC

**Decision**: EM does not perform ServiceNow-specific verification for the POC.

**Rationale**: ServiceNow is its own audit trail. Every ticket state change (creation, resolution, escalation, comment) is recorded with timestamps, actors, and reasons. Unlike K8s resources -- where Kubernaut has no visibility into what changed and must verify post-remediation state via spec hashing and health checks -- ServiceNow ticket history already confirms what happened. Verifying that the workflow "did what it was supposed to do" adds little value when the ticket history already documents the outcome.

For the POC, WFE success/failure is the only signal EM needs. If WFE reports success, the ServiceNow CLI container executed its API calls successfully (ticket closed, escalation created, etc.). If WFE reports failure, the Job failed.

**What this means for EM**: ServiceNow EAs will complete through the standard K8s-skip path with no ServiceNow-specific components. The existing `scopeNoExecution` / degraded mode patterns in EM handle this gracefully.

**Deferred items** (if revisited for production GA):
- D1: EA CRD extension (`TargetType`, `ProviderData`, `WorkflowID`, `ServiceNowAssessed`, `ServiceNowScore`)
- D2: EA creator populating ServiceNow fields
- D3: ServiceNow scorer as HTTP client to KA verification endpoint
- D5: Early-exit branch in `runComponentPipeline`
- D6: Completion logic for `servicenow_verified` assessment reason

### ProviderData Schema

```json
{
  "instance_url": "https://customer.service-now.com",
  "ticket": {
    "sys_id": "abc123def456",
    "number": "INC0067890",
    "short_description": "Application X unresponsive in cluster prod-east-1",
    "description": "Users reporting 503 errors since 14:30 UTC...",
    "state": "active",
    "priority": "2",
    "impact": "2",
    "urgency": "2",
    "assigned_to": "sre-team-alpha",
    "assignment_group": "Cloud Operations",
    "category": "Infrastructure",
    "subcategory": "Kubernetes",
    "cmdb_ci": {
      "sys_id": "ci_node_789",
      "name": "prod-east-1-node-03",
      "ci_class": "cmdb_ci_linux_server"
    },
    "opened_at": "2026-06-03T14:32:00Z",
    "sys_updated_on": "2026-06-03T14:35:12Z"
  }
}
```

Serves as KA investigation context. If EM verification is introduced in the future, this same snapshot serves as the pre-remediation baseline.

---

## Part E: Multi-Cluster Resource Status Investigation

### E1. Problem

ServiceNow tickets reference resources across multiple workload clusters. Kubernaut runs in a management cluster. KA needs to assess the health of resources in those workload clusters to determine if the issue is explained by maintenance or requires escalation.

### E2. CMDB CI Scope and Label Mapping

Kubernaut v1.5 supports exactly **two CMDB CI types** for ServiceNow signals:

| CMDB CI Class | Prometheus Label | Mapping |
|---------------|-----------------|---------|
| `cmdb_ci_kubernetes_cluster` | `cluster="<ci_name>"` | Direct 1:1 -- CMDB CI `name` = Prometheus `cluster` label |
| `cmdb_ci_linux_server` (node) | `node="<ci_name>"`, `cluster="<parent_ci_name>"` | Node CI `name` = Prometheus `node` label. Parent cluster CI from CMDB relationship provides `cluster` label. |

**No mapping table or ConfigMap needed.** CMDB CI names are infrastructure identifiers set during provisioning -- the same names appear as Prometheus labels because both originate from the platform provisioner. KA extracts labels directly from `ProviderData`:

```json
{
  "cmdb_ci": {
    "sys_id": "abc123",
    "name": "prod-east-1-node-03",
    "sys_class_name": "cmdb_ci_linux_server"
  },
  "cmdb_ci_parent": {
    "sys_id": "def456",
    "name": "prod-east-1",
    "sys_class_name": "cmdb_ci_kubernetes_cluster"
  }
}
```

From this, KA derives:
- `cluster = "prod-east-1"` (from `cmdb_ci_parent.name` or `cmdb_ci.name` if the CI is a cluster)
- `node = "prod-east-1-node-03"` (from `cmdb_ci.name`, only when CI is a node)

### E3. v1.5 MVP Approach: Thanos Metrics

For v1.5, KA uses **Thanos Querier** as the cross-cluster observability layer. Thanos already aggregates Prometheus metrics from all workload clusters, including the `cluster` label that identifies the source.

```
Workload Cluster A                    Workload Cluster B
├── Prometheus ──sidecar──┐            ├── Prometheus ──sidecar──┐
│   (metrics + alerts)    │            │   (metrics + alerts)    │
└─────────────────────────┘            └─────────────────────────┘
                          │                                      │
                          └──────────┬───────────────────────────┘
                                     ▼
                          Management Cluster
                          ├── Thanos Querier ◄── KA (existing Prometheus tools)
                          │   (aggregated view)
                          └── KA uses PromQL with cluster= and node= labels
```

**Why this works**:
- KA already has 8 Prometheus tools (`execute_prometheus_instant_query`, `execute_prometheus_range_query`, `get_metric_names`, `get_label_values`, `get_all_labels`, `get_metric_metadata`, `list_prometheus_rules`, `get_series`)
- These tools talk to any Prometheus-compatible API -- Thanos Querier is fully compatible
- **Zero new code required** -- just point `cfg.Integrations.Tools.Prometheus.URL` at Thanos Querier
- Cross-cluster queries use the `cluster` and `node` labels derived directly from CMDB CI names

**PromQL patterns for cluster-level investigation** (CI = `cmdb_ci_kubernetes_cluster`):

| Investigation Need | PromQL Query |
|-------------------|-------------|
| All nodes healthy? | `kube_node_status_condition{cluster="<cluster>", condition="Ready", status="true"}` |
| Any nodes not ready? | `kube_node_status_condition{cluster="<cluster>", condition="Ready", status="true"} == 0` |
| Active alerts on cluster? | `ALERTS{cluster="<cluster>", alertstate="firing"}` |
| Cluster metrics flowing? | `up{cluster="<cluster>"}` |
| Pod pressure across cluster? | `sum(kube_pod_status_phase{cluster="<cluster>", phase=~"Pending\|Unknown\|Failed"})` |

**PromQL patterns for node-level investigation** (CI = `cmdb_ci_linux_server`):

| Investigation Need | PromQL Query |
|-------------------|-------------|
| Is the node ready? | `kube_node_status_condition{cluster="<cluster>", node="<node>", condition="Ready", status="true"}` |
| Node CPU pressure? | `1 - avg(rate(node_cpu_seconds_total{cluster="<cluster>", node="<node>", mode="idle"}[5m]))` |
| Node memory available? | `node_memory_MemAvailable_bytes{cluster="<cluster>", node="<node>"}` |
| Node disk pressure? | `kube_node_status_condition{cluster="<cluster>", node="<node>", condition="DiskPressure", status="true"}` |
| Pods on this node? | `kube_pod_info{cluster="<cluster>", node="<node>"}` |
| Alerts on this node? | `ALERTS{cluster="<cluster>", node="<node>", alertstate="firing"}` |

### E4. RBAC Requirement

KA's ServiceAccount must be granted access to Thanos Querier. On OpenShift, this requires the `cluster-monitoring-view` ClusterRole:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-agent-thanos-access
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-monitoring-view
subjects:
  - kind: ServiceAccount
    name: kubernaut-agent
    namespace: kubernaut-system
```

The existing Prometheus client already supports SA bearer token auth via `auth.NewAuthTransport` + TLS via `sharedtls.NewTLSTransport`. The Thanos Querier endpoint on OCP is typically `https://thanos-querier.openshift-monitoring.svc:9091`.

### E5. KA Prompt Integration

The ServiceNow investigation prompt (`incident_investigation.tmpl`) must guide the LLM to use PromQL with labels derived directly from `ProviderData` CMDB CI names:

**For cluster-level CIs:**
```
The resource under investigation is Kubernetes cluster "{{.ClusterName}}".
Use execute_prometheus_instant_query and execute_prometheus_range_query
with cluster="{{.ClusterName}}" in your PromQL queries to assess the
cluster's health, check for active alerts, and identify node-level issues.
```

**For node-level CIs:**
```
The resource under investigation is node "{{.NodeName}}" in cluster "{{.ClusterName}}".
Use execute_prometheus_instant_query and execute_prometheus_range_query
with cluster="{{.ClusterName}}" and node="{{.NodeName}}" in your PromQL
queries to assess the node's health (Ready condition, CPU, memory, disk
pressure) and check for active alerts.
```

### E6. What Thanos Covers vs. Doesn't

| Capability | Thanos | Sufficient for MVP? |
|-----------|--------|---------------------|
| Resource health (up/down, CPU, memory) | Yes | Yes |
| Active alerts cross-cluster | Yes (`ALERTS{}`) | Yes |
| Node status (Ready, pressure) | Yes (`kube_node_*`) | Yes |
| Pod phase, restarts | Yes (`kube_pod_*`) | Yes |
| Pod specs, events, logs | No | Acceptable -- metrics + ServiceNow context is sufficient for triage |
| Resource YAML/describe | No | Acceptable -- deferred to v1.6+ |

### E7. v1.6+ Path: Direct KA→OCP MCP Server

For v1.6+, KA connects directly to per-cluster OCP MCP servers (no MCP Gateway) for full K8s API access to remote clusters (events, logs, resource specs). KA authenticates with a short-lived JWT per cluster. The spike validated:
- OCP MCP server covers 82% of KA's investigation tools
- KA MCP client prototype working (`StreamableProvider` + `BridgeTool`, 14 tests passing)
- MCP Gateway evaluated and removed from design -- KA knows the target cluster from CMDB CI, so gateway aggregation adds no value

See `docs/spikes/multi-cluster-mcp-gateway/` for spike reports and ADR-064 for the deferred architecture.

### E8. Files Affected (v1.5)

| File | Change |
|------|--------|
| Helm chart / deployment config | MODIFIED: Set `Prometheus.URL` to Thanos Querier endpoint |
| RBAC manifests | MODIFIED: Grant `cluster-monitoring-view` to KA ServiceAccount |
| `internal/kubernautagent/investigator/types.go` | MODIFIED: `ServiceNowPhaseToolMap` includes existing Prometheus tools |
| `internal/kubernautagent/prompt/templates/incident_investigation.tmpl` | MODIFIED: Add cluster-label PromQL guidance for ServiceNow signals |

No new Go code required for multi-cluster resource status in v1.5.

---

## Consequences

### Positive Consequences

1. End-to-end ServiceNow signal support (AF -> RR -> SP -> AA -> KA -> WFE) reusing existing pipeline
2. EM remains untouched for the POC -- zero regression risk for K8s signals
3. Minimal blast radius: ServiceNow-specific logic isolated to KA (tools, prompts, gating)
4. WFE success/failure + ServiceNow audit trail provide sufficient verification
5. Architecture is ticket-type-agnostic: POC targets INC (incidents), but CHG (change requests) and PRB (problems) can be added later with new prompt blocks + workflow CRDs only -- no pipeline or architecture changes

### Negative Consequences

1. 9-file plumbing change for AA boundary gap
   - **Mitigation**: Mechanical field propagation, validated by spikes
2. Two prompt templates need ServiceNow awareness (Phase 1 + Phase 3)
   - **Mitigation**: Conditional blocks mirror existing `SignalMode` pattern
3. No automated verification of ServiceNow workflow outcomes beyond WFE success/failure
   - **Mitigation**: ServiceNow's native audit trail documents what changed. For the POC, this is acceptable. For GA, a KA verification endpoint could be added.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Phase 3 template ServiceNow action types not matched by DS catalog | Low | High | Ensure ServiceNow RemediationWorkflow CRDs deployed before demo |
| WFE reports success but API call partially failed | Low | Medium | ServiceNow API returns errors on partial failure; CLI container exits non-zero |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| BR-INT-004 | Partial | ServiceNow ticket creation via WFE Job executor |
| BR-INT-020 | Full | ServiceNow as signal target type end-to-end (AF through WFE) |

---

## Validation Strategy

1. **Unit tests**: Per-component logic (parser, prompt rendering, tool gating) with mock dependencies
2. **Integration tests**: Pipeline plumbing (RR -> SP -> AA -> KA) with test CRDs
3. **E2E tests**: Full pipeline with mock ServiceNow API + mock LLM
4. **Spike validation**: 11 spikes executed pre-implementation confirming all assumptions

---

## References

- Issue #1338: feat: Add ServiceNow as a signal target type
- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go`: Workflow description contract
- `internal/kubernautagent/api/openapi.json`: KA OpenAPI specification

---

**Document Version**: 1.5
**Last Updated**: 2026-06-05
