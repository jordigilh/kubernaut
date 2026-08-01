# Audit Event Catalog — Remediation Orchestrator (RO)

Authoritative reference for all structured audit events emitted by the `remediationorchestrator` controller.

**Source of truth:** `pkg/remediationorchestrator/audit/manager.go` (`EventType*`/`Action*` const blocks). The generated OpenAPI discriminator enum `RemediationOrchestratorAuditPayloadEventType` (`pkg/datastorage/ogen-client/oas_schemas_gen.go`) mirrors these 15 values 1:1 and is the closest thing to an `AllEventTypes` list — no such exported Go slice exists in `pkg/remediationorchestrator`.
**Payload mapping:** `pkg/remediationorchestrator/audit/manager.go` (`Build*Event` methods), `decision_mapping.go` (`ApprovalDecisionMapping`), `error_classifier.go` (`ClassifyError`), `actor_helpers.go` (`DetermineActor`)
**Emission call sites:** `internal/controller/remediationorchestrator/audit_events.go` and `remediation_approval_request.go` — the builders themselves are never called from within `pkg/remediationorchestrator/` in production; only the controller layer invokes them.
**Predecessor doc:** [DD-AUDIT-003](../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"Remediation Orchestrator" documents 12 events; code actually defines **15** (13 wired, 2 orphaned/dead), with 1 renamed string, 1 wrong outcome, and 1 event that doesn't exist as a distinct type. This catalog is the current, code-verified reference.

**Schema:** `EventCategory = "orchestration"` (`CategoryOrchestration`) for all events. Resource: `RemediationRequest`/`rr.Name` (except `orchestrator.ea.created`, which targets `EffectivenessAssessment`/`ea.Name`). Fleet provenance: `cluster_id` set on every event per DD-AUDIT-003 v2.2 (CC8.1).

---

## Lifecycle Events

| Event Type | Constant | Action | Outcome | Trigger | Data Fields |
|-----------|----------|--------|---------|---------|--------------|
| `orchestrator.lifecycle.created` | `EventTypeLifecycleCreated` | `created` | `success` | First reconcile of a new RemediationRequest, right after status initialization (BR-AUDIT-005 Gap #8) | `rr_name`, `namespace`, `timeout_config` (opt: global/processing/analyzing/executing durations) |
| `orchestrator.lifecycle.started` | `EventTypeLifecycleStarted` | `started` | **`pending`** | Emitted once, immediately after a new RR is first initialized | `rr_name`, `namespace` |
| `orchestrator.lifecycle.transitioned` | `EventTypeLifecycleTransitioned` | `transitioned` | `success` | Every successful RR phase transition (Pending→Processing→Analyzing→Executing→Verifying→Completed) | `from_phase`, `to_phase`, `namespace`, `rr_name` |
| `orchestrator.lifecycle.verifying_started` | `EventTypeLifecycleVerifyingStarted` | `verifying_started` | `pending` | RR transitions Executing→Verifying after successful WorkflowExecution completion (Issue #280) | `rr_name`, `namespace` |
| `orchestrator.lifecycle.verification_completed` | `EventTypeLifecycleVerificationCompleted` | `completed` | `success` | EA reaches a terminal phase; RR transitions Verifying→Completed | `rr_name`, `namespace`, `ea_name` (opt), `duration_ms` (opt) |
| `orchestrator.lifecycle.verification_timed_out` | `EventTypeLifecycleVerificationTimedOut` | `expired` | `failure` | `VerificationDeadline` (or safety-net timeout) expires before EA reaches terminal phase | `rr_name`, `namespace`, `ea_name` (opt), `duration_ms` (opt) |
| `orchestrator.lifecycle.completed` | `EventTypeLifecycleCompleted` | `completed` (all 4 variants) | varies — see below | RR reaches a terminal state; reused across 4 distinct call sites | See "Reused Event" note below |
| `orchestrator.ea.created` | `EventTypeEACreated` | `ea_created` | `success` | RO successfully creates an `EffectivenessAssessment` CRD (Issue #277, RO is source-of-truth for propagation-delay breakdown) | `rr_name`, `namespace`, `ea_name`, `hash_compute_delay`/`alert_check_delay`/`gitops_sync_delay`/`operator_reconcile_delay` (opt), `is_gitops_managed`, `is_crd`. **Resource is `EffectivenessAssessment`, not `RemediationRequest`.** |

### ⚠️ `orchestrator.lifecycle.completed` is reused by 4 production call sites with different semantics

| Variant | Outcome | Trigger |
|---|---|---|
| Normal success | `success` | RR reaches terminal `Completed` phase (any CRD-level outcome — carried separately in `crd_outcome`) |
| Failure | `failure` | RR reaches terminal `Failed` phase; includes standardized `error_details` (BR-AUDIT-005 Gap #7) |
| Timeout | `failure` | Global or per-phase timeout fires (hand-built inline, not via a `Build*` method) |
| Retention cleanup | `success` | Expired terminal RR about to be deleted per TTL (Issue #265); uses literal action string `"retention_cleanup"`, not `ActionCompleted` |

---

## Routing & Approval Events

| Event Type | Constant | Action | Outcome | Trigger | Data Fields |
|-----------|----------|--------|---------|---------|--------------|
| `orchestrator.routing.blocked` | `EventTypeRoutingBlocked` | `blocked` | `pending` | Routing Engine finds a blocking condition (cooldown, duplicate, resource-busy, consecutive failures, unmanaged resource, ineffective chain) → RR transitions to `Blocked` | `rr_name`, `namespace` only — see Known Gaps #4 |
| `orchestrator.approval.requested` | `EventTypeApprovalRequested` | `approval_requested` | `pending` | Selected workflow requires human approval (low confidence/high risk); RR transitions Analyzing→AwaitingApproval | `rar_name`, `rr_name`, `namespace`, `required_by`, `workflow_id`, `confidence` |
| `orchestrator.approval.approved` | `EventTypeApprovalApproved` | `approved` | `success` | `RemediationApprovalRequest.Status.Decision == Approved` | `rar_name`, `rr_name`, `namespace`, `decision` (opt). `decided_by`/`message` deliberately omitted (ADR-034 v1.7 Two-Event Pattern — captured by Auth Webhook) |
| `orchestrator.approval.rejected` | `EventTypeApprovalRejected` | `rejected` **or** `expired` | `failure` | `Decision == Rejected` (human), **or** `Decision == Expired` (RO auto-expires when the approval deadline passes with no decision) — see Known Gaps #1 | Same shape as `approved` |
| `remediation.workflow_created` | `EventTypeRemediationWorkflowCreated` | `workflow_created` | `success` | RO creates a `WorkflowExecution` CRD, from either the Analyzing or AwaitingApproval path (DD-EM-002, GAP-RO-1) | `rr_name`, `namespace`, `pre_remediation_spec_hash`, `target_resource`, `workflow_id`, `workflow_version`/`action_type`/`signal_type`/`signal_fingerprint` (opt). Note category prefix is `remediation.*`, not `orchestrator.*` |

**Emitted from:** `internal/controller/remediationorchestrator/audit_events.go` (`emitLifecycleStartedAudit`, `emitPhaseTransitionAudit`, `emitVerifyingStartedAudit`, `emitVerificationCompletedAudit`, `emitVerificationTimedOutAudit`, `emitCompletionAudit`, `emitFailureAudit`, `emitTimeoutAudit`, `emitRetentionCleanupAudit`, `emitRoutingBlockedAudit`, `emitApprovalRequestedAudit`, `emitRemediationCreatedAudit`, `emitEACreatedAudit`, `emitWorkflowCreatedAudit`), `remediation_approval_request.go` (`buildAndStoreApprovalAudit`), called from `reconcile_loop.go`, `terminal_transitions.go`, `verifying_handler.go`, `blocking.go`/`blocked_transitions.go`, `analyzing_handler.go`, `timeout_management.go`, `timeout_handling.go`, and `wfe_creation_helper.go`.

---

## Known Gaps (tracked, not fixed by this catalog)

1. **`orchestrator.approval.expired` does not exist as a distinct event type.** Expired approval decisions reuse the `orchestrator.approval.rejected` discriminator with `event_action=expired` (explicit code comment: "M-4: Expired uses PayloadEventType (orchestrator.approval.rejected) because there is no separate 'expired' discriminator in the OpenAPI spec"). Do not document it as a 13th, separately-queryable event type.
2. **`orchestrator.phase.transitioned` was renamed to `orchestrator.lifecycle.transitioned`.** The old name only survives in a code comment ("Replaces `orchestrator.phase.transitioned`"); only DD-AUDIT-003 still uses the stale string.
3. **`orchestrator.remediation.manual_review` is orphaned — defined and unit-tested, but has zero production callers.** `BuildManualReviewEvent` is fully implemented, but manual-review escalation in production is implemented purely via `NotificationRequest` creation (`blocked_transitions.go`, `terminal_transitions.go`), which never calls this builder. Per this repo's own wiring-verification standard, this is effectively a `CHECKPOINT W` violation (component with no production caller) — worth a follow-up fix (wire it, or remove it), not addressed by this doc-only catalog migration.
4. **`orchestrator.routing.blocked`'s rich payload fields are built but never attached to the emitted event.** The caller constructs a `RoutingBlockedData` struct with `block_reason`, `workflow_id`, `backoff_seconds`, etc., but `BuildRoutingBlockedEvent` only copies `rr_name`/`namespace` into the outgoing OpenAPI payload — the richer fields are silently dropped. Flagged here as a payload-mapping gap, not a documentation issue; a future fix should extend the payload schema before this catalog can document those fields as reliably present.
5. **`orchestrator.lifecycle.started`'s outcome is `pending`, not `success`.** The prior DD-AUDIT-003 baseline implied `success`.
6. **Dead constant, never emitted**: `EventTypeLifecycleFailed = "orchestrator.lifecycle.failed"` exists in both the Go const block and the OpenAPI enum, but no code path ever sets it — `BuildFailureEvent` uses `EventTypeLifecycleCompleted` (with `outcome=failure`) instead. Only referenced as a negative-check constant in `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go`.
7. **Minor dead code, out of scope for this catalog**: `ActionFailed` and `ActionUnblocked` (marked "future") constants in `manager.go` are defined but never referenced by any builder; `EventCategoryOrchestration` duplicates `CategoryOrchestration` (same `"orchestration"` value, two names in use across the package and its test/controller callers).

---

## Adding New Events

1. Define the `EventType`/`Action` constants in `pkg/remediationorchestrator/audit/manager.go`
2. Add a `Build*Event` method, wired at the production controller entry point (`internal/controller/remediationorchestrator/`), never only in a test
3. Add/extend the OpenAPI discriminator schema in `api/openapi/data-storage-v1.yaml` (`RemediationOrchestratorAuditPayloadEventType`)
4. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
5. Ensure a UT proves the emission decision and an IT proves it fires through the controller entry point (Pyramid Invariant) — and confirm the wiring with a grep for the `Build*Event` call site, per Known Gap #3 above

---

*Last updated: 2026-07-31 | QE readiness audit follow-up (DD-AUDIT-003 single-source-of-truth migration) | Covers all 15 event types in the `pkg/remediationorchestrator/audit/manager.go` const block (13 wired, 2 orphaned — see Known Gaps)*
