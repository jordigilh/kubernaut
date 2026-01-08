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

// Package testutil provides test utilities for Kubernaut services.
package testutil

import (
	"fmt"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	. "github.com/onsi/gomega"
)

// ExpectedAuditEvent defines expected values for an audit event.
// Fields left empty (zero value) will not be validated.
// Based on dsgen.AuditEvent schema from pkg/datastorage/client/generated.go
type ExpectedAuditEvent struct {
	// Required fields (always validated)
	EventType     string
	EventCategory dsgen.AuditEventEventCategory // Use response type, not request type
	EventAction   string
	EventOutcome  dsgen.AuditEventEventOutcome
	CorrelationID string

	// Optional fields (validated only if non-empty/non-nil)
	Severity     *string // Pointer type per schema
	ActorID      *string
	ActorType    *string
	ResourceID   *string
	ResourceType *string
	Namespace    *string
	ClusterName  *string

	// EventData fields (validated if non-nil)
	EventDataFields map[string]interface{}
}

// ValidateAuditEvent validates that an audit event matches expected values.
// Use this helper to ensure consistent audit field validation across all tests.
//
// Example usage:
//
//	severity := "info"
//	testutil.ValidateAuditEvent(event, testutil.ExpectedAuditEvent{
//	    EventType:     "signal.categorization.completed",
//	    EventCategory: dsgen.AuditEventEventCategorySignalprocessing,
//	    EventAction:   "categorize",
//	    EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
//	    CorrelationID: string(sp.UID),
//	    Severity:      &severity,
//	    EventDataFields: map[string]interface{}{
//	        "signal_name": "TestSignal",
//	    },
//	})
func ValidateAuditEvent(event dsgen.AuditEvent, expected ExpectedAuditEvent) {
	// Validate required fields
	Expect(event.EventType).To(Equal(expected.EventType),
		"Audit event type mismatch")

	Expect(event.EventCategory).To(Equal(expected.EventCategory),
		"Audit event category mismatch")

	Expect(event.EventAction).To(Equal(expected.EventAction),
		"Audit event action mismatch")

	Expect(event.EventOutcome).To(Equal(expected.EventOutcome),
		"Audit event outcome mismatch")

	if expected.CorrelationID != "" {
		Expect(event.CorrelationId).To(Equal(expected.CorrelationID),
			"Audit event correlation ID mismatch")
	}

	// Validate optional pointer fields if specified
	if expected.Severity != nil {
		Expect(event.Severity).ToNot(BeNil(), "Audit event severity should not be nil")
		Expect(*event.Severity).To(Equal(*expected.Severity),
			"Audit event severity mismatch")
	}

	if expected.ActorID != nil {
		Expect(event.ActorId).ToNot(BeNil(), "Audit event actor_id should not be nil")
		Expect(*event.ActorId).To(Equal(*expected.ActorID),
			"Audit event actor ID mismatch")
	}

	if expected.ActorType != nil {
		Expect(event.ActorType).ToNot(BeNil(), "Audit event actor_type should not be nil")
		Expect(*event.ActorType).To(Equal(*expected.ActorType),
			"Audit event actor type mismatch")
	}

	if expected.ResourceID != nil {
		Expect(event.ResourceId).ToNot(BeNil(), "Audit event resource_id should not be nil")
		Expect(*event.ResourceId).To(Equal(*expected.ResourceID),
			"Audit event resource ID mismatch")
	}

	if expected.ResourceType != nil {
		Expect(event.ResourceType).ToNot(BeNil(), "Audit event resource_type should not be nil")
		Expect(*event.ResourceType).To(Equal(*expected.ResourceType),
			"Audit event resource type mismatch")
	}

	if expected.Namespace != nil {
		Expect(event.Namespace).ToNot(BeNil(), "Audit event namespace should not be nil")
		Expect(*event.Namespace).To(Equal(*expected.Namespace),
			"Audit event namespace mismatch")
	}

	if expected.ClusterName != nil {
		Expect(event.ClusterName).ToNot(BeNil(), "Audit event cluster_name should not be nil")
		Expect(*event.ClusterName).To(Equal(*expected.ClusterName),
			"Audit event cluster name mismatch")
	}

	// Validate EventData fields if specified
	if expected.EventDataFields != nil && len(expected.EventDataFields) > 0 {
		eventData, ok := event.EventData.(map[string]interface{})
		Expect(ok).To(BeTrue(),
			"Audit event EventData should be map[string]interface{}")

		for key, expectedValue := range expected.EventDataFields {
			actualValue, exists := eventData[key]
			Expect(exists).To(BeTrue(),
				fmt.Sprintf("Audit event EventData missing required field: %s", key))

			if expectedValue != nil {
				Expect(actualValue).To(Equal(expectedValue),
					fmt.Sprintf("Audit event EventData[%s] mismatch", key))
			}
		}
	}
}

