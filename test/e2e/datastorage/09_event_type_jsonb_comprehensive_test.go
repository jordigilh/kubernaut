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
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// GAP 1.1: COMPREHENSIVE EVENT TYPE + JSONB VALIDATION
// ========================================
//
// Business Requirement: BR-STORAGE-001 (Audit persistence), BR-STORAGE-032 (Unified audit trail)
// Gap Analysis: TRIAGE_DS_TEST_COVERAGE_GAP_ANALYSIS_V3.md - Gap 1.1
// Authority: api/openapi/data-storage-v1.yaml (lines 1614-1649) - OpenAPI discriminator mapping
// Compliance: DD-AUDIT-004 (zero unstructured data)
// Priority: P0
// Estimated Effort: 7 hours
// Confidence: 100%
//
// BUSINESS OUTCOME:
// DS accepts ALL 36 valid event types from OpenAPI schema AND validates JSONB queryability
//
// CURRENT REALITY (BEFORE THIS FIX):
// - Test validated 21 deprecated event types (only 5 matched OpenAPI schema)
// - Used unstructured map[string]interface{} (violated DD-AUDIT-004)
// - Test alignment: 23.8% (5/21 valid types)
// - OpenAPI coverage: 13.9% (5/36 types tested)
//
// AFTER THIS FIX:
// - Test validates 36 current event types from OpenAPI discriminator mapping
// - Uses structured ogenclient types (DD-AUDIT-004 compliant)
// - Test alignment: 100% (36/36 valid types)
// - OpenAPI coverage: 100% (36/36 types tested)
//
// TDD RED PHASE: Tests define contract for all 36 valid event types from OpenAPI
// ========================================

// eventTypeTestCase defines a complete test case for one event type using structured types
type eventTypeTestCase struct {
	Service       string
	EventType     string
	EventCategory ogenclient.AuditEventRequestEventCategory
	EventAction   string
	CreateEvent   func() ogenclient.AuditEventRequest // Factory function for type-safe event creation
	JSONBQueries  []jsonbQueryTest
}

// jsonbQueryTest defines a JSONB query to validate
type jsonbQueryTest struct {
	Field        string
	Operator     string // "->>" (text) or "->" (JSON)
	Value        string
	ExpectedRows int
}

