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
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
)

// WorkflowExecution Controller Unit Tests
//
// Per TESTING_GUIDELINES.md:
// - Unit tests focus on specific function/method behavior
// - Unit tests validate implementation correctness, error handling, edge cases
// - Reconciliation flow tests belong in Integration tests
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation
// - Integration tests (>50%): Infrastructure interaction
// - E2E tests (10-15%): Complete workflow validation

var _ = Describe("WorkflowExecution Controller", func() {
	var (
		scheme   *runtime.Scheme
		recorder *record.FakeRecorder
		ctx      context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup scheme with WorkflowExecution CRD and Tekton types
		scheme = runtime.NewScheme()
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		// Create fake recorder for events
		recorder = record.NewFakeRecorder(10)
	})

	// ========================================
	// Day 2: Controller Instantiation
	// ========================================

	Describe("Controller Instantiation", func() {
		It("should create reconciler with required configuration", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			reconciler := &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
				CooldownPeriod:     5 * time.Minute,
			}

			Expect(reconciler.Client).ToNot(BeNil())
			Expect(reconciler.Scheme).ToNot(BeNil())
			Expect(reconciler.ExecutionNamespace).To(Equal("kubernaut-workflows"))
			Expect(reconciler.CooldownPeriod).To(Equal(5 * time.Minute))
		})

		It("should use default values from constants", func() {
			Expect(workflowexecution.FinalizerName).To(Equal("workflowexecution.kubernaut.ai/finalizer"))
			Expect(workflowexecution.DefaultCooldownPeriod).To(Equal(5 * time.Minute))
			Expect(workflowexecution.DefaultServiceAccountName).To(Equal("kubernaut-workflow-runner"))
		})
	})

	// ========================================
	// Day 3: pipelineRunName() Tests (DD-WE-003)
	// TDD RED Phase: Tests written, implementation pending
	// ========================================

	Describe("pipelineRunName", func() {
		// Tests deterministic naming per DD-WE-003
		// Name = "wfe-" + SHA256(targetResource)[:16]

		It("should generate deterministic name from targetResource", func() {
			// Same input should always produce same output
			targetResource := "default/deployment/my-app"
			name1 := workflowexecution.PipelineRunName(targetResource)
			name2 := workflowexecution.PipelineRunName(targetResource)
			Expect(name1).To(Equal(name2))
		})

		It("should prefix name with 'wfe-'", func() {
			targetResource := "default/deployment/my-app"
			name := workflowexecution.PipelineRunName(targetResource)
			Expect(name).To(HavePrefix("wfe-"))
		})

		It("should generate valid Kubernetes name (max 63 chars)", func() {
			// Very long targetResource should still produce valid name
			targetResource := "very-long-namespace/deployment/very-long-deployment-name-that-exceeds-normal-limits"
			name := workflowexecution.PipelineRunName(targetResource)
			Expect(len(name)).To(BeNumerically("<=", 63))
		})

		It("should generate 20-character name (wfe- + 16 hex chars)", func() {
			targetResource := "default/deployment/my-app"
			name := workflowexecution.PipelineRunName(targetResource)
			// "wfe-" (4 chars) + 16 hex chars = 20 chars
			Expect(len(name)).To(Equal(20))
		})

		It("should generate different names for different targetResources", func() {
			name1 := workflowexecution.PipelineRunName("ns1/deployment/app1")
			name2 := workflowexecution.PipelineRunName("ns1/deployment/app2")
			name3 := workflowexecution.PipelineRunName("ns2/deployment/app1")
			Expect(name1).ToNot(Equal(name2))
			Expect(name1).ToNot(Equal(name3))
			Expect(name2).ToNot(Equal(name3))
		})

		It("should use only lowercase hex characters", func() {
			targetResource := "default/deployment/my-app"
			name := workflowexecution.PipelineRunName(targetResource)
			// Remove "wfe-" prefix and check hex chars
			hexPart := name[4:]
			for _, c := range hexPart {
				Expect(c).To(SatisfyAny(
					BeNumerically(">=", '0'), // 0-9
					BeNumerically("<=", '9'),
					BeNumerically(">=", 'a'), // a-f
					BeNumerically("<=", 'f'),
				))
			}
		})
	})

	// ========================================
	// Day 3: checkResourceLock() Tests (DD-WE-001, DD-WE-003)
	// TDD RED Phase: Tests written, implementation pending
	// ========================================

	Describe("CheckResourceLock", func() {
		var (
			fakeClient     *fake.ClientBuilder
			reconciler     *workflowexecution.WorkflowExecutionReconciler
			targetResource string
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			targetResource = "default/deployment/my-app"
		})

		createReconciler := func(objects ...runtime.Object) *workflowexecution.WorkflowExecutionReconciler {
			client := fakeClient.WithRuntimeObjects(objects...).Build()
			return &workflowexecution.WorkflowExecutionReconciler{
				Client:             client,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
				CooldownPeriod:     5 * time.Minute,
			}
		}

		It("should return not-blocked when no Running WFE exists for targetResource", func() {
			reconciler = createReconciler()
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, details, err := reconciler.CheckResourceLock(ctx, wfe)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
			Expect(details).To(BeNil())
		})

		It("should return blocked when Running WFE exists for same targetResource", func() {
			// Create existing Running WFE for same targetResource
			existingWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-wfe",
					Namespace: "default",
					UID:       "existing-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:     workflowexecutionv1alpha1.PhaseRunning,
					StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			}
			reconciler = createReconciler(existingWFE)

			newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
					UID:       "new-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, details, err := reconciler.CheckResourceLock(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeTrue())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
		})

		It("should not block when existing WFE is in terminal phase", func() {
			// Completed WFE should not block
			completedWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "completed-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1alpha1.PhaseCompleted,
				},
			}
			reconciler = createReconciler(completedWFE)

			newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, _, err := reconciler.CheckResourceLock(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
		})

		It("should not block WFE from checking itself", func() {
			// WFE should not block itself
			selfWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "self-wfe",
					Namespace: "default",
					UID:       "self-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1alpha1.PhaseRunning,
				},
			}
			reconciler = createReconciler(selfWFE)

			blocked, _, err := reconciler.CheckResourceLock(ctx, selfWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
		})

		It("should not block when targeting different resource", func() {
			// WFE for different resource should not block
			otherWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "different/deployment/other-app",
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1alpha1.PhaseRunning,
				},
			}
			reconciler = createReconciler(otherWFE)

			newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, _, err := reconciler.CheckResourceLock(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
		})
	})

	// ========================================
	// Day 3: checkCooldown() Tests (DD-WE-001)
	// TDD RED Phase: Tests written, implementation pending
	// ========================================

	Describe("CheckCooldown", func() {
		var (
			fakeClient     *fake.ClientBuilder
			targetResource string
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			targetResource = "default/deployment/my-app"
		})

		createReconciler := func(cooldown time.Duration, objects ...runtime.Object) *workflowexecution.WorkflowExecutionReconciler {
			client := fakeClient.WithRuntimeObjects(objects...).Build()
			return &workflowexecution.WorkflowExecutionReconciler{
				Client:             client,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
				CooldownPeriod:     cooldown,
			}
		}

		It("should not block when no recent completed WFE exists", func() {
			reconciler := createReconciler(5 * time.Minute)
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, details, err := reconciler.CheckCooldown(ctx, wfe)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
			Expect(details).To(BeNil())
		})

		It("should block when recent completed WFE exists within cooldown", func() {
			// Completed 2 minutes ago, cooldown is 5 minutes
			completionTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			recentWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "recent-wfe",
					Namespace: "default",
					UID:       "recent-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1alpha1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			reconciler := createReconciler(5*time.Minute, recentWFE)

			newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
					UID:       "new-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, details, err := reconciler.CheckCooldown(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeTrue())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated))
		})

		It("should not block when completed WFE is outside cooldown period", func() {
			// Completed 10 minutes ago, cooldown is 5 minutes
			completionTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
			oldWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "old-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1alpha1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			reconciler := createReconciler(5*time.Minute, oldWFE)

			newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, _, err := reconciler.CheckCooldown(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
		})

		It("should not block when cooldown is zero (disabled)", func() {
			// Recent completed WFE but cooldown disabled
			completionTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			recentWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "recent-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1alpha1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			reconciler := createReconciler(0, recentWFE) // Cooldown disabled

			newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, _, err := reconciler.CheckCooldown(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
		})

		It("should also block when recent Failed WFE exists within cooldown", func() {
			// Failed 2 minutes ago, cooldown is 5 minutes
			completionTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			failedWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failed-wfe",
					Namespace: "default",
					UID:       "failed-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1alpha1.PhaseFailed,
					CompletionTime: &completionTime,
				},
			}
			reconciler := createReconciler(5*time.Minute, failedWFE)

			newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
					UID:       "new-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
			}

			blocked, details, err := reconciler.CheckCooldown(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeTrue())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated))
		})
	})

	// ========================================
	// Day 3: HandleAlreadyExists() Tests (DD-WE-003)
	// Race condition handling for PipelineRun creation
	// ========================================

	Describe("HandleAlreadyExists", func() {
		var (
			targetResource string
		)

		BeforeEach(func() {
			targetResource = "default/deployment/my-app"
		})

		It("should return nil when error is not AlreadyExists", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &workflowexecution.WorkflowExecutionReconciler{
				Client:             client,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
			}
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "test-wfe", Namespace: "default"},
				Spec:       workflowexecutionv1alpha1.WorkflowExecutionSpec{TargetResource: targetResource},
			}

			// Some other error (not AlreadyExists)
			otherErr := fmt.Errorf("some other error")
			details, err := reconciler.HandleAlreadyExists(ctx, wfe, otherErr)
			Expect(err).To(MatchError("some other error"))
			Expect(details).To(BeNil())
		})

		It("should return nil details when PipelineRun is ours", func() {
			prName := workflowexecution.PipelineRunName(targetResource)
			existingPR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "test-wfe",
						"kubernaut.ai/source-namespace":   "default",
					},
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPR).Build()
			reconciler := &workflowexecution.WorkflowExecutionReconciler{
				Client:             client,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
			}
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "test-wfe", Namespace: "default"},
				Spec:       workflowexecutionv1alpha1.WorkflowExecutionSpec{TargetResource: targetResource},
			}

			// AlreadyExists error
			alreadyExistsErr := apierrors.NewAlreadyExists(tektonv1.Resource("pipelineruns"), prName)
			details, err := reconciler.HandleAlreadyExists(ctx, wfe, alreadyExistsErr)
			Expect(err).ToNot(HaveOccurred())
			Expect(details).To(BeNil()) // Ours, no skip needed
		})

		It("should return skip details when PipelineRun belongs to another WFE", func() {
			prName := workflowexecution.PipelineRunName(targetResource)
			existingPR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "other-wfe",
						"kubernaut.ai/source-namespace":   "default",
					},
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPR).Build()
			reconciler := &workflowexecution.WorkflowExecutionReconciler{
				Client:             client,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
			}
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "test-wfe", Namespace: "default"},
				Spec:       workflowexecutionv1alpha1.WorkflowExecutionSpec{TargetResource: targetResource},
			}

			// AlreadyExists error
			alreadyExistsErr := apierrors.NewAlreadyExists(tektonv1.Resource("pipelineruns"), prName)
			details, err := reconciler.HandleAlreadyExists(ctx, wfe, alreadyExistsErr)
			Expect(err).ToNot(HaveOccurred())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
			Expect(details.ConflictingWorkflow.Name).To(Equal("other-wfe"))
		})
	})

	// ========================================
	// Day 3: markSkipped() Tests
	// ========================================

	Describe("MarkSkipped", func() {
		It("should set phase to Skipped with details", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/my-app",
				},
			}
			// Use WithStatusSubresource to enable status updates
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(wfe).
				WithStatusSubresource(wfe).
				Build()
			reconciler := &workflowexecution.WorkflowExecutionReconciler{
				Client:             client,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
				CooldownPeriod:     5 * time.Minute,
			}

			skipDetails := &workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonResourceBusy,
				Message:   "Another workflow is already running",
				SkippedAt: metav1.Now(),
				ConflictingWorkflow: &workflowexecutionv1alpha1.ConflictingWorkflowRef{
					Name:           "conflicting-wfe",
					WorkflowID:     "test-workflow",
					StartedAt:      metav1.Now(),
					TargetResource: "default/deployment/my-app",
				},
			}

			err := reconciler.MarkSkipped(ctx, wfe, skipDetails)
			Expect(err).ToNot(HaveOccurred())

			// Fetch updated WFE
			updated := &workflowexecutionv1alpha1.WorkflowExecution{}
			err = client.Get(ctx, types.NamespacedName{Name: "test-wfe", Namespace: "default"}, updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseSkipped))
			Expect(updated.Status.SkipDetails).ToNot(BeNil())
			Expect(updated.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
		})
	})

	// ========================================
	// Day 4: BuildPipelineRun() Tests
	// TDD RED Phase: Tests written, implementation pending
	// ========================================

	Describe("BuildPipelineRun", func() {
		var (
			wfe        *workflowexecutionv1alpha1.WorkflowExecution
			reconciler *workflowexecution.WorkflowExecutionReconciler
		)

		BeforeEach(func() {
			wfe = &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe",
					Namespace: "payment",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "payment/deployment/payment-api",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "restart-deployment",
						Version:        "1.0.0",
						ContainerImage: "ghcr.io/kubernaut/workflows/restart-deployment:v1.0.0",
					},
					Parameters: map[string]string{
						"NAMESPACE":       "payment",
						"DEPLOYMENT_NAME": "payment-api",
					},
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
				CooldownPeriod:     5 * time.Minute,
				ServiceAccountName: "kubernaut-workflow-runner",
			}
		})

		It("should create PipelineRun with deterministic name", func() {
			pr := reconciler.BuildPipelineRun(wfe)

			// Name should be deterministic based on targetResource
			expectedName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
			Expect(pr.Name).To(Equal(expectedName))
		})

		It("should create PipelineRun in execution namespace", func() {
			pr := reconciler.BuildPipelineRun(wfe)

			// DD-WE-002: PipelineRuns always in kubernaut-workflows
			Expect(pr.Namespace).To(Equal("kubernaut-workflows"))
		})

		It("should set cross-namespace tracking labels", func() {
			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution", "test-wfe"))
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/source-namespace", "payment"))
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-id", "restart-deployment"))
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/target-resource", "payment/deployment/payment-api"))
		})

		It("should use bundle resolver with correct params", func() {
			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr.Spec.PipelineRef).ToNot(BeNil())
			Expect(string(pr.Spec.PipelineRef.ResolverRef.Resolver)).To(Equal("bundles"))

			// Check bundle param
			var bundleParam, nameParam, kindParam *tektonv1.Param
			for i := range pr.Spec.PipelineRef.ResolverRef.Params {
				p := &pr.Spec.PipelineRef.ResolverRef.Params[i]
				switch p.Name {
				case "bundle":
					bundleParam = p
				case "name":
					nameParam = p
				case "kind":
					kindParam = p
				}
			}

			Expect(bundleParam).ToNot(BeNil())
			Expect(bundleParam.Value.StringVal).To(Equal("ghcr.io/kubernaut/workflows/restart-deployment:v1.0.0"))

			Expect(nameParam).ToNot(BeNil())
			Expect(nameParam.Value.StringVal).To(Equal("workflow"))

			Expect(kindParam).ToNot(BeNil())
			Expect(kindParam.Value.StringVal).To(Equal("pipeline"))
		})

		It("should pass workflow parameters to PipelineRun", func() {
			pr := reconciler.BuildPipelineRun(wfe)

			// Check params are passed
			paramMap := make(map[string]string)
			for _, p := range pr.Spec.Params {
				paramMap[p.Name] = p.Value.StringVal
			}

			Expect(paramMap).To(HaveKeyWithValue("NAMESPACE", "payment"))
			Expect(paramMap).To(HaveKeyWithValue("DEPLOYMENT_NAME", "payment-api"))
		})

		It("should set ServiceAccountName in TaskRunTemplate", func() {
			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("kubernaut-workflow-runner"))
		})

		It("should handle empty parameters", func() {
			wfe.Spec.Parameters = nil
			pr := reconciler.BuildPipelineRun(wfe)

			Expect(pr.Spec.Params).To(BeEmpty())
		})
	})

	// ========================================
	// Day 4: ConvertParameters() Tests
	// ========================================

	Describe("ConvertParameters", func() {
		var reconciler *workflowexecution.WorkflowExecutionReconciler

		BeforeEach(func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}
		})

		It("should convert map to Tekton params", func() {
			params := map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
			}

			tektonParams := reconciler.ConvertParameters(params)

			Expect(tektonParams).To(HaveLen(2))

			paramMap := make(map[string]string)
			for _, p := range tektonParams {
				paramMap[p.Name] = p.Value.StringVal
			}

			Expect(paramMap).To(HaveKeyWithValue("KEY1", "value1"))
			Expect(paramMap).To(HaveKeyWithValue("KEY2", "value2"))
		})

		It("should return empty slice for nil params", func() {
			tektonParams := reconciler.ConvertParameters(nil)
			Expect(tektonParams).To(BeEmpty())
		})

		It("should return empty slice for empty map", func() {
			tektonParams := reconciler.ConvertParameters(map[string]string{})
			Expect(tektonParams).To(BeEmpty())
		})

		It("should set param type to string", func() {
			params := map[string]string{"KEY": "value"}
			tektonParams := reconciler.ConvertParameters(params)

			Expect(tektonParams[0].Value.Type).To(Equal(tektonv1.ParamTypeString))
		})
	})

	// ========================================
	// Day 4: FindWFEForPipelineRun() Tests
	// ========================================

	Describe("FindWFEForPipelineRun", func() {
		var reconciler *workflowexecution.WorkflowExecutionReconciler

		BeforeEach(func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		It("should return reconcile request for labeled PipelineRun", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-abc123",
					Namespace: "kubernaut-workflows",
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "my-wfe",
						"kubernaut.ai/source-namespace":   "payment",
					},
				},
			}

			requests := reconciler.FindWFEForPipelineRun(ctx, pr)

			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("my-wfe"))
			Expect(requests[0].Namespace).To(Equal("payment"))
		})

		It("should return empty for PipelineRun without workflow-execution label", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-other-pr",
					Namespace: "kubernaut-workflows",
					Labels: map[string]string{
						"kubernaut.ai/source-namespace": "payment",
					},
				},
			}

			requests := reconciler.FindWFEForPipelineRun(ctx, pr)

			Expect(requests).To(BeEmpty())
		})

		It("should return empty for PipelineRun without source-namespace label", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-other-pr",
					Namespace: "kubernaut-workflows",
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "my-wfe",
					},
				},
			}

			requests := reconciler.FindWFEForPipelineRun(ctx, pr)

			Expect(requests).To(BeEmpty())
		})

		It("should return empty for PipelineRun without labels", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-other-pr",
					Namespace: "kubernaut-workflows",
				},
			}

			requests := reconciler.FindWFEForPipelineRun(ctx, pr)

			Expect(requests).To(BeEmpty())
		})
	})
})
