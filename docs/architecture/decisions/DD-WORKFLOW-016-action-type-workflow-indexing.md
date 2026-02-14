# DD-WORKFLOW-016: Action-Type Workflow Catalog Indexing

**Date**: February 5, 2026
**Status**: Approved
**Decision Maker**: Kubernaut Architecture Team
**Authority**: AUTHORITATIVE - This document governs workflow catalog matching strategy
**Affects**: Data Storage Service, HolmesGPT API, Workflow Catalog, Signal Processing
**Related**: DD-WORKFLOW-001 (Label Schema), DD-LLM-001 (MCP Search Taxonomy), DD-HAPI-016 (Remediation History Context), DD-017 (Effectiveness Monitor), ADR-054 (Predictive Signal Mode Classification)
**Version**: 1.1

---

## Changelog

### Version 1.1 (2026-02-13)
- **BR-WORKFLOW-004**: JSONB labels keys unified to camelCase (`signal_type` -> `signalType`)
- **BR-WORKFLOW-004**: `riskTolerance` deprecated (never stored in DB, removed from workflow-schema.yaml)
- **BR-WORKFLOW-004**: `actionType` is now a top-level field in workflow-schema.yaml (not inside labels)
- SQL queries updated: `labels->>'signal_type'` -> `labels->>'signalType'` (migration 026)
- See `docs/requirements/BR-WORKFLOW-004-workflow-schema-format.md` for authoritative format specification

## Changelog

### Version 1.0 (2026-02-05) -- CURRENT

**INITIAL**: Action-type workflow catalog indexing design

- Replaces `signal_type` as primary workflow matching key with `action_type`
- Defines enforced action type taxonomy (V1.0 initial set)
- Introduces `ListAvailableActions` and `ListWorkflows` context-aware HAPI tools
- Defines LLM three-step workflow discovery protocol (list actions -> list workflows -> get parameters)
- Aligns with DD-HAPI-016 remediation history context (action-based history)

---

## Problem Statement

### Cross-Source Signal Type Mismatch

The current workflow catalog matching (DD-WORKFLOW-001 v2.5) requires exact match on `signal_type` as the primary key:

```sql
WHERE labels->>'signal_type' = $1  -- Exact match required
```

Different source adapters produce incompatible signal type vocabularies for overlapping problems:

| Source Adapter | Signal Name | Underlying Problem |
|---------------|------------|-------------------|
| Kubernetes Events | `CPUThrottling` | High CPU usage |
| Prometheus Alerts | `HighCPULoad` | High CPU usage |
| Kubernetes Events | `OOMKilled` | Memory exhaustion |
| Prometheus Alerts | `PodOOMKilling` | Memory exhaustion |

If workflows are cataloged under `CPUThrottling` but the signal arrives as `HighCPULoad`, there is no match.

### Normalization Is Not the Answer

