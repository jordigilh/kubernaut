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

package authwebhook_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = rwv1alpha1.AddToScheme(s)
	_ = atv1alpha1.AddToScheme(s)
	return s
}

func buildRWForReconciler(name, namespace, workflowID, actionType string) *rwv1alpha1.RemediationWorkflow {
	rw := &rwv1alpha1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: rwv1alpha1.RemediationWorkflowSpec{
			Version: "1.0.0",
			Description: rwv1alpha1.RemediationWorkflowDescription{
				What:      "Unit test workflow",
				WhenToUse: "During unit tests",
			},
			ActionType: actionType,
			Labels: rwv1alpha1.RemediationWorkflowLabels{
				Severity:    []string{"critical"},
				Environment: []string{"production"},
				Component:   []string{"v1/Pod"},
				Priority:    "P1",
			},
			Execution: rwv1alpha1.RemediationWorkflowExecution{
				Engine: "job",
				Bundle: "quay.io/test:v1",
			},
			Parameters: []rwv1alpha1.RemediationWorkflowParameter{
				{Name: "TARGET", Type: "string", Required: true, Description: "Target"},
			},
		},
	}
	if workflowID != "" {
		rw.Status = rwv1alpha1.RemediationWorkflowStatus{
			WorkflowID:    workflowID,
			CatalogStatus: sharedtypes.CatalogStatusActive,
		}
	}
	return rw
}

func buildATForReconciler(name, namespace, specName string, workflowCount int) *atv1alpha1.ActionType {
	return &atv1alpha1.ActionType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: atv1alpha1.ActionTypeSpec{
			Name: specName,
			Description: atv1alpha1.ActionTypeDescription{
				What:      "Unit test action type",
				WhenToUse: "During tests",
			},
		},
		Status: atv1alpha1.ActionTypeStatus{
			Registered:          true,
			ActiveWorkflowCount: workflowCount,
		},
	}
}

