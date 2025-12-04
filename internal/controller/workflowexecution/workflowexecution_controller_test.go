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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// =============================================================================
// Unit Tests: Implementation Correctness
// Per TESTING_GUIDELINES.md: Unit tests focus on function/method behavior,
// error handling, edge cases, and internal component logic.
// =============================================================================

// =============================================================================
// BuildPipelineRun - Function Behavior Tests
// =============================================================================

var _ = Describe("BuildPipelineRun", func() {
	var (
		testNS     string
		reconciler *WorkflowExecutionReconciler
	)

	BeforeEach(func() {
		testNS = "test-build-pr-" + time.Now().Format("150405")
		createTestNamespace(testNS)
		reconciler = NewWorkflowExecutionReconciler(
			k8sClient,
			testScheme,
			"kubernaut-workflows",
		)
	})

	Context("bundle resolver configuration", func() {
		It("should configure PipelineRef with bundles resolver", func() {
			wfe := newTestWorkflowExecution("wfe-bundle-test", testNS)

			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr).NotTo(BeNil())
			Expect(pr.Spec.PipelineRef).NotTo(BeNil())
			Expect(string(pr.Spec.PipelineRef.ResolverRef.Resolver)).To(Equal("bundles"))
		})

		It("should set bundle param with container image from spec", func() {
			wfe := newTestWorkflowExecution("wfe-image-test", testNS)
			wfe.Spec.WorkflowRef.ContainerImage = "ghcr.io/kubernaut/workflows/custom:v1.0.0"

			pr := reconciler.BuildPipelineRun(wfe)

			var bundleParam *tektonv1.Param
			for i := range pr.Spec.PipelineRef.ResolverRef.Params {
				if pr.Spec.PipelineRef.ResolverRef.Params[i].Name == "bundle" {
					bundleParam = &pr.Spec.PipelineRef.ResolverRef.Params[i]
					break
				}
			}
			Expect(bundleParam).NotTo(BeNil())
			Expect(bundleParam.Value.StringVal).To(Equal("ghcr.io/kubernaut/workflows/custom:v1.0.0"))
		})
	})

	Context("execution namespace configuration", func() {
		It("should create PipelineRun in dedicated execution namespace", func() {
			wfe := newTestWorkflowExecution("wfe-namespace-test", testNS)

			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr).NotTo(BeNil())
			Expect(pr.Namespace).To(Equal("kubernaut-workflows"))
			Expect(pr.Namespace).NotTo(Equal(wfe.Namespace))
		})
	})

	Context("deterministic naming", func() {
		It("should generate identical names for same target resource", func() {
			wfe1 := newTestWorkflowExecutionWithTarget("wfe1", testNS, "production/deployment/app")
			wfe2 := newTestWorkflowExecutionWithTarget("wfe2", testNS, "production/deployment/app")

			pr1 := reconciler.BuildPipelineRun(wfe1)
			pr2 := reconciler.BuildPipelineRun(wfe2)

			Expect(pr1).NotTo(BeNil())
			Expect(pr2).NotTo(BeNil())
			Expect(pr1.Name).To(Equal(pr2.Name))
			Expect(pr1.Name).To(HavePrefix("wfe-"))
		})

		It("should generate different names for different target resources", func() {
			wfe1 := newTestWorkflowExecutionWithTarget("wfe1", testNS, "production/deployment/app-a")
			wfe2 := newTestWorkflowExecutionWithTarget("wfe2", testNS, "production/deployment/app-b")

			pr1 := reconciler.BuildPipelineRun(wfe1)
			pr2 := reconciler.BuildPipelineRun(wfe2)

			Expect(pr1.Name).NotTo(Equal(pr2.Name))
		})
	})

	Context("cross-namespace tracking labels", func() {
		It("should set labels for tracking back to source WFE", func() {
			wfe := newTestWorkflowExecution("wfe-labels-test", testNS)

			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr).NotTo(BeNil())
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution", wfe.Name))
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/source-namespace", wfe.Namespace))
		})

		It("should NOT set ownerReferences (cross-namespace limitation)", func() {
			wfe := newTestWorkflowExecution("wfe-owner-test", testNS)

			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr.OwnerReferences).To(BeEmpty())
		})
	})
})

