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
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// BR-WE-004: Failure Details Actionable - Tekton Failure Reason Classification
// ========================================
//
// Test Strategy:
// These integration tests validate `mapTektonReasonToFailureReason` and
// `determineWasExecutionFailure` functions by simulating various Tekton
// PipelineRun failure scenarios.
//
// Coverage Goal:
// - `mapTektonReasonToFailureReason`: 45.5% → 80%+ (all 8 reasons)
// - `determineWasExecutionFailure`: 45.5% → 80%+ (execution vs pre-execution)
//
// Test Approach:
// 1. Create WorkflowExecution CRD
// 2. Wait for controller to create PipelineRun
// 3. Update PipelineRun status to simulate specific failure
// 4. Verify WFE.Status.FailureDetails.Reason is correctly mapped
// 5. Verify WFE.Status.FailureDetails.WasExecutionFailure is correct

var _ = Describe("BR-WE-004: Tekton Failure Reason Classification", Ordered, func() {
	var (
		testCtx   context.Context
		namespace string
	)

	BeforeAll(func() {
		testCtx = context.Background()
		namespace = WorkflowExecutionNS
	})

	// ========================================
	// Test 1: TaskFailed - EXECUTION FAILURE
	// ========================================
	Context("TaskFailed: Task execution failed", func() {
		It("should map TaskFailed reason correctly", func() {
			testFailureClassification(testCtx, namespace,
				"taskfailed",
				"TaskRunFailed",
				"task failed with exit code 1",
				true, // execution started
				workflowexecutionv1alpha1.FailureReasonTaskFailed,
				true, // is execution failure
			)
		})
	})

	// ========================================
	// Test 2: OOMKilled - EXECUTION FAILURE
	// ========================================
	Context("OOMKilled: Out of memory", func() {
		It("should map OOMKilled reason correctly", func() {
			testFailureClassification(testCtx, namespace,
				"oomkilled",
				"TaskRunFailed",
				"task failed: OOMKilled",
				true, // execution started
				workflowexecutionv1alpha1.FailureReasonOOMKilled,
				true, // is execution failure
			)
		})
	})

	// ========================================
	// Test 3: DeadlineExceeded - EXECUTION FAILURE
	// ========================================
	Context("DeadlineExceeded: Timeout", func() {
		It("should map timeout reason correctly", func() {
			testFailureClassification(testCtx, namespace,
				"timeout",
				"PipelineRunTimeout",
				"PipelineRun exceeded timeout of 10 minutes",
				true, // execution started
				workflowexecutionv1alpha1.FailureReasonDeadlineExceeded,
				true, // is execution failure
			)
		})
	})

	// ========================================
	// Test 4: Forbidden - EXECUTION FAILURE
	// ========================================
	Context("Forbidden: RBAC denied", func() {
		It("should map forbidden reason correctly", func() {
			testFailureClassification(testCtx, namespace,
				"forbidden",
				"TaskRunFailed",
				"task failed: pods is forbidden: User cannot create resource",
				true, // execution started (RBAC checked during task execution)
				workflowexecutionv1alpha1.FailureReasonForbidden,
				true, // is execution failure
			)
		})
	})

	// ========================================
	// Test 5: ResourceExhausted - PRE-EXECUTION FAILURE
	// ========================================
	Context("ResourceExhausted: Quota exceeded", func() {
		It("should map resource exhausted reason correctly", func() {
			testFailureClassification(testCtx, namespace,
				"quota",
				"TaskRunCreationFailed",
				"exceeded quota: compute resources",
				false, // pre-execution (quota check before pod creation)
				workflowexecutionv1alpha1.FailureReasonResourceExhausted,
				false, // is NOT execution failure
			)
		})
	})

	// ========================================
	// Test 6: ConfigurationError - PRE-EXECUTION FAILURE
	// ========================================
	Context("ConfigurationError: Invalid configuration", func() {
		It("should map configuration error reason correctly", func() {
			testFailureClassification(testCtx, namespace,
				"config",
				"TaskRunValidationFailed",
				"invalid configuration: missing required parameter",
				false, // pre-execution (validation before pod creation)
				workflowexecutionv1alpha1.FailureReasonConfigurationError,
				false, // is NOT execution failure
			)
		})
	})

	// ========================================
	// Test 7: ImagePullBackOff - PRE-EXECUTION FAILURE
	// ========================================
	Context("ImagePullBackOff: Image pull failed", func() {
		It("should map image pull failure reason correctly", func() {
			testFailureClassification(testCtx, namespace,
				"imagepull",
				"TaskRunImagePullFailed",
				"Failed to pull image: image pull backoff",
				false, // pre-execution (image pull before container start)
				workflowexecutionv1alpha1.FailureReasonImagePullBackOff,
				false, // is NOT execution failure
			)
		})
	})

	// ========================================
	// Test 8: Unknown - Generic failure
	// ========================================
	Context("Unknown: Generic failure", func() {
		It("should map unknown failure reason correctly", func() {
			testFailureClassification(testCtx, namespace,
				"unknown",
				"PipelineRunFailed",
				"unexpected error occurred",
				false, // pre-execution (generic error, no task execution)
				workflowexecutionv1alpha1.FailureReasonUnknown,
				false, // is NOT execution failure
			)
		})
	})
})

