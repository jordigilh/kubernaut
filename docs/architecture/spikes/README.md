# Architecture Spikes

Technical spikes validating key architectural decisions before committing to implementation. Each spike produces a time-boxed proof-of-concept with documented findings, confidence scores, and architecture implications.

Spike code lives in [`spikes/`](../../../spikes/) at the repo root. This directory contains the summary documents.

**Tracking**: [#1240 — v1.6: AgenticWorkflow Multi-Runtime Architecture](https://github.com/jordigilh/kubernaut/issues/1240)

---

## Spike Index

### v1.6 — AgenticWorkflow Multi-Runtime Architecture

| Spike | Status | Confidence | Proposal | Code |
|---|---|---|---|---|
| [SPIKE-PYAGENTSPEC-LANGGRAPH](SPIKE-PYAGENTSPEC-LANGGRAPH.md) | COMPLETED | 97% | [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md) | [spikes/pyagentspec-langgraph/](../../../spikes/pyagentspec-langgraph/) |
| [SPIKE-ACP-ENFORCEMENT](SPIKE-ACP-ENFORCEMENT.md) | COMPLETED | 95% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md), [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md), [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) | [spikes/acp-enforcement/](../../../spikes/acp-enforcement/) |
| [SPIKE-OCI-RUNTIME-CONTRACT](SPIKE-OCI-RUNTIME-CONTRACT.md) | COMPLETED | 92% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md), [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md), [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) | [spikes/oci-runtime-contract/](../../../spikes/oci-runtime-contract/) |
| [SPIKE-GOOSE-MCP-ROUNDTRIP](SPIKE-GOOSE-MCP-ROUNDTRIP.md) | COMPLETED | 96% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md) | [spikes/goose-mcp-roundtrip/](../../../spikes/goose-mcp-roundtrip/) |
| [SPIKE-DEEP-AGENTS](SPIKE-DEEP-AGENTS.md) | COMPLETED | 96% | [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) | [spikes/pyagentspec-langgraph/05_deepagents_validation.py](../../../spikes/pyagentspec-langgraph/05_deepagents_validation.py) |

### v1.5 — Agent Runtime Evaluation

| Spike | Status | Confidence | Proposal | Code |
|---|---|---|---|---|
| [SPIKE-OAS-RUNTIME](SPIKE-OAS-RUNTIME.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [spikes/oas-runtime/](../../../spikes/oas-runtime/) |
| [SPIKE-OPENSHELL-OAS-RUNTIME](SPIKE-OPENSHELL-OAS-RUNTIME.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [spikes/openshell-oas-runtime/](../../../spikes/openshell-oas-runtime/) |
| [SPIKE-SHADOW-ACP](SPIKE-SHADOW-ACP.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [spikes/shadow-acp/](../../../spikes/shadow-acp/) |
| [SPIKE-RUN-STATE-PERSISTENCE](SPIKE-RUN-STATE-PERSISTENCE.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [spikes/run-state-persistence/](../../../spikes/run-state-persistence/) |
| [SPIKE-HITL-PERMISSION](SPIKE-HITL-PERMISSION.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [spikes/hitl-permission/](../../../spikes/hitl-permission/) |
| [SPIKE-PROVIDER-VALIDATION](SPIKE-PROVIDER-VALIDATION.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | — |
