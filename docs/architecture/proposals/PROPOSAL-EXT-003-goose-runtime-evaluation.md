# PROPOSAL-EXT-003: Goose Runtime Evaluation and Phased Adoption

**Status**: PROPOSAL (under review)
**Date**: April 15, 2026
**Author**: Kubernaut Architecture Team
**Confidence**: 95% (two rounds of adversarial audit; near-term scope narrowed to A2A plus current prompt builder, with Goose ACP explicitly gated by spike findings)
**Related**: [#711](https://github.com/jordigilh/kubernaut/issues/711) (Investigation Prompt Bundles), [#601](https://github.com/jordigilh/kubernaut/issues/601) (Shadow Agent), [#648](https://github.com/jordigilh/kubernaut/issues/648) (Multi-Agent Consensus / Dual Investigation), [PROPOSAL-EXT-001](PROPOSAL-EXT-001-external-integration-strategy.md) (External Integration Strategy), [PROPOSAL-EXT-002](PROPOSAL-EXT-002-investigation-prompt-bundles.md) (Investigation Prompt Bundles)

---

## Purpose

This proposal evaluates [Goose](https://github.com/block/goose) (AAIF -- an extensible, open-source AI agent framework) as a future candidate runtime for executing Kubernaut Agent's investigation phases. It defines how KA's Prompt Bundle format relates to Goose Recipes, records an ACP SDK spike using `coder/acp-go-sdk`, proposes a 6-phase pipeline model with `InvestigationHook` CRDs, and establishes a phased roadmap that keeps v1.5 focused on validating the current `prompt.Builder`-driven approach, narrows v1.6 remote delegation to A2A only, and defers any Goose adoption until the Goose ACP/API surface is stable enough to support it.

This evaluation was refined through two rounds of adversarial audit (14 findings resolved), covering self-correction loop compatibility, template rendering ownership, protocol consistency, audit granularity, and operational risks.

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Bundle Format and Compilation](#2-bundle-format-and-compilation)
3. [Six-Phase Pipeline Model](#3-six-phase-pipeline-model)
4. [InvestigationHook CRD](#4-investigationhook-crd)
5. [What KA Keeps (Domain-Specific Orchestration)](#5-what-ka-keeps-domain-specific-orchestration)
6. [What KA Could Drop on a Future Goose Path](#6-what-ka-could-drop-on-a-future-goose-path)
7. [ACP Go SDK Spike Findings](#7-acp-go-sdk-spike-findings)
8. [Shadow Agent and Dual Investigation Fit](#8-shadow-agent-and-dual-investigation-fit)
9. [Runtime Comparison](#9-runtime-comparison)
10. [Option A vs Option B: Enhance KA vs Adopt Goose](#10-option-a-vs-option-b-enhance-ka-vs-adopt-goose)
11. [Phased Adoption Roadmap](#11-phased-adoption-roadmap)
12. [Adversarial Audit Findings and Resolutions](#12-adversarial-audit-findings-and-resolutions)
13. [Risk Register](#13-risk-register)
14. [Design Gates](#14-design-gates)
15. [Impact on PROPOSAL-EXT-002](#15-impact-on-proposal-ext-002)

---

## 1. Executive Summary

PROPOSAL-EXT-002 defines PromptBundles as a Kubernaut-specific YAML format for packaging prompts, skill references, and output schemas as OCI artifacts. This evaluation finds that **Goose Recipes share the same structural concepts** (instructions + tools + output schema), but the near-term roadmap does not depend on Goose.

The key future enabler is the **`coder/acp-go-sdk`** -- a typed Go client for the Agent Client Protocol (ACP). A spike confirms that it supports session creation, prompt turns, and streamed updates, but it does **not** by itself close the current Goose ACP gap around recipe/session configuration.

**Key architectural decisions from two rounds of adversarial review:**

- **PromptBundle is a Kubernaut-native format**. Conceptual alignment with Goose Recipes exists at the naming/structure level, not file-format level.
- **ACP Go SDK (`coder/acp-go-sdk`)** is a promising future integration mechanism, but only after Goose ACP can support Kubernaut's required session semantics for instructions, extensions, schema, and settings.
- **6-phase pipeline** with `pre-workflow-selection` as a new optional hook phase (extends the 5-phase model in EXT-002).
- **InvestigationHook CRD** for optional phases (parallel execution within a phase). Mandatory phases remain in KA's YAML config.
- **KA as "compiler"**: in a future Goose path, KA would render Go templates and resolve OCI skill refs before invoking the runtime.
- **Near-term remote protocol scope is A2A only**. ACP remains a future candidate once Goose API/SDK support is mature enough.
- **EXT-002 updates deferred** to a follow-up PR so this proposal can stay focused on the narrowed near-term scope: current prompt builder validation first, A2A-only remote hooks second, Goose later.

**Phased adoption (no mandatory Goose dependency in the near term):**

| Version | Goose Dependency | KA Role | Runtime |
|---------|-----------------|---------|---------|
| v1.5 | None | Current inline executor + orchestrator | Validate the existing typed `prompt.Builder` flow; no manifest-driven loading yet |
| v1.6 | None for core; optional remote A2A hooks only | Orchestrator + inline for core | Hook phases can delegate to A2A agents (for example, DocsClaw or customer-managed agents) |
| Future (post-v1.5 re-evaluation) | Candidate only, contingent on ACP/API stability | Potential pure orchestrator/compiler | Revisit Goose once ACP can support the required session configuration model |

---

## 2. Bundle Format and Compilation

The PromptBundle is a **Kubernaut-native OCI artifact**. It shares structural concepts with Goose Recipes (instructions, tools, output schema) but is NOT a valid Goose Recipe file -- it uses Go templates and OCI skill references that Goose cannot parse directly.

### 2.1 Bundle Manifest (OCI Artifact)

```yaml
apiVersion: kubernaut.ai/v1alpha1
version: 1.0.0
title: "ACME CMDB Pre-Check"
description: "Verify resource exists in CMDB before investigation"
instructions: |
  You are a pre-investigation assistant for Kubernaut.
  A {{ .Signal.Severity }} signal fired for {{ .Signal.ResourceKind }}/{{ .Signal.ResourceName }}
  in namespace {{ .Signal.Namespace }}.
  Check the CMDB for this resource's status.
extensions:
  - ref: "registry.example.com/skills/cmdb-lookup@sha256:abc123..."
  - ref: "builtin://get_namespaced_resource_context"
response:
  json_schema: null
```

### 2.2 Field Semantics

| Field | Purpose | Goose Recipe Equivalent |
|-------|---------|----------------------|
| `apiVersion` | Kubernaut template data contract version (which `.Signal`, `.Enrichment`, `.Investigation` fields are available). Distinct from `version`. | No equivalent (Kubernaut extension) |
| `version` | Bundle format version. | `version` (identical) |
| `title` / `description` | Human-readable metadata. | `title` / `description` (identical) |
| `instructions` | Go template rendered by KA with `missingkey=error`. Supports conditionals (`{{if}}`), iteration (`{{range}}`), and nested object access (`{{ .Signal.Namespace }}`). | `instructions` (Goose uses flat `{{key}}` parameter substitution only) |
| `extensions[].ref` | OCI digest refs (`@sha256:`) and `builtin://` scheme. KA resolves these to live MCP endpoint URLs. | `extensions[]` with `type: sse`, `url: "..."` format |
| `response.json_schema` | JSON Schema for structured output phases. | `response.json_schema` (directly compatible) |

### 2.3 Near-Term Implementation Scope (v1.5-v1.6)

The near-term implementation does **not** replace the current typed prompt-building path. KA's existing `prompt.Builder` remains the source of truth for prompt rendering in v1.5:

1. **Render** embedded Go templates using typed Go structs (`SignalData`, `EnrichmentData`, `Phase1Data`)
2. **Execute** investigation, RCA resolution, and workflow selection inline inside KA
3. **Validate** the current prompt structure and output contracts before introducing manifest-driven loading

This preserves the current implementation in `internal/kubernautagent/prompt/builder.go` and keeps the manifest-driven model as a later step rather than a v1.5 commitment.

### 2.4 Future Compilation Path (Goose/ACP, Gated)

If Goose adoption is revisited later, KA would still perform the Kubernaut-specific compilation steps before invoking Goose:

1. **Render** Go templates against the phase-specific data contract -> rendered instructions string
2. **Resolve** OCI skill refs to concrete remote tool endpoints
3. **Resolve** `builtin://` refs to KA-hosted or extracted MCP endpoints
4. **Create** a session and send prompt turns over ACP

However, the current Goose ACP surface does not yet provide recipe/session parity for instructions, extensions, schema, and settings during session creation. See Section 7 for the spike results and current gap.

### 2.5 Why Not Use Goose Recipes Directly?

Goose Recipes support `{{key}}` flat parameter substitution in `instructions`. This is insufficient for Kubernaut's nested data contract:

- Nested object access: `.Signal.Namespace`, `.PriorPhaseOutputs[0].Output`
- Iteration: `{{range .PriorPhaseOutputs}}`
- Conditionals: `{{if .Enrichment.OwnerChain}}`

If Goose adds nested object access or a richer template engine in the future, format convergence becomes possible. Until then, KA renders Go templates and passes rendered strings to Goose as the `instructions` field.

Additionally, Kubernaut's `extensions[].ref` uses OCI digest references and `builtin://` schemes that Goose cannot resolve natively. KA's skill resolver translates these to live MCP endpoint URLs at compilation time.

---

## 3. Six-Phase Pipeline Model

This proposal extends the 5-phase model defined in PROPOSAL-EXT-002 by adding a `pre-workflow-selection` hook phase.

```
pre-investigation         (optional, InvestigationHook CRDs, parallel)
  |
investigation             (mandatory, KA config, single bundle)
  |
post-investigation        (optional, InvestigationHook CRDs, parallel)
  |
rca-resolution            (mandatory, KA config, single bundle)
  |
pre-workflow-selection    (optional, InvestigationHook CRDs, parallel)  [NEW]
  |
workflow-selection        (mandatory, KA config, single bundle)
```

### 3.1 Mandatory Phases (3)

Configured in KA's YAML config. Built-in bundles are embedded in the binary and overridable by the operator. Exactly one bundle per mandatory phase.

### 3.2 Optional Hook Phases (3)

Defined as `InvestigationHook` CRDs (see Section 4). Zero or many per phase. Executed **in parallel** within a phase (hooks are independent of each other). KA collects all outputs and passes them as `PriorPhaseOutputs` to the next phase.

### 3.3 New: `pre-workflow-selection` Hook

Allows customers to inject constraints before workflow selection:

- "Only select workflows approved for production"
- "Namespace is in change freeze -- only diagnostic workflows"
- "Check ITSM for open change requests before selecting a remediation workflow"

**Template data contract**: All fields are available at this phase (`.Signal`, `.Enrichment`, `.PriorPhaseOutputs`, `.Investigation.RCANarrative`, `.Investigation.RCASummary`). This is the richest data contract of any hook phase, since it runs after both the investigation and RCA resolution have completed.

---

## 4. InvestigationHook CRD

### 4.1 Schema

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: InvestigationHook
metadata:
  name: acme-cmdb-precheck
  namespace: kubernaut-system
spec:
  phase: pre-investigation
  bundleRef: "registry.example.com/acme-cmdb-precheck@sha256:abc..."
  priority: 100
  failurePolicy: failClosed
  runtime:
    endpoint: "http://docsclaw-hooks.svc:8080/a2a"
    timeout: 30s
```

### 4.2 Field Reference

| Field | Description |
|-------|-------------|
| `phase` | Hook point: `pre-investigation`, `post-investigation`, or `pre-workflow-selection` |
| `bundleRef` | OCI digest reference to the bundle artifact |
| `priority` | Execution order hint. All hooks in a phase run in parallel, but priority determines output ordering in `PriorPhaseOutputs` |
| `failurePolicy` | `failClosed` (abort pipeline, default) or `failOpen` (skip this hook, log warning) |
| `runtime.endpoint` | A2A endpoint URL for the remote hook agent |
| `runtime.timeout` | Per-hook timeout. Aggregate phase timeout in KA config caps total phase duration |

### 4.3 Benefits

- **Dynamic**: Add/remove hooks without KA restart (GitOps friendly)
- **Individual RBAC**: Each hook can have its own RBAC policy
- **K8s-native config surface**: Hook definitions are managed as Kubernetes resources with standard discovery and RBAC
- **Parallel execution**: Independent hooks execute concurrently for lower latency

**Near-term protocol scope**: v1.6 supports **A2A only** for remote hook execution. We intentionally do not add a `protocol` or `type` field yet because only one remote protocol is supported in the near term. If ACP/Goose is adopted later, the CRD should gain an explicit discriminator rather than overloading `runtime.endpoint`.

### 4.4 CRD Discovery

KA uses a `controller-runtime` shared informer cache (already a dependency at v0.23.3) to watch `InvestigationHook` CRDs. No reconciler loop is needed -- KA reads the cache at investigation time to discover hooks for each phase. The `InvestigationHook` CRD definition and OpenAPI validation schema require code generation (`make generate`, `make manifests`).

### 4.5 Failure Policy Behavior During Parallel Execution

- **`failClosed`**: When any hook in a parallel batch fails, KA cancels in-flight hooks via context cancellation and aborts the pipeline. The failing hook's error is propagated in the `InvestigationResult`.
- **`failOpen`**: KA waits for all hooks to complete. Failed hooks are skipped (logged as warnings). Successful outputs are collected into `PriorPhaseOutputs`.

---

## 5. What KA Keeps (Domain-Specific Orchestration)

KA does **not** manage CRDs for the remediation lifecycle -- it receives a `SignalContext` via HTTP and returns an `InvestigationResult`. CRD lifecycle (RemediationRequest, child CRDs) is handled by the Remediation Orchestrator upstream.

KA **does** watch `InvestigationHook` CRDs to discover optional phase hooks.

Even with Goose as the LLM engine, KA retains:

| Responsibility | Description |
|---------------|-------------|
| **Pipeline orchestration** | 6-phase sequencing, hook CRD discovery, parallel hook dispatch, context propagation (`PriorPhaseOutputs`) |
| **Bundle compilation** | Go template rendering and existing typed prompt builder flow in the near term; future Goose compilation remains gated |
| **API contract** | `SignalContext` in, `InvestigationResult` out -- unchanged regardless of runtime |
| **Signal enrichment** | K8s owner chain resolution, label merging, re-enrichment when RCA identifies a different target |
| **Result assembly** | Merging phase outputs into `InvestigationResult` (severity backfill, remediation target injection, detected labels, catalog enrichment) |
| **Audit assembly** | A2A execution-trace collection for v1.6 hooks; future Goose/ACP streaming remains a gated candidate. Stored via DataStorage audit pipeline |
| **Failure policy enforcement** | `failClosed`/`failOpen` per hook, aggregate phase timeout, context cancellation for parallel hooks |
| **Catalog validation** | Workflow self-correction loop with retries in the current inline model. A future Goose implementation could map this to ACP prompt turns, but that path is still gated by Section 7 findings |

---

## 6. What KA Could Drop on a Future Goose Path

If Goose later becomes the runtime for all phases, KA could drop:

| Component | Current Role | Replacement |
|-----------|-------------|-------------|
| `runLLMLoop()` | Multi-turn conversation loop | ACP session with `Prompt()` for follow-ups |
| `llm.Client` interface | LLM provider abstraction (LangChainGo, Vertex Anthropic) | Goose handles provider selection via `settings` |
| Tool registry | Tool execution dispatch | Goose extension system (MCP-native) |
| LLM provider config | Model, API keys, temperature | Goose pod config + K8s Secrets |
| Token accumulation | Per-turn token tracking | Goose tracks natively; KA extracts from execution trace |

---

## 7. ACP Go SDK Spike Findings

The [`coder/acp-go-sdk`](https://github.com/coder/acp-go-sdk) is a Go client library that provides typed bindings for the Agent Client Protocol. This section records a focused spike on what the SDK and current Goose ACP implementation prove today, and what remains missing before Kubernaut could rely on it.

### 7.1 Key Capabilities

| SDK Method | What the spike validates |
|-----------|-------------------------|
| `Initialize(...)` | ACP capability negotiation before opening a session |
| `NewSession(request)` | Session creation with `cwd` and `mcpServers` |
| `Prompt(request)` | Sending a prompt to an existing session |
| `SessionUpdate` callback | Streaming agent message chunks, thought chunks, tool calls, tool call updates, and plans |

### 7.2 What the SDK Eliminates

- **No custom ACP client plumbing**: SDK handles ACP request/response types and streamed updates.
- **No custom SSE event parser**: streamed updates are surfaced via typed callbacks.

### 7.3 Spike Appendix: Exact Request/Response Flow

The SDK README and example client demonstrate the concrete flow below:

```go
initResp, err := conn.Initialize(ctx, acp.InitializeRequest{...})
sessResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
	Cwd:        mustCwd(),
	McpServers: []acp.McpServer{},
})
_, err = conn.Prompt(ctx, acp.PromptRequest{
	SessionId: sessResp.SessionId,
	Prompt:    []acp.ContentBlock{acp.TextBlock("Hello, agent!")},
})
```

The example client also demonstrates `SessionUpdate(...)` handling for:

- `AgentMessageChunk`
- `AgentThoughtChunk`
- `ToolCall`
- `ToolCallUpdate`
- `Plan`

This is enough to validate the **transport and interaction model**: session creation, prompt turns, and streaming updates are all available in the SDK today. Source references: the ACP Go SDK [README](https://raw.githubusercontent.com/coder/acp-go-sdk/main/README.md) and example client [`example/client/main.go`](https://raw.githubusercontent.com/coder/acp-go-sdk/main/example/client/main.go).

### 7.4 Current Gap: Goose ACP Lacks Recipe/Session Parity

The spike also found a material upstream limitation:

- `acp-go-sdk`'s `NewSessionRequest` currently exposes `cwd` and `mcpServers`, not a rich session payload for instructions, response schema, settings, or recipe application.
- Upstream Goose has an open issue stating that `goose-acp` does **not** yet support creating a new session from a recipe the way `goose-server` does: [aaif-goose/goose#7596](https://github.com/block/goose/issues/7596).
- As a result, the current ACP path does **not** yet prove that KA can compile a PromptBundle directly into a Goose ACP session with full parity for instructions, extensions, schema, and settings.

### 7.5 Impact on Work Estimates

The spike reduces uncertainty around the ACP interaction model, but it does **not** eliminate the need for additional upstream Goose ACP support or a custom extension method. The SDK therefore strengthens Goose as a future candidate, but it does not justify treating Goose integration as a committed near-term implementation path.

---

## 8. Shadow Agent and Dual Investigation Fit

### 8.1 Shadow Agent (#601)

In the current inline architecture, a shadow agent runs in parallel with the primary investigation, monitoring for prompt injection. If Goose becomes viable later:

- The ACP Go SDK's `SessionUpdate` callback provides the same real-time event stream the shadow agent needs.
- KA spawns the shadow agent goroutine which receives events from the primary Goose session's callback and runs prompt injection detection.
- If injection is detected, KA cancels the primary session (context cancellation) and aborts the investigation with an audit record.
- The shadow agent itself could also be a Goose session with a security-focused recipe, enabling recipe-based extensibility for the security monitoring pipeline.

### 8.2 Dual Investigation / Multi-Agent Consensus (#648)

KA's strategy config (`single`, `consensus`, `consensus-fast`) already defines whether to run one or two parallel investigations. If Goose becomes viable later:

- KA creates two ACP sessions using the **same compiled bundle** but different `settings` blocks (different provider/model, e.g., Claude vs GPT-4o).
- Both sessions execute in parallel. KA collects both `InvestigationResult` structured outputs.
- The consensus algorithm (voting, merge, or comparison) runs in KA -- it is domain logic, not LLM execution.
- Goose's `settings.provider` and `settings.model` fields make this natural: same recipe, different settings = dual investigation. No recipe duplication needed.

---

## 9. Runtime Comparison

| Characteristic | DocsClaw | Goose |
|---|---|---|
| Footprint | ~5 MiB per pod | ~50-100 MiB (Rust binary + deps) |
| Startup | Sub-second | 1-3 seconds |
| MCP support | ConfigMap-driven | Native, first-class |
| A2A support | Native | Via MCP extension (evolving) |
| Multi-turn | Basic | Full (sub-agents, retries, recipes) |
| Structured output | Via A2A artifact | Native `response.json_schema` |
| Session continuity | No (stateless) | Yes (ACP `session/prompt`) |
| Ideal for | Simple hooks (pre/post-investigation) | Complex multi-tool phases, self-correction flows |
| Deployment | K8s-native, ConfigMap | Container, env var config |
| License | TBD (Red Hat OCTO) | Apache 2.0 |

**Recommendation**: DocsClaw or customer-managed A2A agents for lightweight hook phases in the near term. Goose remains a future option for more complex phases, contingent on ACP/API maturity.

---

## 10. Option A vs Option B: Enhance KA vs Adopt Goose

### 10.1 Option A: Enhance Existing KA

Add MCP support to `runLLMLoop`. KA stays self-contained.

| Attribute | Assessment |
|-----------|-----------|
| **Effort** | ~2-3 weeks |
| **New dependency** | None |
| **Testing** | Simpler (single binary, no IPC) |
| **Provider support** | KA manages directly |
| **Sub-agent support** | Not available |
| **Recipe ecosystem** | Not available |
| **Long-term maintenance** | KA team owns full LLM stack |

### 10.2 Option B: Adopt Goose (Future Candidate)

Delegate LLM execution to Goose via ACP Go SDK once the Goose ACP/API surface is mature enough. KA would become pure orchestrator/compiler at that point.

| Attribute | Assessment |
|-----------|-----------|
| **Effort** | Tentative; re-estimate after Goose ACP session-configuration gap closes |
| **New dependency** | Goose runtime + `coder/acp-go-sdk` |
| **Testing** | More complex (multi-process, requires Goose in CI) |
| **Provider support** | Goose manages; must validate full matrix |
| **Sub-agent support** | Native (Goose sub-agents) |
| **Recipe ecosystem** | Access to community recipes and extensions |
| **Long-term maintenance** | KA team focuses on orchestration; LLM execution delegated |

### 10.3 Recommendation

**Option A for v1.5** and for the likely v1.6 baseline. Option B remains a future candidate only after Goose ACP can support the required session semantics. Near-term work should validate the current prompt-builder-driven approach before any manifest-driven or Goose-backed execution shift.

---

## 11. Phased Adoption Roadmap

### 11.1 v1.5: Validate Current Inline Execution Path

- Validate the current typed `prompt.Builder` path and prompt contracts before changing the execution model.
- KA executes all phases inline (current architecture + MCP support).
- `InvestigationHook` CRD for optional hook phases (parallel execution).
- **No Goose runtime dependency.**

### 11.2 v1.6: Optional External Delegation for Hooks (A2A Only)

- Optional hook phases (`pre-investigation`, `post-investigation`, `pre-workflow-selection`) can delegate to external **A2A** runtimes (DocsClaw or customer-managed A2A agents) via `runtime.endpoint` in `InvestigationHook` CRD.
- Core phases (`investigation`, `rca-resolution`, `workflow-selection`) remain inline.
- Audit for delegated hooks uses the A2A execution trace contract already defined in PROPOSAL-EXT-002.

### 11.3 Future: Revisit Goose Delegation (Contingent on ACP/API Stability)

- Re-evaluate Goose only after the v1.5 validation milestone and only if Goose ACP can support the required session configuration semantics.
- If viable, KA could later drop `runLLMLoop`, `llm.Client`, and tool registry responsibilities.
- Prerequisites:
  - Goose ACP supports recipe/session parity or a supported extension method with equivalent semantics
  - KA builtins extracted to standalone MCP servers
  - Credential management via K8s Secrets validated against full provider matrix
  - Anomaly detection implemented as a Goose extension (custom MCP server)
  - Real-time audit via ACP `SessionUpdate` streaming

---

## 12. Adversarial Audit Findings and Resolutions

Two rounds of adversarial audit produced 14 findings (3 critical, 4 high, 4 medium, 3 low). The findings below are incorporated into the revised scope and assumptions in this document.

### Round 1

#### CRITICAL-1: Self-Correction Loop vs Dropping runLLMLoop

**Problem**: Catalog validation (workflow-selection phase) retries within the same LLM session -- appending correction messages and calling `runLLMLoop` again. If the loop moves to Goose, KA loses stateful mid-session retries.

**Resolution**: Use ACP Go SDK's `Prompt()` method on the existing session to continue with correction context. KA: (1) creates a Goose session via `NewSession`, (2) sends the workflow prompt via `Prompt()`, (3) receives structured output via `SessionUpdate` callback, (4) validates against the catalog, (5) if invalid, calls `Prompt()` again on the same session with the correction message. The Goose session maintains full conversation state across retries. This maps directly to KA's current pattern where `correctionFn` appends to `messages` and re-calls `runLLMLoop`. Verified: ACP `session/prompt` sends a user message to an existing session and streams the response.

#### CRITICAL-2: Template Rendering Ownership

**Problem**: Unclear whether KA or Goose renders Go templates in the instructions field.

**Resolution**: KA always renders Go templates before passing to the runtime. Goose receives a **rendered string** as `instructions`, never Go template syntax. KA is the "compiler" (see Section 2).

#### CRITICAL-3: "Do Nothing" Alternative Must Be Presented

**Problem**: The plan lacked a comparison with enhancing the existing KA architecture.

**Resolution**: Section 10 presents Option A (Enhance existing KA, ~2-3w) vs Option B (Goose adoption as a future candidate), with a clear recommendation for Option A in the near term and Option B only after the Goose ACP gap is closed.

#### HIGH-1: Protocol Scope Must Be Explicit

**Problem**: Initial recommendation of KA speaking ACP directly conflicted with A2A as the sole delegation protocol defined in PROPOSAL-EXT-002.

**Resolution**: Narrow the near-term scope to **A2A only** for remote execution. v1.6 hook delegation assumes a single remote protocol and therefore does not need a `protocol` discriminator in the CRD yet. ACP remains a future evaluation track for Goose once the Goose ACP surface can support Kubernaut's required session configuration.

#### HIGH-2: Anomaly Detection in Remote Execution

**Problem**: KA's anomaly detection occurs mid-`runLLMLoop` (per-turn checks). Moving LLM execution to Goose loses this mid-loop inspection.

**Resolution**: On a future Goose path, anomaly detection would likely become a **Goose extension** -- a custom MCP server that wraps tool calls with KA's anomaly checking logic. Alternatively, KA's aggregate phase timeout + `failClosed` provides a coarser safety net. Detailed design is deferred until Goose is back in active scope.

#### HIGH-3: Audit Granularity in Remote Execution

**Problem**: Moving LLM execution to Goose could degrade real-time, per-turn audit events to post-hoc trace extraction.

**Resolution**: For the near term, v1.6 remote hooks rely on the A2A execution-trace artifact already defined in PROPOSAL-EXT-002. The ACP spike indicates that `SessionUpdate` is a promising future fit for Goose-side streaming, but that remains contingent on Goose ACP supporting the required session configuration model.

#### HIGH-4: ACP Instability

**Problem**: ACP is mid-migration (Phase 3), not yet stable. Building against an unstable protocol is risky.

**Resolution**: Goose adoption remains a future candidate only. v1.5 validates the current inline approach, and v1.6 remote hooks use A2A only. ACP stays off the critical path until Goose ACP can configure sessions with the semantics Kubernaut needs.

### Round 2

#### MEDIUM-1: Skill Translation is Non-Trivial

**Problem**: OCI digest references in `extensions[].ref` are not native Goose format.

**Resolution**: KA's skill resolver still handles OCI-to-endpoint translation, but the Goose-specific mapping is future work. Near-term remote execution remains A2A-only, so ACP extension construction is no longer assumed to be part of v1.6 scope.

#### MEDIUM-2: `submit_result` vs Goose `final_tool` Semantic Gap

**Problem**: Behavioral difference between KA's `submit_result` sentinel tool and Goose's `final_tool` concept.

**Resolution**: This remains a future Goose design concern, not a near-term delivery item. The current inline flow continues to use `submit_result`, and any Goose mapping must be revisited only after the ACP session-configuration gap is closed.

#### MEDIUM-3: Work Estimate Revised

**Problem**: Initial estimate of 5.5 weeks was optimistic.

**Resolution**: Any Goose estimate remains tentative until the ACP session-configuration gap is closed upstream. The more important near-term decision is sequencing: validate the current prompt builder first, then narrow any remote execution work to A2A.

#### MEDIUM-4: LLM Credential Migration Unaddressed

**Problem**: How Goose accesses LLM credentials (API keys, service accounts) was not specified.

**Resolution**: LLM credentials move to Goose pod via K8s Secrets. Provider compatibility matrix (Vertex AI with service accounts, Azure with managed identity, Bedrock with IAM roles) must be validated against Goose's provider support. Documented as a future prerequisite (Design Gate DG-9).

#### LOW-1: DocsClaw Structured Output Description

**Problem**: Runtime comparison table described DocsClaw structured output incorrectly.

**Resolution**: Corrected to "Via A2A artifact."

#### LOW-2: Goose License Was Disputed

**Problem**: Missing context on Goose's licensing history.

**Resolution**: Apache 2.0 confirmed. An Acceptable Use Policy (AUP) dispute in late 2025 (issue #6200) was resolved in January 2026 by removing the AUP. Noted in risk register as a governance consideration.

#### ISSUE-5: `apiVersion` in Bundle vs Goose Recipe `version`

**Problem**: Two version fields could confuse developers.

**Resolution**: Both fields are kept with distinct purposes. `version` is the bundle format version (aligned with Goose Recipe convention). `apiVersion` is a Kubernaut extension for template data contract versioning (which `.Signal`, `.Enrichment`, `.Investigation` fields are available at a given phase).

#### ISSUE-6: Goose `settings` Field is Strategically Important

**Problem**: The `settings` block in Goose Recipes (provider, model, temperature) was not discussed.

**Resolution**: `settings` is a pass-through field from KA config to the compiled ACP session config. It is strategically important for dual investigation (#648): same recipe, different `settings` (different provider/model) = two parallel investigations. KA's strategy config (`single`/`consensus`/`consensus-fast`) determines which `settings` get injected into each ACP session invocation.

---

## 13. Risk Register

| Risk | Severity | Mitigation | Phase |
|------|----------|-----------|-------|
| **ACP protocol instability** | High | Goose adoption is off the near-term critical path. Revisit only after current v1.5 validation and once ACP is stable enough for required session semantics. | Future |
| **Goose ACP session configuration gap** | High | Current Goose ACP lacks recipe/session parity for instructions, extensions, schema, and settings during session creation. Track upstream gap and do not commit Goose delivery dates until resolved. | Future |
| **`coder/acp-go-sdk` maturity** | Medium | Third-party SDK (Coder). API stability and maintenance commitment not guaranteed. Validate against live Goose instance before any Goose commitment. | Future |
| **Goose provider matrix gaps** | Medium | Must validate Goose supports Vertex AI (service accounts), Azure (managed identity), Bedrock (IAM roles). Documented as DG-9 gate. | Future |
| **Latency increase** | Medium | Goose adds IPC overhead (~50-100ms per invocation). Acceptable for investigation phases (multi-second LLM calls). Monitor aggregate pipeline latency. | Future |
| **Governance / licensing** | Low | Apache 2.0 confirmed. AUP dispute resolved. Monitor for future governance changes in Block/Goose project. | Ongoing |
| **Testing complexity** | Medium | Goose in CI requires containerized Goose instance. Mock ACP server for unit tests; real Goose for integration tests. | Future |
| **InvestigationHook CRD adoption** | Low | CRD requires code generation and documentation. KA uses informer cache (no reconciler). Established pattern in the codebase. | v1.5 |

---

## 14. Design Gates

| Gate | Question | Status |
|------|----------|--------|
| **DG-7: Runtime selection** | How does KA select which runtime executes a given phase? | **Resolved for near-term scope** -- Hook phases use A2A endpoints only. Core phases stay inline. If ACP is introduced later, add an explicit protocol/type discriminator to the hook spec. |
| **DG-8: ACP stability gate** | When is ACP stable enough for production use? | **Deferred** -- Goose ACP must support the required session configuration semantics (or a supported extension method), not just basic session creation and prompt turns. |
| **DG-9: Credential management** | How do LLM credentials reach Goose pods? | **Deferred** -- K8s Secrets injection. Must validate Goose supports KA's full provider matrix (Vertex AI SA, Azure MI, Bedrock IAM). |

---

## 15. Impact on PROPOSAL-EXT-002

The following updates to PROPOSAL-EXT-002 are proposed but **deferred to a follow-up PR** so this document can stay focused on the narrowed near-term scope:

| EXT-002 Section | Proposed Change |
|----------------|----------------|
| Section 3 | Add 6th phase (`pre-workflow-selection`) |
| Section 3.2 | Add parallel execution within hook phases |
| Section 2 | Move `phase` and `agent` out of bundle manifest into InvestigationHook CRD |
| Section 3.4 | Reference InvestigationHook CRD for hook phases, KA config for core phases |
| Section 5.2 | Add `pre-workflow-selection` template data contract |
| Section 7 | Add InvestigationHook CRD-based bundle resolution for optional phases |
| Section 11 | Add Goose alignment milestones (v1.5 validation, v1.6 A2A hooks, future Goose re-evaluation) |
| Appendix B | Update WAR analogy -- KA as compiler, Goose as application server |
| Appendix D | Add glossary terms: Goose, ACP, ACP Go SDK, Recipe, InvestigationHook, pre-workflow-selection, settings |
