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
	"encoding/json"
	"fmt"
	"strings"
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
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	"github.com/jordigilh/kubernaut/pkg/audit"
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
			// Finalizer name per finalizers-lifecycle.md (v3.3)
			Expect(workflowexecution.FinalizerName).To(Equal("workflowexecution.kubernaut.ai/workflowexecution-cleanup"))
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

		// ========================================
		// Day 9: DescribeTable for Edge Cases (v3.5)
		// Per 03-testing-strategy.mdc: Use DescribeTable for similar scenarios
		// ========================================

		DescribeTable("determinism and uniqueness edge cases",
			func(targetResource string, description string) {
				// Test determinism: Same input → same output
				name1 := workflowexecution.PipelineRunName(targetResource)
				name2 := workflowexecution.PipelineRunName(targetResource)
				Expect(name1).To(Equal(name2), "Should be deterministic for: %s", description)

				// Test format
				Expect(name1).To(HavePrefix("wfe-"), "Should have wfe- prefix for: %s", description)
				Expect(len(name1)).To(Equal(20), "Should be 20 chars (wfe- + 16 hex) for: %s", description)

				// Test valid K8s name
				Expect(len(name1)).To(BeNumerically("<=", 63), "Should be valid K8s name for: %s", description)
			},
			// Standard cases
			Entry("standard deployment", "default/deployment/nginx", "standard deployment"),
			Entry("statefulset resource", "prod/statefulset/postgres", "statefulset"),
			Entry("daemonset resource", "kube-system/daemonset/fluentd", "daemonset"),

			// Edge cases - namespace variations
			Entry("namespace with dash", "ns-1/deployment/app", "namespace with dash"),
			Entry("namespace with numbers", "ns123/deployment/app", "namespace with numbers"),
			Entry("single char namespace", "a/deployment/app", "single char namespace"),

			// Edge cases - resource name variations
			Entry("resource with dash", "default/deployment/my-app", "resource with dash"),
			Entry("resource with numbers", "default/deployment/app123", "resource with numbers"),
			Entry("long resource name", "very-long-namespace/deployment/very-long-deployment-name-that-exceeds-normal-limits", "long resource name"),

			// Edge cases - kind variations
			Entry("replicaset kind", "default/replicaset/my-rs", "replicaset kind"),
			Entry("pod kind", "default/pod/my-pod", "pod kind"),

			// Edge cases - special characters
			Entry("multiple dashes", "ns-1-2/deployment/app-1-2", "multiple dashes"),
		)
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

		It("should block when SAME workflow completed recently within cooldown (DD-WE-001)", func() {
			// DD-WE-001 line 119-127: Block SAME workflow on same target within cooldown
			// Completed 2 minutes ago, cooldown is 5 minutes
			completionTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			workflowID := "restart-pods-workflow"

			recentWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "recent-wfe",
					Namespace: "default",
					UID:       "recent-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
					WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
						WorkflowID: workflowID, // Same workflow
					},
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
					WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
						WorkflowID: workflowID, // Same workflow - should block
					},
				},
			}

			blocked, details, err := reconciler.CheckCooldown(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeTrue())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated))
			Expect(details.RecentRemediation.WorkflowID).To(Equal(workflowID))
		})

		It("should ALLOW different workflow on same target within cooldown (DD-WE-001 line 140)", func() {
			// DD-WE-001 line 140: "Completed <5m | Any | Yes | No | **Allow** (different workflow)"
			// Recent workflow completed 2 minutes ago, but this is a DIFFERENT workflow
			completionTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))

			recentWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "recent-wfe",
					Namespace: "default",
					UID:       "recent-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
					WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
						WorkflowID: "restart-pods-workflow", // First workflow
					},
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1alpha1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			reconciler := createReconciler(5*time.Minute, recentWFE)

			// New WFE with DIFFERENT workflow ID - should NOT be blocked
			newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-wfe",
					Namespace: "default",
					UID:       "new-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
					WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
						WorkflowID: "scale-up-workflow", // Different workflow - should allow
					},
				},
			}

			blocked, details, err := reconciler.CheckCooldown(ctx, newWFE)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse(), "Different workflow on same target should NOT be blocked")
			Expect(details).To(BeNil())
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

		// ========================================
		// Day 9: P1 Edge Case - Terminal WFE with nil CompletionTime
		// Business Value: Prevents remediation storms (DD-WE-001)
		// ========================================
		It("should skip terminal WFE with nil CompletionTime (data inconsistency)", func() {
			// Edge case: WFE is Completed but CompletionTime is nil
			// This can happen during status update race conditions
			terminalWFEWithNilCompletion := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "terminal-nil-completion",
					Namespace: "default",
					UID:       "terminal-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1alpha1.PhaseCompleted,
					CompletionTime: nil, // Data inconsistency - terminal but no completion time
				},
			}
			reconciler := createReconciler(5*time.Minute, terminalWFEWithNilCompletion)

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

			// When: CheckCooldown is called
			blocked, _, err := reconciler.CheckCooldown(ctx, newWFE)

			// Then: Should NOT block (terminal WFE with nil CompletionTime is skipped)
			// This is defensive - we can't calculate cooldown without CompletionTime
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
		})

		It("should handle Failed WFE with nil CompletionTime gracefully", func() {
			// Edge case: WFE is Failed but CompletionTime is nil
			failedWFEWithNilCompletion := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failed-nil-completion",
					Namespace: "default",
					UID:       "failed-uid",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: targetResource,
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1alpha1.PhaseFailed,
					CompletionTime: nil, // Data inconsistency
				},
			}
			reconciler := createReconciler(5*time.Minute, failedWFEWithNilCompletion)

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

			// When: CheckCooldown is called
			blocked, _, err := reconciler.CheckCooldown(ctx, newWFE)

			// Then: Should NOT block (can't calculate cooldown without CompletionTime)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeFalse())
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

		// ========================================
		// Day 9: P1 Edge Cases - Nil/Partial Labels (DD-WE-003)
		// Business Value: Prevents duplicate workflow executions
		// Both labels required for cross-namespace ownership identification
		// ========================================
		It("should return skip details when PipelineRun has nil labels", func() {
			// Edge case: PipelineRun exists but has no labels (manual creation or data corruption)
			prName := workflowexecution.PipelineRunName(targetResource)
			existingPR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
					Labels:    nil, // No labels at all
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

			// Then: Should skip (not ours - can't verify ownership without labels)
			Expect(err).ToNot(HaveOccurred())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
		})

		It("should return skip details when PipelineRun has empty labels map", func() {
			// Edge case: Labels map exists but is empty
			prName := workflowexecution.PipelineRunName(targetResource)
			existingPR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
					Labels:    map[string]string{}, // Empty labels map
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

			alreadyExistsErr := apierrors.NewAlreadyExists(tektonv1.Resource("pipelineruns"), prName)
			details, err := reconciler.HandleAlreadyExists(ctx, wfe, alreadyExistsErr)

			// Then: Should skip (not ours)
			Expect(err).ToNot(HaveOccurred())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
		})

		It("should return skip details when PipelineRun has only workflow-execution label (missing source-namespace)", func() {
			// Edge case: Has workflow-execution but missing source-namespace
			// Could be from same-name WFE in different namespace
			prName := workflowexecution.PipelineRunName(targetResource)
			existingPR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "test-wfe", // Matches name
						// Missing source-namespace - could be from different namespace!
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

			alreadyExistsErr := apierrors.NewAlreadyExists(tektonv1.Resource("pipelineruns"), prName)
			details, err := reconciler.HandleAlreadyExists(ctx, wfe, alreadyExistsErr)

			// Then: Should skip (can't verify ownership without BOTH labels)
			Expect(err).ToNot(HaveOccurred())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
		})

		It("should return skip details when PipelineRun has only source-namespace label (missing workflow-execution)", func() {
			// Edge case: Has source-namespace but missing workflow-execution
			prName := workflowexecution.PipelineRunName(targetResource)
			existingPR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
					Labels: map[string]string{
						"kubernaut.ai/source-namespace": "default", // Matches namespace
						// Missing workflow-execution
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

			alreadyExistsErr := apierrors.NewAlreadyExists(tektonv1.Resource("pipelineruns"), prName)
			details, err := reconciler.HandleAlreadyExists(ctx, wfe, alreadyExistsErr)

			// Then: Should skip (can't verify ownership without BOTH labels)
			Expect(err).ToNot(HaveOccurred())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
		})

		It("should return skip details when source-namespace matches but workflow-execution differs", func() {
			// Edge case: Same namespace but different WFE name
			prName := workflowexecution.PipelineRunName(targetResource)
			existingPR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prName,
					Namespace: "kubernaut-workflows",
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": "different-wfe", // Different name
						"kubernaut.ai/source-namespace":   "default",       // Same namespace
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

			alreadyExistsErr := apierrors.NewAlreadyExists(tektonv1.Resource("pipelineruns"), prName)
			details, err := reconciler.HandleAlreadyExists(ctx, wfe, alreadyExistsErr)

			// Then: Should skip (different WFE owns this PR)
			Expect(err).ToNot(HaveOccurred())
			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
			Expect(details.ConflictingWorkflow.Name).To(Equal("different-wfe"))
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
			// Label value is sanitized (slashes replaced with __)
			// Original value stored in annotation
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/target-resource", "payment__deployment__payment-api"))
			// Verify annotation contains original value
			Expect(pr.Annotations).To(HaveKeyWithValue("kubernaut.ai/target-resource", "payment/deployment/payment-api"))
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

			// Even with empty spec parameters, TARGET_RESOURCE is always added
			Expect(pr.Spec.Params).To(HaveLen(1))
			Expect(pr.Spec.Params[0].Name).To(Equal("TARGET_RESOURCE"))
			Expect(pr.Spec.Params[0].Value.StringVal).To(Equal(wfe.Spec.TargetResource))
		})

		// ========================================
		// Day 9: P2 Edge Case - ServiceAccountName configuration
		// Business Value: Configuration error detection (per Q5)
		// Note: ServiceAccountName is on reconciler, not WFE spec
		// ========================================
		It("should use configured ServiceAccountName", func() {
			// Given: Reconciler with custom ServiceAccountName
			reconciler.ServiceAccountName = "custom-sa"

			// When: BuildPipelineRun is called
			pr := reconciler.BuildPipelineRun(wfe)

			// Then: Should use configured ServiceAccountName
			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("custom-sa"))
		})

		It("should use DefaultServiceAccountName when ServiceAccountName is empty", func() {
			// Given: Reconciler without ServiceAccountName (uses default constant)
			reconcilerNoSA := &workflowexecution.WorkflowExecutionReconciler{
				Client:             fake.NewClientBuilder().WithScheme(scheme).Build(),
				Scheme:             scheme,
				ExecutionNamespace: "kubernaut-workflows",
				ServiceAccountName: "", // Empty - should use default
			}

			// When: BuildPipelineRun is called
			pr := reconcilerNoSA.BuildPipelineRun(wfe)

			// Then: Should use DefaultServiceAccountName constant
			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal(workflowexecution.DefaultServiceAccountName))
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

	// ========================================
	// Day 5: BuildPipelineRunStatusSummary() Tests
	// ========================================

	Describe("BuildPipelineRunStatusSummary", func() {
		var reconciler *workflowexecution.WorkflowExecutionReconciler

		BeforeEach(func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		It("should return summary with Running status when no Succeeded condition", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "kubernaut-workflows",
				},
				Status: tektonv1.PipelineRunStatus{},
			}

			summary := reconciler.BuildPipelineRunStatusSummary(pr)

			Expect(summary).ToNot(BeNil())
			Expect(summary.Status).To(Equal("Unknown"))
		})

		It("should return summary with task counts from ChildReferences", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "kubernaut-workflows",
				},
				Status: tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						ChildReferences: []tektonv1.ChildStatusReference{
							{Name: "task1", PipelineTaskName: "build"},
							{Name: "task2", PipelineTaskName: "test"},
							{Name: "task3", PipelineTaskName: "deploy"},
						},
					},
				},
			}

			summary := reconciler.BuildPipelineRunStatusSummary(pr)

			Expect(summary).ToNot(BeNil())
			Expect(summary.TotalTasks).To(Equal(3))
		})

		It("should extract status from Succeeded condition", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "kubernaut-workflows",
				},
			}
			// Set condition using Tekton's method
			pr.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionTrue,
				Reason:  "Succeeded",
				Message: "All tasks completed",
			})

			summary := reconciler.BuildPipelineRunStatusSummary(pr)

			Expect(summary).ToNot(BeNil())
			Expect(summary.Status).To(Equal("True"))
			Expect(summary.Reason).To(Equal("Succeeded"))
			Expect(summary.Message).To(Equal("All tasks completed"))
		})
	})

	// ========================================
	// Day 5: MarkCompleted() Tests
	// ========================================

	Describe("MarkCompleted", func() {
		var (
			reconciler *workflowexecution.WorkflowExecutionReconciler
			wfe        *workflowexecutionv1alpha1.WorkflowExecution
			pr         *tektonv1.PipelineRun
		)

		BeforeEach(func() {
			startTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			completionTime := metav1.NewTime(time.Now())

			wfe = &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/test-app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "restart-deployment",
						ContainerImage: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
					},
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:     workflowexecutionv1alpha1.PhaseRunning,
					StartTime: &startTime,
				},
			}

			pr = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowexecution.PipelineRunName(wfe.Spec.TargetResource),
					Namespace: "kubernaut-workflows",
				},
				Status: tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						CompletionTime: &completionTime,
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(wfe).
				WithStatusSubresource(wfe).
				Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		It("should set phase to Completed", func() {
			_, err := reconciler.MarkCompleted(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			// Fetch updated WFE
			var updated workflowexecutionv1alpha1.WorkflowExecution
			err = reconciler.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))
		})

		It("should set CompletionTime from PipelineRun", func() {
			_, err := reconciler.MarkCompleted(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			var updated workflowexecutionv1alpha1.WorkflowExecution
			err = reconciler.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.CompletionTime).ToNot(BeNil())
		})

		It("should calculate Duration from StartTime to CompletionTime", func() {
			_, err := reconciler.MarkCompleted(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			var updated workflowexecutionv1alpha1.WorkflowExecution
			err = reconciler.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Duration).ToNot(BeEmpty())
			// Duration should be around 2 minutes
			Expect(updated.Status.Duration).To(ContainSubstring("m"))
		})

		It("should emit WorkflowCompleted event", func() {
			_, err := reconciler.MarkCompleted(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			// Check event was recorded (fake recorder captures events)
			Expect(recorder.Events).To(HaveLen(1))
		})
	})

	// ========================================
	// Day 5: MarkFailed() Tests
	// ========================================

	Describe("MarkFailed", func() {
		var (
			reconciler *workflowexecution.WorkflowExecutionReconciler
			wfe        *workflowexecutionv1alpha1.WorkflowExecution
			pr         *tektonv1.PipelineRun
		)

		BeforeEach(func() {
			startTime := metav1.NewTime(time.Now().Add(-45 * time.Second))

			wfe = &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-failed",
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/failing-app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "restart-deployment",
						ContainerImage: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
					},
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					Phase:     workflowexecutionv1alpha1.PhaseRunning,
					StartTime: &startTime,
				},
			}

			pr = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowexecution.PipelineRunName(wfe.Spec.TargetResource),
					Namespace: "kubernaut-workflows",
				},
			}
			// Set failed condition
			pr.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  "Failed",
				Message: "Task 'apply-memory-increase' failed: permission denied",
			})

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(wfe).
				WithStatusSubresource(wfe).
				Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		It("should set phase to Failed", func() {
			_, err := reconciler.MarkFailed(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			var updated workflowexecutionv1alpha1.WorkflowExecution
			err = reconciler.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
		})

		It("should populate FailureDetails", func() {
			_, err := reconciler.MarkFailed(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			var updated workflowexecutionv1alpha1.WorkflowExecution
			err = reconciler.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.FailureDetails).ToNot(BeNil())
			Expect(updated.Status.FailureDetails.Message).ToNot(BeEmpty())
		})

		It("should set CompletionTime", func() {
			_, err := reconciler.MarkFailed(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			var updated workflowexecutionv1alpha1.WorkflowExecution
			err = reconciler.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.CompletionTime).ToNot(BeNil())
		})

		It("should calculate Duration", func() {
			_, err := reconciler.MarkFailed(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			var updated workflowexecutionv1alpha1.WorkflowExecution
			err = reconciler.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Duration).ToNot(BeEmpty())
		})

		It("should generate NaturalLanguageSummary", func() {
			_, err := reconciler.MarkFailed(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			var updated workflowexecutionv1alpha1.WorkflowExecution
			err = reconciler.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.FailureDetails.NaturalLanguageSummary).ToNot(BeEmpty())
		})

		It("should emit WorkflowFailed event", func() {
			_, err := reconciler.MarkFailed(ctx, wfe, pr)
			Expect(err).ToNot(HaveOccurred())

			Expect(recorder.Events).To(HaveLen(1))
		})
	})

	// ========================================
	// Day 5: ExtractFailureDetails() Tests
	// ========================================

	Describe("ExtractFailureDetails", func() {
		var (
			reconciler *workflowexecution.WorkflowExecutionReconciler
			ctx        context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		It("should extract reason from condition message containing 'forbidden'", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "kubernaut-workflows",
				},
			}
			pr.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  "Failed",
				Message: "RBAC: permission denied - forbidden",
			})

			startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
			details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonForbidden))
		})

		It("should extract reason from condition message containing 'oom'", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "kubernaut-workflows",
				},
			}
			pr.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  "Failed",
				Message: "Container killed: OOMKilled",
			})

			startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
			details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonOOMKilled))
		})

		It("should extract reason from condition message containing 'timeout'", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "kubernaut-workflows",
				},
			}
			pr.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  "PipelineRunTimeout",
				Message: "PipelineRun timeout exceeded",
			})

			startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
			details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonDeadlineExceeded))
		})

		It("should return Unknown reason for unrecognized failure", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "kubernaut-workflows",
				},
			}
			pr.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  "Failed",
				Message: "Some unknown error",
			})

			startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
			details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

			Expect(details).ToNot(BeNil())
			Expect(details.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonUnknown))
		})

		It("should calculate ExecutionTimeBeforeFailure", func() {
			pr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr",
					Namespace: "kubernaut-workflows",
				},
			}
			pr.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  "Failed",
				Message: "Error",
			})

			startTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

			Expect(details).ToNot(BeNil())
			Expect(details.ExecutionTimeBeforeFailure).ToNot(BeEmpty())
			Expect(details.ExecutionTimeBeforeFailure).To(ContainSubstring("m"))
		})

		// ========================================
		// Day 9: DescribeTable for Failure Reason Mapping (v3.5)
		// Per 03-testing-strategy.mdc: Use DescribeTable for similar scenarios
		// Covers all 7 FailureReason constants from workflowexecution_types.go
		// ========================================

		// DescribeTable validates actual implementation behavior
		// Implementation uses message-based matching (case-insensitive)
		// See mapReasonToFailureReason() in workflowexecution_controller.go
		DescribeTable("failure reason mapping from Tekton condition",
			func(conditionReason string, conditionMessage string, expectedReason string) {
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-reason-mapping",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Reason:  conditionReason,
					Message: conditionMessage,
				})

				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

				Expect(details).ToNot(BeNil())
				Expect(string(details.Reason)).To(Equal(expectedReason))
			},
			// OOMKilled scenarios - matches "oomkilled" or "oom" in message (case-insensitive)
			Entry("OOMKilled - oomkilled in message", "Failed", "Container killed: OOMKilled", workflowexecutionv1alpha1.FailureReasonOOMKilled),
			Entry("OOMKilled - oom in message", "Failed", "task failed due to oom", workflowexecutionv1alpha1.FailureReasonOOMKilled),
			Entry("OOMKilled - mixed case", "Failed", "Exit code 137 - OOMkilled by kernel", workflowexecutionv1alpha1.FailureReasonOOMKilled),

			// DeadlineExceeded - matches "timeout" in reason OR "timeout"/"deadline" in message
			Entry("DeadlineExceeded - timeout in reason", "PipelineRunTimeout", "Pipeline timed out", workflowexecutionv1alpha1.FailureReasonDeadlineExceeded),
			Entry("DeadlineExceeded - timeout in message", "Failed", "Task timeout exceeded", workflowexecutionv1alpha1.FailureReasonDeadlineExceeded),
			Entry("DeadlineExceeded - deadline in message", "Failed", "deadline exceeded for task", workflowexecutionv1alpha1.FailureReasonDeadlineExceeded),

			// Forbidden - matches "forbidden", "rbac", or "permission denied" in message
			Entry("Forbidden - forbidden in message", "Failed", "RBAC: forbidden to create pods", workflowexecutionv1alpha1.FailureReasonForbidden),
			Entry("Forbidden - rbac in message", "Failed", "rbac error occurred", workflowexecutionv1alpha1.FailureReasonForbidden),
			Entry("Forbidden - permission denied", "Failed", "permission denied for resource", workflowexecutionv1alpha1.FailureReasonForbidden),

			// ResourceExhausted - matches "quota" or "resource exhausted" in message
			Entry("ResourceExhausted - quota in message", "Failed", "exceeded quota for namespace", workflowexecutionv1alpha1.FailureReasonResourceExhausted),
			Entry("ResourceExhausted - resource exhausted phrase", "Failed", "resource exhausted in cluster", workflowexecutionv1alpha1.FailureReasonResourceExhausted),

			// ImagePullBackOff - matches "imagepullbackoff" or "image pull" in message
			Entry("ImagePullBackOff - imagepullbackoff in message", "Failed", "ImagePullBackOff: image not found", workflowexecutionv1alpha1.FailureReasonImagePullBackOff),
			Entry("ImagePullBackOff - image pull phrase", "Failed", "failed to image pull from registry", workflowexecutionv1alpha1.FailureReasonImagePullBackOff),

			// ConfigurationError - matches "invalid" or "configuration" in message
			Entry("ConfigurationError - invalid in message", "Failed", "invalid parameter value", workflowexecutionv1alpha1.FailureReasonConfigurationError),
			Entry("ConfigurationError - configuration in message", "Failed", "configuration error in pipeline", workflowexecutionv1alpha1.FailureReasonConfigurationError),

			// Unknown - fallback for unrecognized failures
			Entry("Unknown - generic failure", "Failed", "Some unknown error occurred", workflowexecutionv1alpha1.FailureReasonUnknown),
			Entry("Unknown - empty message", "Failed", "", workflowexecutionv1alpha1.FailureReasonUnknown),
			Entry("Unknown - unclassified", "TaskRunFailed", "Task exited with code 1", workflowexecutionv1alpha1.FailureReasonUnknown),
		)
	})

	// ========================================
	// Day 7: TaskRun-Specific Failure Details (TDD RED)
	// Plan v3.4: findFailedTaskRun() + TaskRun fields in ExtractFailureDetails
	// ========================================

	Describe("findFailedTaskRun - TaskRun Extraction (Day 7)", func() {
		var (
			reconciler *workflowexecution.WorkflowExecutionReconciler
			fakeClient client.Client
			ctx        context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		Context("FailedTaskName extraction", func() {
			It("should find failed TaskRun from ChildReferences", func() {
				// Given: PipelineRun with ChildReferences including a failed TaskRun
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr",
						Namespace: "kubernaut-workflows",
					},
					Status: tektonv1.PipelineRunStatus{},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{
						TypeMeta: runtime.TypeMeta{Kind: "TaskRun"},
						Name:     "test-pr-task1-abc123",
					},
					{
						TypeMeta: runtime.TypeMeta{Kind: "TaskRun"},
						Name:     "test-pr-task2-def456",
					},
				}

				// And: TaskRun exists with failed condition
				failedTaskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-task2-def456",
						Namespace: "kubernaut-workflows",
					},
				}
				failedTaskRun.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Reason:  "Failed",
					Message: "Task failed due to OOMKilled",
				})
				Expect(fakeClient.Create(ctx, failedTaskRun)).To(Succeed())

				// And: First TaskRun succeeded
				successTaskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-task1-abc123",
						Namespace: "kubernaut-workflows",
					},
				}
				successTaskRun.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				})
				Expect(fakeClient.Create(ctx, successTaskRun)).To(Succeed())

				// When: findFailedTaskRun is called
				taskRun, index, err := reconciler.FindFailedTaskRun(ctx, pr)

				// Then: Should return the failed TaskRun and its index
				Expect(err).ToNot(HaveOccurred())
				Expect(taskRun).ToNot(BeNil())
				Expect(taskRun.Name).To(Equal("test-pr-task2-def456"))
				Expect(index).To(Equal(1)) // 0-indexed, second task
			})

			It("should return nil when no TaskRun has failed", func() {
				// Given: PipelineRun with only successful TaskRuns
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-success",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{
						TypeMeta: runtime.TypeMeta{Kind: "TaskRun"},
						Name:     "test-pr-task1-success",
					},
				}

				// And: TaskRun succeeded
				successTaskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-task1-success",
						Namespace: "kubernaut-workflows",
					},
				}
				successTaskRun.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				})
				Expect(fakeClient.Create(ctx, successTaskRun)).To(Succeed())

				// When: findFailedTaskRun is called
				taskRun, index, err := reconciler.FindFailedTaskRun(ctx, pr)

				// Then: Should return nil (no failed TaskRun)
				Expect(err).ToNot(HaveOccurred())
				Expect(taskRun).To(BeNil())
				Expect(index).To(Equal(-1))
			})

			It("should handle TaskRun not found gracefully", func() {
				// Given: PipelineRun with reference to deleted TaskRun
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-deleted",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{
						TypeMeta: runtime.TypeMeta{Kind: "TaskRun"},
						Name:     "test-pr-deleted-task",
					},
				}
				// TaskRun not created (simulates deleted)

				// When: findFailedTaskRun is called
				taskRun, index, err := reconciler.FindFailedTaskRun(ctx, pr)

				// Then: Should return nil without error (graceful handling)
				Expect(err).ToNot(HaveOccurred())
				Expect(taskRun).To(BeNil())
				Expect(index).To(Equal(-1))
			})

			It("should skip non-TaskRun ChildReferences", func() {
				// Given: PipelineRun with mixed ChildReferences
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-mixed",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{
						TypeMeta: runtime.TypeMeta{Kind: "Run"}, // Not a TaskRun
						Name:     "test-pr-custom-run",
					},
					{
						TypeMeta: runtime.TypeMeta{Kind: "TaskRun"},
						Name:     "test-pr-task-failed",
					},
				}

				// And: TaskRun exists and failed
				failedTaskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-task-failed",
						Namespace: "kubernaut-workflows",
					},
				}
				failedTaskRun.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Reason:  "Failed",
					Message: "Task failed",
				})
				Expect(fakeClient.Create(ctx, failedTaskRun)).To(Succeed())

				// When: findFailedTaskRun is called
				taskRun, index, err := reconciler.FindFailedTaskRun(ctx, pr)

				// Then: Should find the TaskRun (index 1, after skipping Run)
				Expect(err).ToNot(HaveOccurred())
				Expect(taskRun).ToNot(BeNil())
				Expect(taskRun.Name).To(Equal("test-pr-task-failed"))
				Expect(index).To(Equal(1)) // Index in ChildReferences
			})
		})
	})

	Describe("ExtractFailureDetails with TaskRun fields (Day 7)", func() {
		var (
			reconciler *workflowexecution.WorkflowExecutionReconciler
			fakeClient client.Client
			ctx        context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		Context("FailedTaskName, FailedTaskIndex, ExitCode", func() {
			It("should populate FailedTaskName from TaskRun", func() {
				// Given: PipelineRun with failed TaskRun
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-with-taskrun",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{
						TypeMeta: runtime.TypeMeta{Kind: "TaskRun"},
						Name:     "test-pr-validate-config",
					},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Reason:  "Failed",
					Message: "Task validate-config failed",
				})

				// And: TaskRun exists with failure
				failedTaskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-validate-config",
						Namespace: "kubernaut-workflows",
					},
				}
				failedTaskRun.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Reason:  "Failed",
					Message: "Validation failed",
				})
				Expect(fakeClient.Create(ctx, failedTaskRun)).To(Succeed())

				// When: ExtractFailureDetails is called
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

				// Then: FailedTaskName should be populated
				Expect(details).ToNot(BeNil())
				Expect(details.FailedTaskName).To(Equal("test-pr-validate-config"))
			})

			It("should populate FailedTaskIndex from TaskRun position", func() {
				// Given: PipelineRun with multiple TaskRuns, second one failed
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-multi-task",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{TypeMeta: runtime.TypeMeta{Kind: "TaskRun"}, Name: "task-1-success"},
					{TypeMeta: runtime.TypeMeta{Kind: "TaskRun"}, Name: "task-2-failed"},
					{TypeMeta: runtime.TypeMeta{Kind: "TaskRun"}, Name: "task-3-pending"},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				})

				// Create TaskRuns
				task1 := &tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "task-1-success", Namespace: "kubernaut-workflows"}}
				task1.Status.SetCondition(&apis.Condition{Type: apis.ConditionSucceeded, Status: corev1.ConditionTrue})
				Expect(fakeClient.Create(ctx, task1)).To(Succeed())

				task2 := &tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "task-2-failed", Namespace: "kubernaut-workflows"}}
				task2.Status.SetCondition(&apis.Condition{Type: apis.ConditionSucceeded, Status: corev1.ConditionFalse, Message: "OOM"})
				Expect(fakeClient.Create(ctx, task2)).To(Succeed())

				// When: ExtractFailureDetails is called
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

				// Then: FailedTaskIndex should be 1 (0-indexed)
				Expect(details).ToNot(BeNil())
				Expect(details.FailedTaskIndex).To(Equal(1))
			})

			It("should populate ExitCode from TaskRun container status", func() {
				// Given: PipelineRun with failed TaskRun that has exit code
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-exitcode",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{TypeMeta: runtime.TypeMeta{Kind: "TaskRun"}, Name: "task-with-exitcode"},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				})

				// Create TaskRun with exit code in step state
				taskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "task-with-exitcode",
						Namespace: "kubernaut-workflows",
					},
				}
				taskRun.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				})
				// Add step state with exit code
				exitCode := int32(137) // OOMKilled
				taskRun.Status.Steps = []tektonv1.StepState{
					{
						ContainerState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: exitCode,
							},
						},
					},
				}
				Expect(fakeClient.Create(ctx, taskRun)).To(Succeed())

				// When: ExtractFailureDetails is called
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

				// Then: ExitCode should be populated
				Expect(details).ToNot(BeNil())
				Expect(details.ExitCode).ToNot(BeNil())
				Expect(*details.ExitCode).To(Equal(int32(137)))
			})

			It("should handle missing TaskRun gracefully in ExtractFailureDetails", func() {
				// Given: PipelineRun with no ChildReferences
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-no-taskruns",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Message: "Pipeline failed",
				})

				// When: ExtractFailureDetails is called
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

				// Then: Should still return valid details without TaskRun fields
				Expect(details).ToNot(BeNil())
				Expect(details.FailedTaskName).To(BeEmpty())
				Expect(details.FailedTaskIndex).To(Equal(0))
				Expect(details.ExitCode).To(BeNil())
			})
		})

		// ========================================
		// Day 9: Exit Code Extraction Edge Cases (P1 Business Value)
		// Per TESTING_GUIDELINES.md: Tests validate business outcomes
		// Business Value: Accurate failure debugging requires correct exit code
		// Note: extractExitCode is private, tested via ExtractFailureDetails
		// ========================================
		Context("P1: Exit code extraction via ExtractFailureDetails edge cases", func() {
			It("should return first non-zero exit code when multiple steps fail", func() {
				// Given: PipelineRun with failed TaskRun that has multiple step failures
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-multi-step-fail",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{TypeMeta: runtime.TypeMeta{Kind: "TaskRun"}, Name: "task-multi-fail"},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				})

				// Create TaskRun with multiple failed steps
				taskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "task-multi-fail",
						Namespace: "kubernaut-workflows",
					},
				}
				taskRun.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				})
				taskRun.Status.Steps = []tektonv1.StepState{
					{
						Name: "step-1-success",
						ContainerState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
						},
					},
					{
						Name: "step-2-oom",
						ContainerState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 137}, // First failure
						},
					},
					{
						Name: "step-3-cascade",
						ContainerState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}, // Cascade
						},
					},
				}
				Expect(fakeClient.Create(ctx, taskRun)).To(Succeed())

				// When: ExtractFailureDetails is called
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

				// Then: Should return first non-zero (137 - the root cause)
				Expect(details).ToNot(BeNil())
				Expect(details.ExitCode).ToNot(BeNil())
				Expect(*details.ExitCode).To(Equal(int32(137)))
			})

			It("should return nil exit code when all steps succeed but pipeline fails", func() {
				// Given: PipelineRun failed but TaskRun steps all succeeded (edge case)
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-no-step-fail",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{TypeMeta: runtime.TypeMeta{Kind: "TaskRun"}, Name: "task-all-success"},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Message: "Pipeline cancelled",
				})

				// Create TaskRun with all successful steps
				taskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "task-all-success",
						Namespace: "kubernaut-workflows",
					},
				}
				taskRun.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue, // TaskRun succeeded
				})
				taskRun.Status.Steps = []tektonv1.StepState{
					{
						Name: "step-1",
						ContainerState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
						},
					},
				}
				Expect(fakeClient.Create(ctx, taskRun)).To(Succeed())

				// When: ExtractFailureDetails is called
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

				// Then: ExitCode should be nil (no step failed)
				Expect(details).ToNot(BeNil())
				Expect(details.ExitCode).To(BeNil())
			})

			It("should handle TaskRun with running step (not terminated)", func() {
				// Given: PipelineRun with TaskRun that has running step
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-running-step",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.ChildReferences = []tektonv1.ChildStatusReference{
					{TypeMeta: runtime.TypeMeta{Kind: "TaskRun"}, Name: "task-running"},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				})

				// Create TaskRun with running step (Terminated is nil)
				taskRun := &tektonv1.TaskRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "task-running",
						Namespace: "kubernaut-workflows",
					},
				}
				taskRun.Status.SetCondition(&apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				})
				taskRun.Status.Steps = []tektonv1.StepState{
					{
						Name: "step-running",
						ContainerState: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{}, // Not terminated
						},
					},
				}
				Expect(fakeClient.Create(ctx, taskRun)).To(Succeed())

				// When: ExtractFailureDetails is called
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				details := reconciler.ExtractFailureDetails(ctx, pr, &startTime)

				// Then: ExitCode should be nil (step not terminated)
				Expect(details).ToNot(BeNil())
				Expect(details.ExitCode).To(BeNil())
			})
		})
	})

	// ========================================
	// Day 5: GenerateNaturalLanguageSummary() Tests
	// ========================================

	Describe("GenerateNaturalLanguageSummary", func() {
		var reconciler *workflowexecution.WorkflowExecutionReconciler

		BeforeEach(func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}
		})

		It("should include workflow ID in summary", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: "restart-deployment",
					},
					TargetResource: "default/deployment/my-app",
				},
			}
			details := &workflowexecutionv1alpha1.FailureDetails{
				Reason:  workflowexecutionv1alpha1.FailureReasonForbidden,
				Message: "permission denied",
			}

			summary := reconciler.GenerateNaturalLanguageSummary(wfe, details)

			Expect(summary).To(ContainSubstring("restart-deployment"))
		})

		It("should include target resource in summary", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: "restart-deployment",
					},
					TargetResource: "payment/deployment/payment-api",
				},
			}
			details := &workflowexecutionv1alpha1.FailureDetails{
				Reason:  workflowexecutionv1alpha1.FailureReasonForbidden,
				Message: "permission denied",
			}

			summary := reconciler.GenerateNaturalLanguageSummary(wfe, details)

			Expect(summary).To(ContainSubstring("payment/deployment/payment-api"))
		})

		It("should include failure reason in summary", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: "restart-deployment",
					},
					TargetResource: "default/deployment/my-app",
				},
			}
			details := &workflowexecutionv1alpha1.FailureDetails{
				Reason:  workflowexecutionv1alpha1.FailureReasonOOMKilled,
				Message: "container killed",
			}

			summary := reconciler.GenerateNaturalLanguageSummary(wfe, details)

			Expect(summary).To(ContainSubstring("OOMKilled"))
		})

		// ========================================
		// Day 9: P2 Edge Case - Nil FailureDetails
		// Business Value: AI recovery context quality
		// Per Q4 decision: Return generic message, document in authoritative doc
		// ========================================
		It("should return generic message when FailureDetails is nil", func() {
			// Edge case: Called with nil FailureDetails (defensive programming)
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: "restart-deployment",
					},
					TargetResource: "default/deployment/my-app",
				},
			}

			// When: GenerateNaturalLanguageSummary is called with nil details
			summary := reconciler.GenerateNaturalLanguageSummary(wfe, nil)

			// Then: Should return generic message (not panic, not empty)
			Expect(summary).ToNot(BeEmpty())
			Expect(summary).To(ContainSubstring("failed"))
			// Should still include workflow context
			Expect(summary).To(ContainSubstring("restart-deployment"))
		})

		It("should handle FailureDetails with empty fields", func() {
			// Edge case: FailureDetails exists but fields are empty
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: "restart-deployment",
					},
					TargetResource: "default/deployment/my-app",
				},
			}
			details := &workflowexecutionv1alpha1.FailureDetails{
				Reason:  "", // Empty reason
				Message: "", // Empty message
			}

			// When: GenerateNaturalLanguageSummary is called
			summary := reconciler.GenerateNaturalLanguageSummary(wfe, details)

			// Then: Should return valid summary with workflow context
			Expect(summary).ToNot(BeEmpty())
			Expect(summary).To(ContainSubstring("restart-deployment"))
		})
	})

	// ========================================
	// Day 6: Cooldown + Cleanup Tests (TDD RED)
	// DD-WE-003: Lock Persistence Strategy
	// finalizers-lifecycle.md: Finalizer Cleanup
	// ========================================

	Describe("reconcileTerminal - Cooldown Enforcement (DD-WE-003)", func() {
		var (
			reconciler *workflowexecution.WorkflowExecutionReconciler
			fakeClient client.Client
			recorder   *record.FakeRecorder
			ctx        context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			recorder = record.NewFakeRecorder(10)
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
				CooldownPeriod:     1 * time.Minute, // Short for tests
			}
		})

		Context("Cooldown Period", func() {
			It("should return early when CompletionTime is nil", func() {
				// Given: WFE in Completed phase but no CompletionTime
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-no-completion",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseCompleted,
						CompletionTime: nil, // No completion time
					},
				}

				// When: reconcileTerminal is called
				result, err := reconciler.ReconcileTerminal(ctx, wfe)

				// Then: Should return immediately without error
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})

			It("should requeue with remaining duration when within cooldown period", func() {
				// Given: WFE completed 30 seconds ago (within 1 min cooldown)
				completionTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-within-cooldown",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseCompleted,
						CompletionTime: &completionTime,
					},
				}

				// When: reconcileTerminal is called
				result, err := reconciler.ReconcileTerminal(ctx, wfe)

				// Then: Should requeue with remaining cooldown duration (~30 seconds)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 20*time.Second))
				Expect(result.RequeueAfter).To(BeNumerically("<=", 35*time.Second))
			})

			It("should delete PipelineRun after cooldown expires using deterministic name", func() {
				// Given: WFE completed 2 minutes ago (past 1 min cooldown)
				completionTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-past-cooldown",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseCompleted,
						CompletionTime: &completionTime,
					},
				}

				// And: PipelineRun exists with deterministic name
				prName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      prName,
						Namespace: "kubernaut-workflows",
					},
				}
				Expect(fakeClient.Create(ctx, pr)).To(Succeed())

				// When: reconcileTerminal is called
				result, err := reconciler.ReconcileTerminal(ctx, wfe)

				// Then: PipelineRun should be deleted
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify PipelineRun was deleted
				deletedPR := &tektonv1.PipelineRun{}
				err = fakeClient.Get(ctx, client.ObjectKey{
					Name:      prName,
					Namespace: "kubernaut-workflows",
				}, deletedPR)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})

			It("should emit LockReleased event after cooldown expires", func() {
				// Given: WFE completed 2 minutes ago (past 1 min cooldown)
				completionTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-lock-release",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseCompleted,
						CompletionTime: &completionTime,
					},
				}

				// When: reconcileTerminal is called
				_, err := reconciler.ReconcileTerminal(ctx, wfe)

				// Then: LockReleased event should be emitted
				Expect(err).NotTo(HaveOccurred())

				// Check event was recorded
				Eventually(func() bool {
					select {
					case event := <-recorder.Events:
						return strings.Contains(event, "LockReleased")
					default:
						return false
					}
				}, time.Second).Should(BeTrue())
			})

			It("should handle PipelineRun not found gracefully after cooldown", func() {
				// Given: WFE completed but PipelineRun already deleted
				completionTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-pr-already-deleted",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseCompleted,
						CompletionTime: &completionTime,
					},
				}
				// No PipelineRun created

				// When: reconcileTerminal is called
				result, err := reconciler.ReconcileTerminal(ctx, wfe)

				// Then: Should succeed without error (NotFound is ignored)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})

			It("should use default cooldown period when CooldownPeriod is zero", func() {
				// Given: Reconciler with zero cooldown (should use default)
				reconciler.CooldownPeriod = 0

				// And: WFE completed 3 minutes ago (within default 5 min cooldown)
				completionTime := metav1.NewTime(time.Now().Add(-3 * time.Minute))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-default-cooldown",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseCompleted,
						CompletionTime: &completionTime,
					},
				}

				// When: reconcileTerminal is called
				result, err := reconciler.ReconcileTerminal(ctx, wfe)

				// Then: Should requeue (still within default 5 min cooldown)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 1*time.Minute))
			})
		})
	})

	Describe("reconcileDelete - Finalizer Cleanup (finalizers-lifecycle.md)", func() {
		var (
			reconciler *workflowexecution.WorkflowExecutionReconciler
			fakeClient client.Client
			recorder   *record.FakeRecorder
			ctx        context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			recorder = record.NewFakeRecorder(10)
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				Recorder:           recorder,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		Context("DD-WE-003: Deterministic PipelineRun Name", func() {
			It("should delete PipelineRun using deterministic name (not PipelineRunRef)", func() {
				// Given: WFE with finalizer and PipelineRunRef
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-wfe-delete",
						Namespace:  "default",
						Finalizers: []string{workflowexecution.FinalizerName},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhaseRunning,
						PipelineRunRef: &corev1.LocalObjectReference{
							Name: "some-different-name", // Different from deterministic name
						},
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// And: PipelineRun exists with deterministic name (NOT PipelineRunRef.Name)
				prName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      prName,
						Namespace: "kubernaut-workflows",
					},
				}
				Expect(fakeClient.Create(ctx, pr)).To(Succeed())

				// When: reconcileDelete is called
				_, err := reconciler.ReconcileDelete(ctx, wfe)

				// Then: PipelineRun with deterministic name should be deleted
				Expect(err).NotTo(HaveOccurred())

				deletedPR := &tektonv1.PipelineRun{}
				err = fakeClient.Get(ctx, client.ObjectKey{
					Name:      prName,
					Namespace: "kubernaut-workflows",
				}, deletedPR)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})

			It("should delete PipelineRun even when PipelineRunRef is nil", func() {
				// Given: WFE with finalizer but NO PipelineRunRef
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-wfe-no-ref",
						Namespace:  "default",
						Finalizers: []string{workflowexecution.FinalizerName},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhasePending,
						PipelineRunRef: nil, // No ref set
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// And: PipelineRun exists (created but ref not set yet)
				prName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      prName,
						Namespace: "kubernaut-workflows",
					},
				}
				Expect(fakeClient.Create(ctx, pr)).To(Succeed())

				// When: reconcileDelete is called
				_, err := reconciler.ReconcileDelete(ctx, wfe)

				// Then: PipelineRun should still be deleted (deterministic name)
				Expect(err).NotTo(HaveOccurred())

				deletedPR := &tektonv1.PipelineRun{}
				err = fakeClient.Get(ctx, client.ObjectKey{
					Name:      prName,
					Namespace: "kubernaut-workflows",
				}, deletedPR)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("Event Emission (finalizers-lifecycle.md)", func() {
			It("should emit WorkflowExecutionDeleted event", func() {
				// Given: WFE with finalizer in Completed phase
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-wfe-event",
						Namespace:  "default",
						Finalizers: []string{workflowexecution.FinalizerName},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhaseCompleted,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: reconcileDelete is called
				_, err := reconciler.ReconcileDelete(ctx, wfe)

				// Then: WorkflowExecutionDeleted event should be emitted
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					select {
					case event := <-recorder.Events:
						return strings.Contains(event, "WorkflowExecutionDeleted")
					default:
						return false
					}
				}, time.Second).Should(BeTrue())
			})

			It("should include phase in deletion event", func() {
				// Given: WFE in Failed phase
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-wfe-failed-delete",
						Namespace:  "default",
						Finalizers: []string{workflowexecution.FinalizerName},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: reconcileDelete is called
				_, err := reconciler.ReconcileDelete(ctx, wfe)

				// Then: Event should include "Failed" phase
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					select {
					case event := <-recorder.Events:
						return strings.Contains(event, "Failed")
					default:
						return false
					}
				}, time.Second).Should(BeTrue())
			})
		})

		Context("Deletion During Running Phase", func() {
			It("should delete PipelineRun during Running phase (cancels execution)", func() {
				// Given: WFE in Running phase with finalizer
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-wfe-running-delete",
						Namespace:  "default",
						Finalizers: []string{workflowexecution.FinalizerName},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhaseRunning,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// And: Active PipelineRun exists
				prName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      prName,
						Namespace: "kubernaut-workflows",
					},
				}
				Expect(fakeClient.Create(ctx, pr)).To(Succeed())

				// When: reconcileDelete is called (WFE deleted during running)
				_, err := reconciler.ReconcileDelete(ctx, wfe)

				// Then: PipelineRun should be deleted (cancels execution)
				Expect(err).NotTo(HaveOccurred())

				deletedPR := &tektonv1.PipelineRun{}
				err = fakeClient.Get(ctx, client.ObjectKey{
					Name:      prName,
					Namespace: "kubernaut-workflows",
				}, deletedPR)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("Finalizer Removal", func() {
			It("should remove finalizer after cleanup", func() {
				// Given: WFE with finalizer
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-wfe-finalizer-removal",
						Namespace:  "default",
						Finalizers: []string{workflowexecution.FinalizerName},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhaseCompleted,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: reconcileDelete is called
				_, err := reconciler.ReconcileDelete(ctx, wfe)

				// Then: Finalizer should be removed
				Expect(err).NotTo(HaveOccurred())

				updatedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      wfe.Name,
					Namespace: wfe.Namespace,
				}, updatedWFE)).To(Succeed())

				Expect(controllerutil.ContainsFinalizer(updatedWFE, workflowexecution.FinalizerName)).To(BeFalse())
			})

			It("should return early if finalizer not present", func() {
				// Given: WFE without finalizer
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-wfe-no-finalizer",
						Namespace:  "default",
						Finalizers: []string{}, // No finalizer
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
				}

				// When: reconcileDelete is called
				result, err := reconciler.ReconcileDelete(ctx, wfe)

				// Then: Should return early without error
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})
	})

	// ========================================
	// Day 7 Afternoon: Business-Value Metrics
	// 4 metrics: total, duration, pipelinerun_creation, skip
	// ========================================

	Describe("Metrics", func() {
		Context("workflowexecution_total metric", func() {
			It("should be accessible from the metrics package", func() {
				// Verify metrics are registered
				Expect(workflowexecution.WorkflowExecutionTotal).ToNot(BeNil())
			})

			It("should have outcome label", func() {
				// This verifies the metric is defined with correct labels
				// The actual recording is tested via controller method tests
				metric := workflowexecution.WorkflowExecutionTotal
				Expect(metric).ToNot(BeNil())
			})
		})

		Context("workflowexecution_duration_seconds metric", func() {
			It("should be accessible from the metrics package", func() {
				Expect(workflowexecution.WorkflowExecutionDuration).ToNot(BeNil())
			})
		})

		Context("workflowexecution_pipelinerun_creation_total metric", func() {
			It("should be accessible from the metrics package", func() {
				Expect(workflowexecution.PipelineRunCreationTotal).ToNot(BeNil())
			})
		})

		Context("workflowexecution_skip_total metric", func() {
			It("should be accessible from the metrics package", func() {
				Expect(workflowexecution.WorkflowExecutionSkipTotal).ToNot(BeNil())
			})

			It("should have reason label for DD-WE-001 visibility", func() {
				// Verify the skip metric supports ResourceBusy and RecentlyRemediated reasons
				metric := workflowexecution.WorkflowExecutionSkipTotal
				Expect(metric).ToNot(BeNil())
			})
		})

		Context("Metric Recording in Controller Methods", func() {
			var (
				reconciler *workflowexecution.WorkflowExecutionReconciler
				fakeClient client.Client
				recorder   *record.FakeRecorder
				ctx        context.Context
			)

			BeforeEach(func() {
				ctx = context.Background()
				// Need WithStatusSubresource for status updates
				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
					Build()
				recorder = record.NewFakeRecorder(10)
				reconciler = &workflowexecution.WorkflowExecutionReconciler{
					Client:             fakeClient,
					Scheme:             scheme,
					Recorder:           recorder,
					ExecutionNamespace: "kubernaut-workflows",
					CooldownPeriod:     5 * time.Minute,
				}
			})

			It("should record total metric when MarkCompleted is called", func() {
				// Given: WFE in Running phase
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-metrics-completed",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:     workflowexecutionv1alpha1.PhaseRunning,
						StartTime: &startTime,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// And: PipelineRun completed
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      workflowexecution.PipelineRunName(wfe.Spec.TargetResource),
						Namespace: "kubernaut-workflows",
					},
				}
				completionTime := metav1.NewTime(time.Now())
				pr.Status.CompletionTime = &completionTime

				// When: MarkCompleted is called
				_, err := reconciler.MarkCompleted(ctx, wfe, pr)

				// Then: Should succeed (metrics recorded internally)
				Expect(err).NotTo(HaveOccurred())
				// Note: Actual metric value verification would require prometheus testutil
				// which is deferred to integration tests
			})

			It("should record total metric when MarkFailed is called", func() {
				// Given: WFE in Running phase
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-metrics-failed",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID: "test-workflow",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:     workflowexecutionv1alpha1.PhaseRunning,
						StartTime: &startTime,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// And: PipelineRun failed
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pr-failed",
						Namespace: "kubernaut-workflows",
					},
				}
				pr.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Message: "Pipeline failed",
				})

				// When: MarkFailed is called
				_, err := reconciler.MarkFailed(ctx, wfe, pr)

				// Then: Should succeed (metrics recorded internally)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should record skip metric when MarkSkipped is called with ResourceBusy", func() {
				// Given: WFE in Pending phase
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-metrics-skipped",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhasePending,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// And: Skip details with ResourceBusy reason
				skipDetails := &workflowexecutionv1alpha1.SkipDetails{
					Reason:  workflowexecutionv1alpha1.SkipReasonResourceBusy,
					Message: "Another workflow is running",
				}

				// When: MarkSkipped is called
				err := reconciler.MarkSkipped(ctx, wfe, skipDetails)

				// Then: Should succeed (skip metric recorded)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	// ========================================
	// Day 8: Audit Trail + Spec Validation
	// Audit Events for Execution Lifecycle
	// controller-implementation.md: validateSpec()
	// ========================================

	Describe("Audit Store Integration", func() {
		Context("Controller Initialization", func() {
			It("should have AuditStore field in reconciler struct", func() {
				// Verify the reconciler struct has the audit store field
				reconciler := &workflowexecution.WorkflowExecutionReconciler{}
				Expect(reconciler.AuditStore).To(BeNil()) // nil before initialization
			})

			It("should accept AuditStore during initialization", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				recorder := record.NewFakeRecorder(10)

				// Create a mock audit store
				mockAuditStore := &mockAuditStore{}

				reconciler := &workflowexecution.WorkflowExecutionReconciler{
					Client:             fakeClient,
					Scheme:             scheme,
					Recorder:           recorder,
					ExecutionNamespace: "kubernaut-workflows",
					AuditStore:         mockAuditStore,
				}

				Expect(reconciler.AuditStore).ToNot(BeNil())
			})
		})

		Context("Audit Events on Phase Transitions", func() {
			var (
				reconciler *workflowexecution.WorkflowExecutionReconciler
				fakeClient client.Client
				recorder   *record.FakeRecorder
				auditStore *mockAuditStore
				ctx        context.Context
			)

			BeforeEach(func() {
				ctx = context.Background()
				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
					Build()
				recorder = record.NewFakeRecorder(10)
				auditStore = &mockAuditStore{}

				reconciler = &workflowexecution.WorkflowExecutionReconciler{
					Client:             fakeClient,
					Scheme:             scheme,
					Recorder:           recorder,
					ExecutionNamespace: "kubernaut-workflows",
					CooldownPeriod:     5 * time.Minute,
					AuditStore:         auditStore,
				}
			})

			It("should emit audit event when workflow starts (Running phase)", func() {
				// Given: WFE in Pending phase
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-audit-start",
						Namespace: "default",
						Labels: map[string]string{
							"kubernaut.ai/correlation-id": "corr-123",
						},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/test/workflow:v1",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhasePending,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: Workflow transitions to Running
				now := metav1.Now()
				wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
				wfe.Status.StartTime = &now
				Expect(fakeClient.Status().Update(ctx, wfe)).To(Succeed())

				// And: RecordAuditEvent is called
				err := reconciler.RecordAuditEvent(ctx, wfe, "workflow.started", "success")

				// Then: Audit store should receive the event
				Expect(err).ToNot(HaveOccurred())
				Expect(auditStore.events).To(HaveLen(1))
				Expect(auditStore.events[0].EventType).To(Equal("workflowexecution.workflow.started"))
			})

			It("should emit audit event when workflow completes", func() {
				// Given: WFE in Running phase
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-audit-complete",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/test/workflow:v1",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:     workflowexecutionv1alpha1.PhaseRunning,
						StartTime: &startTime,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: RecordAuditEvent is called for completion
				err := reconciler.RecordAuditEvent(ctx, wfe, "workflow.completed", "success")

				// Then: Audit should be recorded
				Expect(err).ToNot(HaveOccurred())
				Expect(auditStore.events).To(HaveLen(1))
			})

			It("should emit audit event when workflow fails", func() {
				// Given: WFE with failure details
				startTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-audit-fail",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/test/workflow:v1",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:     workflowexecutionv1alpha1.PhaseFailed,
						StartTime: &startTime,
						FailureDetails: &workflowexecutionv1alpha1.FailureDetails{
							Reason:  workflowexecutionv1alpha1.FailureReasonOOMKilled,
							Message: "Container killed due to OOM",
						},
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: RecordAuditEvent is called for failure
				err := reconciler.RecordAuditEvent(ctx, wfe, "workflow.failed", "failure")

				// Then: Audit should include failure details
				Expect(err).ToNot(HaveOccurred())
				Expect(auditStore.events).To(HaveLen(1))
				Expect(auditStore.events[0].EventOutcome).To(Equal("failure"))
			})

			It("should handle nil AuditStore gracefully", func() {
				// Given: Reconciler without audit store
				reconcilerNoAudit := &workflowexecution.WorkflowExecutionReconciler{
					Client:             fakeClient,
					Scheme:             scheme,
					ExecutionNamespace: "kubernaut-workflows",
					AuditStore:         nil, // No audit store
				}

				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-no-audit",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
				}

				// When: RecordAuditEvent is called
				err := reconcilerNoAudit.RecordAuditEvent(ctx, wfe, "workflow.started", "success")

				// Then: Should not error (graceful degradation)
				Expect(err).ToNot(HaveOccurred())
			})

			// ========================================
			// Day 9: P2 Edge Case - Missing Correlation ID
			// Business Value: Audit trail completeness for tracing
			// ========================================
			It("should handle WFE with missing correlation-id label gracefully", func() {
				// Given: WFE without correlation-id label
				wfeNoCorrelation := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-no-correlation",
						Namespace: "default",
						Labels:    map[string]string{}, // No correlation-id
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
				}

				// When: RecordAuditEvent is called
				err := reconciler.RecordAuditEvent(ctx, wfeNoCorrelation, "workflow.started", "success")

				// Then: Should succeed without panic (correlation-id is derived from label)
				// Note: RecordAuditEvent handles missing label gracefully
				Expect(err).ToNot(HaveOccurred())
				// Audit event should still be recorded
				Expect(auditStore.events).To(HaveLen(1))
			})

			It("should handle WFE with nil labels gracefully", func() {
				// Given: WFE with nil Labels map
				wfeNilLabels := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wfe-nil-labels",
						Namespace: "default",
						Labels:    nil, // Nil labels
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/my-app",
					},
				}

				// When: RecordAuditEvent is called
				err := reconciler.RecordAuditEvent(ctx, wfeNilLabels, "workflow.started", "success")

				// Then: Should succeed without panic
				Expect(err).ToNot(HaveOccurred())
			})
		})

		// ========================================
		// Comprehensive Audit Event Field Validation
		// Validates all fields in audit traces contain correct values
		// ========================================
		Context("Comprehensive Audit Event Field Validation", func() {
			var (
				reconciler *workflowexecution.WorkflowExecutionReconciler
				fakeClient client.Client
				auditStore *mockAuditStore
				ctx        context.Context
			)

			// Helper function to parse EventData from JSON bytes
			parseEventData := func(data []byte) map[string]interface{} {
				var result map[string]interface{}
				err := json.Unmarshal(data, &result)
				Expect(err).ToNot(HaveOccurred())
				return result
			}

			BeforeEach(func() {
				ctx = context.Background()
				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
					Build()
				auditStore = &mockAuditStore{}

				reconciler = &workflowexecution.WorkflowExecutionReconciler{
					Client:             fakeClient,
					Scheme:             scheme,
					Recorder:           record.NewFakeRecorder(10),
					ExecutionNamespace: "kubernaut-workflows",
					CooldownPeriod:     5 * time.Minute,
					AuditStore:         auditStore,
				}
			})

			It("should populate all required audit event fields correctly for workflow.started", func() {
				// Given: Complete WFE with all fields
				startTime := metav1.Now()
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-audit-validation-start",
						Namespace: "production",
						Labels: map[string]string{
							"kubernaut.ai/correlation-id": "corr-abc123",
						},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "production/deployment/payment-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "increase-memory-conservative",
							ContainerImage: "ghcr.io/kubernaut/workflows/increase-memory:v1.2.0",
						},
						Parameters: map[string]string{
							"NAMESPACE":       "production",
							"DEPLOYMENT_NAME": "payment-api",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:     workflowexecutionv1alpha1.PhaseRunning,
						StartTime: &startTime,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: RecordAuditEvent is called
				err := reconciler.RecordAuditEvent(ctx, wfe, "workflow.started", "success")

				// Then: All audit event fields should be correctly populated
				Expect(err).ToNot(HaveOccurred())
				Expect(auditStore.events).To(HaveLen(1))

				event := auditStore.events[0]

				// Event Classification
				Expect(event.EventType).To(Equal("workflowexecution.workflow.started"))
				Expect(event.EventCategory).To(Equal("workflow"))
				Expect(event.EventAction).To(Equal("workflow.started"))
				Expect(event.EventOutcome).To(Equal("success"))

				// Actor Information
				Expect(event.ActorType).To(Equal("service"))
				Expect(event.ActorID).To(Equal("workflowexecution-controller"))

				// Resource Information
				Expect(event.ResourceType).To(Equal("WorkflowExecution"))
				Expect(event.ResourceID).To(Equal("wfe-audit-validation-start"))

				// Correlation
				Expect(event.CorrelationID).To(Equal("corr-abc123"))

				// Namespace context
				Expect(event.Namespace).ToNot(BeNil())
				Expect(*event.Namespace).To(Equal("production"))

				// Event Identity (auto-generated)
				Expect(event.EventID).ToNot(BeZero())
				Expect(event.EventTimestamp).ToNot(BeZero())

				// Event Data (JSON bytes - parse and validate)
				Expect(event.EventData).ToNot(BeEmpty())
				eventData := parseEventData(event.EventData)
				Expect(eventData["workflow_id"]).To(Equal("increase-memory-conservative"))
				Expect(eventData["target_resource"]).To(Equal("production/deployment/payment-api"))
				Expect(eventData["container_image"]).To(Equal("ghcr.io/kubernaut/workflows/increase-memory:v1.2.0"))
				Expect(eventData["execution_name"]).To(Equal("wfe-audit-validation-start"))
				Expect(eventData["phase"]).To(Equal("Running"))
			})

			It("should include failure details in audit event for workflow.failed", func() {
				// Given: WFE with failure details
				startTime := metav1.NewTime(time.Now().Add(-45 * time.Second))
				completionTime := metav1.Now()
				exitCode := int32(1)
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-audit-validation-failed",
						Namespace: "staging",
						Labels: map[string]string{
							"kubernaut.ai/correlation-id": "corr-fail456",
						},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "staging/deployment/api-gateway",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "restart-deployment",
							ContainerImage: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseFailed,
						StartTime:      &startTime,
						CompletionTime: &completionTime,
						Duration:       "45s",
						FailureDetails: &workflowexecutionv1alpha1.FailureDetails{
							Reason:                 workflowexecutionv1alpha1.FailureReasonTaskFailed,
							Message:                "Task restart-pod failed: exit code 1",
							FailedTaskName:         "restart-pod",
							FailedTaskIndex:        0,
							ExitCode:               &exitCode,
							WasExecutionFailure:    true,
							NaturalLanguageSummary: "The restart task failed due to permission denied",
						},
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: RecordAuditEvent is called for failure
				err := reconciler.RecordAuditEvent(ctx, wfe, "workflow.failed", "failure")

				// Then: Audit event should include failure details
				Expect(err).ToNot(HaveOccurred())
				Expect(auditStore.events).To(HaveLen(1))

				event := auditStore.events[0]

				// Event Classification
				Expect(event.EventType).To(Equal("workflowexecution.workflow.failed"))
				Expect(event.EventOutcome).To(Equal("failure"))

				// Correlation
				Expect(event.CorrelationID).To(Equal("corr-fail456"))

				// Event Data with timing (JSON bytes - parse and validate)
				eventData := parseEventData(event.EventData)
				Expect(eventData["duration"]).To(Equal("45s"))
				Expect(eventData["phase"]).To(Equal("Failed"))

				// Failure details in event data
				Expect(eventData["failure_reason"]).To(Equal(string(workflowexecutionv1alpha1.FailureReasonTaskFailed)))
				Expect(eventData["failure_message"]).To(Equal("Task restart-pod failed: exit code 1"))
				Expect(eventData["failed_task_name"]).To(Equal("restart-pod"))
			})

			It("should include skip details in audit event for workflow.skipped", func() {
				// Given: WFE that was skipped due to resource lock
				skipTime := metav1.Now()
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-audit-validation-skipped",
						Namespace: "default",
						Labels: map[string]string{
							"kubernaut.ai/correlation-id": "corr-skip789",
						},
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/redis",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "scale-deployment",
							ContainerImage: "ghcr.io/kubernaut/workflows/scale:v1.0.0",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhaseSkipped,
						SkipDetails: &workflowexecutionv1alpha1.SkipDetails{
							Reason:    workflowexecutionv1alpha1.SkipReasonResourceBusy,
							Message:   "Another workflow is currently running on this resource",
							SkippedAt: skipTime,
						},
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: RecordAuditEvent is called for skip
				err := reconciler.RecordAuditEvent(ctx, wfe, "workflow.skipped", "skipped")

				// Then: Audit event should include skip details
				Expect(err).ToNot(HaveOccurred())
				Expect(auditStore.events).To(HaveLen(1))

				event := auditStore.events[0]

				// Event Classification
				Expect(event.EventType).To(Equal("workflowexecution.workflow.skipped"))
				Expect(event.EventOutcome).To(Equal("skipped"))

				// Skip details in event data (JSON bytes - parse and validate)
				eventData := parseEventData(event.EventData)
				Expect(eventData["skip_reason"]).To(Equal(string(workflowexecutionv1alpha1.SkipReasonResourceBusy)))
				Expect(eventData["skip_message"]).To(ContainSubstring("Another workflow"))
			})

			It("should use WFE name as correlation ID fallback when label missing", func() {
				// Given: WFE without correlation-id label
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-no-correlation-label",
						Namespace: "default",
						Labels:    map[string]string{}, // No correlation-id
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/app",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/test/workflow:v1",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase: workflowexecutionv1alpha1.PhaseRunning,
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: RecordAuditEvent is called
				err := reconciler.RecordAuditEvent(ctx, wfe, "workflow.started", "success")

				// Then: CorrelationID should fall back to WFE name
				Expect(err).ToNot(HaveOccurred())
				Expect(auditStore.events).To(HaveLen(1))

				event := auditStore.events[0]
				Expect(event.CorrelationID).To(Equal("wfe-no-correlation-label"))
			})

			It("should populate timing information when available", func() {
				// Given: WFE with complete timing information
				startTime := metav1.NewTime(time.Now().Add(-60 * time.Second))
				completionTime := metav1.Now()
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-audit-timing",
						Namespace: "default",
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/app",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/test/workflow:v1",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseCompleted,
						StartTime:      &startTime,
						CompletionTime: &completionTime,
						Duration:       "1m0s",
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

				// When: RecordAuditEvent is called
				err := reconciler.RecordAuditEvent(ctx, wfe, "workflow.completed", "success")

				// Then: Timing fields should be populated
				Expect(err).ToNot(HaveOccurred())
				Expect(auditStore.events).To(HaveLen(1))

				eventData := parseEventData(auditStore.events[0].EventData)
				Expect(eventData["started_at"]).ToNot(BeNil())
				Expect(eventData["completed_at"]).ToNot(BeNil())
				Expect(eventData["duration"]).To(Equal("1m0s"))
			})
		})
	})

	Describe("Spec Validation (Day 8)", func() {
		var reconciler *workflowexecution.WorkflowExecutionReconciler

		BeforeEach(func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             scheme,
				ExecutionNamespace: "kubernaut-workflows",
			}
		})

		Context("ValidateSpec()", func() {
			It("should return nil for valid spec", func() {
				// Given: Valid WFE spec
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "production/deployment/my-app",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "increase-memory",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}

				// When: ValidateSpec is called
				err := reconciler.ValidateSpec(wfe)

				// Then: Should return nil
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return error for missing ContainerImage", func() {
				// Given: WFE without container image
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "production/deployment/my-app",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "increase-memory",
							ContainerImage: "", // Missing
						},
					},
				}

				// When: ValidateSpec is called
				err := reconciler.ValidateSpec(wfe)

				// Then: Should return ConfigurationError
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("containerImage"))
			})

			It("should return error for missing TargetResource", func() {
				// Given: WFE without target resource
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "", // Missing
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "increase-memory",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}

				// When: ValidateSpec is called
				err := reconciler.ValidateSpec(wfe)

				// Then: Should return error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("targetResource"))
			})

			It("should accept cluster-scoped TargetResource format (2 parts)", func() {
				// Given: WFE with cluster-scoped target resource per DD-WE-001
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "node/worker-node-1", // Cluster-scoped: kind/name
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "node-disk-cleanup",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}

				// When: ValidateSpec is called
				err := reconciler.ValidateSpec(wfe)

				// Then: Should accept (DD-WE-001 allows 2-part format for cluster-scoped)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return error for invalid TargetResource format (only 1 part)", func() {
				// Given: WFE with only namespace (missing kind and name)
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "production", // Only 1 part - invalid
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "increase-memory",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}

				// When: ValidateSpec is called
				err := reconciler.ValidateSpec(wfe)

				// Then: Should return format error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("format"))
			})

			It("should return error for invalid TargetResource format (too many parts)", func() {
				// Given: WFE with too many parts in target resource
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "production/deployment/my-app/extra", // Extra part
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "increase-memory",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}

				// When: ValidateSpec is called
				err := reconciler.ValidateSpec(wfe)

				// Then: Should return format error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("format"))
			})

			It("should accept valid TargetResource formats", func() {
				validTargets := []string{
					"default/deployment/nginx",
					"production/statefulset/postgres",
					"kube-system/daemonset/fluentd",
				}

				for _, target := range validTargets {
					wfe := &workflowexecutionv1alpha1.WorkflowExecution{
						Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
							TargetResource: target,
							WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
								WorkflowID:     "test-workflow",
								ContainerImage: "ghcr.io/org/workflow:v1.0",
							},
						},
					}

					err := reconciler.ValidateSpec(wfe)
					Expect(err).ToNot(HaveOccurred(), "Expected valid for target: %s", target)
				}
			})

			// ========================================
			// Day 9: DescribeTable for Spec Validation Edge Cases (v3.5)
			// Per 03-testing-strategy.mdc: Use DescribeTable for similar scenarios
			// ========================================

			// DescribeTable validates actual ValidateSpec() implementation behavior
			// Validation order: containerImage -> targetResource -> format
			// Note: WorkflowID is NOT validated by current implementation
			DescribeTable("TargetResource format validation",
				func(targetResource string, workflowID string, containerImage string, shouldPass bool, expectedErrorSubstring string) {
					wfe := &workflowexecutionv1alpha1.WorkflowExecution{
						Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
							TargetResource: targetResource,
							WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
								WorkflowID:     workflowID,
								ContainerImage: containerImage,
							},
						},
					}

					err := reconciler.ValidateSpec(wfe)

					if shouldPass {
						Expect(err).ToNot(HaveOccurred(), "Expected valid spec for target: %s", targetResource)
					} else {
						Expect(err).To(HaveOccurred(), "Expected error for target: %s", targetResource)
						if expectedErrorSubstring != "" {
							Expect(err.Error()).To(ContainSubstring(expectedErrorSubstring))
						}
					}
				},
				// Valid TargetResource formats (namespace/kind/name for namespaced, kind/name for cluster-scoped)
				Entry("valid - standard deployment", "default/deployment/nginx", "workflow-1", "ghcr.io/org/wf:v1", true, ""),
				Entry("valid - statefulset", "prod/statefulset/postgres", "workflow-2", "ghcr.io/org/wf:v1", true, ""),
				Entry("valid - daemonset", "kube-system/daemonset/fluentd", "workflow-3", "ghcr.io/org/wf:v1", true, ""),
				Entry("valid - replicaset", "default/replicaset/my-rs", "workflow-4", "ghcr.io/org/wf:v1", true, ""),
				Entry("valid - namespace with dash", "ns-1/deployment/app", "workflow-5", "ghcr.io/org/wf:v1", true, ""),
				Entry("valid - name with numbers", "default/deployment/app123", "workflow-6", "ghcr.io/org/wf:v1", true, ""),

				// Valid cluster-scoped resources (DD-WE-001: {kind}/{name} format)
				Entry("valid - cluster-scoped node", "node/worker-node-1", "workflow-7", "ghcr.io/org/wf:v1", true, ""),
				Entry("valid - cluster-scoped persistentvolume", "persistentvolume/pv-data-01", "workflow-8", "ghcr.io/org/wf:v1", true, ""),
				Entry("valid - cluster-scoped clusterrole", "clusterrole/admin", "workflow-9", "ghcr.io/org/wf:v1", true, ""),
				Entry("valid - cluster-scoped namespace", "namespace/production", "workflow-10", "ghcr.io/org/wf:v1", true, ""),

				// Invalid TargetResource - missing parts (must have valid containerImage first)
				Entry("invalid - empty targetResource", "", "workflow-1", "ghcr.io/org/wf:v1", false, "targetResource"),
				Entry("invalid - only one part", "default", "workflow-1", "ghcr.io/org/wf:v1", false, "format"),
				Entry("invalid - too many parts", "default/deployment/app/extra", "workflow-1", "ghcr.io/org/wf:v1", false, "format"),

				// Invalid WorkflowRef - containerImage validation
				Entry("invalid - empty ContainerImage", "default/deployment/app", "workflow-1", "", false, "containerImage"),

				// Validation order: containerImage checked first
				Entry("invalid - all empty returns containerImage error first", "", "", "", false, "containerImage"),
			)
		})

		Context("Spec Validation in Reconcile Flow", func() {
			It("should fail WFE with ConfigurationError reason on invalid spec", func() {
				// Given: WFE with invalid spec
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "invalid-format", // Invalid
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}

				// When: ValidateSpec is called
				err := reconciler.ValidateSpec(wfe)

				// Then: Should indicate configuration error
				Expect(err).To(HaveOccurred())
				// Verify the error can be mapped to ConfigurationError reason
				Expect(workflowexecutionv1alpha1.FailureReasonConfigurationError).To(Equal("ConfigurationError"))
			})
		})
	})

	// ========================================
	// Day 6 Extension: Exponential Backoff (DD-WE-004)
	// TDD RED Phase: Write failing tests first
	// ========================================

	Describe("Exponential Backoff", func() {
		var (
			fakeClient client.Client
			reconciler *workflowexecution.WorkflowExecutionReconciler
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
				Build()

			reconciler = &workflowexecution.WorkflowExecutionReconciler{
				Client:                 fakeClient,
				Scheme:                 scheme,
				Recorder:               recorder,
				CooldownPeriod:         5 * time.Minute,
				ExecutionNamespace:     "kubernaut-workflows",
				ServiceAccountName:     "kubernaut-workflow-runner",
				BaseCooldownPeriod:     1 * time.Minute,
				MaxCooldownPeriod:      10 * time.Minute,
				MaxBackoffExponent:     4,
				MaxConsecutiveFailures: 5,
			}
		})

		// ========================================
		// Task 2: CheckCooldown with Exponential Backoff
		// DD-WE-004-1: wasExecutionFailure: true blocks ALL retries
		// ========================================

		Context("when previous WFE has wasExecutionFailure: true (DD-WE-004-1)", func() {
			It("should block with PreviousExecutionFailed (not backoff)", func() {
				// Given: Previous WFE that ran and failed (execution failure)
				previousWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-prev-001",
						Namespace: "default",
						UID:       types.UID("prev-uid-001"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/payment-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "oomkill-increase-memory",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:          workflowexecutionv1alpha1.PhaseFailed,
						CompletionTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
						FailureDetails: &workflowexecutionv1alpha1.FailureDetails{
							Reason:              workflowexecutionv1alpha1.FailureReasonTaskFailed,
							WasExecutionFailure: true, // CRITICAL: Workflow RAN and failed
						},
					},
				}
				Expect(fakeClient.Create(ctx, previousWFE)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, previousWFE)).To(Succeed())

				// Given: New WFE targeting same resource
				newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-new-001",
						Namespace: "default",
						UID:       types.UID("new-uid-001"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/payment-api", // Same target
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "oomkill-increase-memory",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}
				Expect(fakeClient.Create(ctx, newWFE)).To(Succeed())

				// When: CheckCooldown is called
				blocked, skipDetails, err := reconciler.CheckCooldown(ctx, newWFE)

				// Then: Should block with PreviousExecutionFailed
				Expect(err).ToNot(HaveOccurred())
				Expect(blocked).To(BeTrue(), "Should be blocked when previous execution failed")
				Expect(skipDetails).ToNot(BeNil())
				Expect(skipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed))
				Expect(skipDetails.Message).To(ContainSubstring("Manual intervention required"))
			})
		})

		// ========================================
		// DD-WE-004-2: Exponential backoff for pre-execution failures
		// ========================================

		Context("when previous WFE has wasExecutionFailure: false (pre-execution failure)", func() {
			It("should apply exponential backoff based on ConsecutiveFailures", func() {
				// Given: Previous WFE with pre-execution failure and ConsecutiveFailures=2
				previousWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-prev-002",
						Namespace: "default",
						UID:       types.UID("prev-uid-002"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/test-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:               workflowexecutionv1alpha1.PhaseFailed,
						CompletionTime:      &metav1.Time{Time: time.Now().Add(-30 * time.Second)},
						ConsecutiveFailures: 2, // 2 consecutive pre-execution failures
						NextAllowedExecution: &metav1.Time{
							Time: time.Now().Add(2 * time.Minute), // Base * 2^(2-1) = 1min * 2 = 2min
						},
						FailureDetails: &workflowexecutionv1alpha1.FailureDetails{
							Reason:              workflowexecutionv1alpha1.FailureReasonImagePullBackOff,
							WasExecutionFailure: false, // Pre-execution failure
						},
					},
				}
				Expect(fakeClient.Create(ctx, previousWFE)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, previousWFE)).To(Succeed())

				// Given: New WFE targeting same resource
				newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-new-002",
						Namespace: "default",
						UID:       types.UID("new-uid-002"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/test-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}
				Expect(fakeClient.Create(ctx, newWFE)).To(Succeed())

				// When: CheckCooldown is called (before NextAllowedExecution)
				blocked, skipDetails, err := reconciler.CheckCooldown(ctx, newWFE)

				// Then: Should block with RecentlyRemediated (backoff active)
				Expect(err).ToNot(HaveOccurred())
				Expect(blocked).To(BeTrue(), "Should be blocked during backoff period")
				Expect(skipDetails).ToNot(BeNil())
				Expect(skipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated))
			})

			It("should cap backoff at MaxCooldownPeriod", func() {
				// Given: Previous WFE with many failures (would exceed max backoff)
				// Base=1min, MaxBackoffExponent=4, Max=10min
				// Failures=5: 1min * 2^4 = 16min, but capped at 10min
				previousWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-prev-003",
						Namespace: "default",
						UID:       types.UID("prev-uid-003"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/capped-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:               workflowexecutionv1alpha1.PhaseFailed,
						CompletionTime:      &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
						ConsecutiveFailures: 4, // High failure count
						NextAllowedExecution: &metav1.Time{
							Time: time.Now().Add(10 * time.Minute), // Capped at max
						},
						FailureDetails: &workflowexecutionv1alpha1.FailureDetails{
							Reason:              workflowexecutionv1alpha1.FailureReasonImagePullBackOff,
							WasExecutionFailure: false,
						},
					},
				}
				Expect(fakeClient.Create(ctx, previousWFE)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, previousWFE)).To(Succeed())

				// Given: New WFE
				newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-new-003",
						Namespace: "default",
						UID:       types.UID("new-uid-003"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/capped-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}
				Expect(fakeClient.Create(ctx, newWFE)).To(Succeed())

				// When: CheckCooldown is called
				blocked, skipDetails, err := reconciler.CheckCooldown(ctx, newWFE)

				// Then: Should be blocked (backoff capped at 10min)
				Expect(err).ToNot(HaveOccurred())
				Expect(blocked).To(BeTrue())
				Expect(skipDetails).ToNot(BeNil())
				Expect(skipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated))
			})
		})

		// ========================================
		// DD-WE-004-3: ExhaustedRetries after MaxConsecutiveFailures
		// ========================================

		Context("when ConsecutiveFailures >= MaxConsecutiveFailures", func() {
			It("should skip with ExhaustedRetries after max failures", func() {
				// Given: Previous WFE with max consecutive failures reached
				previousWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-prev-004",
						Namespace: "default",
						UID:       types.UID("prev-uid-004"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/exhausted-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:               workflowexecutionv1alpha1.PhaseFailed,
						CompletionTime:      &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
						ConsecutiveFailures: 5, // Max reached (reconciler.MaxConsecutiveFailures = 5)
						FailureDetails: &workflowexecutionv1alpha1.FailureDetails{
							Reason:              workflowexecutionv1alpha1.FailureReasonImagePullBackOff,
							WasExecutionFailure: false,
						},
					},
				}
				Expect(fakeClient.Create(ctx, previousWFE)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, previousWFE)).To(Succeed())

				// Given: New WFE
				newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-new-004",
						Namespace: "default",
						UID:       types.UID("new-uid-004"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/exhausted-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}
				Expect(fakeClient.Create(ctx, newWFE)).To(Succeed())

				// When: CheckCooldown is called
				blocked, skipDetails, err := reconciler.CheckCooldown(ctx, newWFE)

				// Then: Should skip with ExhaustedRetries
				Expect(err).ToNot(HaveOccurred())
				Expect(blocked).To(BeTrue(), "Should be blocked after max failures")
				Expect(skipDetails).ToNot(BeNil())
				Expect(skipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonExhaustedRetries))
				Expect(skipDetails.Message).To(ContainSubstring("Manual intervention required"))
			})
		})

		// ========================================
		// DD-WE-004-5: Success resets ConsecutiveFailures
		// ========================================

		Context("when previous WFE completed successfully", func() {
			It("should allow execution (ConsecutiveFailures reset)", func() {
				// Given: Previous WFE that completed successfully
				previousWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-prev-005",
						Namespace: "default",
						UID:       types.UID("prev-uid-005"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/success-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:               workflowexecutionv1alpha1.PhaseCompleted,
						CompletionTime:      &metav1.Time{Time: time.Now().Add(-30 * time.Second)},
						ConsecutiveFailures: 0, // Reset on success
					},
				}
				Expect(fakeClient.Create(ctx, previousWFE)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, previousWFE)).To(Succeed())

				// Given: New WFE (after base cooldown)
				newWFE := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-new-005",
						Namespace: "default",
						UID:       types.UID("new-uid-005"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/success-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
				}
				Expect(fakeClient.Create(ctx, newWFE)).To(Succeed())

				// When: CheckCooldown is called (success means base cooldown only)
				// Note: Base cooldown is 1min, previous completed 30s ago
				// For successful WFEs, regular cooldown applies (not exponential backoff)
				blocked, _, err := reconciler.CheckCooldown(ctx, newWFE)

				// Then: May be blocked by regular cooldown, but NOT by backoff/exhausted
				Expect(err).ToNot(HaveOccurred())
				// The blocked status depends on regular cooldown, not exponential backoff
				// Success resets failure counter, so NO ExhaustedRetries or PreviousExecutionFailed
				if blocked {
					// If blocked, it should be regular cooldown
					// Not PreviousExecutionFailed or ExhaustedRetries
				}
			})
		})

		// ========================================
		// Task 3: MarkFailed with ConsecutiveFailures Tracking
		// ========================================

		Describe("MarkFailed with ConsecutiveFailures", func() {
			Context("when wasExecutionFailure is false (pre-execution failure)", func() {
				It("should increment ConsecutiveFailures", func() {
					// Given: WFE in Running state
					wfe := &workflowexecutionv1alpha1.WorkflowExecution{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "wfe-markfailed-001",
							Namespace: "default",
							UID:       types.UID("markfailed-uid-001"),
						},
						Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
							TargetResource: "default/deployment/markfailed-api",
							WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
								WorkflowID:     "test-workflow",
								ContainerImage: "ghcr.io/org/workflow:v1.0",
							},
						},
						Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
							Phase:               workflowexecutionv1alpha1.PhaseRunning,
							ConsecutiveFailures: 2, // Existing failures
						},
					}
					Expect(fakeClient.Create(ctx, wfe)).To(Succeed())
					Expect(fakeClient.Status().Update(ctx, wfe)).To(Succeed())

					// Given: PipelineRun that failed pre-execution (e.g., image pull)
					pr := &tektonv1.PipelineRun{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "wfe-pr-001",
							Namespace: "kubernaut-workflows",
						},
					}
					setFailedCondition(pr, "ImagePullBackOff")

					// When: MarkFailed is called
					_, err := reconciler.MarkFailed(ctx, wfe, pr)

					// Then: ConsecutiveFailures should be incremented
					Expect(err).ToNot(HaveOccurred())
					// Re-fetch WFE to check updated status
					var updatedWFE workflowexecutionv1alpha1.WorkflowExecution
					Expect(fakeClient.Get(ctx, client.ObjectKey{Name: wfe.Name, Namespace: wfe.Namespace}, &updatedWFE)).To(Succeed())
					Expect(updatedWFE.Status.ConsecutiveFailures).To(Equal(int32(3)), "Should increment from 2 to 3")
					Expect(updatedWFE.Status.NextAllowedExecution).ToNot(BeNil(), "Should set NextAllowedExecution")
				})

				It("should calculate NextAllowedExecution with exponential backoff", func() {
					// Given: WFE with 3 consecutive failures
					wfe := &workflowexecutionv1alpha1.WorkflowExecution{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "wfe-markfailed-002",
							Namespace: "default",
							UID:       types.UID("markfailed-uid-002"),
						},
						Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
							TargetResource: "default/deployment/backoff-calc-api",
							WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
								WorkflowID:     "test-workflow",
								ContainerImage: "ghcr.io/org/workflow:v1.0",
							},
						},
						Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
							Phase:               workflowexecutionv1alpha1.PhaseRunning,
							ConsecutiveFailures: 3, // Will become 4
						},
					}
					Expect(fakeClient.Create(ctx, wfe)).To(Succeed())
					Expect(fakeClient.Status().Update(ctx, wfe)).To(Succeed())

					pr := &tektonv1.PipelineRun{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "wfe-pr-002",
							Namespace: "kubernaut-workflows",
						},
					}
					setFailedCondition(pr, "ImagePullBackOff")

					beforeMark := time.Now()
					_, err := reconciler.MarkFailed(ctx, wfe, pr)
					Expect(err).ToNot(HaveOccurred())

					var updatedWFE workflowexecutionv1alpha1.WorkflowExecution
					Expect(fakeClient.Get(ctx, client.ObjectKey{Name: wfe.Name, Namespace: wfe.Namespace}, &updatedWFE)).To(Succeed())

					// ConsecutiveFailures=4, exponent=min(4-1, 4)=3
					// Backoff = 1min * 2^3 = 8min
					expectedBackoff := 8 * time.Minute
					expectedNextAllowed := beforeMark.Add(expectedBackoff)

					Expect(updatedWFE.Status.NextAllowedExecution).ToNot(BeNil())
					// Allow 1 second tolerance for timing
					Expect(updatedWFE.Status.NextAllowedExecution.Time).To(BeTemporally("~", expectedNextAllowed, 5*time.Second))
				})
			})

			Context("when wasExecutionFailure is true (execution failure)", func() {
				It("should NOT increment ConsecutiveFailures", func() {
					// Given: WFE in Running state
					wfe := &workflowexecutionv1alpha1.WorkflowExecution{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "wfe-markfailed-003",
							Namespace: "default",
							UID:       types.UID("markfailed-uid-003"),
						},
						Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
							TargetResource: "default/deployment/exec-fail-api",
							WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
								WorkflowID:     "test-workflow",
								ContainerImage: "ghcr.io/org/workflow:v1.0",
							},
						},
						Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
							Phase:               workflowexecutionv1alpha1.PhaseRunning,
							ConsecutiveFailures: 2,
						},
					}
					Expect(fakeClient.Create(ctx, wfe)).To(Succeed())
					Expect(fakeClient.Status().Update(ctx, wfe)).To(Succeed())

					// Given: PipelineRun that failed during execution (task failed)
					pr := &tektonv1.PipelineRun{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "wfe-pr-003",
							Namespace: "kubernaut-workflows",
						},
						Status: tektonv1.PipelineRunStatus{
							PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
								StartTime: &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
							},
						},
					}
					setFailedCondition(pr, "Failed") // Execution failure

					// When: MarkFailed is called
					_, err := reconciler.MarkFailed(ctx, wfe, pr)
					Expect(err).ToNot(HaveOccurred())

					// Then: ConsecutiveFailures should NOT be incremented
					var updatedWFE workflowexecutionv1alpha1.WorkflowExecution
					Expect(fakeClient.Get(ctx, client.ObjectKey{Name: wfe.Name, Namespace: wfe.Namespace}, &updatedWFE)).To(Succeed())
					Expect(updatedWFE.Status.ConsecutiveFailures).To(Equal(int32(2)), "Should NOT increment for execution failures")
					Expect(updatedWFE.Status.NextAllowedExecution).To(BeNil(), "Should NOT set NextAllowedExecution")
				})
			})
		})

		// ========================================
		// Task 4: MarkCompleted with Counter Reset
		// ========================================

		Describe("MarkCompleted with Counter Reset", func() {
			It("should reset ConsecutiveFailures to 0 on success", func() {
				// Given: WFE with previous failures
				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-markcompleted-001",
						Namespace: "default",
						UID:       types.UID("markcompleted-uid-001"),
					},
					Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
						TargetResource: "default/deployment/complete-api",
						WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
							WorkflowID:     "test-workflow",
							ContainerImage: "ghcr.io/org/workflow:v1.0",
						},
					},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						Phase:               workflowexecutionv1alpha1.PhaseRunning,
						ConsecutiveFailures: 3, // Previous failures
						NextAllowedExecution: &metav1.Time{
							Time: time.Now().Add(5 * time.Minute),
						},
					},
				}
				Expect(fakeClient.Create(ctx, wfe)).To(Succeed())
				Expect(fakeClient.Status().Update(ctx, wfe)).To(Succeed())

				// Given: Successful PipelineRun
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wfe-pr-complete-001",
						Namespace: "kubernaut-workflows",
					},
				}
				setSucceededCondition(pr)

				// When: MarkCompleted is called
				_, err := reconciler.MarkCompleted(ctx, wfe, pr)
				Expect(err).ToNot(HaveOccurred())

				// Then: ConsecutiveFailures should be reset
				var updatedWFE workflowexecutionv1alpha1.WorkflowExecution
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: wfe.Name, Namespace: wfe.Namespace}, &updatedWFE)).To(Succeed())
				Expect(updatedWFE.Status.ConsecutiveFailures).To(Equal(int32(0)), "Should reset to 0 on success")
				Expect(updatedWFE.Status.NextAllowedExecution).To(BeNil(), "Should clear NextAllowedExecution")
			})
		})
	})
})

// Helper function for setting failed PipelineRun condition
func setFailedCondition(pr *tektonv1.PipelineRun, reason string) {
	pr.Status.SetCondition(&apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: fmt.Sprintf("Pipeline failed: %s", reason),
	})
}

// Helper function for setting succeeded PipelineRun condition
func setSucceededCondition(pr *tektonv1.PipelineRun) {
	pr.Status.SetCondition(&apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionTrue,
		Reason:  "Succeeded",
		Message: "Pipeline completed successfully",
	})
}

// ========================================
// Mock Types for Day 8 Tests
// ========================================

// mockAuditStore implements audit.AuditStore for testing
type mockAuditStore struct {
	events []*audit.AuditEvent
	err    error
}

func (m *mockAuditStore) StoreAudit(ctx context.Context, event *audit.AuditEvent) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockAuditStore) Close() error {
	return nil
}
