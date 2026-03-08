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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ========================================
// Mocks
// ========================================

// mockWorkflowCatalogClient implements authwebhook.WorkflowCatalogClient for unit tests.
type mockWorkflowCatalogClient struct {
	createFn  func(ctx context.Context, content, source, registeredBy string) (*authwebhook.WorkflowRegistrationResult, error)
	disableFn func(ctx context.Context, workflowID, reason, updatedBy string) error
}

func (m *mockWorkflowCatalogClient) CreateWorkflowInline(ctx context.Context, content, source, registeredBy string) (*authwebhook.WorkflowRegistrationResult, error) {
	if m.createFn != nil {
		return m.createFn(ctx, content, source, registeredBy)
	}
	return &authwebhook.WorkflowRegistrationResult{
		WorkflowID:   "550e8400-e29b-41d4-a716-446655440000",
		WorkflowName: "test-workflow",
		Version:      "1.0.0",
		Status:       "active",
	}, nil
}

func (m *mockWorkflowCatalogClient) DisableWorkflow(ctx context.Context, workflowID, reason, updatedBy string) error {
	if m.disableFn != nil {
		return m.disableFn(ctx, workflowID, reason, updatedBy)
	}
	return nil
}

// MockAuditStoreRW captures audit events for validation.
// Named with RW suffix to avoid collision with MockAuditStore in other test files.
type MockAuditStoreRW struct {
	StoredEvents []*ogenclient.AuditEventRequest
	StoreError   error
}

func (m *MockAuditStoreRW) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	if m.StoreError != nil {
		return m.StoreError
	}
	m.StoredEvents = append(m.StoredEvents, event)
	return nil
}

func (m *MockAuditStoreRW) Flush(_ context.Context) error { return nil }
func (m *MockAuditStoreRW) Close() error                  { return nil }

// ========================================
// Test Helpers
// ========================================

const testUserEmail = "admin@example.com"
const testUserUID = "uid-12345"

func buildRemediationWorkflow(name, namespace string) *rwv1alpha1.RemediationWorkflow {
	return &rwv1alpha1.RemediationWorkflow{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubernaut.ai/v1alpha1",
			Kind:       "RemediationWorkflow",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "crd-uid-001",
		},
		Spec: rwv1alpha1.RemediationWorkflowSpec{
			Metadata: rwv1alpha1.RemediationWorkflowMetadata{
				WorkflowID: name,
				Version:    "1.0.0",
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "Test workflow",
					WhenToUse: "During tests",
				},
			},
			ActionType: "ScaleMemory",
			Labels: rwv1alpha1.RemediationWorkflowLabels{
				Severity:    []string{"critical"},
				Environment: []string{"production"},
				Component:   "pod",
				Priority:    "P1",
			},
			Execution: rwv1alpha1.RemediationWorkflowExecution{
				Engine: "job",
				Bundle: "quay.io/kubernaut/workflows/test:v1.0.0@sha256:abc123",
			},
			Parameters: []rwv1alpha1.RemediationWorkflowParameter{
				{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target"},
			},
		},
	}
}

func buildRemediationWorkflowWithStatus(name, namespace, workflowID string) *rwv1alpha1.RemediationWorkflow {
	rw := buildRemediationWorkflow(name, namespace)
	rw.Status = rwv1alpha1.RemediationWorkflowStatus{
		WorkflowID:    workflowID,
		CatalogStatus: "active",
		RegisteredBy:  testUserEmail,
	}
	return rw
}

func buildCreateAdmissionRequest(rw *rwv1alpha1.RemediationWorkflow) admission.Request {
	rwJSON, _ := json.Marshal(rw)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "admission-create-001",
			Kind: metav1.GroupVersionKind{
				Group:   "kubernaut.ai",
				Version: "v1alpha1",
				Kind:    "RemediationWorkflow",
			},
			Name:      rw.Name,
			Namespace: rw.Namespace,
			Operation: admissionv1.Create,
			UserInfo: authv1.UserInfo{
				Username: testUserEmail,
				UID:      testUserUID,
				Groups:   []string{"system:masters"},
			},
			Object: runtime.RawExtension{Raw: rwJSON},
		},
	}
}

