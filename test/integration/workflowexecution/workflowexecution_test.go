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
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("WorkflowExecution CRD Integration Tests", func() {
	const (
		testNamespace = "wfe-integration-test"
		timeout       = 10 * time.Second
		interval      = 100 * time.Millisecond
	)

	BeforeEach(func() {
		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		err := k8sClient.Create(ctx, ns)
		if err != nil {
			// Namespace might already exist from previous test
			_ = k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace}, ns)
		}
	})

	AfterEach(func() {
		// Clean up WorkflowExecutions in test namespace
		wfeList := &workflowexecutionv1alpha1.WorkflowExecutionList{}
		err := k8sClient.List(ctx, wfeList, client.InNamespace(testNamespace))
		if err == nil {
			for _, wfe := range wfeList.Items {
				_ = k8sClient.Delete(ctx, &wfe)
			}
		}
	})

	// ========================================
	// CRD Lifecycle Tests (BR-WE-001)
	// ========================================
	Describe("CRD Lifecycle", func() {
		It("should create a WorkflowExecution CRD successfully", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-create",
					Namespace: testNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "oomkill-increase-memory",
						Version:        "1.0.0",
						ContainerImage: "quay.io/kubernaut/workflow-oomkill:v1.0.0",
					},
					TargetResource: testNamespace + "/deployment/test-app",
					Confidence:     0.92,
					Rationale:      "High confidence OOMKill pattern match",
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Verify it was created
			createdWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-create", Namespace: testNamespace}, createdWFE)
			}, timeout, interval).Should(Succeed())

			Expect(createdWFE.Spec.WorkflowRef.WorkflowID).To(Equal("oomkill-increase-memory"))
			Expect(createdWFE.Spec.TargetResource).To(Equal(testNamespace + "/deployment/test-app"))
			Expect(createdWFE.Spec.Confidence).To(BeNumerically("~", 0.92, 0.01))
		})

		It("should update WorkflowExecution status", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-status",
					Namespace: testNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "1.0.0",
						ContainerImage: "quay.io/kubernaut/workflow-test:v1.0.0",
					},
					TargetResource: testNamespace + "/deployment/test-app",
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Update status
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-status", Namespace: testNamespace}, wfe)
				if err != nil {
					return err
				}
				now := metav1.Now()
				wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
				wfe.Status.StartTime = &now
				return k8sClient.Status().Update(ctx, wfe)
			}, timeout, interval).Should(Succeed())

			// Verify status was updated
			updatedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-status", Namespace: testNamespace}, updatedWFE)
				if err != nil {
					return ""
				}
				return updatedWFE.Status.Phase
			}, timeout, interval).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))
		})

		It("should delete WorkflowExecution CRD successfully", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-delete",
					Namespace: testNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "1.0.0",
						ContainerImage: "quay.io/kubernaut/workflow-test:v1.0.0",
					},
					TargetResource: testNamespace + "/deployment/test-app",
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Delete it
			Expect(k8sClient.Delete(ctx, wfe)).To(Succeed())

			// Verify it was deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-delete", Namespace: testNamespace}, &workflowexecutionv1alpha1.WorkflowExecution{})
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	// ========================================
	// SkipDetails Tests (DD-WE-001)
	// ========================================
	Describe("SkipDetails Population", func() {
		It("should populate ResourceBusy skip details", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-skip-busy",
					Namespace: testNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "1.0.0",
						ContainerImage: "quay.io/kubernaut/workflow-test:v1.0.0",
					},
					TargetResource: testNamespace + "/deployment/test-app",
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Update with skip details
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-skip-busy", Namespace: testNamespace}, wfe)
				if err != nil {
					return err
				}
				now := metav1.Now()
				wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
				wfe.Status.CompletionTime = &now
				wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
					Reason:    workflowexecutionv1alpha1.SkipReasonResourceBusy,
					Message:   "Resource is busy with another workflow",
					SkippedAt: now,
					ConflictingWorkflow: &workflowexecutionv1alpha1.ConflictingWorkflowRef{
						Name:           "blocking-wfe",
						WorkflowID:     "same-workflow",
						TargetResource: testNamespace + "/deployment/test-app",
						StartedAt:      now,
					},
				}
				return k8sClient.Status().Update(ctx, wfe)
			}, timeout, interval).Should(Succeed())

			// Verify skip details
			updatedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-skip-busy", Namespace: testNamespace}, updatedWFE)
				if err != nil || updatedWFE.Status.SkipDetails == nil {
					return ""
				}
				return updatedWFE.Status.SkipDetails.Reason
			}, timeout, interval).Should(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))

			Expect(updatedWFE.Status.SkipDetails.ConflictingWorkflow).ToNot(BeNil())
			Expect(updatedWFE.Status.SkipDetails.ConflictingWorkflow.Name).To(Equal("blocking-wfe"))
		})
	})

	// ========================================
	// FailureDetails Tests (BR-WE-004)
	// ========================================
	Describe("FailureDetails Population", func() {
		It("should populate failure details with all fields", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-failure",
					Namespace: testNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "1.0.0",
						ContainerImage: "quay.io/kubernaut/workflow-test:v1.0.0",
					},
					TargetResource: testNamespace + "/deployment/test-app",
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Update with failure details
			exitCode := int32(137)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-failure", Namespace: testNamespace}, wfe)
				if err != nil {
					return err
				}
				now := metav1.Now()
				wfe.Status.Phase = workflowexecutionv1alpha1.PhaseFailed
				wfe.Status.CompletionTime = &now
				wfe.Status.FailureReason = workflowexecutionv1alpha1.FailureReasonOOMKilled
				wfe.Status.FailureDetails = &workflowexecutionv1alpha1.FailureDetails{
					FailedTaskIndex:            1,
					FailedTaskName:             "apply-memory-increase",
					FailedStepName:             "kubectl-apply",
					Reason:                     workflowexecutionv1alpha1.FailureReasonOOMKilled,
					Message:                    "Container exceeded memory limits",
					ExitCode:                   &exitCode,
					FailedAt:                   now,
					ExecutionTimeBeforeFailure: "2m30s",
					NaturalLanguageSummary:     "Task 'apply-memory-increase' failed with OOMKilled after 2m30s",
					WasExecutionFailure:        true,
				}
				return k8sClient.Status().Update(ctx, wfe)
			}, timeout, interval).Should(Succeed())

			// Verify failure details
			updatedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-failure", Namespace: testNamespace}, updatedWFE)
				if err != nil || updatedWFE.Status.FailureDetails == nil {
					return ""
				}
				return updatedWFE.Status.FailureDetails.Reason
			}, timeout, interval).Should(Equal(workflowexecutionv1alpha1.FailureReasonOOMKilled))

			Expect(updatedWFE.Status.FailureDetails.FailedTaskName).To(Equal("apply-memory-increase"))
			Expect(*updatedWFE.Status.FailureDetails.ExitCode).To(Equal(int32(137)))
			Expect(updatedWFE.Status.FailureDetails.WasExecutionFailure).To(BeTrue())
		})
	})

	// ========================================
	// Phase Transition Tests
	// ========================================
	Describe("Phase Transitions", func() {
		It("should allow valid phase transitions", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-phases",
					Namespace: testNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "1.0.0",
						ContainerImage: "quay.io/kubernaut/workflow-test:v1.0.0",
					},
					TargetResource: testNamespace + "/deployment/test-app",
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Transition: Pending -> Running
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-phases", Namespace: testNamespace}, wfe)
				if err != nil {
					return err
				}
				now := metav1.Now()
				wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
				wfe.Status.StartTime = &now
				wfe.Status.PipelineRunRef = &corev1.LocalObjectReference{Name: "test-pipelinerun"}
				return k8sClient.Status().Update(ctx, wfe)
			}, timeout, interval).Should(Succeed())

			// Transition: Running -> Completed
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-phases", Namespace: testNamespace}, wfe)
				if err != nil {
					return err
				}
				now := metav1.Now()
				wfe.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
				wfe.Status.CompletionTime = &now
				wfe.Status.Duration = "1m30s"
				return k8sClient.Status().Update(ctx, wfe)
			}, timeout, interval).Should(Succeed())

			// Verify final phase
			finalWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-phases", Namespace: testNamespace}, finalWFE)
				if err != nil {
					return ""
				}
				return finalWFE.Status.Phase
			}, timeout, interval).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted))

			Expect(finalWFE.Status.Duration).To(Equal("1m30s"))
		})
	})

	// ========================================
	// Labels and Annotations Tests
	// ========================================
	Describe("Labels and Annotations", func() {
		It("should preserve custom labels and annotations", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-labels",
					Namespace: testNamespace,
					Labels: map[string]string{
						"kubernaut.ai/workflow-id":         "test-workflow",
						"kubernaut.ai/remediation-request": "test-remediation",
					},
					Annotations: map[string]string{
						"kubernaut.ai/correlation-id": "corr-12345",
					},
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-remediation",
						Namespace: testNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "1.0.0",
						ContainerImage: "quay.io/kubernaut/workflow-test:v1.0.0",
					},
					TargetResource: testNamespace + "/deployment/test-app",
				},
			}

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Verify labels and annotations
			createdWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: "test-wfe-labels", Namespace: testNamespace}, createdWFE)
			}, timeout, interval).Should(Succeed())

			Expect(createdWFE.Labels["kubernaut.ai/workflow-id"]).To(Equal("test-workflow"))
			Expect(createdWFE.Annotations["kubernaut.ai/correlation-id"]).To(Equal("corr-12345"))
		})
	})
})

