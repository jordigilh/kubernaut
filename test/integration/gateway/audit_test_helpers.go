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

package gateway

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// AUDIT EVENT QUERY HELPERS (DataStorage HTTP API)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// These helpers query audit events from DataStorage via HTTP API.
// Pattern follows SignalProcessing and AIAnalysis proven approach.
//
// Key Design Decisions:
// - Use QueryAuditEvents (not ListAuditEvents) - standard across all services
// - Response has Data field containing []ogenclient.AuditEvent
// - Use NewOptString/NewOptInt for optional parameters
// - Include debug logging for parallel execution troubleshooting
//
// Usage Example:
//   events, err := queryAuditEventsByType(ctx, "gateway.crd.created", correlationID)
//   Expect(err).ToNot(HaveOccurred())
//   Expect(events).To(HaveLen(1))
//
//   event := events[0]
//   payload, ok := event.EventData.GetGatewayAuditPayload()
//   Expect(ok).To(BeTrue())
//   Expect(payload.Namespace).To(Equal(testNamespace))
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// createOgenClientForAuditQueries creates an ogen HTTP client for querying DataStorage
// Gateway integration tests use this to query audit events via HTTP API
// Port 18091 per DD-TEST-001: Gateway DataStorage HTTP API port
func createOgenClientForAuditQueries() (*ogenclient.Client, error) {
	const gatewayDataStoragePort = 18091 // DD-TEST-001: Gateway DataStorage HTTP API port
	return ogenclient.NewClient(
		fmt.Sprintf("http://localhost:%d", gatewayDataStoragePort),
	)
}

// queryAuditEventsByType queries audit events by event type and correlation ID
// Returns all matching events (use with Eventually() for asynchronous validation)
func queryAuditEventsByType(ctx context.Context, eventType, correlationID string) ([]ogenclient.AuditEvent, error) {
	ogenClient, err := createOgenClientForAuditQueries()
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client: %w", err)
	}

	params := ogenclient.QueryAuditEventsParams{
		EventType:     ogenclient.NewOptString(eventType),
		CorrelationID: ogenclient.NewOptString(correlationID),
	}

	resp, err := ogenClient.QueryAuditEvents(ctx, params)
	if err != nil {
		GinkgoWriter.Printf("⏳ Query error for %s (correlation_id=%s): %v\n",
			eventType, correlationID, err)
		return nil, err
	}

	events := resp.Data
	if len(events) == 0 {
		GinkgoWriter.Printf("⏳ No events yet for %s (correlation_id=%s)\n",
			eventType, correlationID)
	} else {
		GinkgoWriter.Printf("✅ Found %d event(s) for %s (correlation_id=%s)\n",
			len(events), eventType, correlationID)
	}

	return events, nil
}

// countAuditEventsByType counts audit events by type and correlation ID
// Convenience wrapper for queryAuditEventsByType when only count is needed
func countAuditEventsByType(ctx context.Context, eventType, correlationID string) int {
	events, err := queryAuditEventsByType(ctx, eventType, correlationID)
	if err != nil {
		return 0
	}
	return len(events)
}

// getLatestAuditEvent retrieves the most recent event by type and correlation ID
// Returns nil if no events found (use with Eventually() for async validation)
func getLatestAuditEvent(ctx context.Context, eventType, correlationID string) (*ogenclient.AuditEvent, error) {
	ogenClient, err := createOgenClientForAuditQueries()
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client: %w", err)
	}

	params := ogenclient.QueryAuditEventsParams{
		EventType:     ogenclient.NewOptString(eventType),
		CorrelationID: ogenclient.NewOptString(correlationID),
		Limit:         ogenclient.NewOptInt(1), // Get most recent only
	}

	resp, err := ogenClient.QueryAuditEvents(ctx, params)
	if err != nil {
		GinkgoWriter.Printf("⏳ Query error for %s (correlation_id=%s): %v\n",
			eventType, correlationID, err)
		return nil, err
	}

	events := resp.Data
	if len(events) == 0 {
		GinkgoWriter.Printf("⏳ No events yet for %s (correlation_id=%s)\n",
			eventType, correlationID)
		return nil, nil
	}

	return &events[0], nil
}

// queryAllAuditEvents queries all audit events for a correlation ID
// Useful for validating complete audit trail (signal.received + crd.created + deduplicated, etc.)
func queryAllAuditEvents(ctx context.Context, correlationID string) ([]ogenclient.AuditEvent, error) {
	ogenClient, err := createOgenClientForAuditQueries()
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client: %w", err)
	}

	params := ogenclient.QueryAuditEventsParams{
		CorrelationID: ogenclient.NewOptString(correlationID),
		Limit:         ogenclient.NewOptInt(100), // Get all events for this correlation ID
	}

	resp, err := ogenClient.QueryAuditEvents(ctx, params)
	if err != nil {
		GinkgoWriter.Printf("⏳ Query error for correlation_id=%s: %v\n", correlationID, err)
		return nil, err
	}

	events := resp.Data
	GinkgoWriter.Printf("✅ Found %d total event(s) for correlation_id=%s\n", len(events), correlationID)

	return events, nil
}

