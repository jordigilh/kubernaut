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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// callRecord tracks the order and type of DS API calls.
type callRecord struct {
	Type string // "CreateActionType" or "CreateWorkflowInline"
	Name string // action type name or workflow CRD name
}

// mockStartupDSClient records calls from the startup reconciler.
type mockStartupDSClient struct {
	mu             sync.Mutex
	calls          []callRecord
	failCount      int // number of times to fail before succeeding
	currentFails   int
	atResult       *authwebhook.ActionTypeRegistrationResult
	rwResult       *authwebhook.WorkflowRegistrationResult
	alwaysFail     bool
}

func (m *mockStartupDSClient) CreateActionType(_ context.Context, name string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeRegistrationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.alwaysFail || m.currentFails < m.failCount {
		m.currentFails++
		return nil, fmt.Errorf("connection refused")
	}
	m.calls = append(m.calls, callRecord{Type: "CreateActionType", Name: name})
	if m.atResult != nil {
		return m.atResult, nil
	}
	return &authwebhook.ActionTypeRegistrationResult{ActionType: name, Status: "created"}, nil
}

func (m *mockStartupDSClient) UpdateActionType(_ context.Context, _ string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeUpdateResult, error) {
	return &authwebhook.ActionTypeUpdateResult{}, nil
}

func (m *mockStartupDSClient) DisableActionType(_ context.Context, _ string, _ string) (*authwebhook.ActionTypeDisableResult, error) {
	return &authwebhook.ActionTypeDisableResult{}, nil
}

func (m *mockStartupDSClient) ForceDisableActionType(_ context.Context, _ string, _ string, _ []string) (*authwebhook.ActionTypeDisableResult, error) {
	return &authwebhook.ActionTypeDisableResult{}, nil
}

func (m *mockStartupDSClient) CreateWorkflowInline(_ context.Context, content, source, registeredBy string) (*authwebhook.WorkflowRegistrationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.alwaysFail || m.currentFails < m.failCount {
		m.currentFails++
		return nil, fmt.Errorf("connection refused")
	}
	m.calls = append(m.calls, callRecord{Type: "CreateWorkflowInline", Name: source})
	if m.rwResult != nil {
		return m.rwResult, nil
	}
	return &authwebhook.WorkflowRegistrationResult{
		WorkflowID:   "wf-uuid-001",
		WorkflowName: "test-wf",
		Version:      "1.0.0",
		Status:       "Active",
	}, nil
}

func (m *mockStartupDSClient) DisableWorkflow(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockStartupDSClient) GetActiveWorkflowCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockStartupDSClient) getCalls() []callRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]callRecord, len(m.calls))
	copy(out, m.calls)
	return out
}

func makeActionTypeCRD(name, specName string) *atv1alpha1.ActionType {
	return &atv1alpha1.ActionType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: atv1alpha1.ActionTypeSpec{
			Name: specName,
			Description: atv1alpha1.ActionTypeDescription{
				What:      "Test action type",
				WhenToUse: "Testing",
			},
		},
	}
}

func makeWorkflowCRD(name, actionType string) *rwv1alpha1.RemediationWorkflow {
	return &rwv1alpha1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: rwv1alpha1.RemediationWorkflowSpec{
			Version:    "1.0.0",
			ActionType: actionType,
			Description: rwv1alpha1.RemediationWorkflowDescription{
				What:      "Test workflow",
				WhenToUse: "Testing",
			},
		},
	}
}