// =============================================================================
// ConvertParameters - Function Behavior Tests
// =============================================================================

var _ = Describe("ConvertParameters (via BuildPipelineRun)", func() {
	var reconciler *WorkflowExecutionReconciler

	BeforeEach(func() {
		reconciler = NewWorkflowExecutionReconciler(k8sClient, testScheme, "kubernaut-workflows")
	})

	Context("parameter conversion", func() {
		It("should convert map[string]string to Tekton params", func() {
			wfe := newTestWorkflowExecution("wfe-params-test", "default")
			wfe.Spec.Parameters = map[string]string{
				"NAMESPACE":           "production",
				"MEMORY_INCREMENT_MB": "512",
				"TARGET_POD":          "payment-api-abc123",
			}

			pr := reconciler.BuildPipelineRun(wfe)

			paramMap := make(map[string]string)
			for _, p := range pr.Spec.Params {
				paramMap[p.Name] = p.Value.StringVal
			}
			Expect(paramMap).To(HaveKeyWithValue("NAMESPACE", "production"))
			Expect(paramMap).To(HaveKeyWithValue("MEMORY_INCREMENT_MB", "512"))
			Expect(paramMap).To(HaveKeyWithValue("TARGET_POD", "payment-api-abc123"))
		})

		It("should preserve UPPER_SNAKE_CASE parameter names", func() {
			wfe := newTestWorkflowExecution("wfe-case-test", "default")
			wfe.Spec.Parameters = map[string]string{
				"MY_SPECIAL_PARAM": "value",
			}

			pr := reconciler.BuildPipelineRun(wfe)

			var found bool
			for _, p := range pr.Spec.Params {
				if p.Name == "MY_SPECIAL_PARAM" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Parameter name should be preserved as UPPER_SNAKE_CASE")
		})

		It("should handle empty parameters gracefully", func() {
			wfe := newTestWorkflowExecution("wfe-empty-params", "default")
			wfe.Spec.Parameters = nil

			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr.Spec.Params).To(BeEmpty())
		})

		// Edge case: Very long parameter values
		It("should preserve very long parameter values (10KB)", func() {
			longValue := strings.Repeat("x", 10*1024)
			wfe := newTestWorkflowExecution("wfe-long-param", "default")
			wfe.Spec.Parameters = map[string]string{
				"LONG_VALUE": longValue,
			}

			pr := reconciler.BuildPipelineRun(wfe)

			var found bool
			for _, p := range pr.Spec.Params {
				if p.Name == "LONG_VALUE" {
					Expect(p.Value.StringVal).To(Equal(longValue))
					found = true
				}
			}
			Expect(found).To(BeTrue())
		})

		// Edge case: Unicode in parameters
		It("should correctly preserve unicode in parameter values", func() {
			wfe := newTestWorkflowExecution("wfe-unicode", "default")
			wfe.Spec.Parameters = map[string]string{
				"UNICODE_VALUE": "日本語テスト 🚀 émojis",
			}

			pr := reconciler.BuildPipelineRun(wfe)

			var found bool
			for _, p := range pr.Spec.Params {
				if p.Name == "UNICODE_VALUE" {
					Expect(p.Value.StringVal).To(Equal("日本語テスト 🚀 émojis"))
					found = true
				}
			}
			Expect(found).To(BeTrue())
		})
	})
})

// =============================================================================
// ServiceAccount Configuration - Function Behavior Tests
// =============================================================================

var _ = Describe("ServiceAccount Configuration (via BuildPipelineRun)", func() {
	var reconciler *WorkflowExecutionReconciler

	BeforeEach(func() {
		reconciler = NewWorkflowExecutionReconciler(k8sClient, testScheme, "kubernaut-workflows")
	})

	Context("ServiceAccountName configuration", func() {
		It("should use specified ServiceAccount when provided", func() {
			wfe := newTestWorkflowExecution("wfe-sa-test", "default")
			wfe.Spec.ExecutionConfig.ServiceAccountName = "custom-workflow-runner"

			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("custom-workflow-runner"))
		})

		It("should use default ServiceAccount when not specified", func() {
			wfe := newTestWorkflowExecution("wfe-default-sa-test", "default")
			wfe.Spec.ExecutionConfig.ServiceAccountName = ""

			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("kubernaut-workflow-runner"))
		})
	})
})