// ValidateAuditEventHasRequiredFields validates that all standard audit fields are present.
// Use this for quick validation that event structure is correct.
func ValidateAuditEventHasRequiredFields(event dsgen.AuditEvent) {
	Expect(event.EventType).ToNot(BeEmpty(), "Audit event missing event_type")
	Expect(event.EventCategory).ToNot(BeZero(), "Audit event missing event_category")
	Expect(event.EventAction).ToNot(BeEmpty(), "Audit event missing event_action")
	Expect(event.EventOutcome).ToNot(BeZero(), "Audit event missing event_outcome")
	Expect(event.CorrelationId).ToNot(BeEmpty(), "Audit event missing correlation_id")
	Expect(event.EventTimestamp).ToNot(BeZero(), "Audit event missing event_timestamp")
	Expect(event.Version).ToNot(BeEmpty(), "Audit event missing version")
}

// AuditEventMatcher is a Gomega matcher for audit events.
// Example: Expect(event).To(MatchAuditEvent(expected))
type AuditEventMatcher struct {
	expected ExpectedAuditEvent
}

// MatchAuditEvent returns a Gomega matcher for audit events.
func MatchAuditEvent(expected ExpectedAuditEvent) *AuditEventMatcher {
	return &AuditEventMatcher{expected: expected}
}

// Match implements GomegaMatcher.
func (m *AuditEventMatcher) Match(actual interface{}) (bool, error) {
	event, ok := actual.(dsgen.AuditEvent)
	if !ok {
		return false, fmt.Errorf("MatchAuditEvent expects dsgen.AuditEvent, got %T", actual)
	}

	if event.EventType != m.expected.EventType {
		return false, nil
	}
	if event.EventCategory != m.expected.EventCategory {
		return false, nil
	}
	if event.EventAction != m.expected.EventAction {
		return false, nil
	}
	if event.EventOutcome != m.expected.EventOutcome {
		return false, nil
	}
	if m.expected.CorrelationID != "" && event.CorrelationId != m.expected.CorrelationID {
		return false, nil
	}
	if m.expected.Severity != nil {
		if event.Severity == nil || *event.Severity != *m.expected.Severity {
			return false, nil
		}
	}

	return true, nil
}

// FailureMessage implements GomegaMatcher.
func (m *AuditEventMatcher) FailureMessage(actual interface{}) string {
	event := actual.(dsgen.AuditEvent)
	severityStr := "<nil>"
	if event.Severity != nil {
		severityStr = *event.Severity
	}
	expectedSeverityStr := "<nil>"
	if m.expected.Severity != nil {
		expectedSeverityStr = *m.expected.Severity
	}
	return fmt.Sprintf(
		"Expected audit event to match:\n"+
			"  EventType: %s (got %s)\n"+
			"  EventCategory: %s (got %s)\n"+
			"  EventAction: %s (got %s)\n"+
			"  EventOutcome: %s (got %s)\n"+
			"  CorrelationID: %s (got %s)\n"+
			"  Severity: %s (got %s)",
		m.expected.EventType, event.EventType,
		m.expected.EventCategory, event.EventCategory,
		m.expected.EventAction, event.EventAction,
		m.expected.EventOutcome, event.EventOutcome,
		m.expected.CorrelationID, event.CorrelationId,
		expectedSeverityStr, severityStr,
	)
}

// NegatedFailureMessage implements GomegaMatcher.
func (m *AuditEventMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected audit event NOT to match %+v", m.expected)
}

// Helper functions for creating pointer values (common pattern in tests)

// StringPtr returns a pointer to the given string.
func StringPtr(s string) *string {
	return &s
}

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int {
	return &i
}
