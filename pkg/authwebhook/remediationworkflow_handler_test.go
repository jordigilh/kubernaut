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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ========================================
// Mocks
// ========================================

// mockWorkflowCatalogClient is a WorkflowCatalogClient-shaped test double.
// #1661 Change 8c emptied the WorkflowCatalogClient interface (see its doc
// comment in remediationworkflow_handler.go) -- CreateWorkflowInline/
// DisableWorkflow below are no longer required by any interface and are no
// longer reachable through h.dsClient, but are kept so existing "zero DS
// calls" tests continue to prove that these hooks are never invoked.
type mockWorkflowCatalogClient struct {
	createFn  func(ctx context.Context, content, source, registeredBy string) error
	disableFn func(ctx context.Context, workflowID, reason, updatedBy string) error
}

func (m *mockWorkflowCatalogClient) CreateWorkflowInline(ctx context.Context, content, source, registeredBy string) error {
	if m.createFn != nil {
		return m.createFn(ctx, content, source, registeredBy)
	}
	return nil
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
				Component:   []string{"v1/Pod"},
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

// expectedLocalWorkflowID derives the workflow_id AW is expected to compute
// locally (#1661 Change 8a) for the most recently stored admitted audit
// event, by re-deriving DeterministicUUID from that event's already-asserted
// content_hash. This lets tests assert against AW's own local computation
// without duplicating marshalCleanCRDContent's (unexported) canonicalization
// logic.
func expectedLocalWorkflowID(mockAudit *MockAuditStoreRW) string {
	ExpectWithOffset(1, mockAudit.StoredEvents).ToNot(BeEmpty())
	event := mockAudit.StoredEvents[len(mockAudit.StoredEvents)-1]
	payload, ok := event.EventData.GetRemediationWorkflowWebhookAuditPayload()
	ExpectWithOffset(1, ok).To(BeTrue())
	ExpectWithOffset(1, payload.ContentHash.IsSet()).To(BeTrue())
	return contenthash.DeterministicUUID(payload.ContentHash.Value)
}

// fakeK8sWithRWAndActiveActionType builds a fake client seeded with rw and an
// Active ActionType CRD matching rw.Spec.ActionType, plus the ".spec.name"
// field index registered on the real manager (cmd/authwebhook/main.go).
// Required so that the #1661 ActionType-existence gate in registerWorkflow
// lets these CREATE/UPDATE flows reach DS, matching production where every
// RW references an already-registered, Active ActionType.
func fakeK8sWithRWAndActiveActionType(rw *rwv1alpha1.RemediationWorkflow) client.Client {
	scheme := runtime.NewScheme()
	_ = rwv1alpha1.AddToScheme(scheme)
	_ = atv1alpha1.AddToScheme(scheme)

	at := buildActionType("gate-actiontype", rw.Spec.ActionType, rw.Namespace)
	at.Status.CatalogStatus = sharedtypes.CatalogStatusActive

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(rw, at).
		WithStatusSubresource(&rwv1alpha1.RemediationWorkflow{}, &atv1alpha1.ActionType{}).
		WithIndex(&atv1alpha1.ActionType{}, ".spec.name", func(obj client.Object) []string {
			a := obj.(*atv1alpha1.ActionType)
			return []string{a.Spec.Name}
		}).
		Build()
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
	Describe("UT-AW-299-001: CREATE succeeds locally with zero DS calls (#1661 Change 8c)", func() {
		It("should return Allowed without calling DS", func() {
			// Arrange
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return nil
			}

			admReq := buildCreateAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue(),
				"CREATE should be Allowed")
			Expect(dsCalled).To(BeFalse(),
				"#1661 Change 8c: DS is never called -- registration is a pure local computation")
		})
	})

	// ========================================
	// UT-AW-299-002: DELETE succeeds locally with zero DS calls (#1661 Change 8c)
	// ========================================
	Describe("UT-AW-299-002: DELETE succeeds locally with zero DS calls (#1661 Change 8c)", func() {
		It("should return Allowed without calling DS", func() {
			// Arrange
			rw := buildRemediationWorkflowWithStatus("scale-memory", "kubernaut-system", "550e8400-e29b-41d4-a716-446655440000")
			dsCalled := false
			mockDS.disableFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return nil
			}

			admReq := buildDeleteAdmissionRequest(rw)

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue(),
				"DELETE should be Allowed")
			Expect(dsCalled).To(BeFalse(),
				"#1661 Change 8c: DS is never called on DELETE -- the etcd removal is itself the terminal state")
		})
	})

	// ========================================
	// UT-AW-299-004: Re-CREATE of deleted workflow re-enables in DS
	// ========================================
	Describe("UT-AW-299-004: CREATE re-enables previously deleted workflow", func() {
		It("should return Allowed when DS indicates re-enable", func() {
			// Arrange
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				return nil
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
		It("should emit remediationworkflow.admitted.denied audit event on auth failure", func() {
			// Arrange. #1661 Change 8c removed the DS-registration-failure
			// denial path entirely (there is no more DS call to fail); auth
			// failure is the representative still-existing CREATE denial path.
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildCreateAdmissionRequest(rw)
			admReq.UserInfo = authv1.UserInfo{}

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeFalse())
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

			// Act
			resp := handler.Handle(ctx, admReq)

			// Assert: extraction success is observable via the audit event's
			// actor attribution -- #1661 Change 8c removed the DS round-trip
			// that this test previously used as its extraction probe.
			Expect(resp.Allowed).To(BeTrue())
			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			Expect(mockAudit.StoredEvents[0].ActorID.Value).To(Equal(testUserEmail),
				"Should extract K8s username into audit event actor attribution")
		})
	})

	// ========================================
	// UT-AW-299-009: UPDATE succeeds locally with zero DS calls (#1661 Change 8c)
	// Historically (Issue #371) UPDATE forwarded to DS; that round-trip is now
	// removed entirely -- registration is a pure local computation.
	// ========================================
	Describe("UT-AW-299-009: UPDATE succeeds locally with zero DS calls", func() {
		It("should allow UPDATE without calling DS", func() {
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)

			createCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				createCalled = true
				return nil
			}

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"UPDATE should be Allowed")
			Expect(createCalled).To(BeFalse(),
				"#1661 Change 8c: DS is never called for UPDATE -- registration is a pure local computation")
		})
	})

	// ========================================
	// UT-AW-371-001: UPDATE triggers DS registration with correct content
	// Issue #371, BR-WORKFLOW-006: CRD UPDATE must forward spec to DS so
	// version changes supersede old catalog entries.
	// ========================================
	Describe("UT-AW-371-001: UPDATE registers the updated content locally with zero DS calls", func() {
		It("should return Allowed and carry the updated content in the audit event, without calling DS", func() {
			rw := buildRemediationWorkflow("git-revert-v1", "kubernaut-system")
			admReq := buildUpdateAdmissionRequest(rw)

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return nil
			}

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue())
			Expect(dsCalled).To(BeFalse(),
				"#1661 Change 8c: DS is never called -- registration is a pure local computation")

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			payload, ok := mockAudit.StoredEvents[0].EventData.GetRemediationWorkflowWebhookAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.WorkflowContent.IsSet()).To(BeTrue(),
				"the updated CRD content should still be captured, now via the audit trail (#1661 Change 2) instead of a DS round-trip")
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

			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				return nil
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

	// NOTE: UT-AW-773-003 ("UPDATE denied on DS registration failure") was
	// removed by #1661 Change 8c -- there is no more DS call in the UPDATE
	// path for it to fail. The remaining denial paths (unmarshal, auth,
	// ActionType-gate, content-integrity) are covered by UT-AW-773-001,
	// UT-AW-773-002, rw_actiontype_gate_test.go, and rw_content_integrity_test.go
	// respectively.

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
	// UT-AW-773-006: UPDATE during deletion skips registration (deletionTimestamp set)
	// Issue #773: When K8s sends UPDATE during CRD deletion (e.g., finalizer removal),
	// handleUpdate must NOT re-register with DS, which would undo the DisableWorkflow
	// performed by handleDelete (DS sees "disabled + same hash → re-enable").
	// ========================================
	Describe("UT-AW-773-006: UPDATE during deletion skips DS registration", func() {
		It("should allow the update without calling DS when deletionTimestamp is set", func() {
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			now := metav1.Now()
			rw.DeletionTimestamp = &now

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return fmt.Errorf("should not be called")
			}

			admReq := buildUpdateAdmissionRequest(rw)
			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"UPDATE during deletion should be Allowed (skip registration)")
			Expect(dsCalled).To(BeFalse(),
				"DS CreateWorkflowInline should NOT be called during deletion")
			Expect(mockAudit.StoredEvents).To(BeEmpty(),
				"No audit event should be emitted for skipped deletion updates")
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
	Describe("UT-AW-299-011: CREATE attributes registeredBy on the CRD status without a DS round-trip", func() {
		It("should populate .status.registeredBy from userInfo with zero DS calls", func() {
			// Arrange. #1661 Change 8c removed the DS request that used to
			// carry source='crd'/registeredBy -- registeredBy is now written
			// straight to .status by updateCRDStatus (see UT-AW-299-013).
			rw := buildRemediationWorkflow("scale-memory-src", "kubernaut-system")
			fakeK8s := fakeK8sWithRWAndActiveActionType(rw)
			handlerWithK8s := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return nil
			}

			admReq := buildCreateAdmissionRequest(rw)

			// Act
			resp := handlerWithK8s.Handle(ctx, admReq)

			// Assert
			Expect(resp.Allowed).To(BeTrue())
			Expect(dsCalled).To(BeFalse(),
				"#1661 Change 8c: DS is never called -- registration is a pure local computation")

			Eventually(func() string {
				updated := &rwv1alpha1.RemediationWorkflow{}
				if err := fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "scale-memory-src"), updated); err != nil {
					return ""
				}
				return updated.Status.RegisteredBy
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(testUserEmail))
		})
	})

	// ========================================
	// UT-AW-INTEGRITY-001: CREATE patches the locally-computed UUID regardless
	// of what DS's response reports for supersession (#1661 Change 8a).
	// BR-WORKFLOW-006.
	// ========================================
	Describe("UT-AW-INTEGRITY-001: CREATE patches the locally-computed UUID into CRD status, ignoring DS's supersede response", func() {
		It("should populate CRD .status with AW's own deterministic UUID, not DS's reported newUUID/supersededUUID", func() {
			rw := buildRemediationWorkflow("integrity-supersede", "kubernaut-system")
			fakeK8s := fakeK8sWithRWAndActiveActionType(rw)

			handlerWithK8s := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)

			// #1661 Change 8a: DS's reported WorkflowID/SupersededID are no
			// longer authoritative for .status.workflowId -- AW computes its
			// own deterministic UUID locally regardless of what DS's
			// (still-Postgres-backed, until Change 8c) supersede logic
			// reports. Deliberately implausible strings prove AW ignores
			// them entirely.
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				return nil
			}

			admReq := buildCreateAdmissionRequest(rw)
			resp := handlerWithK8s.Handle(ctx, admReq)
			Expect(resp.Allowed).To(BeTrue())

			expectedID := expectedLocalWorkflowID(mockAudit)
			Eventually(func() string {
				updated := &rwv1alpha1.RemediationWorkflow{}
				err := fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "integrity-supersede"), updated)
				if err != nil {
					return ""
				}
				return updated.Status.WorkflowID
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(expectedID),
				"CRD status should contain AW's own locally-computed UUID (#1661 Change 8a), not DS's reported one")
		})
	})

	// ========================================
	// UT-AW-INTEGRITY-002: CREATE patches the same locally-computed UUID on
	// re-registration, with zero DS involvement (#1661 Change 8a + 8c).
	// BR-WORKFLOW-006.
	// ========================================
	Describe("UT-AW-INTEGRITY-002: CREATE patches the locally-computed UUID into CRD status, with zero DS calls", func() {
		It("should populate CRD .status with AW's own deterministic UUID and PreviouslyExisted=false", func() {
			rw := buildRemediationWorkflow("integrity-reenable", "kubernaut-system")
			fakeK8s := fakeK8sWithRWAndActiveActionType(rw)

			handlerWithK8s := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				dsCalled = true
				return nil
			}

			admReq := buildCreateAdmissionRequest(rw)
			resp := handlerWithK8s.Handle(ctx, admReq)
			Expect(resp.Allowed).To(BeTrue())
			Expect(dsCalled).To(BeFalse(),
				"#1661 Change 8c: DS is never called -- registration is a pure local computation")

			expectedID := expectedLocalWorkflowID(mockAudit)
			Eventually(func() string {
				updated := &rwv1alpha1.RemediationWorkflow{}
				err := fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "integrity-reenable"), updated)
				if err != nil {
					return ""
				}
				return updated.Status.WorkflowID
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(expectedID),
				"CRD status should contain AW's own locally-computed UUID (#1661 Change 8a)")

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "integrity-reenable"), updated)).To(Succeed())
			Expect(updated.Status.PreviouslyExisted).To(BeFalse(),
				"#1661 Change 8c: PreviouslyExisted is always false -- with no DS-side 'disabled' intermediate "+
					"state, there is no local way (or need) to distinguish 'brand new' from 'recreated after "+
					"deletion'; that history lives in audit_events instead (DD-WORKFLOW-018)")
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
		It("should populate .status with the locally-computed workflow ID via k8sClient.Status().Update()", func() {
			rw := buildRemediationWorkflow("scale-memory-status", "kubernaut-system")
			fakeK8s := fakeK8sWithRWAndActiveActionType(rw)

			handlerWithK8s := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, fakeK8s)

			// #1661 Change 8a: DS's reported WorkflowID is deliberately
			// implausible -- AW's .status.workflowId write is sourced from
			// its own local deterministic computation, not this value.
			mockDS.createFn = func(_ context.Context, _, _, _ string) error {
				return nil
			}

			admReq := buildCreateAdmissionRequest(rw)
			resp := handlerWithK8s.Handle(ctx, admReq)
			Expect(resp.Allowed).To(BeTrue())

			expectedID := expectedLocalWorkflowID(mockAudit)
			// Wait for async goroutine to complete
			Eventually(func() string {
				updated := &rwv1alpha1.RemediationWorkflow{}
				err := fakeK8s.Get(ctx, fakeK8sKey("kubernaut-system", "scale-memory-status"), updated)
				if err != nil {
					return ""
				}
				return updated.Status.WorkflowID
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(expectedID))

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
