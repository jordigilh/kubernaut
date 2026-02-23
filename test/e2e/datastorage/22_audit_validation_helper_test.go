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
	"github.com/jordigilh/kubernaut/test/shared/validators"
)

// ========================================
// TESTUTIL VALIDATOR INTEGRATION TEST
// ========================================
// This test demonstrates usage of validators.ValidateAuditEvent
// for structured audit event validation (V1.0 maturity requirement).
// Per scripts/validate-service-maturity.sh check for testutil usage.
// ========================================

var _ = Describe("Audit Event Validation Helper", func() {
	var (
		client *ogenclient.Client
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// DD-AUTH-014: Use shared authenticated DSClient from suite setup
		client = DSClient
	})

	Context("validators.ValidateAuditEvent usage", func() {
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
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
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
			Expect(eventID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "eventID must be a valid UUID")

			// E2E NOTE: No database schema manipulation needed here!
			// E2E tests use HTTP API only (no direct DB access).
			// The DataStorage server handles all database operations internally.

			// Query the event back via HTTP API with pagination
			// Per docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md: ALWAYS handle pagination
			var allEvents []ogenclient.AuditEvent
			offset := 0
			limit := 100

			for {
				resp, err := client.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					Limit:         ogenclient.NewOptInt(limit),
					Offset:        ogenclient.NewOptInt(offset),
				})
				Expect(err).ToNot(HaveOccurred())

				if len(resp.Data) == 0 {
					break
				}

				allEvents = append(allEvents, resp.Data...)

				if len(resp.Data) < limit {
					break
				}

				offset += limit
			}

			Expect(allEvents).To(Not(BeNil()), "query must return events")
			Expect(allEvents).To(HaveLen(1), "should return exactly one event")

			// ASSERT: Validate using testutil helper (V1.0 maturity requirement)
			expectedOutcome := ogenclient.AuditEventEventOutcomeSuccess
			validators.ValidateAuditEvent(allEvents[0], validators.ExpectedAuditEvent{
				EventType:     "gateway.signal.received",
				EventCategory: ogenclient.AuditEventEventCategoryGateway,
				EventAction:   "test_action",
				EventOutcome:  &expectedOutcome,
				CorrelationID: correlationID,
			})
		})
	})
})
