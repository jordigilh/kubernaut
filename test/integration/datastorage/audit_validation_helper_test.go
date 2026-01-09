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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsgen ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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
		client  *dsgen.ClientWithResponses
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		ctx = context.Background()

		// Use datastorageURL from suite_test.go (DD-TEST-001)
		baseURL = datastorageURL

		// Create OpenAPI client
		client, err = dsgen.NewClientWithResponses(baseURL)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("testutil.ValidateAuditEvent usage", func() {
		It("should validate audit event response using testutil helper", func() {
			// BR-STORAGE-019: Write API with structured validation
			// BR-STORAGE-020: Audit event ingestion

			// ARRANGE: Create test audit event using OpenAPI client
			correlationID := generateTestID()
			eventData := map[string]interface{}{
				"test_field": "test_value",
			}

			auditEvent := createAuditEventRequest(
				"test.validation.event",
				"gateway",
				"test_action",
				"success",
				correlationID,
				eventData,
			)

			// ACT: Post audit event
			eventID, err := postAuditEvent(ctx, client, auditEvent)
			Expect(err).ToNot(HaveOccurred())
			Expect(eventID).ToNot(BeEmpty())

			// Query the event back
			resp, err := client.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))
			Expect(resp.JSON200).ToNot(BeNil())
			Expect(resp.JSON200.Data).ToNot(BeNil())

			data := *resp.JSON200.Data
			Expect(data).To(HaveLen(1), "should return exactly one event")

			// ASSERT: Validate using testutil helper (V1.0 maturity requirement)
			testutil.ValidateAuditEvent(data[0], testutil.ExpectedAuditEvent{
				EventType:     "test.validation.event",
				EventCategory: dsgen.AuditEventEventCategoryGateway,
				EventAction:   "test_action",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
			})
		})
	})
})
