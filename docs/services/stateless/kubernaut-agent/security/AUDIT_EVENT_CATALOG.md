# Audit Event Catalog — Kubernaut Agent (KA)

Authoritative reference for all structured audit events emitted by the `kubernaut-agent` service.

**Source of truth:** `internal/kubernautagent/audit/emitter.go` (`EventType*`/`Action*` constants, `AllEventTypes`)
**Payload mapping:** `internal/kubernautagent/audit/ds_payloads.go` (`eventTypeToPayloadBuilder` in `ds_store.go`)
**Predecessor doc:** [DD-AUDIT-003](../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"v1.3 Update: Kubernaut Agent Audit Traces" documents KA's original 7 `aiagent.*` events (v1.3) plus the 5 session-lifecycle events added in v1.9 (Issue #823) — this catalog supersedes that narrative as the complete, current reference; DD-AUDIT-003 is not updated for the alignment/shadow/fleet/secret/interactive events added since.

**Schema:** All events are built as an `audit.AuditEvent` (`internal/kubernautagent/audit/emitter.go`) and normalized to a `pkg/audit.AuditStore`-compatible record before being forwarded to Data Storage:

```go
type AuditEvent struct {
    EventType     string
    EventCategory string // "aiagent" for all events below except the 4 workflow.catalog.* events ("workflow")
    EventAction   string
    EventOutcome  string // "success" | "failure" | "pending"
    CorrelationID string // the investigation's RemediationID — CC8.1 reconstruction key
    SessionID     string
    ActingUser    string
    ClusterID     string // DD-AUDIT-003 v2.2, fleet-target investigations only
    ParentEventID *uuid.UUID
    Data          map[string]interface{} // always includes event_id (UUID)
    ActorID       string // "kubernaut-agent" unless WithActor overrides it
    ActorType     string // "Service" unless WithActor overrides it
    ResourceType  string
    ResourceID    string
}
```

**Actor fields:** `ActorType=Service`, `ActorID=kubernaut-agent` for all service-initiated events. Interactive/user-attributed events (workflow catalog discovery, some session events) additionally carry `ActingUser`/`ResourceType`/`ResourceID` for per-operator SOC2 CC8.1 attribution (`WithActingUser`, `WithResource`).

---

## LLM Interaction

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.llm.request` | `EventTypeLLMRequest` | AU-2, AU-3 | Every LLM chat completion call KA makes (investigation turns, gate checks, retries) | `model`, `prompt_length`, `prompt_preview` (truncated), `toolsets_enabled` |
| `aiagent.llm.response` | `EventTypeLLMResponse` | AU-2, AU-3 | LLM response received for a turn | `has_analysis`, `analysis_length`, `analysis_preview`/`analysis_full`, `total_tokens`, `tool_call_count`, `reasoning_text`/`reasoning_redacted` (if the model returned reasoning tokens) |
| `aiagent.llm.tool_call` | `EventTypeLLMToolCall` | AU-2, AU-12 | Each individual tool call the LLM makes during a turn (one event per call, not per turn) | `tool_name`, `arguments` (redacted/truncated), `result_preview`, `outcome` |

**Emitted from:** `internal/kubernautagent/investigator/investigator_loop.go`, `investigator_tools.go`, `investigator_gates.go` (retry/gate-check paths also emit `aiagent.llm.request`)

---

## Investigation Lifecycle

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.workflow.validation_attempt` | `EventTypeValidationAttempt` | AU-2, CC7.2 | Each attempt to validate the LLM's proposed workflow selection against catalog constraints (same-kind gate, API-version gate, target-alignment gate) | `workflow_id`, `is_final_attempt`, `gate` (`same_kind`/`api_version`/`target_alignment`), `outcome`, `reason` (on failure) |
| `aiagent.response.complete` | `EventTypeResponseComplete` | AU-2, CC8.1 | Investigation reaches a terminal successful result (`submit_result_*` tool call accepted) | `rca_summary`, `confidence`, `workflow_id` (if one was selected), `severity`, `total_tokens` |
| `aiagent.response.failed` | `EventTypeResponseFailed` | AU-2, CC8.1 | Investigation loop exhausts `MaxTurns`/`MaxReinvocations` or the LLM never calls a terminal tool | `reason`, `turns_used` |
| `aiagent.rca.complete` | `EventTypeRCAComplete` | AU-2, AU-3, CC8.1 | RCA phase specifically completes (distinct from the whole-investigation `response.complete`, carries the forensic RCA detail) | `rca_summary`, `confidence`, `reasoning_text` (redacted if oversized), `total_tokens` |
| `aiagent.investigation.cancelled` | `EventTypeInvestigationCancelled` | AU-2, CC8.1 | Investigator detects context cancellation mid-investigation (BR-SESSION-001) | `phase`, `turn`, `tokens_used_so_far` — carries investigation-internal state a plain session-level cancellation event cannot, for partial-progress audit reconstruction |

**Emitted from:** `internal/kubernautagent/investigator/investigator_audit.go`, `investigator_loop.go`, `investigator_gates.go`

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

**Reconstruction note (Issue #1818):** a session created by `handleStart`'s interactive-fallback path (`reattachOrCreateFallback`/`createFallbackSession` in `internal/kubernautagent/mcp/tools/investigate_start.go` and `investigate_autonomous.go`, entered when no `StatusRunning` autonomous session exists for the RR) carries a `mode` tag in the session's `Metadata` — `interactive_fallback` (genuine placeholder, no prior investigation) or `interactive_reattached` (seeded with the real RCA `InvestigationResult` copied from an autonomous investigation that completed and raced ahead of the interactive request). The `mode` tag is not itself a field on the `aiagent.session.started` event's `Data`; reconstructing which case occurred for a given `correlation_id` requires resolving the event's `session_id` against the session store (`Manager.GetSession`) to inspect `Metadata["mode"]` and `Result`. Before this fix, every fallback session was unconditionally `interactive_fallback` with a hardcoded placeholder `RCASummary`, silently discarding real RCA content the autonomous investigation had already produced — a BR-AUDIT-005/CC8.1 reconstruction gap (querying by `correlation_id` alone would surface a real, completed investigation session alongside a disconnected placeholder session, not one coherent narrative). Covered by UT-KA-1818-001..004 (unit) and IT-KA-1818-001/002 (integration, including an explicit audit-reconstruction-by-`correlation_id` proof) in `internal/kubernautagent/mcp/tools/`.
| `aiagent.session.cancelled` | `EventTypeSessionCancelled` | AU-2, CC8.1 | An interactive session is explicitly cancelled by the operator (`terminateSession`) | `session_id`, `reason` |
| `aiagent.session.completed` | `EventTypeSessionCompleted` | AU-2, CC8.1 | Session reaches a terminal `StatusCompleted` state | `session_id`, `total_duration_ms` |
| `aiagent.session.failed` | `EventTypeSessionFailed` | AU-2, CC8.1 | Session reaches a terminal `StatusFailed` state (investigation error, LLM exhaustion, etc.) | `session_id`, `error` |
| `aiagent.session.observed` | `EventTypeSessionObserved` | AU-2, CC8.1 | An operator subscribes to an active investigation's SSE stream (BR-SESSION-005) | `session_id`, `observer_user` (authenticated identity of the subscriber, from `auth.UserContextKey`) |
| `aiagent.session.access_denied` | `EventTypeSessionAccessDenied` | AC-3, AU-2, CC8.1 | An authenticated user attempts to access a session they do not own | `requesting_user`, `session_id`, `endpoint` |
| `aiagent.session.resumed` | `EventTypeSessionResumed` | AU-2, CC8.1 | An autonomous investigation resumes control after an interactive session ends, via cancel+reconstruct (BR-INTERACTIVE-003 #5) | `session_id` (the ended interactive session), `reconstructed_turn_count` |

**Emitted from:** `internal/kubernautagent/session/manager.go`, `manager_events.go`, `manager_query.go`, `internal/kubernautagent/mcp/reconstruct.go` (`ReconstructionSpawner.emitSessionResumed`, wired via `SetAuditStore` in `cmd/kubernautagent/routes.go`)

---

## Interactive Mode (BR-INTERACTIVE)

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.interactive.started` | `EventTypeInteractiveStarted` | AU-2, CC8.1 | A user acquires the interactive Lease and begins driving the investigation (BR-INTERACTIVE-004) | `session_id`, `acting_user` |
| `aiagent.interactive.completed` | `EventTypeInteractiveCompleted` | AU-2, CC8.1 | An interactive session ends (explicit complete, cancel, disconnect, or timeout) | `session_id`, `reason` |
| `aiagent.session.suspended` | `EventTypeSessionSuspended` | AU-2, CC8.1 | An autonomous investigation is suspended for dynamic takeover (BR-INTERACTIVE-004); identity transition KA SA → human operator (DD-INTERACTIVE-002) | `session_id`, `reason` |
| `aiagent.interactive.k8s_call` | `EventTypeInteractiveK8sCall` | AC-6, AU-12, CC8.1 | The K8s lookup `enrichment.Enricher` performs on behalf of the acting user during interactive `kubernaut_select_workflow` (BR-INTERACTIVE-003 #3). One event summarizes the enrichment call (owner-chain walk + spec-hash fetch), matching `EventTypeEnrichmentCompleted`'s per-`Enrich()`-call granularity rather than per raw K8s `Get`. Per #1288, KA never impersonates the user at the K8s API level — the call always runs under KA's own SA, so this event is for audit *attribution*, not RBAC impersonation. | `resource`, `verb`, `namespace`, `resource_name`, `http_status_code` (200/403/500) |

**Emitted from:** `internal/kubernautagent/mcp/tools/investigate_autonomous.go`, `internal/kubernautagent/session/manager_interactive.go`, `internal/kubernautagent/mcp/tools/select_workflow.go` (`SelectWorkflowTool.emitInteractiveK8sCall`, wired via `WithSelectWorkflowAuditStore` in `cmd/kubernautagent/routes.go`)

---

## Fleet Federation (DD-FLEET-004, Issue #1768)

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.fleet.overlay_failed` | `EventTypeFleetOverlayFailed` | AU-3, AC-4 | A fleet-target investigation's `FleetOverlayResolver.Overlay` call fails (resolver configured but errored). Investigation fails open — proceeds without the remote cluster's tools, like a hub-local investigation minus remote access | `cluster_id`, `error_message` |
| `aiagent.fleet.overlay_unavailable` | `EventTypeFleetOverlayUnavailable` | AU-3, AC-4 | A fleet-target investigation (non-empty `ClusterID`) reaches `prescopeFleetOverlay` on a KA instance with **no** `FleetOverlayResolver` configured at all (fleet mode not wired on this instance). Distinct from `overlay_failed`: nothing errored, fleet mode simply isn't wired here (Issue #1768 follow-up, QE audit #1799) | `cluster_id`, `reason` (`"no FleetOverlayResolver configured on this kubernaut-agent instance"`) |

**Emitted from:** `internal/kubernautagent/investigator/fleet_overlay.go` (`emitFleetOverlayFailedAudit`, `emitFleetOverlayUnavailableAudit`, both called from `prescopeFleetOverlay`)

**Test coverage:** UT-KA-FLEET-028 (unit, decision logic in isolation) + IT-KA-FLEET-020 (resolver error, wired) + IT-KA-FLEET-029 (unconfigured resolver, wired) — both entry points (`Investigate()`, `RunInteractiveTurn()`) proven for both degradation events. A third condition — resolver configured, succeeds, but resolves to an **empty** overlay — currently emits neither event (characterized by UT-KA-FLEET-028's "empty overlay on success" case as a tracked follow-up gap, not yet fixed).

---

## Security & Access Control

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `aiagent.auth.failure` | `EventTypeAuthFailure` | AU-2, AC-7 | Inbound request authentication fails (invalid/expired token) at the server auth middleware | `reason`, `source_ip` |
| `aiagent.auth.denied` | `EventTypeAuthDenied` | AC-3, AC-6 | Authenticated request denied by authorization policy (RBAC/SAR) | `user`, `endpoint`, `reason` |
| `aiagent.ratelimit.denied` | `EventTypeRateLimitDenied` | SC-5 | Request rejected by KA's rate limiter | `client_ip`, `limit` |
| `aiagent.secret.accessed` | `EventTypeSecretAccessed` | AC-6, AU-12, CC7.2 | KA's K8s resource resolver reads a core `Secret` (Get or List), regardless of outcome (Issue #1505). KA intentionally retains broad read RBAC on Secrets for investigation completeness; this is the detective control that compensates for not narrowing that RBAC | `secret_name`, `secret_namespace`, `verb` |

**Emitted from:** `internal/kubernautagent/server/audit_middleware.go`, `ratelimit.go`, `cmd/kubernautagent/toolregistry.go`

---

## Workflow Catalog Discovery (Issue #1677, DD-WORKFLOW-019)

Event category is `"workflow"` (via `WithEventCategory(WorkflowCatalogEventCategory)`), not the default `"aiagent"` — these describe the workflow-catalog domain, not KA's own investigation lifecycle. Relocated from Data Storage to KA in DD-WORKFLOW-019 (KA is the correct generator: it decides what to show the LLM, not DS). Event type/action string values are unchanged from their DS-generated predecessors, so existing audit-query consumers keyed on `event_type` are unaffected.

| Event Type | Constant | NIST/SOC2 Control | Trigger | Detail Fields (`Data`) |
|-----------|----------|-------------|---------|---------------|
| `workflow.catalog.actions_listed` | `EventTypeActionsListed` | AU-2, CC7.2 | Step 1: action types returned for a signal context (DD-WORKFLOW-014 v3.0) | `action_types`, `filters` |
| `workflow.catalog.workflows_listed` | `EventTypeWorkflowsListed` | AU-2, CC7.2 | Step 2: workflows returned for a selected action type | `action_type`, `workflow_count` |
| `workflow.catalog.workflow_retrieved` | `EventTypeWorkflowRetrieved` | AU-2, CC7.2 | Step 3: a single workflow's parameter schema retrieved (`ResourceType`/`ResourceID` = `"Workflow"`/workflow ID) | `workflow_id` |
| `workflow.catalog.selection_validated` | `EventTypeSelectionValidated` | AU-2, CC7.2, CC8.1 | Post-selection: re-validation query result for the LLM's chosen workflow | `workflow_id`, `valid` |

**Emitted from:** `internal/kubernautagent/tools/custom/discovery_audit.go`

---

## Backend & Delivery

Events are delivered through the `audit.AuditStore` interface (`internal/kubernautagent/audit/emitter.go`), typically via the fire-and-forget `StoreBestEffort` helper — an audit store failure is logged but never turns an otherwise-successful operation into a caller-visible error (ADR-038: audit must never block business logic).

`event.ClusterID`/`event.ActorID`/`event.ActorType` are back-filled from context (`audit.WithClusterID`, `audit.WithActor`) if not already set explicitly on the event — see `StoreBestEffort`/`inheritActorFromContext`.

Payloads are normalized to Data Storage's OpenAPI discriminated-union schema (`pkg/datastorage/ogen-client`) by `eventTypeToPayloadBuilder` (`internal/kubernautagent/audit/ds_store.go`) before being forwarded. Four event types (tracked in `ds_store_test.go`'s `skipPayloadCheck`) do not yet have dedicated OpenAPI discriminator variants and fall back to a generic, outer-fields-only shape (`event_type`, `cluster_id`, `correlation_id`, `actor`, `event_action`/`outcome`, no typed `Data`): `aiagent.alignment.grounding.request`, `aiagent.alignment.grounding.response`, `aiagent.fleet.overlay_failed`, and `aiagent.fleet.overlay_unavailable`.

---

## Known Gaps (tracked, not fixed by this catalog)

1. ~~**`aiagent.session.resumed` and `aiagent.interactive.k8s_call` are defined and DS-payload-wired but have no production emitter**~~ **CLOSED (2026-07).** Both now have production emitters: `aiagent.session.resumed` from `ReconstructionSpawner.emitSessionResumed` (`internal/kubernautagent/mcp/reconstruct.go`, called from `SpawnReconstruct` before the reconstructed `RunReconTurn`), and `aiagent.interactive.k8s_call` from `SelectWorkflowTool.emitInteractiveK8sCall` (`internal/kubernautagent/mcp/tools/select_workflow.go`, called from the `WithEnrichmentRunner` hook after `enrichment.Enrich()`). Both are wired into production via `cmd/kubernautagent/routes.go` (`SetAuditStore`/`WithSelectWorkflowAuditStore`) and covered by unit tests plus a real-`Enricher` IT (`select_workflow_k8scall_it_test.go`, IT-KA-898-K8SCALL-001/002) proving the full production call path, not just a mocked `EnrichmentRunner`. This is the same gap class as `apifrontend`'s documented-but-unemitted `a2a.stream_opened`/`a2a.stream_closed` (see that service's own `AUDIT_EVENT_CATALOG.md`), which remains open.
2. **Empty-overlay-on-success is not independently observable** — see the Fleet Federation section above. A `FleetOverlayResolver` that succeeds but returns zero tools currently behaves identically to a hub-local investigation from an audit perspective (no event), the same class of blind spot `overlay_unavailable` was created to close for the nil-resolver case. Characterized by a UT (`fleet_overlay_internal_test.go`) as a known follow-up, not fixed.

---

## Adding New Events

1. Define the `EventType`/`Action` constants in `internal/kubernautagent/audit/emitter.go` and add to `AllEventTypes`
2. Add the emit call at the appropriate production call site (never only in a test)
3. Add a payload builder to `ds_payloads.go` and register it in `ds_store.go`'s `eventTypeToPayloadBuilder` (or add to `skipPayloadCheck` in `ds_store_test.go` with a tracked follow-up if the OpenAPI schema variant doesn't exist yet)
4. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
5. Ensure a UT proves the emission decision and, for any new production entry point, an IT proves it fires through that entry point (Pyramid Invariant — UT proves logic, IT proves wiring)

---

*Last updated: 2026-08-01 | Issue #1818 reattach-on-race fix | Covers all 37 event types in `AllEventTypes` as of this date*
