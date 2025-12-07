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
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
)

// WorkflowExecution CRD Lifecycle Integration Tests
//
// These tests validate CRD operations with real Kubernetes API (EnvTest):
// - Create, Read, Update, Delete operations
// - Status updates (manual - no controller running)
// - CRD schema validation
//
// NOTE: Controller is NOT running in EnvTest (Tekton CRDs not available)
// Controller behavior tests are in E2E suite (KIND + Tekton)
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
				_ = deleteWFEAndWait(wfe, 10*time.Second)
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
				_ = deleteWFEAndWait(wfe, 10*time.Second)
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

	Context("CRD Status Updates (Manual)", func() {
		It("should allow status subresource updates", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-status-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("status", targetResource)

			defer func() {
				_ = deleteWFEAndWait(wfe, 10*time.Second)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Manually update status (simulating controller)
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
			now := metav1.Now()
			wfe.Status.StartTime = &now
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

			// Verify status updated
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning))
			Expect(updated.Status.StartTime).ToNot(BeNil())

			GinkgoWriter.Printf("✅ Status updated to: %s\n", updated.Status.Phase)
		})

		It("should update ConsecutiveFailures and NextAllowedExecution", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-backoff-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("backoff", targetResource)

			defer func() {
				_ = deleteWFEAndWait(wfe, 10*time.Second)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Manually set failure status with backoff fields
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseFailed
			wfe.Status.ConsecutiveFailures = 3
			nextAllowed := metav1.NewTime(time.Now().Add(30 * time.Second))
			wfe.Status.NextAllowedExecution = &nextAllowed
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

			// Verify backoff fields persisted
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.ConsecutiveFailures).To(Equal(int32(3)))
			Expect(updated.Status.NextAllowedExecution).ToNot(BeNil())

			GinkgoWriter.Println("✅ Backoff fields persisted correctly")
		})
	})

	Context("CRD Deletion", func() {
		It("should delete WorkflowExecution without finalizer", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-delete-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("delete", targetResource)

			// Create first (without finalizer since controller not running)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for creation
			Eventually(func() error {
				_, err := getWFE(wfe.Name, wfe.Namespace)
				return err
			}, 5*time.Second).Should(Succeed())

			// Delete (should succeed immediately without finalizer)
			err := deleteWFEAndWait(wfe, 15*time.Second)
			Expect(err).ToNot(HaveOccurred())

			// Verify deleted
			_, err = getWFE(wfe.Name, wfe.Namespace)
			Expect(err).To(HaveOccurred(), "WFE should be deleted")

			GinkgoWriter.Println("✅ WFE deleted successfully")
		})

		It("should block deletion when finalizer is present", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-finalizer-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("finalizer", targetResource)
			// Manually add finalizer (simulating controller)
			wfe.Finalizers = []string{workflowexecution.FinalizerName}

			defer func() {
				// Remove finalizer to allow cleanup
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					updated.Finalizers = nil
					_ = k8sClient.Update(ctx, updated)
					_ = deleteWFEAndWait(wfe, 10*time.Second)
				}
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Try to delete - should not be removed immediately due to finalizer
			Expect(k8sClient.Delete(ctx, wfe)).To(Succeed())

			// WFE should still exist (deletion timestamp set, but not removed)
			time.Sleep(500 * time.Millisecond)
			existing, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(existing.DeletionTimestamp).ToNot(BeNil(), "DeletionTimestamp should be set")

			GinkgoWriter.Println("✅ Finalizer blocks immediate deletion")
		})
	})

	Context("Spec Validation", func() {
		It("should accept valid WorkflowExecution spec", func() {
			targetResource := fmt.Sprintf("default/deployment/lifecycle-valid-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("valid", targetResource)

			defer func() {
				_ = deleteWFEAndWait(wfe, 10*time.Second)
			}()

			// Should succeed
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			GinkgoWriter.Println("✅ Valid WFE spec accepted")
		})
	})
})

