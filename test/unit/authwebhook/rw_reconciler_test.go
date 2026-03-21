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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// mockATCounter implements authwebhook.ActionTypeWorkflowCounter for unit tests.
type mockATCounter struct {
	getCountFn func(ctx context.Context, actionType string) (int, error)
	callCount  int
}

func (m *mockATCounter) GetActiveWorkflowCount(ctx context.Context, actionType string) (int, error) {
	m.callCount++
	if m.getCountFn != nil {
		return m.getCountFn(ctx, actionType)
	}
	return 0, nil
}

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
				Component:   "pod",
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
			CatalogStatus: "active",
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
	// UT-AW-418-002: Deletion disables DS workflow and removes finalizer
	// BR-WORKFLOW-006
	// ========================================
	Describe("UT-AW-418-002: Deletion disables DS workflow and removes finalizer", func() {
		It("should call DS DisableWorkflow and remove finalizer on successful disable", func() {
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

			var capturedID string
			mockDS := &mockWorkflowCatalogClient{
				disableFn: func(_ context.Context, workflowID, _, _ string) error {
					capturedID = workflowID
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
			Expect(capturedID).To(Equal("uuid-002"),
				"DS should receive the correct WorkflowID")

			// After finalizer removal, the fake client GCs the object (DeletionTimestamp + no finalizers).
			// NotFound is the correct business outcome: the RW is fully deleted.
			updated := &rwv1alpha1.RemediationWorkflow{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "rw-002", Namespace: "default"}, updated)
			Expect(err).To(HaveOccurred(),
				"RW should be fully deleted (GC'd) after finalizer removal")
		})
	})

	// ========================================
	// UT-AW-418-003: DS failure causes requeue
	// BR-WORKFLOW-006
	// ========================================
	Describe("UT-AW-418-003: DS server error causes requeue", func() {
		It("should requeue after 5s and keep finalizer when DS returns a server error", func() {
			now := metav1.Now()
			rw := buildRWForReconciler("rw-003", "default", "uuid-003", "RestartPod")
			rw.DeletionTimestamp = &now
			rw.Finalizers = []string{authwebhook.RWFinalizerName}
			scheme := newTestScheme()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockWorkflowCatalogClient{
				disableFn: func(_ context.Context, _, _, _ string) error {
					return fmt.Errorf("data storage DisableWorkflow failed: server error: database timeout")
				},
			}

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:   fakeClient,
				Log:      ctrl.Log.WithName("test"),
				DSClient: mockDS,
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "rw-003", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred(),
				"Error should not propagate — controller uses RequeueAfter instead")
			Expect(result.RequeueAfter).To(Equal(5*time.Second),
				"Should requeue after 5s to retry DS disable")

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "rw-003", Namespace: "default"}, updated)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updated, authwebhook.RWFinalizerName)).To(BeTrue(),
				"Finalizer should remain when DS disable fails with a server error")
		})
	})

	// ========================================
	// UT-AW-469-004: Connection error during deletion removes finalizer
	// Issue #469 — DS may be unreachable during helm uninstall; don't block CRD deletion
	// ========================================
	Describe("UT-AW-469-004: Connection error during deletion proceeds with finalizer removal", func() {
		It("should remove finalizer when DS returns connection refused", func() {
			now := metav1.Now()
			rw := buildRWForReconciler("rw-469-conn", "default", "uuid-469", "RestartPod")
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
					return fmt.Errorf("data storage DisableWorkflow failed: dial tcp 10.96.0.42:8080: connection refused")
				},
			}

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:   fakeClient,
				Log:      ctrl.Log.WithName("test"),
				DSClient: mockDS,
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "rw-469-conn", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero(),
				"Connection errors during deletion should NOT cause requeue")
			Expect(disableCalled).To(BeTrue(),
				"DS DisableWorkflow should still be attempted")

			updated := &rwv1alpha1.RemediationWorkflow{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{Name: "rw-469-conn", Namespace: "default"}, updated)
			Expect(getErr).To(HaveOccurred(),
				"RW should be deleted after finalizer removal (fake client GC)")
		})
	})

	// ========================================
	// UT-AW-418-004: Empty WorkflowID skips DS disable
	// BR-WORKFLOW-006
	// ========================================
	Describe("UT-AW-418-004: Empty WorkflowID skips DS disable", func() {
		It("should remove finalizer without calling DS when WorkflowID is empty", func() {
			now := metav1.Now()
			rw := buildRWForReconciler("rw-004", "default", "", "RestartPod")
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
				NamespacedName: types.NamespacedName{Name: "rw-004", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(disableCalled).To(BeFalse(),
				"DS DisableWorkflow should NOT be called when WorkflowID is empty")

			// After finalizer removal, the fake client GCs the object.
			updated := &rwv1alpha1.RemediationWorkflow{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{Name: "rw-004", Namespace: "default"}, updated)
			Expect(getErr).To(HaveOccurred(),
				"RW should be fully deleted after finalizer removal")
		})
	})

	// ========================================
	// UT-AW-418-005: AT activeWorkflowCount refreshed from DS
	// BR-WORKFLOW-007
	// ========================================
	Describe("UT-AW-418-005: AT activeWorkflowCount refreshed from DS after deletion", func() {
		It("should update the parent AT's activeWorkflowCount to the value from DS", func() {
			now := metav1.Now()
			rw := buildRWForReconciler("rw-005", "default", "uuid-005", "RestartPod")
			rw.DeletionTimestamp = &now
			rw.Finalizers = []string{authwebhook.RWFinalizerName}

			at := buildATForReconciler("restart-pod", "default", "RestartPod", 1)
			scheme := newTestScheme()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw, at).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}, &atv1alpha1.ActionType{}).
				Build()

			mockDS := &mockWorkflowCatalogClient{}
			mockCounter := &mockATCounter{
				getCountFn: func(_ context.Context, actionType string) (int, error) {
					Expect(actionType).To(Equal("RestartPod"))
					return 0, nil
				},
			}

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:    fakeClient,
				Log:       ctrl.Log.WithName("test"),
				DSClient:  mockDS,
				ATCounter: mockCounter,
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "rw-005", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(mockCounter.callCount).To(Equal(1),
				"GetActiveWorkflowCount should be called exactly once")

			updatedAT := &atv1alpha1.ActionType{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "restart-pod", Namespace: "default"}, updatedAT)).To(Succeed())
			Expect(updatedAT.Status.ActiveWorkflowCount).To(Equal(0),
				"AT activeWorkflowCount should be updated to 0 (value from DS)")
		})
	})

	// ========================================
	// UT-AW-418-006: AT count refresh failure does not block deletion
	// BR-WORKFLOW-007
	// ========================================
	Describe("UT-AW-418-006: AT count refresh failure does not block finalizer removal", func() {
		It("should remove finalizer even when AT count refresh fails", func() {
			now := metav1.Now()
			rw := buildRWForReconciler("rw-006", "default", "uuid-006", "RestartPod")
			rw.DeletionTimestamp = &now
			rw.Finalizers = []string{authwebhook.RWFinalizerName}
			scheme := newTestScheme()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockWorkflowCatalogClient{}
			mockCounter := &mockATCounter{
				getCountFn: func(_ context.Context, _ string) (int, error) {
					return 0, fmt.Errorf("DS GetActiveWorkflowCount unavailable")
				},
			}

			reconciler := &authwebhook.RemediationWorkflowReconciler{
				Client:    fakeClient,
				Log:       ctrl.Log.WithName("test"),
				DSClient:  mockDS,
				ATCounter: mockCounter,
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
				NamespacedName: types.NamespacedName{Name: "rw-nofin", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(disableCalled).To(BeFalse(),
				"DS should not be called when our finalizer is not present")
		})
	})
})

// Verify interface compliance at compile time.
var _ authwebhook.ActionTypeWorkflowCounter = &mockATCounter{}
var _ authwebhook.WorkflowCatalogClient = &mockWorkflowCatalogClient{}

// Silence unused import warnings.
var _ = client.ObjectKey{}
