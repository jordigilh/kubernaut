# DD-HAPI-019-001: LLM Framework Selection

**Status**: ✅ Approved
**Decision Date**: 2026-03-04
**Version**: 1.0
**Confidence**: 90%
**Deciders**: Architecture Team, HAPI Team
**Applies To**: HolmesGPT-API (HAPI)

**Related Business Requirements**:
- [BR-HAPI-433-001: Framework Evaluation](../../../requirements/BR-HAPI-433-go-language-migration/BR-HAPI-433-001-framework-evaluation.md)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-03-04 | Architecture Team | Initial decision: LangChainGo selected after evaluating kagent, Eino, openai-go |

---

## Context & Problem

The HAPI Go rewrite (BR-HAPI-433) requires a Go LLM framework to replace HolmesGPT's Python SDK. The framework must provide multi-provider LLM access and tool calling support while remaining lean enough to meet the security motivation (reduced dependency attack surface).

### Constraints

- Must be pure Go (no Python runtime)
- Must support OpenAI, Azure OpenAI, Vertex AI, and Ollama
- Must support tool/function calling
- Must support structured output (JSON mode)
- Dependency footprint matters (security motivation)

---

## Decision Drivers

1. **Lean dependency footprint** — fewer dependencies = smaller attack surface (core motivation of #433)
2. **Multi-provider support** — customers use different LLM providers
3. **Community maturity** — long-term maintenance confidence
4. **English documentation** — team productivity
5. **Framework isolation** — Kubernaut-owned interface absorbs framework changes

---

## Alternatives Considered

### Alternative A: kagent ❌ Rejected

**Approach**: Use kagent's Kubernetes-native CRD-based agent platform.

**Pros**:
- Kubernetes-native (CRD-based agent definitions)
- Multi-agent orchestration via Team CRD

**Cons**:
- Agent runtime is Python-based (AutoGen 0.4) — does not eliminate Python
- No per-turn tool scoping (tools static per Agent CRD)
- No CaMeL support

**Confidence**: 0% (disqualified)

**Evidence**: kagent `CLAUDE.md`: "the Python components handle the actual agent execution and LLM interactions." Issue [kagent-dev/kagent#480](https://github.com/kagent-dev/kagent/issues/480) confirms.

### Alternative B: LangChainGo ✅ CHOSEN

**Approach**: Use LangChainGo as the LLM abstraction layer, isolated behind Kubernaut-owned `llm.Client` interface.

**Pros**:
- Pure Go, no Python
- Multi-provider: OpenAI, Azure OpenAI, Vertex AI, Anthropic, Ollama, Bedrock
- Tool calling via `llms.Tool` + `llms.WithTools()`
- Structured output via `llms.WithJSONMode()`
- Streaming via `llms.WithStreamingFunc()`
- ~5.5k stars, active community, LangChain org backing
- English documentation
- Lean dependency footprint (~15-20MB binary impact)

**Cons**:
- Single primary maintainer (tmc)
- No built-in multi-agent orchestration (needs LangGraphGo)
- Breaking API changes possible

**Confidence**: 90% (chosen)

**PoC validation**: `kubernaut-poc-langchaingo/` — full investigation flow against mock-llm verified.

### Alternative C: Eino (CloudWeGo/ByteDance) ❌ Deferred to v1.4+

**Approach**: Use Eino's Agent Development Kit with graph-based orchestration.

**Pros**:
- Pure Go, backed by ByteDance production usage
- Agent Development Kit (ADK) with graph-based workflows
- Built-in multi-agent support
- Tool calling, streaming, structured output

**Cons**:
- Larger dependency tree (CloudWeGo ecosystem) — increases attack surface
- Chinese-primary documentation
- ~1.5k stars (smaller community)
- No native Vertex AI support
- Graph/multi-agent features not needed for v1.3

**Confidence**: 70% (viable but deferred)

**PoC validation**: `kubernaut-poc-eino/` — full investigation flow against mock-llm verified.

**Re-evaluate when**: Multi-agent capabilities needed, KB-agent interaction scenarios, or English docs mature.

### Alternative D: Raw openai-go ❌ Not selected

**Approach**: Use the official OpenAI Go SDK directly without a framework.

**Pros**:
- Minimal dependencies (official SDK only)
- Maximum control
- No framework abstractions to learn

**Cons**:
- No multi-provider abstraction — Kubernaut must build adapters for Azure, Vertex AI, Ollama
- More boilerplate for tool calling marshaling
- Reinventing what LangChainGo provides

**Confidence**: 50% (not selected)

---

## Decision

### Chosen: Alternative B — LangChainGo

**Rationale**:
1. **Leanest viable option**: LangChainGo provides multi-provider support with a smaller dependency footprint than Eino. Raw openai-go would require us to build provider adapters.
2. **Native Vertex AI**: Important for GCP customers. Eino lacks this.
3. **English-first ecosystem**: Team productivity over Chinese-primary docs.
4. **Kubernaut-owned interface absorbs risk**: The ~60 LOC adapter means LangChainGo breaking changes are trivial to fix. If LangChainGo becomes unmaintained, swapping to Eino or raw openai-go is a small change.

### Framework Isolation Pattern

```go
// pkg/hapi/llm/client.go — Kubernaut-owned interface
type Client interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// pkg/hapi/llm/langchaingo.go — Framework adapter (~60 LOC)
type LangChainGoClient struct {
    model llms.Model
}

func (c *LangChainGoClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
    // Convert Kubernaut types → LangChainGo types
    // Call llms.GenerateContent()
    // Convert LangChainGo response → Kubernaut types
}
```

This pattern means:
- Business logic (`investigator.go`, `tools/`, `result/`) never imports LangChainGo
- Provider switching (OpenAI → Vertex AI) is configuration, not code
- Framework switching (LangChainGo → Eino) changes one file

---

## Consequences

### Positive Consequences

1. Multi-provider LLM support out of the box (OpenAI, Azure, Vertex AI, Ollama, Bedrock, Anthropic)
2. ~60 LOC adapter — minimal coupling to framework
3. Active community with English docs
4. Binary size impact ~15-20MB (lean)

### Negative Consequences

1. Single primary maintainer (tmc) for LangChainGo
   - **Mitigation**: Large community (5.5k stars), LangChain org backing, Kubernaut-owned interface means we can swap if abandoned
2. No built-in multi-agent for v1.4+
   - **Mitigation**: LangGraphGo is emerging. Alternatively, re-evaluate Eino when multi-agent is needed.

---

## Validation Strategy

1. PoC validated against mock-llm (full investigation flow)
2. Provider switching tested (OpenAI-compatible endpoint)
3. Tool calling round-trip verified (LLM → tool → result → LLM)
4. Adapter LOC measured (~60 lines — confirmed lean)

---

## References

- [#505](https://github.com/jordigilh/kubernaut/issues/505): kagent evaluation
- [#506](https://github.com/jordigilh/kubernaut/issues/506): LangChainGo evaluation
- [#507](https://github.com/jordigilh/kubernaut/issues/507): Eino evaluation
- PoC: `kubernaut-poc-langchaingo/`, `kubernaut-poc-eino/`, `kubernaut-poc-kagent/`

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
