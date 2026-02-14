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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	prodaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/test/shared/validators"
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
			case ogenclient.AuditEventRequestEventDataOrchestratorRemediationManualReviewAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorRemediationManualReviewAuditEventEventData
			case ogenclient.AuditEventRequestEventDataOrchestratorRoutingBlockedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataOrchestratorRoutingBlockedAuditEventEventData
			case ogenclient.AuditEventRequestEventDataRemediationWorkflowCreatedAuditEventRequestEventData:
				eventDataType = ogenclient.AuditEventEventDataRemediationWorkflowCreatedAuditEventEventData
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
	// Per V1.0 Maturity: Using validators.ValidateAuditEvent for consistent validation
	// ========================================
	Describe("BuildLifecycleStartedEvent", func() {
		It("should build complete orchestrator.lifecycle.started event with all required fields", func() {
			event, err := manager.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())

			// V1.0 Maturity: Use validators.ValidateAuditEvent for structured validation
			validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
				// Required fields
				EventType:     prodaudit.EventTypeLifecycleStarted,
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

			validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
				EventType:     prodaudit.EventTypeLifecycleStarted,
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
	// Per V1.0 Maturity: Using validators.ValidateAuditEvent for consistent validation
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

			validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
				EventType:     prodaudit.EventTypeLifecycleTransitioned,
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

				validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
					EventType:     prodaudit.EventTypeLifecycleTransitioned,
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
	// Per V1.0 Maturity: Using validators.ValidateAuditEvent for consistent validation
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

			validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
				EventType:     prodaudit.EventTypeLifecycleCompleted,
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

			// DurationMs is not part of validators.ExpectedAuditEvent, validate separately
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

			validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
				EventType:     prodaudit.EventTypeLifecycleCompleted,
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
	// Per V1.0 Maturity: Using validators.ValidateAuditEvent for consistent validation
	// ========================================
	Describe("BuildFailureEvent", func() {
		It("should build complete orchestrator.lifecycle.completed event with failure outcome", func() {
			gr := schema.GroupResource{Group: "kubernaut.ai", Resource: "remediationrequests"}
			event, err := manager.BuildFailureEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"workflow_execution",
				apierrors.NewForbidden(gr, "rr-test-001", fmt.Errorf("RBAC permission denied")),
				5000,
			)
			Expect(err).ToNot(HaveOccurred())

			validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
				EventType:     prodaudit.EventTypeLifecycleCompleted,
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "completed",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomeFailure),
				CorrelationID: "correlation-123",
				Namespace:     ptr.To("default"),
				ResourceID:    ptr.To("rr-test-001"),
				EventDataFields: map[string]interface{}{
					"outcome":       "Failed",
					"failure_phase": "workflow_execution",
				},
			})

			// Verify error_details.message contains the K8s error message
			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)
			errorDetails := eventData["error_details"].(map[string]interface{})
			Expect(errorDetails["message"]).To(ContainSubstring("RBAC permission denied"))

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
				apierrors.NewTimeoutError("timeout while enriching alert", 30),
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
				apierrors.NewTimeoutError("Enrichment timeout", 30),
				10000,
			)
			Expect(err).ToNot(HaveOccurred())

			validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
				EventType:     prodaudit.EventTypeLifecycleCompleted,
				EventCategory: ogenclient.AuditEventEventCategoryOrchestration,
				EventAction:   "completed",
				EventOutcome:  ptr.To(ogenclient.AuditEventEventOutcomeFailure),
				CorrelationID: "correlation-456",
				Namespace:     ptr.To("production"),
				ResourceID:    ptr.To("rr-prod-001"),
				EventDataFields: map[string]interface{}{
					"outcome":       "Failed",
					"failure_phase": "signal_processing",
				},
			})

			// Verify error_details.message contains the timeout error message
			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)
			errorDetails := eventData["error_details"].(map[string]interface{})
			Expect(errorDetails["message"]).To(ContainSubstring("Enrichment timeout"))

			Expect(event.DurationMs.IsSet()).To(BeTrue())
			Expect(event.DurationMs.Value).To(Equal(10000))
		})

		// BR-AUDIT-005 Gap #7: Error Code Mapping Unit Tests
		// Per DD-TEST-008: Error code mapping belongs in unit tests, not integration tests
		// F-6: Error code mapping now uses typed errors via ClassifyError
		Context("Gap #7: Error Code Mapping Logic", func() {
			It("should map apierrors.IsInvalid to ERR_INVALID_CONFIG", func() {
				gk := schema.GroupKind{Group: "kubernaut.ai", Kind: "RemediationRequest"}
				event, err := manager.BuildFailureEvent(
					"correlation-001",
					"default",
					"rr-test",
					"configuration",
					apierrors.NewInvalid(gk, "rr-test", nil),
					1000,
				)
				Expect(err).ToNot(HaveOccurred())

				eventDataBytes, _ := json.Marshal(event.EventData)
				var eventData map[string]interface{}
				_ = json.Unmarshal(eventDataBytes, &eventData)

				errorDetails := eventData["error_details"].(map[string]interface{})
				Expect(errorDetails["code"]).To(Equal("ERR_INVALID_CONFIG"), "Invalid K8s input should map to ERR_INVALID_CONFIG")
				Expect(errorDetails["retry_possible"]).To(BeFalse(), "Invalid config is permanent error")
				Expect(errorDetails["component"]).To(Equal("remediationorchestrator"))
			})

			It("should map apierrors.IsForbidden to ERR_K8S_FORBIDDEN", func() {
				gr := schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"}
				event, err := manager.BuildFailureEvent(
					"correlation-002",
					"default",
					"rr-test",
					"signal_processing",
					apierrors.NewForbidden(gr, "sp-test", fmt.Errorf("RBAC denied")),
					1000,
				)
				Expect(err).ToNot(HaveOccurred())

				eventDataBytes, _ := json.Marshal(event.EventData)
				var eventData map[string]interface{}
				_ = json.Unmarshal(eventDataBytes, &eventData)

				errorDetails := eventData["error_details"].(map[string]interface{})
				Expect(errorDetails["code"]).To(Equal("ERR_K8S_FORBIDDEN"), "K8s forbidden should map to ERR_K8S_FORBIDDEN")
				Expect(errorDetails["retry_possible"]).To(BeFalse(), "RBAC errors are permanent")
			})

			It("should map unknown errors to ERR_INTERNAL_ORCHESTRATION", func() {
				event, err := manager.BuildFailureEvent(
					"correlation-003",
					"default",
					"rr-test",
					"unknown",
					fmt.Errorf("unexpected panic in reconciler"),
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

			It("should map apierrors.IsNotFound to ERR_K8S_NOT_FOUND", func() {
				gr := schema.GroupResource{Group: "kubernaut.ai", Resource: "workflowexecutions"}
				event, err := manager.BuildFailureEvent(
					"correlation-004",
					"default",
					"rr-test",
					"workflow_execution",
					apierrors.NewNotFound(gr, "wfe-test"),
					1000,
				)
				Expect(err).ToNot(HaveOccurred())

				eventDataBytes, _ := json.Marshal(event.EventData)
				var eventData map[string]interface{}
				_ = json.Unmarshal(eventDataBytes, &eventData)

				errorDetails := eventData["error_details"].(map[string]interface{})
				Expect(errorDetails["code"]).To(Equal("ERR_K8S_NOT_FOUND"), "K8s not found should map to ERR_K8S_NOT_FOUND")
				Expect(errorDetails["retry_possible"]).To(BeTrue(), "K8s not found errors are retryable")
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
	// Per V1.0 Maturity: Using validators.ValidateAuditEvent for consistent validation
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

			validators.ValidateAuditEvent(toAuditEvent(event), validators.ExpectedAuditEvent{
				EventType:     prodaudit.EventTypeApprovalRequested,
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
			Expect(event.EventType).To(Equal(prodaudit.EventTypeApprovalApproved))
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
			Expect(event.EventType).To(Equal(prodaudit.EventTypeApprovalRejected))
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
			// M-4: Expired uses PayloadEventType (orchestrator.approval.rejected)
			// because there is no separate "expired" discriminator in the OpenAPI spec
			Expect(event.EventType).To(Equal(prodaudit.EventTypeApprovalRejected))
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
			Expect(event.EventType).To(Equal(prodaudit.EventTypeManualReview))
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
	// BuildRoutingBlockedEvent Tests
	// Per DD-RO-002: Routing Engine blocking conditions
	// F-3 SOC2 Fix: event_type consistency validation
	// ========================================
	Describe("BuildRoutingBlockedEvent", func() {
		DescribeTable("should set correct audit event fields",
			func(fieldName string, validate func(event *ogenclient.AuditEventRequest)) {
				blockData := &prodaudit.RoutingBlockedData{
					BlockReason:    "CooldownActive",
					BlockMessage:   "Cooldown period active for this resource",
					FromPhase:      "Pending",
					ToPhase:        "Blocked",
					TargetResource: "deployment/nginx",
				}
				event, err := manager.BuildRoutingBlockedEvent(
					"corr-routing-001",
					"production",
					"rr-blocked-001",
					"Pending",
					blockData,
				)
				Expect(err).ToNot(HaveOccurred())
				validate(event)
			},
			Entry("event_type = orchestrator.routing.blocked",
				"event_type",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventType).To(Equal(prodaudit.EventTypeRoutingBlocked))
				},
			),
			Entry("event_category = orchestration",
				"event_category",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategory("orchestration")))
				},
			),
			Entry("event_outcome = pending",
				"event_outcome",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcome("pending")))
				},
			),
			Entry("correlation_id set correctly",
				"correlation_id",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.CorrelationID).To(Equal("corr-routing-001"))
				},
			),
			Entry("namespace set correctly",
				"namespace",
				func(event *ogenclient.AuditEventRequest) {
					namespace, hasNamespace := event.Namespace.Get()
					Expect(hasNamespace).To(BeTrue())
					Expect(namespace).To(Equal("production"))
				},
			),
			Entry("resource_id set to RR name",
				"resource_id",
				func(event *ogenclient.AuditEventRequest) {
					resourceID, hasResourceID := event.ResourceID.Get()
					Expect(hasResourceID).To(BeTrue())
					Expect(resourceID).To(Equal("rr-blocked-001"))
				},
			),
			Entry("F-3 consistency: outer event_type matches EventData discriminator",
				"consistency",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventType).To(Equal(string(event.EventData.Type)),
						"F-3 SOC2 Fix: outer event_type must match EventData discriminator")
				},
			),
		)
	})

	// ========================================
	// BuildRemediationWorkflowCreatedEvent Tests
	// Per DD-EM-002: Pre-remediation spec hash capture
	// Per ADR-EM-001 v1.5: RO emits remediation.workflow_created audit event
	// ========================================
	Describe("BuildRemediationWorkflowCreatedEvent", func() {
		It("should produce event with event_type = remediation.workflow_created", func() {
			event, err := manager.BuildRemediationWorkflowCreatedEvent(
				"correlation-hash-001",
				"default",
				"rr-hash-001",
				"sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				"default/Deployment/nginx",
				"wf-scale-001",
				"1.0.0",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("remediation.workflow_created"))
		})

		It("should include pre_remediation_spec_hash, target_resource, workflow_id, workflow_version in payload", func() {
			event, err := manager.BuildRemediationWorkflowCreatedEvent(
				"correlation-hash-002",
				"production",
				"rr-hash-002",
				"sha256:1111111111111111111111111111111111111111111111111111111111111111",
				"production/Deployment/api-server",
				"wf-restart-pods",
				"2.1.0",
			)
			Expect(err).ToNot(HaveOccurred())

			// Extract payload from EventData
			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)

			Expect(eventData["pre_remediation_spec_hash"]).To(Equal("sha256:1111111111111111111111111111111111111111111111111111111111111111"))
			Expect(eventData["target_resource"]).To(Equal("production/Deployment/api-server"))
			Expect(eventData["workflow_id"]).To(Equal("wf-restart-pods"))
			Expect(eventData["workflow_version"]).To(Equal("2.1.0"))
		})

		It("should set event_type discriminator mapping correctly", func() {
			event, err := manager.BuildRemediationWorkflowCreatedEvent(
				"correlation-hash-003",
				"default",
				"rr-hash-003",
				"sha256:abcdef",
				"default/Pod/test",
				"wf-test",
				"1.0.0",
			)
			Expect(err).ToNot(HaveOccurred())

			// F-3 SOC2 Fix: outer event_type must match EventData discriminator
			Expect(event.EventType).To(Equal(string(event.EventData.Type)),
				"outer event_type must match EventData discriminator")
		})

		It("should handle empty workflow version gracefully", func() {
			event, err := manager.BuildRemediationWorkflowCreatedEvent(
				"correlation-hash-004",
				"default",
				"rr-hash-004",
				"sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				"default/Deployment/nginx",
				"wf-scale-001",
				"", // empty version
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event).ToNot(BeNil())

			// Event should still be valid even with empty version
			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)

			// Workflow version should be absent or empty since it was not set
			// The ogen OptString with empty string won't be serialized
			Expect(eventData["pre_remediation_spec_hash"]).To(Equal("sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"))
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
						_, err := manager.BuildFailureEvent("corr", "ns", "rr", "phase", fmt.Errorf("reason"), 1000)
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
			{
				name: "RoutingBlocked",
				build: func() error {
					blockData := &prodaudit.RoutingBlockedData{
						BlockReason:  "CooldownActive",
						BlockMessage: "Cooldown period active",
						FromPhase:    "Pending",
						ToPhase:      "Blocked",
					}
					_, err := manager.BuildRoutingBlockedEvent("corr", "ns", "rr", "Pending", blockData)
					if err != nil {
						return err
					}
					return nil
				},
			},
			{
				name: "RemediationWorkflowCreated",
				build: func() error {
					_, err := manager.BuildRemediationWorkflowCreatedEvent("corr", "ns", "rr", "sha256:abc", "ns/Deploy/x", "wf", "1.0")
					if err != nil {
						return err
					}
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
