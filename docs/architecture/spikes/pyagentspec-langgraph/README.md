# Spike: PyAgentSpec + LangGraph + Deep Agents Validation

**Status**: Complete  
**Date**: 2026-05-19 (initial), 2026-05-23 (follow-up spikes)  
**Confidence**: 97% (OAS), 96% (Deep Agents)

## Objective

Validate that Oracle Agent Spec (OAS) definitions can be authored, serialized to
YAML, loaded by the PyAgentSpec LangGraph adapter, and compiled into an
executable LangGraph `CompiledStateGraph` suitable for Kubernaut investigations.

## Environment

| Dependency | Version |
|---|---|
| Python | 3.14.3 |
| pyagentspec | 26.1.0 |
| langgraph | 0.6.11 |
| langchain-core | 0.3.86 |
| OAS spec version | 25.4.1 |

## Test Results

| # | Test | Result |
|---|---|---|
| 1 | OAS YAML spec creation (Agent + ServerTool) | PASS |
| 2 | YAML round-trip (serialize + load) | PASS |
| 3 | LangGraph compilation (CompiledStateGraph) | PASS |
| 4 | Graph structure validation (ReAct pattern) | PASS |
| 5 | Mock invocation (3 tool calls, RCA extraction) | PASS |
| 6 | RCA schema validation | PASS |

## Key Findings

### F1: OAS YAML Serialization Works Cleanly

PyAgentSpec's `AgentSpecSerializer` produces valid YAML from programmatic
`Agent` definitions. The `AgentSpecLoader.load_yaml()` round-trips without
data loss.

### F2: Agent -> LangGraph Compilation

`AgentSpecLoader.load_yaml()` produces a `CompiledStateGraph` with the
standard ReAct pattern:

```
__start__ -> agent -> tools -> agent (loop) -> __end__
```

### F3: Tool Mapping via ServerTool + tool_registry

OAS `ServerTool` entries map to runtime-provided tool handlers via a
`tool_registry` dictionary passed to `AgentSpecLoader`. The ACP server
would populate this registry with handlers that proxy KA's MCP tools.

**Critical constraint**: ALL tools declared in the OAS spec MUST exist in
the tool_registry at compile time. Missing tools cause `ValueError`.

### F4: LLM Credentials Eager-Validated

The LangGraph adapter creates the `ChatOpenAI` (or compatible) client
during graph compilation, which triggers credential validation. This means:

- The ACP server MUST have API credentials available before compiling the
  OAS spec
- For Vertex AI: `OPENAI_API_KEY` env var or `api_key` field in
  `OpenAiCompatibleConfig`
- Credentials cannot be deferred to execution time

### F5: Conversational Model (No Typed I/O)

OAS `Agent` does not accept arbitrary named `inputs`/`outputs` properties.
The agent operates in a conversational model where context is injected via
`HumanMessage`. This means:

- Signal context and enrichment data must be formatted as a text message
- The ACP server is responsible for formatting the Kubernaut signal into
  the investigation prompt
- Structured output is extracted from `submit_result` tool calls, not from
  typed output properties

### F6: Component Type Hierarchy

Several OAS types are abstract and cannot be instantiated directly:

| Abstract | Concrete Subclass(es) |
|---|---|
| `LlmConfig` | `OpenAiCompatibleConfig`, `OpenAiConfig`, `OciGenAiConfig`, `OllamaConfig`, `VllmConfig` |
| `Tool` | `ServerTool`, `ClientTool`, `RemoteTool`, `BuiltinTool`, `MCPTool` |

For Kubernaut, `ServerTool` is the natural fit (tools provided by the
server-side runtime), and `OpenAiCompatibleConfig` supports Vertex AI's
OpenAI-compatible endpoint.

## Architecture Implications for ACP Server

1. **Runtime compilation**: ACP server compiles OAS YAML at pod startup
   or per-investigation, not at build time
2. **Tool registry**: Maps OAS tool names to ACP handler functions that
   wrap KA's MCP tools with interception (audit, budgets, shadow agent)
3. **Credential injection**: K8s Secret -> env var -> `OpenAiCompatibleConfig`
4. **Signal formatting**: ACP server formats Kubernaut signal + enrichment
   into a `HumanMessage` for the agent
5. **Result extraction**: ACP server watches for `submit_result` tool calls
   and extracts the structured RCA
6. **Python runtime**: OCI image needs Python 3.10+ with pyagentspec[langgraph]

## Follow-up Spike 2: Real Vertex AI Invocation (2026-05-23)

**Result**: PASS

Validated real LLM invocation through LangGraph using `ChatAnthropic` with
`AnthropicVertex` client injection.

| Aspect | Result |
|---|---|
| LLM client | `ChatAnthropic` + `AnthropicVertex` client (not `ChatVertexAI`) |
| Model | `claude-sonnet-4@20250514` via Vertex AI us-east5 |
| Auth | GCP ADC (no API keys in config) |
| Tool calls | 7 autonomous: kubectl_get x3, kubectl_list_events, prometheus_query x2, submit_result |
| RCA | Structured root_cause, confidence, affected_resources, remediation |
| Messages | 16 total (multi-turn ReAct) |
| Time | 21 seconds |

**Key Finding**: `ChatVertexAI` routes to `publishers/google/` (Gemini only).
For Claude on Vertex, use `ChatAnthropic` with `AnthropicVertex` client injection.

## Follow-up Spike 3: Deep Agents Validation (2026-05-23)

**Result**: PASS

Validated `create_deep_agent` with sub-agent delegation, tool scoping, and
budget tracking using real Vertex AI.

| Aspect | Result |
|---|---|
| Framework | deepagents v0.6.3 (`create_deep_agent`) |
| Sub-agents | k8s-investigator + metrics-investigator, scoped tool sets |
| Coordinator | submit_result only; delegates via `task()` tool |
| Budget | Per-tool and total counting via handler wrappers |
| RCA | Coordinator synthesized sub-agent findings into structured result |
| Time | ~100 seconds for full planning + specialist + synthesis |

## Files

| File | Purpose |
|---|---|
| `01_create_spec.py` | Creates OAS Agent and serializes to YAML |
| `02_load_and_compile.py` | Loads YAML and compiles to LangGraph graph |
| `03_invoke_mock.py` | End-to-end mock investigation with FakeChatModel |
| `04_invoke_vertexai.py` | Real Vertex AI invocation with mock tools (Follow-up Spike 2) |
| `05_deepagents_validation.py` | Deep Agents sub-agent delegation + budget (Follow-up Spike 3) |
| `kubernaut-rca-investigator.yaml` | Generated OAS YAML spec |

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Python 3.14 compatibility warnings (pydantic v1) | Low | Langchain team tracking; no functional impact |
| Eager credential validation blocks offline testing | Medium | Use dummy key for graph structure tests; mock LLM for invocation tests |
| `ServerTool` handler interface may change across pyagentspec versions | Medium | Pin pyagentspec version in requirements.txt |
| No typed I/O means schema enforcement is the ACP server's responsibility | Medium | Validate submit_result args against JSON schema in ACP |
