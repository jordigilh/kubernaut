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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Exponential Backoff State Persistence Integration Tests
//
// Purpose: Validate that ConsecutiveFailures and NextAllowedExecution
// are correctly persisted to the CRD status subresource.
//
// Test Approach (per IMPLEMENTATION_PLAN_V3.8.md):
// - EnvTest doesn't restart the controller
// - We test state persistence by updating CRD status directly
// - Verify values are stored and retrieved correctly via K8s API
//
// Note: Full backoff behavior with timing is tested in unit tests.
// E2E test is skipped (would require 10+ minutes for backoff cycles).

var _ = Describe("Exponential Backoff State Persistence", func() {

	Context("ConsecutiveFailures field persistence", func() {

		It("should persist ConsecutiveFailures=0 for new WFE", func() {
			// Create WFE with default (zero) ConsecutiveFailures
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backoff-test-zero-" + time.Now().Format("150405"),
					Namespace: DefaultNamespace,
				},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				TargetResource: "default/deployment/test-app",
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:     "test-workflow",
					Version:        "v1.0.0",
					ContainerImage: "quay.io/kubernaut/workflows/test:v1.0.0",
				},
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "test-rr",
					Namespace: "default",
				},
			},
			}

			// Create the WFE
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			// Verify ConsecutiveFailures defaults to 0
			fetchedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, fetchedWFE)).To(Succeed())

			Expect(fetchedWFE.Status.ConsecutiveFailures).To(Equal(int32(0)))
		})

		It("should persist ConsecutiveFailures increment via status update", func() {
			// Create WFE
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backoff-test-inc-" + time.Now().Format("150405"),
					Namespace: DefaultNamespace,
				},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				TargetResource: "default/deployment/test-app-inc",
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:     "test-workflow",
					Version:        "v1.0.0",
					ContainerImage: "quay.io/kubernaut/workflows/test:v1.0.0",
				},
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "test-rr-inc",
					Namespace: "default",
				},
			},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			// Update status with ConsecutiveFailures = 1
			wfe.Status.ConsecutiveFailures = 1
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

			// Fetch and verify
			fetchedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, fetchedWFE)).To(Succeed())

			Expect(fetchedWFE.Status.ConsecutiveFailures).To(Equal(int32(1)))

			// Increment again to 2
			fetchedWFE.Status.ConsecutiveFailures = 2
			Expect(k8sClient.Status().Update(ctx, fetchedWFE)).To(Succeed())

			// Fetch and verify persistence
			finalWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, finalWFE)).To(Succeed())

			Expect(finalWFE.Status.ConsecutiveFailures).To(Equal(int32(2)))
		})

		It("should persist ConsecutiveFailures reset to 0 on success", func() {
			// Create WFE with existing failure count
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backoff-test-reset-" + time.Now().Format("150405"),
					Namespace: DefaultNamespace,
				},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				TargetResource: "default/deployment/test-app-reset",
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:     "test-workflow",
					Version:        "v1.0.0",
					ContainerImage: "quay.io/kubernaut/workflows/test:v1.0.0",
				},
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "test-rr-reset",
					Namespace: "default",
				},
			},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			// Set ConsecutiveFailures = 3 (simulating previous failures)
			wfe.Status.ConsecutiveFailures = 3
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

			// Verify it was set
			fetchedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, fetchedWFE)).To(Succeed())
			Expect(fetchedWFE.Status.ConsecutiveFailures).To(Equal(int32(3)))

			// Reset to 0 (simulating success)
			fetchedWFE.Status.ConsecutiveFailures = 0
			Expect(k8sClient.Status().Update(ctx, fetchedWFE)).To(Succeed())

			// Verify reset persisted
			finalWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, finalWFE)).To(Succeed())

			Expect(finalWFE.Status.ConsecutiveFailures).To(Equal(int32(0)))
		})
	})

	Context("NextAllowedExecution field persistence", func() {

		It("should persist NextAllowedExecution timestamp", func() {
			// Create WFE
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backoff-test-next-" + time.Now().Format("150405"),
					Namespace: DefaultNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/test-app-next",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						ContainerImage: "quay.io/kubernaut/workflows/test:v1.0.0",
					},
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr-next",
						Namespace: "default",
					},
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			// Set NextAllowedExecution (simulating backoff calculation)
			futureTime := metav1.NewTime(time.Now().Add(5 * time.Minute))
			wfe.Status.NextAllowedExecution = &futureTime
			wfe.Status.ConsecutiveFailures = 2
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

			// Fetch and verify
			fetchedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, fetchedWFE)).To(Succeed())

			Expect(fetchedWFE.Status.NextAllowedExecution).ToNot(BeNil())
			// Allow 1 second tolerance for serialization
			Expect(fetchedWFE.Status.NextAllowedExecution.Time).To(BeTemporally("~", futureTime.Time, time.Second))
		})

		It("should persist NextAllowedExecution=nil after reset", func() {
			// Create WFE
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backoff-test-nil-" + time.Now().Format("150405"),
					Namespace: DefaultNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/test-app-nil",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						ContainerImage: "quay.io/kubernaut/workflows/test:v1.0.0",
					},
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr-nil",
						Namespace: "default",
					},
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			// Set NextAllowedExecution first
			futureTime := metav1.NewTime(time.Now().Add(5 * time.Minute))
			wfe.Status.NextAllowedExecution = &futureTime
			wfe.Status.ConsecutiveFailures = 2
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

			// Verify it was set
			fetchedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, fetchedWFE)).To(Succeed())
			Expect(fetchedWFE.Status.NextAllowedExecution).ToNot(BeNil())

			// Reset to nil (simulating success)
			fetchedWFE.Status.NextAllowedExecution = nil
			fetchedWFE.Status.ConsecutiveFailures = 0
			Expect(k8sClient.Status().Update(ctx, fetchedWFE)).To(Succeed())

			// Verify reset persisted
			finalWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, finalWFE)).To(Succeed())

			Expect(finalWFE.Status.NextAllowedExecution).To(BeNil())
			Expect(finalWFE.Status.ConsecutiveFailures).To(Equal(int32(0)))
		})
	})

	Context("SkipDetails with backoff reasons", func() {

		It("should persist SkipDetails with ExhaustedRetries reason", func() {
			// Create WFE
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backoff-test-exhausted-" + time.Now().Format("150405"),
					Namespace: DefaultNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/test-app-exhausted",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						ContainerImage: "quay.io/kubernaut/workflows/test:v1.0.0",
					},
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr-exhausted",
						Namespace: "default",
					},
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			// Set SkipDetails with ExhaustedRetries
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
			wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonExhaustedRetries,
				Message:   "Maximum consecutive failures (5) reached for target resource",
				SkippedAt: metav1.Now(),
			}
			wfe.Status.ConsecutiveFailures = 5
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

			// Fetch and verify
			fetchedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, fetchedWFE)).To(Succeed())

			Expect(fetchedWFE.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseSkipped))
			Expect(fetchedWFE.Status.SkipDetails).ToNot(BeNil())
			Expect(fetchedWFE.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonExhaustedRetries))
			Expect(fetchedWFE.Status.ConsecutiveFailures).To(Equal(int32(5)))
		})

		It("should persist SkipDetails with PreviousExecutionFailed reason", func() {
			// Create WFE
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backoff-test-prevfail-" + time.Now().Format("150405"),
					Namespace: DefaultNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/test-app-prevfail",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						ContainerImage: "quay.io/kubernaut/workflows/test:v1.0.0",
					},
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr-prevfail",
						Namespace: "default",
					},
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			// Set SkipDetails with PreviousExecutionFailed (wasExecutionFailure=true case)
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
			wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed,
				Message:   "Previous workflow execution failed - manual intervention required",
				SkippedAt: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

			// Fetch and verify
			fetchedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: wfe.Namespace,
			}, fetchedWFE)).To(Succeed())

			Expect(fetchedWFE.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseSkipped))
			Expect(fetchedWFE.Status.SkipDetails).ToNot(BeNil())
			Expect(fetchedWFE.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed))
		})
	})
})

