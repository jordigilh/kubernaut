# Spike: PyAgentSpec + LangGraph Adapter Validation

**Date**: May 19, 2026 (initial), May 23, 2026 (follow-up)
**Status**: COMPLETED — technique validated; the CRD-level runtime-selection model it targeted is superseded by [#1536](https://github.com/jordigilh/kubernaut/issues/1536)
**Duration**: 2 sessions
**Relates to**: [PROPOSAL-EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md) (superseded)
**Code**: [pyagentspec-langgraph/](pyagentspec-langgraph/)

---

## Objective

Validate that Oracle Agent Spec (OAS) definitions can be authored, serialized
to YAML, loaded by PyAgentSpec's LangGraph adapter, compiled into executable
`CompiledStateGraph` instances, and invoked with real Vertex AI for Kubernaut
investigations.

## What Was Built

### Initial Spike (May 19)

| File | Purpose |
|---|---|
| `01_create_spec.py` | Programmatic OAS Agent creation with `ServerTool` + `OpenAiCompatibleConfig` |
| `02_load_and_compile.py` | YAML round-trip and LangGraph graph compilation/structure validation |
| `03_invoke_mock.py` | End-to-end mock investigation with custom `FakeToolChatModel` |
| `kubernaut-rca-investigator.yaml` | Generated OAS YAML spec for RCA investigation |

### Follow-up Spike 2 (May 23)

| File | Purpose |
|---|---|
| `04_invoke_vertexai.py` | Real Vertex AI invocation via `ChatAnthropic` + `AnthropicVertex` client |

## Test Results

| # | Test | Result |
|---|---|---|
| 1 | OAS YAML spec creation (Agent + ServerTool) | PASS |
| 2 | YAML round-trip (serialize + load) | PASS |
| 3 | LangGraph compilation (CompiledStateGraph) | PASS |
| 4 | Graph structure validation (ReAct pattern) | PASS |
| 5 | Mock invocation (3 tool calls, RCA extraction) | PASS |
| 6 | RCA schema validation | PASS |
| 7 | Real Vertex AI invocation (7 autonomous tool calls) | PASS |

## Key Findings

| Finding | Impact |
|---|---|
| **F1**: PyAgentSpec YAML round-trips cleanly | OAS specs can be authored programmatically or by hand |
| **F2**: `AgentSpecLoader.load_yaml()` produces `CompiledStateGraph` | Standard LangGraph execution model |
| **F3**: Tools map to `ServerTool` via `tool_registry` | Natural interception point for ACP enforcement |
| **F4**: LLM credentials eager-validated at compile time | ACP server must inject credentials before compilation |
| **F5**: Conversational model (no typed I/O on Agent) | Signal context via `HumanMessage`; RCA via `submit_result` |
| **F6**: Missing tools cause `ValueError` at compile time | Tool registry must be complete before loading spec |
| **F7**: `ChatVertexAI` routes to `publishers/google/` only | For Claude on Vertex, use `ChatAnthropic` + `AnthropicVertex` client injection |
| **F8**: Real Vertex AI produces autonomous multi-turn investigation | 7 tool calls, 16 messages, 21 seconds |

## Environment

| Dependency | Version |
|---|---|
| Python | 3.14.3 |
| pyagentspec | 26.1.0 |
| langgraph | 0.6.11 |
| langchain-core | 0.3.86 |
| langchain-anthropic | (latest) |
| anthropic | (latest, with vertex extras) |
| OAS spec version | 25.4.1 |

## Confidence

**97%** — Both mock and real Vertex AI invocations pass. Remaining 3% is
production hardening (error recovery, timeout handling, credential rotation).
