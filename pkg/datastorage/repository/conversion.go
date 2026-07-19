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

package repository

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/audit"
)

// ConvertFromAuditEvent converts a shared audit.AuditEvent into a
// repository.AuditEvent suitable for database persistence.
//
// ARCH-C1: This function lives in the repository package (leaf) so that
// both server/handlers and dlq can import it without creating an upward
// dependency from dlq -> server/helpers.
func ConvertFromAuditEvent(event *audit.AuditEvent) (*AuditEvent, error) {
	var eventDataMap map[string]interface{}
	if err := json.Unmarshal(event.EventData, &eventDataMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
	}

	fields := convertAuditEventOptionalFields(event)

	return &AuditEvent{
		EventID:           event.EventID,
		Version:           event.EventVersion,
		EventTimestamp:    event.EventTimestamp,
		EventDate:         DateOnly(event.EventTimestamp.Truncate(24 * time.Hour)),
		EventType:         event.EventType,
		EventCategory:     event.EventCategory,
		EventAction:       event.EventAction,
		EventOutcome:      event.EventOutcome,
		CorrelationID:     event.CorrelationID,
		ParentEventID:     event.ParentEventID,
		ParentEventDate:   event.ParentEventDate,
		ResourceType:      event.ResourceType,
		ResourceID:        event.ResourceID,
		ResourceNamespace: fields.resourceNamespace,
		ClusterID:         fields.clusterID,
		Severity:          fields.severity,
		DurationMs:        fields.durationMs,
		ErrorCode:         fields.errorCode,
		ErrorMessage:      fields.errorMessage,
		ActorID:           event.ActorID,
		ActorType:         event.ActorType,
		ActorIP:           ptrStringOrEmpty(event.ActorIP),
		EventData:         eventDataMap,
		RetentionDays:     fields.retentionDays,
		IsSensitive:       event.IsSensitive,
	}, nil
}

// convertAuditEventOptionalFieldsResult holds the defaulted/dereferenced
// values for audit.AuditEvent's optional (pointer) fields, computed by
// convertAuditEventOptionalFields.
type convertAuditEventOptionalFieldsResult struct {
	resourceNamespace string
	clusterID         string
	severity          string
	durationMs        int
	retentionDays     int
	errorCode         string
	errorMessage      string
}

// convertAuditEventOptionalFields dereferences event's optional pointer
// fields, applying the same defaults ConvertFromAuditEvent has always used
// (empty string, "info" severity, 2555-day/7-year retention floor).
// Extracted from ConvertFromAuditEvent (Wave 6 6f GREEN: funlen remediation)
// — pure code motion, no behavior change.
func convertAuditEventOptionalFields(event *audit.AuditEvent) convertAuditEventOptionalFieldsResult {
	retentionDays := event.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 2555
	}

	severity := "info"
	if event.Severity != nil {
		severity = *event.Severity
	}

	durationMs := 0
	if event.DurationMs != nil {
		durationMs = *event.DurationMs
	}

	return convertAuditEventOptionalFieldsResult{
		resourceNamespace: ptrStringOrEmpty(event.Namespace),
		clusterID:         ptrStringOrEmpty(event.ClusterID),
		severity:          severity,
		durationMs:        durationMs,
		retentionDays:     retentionDays,
		errorCode:         ptrStringOrEmpty(event.ErrorCode),
		errorMessage:      ptrStringOrEmpty(event.ErrorMessage),
	}
}

func ptrStringOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
