# DD-INTERACTIVE-002: Dynamic Takeover Model

**Status**: Proposed
**Decision Date**: 2026-04-29
**Version**: 1.1
**Confidence**: 95%
**Deciders**: Architecture Team
**Applies To**: kubernaut-agent, aianalysis-controller, remediation-orchestrator

**Related Business Requirements**:
- BR-INTERACTIVE-001: Interactive investigation sessions
- BR-INTERACTIVE-004: Dynamic takeover of autonomous investigations
- BR-INTERACTIVE-005: Session lifecycle and timeout management
- BR-INTERACTIVE-006: Cross-session visibility via audit trail

**Related Design Decisions**:
- DD-INTERACTIVE-001: Interactive mode CRD placement and timeouts (**SUPERSEDED by this document**)
- DD-AUTH-MCP-001: MCP endpoint security and user impersonation
- DD-AUDIT-003: Service audit trace requirements
- ADR-038: Async buffered audit ingestion

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-04-29 | AI-assisted | Initial design. Supersedes DD-INTERACTIVE-001. |
| 1.1 | 2026-05-15 | AI-assisted | Clarified v1.5 takeover-abandon semantics: no autonomous resume on disconnect. Added SEC-TAKEOVER-001 security rationale. Updated connection flow diagram. |

---

## Supersession Notice

**This document supersedes DD-INTERACTIVE-001** (Interactive Mode CRD Placement and Timeouts, approved 2026-04-15).

### Delta Table

| Aspect | DD-INTERACTIVE-001 | DD-INTERACTIVE-002 (this) |
|--------|---------------------|---------------------------|
| Mode selection | `spec.interactiveMode` immutable field set at RR creation | Dynamic: every RR is takeover-capable. No spec field. |
| Annotation propagation | `kubernaut.ai/interactive-mode` annotation on RR | Eliminated. No annotation. |
| `kubernaut_watch` tool | Specified for passive observation | Replaced by NotificationBus + Observer role |
| Timeout model | Elevated static defaults (30m investigating, 45m analyzing) | Global timeout (1h) as hard cap. Dynamic extension bounded by global. |
| Session lifecycle | Static: interactive from creation to completion | Dynamic: autonomous → takeover → drive → disconnect → reconstruct → autonomous |
| CRD changes | `spec.interactiveMode` on AA, annotation on RR | `status.interactiveSession` on AA only (observability) |
| Evolution path | V1 static → V2 heartbeat → V3 suspend/resume | Cancel+reconstruct. NotificationBus extensible to v1.6 token streaming. |

---

## Context & Problem

### Current State

DD-INTERACTIVE-001 designed interactive mode as a **binary, creation-time choice**: either a remediation is interactive (set at RR creation) or it's autonomous. This requires predicting at creation time whether human intervention will be needed.

### Problem Statement

Real-world operations don't work this way. An SRE sees an ongoing autonomous investigation, decides the AI is going in the wrong direction, and wants to jump in -- without having predicted this need at creation time. Conversely, if no humans are available, the system should run fully autonomously with no configuration changes.

### Constraints

- Autonomous mode must be completely unaffected (zero regression)
- KA codebase has no native suspend/resume for goroutines
- `context.Cancel` is the existing mechanism for stopping autonomous work
- DS audit events already store full conversation turns
- Global RO timeout (1h) must be respected as hard boundary

---

## Decision Drivers

1. **Operational reality**: SREs join ongoing incidents, they don't predict them at creation time
2. **Simplicity**: Use existing Go primitives (`context.Cancel`) over new state machine infrastructure
3. **Zero waste**: Cancel+reconstruct reuses existing codepaths (DS query, conversation building)
4. **Extensibility**: Foundation for v1.6 multi-agent concurrent sessions
5. **UX**: Must feel like joining a Slack thread -- see full history, pick up where AI left off

---

## Alternatives Considered

### Alternative A: Static Mode (DD-INTERACTIVE-001) -- REJECTED

**Approach**: `spec.interactiveMode: true` set at creation. Binary choice.

