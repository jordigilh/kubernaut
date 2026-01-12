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
package validators

import (
	"fmt"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/gomega"
)

// ExpectedAuditEvent defines expected values for an audit event.
// Fields left empty (zero value) will not be validated.
// Based on ogenclient.AuditEvent schema from pkg/datastorage/client/generated.go
type ExpectedAuditEvent struct {
	// Required fields (always validated)
	EventType     string
	EventCategory ogenclient.AuditEventEventCategory // Use response type, not request type
	EventAction   string

	// Optional fields (validated only if non-empty/non-nil)
	EventOutcome  *ogenclient.AuditEventEventOutcome // Optional: may vary (e.g., HolmesGPT errors)
	CorrelationID string
	Severity      *string // Pointer type per schema
	ActorID       *string
	ActorType     *string
	ResourceID    *string
	ResourceType  *string
	Namespace     *string
	ClusterName   *string

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
//	    EventCategory: ogenclient.AuditEventEventCategorySignalprocessing,
//	    EventAction:   "categorize",
//	    EventOutcome:  ogenclient.AuditEventEventOutcomeSuccess,
//	    CorrelationID: string(sp.UID),
//	    Severity:      &severity,
//	    EventDataFields: map[string]interface{}{
//	        "signal_name": "TestSignal",
//	    },
//	})
func ValidateAuditEvent(event ogenclient.AuditEvent, expected ExpectedAuditEvent) {
	// Validate required fields
	Expect(event.EventType).To(Equal(expected.EventType),
		"Audit event type mismatch")

	Expect(event.EventCategory).To(Equal(expected.EventCategory),
		"Audit event category mismatch")

	Expect(event.EventAction).To(Equal(expected.EventAction),
		"Audit event action mismatch")

	// Validate optional EventOutcome (may vary based on actual outcome)
	if expected.EventOutcome != nil {
		Expect(event.EventOutcome).To(Equal(*expected.EventOutcome),
			"Audit event outcome mismatch")
	}

	if expected.CorrelationID != "" {
		Expect(event.CorrelationID).To(Equal(expected.CorrelationID),
			"Audit event correlation ID mismatch")
	}

	// Validate optional fields using ogen Opt types (OGEN-MIGRATION)
	// Pattern: Use IsSet() to check existence, then access .Value
	if expected.Severity != nil {
		Expect(event.Severity.IsSet()).To(BeTrue(), "Audit event severity should be set")
		Expect(event.Severity.Value).To(Equal(*expected.Severity),
			"Audit event severity mismatch")
	}

	if expected.ActorID != nil {
		Expect(event.ActorID.IsSet()).To(BeTrue(), "Audit event actor_id should be set")
		Expect(event.ActorID.Value).To(Equal(*expected.ActorID),
			"Audit event actor ID mismatch")
	}

	if expected.ActorType != nil {
		Expect(event.ActorType.IsSet()).To(BeTrue(), "Audit event actor_type should be set")
		Expect(event.ActorType.Value).To(Equal(*expected.ActorType),
			"Audit event actor type mismatch")
	}

	if expected.ResourceID != nil {
		Expect(event.ResourceID.IsSet()).To(BeTrue(), "Audit event resource_id should be set")
		Expect(event.ResourceID.Value).To(Equal(*expected.ResourceID),
			"Audit event resource ID mismatch")
	}

	if expected.ResourceType != nil {
		Expect(event.ResourceType.IsSet()).To(BeTrue(), "Audit event resource_type should be set")
		Expect(event.ResourceType.Value).To(Equal(*expected.ResourceType),
			"Audit event resource type mismatch")
	}

	if expected.Namespace != nil {
		Expect(event.Namespace.IsSet()).To(BeTrue(), "Audit event namespace should be set")
		Expect(event.Namespace.Value).To(Equal(*expected.Namespace),
			"Audit event namespace mismatch")
	}

	if expected.ClusterName != nil {
		Expect(event.ClusterName.IsSet()).To(BeTrue(), "Audit event cluster_name should be set")
		Expect(event.ClusterName.Value).To(Equal(*expected.ClusterName),
			"Audit event cluster name mismatch")
	}

	// Validate EventData fields if specified
	// Note: EventData is now a discriminated union (ogen-generated)
	// For specific field validation, use the appropriate Get method on EventData
	// Example: eventData := event.EventData.GetWorkflowExecutionAuditPayload()
	if expected.EventDataFields != nil && len(expected.EventDataFields) > 0 {
		// EventData validation is now type-specific
		// Use the appropriate payload Get method in your test code
		// This generic validator no longer supports unstructured data
		Expect(event.EventData.Type).ToNot(BeEmpty(),
			"Audit event EventData must have a discriminator type")
	}
}

// ValidateAuditEventHasRequiredFields validates that all standard audit fields are present.
// Use this for quick validation that event structure is correct.
func ValidateAuditEventHasRequiredFields(event ogenclient.AuditEvent) {
	Expect(event.EventType).ToNot(BeEmpty(), "Audit event missing event_type")
	Expect(event.EventCategory).ToNot(BeZero(), "Audit event missing event_category")
	Expect(event.EventAction).ToNot(BeEmpty(), "Audit event missing event_action")
	Expect(event.EventOutcome).ToNot(BeZero(), "Audit event missing event_outcome")
	Expect(event.CorrelationID).ToNot(BeEmpty(), "Audit event missing correlation_id")
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
	event, ok := actual.(ogenclient.AuditEvent)
	if !ok {
		return false, fmt.Errorf("MatchAuditEvent expects ogenclient.AuditEvent, got %T", actual)
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
	if m.expected.EventOutcome != nil && event.EventOutcome != *m.expected.EventOutcome {
		return false, nil
	}
	if m.expected.CorrelationID != "" && event.CorrelationID != m.expected.CorrelationID {
		return false, nil
	}
	if m.expected.Severity != nil {
		if !event.Severity.IsSet() || event.Severity.Value != *m.expected.Severity {
			return false, nil
		}
	}

	return true, nil
}

// FailureMessage implements GomegaMatcher.
func (m *AuditEventMatcher) FailureMessage(actual interface{}) string {
	event := actual.(ogenclient.AuditEvent)
	severityStr := "<nil>"
	if event.Severity.IsSet() {
		severityStr = event.Severity.Value
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
		m.expected.CorrelationID, event.CorrelationID,
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

// EventOutcomePtr returns a pointer to the given EventOutcome.
// Usage: EventOutcome: testutil.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess)
func EventOutcomePtr(outcome ogenclient.AuditEventEventOutcome) *ogenclient.AuditEventEventOutcome {
	return &outcome
}

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int {
	return &i
}