var _ = Describe("StartupReconciler (#548)", func() {

	// ========================================
	// UT-AW-548-001: Registers ActionType CRDs with DS
	// ========================================
	Describe("UT-AW-548-001: Startup reconciler registers ActionType CRDs", func() {
		It("should call CreateActionType for each ActionType CRD with registeredBy system:authwebhook-startup", func() {
			scheme := newTestScheme()
			at1 := makeActionTypeCRD("at-1", "ScaleMemory")
			at2 := makeActionTypeCRD("at-2", "RestartPod")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at1, at2).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:    k8sClient,
				DSWorkflow:   mockDS,
				DSActionType: mockDS,
				Logger:       ctrl.Log.WithName("test"),
				Timeout:      10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			calls := mockDS.getCalls()
			atCalls := filterCalls(calls, "CreateActionType")
			Expect(atCalls).To(HaveLen(2), "should register 2 ActionType CRDs")

			names := []string{atCalls[0].Name, atCalls[1].Name}
			Expect(names).To(ContainElements("ScaleMemory", "RestartPod"))
		})
	})

	// ========================================
	// UT-AW-548-002: Registers RemediationWorkflow CRDs with DS
	// ========================================
	Describe("UT-AW-548-002: Startup reconciler registers RemediationWorkflow CRDs", func() {
		It("should call CreateWorkflowInline for each RemediationWorkflow CRD", func() {
			scheme := newTestScheme()
			rw1 := makeWorkflowCRD("wf-1", "ScaleMemory")
			rw2 := makeWorkflowCRD("wf-2", "RestartPod")
			rw3 := makeWorkflowCRD("wf-3", "RollbackDeployment")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw1, rw2, rw3).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:    k8sClient,
				DSWorkflow:   mockDS,
				DSActionType: mockDS,
				Logger:       ctrl.Log.WithName("test"),
				Timeout:      10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			calls := mockDS.getCalls()
			rwCalls := filterCalls(calls, "CreateWorkflowInline")
			Expect(rwCalls).To(HaveLen(3), "should register 3 RemediationWorkflow CRDs")
		})
	})

	// ========================================
	// UT-AW-548-003: ActionTypes registered before Workflows
	// ========================================
	Describe("UT-AW-548-003: ActionTypes registered before Workflows (ordering)", func() {
		It("should complete all CreateActionType calls before any CreateWorkflowInline call", func() {
			scheme := newTestScheme()
			at1 := makeActionTypeCRD("at-1", "ScaleMemory")
			rw1 := makeWorkflowCRD("wf-1", "ScaleMemory")
			rw2 := makeWorkflowCRD("wf-2", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at1, rw1, rw2).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:    k8sClient,
				DSWorkflow:   mockDS,
				DSActionType: mockDS,
				Logger:       ctrl.Log.WithName("test"),
				Timeout:      10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			calls := mockDS.getCalls()
			Expect(calls).To(HaveLen(3))

			lastATIndex := -1
			firstRWIndex := len(calls)
			for i, c := range calls {
				if c.Type == "CreateActionType" {
					lastATIndex = i
				}
				if c.Type == "CreateWorkflowInline" && i < firstRWIndex {
					firstRWIndex = i
				}
			}
			Expect(lastATIndex).To(BeNumerically("<", firstRWIndex),
				"all ActionType calls must precede all Workflow calls, got calls: %v", calls)
		})
	})

	// ========================================
	// UT-AW-548-004: CRD status updated after registration
	// ========================================
	Describe("UT-AW-548-004: CRD status updated after registration", func() {
		It("should set catalogStatus=Active, workflowId, and registeredBy on RW CRD status", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-status", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{
				rwResult: &authwebhook.WorkflowRegistrationResult{
					WorkflowID:   "abc-123-det",
					WorkflowName: "wf-status",
					Version:      "1.0.0",
					Status:       "Active",
				},
			}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:    k8sClient,
				DSWorkflow:   mockDS,
				DSActionType: mockDS,
				Logger:       ctrl.Log.WithName("test"),
				Timeout:      10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-status"), updated)).To(Succeed())

			Expect(updated.Status.WorkflowID).To(Equal("abc-123-det"),
				"workflowId should be populated from DS response")
			Expect(string(updated.Status.CatalogStatus)).To(Equal("Active"),
				"catalogStatus should be Active")
			Expect(updated.Status.RegisteredBy).To(Equal("system:authwebhook-startup"),
				"registeredBy should identify the startup reconciler")
			Expect(updated.Status.RegisteredAt).NotTo(BeNil(),
				"registeredAt should be set")
		})
	})

	// ========================================
	// UT-AW-548-005: Retries with backoff when DS unavailable
	// ========================================
	Describe("UT-AW-548-005: Retries with backoff when DS unavailable", func() {
		It("should retry until DS becomes available then complete successfully", func() {
			scheme := newTestScheme()
			at := makeActionTypeCRD("at-retry", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{failCount: 3}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        30 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			calls := mockDS.getCalls()
			atCalls := filterCalls(calls, "CreateActionType")
			Expect(atCalls).To(HaveLen(1), "should eventually register the ActionType")
		})
	})

	// ========================================
	// UT-AW-548-006: Returns error when DS never responds
	// ========================================
	Describe("UT-AW-548-006: Returns error when DS never responds (blocks readiness)", func() {
		It("should return an error after the timeout expires", func() {
			scheme := newTestScheme()
			at := makeActionTypeCRD("at-timeout", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{alwaysFail: true}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        500 * time.Millisecond,
				InitialBackoff: 50 * time.Millisecond,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).To(HaveOccurred(), "should return error when DS is never available")
			Expect(err.Error()).To(ContainSubstring("unavailable"),
				"error should indicate DS unavailability")
		})
	})

	// ========================================
	// UT-AW-548-007: Empty CRD lists handled gracefully
	// ========================================
	Describe("UT-AW-548-007: Empty CRD lists handled gracefully", func() {
		It("should complete with no DS calls when no CRDs exist", func() {
			scheme := newTestScheme()

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:    k8sClient,
				DSWorkflow:   mockDS,
				DSActionType: mockDS,
				Logger:       ctrl.Log.WithName("test"),
				Timeout:      10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			calls := mockDS.getCalls()
			Expect(calls).To(BeEmpty(), "no DS calls for empty CRD lists")
		})
	})

	// ========================================
	// UT-AW-548-008: Idempotent re-registration
	// ========================================
	Describe("UT-AW-548-008: Idempotent re-registration of already-synced CRDs", func() {
		It("should update CRD status and complete without error for already-registered CRDs", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-idem", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &mockStartupDSClient{
				rwResult: &authwebhook.WorkflowRegistrationResult{
					WorkflowID:        "idem-uuid-001",
					WorkflowName:      "wf-idem",
					Version:           "1.0.0",
					Status:            "Active",
					PreviouslyExisted: true,
				},
			}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:    k8sClient,
				DSWorkflow:   mockDS,
				DSActionType: mockDS,
				Logger:       ctrl.Log.WithName("test"),
				Timeout:      10 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-idem"), updated)).To(Succeed())
			Expect(updated.Status.WorkflowID).To(Equal("idem-uuid-001"))
			Expect(string(updated.Status.CatalogStatus)).To(Equal("Active"))
		})
	})
})

func filterCalls(calls []callRecord, callType string) []callRecord {
	var filtered []callRecord
	for _, c := range calls {
		if c.Type == callType {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func nsName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}