**Pros**:
- Simple implementation
- Clear audit trail (spec is source of truth)

**Cons**:
- Requires predicting human intervention at creation time
- Cannot join ongoing autonomous investigations
- Requires annotation propagation machinery

**Confidence**: 40% (rejected -- superseded)

### Alternative B: Dynamic Takeover with Suspend/Resume -- REJECTED

**Approach**: User connects, autonomous goroutine is suspended (serialized to memory), resumed on disconnect.

**Pros**:
- No work lost
- True pause/resume semantics

**Cons**:
- Go goroutines are not serializable
- Complex custom state machine needed
- No prior art in codebase

**Confidence**: 30% (rejected -- over-engineered)

### Alternative C: Dynamic Takeover with Cancel+Reconstruct -- CHOSEN

**Approach**: User connects, autonomous goroutine completes current turn then is cancelled. On disconnect, a NEW autonomous session reconstructs full conversation from DS audit events.

**Pros**:
- Uses existing `context.Cancel` (battle-tested Go primitive)
- DS audit events already contain full conversation for reconstruction
- Same recovery codepath as pod-restart recovery
- Simple: no serializable state, no custom suspend machinery

**Cons**:
- Reconstruction has DS query latency (~100ms)
- LLM context rebuilding consumes tokens (acceptable for infrequent transitions)

**Confidence**: 95% (chosen)

---

## Decision

### Chosen: Alternative C -- Dynamic Takeover with Cancel+Reconstruct

All remediations start autonomous (KA SA drives). A human user can take over at any time by connecting via MCP and sending explicit `action: takeover`. The autonomous investigation is cancelled (after completing its current LLM turn) and the user drives. On disconnect, a NEW autonomous session reconstructs the full conversation from DS audit events.

### Architecture

#### Connection Flow

```
Time →
  KA SA ──▶ [autonomous turns] ──▶ user connects (observes via NotificationBus)
                                           │
                                      user sends action: takeover
                                           │
                                           ▼
                            LLM completes current turn → autonomous CANCELLED (one-way)
                                           │
                                           ▼
  User ────▶ [auto-inject context from DS] ──▶ [interactive turns (impersonated)]
                                           │
                                  ┌────────┴────────┐
                                  │                  │
                           user completes      user abandons / disconnects
                                  │                  │
                                  ▼                  ▼
                         Lease released     Lease expires (inactivity timeout)
                         RR progresses      AA phase times out → RO creates NEW RR
```

> **v1.5 scope (SEC-TAKEOVER-001)**: Autonomous investigation is **not** resumed after
> takeover. The user who takes over owns the investigation until completion. If the user
> abandons it, the inactivity timeout releases the Lease, the AA phase times out on the
> RO side, and the Gateway creates a fresh RemediationRequest.
>
> **Rationale**: A user who takes over can steer the LLM context in any direction —
> including nudging it toward executing destructive workflows that the user would not
> have RBAC access to trigger directly, but that KA's SA can execute. If autonomous
> mode auto-resumed from poisoned context, it could execute attacker-influenced
> workflows with KA SA privileges. This is "investigation hacking": the user manipulates
> the conversation, walks away, and lets KA auto-execute the tainted result.
>
> By making takeover a **one-way door** (`TransitionToUserDriving` cancels the autonomous
> goroutine permanently), there is no path for tainted context to flow back into
> autonomous execution. The `aiagent.session.resumed` audit event type is pre-defined
> for future use but is intentionally **not emitted** in v1.5.
>
> **Future (v1.6+)**: Autonomous resume-on-disconnect may be revisited once alignment
> grounding review can verify the user's interactive turns did not introduce unsafe
> directives. This requires the shadow agent to evaluate the full interactive
> conversation before allowing autonomous continuation.

#### Observer/Driver Model

