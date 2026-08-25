/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package audit

import (
	"github.com/google/uuid"

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// GAP-11 (Issue #2285): Data Storage's own outbound CA-cert hot-reload had
// no audit trail parity with every other hot-reloadable component
// (SOC2 CC7.2 / FedRAMP CM-3, AU-2, AU-12). These are self-audit events,
// same pattern as NewRatelimitDeniedAuditEvent (BR-STORAGE-1505).
const (
	EventTypeConfigReloaded = "datastorage.config.reloaded"
	EventTypeConfigRejected = "datastorage.config.rejected"
	ActionConfigReloaded    = "reloaded"
	ActionConfigRejected    = "rejected"
)

// NewConfigReloadedAuditEvent creates a self-audit event recording that a
// hot-reloadable component (e.g. "ca_cert") was successfully reloaded.
func NewConfigReloadedAuditEvent(component string) *ogenclient.AuditEventRequest {
	eventID := uuid.New().String()

	auditEvent := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(auditEvent, EventTypeConfigReloaded)
	pkgaudit.SetEventCategory(auditEvent, EventCategorySecurity)
	pkgaudit.SetEventAction(auditEvent, ActionConfigReloaded)
	pkgaudit.SetEventOutcome(auditEvent, pkgaudit.OutcomeSuccess)
	pkgaudit.SetActor(auditEvent, "service", "datastorage")
	pkgaudit.SetResource(auditEvent, "Config", component)
	pkgaudit.SetCorrelationID(auditEvent, eventID)

	payload := ogenclient.DatastorageConfigReloadedPayload{
		EventType: ogenclient.DatastorageConfigReloadedPayloadEventTypeDatastorageConfigReloaded,
		Component: component,
	}
	auditEvent.EventData = ogenclient.NewDatastorageConfigReloadedPayloadAuditEventRequestEventData(payload)

	return auditEvent
}

// NewConfigRejectedAuditEvent creates a self-audit event recording that a
// hot-reloadable component's reload attempt was rejected, keeping the
// previous configuration in effect.
func NewConfigRejectedAuditEvent(component string, reloadErr error) *ogenclient.AuditEventRequest {
	eventID := uuid.New().String()

	auditEvent := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(auditEvent, EventTypeConfigRejected)
	pkgaudit.SetEventCategory(auditEvent, EventCategorySecurity)
	pkgaudit.SetEventAction(auditEvent, ActionConfigRejected)
	pkgaudit.SetEventOutcome(auditEvent, pkgaudit.OutcomeFailure)
	pkgaudit.SetActor(auditEvent, "service", "datastorage")
	pkgaudit.SetResource(auditEvent, "Config", component)
	pkgaudit.SetCorrelationID(auditEvent, eventID)

	reason := ""
	if reloadErr != nil {
		reason = reloadErr.Error()
	}
	payload := ogenclient.DatastorageConfigRejectedPayload{
		EventType:       ogenclient.DatastorageConfigRejectedPayloadEventTypeDatastorageConfigRejected,
		Component:       component,
		RejectionReason: reason,
	}
	auditEvent.EventData = ogenclient.NewDatastorageConfigRejectedPayloadAuditEventRequestEventData(payload)

	return auditEvent
}
