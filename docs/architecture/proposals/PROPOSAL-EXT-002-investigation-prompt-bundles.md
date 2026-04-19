# PROPOSAL-EXT-002: Investigation Prompt Bundles

**Status**: PROPOSAL (under review)
**Date**: April 15, 2026
**Author**: Kubernaut Architecture Team
**Confidence**: 90% (architecture validated through adversarial review; A2A task contracts and execution trace enforcement finalized)
**Related**: [#711](https://github.com/jordigilh/kubernaut/issues/711) (Investigation Prompt Bundles), [PROPOSAL-EXT-001](PROPOSAL-EXT-001-external-integration-strategy.md) (External Integration Strategy), [DD-016](../decisions/DD-016-dynamic-toolset-v2-deferral.md) (Dynamic Toolset Deferral)

---

## Purpose

This proposal defines how customers inject their Standard Operating Procedures (SOPs) into Kubernaut Agent's investigation pipeline. Prompts and skill dependencies are packaged as OCI artifacts called **Prompt Bundles**, enabling a standardized, versionable, and distributable mechanism for customizing the investigation flow.

The design makes KA **prompt-bundle-driven**: every prompt -- including Kubernaut's own defaults embedded in the binary -- follows the same Prompt Bundle format. This makes all prompts overridable by customer bundles at well-defined hook points, using the same schema, validation, and distribution mechanism.

---

## Table of Contents

1. [Concepts and Terminology](#1-concepts-and-terminology)
2. [Prompt Bundle Manifest](#2-prompt-bundle-manifest)
3. [Investigation Pipeline Phases](#3-investigation-pipeline-phases)
4. [Skills: Discovery and Resolution](#4-skills-discovery-and-resolution)
5. [Template Data Contract (v1)](#5-template-data-contract-v1)
6. [Output Propagation Model](#6-output-propagation-model)
7. [Bundle Resolution and Loading](#7-bundle-resolution-and-loading)
8. [Validation Strategy](#8-validation-strategy)
9. [Security Considerations](#9-security-considerations)
10. [Work Estimation](#10-work-estimation)
11. [Evolution Path](#11-evolution-path)

---

## 1. Concepts and Terminology

| Term | Definition |
|------|------------|
| **Prompt Bundle** | An OCI artifact containing a YAML manifest that defines a prompt, its skill dependencies, and its expected output. Media type: `application/vnd.kubernaut.promptbundle.v1+yaml` |
| **Skill** | An OCI artifact containing tool definitions (JSON schemas, descriptions) and the endpoint URL of a pre-deployed MCP server that provides those tools. Defined by the skills marketplace project -- KA consumes but does not define this format |
| **Skills Marketplace** | An OCI registry that hosts skill artifacts. KA discovers and pulls skills from this registry at bundle resolution time |
| **Agent Marketplace** | A separate OCI registry for agent artifacts (A2A agent definitions). Out of scope for this proposal; see evolution path |
| **Hook Point** | A named position in the investigation pipeline where prompt bundles execute. Five hook points are defined |
| **Phase Output** | The natural language or structured result produced by a prompt bundle, propagated to subsequent phases via the template data contract |
| **Prompt-Bundle-Driven KA** | The principle that all prompts -- including Kubernaut's defaults embedded in the binary -- follow the Prompt Bundle format. This makes every prompt overridable by customer bundles using the same schema, validation, and distribution mechanism |

---

## 2. Prompt Bundle Manifest

### 2.1 Schema

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: PromptBundle
metadata:
  name: <bundle-name>
  description: <human-readable description>
  labels:
    kubernaut.ai/phase: <hook-point>
    kubernaut.ai/priority: "<integer>"   # execution order within a phase
spec:
  phase: <pre-investigation|investigation|post-investigation|rca-resolution|workflow-selection>
  agent:
    endpoint: <A2A endpoint URL>        # required for pre/post-investigation; optional for core phases (see 3.4)
    timeout: <duration>                 # default: 60s
  prompt: |
    <Go template with access to .Signal, .Enrichment, .PriorPhaseOutputs, .Investigation>
  skills:
    - ref: "registry.example.com/skills/cmdb-lookup@sha256:abc123..."
    - ref: "registry.example.com/skills/change-management@sha256:def456..."
    - ref: "builtin://get_namespaced_resource_context"
    - ref: "builtin://get_cluster_resource_context"
  outputSchema:    # required for rca-resolution, workflow-selection
    type: object
    properties:
      root_cause_analysis:
        type: object
        # ... JSON Schema for structured output
```

### 2.2 Field Descriptions

| Field | Required | Description |
|-------|----------|-------------|
| `apiVersion` | Yes | Must be `kubernaut.ai/v1alpha1`. Tells KA which template data contract version this bundle expects |
| `kind` | Yes | Must be `PromptBundle` |
| `metadata.name` | Yes | Unique identifier for the bundle |
| `metadata.labels` | No | Standard labels for filtering and ordering. `kubernaut.ai/phase` must match `spec.phase` |
| `spec.phase` | Yes | The hook point where this bundle executes. The phase determines the execution model (see Section 3.4) |
| `spec.agent` | Conditional | Remote agent configuration. Required for `pre-investigation` and `post-investigation` phases (always remote). Optional for core phases — when present, KA delegates to the agent instead of executing in-process (see Section 3.4, Target Architecture) |
| `spec.agent.endpoint` | Yes (when `agent` present) | A2A endpoint URL. KA sends the rendered prompt and signal context as an A2A task. The remote agent must return an `execution-trace` artifact alongside the result (see Section 9.5) |
| `spec.agent.timeout` | No | Maximum duration to wait for the remote agent to complete. Default: `60s` |
| `spec.prompt` | Yes | Go template string rendered with `missingkey=error`. For inline execution: used as the LLM system prompt. For remote execution: rendered and sent as the task description to the A2A agent. Undefined field references produce hard errors, not silent empty strings |
| `spec.skills` | No | List of skill references. OCI digests (`@sha256:`) for external skills, `builtin://` for KA's internal tools. For remote execution, skills are informational — the remote agent resolves and manages its own tools |
| `spec.outputSchema` | Conditional | Required for `rca-resolution` and `workflow-selection` phases. JSON Schema enforced via `submit_result` tool. Omitted for `pre-investigation`, `investigation`, and `post-investigation` (natural language output) |

### 2.3 Skill Reference Format

Skill references use two schemes:

| Scheme | Format | Example | Resolution |
|--------|--------|---------|------------|
| **OCI digest** | `<registry>/<repo>@sha256:<digest>` | `registry.example.com/skills/cmdb-lookup@sha256:abc123...` | Pulled from skills marketplace OCI registry |
| **Builtin** | `builtin://<tool-name>` | `builtin://get_namespaced_resource_context` | Resolved from KA's internal tool registry |

OCI digests are mandatory for external skills to guarantee immutability. Mutable tags (`:latest`, `:v1`) are not permitted in skill references.

### 2.4 Example: Pre-Investigation CMDB Check (Remote)

Pre/post-investigation bundles always execute remotely via A2A delegation. The customer deploys a standing agent (e.g., a DocsClaw pod) configured with the CMDB tools and its own ServiceAccount. KA sends the rendered prompt as an A2A task and collects the natural language result.

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: PromptBundle
metadata:
  name: acme-cmdb-precheck
  description: "Verify resource exists in CMDB before investigation"
  labels:
    kubernaut.ai/phase: pre-investigation
    kubernaut.ai/priority: "10"
spec:
  phase: pre-investigation
  agent:
    endpoint: "http://acme-cmdb-agent.acme-sops.svc:8080"
    timeout: 30s
  prompt: |
    You are a pre-investigation assistant for Kubernaut.

    A {{ .Signal.Severity }} signal "{{ .Signal.Name }}" has been received for
    {{ .Signal.Namespace }}/{{ .Signal.ResourceKind }}/{{ .Signal.ResourceName }}
    in cluster {{ .Signal.ClusterName }}.

    Before the main investigation begins, verify this resource in the CMDB:
    1. Call the cmdb_lookup tool with the resource identity
    2. Based on the response, return ONE of these statements:
       - "Resource is registered in CMDB and available for remediation"
       - "Resource is not registered in CMDB and should be escalated to manual review"
       - "Resource is registered but marked as decommissioned -- skip remediation"
  skills:
    - ref: "registry.acme.com/skills/cmdb-lookup@sha256:a1b2c3d4..."
```

Note: `spec.skills` is informational for remote bundles — the remote agent manages its own tool resolution. The skill references document the bundle's dependencies for auditing and the `kubernaut bundle test` CLI (DG-5).

### 2.5 Example: Investigation Bundle (Default, Inline)

This is how the current `incident_investigation.tmpl` would look as a Prompt Bundle. In the target architecture (v1.6+), this bundle gains a `spec.agent` block pointing to an MCP-backed investigation agent.

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: PromptBundle
metadata:
  name: kubernaut-default-investigation
  description: "Kubernaut built-in RCA investigation prompt"
  labels:
    kubernaut.ai/phase: investigation
    kubernaut.ai/priority: "0"
spec:
  phase: investigation
  prompt: |
    # Incident Analysis Request

    ## Incident Summary

    {{ .Signal.Severity }} {{ .Signal.Name }} in {{ .Signal.Namespace }}: {{ .Signal.Message }}

    **Business Impact Assessment**:
    - **Priority**: {{ .Signal.Priority }}
    - **Environment**: {{ .Signal.Environment }}
    - **Risk Tolerance**: {{ .Signal.RiskTolerance }}

    **Technical Details**:
    - Signal Name: {{ .Signal.Name }}
    - Severity: {{ .Signal.Severity }}
    - Resource: {{ .Signal.Namespace }}/{{ .Signal.ResourceKind }}/{{ .Signal.ResourceName }}
    - Error: {{ .Signal.Message }}
    - Signal Source: {{ .Signal.SignalSource }}
    - Cluster: {{ .Signal.ClusterName }}

    {{ if .Enrichment.OwnerChain }}
    ## Enrichment Context (AUTO-DETECTED)
    **Owner Chain**: {{ .Enrichment.OwnerChain }}
    {{ end }}
    {{ if .Enrichment.DetectedLabels }}
    **Detected Labels**: {{ .Enrichment.DetectedLabels }}
    {{ end }}

    {{ range .PriorPhaseOutputs }}
    ## {{ .Name }} ({{ .Phase }})
    {{ .Output }}
    {{ end }}

    ## Your Investigation Workflow
    ... (investigation instructions, guardrails)

    ## Expected Output

    Write a detailed diagnostic narrative covering:
    - What you found during investigation (pod status, logs, metrics)
    - The root cause you identified and why
    - The affected resource and its owner chain
    - Severity assessment and confidence level
    - Contributing factors

    Do NOT call submit_result. Write your findings as natural language text.
  skills:
    - ref: "builtin://get_namespaced_resource_context"
    - ref: "builtin://get_cluster_resource_context"
    - ref: "builtin://get_pod_status"
    - ref: "builtin://get_pod_logs"
    - ref: "builtin://get_events"
    - ref: "builtin://get_metrics"
    - ref: "builtin://query_prometheus"
```

### 2.6 Example: RCA Resolution Bundle (Default, Inline)

This phase takes the free-form investigation narrative and structures it into the `InvestigationResult` that KA needs. It must handle all four investigation outcomes, not just actionable RCAs. In the target architecture (v1.6+), this bundle gains a `spec.agent` block.

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: PromptBundle
metadata:
  name: kubernaut-default-rca-resolution
  description: "Kubernaut built-in RCA resolution -- structures investigation narrative into InvestigationResult"
  labels:
    kubernaut.ai/phase: rca-resolution
    kubernaut.ai/priority: "0"
spec:
  phase: rca-resolution
  prompt: |
    # RCA Resolution

    You are given the full investigation narrative below. Your job is to extract
    structured findings by calling `submit_result`. Do NOT re-investigate -- only
    structure what the investigation already found.

    ## Investigation Narrative

    {{ .Investigation.RCANarrative }}

    {{ range .PriorPhaseOutputs }}
    ## {{ .Name }} ({{ .Phase }})
    {{ .Output }}
    {{ end }}

    ## Signal Context
    - Signal: {{ .Signal.Name }} ({{ .Signal.Severity }})
    - Resource: {{ .Signal.Namespace }}/{{ .Signal.ResourceKind }}/{{ .Signal.ResourceName }}
    - Cluster: {{ .Signal.ClusterName }}

    ## Instructions

    Based on the investigation narrative, determine which outcome applies:

    **Outcome A: Actionable RCA** -- The investigation identified a root cause
    and remediation is warranted. Populate root_cause_analysis with summary,
    severity, signal_name, remediation_target. Set actionable=true.

    **Outcome B: Problem Self-Resolved** -- The investigation found the issue
    has already resolved itself. Set investigation_outcome="problem_resolved",
    actionable=false. Still provide root_cause_analysis.summary describing
    what happened and why it resolved.

    **Outcome C: Not Actionable** -- The signal does not warrant remediation
    (e.g., expected behavior, informational alert). Set actionable=false,
    investigation_outcome="not_actionable".

    **Outcome D: Insufficient Data** -- The investigation could not determine
    a root cause with confidence. Set investigation_outcome="insufficient_data".
    This triggers human review.

    Call `submit_result` with the structured JSON.
  skills: []
  outputSchema:
    type: object
    properties:
      root_cause_analysis:
        type: object
        required: [summary]
        properties:
          summary: { type: string }
          severity: { type: string, enum: [critical, high, medium, low, info, unknown] }
          signal_name: { type: string }
          contributing_factors: { type: array, items: { type: string } }
          remediation_target:
            type: object
            properties:
              kind: { type: string }
              name: { type: string }
              namespace: { type: string }
      severity: { type: string, enum: [critical, high, medium, low, info, unknown] }
      confidence: { type: number, minimum: 0, maximum: 1 }
      investigation_outcome: { type: string, enum: [actionable, not_actionable, problem_resolved, insufficient_data] }
      actionable: { type: boolean }
    required: [root_cause_analysis, confidence]
```

The `rca-resolution` phase has **no skills** -- it does not call tools. It operates purely on the investigation narrative and prior phase outputs, structuring them into the JSON format that KA's result parser and downstream controllers expect.

### 2.7 Example: Workflow Selection Bundle (Default, Inline)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: PromptBundle
metadata:
  name: kubernaut-default-workflow-selection
  description: "Kubernaut built-in workflow selection prompt"
  labels:
    kubernaut.ai/phase: workflow-selection
    kubernaut.ai/priority: "0"
spec:
  phase: workflow-selection
  prompt: |
    # Workflow Selection Request

    ## Incident Context

    A **{{ .Signal.Severity }} {{ .Signal.Name }}** event occurred for
    **{{ .Signal.Namespace }}/{{ .Signal.ResourceKind }}/{{ .Signal.ResourceName }}**
    in cluster **{{ .Signal.ClusterName }}**.

    ## Root Cause Analysis Summary

    {{ .Investigation.RCASummary }}

    {{ if .Enrichment.HistoryRendered }}
    ## Enrichment Context
    {{ .Enrichment.HistoryRendered }}
    {{ end }}

    {{ range .PriorPhaseOutputs }}
    ## {{ .Name }} ({{ .Phase }})
    {{ .Output }}
    {{ end }}

    ## Workflow Discovery Protocol (Three-Step -- MANDATORY)
    ... (discovery instructions, submit_result schema)
  skills:
    - ref: "builtin://list_available_actions"
    - ref: "builtin://list_workflows"
    - ref: "builtin://get_workflow"
  outputSchema:
    type: object
    properties:
      root_cause_analysis:
        type: object
        required: [summary, severity, signal_name]
        properties:
          summary: { type: string }
          severity: { type: string }
          signal_name: { type: string }
          remediation_target:
            type: object
            properties:
              kind: { type: string }
              name: { type: string }
              namespace: { type: string }
      selected_workflow:
        type: object
        properties:
          workflow_id: { type: string }
          confidence: { type: number }
          rationale: { type: string }
          parameters: { type: object }
          execution_engine: { type: string }
      alternative_workflows:
        type: array
        items:
          type: object
          properties:
            workflow_id: { type: string }
            confidence: { type: number }
            rationale: { type: string }
      severity: { type: string }
      confidence: { type: number }
      actionable: { type: boolean }
```

---

## 3. Investigation Pipeline Phases

### 3.1 Phase Definitions

The investigation pipeline consists of five ordered phases. Each phase has one or more prompt bundles that execute sequentially (ordered by `kubernaut.ai/priority`).

```
pre-investigation --> investigation --> post-investigation --> rca-resolution --> workflow-selection
```

| Phase | Purpose | Output Type | Required | Default Bundle |
|-------|---------|-------------|----------|----------------|
| `pre-investigation` | Customer SOP checks before investigation (CMDB, change freeze, compliance) | Natural language | No | None |
| `investigation` | Root cause analysis -- inspect pods, logs, metrics, produce diagnostic narrative | Natural language | Yes | `kubernaut-default-investigation` |
| `post-investigation` | Customer SOP checks after RCA (enrich findings, cross-reference with internal systems) | Natural language | No | None |
| `rca-resolution` | Structure the investigation narrative into the `InvestigationResult` that KA needs (remediation target, severity, confidence, signal name) | Structured JSON via `submit_result` | Yes | `kubernaut-default-rca-resolution` |
| `workflow-selection` | Select remediation workflow from catalog | Structured JSON via `submit_result` | Yes | `kubernaut-default-workflow-selection` |

### 3.2 Phase Execution Model

For each phase, KA:

1. Resolves all prompt bundles assigned to that phase (built-in + customer overrides)
2. Orders them by `kubernaut.ai/priority` (lower number = earlier execution)
3. Renders the Go template with `Option("missingkey=error")` and the current template data context
4. Executes the bundle according to the phase execution model (see 3.4):
   - **Inline**: KA resolves skills, runs the LLM loop in-process, captures output
   - **Remote**: KA sends the rendered prompt + signal context as an A2A task to the agent endpoint, waits for the response + execution trace
5. Captures the output (natural language or structured JSON)
6. Appends the output to `PriorPhaseOutputs` for subsequent phases
7. If a phase has no bundles (e.g., no `pre-investigation` configured), it is skipped
8. If the aggregate time for all bundles in a phase exceeds the configured `phaseTimeout` (optional, default: none), the remaining bundles are skipped and the failure policy applies

### 3.3 Mapping to Current Investigator Architecture

The current `Investigator.Investigate()` method in `internal/kubernautagent/investigator/investigator.go` executes two LLM invocations:

| Current Method | Current Phase | New Phase |
|---------------|---------------|-----------|
| `runRCA()` | Phase 1 (RCA) | `investigation` |
| `runWorkflowSelection()` | Phase 3 (Workflow Selection) | `workflow-selection` |

The prompt bundle system wraps these invocations:

```
[pre-investigation bundles]   <-- REMOTE: A2A delegation to customer agent
    |
    v (natural language outputs -> PriorPhaseOutputs)
[investigation bundle]         <-- INLINE (v1.5) / REMOTE (target): replaces runRCA()
    |
    v (natural language narrative -> PriorPhaseOutputs + .Investigation.RCANarrative)
[post-investigation bundles]   <-- REMOTE: A2A delegation to customer agent
    |
    v (natural language outputs -> PriorPhaseOutputs)
[rca-resolution bundle]        <-- INLINE (v1.5) / REMOTE (target): structures narrative
    |
    v (structured InvestigationResult -> drives re-enrichment, severity, remediation target)
[workflow-selection bundle]    <-- INLINE (v1.5) / REMOTE (target): replaces runWorkflowSelection()
    |
    v (structured InvestigationResult with workflow selection)
```

### 3.4 Execution Model: Phase Determines Runtime

The phase determines whether a bundle executes inline (KA in-process) or remote (A2A delegation). There is no per-bundle `mode` field — the execution model is an architectural property of the phase.

**v1.5 (Hybrid)**:

| Phase | Execution | Reason |
|-------|-----------|--------|
| `pre-investigation` | **Remote** | Customer SOPs accessing external systems. `spec.agent.endpoint` required. |
| `investigation` | **Inline** | Uses KA's builtin K8s tools (get_pod_logs, get_events, query_prometheus). |
| `post-investigation` | **Remote** | Customer SOPs accessing external systems. `spec.agent.endpoint` required. |
| `rca-resolution` | **Inline** | Structures narrative via `submit_result`, parsed by KA's result handler. |
| `workflow-selection` | **Inline** | Queries workflow CRDs via KA's builtin tools (list_workflows, get_workflow). |

**Target Architecture (v1.6+): KA as Pure Orchestrator**:

All phases execute remotely. KA's current builtin tools are extracted into standalone MCP servers that remote agents consume. KA delegates every phase via A2A and collects results + execution traces.

| Phase | Execution | MCP Servers Consumed |
|-------|-----------|---------------------|
| `pre-investigation` | **Remote** | Customer-managed (CMDB, change-mgmt, etc.) |
| `investigation` | **Remote** | `k8s-investigation-tools` (pod status, logs, events), `prometheus-tools` (metrics, queries) |
| `post-investigation` | **Remote** | Customer-managed |
| `rca-resolution` | **Remote** | None (structuring only) |
| `workflow-selection` | **Remote** | `workflow-tools` (list/get workflows, list actions) |

In the target architecture, KA's role narrows to:

1. **Pipeline orchestration**: Resolve bundles, render templates, sequence phases, propagate context
2. **CRD lifecycle management**: Update `RemediationRequest` status, create child CRDs
3. **Audit trail assembly**: Collect execution traces from remote agents, store in unified audit trail
4. **Failure policy enforcement**: Apply `failClosed`/`failOpen` based on A2A task outcome

KA no longer runs LLM loops, manages tool registries, or holds Kubernetes RBAC for investigation tools. Each remote agent pod runs with its own ServiceAccount scoped to exactly the MCP servers it needs — enforcing least-privilege per phase.

The transition from v1.5 (hybrid) to v1.6+ (fully remote) is a breaking change — the inline execution path is removed and all bundles require `spec.agent`. This is acceptable since prompt bundles ship as a new feature in v1.5 with no pre-existing consumers. The v1alpha1 API version signals instability; breaking changes are expected before promotion to v1beta1.

**Prerequisites for target architecture**:

- KA's builtin tools extracted into MCP server deployments (k8s-investigation-tools, prometheus-tools, workflow-tools)
- A2A client in KA for task delegation and streaming (shared with #705)
- Execution trace artifact contract defined (see Section 9.5)

---

## 4. Skills: Discovery and Resolution

> **Scope note**: This section describes skill resolution for **inline execution** (v1.5 core phases). For **remote execution**, the remote agent manages its own skill resolution — `spec.skills` in the bundle manifest is informational metadata used for auditing, dependency tracking, and the `kubernaut bundle test` CLI (DG-5). In the target architecture (Section 3.4), all skill resolution moves to remote agents.

### 4.1 KA as Skill Consumer (Inline Execution)

KA is a **consumer** of MCP servers, not a lifecycle manager. The skill artifact tells KA:
- What tools exist (name, description, JSON Schema parameters)
- Where the MCP server is running (endpoint URL)

KA does **not** start, stop, or health-check MCP servers. The infrastructure that deploys and runs MCP servers is outside KA's scope. This aligns with the existing `MCPToolProvider` interface in `pkg/kubernautagent/tools/mcp/provider.go`, which discovers tools from a pre-existing endpoint.

### 4.2 Skill Resolution Flow

```
Prompt Bundle YAML
  |
  |- skills[0].ref: "registry.example.com/skills/cmdb-lookup@sha256:abc..."
  |- skills[1].ref: "builtin://get_namespaced_resource_context"
  |
  v
Skill Resolver
  |
  |- OCI ref -> Pull artifact from skills marketplace registry
  |              -> Extract tool definitions (name, description, parameters)
  |              -> Extract MCP server endpoint URL
  |              -> Register tools with endpoint in LLM tool set
  |
  |- builtin:// ref -> Look up tool in KA's internal registry
  |                     (pkg/kubernautagent/tools/registry)
  |
  v
Merged Tool Set for LLM Invocation
  (builtin tools + external MCP tools from skills)
```

### 4.3 Skill Caching

Skills are cached locally by OCI digest. Since digests are content-addressable, the cache is inherently immutable -- a given digest always resolves to the same artifact. Cache invalidation is not needed; cache eviction uses LRU with a configurable size limit.

### 4.4 Skill Artifact Format

The skill artifact format is **defined by the skills marketplace project** and inherited by KA. This proposal treats it as an opaque dependency. At minimum, KA expects a skill artifact to contain:

- Tool definitions: `name`, `description`, `parameters` (JSON Schema)
- MCP server endpoint: URL where the tools are served

The specific OCI media type, layer structure, and metadata schema are owned by the skills marketplace. KA will implement a `SkillResolver` interface that can be updated as the marketplace format stabilizes.

---

## 5. Template Data Contract (v1)

The template data contract defines what fields are available to Go templates in prompt bundles. This is the `v1alpha1` contract -- additive changes are non-breaking; field removal or rename requires a version bump.

### 5.1 Available Fields

```yaml
.Signal:
  Name: string              # Signal type (e.g., "OOMKilled")
  Namespace: string         # Kubernetes namespace
  Severity: string          # critical, warning, info
  Message: string           # Human-readable error message
  ResourceKind: string      # Pod, Deployment, StatefulSet, etc.
  ResourceName: string      # Kubernetes object name
  ClusterName: string       # Cluster identifier
  Environment: string       # production, staging, etc.
  Priority: string          # P1, P2, P3 (may be inferred from severity)
  RiskTolerance: string     # Low, Medium, High (may be inferred from severity)
  SignalSource: string      # alertmanager, kubernetes-events, etc.
  BusinessCategory: string  # Optional business categorization
  Description: string       # Extended description
  SignalMode: string        # reactive or proactive
  FiringTime: string        # When the signal started firing
  ReceivedTime: string      # When KA received the signal

.Enrichment:
  OwnerChain: string        # "Pod/foo -> ReplicaSet/foo-abc -> Deployment/foo"
  DetectedLabels: string    # "gitOpsManaged=true, hpaEnabled=true"
  HistoryRendered: string   # Pre-formatted remediation history block

.PriorPhaseOutputs: []PhaseOutput
  - Name: string            # Bundle name that produced this output
    Phase: string           # Phase name (e.g., "pre-investigation")
    Output: string          # Natural language output from the bundle

.Investigation:             # Available from post-investigation onward
  RCANarrative: string      # Full natural language diagnostic narrative from investigation phase
  RCASummary: string        # Structured RCA summary extracted by rca-resolution phase (available from workflow-selection onward)
```

### 5.2 Field Availability by Phase

| Field Group | pre-investigation | investigation | post-investigation | rca-resolution | workflow-selection |
|-------------|:-:|:-:|:-:|:-:|:-:|
| `.Signal` | Yes | Yes | Yes | Yes | Yes |
| `.Enrichment` | Yes | Yes | Yes | Yes | Yes |
| `.PriorPhaseOutputs` | No | Yes | Yes | Yes | Yes |
| `.Investigation` | No | No | Yes | Yes | Yes |

Referencing a field group not available for the current phase (e.g., `{{ .Investigation.RCANarrative }}` in `pre-investigation`) produces a hard error at template render time due to `missingkey=error`. The error message identifies the unavailable field and the phase, preventing silent misconfiguration.

### 5.3 Versioning Policy

- `v1alpha1`: Current iteration. Fields may be added freely. Fields will not be removed or renamed without advancing to `v1alpha2`.
- `v1beta1`: Promoted when the field set stabilizes after production usage. No removals or renames without a major version bump.
- `v1`: Stable. Full backward compatibility guaranteed.

The `apiVersion` field in the Prompt Bundle manifest tells KA which contract version the bundle expects. KA will support multiple contract versions concurrently during migration periods.

---

## 6. Output Propagation Model

### 6.1 Two Output Modes

| Phase Type | Output Mode | How Output is Captured | How Output Reaches Next Phase |
|------------|-------------|----------------------|-------------------------------|
| `pre-investigation`, `investigation`, `post-investigation` | Natural language | LLM's final text response (no `submit_result`) | Appended to `.PriorPhaseOutputs` as a `PhaseOutput` entry. For `investigation`, also stored in `.Investigation.RCANarrative` |
| `rca-resolution`, `workflow-selection` | Structured JSON | LLM calls `submit_result` tool with JSON payload | Parsed into `InvestigationResult`; `.Investigation.RCASummary` populated from the structured result for downstream phases |

### 6.2 Natural Language Propagation

When a `pre-investigation`, `investigation`, or `post-investigation` bundle completes, KA captures the result and wraps it:

```go
// Inline execution: capture LLM's final text response
PhaseOutput{
    Name:   bundle.Metadata.Name,    // e.g., "kubernaut-default-investigation"
    Phase:  bundle.Spec.Phase,       // e.g., "investigation"
    Output: llmResponse.Content,     // LLM's final text response
}

// Remote execution: capture A2A task result text
PhaseOutput{
    Name:   bundle.Metadata.Name,    // e.g., "acme-cmdb-precheck"
    Phase:  bundle.Spec.Phase,       // e.g., "pre-investigation"
    Output: a2aTaskResult.Text,      // NL result from A2A response
}
```

This is appended to `PriorPhaseOutputs` and available to all subsequent phases via the Go template `{{ range .PriorPhaseOutputs }}`.

For the `investigation` phase specifically, the natural language output is also stored in `.Investigation.RCANarrative`, giving the `rca-resolution` phase direct access to the full diagnostic narrative without needing to iterate over `PriorPhaseOutputs`.

### 6.3 Structured Output Propagation

The `rca-resolution` and `workflow-selection` phases use the existing `submit_result` tool and `InvestigationResult` parser. The `outputSchema` field in the manifest defines what the LLM must produce. KA validates the output against this schema at runtime.

The `rca-resolution` phase is the critical structuring step: it takes the free-form investigation narrative and produces the `InvestigationResult` that KA needs to drive re-enrichment (via `RemediationTarget`) and the rest of the pipeline (severity, confidence, human review signal).

### 6.4 Why Two Modes

The investigation phase is exploratory -- the LLM inspects pods, pulls logs, queries metrics, and reasons about what happened. Forcing structured JSON output at this stage would constrain the LLM's diagnostic reasoning. Natural language lets it produce a rich, free-form narrative.

The `rca-resolution` phase is the structuring boundary. It takes the narrative and extracts the fields that KA's runtime needs: remediation target, severity, confidence, signal name, contributing factors. This separation means customers can customize the investigation prompt freely without worrying about JSON schema compliance -- that concern lives in `rca-resolution`.

`workflow-selection` produces structured data that downstream controllers (AA, RO, WFE) consume programmatically for workflow execution.

---

## 7. Bundle Resolution and Loading

### 7.1 Bundle Sources

KA resolves prompt bundles from two sources, in priority order:

1. **Customer bundles**: OCI artifacts pulled from a configured registry. Referenced in KA's configuration.
2. **Built-in bundles**: Embedded in the KA binary as default YAML files. Used when no customer override exists for a required phase.

### 7.2 Override Semantics

- For `pre-investigation` and `post-investigation`: customer bundles are **additive**. Multiple bundles can execute in priority order.
- For `investigation`, `rca-resolution`, and `workflow-selection`: **exactly one bundle** executes per phase. A customer bundle with the same phase replaces the default. If multiple customer bundles declare the same core phase, KA rejects the configuration at pull-time with an error — the operator must resolve the conflict.

### 7.3 Configuration

```yaml
# kubernaut-agent config
promptBundles:
  registryURL: "registry.example.com/kubernaut-bundles"
  failurePolicy: failClosed   # global default: abort pipeline on bundle failure
  phaseTimeout: 120s           # optional: max aggregate time for all bundles in a single phase
  pullSecrets:
    - name: "registry-credentials"
  bundles:
    - ref: "registry.example.com/bundles/acme-cmdb-precheck@sha256:abc123..."
    - ref: "registry.example.com/bundles/acme-post-rca-enrichment@sha256:def456..."
    - ref: "registry.example.com/bundles/acme-custom-investigation@sha256:789abc..."
    - ref: "registry.example.com/bundles/optional-metrics-enrichment@sha256:fed987..."
      failurePolicy: failOpen  # per-bundle override: this enrichment is nice-to-have

skillsMarketplace:
  registryURL: "marketplace.kubernaut.ai/skills"
  pullSecrets:
    - name: "marketplace-credentials"
  cachePath: "/var/cache/kubernaut/skills"
  cacheSizeMB: 512
```

### 7.4 Pull and Cache Flow

```
KA Startup / Config Reload
  |
  v
For each configured bundle ref:
  1. Pull OCI artifact from registry (by digest)
  2. Extract YAML manifest
  3. Validate manifest (apiVersion, kind, required fields)
  4. Skill verification (depends on execution model):
     a. Inline bundles: builtin:// -> verify tool exists in internal registry;
        OCI digest -> pull skill artifact from skills marketplace, cache by digest
     b. Remote bundles: skip verification (skills are informational;
        the remote agent resolves its own tools)
  5. For remote bundles: optionally verify spec.agent.endpoint is reachable (health check)
  6. Register bundle in phase-ordered execution plan
  |
  v
Ready to execute investigation pipeline
```

---

## 8. Validation Strategy

### 8.1 Pull-Time Validation

When a bundle is pulled, KA validates:

| Check | Failure Action |
|-------|---------------|
| YAML parse | Reject bundle, log error, apply failure policy (see 8.3) |
| `apiVersion` supported | Reject if KA doesn't support the requested contract version |
| `kind` == `PromptBundle` | Reject |
| `spec.phase` is a known hook point | Reject |
| `spec.prompt` is a valid Go template | Reject (template parse error). Templates are parsed with `Option("missingkey=error")` -- any reference to an undefined field is a hard error, not a silent empty string |
| `spec.skills` references resolvable (inline bundles) | Reject if any OCI digest fails to pull or any `builtin://` tool is unknown. Skipped for remote bundles (skills are informational) |
| `spec.agent.endpoint` reachable (remote bundles) | Optional: KA performs a lightweight A2A health check at startup/config-reload. Failure logs a warning but does not reject the bundle (the agent may start later). Runtime failure applies the failure policy |
| `spec.outputSchema` present for structured phases | Reject if missing for `rca-resolution`, `workflow-selection` |
| `spec.outputSchema` compatible with required schema | Reject if required fields are missing from the customer's schema |
| Multiple bundles for same core phase | Reject configuration if more than one bundle declares `investigation`, `rca-resolution`, or `workflow-selection` |

### 8.2 Runtime Validation

After the LLM produces output via `submit_result`:

1. Validate the JSON against the bundle's `outputSchema`
2. If validation fails, log the error and attempt self-correction (existing `SelfCorrect` pattern from `investigator.go`)
3. If self-correction exhausts retries, apply the configured failure policy (see 8.3)
4. Record the failure event in the audit trail

### 8.3 Failure Policy

Bundle execution failures are governed by a configurable **failure policy**. The policy determines whether the investigation pipeline continues with defaults or aborts when a customer bundle fails.

| Policy | Behavior | Default |
|--------|----------|---------|
| `failClosed` | **Abort the investigation**. The `RemediationRequest` transitions to a terminal error state with a clear diagnostic message identifying the failed bundle and phase. Requires human review. | **Yes (default)** |
| `failOpen` | Fall back to the built-in default bundle for that phase. The investigation continues but the customer's SOP was not applied. A warning is recorded in the audit trail. | No |

**Why `failClosed` is the default**: Silently falling back to defaults produces an investigation that *looks* correct but skips the customer's SOPs. If a customer configured a CMDB pre-check to prevent remediation of decommissioned resources, falling back means that check never ran -- yet the pipeline proceeds as if everything is fine. This is worse than failing loudly, because the operator sees "investigation complete, workflow selected" without realizing their safety gate was skipped.

`failOpen` is available for non-critical hook points (e.g., a `pre-investigation` enrichment bundle where the investigation is still valid without it), but the customer must explicitly opt in.

The failure policy is configured per-bundle and globally:

```yaml
# Global default (applies to all bundles unless overridden)
promptBundles:
  failurePolicy: failClosed   # default

  bundles:
    # Per-bundle override
    - ref: "registry.example.com/bundles/acme-cmdb-precheck@sha256:abc123..."
      failurePolicy: failClosed   # explicit: abort if CMDB check fails

    - ref: "registry.example.com/bundles/optional-enrichment@sha256:def456..."
      failurePolicy: failOpen     # opt-in: this enrichment is nice-to-have
```

**`failOpen` behavior depends on whether the phase has a default bundle**:

- **Core phases** (`investigation`, `rca-resolution`, `workflow-selection`): KA always has a built-in default bundle. `failOpen` falls back to the default with full audit trail visibility.
- **Hook phases** (`pre-investigation`, `post-investigation`): No default bundle exists. `failOpen` **skips the phase entirely** and continues the pipeline. A warning is recorded in the audit trail noting the skipped SOP.

When `failClosed` is configured, the pipeline aborts regardless of whether a default exists.

---

## 9. Security Considerations

### 9.1 Prompt Injection Mitigation

Customer-authored prompts are rendered server-side by KA. The existing `sanitizeField()` function in `builder.go` applies injection pattern detection to signal data before it reaches templates. This protection extends to prompt bundles because template data flows through the same sanitization pipeline.

Prompt bundle content itself (the `spec.prompt` field) is trusted -- it is authored by the customer and pulled from their OCI registry. KA does not sanitize the prompt text, only the dynamic data injected into it.

### 9.2 OCI Artifact Integrity

**Current**: Skill and bundle references use content-addressable OCI digests (`@sha256:`), ensuring the artifact KA pulls is exactly what was referenced. Mutable tags are not permitted.

**Future (Design Gate DG-1)**: OCI signature verification via sigstore/cosign. This is a known requirement blocked on broader sigstore adoption within the project (workflow execution bundles face the same gap). When sigstore support lands for workflow bundles, the same verification infrastructure will apply to prompt bundles and skills.

### 9.3 Skill Endpoint Trust (Inline Execution)

> **Scope**: This section applies to **inline execution** (v1.5 core phases). In the target architecture (Section 3.4), MCP endpoint trust is the remote agent's responsibility — the agent pod's ServiceAccount, NetworkPolicy, and TLS configuration govern tool access. KA does not connect to MCP endpoints.

Skills declare an MCP server endpoint URL. For inline phases, KA connects to this endpoint to execute tool calls. The security of this connection depends on:

- **Network policy**: The MCP server must be reachable from the KA pod. Standard Kubernetes NetworkPolicy applies.
- **TLS**: MCP server endpoints should use TLS. KA's MCP client will verify TLS certificates using the cluster's CA bundle.
- **Authentication**: The skill artifact may declare an auth mechanism (bearer token, mTLS). KA will support configurable auth per skill endpoint.

### 9.4 Audit Trail

Every prompt bundle execution is recorded in the audit trail:

- Bundle name, phase, OCI digest
- Skill artifacts resolved and their digests
- LLM prompt (rendered template)
- LLM response (natural language or structured JSON)
- Validation result (pass/fail)
- Failure policy applied (`failClosed` -> pipeline aborted, `failOpen` -> fell back to default)
- Fallback events with reason (if `failOpen` was configured and default was used)

### 9.5 Remote Execution Audit Contract

When a bundle executes remotely via A2A delegation, KA loses real-time visibility into the agent's intermediate steps (tool calls, LLM reasoning, retries). To maintain audit compliance, remote agents are expected to return an **execution trace** as a structured A2A artifact alongside their result. Enforcement follows the bundle's failure policy.

**Execution Trace Contract**:

The A2A task response must include an artifact with media type `application/vnd.kubernaut.execution-trace.v1+json` containing:

```json
{
  "bundle": "acme-cmdb-precheck",
  "phase": "pre-investigation",
  "agent": "http://acme-cmdb-agent.acme-sops.svc:8080",
  "started_at": "2026-04-15T10:00:00Z",
  "completed_at": "2026-04-15T10:00:12Z",
  "llm_provider": "openai/gpt-4o-mini",
  "token_usage": { "prompt": 1200, "completion": 350, "total": 1550 },
  "tool_calls": [
    {
      "tool": "cmdb_lookup",
      "input": { "resource_kind": "Pod", "resource_name": "foo-pod", "namespace": "production" },
      "output": { "status": "active", "owner": "team-platform", "decommission_date": null },
      "duration_ms": 230
    }
  ],
  "llm_responses": [
    {
      "role": "assistant",
      "content": "Resource is registered in CMDB and available for remediation",
      "finish_reason": "stop"
    }
  ]
}
```

**Audit enforcement** (follows the bundle's failure policy):

| Scenario | `failClosed` (default) | `failOpen` |
|----------|----------------------|------------|
| Execution trace present and valid | KA stores trace in audit trail alongside its own orchestration events | Same |
| Execution trace missing | **Abort the pipeline**. A missing trace means KA cannot verify what the agent did — the investigation is unauditable. | Log warning, accept result, record "trace unavailable" in audit trail |
| Execution trace malformed | **Abort the pipeline**. Malformed trace indicates agent incompatibility. | Log warning, store raw artifact as-is with "parse error" annotation |

**A2A protocol version**: KA requires A2A protocol version >= 1.0. The execution trace and task input contracts use A2A's standard artifact mechanism. Version negotiation follows A2A's built-in capability discovery.

**Real-time streaming (Kubernaut Console)**: For live investigation visibility (#713), remote agents can emit A2A streaming updates during execution. KA forwards these events to connected console clients. This is optional — the execution trace is the mandatory post-hoc audit record; streaming is a UX enhancement.

### 9.6 Remote Agent Security Isolation

Remote bundle execution provides defense-in-depth through Kubernetes-native isolation:

- **ServiceAccount per agent**: Each remote agent pod runs with its own SA, scoped to only the MCP servers it needs. A CMDB pre-check agent has no Kubernetes RBAC. An investigation agent (target architecture) has read-only access to pods, events, and logs. No single component holds all permissions.
- **NetworkPolicy**: Each agent pod's network access is restricted to its declared MCP server endpoints. The CMDB agent can reach the CMDB MCP server but not Prometheus. The investigation agent can reach k8s-tools and prometheus-tools but not the CMDB.
- **Resource limits**: Each agent pod has its own CPU/memory limits. A misbehaving LLM loop in one phase cannot starve other phases or KA.
- **Namespace isolation**: Customer SOP agents can run in a dedicated namespace (e.g., `acme-sops`) with their own RBAC, secrets, and network policies, separate from the `kubernaut-system` namespace.

KA does not manage the lifecycle of remote agent pods. The customer (or their platform team) deploys and maintains the agent infrastructure. KA's only contract with the agent is the A2A protocol: send a task, receive a result + execution trace.

### 9.7 A2A Task Input Contract

When KA delegates a bundle to a remote agent, the A2A task payload contains structured data that gives the agent full context for execution. This is the input counterpart to the execution trace output contract (Section 9.5).

**Task payload** (media type `application/vnd.kubernaut.bundle-task.v1+json`):

```json
{
  "bundle": {
    "name": "acme-cmdb-precheck",
    "phase": "pre-investigation",
    "oci_digest": "sha256:abc123..."
  },
  "rendered_prompt": "You are a pre-investigation assistant for Kubernaut.\n\nA critical signal ...",
  "signal_context": {
    "name": "OOMKilled",
    "namespace": "production",
    "severity": "critical",
    "resource_kind": "Pod",
    "resource_name": "foo-pod",
    "cluster_name": "prod-east-1",
    "message": "Container exceeded memory limit"
  },
  "enrichment_context": {
    "owner_chain": "Pod/foo-pod -> ReplicaSet/foo-abc -> Deployment/foo",
    "detected_labels": "gitOpsManaged=true"
  },
  "prior_phase_outputs": [
    {
      "name": "acme-change-freeze-check",
      "phase": "pre-investigation",
      "output": "No active change freeze for namespace production"
    }
  ],
  "output_schema": null,
  "timeout": "30s"
}
```

**Field descriptions**:

| Field | Present | Description |
|-------|---------|-------------|
| `bundle` | Always | Bundle metadata (name, phase, digest) for traceability |
| `rendered_prompt` | Always | The Go template rendered with the current context. This is the agent's primary instruction |
| `signal_context` | Always | Structured signal data. Gives the agent programmatic access to signal fields beyond what the prompt author included in the template |
| `enrichment_context` | Always | Structured enrichment data (owner chain, labels, history) |
| `prior_phase_outputs` | When available | NL outputs from earlier phases. Empty array for `pre-investigation` |
| `output_schema` | Structured phases only | JSON Schema that the agent must satisfy via `submit_result`. Null for NL phases. Present for `rca-resolution` and `workflow-selection` in the target architecture |
| `timeout` | Always | Maximum execution time. The agent should self-terminate if exceeded |

The `signal_context` and `enrichment_context` fields ensure the remote agent has structured access to all signal data, not just what the prompt template author chose to include. This is important for agents that need to make programmatic decisions (e.g., route to different tools based on `resource_kind`).

---

## 10. Work Estimation

### 10.1 Implementation Scope

| Item | Estimate | Notes |
|------|----------|-------|
| Prompt Bundle YAML schema + parser | 1w | `apiVersion`, `kind`, field validation, Go template parse |
| Bundle OCI puller + cache | 1.5w | OCI client, digest-based cache, pull secret support |
| Skill resolver (OCI pull + builtin lookup) | 1w | Integrate with existing `MCPToolProvider` interface |
| Template data contract types | 0.5w | `PhaseOutput`, extended template data struct |
| Phase execution orchestrator | 2w | Replace hardcoded `runRCA`/`runWorkflowSelection` with phase-driven loop. Inline executor for core phases, A2A delegator for hook phases |
| A2A task delegation (pre/post hooks) | 1.5w | A2A client for remote bundle execution, execution trace collection, timeout handling |
| Output propagation (natural language + structured) | 1w | `PriorPhaseOutputs` accumulation, `.Investigation` population |
| Pull-time + runtime validation | 1w | Schema compatibility check, failure policy (failClosed/failOpen), per-bundle override |
| Configuration (Helm, config YAML) | 0.5w | `promptBundles` and `skillsMarketplace` config sections |
| Built-in bundle extraction | 1w | Extract current `.tmpl` files into embedded YAML bundles |
| Audit trail integration | 1w | New audit event types for bundle execution, execution trace ingestion from remote agents |
| Unit tests (Ginkgo/Gomega) | 2w | Bundle parsing, template rendering, phase orchestration, validation, failure policy (failClosed/failOpen) |
| Integration tests | 1.5w | End-to-end bundle resolution, skill discovery, phase execution with mock LLM |

### 10.2 Totals

| Scope | 1 Developer | 2 Developers (parallel) |
|-------|-------------|------------------------|
| v1.5 (hybrid: inline core + remote hooks) | 16 weeks | 9.5 weeks |

**Note**: There is no separate MVP scope because the phase executor is a single reusable component. The A2A delegation for remote hooks adds ~2.5 weeks (A2A client + execution trace ingestion) over the original estimate. Once `executeBundlePhase(phase, bundles, context)` works for one hook point, wiring it to the remaining four is configuration. The inline and remote code paths share the same orchestration logic — they differ only in how the LLM loop is invoked (in-process vs A2A task).

**Target architecture (v1.6+) additional effort**:

| Item | Estimate | Notes |
|------|----------|-------|
| Extract KA builtin tools into MCP servers | 3-4w | k8s-investigation-tools, prometheus-tools, workflow-tools as separate deployments |
| Migrate core phases to remote execution | 1w | Add `spec.agent` to built-in bundles, point at MCP-backed agents |
| Remove inline execution path from KA | 0.5w | KA becomes pure orchestrator |

### 10.3 Dependencies

- **Skills marketplace**: Must define and publish the skill artifact format before KA can implement the skill resolver. The `SkillResolver` interface will be designed to accommodate format changes.
- **PROPOSAL-EXT-001**: The MCP interactive mode (#703) and prompt bundle system share the `MCPToolProvider` infrastructure. MCP interactive mode can proceed independently -- prompt bundles extend, not replace, the MCP integration.
- **A2A client (#705)**: Required for remote bundle execution. The A2A task delegation pattern is shared between prompt bundles (KA → customer agent) and inbound A2A requests (external agent → KA). The same A2A client implementation serves both.
- **MCP server extraction (target architecture)**: Extracting KA's builtin tools into standalone MCP servers is a prerequisite for fully remote execution. This is independently valuable (K8s investigation tools as MCP servers are useful beyond Kubernaut) and can proceed in parallel with v1.5 development.

### 10.4 Risks

1. **Skills marketplace format instability** -- The marketplace project is actively defining its artifact format. KA's `SkillResolver` must be designed as an interface that can adapt to format changes without requiring prompt bundle manifest changes.
2. **Template complexity** -- Customer-authored Go templates may produce unexpected output. Mitigated by `missingkey=error` (typos and undefined fields produce hard errors, not silent empty strings) and pull-time template parsing (catches syntax errors). Semantic issues (e.g., referencing `.Investigation.RCASummary` in `pre-investigation` where it's not yet populated) are caught at render time since the field is nil. Full semantic validation deferred to `kubernaut bundle test` (DG-5).
3. **LLM compliance with custom schemas** -- Custom `outputSchema` values may be harder for the LLM to follow than Kubernaut's well-tested defaults. The self-correction mechanism mitigates this. Under `failClosed` (default), schema non-compliance after retries aborts the pipeline -- customers must validate their schemas thoroughly. Under `failOpen`, the built-in bundle takes over, but the customer's intent is lost.

---

## 11. Evolution Path

| Version | Capability | Notes |
|---------|-----------|-------|
| **v1.5** | Hybrid execution: inline core phases + remote pre/post hooks via A2A | Phase executor is a single reusable component. Pre/post-investigation hooks always delegate to customer-managed A2A agents. Core phases (investigation, rca-resolution, workflow-selection) execute inline. `failClosed` default. |
| **v1.5** | Skills marketplace integration | External skill resolution via OCI pull for inline phases. Requires marketplace format to be stable. |
| **v1.5** | Execution trace contract | Remote agents return structured execution traces (tool calls, LLM responses, token usage) as A2A artifacts. KA ingests these into the unified audit trail. |
| **v1.6** | KA builtin tools extracted into MCP servers | `k8s-investigation-tools`, `prometheus-tools`, `workflow-tools` as standalone deployments. Prerequisite for fully remote execution. |
| **v1.6** | **Target architecture: fully remote execution** | All phases delegate to A2A agents. KA becomes a pure pipeline orchestrator + CRD lifecycle manager. Each agent pod runs with its own SA, enforcing least-privilege per phase. |
| **v1.6** | `kubernaut bundle test` CLI | Validates a bundle against synthetic signal data with a mock or real LLM. Verifies: template renders, declared skills are called, output meets expectations. |
| **v1.6+** | OCI signature verification (sigstore) | Blocked on sigstore adoption for workflow bundles. Same infrastructure reused. |
| **v1.6+** | Agent marketplace integration | A2A agent artifacts as skill-like dependencies. |
| **v1.6+** | Prompt Bundle marketplace | Customers publish and share prompt bundles via a dedicated marketplace. |

### Design Gates

| Gate | Question | Status |
|------|----------|--------|
| **DG-1: OCI signature verification** | How do we verify prompt bundle and skill artifact signatures? | **Deferred** -- blocked on sigstore adoption for workflow bundles. Digests provide integrity; signatures add authenticity. |
| **DG-2: Template data contract versioning** | How do we evolve the template data contract without breaking existing bundles? | **Resolved** -- `apiVersion` field in manifest. Additive changes in `v1alpha1`, breaking changes require version bump. Multiple versions supported concurrently. |
| **DG-3: OCI media type for prompt bundles** | What media type identifies prompt bundle artifacts in OCI registries? | **Resolved** -- `application/vnd.kubernaut.promptbundle.v1+yaml`. |
| **DG-4: Core phase override safety** | How do we prevent customer bundles from breaking required phases? | **Resolved** -- Pull-time schema compatibility check + runtime validation with self-correction + configurable failure policy (`failClosed` default aborts pipeline; `failOpen` falls back to built-in bundle). |
| **DG-5: Bundle testing** | How do customers validate bundles before deployment? | **Deferred** to v1.6 -- `kubernaut bundle test` CLI that runs a bundle against synthetic signal data, verifies template rendering, skill invocation, and output shape. Without this, semantic validation (prompt produces correct behavior) only happens at runtime. |
| **DG-6: Remote execution audit completeness** | How does KA maintain full audit trail when phases execute on remote agents? | **Resolved** -- Remote agents must return an `execution-trace` artifact (media type `application/vnd.kubernaut.execution-trace.v1+json`) containing tool calls, LLM responses, and token usage. KA ingests this into the unified audit trail. See Section 9.5. |

---

## Appendix A: Current Architecture Integration Points

The following existing code locations are the primary integration points for prompt bundles:

| Component | File | Integration |
|-----------|------|-------------|
| Prompt rendering | `internal/kubernautagent/prompt/builder.go` | `Builder` gains a `RenderBundle(bundle, data)` method alongside existing `RenderInvestigation` and `RenderWorkflowSelection` |
| Template data types | `internal/kubernautagent/prompt/builder.go` | `SignalData` and `EnrichmentData` are wrapped in a `BundleTemplateData` struct that adds `PriorPhaseOutputs` and `Investigation` |
| Investigation orchestration | `internal/kubernautagent/investigator/investigator.go` | `Investigate()` method refactored to iterate over phase-ordered bundles instead of calling `runRCA` and `runWorkflowSelection` directly |
| Tool resolution | `internal/kubernautagent/investigator/types.go` | `DefaultPhaseToolMap()` extended to merge bundle-declared skills into phase tool sets |
| MCP tool discovery | `pkg/kubernautagent/tools/mcp/provider.go` | `MCPToolProvider` used for external skill MCP server connections |
| MCP server config | `pkg/kubernautagent/tools/mcp/config.go` | `ServerConfig` reused for skill endpoint configuration |
| LLM loop (inline phases) | `internal/kubernautagent/investigator/investigator.go` | `runLLMLoop()` unchanged for inline execution -- receives merged tool definitions from the phase orchestrator |
| A2A delegation (remote phases) | `pkg/a2a/client/` (new) | A2A task client for delegating pre/post-investigation hooks to remote agents. Shared with #705 inbound A2A support |
| Execution trace ingestion | `internal/kubernautagent/audit/` | New audit event types for ingesting execution traces from remote agents into the unified audit trail |

## Appendix B: Design Rationale -- The WAR File Analogy

The PromptBundle is a novel artifact in the LLM agent ecosystem. While components exist -- OCI for distribution, tool definitions (e.g., in Agent Skills OCI), prompt content itself -- no existing standard combines a prompt, its required tool/skill dependencies, the expected output contract, and pipeline metadata into a single, deployable, OCI-packaged unit for agentic execution.

To understand the PromptBundle's role, consider the analogy of a Java Web Archive (WAR) file in an application server (e.g., Tomcat, JBoss):

| Concept | Java WAR File Analogy | PromptBundle Analogy |
|---|---|---|
| **Deployable Unit** | WAR file (`.war`) | PromptBundle OCI artifact |
| **Contains** | Web application code (`.java`, `.jsp`, `.js`), libraries (`.jar`), configuration (`web.xml`) | Prompt template, skill references, output schema, pipeline hooks |
| **Runtime** | Java Virtual Machine (JVM) | Large Language Model (LLM) |
| **Application Server**| Tomcat, JBoss (provides environment, manages lifecycle) | v1.5: KA (executes inline phases). Target: DocsClaw or similar lightweight runtime (execution environment); KA serves as the deployment orchestrator |
| **Dependencies** | Shared JARs in app server's lib | Built-in tools, OCI-packaged skills (from marketplace) |
| **Execution Flow** | Servlet invocation, JSP rendering | Prompt injection, tool execution, output parsing |
| **Goal** | Standardized, portable web app deployment | Standardized, portable agentic behavior definition |

**Why not existing formats?**

*   **Agent Skills OCI (e.g., from A2A Project)**: Defines tools/skills (schemas, descriptions, endpoints) and their distribution as OCI artifacts. This is analogous to a `.jar` library. A PromptBundle *consumes* these skills, but doesn't define the prompt content or output contract.
*   **A2A Agent Card**: Describes an agent's capabilities and services it exposes, similar to a `service.yaml` in Kubernetes or a WSDL. It doesn't contain the specific prompts for *how* the agent should operate internally, nor its skill dependencies.
*   **MCP Prompts (e.g., raw prompt text)**: This is just the "code" or "script" (`.java` or `.jsp` equivalent). It lacks packaging, metadata, dependencies, and output contracts required for robust, shareable, and verifiable agentic behavior.

The PromptBundle bridges this gap by combining these elements into a single, deployable artifact that defines a specific LLM interaction pattern within the Kubernaut Agent's pipeline.

**Complementary runtimes**: Lightweight agentic runtimes like [DocsClaw](https://redhat-et.github.io/docsclaw/docsclaw-intro.html) (Red Hat OCTO, 5 MiB per pod, ConfigMap-driven, A2A native) are natural execution targets for prompt bundles. A customer deploys a DocsClaw pod configured with the bundle's prompt and skills, exposes it as an A2A endpoint, and KA delegates to it. DocsClaw independently arrived at the same "same binary, different config = different agent" pattern that validates the prompt-free KA principle. In the target architecture (Section 3.4), DocsClaw or similar lightweight runtimes serve as the execution layer for all phases, while KA focuses on pipeline orchestration and CRD lifecycle management.

## Appendix C: Adversarial Review and Design Decisions

This design was subjected to adversarial review to identify weaknesses. The following critiques were raised and resolved:

### Critique 1: Silent Fallback Masks SOP Bypass (Severity: High)

**Concern**: The original design defaulted to `failOpen`, silently falling back to built-in bundles when customer bundles failed. This means a CMDB pre-check could fail silently, and the pipeline would proceed as if the check never existed -- producing an investigation that *looks* correct but skipped the customer's safety gate.

**Resolution**: Default failure policy changed to `failClosed`. The pipeline aborts on bundle failure, producing a clear error in the `RemediationRequest` status. `failOpen` is available as an explicit opt-in for non-critical enrichment bundles. See Section 8.3.

### Critique 2: LLM Cost Multiplier

**Concern**: Five sequential LLM invocations (one per phase) multiplies cost and latency vs. the current single-shot approach.

**Assessment**: Dismissed as a design concern. The phase model is correct -- cost is a deployment consideration. Customers who don't configure pre/post hooks execute the same number of LLM calls as today (investigation + rca-resolution + workflow-selection vs. current RCA + workflow-selection, plus one new rca-resolution call that is a low-cost structuring pass with no tool calls). Pre/post hooks are opt-in and customers accept the cost trade-off for SOP compliance.

### Critique 3: Reusable Phase Executor

**Concern**: Five hook points suggested overengineering; an MVP should start with one or two phases.

**Assessment**: Dismissed. The implementation is a single reusable `executeBundlePhase(phase, bundles, context)` function: download bundle, unpack, resolve skills, render template, run LLM with skills, capture output, append to context. Wiring this function to five hook points is configuration, not new code. The effort is in the executor, not the number of phases. See Section 10.2.

### Critique 4: Go Template Power Gives Customers Enough Rope

**Concern**: Go templates are Turing-complete (`{{if}}`, `{{range}}`, function maps). A customer could write a template that panics KA or produces unexpected behavior.

**Status**: Mitigated. Templates are rendered with `Option("missingkey=error")`, which turns undefined field references (typos like `{{ .Signal.Namspace }}`) into hard errors instead of silent empty strings. Combined with pull-time template parsing (syntax errors) and a restricted function map (no arbitrary function calls), the attack surface is limited to valid template operations over the declared data contract. Full semantic validation deferred to `kubernaut bundle test` CLI (see DG-5).

### Critique 5: Non-Deterministic Output from Prompt Bundles

**Concern**: Prompts are probabilistic. The same bundle, same data, same LLM can produce different natural language outputs. OCI digests guarantee the prompt didn't change, but guarantee nothing about what it will produce. The deterministic infrastructure (OCI, schemas, digests) creates a false sense of safety over a non-deterministic artifact.

**Assessment**: This critique applies a wrong mental model to the NL phases (`pre-investigation`, `investigation`, `post-investigation`). These phases produce **context for another LLM**, not output for a parser or a human decision gate. "Resource is registered in CMDB and available" vs. "CMDB confirms foo-pod is active" are semantically equivalent to the consuming LLM. The design is **mandatory context injection**: instead of hoping the investigation LLM will discover and call the CMDB tool (it might skip it), the pre-investigation phase forces the call, captures the result, and delivers it as given context (RAG). The investigation LLM sees it as fact, not as an optional tool. Non-determinism of phrasing is irrelevant when the consumer understands natural language.

Initially this critique was thought to hold for `rca-resolution`, where natural language is structured into JSON. However, the structuring mechanism is `submit_result` -- a **tool call**, not raw JSON output. Tool calling is a first-class LLM capability with typed parameter schemas, supported by virtually all models (unlike native structured/JSON output, which is model-dependent). The rca-resolution LLM reads the investigation narrative and calls `submit_result` with the `InvestigationResult` schema -- the same tool-calling pattern the investigation phase uses for `get_pod_logs` or `query_prometheus`. This is not NL-to-JSON-via-parsing; it is NL-to-tool-call, which is a well-proven pattern already working in the current KA codebase. Additional safety: `outputSchema` validation + self-correction retries + `failClosed` if structuring fails.

### Critique 6: No Customer Testing Story

**Concern**: Customers have no way to validate a prompt bundle before deployment. Pull-time validation catches syntax but not semantics. The first semantic failure happens at runtime.

**Status**: Accepted. `kubernaut bundle test` CLI is planned for v1.6 (see DG-5, Evolution Path). The expected contract: a bundle test validates that the template renders, the declared skills are called, and the output meets expected shape/patterns. Until then, customers validate in staging environments.

## Appendix D: Glossary

| Term | Definition |
|------|------------|
| **A2A** | Agent-to-Agent protocol -- standard for inter-agent communication and task delegation |
| **CMDB** | Configuration Management Database -- customer infrastructure inventory |
| **DocsClaw** | Lightweight agentic runtime from Red Hat OCTO (5 MiB per pod, ConfigMap-driven, A2A native). A complementary execution runtime for prompt bundles |
| **Execution Trace** | Structured record of a remote agent's intermediate steps (tool calls, LLM responses, token usage) returned as an A2A artifact |
| **failClosed** | Failure policy that aborts the pipeline when a bundle fails. Default behavior |
| **failOpen** | Failure policy that skips the failed bundle (hook phases) or falls back to the default bundle (core phases) |
| **Inline Execution** | Bundle execution mode where KA runs the LLM loop in-process. Used for core phases in v1.5 |
| **KA** | Kubernaut Agent -- the investigation pipeline orchestrator |
| **LRU** | Least Recently Used -- cache eviction strategy |
| **MCP** | Model Context Protocol -- protocol for connecting AI agents to tools and data sources |
| **OCI** | Open Container Initiative -- standards for container image and artifact distribution |
| **RCA** | Root Cause Analysis -- the investigation output identifying what caused the incident |
| **Remote Execution** | Bundle execution mode where KA delegates to an external A2A agent. Used for hook phases in v1.5, all phases in target architecture |
| **SOP** | Standard Operating Procedure -- customer-defined investigation procedures |