// =============================================================================
// MapPipelineRunStatus - Function Behavior Tests
// =============================================================================

var _ = Describe("MapPipelineRunStatus", func() {
	var reconciler *WorkflowExecutionReconciler

	BeforeEach(func() {
		reconciler = NewWorkflowExecutionReconciler(k8sClient, testScheme, "kubernaut-workflows")
	})

	Context("Tekton condition mapping", func() {
		It("should map Succeeded=Unknown to Running phase", func() {
			pr := &tektonv1.PipelineRun{
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionUnknown,
							Reason: "Running",
						}},
					},
				},
			}

			phase, outcome := reconciler.MapPipelineRunStatus(pr)

			Expect(phase).To(Equal(workflowexecutionv1.PhaseRunning))
			Expect(outcome).To(BeEmpty())
		})

		It("should map Succeeded=True to Completed phase with Success outcome", func() {
			pr := &tektonv1.PipelineRun{
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
							Reason: "Succeeded",
						}},
					},
				},
			}

			phase, outcome := reconciler.MapPipelineRunStatus(pr)

			Expect(phase).To(Equal(workflowexecutionv1.PhaseCompleted))
			Expect(outcome).To(Equal(workflowexecutionv1.OutcomeSuccess))
		})

		It("should map Succeeded=False to Failed phase with Failure outcome", func() {
			pr := &tektonv1.PipelineRun{
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionFalse,
							Reason:  "TaskRunFailed",
							Message: "Task increase-memory failed",
						}},
					},
				},
			}

			phase, outcome := reconciler.MapPipelineRunStatus(pr)

			Expect(phase).To(Equal(workflowexecutionv1.PhaseFailed))
			Expect(outcome).To(Equal(workflowexecutionv1.OutcomeFailure))
		})

		// Edge case: No conditions at all
		It("should return Pending phase when no conditions present", func() {
			pr := &tektonv1.PipelineRun{
				Status: tektonv1.PipelineRunStatus{},
			}

			phase, _ := reconciler.MapPipelineRunStatus(pr)

			Expect(phase).To(Equal(workflowexecutionv1.PhasePending))
		})

		// Edge case: Cancelled PipelineRun
		It("should map cancelled PipelineRun to Failed phase", func() {
			pr := &tektonv1.PipelineRun{
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionFalse,
							Reason: "Cancelled",
						}},
					},
				},
			}

			phase, outcome := reconciler.MapPipelineRunStatus(pr)

			Expect(phase).To(Equal(workflowexecutionv1.PhaseFailed))
			Expect(outcome).To(Equal(workflowexecutionv1.OutcomeFailure))
		})
	})
})

// =============================================================================
// UpdateWFEStatus - Function Behavior Tests
// =============================================================================

var _ = Describe("UpdateWFEStatus", func() {
	var reconciler *WorkflowExecutionReconciler

	BeforeEach(func() {
		reconciler = NewWorkflowExecutionReconciler(k8sClient, testScheme, "kubernaut-workflows")
	})

	Context("completion time tracking", func() {
		It("should set CompletionTime when PipelineRun completes", func() {
			wfe := newTestWorkflowExecution("wfe-completion-test", "default")
			wfe.Status.Phase = workflowexecutionv1.PhaseRunning
			wfe.Status.StartTime = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}

			pr := &tektonv1.PipelineRun{
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:   apis.ConditionSucceeded,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			}

			reconciler.UpdateWFEStatus(wfe, pr)

			Expect(wfe.Status.CompletionTime).NotTo(BeNil())
			Expect(wfe.Status.Duration).NotTo(BeEmpty())
		})
	})
})

// =============================================================================
// CheckResourceLock - Function Behavior Tests
// =============================================================================