| Role | Can Observe | Can Takeover | Lease Required | K8s RBAC |
|------|-------------|-------------|----------------|----------|
| Observer | Yes (NotificationBus) | No | No | `get` on `services/kubernaut-agent` |
| Driver | Yes | Yes (explicit `action: takeover`) | Yes (single-driver) | `create` on `services/kubernaut-agent` |

Multiple observers can connect simultaneously. Only one driver at a time (Lease enforces).

#### Explicit Takeover Requirement

A user's first regular message does **NOT** trigger takeover. The `action: takeover` parameter is required:

```json
{
  "tool": "kubernaut_investigate",
  "arguments": {
    "rr_id": "my-remediation",
    "action": "takeover"
  }
}
```

This prevents accidental takeover from stray keypresses during incidents.

#### Identity Transitions in Audit Trail

```
session_id: sess-KA-01  | acting_user: system:sa:ka     | aiagent.llm.request
session_id: sess-KA-01  | acting_user: system:sa:ka     | aiagent.llm.response
session_id: sess-KA-01  | acting_user: system:sa:ka     | aiagent.session.suspended  ← takeover
session_id: sess-USR-02 | acting_user: user-a@corp      | aiagent.interactive.started
session_id: sess-USR-02 | acting_user: user-a@corp      | aiagent.llm.request
session_id: sess-USR-02 | acting_user: user-a@corp      | aiagent.interactive.k8s_call
session_id: sess-USR-02 | acting_user: user-a@corp      | aiagent.interactive.completed  ← disconnect
session_id: sess-KA-03  | acting_user: system:sa:ka     | aiagent.session.resumed
session_id: sess-KA-03  | acting_user: system:sa:ka     | aiagent.llm.request  ← continues
```

Note: `sess-KA-03` is a NEW session (not `sess-KA-01`). Cancel+reconstruct creates a fresh session that reads the full audit history.

### NotificationBus

Generic pub/sub (~120 lines) delivering completed audit events to connected observers/drivers:

```go
type NotificationType string
const (
    NotificationAuditEvent NotificationType = "audit_event" // v1.5
    NotificationToken      NotificationType = "token"       // v1.6
)

type NotificationBus interface {
    Subscribe(correlationID, sessionID string) <-chan Notification
    Publish(correlationID string, n Notification)
    Unsubscribe(correlationID, sessionID string)
}
```

