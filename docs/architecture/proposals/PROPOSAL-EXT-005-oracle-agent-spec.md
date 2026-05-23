# PROPOSAL-EXT-005: Oracle Agent Spec (PyAgentSpec + LangGraph) as AgenticWorkflow Runtime

**Status**: Draft  
**Date**: May 19, 2026  
**Author**: Kubernaut Architecture Team  
**Confidence**: 97% (Spike 1 + Follow-up Spike 2 validated: OAS YAML creation, LangGraph compilation, real Vertex AI invocation with 7 autonomous tool calls and structured RCA)  
**Related**: [PROPOSAL-EXT-003](PROPOSAL-EXT-003-goose-runtime-evaluation.md) (OAS addendum), [PROPOSAL-EXT-004](PROPOSAL-EXT-004-goose-recipes.md) (Goose runtime), [PROPOSAL-EXT-006](PROPOSAL-EXT-006-deep-agents.md) (Deep Agents runtime)  
**Spike Code**: [spikes/pyagentspec-langgraph/](../../../spikes/pyagentspec-langgraph/) (scripts 01-04, all pass)  
**Spike Summary**: [SPIKE-PYAGENTSPEC-LANGGRAPH](../spikes/SPIKE-PYAGENTSPEC-LANGGRAPH.md), [SPIKE-ACP-ENFORCEMENT](../spikes/SPIKE-ACP-ENFORCEMENT.md), [SPIKE-OCI-RUNTIME-CONTRACT](../spikes/SPIKE-OCI-RUNTIME-CONTRACT.md)  
**Tracking**: [#1240](https://github.com/jordigilh/kubernaut/issues/1240) (umbrella)  
**Target**: v1.6 milestone

---

## 1. Purpose

This proposal defines Oracle Agent Spec (OAS) as one of three pluggable
runtime types for the `AgenticWorkflow` CRD. The runtime uses PyAgentSpec
(Oracle's Python SDK for Agent Spec) with the LangGraph adapter to compile
declarative agent definitions into executable LangGraph `CompiledStateGraph`
instances.

This supersedes the `open-agent-sdk-go` (Go-based) approach documented in
PROPOSAL-EXT-003's addendum. That spike (`spikes/oas-runtime/`) validated
the Go SDK but required maintaining a custom OAS-to-Go adapter. The
PyAgentSpec + LangGraph path uses upstream native tooling with zero
custom format conversion.

---

## 2. Architecture

### 2.1 AgenticWorkflow CRD (runtime: oas)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: AgenticWorkflow
metadata:
  name: oomkill-rca-oas
spec:
  version: "1.0.0"
  description:
    what: "OOMKill RCA investigation using Oracle Agent Spec"
    whenToUse: "OOMKill alerts on production deployments"
  execution:
    runtime: oas
    bundle: registry.example.com/kubernaut/oomkill-oas:v1.0.0
    bundleDigest: sha256:def456...
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
KA → ACP Server → Python runtime (PyAgentSpec + LangGraph) → Tools (via ACP enforcement)
```

1. KA receives signal and selects matching `AgenticWorkflow` (type: oas)
2. ACP server pulls OCI image, extracts agent spec from `/spec/agent.yaml`
3. ACP server builds `tool_registry` from enforcement layer
4. `AgentSpecLoader.load_yaml()` compiles spec to `CompiledStateGraph`
5. ACP server formats signal context as `HumanMessage` and invokes graph
6. LangGraph ReAct loop drives tool calls through enforcement layer
7. ACP server extracts structured RCA from `submit_result` tool call

### 2.3 OCI Image Contract

| Label | Value |
|---|---|
| `ai.kubernaut.runtime` | `oas` |
| `ai.kubernaut.spec-version` | `25.4.1` |
| `ai.kubernaut.entrypoint` | `/spec/agent.yaml` |
| `ai.kubernaut.tools` | Comma-separated tool names |

```dockerfile
FROM python:3.12-slim
LABEL ai.kubernaut.runtime="oas"
LABEL ai.kubernaut.spec-version="25.4.1"
LABEL ai.kubernaut.entrypoint="/spec/agent.yaml"

COPY requirements.txt /runtime/requirements.txt
RUN pip install --no-cache-dir -r /runtime/requirements.txt

COPY agent.yaml /spec/agent.yaml
COPY kubernaut_oas/ /runtime/kubernaut_oas/
ENTRYPOINT ["python", "-m", "kubernaut_oas", "/spec/agent.yaml"]
```

---

## 3. OAS Agent Definition

The OAS YAML spec defines the investigation agent declaratively. Validated
in Spike 1 (`spikes/pyagentspec-langgraph/kubernaut-rca-investigator.yaml`):

```yaml
component_type: Agent
name: kubernaut-rca-investigator
description: Investigates Kubernetes alert signals and produces structured RCA
llm_config:
  component_type: OpenAiCompatibleConfig
  name: vertex-anthropic
  url: https://us-east5-aiplatform.googleapis.com/v1/projects/PROJECT/locations/us-east5/endpoints/openapi  # pre-commit:allow-sensitive
  model_id: claude-sonnet-4-20250514
system_prompt: |
  You are a Kubernetes incident investigator for the Kubernaut platform.
  A signal has fired. Use the available tools to inspect the affected
  resource, check events, query metrics, and determine the root cause.
tools:
  - component_type: ServerTool
    name: kubectl_get
    description: Get a Kubernetes resource by kind, name, and namespace
    inputs:
      - title: kind
        type: string
      - title: name
        type: string
      - title: namespace
        type: string
  - component_type: ServerTool
    name: kubectl_list_events
    description: List Kubernetes events for a resource
    inputs:
      - title: namespace
        type: string
      - title: resource_name
        type: string
  - component_type: ServerTool
    name: prometheus_query
    description: Execute a PromQL query
    inputs:
      - title: query
        type: string
      - title: time_range
        type: string
  - component_type: ServerTool
    name: submit_result
    description: Submit the structured RCA result
    inputs:
      - title: result
        type: object
        properties:
          root_cause:
            type: string
          confidence:
            type: number
            minimum: 0
            maximum: 1
          affected_resources:
            type: array
            items:
              type: string
          remediation_suggested:
            type: boolean
        required:
          - root_cause
          - confidence
agentspec_version: 25.4.1
```

---

## 4. Spike Findings (Validated)

### Spike 1: PyAgentSpec + LangGraph Adapter (Mock LLM)

| Finding | Impact |
|---|---|
| **F1**: PyAgentSpec YAML round-trips cleanly | OAS specs can be authored programmatically or by hand |
| **F2**: `AgentSpecLoader.load_yaml()` produces `CompiledStateGraph` | Standard LangGraph execution model |
| **F3**: Tools map to `ServerTool` via `tool_registry` | Natural interception point for enforcement |
| **F4**: LLM credentials eager-validated at compile time | ACP server must inject credentials before compilation |
| **F5**: Conversational model (no typed I/O on Agent) | Signal context via `HumanMessage`; RCA via `submit_result` |
| **F6**: Missing tools cause `ValueError` at compile time | Tool registry must be complete before loading spec |

### Follow-up Spike 2: Real Vertex AI Invocation (May 2026)

**Result**: PASS

| Aspect | Result |
|---|---|
| LLM client | `ChatAnthropic` with `AnthropicVertex` client injection |
| Model | `claude-sonnet-4@20250514` via Vertex AI us-east5 |
| Authentication | GCP ADC (Application Default Credentials) |
| Tool calls | 7 autonomous calls: kubectl_get x3, kubectl_list_events, prometheus_query x2, submit_result |
| RCA quality | Structured root cause with confidence, affected_resources, remediation |
| Messages | 16 total (multi-turn ReAct loop) |
| Execution time | 21 seconds |

**Key Finding**: `ChatAnthropic` + `AnthropicVertex` client injection works
seamlessly. The ACP server creates the `AnthropicVertex` client from
K8s Secret credentials and injects it into the `ChatAnthropic` model before
graph compilation. No credential environment variables needed in the OCI image.

---

## 5. Feature Parity with KA v1.5

| Feature | KA v1.5 | OAS Runtime | Notes |
|---|---|---|---|
| Tool call budget | AnomalyDetector | ACP enforcement layer | Spike 2 validated |
| Shadow agent | alignment.ToolProxy | ACP shadow feed | Hybrid: local canary + remote grounding |
| Audit events | audit.StoreBestEffort | ACP audit sink | Same event schema |
| Phase-based tools | PhaseToolMap | ACP tool filtering | Rebuild tool_registry per phase |
| Structured output | submit_result sentinel | submit_result ServerTool | JSON Schema validated in spec |
| LLM streaming | llm.Client | LangGraph streaming | Native async streaming |
| Multi-provider | SwappableClient | OpenAiCompatibleConfig | Supports Vertex AI, OpenAI, Ollama, OCI GenAI |

---

## 6. Strengths

- **Declarative schema**: Full JSON Schema support for tool I/O; strongest
  typing of the three runtimes
- **Framework-agnostic**: OAS specs can be executed by any adapter (LangGraph,
  AutoGen, CrewAI); not locked to one runtime
- **Upstream SDK**: PyAgentSpec is maintained by Oracle; production-grade
- **LangGraph integration**: Native adapter compiles to standard ReAct pattern
- **Composable**: OAS supports `ManagerWorkers`, `Swarm`, and `Flow` patterns
  for multi-agent compositions (v1.7+ potential)

---

## 7. Limitations

- **Python dependency**: Requires Python runtime in OCI image (shared with
  Deep Agents, but not with Goose)
- **Agent inputs/outputs are conversational**: OAS `Agent` component does not
  accept arbitrary typed properties. Schema enforcement for signal context is
  the ACP server's responsibility.
- **Eager credential validation**: `ChatOpenAI` client created at compile time
  requires API credentials before graph compilation
- **pyagentspec version coupling**: Abstract types (`Tool`, `LlmConfig`) require
  concrete subclasses; API may change across major versions

---

## 8. Comparison with Previous Go-Based Approach

| Aspect | open-agent-sdk-go (PROPOSAL-EXT-003 addendum) | PyAgentSpec + LangGraph (this proposal) |
|---|---|---|
| Language | Go (native to Kubernaut) | Python (separate runtime) |
| OAS coverage | Subset (custom adapter) | Full spec (upstream SDK) |
| Maintenance | Kubernaut-owned adapter | Upstream Oracle-maintained |
| Multi-agent support | Limited (single agent) | Full (ManagerWorkers, Swarm, Flow) |
| Runtime isolation | In-process (Go) | Separate container (Python) |
| Dependency footprint | Go binary (~10MB) | Python image (~200MB slim) |

The PyAgentSpec approach trades Go-native in-process execution for full spec
coverage and zero adapter maintenance. The separate Python container aligns
with the multi-runtime OCI packaging model defined in Spike 3.

---

## 9. Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| Python 3.14 compatibility warnings | Low | Use Python 3.12-slim in OCI image; no functional impact |
| PyAgentSpec major version breaking changes | Medium | Pin version in requirements.txt; integration test suite |
| LangGraph deprecation warnings (pydantic v1) | Low | Upstream tracking; cosmetic only |
| Image size (~200MB for Python) | Medium | Multi-stage builds; shared base layer with Deep Agents |

---

## 10. Dependencies

- PyAgentSpec v26.1.0+ (`pyagentspec[langgraph]`)
- LangGraph v0.6.11+
- Python 3.12+ (3.10 minimum)
- ACP server (Go) - enforcement layer (Spike 2 validated)

---

## 11. Implementation Estimate

| Component | Effort |
|---|---|
| Python runtime adapter (`kubernaut_oas` package) | 2 weeks |
| OAS spec authoring for core investigation types | 2 weeks |
| ACP server OAS adapter (lifecycle, signal formatting) | 3 weeks |
| OCI image build pipeline | 1 week |
| Integration testing with KA | 2 weeks |
| **Total** | **10 weeks (1 dev)** |
