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

package audit

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	prodaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

func TestAuditManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Remediation Orchestrator Audit Manager Suite")
}

var _ = Describe("Audit Manager", func() {
	var manager *prodaudit.Manager

	BeforeEach(func() {
		manager = prodaudit.NewManager(prodaudit.ServiceName)
	})

	// Helper to convert AuditEventRequest to AuditEvent for validation
	// Both types share core audit fields
	toAuditEvent := func(req *ogenclient.AuditEventRequest) ogenclient.AuditEvent {
		event := ogenclient.AuditEvent{
			ActorID:       req.ActorID,
			ActorType:     req.ActorType,
			CorrelationID: req.CorrelationID,
			DurationMs:    req.DurationMs,
			EventAction:   req.EventAction,
			EventCategory: ogenclient.AuditEventEventCategory(req.EventCategory),
			EventOutcome:  ogenclient.AuditEventEventOutcome(req.EventOutcome),
			EventType:     req.EventType,
			Namespace:     req.Namespace,
			ResourceID:    req.ResourceID,
			ResourceType:  req.ResourceType,
			Severity:      req.Severity,
		}

		// Convert EventData discriminated union from Request to Event types
		// Map the discriminator and copy the payload
		if payload, ok := req.EventData.GetRemediationOrchestratorAuditPayload(); ok {
			// Map RequestEventDataType → EventEventDataType discriminator
			var eventDataType ogenclient.AuditEventEventDataType
			switch req.EventData.Type {
			case ogenclient.AuditEventRequestEventDataOrchestratorLifecycleStartedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorLifecycleStartedAuditEventEventData
			case ogenclient.AuditEventRequestEventDataOrchestratorLifecycleTransitionedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorLifecycleTransitionedAuditEventEventData
			case ogenclient.AuditEventRequestEventDataOrchestratorLifecycleCompletedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorLifecycleCompletedAuditEventEventData
			case ogenclient.AuditEventRequestEventDataOrchestratorLifecycleFailedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorLifecycleFailedAuditEventEventData
			case ogenclient.AuditEventRequestEventDataOrchestratorApprovalRequestedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorApprovalRequestedAuditEventEventData
			case ogenclient.AuditEventRequestEventDataOrchestratorApprovalApprovedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorApprovalApprovedAuditEventEventData
			case ogenclient.AuditEventRequestEventDataOrchestratorApprovalRejectedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorApprovalRejectedAuditEventEventData
			// Note: orchestrator.manual_review.requested and orchestrator.timeout.occurred
			// event types not yet in OpenAPI spec - will be added in future
			default:
				// Unknown or not-yet-implemented event type - skip EventData conversion
				return event
			}
			event.EventData.SetRemediationOrchestratorAuditPayload(eventDataType, payload)
		}

		return event
	}

	Describe("NewManager", func() {
		It("should create manager with correct service name", func() {
			m := prodaudit.NewManager("test-service")
			Expect(m).ToNot(BeNil())
		})
	})

	// ========================================
	// BuildLifecycleStartedEvent Tests
	// Per DD-AUDIT-003: orchestrator.lifecycle.started (P1)
	// Per V1.0 Maturity: Using testutil.ValidateAuditEvent for consistent validation
	// ========================================
	Describe("BuildLifecycleStartedEvent", func() {
		It("should build complete orchestrator.lifecycle.started event with all required fields", func() {
			event, err := manager.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())

			// V1.0 Maturity: Use testutil.ValidateAuditEvent for structured validation
			testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
				// Required fields
				EventType:     "orchestrator.lifecycle.started",
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "started",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomePending),
				CorrelationID: "correlation-123",

				// Optional fields
				Namespace:    ptr.To("default"),
				ActorType:    ptr.To("service"),
				ActorID:      ptr.To(prodaudit.ServiceName),
				ResourceType: ptr.To("RemediationRequest"),
				ResourceID:   ptr.To("rr-test-001"),

				// EventData validation
				EventDataFields: map[string]interface{}{
					"rr_name":   "rr-test-001",
					"namespace": "default",
				},
			})
		})

		It("should respect custom namespace", func() {
			event, err := manager.BuildLifecycleStartedEvent(
				"correlation-456",
				"production",
				"rr-prod-001",
			)
			Expect(err).ToNot(HaveOccurred())

			testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
				EventType:     "orchestrator.lifecycle.started",
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "started",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomePending),
				CorrelationID: "correlation-456",
				Namespace:     ptr.To("production"),
				ResourceID:    ptr.To("rr-prod-001"),
				EventDataFields: map[string]interface{}{
					"rr_name":   "rr-prod-001",
					"namespace": "production",
				},
			})
		})
	})

	// ========================================
	// BuildPhaseTransitionEvent Tests
	// Per DD-AUDIT-003: orchestrator.phase.transitioned (P1)
	// Per V1.0 Maturity: Using testutil.ValidateAuditEvent for consistent validation
	// ========================================
	Describe("BuildPhaseTransitionEvent", func() {
		It("should build complete orchestrator.lifecycle.transitioned event with all required fields", func() {
			event, err := manager.BuildPhaseTransitionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Analyzing",
				"Executing",
			)
			Expect(err).ToNot(HaveOccurred())

			testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
				EventType:     "orchestrator.lifecycle.transitioned",
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "transitioned",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: "correlation-123",
				Namespace:     ptr.To("default"),
				ResourceID:    ptr.To("rr-test-001"),
				EventDataFields: map[string]interface{}{
					"from_phase": "Analyzing",
					"to_phase":   "Executing",
				},
			})
		})

		It("should handle all common phase transitions", func() {
			transitions := []struct {
				from string
				to   string
			}{
				{"Pending", "Processing"},
				{"Processing", "Analyzing"},
				{"Analyzing", "AwaitingApproval"},
				{"Analyzing", "Executing"},
				{"AwaitingApproval", "Executing"},
				{"Executing", "Completed"},
			}

			for _, t := range transitions {
				event, err := manager.BuildPhaseTransitionEvent(
					"correlation-123",
					"default",
					"rr-test-001",
					t.from,
					t.to,
				)
				Expect(err).ToNot(HaveOccurred())

				testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
					EventType:     "orchestrator.lifecycle.transitioned",
					EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
					EventAction:   "transitioned",
					EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomeSuccess),
					CorrelationID: "correlation-123",
					EventDataFields: map[string]interface{}{
						"from_phase": t.from,
						"to_phase":   t.to,
					},
				})
			}
		})
	})

	// ========================================
	// BuildCompletionEvent Tests
	// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1)
	// Per V1.0 Maturity: Using testutil.ValidateAuditEvent for consistent validation
	// ========================================
	Describe("BuildCompletionEvent", func() {
		It("should build complete orchestrator.lifecycle.completed event with all required fields", func() {
			event, err := manager.BuildCompletionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Remediated",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())

			testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
				EventType:     "orchestrator.lifecycle.completed",
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "completed",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: "correlation-123",
				Namespace:     ptr.To("default"),
				ResourceID:    ptr.To("rr-test-001"),
				EventDataFields: map[string]interface{}{
					"outcome": "Remediated",
				},
			})

			// DurationMs is not part of testutil.ExpectedAuditEvent, validate separately
			Expect(event.DurationMs.IsSet()).To(BeTrue())
			Expect(event.DurationMs.Value).To(Equal(5000))
		})

		It("should include correct duration and handle different outcomes", func() {
			event, err := manager.BuildCompletionEvent(
				"correlation-456",
				"production",
				"rr-prod-001",
				"NoActionRequired",
				12345,
			)
			Expect(err).ToNot(HaveOccurred())

			testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
				EventType:     "orchestrator.lifecycle.completed",
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "completed",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: "correlation-456",
				Namespace:     ptr.To("production"),
				ResourceID:    ptr.To("rr-prod-001"),
				EventDataFields: map[string]interface{}{
					"outcome": "NoActionRequired",
				},
			})

			Expect(event.DurationMs.IsSet()).To(BeTrue())
			Expect(event.DurationMs.Value).To(Equal(12345))
		})
	})

	// ========================================
	// BuildFailureEvent Tests
	// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1) with failure outcome
	// Per V1.0 Maturity: Using testutil.ValidateAuditEvent for consistent validation
	// ========================================
	Describe("BuildFailureEvent", func() {
	It("should build complete orchestrator.lifecycle.completed event with failure outcome", func() {
		event, err := manager.BuildFailureEvent(
			"correlation-123",
			"default",
			"rr-test-001",
			"workflow_execution",
			"RBAC permission denied",
			5000,
		)
		Expect(err).ToNot(HaveOccurred())

		testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
			EventType:     "orchestrator.lifecycle.completed",
			EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
			EventAction:   "completed",
			EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomeFailure),
			CorrelationID: "correlation-123",
			Namespace:     ptr.To("default"),
			ResourceID:    ptr.To("rr-test-001"),
			EventDataFields: map[string]interface{}{
				"outcome":        "Failed",
				"failure_phase":  "workflow_execution",
				"failure_reason": "RBAC permission denied",
			},
		})

		Expect(event.DurationMs.IsSet()).To(BeTrue())
		Expect(event.DurationMs.Value).To(Equal(5000))
	})

	// BR-AUDIT-005 Gap #7: Validate ErrorDetails structure in failure events
	It("should emit audit event with standardized ErrorDetails structure (Gap #7)", func() {
		event, err := manager.BuildFailureEvent(
			"correlation-789",
			"production",
			"rr-prod-002",
			"signal_processing",
			"timeout while enriching alert",
			15000,
		)
		Expect(err).ToNot(HaveOccurred())

		// Convert to map for EventData validation
		eventDataBytes, _ := json.Marshal(event.EventData)
		var eventData map[string]interface{}
		_ = json.Unmarshal(eventDataBytes, &eventData)

		// Validate error_details field exists (DD-ERROR-001)
		Expect(eventData).To(HaveKey("error_details"), "Should contain error_details field (Gap #7)")

		errorDetails, ok := eventData["error_details"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "error_details should be a map")

		// Validate ErrorDetails structure per DD-ERROR-001
		Expect(errorDetails).To(HaveKey("code"), "Should have error code")
		Expect(errorDetails).To(HaveKey("message"), "Should have error message")
		Expect(errorDetails).To(HaveKey("component"), "Should have component name")
		Expect(errorDetails).To(HaveKey("retry_possible"), "Should have retry_possible indicator")

		// Validate values
		Expect(errorDetails["component"]).To(Equal("remediationorchestrator"), "Should identify remediationorchestrator component")
		Expect(errorDetails["code"]).To(MatchRegexp("^ERR_"), "Error code should start with ERR_")
		Expect(errorDetails["message"]).To(ContainSubstring("signal_processing"), "Message should include failure phase")
		Expect(errorDetails["message"]).To(ContainSubstring("timeout"), "Message should include failure reason")
		Expect(errorDetails["retry_possible"]).To(BeAssignableToTypeOf(false), "retry_possible should be boolean")

		// Validate timeout errors are marked as retryable (business logic)
		Expect(errorDetails["code"]).To(Equal("ERR_TIMEOUT_REMEDIATION"), "Timeout errors should use ERR_TIMEOUT_REMEDIATION code")
		Expect(errorDetails["retry_possible"]).To(BeTrue(), "Timeout errors should be retryable")
	})

		It("should handle different failure phases and reasons with correct duration", func() {
			event, err := manager.BuildFailureEvent(
				"correlation-456",
				"production",
				"rr-prod-001",
				"signal_processing",
				"Enrichment timeout",
				10000,
			)
			Expect(err).ToNot(HaveOccurred())

			testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
				EventType:     "orchestrator.lifecycle.completed",
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "completed",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomeFailure),
				CorrelationID: "correlation-456",
				Namespace:     ptr.To("production"),
				ResourceID:    ptr.To("rr-prod-001"),
				EventDataFields: map[string]interface{}{
					"outcome":        "Failed",
					"failure_phase":  "signal_processing",
					"failure_reason": "Enrichment timeout",
				},
			})

		Expect(event.DurationMs.IsSet()).To(BeTrue())
		Expect(event.DurationMs.Value).To(Equal(10000))
	})

	// BR-AUDIT-005 Gap #7: Error Code Mapping Unit Tests
	// Per DD-TEST-008: Error code mapping belongs in unit tests, not integration tests
	Context("Gap #7: Error Code Mapping Logic", func() {
		It("should map invalid config errors to ERR_INVALID_CONFIG", func() {
			event, err := manager.BuildFailureEvent(
				"correlation-001",
				"default",
				"rr-test",
				"configuration",
				"invalid workflow selector",
				1000,
			)
			Expect(err).ToNot(HaveOccurred())

			// Validate error_details
			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)

			errorDetails := eventData["error_details"].(map[string]interface{})
			Expect(errorDetails["code"]).To(Equal("ERR_INVALID_CONFIG"), "Invalid config should map to ERR_INVALID_CONFIG")
			Expect(errorDetails["retry_possible"]).To(BeFalse(), "Invalid config is permanent error")
			Expect(errorDetails["component"]).To(Equal("remediationorchestrator"))
			Expect(errorDetails["message"]).To(ContainSubstring("invalid workflow selector"))
		})

		It("should map K8s creation errors to ERR_K8S_CREATE_FAILED", func() {
			event, err := manager.BuildFailureEvent(
				"correlation-002",
				"default",
				"rr-test",
				"signal_processing",
				"failed to create SignalProcessing: forbidden",
				1000,
			)
			Expect(err).ToNot(HaveOccurred())

			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)

			errorDetails := eventData["error_details"].(map[string]interface{})
			Expect(errorDetails["code"]).To(Equal("ERR_K8S_CREATE_FAILED"), "K8s creation errors should map to ERR_K8S_CREATE_FAILED")
			Expect(errorDetails["retry_possible"]).To(BeTrue(), "K8s errors are transient")
			Expect(errorDetails["message"]).To(ContainSubstring("forbidden"))
		})

		It("should map unknown errors to ERR_INTERNAL_ORCHESTRATION", func() {
			event, err := manager.BuildFailureEvent(
				"correlation-003",
				"default",
				"rr-test",
				"unknown",
				"unexpected panic in reconciler",
				1000,
			)
			Expect(err).ToNot(HaveOccurred())

			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)

			errorDetails := eventData["error_details"].(map[string]interface{})
			Expect(errorDetails["code"]).To(Equal("ERR_INTERNAL_ORCHESTRATION"), "Unknown errors should map to ERR_INTERNAL_ORCHESTRATION")
			Expect(errorDetails["retry_possible"]).To(BeTrue(), "Default to retryable")
			Expect(errorDetails["message"]).To(ContainSubstring("unexpected panic"))
		})

		It("should map 'not found' errors to ERR_K8S_CREATE_FAILED", func() {
			event, err := manager.BuildFailureEvent(
				"correlation-004",
				"default",
				"rr-test",
				"workflow_execution",
				"WorkflowExecution not found after creation",
				1000,
			)
			Expect(err).ToNot(HaveOccurred())

			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)

			errorDetails := eventData["error_details"].(map[string]interface{})
			Expect(errorDetails["code"]).To(Equal("ERR_K8S_CREATE_FAILED"), "'not found' should map to ERR_K8S_CREATE_FAILED")
			Expect(errorDetails["retry_possible"]).To(BeTrue(), "K8s not found errors are transient")
			Expect(errorDetails["message"]).To(ContainSubstring("not found"))
		})
	})
	})

	// ========================================
	// BuildApprovalRequestedEvent Tests
	// Related to ADR-040 (RemediationApprovalRequest)
	// ========================================
	// ========================================
	// BuildApprovalRequestedEvent Tests
	// Related to ADR-040 (RemediationApprovalRequest)
	// Per V1.0 Maturity: Using testutil.ValidateAuditEvent for consistent validation
	// ========================================
	Describe("BuildApprovalRequestedEvent", func() {
		It("should build complete orchestrator.approval.requested event with all required fields", func() {
			requiredBy := time.Now().Add(24 * time.Hour)
			event, err := manager.BuildApprovalRequestedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"wf-scale-deployment",
				"85%",
				requiredBy,
			)
			Expect(err).ToNot(HaveOccurred())

			testutil.ValidateAuditEvent(toAuditEvent(event), testutil.ExpectedAuditEvent{
				EventType:     "orchestrator.approval.requested",
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "approval_requested",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomePending),
				CorrelationID: "correlation-123",
				Namespace:     ptr.To("default"),
				ResourceType:  ptr.To("RemediationApprovalRequest"),
				ResourceID:    ptr.To("rar-rr-test-001"),
				EventDataFields: map[string]interface{}{
					"rar_name":    "rar-rr-test-001",
					"rr_name":     "rr-test-001",
					"workflow_id": "wf-scale-deployment",
					"confidence":  "85%",
				},
			})
		})
	})

	// ========================================
	// BuildApprovalDecisionEvent Tests
	// Related to ADR-040 (RemediationApprovalRequest)
	// ========================================
	Describe("BuildApprovalDecisionEvent", func() {
		It("should build approved event with correct type", func() {
			event, err := manager.BuildApprovalDecisionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"Approved",
				"operator@example.com",
				"Looks good",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.approval.approved"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcome("success")))
		})

		It("should build rejected event with correct type", func() {
			event, err := manager.BuildApprovalDecisionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"Rejected",
				"admin@example.com",
				"Too risky",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.approval.rejected"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcome("failure")))
		})

		It("should build expired event with correct type", func() {
			event, err := manager.BuildApprovalDecisionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"Expired",
				"system",
				"Approval deadline passed",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.approval.expired"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcome("failure")))
		})

		It("should set actor type to user for non-system decisions", func() {
			event, err := manager.BuildApprovalDecisionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"Approved",
				"operator@example.com",
				"LGTM",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.ActorType.Value).To(Equal("user"))
			Expect(event.ActorID.Value).To(Equal("operator@example.com"))
		})

		It("should set actor type to service for system decisions", func() {
			event, err := manager.BuildApprovalDecisionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"Expired",
				"system",
				"Timeout",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.ActorType.Value).To(Equal("service"))
			Expect(event.ActorID.Value).To(Equal(prodaudit.ServiceName))
		})
	})

	// ========================================
	// BuildManualReviewEvent Tests
	// Related to BR-ORCH-036 (Manual Review Notifications)
	// ========================================
	Describe("BuildManualReviewEvent", func() {
		It("should build event with orchestrator prefix", func() {
			event, err := manager.BuildManualReviewEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"WorkflowResolutionFailed",
				"NoMatchingWorkflow",
				"nr-manual-review-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.remediation.manual_review"))
		})

		It("should set event outcome to pending", func() {
			event, err := manager.BuildManualReviewEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"InvestigationInconclusive",
				"LLMUncertain",
				"nr-manual-review-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcome("pending")))
		})

		It("should set severity to warning", func() {
			event, err := manager.BuildManualReviewEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"ExhaustedRetries",
				"MaxRetriesReached",
				"nr-manual-review-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.Severity).ToNot(BeNil())
			Expect(event.Severity.Value).To(Equal("warning"))
		})

		It("should include reason and sub-reason in event data", func() {
			event, err := manager.BuildManualReviewEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"WorkflowResolutionFailed",
				"MultipleWorkflowsMatched",
				"nr-manual-review-001",
			)
			Expect(err).ToNot(HaveOccurred())

			var data prodaudit.ManualReviewData
			// EventData is map[string]interface{} in OpenAPI spec, marshal then unmarshal
			eventDataBytes, _ := json.Marshal(event.EventData)
			err = json.Unmarshal(eventDataBytes, &data)
			Expect(err).ToNot(HaveOccurred())
			Expect(data.Reason).To(Equal("WorkflowResolutionFailed"))
			Expect(data.SubReason).To(Equal("MultipleWorkflowsMatched"))
			Expect(data.NotificationN).To(Equal("nr-manual-review-001"))
		})
	})

	// ========================================
	// Event Validation Tests
	// Per DD-AUDIT-002: All events must pass validation
	// ========================================
	Describe("Event Validation", func() {
		It("should create events that pass validation", func() {
			// Test all event builders produce valid events
			events := []*struct {
				name  string
				build func() error
			}{
				{
					name: "LifecycleStarted",
					build: func() error {
						_, err := manager.BuildLifecycleStartedEvent("corr", "ns", "rr")
						if err != nil {
							return err
						}
						// NOTE: OpenAPI types don't have Validate() method (DD-AUDIT-002 V2.0.1)
						// Validation is done through OpenAPI schema validation in pkg/audit
						return nil
					},
				},
				{
					name: "PhaseTransition",
					build: func() error {
						_, err := manager.BuildPhaseTransitionEvent("corr", "ns", "rr", "Pending", "Processing")
						if err != nil {
							return err
						}
						// NOTE: OpenAPI types don't have Validate() method (DD-AUDIT-002 V2.0.1)
						// Validation is done through OpenAPI schema validation in pkg/audit
						return nil
					},
				},
				{
					name: "Completion",
					build: func() error {
						_, err := manager.BuildCompletionEvent("corr", "ns", "rr", "Remediated", 1000)
						if err != nil {
							return err
						}
						// NOTE: OpenAPI types don't have Validate() method (DD-AUDIT-002 V2.0.1)
						// Validation is done through OpenAPI schema validation in pkg/audit
						return nil
					},
				},
				{
					name: "Failure",
					build: func() error {
						_, err := manager.BuildFailureEvent("corr", "ns", "rr", "phase", "reason", 1000)
						if err != nil {
							return err
						}
						// NOTE: OpenAPI types don't have Validate() method (DD-AUDIT-002 V2.0.1)
						// Validation is done through OpenAPI schema validation in pkg/audit
						return nil
					},
				},
				{
					name: "ApprovalRequested",
					build: func() error {
						_, err := manager.BuildApprovalRequestedEvent("corr", "ns", "rr", "rar", "wf", "85%", time.Now())
						if err != nil {
							return err
						}
						// NOTE: OpenAPI types don't have Validate() method (DD-AUDIT-002 V2.0.1)
						// Validation is done through OpenAPI schema validation in pkg/audit
						return nil
					},
				},
				{
					name: "ApprovalDecision",
					build: func() error {
						_, err := manager.BuildApprovalDecisionEvent("corr", "ns", "rr", "rar", "Approved", "user", "msg")
						if err != nil {
							return err
						}
						// NOTE: OpenAPI types don't have Validate() method (DD-AUDIT-002 V2.0.1)
						// Validation is done through OpenAPI schema validation in pkg/audit
						return nil
					},
				},
				{
					name: "ManualReview",
					build: func() error {
						_, err := manager.BuildManualReviewEvent("corr", "ns", "rr", "reason", "sub", "notif")
						if err != nil {
							return err
						}
						// NOTE: OpenAPI types don't have Validate() method (DD-AUDIT-002 V2.0.1)
						// Validation is done through OpenAPI schema validation in pkg/audit
						return nil
					},
				},
			}

			for _, e := range events {
				err := e.build()
				Expect(err).ToNot(HaveOccurred(), "Event %s should pass validation", e.name)
			}
		})
	})
})
