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

package audit_test

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
)

func TestAuditHelpers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Remediation Orchestrator Audit Helpers Suite")
}

var _ = Describe("Audit Helpers", func() {
	var helpers *roaudit.Helpers

	BeforeEach(func() {
		helpers = roaudit.NewHelpers(roaudit.ServiceName)
	})

	Describe("NewHelpers", func() {
		It("should create helpers with correct service name", func() {
			h := roaudit.NewHelpers("test-service")
			Expect(h).ToNot(BeNil())
		})
	})

	// ========================================
	// BuildLifecycleStartedEvent Tests
	// Per DD-AUDIT-003: orchestrator.lifecycle.started (P1)
	// ========================================
	Describe("BuildLifecycleStartedEvent", func() {
		It("should build event with correct event type per DD-AUDIT-003", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.started"))
		})

		It("should set event category to lifecycle", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventCategory).To(Equal("lifecycle"))
		})

		It("should set event action to started", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventAction).To(Equal("started"))
		})

		It("should set event outcome to success", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventOutcome).To(Equal("success"))
		})

		It("should set actor type to service", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.ActorType).To(Equal("service"))
			Expect(event.ActorID).To(Equal(roaudit.ServiceName))
		})

		It("should set resource type to RemediationRequest", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.ResourceType).To(Equal("RemediationRequest"))
			Expect(event.ResourceID).To(Equal("rr-test-001"))
		})

		It("should set correlation ID", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.CorrelationID).To(Equal("correlation-123"))
		})

		It("should set namespace", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"production",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.Namespace).ToNot(BeNil())
			Expect(*event.Namespace).To(Equal("production"))
		})

		It("should include event data with RR name and namespace", func() {
			event, err := helpers.BuildLifecycleStartedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventData).ToNot(BeEmpty())

			var data roaudit.LifecycleStartedData
			err = json.Unmarshal(event.EventData, &data)
			Expect(err).ToNot(HaveOccurred())
			Expect(data.RRName).To(Equal("rr-test-001"))
			Expect(data.Namespace).To(Equal("default"))
		})
	})

	// ========================================
	// BuildPhaseTransitionEvent Tests
	// Per DD-AUDIT-003: orchestrator.phase.transitioned (P1)
	// ========================================
	Describe("BuildPhaseTransitionEvent", func() {
		It("should build event with correct event type per DD-AUDIT-003", func() {
			event, err := helpers.BuildPhaseTransitionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Pending",
				"Processing",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.phase.transitioned"))
		})

		It("should set event category to phase", func() {
			event, err := helpers.BuildPhaseTransitionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Pending",
				"Processing",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventCategory).To(Equal("phase"))
		})

		It("should set event action to transitioned", func() {
			event, err := helpers.BuildPhaseTransitionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Pending",
				"Processing",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventAction).To(Equal("transitioned"))
		})

		It("should include from and to phases in event data", func() {
			event, err := helpers.BuildPhaseTransitionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Analyzing",
				"Executing",
			)
			Expect(err).ToNot(HaveOccurred())

			var data roaudit.PhaseTransitionData
			err = json.Unmarshal(event.EventData, &data)
			Expect(err).ToNot(HaveOccurred())
			Expect(data.FromPhase).To(Equal("Analyzing"))
			Expect(data.ToPhase).To(Equal("Executing"))
		})

		It("should handle all phase transitions", func() {
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
				event, err := helpers.BuildPhaseTransitionEvent(
					"correlation-123",
					"default",
					"rr-test-001",
					t.from,
					t.to,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(event.EventType).To(Equal("orchestrator.phase.transitioned"))
			}
		})
	})

	// ========================================
	// BuildCompletionEvent Tests
	// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1)
	// ========================================
	Describe("BuildCompletionEvent", func() {
		It("should build event with correct event type per DD-AUDIT-003", func() {
			event, err := helpers.BuildCompletionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Remediated",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.completed"))
		})

		It("should set event category to lifecycle", func() {
			event, err := helpers.BuildCompletionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Remediated",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventCategory).To(Equal("lifecycle"))
		})

		It("should set event outcome to success for completion", func() {
			event, err := helpers.BuildCompletionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Remediated",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventOutcome).To(Equal("success"))
		})

		It("should include duration in milliseconds", func() {
			event, err := helpers.BuildCompletionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"Remediated",
				12345,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.DurationMs).ToNot(BeNil())
			Expect(*event.DurationMs).To(Equal(12345))
		})

		It("should include outcome in event data", func() {
			event, err := helpers.BuildCompletionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"NoActionRequired",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())

			var data roaudit.CompletionData
			err = json.Unmarshal(event.EventData, &data)
			Expect(err).ToNot(HaveOccurred())
			Expect(data.Outcome).To(Equal("NoActionRequired"))
		})
	})

	// ========================================
	// BuildFailureEvent Tests
	// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1) with failure outcome
	// ========================================
	Describe("BuildFailureEvent", func() {
		It("should build event with correct event type per DD-AUDIT-003", func() {
			event, err := helpers.BuildFailureEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"workflow_execution",
				"RBAC permission denied",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())
			// Failures are also lifecycle.completed events
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.completed"))
		})

		It("should set event outcome to failure", func() {
			event, err := helpers.BuildFailureEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"ai_analysis",
				"LLM timeout",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventOutcome).To(Equal("failure"))
		})

		It("should include error message", func() {
			event, err := helpers.BuildFailureEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"workflow_execution",
				"Deployment not found",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.ErrorMessage).ToNot(BeNil())
			Expect(*event.ErrorMessage).To(Equal("Deployment not found"))
		})

		It("should include failure phase and reason in event data", func() {
			event, err := helpers.BuildFailureEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"signal_processing",
				"Enrichment timeout",
				10000,
			)
			Expect(err).ToNot(HaveOccurred())

			var data roaudit.CompletionData
			err = json.Unmarshal(event.EventData, &data)
			Expect(err).ToNot(HaveOccurred())
			Expect(data.FailurePhase).To(Equal("signal_processing"))
			Expect(data.FailureReason).To(Equal("Enrichment timeout"))
			Expect(data.Outcome).To(Equal("Failed"))
		})
	})

	// ========================================
	// BuildApprovalRequestedEvent Tests
	// Related to ADR-040 (RemediationApprovalRequest)
	// ========================================
	Describe("BuildApprovalRequestedEvent", func() {
		It("should build event with orchestrator prefix", func() {
			event, err := helpers.BuildApprovalRequestedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"wf-scale-deployment",
				"85%",
				time.Now().Add(24*time.Hour),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.approval.requested"))
		})

		It("should set resource type to RemediationApprovalRequest", func() {
			event, err := helpers.BuildApprovalRequestedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"wf-scale-deployment",
				"85%",
				time.Now().Add(24*time.Hour),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.ResourceType).To(Equal("RemediationApprovalRequest"))
			Expect(event.ResourceID).To(Equal("rar-rr-test-001"))
		})

		It("should include approval context in event data", func() {
			requiredBy := time.Now().Add(24 * time.Hour)
			event, err := helpers.BuildApprovalRequestedEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"wf-scale-deployment",
				"85%",
				requiredBy,
			)
			Expect(err).ToNot(HaveOccurred())

			var data roaudit.ApprovalData
			err = json.Unmarshal(event.EventData, &data)
			Expect(err).ToNot(HaveOccurred())
			Expect(data.RARName).To(Equal("rar-rr-test-001"))
			Expect(data.RRName).To(Equal("rr-test-001"))
			Expect(data.WorkflowID).To(Equal("wf-scale-deployment"))
			Expect(data.ConfidenceStr).To(Equal("85%"))
		})
	})

	// ========================================
	// BuildApprovalDecisionEvent Tests
	// Related to ADR-040 (RemediationApprovalRequest)
	// ========================================
	Describe("BuildApprovalDecisionEvent", func() {
		It("should build approved event with correct type", func() {
			event, err := helpers.BuildApprovalDecisionEvent(
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
			Expect(event.EventOutcome).To(Equal("success"))
		})

		It("should build rejected event with correct type", func() {
			event, err := helpers.BuildApprovalDecisionEvent(
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
			Expect(event.EventOutcome).To(Equal("failure"))
		})

		It("should build expired event with correct type", func() {
			event, err := helpers.BuildApprovalDecisionEvent(
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
			Expect(event.EventOutcome).To(Equal("failure"))
		})

		It("should set actor type to user for non-system decisions", func() {
			event, err := helpers.BuildApprovalDecisionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"Approved",
				"operator@example.com",
				"LGTM",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.ActorType).To(Equal("user"))
			Expect(event.ActorID).To(Equal("operator@example.com"))
		})

		It("should set actor type to service for system decisions", func() {
			event, err := helpers.BuildApprovalDecisionEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"rar-rr-test-001",
				"Expired",
				"system",
				"Timeout",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.ActorType).To(Equal("service"))
			Expect(event.ActorID).To(Equal(roaudit.ServiceName))
		})
	})

	// ========================================
	// BuildManualReviewEvent Tests
	// Related to BR-ORCH-036 (Manual Review Notifications)
	// ========================================
	Describe("BuildManualReviewEvent", func() {
		It("should build event with orchestrator prefix", func() {
			event, err := helpers.BuildManualReviewEvent(
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
			event, err := helpers.BuildManualReviewEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"InvestigationInconclusive",
				"LLMUncertain",
				"nr-manual-review-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventOutcome).To(Equal("pending"))
		})

		It("should set severity to warning", func() {
			event, err := helpers.BuildManualReviewEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"ExhaustedRetries",
				"MaxRetriesReached",
				"nr-manual-review-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.Severity).ToNot(BeNil())
			Expect(*event.Severity).To(Equal("warning"))
		})

		It("should include reason and sub-reason in event data", func() {
			event, err := helpers.BuildManualReviewEvent(
				"correlation-123",
				"default",
				"rr-test-001",
				"WorkflowResolutionFailed",
				"MultipleWorkflowsMatched",
				"nr-manual-review-001",
			)
			Expect(err).ToNot(HaveOccurred())

			var data roaudit.ManualReviewData
			err = json.Unmarshal(event.EventData, &data)
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
						event, err := helpers.BuildLifecycleStartedEvent("corr", "ns", "rr")
						if err != nil {
							return err
						}
						return event.Validate()
					},
				},
				{
					name: "PhaseTransition",
					build: func() error {
						event, err := helpers.BuildPhaseTransitionEvent("corr", "ns", "rr", "Pending", "Processing")
						if err != nil {
							return err
						}
						return event.Validate()
					},
				},
				{
					name: "Completion",
					build: func() error {
						event, err := helpers.BuildCompletionEvent("corr", "ns", "rr", "Remediated", 1000)
						if err != nil {
							return err
						}
						return event.Validate()
					},
				},
				{
					name: "Failure",
					build: func() error {
						event, err := helpers.BuildFailureEvent("corr", "ns", "rr", "phase", "reason", 1000)
						if err != nil {
							return err
						}
						return event.Validate()
					},
				},
				{
					name: "ApprovalRequested",
					build: func() error {
						event, err := helpers.BuildApprovalRequestedEvent("corr", "ns", "rr", "rar", "wf", "85%", time.Now())
						if err != nil {
							return err
						}
						return event.Validate()
					},
				},
				{
					name: "ApprovalDecision",
					build: func() error {
						event, err := helpers.BuildApprovalDecisionEvent("corr", "ns", "rr", "rar", "Approved", "user", "msg")
						if err != nil {
							return err
						}
						return event.Validate()
					},
				},
				{
					name: "ManualReview",
					build: func() error {
						event, err := helpers.BuildManualReviewEvent("corr", "ns", "rr", "reason", "sub", "notif")
						if err != nil {
							return err
						}
						return event.Validate()
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

