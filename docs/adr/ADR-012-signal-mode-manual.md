# ADR-012: Signal Mode `manual` for User-Initiated Investigations

**Status:** Accepted
**Date:** 2026-04-30
**Deciders:** AF team, kubernaut team
**Source:** DEV-2 (#58), kubernaut#1014

## Context

kubernaut's SignalProcessing pipeline classifies signals by mode: `reactive` (from AlertManager) and `proactive` (from predict_linear queries). AF introduces a third origin: user-initiated NL queries that create RemediationRequests without any preceding alert.

## Decision

Add `signal_mode=manual` as a new classification value for AF-originated signals.

- AF sets `SignalType: "manual"` on RRs it creates
- SP classifies `signal.Type == "manual"` → `signal_mode=manual`
- Severity defaults to `unknown` (no alert-derived severity exists) per kubernaut#1015
- Workflow catalog treats `severity=unknown` as wildcard or P3 priority

## Consequences

- Clear provenance: operators and Rego policies can distinguish human-initiated investigations from automated ones
- `unknown` severity avoids misrepresenting confidence (not "low", not "high" — genuinely unknown)
- LLM-derived severity (kubernaut#1017) can supplement `unknown` in future for policy decisions
- Requires kubernaut#1014 (enum addition) and #1015 (severity addition) — hard blockers for E2E
- AF can develop against mocks until these land in kubernaut `development/v1.5`

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| Reuse `reactive` mode | Conflates human intent with automated alerting; Rego policies can't distinguish |
| Reuse `proactive` mode | Semantically wrong (proactive = predictive, not user-driven) |
| No signal mode (omit field) | SP classifier rejects signals without mode; breaks pipeline |
| `interactive` mode | Too specific to MCP/A2A mechanism; `manual` is intent-based regardless of protocol |
