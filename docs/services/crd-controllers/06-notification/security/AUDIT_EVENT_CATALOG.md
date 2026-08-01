# Audit Event Catalog — Notification Service (NT)

Authoritative reference for all structured audit events emitted by the `notification` controller.

**Source of truth:** `pkg/notification/audit/manager.go` (`EventType*` const block, lines 56-65). No `AllEventTypes`-style exported slice exists.
**Payload mapping:** `pkg/notification/audit/manager.go` (`Create*Event` functions build `ogenclient.NotificationMessage*Payload` records)
**Predecessor doc:** [DD-AUDIT-003](../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"Notification Service" documents 5 events; 2 of those (`notification.message.delivered`, `notification.crd.updated`) do not exist in code, and a 3rd (`notification.request.cancelled`) is actually a different event owned by a different service (see Known Gaps). This catalog is the current, code-verified reference.

**Schema:** `EventCategory = "notification"`. Actor: `service`/`notification-controller`. Resource: `NotificationRequest`/name. CorrelationID: `Spec.RemediationRequestRef.Name`, falling back to the notification's own UID (DD-AUDIT-CORRELATION-002). Fleet provenance: `cluster_id` from `Spec.ClusterID` (DD-AUDIT-003 v2.2, CC8.1) on every event.

---

## Events

| Event Type | Constant | Action | Outcome | Trigger | Data Fields |
|-----------|----------|--------|---------|---------|--------------|
| `notification.message.sent` | `EventTypeMessageSent` | `sent` | `success` | A single delivery attempt to one channel succeeds | `NotificationID`, `Channel`, `Subject`, `Body`, `Priority`, `Type`, optional `Metadata` |
| `notification.message.failed` | `EventTypeMessageFailed` | `sent` (attempted) | `failure` | A single delivery attempt to one channel fails — either an actual delivery error or a pre-delivery circuit-breaker rejection | `NotificationID`, `Channel`, `Subject`, `Body`, `Priority`, `ErrorType` (currently hardcoded `"transient"` regardless of actual classification — see Known Gaps), optional `Metadata`/`Error` |
| `notification.message.acknowledged` | `EventTypeMessageAcknowledged` | `acknowledged` | `success` | ⚠️ **Not** a user/operator acknowledgment — fires when the NotificationRequest transitions to phase `Sent` (all channels succeeded) | `NotificationID`, `Subject`, `Priority`, optional `Metadata` |
| `notification.message.escalated` | `EventTypeMessageEscalated` | `escalated` | `success` (reflects the audit write, not notification outcome) | ⚠️ **Not** a priority-escalation feature — fires when the NotificationRequest transitions to phase `Failed` (all retries exhausted) | `NotificationID`, `Subject`, `Priority`, `Reason` (canned string: `"Escalated due to <priority> priority"`, not the actual delivery-failure reason), optional `Metadata` |

**Emitted from:** `pkg/notification/audit/manager.go` (`CreateMessageSentEvent`, `CreateMessageFailedEvent`, `CreateMessageAcknowledgedEvent`, `CreateMessageEscalatedEvent`), called from `internal/controller/notification/notificationrequest_controller.go` (`auditMessageSent`, `auditMessageFailed`, `auditMessageAcknowledged`, `auditMessageEscalated`), the first two wired as `DeliveryCallbacks` invoked from `pkg/notification/delivery/orchestrator.go`; the latter two called directly from `transitionToSent()`/`transitionToFailed()`. Each event is idempotency-guarded (`shouldEmitAuditEvent`/`markAuditEventEmitted`, NT-BUG-001) so it fires at most once per notification (per channel/attempt for `message.failed`).

**Related event in a different service:** `webhook.notification.cancelled` (NotificationRequest DELETE/cancellation) is emitted by **Auth Webhook**, not this service — see the [Auth Webhook catalog](../../../shared/authentication-webhook/security/AUDIT_EVENT_CATALOG.md). This is almost certainly what DD-AUDIT-003's `notification.request.cancelled` baseline entry was describing (see Known Gaps).

---

## Known Gaps (tracked, not fixed by this catalog)

1. **`notification.message.delivered` and `notification.crd.updated` (DD-AUDIT-003 baseline) do not exist anywhere in code.**
2. **`notification.request.cancelled` (DD-AUDIT-003 baseline) is the wrong event, wrong package, wrong service.** The real cancellation event is `webhook.notification.cancelled` (`EventTypeNotifCancelled`), owned by **Auth Webhook** (`pkg/authwebhook/notificationrequest_handler.go`), not the Notification service. DD-AUDIT-003's described `cancelled_by`/`cancellation_reason` fields roughly correspond to the real `CancelledBy` field, just under the wrong string and wrong owning catalog — see the Auth Webhook catalog for the authoritative definition.
3. **`notification.message.failed`'s `ErrorType` field is hardcoded to `"transient"`** regardless of the orchestrator's actual permanent/retryable classification — do not treat this field as meaningful/dynamic today.
4. **`notification.message.acknowledged`/`notification.message.escalated` names are misleading.** Despite their names, neither corresponds to an actual user acknowledgment or priority-escalation feature — they are phase-transition markers for all-channels-succeeded and all-retries-exhausted respectively. A future rename would require coordinating with existing audit-query consumers keyed on `event_type`.
5. **`docs/architecture/decisions/ADR-034-unified-audit-table-design.md`** independently lists a third, also-nonexistent variant, `notification.delivery.failed` — also stale, out of scope for this catalog but worth a follow-up pass.

---

## Adding New Events

1. Define the `EventType` constant in `pkg/notification/audit/manager.go`
2. Add a `Create*Event` function building the typed OpenAPI payload
3. Wire the emit call at the production reconciler entry point (`internal/controller/notification/notificationrequest_controller.go`) or the delivery orchestrator callback, never only in a test
4. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
5. Ensure a UT proves the emission decision and an IT proves it fires through the reconciler entry point (Pyramid Invariant)

---

*Last updated: 2026-07-31 | QE readiness audit follow-up (DD-AUDIT-003 single-source-of-truth migration) | Covers all 4 event types in the `pkg/notification/audit/manager.go` const block*
