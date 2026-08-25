/*
Copyright 2025 Jordi Gil.

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

package authwebhook

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Config hot-reload audit-trail parity event types (GAP-11, Issue #2285):
// mirrors gateway.config.{reloaded,rejected}'s component-based shape.
const (
	EventTypeConfigReloaded = "authwebhook.config.reloaded"
	EventTypeConfigRejected = "authwebhook.config.rejected"
	EventCategorySecurity   = "security"
	ActorIDAuthWebhook      = "authwebhook"
)

// WebhookAuditOpts holds the parameters for constructing a webhook audit event envelope.
// The envelope fields are identical across all AW handlers; only the payload differs.
type WebhookAuditOpts struct {
	EventType    string
	Category     string
	Action       string
	Outcome      api.AuditEventRequestEventOutcome
	ResourceKind string
	ResourceID   string
	LoggerName   string
}

// buildAuditEnvelope creates a fully populated audit event envelope from an admission
// request and options. The caller sets EventData on the returned event before storing.
func buildAuditEnvelope(req admission.Request, opts WebhookAuditOpts) *api.AuditEventRequest {
	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, opts.EventType)
	audit.SetEventCategory(event, opts.Category)
	audit.SetEventAction(event, opts.Action)
	audit.SetEventOutcome(event, opts.Outcome)
	audit.SetActor(event, "user", req.UserInfo.Username)
	audit.SetResource(event, opts.ResourceKind, opts.ResourceID)
	audit.SetCorrelationID(event, string(req.UID))
	audit.SetNamespace(event, req.Namespace)
	return event
}

// storeAuditBestEffort persists an audit event, logging any error without propagating.
func storeAuditBestEffort(ctx context.Context, store audit.AuditStore, event *api.AuditEventRequest, loggerName, eventType string) {
	if err := store.StoreAudit(ctx, event); err != nil {
		logger := ctrl.Log.WithName(loggerName)
		logger.Error(err, "Audit event storage failed (non-blocking)", "eventType", eventType)
	}
}

// RecordConfigReloaded records a hot-reloadable component's reload outcome
// (GAP-11, Issue #2285: CA hot-reload audit-trail parity). component
// identifies which hot-reloadable component fired (e.g. "ca_cert" for the
// TLS_CA_FILE watcher); reloadErr is nil on success (emits
// authwebhook.config.reloaded) or non-nil on rejection, in which case the
// previous configuration is kept in place (emits authwebhook.config.rejected).
//
// Mirrors Gateway's EmitConfigReloadAudit shape (pkg/gateway/audit_emission.go),
// the shipped reference implementation for this exact event pair. Unlike the
// admission-webhook audit envelope above, this event is not tied to an
// admission.Request (process-level config change, not a user action), so it
// is built directly rather than via buildAuditEnvelope.
func RecordConfigReloaded(ctx context.Context, store audit.AuditStore, component string, reloadErr error) {
	if store == nil {
		return
	}

	event := audit.NewAuditEventRequest()
	audit.SetEventCategory(event, EventCategorySecurity)
	audit.SetActor(event, "service", ActorIDAuthWebhook)
	audit.SetResource(event, "Config", component)
	audit.SetCorrelationID(event, fmt.Sprintf("config-reload-%s-%d", component, time.Now().UnixNano()))

	if reloadErr != nil {
		audit.SetEventType(event, EventTypeConfigRejected)
		audit.SetEventAction(event, "rejected")
		audit.SetEventOutcome(event, audit.OutcomeFailure)
		event.EventData = api.NewAuthwebhookConfigRejectedPayloadAuditEventRequestEventData(api.AuthwebhookConfigRejectedPayload{
			EventType:       api.AuthwebhookConfigRejectedPayloadEventTypeAuthwebhookConfigRejected,
			Component:       component,
			RejectionReason: reloadErr.Error(),
		})
	} else {
		audit.SetEventType(event, EventTypeConfigReloaded)
		audit.SetEventAction(event, "reloaded")
		audit.SetEventOutcome(event, audit.OutcomeSuccess)
		event.EventData = api.NewAuthwebhookConfigReloadedPayloadAuditEventRequestEventData(api.AuthwebhookConfigReloadedPayload{
			EventType: api.AuthwebhookConfigReloadedPayloadEventTypeAuthwebhookConfigReloaded,
			Component: component,
		})
	}

	storeAuditBestEffort(ctx, store, event, "config-reload-watcher", event.EventType)
}