var _ = Describe("CheckResourceLock", func() {
	var (
		testNS     string
		reconciler *WorkflowExecutionReconciler
		fakeClient client.Client
	)

	BeforeEach(func() {
		testNS = "test-lock-" + time.Now().Format("150405")
		createTestNamespace(testNS)
	})

	Context("parallel execution detection", func() {
		It("should return blocked=true when Running WFE exists for target", func() {
			runningWFE := newTestWorkflowExecutionWithTarget("wfe-running", testNS, "production/deployment/payment-api")
			runningWFE.Status.Phase = workflowexecutionv1.PhaseRunning

			fakeClient = newFakeClient(runningWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")

			blocked, reason, conflicting := reconciler.CheckResourceLock(ctx, "production/deployment/payment-api", "any-workflow")

			Expect(blocked).To(BeTrue())
			Expect(reason).To(Equal(workflowexecutionv1.SkipReasonResourceBusy))
			Expect(conflicting).NotTo(BeNil())
			Expect(conflicting.Name).To(Equal("wfe-running"))
		})

		It("should return blocked=true when Pending WFE exists for target", func() {
			pendingWFE := newTestWorkflowExecutionWithTarget("wfe-pending", testNS, "production/deployment/payment-api")
			pendingWFE.Status.Phase = workflowexecutionv1.PhasePending

			fakeClient = newFakeClient(pendingWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")

			blocked, reason, _ := reconciler.CheckResourceLock(ctx, "production/deployment/payment-api", "any-workflow")

			Expect(blocked).To(BeTrue())
			Expect(reason).To(Equal(workflowexecutionv1.SkipReasonResourceBusy))
		})

		It("should return blocked=false when no Running/Pending WFE exists", func() {
			fakeClient = newFakeClient()
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")

			blocked, _, _ := reconciler.CheckResourceLock(ctx, "staging/deployment/other-app", "any-workflow")

			Expect(blocked).To(BeFalse())
		})

		It("should return blocked=false for different target resources", func() {
			runningWFE := newTestWorkflowExecutionWithTarget("wfe-running-a", testNS, "production/deployment/app-a")
			runningWFE.Status.Phase = workflowexecutionv1.PhaseRunning

			fakeClient = newFakeClient(runningWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")

			blocked, _, _ := reconciler.CheckResourceLock(ctx, "production/deployment/app-b", "any-workflow")

			Expect(blocked).To(BeFalse())
		})

		// V1.0 strict blocking: Different workflow on same target
		It("should block different workflow on same running target (V1.0 strict)", func() {
			runningWFE := newTestWorkflowExecutionWithWorkflow("wfe-workflow-a", testNS, "restart-pod")
			runningWFE.Spec.TargetResource = "production/deployment/payment-api"
			runningWFE.Status.Phase = workflowexecutionv1.PhaseRunning

			fakeClient = newFakeClient(runningWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")

			blocked, reason, _ := reconciler.CheckResourceLock(ctx, "production/deployment/payment-api", "increase-memory")

			Expect(blocked).To(BeTrue())
			Expect(reason).To(Equal(workflowexecutionv1.SkipReasonResourceBusy))
		})

		// Skipped WFEs don't count as locks
		It("should NOT block for Skipped WFE on same target", func() {
			skippedWFE := newTestWorkflowExecutionWithTarget("wfe-skipped", testNS, "production/deployment/app")
			skippedWFE.Status.Phase = workflowexecutionv1.PhaseSkipped

			fakeClient = newFakeClient(skippedWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")

			blocked, _, _ := reconciler.CheckResourceLock(ctx, "production/deployment/app", "any-workflow")

			Expect(blocked).To(BeFalse())
		})
	})
})

// =============================================================================
// CheckCooldown - Function Behavior Tests
// =============================================================================

var _ = Describe("CheckCooldown", func() {
	var (
		testNS     string
		reconciler *WorkflowExecutionReconciler
		fakeClient client.Client
	)

	BeforeEach(func() {
		testNS = "test-cooldown-" + time.Now().Format("150405")
	})

	Context("cooldown period enforcement", func() {
		It("should return blocked=true when same workflow+target completed within cooldown", func() {
			completedWFE := newTestWorkflowExecutionWithWorkflow("wfe-completed", testNS, "increase-memory")
			completedWFE.Spec.TargetResource = "production/deployment/payment-api"
			completedWFE.Status.Phase = workflowexecutionv1.PhaseCompleted
			completedWFE.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}

			fakeClient = newFakeClient(completedWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")
			reconciler.CooldownPeriod = 5 * time.Minute

			blocked, reason, recent := reconciler.CheckCooldown(ctx, "production/deployment/payment-api", "increase-memory")

			Expect(blocked).To(BeTrue())
			Expect(reason).To(Equal(workflowexecutionv1.SkipReasonRecentlyRemediated))
			Expect(recent).NotTo(BeNil())
			Expect(recent.WorkflowID).To(Equal("increase-memory"))
		})

		It("should return blocked=false after cooldown expires", func() {
			completedWFE := newTestWorkflowExecutionWithWorkflow("wfe-old", testNS, "increase-memory")
			completedWFE.Spec.TargetResource = "production/deployment/payment-api"
			completedWFE.Status.Phase = workflowexecutionv1.PhaseCompleted
			completedWFE.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-10 * time.Minute)}

			fakeClient = newFakeClient(completedWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")
			reconciler.CooldownPeriod = 5 * time.Minute

			blocked, _, _ := reconciler.CheckCooldown(ctx, "production/deployment/payment-api", "increase-memory")

			Expect(blocked).To(BeFalse())
		})

		It("should return blocked=false for different workflow on same target", func() {
			completedWFE := newTestWorkflowExecutionWithWorkflow("wfe-workflow-a", testNS, "increase-memory")
			completedWFE.Spec.TargetResource = "production/deployment/payment-api"
			completedWFE.Status.Phase = workflowexecutionv1.PhaseCompleted
			completedWFE.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}

			fakeClient = newFakeClient(completedWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")
			reconciler.CooldownPeriod = 5 * time.Minute

			blocked, _, _ := reconciler.CheckCooldown(ctx, "production/deployment/payment-api", "restart-pods")

			Expect(blocked).To(BeFalse())
		})

		// Edge case: Exactly at cooldown boundary
		It("should return blocked=false exactly at cooldown boundary", func() {
			completedWFE := newTestWorkflowExecutionWithWorkflow("wfe-boundary", testNS, "increase-memory")
			completedWFE.Spec.TargetResource = "production/deployment/payment-api"
			completedWFE.Status.Phase = workflowexecutionv1.PhaseCompleted
			completedWFE.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}

			fakeClient = newFakeClient(completedWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")
			reconciler.CooldownPeriod = 5 * time.Minute

			blocked, _, _ := reconciler.CheckCooldown(ctx, "production/deployment/payment-api", "increase-memory")

			Expect(blocked).To(BeFalse())
		})

		// Failed WFEs also trigger cooldown
		It("should apply cooldown for Failed WFE", func() {
			failedWFE := newTestWorkflowExecutionWithWorkflow("wfe-failed", testNS, "increase-memory")
			failedWFE.Spec.TargetResource = "production/deployment/app"
			failedWFE.Status.Phase = workflowexecutionv1.PhaseFailed
			failedWFE.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}

			fakeClient = newFakeClient(failedWFE)
			reconciler = NewWorkflowExecutionReconciler(fakeClient, testScheme, "kubernaut-workflows")
			reconciler.CooldownPeriod = 5 * time.Minute

			blocked, reason, _ := reconciler.CheckCooldown(ctx, "production/deployment/app", "increase-memory")

			Expect(blocked).To(BeTrue())
			Expect(reason).To(Equal(workflowexecutionv1.SkipReasonRecentlyRemediated))
		})
	})
})

// =============================================================================
// MarkSkipped - Function Behavior Tests
// =============================================================================

var _ = Describe("MarkSkipped", func() {
	var reconciler *WorkflowExecutionReconciler

	BeforeEach(func() {
		reconciler = NewWorkflowExecutionReconciler(k8sClient, testScheme, "kubernaut-workflows")
	})

	Context("skip details population", func() {
		It("should populate SkipDetails for ResourceBusy reason", func() {
			wfe := newTestWorkflowExecution("wfe-to-skip", "default")
			conflicting := &workflowexecutionv1.ConflictingWorkflowRef{
				Name:           "wfe-blocking",
				WorkflowID:     "restart-pod",
				StartedAt:      metav1.Now(),
				TargetResource: "production/deployment/app",
			}

			reconciler.MarkSkipped(wfe, workflowexecutionv1.SkipReasonResourceBusy, conflicting, nil)

			Expect(wfe.Status.Phase).To(Equal(workflowexecutionv1.PhaseSkipped))
			Expect(wfe.Status.SkipDetails).NotTo(BeNil())
			Expect(wfe.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1.SkipReasonResourceBusy))
			Expect(wfe.Status.SkipDetails.ConflictingWorkflow).NotTo(BeNil())
			Expect(wfe.Status.SkipDetails.ConflictingWorkflow.Name).To(Equal("wfe-blocking"))
		})

		It("should populate SkipDetails for RecentlyRemediated reason", func() {
			wfe := newTestWorkflowExecution("wfe-cooldown-skip", "default")
			recent := &workflowexecutionv1.RecentRemediationRef{
				Name:              "wfe-recent",
				WorkflowID:        "increase-memory",
				CompletedAt:       metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
				Outcome:           workflowexecutionv1.OutcomeSuccess,
				TargetResource:    "production/deployment/app",
				CooldownRemaining: "3m0s",
			}

			reconciler.MarkSkipped(wfe, workflowexecutionv1.SkipReasonRecentlyRemediated, nil, recent)

			Expect(wfe.Status.Phase).To(Equal(workflowexecutionv1.PhaseSkipped))
			Expect(wfe.Status.SkipDetails).NotTo(BeNil())
			Expect(wfe.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1.SkipReasonRecentlyRemediated))
			Expect(wfe.Status.SkipDetails.RecentRemediation).NotTo(BeNil())
			Expect(wfe.Status.SkipDetails.RecentRemediation.CooldownRemaining).To(Equal("3m0s"))
		})
	})
})

// =============================================================================
// HandleMissingPipelineRun - Function Behavior Tests
// =============================================================================

var _ = Describe("HandleMissingPipelineRun", func() {
	var reconciler *WorkflowExecutionReconciler

	BeforeEach(func() {
		reconciler = NewWorkflowExecutionReconciler(k8sClient, testScheme, "kubernaut-workflows")
	})

	Context("externally deleted PipelineRun handling", func() {
		It("should mark WFE as Failed with clear message", func() {
			wfe := newTestWorkflowExecution("wfe-external-delete", "default")
			wfe.Status.Phase = workflowexecutionv1.PhaseRunning
			wfe.Status.PipelineRunRef = &corev1.LocalObjectReference{Name: "pr-deleted"}

			reconciler.HandleMissingPipelineRun(wfe)

			Expect(wfe.Status.Phase).To(Equal(workflowexecutionv1.PhaseFailed))
			Expect(wfe.Status.FailureDetails).NotTo(BeNil())
			Expect(wfe.Status.FailureDetails.Message).To(ContainSubstring("deleted"))
			Expect(wfe.Status.FailureDetails.WasExecutionFailure).To(BeFalse())
		})
	})
})

// =============================================================================
// GetPipelineRunName - Function Behavior Tests
// =============================================================================

var _ = Describe("GetPipelineRunName", func() {
	var reconciler *WorkflowExecutionReconciler

	BeforeEach(func() {
		reconciler = NewWorkflowExecutionReconciler(k8sClient, testScheme, "kubernaut-workflows")
	})

	Context("deterministic name generation", func() {
		It("should produce deterministic hash for namespaced resource", func() {
			target := "production/deployment/payment-api"

			name1 := reconciler.GetPipelineRunName(target)
			name2 := reconciler.GetPipelineRunName(target)

			Expect(name1).To(Equal(name2))
			Expect(name1).To(HavePrefix("wfe-"))
			Expect(len(name1)).To(BeNumerically("<=", 63))
		})

		It("should produce deterministic hash for cluster-scoped resource", func() {
			target := "node/worker-node-1"

			name := reconciler.GetPipelineRunName(target)

			Expect(name).To(HavePrefix("wfe-"))
			Expect(name).To(MatchRegexp(`^wfe-[a-z0-9]+$`))
		})

		It("should produce different names for different targets", func() {
			target1 := "production/deployment/app-a"
			target2 := "production/deployment/app-b"

			name1 := reconciler.GetPipelineRunName(target1)
			name2 := reconciler.GetPipelineRunName(target2)

			Expect(name1).NotTo(Equal(name2))
		})

		// Edge case: Very long targetResource
		It("should handle very long targetResource (500 chars)", func() {
			longTarget := strings.Repeat("a", 500)

			name := reconciler.GetPipelineRunName(longTarget)

			Expect(name).To(HavePrefix("wfe-"))
			Expect(len(name)).To(Equal(20))
			Expect(len(name)).To(BeNumerically("<=", 63))
		})

		// Edge case: Special characters in targetResource
		It("should handle special characters in targetResource", func() {
			specialTarget := "ns/deploy/app-v2.1_special@chars"

			name := reconciler.GetPipelineRunName(specialTarget)

			Expect(name).To(HavePrefix("wfe-"))
			Expect(name).To(MatchRegexp(`^wfe-[a-z0-9]+$`))
		})
	})
})

// =============================================================================
// ExtractFailureDetails - Function Behavior Tests
// =============================================================================

var _ = Describe("ExtractFailureDetails", func() {
	var reconciler *WorkflowExecutionReconciler

	BeforeEach(func() {
		reconciler = NewWorkflowExecutionReconciler(k8sClient, testScheme, "kubernaut-workflows")
	})

	Context("failure detail extraction", func() {
		It("should extract reason and message from failed PipelineRun", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-failed",
					Namespace: "kubernaut-workflows",
				},
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionFalse,
							Reason:  "TaskRunFailed",
							Message: "Task increase-memory failed: OOMKilled",
						}},
					},
				},
			}

			details := reconciler.ExtractFailureDetails(pr)

			Expect(details).NotTo(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1.FailureReasonOOMKilled))
			Expect(details.Message).To(ContainSubstring("OOMKilled"))
			Expect(details.WasExecutionFailure).To(BeTrue())
		})

		It("should generate natural language summary", func() {
			pr := &tektonv1.PipelineRun{
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionFalse,
							Reason:  "TaskRunFailed",
							Message: "Task step failed: OOMKilled",
						}},
					},
				},
			}

			details := reconciler.ExtractFailureDetails(pr)

			Expect(details.NaturalLanguageSummary).NotTo(BeEmpty())
			Expect(details.NaturalLanguageSummary).To(ContainSubstring("memory"))
		})
	})

	// Table-driven test for failure reason mapping
	DescribeTable("should map Tekton messages to Kubernetes-style reason codes",
		func(message string, expectedReason string) {
			pr := &tektonv1.PipelineRun{
				Status: tektonv1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionFalse,
							Message: message,
						}},
					},
				},
			}

			details := reconciler.ExtractFailureDetails(pr)

			Expect(details.Reason).To(Equal(expectedReason))
		},
		Entry("OOMKilled lowercase", "container oomkilled", workflowexecutionv1.FailureReasonOOMKilled),
		Entry("OOMKilled uppercase", "OOMKILLED", workflowexecutionv1.FailureReasonOOMKilled),
		Entry("Deadline exceeded", "DeadlineExceeded: timeout", workflowexecutionv1.FailureReasonDeadlineExceeded),
		Entry("Timeout keyword", "operation timeout after 30s", workflowexecutionv1.FailureReasonDeadlineExceeded),
		Entry("Forbidden", "Forbidden: RBAC denied", workflowexecutionv1.FailureReasonForbidden),
		Entry("Permission denied", "permission denied for resource", workflowexecutionv1.FailureReasonForbidden),
		Entry("ImagePullBackOff", "ImagePullBackOff: failed to pull", workflowexecutionv1.FailureReasonImagePullBackOff),
		Entry("ErrImagePull", "ErrImagePull for image", workflowexecutionv1.FailureReasonImagePullBackOff),
		Entry("Resource exhausted", "resourceexhausted: quota exceeded", workflowexecutionv1.FailureReasonResourceExhausted),
		Entry("Quota limit", "quota limit reached", workflowexecutionv1.FailureReasonResourceExhausted),
		Entry("Configuration error", "configuration error in params", workflowexecutionv1.FailureReasonConfigurationError),
		Entry("Invalid params", "invalid parameter value", workflowexecutionv1.FailureReasonConfigurationError),
		Entry("Unknown reason", "some unknown error occurred", workflowexecutionv1.FailureReasonUnknown),
	)
})
