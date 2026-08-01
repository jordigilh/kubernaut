# Audit Event Catalog — Auth Webhook (AW)

Authoritative reference for all structured audit events emitted by the `authwebhook` service.

**Source of truth:** `pkg/authwebhook/types.go` (`EventType*` const block, lines 9-32). No `AllEventTypes`-style exported slice exists — the flat const block is the definitive enumeration.
**Payload mapping:** `pkg/authwebhook/actiontype_audit.go`, `remediationworkflow_audit.go`, `startup_reconciler.go`, `workflowexecution_handler.go`, `remediationrequest_handler.go`, `remediationapprovalrequest_handler.go` (+ `audit_payload_builder.go`), `notificationrequest_handler.go`
**Predecessor doc:** [DD-AUDIT-003](../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"Auth Webhook Service" documents 11 events; **AW actually emits 15** — 4 admission-webhook event types for WorkflowExecution, RemediationRequest, RemediationApprovalRequest, and NotificationRequest are undocumented there. This catalog is the current, code-verified reference.

---

## ActionType CRD Admission (`EventCategoryActionType = "actiontype"`, ADR-059, BR-WORKFLOW-007)

| Event Type | Constant | Action | Trigger | Data Fields |
|-----------|----------|--------|---------|--------------|
| `actiontype.admitted.create` | `EventTypeATAdmittedCreate` | `admitted` | ActionType CREATE admitted (no denial path exists for CREATE — see Known Gaps) | `ActionTypeName`, `CrdName`/`CrdNamespace`, `Action=Create`, `PreviouslyExisted` (opt), `CatalogStatus="Active"` (opt) |
| `actiontype.admitted.update` | `EventTypeATAdmittedUpdate` | `admitted` | UPDATE admitted, only when `spec.description` actually changed and name unchanged | Same shape, `Action=Update` |
| `actiontype.admitted.delete` | `EventTypeATAdmittedDelete` | `admitted` | DELETE admitted (no dependent RemediationWorkflows found) | Same shape, `Action=Delete`, `CatalogStatus="Disabled"` |
| `actiontype.denied.update` | `EventTypeATDeniedUpdate` | `denied` | UPDATE denied because `spec.name` is immutable | `ActionTypeName`, `CrdName`/`CrdNamespace`, `Action=Denied`, `DenialReason`, `DenialOperation="UPDATE"` |
| `actiontype.denied.delete` | `EventTypeATDeniedDelete` | `denied` | DELETE denied — dependents-list K8s call failed, or ≥1 active RemediationWorkflow depends on it | Same shape, `DenialOperation="DELETE"` |

**Emitted from:** `pkg/authwebhook/actiontype_handler.go`, emission functions in `pkg/authwebhook/actiontype_audit.go`.

## RemediationWorkflow CRD Admission (`EventCategoryWorkflow = "workflow"`, ADR-058)

| Event Type | Constant | Action | Trigger | Data Fields |
|-----------|----------|--------|---------|--------------|
| `remediationworkflow.admitted.create` | `EventTypeRWAdmittedCreate` | `admitted` | CREATE admitted with a content hash differing from any prior state | `WorkflowName`, `Action=Create`, `WorkflowID` (opt), `WorkflowContent` (opt, full structured spec), `ContentHash` (opt) |
| `remediationworkflow.admitted.update` | `EventTypeRWAdmittedUpdate` | `admitted` | UPDATE admitted with an actual content change (metadata-only churn suppressed, Issue #1759) | Same shape, `Action=Update` |
| `remediationworkflow.admitted.delete` | `EventTypeRWAdmittedDelete` | `admitted` | DELETE admitted | `Action=Delete` (no `WorkflowContent`/`ContentHash` — Change 2) |
| `remediationworkflow.admitted.denied` | `EventTypeRWAdmittedDenied` | `denied` | Single event for all CREATE/UPDATE denial reasons (unmarshal failure, auth failure, ActionType missing, content-integrity violation) | `WorkflowName`, `Action=Denied`, `DenialReason`, best-effort `WorkflowContent`/`ContentHash` when spec parsed |

**Emitted from:** `pkg/authwebhook/remediationworkflow_handler.go`, emission functions in `pkg/authwebhook/remediationworkflow_audit.go`.

## Startup Reconciler (`EventCategoryWorkflow = "workflow"`, Issue #1246)

| Event Type | Constant | Action | Trigger | Data Fields |
|-----------|----------|--------|---------|--------------|
| `authwebhook.workflow.registration_failed` | `EventTypeRWRegistrationFailed` | `registration_failed` | Startup reconciler fails to compute/patch a RemediationWorkflow's status during PVC-wipe recovery | `WorkflowName`, `Reason` (typed CRD condition reason), `Message`, `Namespace` (opt). Actor: `system:authwebhook-startup`, Outcome: `failure`, `Severity="high"` |

**Emitted from:** `pkg/authwebhook/startup_reconciler.go` (`markWorkflowFailed` → `emitRegistrationFailedAudit`).

## Additional Admission Handlers (undocumented in DD-AUDIT-003)

| Event Type | Constant | Category | Action | Trigger | Data Fields |
|-----------|----------|----------|--------|---------|--------------|
| `workflowexecution.block.cleared` | `EventTypeBlockCleared` | `workflowexecution` | `block_cleared` | A WFE admission request sets `status.blockClearance` for the first time, with a valid ≥10-word clearance reason (SOC2 CC7.4, BR-WE-013) | `WorkflowName`, `ClearReason`, `ClearedAt`, `PreviousState=Blocked`, `NewState=Running`. Actor: `user/authCtx.Username`; CorrelationID: parent RR name |
| `webhook.remediationrequest.timeout_modified` | `EventTypeTimeoutModified` | `orchestration` (cross-package: `roaudit.EventCategoryOrchestration`) | `timeout_modified` | A RemediationRequest status UPDATE changes `status.timeoutConfig` (operator `kubectl edit` mid-remediation) | `RrName`, `Namespace`, `ModifiedBy`, `ModifiedAt`, `OldTimeoutConfig`/`NewTimeoutConfig` (opt, per-phase duration strings) |
| `webhook.remediationapprovalrequest.decided` | `EventTypeRARDecided` | `approval` | `approval_decided` | A RemediationApprovalRequest status update sets `status.decision` for the first time (Approved/Rejected/Expired) — one event type covers all three decision values (ADR-034 v1.7) | `RequestName`, `Decision`, `DecidedAt`, `DecisionMessage`, `AiAnalysisRef`, `DelegatedUser`/`DelegatedVia` (opt, AF service-account delegation). Actor: `service` if delegated else `user`; CorrelationID: parent RR name |
| `webhook.notification.cancelled` | `EventTypeNotifCancelled` | `notification` | `deleted` | A NotificationRequest DELETE (cancellation — spec is immutable per DD-NOT-005) | `NotificationID`, `Type`, `Priority`, `CancelledBy`, `UserUID`, `UserGroups`, `Action=NotificationCancelled`. **Fail-closed**: DELETE is denied if `StoreAudit` fails (ADR-032 §2 / FedRAMP AC-6) — the one AW event that blocks its triggering operation on audit-write failure |

**Emitted from:** `pkg/authwebhook/workflowexecution_handler.go`, `remediationrequest_handler.go`, `remediationapprovalrequest_handler.go` (+ `audit_payload_builder.go`), `notificationrequest_handler.go` (`NotificationRequestDeleteHandler`). All 4 confirmed wired in `cmd/authwebhook/main.go`.

---

## Known Gaps (tracked, not fixed by this catalog)

1. **`actiontype.denied.create` is dormant/unreachable in production.** It's fully defined and wired in the resolver/switch (`actiontype_audit.go`), but `handleCreate` never calls it — its two failure branches (unmarshal error, auth error) return `admission.Denied(...)` directly with no audit emission. Documented as if it fires on CREATE denial; it currently cannot.
2. **`webhook.notification.cancelled` has two divergent, non-test emission code paths.** `NotificationRequestDeleteHandler` (`notificationrequest_handler.go`) is the one wired into `cmd/authwebhook/main.go` and is authoritative for the payload shape above (`NotificationID`, `Type`, `CancelledBy`, etc., fail-closed). A second implementation, `NotificationRequestValidator.ValidateDelete` (`notificationrequest_validator.go`), emits the *same event-type string* with a **different payload shape** (`NotificationName`, `NotificationType`, `FinalStatus`, `DeliveryChannels` — no `CancelledBy`/`UserUID`/`UserGroups`) and fails open. It is never registered in `cmd/authwebhook/main.go` — only used in `test/integration/authwebhook/suite_test.go` and unit tests. `docs/tests/668/TEST_PLAN.md` independently confirms this IT/production handler divergence. Do not use `notificationrequest_validator.go`'s field names when integrating against this event.
3. **Inconsistent event-type naming convention.** `workflowexecution.block.cleared` uses a dotted `<category>.<subject>.<verb>` form with no prefix; `webhook.remediationrequest.timeout_modified`, `webhook.remediationapprovalrequest.decided`, and `webhook.notification.cancelled` all use a `webhook.<resource>.<verb>` prefix instead. Not a functional bug, but worth normalizing if a new admission-handler event is ever added.

---

## Adding New Events

1. Define the `EventType` constant in `pkg/authwebhook/types.go`
2. Add the payload builder and emission call in the relevant handler file, wired at the production admission-webhook entry point in `cmd/authwebhook/main.go` — never only in a test
3. Add/extend the OpenAPI discriminator schema in `api/openapi/data-storage-v1.yaml`
4. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
5. Ensure a UT proves the emission decision and an IT proves it fires through the production admission-webhook entry point (Pyramid Invariant)

---

*Last updated: 2026-07-31 | QE readiness audit follow-up (DD-AUDIT-003 single-source-of-truth migration) | Covers all 15 event types in the `pkg/authwebhook/types.go` const block*
