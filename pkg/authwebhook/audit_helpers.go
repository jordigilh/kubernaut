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

	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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
