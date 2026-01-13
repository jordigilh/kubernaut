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

package datastorage

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// RECONSTRUCTION BUSINESS LOGIC INTEGRATION TESTS
// ========================================
//
// Purpose: Test reconstruction business logic components against REAL PostgreSQL database
// to validate component interactions with real infrastructure.
//
// Business Requirements:
// - BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
//
// Test Strategy (per 03-testing-strategy.mdc):
// - Integration Tests (>50%): Test business components with REAL database
// - NO HTTP server (that's for E2E tests)
// - Call reconstruction.* functions directly
// - Validate database interactions, field mapping, and business logic
//
// Test Plan Reference:
// - docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
// - Test Tier: Integration Tests (Tier 2)
// - Gap Coverage: Gaps #1-3, #8 (business logic with real DB)
//
// ========================================

var _ = Describe("Reconstruction Business Logic Integration Tests (BR-AUDIT-006)", func() {
	var (
		auditRepo     *repository.AuditEventsRepository
		testID        string
		correlationID string
	)

	BeforeEach(func() {
		// Create audit events repository
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)

		// Generate unique test ID for isolation
		testID = generateTestID()
		correlationID = fmt.Sprintf("test-reconstruction-%s", testID)

		// Clean up test data
		_, err := db.ExecContext(ctx,
			"DELETE FROM audit_events WHERE correlation_id = $1",
			correlationID)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up test data
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM audit_events WHERE correlation_id = $1",
				correlationID)
		}
	})

	// ========================================
	// QUERY COMPONENT INTEGRATION TESTS
	// ========================================
	Context("INTEGRATION-QUERY-01: Query audit events from real database", func() {
		It("should retrieve audit events by correlation ID", func() {
			// ARRANGE: Seed audit events in real PostgreSQL
			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-10 * time.Second).UTC(),
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "signal-123",
				EventData: map[string]interface{}{
					"event_type":  "gateway.signal.received", // Required for discriminator
					"signal_type": "prometheus-alert",
					"alert_name":  "HighCPU",     // Required field
					"namespace":   "default",     // Required field
					"fingerprint": "test-fp-123", // Required field
				},
			}

			_, err := auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			orchestratorEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-123",
				EventData: map[string]interface{}{
					"event_type": "orchestrator.lifecycle.created", // Required for discriminator
					"rr_name":    "test-rr",                        // Required field
					"namespace":  "default",                        // Required field
					"timeout_config": map[string]interface{}{
						"global": "1h",
					},
				},
			}

			_, err = auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Call query component directly (business logic, not HTTP)
			events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)

			// ASSERT: Query succeeds and returns events
			Expect(err).ToNot(HaveOccurred(), "Query should succeed")
			Expect(events).To(HaveLen(2), "Should return 2 events")

			// ASSERT: Events are ordered chronologically
			Expect(events[0].EventType).To(Equal("gateway.signal.received"))
			Expect(events[1].EventType).To(Equal("orchestrator.lifecycle.created"))
		})

		It("should return empty slice for non-existent correlation ID", func() {
			// ACT: Query with non-existent correlation ID
			nonExistentID := fmt.Sprintf("nonexistent-%s", testID)
			events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, nonExistentID)

			// ASSERT: No error, empty slice
			Expect(err).ToNot(HaveOccurred())
			Expect(events).To(BeEmpty())
		})
	})

	// ========================================
	// PARSE + MAP + BUILD COMPONENT INTEGRATION TESTS
	// ========================================
	Context("INTEGRATION-COMPONENTS-01: Full reconstruction pipeline with real database", func() {
		It("should reconstruct RR from gateway and orchestrator events", func() {
			// ARRANGE: Seed audit events in real PostgreSQL
			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-10 * time.Second).UTC(),
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "signal-123",
				EventData: map[string]interface{}{
					"event_type":  "gateway.signal.received", // Required for discriminator
					"signal_type": "prometheus-alert",
					"alert_name":  "HighCPU",     // Required field
					"namespace":   "default",     // Required field
					"fingerprint": "test-fp-456", // Required field
					"signal_labels": map[string]interface{}{
						"alertname": "HighCPU",
						"severity":  "critical",
					},
					"signal_annotations": map[string]interface{}{
						"summary": "CPU usage is high",
					},
					"original_payload": map[string]interface{}{
						"alert": "data",
					},
				},
			}

			_, err := auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			orchestratorEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-123",
				EventData: map[string]interface{}{
					"event_type": "orchestrator.lifecycle.created", // Required for discriminator
					"rr_name":    "test-rr",                        // Required field
					"namespace":  "default",                        // Required field
					"timeout_config": map[string]interface{}{
						"global":     "1h",
						"processing": "10m",
						"analyzing":  "15m",
					},
				},
			}

			_, err = auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Call business logic components directly (NO HTTP)

			// Step 1: Query events from real database
			events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
			Expect(err).ToNot(HaveOccurred())
			Expect(events).To(HaveLen(2))

			// Step 2: Parse events to extract structured data
			parsedData := make([]reconstruction.ParsedAuditData, 0, len(events))
			for _, event := range events {
				parsed, err := reconstruction.ParseAuditEvent(event)
				Expect(err).ToNot(HaveOccurred())
				parsedData = append(parsedData, *parsed)
			}
			Expect(parsedData).To(HaveLen(2))

			// Step 3: Map parsed data to RR Spec/Status fields
			rrFields, err := reconstruction.MergeAuditData(parsedData)
			Expect(err).ToNot(HaveOccurred())
			Expect(rrFields).ToNot(BeNil())

			// Step 4: Build complete RemediationRequest CRD
			rr, err := reconstruction.BuildRemediationRequest(correlationID, rrFields)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())

			// Step 5: Validate reconstructed RR
			validationResult, err := reconstruction.ValidateReconstructedRR(rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(validationResult.IsValid).To(BeTrue())
			Expect(validationResult.Completeness).To(BeNumerically(">=", 50))

			// ASSERT: Reconstructed fields match seeded data
			Expect(rr.Spec.SignalName).To(Equal("HighCPU"))
			Expect(rr.Spec.SignalType).To(Equal("prometheus-alert"))
			Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("alertname", "HighCPU"))
			Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("severity", "critical"))
			Expect(rr.Spec.SignalAnnotations).To(HaveKeyWithValue("summary", "CPU usage is high"))
			Expect(rr.Spec.OriginalPayload).To(ContainSubstring("alert"))
			Expect(rr.Status.TimeoutConfig.Global.Duration).To(Equal(time.Hour))
			Expect(rr.Status.TimeoutConfig.Processing.Duration).To(Equal(10 * time.Minute))
			Expect(rr.Status.TimeoutConfig.Analyzing.Duration).To(Equal(15 * time.Minute))
		})
	})

	// ========================================
	// ERROR HANDLING INTEGRATION TESTS
	// ========================================
	Context("INTEGRATION-ERROR-01: Missing gateway event", func() {
		It("should return error when only orchestrator event exists", func() {
			// ARRANGE: Seed only orchestrator event (no gateway event)
			orchestratorEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-123",
				EventData: map[string]interface{}{
					"event_type": "orchestrator.lifecycle.created", // Required for discriminator
					"rr_name":    "test-rr",                        // Required field
					"namespace":  "default",                        // Required field
					"timeout_config": map[string]interface{}{
						"global": "1h",
					},
				},
			}

			_, err := auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Call business logic components directly

			// Step 1: Query events
			events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
			Expect(err).ToNot(HaveOccurred())
			Expect(events).To(HaveLen(1))

			// Step 2: Parse events
			parsedData := make([]reconstruction.ParsedAuditData, 0, len(events))
			for _, event := range events {
				parsed, err := reconstruction.ParseAuditEvent(event)
				Expect(err).ToNot(HaveOccurred())
				parsedData = append(parsedData, *parsed)
			}

			// Step 3: Map should fail due to missing gateway event
			_, err = reconstruction.MergeAuditData(parsedData)

			// ASSERT: Error indicates missing gateway event
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gateway.signal.received"))
		})
	})

	Context("INTEGRATION-VALIDATION-01: Incomplete reconstruction", func() {
		It("should report low completeness when fields are missing", func() {
			// ARRANGE: Seed minimal gateway event (missing optional fields)
			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "signal-123",
				EventData: map[string]interface{}{
					"event_type":  "gateway.signal.received", // Required for discriminator
					"signal_type": "prometheus-alert",
					"alert_name":  "HighCPU",     // Required field
					"namespace":   "default",     // Required field
					"fingerprint": "test-fp-789", // Required field
					// Missing: signal_labels, signal_annotations, original_payload (intentional for incomplete validation test)
				},
			}

			_, err := auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Full reconstruction pipeline
			events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
			Expect(err).ToNot(HaveOccurred())

			parsedData := make([]reconstruction.ParsedAuditData, 0, len(events))
			for _, event := range events {
				parsed, err := reconstruction.ParseAuditEvent(event)
				Expect(err).ToNot(HaveOccurred())
				parsedData = append(parsedData, *parsed)
			}

			rrFields, err := reconstruction.MergeAuditData(parsedData)
			Expect(err).ToNot(HaveOccurred())

			rr, err := reconstruction.BuildRemediationRequest(correlationID, rrFields)
			Expect(err).ToNot(HaveOccurred())

			// Step 5: Validate - should report low completeness
			validationResult, err := reconstruction.ValidateReconstructedRR(rr)
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Low completeness due to missing fields
			Expect(validationResult.Completeness).To(BeNumerically("<", 80))
			Expect(validationResult.Warnings).ToNot(BeEmpty())
		})
	})

	// NOTE: Additional integration tests for database constraints,
	// concurrent access, and transaction handling can be added here
})
