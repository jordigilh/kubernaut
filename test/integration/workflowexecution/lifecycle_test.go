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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// WorkflowExecution CRD Lifecycle Integration Tests
//
// V2.0 UPDATE: Controller IS running - tests work WITH the controller
//
// These tests validate CRD operations with real Kubernetes API (EnvTest):
// - Create, Read, Update, Delete operations
// - Status is managed BY THE CONTROLLER (not manually set)
// - CRD schema validation
//
// Per 03-testing-strategy.mdc: >50% integration coverage for microservices

var _ = Describe("WorkflowExecution CRD Lifecycle", func() {
	// Use unique target resources per test for parallel isolation (4 procs)

	Context("CRD Creation", func() {
		It("should create WorkflowExecution successfully", func() {
			// Unique target for this test
			targetResource := fmt.Sprintf("default/deployment/lifecycle-create-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("create", targetResource)

			// Cleanup in defer (parallel-safe pattern per 03-testing-strategy.mdc)
			defer func() {
				cleanupWFE(wfe)
			}()

			// Create WFE
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Verify created
			Eventually(func() error {
				_, err := getWFE(wfe.Name, wfe.Namespace)
				return err
			}, 5*time.Second).Should(Succeed())

			GinkgoWriter.Printf("✅ WFE created: %s\n", wfe.Name)
		})

		It("should preserve spec fields after creation", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-spec-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("spec", targetResource)
			wfe.Spec.Parameters = map[string]string{"KEY": "value"}
			wfe.Spec.Confidence = 0.95

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Verify spec preserved
			created, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.TargetResource).To(Equal(targetResource))
			Expect(created.Spec.WorkflowRef.WorkflowID).To(Equal("test-workflow"))
			Expect(created.Spec.Parameters).To(HaveKeyWithValue("KEY", "value"))
			Expect(created.Spec.Confidence).To(Equal(0.95))

			GinkgoWriter.Println("✅ Spec fields preserved")
		})
	})

	Context("CRD Status Updates (Controller-Driven)", func() {
		// V2.0: Controller is running, so we observe status changes

		It("should update status to Running via controller", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-status-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("status", targetResource)

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for controller to set status to Running (creates PipelineRun)
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			// Verify status fields set by controller
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning))
			Expect(updated.Status.StartTime).ToNot(BeNil())

			GinkgoWriter.Printf("✅ Status updated by controller to: %s\n", updated.Status.Phase)
		})

		It("should set PipelineRunRef when Running", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-prref-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("prref", targetResource)

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for controller to set PipelineRunRef
			Eventually(func() bool {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return false
				}
				return updated.Status.PipelineRunRef != nil && updated.Status.PipelineRunRef.Name != ""
			}, 10*time.Second, 200*time.Millisecond).Should(BeTrue())

			// Verify PipelineRunRef
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.PipelineRunRef).ToNot(BeNil())
			Expect(updated.Status.PipelineRunRef.Name).ToNot(BeEmpty())

			GinkgoWriter.Printf("✅ PipelineRunRef set: %s\n", updated.Status.PipelineRunRef.Name)
		})

		It("should persist ConsecutiveFailures from status", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-backoff-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("backoff", targetResource)

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for Running phase first
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			// Get fresh copy and update ConsecutiveFailures
			// Note: This simulates what the controller does after a failure
			Eventually(func() error {
				fresh, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return err
				}
				fresh.Status.ConsecutiveFailures = 3
				nextAllowed := metav1.NewTime(time.Now().Add(30 * time.Second))
				fresh.Status.NextAllowedExecution = &nextAllowed
				return k8sClient.Status().Update(ctx, fresh)
			}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

			// Verify backoff fields persisted
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.ConsecutiveFailures).To(Equal(int32(3)))
			Expect(updated.Status.NextAllowedExecution).ToNot(BeNil())

			GinkgoWriter.Println("✅ Backoff fields persisted correctly")
		})
	})

	Context("CRD Deletion", func() {
		It("should delete WorkflowExecution with controller cleanup", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-delete-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("delete", targetResource)

			// Create first
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for Running phase (controller adds finalizer)
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			// Delete - controller will handle cleanup
			err := deleteWFEAndWait(wfe, 15*time.Second)
			Expect(err).ToNot(HaveOccurred())

			// Verify deleted
			_, err = getWFE(wfe.Name, wfe.Namespace)
			Expect(err).To(HaveOccurred(), "WFE should be deleted")

			GinkgoWriter.Println("✅ WFE deleted successfully with controller cleanup")
		})

		It("should handle finalizer during deletion", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-finalizer-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("finalizer", targetResource)

			// Create WFE
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for controller to add finalizer (happens when Running)
			Eventually(func() bool {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return false
				}
				return len(updated.Finalizers) > 0
			}, 10*time.Second, 200*time.Millisecond).Should(BeTrue())

			// Verify finalizer is present
			withFinalizer, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(withFinalizer.Finalizers).ToNot(BeEmpty())

			// Delete - controller will remove finalizer after cleanup
			Expect(k8sClient.Delete(ctx, wfe)).To(Succeed())

			// Eventually should be fully deleted
			Eventually(func() bool {
				_, err := getWFE(wfe.Name, wfe.Namespace)
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

			GinkgoWriter.Println("✅ Finalizer handled correctly during deletion")
		})
	})

	Context("Spec Validation", func() {
		It("should accept valid WorkflowExecution spec", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-valid-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("valid", targetResource)

			defer func() {
				cleanupWFE(wfe)
			}()

			// Should succeed
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			GinkgoWriter.Println("✅ Valid WFE spec accepted")
		})
	})
})
