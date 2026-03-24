# BR-HAPI-433-001: Go LLM Framework Evaluation

**Parent**: [BR-HAPI-433: Go Language Migration](BR-HAPI-433-go-language-migration.md)
**Category**: HolmesGPT-API Service
**Priority**: P0
**Status**: ✅ Approved — LangChainGo SELECTED
**Date**: 2026-03-04

---

## 📋 **Business Need**

The HAPI Go rewrite requires a Go LLM framework to replace HolmesGPT's Python SDK. The framework must support multi-provider LLM access, tool calling, and structured output while maintaining a lean dependency footprint.

---

## 🎯 **Evaluation Criteria**

| Criterion | Weight | Rationale |
|-----------|--------|-----------|
| Pure Go implementation | Must-have | Core objective of #433 is eliminating Python |
| Multi-provider LLM support | Must-have | Customers use OpenAI, Azure, Vertex AI, Ollama |
| Tool calling support | Must-have | Agentic loop requires function/tool calls |
| Structured output (JSON) | Must-have | Result parsing requires deterministic JSON |
| Lean dependency footprint | High | Security motivation — fewer deps = smaller attack surface |
| English documentation | High | Team productivity |
| Community maturity | Medium | Long-term maintenance confidence |
| Per-turn tool scoping | Desired | Token optimization and security (can be Kubernaut-owned) |
| CaMeL support | Desired | Prompt injection defense (can be Kubernaut-owned) |
| Multi-agent orchestration | Low (v1.3) | Not needed for current investigation flow |

---

## 📊 **Frameworks Evaluated**

### kagent — ❌ NO-GO

**What it is**: Kubernetes-native CRD-based agent platform by kagent-dev.

**Disqualification reason**: Agent runtime is **Python-based** (AutoGen 0.4). The Go layer is limited to CRD management and an API server. Does not eliminate Python from the HAPI stack.

