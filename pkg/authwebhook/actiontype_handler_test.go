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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ========================================
// ActionType Mocks
// ========================================

type mockActionTypeCatalogClient struct {
	createFn       func(ctx context.Context, name string, desc ogenclient.ActionTypeDescription, registeredBy string) (*authwebhook.ActionTypeRegistrationResult, error)
	updateFn       func(ctx context.Context, name string, desc ogenclient.ActionTypeDescription, updatedBy string) (*authwebhook.ActionTypeUpdateResult, error)
	disableFn      func(ctx context.Context, name string, disabledBy string) (*authwebhook.ActionTypeDisableResult, error)
	forceDisableFn func(ctx context.Context, name string, disabledBy string, orphanedWorkflows []string) (*authwebhook.ActionTypeDisableResult, error)
}

func (m *mockActionTypeCatalogClient) CreateActionType(ctx context.Context, name string, desc ogenclient.ActionTypeDescription, registeredBy string) (*authwebhook.ActionTypeRegistrationResult, error) {
	if m.createFn != nil {
		return m.createFn(ctx, name, desc, registeredBy)
	}
	return &authwebhook.ActionTypeRegistrationResult{
		ActionType: name, Status: "created", WasReenabled: false,
	}, nil
}

func (m *mockActionTypeCatalogClient) UpdateActionType(ctx context.Context, name string, desc ogenclient.ActionTypeDescription, updatedBy string) (*authwebhook.ActionTypeUpdateResult, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, name, desc, updatedBy)
	}
	return &authwebhook.ActionTypeUpdateResult{ActionType: name, UpdatedFields: []string{"description"}}, nil
}

func (m *mockActionTypeCatalogClient) DisableActionType(ctx context.Context, name string, disabledBy string) (*authwebhook.ActionTypeDisableResult, error) {
	if m.disableFn != nil {
		return m.disableFn(ctx, name, disabledBy)
	}
	return &authwebhook.ActionTypeDisableResult{Disabled: true}, nil
}

func (m *mockActionTypeCatalogClient) ForceDisableActionType(ctx context.Context, name string, disabledBy string, orphanedWorkflows []string) (*authwebhook.ActionTypeDisableResult, error) {
	if m.forceDisableFn != nil {
		return m.forceDisableFn(ctx, name, disabledBy, orphanedWorkflows)
	}
	return &authwebhook.ActionTypeDisableResult{Disabled: true}, nil
}

// ========================================
// ActionType Test Helpers
// ========================================

func buildActionType(name, specName, namespace string) *atv1alpha1.ActionType {
	return &atv1alpha1.ActionType{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubernaut.ai/v1alpha1",
			Kind:       "ActionType",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "at-uid-001",
		},
		Spec: atv1alpha1.ActionTypeSpec{
			Name: specName,
			Description: atv1alpha1.ActionTypeDescription{
				What:          "Kill and recreate one or more pods.",
				WhenToUse:     "Root cause is a transient runtime state issue.",
				Preconditions: "Evidence that the issue is transient.",
			},
		},
	}
}

func buildATCreateAdmissionRequest(at *atv1alpha1.ActionType) admission.Request {
	raw, _ := json.Marshal(at)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "at-admission-create-001",
			Kind: metav1.GroupVersionKind{
				Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType",
			},
			Name:      at.Name,
			Namespace: at.Namespace,
			Operation: admissionv1.Create,
			UserInfo: authv1.UserInfo{
				Username: testUserEmail,
				UID:      testUserUID,
				Groups:   []string{"system:masters"},
			},
			Object: runtime.RawExtension{Raw: raw},
		},
	}
}

func buildATUpdateAdmissionRequest(oldAT, newAT *atv1alpha1.ActionType) admission.Request {
	oldRaw, _ := json.Marshal(oldAT)
	newRaw, _ := json.Marshal(newAT)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "at-admission-update-001",
			Kind: metav1.GroupVersionKind{
				Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType",
			},
			Name:      newAT.Name,
			Namespace: newAT.Namespace,
			Operation: admissionv1.Update,
			UserInfo: authv1.UserInfo{
				Username: testUserEmail,
				UID:      testUserUID,
			},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	}
}

