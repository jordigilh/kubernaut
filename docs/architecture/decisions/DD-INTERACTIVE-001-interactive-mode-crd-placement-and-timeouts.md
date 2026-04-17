# DD-INTERACTIVE-001: Interactive Mode CRD Placement and Timeout Policy

## Status
**✅ Approved** (2026-04-15)
**Last Reviewed**: 2026-04-15
**Confidence**: 95%

---

## Related Decisions
- **ADR-001**: CRD-Based Microservices Architecture (spec immutability)
- **DD-TIMEOUT-001**: Timeout configuration strategy
- **BR-ORCH-027/028**: Global and per-phase timeouts
- **Enhancement #703**: KA MCP Interactive Mode
- **Enhancement #705**: A2A Protocol Support

---

## Context & Problem

### Problem 1: CRD Placement (G1)

The MCP Interactive Mode (#703) introduces human-in-the-loop investigation where a user
drives the RCA conversation via an MCP-compatible chat interface instead of the autonomous
KA flow. This requires the AIAnalysis CRD to distinguish between autonomous and interactive
analysis modes.

`AIAnalysisSpec` is immutable after creation via CEL validation:

```
+kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (ADR-001)"
```

The question: where does the `InteractiveMode` flag live?

### Problem 2: Timeout Policy (G2)

The Remediation Orchestrator enforces per-phase timeouts (BR-ORCH-028):
- `TimeoutConfig.Analyzing`: default 10 minutes (RR-level)
- `AIAnalysisTimeoutConfig.InvestigatingTimeout`: default 60 seconds (AIAnalysis-level)

Interactive sessions routinely exceed both -- a human investigation can take 30 minutes.
Without accommodation, the RO will time out the RR while the user is mid-conversation.

---

## Decision: G1 -- Hybrid Placement (Option C)

### `InteractiveMode` in spec, `InteractiveSessionInfo` in status

- **`spec.interactiveMode: bool`** -- Immutable intent, set at creation by the RO based on
  the `kubernaut.ai/interactive-mode` annotation on the parent RemediationRequest.
  "This analysis was requested as interactive from the start."

- **`status.interactiveSession: *InteractiveSessionInfo`** -- Mutable runtime state tracking
  session ID, user identity, timestamps, and TTL. Only populated when
  `spec.interactiveMode` is true and a session is active.

- **Attach to ongoing** -- For users joining an in-progress autonomous analysis, the
  `kubernaut_watch` MCP tool reads existing CRD state without requiring interactive mode
  on the AIAnalysis itself.

### Rationale

1. Follows existing CRD conventions (spec = immutable intent, status = mutable runtime)
2. No CEL rule changes required -- `self == oldSelf` applies naturally
3. Clean audit trail: spec is the source of truth for how the analysis was initiated
4. The existing `InvestigationSession` (status) pattern provides precedent for runtime
   session tracking in status

### Annotation propagation

The API Frontend (or MCP session initiator) sets `kubernaut.ai/interactive-mode: "true"`
on the RemediationRequest metadata annotations. The RO reads this annotation when creating
the AIAnalysis and sets `spec.interactiveMode: true`.

### New types

```go
type InteractiveSessionInfo struct {
    SessionID string       `json:"sessionId,omitempty"`
    User      string       `json:"user,omitempty"`
    StartedAt *metav1.Time `json:"startedAt,omitempty"`
    ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}
```

---

## Decision: G2 -- Elevated Timeout Values (Option A)

### Interactive-specific timeout values, reusing existing infrastructure

When `spec.interactiveMode` is true, the RO passes elevated default timeouts through
`AIAnalysisSpec.TimeoutConfig`:

- `InvestigatingTimeout`: 30 minutes (vs. 60s default)
- RR-level `TimeoutConfig.Analyzing`: 45 minutes (vs. 10m default)

These values are configurable via Helm:

```yaml
kubernautAgent:
  interactive:
    enabled: false
    investigatingTimeout: 30m
    analyzingPhaseTimeout: 45m
    sessionTTL: 60m
```

### Rationale

1. Zero new timeout infrastructure -- the existing `AIAnalysisTimeoutConfig` and
   `TimeoutConfig` fields are fully wired in both the AA controller and the RO
2. Both controllers already respect these fields; only the default values change
3. 30-minute interactive ceiling is generous and operator-tunable via `kubectl edit`
4. Evolution path: Option C (heartbeat-based extension) can be added in a follow-up
   iteration if real-world usage demands unbounded sessions

### AA controller behavior

When `spec.interactiveMode` is true, the `InvestigatingHandler` skips the autonomous KA
invocation and returns a requeue with a poll interval. The interactive MCP session drives
completion externally. The existing timeout infrastructure enforces the elevated ceiling.

---

## Consequences

### Positive
- No breaking changes to existing CRD schema or CEL rules
- Existing autonomous flow is completely unaffected
- Timeout behavior is observable and operator-tunable
- Clean audit trail distinguishes interactive from autonomous analyses

### Negative
- RO must propagate the annotation, adding a small amount of logic
- Fixed timeout ceiling (30m) may be insufficient for very long sessions (mitigated by
  operator tunability and future heartbeat extension)

### Risks
- If the API Frontend fails to set the annotation, the analysis runs autonomously (safe
  default -- fail-open to existing behavior)
- Orphaned interactive sessions are cleaned up by the timeout infrastructure (same as
  autonomous sessions, just with a longer ceiling)

---

## Evolution Path

| Phase | Capability | Complexity |
|-------|-----------|------------|
| V1 (this DD) | Elevated static timeouts | Minimal (reuse existing infrastructure) |
| V2 (future) | Heartbeat-based extension (Option C) | Medium (heartbeat patch loop in KA) |
| V3 (future) | Suspend/resume with session TTL | Higher (session lifecycle management) |