- Bounded channel buffer with configurable drop policy (slow consumers don't block publisher)
- v1.6 extends with `NotificationToken` for sub-turn LLM token streaming
- Zero v1.5 code thrown away

### Timeout Model

- **Global timeout (1h)**: Hard cap. No extension beyond global. Late takeovers are bounded by remaining global time.
- **Inactivity timeout (10m)**: Configurable. Session released if no tool call arrives within window.
- **Timeout warnings**: MCP `notifications/progress` at T-10m and T-2m before global timeout, T-2m before inactivity cutoff.
- **Commands in-flight at cutoff**: Cancelled gracefully, partial result saved as audit event.

### Cross-Session Visibility

`session_id` is a **mandatory field** on ALL `AuditEvent` instances. This enables any session to see what other sessions found by querying DS by `correlation_id`. Auto-inject seeds the new session's LLM context with prior findings -- feels like joining a Slack channel.

### CRD Impact

**Minimal**: Only `status.interactiveSession` added to `AIAnalysisStatus` for observability:

```go
type InteractiveSessionInfo struct {
    SessionID    string       `json:"sessionId,omitempty"`
    MCPSessionID string       `json:"mcpSessionId,omitempty"`
    ActingUser   string       `json:"actingUser,omitempty"`
    StartedAt    *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt  *metav1.Time `json:"completedAt,omitempty"`
}
```

No spec changes. No annotation changes. Every RR is takeover-capable by default.

---

## Consequences

### Positive Consequences
1. SREs can join ongoing investigations at any time without prior configuration
2. Autonomous mode is completely unaffected (zero regression risk)
3. Cancel+reconstruct is simpler than suspend/resume (no serializable state)
4. DS audit trail provides full conversation reconstruction (same codepath as pod-restart recovery)
5. NotificationBus extensible to v1.6 token streaming with zero throwaway
6. Observer role enables passive monitoring without disrupting investigations

### Negative Consequences
1. Reconstruction has DS query latency (~100ms)
   - **Mitigation**: Best-effort auto-inject; takeover proceeds even if DS is slow
2. LLM context rebuilding on reconstruct consumes tokens
   - **Mitigation**: Infrequent transitions (typically 0-1 per investigation). Summarization reduces token count.

### Risks
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| DS unavailable during auto-inject | Low | Medium | Best-effort; session starts with empty prior context, warning logged |
| Rapid connect/disconnect creates many reconstruct cycles | Low | Low | Each reconstruct is independent; Lease prevents concurrent drivers |
| Global timeout too short for complex interactive investigations | Medium | Medium | Documented as hard cap; operator-tunable via config |
| SEC-TAKEOVER-001: Investigation hacking via takeover-then-abandon | Medium | Critical | Takeover is a one-way door; autonomous goroutine cancelled permanently. No resume-on-disconnect in v1.5. User must complete or session times out; RO creates fresh RR. See Connection Flow diagram. |
| Concurrent `action: start` requests for same RR (double-click) | Low | Low | K8s Lease `Create` is atomic — exactly one request wins. Losing request gets `ErrLeaseHeld`. Local `rrIndex` check may race with `rrIndex.Store`, but K8s is the source of truth and no data corruption results. User sees a clear "session active" error. No fix needed in v1.5. |
| KA pod restart with orphaned K8s Lease | Medium | Medium | v1.5: `tryReclaimExpiredLease` checks `AcquireTime + LeaseDurationSeconds`; if expired, deletes the orphaned Lease and allows a fresh `Takeover`. Non-expired Leases from live pods are not reclaimed. |
| User network disconnect during interactive session | High | Medium | v1.5: Same-user reconnect — `Takeover` detects existing session held by the same username and returns it with `Reconnected=true`, resetting the inactivity timer. No new Lease or audit event is emitted. Different user still gets `ErrLeaseHeld`. |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| BR-INTERACTIVE-001 | Pending | Interactive sessions via dynamic takeover |
| BR-INTERACTIVE-004 | Pending | Dynamic takeover via `action: takeover` |
| BR-INTERACTIVE-005 | Pending | Session lifecycle: observe → takeover → drive → disconnect → reconstruct |
| BR-INTERACTIVE-006 | Pending | Cross-session visibility via `session_id` on all audit events |

---

## Validation Strategy

1. CP-3 (SESSION & AUDIT GATE): 14 TAKE-* adversarial scenarios
2. Integration test: full takeover flow (autonomous → observe → takeover → drive → disconnect → reconstruct)
3. Unit tests: cancel+reconstruct state transitions, NotificationBus ordering, timeout warnings
4. Golden audit sequence: fixture-backed scenarios verifying event ordering

---

## Evolution Path

| Phase | Capability | Complexity |
|-------|-----------|------------|
| v1.5 (this DD) | Dynamic takeover, single driver, cancel+reconstruct, NotificationBus (audit events) | Current scope |
| v1.6 | Per-user agent isolation (concurrent multi-driver), NotificationBus (token streaming) | Medium |
| v1.7+ | Cross-agent awareness, A2A protocol integration | Higher |

---

## References

- [DD-INTERACTIVE-001](DD-INTERACTIVE-001-interactive-mode-crd-placement-and-timeouts.md): Superseded predecessor
- [DD-AUTH-MCP-001](DD-AUTH-MCP-001-mcp-endpoint-security.md): Security and impersonation model
- [ADR-038](ADR-038-async-buffered-audit-ingestion.md): Async buffered audit ingestion
- [DD-AUDIT-003](DD-AUDIT-003-service-audit-trace-requirements.md): Audit trace requirements
- Issue #703: KA MCP Interactive Mode
- Issue #822: Closed (capabilities delivered across #823 + #703)

---

**Document Version**: 1.1
**Last Updated**: 2026-05-15
