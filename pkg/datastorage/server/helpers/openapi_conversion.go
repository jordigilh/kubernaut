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

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// MaxEffectivenessResults caps the number of rows returned by the
// effectiveness query to prevent unbounded memory usage (PERF-H2).
const MaxEffectivenessResults = 10000

// ========================================
// OPENAPI TYPE CONVERSION HELPERS
// ========================================
// Authority: OpenAPI spec (api/openapi/data-storage-v1.yaml)
// BR-STORAGE-033: Generic audit write API
//
// These functions convert between OpenAPI-generated types
// (pkg/datastorage/client) and internal types (pkg/audit, pkg/datastorage/repository).
//
// Design Decision: Use OpenAPI types in REST handlers for type safety,
// then convert to internal types for business logic.
// ========================================

// ConvertAuditEventRequest converts OpenAPI request to internal audit event
//
// This performs the conversion from the REST API request type (OpenAPI-generated)
// to the internal audit event type used by the audit system.
//
// When authenticatedActorID is a human identity (non-ServiceAccount), server-side
// attribution overrides client body actor fields to prevent spoofing (SEC-S1 / AU-3).
//
// When authenticatedActorID is a Kubernetes ServiceAccount (system:serviceaccount:*),
// the client-submitted actor fields are preserved. Service accounts represent transport
// credentials, not logical actors — the calling service's self-declared identity
// (e.g., "signalprocessing-controller") is the meaningful audit actor.
//
// Parameters:
//   - req: OpenAPI-generated audit event request
//   - authenticatedActorID: trusted identity from auth middleware; SA identities preserve body fields
//
// Returns:
//   - *audit.AuditEvent: Internal audit event ready for storage
//   - error: Conversion error (e.g., invalid event_data JSON)
func ConvertAuditEventRequest(req ogenclient.AuditEventRequest, authenticatedActorID string) (*audit.AuditEvent, error) {
	// Convert event_data from map to JSON bytes
	eventDataJSON, err := json.Marshal(req.EventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", err)
	}

	actorType, actorID := resolveAuditEventActor(req, authenticatedActorID)
	resourceType, resourceID := resolveAuditEventResource(req)

	// Build internal audit event
	// D1/DF-H1: RetentionDays MUST be set before DLQ serialization so that
	// replay Validate() (which requires RetentionDays > 0) never rejects
	// events that succeeded on the synchronous DB path.
	event := &audit.AuditEvent{
		EventID:        uuid.New(), // Generate new UUID
		EventVersion:   req.Version,
		EventTimestamp: req.EventTimestamp,
		EventType:      req.EventType,
		EventCategory:  string(req.EventCategory), // Convert enum to string
		EventAction:    req.EventAction,
		EventOutcome:   string(req.EventOutcome), // Convert enum to string
		ActorType:      actorType,
		ActorID:        actorID,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		CorrelationID:  req.CorrelationID,
		EventData:      eventDataJSON,
		RetentionDays:  2555, // SOC 2 / ISO 27001 default (7 years)
	}

	applyOptionalAuditEventFields(event, req)

	return event, nil
}

// resolveAuditEventActor determines the actor_type/actor_id to persist.
//
// When authenticatedActorID is a human identity (non-ServiceAccount), server-side
// attribution overrides client body actor fields to prevent spoofing (SEC-S1 / AU-3).
//
// When authenticatedActorID is a Kubernetes ServiceAccount (system:serviceaccount:*),
// the client-submitted actor fields are preserved. Service accounts represent transport
// credentials, not logical actors — the calling service's self-declared identity
// (e.g., "signalprocessing-controller") is the meaningful audit actor.
func resolveAuditEventActor(req ogenclient.AuditEventRequest, authenticatedActorID string) (actorType, actorID string) {
	actorType = "service"
	actorID = string(req.EventCategory) + "-service"

	isServiceAccount := strings.HasPrefix(authenticatedActorID, "system:serviceaccount:")
	if authenticatedActorID != "" && !isServiceAccount {
		// SEC-S1 / AU-3: Override actor for human operators to prevent spoofing
		return "user", authenticatedActorID
	}

	if req.ActorType.IsSet() {
		actorType = req.ActorType.Value
	}
	if req.ActorID.IsSet() {
		actorID = req.ActorID.Value
	}
	return actorType, actorID
}

// resolveAuditEventResource determines the resource_type/resource_id to
// persist, defaulting to the event category and correlation_id respectively.
func resolveAuditEventResource(req ogenclient.AuditEventRequest) (resourceType, resourceID string) {
	resourceType = string(req.EventCategory)
	if req.ResourceType.IsSet() {
		resourceType = req.ResourceType.Value
	}

	resourceID = req.CorrelationID
	if req.ResourceID.IsSet() {
		resourceID = req.ResourceID.Value
	}
	return resourceType, resourceID
}

