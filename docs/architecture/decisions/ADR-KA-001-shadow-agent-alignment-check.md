# ADR-KA-001: Shadow Agent Alignment Check — Prompt Injection Guardrails

**Status**: ACCEPTED
**Date**: 2026-04-28
**Issue**: [#601](https://github.com/jordigilh/kubernaut/issues/601)
**Related**: [#462](https://github.com/jordigilh/kubernaut/issues/462) (security audit framework), [#657](https://github.com/jordigilh/kubernaut/issues/657) (boundary token hardening), [ADR-KA-002](ADR-KA-002-agent-security-defense-in-depth.md) (defense-in-depth model this ADR's Layer 4 fits into), [#1681](https://github.com/jordigilh/kubernaut/issues/1681) (AuthBridge sidecar path — the dedicated-sidecar alternative this ADR deferred, now scoped for opaque agents)

## Context

The Kubernaut Agent (KA) runs an agentic LLM loop where the model has tool access to live Kubernetes clusters (pod logs, events, metrics, resource descriptions). Tool outputs and LLM reasoning pass through the investigation pipeline unsanitized — an attacker who can influence Kubernetes resource fields (labels, annotations, ConfigMap values, log output, event messages) can embed prompt injection payloads that manipulate the primary investigation LLM into:

- Overriding workflow selection or confidence scores
- Bypassing human review gates
- Exfiltrating system prompts, API keys, or internal reasoning
- Executing destructive remediation workflows

BR-AI-601 requires a parallel security auditor ("shadow agent") that monitors ALL content entering the investigation pipeline and flags prompt injection attempts. The shadow agent must be fail-closed: when it cannot evaluate content (timeout, LLM error, malformed response), the investigation is escalated to human review.

### Alternatives Considered

1. **Regex/heuristic scanner** — Rejected. Pattern matching cannot detect novel or obfuscated injection techniques (Unicode homoglyphs, nested JSON encoding, context-dependent authority impersonation). LLM-based evaluation provides semantic understanding of injection intent.
2. **Inline content filtering before LLM** — Rejected. Pre-filtering would strip potentially legitimate content (e.g., a pod log containing "SYSTEM:" as a normal application prefix). The shadow agent evaluates content in context without modifying the investigation flow.
3. **Post-investigation batch review** — Rejected. Too late — if the primary LLM is already manipulated, the investigation result is compromised. Real-time parallel evaluation catches injection before the primary LLM acts on it.
4. **Dedicated sidecar process** — Deferred. The current in-process goroutine-based design avoids network latency and deployment complexity. [#1681](https://github.com/jordigilh/kubernaut/issues/1681) describes the now-current path for opaque agents: `AuthBridge` (which already intercepts agent traffic for credential injection, [#1535](https://github.com/jordigilh/kubernaut/issues/1535)) tees a copy of the intercepted stream to a shadow-style evaluator. See [ADR-KA-002](ADR-KA-002-agent-security-defense-in-depth.md).

## Decision

### Architecture: Transparent Proxy Pattern

The shadow agent is implemented as a transparent proxy layer that wraps the KA's LLM client and tool registry without modifying the core investigation logic:

```
┌─────────────────────────────────────────────────────┐
│  InvestigatorWrapper                                │
│  ┌───────────┐    ┌──────────┐    ┌──────────────┐ │
│  │ LLMProxy  │───▶│ Observer │───▶│  Evaluator   │ │
│  └───────────┘    └──────────┘    │  (Shadow LLM)│ │
│  ┌───────────┐         │          └──────────────┘ │
│  │ ToolProxy │─────────┘                           │
│  └───────────┘                                     │
│       ▼                                            │
│  InvestigationResult + Verdict ──▶ Audit Trail     │
└─────────────────────────────────────────────────────┘
```

**Components**:

- **`LLMProxy`** — Wraps `llm.Client`. After each `Chat()` call, submits the LLM response content to the Observer as a `StepKindLLMReasoning` step.
- **`ToolProxy`** — Wraps `registry.ToolRegistry`. After each `Execute()` call, submits the tool result (or error message) to the Observer as a `StepKindToolResult` step.
- **`Observer`** — Per-investigation instance (scoped via context). Receives steps asynchronously, dispatches to the Evaluator in goroutines, collects observations, and renders a final verdict.
- **`Evaluator`** — Sends each step to the shadow LLM with a security-focused system prompt. Returns `Observation{Suspicious: bool, Explanation: string}`. Fail-closed on all error paths.
- **`InvestigatorWrapper`** — Wraps the real `InvestigationRunner`. Creates a fresh Observer per investigation, injects it into the context, delegates to the inner runner, waits for all evaluations to complete (with timeout), and applies the verdict.

### Content Evaluation

Each step's content is wrapped in random boundary markers before sending to the shadow LLM:

```
<<<EVAL_{random_token}>>>
{untrusted content}
<<<END_EVAL_{random_token}>>>
```

The boundary token is unique per evaluation and the raw content is pre-scanned for escape attempts (content containing the closing marker). This prevents the untrusted content from breaking out of the evaluation frame and manipulating the shadow agent's own prompt.

Content exceeding `maxStepTokens` (default: 500 runes) is truncated using a head+tail strategy that preserves both the beginning and end of the content, which are the most common injection sites.

### Signal Input Evaluation

The signal context (alert message, severity, resource identity) is submitted to the shadow as step 0 before the primary investigation begins. This ensures injection-like content in incident fields (e.g., a crafted `ErrorMessage` in a Kubernetes event) is evaluated even if the primary LLM does not echo it in its response.

### Verdict and Escalation

After the primary investigation completes, the wrapper waits up to `verdictTimeout` (30s) for all shadow evaluations to finish:

- **Clean** — All steps passed. Investigation result is returned unchanged.
- **Suspicious** — One or more steps flagged. The investigation result is annotated with `HumanReviewNeeded=true`, `HumanReviewReason="alignment_check_failed"`, and a warning summarizing flagged steps.
- **Timeout** — Pending evaluations that did not complete within the deadline are treated as suspicious (fail-closed). The verdict includes `TimedOut=true` and `Pending=N`.

### Audit Trail

Every suspicious step and the final verdict are emitted as audit events:
- `alignment.step` — Per-step event with step index, kind, tool name, and explanation
- `alignment.verdict` — Final verdict with result, summary, flagged/total counts

### LLM Configuration

The shadow agent can use a dedicated LLM or share the investigation LLM:

- **Shared** (default when `alignmentCheck.llm` is empty) — Reuses the investigation LLM client. Simpler but increases latency on the shared client.
- **Dedicated** — Separate provider/model/endpoint. Recommended for production: use a fast, cost-effective model (e.g., `gpt-4o-mini`) while the investigation uses a more capable model.

Configuration merges with the base `llm` config — only overridden fields need to be specified.

### Fail-Closed Guarantees

Every error path in the shadow agent results in `Suspicious=true`:
- LLM client error → `evaluator_unavailable (fail-closed)`
- JSON parse error → `evaluator_unavailable (fail-closed)`
- Missing `suspicious` field in response → `evaluator_unavailable (fail-closed)`
- Context cancellation → `evaluator_unavailable (fail-closed)`
- Boundary escape detected → `boundary escape detected (fail-closed)`
- Evaluation timeout → `verdict_timeout: N pending evaluations (fail-closed)`

## Configuration

### Helm Values

```yaml
kubernautAgent:
  alignmentCheck:
    enabled: false              # Enable shadow agent alignment check
    timeout: "10s"              # Per-step evaluation timeout
    maxStepTokens: 500          # Max runes per step (head+tail truncation)
    llm: {}                     # Optional: dedicated LLM for shadow evaluation
    #   provider: "openai"      # LLM provider (inherits from base if omitted)
    #   model: "gpt-4o-mini"    # Recommended: fast, cost-effective model
    #   endpoint: ""            # Provider endpoint (inherits from base if omitted)
    #   apiKey: ""              # API key (inherits from base if omitted)
```

### Service Configuration (YAML)

```yaml
ai:
  alignmentCheck:
    enabled: true
    timeout: 10s
    maxStepTokens: 500
    llm:
      provider: "openai"
      model: "gpt-4o-mini"
```

### Validation Rules

When `ai.alignmentCheck.enabled=true`:
- `timeout` must be positive
- `maxStepTokens` must be positive
- If `llm` is set, `model` must be non-empty and `endpoint` is required for non-managed providers (bedrock, huggingface, anthropic, openai are managed)

## Consequences

### Positive

- Zero-modification to core investigation logic — proxies are transparent decorators
- Per-investigation isolation via context-scoped Observer prevents cross-request state leakage
- Concurrent evaluation — shadow runs in parallel with investigation, adding minimal latency
- Fail-closed design ensures security posture never silently degrades
- Audit trail provides forensic evidence for security review
- Boundary token randomization prevents recursive injection (attacker cannot predict the evaluation frame)

### Negative

- Doubles LLM API cost when using a dedicated shadow model (mitigated by using a cheaper model)
- Adds `verdictTimeout` (30s max) latency at investigation completion while waiting for final evaluations
- False positives from legitimate content that resembles injection patterns require human review time

### Risks

- Shadow LLM itself could be manipulated if the system prompt is weak. Mitigated by boundary token isolation and pre-scan for escape attempts.
- High-volume tool calls (8-12 per investigation) generate proportional shadow evaluations. Mitigated by async goroutine-based design and configurable timeout.
- `maxStepTokens` too low could truncate injection payloads, allowing them to pass. Default of 500 runes covers typical injection patterns while limiting evaluation cost.

## Changelog

### v1.1 (2026-05-10) — #1076, #1077, #1078, C-1

- **Circuit breaker** (#1076): Enforce mode now uses `context.WithCancelCause(ErrCircuitBreaker)` to halt the primary LLM investigation when the shadow agent detects suspicious content. Shadow evaluations continue on the parent context (ARCH-3 resolution).
- **PinDecorator** (C-1): Fixed LLMProxy bypass when `SwappableClient` pins the client snapshot. `PinDecorator` re-applies the LLMProxy around the pinned client so shadow observes all LLM traffic.
- **AlignmentVerdict schema**: New `alignment_verdict` field on `IncidentResponse` (OpenAPI) and `AIAnalysisStatus` (CRD). Populated for ALL investigations (not just suspicious). Carries `result`, `circuit_breaker_activated`, `summary`, `flagged`, `total`, and `findings`.
- **RO notification rendering**: Alignment verdict rendered prominently in manual review notifications. Circuit breaker verdicts show "SUSPICIOUS (Circuit Breaker Activated)" with findings listed before the (relegated) primary LLM RCA.
- **Priority escalation**: `alignment_check_failed` SubReason maps to `NotificationPriorityCritical`.
- **Panic recovery** (#1078): Session goroutines now recover from panics, log stack trace, and transition to `StatusFailed`.
- **Two-tier TTL eviction** (#1078): Terminal sessions evicted after `ttl`, non-terminal after `maxSessionAge` (default 2×ttl).
- **AA investigation timeout** (#1078): Wall-clock cap (`DefaultMaxInvestigationDuration = 25min`) prevents unbounded sessions.
- **Verdict label rename** (#1077): `VerdictClean` constant changed from `"clean"` to `"aligned"` for API consistency (pre-GA breaking change).

## References

- [ADR-KA-002](ADR-KA-002-agent-security-defense-in-depth.md) — defense-in-depth model; this ADR implements Layer 4 for KA-side (non-opaque-agent) investigations
- [#1681](https://github.com/jordigilh/kubernaut/issues/1681) — AuthBridge shadow-evaluator tee (extends Layer 4 coverage to opaque agents)
- [ADR-039](ADR-039-llm-prompt-response-contract.md) — LLM prompt/response contract
- [BR-AI-601](../../requirements/) — Prompt injection guardrails business requirement
- [TP-601-v2.0](../../tests/601/TEST_PLAN_v2.md) — Shadow agent test plan
