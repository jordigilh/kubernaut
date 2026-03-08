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
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// emitAdmitAudit emits a success audit event for a CREATE or DELETE operation.
func (h *RemediationWorkflowHandler) emitAdmitAudit(ctx context.Context, req admission.Request, eventType, workflowID, resourceName string) {
	if h.auditStore == nil {
		return
	}

	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, eventType)
	audit.SetEventCategory(event, EventCategoryWebhook)
	audit.SetEventAction(event, "admitted")
	audit.SetEventOutcome(event, ogenclient.AuditEventRequestEventOutcomeSuccess)
	audit.SetActor(event, "user", req.UserInfo.Username)
	resourceID := workflowID
	if resourceID == "" {
		resourceID = resourceName
	}
	audit.SetResource(event, "RemediationWorkflow", resourceID)
	audit.SetCorrelationID(event, string(req.UID))
	audit.SetNamespace(event, req.Namespace)

	if err := h.auditStore.StoreAudit(ctx, event); err != nil {
		logger := ctrl.Log.WithName("rw-webhook")
		logger.Error(err, "Audit event storage failed (non-blocking)",
			"eventType", eventType)
	}
}

// emitDeniedAudit emits a denied audit event when CREATE is rejected.
func (h *RemediationWorkflowHandler) emitDeniedAudit(ctx context.Context, req admission.Request, reason string) {
	if h.auditStore == nil {
		return
	}

	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, EventTypeRWAdmittedDenied)
	audit.SetEventCategory(event, EventCategoryWebhook)
	audit.SetEventAction(event, "denied")
	audit.SetEventOutcome(event, ogenclient.AuditEventRequestEventOutcomeFailure)
	audit.SetActor(event, "user", req.UserInfo.Username)
	audit.SetResource(event, "RemediationWorkflow", req.Name)
	audit.SetCorrelationID(event, string(req.UID))
	audit.SetNamespace(event, req.Namespace)

	if err := h.auditStore.StoreAudit(ctx, event); err != nil {
		logger := ctrl.Log.WithName("rw-webhook")
		logger.Error(err, "Denied audit event storage failed (non-blocking)",
			"reason", reason)
	}
}