// applyOptionalAuditEventFields copies the optional (ogen OptNil) request
// fields onto event, leaving them unset (nil / zero value) when absent.
func applyOptionalAuditEventFields(event *audit.AuditEvent, req ogenclient.AuditEventRequest) {
	applyOptionalContextFields(event, req)
	applyOptionalComplianceFields(event, req)
}

// applyOptionalContextFields copies the optional contextual/identity fields
// (parent event linkage, location, severity, duration, actor IP).
func applyOptionalContextFields(event *audit.AuditEvent, req ogenclient.AuditEventRequest) {
	if req.ParentEventID.IsSet() {
		parentUUID := req.ParentEventID.Value
		event.ParentEventID = &parentUUID
	}
	if req.Namespace.IsSet() {
		event.Namespace = &req.Namespace.Value
	}
	if req.ClusterName.IsSet() {
		event.ClusterName = &req.ClusterName.Value
	}
	if req.Severity.IsSet() {
		event.Severity = &req.Severity.Value
	}
	if req.DurationMs.IsSet() {
		durationValue := req.DurationMs.Value
		event.DurationMs = &durationValue
	}
	if req.ActorIP.IsSet() {
		event.ActorIP = &req.ActorIP.Value
	}
}

// applyOptionalComplianceFields copies the optional compliance/error fields
// (parent event date, error details, retention, sensitivity).
func applyOptionalComplianceFields(event *audit.AuditEvent, req ogenclient.AuditEventRequest) {
	if req.ParentEventDate.IsSet() && !req.ParentEventDate.Null {
		t := req.ParentEventDate.Value
		event.ParentEventDate = &t
	}
	if req.ErrorCode.IsSet() && !req.ErrorCode.Null {
		v := req.ErrorCode.Value
		event.ErrorCode = &v
	}
	if req.ErrorMessage.IsSet() && !req.ErrorMessage.Null {
		v := req.ErrorMessage.Value
		event.ErrorMessage = &v
	}
	if req.RetentionDays.IsSet() && !req.RetentionDays.Null {
		event.RetentionDays = req.RetentionDays.Value
	}
	if req.IsSensitive.IsSet() && !req.IsSensitive.Null {
		event.IsSensitive = req.IsSensitive.Value
	}
}

// ConvertToRepositoryAuditEvent delegates to repository.ConvertFromAuditEvent.
// ARCH-C1: Canonical implementation moved to repository package so dlq can
// import it without an upward dependency on server/helpers.
func ConvertToRepositoryAuditEvent(event *audit.AuditEvent) (*repository.AuditEvent, error) {
	return repository.ConvertFromAuditEvent(event)
}

// ConvertToAuditEventResponse converts repository event to OpenAPI response
//
// This converts the database result to the OpenAPI response type that
// will be returned to the REST API client.
//
// Parameters:
//   - event: Repository audit event from database
//
// Returns:
//   - ogenclient.AuditEventResponse: OpenAPI response type
func ConvertToAuditEventResponse(event *repository.AuditEvent) ogenclient.AuditEventResponse {
	return ogenclient.AuditEventResponse{
		EventID:        event.EventID,
		EventTimestamp: event.EventTimestamp,
		Message:        fmt.Sprintf("audit event %s created successfully", event.EventID),
	}
}

// ValidateAuditEventRequest validates OpenAPI audit event request
//
// This performs CUSTOM business rule validation AFTER OpenAPI middleware has already
// validated required fields, types, and enums.
//
// OpenAPI middleware (pkg/datastorage/server/middleware/openapi.go) handles:
//   - Required fields (including empty strings via minLength: 1)
//   - Enum validation (event_outcome: success/failure/pending)
//   - Field lengths (via maxLength constraints)
//   - Type validation (string, int, timestamp formats)
//
// This function only handles CUSTOM BUSINESS VALIDATION not expressible in OpenAPI spec:
//   - Timestamp bounds (not in future, not too old)
//
// Parameters:
//   - req: OpenAPI audit event request
//
// Returns:
//   - error: Validation error if any business rules violated
func ValidateAuditEventRequest(req *ogenclient.AuditEventRequest) error {
	// BR-STORAGE-034: OpenAPI middleware now handles:
	// - ✅ Required fields (including empty strings via minLength: 1)
	// - ✅ Enum validation (event_outcome)
	// - ✅ Field lengths (via maxLength constraints)
	// - ✅ Type validation
	//
	// This function now only handles CUSTOM BUSINESS VALIDATION not expressible in OpenAPI spec

	// Gap 1.2 REFACTOR: Validate timestamp bounds (custom business rule)
	// Not handled by OpenAPI: Complex time-based rules (5 min future, 7 days past)
	if problem := ValidateTimestampBounds(req.EventTimestamp); problem != nil {
		return fmt.Errorf("invalid timestamp: %s", problem.Detail)
	}

	return nil
}