func buildDeleteAdmissionRequest(rw *rwv1alpha1.RemediationWorkflow) admission.Request {
	rwJSON, _ := json.Marshal(rw)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "admission-delete-001",
			Kind: metav1.GroupVersionKind{
				Group:   "kubernaut.ai",
				Version: "v1alpha1",
				Kind:    "RemediationWorkflow",
			},
			Name:      rw.Name,
			Namespace: rw.Namespace,
			Operation: admissionv1.Delete,
			UserInfo: authv1.UserInfo{
				Username: testUserEmail,
				UID:      testUserUID,
				Groups:   []string{"system:masters"},
			},
			OldObject: runtime.RawExtension{Raw: rwJSON},
		},
	}
}

func buildUpdateAdmissionRequest(rw *rwv1alpha1.RemediationWorkflow) admission.Request {
	rwJSON, _ := json.Marshal(rw)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "admission-update-001",
			Kind: metav1.GroupVersionKind{
				Group:   "kubernaut.ai",
				Version: "v1alpha1",
				Kind:    "RemediationWorkflow",
			},
			Name:      rw.Name,
			Namespace: rw.Namespace,
			Operation: admissionv1.Update,
			UserInfo: authv1.UserInfo{
				Username: testUserEmail,
				UID:      testUserUID,
			},
			Object:    runtime.RawExtension{Raw: rwJSON},
			OldObject: runtime.RawExtension{Raw: rwJSON},
		},
	}
}

