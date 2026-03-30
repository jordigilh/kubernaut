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

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// emitAdmitAudit emits a success audit event for a CREATE or DELETE operation.
func (h *RemediationWorkflowHandler) emitAdmitAudit(ctx context.Context, req admission.Request, eventType, workflowID, resourceName string) {
	if h.auditStore == nil {
		return
	}

	resourceID := workflowID
	if resourceID == "" {
		resourceID = resourceName
	}

	event := buildAuditEnvelope(req, WebhookAuditOpts{
		EventType:    eventType,
		Category:     EventCategoryWorkflow,
		Action:       "admitted",
		Outcome:      api.AuditEventRequestEventOutcomeSuccess,
		ResourceKind: "RemediationWorkflow",
		ResourceID:   resourceID,
	})

	action := api.RemediationWorkflowWebhookAuditPayloadActionCreate
	ogenEventType := api.RemediationWorkflowWebhookAuditPayloadEventTypeRemediationworkflowAdmittedCreate
	wrapFn := api.NewAuditEventRequestEventDataRemediationworkflowAdmittedCreateAuditEventRequestEventData
	if eventType == EventTypeRWAdmittedDelete {
		action = api.RemediationWorkflowWebhookAuditPayloadActionDelete
		ogenEventType = api.RemediationWorkflowWebhookAuditPayloadEventTypeRemediationworkflowAdmittedDelete
		wrapFn = api.NewAuditEventRequestEventDataRemediationworkflowAdmittedDeleteAuditEventRequestEventData
	}

	payload := api.RemediationWorkflowWebhookAuditPayload{
		EventType:    ogenEventType,
		WorkflowName: resourceName,
		Action:       action,
	}
	if workflowID != "" {
		payload.WorkflowID.SetTo(workflowID)
	}
	event.EventData = wrapFn(payload)

	storeAuditBestEffort(ctx, h.auditStore, event, "rw-webhook", eventType)
}

// emitDeniedAudit emits a denied audit event when CREATE is rejected.
func (h *RemediationWorkflowHandler) emitDeniedAudit(ctx context.Context, req admission.Request, reason string) {
	if h.auditStore == nil {
		return
	}

	event := buildAuditEnvelope(req, WebhookAuditOpts{
		EventType:    EventTypeRWAdmittedDenied,
		Category:     EventCategoryWorkflow,
		Action:       "denied",
		Outcome:      api.AuditEventRequestEventOutcomeFailure,
		ResourceKind: "RemediationWorkflow",
		ResourceID:   req.Name,
	})

	payload := api.RemediationWorkflowWebhookAuditPayload{
		EventType:    api.RemediationWorkflowWebhookAuditPayloadEventTypeRemediationworkflowAdmittedDenied,
		WorkflowName: req.Name,
		Action:       api.RemediationWorkflowWebhookAuditPayloadActionDenied,
	}
	payload.DenialReason.SetTo(reason)
	event.EventData = api.NewAuditEventRequestEventDataRemediationworkflowAdmittedDeniedAuditEventRequestEventData(payload)

	storeAuditBestEffort(ctx, h.auditStore, event, "rw-webhook", EventTypeRWAdmittedDenied)
}