**Evidence**: kagent `CLAUDE.md` explicitly states "the Python components handle the actual agent execution and LLM interactions." See [#505](https://github.com/jordigilh/kubernaut/issues/505).

### LangChainGo — ✅ SELECTED for v1.3

**What it is**: Go port of LangChain. ~5.5k stars, active community, backed by LangChain org.

**PoC**: `kubernaut-poc-langchaingo/` — full investigation flow validated against mock-llm.

### Eino (CloudWeGo/ByteDance) — ⚠️ VIABLE, deferred to v1.4+

**What it is**: Pure Go framework with Agent Development Kit (ADK), graph-based orchestration, multi-agent support. ~1.5k stars.

**PoC**: `kubernaut-poc-eino/` — full investigation flow validated against mock-llm.

**Deferral reasons**: Larger dependency tree (security surface), Chinese-primary documentation, advanced features (multi-agent, graph ADK) not needed for v1.3.

### Raw openai-go — Considered, not selected

**What it is**: Official OpenAI Go SDK. Tool calling and structured output only for OpenAI-compatible endpoints.

**Not selected because**: No multi-provider abstraction. Would require Kubernaut to build provider adapters for Azure, Vertex AI, Ollama. LangChainGo provides this out of the box.

---

## 📊 **Comprehensive Requirements Comparison**

### Core Engine Capabilities

| Requirement | Current Python HAPI | LangChainGo | Eino | kagent |
|---|---|---|---|---|
| Agentic loop (multi-turn) | HolmesGPT SDK (Python) | Kubernaut-owned | Kubernaut-owned | Python (AutoGen) ❌ |
| Tool calling | HolmesGPT SDK | Built-in (`llms.Tool`) | Built-in (`schema.ToolInfo`) | Python ❌ |
| Structured output (JSON) | HolmesGPT SDK | Built-in (`WithJSONMode`) | Built-in (`WithJSONMode`) | Python ❌ |
| Streaming | HolmesGPT SDK | Built-in (`WithStreamingFunc`) | Built-in (callback-based) | Python ❌ |
| Context window management | HolmesGPT SDK | Kubernaut-owned | Kubernaut-owned | Python ❌ |

### LLM Provider Support

| Provider | Current Python HAPI | LangChainGo | Eino | kagent |
|---|---|---|---|---|
| OpenAI | ✅ via HolmesGPT | ✅ Built-in | ✅ Built-in | ✅ (Python) |
| Azure OpenAI | ✅ via HolmesGPT | ✅ Built-in | ✅ Built-in | ✅ (Python) |
| Google Vertex AI | ❌ | ✅ Built-in | ❌ (needs proxy) | ❌ |
| Anthropic Claude | ❌ | ✅ Built-in | ✅ Built-in | ✅ (Python) |
| Ollama (local) | ✅ via HolmesGPT | ✅ Built-in | ✅ Built-in | ❌ |
| AWS Bedrock | ❌ | ✅ Built-in | ❌ | ❌ |

### New Capabilities (v1.3 desired)

| Capability | Current Python HAPI | LangChainGo | Eino | Notes |
|---|---|---|---|---|
| Per-turn tool scoping | ❌ (all tools every turn) | Kubernaut-owned | Kubernaut-owned | Pass different `Tools` per `ChatRequest` |
| Per-phase tool restriction | ❌ | Kubernaut-owned | Kubernaut-owned | Investigation phase determines tool subset |
| Tool-output sanitization | ❌ (BR-HAPI-211 planned) | Kubernaut-owned | Kubernaut-owned | Strip injection-like content from tool results |
| API role separation | ❌ (content-level delimiters) | ✅ (native `role: "tool"`) | ✅ (native roles) | Structural boundary, not content-level |

### New Capabilities (v1.4 desired)

| Capability | Current Python HAPI | LangChainGo | Eino | Notes |
|---|---|---|---|---|
| CaMeL defense | ❌ | Kubernaut-owned | Kubernaut-owned | Dual-LLM architecture (privileged + quarantined) |
| Multi-LLM support | ❌ | Kubernaut-owned | Kubernaut-owned | Audit/guardrail LLM alongside investigation LLM |
| Multi-agent orchestration | ❌ | ❌ (needs LangGraphGo) | ✅ Built-in (ADK) | Eino advantage if needed in future |
| MCP tool extensibility | ❌ | Kubernaut-owned | Kubernaut-owned | Custom operator tools via MCP protocol |

### Elimination Targets (removed in Go rewrite)

| What's Eliminated | Current Python HAPI | Go Rewrite |
|---|---|---|
| Python runtime | ✅ Required | ❌ Eliminated |
| HolmesGPT SDK dependency | ✅ Required | ❌ Eliminated |
| Shell execution (`subprocess.run`) | ✅ All toolsets | ❌ Eliminated — Go bindings only |
| kubectl binary | ✅ ~50MB in image | ❌ Eliminated — client-go |
| prometrix Python library | ✅ Required | ❌ Eliminated — Go net/http |
| Jinja2 templates | ✅ Required | ❌ Eliminated — Go text/template |
| QEMU arm64 pip builds | ✅ 40+ min CI | ❌ Eliminated — Go cross-compile |

---

## 🔧 **Kubernaut-Owned Interface Architecture**

The selected approach isolates framework-specific code behind Kubernaut-owned interfaces. This means we can swap LangChainGo for Eino (or raw openai-go) without touching business logic.

```
pkg/llm/client.go              → Generic LLM interface (Kubernaut-owned)
pkg/llm/langchaingo_client.go  → LangChainGo adapter (~60 LOC)
pkg/agent/investigator.go      → Multi-turn investigation loop (framework-agnostic)
pkg/tools/registry.go          → Tool registry with LLM-compatible definitions
pkg/tools/{k8s,prometheus,workflow_discovery,resource_context}.go → Tool implementations
```

The `llm.Client` interface:

```go
type Client interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type ChatRequest struct {
    Messages []Message
    Tools    []ToolDefinition  // per-turn tool scoping happens here
}
```

---

## ✅ **Decision**

**LangChainGo is SELECTED as the LLM framework for v1.3.**

| Decision Factor | LangChainGo | Eino |
|---|---|---|
| Dependency footprint | Leaner (~15-20MB binary impact) | Larger (ByteDance ecosystem) |
| Documentation | English-first | Chinese-primary |
| Community | ~5.5k stars, LangChain org | ~1.5k stars, ByteDance |
| v1.3 feature coverage | Complete | Complete |
| v1.4+ multi-agent | Needs LangGraphGo | Built-in ADK |
| Native Vertex AI | ✅ | ❌ |

**Re-evaluate Eino for v1.4+** if multi-agent capabilities, graph-based orchestration, or KB-agent interaction becomes a requirement.

---

## 📚 **References**

- [#505](https://github.com/jordigilh/kubernaut/issues/505): kagent evaluation (NO-GO)
- [#506](https://github.com/jordigilh/kubernaut/issues/506): LangChainGo evaluation (SELECTED)
- [#507](https://github.com/jordigilh/kubernaut/issues/507): Eino evaluation (VIABLE, deferred)
- PoC repositories: `kubernaut-poc-kagent/`, `kubernaut-poc-langchaingo/`, `kubernaut-poc-eino/` (local)

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
