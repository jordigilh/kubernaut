# PROPOSAL-EXT-007: Pre-Investigation AgenticWorkflow Pipeline in Signal Processing

**Status**: ❌ REJECTED (July 19, 2026) — the classification problem this proposal targets is already solved without a new agentic layer: signal-source routing (`targetSystem`-style domain) is Gateway/AF-owned at ingestion (`RemediationRequestSpec.TargetType`, `SignalSource`, both wired in `pkg/gateway/processing/crd_creator.go`), and cross-domain root-cause reasoning is investigation work that belongs to the downstream investigation agent (KA / opaque OCI agent per [#1536](https://github.com/jordigilh/kubernaut/issues/1536)), not a new SP-owned Triage/Correlator agent tier. SP's role stays deterministic (Rego) enrichment/consolidation, consistent with the precedent set by [#739](https://github.com/jordigilh/kubernaut/issues/739) (non-K8s `targetSystem` routing via DS catalog field, not an LLM classifier). Retained for historical context only; also depends on [PROPOSAL-EXT-004/005/006](.), all ❌ Superseded.  
**Date**: May 23, 2026  
**Author**: Kubernaut Architecture Team  
**Confidence**: 65% (architectural design, no spike validation yet)  
**Related**: [PROPOSAL-EXT-004](PROPOSAL-EXT-004-goose-recipes.md) (superseded), [PROPOSAL-EXT-005](PROPOSAL-EXT-005-oracle-agent-spec.md) (superseded), [PROPOSAL-EXT-006](PROPOSAL-EXT-006-deep-agents.md) (superseded)  
**Inspiration**: [Athena AIOps Deep Agent](https://github.com/rhpds/athena-aiops-deep-agent) (Red Hat Summit 2026 workshop)  
**Tracking**: [#1242](https://github.com/jordigilh/kubernaut/issues/1242)  
**Target**: ~~v1.6 milestone~~ — rejected, no successor planned  

---

## 1. Problem Statement

The v1.6 `AgenticWorkflow` CRD (EXT-004/005/006) introduces pluggable agent runtimes for investigation. A critical open question is **how signals are classified and routed to the correct investigation workflow**.

Analysis of the Athena AIOps workshop revealed that:

- **Regex matching is insufficient**: The same surface-level signal (e.g., an Ansible task failure) can have completely different root causes (package management, networking, linux, openshift). Pattern matching on error strings cannot reliably determine the actual domain.
- **Single-domain classification is brittle**: Real-world signals frequently span multiple domains. A pod CrashLoopBackOff (openshift) may be caused by a failed volume mount (linux/storage) due to an unreachable NFS server (networking). Forcing a single domain risks sending the investigation down the wrong path.
- **LLM-based classification works**: The Athena workshop uses an LLM-driven error-classifier skill that understands semantic context (e.g., "Ansible task failed because package not found" → `package_management`, not `ansible`). This outperforms deterministic approaches for ambiguous signals.

---

## 2. Proposed Architecture

This proposal introduces a **pre-investigation pipeline** in Signal Processing (SP), implemented as a chain of `AgenticWorkflow` instances that enrich and classify signals before KA performs the full investigation.

### 2.1 Architecture Overview

```
Signal arrives
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  Signal Processing (SP)                                 │
│                                                         │
│  1. Normalize signal                                    │
│  2. Run Triage AgenticWorkflow                          │
│     ├─ High confidence, single domain → skip to output  │
│     └─ Low confidence / multi-domain → Phase 3+4        │
│  3. Spawn parallel domain pre-investigation agents      │
│  4. Run Correlator AgenticWorkflow (consumes all above) │
│  5. Emit enriched SignalProcessing output                │
│                                                         │
└──────────────────────────┬──────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│  Kubernaut Agent (KA)                                   │
│                                                         │
│  Reads SP output → spawns investigation AgenticWorkflow │
│  Investigation agent receives pre-investigated context  │
│  No classification responsibility                       │
│                                                         │
└──────────────────────────┬──────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│  Effectiveness Monitor (EM)                             │
│                                                         │
│  Reads SP domain signals + investigation results        │
│  Spawns EA AgenticWorkflows per affected domain         │
│  Each EA workflow assesses its domain independently     │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 2.2 Phase 1: Triage AgenticWorkflow

The triage agent is a lightweight, fast LLM call that receives the normalized signal and produces a structured hypothesis.

**Input**: Normalized signal data (error excerpts, metrics, events, metadata).

**Output**: Structured triage result:
```yaml
hypothesis:
  domains:
    - domain: openshift
      confidence: 85
      evidence: "CrashLoopBackOff on pod payment-service-7f8d9"
    - domain: networking
      confidence: 60
      evidence: "connection refused to nfs-server.internal:2049"
    - domain: linux
      confidence: 40
      evidence: "mount.nfs: access denied by server"
  recommended_approach: parallel  # or "single"
  primary_domain: openshift
```

**Adaptive decision**: If the highest-confidence domain exceeds a configurable threshold **and** no secondary domain exceeds a minimum relevance threshold, the triage agent emits a single-domain result and the pipeline skips directly to SP output (fast path). Otherwise, it triggers parallel domain agents.

### 2.3 Phase 2: Parallel Domain Pre-Investigation Agents (conditional)

SP spawns one `AgenticWorkflow` per identified domain signal, running in parallel. Each agent performs **lightweight, domain-scoped signal analysis** -- not a full investigation. The goal is to validate or refute the triage hypothesis with domain-specific reasoning.

Each domain agent:
- Receives only the signal data relevant to its domain (context isolation)
- Uses domain-specific skills (e.g., Kubernetes pod lifecycle knowledge, Linux filesystem diagnostics)
- Returns a domain-specific pre-investigation result with evidence, confidence, and potential causal links to other domains

### 2.4 Phase 3: Correlator AgenticWorkflow (conditional)

The correlator runs after all Phase 2 agents complete. It consumes all domain agent outputs and builds a correlated picture.

**Input**: All Phase 2 domain agent outputs.

**Output**: Correlated pre-investigation result:
```yaml
correlation:
  root_cause_chain:
    - domain: networking
      finding: "NFS server nfs-server.internal unreachable on port 2049"
      role: root_cause
    - domain: linux
      finding: "Volume mount failed due to NFS timeout"
      role: consequence
    - domain: openshift
      finding: "Pod CrashLoopBackOff because required volume not available"
      role: symptom
  primary_investigation_domain: networking
  secondary_domains: [linux, openshift]
  confidence: 78
  recommended_investigation: "Investigate networking connectivity to NFS server first"
```

### 2.5 SP Output

The enriched `SignalProcessing` output includes:
- Original normalized signal data
- Pre-investigation hypothesis (single or multi-domain)
- Per-domain evidence and confidence scores
- Root cause chain (if multi-domain)
- Recommended investigation approach and primary domain

### 2.6 KA Role (simplified)

KA reads SP's enriched output and spawns the appropriate investigation `AgenticWorkflow` based on the pre-investigation hypothesis. The investigation agent receives pre-investigated context and performs deep analysis with full tooling (MCP tools, live infrastructure access, etc.).

KA has **no classification responsibility** -- it trusts SP's pre-investigation output. If SP marks the signal as "unclassified" (triage failure), KA can fall back to a general-purpose investigation workflow.

### 2.7 EM Role (enhanced)

EM reads SP's domain signals alongside investigation results. For non-K8s remediations, EM spawns EA `AgenticWorkflows` per affected domain. Each EA workflow knows which domain to assess without re-classifying, because SP already identified the domains.

---

## 3. Why Pre-Investigation Belongs in SP

| Concern | SP ownership | KA ownership |
|---|---|---|
| Separation of concerns | Clean -- SP owns signal enrichment, KA owns investigation | Blurred -- KA does classification + investigation |
| Reusability | EM and Remediation Orchestrator reuse SP's domain classification | Each consumer needs its own classification |
| Non-K8s alignment | SP routes to different AgenticWorkflow types (investigation, remediation, EA) | KA needs to handle all routing |
| Cost control | Triage uses a cheap model; investigation uses a capable model | Investigation model handles both |
| Consistency | Single classification result consumed by all downstream components | Risk of inconsistent classification across consumers |

Additionally, EM already uses LLM for effectiveness assessment validation, establishing the precedent that non-KA components can leverage LLM-powered workflows.

---

## 4. Relationship to EXT-004/005/006

The pre-investigation agents use the same `AgenticWorkflow` CRD and the same pluggable runtimes (Goose, OAS/LangGraph, Deep Agents). The ACP Server enforces the same policies (audit, budget, tool call limits) regardless of whether the workflow is a pre-investigation triage or a full investigation.

A new field or label on the `AgenticWorkflow` CRD may be needed to distinguish workflow phases:
- `phase: triage` -- lightweight classification agent
- `phase: pre-investigation` -- domain-specific signal analysis agent
- `phase: correlation` -- cross-domain correlation agent
- `phase: investigation` -- full investigation agent (existing EXT-004/005/006 scope)
- `phase: effectiveness-assessment` -- EA validation agent

---

## 5. Parallels with Athena AIOps Workshop

| Athena concept | Kubernaut equivalent | Divergence |
|---|---|---|
| `error-classifier` skill | Triage AgenticWorkflow | Kubernaut produces multi-domain hypotheses, not single-domain |
| `ops_manager` routing | SP pre-investigation pipeline | Kubernaut separates classification (SP) from investigation (KA) |
| `sre_*` subagents | Domain pre-investigation agents | Kubernaut runs domain agents in parallel, not single selection |
| `reviewer` subagent | Correlator AgenticWorkflow | Kubernaut correlator builds causal chains, not just quality review |
| `AGENTS.md` routing rules | AgenticWorkflow CRD + SP configuration | Kubernaut uses CRD-based declarative routing, not markdown prompts |
| Single-domain classification | Multi-domain hypothesis with confidence | Kubernaut supports root cause chains spanning multiple domains |

---

## 6. Open Questions (Requires Further Refinement)

1. **Adaptive thresholds**: What confidence levels trigger single-domain fast path vs. parallel fan-out? Should this be configurable per signal source or signal type?

2. **Domain agent registry**: How are domain-specific pre-investigation agents registered and discovered? Via `AgenticWorkflow` CRD with a `phase: pre-investigation` label? Or a dedicated registry?

3. **Budget and cost control**: Pre-investigation adds LLM cost before investigation even starts. The triage agent's adaptive decision is the primary cost control mechanism, but what are acceptable cost bounds?

4. **Failure handling**: If the triage agent fails, does SP mark the signal as "unclassified" for KA to handle? If a domain agent times out, does the correlator proceed with partial results?

5. **CRD implications**: Does the `AgenticWorkflow` CRD need a `phase` field, or is this metadata better represented as labels/annotations?

6. **Feedback loop**: Should investigation results feed back to improve triage accuracy over time? If so, how is this captured?

7. **Correlation model**: What LLM capabilities are needed for the correlator? Can a smaller model handle causal chain inference, or does this require a more capable model?

8. **Signal enrichment**: Should SP enrich the signal with additional data (metrics, recent changes, topology) before the triage agent runs, or does the triage agent request that data via tools?

---

## 7. Validation Spikes Needed

Before this architecture can be committed to, the following spikes should validate key assumptions:

1. **Adaptive triage accuracy**: Test a lightweight LLM (e.g., 20B parameter) on a corpus of real multi-domain signals to measure classification accuracy and multi-domain detection rate.

2. **Parallel domain agent overhead**: Measure the latency and cost of spawning 2-3 domain agents in parallel vs. sequential investigation with a single agent.

3. **Correlator effectiveness**: Validate that an LLM can reliably build causal chains from independent domain agent outputs, especially for 3+ domain signals.

---

## 8. Next Steps

This is a first-pass design that requires iteration. Immediate next steps:

1. Gather feedback on the architecture from the team
2. Refine the triage output schema and adaptive threshold logic
3. Define the CRD changes needed to support workflow phases
4. Plan and execute the validation spikes listed above
5. Update EXT-004/005/006 to reference this pre-investigation pipeline
