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

// BR-STORAGE-1505 (GAP-09, Issue #1505): Data Storage HTTP API self-audited
// rate-limit denial.
const (
	EventTypeRatelimitDenied = "datastorage.ratelimit.denied"
	EventCategorySecurity    = "security"
	ActionDenied             = "denied"
)

// NewRatelimitDeniedAuditEvent creates a self-audit event recording that the
// Data Storage HTTP API rejected a request due to per-IP rate limiting
// (BR-STORAGE-1505 / FedRAMP AU-12: audit generation for security-relevant events).
//
// sourceIP, path, and method are best-effort context and may be empty.
func NewRatelimitDeniedAuditEvent(sourceIP, path, method string) *ogenclient.AuditEventRequest {
	eventID := uuid.New().String()

	auditEvent := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(auditEvent, EventTypeRatelimitDenied)
	pkgaudit.SetEventCategory(auditEvent, EventCategorySecurity)
	pkgaudit.SetEventAction(auditEvent, ActionDenied)
	pkgaudit.SetEventOutcome(auditEvent, pkgaudit.OutcomeFailure)
	pkgaudit.SetActor(auditEvent, "external", firstNonEmpty(sourceIP, "unknown"))
	pkgaudit.SetResource(auditEvent, "HTTPRequest", eventID)
	// No upstream correlation ID is available for a pre-auth rate-limit denial
	// (the request never reached a handler); use the event's own ID so the
	// event remains independently queryable per ADR-034.
	pkgaudit.SetCorrelationID(auditEvent, eventID)

	payload := ogenclient.DatastorageRatelimitDeniedPayload{
		EventType: ogenclient.DatastorageRatelimitDeniedPayloadEventTypeDatastorageRatelimitDenied,
		EventID:   eventID,
	}
	if sourceIP != "" {
		payload.SourceIP.SetTo(sourceIP)
	}
	if path != "" {
		payload.Path.SetTo(path)
	}
	if method != "" {
		payload.Method.SetTo(method)
	}
	auditEvent.EventData = ogenclient.NewDatastorageRatelimitDeniedPayloadAuditEventRequestEventData(payload)

	return auditEvent
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
