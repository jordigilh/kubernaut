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
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/testutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// integrationMockDS records DS calls from the startup reconciler during integration tests.
type integrationMockDS struct {
	atCreated []string
	rwCreated []string
}

func (m *integrationMockDS) CreateActionType(_ context.Context, name string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeRegistrationResult, error) {
	m.atCreated = append(m.atCreated, name)
	return &authwebhook.ActionTypeRegistrationResult{ActionType: name, Status: "created"}, nil
}

func (m *integrationMockDS) CreateWorkflowInline(_ context.Context, _, source, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
	m.rwCreated = append(m.rwCreated, source)
	return &authwebhook.WorkflowRegistrationResult{
		WorkflowID:   fmt.Sprintf("it-uuid-%s", source),
		WorkflowName: source,
		Version:      "1.0.0",
		Status:       "Active",
	}, nil
}

var _ = Describe("StartupReconciler Integration (#548)", Ordered, func() {

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
						Component:   []string{"pod"},
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
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, testAT)
			_ = k8sClient.Delete(ctx, testRW)
		})

		It("should sync CRDs to mock DS and update CRD statuses", func() {
			mockDS := &integrationMockDS{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:    k8sClient,
				DSWorkflow:   mockDS,
				DSActionType: mockDS,
				Logger:       ctrl.Log.WithName("it-startup-548"),
				Timeout:      30 * time.Second,
			}

			testCtx, testCancel := context.WithTimeout(ctx, 30*time.Second)
			defer testCancel()
			err := reconciler.Start(testCtx)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDS.atCreated).To(ContainElement(testAT.Spec.Name))

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

	// ========================================
	// IT-AW-548-002: Startup failure halts manager readiness
	// ========================================
	Describe("IT-AW-548-002: Startup failure returns error (fail-closed)", func() {
		It("should return an error when DS is unavailable and timeout expires", func() {
			failDS := &failingIntegrationDS{}

			testAT := &atv1alpha1.ActionType{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("it-at-548-fail-%d", GinkgoParallelProcess()),
					Namespace: "default",
				},
				Spec: atv1alpha1.ActionTypeSpec{
					Name: fmt.Sprintf("ITFailType%d", GinkgoParallelProcess()),
					Description: atv1alpha1.ActionTypeDescription{
						What:      "Failing integration test action type",
						WhenToUse: "Testing fail-closed behavior",
					},
				},
			}
			Expect(k8sClient.Create(ctx, testAT)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, testAT) }()

			By("Waiting for AT to be visible in the cached client")
			Eventually(func(g Gomega) {
				var atList atv1alpha1.ActionTypeList
				g.Expect(k8sClient.List(ctx, &atList)).To(Succeed())
				g.Expect(atList.Items).NotTo(BeEmpty(),
					"informer cache must reflect the ActionType before starting the reconciler")
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     failDS,
				DSActionType:   failDS,
				Logger:         ctrl.Log.WithName("it-startup-548-fail"),
				Timeout:        1 * time.Second,
				InitialBackoff: 100 * time.Millisecond,
			}

			testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
			defer testCancel()
			err := reconciler.Start(testCtx)
			Expect(err).To(HaveOccurred(),
				"startup reconciler should fail-closed when DS is unavailable")
		})
	})
})

type failingIntegrationDS struct{}

func (f *failingIntegrationDS) CreateActionType(_ context.Context, _ string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeRegistrationResult, error) {
	return nil, fmt.Errorf("connection refused")
}

func (f *failingIntegrationDS) CreateWorkflowInline(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
	return nil, fmt.Errorf("connection refused")
}

// Compile-time interface compliance check
var _ authwebhook.WorkflowDSClient = (*integrationMockDS)(nil)
var _ authwebhook.ActionTypeDSClient = (*integrationMockDS)(nil)
var _ authwebhook.WorkflowDSClient = (*failingIntegrationDS)(nil)
var _ authwebhook.ActionTypeDSClient = (*failingIntegrationDS)(nil)
