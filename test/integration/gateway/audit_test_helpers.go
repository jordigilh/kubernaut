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
	"fmt"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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
