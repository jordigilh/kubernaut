/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF the License governing permissions and
limitations under the License.
*/

package datastorage

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// ========================================
// TESTUTIL VALIDATOR INTEGRATION TEST
// ========================================
// This test demonstrates usage of testutil.ValidateAuditEvent
// for structured audit event validation (V1.0 maturity requirement).
// Per scripts/validate-service-maturity.sh check for testutil usage.
// ========================================

var _ = Describe("Audit Event Validation Helper",  func() {
	var (
		baseURL string
		client  *ogenclient.Client
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		ctx = context.Background()

		// Use dataStorageURL from suite_test.go (DD-TEST-001)
		baseURL = dataStorageURL

		// Create OpenAPI client
		client, err = ogenclient.NewClient(baseURL)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("testutil.ValidateAuditEvent usage", func() {
		It("should validate audit event response using testutil helper", func() {
			// BR-STORAGE-019: Write API with structured validation
			// BR-STORAGE-020: Audit event ingestion

			// ARRANGE: Create test audit event using valid discriminated union types
			correlationID := generateTestID()

			// Use valid event type from discriminator mapping
			eventData := ogenclient.AuditEventRequestEventData{
				Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
				GatewayAuditPayload: ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
					AlertName:   "ValidationTest",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				},
			}

			auditEvent := ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "gateway.signal.received",
				EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
				EventAction:    "test_action",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				CorrelationID:  correlationID,
				EventData:      eventData,
			}

		// ACT: Post audit event via HTTP API
			eventID, err := postAuditEvent(ctx, client, auditEvent)
			Expect(err).ToNot(HaveOccurred())
			Expect(eventID).ToNot(BeEmpty())

			// E2E NOTE: No database schema manipulation needed here!
			// E2E tests use HTTP API only (no direct DB access).
			// The DataStorage server handles all database operations internally.

			// Query the event back via HTTP API
			resp, err := client.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.Data).ToNot(BeNil())
			Expect(resp.Data).To(HaveLen(1), "should return exactly one event")

		// ASSERT: Validate using testutil helper (V1.0 maturity requirement)
		expectedOutcome := ogenclient.AuditEventEventOutcomeSuccess
		testutil.ValidateAuditEvent(resp.Data[0], testutil.ExpectedAuditEvent{
			EventType:     "gateway.signal.received",
			EventCategory: ogenclient.AuditEventEventCategoryGateway,
			EventAction:   "test_action",
			EventOutcome:  &expectedOutcome,
			CorrelationID: correlationID,
		})
		})
	})
})