func buildATDeleteAdmissionRequest(at *atv1alpha1.ActionType) admission.Request {
	raw, _ := json.Marshal(at)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "at-admission-delete-001",
			Kind: metav1.GroupVersionKind{
				Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType",
			},
			Name:      at.Name,
			Namespace: at.Namespace,
			Operation: admissionv1.Delete,
			UserInfo: authv1.UserInfo{
				Username: testUserEmail,
				UID:      testUserUID,
			},
			OldObject: runtime.RawExtension{Raw: raw},
		},
	}
}

func newATScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = atv1alpha1.AddToScheme(s)
	_ = rwv1alpha1.AddToScheme(s)
	return s
}

// ========================================
// Tests
// ========================================

var _ = Describe("ActionType Admission Handler (#300)", func() {
	var (
		ctx       context.Context
		handler   *authwebhook.ActionTypeHandler
		mockDS    *mockActionTypeCatalogClient
		mockAudit *MockAuditStoreRW
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockDS = &mockActionTypeCatalogClient{}
		mockAudit = &MockAuditStoreRW{}
		handler = authwebhook.NewActionTypeHandler(mockDS, mockAudit, nil)
	})

	// ========================================
	// UT-AT-300-001: CREATE registers new ActionType locally (#1661 Change 8d)
	// BR-WORKFLOW-007.1
	// ========================================
	Describe("UT-AT-300-001: CREATE registers new ActionType locally", func() {
		It("should return Allowed and audit the spec fields with zero DS calls", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")

			resp := handler.Handle(ctx, buildATCreateAdmissionRequest(at))

			Expect(resp.Allowed).To(BeTrue(),
				"CREATE should be Allowed for a well-formed ActionType with zero DS round-trips")

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			payload, ok := mockAudit.StoredEvents[0].EventData.GetActionTypeWebhookAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ActionTypeName).To(Equal("RestartPod"),
				"audit should capture the spec.name")
		})
	})

	// ========================================
	// UT-AT-300-002: Idempotent CREATE for already-active action type
	// BR-WORKFLOW-007.1
	// ========================================
	Describe("UT-AT-300-002: Idempotent CREATE for already-active action type", func() {
		It("should return Allowed when DS indicates action type already exists", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			mockDS.createFn = func(_ context.Context, name string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeRegistrationResult, error) {
				return &authwebhook.ActionTypeRegistrationResult{
					ActionType: name, Status: "exists", WasReenabled: false,
				}, nil
			}

			resp := handler.Handle(ctx, buildATCreateAdmissionRequest(at))

			Expect(resp.Allowed).To(BeTrue(),
				"Idempotent CREATE should be Allowed when action type already exists")
		})
	})

	// ========================================
	// UT-AT-300-003: CREATE always reports previouslyExisted=false (#1661 Change 8d)
	// BR-WORKFLOW-007.1
	// ========================================
	Describe("UT-AT-300-003: CREATE always reports previouslyExisted=false", func() {
		It("should return Allowed and reset status.previouslyExisted to false even for a CRD previously marked Disabled", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			// Simulate a CRD that carries stale status from a prior local-only
			// disable/re-create cycle -- there is no DS-side "re-enable" signal
			// left to distinguish this from a brand-new registration
			// (DD-WORKFLOW-018), so CREATE must unconditionally reset both fields.
			at.Status.CatalogStatus = sharedtypes.CatalogStatusDisabled
			at.Status.PreviouslyExisted = true
			scheme := newATScheme()
			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at).
				WithStatusSubresource(&atv1alpha1.ActionType{}).
				Build()

			handlerWithK8s := authwebhook.NewActionTypeHandler(mockDS, mockAudit, fakeK8s)

			resp := handlerWithK8s.Handle(ctx, buildATCreateAdmissionRequest(at))
			Expect(resp.Allowed).To(BeTrue(),
				"CREATE should be Allowed with zero DS round-trips")

			Eventually(func() sharedtypes.CatalogStatus {
				updated := &atv1alpha1.ActionType{}
				if err := fakeK8s.Get(ctx, client.ObjectKeyFromObject(at), updated); err != nil {
					return ""
				}
				return updated.Status.CatalogStatus
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(sharedtypes.CatalogStatusActive),
				"CRD status.catalogStatus should always become Active locally")

			updated := &atv1alpha1.ActionType{}
			Expect(fakeK8s.Get(ctx, client.ObjectKeyFromObject(at), updated)).To(Succeed())
			Expect(updated.Status.PreviouslyExisted).To(BeFalse(),
				"there is no DS-side lifecycle left to distinguish 're-enabled' from 'new' -- always false")
		})
	})

	// ========================================
	// UT-AT-300-004: UPDATE description change with audit
	// BR-WORKFLOW-007.2
	// ========================================
	Describe("UT-AT-300-004: UPDATE description change generates audit", func() {
		It("should return Allowed and audit the updated description with zero DS calls", func() {
			oldAT := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			newAT := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			newAT.Spec.Description.What = "Gracefully restart one or more pods with rolling strategy."

			resp := handler.Handle(ctx, buildATUpdateAdmissionRequest(oldAT, newAT))

			Expect(resp.Allowed).To(BeTrue(),
				"UPDATE with description change should be Allowed with zero DS round-trips")

			Expect(mockAudit.StoredEvents).To(HaveLen(1),
				"One audit event should be emitted for UPDATE")
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal(authwebhook.EventTypeATAdmittedUpdate))
			Expect(string(event.EventOutcome)).To(Equal("success"))

			payload, ok := event.EventData.GetActionTypeWebhookAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ActionTypeName).To(Equal("RestartPod"))
		})
	})

	// ========================================
	// UT-AT-300-005: UPDATE spec.name change denied
	// BR-WORKFLOW-007.2
	// ========================================
	Describe("UT-AT-300-005: UPDATE spec.name change is denied by webhook", func() {
		It("should return Denied when spec.name is changed", func() {
			oldAT := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			newAT := buildActionType("restart-pod", "GracefulRestart", "kubernaut-system")

			updateCalled := false
			mockDS.updateFn = func(_ context.Context, _ string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeUpdateResult, error) {
				updateCalled = true
				return nil, fmt.Errorf("should not be called")
			}

			resp := handler.Handle(ctx, buildATUpdateAdmissionRequest(oldAT, newAT))

			Expect(resp.Allowed).To(BeFalse(),
				"UPDATE should be Denied when spec.name changes")
			Expect(resp.Result.Message).To(ContainSubstring("immutable"),
				"Denial message should explain that spec.name is immutable")
			Expect(resp.Result.Message).To(ContainSubstring("RestartPod"))
			Expect(resp.Result.Message).To(ContainSubstring("GracefulRestart"))
			Expect(updateCalled).To(BeFalse(),
				"DS should NOT be called when spec.name change is detected")

			Expect(mockAudit.StoredEvents).To(HaveLen(1),
				"Denied audit event should be emitted")
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal(authwebhook.EventTypeATDeniedUpdate))
			Expect(string(event.EventOutcome)).To(Equal("failure"))
		})
	})

	// ========================================
	// UT-AT-300-006: DELETE with no dependent workflows
	// BR-WORKFLOW-007.3
	// ========================================
	Describe("UT-AT-300-006: DELETE with no dependent workflows soft-disables", func() {
		It("should return Allowed when the K8s-native dependents list is empty, with zero DS calls", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")

			resp := handler.Handle(ctx, buildATDeleteAdmissionRequest(at))

			Expect(resp.Allowed).To(BeTrue(),
				"DELETE should be Allowed when no dependent workflows exist")

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal(authwebhook.EventTypeATAdmittedDelete))
			Expect(string(event.EventOutcome)).To(Equal("success"))
		})
	})

	// ========================================
	// UT-AT-300-007: DELETE denied with dependent workflows (#1661 Change 8d:
	// K8s-native list replaces the DS-backed dependents check)
	// BR-WORKFLOW-007.3
	// ========================================
	Describe("UT-AT-300-007: DELETE denied with N dependent workflows", func() {
		It("should return Denied with count and workflow names when live RemediationWorkflow CRDs depend on it", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			wf1 := buildRemediationWorkflow("restart-pod-graceful", "kubernaut-system")
			wf1.Spec.ActionType = "RestartPod"
			wf2 := buildRemediationWorkflow("restart-pod-force", "kubernaut-system")
			wf2.Spec.ActionType = "RestartPod"
			wf3 := buildRemediationWorkflow("restart-pod-canary", "kubernaut-system")
			wf3.Spec.ActionType = "RestartPod"

			scheme := newATScheme()
			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at, wf1, wf2, wf3).
				Build()
			handlerWithK8s := authwebhook.NewActionTypeHandler(mockDS, mockAudit, fakeK8s)

			resp := handlerWithK8s.Handle(ctx, buildATDeleteAdmissionRequest(at))

			Expect(resp.Allowed).To(BeFalse(),
				"DELETE should be Denied when dependent workflows exist")
			Expect(resp.Result.Message).To(ContainSubstring("3"),
				"Denial message should contain the workflow count")
			Expect(resp.Result.Message).To(ContainSubstring("restart-pod-graceful"),
				"Denial message should contain workflow names")
			Expect(resp.Result.Message).To(ContainSubstring("restart-pod-force"))
			Expect(resp.Result.Message).To(ContainSubstring("restart-pod-canary"))

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal(authwebhook.EventTypeATDeniedDelete))
			Expect(string(event.EventOutcome)).To(Equal("failure"))
		})
	})

	// ========================================
	// UT-AT-300-008: CREATE audit event payload
	// BR-WORKFLOW-007.4
	// ========================================
	Describe("UT-AT-300-008: CREATE audit event payload contains all required fields", func() {
		It("should emit actiontype.admitted.create with correct payload structure", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")

			resp := handler.Handle(ctx, buildATCreateAdmissionRequest(at))
			Expect(resp.Allowed).To(BeTrue())

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("actiontype.admitted.create"))
			Expect(string(event.EventCategory)).To(Equal("actiontype"))
			Expect(event.EventAction).To(Equal("admitted"))
			Expect(string(event.EventOutcome)).To(Equal("success"))
			Expect(event.ActorID.Value).To(Equal(testUserEmail))
			Expect(event.ResourceType.Value).To(Equal("ActionType"))
			Expect(event.ResourceID.Value).To(Equal("restart-pod"))

			payload, ok := event.EventData.GetActionTypeWebhookAuditPayload()
			Expect(ok).To(BeTrue(), "EventData should contain ActionTypeWebhookAuditPayload")
			Expect(payload.ActionTypeName).To(Equal("RestartPod"))
			Expect(payload.CrdName).To(Equal("restart-pod"))
			Expect(payload.CrdNamespace).To(Equal("kubernaut-system"))
			Expect(payload.Action).To(Equal(ogenclient.ActionTypeWebhookAuditPayloadActionCreate))
		})
	})

	// ========================================
	// UT-AT-300-009: UPDATE audit event with old+new
	// BR-WORKFLOW-007.4
	// ========================================
	Describe("UT-AT-300-009: UPDATE audit event contains correct payload", func() {
		It("should emit actiontype.admitted.update with action=update", func() {
			oldAT := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			newAT := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			newAT.Spec.Description.What = "Updated description."

			resp := handler.Handle(ctx, buildATUpdateAdmissionRequest(oldAT, newAT))
			Expect(resp.Allowed).To(BeTrue())

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("actiontype.admitted.update"))
			Expect(string(event.EventOutcome)).To(Equal("success"))

			payload, ok := event.EventData.GetActionTypeWebhookAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ActionTypeName).To(Equal("RestartPod"))
			Expect(payload.Action).To(Equal(ogenclient.ActionTypeWebhookAuditPayloadActionUpdate))
		})
	})

	// ========================================
	// UT-AT-300-010: Disable denied audit contains dependentWorkflows
	// BR-WORKFLOW-007.4
	// ========================================
	Describe("UT-AT-300-010: Disable denied audit event contains denial details", func() {
		It("should emit actiontype.denied.delete with denial reason and operation", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			wfAlpha := buildRemediationWorkflow("wf-alpha", "kubernaut-system")
			wfAlpha.Spec.ActionType = "RestartPod"
			wfBeta := buildRemediationWorkflow("wf-beta", "kubernaut-system")
			wfBeta.Spec.ActionType = "RestartPod"

			scheme := newATScheme()
			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at, wfAlpha, wfBeta).
				Build()
			handlerWithK8s := authwebhook.NewActionTypeHandler(mockDS, mockAudit, fakeK8s)

			resp := handlerWithK8s.Handle(ctx, buildATDeleteAdmissionRequest(at))
			Expect(resp.Allowed).To(BeFalse())

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("actiontype.denied.delete"))
			Expect(string(event.EventOutcome)).To(Equal("failure"))
			Expect(event.EventAction).To(Equal("denied"))

			payload, ok := event.EventData.GetActionTypeWebhookAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Action).To(Equal(ogenclient.ActionTypeWebhookAuditPayloadActionDenied))
			Expect(payload.DenialReason.Value).To(ContainSubstring("2 active workflow"))
			Expect(payload.DenialOperation.Value).To(Equal("DELETE"))
		})
	})

	// ========================================
	// UT-AT-300-UPDATE-NOOP: UPDATE with no description change passes through
	// ========================================
	Describe("UPDATE with no description change is allowed without DS call", func() {
		It("should allow UPDATE without calling DS when descriptions are identical", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")

			updateCalled := false
			mockDS.updateFn = func(_ context.Context, _ string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeUpdateResult, error) {
				updateCalled = true
				return nil, fmt.Errorf("should not be called")
			}

			resp := handler.Handle(ctx, buildATUpdateAdmissionRequest(at, at))

			Expect(resp.Allowed).To(BeTrue(),
				"UPDATE with no changes should be Allowed")
			Expect(updateCalled).To(BeFalse(),
				"DS should NOT be called when description is unchanged")
			Expect(mockAudit.StoredEvents).To(BeEmpty(),
				"No audit event should be emitted for no-op UPDATE")
		})
	})

	// ========================================
	// UT-AT-300-DELETE-K8S-ERROR: DELETE denied when the dependents list
	// itself fails (#1661 Change 8d: fail-closed on K8s list error, since
	// there is no DS-backed check left to fall back on)
	// ========================================
	Describe("DELETE denied when the K8s dependents list fails", func() {
		It("should return Denied when listing RemediationWorkflow CRDs errors (fail-closed)", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			scheme := newATScheme()
			erroringK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
						return fmt.Errorf("simulated K8s API failure")
					},
				}).
				Build()
			handlerWithK8s := authwebhook.NewActionTypeHandler(mockDS, mockAudit, erroringK8s)

			resp := handlerWithK8s.Handle(ctx, buildATDeleteAdmissionRequest(at))

			Expect(resp.Allowed).To(BeFalse(),
				"DELETE should be Denied when the dependents check itself fails (fail-closed)")
			Expect(resp.Result.Message).To(ContainSubstring("dependent workflows"))

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal(authwebhook.EventTypeATDeniedDelete))
		})
	})

	// ========================================
	// UT-AT-300-STATUS: CREATE populates CRD status asynchronously
	// BR-WORKFLOW-007.1
	// ========================================
	Describe("CREATE updates CRD status asynchronously", func() {
		It("should populate .status with DS registration result via k8sClient.Status().Update()", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			scheme := newATScheme()
			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at).
				WithStatusSubresource(&atv1alpha1.ActionType{}).
				Build()

			handlerWithK8s := authwebhook.NewActionTypeHandler(mockDS, mockAudit, fakeK8s)

			mockDS.createFn = func(_ context.Context, name string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeRegistrationResult, error) {
				return &authwebhook.ActionTypeRegistrationResult{
					ActionType: name, Status: "created", WasReenabled: false,
				}, nil
			}

			resp := handlerWithK8s.Handle(ctx, buildATCreateAdmissionRequest(at))
			Expect(resp.Allowed).To(BeTrue())

			Eventually(func() bool {
				updated := &atv1alpha1.ActionType{}
				if err := fakeK8s.Get(ctx, client.ObjectKeyFromObject(at), updated); err != nil {
					return false
				}
				return updated.Status.Registered
			}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
				"CRD status.registered should become true")

			updated := &atv1alpha1.ActionType{}
			Expect(fakeK8s.Get(ctx, client.ObjectKeyFromObject(at), updated)).To(Succeed())
			Expect(updated.Status.CatalogStatus).To(Equal(sharedtypes.CatalogStatusActive))
			Expect(updated.Status.RegisteredBy).To(Equal(testUserEmail))
			Expect(updated.Status.RegisteredAt).NotTo(BeNil())
			Expect(updated.Status.PreviouslyExisted).To(BeFalse())
		})
	})

	// ========================================
	// UT-AT-512-002: DELETE with live RWs — genuine denial (#1661 Change 8d:
	// the K8s-native list IS the dependents check now; there is no DS-side
	// "orphan" state left to reconcile, so Issue #512's original orphan
	// recovery tests (UT-AT-512-001/003/004) are obsolete and removed --
	// what K8s reports is definitionally never orphaned)
	// ========================================
	Describe("UT-AT-512-002: DELETE denied when live RWs exist in K8s", func() {
		It("should return Denied when a live RemediationWorkflow CRD depends on the ActionType, with zero DS calls", func() {
			at := buildActionType("restart-pod", "RestartPod", "kubernaut-system")
			rw := buildRemediationWorkflow("live-wf", "kubernaut-system")
			rw.Spec.ActionType = "RestartPod"

			scheme := newATScheme()
			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(at, rw).
				Build()

			handlerWithK8s := authwebhook.NewActionTypeHandler(mockDS, mockAudit, fakeK8s)
			resp := handlerWithK8s.Handle(ctx, buildATDeleteAdmissionRequest(at))

			Expect(resp.Allowed).To(BeFalse(),
				"DELETE should be Denied when live RWs exist in K8s")
		})
	})

	// ========================================
	// UT-AT-300-011: RW CREATE triggers async activeWorkflowCount update
	// (#1661 Change 8d: count is now a direct K8s-native list, zero DS calls)
	// BR-WORKFLOW-007.5 (Phase 3c cross-update)
	// ========================================
	Describe("UT-AT-300-011: RW CREATE triggers async activeWorkflowCount update", func() {
		It("should update ActionType CRD status.activeWorkflowCount to the live K8s dependents count", func() {
			at := buildActionType("scale-memory-at", "ScaleMemory", "kubernaut-system")
			at.Status.CatalogStatus = sharedtypes.CatalogStatusActive // #1661: RW CREATE gate requires an Active ActionType

			// Five pre-existing RemediationWorkflow CRDs already reference this
			// ActionType -- the count refresh triggered by admitting a sixth
			// (below) must reflect exactly these five, read live from K8s.
			preExisting := make([]client.Object, 0, 6)
			preExisting = append(preExisting, at)
			for i := 0; i < 5; i++ {
				wf := buildRemediationWorkflow(fmt.Sprintf("scale-memory-wf-%d", i), "kubernaut-system")
				wf.Spec.ActionType = "ScaleMemory"
				preExisting = append(preExisting, wf)
			}

			scheme := newATScheme()
			fakeK8s := fake.NewClientBuilder().
				WithScheme(scheme).
				WithIndex(&atv1alpha1.ActionType{}, ".spec.name", func(obj client.Object) []string {
					a := obj.(*atv1alpha1.ActionType)
					return []string{a.Spec.Name}
				}).
				WithObjects(preExisting...).
				WithStatusSubresource(&atv1alpha1.ActionType{}).
				Build()

			rwHandler := authwebhook.NewRemediationWorkflowHandler(
				&mockWorkflowCatalogClient{}, mockAudit, fakeK8s,
			)

			rw := buildRemediationWorkflow("scale-memory-wf-new", "kubernaut-system")
			rw.Spec.ActionType = "ScaleMemory"

			resp := rwHandler.Handle(ctx, buildCreateAdmissionRequest(rw))
			Expect(resp.Allowed).To(BeTrue())

			Eventually(func() int {
				updated := &atv1alpha1.ActionType{}
				if err := fakeK8s.Get(ctx, client.ObjectKeyFromObject(at), updated); err != nil {
					return -1
				}
				return updated.Status.ActiveWorkflowCount
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(5),
				"ActionType CRD status.activeWorkflowCount should reflect the live K8s dependents count "+
					"(the newly-admitted RW itself is not yet persisted by the fake client at Handle()-time, "+
					"mirroring how the real API server only persists after admission returns Allowed)")
		})
	})
})
