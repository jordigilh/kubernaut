# PROPOSAL-EXT-006: LangChain Deep Agents as AgenticWorkflow Runtime

**Status**: Draft  
**Date**: May 19, 2026  
**Author**: Kubernaut Architecture Team  
**Confidence**: 96% (Follow-up Spike 3 validated: create_deep_agent with sub-agent delegation, real Vertex AI, tool scoping, budget tracking, and structured RCA)  
**Related**: [PROPOSAL-EXT-004](PROPOSAL-EXT-004-goose-recipes.md) (Goose runtime), [PROPOSAL-EXT-005](PROPOSAL-EXT-005-oracle-agent-spec.md) (OAS runtime)  
**Motivation**: [RHPDS Agentic AIOps Workshop](https://rhpds.github.io/agentic-aiops-showroom/modules/index.html) (long-horizon, exploratory investigations)  
**Spike Code**: [pyagentspec-langgraph/05_deepagents_validation.py](../spikes/pyagentspec-langgraph/05_deepagents_validation.py) (sub-agent delegation + budget tracking validated)  
**Spike Summary**: [SPIKE-DEEP-AGENTS](../spikes/SPIKE-DEEP-AGENTS.md), [SPIKE-ACP-ENFORCEMENT](../spikes/SPIKE-ACP-ENFORCEMENT.md), [SPIKE-OCI-RUNTIME-CONTRACT](../spikes/SPIKE-OCI-RUNTIME-CONTRACT.md)  
**Tracking**: [#1240](https://github.com/jordigilh/kubernaut/issues/1240) (umbrella)  
**Target**: v1.6 milestone

---

## 1. Purpose

This proposal defines LangChain Deep Agents as one of three pluggable
runtime types for the `AgenticWorkflow` CRD. Deep Agents
(langchain-ai/deepagents) provide an opinionated agent harness built on
LangGraph for **long-horizon, exploratory investigations** that require:

- Multi-step planning with dynamic sub-agent spawning
- Hypothesis generation and parallel exploration
- Sandboxed filesystem for intermediate results
- Self-reflective reasoning across investigation steps

The primary use case is **specialist investigations** where the failure is
novel, the root cause is not obvious, and the agent needs to reason freely
across multiple hypotheses -- the pattern demonstrated by the RHPDS Agentic
AIOps workshop.

---

## 2. Architecture

### 2.1 AgenticWorkflow CRD (runtime: deepagent)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: AgenticWorkflow
metadata:
  name: complex-rca-deepagent
spec:
  version: "1.0.0"
  description:
    what: "Complex multi-hypothesis RCA using Deep Agents"
    whenToUse: "Novel failures requiring exploratory investigation"
    whenNotToUse: "Known failure patterns with established remediation"
  execution:
    runtime: deepagent
    bundle: registry.example.com/kubernaut/complex-rca-deepagent:v1.0.0
    bundleDigest: sha256:ghi789...
    runtimeConfig:
      maxToolCalls: 50
      maxToolCallsPerTool: 15
      shadowAgentEnabled: true
      planningEnabled: true
      maxSubAgents: 3
  labels:
    severity: [critical]
    environment: [production]
    component: [Deployment, StatefulSet, DaemonSet]
```

### 2.2 Execution Flow

```
KA → ACP Server → Python runtime (Deep Agents / LangGraph) → Tools (via ACP enforcement)
```

1. KA receives signal and selects matching `AgenticWorkflow` (type: deepagent)
2. ACP server pulls OCI image, extracts agent definition from `/spec/agent.yaml`
3. ACP server builds `tool_registry` from enforcement layer
4. Deep Agents framework creates planning agent + specialist sub-agents
5. Planning agent decomposes investigation into sub-tasks
6. Sub-agents execute in parallel, each with own tool budget slice
7. Planning agent synthesizes findings and produces structured RCA
8. ACP server extracts RCA from `submit_result` tool call

### 2.3 Deep Agent Investigation Pattern

```
┌─────────────────────────────────────────────┐
│                Planning Agent                │
│  "OOMKill on web-app in production"          │
│                                              │
│  Plan:                                       │
│  1. Check resource state (Sub-Agent A)       │
│  2. Analyze memory metrics (Sub-Agent B)     │
│  3. Review recent changes (Sub-Agent C)      │
└────────┬──────────┬──────────┬──────────────┘
         │          │          │
    ┌────▼───┐ ┌───▼────┐ ┌──▼─────┐
    │ Sub-A  │ │ Sub-B  │ │ Sub-C  │
    │kubectl │ │promQL  │ │events  │
    │get/list│ │queries │ │history │
    └────┬───┘ └───┬────┘ └──┬─────┘
         │         │         │
         ▼         ▼         ▼
    ┌─────────────────────────────────┐
    │      Planning Agent (synthesis) │
    │  Root cause: OOM due to memory  │
    │  leak in v2.3.1 (confidence: 94)│
    └─────────────────────────────────┘
```

### 2.4 OCI Image Contract

| Label | Value |
|---|---|
| `ai.kubernaut.runtime` | `deepagent` |
| `ai.kubernaut.spec-version` | `0.1` |
| `ai.kubernaut.entrypoint` | `/spec/agent.yaml` |
| `ai.kubernaut.tools` | Comma-separated tool names |

```dockerfile
FROM python:3.12-slim
LABEL ai.kubernaut.runtime="deepagent"
LABEL ai.kubernaut.spec-version="0.1"
LABEL ai.kubernaut.entrypoint="/spec/agent.yaml"

COPY requirements.txt /runtime/requirements.txt
RUN pip install --no-cache-dir -r /runtime/requirements.txt

COPY agent.yaml /spec/agent.yaml
COPY kubernaut_deepagent/ /runtime/kubernaut_deepagent/
ENTRYPOINT ["python", "-m", "kubernaut_deepagent", "/spec/agent.yaml"]
```

---

## 3. Deep Agent Definition

```yaml
name: complex-rca-investigator
description: Multi-hypothesis RCA investigation with planning
version: "0.1"

planning_agent:
  system_prompt: |
    You are the lead investigator for the Kubernaut platform.
    Decompose the incident into independent investigation threads.
    Each thread should explore a specific hypothesis.
    Synthesize all findings into a final root cause analysis.
  llm_config:
    provider: vertex-anthropic
    model: claude-sonnet-4-20250514

specialist_agents:
  - name: resource-inspector
    system_prompt: |
      You inspect Kubernetes resources to identify configuration
      issues, status anomalies, and ownership chains.
    tools: [kubectl_get, kubectl_list_events]

  - name: metrics-analyst
    system_prompt: |
      You analyze Prometheus metrics to identify performance
      degradation, resource saturation, and anomalous trends.
    tools: [prometheus_query]

  - name: change-auditor
    system_prompt: |
      You review recent deployment changes, config updates,
      and event history to correlate with the incident timeline.
    tools: [kubectl_get, kubectl_list_events]

synthesis:
  output_tool: submit_result
  schema:
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
      investigation_threads:
        type: array
        items:
          type: object
          properties:
            hypothesis:
              type: string
            findings:
              type: string
            confidence:
              type: number
    required: [root_cause, confidence]
```

---

## 4. Spike Validation Results (May 2026)

### Follow-up Spike 3: Deep Agents Library Validation

**Result**: PASS (both tests)

#### Test 1: Sub-agent Delegation (Planner + Specialists)

| Aspect | Result |
|---|---|
| Framework | `create_deep_agent` from deepagents v0.6.3 |
| LLM | `claude-sonnet-4@20250514` via Vertex AI (AnthropicVertex client injection) |
| Sub-agents | k8s-investigator (kubectl_get, kubectl_list_events) + metrics-investigator (prometheus_query) |
| Coordinator tools | submit_result only |
| Delegation | Coordinator used `task()` tool to delegate to both specialists |
| Tool isolation | k8s-investigator made 29 tool calls, metrics-investigator made separate calls |
| RCA output | submit_result called with structured root_cause, confidence, affected_resources |
| Messages | 7 coordinator-level messages (sub-agent messages are internal) |

#### Test 2: Budget Tracking

| Aspect | Result |
|---|---|
| Tool call counting | Per-tool and total counts tracked via handler wrappers |
| Per-tool breakdown | kubectl_get: 1, kubectl_list_events: 1, submit_result: 1 |
| ACP integration point | Wrap tool handlers in `EnforcementLayer` (Spike 2 validated) |

**Key Findings**:

| Finding | Impact |
|---|---|
| **F1**: `SubAgent` accepts scoped tool lists | Natural budget isolation per specialist |
| **F2**: Sub-agents inherit model but get isolated tool sets | Coordinator can use different model than specialists |
| **F3**: Tool handlers are sync callables | Budget/audit wrapping is trivial -- wrap handler function |
| **F4**: `task()` tool carries delegation context | Shadow agent can inspect sub-agent task descriptions |
| **F5**: Coordinator synthesizes sub-agent findings | Natural RCA aggregation point |
| **F6**: 100+ seconds execution for complex investigation | Budget timeout enforcement is critical |

---

## 5. Feature Parity with KA v1.5

| Feature | KA v1.5 | Deep Agents Runtime | Notes |
|---|---|---|---|
| Tool call budget | AnomalyDetector | ACP enforcement layer | Budget split across sub-agents |
| Shadow agent | alignment.ToolProxy | ACP shadow feed | Per sub-agent shadow evaluation |
| Audit events | audit.StoreBestEffort | ACP audit sink | Per sub-agent audit trail |
| Phase-based tools | PhaseToolMap | Per-specialist tool sets | Finer-grained than phase-based |
| Structured output | submit_result sentinel | submit_result tool | Extended schema with threads |
| LLM streaming | llm.Client | LangGraph streaming | Native async streaming |
| Multi-provider | SwappableClient | Per-agent LLM config | Different models per specialist |

### Budget Distribution for Sub-Agents

The ACP enforcement layer distributes the total budget across sub-agents:

```
Total budget: 50 tool calls
  Planning agent: 5 calls (planning only)
  Sub-Agent A: 15 calls
  Sub-Agent B: 15 calls
  Sub-Agent C: 15 calls
```

Each sub-agent gets an isolated enforcement layer instance with its own
per-tool limits but shared total budget tracking via the parent layer.

---

## 6. Strengths

- **Long-horizon reasoning**: Planning + sub-agent model handles novel
  failures that KA's linear pipeline cannot
- **Parallel exploration**: Multiple hypotheses investigated simultaneously
- **Richer RCA output**: Investigation threads provide auditable reasoning
  chains per hypothesis
- **Sandboxed filesystem**: Intermediate results persisted between steps
  (useful for large metric datasets or log analysis)
- **Model flexibility**: Different LLM models per specialist (e.g., fast
  model for resource inspection, reasoning model for synthesis)

---

## 7. Limitations

- **Higher LLM cost**: Planning + N sub-agents consume more tokens than
  a single ReAct loop
- **Longer execution time**: Planning phase + parallel sub-agents + synthesis
  takes 2-5x longer than single-agent investigation
- **Complexity**: More moving parts; harder to debug when sub-agents diverge
- **Python dependency**: Shares Python base with OAS but larger dep tree
  (deepagents + langgraph + langchain-core)
- **Upstream maturity**: `langchain-ai/deepagents` is relatively new
  (early 2026); API may evolve

---

## 8. When to Use Deep Agents vs Other Runtimes

| Scenario | Recommended Runtime | Rationale |
|---|---|---|
| Known failure pattern with established remediation | **goose** | Simple recipe, fast execution |
| Standard RCA with tool-based investigation | **oas** | Declarative, well-typed, single agent |
| Novel failure, multiple hypotheses needed | **deepagent** | Planning + parallel specialists |
| Complex multi-service cascading failure | **deepagent** | Sub-agents can investigate each service |
| Time-sensitive critical alert | **goose** or **oas** | Faster execution, lower cost |
| Post-incident deep analysis | **deepagent** | Thoroughness over speed |

---

## 9. RHPDS Agentic AIOps Workshop Alignment

The RHPDS workshop demonstrates a pattern where:
1. An orchestrator agent receives an incident
2. It spawns specialist agents for different investigation aspects
3. Specialists use Ansible and K8s tools to gather evidence
4. The orchestrator synthesizes findings

Deep Agents in Kubernaut mirrors this pattern:
- **KA** = workshop orchestrator (CRD lifecycle, signal routing)
- **Planning Agent** = workshop lead investigator
- **Specialist Agents** = workshop domain experts
- **ACP enforcement** = workshop guardrails (budget, shadow, audit)

The key difference is that Kubernaut adds enterprise features the workshop
lacks: budget enforcement, shadow agent security, structured audit trail,
OCI-based workflow distribution, and CRD-driven lifecycle management.

---

## 10. Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| deepagents API instability | Medium | Pin version; abstract behind adapter |
| Higher token consumption | Medium | Budget caps in runtimeConfig; cost monitoring |
| Sub-agent divergence (conflicting findings) | Low | Planning agent handles synthesis; conflict detection |
| Execution time exceeds investigation SLO | Medium | configurable timeout; fallback to single-agent |
| Python image size (~250MB with full deps) | Medium | Shared base layer with OAS runtime |

---

## 11. Dependencies

- LangGraph v0.6.11+ (`langgraph`)
- LangChain Core v0.3.86+ (`langchain-core`)
- deepagents (upstream `langchain-ai/deepagents`)
- Python 3.12+
- ACP server (Go) - enforcement layer (Spike 2 validated)

---

## 12. Implementation Estimate

| Component | Effort |
|---|---|
| Deep Agent definition format and parser | 2 weeks |
| Python runtime adapter (`kubernaut_deepagent` package) | 3 weeks |
| ACP server Deep Agent adapter (sub-agent lifecycle, budget split) | 3 weeks |
| Planning agent templates for core investigation types | 2 weeks |
| OCI image build pipeline | 1 week |
| Integration testing with KA | 2 weeks |
| **Total** | **13 weeks (1 dev)** |
