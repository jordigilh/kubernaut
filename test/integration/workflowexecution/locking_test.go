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
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"knative.dev/pkg/apis"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("Integration: Resource Locking (DD-WE-001)", func() {
	var (
		namespace      string
		targetResource string
	)

	BeforeEach(func() {
		namespace = "kubernaut-test"
		targetResource = "production/deployment/app-" + randomString(5)
	})

	AfterEach(func() {
		// Cleanup all WFEs for this namespace
		wfeList := &workflowexecutionv1.WorkflowExecutionList{}
		_ = k8sClient.List(ctx, wfeList, client.InNamespace(namespace))
		for _, wfe := range wfeList.Items {
			_ = k8sClient.Delete(ctx, &wfe)
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

	Context("Parallel Execution Prevention", func() {
		It("should block second WFE when first is Running", func() {
			By("Creating first WFE that will start Running")
			wfe1 := createTestWFE(namespace, "wfe-first-"+randomString(5), targetResource)

			By("Waiting for first WFE to reach Running")
			Eventually(func() string {
				updated := &workflowexecutionv1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe1.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

			By("Creating second WFE for same target")
			wfe2 := createTestWFE(namespace, "wfe-second-"+randomString(5), targetResource)

			By("Verifying second WFE is Skipped with ResourceBusy")
			Eventually(func() string {
				updated := &workflowexecutionv1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe2.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseSkipped))

			By("Verifying skip reason is ResourceBusy")
			skipped := &workflowexecutionv1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe2.Name,
				Namespace: namespace,
			}, skipped)).To(Succeed())
			Expect(skipped.Status.SkipDetails).ToNot(BeNil())
			Expect(skipped.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1.SkipReasonResourceBusy))
		})
	})

	Context("Cooldown Period Enforcement", func() {
		It("should block WFE during cooldown after completion", func() {
			By("Creating and completing first WFE")
			wfe1 := createTestWFE(namespace, "wfe-cooldown-"+randomString(5), targetResource)

			// Wait for Running
			Eventually(func() string {
				updated := &workflowexecutionv1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe1.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

			// Simulate success
			simulatePipelineRunSuccess(targetResource)

			// Wait for Completed
			Eventually(func() string {
				updated := &workflowexecutionv1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe1.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 15*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1.PhaseCompleted))

			By("Creating second WFE immediately (within cooldown)")
			wfe2 := createTestWFE(namespace, "wfe-during-cooldown-"+randomString(5), targetResource)

			By("Verifying second WFE is Skipped with RecentlyRemediated")
			Eventually(func() string {
				updated := &workflowexecutionv1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe2.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseSkipped))

			By("Verifying skip reason is RecentlyRemediated")
			skipped := &workflowexecutionv1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe2.Name,
				Namespace: namespace,
			}, skipped)).To(Succeed())
			Expect(skipped.Status.SkipDetails).ToNot(BeNil())
			Expect(skipped.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1.SkipReasonRecentlyRemediated))
		})
	})

	Context("Race Condition Handling (DD-WE-003)", func() {
		It("should handle concurrent WFE creation gracefully", func() {
			By("Creating multiple WFEs concurrently for same target")
			var wg sync.WaitGroup
			wfeNames := make([]string, 5)
			results := make([]error, 5)

			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					defer GinkgoRecover()
					name := fmt.Sprintf("wfe-concurrent-%d-%s", index, randomString(5))
					wfeNames[index] = name
					wfe := &workflowexecutionv1.WorkflowExecution{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
						},
						Spec: workflowexecutionv1.WorkflowExecutionSpec{
							TargetResource: targetResource,
							WorkflowRef: workflowexecutionv1.WorkflowRef{
								WorkflowID:     "test-workflow",
								Version:        "v1.0.0",
								ContainerImage: "ghcr.io/kubernaut/workflows/test@sha256:abc123",
							},
						},
					}
					results[index] = k8sClient.Create(ctx, wfe)
				}(i)
			}
			wg.Wait()

			By("Verifying all creations succeeded")
			for i, err := range results {
				Expect(err).ToNot(HaveOccurred(), "WFE %d creation failed", i)
			}

			By("Waiting for all WFEs to reach terminal state")
			time.Sleep(5 * time.Second)

			By("Counting Running vs Skipped WFEs")
			runningCount := 0
			skippedCount := 0

			for _, name := range wfeNames {
				wfe := &workflowexecutionv1.WorkflowExecution{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      name,
					Namespace: namespace,
				}, wfe)).To(Succeed())

				switch wfe.Status.Phase {
				case workflowexecutionv1.PhaseRunning:
					runningCount++
				case workflowexecutionv1.PhaseSkipped:
					skippedCount++
				}
			}

			By("Verifying exactly one WFE is Running")
			Expect(runningCount).To(Equal(1), "Exactly one WFE should be Running")
			Expect(skippedCount).To(Equal(4), "Four WFEs should be Skipped")
		})
	})
})

// Helper functions

func createTestWFE(namespace, name, targetResource string) *workflowexecutionv1.WorkflowExecution {
	wfe := &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
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
	return wfe
}

func simulatePipelineRunSuccess(targetResource string) {
	prName := pipelineRunName(targetResource)
	pr := &tektonv1.PipelineRun{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name:      prName,
		Namespace: "kubernaut-workflows",
	}, pr)).To(Succeed())

	pr.Status.SetCondition(&apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionTrue,
		Reason:  "Succeeded",
		Message: "All Tasks completed",
	})
	Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())
}

