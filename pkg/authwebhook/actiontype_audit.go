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

// emitATAdmitAudit emits a success audit event for an ActionType admission operation.
// BR-WORKFLOW-007, ADR-059: ActionType CRD admission audit trail.
func (h *ActionTypeHandler) emitATAdmitAudit(
	ctx context.Context,
	req admission.Request,
	eventType string,
	actionTypeName string,
	previouslyExisted bool,
	catalogStatus string,
) {
	if h.auditStore == nil {
		return
	}

	event := buildAuditEnvelope(req, WebhookAuditOpts{
		EventType:    eventType,
		Category:     EventCategoryActionType,
		Action:       "admitted",
		Outcome:      api.AuditEventRequestEventOutcomeSuccess,
		ResourceKind: "ActionType",
		ResourceID:   req.Name,
	})

	ogenEventType, action, wrapFn := resolveATAdmitTypes(eventType)

	payload := api.ActionTypeWebhookAuditPayload{
		EventType:      ogenEventType,
		ActionTypeName: actionTypeName,
		CrdName:        req.Name,
		CrdNamespace:   req.Namespace,
		Action:         action,
	}
	payload.PreviouslyExisted.SetTo(previouslyExisted)
	if catalogStatus != "" {
		payload.CatalogStatus.SetTo(catalogStatus)
	}
	event.EventData = wrapFn(payload)

	storeAuditBestEffort(ctx, h.auditStore, event, "at-webhook", eventType)
}

// emitATDeniedAudit emits a denied audit event when an ActionType operation is rejected.
func (h *ActionTypeHandler) emitATDeniedAudit(
	ctx context.Context,
	req admission.Request,
	reason string,
	operation string,
) {
	if h.auditStore == nil {
		return
	}

	var eventType string
	switch operation {
	case "CREATE":
		eventType = EventTypeATDeniedCreate
	case "UPDATE":
		eventType = EventTypeATDeniedUpdate
	case "DELETE":
		eventType = EventTypeATDeniedDelete
	default:
		eventType = EventTypeATDeniedCreate
	}

	event := buildAuditEnvelope(req, WebhookAuditOpts{
		EventType:    eventType,
		Category:     EventCategoryActionType,
		Action:       "denied",
		Outcome:      api.AuditEventRequestEventOutcomeFailure,
		ResourceKind: "ActionType",
		ResourceID:   req.Name,
	})

	_, _, wrapFn := resolveATDeniedTypes(eventType)

	payload := api.ActionTypeWebhookAuditPayload{
		EventType:      resolveATDeniedOgenEventType(eventType),
		ActionTypeName: req.Name,
		CrdName:        req.Name,
		CrdNamespace:   req.Namespace,
		Action:         api.ActionTypeWebhookAuditPayloadActionDenied,
	}
	payload.DenialReason.SetTo(reason)
	payload.DenialOperation.SetTo(operation)
	event.EventData = wrapFn(payload)

	storeAuditBestEffort(ctx, h.auditStore, event, "at-webhook", eventType)
}

type atWrapFn = func(api.ActionTypeWebhookAuditPayload) api.AuditEventRequestEventData

func resolveATAdmitTypes(eventType string) (api.ActionTypeWebhookAuditPayloadEventType, api.ActionTypeWebhookAuditPayloadAction, atWrapFn) {
	switch eventType {
	case EventTypeATAdmittedCreate:
		return api.ActionTypeWebhookAuditPayloadEventTypeActiontypeAdmittedCreate,
			api.ActionTypeWebhookAuditPayloadActionCreate,
			api.NewAuditEventRequestEventDataActiontypeAdmittedCreateAuditEventRequestEventData
	case EventTypeATAdmittedUpdate:
		return api.ActionTypeWebhookAuditPayloadEventTypeActiontypeAdmittedUpdate,
			api.ActionTypeWebhookAuditPayloadActionUpdate,
			api.NewAuditEventRequestEventDataActiontypeAdmittedUpdateAuditEventRequestEventData
	case EventTypeATAdmittedDelete:
		return api.ActionTypeWebhookAuditPayloadEventTypeActiontypeAdmittedDelete,
			api.ActionTypeWebhookAuditPayloadActionDelete,
			api.NewAuditEventRequestEventDataActiontypeAdmittedDeleteAuditEventRequestEventData
	default:
		return api.ActionTypeWebhookAuditPayloadEventTypeActiontypeAdmittedCreate,
			api.ActionTypeWebhookAuditPayloadActionCreate,
			api.NewAuditEventRequestEventDataActiontypeAdmittedCreateAuditEventRequestEventData
	}
}

func resolveATDeniedTypes(eventType string) (api.ActionTypeWebhookAuditPayloadEventType, api.ActionTypeWebhookAuditPayloadAction, atWrapFn) {
	switch eventType {
	case EventTypeATDeniedCreate:
		return api.ActionTypeWebhookAuditPayloadEventTypeActiontypeDeniedCreate,
			api.ActionTypeWebhookAuditPayloadActionDenied,
			api.NewAuditEventRequestEventDataActiontypeDeniedCreateAuditEventRequestEventData
	case EventTypeATDeniedUpdate:
		return api.ActionTypeWebhookAuditPayloadEventTypeActiontypeDeniedUpdate,
			api.ActionTypeWebhookAuditPayloadActionDenied,
			api.NewAuditEventRequestEventDataActiontypeDeniedUpdateAuditEventRequestEventData
	case EventTypeATDeniedDelete:
		return api.ActionTypeWebhookAuditPayloadEventTypeActiontypeDeniedDelete,
			api.ActionTypeWebhookAuditPayloadActionDenied,
			api.NewAuditEventRequestEventDataActiontypeDeniedDeleteAuditEventRequestEventData
	default:
		return api.ActionTypeWebhookAuditPayloadEventTypeActiontypeDeniedCreate,
			api.ActionTypeWebhookAuditPayloadActionDenied,
			api.NewAuditEventRequestEventDataActiontypeDeniedCreateAuditEventRequestEventData
	}
}

func resolveATDeniedOgenEventType(eventType string) api.ActionTypeWebhookAuditPayloadEventType {
	et, _, _ := resolveATDeniedTypes(eventType)
	return et
}
