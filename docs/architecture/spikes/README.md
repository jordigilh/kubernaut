# Architecture Spikes

Technical spikes validating key architectural decisions before committing to implementation. Each spike produces a time-boxed proof-of-concept with documented findings, confidence scores, and architecture implications.

This directory contains both spike summary documents and spike source code.

**Tracking**: [#1240 — AgenticWorkflow Multi-Runtime Architecture](https://github.com/jordigilh/kubernaut/issues/1240) (closed, was tracked under the v1.6 milestone before custom-agent work moved out) → current direction: [#1536](https://github.com/jordigilh/kubernaut/issues/1536) (CRD spec v2, runtime-agnostic OCI), [#1535](https://github.com/jordigilh/kubernaut/issues/1535) / [#1681](https://github.com/jordigilh/kubernaut/issues/1681) (AuthBridge audit relay + shadow-evaluator tee), all now under **milestone v1.7** — see [ROADMAP.md](../../roadmap/ROADMAP.md). **v1.6 is Fleet Operations**, not custom agents; the section headers below intentionally drop the old "v1.6" labels to avoid that confusion.

---

## Spike Index

### AgenticWorkflow Multi-Runtime Architecture (originally v1.6 scope, superseded)

⚠️ **This entire section is superseded** — see [#1536](https://github.com/jordigilh/kubernaut/issues/1536). These spikes were tracked under the v1.6 milestone (#1240) back when custom-agent runtime selection was v1.6 scope; that work is now milestone **v1.7** (per the current [ROADMAP.md](../../roadmap/ROADMAP.md) — v1.6 shipped as Fleet Operations instead). The spikes below validated real techniques (still useful — see [ADR-KA-002](../decisions/ADR-KA-002-agent-security-defense-in-depth.md) for the security-relevant ones) but the multi-runtime CRD model they fed into is dead.

| Spike | Status | Confidence | Proposal | Code |
|---|---|---|---|---|
| [SPIKE-PYAGENTSPEC-LANGGRAPH](SPIKE-PYAGENTSPEC-LANGGRAPH.md) | COMPLETED — superseded | 97% | [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md) (superseded) | [pyagentspec-langgraph/](./pyagentspec-langgraph/) |
| [SPIKE-ACP-ENFORCEMENT](SPIKE-ACP-ENFORCEMENT.md) | COMPLETED — relocated to AuthBridge | 95% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md) (superseded), [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md) (superseded), [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) (superseded) | [acp-enforcement/](./acp-enforcement/) |
| [SPIKE-OCI-RUNTIME-CONTRACT](SPIKE-OCI-RUNTIME-CONTRACT.md) | COMPLETED — superseded | 92% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md) (superseded), [EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md) (superseded), [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) (superseded) | [oci-runtime-contract/](./oci-runtime-contract/) |
| [SPIKE-GOOSE-MCP-ROUNDTRIP](SPIKE-GOOSE-MCP-ROUNDTRIP.md) | COMPLETED — mechanics validated, model superseded | 96% | [EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md) (superseded) | [goose-mcp-roundtrip/](./goose-mcp-roundtrip/) |
| [SPIKE-DEEP-AGENTS](SPIKE-DEEP-AGENTS.md) | COMPLETED — technique validated, model superseded | 96% | [EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md) (superseded) | [pyagentspec-langgraph/05_deepagents_validation.py](./pyagentspec-langgraph/05_deepagents_validation.py) |

### Pre-Investigation Pipeline (was planned for v1.6/v1.7, rejected)

❌ **Rejected** — see [PROPOSAL-EXT-007](../proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md), rejected 2026-07-19. None of the spikes below were started; routing is Gateway/AF-owned (`TargetType`/`SignalSource`) + Rego, not a new SP-owned agent tier.

| Spike | Status | Confidence | Proposal | Code |
|---|---|---|---|---|
| SPIKE-ADAPTIVE-TRIAGE | ❌ Not started, rejected | — | [EXT-007](../proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md) | — |
| SPIKE-PARALLEL-DOMAIN-AGENTS | ❌ Not started, rejected | — | [EXT-007](../proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md) | — |
| SPIKE-CORRELATOR-CAUSAL-CHAIN | ❌ Not started, rejected | — | [EXT-007](../proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md) | — |

### Agent Runtime Evaluation (originally v1.6 scope, superseded)

⚠️ **This entire section is superseded** — see [#1536](https://github.com/jordigilh/kubernaut/issues/1536). This section was previously mislabeled "v1.5" — all six spikes' tracking issues (#1203, #1206, #1207, #1208, and siblings) are actually milestone **v1.6** (closed) in GitHub, from the same now-superseded custom-agent-runtime effort as the section above. All six spikes evaluated the OAS Runtime / ACP-session architecture from [PROPOSAL-EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md), now dead. Security-relevant findings (sandbox isolation, shadow evaluation) are carried forward in [ADR-KA-002](../decisions/ADR-KA-002-agent-security-defense-in-depth.md); see each spike's own status line for what specifically survives vs. is open.

| Spike | Status | Confidence | Proposal | Code |
|---|---|---|---|---|
| [SPIKE-OAS-RUNTIME](SPIKE-OAS-RUNTIME.md) | COMPLETED — superseded | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) (superseded) | [oas-runtime/](./oas-runtime/) |
| [SPIKE-OPENSHELL-OAS-RUNTIME](SPIKE-OPENSHELL-OAS-RUNTIME.md) | COMPLETED — sandbox mechanics survive (ADR-KA-002 L2) | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) (superseded) | [openshell-oas-runtime/](./openshell-oas-runtime/) |
| [SPIKE-SHADOW-ACP](SPIKE-SHADOW-ACP.md) | COMPLETED — superseded, see #1681 | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) (superseded) | [shadow-acp/](./shadow-acp/) |
| [SPIKE-RUN-STATE-PERSISTENCE](SPIKE-RUN-STATE-PERSISTENCE.md) | COMPLETED — superseded | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) (superseded) | [run-state-persistence/](./run-state-persistence/) |
| [SPIKE-HITL-PERMISSION](SPIKE-HITL-PERMISSION.md) | COMPLETED — superseded, no successor yet | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) (superseded) | [hitl-permission/](./hitl-permission/) |
| [SPIKE-PROVIDER-VALIDATION](SPIKE-PROVIDER-VALIDATION.md) | COMPLETED — superseded | — | [EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) (superseded) | — |
