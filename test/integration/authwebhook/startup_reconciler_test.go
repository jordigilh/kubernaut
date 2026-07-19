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

package authwebhook

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	"github.com/jordigilh/kubernaut/test/testutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// integrationMockDS is a placeholder passed to StartupReconciler.DSWorkflow
// during integration tests. #1661 Phase 55c: CreateActionType was removed
// once DS's ActionType REST endpoints were deleted (DD-WORKFLOW-018) and
// StartupReconciler.DSActionType was removed alongside them --
// syncActionTypeCRD already computed/patched CRD status locally with zero
// DS calls since Change 8d. WorkflowDSClient is still an empty marker
// interface (interface{}), so this type needs zero methods to satisfy it --
// it exists only to give DSWorkflow a distinguishable non-nil value,
// pending the equivalent DSWorkflow field removal in Phase B.
type integrationMockDS struct{}

var _ = Describe("StartupReconciler Integration (#548)", Ordered, ContinueOnFailure, func() {

	// ========================================
	// IT-AW-548-001: StartupReconciler with envtest K8s + mock DS
	// ========================================
	Describe("IT-AW-548-001: StartupReconciler syncs CRDs to DS via envtest", func() {
		var (
			testAT *atv1alpha1.ActionType
			testRW *rwv1alpha1.RemediationWorkflow
		)

		BeforeEach(func() {
			testAT = &atv1alpha1.ActionType{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("it-at-548-%d", GinkgoParallelProcess()),
					Namespace: "default",
				},
				Spec: atv1alpha1.ActionTypeSpec{
					Name: fmt.Sprintf("ITScaleMemory%d", GinkgoParallelProcess()),
					Description: atv1alpha1.ActionTypeDescription{
						What:      "Integration test action type",
						WhenToUse: "For startup reconciler integration testing",
					},
				},
			}

			testRW = &rwv1alpha1.RemediationWorkflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("it-rw-548-%d", GinkgoParallelProcess()),
					Namespace: "default",
				},
				Spec: rwv1alpha1.RemediationWorkflowSpec{
					Version:    "1.0.0",
					ActionType: testAT.Spec.Name,
					Description: rwv1alpha1.RemediationWorkflowDescription{
						What:      "Integration test workflow",
						WhenToUse: "For startup reconciler integration testing",
					},
					Labels: rwv1alpha1.RemediationWorkflowLabels{
						Severity:    []string{"critical"},
						Environment: []string{"production"},
						Component:   []string{"v1/Pod"},
						Priority:    "P1",
					},
					Execution: rwv1alpha1.RemediationWorkflowExecution{
						Engine: "job",
						Bundle: testutil.ValidBundleRef,
					},
					Parameters: []rwv1alpha1.RemediationWorkflowParameter{
						{
							Name:        "NAMESPACE",
							Type:        "string",
							Required:    true,
							Description: "Target namespace",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, testAT)).To(Succeed())
			Expect(k8sClient.Create(ctx, testRW)).To(Succeed())

			// Wait for the controller-runtime informer cache to sync both CRDs.
			// The manager's cached client (k8sManager.GetClient()) is eventually consistent;
			// without this gate the reconciler's List call can race and return 0 items.
			Eventually(func() int {
				var rwList rwv1alpha1.RemediationWorkflowList
				_ = k8sClient.List(ctx, &rwList)
				return len(rwList.Items)
			}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, testAT)
			_ = k8sClient.Delete(ctx, testRW)
		})

		It("should sync CRDs locally and update CRD statuses with zero DS calls (#1661 Change 8c/8d)", func() {
			mockDS := &integrationMockDS{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:  k8sClient,
				DSWorkflow: mockDS,
				Logger:     ctrl.Log.WithName("it-startup-548"),
				Timeout:    30 * time.Second,
			}

			testCtx, testCancel := context.WithTimeout(ctx, 30*time.Second)
			defer testCancel()
			err := reconciler.Start(testCtx)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				updatedAT := &atv1alpha1.ActionType{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: testAT.Namespace,
					Name:      testAT.Name,
				}, updatedAT)).To(Succeed())
				g.Expect(updatedAT.Status.Registered).To(BeTrue(),
					"ActionType status should be populated from the local computation")
				g.Expect(string(updatedAT.Status.CatalogStatus)).To(Equal("Active"))
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			Eventually(func(g Gomega) {
				updated := &rwv1alpha1.RemediationWorkflow{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Namespace: testRW.Namespace,
					Name:      testRW.Name,
				}, updated)).To(Succeed())

				g.Expect(updated.Status.WorkflowID).ToNot(BeEmpty(),
					"workflowId should be populated after startup reconciliation")
				g.Expect(string(updated.Status.CatalogStatus)).To(Equal("Active"))
				g.Expect(updated.Status.RegisteredBy).To(Equal("system:authwebhook-startup"))
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
		})
	})

	// IT-AW-548-002 ("Startup failure halts manager readiness when DS is
	// unavailable") is removed: #1661 Change 8c/8d deleted both
	// syncWorkflowCRD's and syncActionTypeCRD's DS round-trips (and the
	// deadline/backoff/retry machinery that only existed to survive DS being
	// transiently unavailable) entirely. Start() no longer talks to DS at
	// all, so there is no DS-unavailability scenario left to exercise --
	// see startup_graceful_test.go (deleted) for the unit-test-level
	// precedent of this same removal.
})

// Compile-time interface compliance check
var _ authwebhook.WorkflowDSClient = (*integrationMockDS)(nil)
