# Audit Event Catalog — Kubernaut Agent (KA)

Authoritative reference for all structured audit events emitted by the `kubernaut-agent` service.

**Source of truth:** `internal/kubernautagent/audit/emitter.go` (`EventType*`/`Action*` constants, `AllEventTypes`)
**Payload mapping:** `internal/kubernautagent/audit/ds_store.go` (`buildEventData`)
**Predecessor doc:** [DD-AUDIT-003](../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"v1.3 Update: Kubernaut Agent Audit Traces" documents KA's original 7 `aiagent.*` events (v1.3) plus the 5 session-lifecycle events added in v1.9 (Issue #823) — this catalog supersedes that narrative as the complete, current reference for the `release/v1.5` line; DD-AUDIT-003 is not updated for the alignment/shadow/interactive events added since.

**Schema:** All events are built as an `audit.AuditEvent` (`internal/kubernautagent/audit/emitter.go`) and normalized to a `pkg/audit.AuditStore`-compatible record before being forwarded to Data Storage:

```go
type AuditEvent struct {
    EventType     string
    EventCategory string // "aiagent" for all events
    EventAction   string
    EventOutcome  string // "success" | "failure" | "pending"
    CorrelationID string // the investigation's RemediationID — CC8.1 reconstruction key
    SessionID     string
    ActingUser    string
    ParentEventID *uuid.UUID
    Data          map[string]interface{} // always includes event_id (UUID)
    ActorID       string // "kubernaut-agent" unless WithActor overrides it
    ActorType     string // "Service" unless WithActor overrides it
}
```

**Actor fields:** `ActorType=Service`, `ActorID=kubernaut-agent` for all service-initiated events. Interactive/user-attributed events additionally carry `ActingUser` for per-operator SOC2 CC8.1 attribution (`WithActingUser`).

---

## LLM Interaction

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.llm.request` | `EventTypeLLMRequest` | AU-2, AU-3 | Every LLM chat completion call KA makes (investigation turns, gate checks, retries) | `model`, `prompt_length`, `prompt_preview` (truncated), `toolsets_enabled` |
| `aiagent.llm.response` | `EventTypeLLMResponse` | AU-2, AU-3 | LLM response received for a turn | `has_analysis`, `analysis_length`, `analysis_preview`/`analysis_full`, `total_tokens`, `tool_call_count` |
| `aiagent.llm.tool_call` | `EventTypeLLMToolCall` | AU-2, AU-12 | Each individual tool call the LLM makes during a turn (one event per call, not per turn) | `tool_name`, `arguments` (redacted/truncated), `result_preview`, `outcome` |

**Emitted from:** `internal/kubernautagent/investigator/investigator.go`, `investigator_gates.go` (retry/gate-check paths also emit `aiagent.llm.request`)

---

## Investigation Lifecycle

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.workflow.validation_attempt` | `EventTypeValidationAttempt` | AU-2, CC7.2 | Each attempt to validate the LLM's proposed workflow selection against catalog constraints (same-kind gate, API-version gate, target-alignment gate) | `workflow_id`, `is_final_attempt`, `gate` (`same_kind`/`api_version`/`target_alignment`), `outcome`, `reason` (on failure) |
| `aiagent.response.complete` | `EventTypeResponseComplete` | AU-2, CC8.1 | Investigation reaches a terminal successful result (`submit_result_*` tool call accepted) | `rca_summary`, `confidence`, `workflow_id` (if one was selected), `severity`, `total_tokens` |
| `aiagent.response.failed` | `EventTypeResponseFailed` | AU-2, CC8.1 | Investigation loop exhausts `MaxTurns`/`MaxReinvocations` or the LLM never calls a terminal tool | `reason`, `turns_used` |
| `aiagent.rca.complete` | `EventTypeRCAComplete` | AU-2, AU-3, CC8.1 | RCA phase specifically completes (distinct from the whole-investigation `response.complete`, carries the forensic RCA detail) | `rca_summary`, `confidence`, `total_tokens` |
| `aiagent.investigation.cancelled` | `EventTypeInvestigationCancelled` | AU-2, CC8.1 | Investigator detects context cancellation mid-investigation (BR-SESSION-001) | `phase`, `turn`, `tokens_used_so_far` — carries investigation-internal state a plain session-level cancellation event cannot, for partial-progress audit reconstruction |

**Emitted from:** `internal/kubernautagent/investigator/investigator_audit.go`, `investigator.go`, `investigator_gates.go`

---

## Enrichment (Phase 2 RCA)

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.enrichment.completed` | `EventTypeEnrichmentCompleted` | AU-2, CC7.2 | Phase 2 enrichment succeeds (root owner resolved, labels detected, remediation history fetched) | `incident_id`, `root_owner_kind`, `root_owner_name`, `root_owner_namespace`, `owner_chain_length`, `remediation_history_fetched` |
| `aiagent.enrichment.failed` | `EventTypeEnrichmentFailed` | AU-2 | Phase 2 enrichment fails after retry exhaustion | `incident_id`, `reason`, `detail`, `affected_resource_kind`, `affected_resource_name`, `affected_resource_namespace` |

**Emitted from:** `internal/kubernautagent/enrichment/enricher.go`

---

## Alignment & Grounding (BR-ALIGNMENT)

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.alignment.step` | `EventTypeAlignmentStep` | CC7.2 | Each step of the workflow-alignment evaluation pipeline runs | `step_name`, `outcome` |
| `aiagent.alignment.verdict` | `EventTypeAlignmentVerdict` | CC7.2, AU-3 | Final alignment verdict is reached for a proposed workflow (approve/reject/needs-revision) | `verdict`, `workflow_id`, `reasoning` |
| `aiagent.alignment.grounding.request` | `EventTypeGroundingRequest` | AU-2 | Alignment evaluator issues a grounding LLM call to verify a claim against retrieved evidence | `claim_preview` |
| `aiagent.alignment.grounding.response` | `EventTypeGroundingResponse` | AU-2 | Grounding LLM call returns | `grounded`, `evidence_preview` |

**Emitted from:** `internal/kubernautagent/alignment/investigator_wrapper.go`, `grounding.go`

---

## Shadow Mode (BR-ALIGNMENT shadow evaluation)

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.shadow.llm.request` | `EventTypeShadowLLMRequest` | AU-2 | A shadow-mode alignment evaluation issues its own (non-production-affecting) LLM call, run in parallel with the real investigation for A/B comparison | `model`, `prompt_length` |
| `aiagent.shadow.llm.response` | `EventTypeShadowLLMResponse` | AU-2 | Shadow-mode LLM call returns | `has_analysis`, `analysis_length` |

**Emitted from:** `internal/kubernautagent/alignment/evaluator.go`

---

## Session Lifecycle

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.session.started` | `EventTypeSessionStarted` | AU-2, CC8.1 | `session.Manager.StartInvestigation` transitions a new session to `StatusRunning` | `session_id` (via `SessionID` field) |
| `aiagent.session.cancelled` | `EventTypeSessionCancelled` | AU-2, CC8.1 | An interactive session is explicitly cancelled by the operator (`terminateSession`) | `session_id`, `reason` |
| `aiagent.session.completed` | `EventTypeSessionCompleted` | AU-2, CC8.1 | Session reaches a terminal `StatusCompleted` state | `session_id`, `total_duration_ms` |
| `aiagent.session.failed` | `EventTypeSessionFailed` | AU-2, CC8.1 | Session reaches a terminal `StatusFailed` state (investigation error, LLM exhaustion, etc.) | `session_id`, `error` |
| `aiagent.session.observed` | `EventTypeSessionObserved` | AU-2, CC8.1 | An operator subscribes to an active investigation's SSE stream (BR-SESSION-005) | `session_id`, `observer_user` (authenticated identity of the subscriber, from `auth.UserContextKey`) |
| `aiagent.session.access_denied` | `EventTypeSessionAccessDenied` | AC-3, AU-2, CC8.1 | An authenticated user attempts to access a session they do not own | `requesting_user`, `session_id`, `endpoint` |
| `aiagent.session.resumed` | `EventTypeSessionResumed` | AU-2, CC8.1 | An autonomous investigation resumes control after an interactive session ends, via cancel+reconstruct (BR-INTERACTIVE-003 #5) | `session_id` (the ended interactive session), `reconstructed_turn_count` |

**Emitted from:** `internal/kubernautagent/session/manager.go`, `internal/kubernautagent/mcp/reconstruct.go` (`ReconstructionSpawner.emitSessionResumed`, wired via `SetAuditStore` in `cmd/kubernautagent/main.go`)

---

## Interactive Mode (BR-INTERACTIVE)

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.interactive.started` | `EventTypeInteractiveStarted` | AU-2, CC8.1 | A user acquires the interactive Lease and begins driving the investigation (BR-INTERACTIVE-004) | `session_id`, `acting_user` |
| `aiagent.interactive.completed` | `EventTypeInteractiveCompleted` | AU-2, CC8.1 | An interactive session ends (explicit complete, cancel, disconnect, or timeout) | `session_id`, `reason` |
| `aiagent.session.suspended` | `EventTypeSessionSuspended` | AU-2, CC8.1 | An autonomous investigation is suspended for dynamic takeover (BR-INTERACTIVE-004); identity transition KA SA → human operator (DD-INTERACTIVE-002) | `session_id`, `reason` |
| `aiagent.interactive.k8s_call` | `EventTypeInteractiveK8sCall` | AC-6, AU-12, CC8.1 | The K8s lookup `enrichment.Enricher` performs on behalf of the acting user during interactive `kubernaut_select_workflow` (BR-INTERACTIVE-003 #3). One event summarizes the enrichment call (owner-chain walk + spec-hash fetch), matching `EventTypeEnrichmentCompleted`'s per-`Enrich()`-call granularity rather than per raw K8s `Get`. Per #1288, KA never impersonates the user at the K8s API level — the call always runs under KA's own SA, so this event is for audit *attribution*, not RBAC impersonation. | `resource`, `verb`, `namespace`, `resource_name`, `http_status_code` (200/403/500) |

**Emitted from:** `internal/kubernautagent/mcp/tools/investigate.go`, `internal/kubernautagent/session/manager.go`, `internal/kubernautagent/mcp/tools/select_workflow.go` (`SelectWorkflowTool.emitInteractiveK8sCall`, wired via `WithSelectWorkflowAuditStore` in `cmd/kubernautagent/main.go`)

---

## Security & Access Control

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.auth.failure` | `EventTypeAuthFailure` | AU-2, AC-7 | Inbound request authentication fails (invalid/expired token) at the server auth middleware | `reason`, `source_ip` |
| `aiagent.auth.denied` | `EventTypeAuthDenied` | AC-3, AC-6 | Authenticated request denied by authorization policy (RBAC/SAR) | `user`, `endpoint`, `reason` |
| `aiagent.ratelimit.denied` | `EventTypeRateLimitDenied` | SC-5 | Request rejected by KA's rate limiter | `client_ip`, `limit` |

**Emitted from:** `internal/kubernautagent/server/audit_middleware.go`, `ratelimit.go`

---

## Backend & Delivery

Events are delivered through the `audit.AuditStore` interface (`internal/kubernautagent/audit/emitter.go`), typically via the fire-and-forget `StoreBestEffort` helper — an audit store failure is logged but never turns an otherwise-successful operation into a caller-visible error (ADR-038: audit must never block business logic).

`event.ActorID`/`event.ActorType` are back-filled from context (`audit.WithActor`) if not already set explicitly on the event.

Payloads are normalized to Data Storage's OpenAPI discriminated-union schema (`pkg/datastorage/ogen-client`) by `buildEventData` (`internal/kubernautagent/audit/ds_store.go`) before being forwarded.

---

## Known Gaps (tracked, not fixed by this catalog)

1. ~~**`aiagent.session.resumed` and `aiagent.interactive.k8s_call` are defined and DS-payload-wired but have no production emitter**~~ **CLOSED.** Both now have production emitters: `aiagent.session.resumed` from `ReconstructionSpawner.emitSessionResumed` (`internal/kubernautagent/mcp/reconstruct.go`, called from `SpawnReconstruct` before the reconstructed `RunReconTurn`), and `aiagent.interactive.k8s_call` from `SelectWorkflowTool.emitInteractiveK8sCall` (`internal/kubernautagent/mcp/tools/select_workflow.go`, called from the `WithEnrichmentRunner` hook after `enrichment.Enrich()`). Both are wired into production via `cmd/kubernautagent/main.go` (`SetAuditStore`/`WithSelectWorkflowAuditStore`) and covered by unit tests plus a real-`Enricher` IT (`select_workflow_k8scall_it_test.go`, IT-KA-898-K8SCALL-001/002) proving the full production call path, not just a mocked `EnrichmentRunner`. Backported from `main` (#898, [PR #1812](https://github.com/jordigilh/kubernaut/pull/1812)).

---

## Adding New Events

1. Define the `EventType`/`Action` constants in `internal/kubernautagent/audit/emitter.go` and add to `AllEventTypes`
2. Add the emit call at the appropriate production call site (never only in a test)
3. Add a payload mapping in `ds_store.go`'s `buildEventData`
4. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
5. Ensure a UT proves the emission decision and, for any new production entry point, an IT proves it fires through that entry point (Pyramid Invariant — UT proves logic, IT proves wiring)

---

*Last updated: 2026-07-31 | #898 backport from main ([PR #1812](https://github.com/jordigilh/kubernaut/pull/1812)) | Covers all 29 event types in `AllEventTypes` as of this date on `release/v1.5`*
