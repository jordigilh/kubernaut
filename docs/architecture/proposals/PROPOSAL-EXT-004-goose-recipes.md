# PROPOSAL-EXT-004: Goose Recipes as AgenticWorkflow Runtime

**Status**: ❌ SUPERSEDED — see [#1536](https://github.com/jordigilh/kubernaut/issues/1536) (runtime-agnostic opaque-OCI-image direction; Kubernaut no longer selects or knows about a "Goose runtime"). Retained for historical context only.  
**Date**: May 19, 2026  
**Superseded**: July 5, 2026  
**Author**: Kubernaut Architecture Team  
**Confidence**: 96% (Follow-up Spike validated: Goose CLI `gcp_vertex_ai` provider -> MCP server -> tool calls -> structured RCA, end-to-end in 6.7s)  
**Related**: [PROPOSAL-EXT-003](PROPOSAL-EXT-003-goose-runtime-evaluation.md) (superseded), [PROPOSAL-EXT-005](PROPOSAL-EXT-005-oracle-agent-spec.md) (superseded), [PROPOSAL-EXT-006](PROPOSAL-EXT-006-deep-agents.md) (superseded)  
**Spike Code**: [goose-mcp-roundtrip/](../spikes/goose-mcp-roundtrip/) (end-to-end MCP roundtrip with Vertex AI)  
**Spike Summary**: [SPIKE-GOOSE-MCP-ROUNDTRIP](../spikes/SPIKE-GOOSE-MCP-ROUNDTRIP.md), [SPIKE-ACP-ENFORCEMENT](../spikes/SPIKE-ACP-ENFORCEMENT.md), [SPIKE-OCI-RUNTIME-CONTRACT](../spikes/SPIKE-OCI-RUNTIME-CONTRACT.md)  
**Tracking**: [#1240](https://github.com/jordigilh/kubernaut/issues/1240) (umbrella, closed) → superseded by [#1536](https://github.com/jordigilh/kubernaut/issues/1536), [#1535](https://github.com/jordigilh/kubernaut/issues/1535)  
**Target**: ~~v1.6 milestone~~ — successor work now tracked under v1.7

---

## 1. Purpose

This proposal defines Goose Recipes as one of three pluggable runtime types
for the `AgenticWorkflow` CRD. Unlike PROPOSAL-EXT-003 which proposed Goose
as the **sole** LLM runtime replacing KA's inline execution, this proposal
positions Goose as an **equal peer** alongside Oracle Agent Spec and Deep
Agents, all orchestrated through a universal ACP server.

Goose Recipes provide a YAML-based, human-readable format for defining
investigation workflows. The upstream Goose CLI (Rust, block/goose) executes
recipes with native MCP tool support, streaming output, and multi-provider
LLM configuration.

---

## 2. Architecture

### 2.1 AgenticWorkflow CRD (runtime: goose)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: AgenticWorkflow
metadata:
  name: oomkill-rca-goose
spec:
  version: "1.0.0"
  description:
    what: "OOMKill RCA investigation using Goose recipe"
    whenToUse: "OOMKill alerts on production deployments"
  execution:
    runtime: goose
    bundle: registry.example.com/kubernaut/oomkill-goose:v1.0.0
    bundleDigest: sha256:abc123...
    runtimeConfig:
      maxToolCalls: 30
      maxToolCallsPerTool: 10
      shadowAgentEnabled: true
  labels:
    severity: [critical, warning]
    environment: [production]
    component: [Deployment, StatefulSet]
```

### 2.2 Execution Flow

```
KA → ACP Server → Goose CLI (recipe) → MCP Tools (via ACP enforcement)
```

1. KA receives signal and selects matching `AgenticWorkflow` (type: goose)
2. ACP server pulls OCI image, extracts recipe from `/spec/recipe.yaml`
3. ACP server starts `goose` CLI with `--recipe /spec/recipe.yaml`
4. Goose connects to ACP server's MCP endpoint for tool access
5. ACP enforcement layer intercepts all tool calls (budget, shadow, audit)
6. Goose streams investigation progress via SSE
7. ACP server extracts structured RCA from `submit_result` tool call

### 2.3 OCI Image Contract

| Label | Value |
|---|---|
| `ai.kubernaut.runtime` | `goose` |
| `ai.kubernaut.spec-version` | `1.0` |
| `ai.kubernaut.entrypoint` | `/spec/recipe.yaml` |
| `ai.kubernaut.tools` | Comma-separated tool names |

```dockerfile
FROM ghcr.io/block/goose:latest
LABEL ai.kubernaut.runtime="goose"
LABEL ai.kubernaut.spec-version="1.0"
LABEL ai.kubernaut.entrypoint="/spec/recipe.yaml"

COPY recipe.yaml /spec/recipe.yaml
COPY entrypoint.sh /runtime/entrypoint.sh
ENTRYPOINT ["/runtime/entrypoint.sh"]
```

---

## 3. Goose Recipe Format

```yaml
version: "1.0"
title: "OOMKill RCA Investigation"
description: "Investigate OOMKill signals on Kubernetes deployments"

instructions: |
  You are a Kubernetes incident investigator for the Kubernaut platform.
  A signal has fired indicating an OOMKill event.

  Use kubectl_get to inspect the affected resource.
  Use kubectl_list_events to check for OOM-related events.
  Use prometheus_query to check memory usage trends.

  When you have identified the root cause, call submit_result with:
  - root_cause: string
  - confidence: 0-1
  - affected_resources: array of resource identifiers
  - remediation_suggested: boolean

extensions:
  - type: mcp
    name: kubernaut-tools
    uri: "http://localhost:${ACP_PORT}/mcp"

context:
  - type: text
    text: |
      Signal: ${SIGNAL_NAME}
      Namespace: ${SIGNAL_NAMESPACE}
      Severity: ${SIGNAL_SEVERITY}
      Resource: ${RESOURCE_KIND}/${RESOURCE_NAME}
```

---

## 4. Feature Parity with KA v1.5

| Feature | KA v1.5 | Goose Runtime | Notes |
|---|---|---|---|
| Tool call budget | AnomalyDetector | ACP enforcement layer | Spike 2 validated |
| Shadow agent | alignment.ToolProxy | ACP shadow feed | Hybrid: local canary + remote grounding |
| Audit events | audit.StoreBestEffort | ACP audit sink | Same event schema |
| Phase-based tools | PhaseToolMap | ACP tool filtering | ACP exposes phase-appropriate tools only |
| Structured output | submit_result sentinel | submit_result tool | Identical extraction pattern |
| LLM streaming | llm.Client | Goose SSE events | Native streaming support |
| Multi-provider | SwappableClient | Goose provider config | Native multi-provider (OpenAI, Anthropic, etc.) |

---

## 5. Strengths

- **Native Goose ecosystem**: Recipes are the upstream format; no adapter needed
- **Rust-compiled CLI**: Fast startup, low memory footprint, single binary
- **No Python dependency**: Self-contained OCI image
- **Rich extension model**: MCP, built-in tools, custom extensions
- **Community momentum**: AAIF/Linux Foundation project, active development

---

## 6. Limitations

- **No structured I/O schema**: Goose recipes use flat string parameters;
  no JSON Schema validation at the recipe boundary. ACP server must
  validate signal context before injection.
- **CLI-based execution**: Goose runs as a subprocess, not as a library.
  ACP server communicates via MCP + SSE rather than in-process calls.
- **Upstream dependency**: AAIF/block owns the roadmap. Structured parameter
  PR (#8934) was closed; JSON Schema validation is not on the upstream roadmap.

---

## 7. Spike Validation Results

### Follow-up Spike: Goose CLI -> ACP MCP Roundtrip (May 2026)

**Result**: PASS

Validated end-to-end Goose CLI integration with Vertex AI and MCP tools:

| Aspect | Result |
|---|---|
| Provider | `gcp_vertex_ai` (GOOSE_PROVIDER) with ADC authentication |
| Model | `claude-sonnet-4@20250514` via Vertex AI us-east5 |
| MCP connection | Goose connected to `FastMCP` server on `streamable-http` transport |
| Tool discovery | Goose discovered `kubectl_get`, `kubectl_list_events`, `submit_result` |
| Tool execution | `kubectl_get` called with correct args, response processed |
| Structured output | `submit_result` called with root_cause, confidence, affected_resources |
| Token usage | 2,911 tokens (2,494 input + 417 output) |
| Execution time | 6.7 seconds |
| JSON output | `--output-format json` produces parseable structured output |

**Key Finding**: Goose's `gcp_vertex_ai` provider handles GCP authentication
transparently via ADC. No API keys needed in configuration -- only
`GCP_PROJECT_ID` and `GCP_LOCATION` environment variables.

**ACP Server Implication**: The ACP server starts Goose as a subprocess with
`--with-streamable-http-extension` pointing to its own MCP proxy. The proxy
intercepts tool calls for budget/audit enforcement before forwarding to KA.

---

## 8. Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| Goose CLI version breaking changes | Medium | Pin version in Dockerfile; test on upgrade |
| No structured I/O at recipe boundary | Medium | ACP server owns schema validation |
| MCP session lifecycle complexity | Low | ACP server manages Goose process lifecycle |
| Upstream project direction changes | Medium | OCI image pins exact version; recipes are portable |

---

## 9. Dependencies

- Goose CLI (`ghcr.io/block/goose`) - Rust binary
- ACP server (Go) - PROPOSAL-EXT-003 addendum architecture
- OCI registry for image distribution

---

## 10. Implementation Estimate

| Component | Effort |
|---|---|
| Goose recipe authoring for core investigation types | 2 weeks |
| ACP server Goose adapter (process lifecycle, MCP proxy) | 3 weeks |
| OCI image build pipeline | 1 week |
| Integration testing with KA | 2 weeks |
| **Total** | **8 weeks (1 dev)** |
