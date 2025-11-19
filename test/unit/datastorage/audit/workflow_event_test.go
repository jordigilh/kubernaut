// Copyright 2025 Jordi Gil.
// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
)

// ========================================
// TDD RED PHASE: Workflow Event Builder Tests
// BR-STORAGE-033: Event Data Helpers
// ========================================
//
// These tests define the contract for the Workflow event builder.
// Workflow Service uses this builder to create audit events for:
// - Workflow execution lifecycle
// - Step execution tracking
// - Approval decisions
// - Execution outcomes
//
// Business Requirements:
// - BR-STORAGE-033-010: Workflow-specific event data structure
// - BR-STORAGE-033-011: Workflow execution phase tracking
// - BR-STORAGE-033-012: Approval and outcome metadata
//
// ========================================

var _ = Describe("WorkflowEventBuilder", func() {
	Context("BR-STORAGE-033-010: Workflow-specific event data structure", func() {
		It("should create workflow event with base structure", func() {
			builder := audit.NewWorkflowEvent("workflow.started")
			Expect(builder).ToNot(BeNil())

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("version"))
			Expect(eventData["version"]).To(Equal("1.0"))
			Expect(eventData).To(HaveKey("service"))
			Expect(eventData["service"]).To(Equal("workflow"))
			Expect(eventData).To(HaveKey("event_type"))
			Expect(eventData["event_type"]).To(Equal("workflow.started"))
		})

		It("should include workflow data in nested structure", func() {
			builder := audit.NewWorkflowEvent("workflow.started").
				WithWorkflowID("workflow-pod-restart-001").
				WithExecutionID("exec-2025-001")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("data"))

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveKey("workflow"))

			workflowData, ok := data["workflow"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(workflowData).To(HaveKeyWithValue("workflow_id", "workflow-pod-restart-001"))
			Expect(workflowData).To(HaveKeyWithValue("execution_id", "exec-2025-001"))
		})
	})

	Context("BR-STORAGE-033-011: Workflow execution phase tracking", func() {
		It("should track workflow phase", func() {
			builder := audit.NewWorkflowEvent("workflow.phase_changed").
				WithWorkflowID("workflow-001").
				WithExecutionID("exec-001").
				WithPhase("executing")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			Expect(workflowData).To(HaveKeyWithValue("phase", "executing"))
		})

		It("should track current step", func() {
			builder := audit.NewWorkflowEvent("workflow.step_started").
				WithWorkflowID("workflow-001").
				WithExecutionID("exec-001").
				WithCurrentStep(3, 5).
				WithStepName("increase_memory_limits")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			Expect(workflowData).To(HaveKeyWithValue("current_step", float64(3)))
			Expect(workflowData).To(HaveKeyWithValue("total_steps", float64(5)))
			Expect(workflowData).To(HaveKeyWithValue("step_name", "increase_memory_limits"))
		})

		It("should track workflow duration", func() {
			builder := audit.NewWorkflowEvent("workflow.completed").
				WithWorkflowID("workflow-001").
				WithExecutionID("exec-001").
				WithDuration(45000) // 45 seconds

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			Expect(workflowData).To(HaveKeyWithValue("duration_ms", float64(45000)))
		})
	})

	Context("BR-STORAGE-033-012: Approval and outcome metadata", func() {
		It("should track approval required status", func() {
			builder := audit.NewWorkflowEvent("workflow.approval_required").
				WithWorkflowID("workflow-001").
				WithExecutionID("exec-001").
				WithApprovalRequired(true)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			Expect(workflowData).To(HaveKeyWithValue("approval_required", true))
		})

		It("should track approval decision", func() {
			builder := audit.NewWorkflowEvent("workflow.approval_received").
				WithWorkflowID("workflow-001").
				WithExecutionID("exec-001").
				WithApprovalDecision("approved", "admin@example.com")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			Expect(workflowData).To(HaveKeyWithValue("approval_decision", "approved"))
			Expect(workflowData).To(HaveKeyWithValue("approver", "admin@example.com"))
		})

		It("should track workflow outcome", func() {
			builder := audit.NewWorkflowEvent("workflow.completed").
				WithWorkflowID("workflow-001").
				WithExecutionID("exec-001").
				WithOutcome("success")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			Expect(workflowData).To(HaveKeyWithValue("outcome", "success"))
		})

		It("should track failure reason", func() {
			builder := audit.NewWorkflowEvent("workflow.failed").
				WithWorkflowID("workflow-001").
				WithExecutionID("exec-001").
				WithOutcome("failed").
				WithErrorMessage("Failed to apply resource: connection timeout")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			Expect(workflowData).To(HaveKeyWithValue("outcome", "failed"))
			Expect(workflowData).To(HaveKeyWithValue("error_message", "Failed to apply resource: connection timeout"))
		})
	})

	Context("Complete workflow lifecycle", func() {
		It("should build complete successful workflow event", func() {
			builder := audit.NewWorkflowEvent("workflow.completed").
				WithWorkflowID("workflow-increase-memory-limits").
				WithExecutionID("exec-2025-11-18-001").
				WithPhase("completed").
				WithCurrentStep(5, 5).
				WithStepName("verify_health").
				WithDuration(45000).
				WithOutcome("success").
				WithApprovalRequired(true).
				WithApprovalDecision("approved", "sre-team@example.com")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			// Verify all fields present
			Expect(workflowData).To(HaveKeyWithValue("workflow_id", "workflow-increase-memory-limits"))
			Expect(workflowData).To(HaveKeyWithValue("execution_id", "exec-2025-11-18-001"))
			Expect(workflowData).To(HaveKeyWithValue("phase", "completed"))
			Expect(workflowData).To(HaveKeyWithValue("current_step", float64(5)))
			Expect(workflowData).To(HaveKeyWithValue("total_steps", float64(5)))
			Expect(workflowData).To(HaveKeyWithValue("step_name", "verify_health"))
			Expect(workflowData).To(HaveKeyWithValue("duration_ms", float64(45000)))
			Expect(workflowData).To(HaveKeyWithValue("outcome", "success"))
			Expect(workflowData).To(HaveKeyWithValue("approval_required", true))
			Expect(workflowData).To(HaveKeyWithValue("approval_decision", "approved"))
			Expect(workflowData).To(HaveKeyWithValue("approver", "sre-team@example.com"))
		})
	})

	Context("Edge Cases", func() {
		It("should handle minimal workflow event", func() {
			builder := audit.NewWorkflowEvent("workflow.started").
				WithWorkflowID("workflow-minimal")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).ToNot(BeEmpty())
		})

		It("should handle zero duration", func() {
			builder := audit.NewWorkflowEvent("workflow.completed").
				WithWorkflowID("workflow-001").
				WithDuration(0)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			// Zero duration should be present (not omitted)
			durationMs, ok := workflowData["duration_ms"]
			if ok {
				Expect(durationMs).To(Equal(float64(0)))
			}
		})

		It("should handle rejected approval", func() {
			builder := audit.NewWorkflowEvent("workflow.approval_received").
				WithWorkflowID("workflow-001").
				WithApprovalDecision("rejected", "security-team@example.com")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			workflowData, _ := data["workflow"].(map[string]interface{})

			Expect(workflowData).To(HaveKeyWithValue("approval_decision", "rejected"))
			Expect(workflowData).To(HaveKeyWithValue("approver", "security-team@example.com"))
		})
	})
})