// ========================================
// Helper Functions
// ========================================

// createMinimalWorkflowExecution creates a minimal WorkflowExecution for testing
func createMinimalWorkflowExecution(name, namespace string) *workflowexecutionv1alpha1.WorkflowExecution {
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1, // K8s increments on create/update
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
				Kind:       "RemediationRequest",
				Name:       "test-rr-" + name,
				Namespace:  namespace,
			},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:     "test-workflow",
				Version:        "v1.0.0",
				ContainerImage: "quay.io/jordigilh/test-workflows/test:v1.0.0",
			},
			TargetResource: "default/deployment/test-app-" + name, // Unique per test for deterministic PR names
			ExecutionEngine: "tekton",
		},
	}
}

// updatePipelineRunStatus updates an existing PipelineRun status to simulate failure
func updatePipelineRunStatus(ctx context.Context, pr *tektonv1.PipelineRun, reason, message string, executionStarted bool) {
	// Create failure condition
	condition := apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}

	// Update status
	pr.Status.Conditions = duckv1.Conditions{condition}

	// If execution started, add StartTime
	if executionStarted {
		now := metav1.Now()
		pr.Status.StartTime = &now
	}

	// Update the PipelineRun status
	Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())
}

// testFailureClassification is a helper that encapsulates the common test pattern
func testFailureClassification(ctx context.Context, namespace, testSuffix, tektonReason, tektonMessage string, executionStarted bool, expectedReason string, expectedWasExecution bool) {
	testName := fmt.Sprintf("test-%s-%s", testSuffix, generateRandomString(8))
	wfe := createMinimalWorkflowExecution(testName, namespace)

	By("Creating WorkflowExecution")
	Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
	DeferCleanup(func() {
		_ = k8sClient.Delete(ctx, wfe)
	})

	By("Waiting for controller to create PipelineRun")
	var pr *tektonv1.PipelineRun
	Eventually(func(g Gomega) {
		updated := &workflowexecutionv1alpha1.WorkflowExecution{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: testName, Namespace: namespace}, updated)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.Status.ExecutionRef).ToNot(BeNil())
		g.Expect(updated.Status.ExecutionRef.Name).ToNot(BeEmpty())

		// Get the created PipelineRun
		pr = &tektonv1.PipelineRun{}
		prKey := types.NamespacedName{Name: updated.Status.ExecutionRef.Name, Namespace: WorkflowExecutionNS}
		g.Expect(k8sClient.Get(ctx, prKey, pr)).To(Succeed())
	}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

	By(fmt.Sprintf("Updating PipelineRun status to simulate %s", expectedReason))
	updatePipelineRunStatus(ctx, pr, tektonReason, tektonMessage, executionStarted)

	By("Waiting for WorkflowExecution to reconcile failure")
	Eventually(func(g Gomega) {
		updated := &workflowexecutionv1alpha1.WorkflowExecution{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: testName, Namespace: namespace}, updated)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
		g.Expect(updated.Status.FailureDetails).ToNot(BeNil())
	}, 30*time.Second, 1*time.Second).Should(Succeed())

	By("Verifying failure classification")
	final := &workflowexecutionv1alpha1.WorkflowExecution{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testName, Namespace: namespace}, final)).To(Succeed())

	Expect(final.Status.FailureDetails.Reason).To(Equal(expectedReason),
		fmt.Sprintf("%s should map to %s", tektonReason, expectedReason))
	Expect(final.Status.FailureDetails.WasExecutionFailure).To(Equal(expectedWasExecution),
		fmt.Sprintf("%s execution failure status should be %v", expectedReason, expectedWasExecution))
}

// generateRandomString generates a random string for unique test names
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
