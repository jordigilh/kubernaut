# Architecture Spikes

Technical spikes validating key architectural decisions before committing to implementation. Each spike produces a time-boxed proof-of-concept with documented findings, confidence scores, and architecture implications.

This directory contains both spike summary documents and spike source code.

**Tracking**: [#1240 — v1.6: AgenticWorkflow Multi-Runtime Architecture](https://github.com/jordigilh/kubernaut/issues/1240) (closed) → current direction: [#1536](https://github.com/jordigilh/kubernaut/issues/1536) (CRD spec v2, runtime-agnostic OCI), [#1535](https://github.com/jordigilh/kubernaut/issues/1535) / [#1681](https://github.com/jordigilh/kubernaut/issues/1681) (AuthBridge audit relay + shadow-evaluator tee), milestone v1.7

---

## Spike Index

### v1.6 — AgenticWorkflow Multi-Runtime Architecture

⚠️ **This entire section is superseded** — see [#1536](https://github.com/jordigilh/kubernaut/issues/1536). The spikes below validated real techniques (still useful — see [ADR-KA-002](../decisions/ADR-KA-002-agent-security-defense-in-depth.md) for the security-relevant ones) but the multi-runtime CRD model they fed into is dead.

| Spike | Status | Confidence | Proposal | Code |
|---|---|---|---|---|
| [SPIKE-PYAGENTSPEC-LANGGRAPH](SPIKE-PYAGENTSPEC-LANGGRAPH.md) | COMPLETED | 97% | [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md) | [pyagentspec-langgraph/](./pyagentspec-langgraph/) |
| [SPIKE-ACP-ENFORCEMENT](SPIKE-ACP-ENFORCEMENT.md) | COMPLETED | 95% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md), [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md), [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) | [acp-enforcement/](./acp-enforcement/) |
| [SPIKE-OCI-RUNTIME-CONTRACT](SPIKE-OCI-RUNTIME-CONTRACT.md) | COMPLETED | 92% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md), [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md), [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) | [oci-runtime-contract/](./oci-runtime-contract/) |
| [SPIKE-GOOSE-MCP-ROUNDTRIP](SPIKE-GOOSE-MCP-ROUNDTRIP.md) | COMPLETED | 96% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md) | [goose-mcp-roundtrip/](./goose-mcp-roundtrip/) |
| [SPIKE-DEEP-AGENTS](SPIKE-DEEP-AGENTS.md) | COMPLETED | 96% | [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) | [pyagentspec-langgraph/05_deepagents_validation.py](./pyagentspec-langgraph/05_deepagents_validation.py) |

### v1.6 — Pre-Investigation Pipeline (planned)

| Spike | Status | Confidence | Proposal | Code |
|---|---|---|---|---|
| SPIKE-ADAPTIVE-TRIAGE | PLANNED | — | [EXT-007](../proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md) | — |
| SPIKE-PARALLEL-DOMAIN-AGENTS | PLANNED | — | [EXT-007](../proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md) | — |
| SPIKE-CORRELATOR-CAUSAL-CHAIN | PLANNED | — | [EXT-007](../proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md) | — |

### v1.5 — Agent Runtime Evaluation

| Spike | Status | Confidence | Proposal | Code |
|---|---|---|---|---|
| [SPIKE-OAS-RUNTIME](SPIKE-OAS-RUNTIME.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [oas-runtime/](./oas-runtime/) |
| [SPIKE-OPENSHELL-OAS-RUNTIME](SPIKE-OPENSHELL-OAS-RUNTIME.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [openshell-oas-runtime/](./openshell-oas-runtime/) |
| [SPIKE-SHADOW-ACP](SPIKE-SHADOW-ACP.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [shadow-acp/](./shadow-acp/) |
| [SPIKE-RUN-STATE-PERSISTENCE](SPIKE-RUN-STATE-PERSISTENCE.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [run-state-persistence/](./run-state-persistence/) |
| [SPIKE-HITL-PERMISSION](SPIKE-HITL-PERMISSION.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | [hitl-permission/](./hitl-permission/) |
| [SPIKE-PROVIDER-VALIDATION](SPIKE-PROVIDER-VALIDATION.md) | COMPLETED | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) | — |
