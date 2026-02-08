/*
Copyright 2026 Jordi Gil.

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

package audit

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	auditpkg "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// F-3 SOC2 Fix: Event Type Consistency Validation Tests
//
// Validates that StoreAudit() rejects events where the outer event_type
// field does not match the EventData discriminator type.
// This prevents spec drift between event_type (plain string) and
// EventData.Type (typed enum from OpenAPI discriminator).

// buildTestEventWithTypes creates a fully populated audit event for consistency testing.
func buildTestEventWithTypes(
	outerEventType string,
	eventData ogenclient.AuditEventRequestEventData,
) *ogenclient.AuditEventRequest {
	event := auditpkg.NewAuditEventRequest()
	auditpkg.SetEventType(event, outerEventType)
	auditpkg.SetEventCategory(event, "gateway")
	auditpkg.SetEventAction(event, "test_action")
	auditpkg.SetEventOutcome(event, auditpkg.OutcomeSuccess)
	auditpkg.SetActor(event, "service", "test-service")
	auditpkg.SetResource(event, "TestResource", "test-001")
	auditpkg.SetCorrelationID(event, "corr-consistency-test")
	auditpkg.SetNamespace(event, "default")
	event.EventData = eventData
	return event
}

var _ = Describe("F-3: Event Type / EventData Discriminator Consistency", func() {
	var (
		ctx    context.Context
		store  auditpkg.AuditStore
		client *MockDataStorageClient
		logger logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = NewMockDataStorageClient()
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())
		var err error
		store, err = auditpkg.NewBufferedStore(client, auditpkg.DefaultConfig(), "test-service", logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if store != nil {
			_ = store.Close()
		}
	})

	DescribeTable("should reject events with mismatched event_type and EventData.Type",
		func(outerEventType string, eventData ogenclient.AuditEventRequestEventData) {
			event := buildTestEventWithTypes(outerEventType, eventData)

			err := store.StoreAudit(ctx, event)
			Expect(err).To(HaveOccurred(),
				"StoreAudit must reject events with mismatched event_type and EventData.Type")
			Expect(err.Error()).To(ContainSubstring("event_type mismatch"))
		},
		Entry("gateway event_type with orchestrator EventData",
			"gateway.signal.received",
			ogenclient.NewAuditEventRequestEventDataOrchestratorLifecycleStartedAuditEventRequestEventData(
				ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleStarted,
					RrName:    "rr-001",
					Namespace: "default",
				},
			),
		),
		Entry("F-3 pre-fix scenario: workflowexecution event_type with orchestrator EventData",
			"workflowexecution.block.cleared",
			ogenclient.NewAuditEventRequestEventDataOrchestratorLifecycleStartedAuditEventRequestEventData(
				ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleStarted,
					RrName:    "rr-001",
					Namespace: "default",
				},
			),
		),
		Entry("orchestrator event_type with gateway EventData",
			"orchestrator.lifecycle.started",
			ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(
				ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
					AlertName:   "test-alert",
					Namespace:   "default",
					Fingerprint: "test-fp",
				},
			),
		),
	)

	It("should accept events where event_type matches EventData.Type", func() {
		payload := ogenclient.RemediationOrchestratorAuditPayload{
			EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleStarted,
			RrName:    "rr-001",
			Namespace: "default",
		}
		eventData := ogenclient.NewAuditEventRequestEventDataOrchestratorLifecycleStartedAuditEventRequestEventData(payload)
		event := buildTestEventWithTypes("orchestrator.lifecycle.started", eventData)
		auditpkg.SetEventCategory(event, "orchestration")

		err := store.StoreAudit(ctx, event)
		Expect(err).ToNot(HaveOccurred(),
			"StoreAudit must accept events with matching event_type and EventData.Type")
	})
})
