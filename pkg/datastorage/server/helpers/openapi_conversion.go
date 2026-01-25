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
	"time"

	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

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
// Parameters:
//   - req: OpenAPI-generated audit event request
//
// Returns:
//   - *audit.AuditEvent: Internal audit event ready for storage
//   - error: Conversion error (e.g., invalid event_data JSON)
func ConvertAuditEventRequest(req ogenclient.AuditEventRequest) (*audit.AuditEvent, error) {
	// Convert event_data from map to JSON bytes
	eventDataJSON, err := json.Marshal(req.EventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", err)
	}

	// Extract optional fields with defaults
	actorType := "service" // Default
	if req.ActorType.IsSet() {
		actorType = req.ActorType.Value
	}

	actorID := string(req.EventCategory) + "-service" // Default: category-service
	if req.ActorID.IsSet() {
		actorID = req.ActorID.Value
	}

	resourceType := string(req.EventCategory) // Default
	if req.ResourceType.IsSet() {
		resourceType = req.ResourceType.Value
	}

	resourceID := req.CorrelationID // Default
	if req.ResourceID.IsSet() {
		resourceID = req.ResourceID.Value
	}

	// Build internal audit event
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
	}

	// Optional fields (ogen OptNil types)
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
		durationValue := int(req.DurationMs.Value)
		event.DurationMs = &durationValue
	}

	return event, nil
}

// ConvertToRepositoryAuditEvent converts internal audit event to repository type
//
// This is a straightforward conversion between the pkg/audit type and
// pkg/datastorage/repository type.
//
// Parameters:
//   - event: Internal audit event
//
// Returns:
//   - *repository.AuditEvent: Repository audit event for database operations
//   - error: Conversion error (e.g., invalid event_data JSON)
func ConvertToRepositoryAuditEvent(event *audit.AuditEvent) (*repository.AuditEvent, error) {
	// Convert EventData from []byte to map[string]interface{}
	var eventDataMap map[string]interface{}
	if err := json.Unmarshal(event.EventData, &eventDataMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
	}

	// Extract pointer fields with defaults
	resourceNamespace := ""
	if event.Namespace != nil {
		resourceNamespace = *event.Namespace
	}

	clusterID := ""
	if event.ClusterName != nil {
		clusterID = *event.ClusterName
	}

	severity := "info" // Default severity
	if event.Severity != nil {
		severity = *event.Severity
	}

	durationMs := 0 // Default duration
	if event.DurationMs != nil {
		durationMs = *event.DurationMs
	}

	return &repository.AuditEvent{
		EventID:           event.EventID,
		Version:           event.EventVersion, // Map EventVersion to Version (DB column event_version)
		EventTimestamp:    event.EventTimestamp,
		EventDate:         repository.DateOnly(event.EventTimestamp.Truncate(24 * time.Hour)), // Generated column, truncated to date
		EventType:         event.EventType,
		EventCategory:     event.EventCategory,
		EventAction:       event.EventAction,
		EventOutcome:      event.EventOutcome,
		CorrelationID:     event.CorrelationID,
		ParentEventID:     event.ParentEventID,
		ResourceType:      event.ResourceType,
		ResourceID:        event.ResourceID,
		ResourceNamespace: resourceNamespace,
		ClusterID:         clusterID,
		Severity:          severity,
		DurationMs:        durationMs,
		ActorID:           event.ActorID,
		ActorType:         event.ActorType,
		EventData:         eventDataMap,
		RetentionDays:     2555,  // Default: 7 years
		IsSensitive:       false, // Default: not sensitive
	}, nil
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
