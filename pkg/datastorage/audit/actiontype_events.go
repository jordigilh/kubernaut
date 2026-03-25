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

package audit

import (
	"fmt"
	"time"

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ActionType catalog event type constants (BR-WORKFLOW-007, ADR-059)
const (
	EventTypeActionTypeCreated       = "datastorage.actiontype.created"
	EventTypeActionTypeUpdated       = "datastorage.actiontype.updated"
	EventTypeActionTypeDisabled      = "datastorage.actiontype.disabled"
	EventTypeActionTypeReenabled     = "datastorage.actiontype.reenabled"
	EventTypeActionTypeDisableDenied = "datastorage.actiontype.disable_denied"
)

const (
	EventCategoryActionType = "actiontype"
	ActionDisable           = "disable"
	ActionReenable          = "reenable"
	ActionDisableDenied     = "disable_denied"
)

// NewActionTypeCreatedAuditEvent creates an audit event for action type creation.
func NewActionTypeCreatedAuditEvent(
	actionType string,
	desc ogenclient.ActionTypeDescriptionPayload,
	registeredBy string,
	wasReenabled bool,
) (*ogenclient.AuditEventRequest, error) {
	event := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(event, EventTypeActionTypeCreated)
	pkgaudit.SetEventCategory(event, EventCategoryActionType)
	pkgaudit.SetEventAction(event, ActionCreate)
	pkgaudit.SetEventOutcome(event, ogenclient.AuditEventRequestEventOutcomeSuccess)
	pkgaudit.SetActor(event, "service", registeredBy)
	pkgaudit.SetResource(event, "ActionType", actionType)

	payload := ogenclient.ActionTypeCatalogCreatedPayload{
		EventType:    ogenclient.ActionTypeCatalogCreatedPayloadEventTypeDatastorageActiontypeCreated,
		ActionType:   actionType,
		Description:  desc,
		RegisteredBy: registeredBy,
		WasReenabled: wasReenabled,
	}
	event.EventData = ogenclient.NewActionTypeCatalogCreatedPayloadAuditEventRequestEventData(payload)

	return event, nil
}

// NewActionTypeUpdatedAuditEvent creates an audit event for description update (SOC2: old+new).
func NewActionTypeUpdatedAuditEvent(
	actionType string,
	oldDesc, newDesc ogenclient.ActionTypeDescriptionPayload,
	updatedBy string,
	updatedFields []string,
) (*ogenclient.AuditEventRequest, error) {
	event := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(event, EventTypeActionTypeUpdated)
	pkgaudit.SetEventCategory(event, EventCategoryActionType)
	pkgaudit.SetEventAction(event, ActionUpdate)
	pkgaudit.SetEventOutcome(event, ogenclient.AuditEventRequestEventOutcomeSuccess)
	pkgaudit.SetActor(event, "service", updatedBy)
	pkgaudit.SetResource(event, "ActionType", actionType)

	payload := ogenclient.ActionTypeCatalogUpdatedPayload{
		EventType:      ogenclient.ActionTypeCatalogUpdatedPayloadEventTypeDatastorageActiontypeUpdated,
		ActionType:     actionType,
		OldDescription: oldDesc,
		NewDescription: newDesc,
		UpdatedBy:      updatedBy,
		UpdatedFields:  updatedFields,
	}
	event.EventData = ogenclient.NewActionTypeCatalogUpdatedPayloadAuditEventRequestEventData(payload)

	return event, nil
}

// NewActionTypeDisabledAuditEvent creates an audit event for action type soft-disable.
func NewActionTypeDisabledAuditEvent(
	actionType, disabledBy string,
	disabledAt time.Time,
) (*ogenclient.AuditEventRequest, error) {
	event := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(event, EventTypeActionTypeDisabled)
	pkgaudit.SetEventCategory(event, EventCategoryActionType)
	pkgaudit.SetEventAction(event, ActionDisable)
	pkgaudit.SetEventOutcome(event, ogenclient.AuditEventRequestEventOutcomeSuccess)
	pkgaudit.SetActor(event, "service", disabledBy)
	pkgaudit.SetResource(event, "ActionType", actionType)

	payload := ogenclient.ActionTypeCatalogDisabledPayload{
		EventType:  ogenclient.ActionTypeCatalogDisabledPayloadEventTypeDatastorageActiontypeDisabled,
		ActionType: actionType,
		DisabledBy: disabledBy,
		DisabledAt: disabledAt,
	}
	event.EventData = ogenclient.NewActionTypeCatalogDisabledPayloadAuditEventRequestEventData(payload)

	return event, nil
}

// NewActionTypeReenabledAuditEvent creates an audit event for re-enabling a disabled action type.
func NewActionTypeReenabledAuditEvent(
	actionType, reenabledBy string,
	prevDisabledAt time.Time,
	prevDisabledBy string,
) (*ogenclient.AuditEventRequest, error) {
	event := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(event, EventTypeActionTypeReenabled)
	pkgaudit.SetEventCategory(event, EventCategoryActionType)
	pkgaudit.SetEventAction(event, ActionReenable)
	pkgaudit.SetEventOutcome(event, ogenclient.AuditEventRequestEventOutcomeSuccess)
	pkgaudit.SetActor(event, "service", reenabledBy)
	pkgaudit.SetResource(event, "ActionType", actionType)

	payload := ogenclient.ActionTypeCatalogReenabledPayload{
		EventType:     ogenclient.ActionTypeCatalogReenabledPayloadEventTypeDatastorageActiontypeReenabled,
		ActionType:    actionType,
		ReenabledBy:   reenabledBy,
		PreviousState: ogenclient.ActionTypeCatalogReenabledPayloadPreviousStateDisabled,
		DisabledAt:    prevDisabledAt,
		DisabledBy:    prevDisabledBy,
	}
	event.EventData = ogenclient.NewActionTypeCatalogReenabledPayloadAuditEventRequestEventData(payload)

	return event, nil
}

// NewActionTypeDisableDeniedAuditEvent creates an audit event when disable is denied.
func NewActionTypeDisableDeniedAuditEvent(
	actionType, requestedBy string,
	count int,
	workflowNames []string,
) (*ogenclient.AuditEventRequest, error) {
	event := pkgaudit.NewAuditEventRequest()
	pkgaudit.SetEventType(event, EventTypeActionTypeDisableDenied)
	pkgaudit.SetEventCategory(event, EventCategoryActionType)
	pkgaudit.SetEventAction(event, ActionDisableDenied)
	pkgaudit.SetEventOutcome(event, ogenclient.AuditEventRequestEventOutcomeFailure)
	pkgaudit.SetActor(event, "service", requestedBy)
	pkgaudit.SetResource(event, "ActionType", actionType)

	payload := ogenclient.ActionTypeCatalogDisableDeniedPayload{
		EventType:              ogenclient.ActionTypeCatalogDisableDeniedPayloadEventTypeDatastorageActiontypeDisableDenied,
		ActionType:             actionType,
		DeniedReason:           fmt.Sprintf("%d active workflows reference this action type", count),
		DependentWorkflowCount: count,
		DependentWorkflows:     workflowNames,
		RequestedBy:            requestedBy,
	}
	event.EventData = ogenclient.NewActionTypeCatalogDisableDeniedPayloadAuditEventRequestEventData(payload)

	return event, nil
}
