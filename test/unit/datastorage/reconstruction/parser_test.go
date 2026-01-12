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

package reconstruction_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
)

// BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
// This test validates the parser component that extracts structured data from audit event payloads.
var _ = Describe("Audit Event Parser", func() {
	var (
		testTimestamp time.Time
		testUUID      uuid.UUID
	)

	BeforeEach(func() {
		testTimestamp = time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC)
		testUUID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	})

	Context("ParseAuditEvent - gateway.signal.received", func() {
		It("should extract signal type, labels, and annotations", func() {
			// TDD RED: This test MUST fail because ParseAuditEvent doesn't exist yet
			event := createGatewaySignalReceivedEvent(testTimestamp, testUUID)

			parsedData, err := reconstruction.ParseAuditEvent(event)

			Expect(err).ToNot(HaveOccurred())
			Expect(parsedData).ToNot(BeNil())
			Expect(parsedData.SignalType).To(Equal("prometheus-alert"))
			Expect(parsedData.AlertName).To(Equal("HighCPU"))
			Expect(parsedData.SignalLabels).To(HaveKeyWithValue("alertname", "HighCPU"))
			Expect(parsedData.SignalAnnotations).To(HaveKeyWithValue("summary", "CPU usage is high"))
		})

		It("should return error for missing alert name", func() {
			// TDD RED: Test validation error handling
			event := createInvalidGatewayEvent(testTimestamp, testUUID)

			_, err := reconstruction.ParseAuditEvent(event)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing alert_name"))
		})
	})

	Context("ParseAuditEvent - orchestrator.lifecycle.created", func() {
		It("should extract TimeoutConfig with all phases", func() {
			// TDD RED: Test TimeoutConfig parsing
			event := createOrchestratorLifecycleCreatedEvent(testTimestamp, testUUID)

			parsedData, err := reconstruction.ParseAuditEvent(event)

			Expect(err).ToNot(HaveOccurred())
			Expect(parsedData).ToNot(BeNil())
			Expect(parsedData.TimeoutConfig).ToNot(BeNil())
			Expect(parsedData.TimeoutConfig.Global).To(Equal("1h0m0s"))
			Expect(parsedData.TimeoutConfig.Processing).To(Equal("10m0s"))
			Expect(parsedData.TimeoutConfig.Analyzing).To(Equal("15m0s"))
		})

		It("should handle missing optional timeout fields", func() {
			// TDD RED: Test partial TimeoutConfig
			event := createOrchestratorEventWithPartialTimeout(testTimestamp, testUUID)

			parsedData, err := reconstruction.ParseAuditEvent(event)

			Expect(err).ToNot(HaveOccurred())
			Expect(parsedData.TimeoutConfig).ToNot(BeNil())
			Expect(parsedData.TimeoutConfig.Global).To(Equal("1h0m0s"))
			// Optional fields should be empty strings, not errors
			Expect(parsedData.TimeoutConfig.Processing).To(Equal(""))
		})
	})

	// NOTE: Additional event type tests (workflow, webhook, errors) will be added
	// during GREEN phase implementation when we understand the actual audit payload structure

	// NOTE: Additional event type tests will be added during GREEN phase implementation
})

// Test fixture factories
func createGatewaySignalReceivedEvent(timestamp time.Time, id uuid.UUID) ogenclient.AuditEvent {
	labels := ogenclient.GatewayAuditPayloadSignalLabels{"alertname": "HighCPU"}
	annotations := ogenclient.GatewayAuditPayloadSignalAnnotations{"summary": "CPU usage is high"}

	return ogenclient.AuditEvent{
		Version:        "1.0",
		EventType:      "gateway.signal.received",
		EventTimestamp: timestamp,
		EventCategory:  ogenclient.AuditEventEventCategoryGateway,
		EventAction:    "signal.received",
		EventOutcome:   ogenclient.AuditEventEventOutcomeSuccess,
		CorrelationID:  "test-correlation-id",
		EventID:        ogenclient.NewOptUUID(id),
		EventData: ogenclient.AuditEventEventData{
			Type: ogenclient.AuditEventEventDataGatewaySignalReceivedAuditEventEventData,
			GatewayAuditPayload: ogenclient.GatewayAuditPayload{
				EventType:         ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				SignalType:        ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
				AlertName:         "HighCPU",
				Fingerprint:       "test-fingerprint-123",
				Namespace:         "default",
				SignalLabels:      ogenclient.NewOptGatewayAuditPayloadSignalLabels(labels),
				SignalAnnotations: ogenclient.NewOptGatewayAuditPayloadSignalAnnotations(annotations),
			},
		},
	}
}

