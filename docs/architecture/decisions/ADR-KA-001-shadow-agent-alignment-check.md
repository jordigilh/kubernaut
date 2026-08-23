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

This ADR covers **two evaluation layers**, added in two increments (v1.0 and v1.2 below): per-step evaluation of individual LLM/tool-output content as it streams through the pipeline, and a full-context grounding review of the entire RCA conversation at RCA-phase completion. The second layer exists because the first has a structural blind spot — see [Two-Layer Evaluation Model](#two-layer-evaluation-model) under Decision.

### Alternatives Considered

1. **Regex/heuristic scanner** — Rejected. Pattern matching cannot detect novel or obfuscated injection techniques (Unicode homoglyphs, nested JSON encoding, context-dependent authority impersonation). LLM-based evaluation provides semantic understanding of injection intent.
2. **Inline content filtering before LLM** — Rejected. Pre-filtering would strip potentially legitimate content (e.g., a pod log containing "SYSTEM:" as a normal application prefix). The shadow agent evaluates content in context without modifying the investigation flow.
3. **Post-investigation batch review** — Rejected. Too late — if the primary LLM is already manipulated, the investigation result is compromised. Real-time parallel evaluation catches injection before the primary LLM acts on it.
4. **Dedicated sidecar process** — Deferred. The current in-process goroutine-based design avoids network latency and deployment complexity. [#1681](https://github.com/jordigilh/kubernaut/issues/1681) describes the now-current path for opaque agents: `AuthBridge` (which already intercepts agent traffic for credential injection, [#1535](https://github.com/jordigilh/kubernaut/issues/1535)) tees a copy of the intercepted stream to a shadow-style evaluator. See [ADR-KA-002](ADR-KA-002-agent-security-defense-in-depth.md).

## Decision

### Two-Layer Evaluation Model

The shadow agent runs **two independent evaluation layers** against the same per-investigation `Observer`/`Evaluator` pair. Both are fail-closed and both feed the same final `Verdict`, but they catch different classes of prompt injection:

| Layer | Method | Scope | Catches |
|---|---|---|---|
| **1. Per-step** | `Evaluator.EvaluateStep` | One LLM/tool-output step at a time, no conversation history | Injection payloads that are aggressive enough to look suspicious *on their own* within a single step |
| **2. Full-context grounding** | `Evaluator.EvaluateGrounding` | The entire RCA conversation, once, at RCA-phase completion | **Distributed / "boiling frog" injection**: payloads spread thin across several tool outputs, each individually clean enough to pass layer 1, that cumulatively nudge the primary LLM's conclusion by the end |

Layer 2 exists because layer 1 has a structural blind spot: it evaluates each step in isolation, so it can never see the evidence chain as a whole. An attacker who can influence several tool-observable fields (K8s labels, annotations, ConfigMap values, log lines, event messages) can spread an injection across steps that individually appear clean but collectively steer the RCA — layer 1 has no mechanism to detect that because no single step it evaluates ever looks wrong. Layer 2 closes this gap by asking the shadow LLM, once, with the full evidence chain in view: "given the actual tool evidence, is this RCA genuinely grounded, or did it drift?"

### Architecture: Transparent Proxy Pattern

The shadow agent is implemented as a transparent proxy layer that wraps the KA's LLM client and tool registry without modifying the core investigation logic:

```
InvestigatorWrapper
│
├─ LLMProxy  ─┐  (per LLM response)
├─ ToolProxy ─┤  (per tool result)
│             ▼
│         Observer (per-investigation)
│             │
│             ├──▶ Layer 1: EvaluateStep(step)          [async, one call per step]
│             │      via Evaluator (Shadow LLM)
│             │
│             └──▶ Layer 2: StartGroundingReview(...)   [fires once, RCA-phase done]
│                    → EvaluateGrounding(conversation)
│                    via Evaluator (Shadow LLM)
│
▼
Observer.RenderVerdict()  — single Verdict, either layer can flag suspicious
│
▼
InvestigationResult + Verdict ──▶ Audit Trail
```

**Components**:

- **`LLMProxy`** — Wraps `llm.Client`. After each `Chat()` call, submits the LLM response content to the Observer as a `StepKindLLMReasoning` step (feeds Layer 1).
- **`ToolProxy`** — Wraps `registry.ToolRegistry`. After each `Execute()` call, submits the tool result (or error message) to the Observer as a `StepKindToolResult` step (feeds Layer 1).
- **`Observer`** — Per-investigation instance (scoped via context). Receives steps asynchronously, dispatches to the Evaluator in goroutines for Layer 1, and separately launches Layer 2 via `StartGroundingReview` when the RCA phase completes. Collects both sets of observations and renders a single final verdict via `RenderVerdict`.
- **`Evaluator`** — Implements both evaluation methods against the shadow LLM:
  - `EvaluateStep` (Layer 1) — sends one step to the shadow LLM with a security-focused system prompt. Returns `Observation{Suspicious: bool, Explanation: string}`.
  - `EvaluateGrounding` (Layer 2) — sends the flattened, full RCA conversation (including any `Reasoning`/thinking-block text, #1935/#1936) to the shadow LLM in a single call. Returns `GroundingObservation{Grounded: bool, Explanation: string}`.
  Both are fail-closed on all error paths.
- **`InvestigatorWrapper`** — Wraps the real `InvestigationRunner`. Creates a fresh Observer per investigation, injects it into the context, delegates to the inner runner, waits for all Layer 1 *and* Layer 2 evaluations to complete (with timeout), and applies the verdict.

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

### Full-Context Grounding Review (Layer 2)

`Observer.StartGroundingReview` launches Layer 2 asynchronously, once, when the RCA phase completes — in parallel with workflow discovery, so it adds no serial latency to the happy path:

1. The full RCA conversation (all messages, flattened, including `Reasoning`/thinking-block text) is rendered to a single text blob, truncated to `maxConversationTokens` runes (head-truncation; default 32000).
2. The blob is sent to the shadow LLM in one `Chat()` call with `JSONMode: true`, asking: is this RCA grounded in the tool evidence actually present in the conversation?
3. The shadow LLM's structured response (`{grounded: bool, explanation: string}`) is parsed fail-closed:
   - Duplicate `"grounded"` JSON key → `duplicate_key_attack (fail-closed)` (defends against the same key-stuffing trick used to defeat naive JSON parsing of the shadow's own verdict)
   - JSON parse error → `grounding_review_failed (fail-closed): parse error`
   - Missing `grounded` field → `grounding_review_failed (fail-closed): missing 'grounded' field`
   - LLM client error / timeout → `grounding_review_failed (fail-closed)`
   - Empty/nil conversation → `grounding_review_failed (fail-closed): empty conversation` (no LLM call made)
4. The result feeds the same `onSuspicious` circuit-breaker callback as Layer 1 (`sync.Once`-guarded) and is folded into the single final `Verdict` alongside Layer 1's per-step observations.

Layer 2 is opt-in and independently configurable from Layer 1 (`groundingReview.enabled`, default `false`) — an operator can run per-step evaluation alone, or both layers together.

### Verdict and Escalation

After the primary investigation completes, the wrapper waits up to `verdictTimeout` (30s) for all shadow evaluations — both layers — to finish:

- **Clean** — All Layer 1 steps passed and (if enabled) Layer 2 returned `grounded=true`. Investigation result is returned unchanged.
- **Suspicious** — One or more Layer 1 steps flagged, or Layer 2 returned `grounded=false`. The investigation result is annotated with `HumanReviewNeeded=true`, `HumanReviewReason="alignment_check_failed"`, and a warning summarizing flagged steps.
- **Timeout** — Pending evaluations (either layer) that did not complete within the deadline are treated as suspicious (fail-closed). The verdict includes `TimedOut=true` and `Pending=N`.

### Audit Trail

Every suspicious step, every grounding review, and the final verdict are emitted as audit events:
- `alignment.step` — Per-step (Layer 1) event with step index, kind, tool name, and explanation
- `alignment.grounding.request` / `alignment.grounding.response` — Layer 2 request/response pair, with conversation length/token estimate and the `grounded`/`result`/duration/token-usage outcome
- `alignment.verdict` — Final verdict with result, summary, flagged/total counts (covers both layers)

### LLM Configuration

The shadow agent can use a dedicated LLM or share the investigation LLM:

- **Shared** (default when `alignmentCheck.llm` is empty) — Reuses the investigation LLM client. Simpler but increases latency on the shared client.
- **Dedicated** — Separate provider/model/endpoint. Recommended for production: use a fast, cost-effective model (e.g., `gpt-4o-mini`) while the investigation uses a more capable model.

Configuration merges with the base `llm` config — only overridden fields need to be specified.

### Fail-Closed Guarantees

Every error path in the shadow agent results in a suspicious/ungrounded verdict, never a silent pass:

**Layer 1 (`EvaluateStep`) → `Suspicious=true`**:
- LLM client error → `evaluator_unavailable (fail-closed)`
- JSON parse error → `evaluator_unavailable (fail-closed)`
- Missing `suspicious` field in response → `evaluator_unavailable (fail-closed)`
- Context cancellation → `evaluator_unavailable (fail-closed)`
- Boundary escape detected → `boundary escape detected (fail-closed)`
- Evaluation timeout → `verdict_timeout: N pending evaluations (fail-closed)`

**Layer 2 (`EvaluateGrounding`) → `Grounded=false`**:
- LLM client error / timeout → `grounding_review_failed (fail-closed)`
- JSON parse error → `grounding_review_failed (fail-closed): parse error`
- Missing `grounded` field → `grounding_review_failed (fail-closed): missing 'grounded' field`
- Duplicate `"grounded"` JSON key → `duplicate_key_attack (fail-closed)`
- Empty/nil conversation → `grounding_review_failed (fail-closed): empty conversation`

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
    groundingReview:               # Layer 2: full-context grounding review (#1096)
      enabled: false               # opt-in, independent of Layer 1
      timeout: 30s                 # per-review timeout for the single Chat() call
      maxConversationTokens: 32000 # head-truncation limit for the flattened conversation
```

> **Helm chart note**: as of this writing, the Helm chart's `values.schema.json` only exposes `kubernautAgent.alignmentCheck.{enabled, llmProfileRef}` — `timeout`, `maxStepTokens`, `mode`, `canary`, and `groundingReview` are configurable via the raw service ConfigMap/YAML (`internal/kubernautagent/config`) but are not yet surfaced as Helm values. Tracked as a documentation/chart-coverage gap, not a functional limitation.

### Validation Rules

When `ai.alignmentCheck.enabled=true`:
- `timeout` must be positive
- `maxStepTokens` must be positive
- If `llm` is set, `model` must be non-empty and `endpoint` is required for non-managed providers (bedrock, huggingface, anthropic, openai are managed)

When `ai.alignmentCheck.groundingReview.enabled=true` (Layer 2, independent of the above):
- `timeout` must be positive
- `maxConversationTokens` must be positive

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
- (Layer 2) `maxConversationTokens` too low could truncate the conversation before the injected/drifted content is visible to the grounding review. Default of 32000 runes covers realistic RCA conversation lengths.
- (Layer 2) Layer 2 alone cannot pinpoint *which* step introduced the drift — it only judges the end state. Layer 1's per-step findings remain the source of per-step attribution; the two layers are complementary, not substitutes for each other.

## Changelog

### v1.2 (2026-05-11) — #1096 (full-context grounding review, Layer 2)

- **`EvaluateGrounding` / `StartGroundingReview`**: Added a second, independent evaluation layer that reviews the *entire* RCA conversation in a single shadow-LLM call at RCA-phase completion, running in parallel with workflow discovery. Closes the structural blind spot of per-step evaluation (Layer 1): an attacker who spreads an injection payload across several tool outputs — each individually clean enough to pass Layer 1 — can otherwise cumulatively steer the RCA without any single step ever looking suspicious (distributed / "boiling frog" injection).
- **Config**: New `ai.alignmentCheck.groundingReview.{enabled, timeout, maxConversationTokens}`, opt-in and independent of Layer 1 (default `enabled: false`).
- **Fail-closed parsing**: `GroundingObservation` defaults to `Grounded=false` on LLM error, timeout, JSON parse error, missing `grounded` field, duplicate `"grounded"` key (prompt-injection defense against the shadow's own response), or empty/nil conversation.
- **Reasoning-block coverage** (#1935/#1936): the conversation renderer includes each turn's `Reasoning`/thinking-block text, not just `Content` — without this, Layer 2 never saw the reasoning that anchored an assistant turn's tool-use decisions.
- **Audit trail**: New `alignment.grounding.request` / `alignment.grounding.response` event pair (conversation length/token estimate on request; `grounded`/`result`/duration/token-usage on response).
- **Metrics**: New `aiagent_alignment_grounding_total{result}` and `aiagent_alignment_grounding_duration_seconds`.
- **Verdict unification**: Both layers feed the same `onSuspicious` circuit-breaker callback (`sync.Once`-guarded) and the same final `Verdict` via `Observer.RenderVerdict` — a suspicious/ungrounded result from either layer triggers the same escalation path.
- Test plan of record: [TP-1096](../../tests/1096/TEST_PLAN.md).

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
- [TP-601-v2.0](../../tests/601/TEST_PLAN_v2.md) — Shadow agent test plan (Layer 1: per-step evaluation)
- [TP-1096](../../tests/1096/TEST_PLAN.md) — Full-context grounding review test plan (Layer 2; distributed/"boiling frog" injection defense, #1096)
- [Shadow Agent Configuration Guide](../../services/kubernaut-agent/shadow-agent-configuration.md) — operator-facing config reference for both layers
