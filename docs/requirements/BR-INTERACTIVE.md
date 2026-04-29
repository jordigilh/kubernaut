# Business Requirements: Interactive Investigation Mode

**Category**: Kubernaut Agent Interactive Mode
**Priority**: P1 (HIGH) - Core v1.5 Feature
**Target Version**: v1.5
**Status**: Proposed
**Date**: April 29, 2026
**Related ADRs**: ADR-038 (Async Buffered Audit Ingestion)
**Related DDs**: DD-AUTH-MCP-001 (MCP Endpoint Security), DD-INTERACTIVE-002 (Dynamic Takeover Model)
**GitHub Issue**: [#703](https://github.com/jordigilh/kubernaut/issues/703)

---

## BR-INTERACTIVE-001: Interactive Investigation Sessions

**Business Requirement ID**: BR-INTERACTIVE-001
**Priority**: P0
**Status**: Proposed

### Business Need

SREs and platform engineers need the ability to connect to an active or autonomous investigation via MCP (Model Context Protocol) to observe, interact with, and direct the AI's root cause analysis in real-time.

### Success Criteria

1. KA exposes an internal MCP Streamable HTTP endpoint at `/api/v1/mcp` when `interactive.enabled: true`
2. MCP clients can connect using K8s service account tokens (Pattern A) or via apifrontend delegation (Pattern B)
3. The endpoint supports `kubernaut_investigate`, `kubernaut_enrich`, and `kubernaut_select_workflow` tools
4. When `interactive.enabled: false` (default), the MCP endpoint is not registered (returns 404)
5. Autonomous mode behavior is completely unaffected by interactive mode code

---

## BR-INTERACTIVE-002: User-Scoped RBAC for Interactive Tools

**Business Requirement ID**: BR-INTERACTIVE-002
**Priority**: P0
**Status**: Proposed

### Business Need

When a human user drives an investigation, K8s API calls made during that session must execute under the user's identity (not KA SA), respecting the user's RBAC permissions. This prevents privilege escalation where a user could access resources beyond their authorization via KA.

### Success Criteria

1. K8s API calls during interactive sessions use `rest.ImpersonationConfig{UserName, Groups}` with the authenticated user's identity
2. Pattern A (direct): identity from TokenReview; Pattern B (apifrontend): identity from SAR-verified `Impersonate-*` headers
3. All `Impersonate-*` headers stripped from incoming requests before processing (defense-in-depth)
4. Non-K8s tools (Prometheus, DS) use KA SA (documented known limitation)
5. KA SA requires `impersonate` RBAC verb (kubernaut-operator#26)

---

## BR-INTERACTIVE-003: Audit Attribution for Interactive Actions

**Business Requirement ID**: BR-INTERACTIVE-003
**Priority**: P0
**Status**: Proposed

### Business Need

Every action during an interactive session must be attributable to the user who performed it, enabling SOC2 CC8.1 compliance and forensic reconstruction.

### Success Criteria

1. `session_id` is a mandatory top-level field on ALL `AuditEvent` instances
2. `acting_user` is set on all interactive audit events (the resolved identity, not KA SA)
3. `EventTypeInteractiveK8sCall` emitted for every impersonated K8s API call with: `acting_user`, `resource`, `verb`, `namespace`, `result`
4. Full conversation reconstruction possible via DS query by `correlation_id` ordered by `event_timestamp`
5. Identity transitions (KA SA → user → KA SA) explicitly visible in audit trail via `session.suspended` and `session.resumed` events

---

## BR-INTERACTIVE-004: Dynamic Takeover of Autonomous Investigations

**Business Requirement ID**: BR-INTERACTIVE-004
**Priority**: P0
**Status**: Proposed

### Business Need

SREs must be able to take over an ongoing autonomous investigation at any point without predicting at creation time whether human intervention will be needed. The transition must preserve all prior work.

### Success Criteria

1. Every RemediationRequest is takeover-capable by default (no spec field, no annotation)
2. Takeover requires explicit `action: takeover` (not implicit on first message)
3. Autonomous investigation completes current LLM turn before being cancelled (no lost work)
4. User's LLM context is auto-injected with autonomous findings from DS audit events
5. On disconnect, a NEW autonomous session reconstructs the full conversation from DS
6. Autonomous resumes as KA SA (user identity not retained)
7. Single-driver guarantee via K8s Lease (concurrent drivers rejected)

---

## BR-INTERACTIVE-005: Session Lifecycle and Timeout Management

**Business Requirement ID**: BR-INTERACTIVE-005
**Priority**: P1
**Status**: Proposed

### Business Need

Interactive sessions must have well-defined lifecycle boundaries (creation, activity, timeout, disconnect) to prevent resource exhaustion and orphaned state.

### Success Criteria

1. K8s Lease lock (30s duration, 15s heartbeat) provides distributed session exclusivity
2. Inactivity timeout (configurable, default 10m) releases session if no tool call arrives
3. Global timeout (1h) is a hard cap -- interactive sessions bounded by remaining global time
4. Timeout warnings emitted at T-10m and T-2m via MCP `notifications/progress`
5. Inactivity warning at T-2m before 10m cutoff
6. Lease explicitly released on session complete/cancel/disconnect
7. Pod restart recovery: Lease expires (30s), client reconnects, conversation reconstructed from DS

---

## BR-INTERACTIVE-006: Cross-Session Visibility via Audit Trail

**Business Requirement ID**: BR-INTERACTIVE-006
**Priority**: P1
**Status**: Proposed

### Business Need

When multiple actors work on the same remediation (autonomous AI + human user, or multiple users in v1.6), each must see what others have found -- like joining an ongoing Slack thread.

### Success Criteria

1. `session_id` mandatory on all audit events enables cross-session queries
2. Auto-inject: on takeover, user's LLM context seeded with autonomous findings (DS query by `correlation_id`, exclude own `session_id`)
3. Auto-inject: on resume, autonomous LLM context seeded with user's findings
4. DS REST API supports `session_id` as optional query parameter (positive match)
5. Client-side exclusion for "exclude own session" (simple, backward-compatible)

---

## BR-INTERACTIVE-007: Observable Interactive State on CRDs

**Business Requirement ID**: BR-INTERACTIVE-007
**Priority**: P2
**Status**: Proposed

### Business Need

Cluster operators need to see who is currently driving an investigation by inspecting the AIAnalysis CRD status, without querying KA directly.

### Success Criteria

1. `AIAnalysisStatus.InteractiveSession` populated when a user takes over (who, when)
2. `InteractiveSession.CompletedAt` set when user disconnects
3. `handleSessionPoll` reports `status: "user_driving"` when interactive session is active
4. AA controller requeues with extended poll interval during interactive sessions
5. All fields on `InteractiveSessionInfo` are observable via `kubectl get aa -o yaml`

---

## BR-INTERACTIVE-008: Graceful Degradation

**Business Requirement ID**: BR-INTERACTIVE-008
**Priority**: P2
**Status**: Proposed

### Business Need

Interactive mode must be additive. Failures in interactive infrastructure must not affect autonomous remediation.

### Success Criteria

1. Feature gate `interactive.enabled: false` (default) results in zero MCP-related code executing
2. DS unavailable during auto-inject: takeover proceeds with empty prior context (warning logged)
3. Apifrontend outage: only interactive clients affected, autonomous pipeline continues
4. Auth middleware nil: KA refuses to start MCP handler (hard guard, not runtime failure)
5. MCP session loss: Lease expires, client reconnects, conversation reconstructed

---

## Traceability Matrix

| BR ID | PR | Checkpoint | Test Tier |
|-------|-----|------------|-----------|
| BR-INTERACTIVE-001 | PR2 | CP-2 | Unit + Integration |
| BR-INTERACTIVE-002 | PR2 | CP-2 | Unit + Integration + E2E |
| BR-INTERACTIVE-003 | PR2 + PR3 | CP-3 | Unit + Integration |
| BR-INTERACTIVE-004 | PR3 | CP-3 | Unit + Integration + E2E |
| BR-INTERACTIVE-005 | PR1 + PR3 | CP-1 + CP-3 | Unit + Integration |
| BR-INTERACTIVE-006 | PR2 + PR3 | CP-3 | Unit + Integration |
| BR-INTERACTIVE-007 | PR1 | CP-1 | Unit |
| BR-INTERACTIVE-008 | PR2 + PR6 | CP-2 + CP-5 | Unit + E2E |