// OpenAPI Event Type Catalog - ALL 36 valid event types from api/openapi/data-storage-v1.yaml
// Authority: api/openapi/data-storage-v1.yaml lines 1614-1649 (discriminator mapping)
// Last Verified: 2026-01-18
// Compliance: DD-AUDIT-004 (zero unstructured data - all payloads use ogenclient types)
var eventTypeCatalog = []eventTypeTestCase{
	// ========================================
	// GATEWAY SERVICE (4 event types)
	// ========================================
	{
		Service:       "gateway",
		EventType:     "gateway.signal.received",
		EventCategory: ogenclient.AuditEventRequestEventCategoryGateway,
		EventAction:   "received",
		CreateEvent: func() ogenclient.AuditEventRequest {
			correlationID := fmt.Sprintf("test-gap-1.1-gateway-received-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "gateway.signal.received",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
				EventAction:    "received",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("gateway-service"),
				ResourceType:   ogenclient.NewOptString("Signal"),
				ResourceID:     ogenclient.NewOptString("fp-abc123"),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData(ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
					SignalType:  "prometheus-alert",
					AlertName:   "HighCPU",
					Namespace:   "production",
					Fingerprint: "fp-abc123",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "alert_name", Operator: "->>", Value: "HighCPU", ExpectedRows: 1},
			{Field: "fingerprint", Operator: "->>", Value: "fp-abc123", ExpectedRows: 1},
		},
	},
	{
		Service:       "gateway",
		EventType:     "gateway.signal.deduplicated",
		EventCategory: ogenclient.AuditEventRequestEventCategoryGateway,
		EventAction:   "deduplicated",
		CreateEvent: func() ogenclient.AuditEventRequest {
			correlationID := fmt.Sprintf("test-gap-1.1-gateway-deduplicated-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "gateway.signal.deduplicated",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
				EventAction:    "deduplicated",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("gateway-service"),
				ResourceType:   ogenclient.NewOptString("Signal"),
				ResourceID:     ogenclient.NewOptString("fp-dedupe-456"),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataGatewaySignalDeduplicatedAuditEventRequestEventData(ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalDeduplicated,
					SignalType:  "prometheus-alert",
					AlertName:   "HighCPU",
					Namespace:   "production",
					Fingerprint: "fp-dedupe-456",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "alert_name", Operator: "->>", Value: "HighCPU", ExpectedRows: 1},
			{Field: "fingerprint", Operator: "->>", Value: "fp-dedupe-456", ExpectedRows: 1},
		},
	},
	{
		Service:       "gateway",
		EventType:     "gateway.crd.created",
		EventCategory: ogenclient.AuditEventRequestEventCategoryGateway,
		EventAction:   "crd_created",
		CreateEvent: func() ogenclient.AuditEventRequest {
			correlationID := fmt.Sprintf("test-gap-1.1-gateway-crd-created-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "gateway.crd.created",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
				EventAction:    "crd_created",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("gateway-service"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString("rr-test-001"),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
					SignalType:  "kubernetes-event",
					AlertName:   "CRDCreated",
					Namespace:   "kubernaut-system",
					Fingerprint: "fp-crd-012",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "alert_name", Operator: "->>", Value: "CRDCreated", ExpectedRows: 1},
			{Field: "fingerprint", Operator: "->>", Value: "fp-crd-012", ExpectedRows: 1},
		},
	},
	{
		Service:       "gateway",
		EventType:     "gateway.crd.failed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryGateway,
		EventAction:   "crd_failed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			correlationID := fmt.Sprintf("test-gap-1.1-gateway-crd-failed-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "gateway.crd.failed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
				EventAction:    "crd_failed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeFailure,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("gateway-service"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString("rr-test-fail-002"),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataGatewayCrdFailedAuditEventRequestEventData(ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdFailed,
					SignalType:  "kubernetes-event",
					AlertName:   "CRDCreationFailed",
					Namespace:   "kubernaut-system",
					Fingerprint: "fp-crd-fail-789",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "alert_name", Operator: "->>", Value: "CRDCreationFailed", ExpectedRows: 1},
			{Field: "fingerprint", Operator: "->>", Value: "fp-crd-fail-789", ExpectedRows: 1},
		},
	},

	// ========================================
	// REMEDIATION ORCHESTRATOR SERVICE (8 event types)
	// ========================================
	{
		Service:       "remediation-orchestrator",
		EventType:     "orchestrator.lifecycle.started",
		EventCategory: ogenclient.AuditEventRequestEventCategoryOrchestration,
		EventAction:   "started",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-orchestrator-started-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "orchestrator.lifecycle.started",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryOrchestration,
				EventAction:    "started",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("remediation-orchestrator-service"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  rrName, // Per DD-AUDIT-CORRELATION-002
				EventData: ogenclient.NewAuditEventRequestEventDataOrchestratorLifecycleStartedAuditEventRequestEventData(ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleStarted,
					RrName:    rrName,
					Namespace: "default",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'namespace' instead of 'phase' (which doesn't exist in schema)
			// Per OpenAPI schema: RemediationOrchestratorAuditPayload has required fields: rr_name, namespace, event_type
			{Field: "namespace", Operator: "->>", Value: "default", ExpectedRows: 1},
		},
	},
	{
		Service:       "remediation-orchestrator",
		EventType:     "orchestrator.lifecycle.created",
		EventCategory: ogenclient.AuditEventRequestEventCategoryOrchestration,
		EventAction:   "created",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-orchestrator-created-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "orchestrator.lifecycle.created",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryOrchestration,
				EventAction:    "created",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("remediation-orchestrator-service"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  rrName,
				EventData: ogenclient.NewAuditEventRequestEventDataOrchestratorLifecycleCreatedAuditEventRequestEventData(ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated,
					RrName:    rrName,
					Namespace: "default",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'namespace' instead of 'phase' (which doesn't exist in schema)
			{Field: "namespace", Operator: "->>", Value: "default", ExpectedRows: 1},
		},
	},
	{
		Service:       "remediation-orchestrator",
		EventType:     "orchestrator.lifecycle.completed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryOrchestration,
		EventAction:   "completed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-orchestrator-completed-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "orchestrator.lifecycle.completed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryOrchestration,
				EventAction:    "completed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("remediation-orchestrator-service"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  rrName,
				EventData: ogenclient.NewAuditEventRequestEventDataOrchestratorLifecycleCompletedAuditEventRequestEventData(ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted,
					RrName:    rrName,
					Namespace: "default",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'namespace' instead of 'phase' (which doesn't exist in schema)
			{Field: "namespace", Operator: "->>", Value: "default", ExpectedRows: 1},
		},
	},
	{
		Service:       "remediation-orchestrator",
		EventType:     "orchestrator.lifecycle.failed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryOrchestration,
		EventAction:   "failed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-orchestrator-failed-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "orchestrator.lifecycle.failed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryOrchestration,
				EventAction:    "failed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeFailure,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("remediation-orchestrator-service"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  rrName,
				EventData: ogenclient.NewAuditEventRequestEventDataOrchestratorLifecycleFailedAuditEventRequestEventData(ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleFailed,
					RrName:    rrName,
					Namespace: "default",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'namespace' instead of 'phase' (which doesn't exist in schema)
			{Field: "namespace", Operator: "->>", Value: "default", ExpectedRows: 1},
		},
	},
	{
		Service:       "remediation-orchestrator",
		EventType:     "orchestrator.lifecycle.transitioned",
		EventCategory: ogenclient.AuditEventRequestEventCategoryOrchestration,
		EventAction:   "transitioned",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-orchestrator-transitioned-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "orchestrator.lifecycle.transitioned",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryOrchestration,
				EventAction:    "transitioned",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("remediation-orchestrator-service"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  rrName,
				EventData: ogenclient.NewAuditEventRequestEventDataOrchestratorLifecycleTransitionedAuditEventRequestEventData(ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleTransitioned,
					RrName:    rrName,
					Namespace: "default",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'namespace' instead of 'phase' (which doesn't exist in schema)
			{Field: "namespace", Operator: "->>", Value: "default", ExpectedRows: 1},
		},
	},
	{
		Service:       "remediation-orchestrator",
		EventType:     "orchestrator.approval.requested",
		EventCategory: ogenclient.AuditEventRequestEventCategoryOrchestration,
		EventAction:   "requested",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-orchestrator-approval-req-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "orchestrator.approval.requested",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryOrchestration,
				EventAction:    "requested",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("remediation-orchestrator-service"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  rrName,
				EventData: ogenclient.NewAuditEventRequestEventDataOrchestratorApprovalRequestedAuditEventRequestEventData(ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalRequested,
					RrName:    rrName,
					Namespace: "default",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'namespace' instead of 'phase' (which doesn't exist in schema)
			{Field: "namespace", Operator: "->>", Value: "default", ExpectedRows: 1},
		},
	},
	{
		Service:       "remediation-orchestrator",
		EventType:     "orchestrator.approval.approved",
		EventCategory: ogenclient.AuditEventRequestEventCategoryOrchestration,
		EventAction:   "approved",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-orchestrator-approval-approved-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "orchestrator.approval.approved",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryOrchestration,
				EventAction:    "approved",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("user"),
				ActorID:        ogenclient.NewOptString("admin@example.com"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  rrName,
				EventData: ogenclient.NewAuditEventRequestEventDataOrchestratorApprovalApprovedAuditEventRequestEventData(ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalApproved,
					RrName:    rrName,
					Namespace: "default",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'namespace' instead of 'phase' (which doesn't exist in schema)
			{Field: "namespace", Operator: "->>", Value: "default", ExpectedRows: 1},
		},
	},
	{
		Service:       "remediation-orchestrator",
		EventType:     "orchestrator.approval.rejected",
		EventCategory: ogenclient.AuditEventRequestEventCategoryOrchestration,
		EventAction:   "rejected",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-orchestrator-approval-rejected-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "orchestrator.approval.rejected",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryOrchestration,
				EventAction:    "rejected",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeFailure,
				ActorType:      ogenclient.NewOptString("user"),
				ActorID:        ogenclient.NewOptString("admin@example.com"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  rrName,
				EventData: ogenclient.NewAuditEventRequestEventDataOrchestratorApprovalRejectedAuditEventRequestEventData(ogenclient.RemediationOrchestratorAuditPayload{
					EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalRejected,
					RrName:    rrName,
					Namespace: "default",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'namespace' instead of 'phase' (which doesn't exist in schema)
			{Field: "namespace", Operator: "->>", Value: "default", ExpectedRows: 1},
		},
	},

	// ========================================
	// SIGNAL PROCESSING SERVICE (6 event types)
	// ========================================
	{
		Service:       "signalprocessing",
		EventType:     "signalprocessing.signal.processed",
		EventCategory: ogenclient.AuditEventRequestEventCategorySignalprocessing,
		EventAction:   "processed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			signalName := fmt.Sprintf("sp-test-processed-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-sp-processed-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "signalprocessing.signal.processed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategorySignalprocessing,
				EventAction:    "processed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("signalprocessing-service"),
				ResourceType:   ogenclient.NewOptString("SignalProcessing"),
				ResourceID:     ogenclient.NewOptString(signalName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataSignalprocessingSignalProcessedAuditEventRequestEventData(ogenclient.SignalProcessingAuditPayload{
					EventType: ogenclient.SignalProcessingAuditPayloadEventTypeSignalprocessingSignalProcessed,
					Phase:     ogenclient.SignalProcessingAuditPayloadPhaseCompleted,
					Signal:    signalName,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Completed", ExpectedRows: 1},
		},
	},
	{
		Service:       "signalprocessing",
		EventType:     "signalprocessing.phase.transition",
		EventCategory: ogenclient.AuditEventRequestEventCategorySignalprocessing,
		EventAction:   "transition",
		CreateEvent: func() ogenclient.AuditEventRequest {
			signalName := fmt.Sprintf("sp-test-transition-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-sp-transition-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "signalprocessing.phase.transition",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategorySignalprocessing,
				EventAction:    "transition",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("signalprocessing-service"),
				ResourceType:   ogenclient.NewOptString("SignalProcessing"),
				ResourceID:     ogenclient.NewOptString(signalName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataSignalprocessingPhaseTransitionAuditEventRequestEventData(ogenclient.SignalProcessingAuditPayload{
					EventType: ogenclient.SignalProcessingAuditPayloadEventTypeSignalprocessingPhaseTransition,
					Phase:     ogenclient.SignalProcessingAuditPayloadPhaseEnriching,
					Signal:    signalName,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Enriching", ExpectedRows: 1},
		},
	},
	{
		Service:       "signalprocessing",
		EventType:     "signalprocessing.classification.decision",
		EventCategory: ogenclient.AuditEventRequestEventCategorySignalprocessing,
		EventAction:   "decision",
		CreateEvent: func() ogenclient.AuditEventRequest {
			signalName := fmt.Sprintf("sp-test-classification-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-sp-classification-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "signalprocessing.classification.decision",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategorySignalprocessing,
				EventAction:    "decision",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("signalprocessing-service"),
				ResourceType:   ogenclient.NewOptString("SignalProcessing"),
				ResourceID:     ogenclient.NewOptString(signalName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataSignalprocessingClassificationDecisionAuditEventRequestEventData(ogenclient.SignalProcessingAuditPayload{
					EventType: ogenclient.SignalProcessingAuditPayloadEventTypeSignalprocessingClassificationDecision,
					Phase:     ogenclient.SignalProcessingAuditPayloadPhaseClassifying,
					Signal:    signalName,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Classifying", ExpectedRows: 1},
		},
	},
	{
		Service:       "signalprocessing",
		EventType:     "signalprocessing.business.classified",
		EventCategory: ogenclient.AuditEventRequestEventCategorySignalprocessing,
		EventAction:   "classified",
		CreateEvent: func() ogenclient.AuditEventRequest {
			signalName := fmt.Sprintf("sp-test-business-classified-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-sp-business-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "signalprocessing.business.classified",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategorySignalprocessing,
				EventAction:    "classified",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("signalprocessing-service"),
				ResourceType:   ogenclient.NewOptString("SignalProcessing"),
				ResourceID:     ogenclient.NewOptString(signalName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataSignalprocessingBusinessClassifiedAuditEventRequestEventData(ogenclient.SignalProcessingAuditPayload{
					EventType: ogenclient.SignalProcessingAuditPayloadEventTypeSignalprocessingBusinessClassified,
					Phase:     ogenclient.SignalProcessingAuditPayloadPhaseCategorizing,
					Signal:    signalName,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Categorizing", ExpectedRows: 1},
		},
	},
	{
		Service:       "signalprocessing",
		EventType:     "signalprocessing.enrichment.completed",
		EventCategory: ogenclient.AuditEventRequestEventCategorySignalprocessing,
		EventAction:   "completed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			signalName := fmt.Sprintf("sp-test-enrichment-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-sp-enrichment-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "signalprocessing.enrichment.completed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategorySignalprocessing,
				EventAction:    "completed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("signalprocessing-service"),
				ResourceType:   ogenclient.NewOptString("SignalProcessing"),
				ResourceID:     ogenclient.NewOptString(signalName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataSignalprocessingEnrichmentCompletedAuditEventRequestEventData(ogenclient.SignalProcessingAuditPayload{
					EventType: ogenclient.SignalProcessingAuditPayloadEventTypeSignalprocessingEnrichmentCompleted,
					Phase:     ogenclient.SignalProcessingAuditPayloadPhaseCompleted,
					Signal:    signalName,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Completed", ExpectedRows: 1},
		},
	},
	{
		Service:       "signalprocessing",
		EventType:     "signalprocessing.error.occurred",
		EventCategory: ogenclient.AuditEventRequestEventCategorySignalprocessing,
		EventAction:   "error",
		CreateEvent: func() ogenclient.AuditEventRequest {
			signalName := fmt.Sprintf("sp-test-error-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-sp-error-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "signalprocessing.error.occurred",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategorySignalprocessing,
				EventAction:    "error",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeFailure,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("signalprocessing-service"),
				ResourceType:   ogenclient.NewOptString("SignalProcessing"),
				ResourceID:     ogenclient.NewOptString(signalName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataSignalprocessingErrorOccurredAuditEventRequestEventData(ogenclient.SignalProcessingAuditPayload{
					EventType: ogenclient.SignalProcessingAuditPayloadEventTypeSignalprocessingErrorOccurred,
					Phase:     ogenclient.SignalProcessingAuditPayloadPhaseFailed,
					Signal:    signalName,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Failed", ExpectedRows: 1},
		},
	},

	// ========================================
	// AI ANALYSIS SERVICE (2 event types)
	// ========================================
	{
		Service:       "aianalysis",
		EventType:     "aianalysis.analysis.completed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryAnalysis,
		EventAction:   "completed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			analysisName := fmt.Sprintf("aa-test-completed-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-ai-completed-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aianalysis.analysis.completed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAnalysis,
				EventAction:    "completed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("aianalysis-service"),
				ResourceType:   ogenclient.NewOptString("AIAnalysis"),
				ResourceID:     ogenclient.NewOptString(analysisName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData(ogenclient.AIAnalysisAuditPayload{
					EventType:        ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
					AnalysisName:     analysisName,
					Namespace:        "default",
					Phase:            ogenclient.AIAnalysisAuditPayloadPhaseCompleted,
					ApprovalRequired: false,
					DegradedMode:     false,
					WarningsCount:    0,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'phase' = 'Completed' to match the payload we're sending
			{Field: "phase", Operator: "->>", Value: "Completed", ExpectedRows: 1},
		},
	},
	{
		Service:       "aianalysis",
		EventType:     "aianalysis.analysis.failed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryAnalysis,
		EventAction:   "failed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			analysisName := fmt.Sprintf("aa-test-failed-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-ai-failed-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aianalysis.analysis.failed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAnalysis,
				EventAction:    "failed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeFailure,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("aianalysis-service"),
				ResourceType:   ogenclient.NewOptString("AIAnalysis"),
				ResourceID:     ogenclient.NewOptString(analysisName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataAianalysisAnalysisFailedAuditEventRequestEventData(ogenclient.AIAnalysisAuditPayload{
					EventType:        ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisFailed,
					AnalysisName:     analysisName,
					Namespace:        "default",
					Phase:            ogenclient.AIAnalysisAuditPayloadPhaseFailed,
					ApprovalRequired: false,
					DegradedMode:     false,
					WarningsCount:    0,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Failed", ExpectedRows: 1},
		},
	},

	// ========================================
	// WORKFLOW EXECUTION SERVICE (5 event types)
	// ========================================
	{
		Service:       "workflowexecution",
		EventType:     "workflowexecution.workflow.started",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
		EventAction:   "started",
		CreateEvent: func() ogenclient.AuditEventRequest {
			wfeName := fmt.Sprintf("wfe-test-started-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-wfe-started-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "workflowexecution.workflow.started",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
				EventAction:    "started",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("workflowexecution-service"),
				ResourceType:   ogenclient.NewOptString("WorkflowExecution"),
				ResourceID:     ogenclient.NewOptString(wfeName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowStartedAuditEventRequestEventData(ogenclient.WorkflowExecutionAuditPayload{
					EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowStarted,
					Phase:           ogenclient.WorkflowExecutionAuditPayloadPhasePending,
					ExecutionName:   wfeName,
					WorkflowID:      "kubectl-restart-deployment-e2e",
					WorkflowVersion: "v1.0.0",
					TargetResource:  "default/deployment/test-deployment",
					ContainerImage:  "ghcr.io/kubernaut/kubectl-actions:v1.28",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Pending", ExpectedRows: 1},
		},
	},
	{
		Service:       "workflowexecution",
		EventType:     "workflowexecution.workflow.completed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
		EventAction:   "completed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			wfeName := fmt.Sprintf("wfe-test-completed-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-wfe-completed-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "workflowexecution.workflow.completed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
				EventAction:    "completed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("workflowexecution-service"),
				ResourceType:   ogenclient.NewOptString("WorkflowExecution"),
				ResourceID:     ogenclient.NewOptString(wfeName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData(ogenclient.WorkflowExecutionAuditPayload{
					EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowCompleted,
					Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseCompleted,
					ExecutionName:   wfeName,
					WorkflowID:      "kubectl-restart-deployment-e2e",
					WorkflowVersion: "v1.0.0",
					TargetResource:  "default/deployment/test-deployment",
					ContainerImage:  "ghcr.io/kubernaut/kubectl-actions:v1.28",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Completed", ExpectedRows: 1},
		},
	},
	{
		Service:       "workflowexecution",
		EventType:     "workflowexecution.workflow.failed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
		EventAction:   "failed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			wfeName := fmt.Sprintf("wfe-test-failed-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-wfe-failed-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "workflowexecution.workflow.failed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
				EventAction:    "failed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeFailure,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("workflowexecution-service"),
				ResourceType:   ogenclient.NewOptString("WorkflowExecution"),
				ResourceID:     ogenclient.NewOptString(wfeName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataWorkflowexecutionWorkflowFailedAuditEventRequestEventData(ogenclient.WorkflowExecutionAuditPayload{
					EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowFailed,
					Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseFailed,
					ExecutionName:   wfeName,
					WorkflowID:      "kubectl-restart-deployment-e2e",
					WorkflowVersion: "v1.0.0",
					TargetResource:  "default/deployment/test-deployment",
					ContainerImage:  "ghcr.io/kubernaut/kubectl-actions:v1.28",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Failed", ExpectedRows: 1},
		},
	},
	{
		Service:       "workflowexecution",
		EventType:     "workflowexecution.selection.completed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
		EventAction:   "selection_completed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			wfeName := fmt.Sprintf("wfe-test-selection-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-wfe-selection-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "workflowexecution.selection.completed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
				EventAction:    "selection_completed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("workflowexecution-service"),
				ResourceType:   ogenclient.NewOptString("WorkflowExecution"),
				ResourceID:     ogenclient.NewOptString(wfeName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataWorkflowexecutionSelectionCompletedAuditEventRequestEventData(ogenclient.WorkflowExecutionAuditPayload{
					EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionSelectionCompleted,
					Phase:           ogenclient.WorkflowExecutionAuditPayloadPhasePending,
					ExecutionName:   wfeName,
					WorkflowID:      "kubectl-restart-deployment-e2e",
					WorkflowVersion: "v1.0.0",
					TargetResource:  "default/deployment/test-deployment",
					ContainerImage:  "ghcr.io/kubernaut/kubectl-actions:v1.28",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "phase", Operator: "->>", Value: "Pending", ExpectedRows: 1},
		},
	},
	{
		Service:       "workflowexecution",
		EventType:     "workflowexecution.execution.started",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
		EventAction:   "execution_started",
		CreateEvent: func() ogenclient.AuditEventRequest {
			wfeName := fmt.Sprintf("wfe-test-exec-started-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-wfe-exec-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "workflowexecution.execution.started",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflowexecution,
				EventAction:    "execution_started",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("workflowexecution-service"),
				ResourceType:   ogenclient.NewOptString("WorkflowExecution"),
				ResourceID:     ogenclient.NewOptString(wfeName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataWorkflowexecutionExecutionStartedAuditEventRequestEventData(ogenclient.WorkflowExecutionAuditPayload{
					EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted,
					ExecutionName:   wfeName,
					WorkflowID:      "kubectl-restart-deployment-e2e",
					WorkflowVersion: "v1.0.0",
					TargetResource:  "default/deployment/test-deployment",
					Phase:           ogenclient.WorkflowExecutionAuditPayloadPhasePending,
					ContainerImage:  "ghcr.io/kubernaut/kubectl-actions:v1.28",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Test 'phase' = 'Pending' to match the payload we're sending
			{Field: "phase", Operator: "->>", Value: "Pending", ExpectedRows: 1},
		},
	},

	// ========================================
	// WEBHOOK SERVICE (5 event types)
	// ========================================
	{
		Service:       "webhooks",
		EventType:     "webhook.notification.cancelled",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWebhook,
		EventAction:   "cancelled",
		CreateEvent: func() ogenclient.AuditEventRequest {
			notifName := fmt.Sprintf("notif-test-cancelled-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-webhook-notif-cancel-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "webhook.notification.cancelled",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWebhook,
				EventAction:    "cancelled",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("user"),
				ActorID:        ogenclient.NewOptString("admin@example.com"),
				ResourceType:   ogenclient.NewOptString("NotificationRequest"),
				ResourceID:     ogenclient.NewOptString(notifName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataWebhookNotificationCancelledAuditEventRequestEventData(ogenclient.NotificationAuditPayload{
					EventType:        ogenclient.NotificationAuditPayloadEventTypeWebhookNotificationCancelled,
					NotificationName: ogenclient.NewOptString(notifName),
					FinalStatus:      ogenclient.NewOptNotificationAuditPayloadFinalStatus(ogenclient.NotificationAuditPayloadFinalStatusCancelled),
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "final_status", Operator: "->>", Value: "Cancelled", ExpectedRows: 1}, // Fixed: OpenAPI spec defines "final_status" not "status"
		},
	},
	{
		Service:       "webhooks",
		EventType:     "webhook.notification.acknowledged",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWebhook,
		EventAction:   "acknowledged",
		CreateEvent: func() ogenclient.AuditEventRequest {
			notifName := fmt.Sprintf("notif-test-acked-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-webhook-notif-ack-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "webhook.notification.acknowledged",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWebhook,
				EventAction:    "acknowledged",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("user"),
				ActorID:        ogenclient.NewOptString("admin@example.com"),
				ResourceType:   ogenclient.NewOptString("NotificationRequest"),
				ResourceID:     ogenclient.NewOptString(notifName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAuditEventRequestEventDataWebhookNotificationAcknowledgedAuditEventRequestEventData(ogenclient.NotificationAuditPayload{
					EventType:        ogenclient.NotificationAuditPayloadEventTypeWebhookNotificationAcknowledged,
					NotificationName: ogenclient.NewOptString(notifName),
					FinalStatus:      ogenclient.NewOptNotificationAuditPayloadFinalStatus(ogenclient.NotificationAuditPayloadFinalStatusSent),
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "final_status", Operator: "->>", Value: "Sent", ExpectedRows: 1}, // Fixed: Event creates with FinalStatusSent, OpenAPI enum: [Pending,Sending,Sent,Failed,Cancelled]
		},
	},
	{
		Service:       "webhooks",
		EventType:     "workflowexecution.block.cleared",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWebhook,
		EventAction:   "block_cleared",
		CreateEvent: func() ogenclient.AuditEventRequest {
			wfeName := fmt.Sprintf("wfe-test-unblocked-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-webhook-wfe-unblock-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "workflowexecution.block.cleared",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWebhook,
				EventAction:    "block_cleared",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("user"),
				ActorID:        ogenclient.NewOptString("admin@example.com"),
				ResourceType:   ogenclient.NewOptString("WorkflowExecution"),
				ResourceID:     ogenclient.NewOptString(wfeName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewWorkflowExecutionWebhookAuditPayloadAuditEventRequestEventData(ogenclient.WorkflowExecutionWebhookAuditPayload{
					EventType:     ogenclient.WorkflowExecutionWebhookAuditPayloadEventTypeWorkflowexecutionBlockCleared,
					WorkflowName:  wfeName,
					ClearReason:   "Manual approval by ops team - E2E test",
					ClearedAt:     time.Now().UTC(),
					PreviousState: ogenclient.WorkflowExecutionWebhookAuditPayloadPreviousStateBlocked,
					NewState:      ogenclient.WorkflowExecutionWebhookAuditPayloadNewStateRunning,
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Query 'previous_state' = 'Blocked' which exists in WorkflowExecutionWebhookAuditPayload
			{Field: "previous_state", Operator: "->>", Value: "Blocked", ExpectedRows: 1},
		},
	},
	{
		Service:       "webhooks",
		EventType:     "webhook.remediationapprovalrequest.decided",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWebhook,
		EventAction:   "decided",
		CreateEvent: func() ogenclient.AuditEventRequest {
			approvalName := fmt.Sprintf("approval-test-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-webhook-approval-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "webhook.remediationapprovalrequest.decided",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWebhook,
				EventAction:    "decided",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("user"),
				ActorID:        ogenclient.NewOptString("admin@example.com"),
				ResourceType:   ogenclient.NewOptString("RemediationApproval"),
				ResourceID:     ogenclient.NewOptString(approvalName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewRemediationApprovalAuditPayloadAuditEventRequestEventData(ogenclient.RemediationApprovalAuditPayload{
					EventType:       ogenclient.RemediationApprovalAuditPayloadEventTypeWebhookRemediationapprovalrequestDecided,
					RequestName:     approvalName,
					Decision:        ogenclient.RemediationApprovalAuditPayloadDecisionApproved,
					DecidedAt:       time.Now().UTC(),
					DecisionMessage: "Approved by ops team for E2E testing",
					AiAnalysisRef:   "aianalysis-test-ref-123",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: OpenAPI enum is "Approved" (title case), not "approved"
			{Field: "decision", Operator: "->>", Value: "Approved", ExpectedRows: 1},
		},
	},
	{
		Service:       "webhooks",
		EventType:     "webhook.remediationrequest.timeout_modified",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWebhook,
		EventAction:   "timeout_modified",
		CreateEvent: func() ogenclient.AuditEventRequest {
			rrName := fmt.Sprintf("rr-test-timeout-modified-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-webhook-rr-timeout-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "webhook.remediationrequest.timeout_modified",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWebhook,
				EventAction:    "timeout_modified",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("user"),
				ActorID:        ogenclient.NewOptString("admin@example.com"),
				ResourceType:   ogenclient.NewOptString("RemediationRequest"),
				ResourceID:     ogenclient.NewOptString(rrName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewRemediationRequestWebhookAuditPayloadAuditEventRequestEventData(ogenclient.RemediationRequestWebhookAuditPayload{
					EventType:  ogenclient.RemediationRequestWebhookAuditPayloadEventTypeWebhookRemediationrequestTimeoutModified,
					RrName:     rrName,
					Namespace:  "default",
					ModifiedBy: "admin@example.com",
					ModifiedAt: time.Now().UTC(),
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Query 'modified_by' which exists in RemediationRequestWebhookAuditPayload
			{Field: "modified_by", Operator: "->>", Value: "admin@example.com", ExpectedRows: 1},
		},
	},

	// ========================================
	// WORKFLOW CATALOG SERVICE (1 event type)
	// ========================================
	{
		Service:       "workflow",
		EventType:     "workflow.catalog.search_completed",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWorkflow,
		EventAction:   "search_completed",
		CreateEvent: func() ogenclient.AuditEventRequest {
			searchID := fmt.Sprintf("search-test-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-workflow-search-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "workflow.catalog.search_completed",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflow,
				EventAction:    "search_completed",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("datastorage-service"),
				ResourceType:   ogenclient.NewOptString("WorkflowSearch"),
				ResourceID:     ogenclient.NewOptString(searchID),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewWorkflowSearchAuditPayloadAuditEventRequestEventData(ogenclient.WorkflowSearchAuditPayload{
					EventType: ogenclient.WorkflowSearchAuditPayloadEventTypeWorkflowCatalogSearchCompleted,
					Query: ogenclient.QueryMetadata{
						TopK: 5,
					},
					Results: ogenclient.ResultsMetadata{
						TotalFound: 5,
						Returned:   5,
						Workflows:  []ogenclient.WorkflowResultAudit{},
					},
					SearchMetadata: ogenclient.SearchExecutionMetadata{
						DurationMs: 150,
					},
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Query nested field results->total_found (WorkflowSearchAuditPayload.results is a ResultsMetadata object)
			// Query construction: event_data->'results'->>'total_found' = '5'
			{Field: "total_found", Operator: "->'results'->>", Value: "5", ExpectedRows: 1},
		},
	},

	// ========================================
	// DATA STORAGE WORKFLOW SERVICE (2 event types)
	// ========================================
	{
		Service:       "datastorage",
		EventType:     "datastorage.workflow.created",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWorkflow,
		EventAction:   "created",
		CreateEvent: func() ogenclient.AuditEventRequest {
			workflowID := uuid.New()
			correlationID := fmt.Sprintf("test-gap-1.1-ds-workflow-created-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "datastorage.workflow.created",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflow,
				EventAction:    "created",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("datastorage-service"),
				ResourceType:   ogenclient.NewOptString("WorkflowCatalog"),
				ResourceID:     ogenclient.NewOptString(workflowID.String()),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewWorkflowCatalogCreatedPayloadAuditEventRequestEventData(ogenclient.WorkflowCatalogCreatedPayload{
					WorkflowID:      workflowID,                                                                                             // REQUIRED
					WorkflowName:    "test-workflow",                                                                                        // REQUIRED
					Version:         "v1.0.0",                                                                                               // REQUIRED
					Status:          ogenclient.WorkflowCatalogCreatedPayloadStatusActive,                                                   // REQUIRED
					IsLatestVersion: true,                                                                                                   // REQUIRED
					ExecutionEngine: "tekton",                                                                                               // REQUIRED
					Name:            "Test Workflow Display Name",                                                                           // REQUIRED
					Description:     ogenclient.NewOptString("Test workflow description"),                                                   // OPTIONAL
					Labels:          ogenclient.NewOptWorkflowCatalogCreatedPayloadLabels(ogenclient.WorkflowCatalogCreatedPayloadLabels{}), // OPTIONAL
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "workflow_name", Operator: "->>", Value: "test-workflow", ExpectedRows: 1},
			{Field: "version", Operator: "->>", Value: "v1.0.0", ExpectedRows: 1},
		},
	},
	{
		Service:       "datastorage",
		EventType:     "datastorage.workflow.updated",
		EventCategory: ogenclient.AuditEventRequestEventCategoryWorkflow,
		EventAction:   "updated",
		CreateEvent: func() ogenclient.AuditEventRequest {
			workflowID := uuid.New()
			correlationID := fmt.Sprintf("test-gap-1.1-ds-workflow-updated-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "datastorage.workflow.updated",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflow,
				EventAction:    "updated",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("datastorage-service"),
				ResourceType:   ogenclient.NewOptString("WorkflowCatalog"),
				ResourceID:     ogenclient.NewOptString(workflowID.String()),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewWorkflowCatalogUpdatedPayloadAuditEventRequestEventData(ogenclient.WorkflowCatalogUpdatedPayload{
					WorkflowID: workflowID, // REQUIRED
					UpdatedFields: ogenclient.WorkflowCatalogUpdatedFields{ // REQUIRED (at least one field updated)
						Version:     ogenclient.NewOptString("v2.0.0"),              // OPTIONAL (but provide at least one)
						Description: ogenclient.NewOptString("Updated description"), // OPTIONAL
					},
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// Query nested field updated_fields->version (WorkflowCatalogUpdatedPayload.updated_fields.version)
			// Query construction: event_data->'updated_fields'->>'version' = 'v2.0.0'
			{Field: "version", Operator: "->'updated_fields'->>", Value: "v2.0.0", ExpectedRows: 1},
		},
	},

	// ========================================
	// AI ANALYSIS EXTENDED SERVICE (3 event types)
	// ========================================
	{
		Service:       "aianalysis",
		EventType:     "aianalysis.phase.transition",
		EventCategory: ogenclient.AuditEventRequestEventCategoryAnalysis,
		EventAction:   "phase_transition",
		CreateEvent: func() ogenclient.AuditEventRequest {
			analysisName := fmt.Sprintf("aa-test-phase-transition-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-ai-phase-trans-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aianalysis.phase.transition",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAnalysis,
				EventAction:    "phase_transition",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("aianalysis-service"),
				ResourceType:   ogenclient.NewOptString("AIAnalysis"),
				ResourceID:     ogenclient.NewOptString(analysisName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAIAnalysisPhaseTransitionPayloadAuditEventRequestEventData(ogenclient.AIAnalysisPhaseTransitionPayload{
					OldPhase: "Pending",   // REQUIRED
					NewPhase: "Analyzing", // REQUIRED
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Schema uses 'new_phase' not 'to_phase' (DD-AUDIT-003)
			{Field: "new_phase", Operator: "->>", Value: "Analyzing", ExpectedRows: 1},
		},
	},
	{
		Service:       "aianalysis",
		EventType:     "aianalysis.holmesgpt.call",
		EventCategory: ogenclient.AuditEventRequestEventCategoryAnalysis,
		EventAction:   "holmesgpt_call",
		CreateEvent: func() ogenclient.AuditEventRequest {
			analysisName := fmt.Sprintf("aa-test-holmesgpt-call-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-ai-holmesgpt-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aianalysis.holmesgpt.call",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAnalysis,
				EventAction:    "holmesgpt_call",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("service"),
				ActorID:        ogenclient.NewOptString("aianalysis-service"),
				ResourceType:   ogenclient.NewOptString("AIAnalysis"),
				ResourceID:     ogenclient.NewOptString(analysisName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAIAnalysisHolmesGPTCallPayloadAuditEventRequestEventData(ogenclient.AIAnalysisHolmesGPTCallPayload{
					Endpoint:       "/api/v1/analyze", // REQUIRED
					HTTPStatusCode: 200,               // REQUIRED
					DurationMs:     150,               // REQUIRED
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			// FIX: Schema uses 'endpoint' not 'call_type' (DD-AUDIT-003)
			{Field: "endpoint", Operator: "->>", Value: "/api/v1/analyze", ExpectedRows: 1},
		},
	},
	{
		Service:       "aianalysis",
		EventType:     "aianalysis.approval.decision",
		EventCategory: ogenclient.AuditEventRequestEventCategoryAnalysis,
		EventAction:   "approval_decision",
		CreateEvent: func() ogenclient.AuditEventRequest {
			analysisName := fmt.Sprintf("aa-test-approval-decision-%s", uuid.New().String()[:8])
			correlationID := fmt.Sprintf("test-gap-1.1-ai-approval-%s", uuid.New().String()[:8])
			return ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aianalysis.approval.decision",
				EventTimestamp: time.Now().UTC(),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAnalysis,
				EventAction:    "approval_decision",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorType:      ogenclient.NewOptString("user"),
				ActorID:        ogenclient.NewOptString("admin@example.com"),
				ResourceType:   ogenclient.NewOptString("AIAnalysis"),
				ResourceID:     ogenclient.NewOptString(analysisName),
				CorrelationID:  correlationID,
				EventData: ogenclient.NewAIAnalysisApprovalDecisionPayloadAuditEventRequestEventData(ogenclient.AIAnalysisApprovalDecisionPayload{
					Decision: "approved",
				}),
			}
		},
		JSONBQueries: []jsonbQueryTest{
			{Field: "decision", Operator: "->>", Value: "approved", ExpectedRows: 1},
		},
	},
}

// ========================================
// COMPREHENSIVE EVENT TYPE VALIDATION TESTS
// ========================================

var _ = Describe("GAP 1.1: Comprehensive Event Type + JSONB Validation", Label("e2e", "gap-1.1", "p0"), Ordered, func() {
	var (
		db  *sql.DB
		ctx context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()

		// Connect to PostgreSQL via NodePort for JSONB query validation
		var err error
		db, err = sql.Open("pgx", postgresURL)
		Expect(err).ToNot(HaveOccurred())
		Expect(db.Ping()).To(Succeed())
	})

	AfterAll(func() {
		if db != nil {
			_ = db.Close()
		}
	})

	Describe("OpenAPI Event Type Catalog Coverage", func() {
		It("should validate all 36 event types from OpenAPI schema are documented", func() {
			// ASSERT: Catalog completeness (36 valid event types per OpenAPI schema)
			// Authority: api/openapi/data-storage-v1.yaml lines 1614-1649
			Expect(eventTypeCatalog).To(HaveLen(36),
				"OpenAPI schema defines 36 event types (per api/openapi/data-storage-v1.yaml discriminator mapping)")

			// Count by service
			serviceCounts := map[string]int{}
			for _, tc := range eventTypeCatalog {
				serviceCounts[tc.Service]++
			}

			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("GAP 1.1: OpenAPI Event Type Coverage (DD-AUDIT-004 Compliant)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("")
			GinkgoWriter.Println("Event Types by Service:")
			GinkgoWriter.Printf("  Gateway:                    %d event types\n", serviceCounts["gateway"])
			GinkgoWriter.Printf("  RemediationOrchestrator:    %d event types\n", serviceCounts["remediation-orchestrator"])
			GinkgoWriter.Printf("  SignalProcessing:           %d event types\n", serviceCounts["signalprocessing"])
			GinkgoWriter.Printf("  AIAnalysis:                 %d event types\n", serviceCounts["aianalysis"])
			GinkgoWriter.Printf("  WorkflowExecution:          %d event types\n", serviceCounts["workflowexecution"])
			GinkgoWriter.Printf("  Webhooks:                   %d event types\n", serviceCounts["webhooks"])
			GinkgoWriter.Printf("  Workflow Catalog:           %d event types\n", serviceCounts["workflow"])
			GinkgoWriter.Printf("  DataStorage:                %d event types\n", serviceCounts["datastorage"])
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ASSERT: Expected counts per OpenAPI schema
			Expect(serviceCounts["gateway"]).To(Equal(4))                  // gateway.*
			Expect(serviceCounts["remediation-orchestrator"]).To(Equal(8)) // orchestrator.*
			Expect(serviceCounts["signalprocessing"]).To(Equal(6))         // signalprocessing.*
			Expect(serviceCounts["aianalysis"]).To(Equal(5))               // aianalysis.* (2 + 3 extended)
			Expect(serviceCounts["workflowexecution"]).To(Equal(5))        // workflowexecution.*
			Expect(serviceCounts["webhooks"]).To(Equal(5))                 // webhook.*
			Expect(serviceCounts["workflow"]).To(Equal(1))                 // workflow.catalog.*
			Expect(serviceCounts["datastorage"]).To(Equal(2))              // datastorage.workflow.*
		})
	})

	// ========================================
	// DATA-DRIVEN TEST: ALL 36 VALID EVENT TYPES
	// ========================================
	Describe("Event Type Acceptance + JSONB Validation (DD-AUDIT-004 Compliant)", func() {
		for _, tc := range eventTypeCatalog {
			tc := tc // Capture range variable

			Context(fmt.Sprintf("Event Type: %s", tc.EventType), Ordered, func() {
				It("should accept event type via OpenAPI client and persist to database", func() {
					// ARRANGE: Create audit event using structured ogenclient types (DD-AUDIT-004 compliant)
					auditEvent := tc.CreateEvent()

					// ACT: Send event using OpenAPI client (replaces raw HTTP POST)
					resp, err := DSClient.CreateAuditEvent(ctx, &auditEvent)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Event type %s should be accepted by DataStorage", tc.EventType))
					Expect(resp).ToNot(BeNil())

					// ASSERT: Event persisted to database (with Eventually for async persistence)
					correlationID := auditEvent.CorrelationID
					Eventually(func() int {
						var count int
						err := db.QueryRow(`
							SELECT COUNT(*)
							FROM audit_events
							WHERE correlation_id = $1 AND event_type = $2
						`, correlationID, tc.EventType).Scan(&count)
						if err != nil {
							return 0
						}
						return count
					}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
						fmt.Sprintf("Event %s should be persisted with correlation_id=%s", tc.EventType, correlationID))
				})

				// JSONB Query Validation
				for _, query := range tc.JSONBQueries {
					query := query // Capture range variable
					It(fmt.Sprintf("should support JSONB query: event_data%s'%s' = '%s'", query.Operator, query.Field, query.Value), func() {
						// ARRANGE: Create and send audit event using structured ogenclient types
						auditEvent := tc.CreateEvent()

						// ⚠️ CRITICAL: Must send event BEFORE querying it!
						_, err := DSClient.CreateAuditEvent(ctx, &auditEvent)
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Event type %s should be accepted by DataStorage", tc.EventType))

						correlationID := auditEvent.CorrelationID

						// ACT: Execute JSONB query
						querySQL := fmt.Sprintf(`
							SELECT COUNT(*)
							FROM audit_events
							WHERE correlation_id = $1
							  AND event_data%s'%s' = $2
						`, query.Operator, query.Field)

						var count int
						err = db.QueryRow(querySQL, correlationID, query.Value).Scan(&count)

						// ASSERT: JSONB operator works
						Expect(err).ToNot(HaveOccurred())
						Expect(count).To(Equal(query.ExpectedRows),
							fmt.Sprintf("JSONB query for field '%s' should return %d rows", query.Field, query.ExpectedRows))
					})
				}
			})
		}
	})
})
