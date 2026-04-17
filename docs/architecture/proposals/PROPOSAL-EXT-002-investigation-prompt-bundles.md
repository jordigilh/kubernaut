# PROPOSAL-EXT-002: Investigation Prompt Bundles

**Status**: PROPOSAL (under review)
**Date**: April 15, 2026
**Author**: Kubernaut Architecture Team
**Confidence**: 99% (all design gates resolved)
**Related**: [#711](https://github.com/jordigilh/kubernaut/issues/711) (Investigation Prompt Bundles), [PROPOSAL-EXT-001](PROPOSAL-EXT-001-external-integration-strategy.md) (External Integration Strategy), [DD-016](../decisions/DD-016-dynamic-toolset-v2-deferral.md) (Dynamic Toolset Deferral)

---

## Purpose

This proposal defines how customers inject their Standard Operating Procedures (SOPs) into Kubernaut Agent's investigation pipeline. Prompts and skill dependencies are packaged as OCI artifacts called **Prompt Bundles**, enabling a standardized, versionable, and distributable mechanism for customizing the investigation flow.

The design makes KA **prompt-free by default**: every prompt -- including Kubernaut's own built-in investigation and workflow selection prompts -- is an OCI-packaged Prompt Bundle. Customer bundles override or extend the defaults at well-defined hook points.

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
| **Prompt-Free KA** | The principle that KA ships without hardcoded prompts. All prompts are OCI bundles -- Kubernaut's defaults are built-in bundles that customers can override |

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
  prompt: |
    <Go template with access to .Signal, .Enrichment, .PriorPhaseOutputs, .Investigation>
  skills:
    - ref: "registry.example.com/skills/cmdb-lookup@sha256:abc123..."
    - ref: "registry.example.com/skills/change-management@sha256:def456..."
    - ref: "builtin://get_namespaced_resource_context"
    - ref: "builtin://get_cluster_resource_context"
  outputSchema:    # required for investigation, rca-resolution, workflow-selection
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
| `spec.phase` | Yes | The hook point where this bundle executes |
| `spec.prompt` | Yes | Go template string. Has access to the template data contract fields for the bundle's phase |
| `spec.skills` | No | List of skill references. OCI digests (`@sha256:`) for external skills, `builtin://` for KA's internal tools |
| `spec.outputSchema` | Conditional | Required for `rca-resolution` and `workflow-selection` phases. JSON Schema enforced via `submit_result` tool. Omitted for `pre-investigation`, `investigation`, and `post-investigation` (natural language output) |

### 2.3 Skill Reference Format

Skill references use two schemes:

| Scheme | Format | Example | Resolution |
|--------|--------|---------|------------|
| **OCI digest** | `<registry>/<repo>@sha256:<digest>` | `registry.example.com/skills/cmdb-lookup@sha256:abc123...` | Pulled from skills marketplace OCI registry |
| **Builtin** | `builtin://<tool-name>` | `builtin://get_namespaced_resource_context` | Resolved from KA's internal tool registry |

OCI digests are mandatory for external skills to guarantee immutability. Mutable tags (`:latest`, `:v1`) are not permitted in skill references.

### 2.4 Example: Pre-Investigation CMDB Check

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

### 2.5 Example: Investigation Bundle (Default)

This is how the current `incident_investigation.tmpl` would look as a Prompt Bundle:

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

### 2.6 Example: RCA Resolution Bundle (Default)

This phase takes the free-form investigation narrative and structures it into the `InvestigationResult` that KA needs. It must handle all four investigation outcomes, not just actionable RCAs.

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

### 2.7 Example: Workflow Selection Bundle (Default)

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
3. For each bundle:
   a. Pulls and caches any referenced skill artifacts from the skills marketplace
   b. Registers the skill tools in the LLM tool set for that invocation
   c. Renders the Go template with the current template data context
   d. Executes the LLM loop (system prompt + tool calls)
   e. Captures the output (natural language or structured JSON)
   f. Appends the output to `PriorPhaseOutputs` for subsequent phases
4. If a phase has no bundles (e.g., no `pre-investigation` configured), it is skipped

### 3.3 Mapping to Current Investigator Architecture

The current `Investigator.Investigate()` method in `internal/kubernautagent/investigator/investigator.go` executes two LLM invocations:

| Current Method | Current Phase | New Phase |
|---------------|---------------|-----------|
| `runRCA()` | Phase 1 (RCA) | `investigation` |
| `runWorkflowSelection()` | Phase 3 (Workflow Selection) | `workflow-selection` |

The prompt bundle system wraps these invocations:

```
[pre-investigation bundles]   <-- NEW: customer hooks
    |
    v (natural language outputs -> PriorPhaseOutputs)
[investigation bundle]         <-- replaces runRCA() system prompt
    |
    v (natural language narrative -> PriorPhaseOutputs + .Investigation.RCANarrative)
[post-investigation bundles]   <-- NEW: customer hooks
    |
    v (natural language outputs -> PriorPhaseOutputs)
[rca-resolution bundle]        <-- NEW: structures the narrative into InvestigationResult
    |
    v (structured InvestigationResult -> drives re-enrichment, severity, remediation target)
[workflow-selection bundle]    <-- replaces runWorkflowSelection() system prompt
    |
    v (structured InvestigationResult with workflow selection)
```

The `runLLMLoop()` mechanics (tool execution, audit events, anomaly detection, summarization) remain unchanged. The prompt bundle system only changes **what prompt** is rendered and **which tools** are available in each invocation.

---

## 4. Skills: Discovery and Resolution

### 4.1 KA as Skill Consumer

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

When a `pre-investigation`, `investigation`, or `post-investigation` bundle completes, KA captures the LLM's final text response and wraps it:

```go
PhaseOutput{
    Name:   bundle.Metadata.Name,    // e.g., "acme-cmdb-precheck"
    Phase:  bundle.Spec.Phase,       // e.g., "pre-investigation"
    Output: llmResponse.Content,     // e.g., "Resource is registered in CMDB and available"
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
- For `investigation`, `rca-resolution`, and `workflow-selection`: a customer bundle with the same phase **replaces** the default. Only one bundle executes per required phase.

### 7.3 Configuration

```yaml
# kubernaut-agent config
promptBundles:
  registryURL: "registry.example.com/kubernaut-bundles"
  pullSecrets:
    - name: "registry-credentials"
  bundles:
    - ref: "registry.example.com/bundles/acme-cmdb-precheck@sha256:abc123..."
    - ref: "registry.example.com/bundles/acme-post-rca-enrichment@sha256:def456..."
    - ref: "registry.example.com/bundles/acme-custom-investigation@sha256:789abc..."

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
  4. For each skill ref in spec.skills:
     a. builtin:// -> verify tool exists in internal registry
     b. OCI digest -> pull skill artifact from skills marketplace, cache by digest
  5. Register bundle in phase-ordered execution plan
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
| YAML parse | Reject bundle, log error, fall back to default |
| `apiVersion` supported | Reject if KA doesn't support the requested contract version |
| `kind` == `PromptBundle` | Reject |
| `spec.phase` is a known hook point | Reject |
| `spec.prompt` is a valid Go template | Reject (template parse error) |
| `spec.skills` references resolvable | Reject if any OCI digest fails to pull or any `builtin://` tool is unknown |
| `spec.outputSchema` present for structured phases | Reject if missing for `rca-resolution`, `workflow-selection` |
| `spec.outputSchema` compatible with required schema | Reject if required fields are missing from the customer's schema |

### 8.2 Runtime Validation

After the LLM produces output via `submit_result`:

1. Validate the JSON against the bundle's `outputSchema`
2. If validation fails, log the error and attempt self-correction (existing `SelfCorrect` pattern from `investigator.go`)
3. If self-correction exhausts retries, fall back to the default built-in bundle for that phase
4. Record the fallback event in the audit trail

### 8.3 Fallback Guarantee

For the three required phases (`investigation`, `rca-resolution`, `workflow-selection`), KA always has a fallback: the built-in default bundle. A customer bundle failure never breaks the pipeline -- it degrades to default behavior with full audit trail visibility.

---

## 9. Security Considerations

### 9.1 Prompt Injection Mitigation

Customer-authored prompts are rendered server-side by KA. The existing `sanitizeField()` function in `builder.go` applies injection pattern detection to signal data before it reaches templates. This protection extends to prompt bundles because template data flows through the same sanitization pipeline.

Prompt bundle content itself (the `spec.prompt` field) is trusted -- it is authored by the customer and pulled from their OCI registry. KA does not sanitize the prompt text, only the dynamic data injected into it.

### 9.2 OCI Artifact Integrity

**Current**: Skill and bundle references use content-addressable OCI digests (`@sha256:`), ensuring the artifact KA pulls is exactly what was referenced. Mutable tags are not permitted.

**Future (Design Gate DG-1)**: OCI signature verification via sigstore/cosign. This is a known requirement blocked on broader sigstore adoption within the project (workflow execution bundles face the same gap). When sigstore support lands for workflow bundles, the same verification infrastructure will apply to prompt bundles and skills.

### 9.3 Skill Endpoint Trust

Skills declare an MCP server endpoint URL. KA connects to this endpoint to execute tool calls. The security of this connection depends on:

- **Network policy**: The MCP server must be reachable from the KA pod. Standard Kubernetes NetworkPolicy applies.
- **TLS**: MCP server endpoints should use TLS. KA's MCP client will verify TLS certificates using the cluster's CA bundle.
- **Authentication**: The skill artifact may declare an auth mechanism (bearer token, mTLS). KA will support configurable auth per skill endpoint.

### 9.4 Audit Trail

Every prompt bundle execution is recorded in the audit trail:

- Bundle name, phase, OCI digest
- Skill artifacts resolved and their digests
- LLM prompt (rendered template)
- LLM response (natural language or structured JSON)
- Validation result (pass/fail/fallback)
- Fallback events (if customer bundle failed and default was used)

---

## 10. Work Estimation

### 10.1 Implementation Scope

| Item | Estimate | Notes |
|------|----------|-------|
| Prompt Bundle YAML schema + parser | 1w | `apiVersion`, `kind`, field validation, Go template parse |
| Bundle OCI puller + cache | 1.5w | OCI client, digest-based cache, pull secret support |
| Skill resolver (OCI pull + builtin lookup) | 1w | Integrate with existing `MCPToolProvider` interface |
| Template data contract types | 0.5w | `PhaseOutput`, extended template data struct |
| Phase execution orchestrator | 2w | Replace hardcoded `runRCA`/`runWorkflowSelection` with phase-driven loop |
| Output propagation (natural language + structured) | 1w | `PriorPhaseOutputs` accumulation, `.Investigation` population |
| Pull-time + runtime validation | 1w | Schema compatibility check, fallback logic |
| Configuration (Helm, config YAML) | 0.5w | `promptBundles` and `skillsMarketplace` config sections |
| Built-in bundle extraction | 1w | Extract current `.tmpl` files into embedded YAML bundles |
| Audit trail integration | 0.5w | New audit event types for bundle execution |
| Unit tests (Ginkgo/Gomega) | 2w | Bundle parsing, template rendering, phase orchestration, validation, fallback |
| Integration tests | 1.5w | End-to-end bundle resolution, skill discovery, phase execution with mock LLM |

### 10.2 Totals

| Scope | 1 Developer | 2 Developers (parallel) |
|-------|-------------|------------------------|
| Full implementation | 13.5 weeks | 8 weeks |
| MVP (investigation + workflow-selection bundles only, no pre/post hooks) | 8 weeks | 5 weeks |

### 10.3 Dependencies

- **Skills marketplace**: Must define and publish the skill artifact format before KA can implement the skill resolver. The `SkillResolver` interface will be designed to accommodate format changes.
- **PROPOSAL-EXT-001**: The MCP interactive mode (#703) and prompt bundle system share the `MCPToolProvider` infrastructure. MCP interactive mode can proceed independently -- prompt bundles extend, not replace, the MCP integration.

### 10.4 Risks

1. **Skills marketplace format instability** -- The marketplace project is actively defining its artifact format. KA's `SkillResolver` must be designed as an interface that can adapt to format changes without requiring prompt bundle manifest changes.
2. **Template complexity** -- Customer-authored Go templates may produce unexpected output. Pull-time template parsing catches syntax errors, but semantic correctness (e.g., referencing `.Investigation.RCASummary` in a `pre-investigation` bundle where it's not yet available) requires documentation and possibly runtime warnings.
3. **LLM compliance with custom schemas** -- Custom `outputSchema` values may be harder for the LLM to follow than Kubernaut's well-tested defaults. The self-correction + fallback mechanism mitigates this, but customers may see higher fallback rates with complex schemas.

---

## 11. Evolution Path

| Version | Capability | Notes |
|---------|-----------|-------|
| **v1.5** | MVP: investigation + workflow-selection bundles from OCI, builtin skill refs only | No external skills, no pre/post hooks. Validates the bundle format and phase orchestration. |
| **v1.5** | Skills marketplace integration | External skill resolution via OCI pull. Requires marketplace format to be stable. |
| **v1.6** | Pre/post-investigation hooks | Customer SOP injection with natural language propagation. |
| **v1.6** | `rca-resolution` phase | Conflict resolution for multi-source RCA findings. |
| **v1.7+** | Agent marketplace integration | A2A agent artifacts as skill-like dependencies. Extends the skill resolver to handle agent delegation. |
| **v1.7+** | OCI signature verification (sigstore) | Blocked on sigstore adoption for workflow bundles. Same infrastructure reused. |
| **v1.7+** | Prompt Bundle marketplace | Customers publish and share prompt bundles via a dedicated marketplace. |

### Design Gates

| Gate | Question | Status |
|------|----------|--------|
| **DG-1: OCI signature verification** | How do we verify prompt bundle and skill artifact signatures? | **Deferred** -- blocked on sigstore adoption for workflow bundles. Digests provide integrity; signatures add authenticity. |
| **DG-2: Template data contract versioning** | How do we evolve the template data contract without breaking existing bundles? | **Resolved** -- `apiVersion` field in manifest. Additive changes in `v1alpha1`, breaking changes require version bump. Multiple versions supported concurrently. |
| **DG-3: OCI media type for prompt bundles** | What media type identifies prompt bundle artifacts in OCI registries? | **Resolved** -- `application/vnd.kubernaut.promptbundle.v1+yaml`. |
| **DG-4: Core phase override safety** | How do we prevent customer bundles from breaking required phases? | **Resolved** -- Pull-time schema compatibility check + runtime validation with self-correction + fallback to default built-in bundle. |

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
| LLM loop | `internal/kubernautagent/investigator/investigator.go` | `runLLMLoop()` unchanged -- receives merged tool definitions from the phase orchestrator |

## Appendix B: Glossary

| Term | Definition |
|------|------------|
| **OCI** | Open Container Initiative -- standards for container image and artifact distribution |
| **MCP** | Model Context Protocol -- protocol for connecting AI agents to tools and data sources |
| **SOP** | Standard Operating Procedure -- customer-defined investigation procedures |
| **KA** | Kubernaut Agent -- the LLM integration service |
| **RCA** | Root Cause Analysis -- the investigation output identifying what caused the incident |
| **CMDB** | Configuration Management Database -- customer infrastructure inventory |
| **LRU** | Least Recently Used -- cache eviction strategy |
