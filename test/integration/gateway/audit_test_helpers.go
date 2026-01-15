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

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedhelpers "github.com/jordigilh/kubernaut/test/shared/helpers"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// AUDIT EVENT QUERY HELPERS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// IMPORTANT: This file uses SHARED audit helpers from test/shared/helpers/audit.go
// DO NOT duplicate query functions here - use shared helpers instead.
//
// Shared Query Functions (use these):
//   - sharedhelpers.QueryAuditEvents()
//   - sharedhelpers.QueryAuditEventsByCorrelationID()
//   - sharedhelpers.QueryAuditEventsByType()
//   - sharedhelpers.QueryAuditEventsByCategory()
//
// This file contains only:
//   1. createOgenClient() - Gateway-specific client factory
//   2. waitForAuditEvent() - Convenience async wrapper
//   3. Gateway-specific payload validation functions
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// createOgenClient creates an ogen HTTP client for querying DataStorage
// Gateway integration tests use this to query audit events via HTTP API
// Port 18091 per DD-TEST-001: Gateway DataStorage HTTP API port
func createOgenClient() (*ogenclient.Client, error) {
	const gatewayDataStoragePort = 18091 // DD-TEST-001: Gateway DataStorage HTTP API port
	return ogenclient.NewClient(
		fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort),
	)
}

// waitForAuditEvent waits for a specific audit event to appear (with timeout)
// Uses polling pattern for async validation (alternative to Eventually())
//
// Example:
//   client, _ := createOgenClient()
//   event, err := waitForAuditEvent(ctx, client, "gateway.crd.created", correlationID, 10*time.Second)
//   Expect(err).ToNot(HaveOccurred())
//   Expect(event).ToNot(BeNil())
func waitForAuditEvent(
	ctx context.Context,
	client *ogenclient.Client,
	eventType string,
	correlationID string,
	timeout time.Duration,
) (*ogenclient.AuditEvent, error) {
	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			return nil, fmt.Errorf("timeout waiting for event %s (correlation_id=%s)", eventType, correlationID)
		case <-ticker.C:
			// Use shared helper to query
			events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
			if err != nil {
				continue // Retry on error
			}
			if len(events) > 0 {
				return &events[0], nil // Return most recent
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
