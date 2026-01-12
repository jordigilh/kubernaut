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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// waitForStatusField polls for a status field to be populated by webhook
// Per TESTING_GUIDELINES.md: Use Eventually(), NEVER time.Sleep()
func waitForStatusField(
	ctx context.Context,
	k8sClient client.Client,
	obj client.Object,
	fieldGetter func() string,
	timeout time.Duration,
) {
	Eventually(func() string {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return ""
		}
		return fieldGetter()
	}, timeout, 500*time.Millisecond).ShouldNot(BeEmpty(),
		"Webhook should populate status field within %s", timeout)
}

// createAndWaitForCRD creates a CRD and waits for it to be ready
// Per TESTING_GUIDELINES.md: Use Eventually() for K8s eventual consistency
func createAndWaitForCRD(ctx context.Context, k8sClient client.Client, obj client.Object) {
	Expect(k8sClient.Create(ctx, obj)).To(Succeed(),
		"CRD creation should succeed")

	// Wait for CRD to be created (eventually consistent)
	Eventually(func() error {
		return k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	}, 10*time.Second, 500*time.Millisecond).Should(Succeed(),
		"CRD should be retrievable after creation")
}

// updateStatusAndWaitForWebhook updates CRD status and waits for webhook mutation
// This is the core pattern for testing webhook side effects
//
// Per TESTING_GUIDELINES.md §1773-1862: Integration tests should:
// 1. Trigger business operation (CRD status update)
// 2. Wait for webhook to mutate object (side effect)
// 3. Verify webhook populated fields correctly
func updateStatusAndWaitForWebhook(
	ctx context.Context,
	k8sClient client.Client,
	obj client.Object,
	updateFunc func(),
	verifyFunc func() bool,
) {
	// Apply status update (business operation)
	updateFunc()
	Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed(),
		"Status update should trigger webhook")

	// Wait for webhook to populate fields (side effect validation)
	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return false
		}
		return verifyFunc()
	}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
		"Webhook should mutate CRD within 10 seconds")
}

// deleteAndWaitForAnnotations deletes a CRD and waits for webhook to add annotations
// Used for NotificationRequest DELETE attribution tests
func deleteAndWaitForAnnotations(
	ctx context.Context,
	k8sClient client.Client,
	obj client.Object,
	expectedAnnotationKey string,
) {
	Expect(k8sClient.Delete(ctx, obj)).To(Succeed(),
		"CRD deletion should succeed")

	// Wait for webhook to add annotations before finalizer cleanup
	Eventually(func() string {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return ""
		}
		annotations := obj.GetAnnotations()
		if annotations == nil {
			return ""
		}
		return annotations[expectedAnnotationKey]
	}, 10*time.Second, 500*time.Millisecond).ShouldNot(BeEmpty(),
		"Webhook should add %s annotation on DELETE", expectedAnnotationKey)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// AUDIT EVENT VALIDATION HELPERS - DD-TESTING-001 Compliance
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// These helpers implement mandatory standards from DD-TESTING-001:
// - OpenAPI-generated client for type safety (DD-API-001)
// - Deterministic event count validation (Equal(N), not BeNumerically(">="))
// - Structured event_data validation (DD-AUDIT-004)
// - Eventually() for async polling (no time.Sleep())
//
// Authority: DD-TESTING-001 v1.0 (2026-01-02)
// Pattern: AIAnalysis E2E tests (test/e2e/aianalysis/)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// randomSuffix generates a random hex suffix for test resource names
func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// queryAuditEvents queries Data Storage for audit events using OpenAPI client.
//
// Parameters:
//   - dsClient: OpenAPI-generated Data Storage client (DD-API-001)
//   - correlationID: Correlation ID to filter events
//   - eventType: Optional event type filter (nil for all events)
//
// Returns: Array of audit events (OpenAPI-generated types)
//
// Pattern: DD-TESTING-001 Pattern 2 (Type-Safe Query Helper)
func queryAuditEvents(
	dsClient *ogenclient.Client,
	correlationID string,
	eventType *string,
) ([]ogenclient.AuditEvent, error) {
	params := ogenclient.QueryAuditEventsParams{}

	// Set CorrelationID using OptString.SetTo()
	params.CorrelationID.SetTo(correlationID)

	// Set Limit using OptInt.SetTo()
	params.Limit.SetTo(100)

	// Set EventType if provided
	if eventType != nil {
		params.EventType.SetTo(*eventType)
	}

	resp, err := dsClient.QueryAuditEvents(context.Background(), params)
	if err != nil {
		return nil, fmt.Errorf("failed to query DataStorage: %w", err)
	}

	// Ogen returns slice directly (not pointer)
	return resp.Data, nil
}