The obvious solution -- normalizing source-specific signals to canonical types (extending ADR-054's predictive mapping pattern) -- was evaluated and rejected:

1. **Mapping maintenance burden**: Every new signal from every source adapter requires a mapping entry. This grows linearly with source adapters and their alert rule sets.
2. **Cross-source equivalence is imprecise**: Prometheus `HighCPULoad` and K8s `CPUThrottling` may have different thresholds, conditions, and semantics. Treating them as identical is often wrong.
3. **New source adapters are blocked**: A new source adapter produces no catalog matches until mappings are added.
4. **Breaks the RCA value proposition**: If the workflow is determined by signal type alone, the LLM's root cause analysis is disconnected from workflow selection.

### The Fundamental Issue: Signal-to-Workflow Is Not 1:1

The same source signal can require different remediation workflows depending on root cause:

| Source Signal | Root Cause | Appropriate Workflow |
|--------------|-----------|---------------------|
| `HighCPULoad` | Legitimate traffic spike | Scale replicas |
| `HighCPULoad` | Resource limits too low | Increase CPU limits |
| `HighCPULoad` | Runaway code loop | Restart pod + escalate |
| `HighCPULoad` | External dependency causing retries | No automated action |

The mapping is not `source_signal -> workflow` but rather `source_signal -> LLM RCA -> root_cause -> workflow`. The LLM is the semantic bridge.

### Confidence Assessment: Action-Type vs SP Normalization

| Dimension | Action-Type (this DD) | SP Normalization (rejected) |
|-----------|:---:|:---:|
| Same signal, different root cause | Handles correctly | Cannot distinguish |
| Mapping maintenance | None (taxonomy only) | Grows with source adapters |
| RCA integration | Full (RCA drives selection) | Disconnected (RCA ignored for matching) |
| Failure mode | Explicit (no match -> human review) | Silent (wrong match, no signal) |
| New source adapters | Work immediately | Blocked until mapping added |
| EM/History alignment | Natural (action-based) | Awkward (signal-based) |
| Determinism | Lower (LLM-driven) | Higher (config-driven) |

**Confidence**: Action-Type approach 90% vs SP Normalization 62%.

The confidence gap comes from the SP normalization's fundamental assumption being incorrect (signal-to-workflow is not 1:1) and its failure mode being worse (silent wrong match vs explicit no-match).

---

## Decision

### Replace `signal_type` with `action_type` as Primary Catalog Key

Workflows are indexed by what they **do** (action type), not what **triggered** them (signal type). The LLM performs RCA, discovers available actions for the current context, and selects the action type that addresses the root cause.

### Enforced Action Type Taxonomy

Action types are system-defined and controlled. The taxonomy is a finite, governed vocabulary that prevents proliferation. Adding new action types is a deliberate architectural decision. Workflow authors select from the predefined set when registering workflows.

### LLM as Semantic Bridge

The LLM bridges the gap between source-specific signals and remediation actions:

1. Receives the signal (any source vocabulary)
2. Performs RCA investigation (kubectl, metrics, logs, etc.)
3. If problem identified and needs remediation:
   a. Discovers available action types (Step 1: taxonomy descriptions, filtered by signal context)
   b. Selects the action type that addresses the root cause
   c. Lists available workflows for that action type (Step 2: per-workflow descriptions, same filters)
   d. Reviews ALL workflows and selects the best fit
   e. Fetches the selected workflow's parameter schema (Step 3) and provides values
4. If problem resolved or inconclusive: follows existing paths without action discovery

### Escalation Behavior Unchanged

The existing `no_matching_workflows` -> `NeedsHumanReview` -> `NotificationRequest(manual-review)` flow is preserved. When the LLM cannot find an appropriate action or no workflows match, the same escalation path triggers.

---

## Action Type Taxonomy (V1.0)

### Design Principles

1. **VerbNoun naming convention**: Action types follow a `VerbNoun` pattern for consistency (e.g., `ScaleReplicas`, `RestartPod`)
2. **Finite and governed**: New action types require a DD amendment
3. **Description is the contract**: Each action type has an authoritative description that workflow authors must align with

### V1.0 Action Types

> **Note**: The tables below show the structured JSONB fields (`what`, `when_to_use`, `when_not_to_use`, `preconditions`) that are stored in the `action_type_taxonomy` table. The "Typical use cases" row is DD documentation context only -- it is not stored in the database and is not rendered to the LLM.

#### ScaleReplicas

| Field | Value |
|-------|-------|
| **what** | Horizontally scale a workload by adjusting the replica count. |
| **when_to_use** | Root cause is insufficient capacity to handle current load and the workload supports horizontal scaling. |
| **preconditions** | Evidence of increased incoming traffic or load correlating with the resource exhaustion. |
| **Typical use cases** | Traffic spikes, legitimate load increases |

#### RestartPod

| Field | Value |
|-------|-------|
| **what** | Kill and recreate one or more pods. |
| **when_to_use** | Root cause is a transient runtime state issue (corrupted cache, leaked connections, stuck threads) that a fresh process would resolve. |
| **preconditions** | Evidence that the issue is transient (e.g., pod was healthy before, no recent code deployment). |
| **Typical use cases** | Memory leaks, deadlocks, corrupted in-process state, stale connections |

#### IncreaseCPULimits

| Field | Value |
|-------|-------|
| **what** | Increase CPU resource limits on containers. |
| **when_to_use** | CPU throttling is caused by resource limits being too low relative to the workload's actual requirements, not by a code-level issue. |
| **preconditions** | Container is actively CPU-throttled (not just using high CPU), and CPU usage pattern is consistent with legitimate workload. |
| **Typical use cases** | CPU throttling with legitimate workload, limits set too conservatively |

#### IncreaseMemoryLimits

| Field | Value |
|-------|-------|
| **what** | Increase memory resource limits on containers. |
| **when_to_use** | OOM kills are caused by memory limits being too low relative to the workload's actual requirements. |
| **preconditions** | Memory usage shows a stable pattern consistent with legitimate workload, not unbounded growth over time. |
| **Typical use cases** | OOMKilled with legitimate memory usage, limits set too conservatively |

#### RollbackDeployment

| Field | Value |
|-------|-------|
| **what** | Revert a deployment to its previous stable revision. |
| **when_to_use** | Root cause is a recent deployment that introduced a regression, and the previous revision was healthy. |
| **preconditions** | A previous healthy revision exists (verify via rollout history) and the issue started after the most recent deployment. |
| **Typical use cases** | Post-deployment crashes, configuration regressions, bad code releases |

#### DrainNode

| Field | Value |
|-------|-------|
| **what** | Drain and cordon a Kubernetes node, evicting all pods and preventing new scheduling. |
| **when_to_use** | Root cause is a node-level issue (hardware degradation, kernel problems, disk pressure) affecting multiple workloads on the node, and pods must be moved to healthy nodes. |
| **when_not_to_use** | Only a single pod is affected on the node. This indicates a pod-level issue, not node-level -- use a pod-targeted action instead. If pods don't need to be evicted yet, use CordonNode instead. |
| **preconditions** | Confirmed that multiple workloads on the same node are affected, indicating node-scoped impact. |
| **Typical use cases** | Node disk pressure, node not ready, hardware failures requiring immediate pod evacuation |

#### CordonNode

| Field | Value |
|-------|-------|
| **what** | Cordon a Kubernetes node to prevent new pod scheduling without evicting existing pods. |
| **when_to_use** | Root cause is an emerging node-level issue that warrants preventing new pods from being scheduled, but existing pods are still running and do not need immediate eviction. |
| **when_not_to_use** | If existing pods on the node are already failing or need to be moved to healthy nodes, use DrainNode instead. |
| **preconditions** | Evidence of degrading node health (intermittent errors, rising resource pressure) but existing workloads still functional. |
| **Typical use cases** | Preventive node isolation, early-stage disk pressure, intermittent node issues |

#### RestartDeployment

| Field | Value |
|-------|-------|
| **what** | Perform a rolling restart of all pods in a workload (Deployment or StatefulSet). |
| **when_to_use** | Root cause is a workload-wide state issue affecting all or most pods, such as stale configuration, expired certificates, or corrupted shared state that requires all pods to be refreshed. |
| **preconditions** | Evidence that the issue affects multiple pods in the same workload (not just a single pod), and a fresh set of pods would resolve the issue. |
| **Typical use cases** | Certificate rotation propagation, ConfigMap changes requiring restart, widespread transient state across all replicas |

#### CleanupNode

| Field | Value |
|-------|-------|
| **what** | Reclaim disk space on a node by purging temporary files, old logs, and unused container images. |
| **when_to_use** | Node disk pressure is caused by accumulated ephemeral data (temp files, old container logs, unused images), not by legitimate workload storage growth. |
| **when_not_to_use** | If disk usage is from legitimate workload data (persistent volumes, application databases). Cleanup would not help and could cause data loss. Use DrainNode instead if the node needs to be decommissioned. |
| **preconditions** | Evidence that disk usage is dominated by ephemeral/reclaimable data (container image cache, log files, tmp directories), not persistent workload data. |
| **Typical use cases** | Node disk pressure from log accumulation, container image cache bloat, orphaned temporary files |

#### DeletePod

| Field | Value |
|-------|-------|
| **what** | Delete one or more specific pods without waiting for graceful termination. |
| **when_to_use** | Pods are stuck in a terminal state (Terminating, Unknown) and cannot be restarted through normal means. |
| **when_not_to_use** | Do not use as a general restart mechanism. Use RestartPod instead for transient runtime issues. |
| **preconditions** | Pod is genuinely stuck and not responding to graceful termination (verify via pod events and state duration). |
| **Typical use cases** | Stuck terminating pods, orphaned pods, force cleanup |

### Adding New Action Types

To add a new action type:

1. Create an amendment to this DD (version bump)
2. Define the action type name (VerbNoun pattern), description, and typical use cases
3. The description must include `what` and `when_to_use` (required). Include `when_not_to_use` and `preconditions` only when genuinely useful (see "Writing Effective Action Type Descriptions")
4. Update the DS action type validation to include the new type
5. Register workflows against the new action type

### Writing Effective Action Type Descriptions

The description is the LLM's primary signal for selecting the right action. Descriptions are stored as a structured object with four named fields:

#### Structured Description Fields

| Field | Required | Purpose |
|-------|----------|---------|
| `what` | Yes | What the action concretely does. One sentence. |
| `when_to_use` | Yes | Root cause conditions under which this action is appropriate. |
| `when_not_to_use` | No | Action-specific exclusion conditions (e.g., "use RestartPod instead of DeletePod for general restarts"). Only populate when there is a genuinely useful exclusion specific to this action. |
| `preconditions` | No | Conditions the LLM must verify through RCA investigation that **cannot** be determined by catalog label filtering. |

**Failure-based exclusions are NOT part of the description.** Conditions like "do not use if already applied without success" are handled automatically by HAPI through the remediation history context (DD-HAPI-016). HAPI injects structured history alongside the action list, giving the LLM evidence of past attempts and their outcomes. This separation ensures:

1. Failure-based exclusions apply to **all** action types systematically (not dependent on author quality)
2. Authors only write action-specific exclusions when they have something genuinely useful to say
3. No garbage data from authors filling required fields with boilerplate

#### Storage Format (JSONB)

```json
{
  "what": "Horizontally scale a workload by adjusting the replica count.",
  "when_to_use": "Root cause is insufficient capacity to handle current load and the workload supports horizontal scaling.",
  "preconditions": "Evidence of increased incoming traffic or load correlating with the resource exhaustion."
}
```

With optional `when_not_to_use` (only when genuinely useful):

```json
{
  "what": "Delete one or more specific pods without waiting for graceful termination.",
  "when_to_use": "Pods are stuck in a terminal state (Terminating, Unknown) and cannot be restarted through normal means.",
  "when_not_to_use": "Do not use as a general restart mechanism. Use RestartPod instead for transient issues.",
  "preconditions": "Pod is genuinely stuck and not responding to graceful termination (verify via pod events and state duration)."
}
```

#### LLM Rendering Format

The `list_available_actions` tool renders descriptions as structured bullet points. Optional fields are omitted when absent:

```
1. ScaleReplicas (2 workflows)
   - What: Horizontally scale a workload by adjusting the replica count.
   - Use when: Root cause is insufficient capacity, workload supports horizontal scaling.
   - Requires: Evidence of increased incoming traffic or load correlating with the resource exhaustion.

2. DeletePod (1 workflow)
   - What: Delete one or more specific pods without waiting for graceful termination.
   - Use when: Pods are stuck in a terminal state and cannot be restarted normally.
   - Do not use if: As a general restart mechanism. Use RestartPod instead.
   - Requires: Pod is genuinely stuck and not responding to graceful termination.
```

#### Preconditions vs Label Filters

**Preconditions are NOT for things already handled by catalog label filtering.** DetectedLabels (hpaEnabled, gitOpsManaged, pdbProtected, etc.) and mandatory labels (severity, component, environment) are automatically applied as filters before the LLM ever sees the action list. Preconditions are for conditions that require **LLM investigation** to verify.

| Example | Where Handled | In Preconditions? |
|---------|--------------|-------------------|
| Workload has HPA | DetectedLabel `hpaEnabled` (catalog filter) | No |
| Workload is GitOps-managed | DetectedLabel `gitOpsManaged` (catalog filter) | No |
| Environment is production | Mandatory label `environment` (catalog filter) | No |
| Evidence of traffic increase correlating with CPU spike | LLM investigation (metrics, network analysis) | Yes |
| Previous healthy revision exists for rollback | LLM investigation (rollout history) | Yes |
| Pod is genuinely stuck, not just slow to terminate | LLM investigation (pod state, events) | Yes |
| Memory usage shows legitimate growth, not unbounded leak | LLM investigation (memory metrics over time) | Yes |

#### Registration Validation

**Taxonomy description** (validated when adding/updating entries in `action_type_taxonomy`):
- `what` is present and non-empty
- `when_to_use` is present and non-empty
- `when_not_to_use` is optional (only populate with action-specific exclusions)
- `preconditions` is optional (only populate with investigation-based conditions)

**Workflow description** (validated when registering a workflow in `remediation_workflow_catalog`):
- `action_type` references a valid entry in `action_type_taxonomy` (FK constraint)
- `description` (free-form text) is present and non-empty -- describes the workflow's unique approach, strategy, and scope without hardcoded parameter values

#### Anti-Patterns

- Vague `what`: "Fixes CPU issues" (which CPU issues? how?)
- Failure-based `when_not_to_use`: "Do not use if already applied without success" (this is handled automatically by remediation history context, DD-HAPI-016)
- Boilerplate `when_not_to_use`: "Do not use when this action is not appropriate" (adds noise, no value)
- Label-based `preconditions`: "Requires no HPA" (this is a DetectedLabel filter, not a precondition)
- Overlapping scope: If two action types have similar `when_to_use` fields, the LLM cannot distinguish them

---

## ListAvailableActions Tool

### Purpose

A new HAPI MCP tool that returns the action types available for the current signal context. This is a **context-aware** query -- it does not return all action types generically. It filters by the SP-determined signal attributes so the LLM only sees actions that have actual backing workflows for this specific signal's environment, component, and severity.

### Tool Specification

**Name**: `list_available_actions`

**Description** (for LLM): "Returns the remediation action types available for the current signal context. Each action type includes a structured description of what it does and when to use it. Call this after your RCA investigation determines that remediation is needed. Select the action type that best fits your root cause. You MUST select only from the returned list."

**Parameters**:

| Parameter | Type | Required | Source | Description |
|-----------|------|----------|--------|-------------|
| `severity` | string | Yes | SP status | Signal severity (critical, high, medium, low) |
| `component` | string | Yes | SP status | Kubernetes resource type (pod, deployment, node, etc.) |
| `environment` | string | Yes | SP status | Deployment environment (production, staging, etc.) |
| `priority` | string | Yes | SP status | Business priority (P0, P1, P2, P3) |
| `custom_labels` | object | No | SP status (Rego) | Operator-defined labels (e.g., `{"constraint": ["cost-constrained"], "team": ["name=payments"]}`) |
| `detected_labels` | object | No | SP status | Auto-detected labels (e.g., `{"gitOpsManaged": true, "pdbProtected": true}`) |
| `offset` | integer | No | LLM | Pagination offset (default: 0). Set to retrieve subsequent pages. |
| `limit` | integer | No | LLM | Page size (default: 10). |

**Note**: All parameters except `offset` and `limit` are auto-populated from the SP signal context. The LLM does not need to determine them. The filter set must match the same criteria used by `get_workflow` (minus `action_type`, which is what this tool discovers) to ensure every returned action type has at least one matching workflow when the full filters are applied.

**Response**:

```json
{
  "available_actions": [
    {
      "action_type": "ScaleReplicas",
      "description": {
        "what": "Horizontally scale a workload by adjusting the replica count.",
        "when_to_use": "Root cause is insufficient capacity to handle current load and the workload supports horizontal scaling.",
        "preconditions": "Evidence of increased incoming traffic or load correlating with the resource exhaustion."
      },
      "workflow_count": 2
    },
    {
      "action_type": "IncreaseCPULimits",
      "description": {
        "what": "Increase CPU resource limits on containers.",
        "when_to_use": "CPU throttling caused by limits too low relative to workload requirements.",
        "preconditions": "Container is actively CPU-throttled, and CPU usage pattern is consistent with legitimate workload."
      },
      "workflow_count": 1
    }
  ],
  "signal_context": {
    "severity": "critical",
    "component": "deployment",
    "environment": "production",
    "priority": "P0",
    "custom_labels": {"constraint": ["cost-constrained"]},
    "detected_labels": {"gitOpsManaged": true}
  },
  "pagination": {
    "total_count": 2,
    "offset": 0,
    "limit": 10,
    "has_more": false
  }
}
```

**Note**: This response contains only action type taxonomy data (static, structured descriptions) and a `workflow_count` indicating how many workflows are available for each type. No workflow-level details are included -- the LLM uses this clean, consistent information to decide which action type fits the root cause. After selecting an action type, the LLM calls `list_workflows` to see the available workflows.

**LLM-Rendered Format** (how the tool presents it to the LLM, optional fields omitted when absent):

```
Available actions for severity=critical, component=deployment, environment=production (showing 1-2 of 2):

1. ScaleReplicas (2 workflows)
   - What: Horizontally scale a workload by adjusting the replica count.
   - Use when: Root cause is insufficient capacity, workload supports horizontal scaling.
   - Requires: Evidence of increased incoming traffic or load correlating with the resource exhaustion.

2. IncreaseCPULimits (1 workflow)
   - What: Increase CPU resource limits on containers.
   - Use when: CPU throttling caused by limits too low relative to workload requirements.
   - Requires: Container is actively CPU-throttled, CPU usage consistent with legitimate workload.
```

When more action types are available than fit in one page:

```
Available actions for severity=critical, component=deployment, environment=production (showing 1-10 of 14):

... (action types with taxonomy descriptions) ...

[4 more action types available - call list_available_actions with offset=10 to see next page]
```

### Action Type Taxonomy Table

Action type descriptions are stored in a dedicated table, separate from workflow entries. This provides a single source of truth for action type metadata. Technically, new action types can be added via DB inserts without application code changes. However, adding new action types is a governed process that requires a DD amendment (see "Adding New Action Types" section) to maintain taxonomy quality and prevent proliferation.

```sql
CREATE TABLE action_type_taxonomy (
    action_type TEXT PRIMARY KEY,          -- ScaleReplicas, RestartPod, etc.
    description JSONB NOT NULL,            -- {what, when_to_use, when_not_to_use?, preconditions?}
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

-- FK on workflow catalog ensures only valid action types
ALTER TABLE remediation_workflow_catalog
    ADD COLUMN action_type TEXT NOT NULL
        REFERENCES action_type_taxonomy(action_type);
```

**Two-level description model**:

| Level | Stored In | Purpose | Used By |
|-------|----------|---------|---------|
| **Action type description** | `action_type_taxonomy.description` (structured JSONB) | What the action category does, when to use it, exclusions, preconditions | `list_available_actions` -- LLM selects action type |
| **Workflow description** | `remediation_workflow_catalog.description` (free-form text) | What this specific workflow does differently (approach, strategy, scope, constraints) | `list_workflows` -- LLM reviews all workflows and selects one |

**Example** for `ScaleReplicas`:
- **Taxonomy**: `{"what": "Horizontally scale a workload...", "when_to_use": "Root cause is insufficient capacity..."}`
- **Workflow A description**: "Percentage-based replica scaling with a configurable upper bound. Conservative approach suitable for stateful workloads where gradual capacity increases are preferred."
- **Workflow B description**: "Multiplier-based replica scaling for rapid capacity increase. Aggressive approach designed for traffic spike emergencies where speed takes priority over cost."

### DataStorage Endpoint

**New endpoint**: `GET /api/v1/workflows/actions`

**Query parameters**:

| Parameter | Type | Required | Example |
|-----------|------|----------|---------|
| `severity` | string | Yes | `critical` |
| `component` | string | Yes | `deployment` |
| `environment` | string | Yes | `production` |
| `priority` | string | Yes | `P0` |
| `custom_labels` | JSON string | No | `{"constraint":["cost-constrained"]}` |
| `detected_labels` | JSON string | No | `{"gitOpsManaged":true,"pdbProtected":true}` |
| `offset` | integer | No | `0` (default) |
| `limit` | integer | No | `10` (default) |

**Example request**:

```
GET /api/v1/workflows/actions?severity=critical&component=deployment&environment=production&priority=P0&custom_labels={"constraint":["cost-constrained"]}&detected_labels={"gitOpsManaged":true}
```

**SQL**:

```sql
-- Count query (for total_count in pagination response -- counts distinct action types)
SELECT COUNT(*) AS total_count
FROM (
    SELECT t.action_type
    FROM action_type_taxonomy t
    INNER JOIN remediation_workflow_catalog w ON w.action_type = t.action_type
    WHERE w.status = 'active'
      AND w.is_latest_version = true
      AND (w.labels->>'severity' = $1 OR w.labels->>'severity' = '*')
      AND (w.labels->>'component' = $2 OR w.labels->>'component' = '*')
      AND (w.labels->'environment' ? $3 OR w.labels->'environment' ? '*')
      AND (w.labels->>'priority' = $4 OR w.labels->>'priority' = '*')
      AND (w.custom_labels @> $5 OR $5 IS NULL)
    GROUP BY t.action_type
) AS action_count;

-- Data query (paginated -- returns one row per action type with taxonomy description)
SELECT
    t.action_type,
    t.description,                         -- Structured JSONB from taxonomy table
    COUNT(w.workflow_id) AS workflow_count
FROM action_type_taxonomy t
INNER JOIN remediation_workflow_catalog w ON w.action_type = t.action_type
WHERE w.status = 'active'
  AND w.is_latest_version = true
  -- Context filters (same filters applied to all three endpoints)
  AND (w.labels->>'severity' = $1 OR w.labels->>'severity' = '*')
  AND (w.labels->>'component' = $2 OR w.labels->>'component' = '*')
  AND (w.labels->'environment' ? $3 OR w.labels->'environment' ? '*')
  AND (w.labels->>'priority' = $4 OR w.labels->>'priority' = '*')
  AND (w.custom_labels @> $5 OR $5 IS NULL)
GROUP BY t.action_type, t.description
ORDER BY t.action_type
LIMIT $6 OFFSET $7;                        -- Default: LIMIT 10 OFFSET 0
```

**Note**: This query returns one row per action type with its taxonomy description and count of matching workflows. Pagination is based on action type count. Only action types that have at least one active, matching workflow are returned (INNER JOIN ensures this).

**Key principle**: The context filter set must be identical across all three endpoints (`list_available_actions`, `list_workflows`, `get_workflow`). This guarantees that (a) every action type from Step 1 has matching workflows in Step 2, (b) every workflow from Step 2 will pass the security gate in Step 3, and (c) the LLM cannot bypass context restrictions by guessing workflow IDs.

**Pagination**: Results are paginated with a default page size of 10. The response includes `total_count`, `offset`, `limit`, and `has_more`. For V1.0 (10 taxonomy types), pagination is unlikely to be needed but is included for consistency with `get_workflow` and future taxonomy growth.

**Sorting**: Results are sorted alphabetically by `action_type` for deterministic, predictable ordering. The LLM selects action types based on structured descriptions and root cause analysis, not list position. Confidence scores are not exposed to the LLM and not used for sorting in this endpoint.

---

## ListWorkflows Tool (Step 2: Workflow Selection)

### Purpose

After the LLM selects an action type from `list_available_actions`, it calls `list_workflows` to see all available workflows for that action type within the current signal context. The LLM **must read all listed workflows** before selecting one -- this prevents premature selection bias and ensures the best-fit workflow is chosen.

### Tool Specification

**Name**: `list_workflows`

**Description** (for LLM): "Returns all available workflows for a specific action type matching the current signal context. Each workflow includes a description of its unique approach and strategy. You MUST read ALL listed workflows before selecting one. Do not select the first match -- compare all options against your root cause analysis."

**Parameters**:

| Parameter | Type | Required | Source | Description |
|-----------|------|----------|--------|-------------|
| `action_type` | string | Yes | LLM selection | The action type selected from `list_available_actions` results |
| `severity` | string | Yes | SP status | Signal severity (same as list_available_actions) |
| `component` | string | Yes | SP status | Kubernetes resource type (same as list_available_actions) |
| `environment` | string | Yes | SP status | Deployment environment (same as list_available_actions) |
| `priority` | string | Yes | SP status | Business priority (same as list_available_actions) |
| `custom_labels` | object | No | SP status (Rego) | Operator-defined labels (same as list_available_actions) |
| `detected_labels` | object | No | SP status | Auto-detected labels (same as list_available_actions) |
| `offset` | integer | No | LLM | Pagination offset (default: 0) |
| `limit` | integer | No | LLM | Page size (default: 10) |

**Note**: All parameters except `action_type`, `offset`, and `limit` are auto-populated from the SP signal context. The same context filters are applied to ensure only workflows matching the current signal context are returned.

**Response**:

```json
{
  "action_type": "ScaleReplicas",
  "workflows": [
    {
      "workflow_id": "wf-scale-conservative-001",
      "description": "Percentage-based replica scaling with a configurable upper bound. Conservative approach suitable for stateful workloads where gradual capacity increases are preferred."
    },
    {
      "workflow_id": "wf-scale-aggressive-002",
      "description": "Multiplier-based replica scaling for rapid capacity increase. Aggressive approach designed for traffic spike emergencies where speed takes priority over cost."
    }
  ],
  "pagination": {
    "total_count": 2,
    "offset": 0,
    "limit": 10,
    "has_more": false
  }
}
```

**LLM-Rendered Format**:

```
Workflows for ScaleReplicas (showing 1-2 of 2):

1. wf-scale-conservative-001
   Percentage-based replica scaling with a configurable upper bound. Conservative approach suitable for stateful workloads where gradual capacity increases are preferred.

2. wf-scale-aggressive-002
   Multiplier-based replica scaling for rapid capacity increase. Aggressive approach designed for traffic spike emergencies where speed takes priority over cost.

IMPORTANT: Review ALL workflows above before selecting. Do not select the first match.
```

### DataStorage Endpoint

**Endpoint**: `GET /api/v1/workflows/actions/{action_type}`

**Path parameters**:

| Parameter | Type | Required | Example |
|-----------|------|----------|---------|
| `action_type` | string | Yes | `ScaleReplicas` |

**Query parameters**:

| Parameter | Type | Required | Example |
|-----------|------|----------|---------|
| `severity` | string | Yes | `critical` |
| `component` | string | Yes | `deployment` |
| `environment` | string | Yes | `production` |
| `priority` | string | Yes | `P0` |
| `custom_labels` | JSON string | No | `{"constraint":["cost-constrained"]}` |
| `detected_labels` | JSON string | No | `{"gitOpsManaged":true}` |
| `offset` | integer | No | `0` (default) |
| `limit` | integer | No | `10` (default) |

**SQL**:

```sql
-- Count query
SELECT COUNT(*) AS total_count
FROM remediation_workflow_catalog
WHERE action_type = $1
  AND status = 'active'
  AND is_latest_version = true
  AND (labels->>'severity' = $2 OR labels->>'severity' = '*')
  AND (labels->>'component' = $3 OR labels->>'component' = '*')
  AND (labels->'environment' ? $4 OR labels->'environment' ? '*')
  AND (labels->>'priority' = $5 OR labels->>'priority' = '*')
  AND (custom_labels @> $6 OR $6 IS NULL);

-- Data query (paginated)
SELECT
    workflow_id,
    description,          -- Free-form text: per-workflow approach
    final_score           -- Internal: used by HAPI for sorting, stripped before LLM rendering
FROM remediation_workflow_catalog
WHERE action_type = $1    -- Selected action type from Step 1
  AND status = 'active'
  AND is_latest_version = true
  -- Same context filters as list_available_actions
  AND (labels->>'severity' = $2 OR labels->>'severity' = '*')
  AND (labels->>'component' = $3 OR labels->>'component' = '*')
  AND (labels->'environment' ? $4 OR labels->'environment' ? '*')
  AND (labels->>'priority' = $5 OR labels->>'priority' = '*')
  AND (custom_labels @> $6 OR $6 IS NULL)
ORDER BY final_score DESC, workflow_id ASC  -- Best matches first, deterministic tiebreaker
LIMIT $7 OFFSET $8;                        -- Default: LIMIT 10 OFFSET 0
```

**Note on `final_score`**: HAPI uses `final_score` internally for sorting results, but **strips it from the LLM tool response**. The LLM sees workflow descriptions and workflow IDs -- not scores. This prevents the LLM from over-indexing on numeric scores instead of reasoning about which workflow best fits the root cause.

**Workflow description guidelines**:
- Describe the **strategy and approach** (conservative, aggressive, adaptive, etc.)
- Describe **constraints and guardrails** the workflow enforces
- Describe the **scope** (what types of workloads or scenarios it targets)
- Do **NOT** include hardcoded parameter values -- the LLM provides parameter values based on the RCA context and the workflow's parameter schema

---

## GetWorkflow (Step 3: Workflow Parameter Lookup)

### Role: Single-Workflow Parameter Lookup

After selecting a workflow from `list_workflows`, the LLM calls `get_workflow` with the `workflow_id` to retrieve the full parameter schema needed to populate the remediation request. This is an exact lookup (not a search) -- it returns exactly one workflow or an error.

### GetWorkflow Tool Parameters

| Parameter | Type | Required | Source | Description |
|-----------|------|----------|--------|-------------|
| `workflow_id` | string | Yes | LLM selection | The workflow ID selected from `list_workflows` results |
| `severity` | string | Yes | SP status | Signal severity (same context filters) |
| `component` | string | Yes | SP status | Kubernetes resource type (same context filters) |
| `environment` | string | Yes | SP status | Deployment environment (same context filters) |
| `priority` | string | Yes | SP status | Business priority (same context filters) |
| `custom_labels` | object | No | SP status (Rego) | Operator-defined labels (same context filters) |
| `detected_labels` | object | No | SP status | Auto-detected labels (same context filters) |

**Note**: All parameters except `workflow_id` are auto-populated from the SP signal context. The filter parameters are applied as a **security gate** -- even though the `workflow_id` uniquely identifies the workflow, the context filters ensure the LLM cannot use a workflow that doesn't match the current signal context. If the LLM provides a valid `workflow_id` that belongs to a different environment or severity, the query returns 0 results.

### DataStorage Endpoint

**Endpoint**: `GET /api/v1/workflows/{workflow_id}`

**Path parameters**:

| Parameter | Type | Required | Example |
|-----------|------|----------|---------|
| `workflow_id` | string | Yes | `wf-scale-conservative-001` |

**Query parameters** (security gate):

| Parameter | Type | Required | Example |
|-----------|------|----------|---------|
| `severity` | string | Yes | `critical` |
| `component` | string | Yes | `deployment` |
| `environment` | string | Yes | `production` |
| `priority` | string | Yes | `P0` |
| `custom_labels` | JSON string | No | `{"constraint":["cost-constrained"]}` |
| `detected_labels` | JSON string | No | `{"gitOpsManaged":true}` |

### SQL

**Before (DD-WORKFLOW-001 v2.5)**:
```sql
WHERE labels->>'signal_type' = $1  -- Required, exact match
```

**After (DD-WORKFLOW-016)**:
```sql
SELECT
    workflow_id,
    action_type,
    description,          -- Free-form text: per-workflow approach (already seen in list_available_actions)
    parameters            -- Full parameter schema (mandatory/optional, types, descriptions)
FROM remediation_workflow_catalog
WHERE workflow_id = $1    -- Exact match by ID (selected from list_available_actions)
  AND status = 'active'
  AND is_latest_version = true
  -- Security: same context filters as list_available_actions
  -- Prevents LLM from using a workflow outside the allowed signal context
  AND (labels->>'severity' = $2 OR labels->>'severity' = '*')
  AND (labels->>'component' = $3 OR labels->>'component' = '*')
  AND (labels->'environment' ? $4 OR labels->'environment' ? '*')
  AND (labels->>'priority' = $5 OR labels->>'priority' = '*')
  AND (custom_labels @> $6 OR $6 IS NULL);
```

**Response** (single workflow):

```json
{
  "workflow_id": "wf-scale-conservative-001",
  "action_type": "ScaleReplicas",
  "description": "Percentage-based replica scaling with a configurable upper bound. Conservative approach suitable for stateful workloads.",
  "parameters": {
    "scale_percentage": {"type": "integer", "required": true, "description": "Percentage to scale by"},
    "max_replicas": {"type": "integer", "required": false, "description": "Upper bound for replica count"}
  }
}
```

**LLM-rendered format**:

```
Workflow: wf-scale-conservative-001 (ScaleReplicas)
Description: Percentage-based replica scaling with a configurable upper bound. Conservative approach suitable for stateful workloads.
Parameters:
  * scale_percentage (integer, required): Percentage to scale by
  * max_replicas (integer, optional): Upper bound for replica count
```

No pagination needed -- this always returns exactly one workflow or an error if the workflow_id is invalid/inactive.

**Workflow description guidelines**:
- Describe the **strategy and approach** (conservative, aggressive, adaptive, etc.)
- Describe **constraints and guardrails** the workflow enforces
- Describe the **scope** (what types of workloads or scenarios it targets)
- Do **NOT** include hardcoded parameter values -- the LLM provides parameter values based on the RCA context and the workflow's parameter schema

### Signal Type as Optional Secondary Filter

`signal_type` remains on workflow entries as optional metadata (not used for matching in V1.0). It serves as documentation for workflow authors to indicate which signal types the workflow was originally designed for.

**Deferred to post-V1.0**: `signal_types []string` as an optional scoring hint for tie-breaking when multiple workflows match the same action type. See "Future Work" section.

---

## LLM Three-Step Workflow Discovery Protocol

### Sequence

```
1. HAPI receives signal context from SP (severity, component, environment)
2. LLM performs RCA investigation (kubectl, metrics, logs, etc.)
3. IF problem resolved or inconclusive:
   -> Follow existing paths (WorkflowNotNeeded / investigation_inconclusive)
   -> Skip action discovery entirely
4. IF problem identified and needs remediation:
   a. LLM calls list_available_actions(severity, component, environment, priority, ...)
      -> HAPI queries DS, returns action types with taxonomy descriptions (clean, static data)
      -> Paginated (default 10 action types per page)
      -> If has_more=true and no action fits, LLM may request next page
   b. LLM selects action_type based on root cause + taxonomy descriptions
      -> MUST select from returned list; if none fit, report no_matching_workflows
   c. LLM calls list_workflows(action_type, severity, component, environment, priority, ...)
      -> HAPI queries DS, returns all matching workflows for that action type
      -> Paginated (default 10 workflows per page)
      -> LLM MUST read ALL listed workflows before selecting one
   d. LLM selects workflow_id based on root cause + per-workflow descriptions
      -> MUST select from returned list; if none fit, report no_matching_workflows
   e. LLM calls get_workflow(workflow_id, severity, component, environment, ...)
      -> HAPI queries DS with security gate filters, returns parameter schema
      -> Single result (no pagination); returns error if workflow_id doesn't match context
   f. LLM populates parameters based on RCA context and the parameter schema
5. HAPI validates the LLM's returned workflow by querying DS directly
   -> Ensures workflow data is current (not stale)
   -> Confirms workflow_id, action_type, and parameter schema match
```

**Rationale for post-RCA action discovery**: The action type list must be fresh in the LLM's context window at the moment of selection. RCA investigation generates many tool call responses (kubectl, metrics, logs) that push earlier context deeper into the window, risking summarization loss. By calling `list_available_actions` only after RCA concludes that remediation is needed, we: (a) avoid wasting tokens when no remediation is needed (problem resolved, inconclusive), and (b) ensure action descriptions are immediately adjacent to the selection decision.

**Rationale for three-step protocol** (list actions -> list workflows -> get parameters):
1. **Reduce noise**: Step 1 shows only clean taxonomy data (static, structured descriptions). The LLM focuses on action type selection without being distracted by workflow details.
2. **Force comprehensive review**: Step 2 lists all workflows for the selected action type. The LLM is explicitly instructed to read ALL workflows before selecting, preventing premature selection bias.
3. **Separation of concerns**: Each step serves a single decision -- what action type, which workflow, what parameters. This makes each decision clearer and more auditable.
4. **Security**: Step 3 applies context filters as a security gate, ensuring the LLM cannot use a workflow outside the allowed signal context even if it provides a valid workflow_id.

### LLM Prompt Contract

The HAPI system prompt must instruct the LLM to:

1. **Perform RCA first**: Investigate the signal thoroughly before considering remediation actions.
2. **Call `list_available_actions` only when remediation is needed**: After RCA, if the problem is identified and active, discover what action types are available. Do not call this tool if the problem is resolved or the investigation is inconclusive.
3. **Select an action type**: Review all returned action type descriptions. Select the one whose description best matches your root cause. Never hallucinate or invent an action type. If none fit, report `no_matching_workflows`.
4. **Call `list_workflows` with the selected action type**: Retrieve all available workflows for that action type.
5. **Read ALL workflows before selecting**: You MUST review every listed workflow before making a selection. Do not select the first match. Compare all workflow descriptions against your root cause analysis to identify the best fit. If no workflow fits, report `no_matching_workflows`.
6. **Call `get_workflow` with the selected `workflow_id`**: Retrieve the full parameter schema. Use the parameter descriptions and your RCA findings to provide appropriate values.
7. **Pagination**: `list_available_actions` and `list_workflows` return paginated results (default 10 per page).
   - **Step 1** (`list_available_actions`): You may select from the first page if a clear action type match exists. Request the next page only if none of the current results fit.
   - **Step 2** (`list_workflows`): You MUST review ALL available workflows before selecting. If `has_more` is true, request the next page and continue reviewing until all workflows have been seen. Do not select from an incomplete list.
   - If all pages are exhausted without a suitable match, report `no_matching_workflows`.

### HAPI Validation

After the LLM returns its selected workflow and parameters, HAPI validates the selection by querying DS directly. This ensures validation is performed against current data, avoiding stale state if workflows were updated or deactivated during the investigation. The validation checks:

1. The `workflow_id` exists and is active
2. The `workflow_id` belongs to a valid `action_type` from the taxonomy
3. The parameter types and mandatory/optional flags match the workflow schema
4. All required parameters are provided with valid types

This is the same validation pattern currently used (DD-WORKFLOW-010), updated to validate by `workflow_id` instead of `signal_type`.

### Action Type Propagation

The `action_type` selected by the LLM in Step 1 (list_available_actions) is propagated through the pipeline:

1. **HAPI response**: Included in `selected_workflow.action_type` (alongside `workflow_id`, `version`, etc.)
2. **AIAnalysis CRD**: Stored in `status.selectedWorkflow.actionType` (DD-CONTRACT-002)
3. **RO audit event**: Emitted as `workflow_type` in the `remediation.workflow_created` event (ADR-EM-001 Section 9.1)
4. **DS remediation history**: Read from `event_data.workflow_type` to populate `RemediationHistoryEntry.workflowType`

This end-to-end propagation enables the EM and DS to associate each remediation with its taxonomy action type without requiring a separate catalog lookup.

### Single Action Type Edge Case

When `ListAvailableActions` returns only one action type and `ListWorkflows` returns only one workflow, the LLM must still:

1. Perform full RCA investigation
2. Validate that the action type's description matches the root cause
3. Validate that the workflow's description matches the root cause
4. Report `no_matching_workflows` if the fit is poor

The confidence score (existing mechanism, internal to HAPI) provides an additional quality gate even in single-action scenarios -- HAPI can reject low-confidence matches before they reach workflow execution.

---

## Integration with DD-HAPI-016 (Remediation History Context)

Action-type indexing aligns naturally with the remediation history context feature:

**Before (signal-type based)**:
> "OOMKilled occurred 3 times in 24 hours for deployment payment-api"

**After (action-type based)**:
> "ScaleReplicas was applied twice for deployment payment-api in the last 24 hours. First attempt: effective (pod health restored). Second attempt: ineffective (OOMKilled recurred within 5 minutes). IncreaseMemoryLimits has not been attempted."

Action-type history is more actionable for the LLM because it describes **what was tried** rather than **what happened**. The LLM can reason: "Scaling didn't work, so the problem is not capacity-related. Let me try increasing memory limits instead."

---

## Schema Changes Summary

### New Table: `action_type_taxonomy`

| Field | Type | Details |
|-------|------|---------|
| `action_type` | TEXT PRIMARY KEY | VerbNoun identifier (e.g., `ScaleReplicas`) |
| `description` | JSONB NOT NULL | Structured: `what` (required), `when_to_use` (required), `when_not_to_use` (optional), `preconditions` (optional) |
| `created_at` | TIMESTAMP | Auto-set on creation |
| `updated_at` | TIMESTAMP | Auto-set on update |

Seeded with V1.0 taxonomy (10 action types). Authoritative source for action type descriptions used by `list_available_actions`.

### Workflow Catalog Entry (`remediation_workflow_catalog`)

| Field | Change | Details |
|-------|--------|---------|
| `action_type` | **NEW** (required) | TEXT, FK to `action_type_taxonomy(action_type)`, indexed |
| `signal_type` | **CHANGED** to optional | Was required primary key, now optional metadata |
| `description` | **CLARIFIED** (two levels) | **Action type description**: Structured JSONB in `action_type_taxonomy` (returned by `list_available_actions`). **Workflow description**: Free-form text per workflow (returned by `list_workflows`). LLM uses action descriptions in Step 1 to select action type, then workflow descriptions in Step 2 to select workflow. |

### DataStorage Endpoints

| Endpoint | Change | Details |
|----------|--------|---------|
| `GET /api/v1/workflows/actions` | **NEW** | Step 1: Context-aware action type discovery (paginated, default 10) |
| `GET /api/v1/workflows/actions/{action_type}` | **NEW** | Step 2: List workflows for a specific action type (paginated, default 10) |
| `GET /api/v1/workflows/{workflow_id}` | **UPDATED** | Step 3: Single-workflow parameter lookup with context filter security gate |

### HAPI Toolset

| Tool | Change | Details |
|------|--------|---------|
| `list_available_actions` | **NEW** | Step 1: Context-aware action type discovery from taxonomy |
| `list_workflows` | **NEW** | Step 2: List workflows for a selected action type |
| `get_workflow` | **UPDATED** | Step 3: Renamed from `search_workflow_catalog`. Single-workflow parameter lookup by `workflow_id` with context filter security gate. |

### Prompt Templates

| Template | Change | Details |
|----------|--------|---------|
| Incident prompt | **UPDATED** | Three-step protocol: list actions -> list workflows -> get parameters |
| Recovery prompt | **UPDATED** | Same three-step protocol |

---

## Impact on Existing DDs

| DD | Change Required | Details |
|----|----------------|---------|
| DD-WORKFLOW-001 v2.5 | **Amendment to v2.6** | `action_type` added as mandatory label, `signal_type` becomes optional |
| DD-LLM-001 | **Amendment** | Query format changes from `<signal_type>` to `<action_type>` |
| DD-WORKFLOW-002 | **Amendment** | New `list_available_actions` tool added to MCP toolset |
| DD-HAPI-016 | **Cross-reference** | History context references action types for correlation |
| DD-017 v2.0 | **Cross-reference** | EM effectiveness data enriches action-type history |

---

## Backward Compatibility

### Migration Strategy

1. **V1.0 catalog is small/empty**: No significant migration burden. Existing workflows (if any) receive an `action_type` assignment as part of the migration.
2. **Workflow registration API**: Requires `action_type` (must reference a valid taxonomy entry) + per-workflow `description` (free-form text describing the workflow's unique approach) going forward. `signal_type` becomes optional.
3. **Transition period**: During migration, `list_available_actions` falls back to `signal_type` filtering if `action_type` is absent on a workflow entry. This fallback is removed once all workflows are migrated.

### ADR-054 Signal Mode Classification

ADR-054's predictive-to-base type mapping (`PredictedOOMKill` -> `OOMKilled`) continues to function for signal classification in SignalProcessing. It is independent of catalog matching and serves a different purpose (signal mode classification: predictive vs reactive).

---

## Future Work

### Post-V1.0: Signal Types as Scoring Hint

`signal_types []string` may be added as an optional field on workflow entries for secondary scoring when multiple workflows share the same `action_type`. This is deferred because:

1. V1.0 workflow-level descriptions and existing label scoring should suffice for the LLM to distinguish between workflows of the same action type
2. Signal-type hints add complexity without clear benefit until we observe real-world tie-breaking patterns
3. It can be added as an additive change without redesigning the core matching

### Taxonomy Evolution

The initial V1.0 taxonomy covers common Kubernetes remediation patterns. As new remediation capabilities are added (e.g., ConfigMap patching, Secret rotation, network policy adjustment), new action types can be added through DD amendments following the VerbNoun convention.

---

## Business Requirements

### New BRs

#### BR-WORKFLOW-016-001: Action Type Taxonomy Table and Seed Data

- **Category**: WORKFLOW
- **Priority**: P0 (blocking for all other BR-WORKFLOW-016 items)
- **Description**: MUST create the `action_type_taxonomy` table in DataStorage and seed it with the V1.0 taxonomy (10 action types with structured JSONB descriptions). MUST add `action_type` column with FK constraint to `remediation_workflow_catalog`.
- **Acceptance Criteria**:
  - `action_type_taxonomy` table created with schema matching this DD
  - All 10 V1.0 action types seeded with structured descriptions (what, when_to_use, and optional fields)
  - FK constraint on `remediation_workflow_catalog.action_type` references `action_type_taxonomy.action_type`
  - Workflow registration rejects unknown action types via FK constraint (descriptive error)
  - Validation uses the `action_type_taxonomy` table as the runtime authority (not hardcoded values)
  - Unit tests cover all valid action types, rejection of invalid ones, and seed data integrity

#### BR-WORKFLOW-016-002: ListAvailableActions HAPI Tool

- **Category**: WORKFLOW
- **Priority**: P0 (blocking for V1.0 workflow discovery)
- **Description**: MUST implement `list_available_actions` MCP tool in HAPI that returns context-filtered, paginated action types with taxonomy descriptions from the `action_type_taxonomy` table.
- **Acceptance Criteria**:
  - Tool accepts severity, component, environment, priority, custom_labels, detected_labels parameters
  - Response includes action types with structured taxonomy descriptions and workflow count per action type
  - Only returns action types that have at least one active, matching workflow (INNER JOIN)
  - Supports pagination via `offset` and `limit` parameters (default: limit=10, offset=0). Pagination is based on action type count.
  - Response includes `pagination` object with `total_count`, `offset`, `limit`, `has_more`
  - Returns empty list when no workflows match context (triggers existing no-match path)
  - Parameters are auto-populated from SP signal context
  - Unit and integration tests cover all filter combinations and pagination edge cases

#### BR-WORKFLOW-016-005: ListWorkflows HAPI Tool

- **Category**: WORKFLOW
- **Priority**: P0 (blocking for V1.0 workflow selection)
- **Description**: MUST implement `list_workflows` MCP tool in HAPI that returns context-filtered, paginated workflows for a specific action type.
- **Acceptance Criteria**:
  - Tool accepts action_type (from Step 1 selection), plus severity, component, environment, priority, custom_labels, detected_labels parameters
  - Response includes workflow_id and per-workflow description (free-form text)
  - `final_score` is used internally for sorting but stripped before LLM rendering
  - Supports pagination via `offset` and `limit` parameters (default: limit=10, offset=0)
  - Response includes `pagination` object with `total_count`, `offset`, `limit`, `has_more`
  - LLM tool description explicitly instructs: "You MUST read ALL listed workflows before selecting one"
  - Returns empty list when no workflows match for the action type (triggers no-match path)
  - Unit and integration tests cover filter combinations, pagination, and empty results

#### BR-WORKFLOW-016-003: GetWorkflow Parameter Lookup

- **Category**: WORKFLOW
- **Priority**: P0 (blocking for V1.0 catalog matching)
- **Description**: MUST implement `get_workflow` as a single-workflow parameter lookup by `workflow_id` (renamed from `search_workflow_catalog`). Applies signal context filters as a security gate to prevent the LLM from using workflows outside the allowed context.
- **Acceptance Criteria**:
  - `workflow_id` is the primary lookup key (exact match)
  - Signal context filters (severity, component, environment, priority, custom_labels, detected_labels) applied as security gate
  - DS endpoint: `GET /api/v1/workflows/{workflow_id}` with context filters as query parameters
  - Returns exactly one workflow with full parameter schema, or error if not found / not allowed
  - Returns 0 results if `workflow_id` exists but doesn't match the signal context (defense in depth)
  - `signal_type` is optional metadata on workflow entries, not a filter
  - HAPI validation confirms workflow by querying DS directly (current data, not cached)
  - Unit and integration tests cover valid lookups, invalid workflow_id, and context mismatch scenarios

#### BR-WORKFLOW-016-004: LLM Three-Step Discovery Protocol

- **Category**: WORKFLOW
- **Priority**: P0 (blocking for V1.0 LLM integration)
- **Description**: MUST update HAPI prompt templates to instruct the LLM to follow the three-step workflow discovery protocol (RCA -> ListAvailableActions -> ListWorkflows -> GetWorkflow). RCA is performed first; action discovery only happens when remediation is needed.
- **Acceptance Criteria**:
  - Incident and recovery prompts include three-step protocol instructions
  - LLM is instructed to select action type from Step 1, then review ALL workflows in Step 2 before selecting
  - LLM is instructed to validate fit before selecting (even with single action or single workflow)
  - LLM is instructed on pagination behavior: for Step 1, prefer first page and request more only if no fit; for Step 2, LLM MUST review ALL pages before selecting (request next page if `has_more` is true). Report `no_matching_workflows` if all pages exhausted without a match.
  - Integration tests validate protocol compliance including pagination scenarios and workflow review enforcement

---

## Related Decisions

- **Builds On**: DD-WORKFLOW-001 (Mandatory Label Schema)
- **Builds On**: DD-LLM-001 (MCP Search Taxonomy)
- **Integrates With**: DD-HAPI-016 (Remediation History Context)
- **Integrates With**: DD-017 v2.0 (Effectiveness Monitor)
- **Independent Of**: ADR-054 (Predictive Signal Mode Classification -- continues to function for SP signal mode)
- **Supersedes**: `signal_type` as primary catalog matching key (DD-WORKFLOW-001 v2.5 matching rules)

---

**Document Version**: 1.0
**Last Updated**: February 5, 2026
**Status**: Approved
**Authority**: AUTHORITATIVE - Governs workflow catalog matching strategy
**Confidence**: 96%

**Confidence Gap (4%)**:
- LLM action-type selection accuracy (~2%): Edge cases with ambiguous root causes where multiple actions could apply. Mitigated by structured descriptions and remediation history context. Testable through integration tests with real signals.
- Three-step protocol compliance (~1%): LLM must reliably follow post-RCA discovery protocol (three steps). Mitigated by explicit prompt contract and clear tool descriptions. Testable through prompt engineering iteration.
- Taxonomy completeness (~1%): V1.0 taxonomy (10 types) covers common Kubernetes remediation patterns but may need additions as new signal sources are integrated. Additive change via DD amendment.

**Next Review**: After V1.0 implementation validates LLM action-type selection accuracy
