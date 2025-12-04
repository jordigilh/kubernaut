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

package workflowexecution

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// Unit Tests: Audit Trail Implementation
// Per TESTING_GUIDELINES.md: These tests validate function/method behavior
// for audit event creation and correlation tracking.
// =============================================================================

var _ = Describe("AuditHelper", func() {
	var (
		auditHelper *AuditHelper
		wfe         *workflowexecutionv1.WorkflowExecution
	)

	BeforeEach(func() {
		// TDD: AuditHelper struct is defined by tests
		auditHelper = NewAuditHelper()

		wfe = &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfe",
				Namespace: "default",
				UID:       "test-uid-123",
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:     "increase-memory",
					Version:        "v1.0.0",
					ContainerImage: "ghcr.io/kubernaut/workflows/increase-memory:v1.0.0",
				},
				TargetResource: "default/deployment/test-app",
			},
		}
	})

	Context("when building audit events", func() {
		It("should create audit event for creation", func() {
			// When: Building creation event
			event := auditHelper.BuildCreatedEvent(wfe)

			// Then: Event has required fields
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal(AuditEventTypeCreated))
			Expect(event.ResourceName).To(Equal("test-wfe"))
			Expect(event.ResourceNamespace).To(Equal("default"))
			Expect(event.WorkflowID).To(Equal("increase-memory"))
			Expect(event.TargetResource).To(Equal("default/deployment/test-app"))
			Expect(event.Timestamp).NotTo(BeZero())
		})

		It("should create audit event for phase transition", func() {
			// When: Building phase transition event
			event := auditHelper.BuildPhaseTransitionEvent(wfe, "Pending", "Running")

			// Then: Event has required fields
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal(AuditEventTypePhaseTransition))
			Expect(event.FromPhase).To(Equal("Pending"))
			Expect(event.ToPhase).To(Equal("Running"))
		})

		It("should create audit event for completion", func() {
			// Given: WFE with completion details
			now := metav1.Now()
			wfe.Status.Phase = workflowexecutionv1.PhaseCompleted
			wfe.Status.StartTime = &metav1.Time{Time: now.Add(-30 * time.Second)}
			wfe.Status.CompletionTime = &now
			wfe.Status.Duration = "30s"

			// When: Building completion event
			event := auditHelper.BuildCompletedEvent(wfe)

			// Then: Event has required fields
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal(AuditEventTypeCompleted))
			Expect(event.Outcome).To(Equal("Success"))
			Expect(event.Duration).To(Equal("30s"))
		})

		It("should create audit event for failure", func() {
			// Given: WFE with failure details
			wfe.Status.Phase = workflowexecutionv1.PhaseFailed
			wfe.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
				Reason:  "OOMKilled",
				Message: "Container exceeded memory limit",
			}

			// When: Building failure event
			event := auditHelper.BuildFailedEvent(wfe)

			// Then: Event has required fields
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal(AuditEventTypeFailed))
			Expect(event.Outcome).To(Equal("Failure"))
			Expect(event.FailureReason).To(Equal("OOMKilled"))
			Expect(event.FailureMessage).To(Equal("Container exceeded memory limit"))
		})

		It("should create audit event for skip", func() {
			// Given: WFE with skip details
			wfe.Status.Phase = workflowexecutionv1.PhaseSkipped
			wfe.Status.SkipDetails = &workflowexecutionv1.SkipDetails{
				Reason:  "ResourceBusy",
				Message: "Resource is being remediated by another workflow",
			}

			// When: Building skip event
			event := auditHelper.BuildSkippedEvent(wfe)

			// Then: Event has required fields
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal(AuditEventTypeSkipped))
			Expect(event.Outcome).To(Equal("Skipped"))
			Expect(event.SkipReason).To(Equal("ResourceBusy"))
		})

		It("should create audit event for PipelineRun creation", func() {
			// When: Building PipelineRun created event
			event := auditHelper.BuildPipelineRunCreatedEvent(wfe, "wfe-abc123")

			// Then: Event has required fields
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal(AuditEventTypePipelineRunCreated))
			Expect(event.PipelineRunName).To(Equal("wfe-abc123"))
		})
	})

	Context("when getting correlation ID", func() {
		It("should return UID as correlation ID", func() {
			// When: Getting correlation ID
			correlationID := auditHelper.GetCorrelationID(wfe)

			// Then: Should be the UID
			Expect(correlationID).To(Equal("test-uid-123"))
		})
	})
})

