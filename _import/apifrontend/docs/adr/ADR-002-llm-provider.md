# ADR-002: LLM Provider and Model Selection

**Status:** Accepted
**Date:** 2026-04-30
**Deciders:** AF team
**Source:** IMP-1 (Tier 0 resolution)

## Context

AF uses its own LLM for NL triage and orchestration (interpreting user queries, selecting tools, synthesizing responses). The LLM must support tool-use (function calling) reliably, have predictable latency, and be deployable in air-gapped environments for some customers.

## Decision

- **Primary model:** Claude Sonnet 4.6 via Vertex AI (Anthropic)
- **Shadow/evaluation model:** Claude Haiku 4.6 (for cost analysis and degradation testing)
- **Provider abstraction:** LLM Client interface enabling provider swap without code changes
- **Configuration:** Provider and model specified in operator ConfigMap / Helm values

## Consequences

- Claude Sonnet 4.6 provides reliable tool-use (validated: 246 tool calls across 21 golden transcripts, 0 hallucinated tool names)
- Vertex AI provides enterprise support, data residency controls, and GCP integration
- Provider abstraction enables future switch to local models (Ollama/vLLM) for air-gapped deployments
- SLO targets (15s P95 triage) calibrated for Claude Sonnet 4.6 latency profile (2-5s per call)
- Local model deployments need adjusted SLO targets (30-45s) via `performance.triageTargetSeconds` config

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| OpenAI GPT-4o | Less predictable tool-use adherence in kubernaut-style workflows |
| Local models only (Ollama) | Latency too high (5-15s per call) for interactive UX; insufficient tool-use reliability |
| KA's LLM (shared) | Tight coupling; AF needs independent triage before KA involvement |
| No LLM (rule-based triage) | Cannot handle NL ambiguity ("something is wrong in prod") |
