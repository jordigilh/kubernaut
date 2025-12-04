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

package workflowexecution_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
)

var _ = Describe("WorkflowExecution Controller Unit Tests", func() {

	// ========================================
	// PipelineRun Name Generation Tests (DD-WE-003)
	// ========================================
	Describe("pipelineRunName", func() {
		// Access the function via the package (it's unexported, so we test indirectly)
		// For unexported functions, we test via exported behaviors
	})

	// ========================================
	// Constants Tests
	// ========================================
	Describe("Constants", func() {
		It("should have correct default cooldown period", func() {
			Expect(workflowexecution.DefaultCooldownPeriod).To(Equal(5 * time.Minute))
		})

		It("should have correct default service account name", func() {
			Expect(workflowexecution.DefaultServiceAccountName).To(Equal("kubernaut-workflow-runner"))
		})
	})

	// ========================================
	// Phase Constants Tests
	// ========================================
	Describe("Phase Constants", func() {
		It("should define all required phases", func() {
			Expect(workflowexecutionv1alpha1.PhasePending).To(Equal("Pending"))
			Expect(workflowexecutionv1alpha1.PhaseRunning).To(Equal("Running"))
			Expect(workflowexecutionv1alpha1.PhaseCompleted).To(Equal("Completed"))
			Expect(workflowexecutionv1alpha1.PhaseFailed).To(Equal("Failed"))
			Expect(workflowexecutionv1alpha1.PhaseSkipped).To(Equal("Skipped"))
		})
	})

	// ========================================
	// Skip Reason Constants Tests
	// ========================================
	Describe("Skip Reason Constants", func() {
		It("should define ResourceBusy skip reason", func() {
			Expect(workflowexecutionv1alpha1.SkipReasonResourceBusy).To(Equal("ResourceBusy"))
		})

		It("should define RecentlyRemediated skip reason", func() {
			Expect(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated).To(Equal("RecentlyRemediated"))
		})
	})

	// ========================================
	// Failure Reason Constants Tests
	// ========================================
	Describe("Failure Reason Constants", func() {
		It("should define all failure reasons", func() {
			Expect(workflowexecutionv1alpha1.FailureReasonOOMKilled).To(Equal("OOMKilled"))
			Expect(workflowexecutionv1alpha1.FailureReasonDeadlineExceeded).To(Equal("DeadlineExceeded"))
			Expect(workflowexecutionv1alpha1.FailureReasonForbidden).To(Equal("Forbidden"))
			Expect(workflowexecutionv1alpha1.FailureReasonResourceExhausted).To(Equal("ResourceExhausted"))
			Expect(workflowexecutionv1alpha1.FailureReasonConfigurationError).To(Equal("ConfigurationError"))
			Expect(workflowexecutionv1alpha1.FailureReasonImagePullBackOff).To(Equal("ImagePullBackOff"))
			Expect(workflowexecutionv1alpha1.FailureReasonUnknown).To(Equal("Unknown"))
		})
	})

	// ========================================
	// Outcome Constants Tests
	// ========================================
	Describe("Outcome Constants", func() {
		It("should define all outcomes", func() {
			Expect(workflowexecutionv1alpha1.OutcomeSuccess).To(Equal("Success"))
			Expect(workflowexecutionv1alpha1.OutcomeFailure).To(Equal("Failure"))
			Expect(workflowexecutionv1alpha1.OutcomeSkipped).To(Equal("Skipped"))
		})
	})

	// ========================================
	// WorkflowExecutionSpec Validation Tests
	// ========================================
	Describe("WorkflowExecutionSpec", func() {
		var spec workflowexecutionv1alpha1.WorkflowExecutionSpec

		BeforeEach(func() {
			spec = workflowexecutionv1alpha1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "test-remediation",
					Namespace: "test-ns",
				},
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:     "oomkill-increase-memory",
					Version:        "1.0.0",
					ContainerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0",
				},
				TargetResource: "test-ns/deployment/test-app",
				Confidence:     0.92,
				Rationale:      "High confidence OOMKill pattern match",
			}
		})

		It("should have valid workflow reference", func() {
			Expect(spec.WorkflowRef.WorkflowID).To(Equal("oomkill-increase-memory"))
			Expect(spec.WorkflowRef.Version).To(Equal("1.0.0"))
			Expect(spec.WorkflowRef.ContainerImage).ToNot(BeEmpty())
		})

		It("should have valid target resource", func() {
			Expect(spec.TargetResource).To(Equal("test-ns/deployment/test-app"))
		})

		It("should have confidence between 0 and 1", func() {
			Expect(spec.Confidence).To(BeNumerically(">=", 0))
			Expect(spec.Confidence).To(BeNumerically("<=", 1))
		})
	})

	// ========================================
	// WorkflowExecutionStatus Tests
	// ========================================
	Describe("WorkflowExecutionStatus", func() {
		var status workflowexecutionv1alpha1.WorkflowExecutionStatus

		BeforeEach(func() {
			now := metav1.Now()
			status = workflowexecutionv1alpha1.WorkflowExecutionStatus{
				Phase:          workflowexecutionv1alpha1.PhaseRunning,
				StartTime:      &now,
				PipelineRunRef: &corev1.LocalObjectReference{Name: "wfe-abc123"},
			}
		})

		It("should track phase correctly", func() {
			Expect(status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning))
		})

		It("should have start time set", func() {
			Expect(status.StartTime).ToNot(BeNil())
		})

		It("should reference PipelineRun", func() {
			Expect(status.PipelineRunRef).ToNot(BeNil())
			Expect(status.PipelineRunRef.Name).To(Equal("wfe-abc123"))
		})
	})

	// ========================================
	// FailureDetails Tests (BR-WE-004)
	// ========================================
	Describe("FailureDetails", func() {
		It("should contain all required failure information", func() {
			now := metav1.Now()
			exitCode := int32(137)

			details := workflowexecutionv1alpha1.FailureDetails{
				FailedTaskIndex:            1,
				FailedTaskName:             "apply-memory-increase",
				FailedStepName:             "kubectl-apply",
				Reason:                     workflowexecutionv1alpha1.FailureReasonOOMKilled,
				Message:                    "Container exceeded memory limits",
				ExitCode:                   &exitCode,
				FailedAt:                   now,
				ExecutionTimeBeforeFailure: "2m30s",
				NaturalLanguageSummary:     "Task 'apply-memory-increase' failed with OOMKilled",
				WasExecutionFailure:        true,
			}

			Expect(details.FailedTaskIndex).To(Equal(1))
			Expect(details.FailedTaskName).To(Equal("apply-memory-increase"))
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonOOMKilled))
			Expect(*details.ExitCode).To(Equal(int32(137)))
			Expect(details.WasExecutionFailure).To(BeTrue())
		})
	})

	// ========================================
	// SkipDetails Tests (DD-WE-001)
	// ========================================
	Describe("SkipDetails", func() {
		It("should contain ResourceBusy skip information", func() {
			now := metav1.Now()

			details := workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonResourceBusy,
				Message:   "Resource test-ns/deployment/test-app is busy",
				SkippedAt: now,
				ConflictingWorkflow: &workflowexecutionv1alpha1.ConflictingWorkflowRef{
					Name:           "blocking-wfe",
					WorkflowID:     "oomkill-increase-memory",
					TargetResource: "test-ns/deployment/test-app",
					StartedAt:      now,
				},
			}

			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
			Expect(details.ConflictingWorkflow).ToNot(BeNil())
			Expect(details.ConflictingWorkflow.Name).To(Equal("blocking-wfe"))
		})

		It("should contain RecentlyRemediated skip information", func() {
			now := metav1.Now()

			details := workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonRecentlyRemediated,
				Message:   "Same workflow ran recently",
				SkippedAt: now,
				RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
					Name:              "recent-wfe",
					WorkflowID:        "oomkill-increase-memory",
					CompletedAt:       now,
					Outcome:           workflowexecutionv1alpha1.PhaseCompleted,
					TargetResource:    "test-ns/deployment/test-app",
					CooldownRemaining: "3m30s",
				},
			}

			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated))
			Expect(details.RecentRemediation).ToNot(BeNil())
			Expect(details.RecentRemediation.CooldownRemaining).To(Equal("3m30s"))
		})
	})

	// ========================================
	// WorkflowRef Tests (ADR-043)
	// ========================================
	Describe("WorkflowRef", func() {
		It("should contain OCI bundle reference", func() {
			ref := workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:      "oomkill-increase-memory",
				Version:         "1.0.0",
				ContainerImage:  "quay.io/kubernaut/workflow-oomkill:v1.0.0",
				ContainerDigest: "sha256:abc123def456",
			}

			Expect(ref.WorkflowID).To(Equal("oomkill-increase-memory"))
			Expect(ref.ContainerImage).To(ContainSubstring("quay.io"))
			Expect(ref.ContainerDigest).To(HavePrefix("sha256:"))
		})
	})

	// ========================================
	// ExecutionConfig Tests
	// ========================================
	Describe("ExecutionConfig", func() {
		It("should have default values when empty", func() {
			config := workflowexecutionv1alpha1.ExecutionConfig{}

			// Empty config should have nil timeout and empty service account
			Expect(config.Timeout).To(BeNil())
			Expect(config.ServiceAccountName).To(BeEmpty())
		})

		It("should accept custom timeout", func() {
			timeout := metav1.Duration{Duration: 30 * time.Minute}
			config := workflowexecutionv1alpha1.ExecutionConfig{
				Timeout:            &timeout,
				ServiceAccountName: "custom-sa",
			}

			Expect(config.Timeout.Duration).To(Equal(30 * time.Minute))
			Expect(config.ServiceAccountName).To(Equal("custom-sa"))
		})
	})

	// ========================================
	// PipelineRunStatusSummary Tests
	// ========================================
	Describe("PipelineRunStatusSummary", func() {
		It("should capture PipelineRun status", func() {
			summary := workflowexecutionv1alpha1.PipelineRunStatusSummary{
				Status:         "True",
				Reason:         "Succeeded",
				Message:        "All tasks completed successfully",
				CompletedTasks: 3,
				TotalTasks:     3,
			}

			Expect(summary.Status).To(Equal("True"))
			Expect(summary.Reason).To(Equal("Succeeded"))
			Expect(summary.CompletedTasks).To(Equal(3))
			Expect(summary.TotalTasks).To(Equal(3))
		})

		It("should capture running status", func() {
			summary := workflowexecutionv1alpha1.PipelineRunStatusSummary{
				Status:         "Unknown",
				Reason:         "Running",
				Message:        "Tasks are still executing",
				CompletedTasks: 1,
				TotalTasks:     3,
			}

			Expect(summary.Status).To(Equal("Unknown"))
			Expect(summary.Reason).To(Equal("Running"))
		})
	})
})