// filterEventsByType filters a list of audit events by event type
// Helper for post-query filtering when querying all events by correlation ID
func filterEventsByType(events []ogenclient.AuditEvent, eventType string) []ogenclient.AuditEvent {
	var filtered []ogenclient.AuditEvent
	for _, event := range events {
		if event.EventType == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// waitForAuditEvent waits for a specific audit event to appear (with timeout)
// Uses Eventually() pattern for async validation
//
// Example:
//   event, err := waitForAuditEvent(ctx, "gateway.crd.created", correlationID, 10*time.Second)
//   Expect(err).ToNot(HaveOccurred())
//   Expect(event).ToNot(BeNil())
func waitForAuditEvent(ctx context.Context, eventType, correlationID string, timeout time.Duration) (*ogenclient.AuditEvent, error) {
	var event *ogenclient.AuditEvent
	var lastErr error

	// Use time.After for timeout instead of Eventually's timeout
	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			if lastErr != nil {
				return nil, fmt.Errorf("timeout waiting for event %s: %w", eventType, lastErr)
			}
			return nil, fmt.Errorf("timeout waiting for event %s (no errors)", eventType)
		case <-ticker.C:
			event, lastErr = getLatestAuditEvent(ctx, eventType, correlationID)
			if lastErr == nil && event != nil {
				return event, nil
			}
		}
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// GATEWAY AUDIT PAYLOAD VALIDATION HELPERS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// These helpers validate Gateway-specific audit payload fields.
// Pattern: Extract GatewayAuditPayload from EventData union, validate fields.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// extractGatewayPayload extracts GatewayAuditPayload from AuditEvent
// Returns payload and true if event has Gateway payload, empty payload and false otherwise
func extractGatewayPayload(event *ogenclient.AuditEvent) (ogenclient.GatewayAuditPayload, bool) {
	if event == nil {
		return ogenclient.GatewayAuditPayload{}, false
	}
	return event.EventData.GetGatewayAuditPayload()
}

// validateSignalReceivedPayload validates gateway.signal.received event payload
// Checks for required fields: signal_type, alert_name, namespace, fingerprint
func validateSignalReceivedPayload(event *ogenclient.AuditEvent, expectedNamespace string) error {
	payload, ok := extractGatewayPayload(event)
	if !ok {
		return fmt.Errorf("event does not contain Gateway audit payload")
	}

	if string(payload.SignalType) == "" {
		return fmt.Errorf("signal_type is empty")
	}

	if payload.AlertName == "" {
		return fmt.Errorf("alert_name is empty")
	}

	if payload.Namespace != expectedNamespace {
		return fmt.Errorf("namespace mismatch: got %s, expected %s", payload.Namespace, expectedNamespace)
	}

	if payload.Fingerprint == "" {
		return fmt.Errorf("fingerprint is empty")
	}

	return nil
}

// validateCRDCreatedPayload validates gateway.crd.created event payload
// Checks for RemediationRequest reference (namespace/name format)
func validateCRDCreatedPayload(event *ogenclient.AuditEvent, expectedNamespace string) error {
	payload, ok := extractGatewayPayload(event)
	if !ok {
		return fmt.Errorf("event does not contain Gateway audit payload")
	}

	rrRef, hasRR := payload.RemediationRequest.Get()
	if !hasRR {
		return fmt.Errorf("RemediationRequest field is not set")
	}

	if rrRef == "" {
		return fmt.Errorf("RemediationRequest is empty")
	}

	// Validate format (should contain namespace)
	if payload.Namespace != expectedNamespace {
		return fmt.Errorf("namespace mismatch: got %s, expected %s", payload.Namespace, expectedNamespace)
	}

	return nil
}

// validateDeduplicatedPayload validates gateway.signal.deduplicated event payload
// Checks for existing_remediation_request field
func validateDeduplicatedPayload(event *ogenclient.AuditEvent, expectedRRName string) error {
	payload, ok := extractGatewayPayload(event)
	if !ok {
		return fmt.Errorf("event does not contain Gateway audit payload")
	}

	rrRef, hasRR := payload.RemediationRequest.Get()
	if !hasRR {
		return fmt.Errorf("RemediationRequest field is not set (should reference existing RR)")
	}

	if rrRef == "" {
		return fmt.Errorf("RemediationRequest is empty")
	}

	// For deduplication events, RemediationRequest should reference the existing CRD
	if expectedRRName != "" && rrRef != expectedRRName {
		return fmt.Errorf("RemediationRequest mismatch: got %s, expected %s", rrRef, expectedRRName)
	}

	return nil
}
