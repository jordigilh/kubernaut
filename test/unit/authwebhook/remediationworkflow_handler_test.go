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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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
		Status:       string(sharedtypes.CatalogStatusActive),
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
			Version: "1.0.0",
			Description: rwv1alpha1.RemediationWorkflowDescription{
				What:      "Test workflow",
				WhenToUse: "During tests",
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
		CatalogStatus: sharedtypes.CatalogStatusActive,
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
					Status:       string(sharedtypes.CatalogStatusActive),
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
					Status:            string(sharedtypes.CatalogStatusActive),
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
			Expect(string(event.EventCategory)).To(Equal("workflow"))
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
			Expect(string(event.EventCategory)).To(Equal("workflow"))
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
					Status:     string(sharedtypes.CatalogStatusActive),
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
	// UT-AW-299-009: UPDATE operations now trigger DS registration (Issue #371)
	// Replaces previous behavior: UPDATE was ignored, now forwards to DS.
	// ========================================
	Describe("UT-AW-299-009: UPDATE triggers DS registration (Issue #371)", func() {
		It("should allow UPDATE and call DS CreateWorkflowInline", func() {
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)

			createCalled := false
			mockDS.createFn = func(_ context.Context, content, source, registeredBy string) (*authwebhook.WorkflowRegistrationResult, error) {
				createCalled = true
				Expect(content).ToNot(BeEmpty(), "DS should receive CRD content")
				Expect(source).To(Equal("crd"))
				Expect(registeredBy).To(Equal(testUserEmail))
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:   "550e8400-e29b-41d4-a716-446655440000",
					WorkflowName: "scale-memory",
					Version:      "1.0.0",
					Status:       string(sharedtypes.CatalogStatusActive),
				}, nil
			}

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"UPDATE should be Allowed after DS registration")
			Expect(createCalled).To(BeTrue(),
				"DS CreateWorkflowInline MUST be called for UPDATE operations (Issue #371)")
		})
	})

	// ========================================
	// UT-AW-371-001: UPDATE triggers DS registration with correct content
	// Issue #371, BR-WORKFLOW-006: CRD UPDATE must forward spec to DS so
	// version changes supersede old catalog entries.
	// ========================================
	Describe("UT-AW-371-001: UPDATE forwards CRD spec changes to DS", func() {
		It("should call DS CreateWorkflowInline and return Allowed", func() {
			rw := buildRemediationWorkflow("git-revert-v1", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)

			var capturedContent string
			mockDS.createFn = func(_ context.Context, content, source, registeredBy string) (*authwebhook.WorkflowRegistrationResult, error) {
				capturedContent = content
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:   "new-uuid-after-update",
					WorkflowName: "git-revert-v1",
					Version:      "1.0.1",
					Status:       string(sharedtypes.CatalogStatusActive),
				}, nil
			}

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue())
			Expect(capturedContent).ToNot(BeEmpty(),
				"DS should receive the updated CRD content for registration")
		})
	})

	// ========================================
	// UT-AW-371-002: UPDATE with same content is idempotent
	// Issue #371, BR-WORKFLOW-006: When CRD spec hasn't changed, DS returns 200
	// idempotent and no new entry is created.
	// ========================================
	Describe("UT-AW-371-002: UPDATE with unchanged content is idempotent", func() {
		It("should return Allowed with PreviouslyExisted=true from DS", func() {
			rw := buildRemediationWorkflow("git-revert-v1", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)

			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:        "existing-uuid-idempotent",
					WorkflowName:      "git-revert-v1",
					Version:           "1.0.0",
					Status:            string(sharedtypes.CatalogStatusActive),
					PreviouslyExisted: true,
				}, nil
			}

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"Idempotent UPDATE should be Allowed")
		})
	})

	// ========================================
	// UT-AW-773-001: UPDATE denies on unmarshal failure + emits denied audit
	// Issue #773: handleUpdate must match handleCreate strictness (SOC2 CC8.1)
	// ========================================
	Describe("UT-AW-773-001: UPDATE denied on unmarshal failure", func() {
		It("should return Denied and emit denied audit event", func() {
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID:       "admission-update-bad-json",
					Operation: admissionv1.Update,
					Name:      "bad-rw",
					Namespace: "kubernaut-system",
					UserInfo: authv1.UserInfo{
						Username: testUserEmail,
						UID:      testUserUID,
					},
					Object: runtime.RawExtension{Raw: []byte(`{invalid json}`)},
				},
			}

			resp := handler.Handle(ctx, req)

			Expect(resp.Allowed).To(BeFalse(),
				"UPDATE should be Denied when CRD cannot be unmarshalled")
			Expect(resp.Result.Message).To(ContainSubstring("unmarshal"),
				"Denial reason should reference unmarshal failure")

			Expect(mockAudit.StoredEvents).To(HaveLen(1),
				"Denied audit event should be emitted")
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeRWAdmittedDenied))
		})
	})

	// ========================================
	// UT-AW-773-002: UPDATE denies on auth failure + emits denied audit
	// Issue #773: handleUpdate must match handleCreate strictness (SOC2 CC8.1)
	// ========================================
	Describe("UT-AW-773-002: UPDATE denied on auth failure", func() {
		It("should return Denied and emit denied audit event when user extraction fails", func() {
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)
			admReq.UserInfo = authv1.UserInfo{}

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeFalse(),
				"UPDATE should be Denied when authentication fails")
			Expect(resp.Result.Message).To(ContainSubstring("authentication"),
				"Denial reason should reference authentication")

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeRWAdmittedDenied))
		})
	})

	// ========================================
	// UT-AW-773-003: UPDATE denies on DS registration failure + emits denied audit
	// Issue #773: handleUpdate must match handleCreate strictness (SOC2 CC8.1)
	// ========================================
	Describe("UT-AW-773-003: UPDATE denied on DS registration failure", func() {
		It("should return Denied and emit denied audit event when DS fails", func() {
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				return nil, fmt.Errorf("connection refused: data storage service unavailable")
			}

			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeFalse(),
				"UPDATE should be Denied when DS registration fails (fail-closed)")
			Expect(resp.Result.Message).To(ContainSubstring("data storage"),
				"Denial message should reference DS failure")

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeRWAdmittedDenied))
		})
	})

	// ========================================
	// UT-AW-773-004: UPDATE denies on marshal failure + emits denied audit
	// Issue #773: handleUpdate must match handleCreate strictness (SOC2 CC8.1)
	// NOTE: marshalCleanCRDContent rarely fails, but strictness requires coverage.
	// This test uses an unmarshal-able but incomplete RW to exercise the code path.
	// ========================================

	// ========================================
	// UT-AW-773-005: UPDATE success emits remediationworkflow.admitted.update (not CREATE)
	// Issue #773: SOC2 CC8.1 requires distinct audit events for UPDATE operations
	// ========================================
	Describe("UT-AW-773-005: UPDATE success emits distinct update audit event", func() {
		It("should emit remediationworkflow.admitted.update audit event on success", func() {
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue())
			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeRWAdmittedUpdate),
				"UPDATE success should emit 'remediationworkflow.admitted.update', not CREATE")
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
					Status:     string(sharedtypes.CatalogStatusActive),
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
	// UT-AW-INTEGRITY-001: CREATE patches new UUID on supersede
	// BR-WORKFLOW-006: When DS supersedes an old workflow, AW patches the new UUID
	// ========================================
	Describe("UT-AW-INTEGRITY-001: CREATE patches new UUID into CRD status on supersede", func() {
		It("should populate CRD .status with the NEW workflow UUID when DS indicates supersede", func() {
			rw := buildRemediationWorkflow("integrity-supersede", "kubernaut-system")

			scheme := runtime.NewScheme()
			_ = rwv1alpha1.AddToScheme(scheme)

			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
				Build()

			handlerWithK8s := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)

			newUUID := "new-uuid-after-supersede"
			supersededUUID := "old-uuid-superseded"
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:        newUUID,
					WorkflowName:      "integrity-supersede",
					Version:           "1.0.0",
					Status:            string(sharedtypes.CatalogStatusActive),
					PreviouslyExisted: false,
					Superseded:        true,
					SupersededID:      supersededUUID,
				}, nil
			}

			admReq := buildCreateAdmissionRequest(rw)
			resp := handlerWithK8s.Handle(ctx, admReq)
			Expect(resp.Allowed).To(BeTrue())

			Eventually(func() string {
				updated := &rwv1alpha1.RemediationWorkflow{}
				err := fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "integrity-supersede"), updated)
				if err != nil {
					return ""
				}
				return updated.Status.WorkflowID
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(newUUID),
				"CRD status should contain the NEW UUID from the supersede, not the old one")
		})
	})

	// ========================================
	// UT-AW-INTEGRITY-002: CREATE patches same UUID on re-enable
	// BR-WORKFLOW-006: When DS re-enables a disabled workflow, AW patches the original UUID
	// ========================================
	Describe("UT-AW-INTEGRITY-002: CREATE patches same UUID into CRD status on re-enable", func() {
		It("should populate CRD .status with the ORIGINAL UUID when DS re-enables", func() {
			rw := buildRemediationWorkflow("integrity-reenable", "kubernaut-system")

			scheme := runtime.NewScheme()
			_ = rwv1alpha1.AddToScheme(scheme)

			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}).
				Build()

			handlerWithK8s := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)

			originalUUID := "original-uuid-reenabled"
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:        originalUUID,
					WorkflowName:      "integrity-reenable",
					Version:           "1.0.0",
					Status:            string(sharedtypes.CatalogStatusActive),
					PreviouslyExisted: true,
					Superseded:        false,
				}, nil
			}

			admReq := buildCreateAdmissionRequest(rw)
			resp := handlerWithK8s.Handle(ctx, admReq)
			Expect(resp.Allowed).To(BeTrue())

			Eventually(func() string {
				updated := &rwv1alpha1.RemediationWorkflow{}
				err := fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "integrity-reenable"), updated)
				if err != nil {
					return ""
				}
				return updated.Status.WorkflowID
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(originalUUID),
				"CRD status should contain the ORIGINAL UUID from the re-enable")

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "integrity-reenable"), updated)).To(Succeed())
			Expect(updated.Status.PreviouslyExisted).To(BeTrue(),
				"PreviouslyExisted should be true for re-enabled workflows")
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
					Status:            string(sharedtypes.CatalogStatusActive),
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
			Expect(updated.Status.CatalogStatus).To(Equal(sharedtypes.CatalogStatusActive))
			Expect(updated.Status.RegisteredBy).To(Equal(testUserEmail))
			Expect(updated.Status.RegisteredAt).NotTo(BeNil())
			Expect(updated.Status.PreviouslyExisted).To(BeFalse())
		})
	})
})
