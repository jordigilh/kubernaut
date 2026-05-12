# Shadow Agent (Alignment Check) — Configuration Guide

**Version**: 2.0
**Feature**: Prompt Injection Guardrails ([#601](https://github.com/jordigilh/kubernaut/issues/601))
**Since**: v1.4
**ADR**: [ADR-KA-001](../../architecture/decisions/ADR-KA-001-shadow-agent-alignment-check.md)
**Issues**: [#1076](https://github.com/jordigilh/kubernaut/issues/1076), [#1077](https://github.com/jordigilh/kubernaut/issues/1077), [#1078](https://github.com/jordigilh/kubernaut/issues/1078)

---

## Overview

The Shadow Agent is a parallel security auditor that runs alongside the Kubernaut Agent's primary investigation loop. It monitors every piece of content entering the agentic pipeline — LLM responses, tool outputs, and signal context — and flags prompt injection attempts before they can influence remediation decisions.

The shadow agent operates in one of two modes:

- **Enforce** (default): Suspicious verdicts escalate the investigation to human review. If a circuit breaker fires mid-investigation, the primary LLM is cancelled and the shadow agent's findings become the primary content presented to the operator.
- **Monitor**: Suspicious verdicts are logged and audited but do not block the investigation or trigger the circuit breaker. Use this mode during initial rollout to observe false-positive rates before enabling enforcement.

### Key Properties

- **Transparent**: No modification to the investigation logic — operates as a proxy layer
- **Fail-closed**: Any error (timeout, LLM failure, malformed response, evaluator panic) results in a suspicious verdict
- **Per-investigation isolation**: Each investigation gets a fresh Observer; no state leaks across requests
- **Concurrent**: Shadow evaluations run in parallel with the investigation, adding minimal latency
- **Canary integrity check**: Before each investigation, a known-malicious payload is sent to the shadow evaluator to detect compromised or misconfigured shadow models

---

## Enabling the Shadow Agent

### Via Helm Values

```yaml
kubernautAgent:
  alignmentCheck:
    enabled: true
    mode: "enforce"
    timeout: "10s"
    verdictTimeout: "30s"
    maxStepTokens: 500
```

### Via Service Configuration (ConfigMap)

```yaml
ai:
  alignmentCheck:
    enabled: true
    mode: enforce
    timeout: 10s
    verdictTimeout: 30s
    maxStepTokens: 500
    maxRetries: 1
    canary:
      forceEscalation: true
```

The shadow agent is **disabled by default** (`enabled: false`). When disabled, the investigation pipeline operates exactly as in v1.3 with zero overhead.

---

## Configuration Reference

YAML path: `ai.alignmentCheck`

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the shadow agent alignment check. |
| `mode` | string | `enforce` | `enforce`: suspicious verdicts escalate to human review and activate the circuit breaker. `monitor`: suspicious verdicts are logged and audited only. |
| `timeout` | duration | `10s` | Per-step evaluation timeout. Each tool output or LLM response is evaluated independently with this timeout. |
| `verdictTimeout` | duration | `30s` | Maximum time to wait for all pending shadow evaluations to complete after the primary investigation finishes. Pending evaluations after this timeout trigger a fail-closed suspicious verdict. |
| `maxStepTokens` | int | `500` | Maximum runes per evaluated step. Content exceeding this limit is truncated using a head+tail strategy that preserves the beginning and end of the content. |
| `maxRetries` | int | `1` | Number of retries for failed evaluator calls (per step). |
| `canary.forceEscalation` | bool | `true` | When true, a canary failure forces human review escalation regardless of the `mode` setting. When false, canary failures only escalate in `enforce` mode. |
| `llm.provider` | string | *(inherits)* | LLM provider for shadow evaluation. If omitted, reuses the investigation LLM. |
| `llm.model` | string | *(inherits)* | Model name. Required when `llm` block is specified. |
| `llm.endpoint` | string | *(inherits)* | Provider endpoint URL. Required for non-managed providers. |
| `llm.apiKey` | string | *(inherits)* | API key for the shadow LLM. |

For the complete LLM override surface (`azureApiVersion`, `vertexProject`, etc.), see [configuration-reference.md §7.1](configuration-reference.md#71-aialignmentcheckllm-overrides-llmoverrideconfig).

### Startup Behavior

- When `enabled=true` and `llm` is **nil**: KA logs an **error-level** warning that shadow traffic shares the primary LLM client (contention risk).
- When `enabled=true` and the dedicated shadow client **fails to build**: the process **exits** (fail-closed — a non-functional shadow agent is worse than no shadow agent).

### Validation Rules

When `enabled: true`:
- `mode` must be `enforce` or `monitor`
- `timeout` must be a positive duration
- `verdictTimeout` must be a positive duration
- `maxStepTokens` must be a positive integer
- `maxRetries` must be >= 0
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

**Trade-offs**: Requires a second API key and model access. Recommended for production: `gpt-4o-mini` is ~10x cheaper than `gpt-4o` and sufficient for binary suspicious/aligned classification.

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
1. Signal arrives → InvestigatorWrapper runs canary integrity check
2. Canary result:
   - PASS → proceed normally
   - FAIL → log warning; escalate to human review if mode=enforce or canary.forceEscalation=true
3. InvestigatorWrapper creates per-investigation Observer
4. Signal context submitted as Step 0 (StepKindSignalInput)
5. Primary investigation begins:
   a. LLMProxy intercepts each Chat() → submits LLM response as step
   b. ToolProxy intercepts each Execute() → submits tool result as step
   c. Observer dispatches each step to Evaluator asynchronously (max 10 concurrent)
   d. Evaluator sends step to shadow LLM with boundary-wrapped security prompt
   e. Shadow LLM returns {suspicious: bool, explanation: string}
6. Primary investigation completes
7. Observer.WaitForCompletion(verdictTimeout) — collects all evaluations
8. Verdict rendered:
   - Aligned → alignment_verdict populated with result="aligned"
   - Suspicious + enforce → HumanReviewNeeded=true, reason="alignment_check_failed"
   - Suspicious + monitor → logged and audited only, investigation proceeds
   - Timeout / pending evals → fail-closed, treated as suspicious
9. alignment_verdict populated on InvestigationResult for ALL investigations
10. Audit events emitted for every flagged step + final verdict
```

### Circuit Breaker (Enforce Mode)

In `enforce` mode, when the shadow agent detects suspicious content during an active investigation, the circuit breaker activates:

1. The Observer's `onSuspicious` callback fires (via `sync.Once` — exactly once per investigation)
2. The primary LLM investigation context is cancelled via `context.WithCancelCause(ErrCircuitBreaker)`
3. Shadow evaluations continue on the parent context (they are not cancelled)
4. The partial primary LLM results are preserved but relegated in the notification
5. The alignment verdict's `circuit_breaker_activated` flag is set to `true`
6. The operator notification prominently displays the shadow agent's findings

In `monitor` mode, the circuit breaker does not fire. Suspicious verdicts are logged and audited but the primary investigation runs to completion.

### LLMProxy and Client Pinning

When the investigation LLM uses hot-reloadable configuration (`SwappableClient`), the investigator pins a client snapshot at the start of each investigation to prevent mid-flight configuration changes. The `PinDecorator` ensures that the `LLMProxy` wrapper is re-applied around the pinned snapshot, so all LLM traffic remains observable by the shadow agent even after pinning. Without this, pinned clients would bypass the shadow proxy.

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

## API Surface

### KA OpenAPI — `IncidentResponse.alignment_verdict`

Every investigation response includes an `alignment_verdict` object:

```json
{
  "alignment_verdict": {
    "result": "aligned",
    "circuit_breaker_activated": false,
    "summary": "all steps passed alignment check",
    "flagged": 0,
    "total": 12,
    "findings": []
  }
}
```

When suspicious content is detected:

```json
{
  "alignment_verdict": {
    "result": "suspicious",
    "circuit_breaker_activated": true,
    "summary": "step 3 (get_pod_logs): Role impersonation via SYSTEM: header",
    "flagged": 1,
    "total": 12,
    "findings": [
      {
        "step_index": 3,
        "step_kind": "tool_result",
        "tool": "get_pod_logs",
        "explanation": "Role impersonation via SYSTEM: header in log output"
      }
    ]
  }
}
```

### AIAnalysis CRD — `status.alignmentVerdict`

The AA controller maps the KA response into the `AIAnalysis` status:

| Field | Type | Description |
|-------|------|-------------|
| `status.alignmentVerdict.result` | string | `"aligned"` or `"suspicious"` |
| `status.alignmentVerdict.circuitBreakerActivated` | bool | Whether the circuit breaker cancelled the primary investigation |
| `status.alignmentVerdict.summary` | string | Human-readable verdict summary |
| `status.alignmentVerdict.flagged` | int | Number of flagged steps |
| `status.alignmentVerdict.total` | int | Total evaluated steps |
| `status.alignmentVerdict.findings` | array | Per-step findings (stepIndex, stepKind, tool, explanation) |

### NotificationRequest CRD — `ReviewContext`

When alignment triggers human review, the `NotificationRequest` carries:

| Field | Type | Description |
|-------|------|-------------|
| `context.review.alignmentVerdict` | string | Rendered verdict summary text |
| `context.review.circuitBreakerActivated` | bool | Whether the circuit breaker was active |

The Remediation Orchestrator renders the alignment verdict prominently in the notification body. When the circuit breaker is activated, the shadow agent's findings are displayed first and the primary LLM's RCA is relegated with a warning that it may be incomplete or compromised.

---

## Observability

### Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `kubernaut_alignment_verdict_total` | Counter | `result` (aligned, suspicious), `mode` (enforce, monitor) | Total alignment verdicts emitted |
| `kubernaut_alignment_step_total` | Counter | `outcome` (aligned, suspicious, panic) | Per-step evaluation outcomes |
| `kubernaut_alignment_canary_total` | Counter | `result` (pass, fail) | Canary integrity check results |
| `kubernaut_alignment_verdict_duration_seconds` | Histogram | *(default buckets)* | Time from canary start to verdict completion |
| `kubernaut_alignment_shadow_audit_total` | Counter | `event_type` (request, response) | Shadow LLM audit events emitted to Data Storage |

### Logs

When the shadow agent is enabled, KA logs include:

```
# Aligned investigation
INFO  shadow agent alignment check passed  signal=my-alert namespace=production total=8

# Suspicious investigation (enforce mode)
INFO  shadow agent flagged suspicious content  signal=my-alert namespace=production
      flagged=2 total=8 pending=0 timed_out=false escalated=true mode=enforce
      summary="step 3 (get_pod_logs): Role impersonation via SYSTEM: header in log output"

# Suspicious investigation (monitor mode)
INFO  shadow agent flagged suspicious content  signal=my-alert namespace=production
      flagged=2 total=8 pending=0 timed_out=false escalated=false mode=monitor
      summary="step 3 (get_pod_logs): Role impersonation via SYSTEM: header in log output"

# Canary failure
INFO  shadow agent canary failed: shadow model did not flag known-malicious content
      signal=my-alert namespace=production explanation="..."
```

### Audit Events

| Event Type | Trigger | Key Fields |
|------------|---------|------------|
| `aiagent.alignment.step` | Each flagged step | `step_index`, `step_kind`, `tool`, `explanation` |
| `aiagent.alignment.verdict` | Investigation complete | `result` (aligned/suspicious), `summary`, `flagged`, `total`, `shadow_prompt_tokens`, `shadow_completion_tokens`, `shadow_total_tokens` |
| `aiagent.shadow.llm.request` | Shadow LLM call sent | `correlation_id`, `model`, `provider` |
| `aiagent.shadow.llm.response` | Shadow LLM call returned | `correlation_id`, `model`, `prompt_tokens`, `completion_tokens`, `total_tokens` |

---

## Tuning Guide

### High False Positive Rate

If the shadow agent flags too many legitimate investigations:

1. **Check `maxStepTokens`** — If too low, truncated content may lose context and appear suspicious. Increase to 750-1000.
2. **Review flagged steps** — Use audit events to identify patterns. Common false positives:
   - Application logs containing "ERROR:" or "WARNING:" prefixes
   - Kubernetes events with descriptive messages that resemble instructions
3. **Consider a more capable shadow model** — `gpt-4o-mini` may flag ambiguous content that `gpt-4o` would correctly classify as aligned.
4. **Start in `monitor` mode** — Run with `mode: monitor` to observe verdicts without impacting investigations, then switch to `enforce` once the false-positive rate is acceptable.

### High Latency

If investigations take too long with shadow enabled:

1. **Use a dedicated shadow LLM** — Avoids contention on the primary LLM client.
2. **Reduce `timeout`** — Default 10s per step is conservative. For fast models, 5s is often sufficient.
3. **Monitor `timed_out` verdicts** — If verdicts frequently time out, the shadow LLM is too slow or overloaded. Check `kubernaut_alignment_verdict_total{result="suspicious"}` and correlate with `kubernaut_alignment_step_total{outcome="panic"}`.
4. **Reduce `verdictTimeout`** — Default 30s waits for all evaluations. If most complete within 10-15s, lowering this reduces tail latency at the cost of more fail-closed verdicts.

### Cost Optimization

Shadow evaluations generate N+1 LLM calls per investigation (N tool/LLM steps + signal context):
- Average investigation: 8-12 tool calls + ~4 LLM responses = 13-17 shadow evaluations
- With `gpt-4o-mini` at ~$0.15/1M input tokens: approximately $0.002-0.005 per investigation

---

## Limitations

- **v1.4 scope**: Global enable/disable only — no per-namespace or per-signal granularity
- **No streaming support**: Shadow evaluation is batch-per-step, not streaming token-by-token
- **Restart required**: Toggling `enabled` requires a pod restart (no hot-reload for this config in v1.4)
- **Single shadow model**: One evaluator per KA instance (multi-model consensus deferred to [#648](https://github.com/jordigilh/kubernaut/issues/648))
- **Verdict timeout waiter**: The internal goroutine waiting for evaluations to complete is not cancelled when `verdictTimeout` fires. It self-heals within `timeout` (default 10s) as all pending evaluations carry per-step timeouts. A clean fix (Observer-scoped context cancellation) is deferred to a follow-up PR.

---

## Migration from v1.3

The shadow agent is a new additive feature with the following migration considerations:

1. **Disabled by default** — v1.3 behavior preserved when `alignmentCheck.enabled: false`
2. **New API fields** — `alignment_verdict` is added to `IncidentResponse` (KA OpenAPI) and `AIAnalysisStatus` (CRD). These are optional fields; existing consumers that do not read them are unaffected.
3. **New CRD fields** — `alignmentVerdict` and `circuitBreakerActivated` are added to `NotificationRequest.ReviewContext`. Existing notification routing rules continue to work; new rules can match on these fields.
4. **Verdict labels** — The shadow agent uses `"aligned"` (not `"clean"`) and `"suspicious"` as verdict result values. This applies to Prometheus metric labels, audit event data, and API response fields.
5. **Rollback** — Set `alignmentCheck.enabled: false` and restart KA to disable. The `alignment_verdict` field will be absent from subsequent responses.