func fakeK8sKey(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

// ========================================
// Tests
// ========================================

var _ = Describe("RemediationWorkflow Admission Handler (#299)", func() {
	var (
		ctx       context.Context
		handler   *authwebhook.RemediationWorkflowHandler
		mockDS    *mockWorkflowCatalogClient
		mockAudit *MockAuditStoreRW
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockDS = &mockWorkflowCatalogClient{}
		mockAudit = &MockAuditStoreRW{}
		handler = authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, nil)
	})

	// ========================================
	// UT-AW-299-001: CREATE → DS registration → Allowed
	// ========================================
	Describe("UT-AW-299-001: CREATE handler forwards CRD spec to DS", func() {
		It("should return Allowed and call DS CreateWorkflowInline", func() {
			// Arrange
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			var capturedContent string
			var capturedSource string
			var capturedBy string
			mockDS.createFn = func(_ context.Context, content, source, registeredBy string) (*authwebhook.WorkflowRegistrationResult, error) {
				capturedContent = content
				capturedSource = source
				capturedBy = registeredBy
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:   "550e8400-e29b-41d4-a716-446655440000",
					WorkflowName: "scale-memory",
					Version:      "1.0.0",
					Status:       "active",
				}, nil
			}

			admReq := buildCreateAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue(),
				"CREATE should be Allowed when DS returns success")
			Expect(capturedContent).ToNot(BeEmpty(),
				"DS client should receive the CRD content")
			Expect(capturedSource).To(Equal("crd"),
				"Source should be 'crd' for webhook-originated registrations")
			Expect(capturedBy).To(Equal(testUserEmail),
				"RegisteredBy should be the K8s user from admission request")
		})
	})

	// ========================================
	// UT-AW-299-002: DELETE → DS disable → Allowed
	// ========================================
	Describe("UT-AW-299-002: DELETE handler disables workflow in DS", func() {
		It("should return Allowed and call DS DisableWorkflow", func() {
			// Arrange
			rw := buildRemediationWorkflowWithStatus("scale-memory", "kubernaut-system", "550e8400-e29b-41d4-a716-446655440000")
			var capturedID string
			var capturedReason string
			var capturedBy string
			mockDS.disableFn = func(_ context.Context, workflowID, reason, updatedBy string) error {
				capturedID = workflowID
				capturedReason = reason
				capturedBy = updatedBy
				return nil
			}

			admReq := buildDeleteAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue(),
				"DELETE should be Allowed")
			Expect(capturedID).To(Equal("550e8400-e29b-41d4-a716-446655440000"),
				"DS should receive the workflowId from CRD status")
			Expect(capturedReason).To(ContainSubstring("CRD deleted"),
				"Reason should indicate CRD deletion")
			Expect(capturedBy).To(Equal(testUserEmail),
				"UpdatedBy should be the K8s user")
		})
	})

	// ========================================
	// UT-AW-299-003: CREATE denied when DS unreachable
	// ========================================
	Describe("UT-AW-299-003: CREATE denied when DS is unreachable", func() {
		It("should return Denied with DS connectivity error", func() {
			// Arrange
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				return nil, fmt.Errorf("connection refused: data storage service unavailable")
			}

			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildCreateAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeFalse(),
				"CREATE should be Denied when DS is unreachable (fail-closed)")
			Expect(resp.Result).To(HaveField("Message", ContainSubstring("data storage")),
				"Denial message should help diagnose DS connectivity issue")
		})
	})

	// ========================================
	// UT-AW-299-004: Re-CREATE of deleted workflow re-enables in DS
	// ========================================
	Describe("UT-AW-299-004: CREATE re-enables previously deleted workflow", func() {
		It("should return Allowed when DS indicates re-enable", func() {
			// Arrange
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:        "550e8400-e29b-41d4-a716-446655440000",
					WorkflowName:      "scale-memory",
					Version:           "1.0.0",
					Status:            "active",
					PreviouslyExisted: true,
				}, nil
			}

			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildCreateAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue(),
				"Re-CREATE should be Allowed when DS re-enables the workflow")
		})
	})

	// ========================================
	// UT-AW-299-005: CREATE audit event with actor attribution
	// ========================================
	Describe("UT-AW-299-005: CREATE audit event with actor attribution", func() {
		It("should emit remediationworkflow.admitted.create audit event", func() {
			// Arrange
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildCreateAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert: admission succeeds and audit event is captured
			Expect(resp.Allowed).To(BeTrue())
			Expect(mockAudit.StoredEvents).To(HaveLen(1),
				"Exactly 1 audit event should be emitted on successful CREATE")

			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("remediationworkflow.admitted.create"))
			Expect(string(event.EventCategory)).To(Equal("webhook"))
			Expect(event.EventAction).To(Equal("admitted"))
			Expect(string(event.EventOutcome)).To(Equal("success"))
			Expect(event.ActorID.Value).To(Equal(testUserEmail))
			Expect(event.ResourceType.Value).To(Equal("RemediationWorkflow"))
		})
	})

	// ========================================
	// UT-AW-299-006: DELETE audit event with actor attribution
	// ========================================
	Describe("UT-AW-299-006: DELETE audit event with actor attribution", func() {
		It("should emit remediationworkflow.admitted.delete audit event", func() {
			// Arrange
			rw := buildRemediationWorkflowWithStatus("scale-memory", "kubernaut-system", "550e8400-e29b-41d4-a716-446655440000")
			admReq := buildDeleteAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue())
			Expect(mockAudit.StoredEvents).To(HaveLen(1),
				"Exactly 1 audit event should be emitted on DELETE")

			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("remediationworkflow.admitted.delete"))
			Expect(string(event.EventCategory)).To(Equal("webhook"))
			Expect(event.EventAction).To(Equal("admitted"))
			Expect(string(event.EventOutcome)).To(Equal("success"))
			Expect(event.ActorID.Value).To(Equal(testUserEmail))
		})
	})

	// ========================================
	// UT-AW-299-007: CREATE denied emits denied audit event
	// ========================================
	Describe("UT-AW-299-007: CREATE denied emits denied audit event", func() {
		It("should emit remediationworkflow.admitted.denied audit event", func() {
			// Arrange
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				return nil, fmt.Errorf("connection refused")
			}

			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildCreateAdmissionRequest(rw)

			// Act
			handler.Handle(ctx, admReq)

			// Assert
			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("remediationworkflow.admitted.denied"))
			Expect(string(event.EventOutcome)).To(Equal("failure"))
		})
	})

	// ========================================
	// UT-AW-299-008: User extraction from AdmissionReview.userInfo
	// ========================================
	Describe("UT-AW-299-008: User extraction from AdmissionReview.userInfo", func() {
		It("should extract username, UID, and groups from admission request", func() {
			// Arrange
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildCreateAdmissionRequest(rw)
			admReq.UserInfo.Groups = []string{"system:masters", "kubernaut-admins"}

			var capturedBy string
			mockDS.createFn = func(_ context.Context, _, _, registeredBy string) (*authwebhook.WorkflowRegistrationResult, error) {
				capturedBy = registeredBy
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID: "test-id",
					Status:     "active",
				}, nil
			}

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue())
			Expect(capturedBy).To(Equal(testUserEmail),
				"Should pass K8s username to DS as registeredBy")
		})
	})

	// ========================================
	// UT-AW-299-009: UPDATE operations pass through without DS call
	// ========================================
	Describe("UT-AW-299-009: UPDATE operations ignored", func() {
		It("should allow UPDATE without calling DS", func() {
			// Arrange
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)

			createCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				createCalled = true
				return nil, fmt.Errorf("should not be called")
			}

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue(),
				"UPDATE should be Allowed without DS interaction (spec is idempotent)")
			Expect(createCalled).To(BeFalse(),
				"DS client should NOT be called for UPDATE operations")
		})
	})

	// ========================================
	// UT-AW-299-010: DELETE always allowed even if DS disable fails
	// ========================================
	Describe("UT-AW-299-010: DELETE always allowed even if DS disable fails", func() {
		It("should return Allowed even when DS disable returns error", func() {
			// Arrange
			mockDS.disableFn = func(_ context.Context, _, _, _ string) error {
				return fmt.Errorf("connection refused: data storage unavailable")
			}

			rw := buildRemediationWorkflowWithStatus("scale-memory", "kubernaut-system", "550e8400-e29b-41d4-a716-446655440000")
			admReq := buildDeleteAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue(),
				"DELETE should ALWAYS be Allowed to prevent GitOps drift")
		})
	})

	// ========================================
	// UT-AW-299-011: CREATE includes source and registeredBy
	// ========================================
	Describe("UT-AW-299-011: CREATE includes source and registeredBy in DS request", func() {
		It("should pass source='crd' and registeredBy from userInfo to DS", func() {
			// Arrange
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			var capturedSource string
			var capturedBy string
			mockDS.createFn = func(_ context.Context, _, source, registeredBy string) (*authwebhook.WorkflowRegistrationResult, error) {
				capturedSource = source
				capturedBy = registeredBy
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID: "test-id",
					Status:     "active",
				}, nil
			}

			admReq := buildCreateAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue())
			Expect(capturedSource).To(Equal("crd"))
			Expect(capturedBy).To(Equal(testUserEmail))
		})
	})

	// ========================================
	// UT-AW-299-012: DELETE with empty Status.WorkflowID skips DS disable
	// ========================================
	Describe("UT-AW-299-012: DELETE with empty status (production scenario)", func() {
		It("should allow DELETE and skip DS disable when WorkflowID is empty", func() {
			rw := buildRemediationWorkflow("test-rw", "kubernaut-system")

			mockDS.disableFn = func(_ context.Context, _, _, _ string) error {
				Fail("DisableWorkflow should NOT be called when WorkflowID is empty")
				return nil
			}

			admReq := buildDeleteAdmissionRequest(rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"DELETE should be Allowed even without a WorkflowID in status")
			Expect(resp.Result.Message).To(ContainSubstring("no workflowId"),
				"Response should indicate the skip reason")

			Expect(mockAudit.StoredEvents).To(HaveLen(1),
				"Audit event should still be emitted for the DELETE")
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal(authwebhook.EventTypeRWAdmittedDelete))
		})
	})

	// ========================================
	// UT-AW-299-013: CREATE populates CRD .status via async k8sClient update
	// ========================================
	Describe("UT-AW-299-013: CREATE updates CRD status asynchronously", func() {
		It("should populate .status with DS registration result via k8sClient.Status().Update()", func() {
			rw := buildRemediationWorkflow("scale-memory-status", "kubernaut-system")

			scheme := runtime.NewScheme()
			_ = rwv1alpha1.AddToScheme(scheme)

			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
				Build()

			handlerWithK8s := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)

			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:        "uuid-status-test-001",
					WorkflowName:      "scale-memory-status",
					Version:           "1.0.0",
					Status:            "active",
					PreviouslyExisted: false,
				}, nil
			}

			admReq := buildCreateAdmissionRequest(rw)
			resp := handlerWithK8s.Handle(ctx, admReq)
			Expect(resp.Allowed).To(BeTrue())

			// Wait for async goroutine to complete
			Eventually(func() string {
				updated := &rwv1alpha1.RemediationWorkflow{}
				err := fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "scale-memory-status"), updated)
				if err != nil {
					return ""
				}
				return updated.Status.WorkflowID
			}, 5*time.Second, 100*time.Millisecond).Should(Equal("uuid-status-test-001"))

			// Verify all status fields
			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "scale-memory-status"), updated)).To(Succeed())
			Expect(updated.Status.CatalogStatus).To(Equal("active"))
			Expect(updated.Status.RegisteredBy).To(Equal(testUserEmail))
			Expect(updated.Status.RegisteredAt).NotTo(BeNil())
			Expect(updated.Status.PreviouslyExisted).To(BeFalse())
		})
	})
})