var _ = Describe("RemediationWorkflow Finalizer Reconciler (#418)", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// UT-AW-418-001: Finalizer added to new RW
	// BR-WORKFLOW-006
	// ========================================
	Describe("UT-AW-418-001: Finalizer added to new RW", func() {
		It("should add the catalog-cleanup finalizer so RW cannot be silently deleted", func() {
			rw := buildRWForReconciler("rw-001", "default", "uuid-001", "RestartPod")
			scheme := newTestScheme()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
				Build()

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:   fakeClient,
				Log:      ctrl.Log.WithName("test"),
				DSClient: &mockWorkflowCatalogClient{},
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "rw-001", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(),
				"Reconciler should requeue after adding finalizer")

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "rw-001", Namespace: "default"}, updated)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updated, authwebhook.RWFinalizerName)).To(BeTrue(),
				"RW should have the catalog-cleanup finalizer")
		})
	})

	// ========================================
	// UT-AW-418-002: Deletion removes finalizer with zero DS calls (#1661 Change 8c)
	// BR-WORKFLOW-006
	// ========================================
	Describe("UT-AW-418-002: Deletion removes finalizer with zero DS calls", func() {
		It("should remove the finalizer without any DS round-trip (#1661 Change 8c)", func() {
			// #1661 Change 8c: DELETE is a true etcd removal -- there is no
			// DS-side "disabled" state left to notify. This finalizer's only
			// remaining job (verified by UT-AW-418-005/006 below) is the
			// ActionType activeWorkflowCount refresh.
			now := metav1.Now()
			rw := buildRWForReconciler("rw-002", "default", "uuid-002", "RestartPod")
			rw.DeletionTimestamp = &now
			rw.Finalizers = []string{authwebhook.RWFinalizerName}
			scheme := newTestScheme()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
				Build()

			disableCalled := false
			mockDS := &mockWorkflowCatalogClient{
				disableFn: func(_ context.Context, _, _, _ string) error {
					disableCalled = true
					return nil
				},
			}

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:   fakeClient,
				Log:      ctrl.Log.WithName("test"),
				DSClient: mockDS,
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "rw-002", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero(),
				"Should not requeue after successful deletion")
			Expect(disableCalled).To(BeFalse(),
				"#1661 Change 8c: DS is never called during finalizer-driven deletion")

			// After finalizer removal, the fake client GCs the object (DeletionTimestamp + no finalizers).
			// NotFound is the correct business outcome: the RW is fully deleted.
			updated := &rwv1alpha1.RemediationWorkflow{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "rw-002", Namespace: "default"}, updated)
			Expect(err).To(HaveOccurred(),
				"RW should be fully deleted (GC'd) after finalizer removal")
		})
	})

	// ========================================
	// UT-AW-418-005: AT activeWorkflowCount refreshed from a K8s-native list
	// (#1661 Change 8d: replaces the DS-backed GetActiveWorkflowCount call)
	// BR-WORKFLOW-007
	// ========================================
	Describe("UT-AW-418-005: AT activeWorkflowCount refreshed from live K8s state after deletion", func() {
		It("should update the parent AT's activeWorkflowCount to the number of remaining live RemediationWorkflow CRDs, with zero DS calls", func() {
			now := metav1.Now()
			rw := buildRWForReconciler("rw-005", "default", "uuid-005", "RestartPod")
			rw.DeletionTimestamp = &now
			rw.Finalizers = []string{authwebhook.RWFinalizerName}

			// A second, unrelated live RW referencing the same ActionType
			// proves the count reflects "what remains in K8s", not a stale
			// DS-side number.
			otherRW := buildRWForReconciler("rw-005-other", "default", "uuid-005-other", "RestartPod")

			at := buildATForReconciler("restart-pod", "default", "RestartPod", 2)
			scheme := newTestScheme()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw, otherRW, at).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}, &atv1alpha1.ActionType{}).
				Build()

			mockDS := &mockWorkflowCatalogClient{}

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:   fakeClient,
				Log:      ctrl.Log.WithName("test"),
				DSClient: mockDS,
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "rw-005", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())

			updatedAT := &atv1alpha1.ActionType{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "restart-pod", Namespace: "default"}, updatedAT)).To(Succeed())
			Expect(updatedAT.Status.ActiveWorkflowCount).To(Equal(1),
				"AT activeWorkflowCount should reflect the one remaining live RW (rw-005-other), read directly from K8s")
		})
	})

	// ========================================
	// UT-AW-418-006: AT count refresh failure does not block deletion
	// BR-WORKFLOW-007
	// ========================================
	Describe("UT-AW-418-006: AT count refresh failure does not block finalizer removal", func() {
		It("should remove finalizer even when the K8s-native AT count refresh fails", func() {
			now := metav1.Now()
			rw := buildRWForReconciler("rw-006", "default", "uuid-006", "RestartPod")
			rw.DeletionTimestamp = &now
			rw.Finalizers = []string{authwebhook.RWFinalizerName}
			scheme := newTestScheme()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
						return fmt.Errorf("simulated K8s API failure")
					},
				}).
				Build()

			mockDS := &mockWorkflowCatalogClient{}

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:   fakeClient,
				Log:      ctrl.Log.WithName("test"),
				DSClient: mockDS,
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "rw-006", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred(),
				"AT count refresh failure should not propagate as error")
			Expect(result.RequeueAfter).To(BeZero(),
				"Should not requeue — AT count is best-effort")

			// After finalizer removal, the fake client GCs the object.
			updated := &rwv1alpha1.RemediationWorkflow{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{Name: "rw-006", Namespace: "default"}, updated)
			Expect(getErr).To(HaveOccurred(),
				"RW should be fully deleted despite AT count refresh failure")
		})
	})

	// ========================================
	// UT-AW-418-NOTFOUND: Reconcile of deleted RW returns no error
	// ========================================
	Describe("UT-AW-418-NOTFOUND: Reconcile returns no error for already-deleted RW", func() {
		It("should return empty result when RW no longer exists in K8s", func() {
			scheme := newTestScheme()
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:   fakeClient,
				Log:      ctrl.Log.WithName("test"),
				DSClient: &mockWorkflowCatalogClient{},
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	// ========================================
	// UT-AW-418-NOFINALIZER: Deletion without our finalizer is a no-op
	// ========================================
	Describe("UT-AW-418-NOFINALIZER: Deletion without catalog-cleanup finalizer is no-op", func() {
		It("should return empty result without DS call when our finalizer is absent", func() {
			now := metav1.Now()
			rw := buildRWForReconciler("rw-nofin", "default", "uuid-nofin", "RestartPod")
			rw.DeletionTimestamp = &now
			// Different finalizer (not ours) — required for fake client to accept DeletionTimestamp
			rw.Finalizers = []string{"some.other.io/finalizer"}
			scheme := newTestScheme()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				Build()

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:   fakeClient,
				Log:      ctrl.Log.WithName("test"),
				DSClient: &mockWorkflowCatalogClient{},
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "rw-nofin", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})

// Verify interface compliance at compile time.
var _ authwebhook.WorkflowCatalogClient = &mockWorkflowCatalogClient{}
