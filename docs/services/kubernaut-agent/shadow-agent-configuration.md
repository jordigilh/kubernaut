# Shadow Agent (Alignment Check) — Configuration Guide

**Version**: 1.0
**Feature**: Prompt Injection Guardrails ([#601](https://github.com/jordigilh/kubernaut/issues/601))
**Since**: v1.4
**ADR**: [ADR-KA-001](../../architecture/decisions/ADR-KA-001-shadow-agent-alignment-check.md)

---

## Overview

The Shadow Agent is a parallel security auditor that runs alongside the Kubernaut Agent's primary investigation loop. It monitors every piece of content entering the agentic pipeline — LLM responses, tool outputs, and signal context — and flags prompt injection attempts before they can influence remediation decisions.

When the shadow agent detects suspicious content, the investigation is automatically escalated to human review rather than proceeding to automated remediation.

### Key Properties

- **Transparent**: No modification to the investigation logic — operates as a proxy layer
- **Fail-closed**: Any error (timeout, LLM failure, malformed response) results in human review escalation
- **Per-investigation isolation**: Each investigation gets a fresh observer; no state leaks across requests
- **Concurrent**: Shadow evaluations run in parallel with the investigation, adding minimal latency

---

## Enabling the Shadow Agent

### Via Helm Values

```yaml
kubernautAgent:
  alignmentCheck:
    enabled: true
    timeout: "10s"
    maxStepTokens: 500
```

### Via Service Configuration (ConfigMap)

```yaml
alignmentCheck:
  enabled: true
  timeout: 10s
  maxStepTokens: 500
```

The shadow agent is **disabled by default** (`enabled: false`). When disabled, the investigation pipeline operates exactly as in v1.3 with zero overhead.

---

## Configuration Reference

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the shadow agent alignment check |
| `timeout` | duration | `10s` | Per-step evaluation timeout. Each tool output or LLM response is evaluated independently with this timeout. |
| `maxStepTokens` | int | `500` | Maximum runes per evaluated step. Content exceeding this limit is truncated using a head+tail strategy that preserves the beginning and end of the content. |
| `llm.provider` | string | *(inherits)* | LLM provider for shadow evaluation. If omitted, reuses the investigation LLM. |
| `llm.model` | string | *(inherits)* | Model name. Required when `llm` block is specified. |
| `llm.endpoint` | string | *(inherits)* | Provider endpoint URL. Required for non-managed providers. |
| `llm.apiKey` | string | *(inherits)* | API key for the shadow LLM. |

### Validation Rules

When `enabled: true`:
- `timeout` must be a positive duration
- `maxStepTokens` must be a positive integer
- If `llm` is specified, `model` must be non-empty
- If `llm.provider` is not a managed provider (openai, anthropic, bedrock, huggingface), `endpoint` is required

---

## LLM Configuration Strategies

### Strategy 1: Shared LLM (Development / Testing)

The shadow agent reuses the investigation LLM client. Simplest configuration — no extra API keys or billing.

```yaml
kubernautAgent:
  llm:
    provider: "openai"
    model: "gpt-4o"
    apiKey: "sk-..."
  alignmentCheck:
    enabled: true
    # No llm block — reuses investigation LLM
```

**Trade-offs**: Increases request volume on the shared client. Acceptable for development or low-volume environments.

### Strategy 2: Dedicated LLM (Production)

A separate, fast, cost-effective model handles shadow evaluations while the investigation uses a more capable model.

```yaml
kubernautAgent:
  llm:
    provider: "openai"
    model: "gpt-4o"
    apiKey: "sk-..."
  alignmentCheck:
    enabled: true
    llm:
      provider: "openai"
      model: "gpt-4o-mini"
      apiKey: "sk-..."
```

**Trade-offs**: Requires a second API key and model access. Recommended for production: `gpt-4o-mini` is ~10x cheaper than `gpt-4o` and sufficient for binary suspicious/clean classification.

### Strategy 3: Cross-Provider Shadow

Use a different provider for the shadow to avoid single-provider failure modes.

```yaml
kubernautAgent:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    apiKey: "sk-ant-..."
  alignmentCheck:
    enabled: true
    llm:
      provider: "openai"
      model: "gpt-4o-mini"
      apiKey: "sk-..."
```

**Trade-offs**: Maximum resilience — if the primary provider is compromised or down, the shadow still operates independently.

---

## How It Works

### Investigation Flow (Shadow Enabled)

```
1. Signal arrives → InvestigatorWrapper creates Observer
2. Signal context submitted as Step 0 (StepKindSignalInput)
3. Primary investigation begins:
   a. LLMProxy intercepts each Chat() → submits LLM response as step
   b. ToolProxy intercepts each Execute() → submits tool result as step
   c. Observer dispatches each step to Evaluator asynchronously
   d. Evaluator sends step to shadow LLM with security prompt
   e. Shadow LLM returns {suspicious: bool, explanation: string}
4. Primary investigation completes
5. Observer.WaitForCompletion(30s) — collects all evaluations
6. Verdict rendered:
   - Clean → result returned unchanged
   - Suspicious → HumanReviewNeeded=true, warnings appended
   - Timeout → fail-closed, treated as suspicious
7. Audit events emitted for every flagged step + final verdict
```

### Content Evaluation

Each step is wrapped in random boundary markers:

```
<<<EVAL_a7f3b2c1>>>
{untrusted content from tool output or LLM response}
<<<END_EVAL_a7f3b2c1>>>
```

The boundary token is cryptographically random per evaluation. Content containing the closing marker is immediately flagged as suspicious (boundary escape attempt).

### What Gets Evaluated

| Step Kind | Source | Example |
|-----------|--------|---------|
| `signal_input` | Alert context | `"Investigate: critical OOMKilled in namespace production — container killed"` |
| `llm_reasoning` | Primary LLM response | Model's analysis, RCA, workflow recommendation |
| `tool_result` | Tool execution output | Pod logs, kubectl output, Prometheus metrics, event descriptions |

---

## Observability

### Logs

When the shadow agent is enabled, KA logs include:

```
# Clean investigation
INFO  shadow agent alignment check passed  signal=my-alert namespace=production total=8

# Suspicious investigation
INFO  shadow agent flagged suspicious content  signal=my-alert namespace=production
      flagged=2 total=8 pending=0 timed_out=false
      summary="step 3 (get_pod_logs): Role impersonation via SYSTEM: header in log output"
```

### Audit Events

| Event Type | Trigger | Key Fields |
|------------|---------|------------|
| `alignment.step` | Each flagged step | `step_index`, `step_kind`, `tool`, `explanation` |
| `alignment.verdict` | Investigation complete | `result` (clean/suspicious), `summary`, `flagged`, `total` |

### Metrics

Shadow agent activity is observable through the existing KA investigation metrics:
- Investigations with `HumanReviewNeeded=true` and `HumanReviewReason="alignment_check_failed"` indicate shadow agent escalations
- Shadow LLM calls appear in the instrumented client's request/latency metrics

---

## Tuning Guide

### High False Positive Rate

If the shadow agent flags too many legitimate investigations:

1. **Check `maxStepTokens`** — If too low, truncated content may lose context and appear suspicious. Increase to 750-1000.
2. **Review flagged steps** — Use audit events to identify patterns. Common false positives:
   - Application logs containing "ERROR:" or "WARNING:" prefixes
   - Kubernetes events with descriptive messages that resemble instructions
3. **Consider a more capable shadow model** — `gpt-4o-mini` may flag ambiguous content that `gpt-4o` would correctly classify as clean.

### High Latency

If investigations take too long with shadow enabled:

1. **Use a dedicated shadow LLM** — Avoids contention on the primary LLM client.
2. **Reduce `timeout`** — Default 10s per step is conservative. For fast models, 5s is often sufficient.
3. **Monitor `timed_out` verdicts** — If verdicts frequently time out, the shadow LLM is too slow or overloaded.

### Cost Optimization

Shadow evaluations generate N+1 LLM calls per investigation (N tool/LLM steps + signal context):
- Average investigation: 8-12 tool calls + ~4 LLM responses = 13-17 shadow evaluations
- With `gpt-4o-mini` at ~$0.15/1M input tokens: approximately $0.002-0.005 per investigation

---

## Limitations

- **v1.4 scope**: Global enable/disable only — no per-namespace or per-signal granularity
- **No streaming support**: Shadow evaluation is batch-per-step, not streaming token-by-token
- **Restart required**: Toggling `enabled` requires a pod restart (no hot-reload for this config in v1.4)
- **Single shadow model**: One evaluator per KA instance (multi-model consensus deferred to #648)

---

## Migration from v1.3

No migration required. The shadow agent is a new additive feature:

1. **Disabled by default** — v1.3 behavior preserved when `alignmentCheck.enabled: false`
2. **No CRD changes** — Shadow agent uses existing `InvestigationResult` fields (`HumanReviewNeeded`, `HumanReviewReason`, `Warnings`)
3. **No API changes** — Transparent to consumers of the investigation API
4. **Rollback** — Set `alignmentCheck.enabled: false` and restart KA to disable
