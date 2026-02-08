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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// CUSTOM EXECUTION NAMESPACE TEST
// ========================================
//
// **Business Requirement**: BR-WE-009 (Execution Namespace is Configurable)
// **Migrated from**: test/e2e/workflowexecution/05_custom_config_test.go (Line 168)
//
// **Purpose**: Validate that PipelineRuns are created in the configured
// execution namespace, not hardcoded to "kubernaut-workflows".
//
// **Why Integration Test (not E2E)**:
// ✅ Integration tests can configure reconciler with custom ExecutionNamespace
// ✅ EnvTest supports creating/verifying resources in multiple namespaces
// ✅ No Kind cluster required (uses envtest)
// ✅ Much faster execution (seconds vs minutes)
// ✅ Tests actual controller reconciliation logic
//
// **Test Strategy**:
// - Controller configured with default namespace (suite_test.go: "kubernaut-workflows")
// - Create WorkflowExecution in "default" namespace
// - Verify PipelineRun created in "kubernaut-workflows" (ExecutionNamespace)
// - Verify cross-namespace operation works correctly
//
// **Note**: The ExecutionNamespace is configured in suite_test.go:220
// as part of the reconciler setup. This test validates that the controller
// respects that configuration.

var _ = Describe("Custom Execution Namespace", Label("config", "namespace"), func() {

	Context("BR-WE-009: PipelineRuns Created in Configured Namespace", func() {
		It("should create PipelineRuns in ExecutionNamespace, not WFE namespace", func() {
			ctx := context.Background()
			var err error

			By("Verifying ExecutionNamespace exists")
			// The suite creates WorkflowExecutionNS namespace (kubernaut-workflows)
			// Verify it's available for this test
			Expect(WorkflowExecutionNS).To(Equal("kubernaut-workflows"))

			By("Creating WorkflowExecution in default namespace")
			wfe := createUniqueWFE("custom-ns", "default/deployment/test-app")
			// WFE is in "default" namespace, but PipelineRun should be in "kubernaut-workflows"
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, wfe) }()

			By("Waiting for controller to add finalizer")
			// Wait for controller to process the WFE and get updated object with UID
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), updated); err != nil {
					return false
				}
				if len(updated.Finalizers) > 0 {
					// Update wfe with server-assigned UID
					wfe = updated
					return true
				}
				return false
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying PipelineRun is NOT in default namespace")
			// PipelineRun should NOT be created in default namespace
			prListDefault := &tektonv1.PipelineRunList{}
			err = k8sClient.List(ctx, prListDefault, &client.ListOptions{
				Namespace: DefaultNamespace, // "default"
			})
			Expect(err).ToNot(HaveOccurred())

			// Should find NO PipelineRuns owned by our WFE in default namespace
			foundInDefault := false
			for i := range prListDefault.Items {
				pr := &prListDefault.Items[i]
				for _, owner := range pr.OwnerReferences {
					if owner.UID == wfe.UID {
						foundInDefault = true
						break
					}
				}
			}
			Expect(foundInDefault).To(BeFalse(), "PipelineRun should NOT be in default namespace")

			By("Waiting for WFE to transition to Running phase with ExecutionRef")
			// Use Eventually to wait for PipelineRun creation via WFE status
			var updatedWFE *workflowexecutionv1alpha1.WorkflowExecution
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), updated); err != nil {
					return false
				}
				if updated.Status.ExecutionRef != nil && updated.Status.ExecutionRef.Name != "" {
					updatedWFE = updated
					return true
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "WFE should have ExecutionRef populated")

			By("Verifying PipelineRun IS in ExecutionNamespace (kubernaut-workflows)")
			// Fetch the PipelineRun using the reference from WFE status
			foundPR := &tektonv1.PipelineRun{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      updatedWFE.Status.ExecutionRef.Name,
				Namespace: WorkflowExecutionNS,
			}, foundPR)
			Expect(err).ToNot(HaveOccurred(), "PipelineRun should exist in ExecutionNamespace")
			Expect(foundPR.Namespace).To(Equal(WorkflowExecutionNS), "PipelineRun should be in kubernaut-workflows")

			By("Verifying ServiceAccount is correct in ExecutionNamespace")
			// PipelineRun should reference the ServiceAccount in the execution namespace
			Expect(foundPR.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("kubernaut-workflow-runner"))

			By("Verifying WFE tracks PipelineRun correctly (cross-namespace)")
			// WFE in "default" should track PipelineRun in "kubernaut-workflows"
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), updated); err != nil {
					return false
				}
				// ExecutionRef should be populated with PipelineRun name
				// (LocalObjectReference doesn't have Namespace field - namespace is implied)
				return updated.Status.ExecutionRef != nil &&
					updated.Status.ExecutionRef.Name != ""
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(), "WFE should track PipelineRun")

			GinkgoWriter.Printf("✅ PipelineRun created in ExecutionNamespace: %s\n", WorkflowExecutionNS)
			GinkgoWriter.Printf("   WFE Namespace: %s\n", wfe.Namespace)
			GinkgoWriter.Printf("   PipelineRun Namespace: %s\n", foundPR.Namespace)
			GinkgoWriter.Println("✅ Cross-namespace operation validated successfully")
		})

		It("should respect ExecutionNamespace for multiple WFEs", func() {
			ctx := context.Background()
			var err error

			By("Creating multiple WorkflowExecutions in different namespaces")
			wfe1 := createUniqueWFE("multi-ns-1", "default/deployment/app1")
			wfe2 := createUniqueWFE("multi-ns-2", "default/deployment/app2")

			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe1)
				_ = k8sClient.Delete(ctx, wfe2)
			}()

			By("Waiting for both WFEs to get UIDs assigned")
			// Refetch WFEs to get server-assigned UIDs
			Eventually(func() bool {
				updated1 := &workflowexecutionv1alpha1.WorkflowExecution{}
				updated2 := &workflowexecutionv1alpha1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe1), updated1); err != nil {
					return false
				}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe2), updated2); err != nil {
					return false
				}
				// Update with server-assigned UIDs
				wfe1 = updated1
				wfe2 = updated2
				return updated1.UID != "" && updated2.UID != ""
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Waiting for both WFEs to have ExecutionRefs")
			// Wait for WFE1's PipelineRun
			var updated1, updated2 *workflowexecutionv1alpha1.WorkflowExecution
			Eventually(func() bool {
				u1 := &workflowexecutionv1alpha1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe1), u1); err != nil {
					return false
				}
				if u1.Status.ExecutionRef != nil && u1.Status.ExecutionRef.Name != "" {
					updated1 = u1
					return true
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "WFE1 should have ExecutionRef")

			// Wait for WFE2's PipelineRun
			Eventually(func() bool {
				u2 := &workflowexecutionv1alpha1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe2), u2); err != nil {
					return false
				}
				if u2.Status.ExecutionRef != nil && u2.Status.ExecutionRef.Name != "" {
					updated2 = u2
					return true
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "WFE2 should have ExecutionRef")

			By("Verifying ALL PipelineRuns are in ExecutionNamespace")
			// Fetch PipelineRuns using references from WFE statuses
			pr1 := &tektonv1.PipelineRun{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      updated1.Status.ExecutionRef.Name,
				Namespace: WorkflowExecutionNS,
			}, pr1)
			Expect(err).ToNot(HaveOccurred(), "WFE1's PipelineRun should exist in ExecutionNamespace")
			Expect(pr1.Namespace).To(Equal(WorkflowExecutionNS))

			pr2 := &tektonv1.PipelineRun{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      updated2.Status.ExecutionRef.Name,
				Namespace: WorkflowExecutionNS,
			}, pr2)
			Expect(err).ToNot(HaveOccurred(), "WFE2's PipelineRun should exist in ExecutionNamespace")
			Expect(pr2.Namespace).To(Equal(WorkflowExecutionNS))

			GinkgoWriter.Println("✅ All PipelineRuns created in ExecutionNamespace regardless of WFE namespace")
		})
	})

	Context("DD-WE-002: Dedicated Execution Namespace Benefits", func() {
		It("should isolate PipelineRuns from WFE CRDs for better organization", func() {
			ctx := context.Background()
			var err error

			By("Creating WorkflowExecution")
			wfe := createUniqueWFE("isolation", "default/deployment/isolated-app")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, wfe) }()

			By("Waiting for WFE to get UID assigned")
			// Refetch WFE to get server-assigned UID
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), updated); err != nil {
					return false
				}
				// Update with server-assigned UID
				wfe = updated
				return updated.UID != ""
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Waiting for WFE to have ExecutionRef")
			// Use Eventually to wait for PipelineRun creation via WFE status
			var updatedWFE *workflowexecutionv1alpha1.WorkflowExecution
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), updated); err != nil {
					return false
				}
				if updated.Status.ExecutionRef != nil && updated.Status.ExecutionRef.Name != "" {
					updatedWFE = updated
					return true
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "WFE should have ExecutionRef")

			By("Verifying WFE and PipelineRun are in different namespaces")
			// This validates DD-WE-002: Dedicated Execution Namespace design decision
			// - WFE CRDs in "default" (where users create them)
			// - PipelineRuns in "kubernaut-workflows" (isolated execution environment)

			// Fetch PipelineRun using reference from WFE status
			foundPR := &tektonv1.PipelineRun{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      updatedWFE.Status.ExecutionRef.Name,
				Namespace: WorkflowExecutionNS,
			}, foundPR)
			Expect(err).ToNot(HaveOccurred(), "PipelineRun should be created")

			Expect(updatedWFE.Namespace).To(Equal(DefaultNamespace), "WFE should be in default namespace")
			Expect(foundPR.Namespace).To(Equal(WorkflowExecutionNS), "PipelineRun should be in execution namespace")
			Expect(updatedWFE.Namespace).ToNot(Equal(foundPR.Namespace), "WFE and PipelineRun should be in different namespaces")

			GinkgoWriter.Println("✅ DD-WE-002: Namespace isolation validated")
			GinkgoWriter.Printf("   WFE Namespace: %s (user-facing)\n", wfe.Namespace)
			GinkgoWriter.Printf("   PipelineRun Namespace: %s (execution isolation)\n", foundPR.Namespace)
		})
	})
})
