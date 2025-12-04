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
	"crypto/sha256"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"knative.dev/pkg/apis"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("Integration: Complete Workflow Lifecycle", func() {
	var (
		wfeName        string
		namespace      string
		targetResource string
	)

	BeforeEach(func() {
		wfeName = "test-lifecycle-" + randomString(5)
		namespace = "kubernaut-test"
		targetResource = "production/deployment/app-" + randomString(5)
	})

	AfterEach(func() {
		// Cleanup WFE
		wfe := &workflowexecutionv1.WorkflowExecution{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, wfe); err == nil {
			_ = k8sClient.Delete(ctx, wfe)
		}

		// Cleanup PipelineRun
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr); err == nil {
			_ = k8sClient.Delete(ctx, pr)
		}
	})

	It("should transition from Pending to Running when PipelineRun is created", func() {
		By("Creating a WorkflowExecution in Pending state")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: namespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:     "disk-cleanup",
					Version:        "v1.0.0",
					ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123def456",
				},
				Parameters: map[string]string{
					"THRESHOLD": "80",
					"DRY_RUN":   "false",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for phase to transition to Running")
		Eventually(func() string {
			updated := &workflowexecutionv1.WorkflowExecution{}
			_ = k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

		By("Verifying PipelineRun was created in execution namespace")
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr)
		Expect(err).ToNot(HaveOccurred())

		By("Verifying PipelineRun has correct configuration")
		// Bundle resolver
		Expect(string(pr.Spec.PipelineRef.ResolverRef.Resolver)).To(Equal("bundles"))

		// Labels for tracking
		Expect(pr.Labels["kubernaut.ai/workflow-execution"]).To(Equal(wfeName))
		Expect(pr.Labels["kubernaut.ai/source-namespace"]).To(Equal(namespace))

		By("Verifying WFE status contains PipelineRun reference")
		updated := &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, updated)).To(Succeed())
		Expect(updated.Status.PipelineRunRef).NotTo(BeNil())
		Expect(updated.Status.PipelineRunRef.Name).To(Equal(prName))
		Expect(updated.Status.StartTime).ToNot(BeNil())
	})

	It("should transition to Completed when PipelineRun succeeds", func() {
		By("Creating a WorkflowExecution")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: namespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:     "disk-cleanup",
					Version:        "v1.0.0",
					ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123def456",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for Running phase")
		Eventually(func() string {
			updated := &workflowexecutionv1.WorkflowExecution{}
			_ = k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

		By("Simulating PipelineRun success")
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr)).To(Succeed())

		// Update PipelineRun status to Succeeded
		pr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionTrue,
			Reason:  "Succeeded",
			Message: "All Tasks have completed executing",
		})
		Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

		By("Waiting for WFE to transition to Completed")
		Eventually(func() string {
			updated := &workflowexecutionv1.WorkflowExecution{}
			_ = k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 15*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1.PhaseCompleted))

		By("Verifying completion details")
		completed := &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, completed)).To(Succeed())
		Expect(completed.Status.CompletionTime).ToNot(BeNil())
		Expect(completed.Status.FailureDetails).To(BeNil())
	})

	It("should transition to Failed with details when PipelineRun fails", func() {
		By("Creating a WorkflowExecution")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: namespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:     "disk-cleanup",
					Version:        "v1.0.0",
					ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123def456",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for Running phase")
		Eventually(func() string {
			updated := &workflowexecutionv1.WorkflowExecution{}
			_ = k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

		By("Simulating PipelineRun failure")
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr)).To(Succeed())

		// Update PipelineRun status to Failed
		pr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  "TaskRunFailed",
			Message: "Task cleanup-disk failed: disk full, cannot cleanup",
		})
		Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

		By("Waiting for WFE to transition to Failed")
		Eventually(func() string {
			updated := &workflowexecutionv1.WorkflowExecution{}
			_ = k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 15*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1.PhaseFailed))

		By("Verifying failure details are populated")
		failed := &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, failed)).To(Succeed())

		Expect(failed.Status.FailureDetails).ToNot(BeNil())
		Expect(failed.Status.FailureDetails.Message).To(ContainSubstring("cleanup-disk"))
		Expect(failed.Status.FailureDetails.NaturalLanguageSummary).ToNot(BeEmpty())
	})
})

// Helper functions

func pipelineRunName(targetResource string) string {
	hash := sha256.Sum256([]byte(targetResource))
	return fmt.Sprintf("wfe-%x", hash[:8])
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

