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

// Package remediationorchestrator_test contains unit tests for the Remediation Orchestrator.
// BR-ORCH-026: Status aggregation from child CRDs
// BR-ORCH-010: State machine orchestration
package remediationorchestrator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// Suppress unused import warnings
var (
	_ = aianalysisv1.GroupVersion
	_ = workflowexecutionv1.GroupVersion
)

var _ = Describe("BR-ORCH-026: Status Aggregation from Child CRDs", func() {
	var (
		ctx            context.Context
		fakeClient     client.Client
		scheme         *runtime.Scheme
		statusAgg      *aggregator.StatusAggregator
		rr             *remediationv1.RemediationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()

		// Register schemes
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		// Create base RemediationRequest
		rr = testutil.NewRemediationRequest("test-rr", "default")

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		statusAgg = aggregator.NewStatusAggregator(fakeClient)
	})

	Describe("Aggregate (BR-ORCH-026)", func() {
		Context("when no child CRDs exist", func() {
			It("should return empty aggregated status", func() {
				result, err := statusAgg.Aggregate(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.SignalProcessingPhase).To(BeEmpty())
				Expect(result.AIAnalysisPhase).To(BeEmpty())
				Expect(result.WorkflowExecutionPhase).To(BeEmpty())
				Expect(result.OverallReady).To(BeFalse())
			})
		})

		Context("when SignalProcessing exists", func() {
			DescribeTable("should aggregate SignalProcessing status correctly",
				func(spPhase signalprocessingv1.SignalProcessingPhase, expectedReady bool) {
					sp := testutil.NewSignalProcessing("sp-test-rr", "default")
					sp.Status.Phase = spPhase
					Expect(fakeClient.Create(ctx, sp)).To(Succeed())

					// Set reference in RR status
					rr.Status.RemediationProcessingRef = &corev1.ObjectReference{Name: sp.Name, Namespace: sp.Namespace}
					Expect(fakeClient.Status().Update(ctx, rr)).To(Succeed())

					result, err := statusAgg.Aggregate(ctx, rr)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.SignalProcessingPhase).To(Equal(string(spPhase)))
					Expect(result.SignalProcessingReady).To(Equal(expectedReady))
				},
				Entry("Pending phase", signalprocessingv1.PhasePending, false),
				Entry("Enriching phase", signalprocessingv1.PhaseEnriching, false),
				Entry("Completed phase", signalprocessingv1.PhaseCompleted, true),
				Entry("Failed phase", signalprocessingv1.SignalProcessingPhase("failed"), false),
			)

			It("should capture enrichment results from completed SignalProcessing", func() {
				sp := testutil.NewCompletedSignalProcessing("sp-test-rr", "default")
				Expect(fakeClient.Create(ctx, sp)).To(Succeed())

				rr.Status.RemediationProcessingRef = &corev1.ObjectReference{Name: sp.Name, Namespace: sp.Namespace}
				Expect(fakeClient.Status().Update(ctx, rr)).To(Succeed())

				result, err := statusAgg.Aggregate(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.SignalProcessingReady).To(BeTrue())
				Expect(result.EnrichmentResults).NotTo(BeNil())
			})
		})

		Context("when AIAnalysis exists", func() {
			DescribeTable("should aggregate AIAnalysis status correctly",
				func(aiPhase string, approvalRequired bool, expectedApproved bool, expectedReady bool) {
					ai := testutil.NewAIAnalysis("ai-test-rr", "default")
					ai.Status.Phase = aiPhase
					ai.Status.ApprovalRequired = approvalRequired
					Expect(fakeClient.Create(ctx, ai)).To(Succeed())

					rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: ai.Name, Namespace: ai.Namespace}
					Expect(fakeClient.Status().Update(ctx, rr)).To(Succeed())

					result, err := statusAgg.Aggregate(ctx, rr)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.AIAnalysisPhase).To(Equal(aiPhase))
					Expect(result.RequiresApproval).To(Equal(approvalRequired))
					// Approved = ApprovalRequired AND Phase=Completed
					Expect(result.Approved).To(Equal(expectedApproved))
					Expect(result.AIAnalysisReady).To(Equal(expectedReady))
				},
				Entry("Pending phase", "Pending", false, false, false),
				Entry("Investigating phase", "Investigating", false, false, false),
				Entry("Completed without approval required", "Completed", false, false, true),
				Entry("Analyzing phase with approval required", "Analyzing", true, false, false),
				Entry("Completed with approval (was required, now approved)", "Completed", true, true, true),
				Entry("Failed phase", "Failed", false, false, false),
			)

			It("should capture selected workflow from completed AIAnalysis", func() {
				ai := testutil.NewCompletedAIAnalysis("ai-test-rr", "default")
				Expect(fakeClient.Create(ctx, ai)).To(Succeed())

				rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: ai.Name, Namespace: ai.Namespace}
				Expect(fakeClient.Status().Update(ctx, rr)).To(Succeed())

				result, err := statusAgg.Aggregate(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.AIAnalysisReady).To(BeTrue())
				Expect(result.SelectedWorkflow).NotTo(BeNil())
			})
		})

		Context("when WorkflowExecution exists", func() {
			DescribeTable("should aggregate WorkflowExecution status correctly",
				func(wePhase string, expectedReady bool) {
					we := testutil.NewWorkflowExecution("we-test-rr", "default")
					we.Status.Phase = wePhase
					Expect(fakeClient.Create(ctx, we)).To(Succeed())

					rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{Name: we.Name, Namespace: we.Namespace}
					Expect(fakeClient.Status().Update(ctx, rr)).To(Succeed())

					result, err := statusAgg.Aggregate(ctx, rr)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.WorkflowExecutionPhase).To(Equal(wePhase))
					Expect(result.WorkflowExecutionReady).To(Equal(expectedReady))
				},
				// WorkflowExecution uses: Pending, Running, Completed, Failed, Skipped
				Entry("Pending phase", workflowexecutionv1.PhasePending, false),
				Entry("Running phase", workflowexecutionv1.PhaseRunning, false),
				Entry("Completed phase", workflowexecutionv1.PhaseCompleted, true),
				Entry("Failed phase", workflowexecutionv1.PhaseFailed, false),
				Entry("Skipped phase", workflowexecutionv1.PhaseSkipped, true),
			)

			It("should capture skip info for Skipped workflow (BR-ORCH-033)", func() {
				we := testutil.NewSkippedWorkflowExecution("we-test-rr", "default", "ResourceBusy", "we-other-rr")
				Expect(fakeClient.Create(ctx, we)).To(Succeed())

				rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{Name: we.Name, Namespace: we.Namespace}
				Expect(fakeClient.Status().Update(ctx, rr)).To(Succeed())

				result, err := statusAgg.Aggregate(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.ExecutionSkipped).To(BeTrue())
				Expect(result.SkipReason).To(Equal("ResourceBusy"))
				Expect(result.DuplicateOf).To(Equal("we-other-rr"))
			})
		})

		Context("when all child CRDs exist and are completed", func() {
			It("should set OverallReady to true", func() {
				// Create all child CRDs in completed state
				sp := testutil.NewCompletedSignalProcessing("sp-test-rr", "default")
				Expect(fakeClient.Create(ctx, sp)).To(Succeed())

				ai := testutil.NewCompletedAIAnalysis("ai-test-rr", "default")
				Expect(fakeClient.Create(ctx, ai)).To(Succeed())

				we := testutil.NewCompletedWorkflowExecution("we-test-rr", "default")
				Expect(fakeClient.Create(ctx, we)).To(Succeed())

				// Set references in RR status
				rr.Status.RemediationProcessingRef = &corev1.ObjectReference{Name: sp.Name, Namespace: sp.Namespace}
				rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: ai.Name, Namespace: ai.Namespace}
				rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{Name: we.Name, Namespace: we.Namespace}
				Expect(fakeClient.Status().Update(ctx, rr)).To(Succeed())

				result, err := statusAgg.Aggregate(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.SignalProcessingReady).To(BeTrue())
				Expect(result.AIAnalysisReady).To(BeTrue())
				Expect(result.WorkflowExecutionReady).To(BeTrue())
				Expect(result.OverallReady).To(BeTrue())
			})
		})
	})

	Describe("CalculateProgress (BR-ORCH-026)", func() {
		DescribeTable("should calculate progress percentage correctly",
			func(spReady, aiReady, weReady bool, expectedProgress float64) {
				aggStatus := &aggregator.AggregatedStatus{
					SignalProcessingReady:  spReady,
					AIAnalysisReady:        aiReady,
					WorkflowExecutionReady: weReady,
				}

				progress := statusAgg.CalculateProgress(aggStatus)
				Expect(progress).To(BeNumerically("~", expectedProgress, 0.1))
			},
			Entry("No progress", false, false, false, 0.0),
			Entry("SP complete only", true, false, false, 33.3),
			Entry("SP + AI complete", true, true, false, 66.6),
			Entry("All complete", true, true, true, 100.0),
		)
	})

	Describe("DetermineOverallPhase (BR-ORCH-010)", func() {
		DescribeTable("should determine correct overall phase based on child statuses",
			func(spPhase string, spReady bool, aiPhase string, aiReady bool, wePhase string, requiresApproval, approved bool, expectedPhase string) {
				aggStatus := &aggregator.AggregatedStatus{
					SignalProcessingPhase:  spPhase,
					SignalProcessingReady:  spReady,
					AIAnalysisPhase:        aiPhase,
					AIAnalysisReady:        aiReady,
					WorkflowExecutionPhase: wePhase,
					RequiresApproval:       requiresApproval,
					Approved:               approved,
				}

				phase := statusAgg.DetermineOverallPhase(aggStatus)
				Expect(phase).To(Equal(expectedPhase))
			},
			// Initial states
			Entry("All empty - Pending", "", false, "", false, "", false, false, "Pending"),

			// Processing phase
			Entry("SP Pending", "pending", false, "", false, "", false, false, "Processing"),
			Entry("SP Enriching", "enriching", false, "", false, "", false, false, "Processing"),

			// Analyzing phase (SP Ready, AI has started)
			Entry("SP Complete, AI Pending", "completed", true, "Pending", false, "", false, false, "Analyzing"),
			Entry("SP Complete, AI Investigating", "completed", true, "Investigating", false, "", false, false, "Analyzing"),

			// AwaitingApproval phase
			Entry("AI requires approval", "completed", true, "Analyzing", false, "", true, false, "AwaitingApproval"),

			// Executing phase (AI Ready, WE has started)
			Entry("AI Complete, WE Pending", "completed", true, "Completed", true, "Pending", false, false, "Executing"),
			Entry("AI Complete (approved), WE Running", "completed", true, "Completed", true, "Running", true, true, "Executing"),

			// Completed phase (WorkflowExecution uses "Completed" not "Succeeded")
			Entry("WE Completed", "completed", true, "Completed", true, workflowexecutionv1.PhaseCompleted, false, false, "Completed"),
			Entry("WE Skipped", "completed", true, "Completed", true, workflowexecutionv1.PhaseSkipped, false, false, "Completed"),

			// Failed states
			Entry("SP Failed", "failed", false, "", false, "", false, false, "Failed"),
			Entry("AI Failed", "completed", true, "Failed", false, "", false, false, "Failed"),
			Entry("WE Failed", "completed", true, "Completed", true, workflowexecutionv1.PhaseFailed, false, false, "Failed"),
		)
	})

	Describe("BuildStatusConditions (BR-ORCH-026)", func() {
		It("should build correct conditions for each child CRD", func() {
			aggStatus := &aggregator.AggregatedStatus{
				SignalProcessingPhase:  "Completed",
				SignalProcessingReady:  true,
				AIAnalysisPhase:        "Completed",
				AIAnalysisReady:        true,
				WorkflowExecutionPhase: "Running",
				WorkflowExecutionReady: false,
			}

			conditions := statusAgg.BuildStatusConditions(aggStatus)

			// Should have conditions for each child CRD
			Expect(conditions).To(HaveLen(3))

			// Find SignalProcessing condition
			var spCondition, aiCondition, weCondition *aggregator.StatusCondition
			for i := range conditions {
				switch conditions[i].Type {
				case "SignalProcessingReady":
					spCondition = &conditions[i]
				case "AIAnalysisReady":
					aiCondition = &conditions[i]
				case "WorkflowExecutionReady":
					weCondition = &conditions[i]
				}
			}

			Expect(spCondition).NotTo(BeNil())
			Expect(spCondition.Status).To(Equal("True"))

			Expect(aiCondition).NotTo(BeNil())
			Expect(aiCondition.Status).To(Equal("True"))

			Expect(weCondition).NotTo(BeNil())
			Expect(weCondition.Status).To(Equal("False"))
			Expect(weCondition.Reason).To(Equal("Running"))
		})
	})
})