// waitForAuditEvents polls Data Storage until events appear.
//
// Parameters:
//   - dsClient: OpenAPI-generated Data Storage client
//   - correlationID: Correlation ID to filter events
//   - eventType: Event type to filter
//   - minCount: Minimum expected count (for Eventually() polling)
//
// Returns: Array of audit events
//
// Pattern: DD-TESTING-001 Pattern 3 (Async Event Polling with Eventually())
//
// Note: This helper uses Eventually() for polling, NOT time.Sleep().
// After polling, tests MUST validate exact counts with Equal(N) per DD-TESTING-001.
func waitForAuditEvents(
	dsClient *ogenclient.Client,
	correlationID string,
	eventType string,
	minCount int,
) []ogenclient.AuditEvent {
	var events []ogenclient.AuditEvent

	Eventually(func() int {
		var err error
		events, err = queryAuditEvents(dsClient, correlationID, &eventType)
		if err != nil {
			GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
			return 0
		}
		return len(events)
	}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", minCount),
		fmt.Sprintf("Should have at least %d %s events for correlation %s", minCount, eventType, correlationID))

	return events
}

// countEventsByType counts occurrences of each event type in the given events.
//
// Returns: map[eventType]count
//
// Pattern: DD-TESTING-001 Pattern 4 (Deterministic Event Count Validation)
//
// Usage:
//
//	eventCounts := countEventsByType(allEvents)
//	Expect(eventCounts["webhook.block_clearance"]).To(Equal(1))  // ✅ CORRECT
//	// NOT: Expect(len(events)).To(BeNumerically(">=", 1))        // ❌ FORBIDDEN
func countEventsByType(events []ogenclient.AuditEvent) map[string]int {
	counts := make(map[string]int)
	for _, event := range events {
		counts[event.EventType]++
	}
	return counts
}

// validateEventMetadata validates event_category, event_outcome, and event_timestamp.
//
// Parameters:
//   - event: Audit event to validate
//   - expectedCategory: Expected event_category value
//
// Pattern: DD-TESTING-001 Pattern 6 (Event Category and Outcome Validation)
func validateEventMetadata(event ogenclient.AuditEvent, expectedCategory string) {
	// Validate event_category matches service
	Expect(string(event.EventCategory)).To(Equal(expectedCategory),
		"Webhook events must have event_category='webhook'")

	// Validate event_outcome is valid
	outcome := string(event.EventOutcome)
	Expect([]string{"success", "failure"}).To(ContainElement(outcome),
		"event_outcome must be 'success' or 'failure'")

	// Validate timestamp is set
	Expect(event.EventTimestamp).NotTo(BeZero(),
		"event_timestamp must be set")
}

// validateEventData validates structured event_data fields.
//
// Parameters:
//   - event: Audit event to validate
//   - expectedFields: Map of field names to expected types or values
//
// Pattern: DD-TESTING-001 Pattern 5 (Structured event_data Validation)
//
// Usage:
//
//	validateEventData(event, map[string]interface{}{
//	    "operator":    "system:serviceaccount:test",
//	    "crd_name":    "test-we-12345",
//	    "namespace":   "default",
//	    "action":      "block_clearance",
//	})
func validateEventData(event ogenclient.AuditEvent, expectedFields map[string]interface{}) {
	// Marshal EventData to JSON, then unmarshal to map for validation
	// Ogen discriminated unions implement json.Marshaler (Q6 answer)
	eventDataBytes, err := json.Marshal(event.EventData)
	Expect(err).ToNot(HaveOccurred(), "event_data should marshal to JSON")

	var eventData map[string]interface{}
	err = json.Unmarshal(eventDataBytes, &eventData)
	Expect(err).ToNot(HaveOccurred(), "event_data JSON should unmarshal to map")

	// Validate all expected fields are present and match
	for field, expectedValue := range expectedFields {
		Expect(eventData).To(HaveKey(field),
			fmt.Sprintf("event_data should have field '%s'", field))

		if expectedValue != nil {
			actualValue := eventData[field]
			// DEBUG: Print actual value for troubleshooting
			if actualValue != expectedValue {
				GinkgoWriter.Printf("⚠️  Field mismatch: %s\n", field)
				GinkgoWriter.Printf("   Expected: %v (type: %T)\n", expectedValue, expectedValue)
				GinkgoWriter.Printf("   Actual:   %v (type: %T)\n", actualValue, actualValue)
			}
			Expect(actualValue).To(Equal(expectedValue),
				fmt.Sprintf("event_data['%s'] should equal '%v' (actual: '%v')", field, expectedValue, actualValue))
		}
	}
}