func createInvalidGatewayEvent(timestamp time.Time, id uuid.UUID) ogenclient.AuditEvent {
	return ogenclient.AuditEvent{
		Version:        "1.0",
		EventType:      "gateway.signal.received",
		EventTimestamp: timestamp,
		EventCategory:  ogenclient.AuditEventEventCategoryGateway,
		EventAction:    "signal.received",
		EventOutcome:   ogenclient.AuditEventEventOutcomeFailure,
		CorrelationID:  "test-correlation-id",
		EventID:        ogenclient.NewOptUUID(id),
		EventData: ogenclient.AuditEventEventData{
			Type: ogenclient.AuditEventEventDataGatewaySignalReceivedAuditEventEventData,
			GatewayAuditPayload: ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
				AlertName:   "", // Missing alert_name - should cause error
				Fingerprint: "test-fingerprint-456",
				Namespace:   "default",
			},
		},
	}
}

func createOrchestratorLifecycleCreatedEvent(timestamp time.Time, id uuid.UUID) ogenclient.AuditEvent {
	tc := ogenclient.TimeoutConfig{
		Global:     ogenclient.NewOptString("1h0m0s"),
		Processing: ogenclient.NewOptString("10m0s"),
		Analyzing:  ogenclient.NewOptString("15m0s"),
		Executing:  ogenclient.NewOptString("30m0s"),
	}

	return ogenclient.AuditEvent{
		Version:        "1.0",
		EventType:      "orchestrator.lifecycle.created",
		EventTimestamp: timestamp,
		EventCategory:  ogenclient.AuditEventEventCategoryOrchestration,
		EventAction:    "lifecycle.created",
		EventOutcome:   ogenclient.AuditEventEventOutcomeSuccess,
		CorrelationID:  "test-correlation-id",
		EventID:        ogenclient.NewOptUUID(id),
		EventData: ogenclient.AuditEventEventData{
			Type: ogenclient.AuditEventEventDataOrchestratorLifecycleCreatedAuditEventEventData,
			RemediationOrchestratorAuditPayload: ogenclient.RemediationOrchestratorAuditPayload{
				EventType:     ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated,
				TimeoutConfig: ogenclient.OptTimeoutConfig{Value: tc, Set: true},
			},
		},
	}
}

func createOrchestratorEventWithPartialTimeout(timestamp time.Time, id uuid.UUID) ogenclient.AuditEvent {
	tc := ogenclient.TimeoutConfig{
		Global: ogenclient.NewOptString("1h0m0s"),
		// Other fields not set (testing partial TimeoutConfig)
	}

	return ogenclient.AuditEvent{
		Version:        "1.0",
		EventType:      "orchestrator.lifecycle.created",
		EventTimestamp: timestamp,
		EventCategory:  ogenclient.AuditEventEventCategoryOrchestration,
		EventAction:    "lifecycle.created",
		EventOutcome:   ogenclient.AuditEventEventOutcomeSuccess,
		CorrelationID:  "test-correlation-id",
		EventID:        ogenclient.NewOptUUID(id),
		EventData: ogenclient.AuditEventEventData{
			Type: ogenclient.AuditEventEventDataOrchestratorLifecycleCreatedAuditEventEventData,
			RemediationOrchestratorAuditPayload: ogenclient.RemediationOrchestratorAuditPayload{
				EventType:     ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated,
				TimeoutConfig: ogenclient.OptTimeoutConfig{Value: tc, Set: true},
			},
		},
	}
}
