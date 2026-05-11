# Shadow Agent Escalation — Production Runbook

**Version**: v1.0
**Last Updated**: 2026-05-10
**Status**: Production Ready
**Related**: [Shadow Agent Configuration Guide](../../services/kubernaut-agent/shadow-agent-configuration.md) | [ADR-KA-001](../../architecture/decisions/ADR-KA-001-shadow-agent-alignment-check.md)
**FedRAMP Controls**: AU-2 (Audit Events), AU-3 (Audit Content), IR-6 (Incident Reporting), SI-4 (System Monitoring)

---

## Runbook Index

| ID | Runbook | Triggers On | Automation |
|----|---------|-------------|------------|
| RB-SA-001 | [Suspicious Verdict Escalation](#rb-sa-001-suspicious-verdict-escalation) | `kubernaut_alignment_verdict_total{result="suspicious"}` | Alert |
| RB-SA-002 | [Circuit Breaker Activation](#rb-sa-002-circuit-breaker-activation) | `alignment_verdict.circuit_breaker_activated=true` in AA status | Alert |
| RB-SA-003 | [Canary Integrity Failure](#rb-sa-003-canary-integrity-failure) | `kubernaut_alignment_canary_total{result="fail"}` | Alert |
| RB-SA-004 | [High Verdict Timeout Rate](#rb-sa-004-high-verdict-timeout-rate) | `kubernaut_alignment_step_total{outcome="panic"}` spike | Dashboard |
| RB-SA-005 | [Shadow Agent Unavailable](#rb-sa-005-shadow-agent-unavailable) | KA startup failure with alignment enabled | Alert |

---

## RB-SA-001: Suspicious Verdict Escalation

### When This Fires

The shadow agent detected content in an LLM response or tool output that resembles a prompt injection attempt. The investigation has been escalated to human review with `humanReviewReason: alignment_check_failed`.

### Alert Definition

```yaml
groups:
  - name: shadow-agent
    rules:
      - alert: ShadowAgentSuspiciousVerdict
        expr: rate(kubernaut_alignment_verdict_total{result="suspicious"}[5m]) > 0
        for: 1m
        labels:
          severity: warning
          component: kubernaut-agent
        annotations:
          summary: "Shadow agent flagged suspicious content"
          description: "{{ $labels.instance }} reported {{ $value | humanize }} suspicious verdicts/sec in the last 5 minutes (mode={{ $labels.mode }})"
```

### Triage Steps

1. **Identify the investigation**: Check the `AIAnalysis` CR with `humanReviewReason: alignment_check_failed`:

   ```bash
   kubectl get aianalysis -A -o jsonpath='{range .items[?(@.status.humanReviewReason=="alignment_check_failed")]}{.metadata.namespace}/{.metadata.name} {.status.alignmentVerdict.summary}{"\n"}{end}'
   ```

2. **Read the alignment verdict**: The `status.alignmentVerdict` field contains the structured verdict:

   ```bash
   kubectl get aianalysis <name> -n <namespace> -o jsonpath='{.status.alignmentVerdict}' | jq .
   ```

   Key fields:
   - `result`: `"suspicious"` confirms the escalation
   - `findings[].explanation`: What the shadow agent detected
   - `findings[].step_kind` / `findings[].tool`: Where the suspicious content appeared
   - `circuit_breaker_activated`: Whether the primary investigation was cancelled

3. **Check audit trail**: Query Data Storage for detailed per-step audit events:

   - Event type `aiagent.alignment.step`: individual flagged steps with explanations
   - Event type `aiagent.alignment.verdict`: final verdict with token usage
   - Filter by `correlation_id` (RemediationRequest name)

4. **Assess the finding**:
   - **True positive**: The content genuinely contains injection patterns (e.g., role impersonation, instruction override). Escalate per your incident response process.
   - **False positive**: The content is legitimate but resembles injection (e.g., log lines containing `SYSTEM:` headers). See tuning steps below.

### Resolution

- **True positive**: Follow your organization's incident response procedure. The suspicious content is preserved in the audit trail and AA status for forensic review.
- **False positive**: Consider tuning `maxStepTokens` (increase to preserve context) or switching to a more capable shadow model. If false positives are frequent, temporarily switch to `mode: monitor` while tuning.

---

## RB-SA-002: Circuit Breaker Activation

### When This Fires

The shadow agent detected suspicious content **during** an active investigation in `enforce` mode and cancelled the primary LLM via the circuit breaker. The primary LLM results may be incomplete or compromised.

### Alert Definition

```yaml
      - alert: ShadowAgentCircuitBreakerActivated
        expr: rate(kubernaut_alignment_verdict_total{result="suspicious",mode="enforce"}[5m]) > 0
        for: 0m
        labels:
          severity: critical
          component: kubernaut-agent
        annotations:
          summary: "Shadow agent circuit breaker activated"
          description: "{{ $labels.instance }} cancelled a primary LLM investigation due to detected prompt injection"
```

### Triage Steps

1. **Identify the investigation**: Same as RB-SA-001 step 1, but filter for `circuitBreakerActivated`:

   ```bash
   kubectl get aianalysis -A -o jsonpath='{range .items[?(@.status.alignmentVerdict.circuitBreakerActivated==true)]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}'
   ```

2. **Review the notification**: The `NotificationRequest` for this investigation contains both the shadow agent's findings (prominently displayed) and the relegated primary LLM RCA. The primary results should be treated as potentially compromised.

3. **Assess the signal source**: Determine whether the signal that triggered the investigation contained malicious content:
   - Check the originating alert/event for injection patterns
   - Review the `signal_input` step in the audit trail (step 0)
   - If the signal source is compromised, investigate the upstream alerting pipeline

### Resolution

- **Confirmed injection**: The circuit breaker worked as designed. Document the incident, review the signal source for compromise, and ensure the injection vector is mitigated.
- **False positive**: The circuit breaker cancelled a legitimate investigation. Switch to `mode: monitor` temporarily, re-trigger the investigation, and tune the shadow model or `maxStepTokens`.

---

## RB-SA-003: Canary Integrity Failure

### When This Fires

The pre-investigation canary check failed — the shadow model did not flag a known-malicious payload. This indicates the shadow model may be compromised, misconfigured, or degraded.

### Alert Definition

```yaml
      - alert: ShadowAgentCanaryFailure
        expr: kubernaut_alignment_canary_total{result="fail"} > 0
        for: 0m
        labels:
          severity: critical
          component: kubernaut-agent
        annotations:
          summary: "Shadow agent canary integrity check failed"
          description: "{{ $labels.instance }} canary failure — shadow model may be compromised or misconfigured"
```

### Triage Steps

1. **Check shadow LLM health**: Verify the shadow model endpoint is reachable and returning valid responses.
2. **Check model configuration**: Ensure the correct model is configured in `ai.alignmentCheck.llm`. A model swap (e.g., from a security-tuned model to a general model) could cause canary failures.
3. **Review shadow LLM audit events**: Check `aiagent.shadow.llm.request` and `aiagent.shadow.llm.response` events for error patterns.

### Resolution

- **Model degraded**: Restart the shadow model or switch to a backup model.
- **Model compromised**: Rotate API keys, switch providers, and escalate per security incident response.
- **Configuration error**: Fix the `ai.alignmentCheck.llm` configuration and restart KA.

---

## RB-SA-004: High Verdict Timeout Rate

### When This Fires

Shadow evaluations are frequently timing out or panicking, leading to fail-closed suspicious verdicts that may not represent actual injection attempts.

### Alert Definition

```yaml
      - alert: ShadowAgentHighTimeoutRate
        expr: rate(kubernaut_alignment_step_total{outcome="panic"}[10m]) / rate(kubernaut_alignment_step_total[10m]) > 0.1
        for: 5m
        labels:
          severity: warning
          component: kubernaut-agent
        annotations:
          summary: "Shadow agent high evaluation failure rate"
          description: "{{ $labels.instance }} has >10% panic/timeout rate in shadow evaluations"
```

### Triage Steps

1. **Check shadow LLM latency**: High latency causes per-step timeouts. Check `kubernaut_alignment_verdict_duration_seconds` histogram.
2. **Check concurrency**: With the default limit of 10 concurrent evaluations and high-volume investigations, evaluations may queue behind the semaphore.
3. **Check shadow LLM error rate**: Query `aiagent.shadow.llm.response` audit events for error responses.

### Resolution

- **High latency**: Use a faster shadow model, increase `timeout`, or use a dedicated LLM to avoid contention.
- **High error rate**: Check shadow LLM provider status, rotate API keys if rate-limited.
- **Concurrency saturation**: This is expected under load; the concurrency cap protects the shadow LLM from overload. Consider increasing `verdictTimeout` to allow more time for queued evaluations.

---

## RB-SA-005: Shadow Agent Unavailable

### When This Fires

KA failed to start because the shadow agent is enabled but the dedicated shadow LLM client could not be built. The process exits (fail-closed).

### Triage Steps

1. **Check KA pod logs**: Look for startup errors related to alignment check configuration.

   ```bash
   kubectl logs -l app=kubernaut-agent -n kubernaut --tail=50 | grep -i alignment
   ```

2. **Validate configuration**: Ensure `ai.alignmentCheck.llm` fields are correct (provider, model, endpoint, API key).

### Resolution

- Fix the LLM configuration and redeploy.
- If immediate investigation capability is needed, set `ai.alignmentCheck.enabled: false` to disable the shadow agent and restart. Investigations will proceed without alignment checks.
