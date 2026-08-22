/*
Copyright 2026 Jordi Gil.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentsessionv1alpha1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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
// AgentSession Test Helpers (Issue #2244)
// ========================================

// buildAgentSession constructs a test fixture; name/namespace are fixed to
// "session-1"/"kubernaut-system" since every call site in this file uses
// the same values (unparam) -- only rrName varies across tests.
func buildAgentSession(rrName string) *agentsessionv1alpha1.AgentSession {
	const namespace = "kubernaut-system"
	return &agentsessionv1alpha1.AgentSession{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubernaut.ai/v1alpha1",
			Kind:       "AgentSession",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "session-1",
			Namespace: namespace,
			UID:       "as-uid-001",
		},
		Spec: agentsessionv1alpha1.AgentSessionSpec{
			RemediationRequestRef: agentsessionv1alpha1.ObjectRef{
				Name:      rrName,
				Namespace: namespace,
			},
			IncidentID:    "aianalysis-001",
			RemediationID: rrName,
			SignalName:    "OOMKilled",
			Severity:      "critical",
		},
	}
}

func buildRemediationRequestForAS(name, namespace string) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubernaut.ai/v1alpha1",
			Kind:       "RemediationRequest",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "rr-uid-001",
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			SignalName:        "OOMKilled",
			Severity:          "critical",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-pod",
				Namespace: namespace,
			},
			FiringTime:   metav1.Now(),
			ReceivedTime: metav1.Now(),
		},
	}
}

func buildASCreateAdmissionRequest(as *agentsessionv1alpha1.AgentSession) admission.Request {
	raw, _ := json.Marshal(as)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "as-admission-create-001",
			Kind: metav1.GroupVersionKind{
				Group: "kubernaut.ai", Version: "v1alpha1", Kind: "AgentSession",
			},
			Name:      as.Name,
			Namespace: as.Namespace,
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

func buildASUpdateAdmissionRequest(oldAS, newAS *agentsessionv1alpha1.AgentSession) admission.Request {
	oldRaw, _ := json.Marshal(oldAS)
	newRaw, _ := json.Marshal(newAS)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "as-admission-update-001",
			Kind: metav1.GroupVersionKind{
				Group: "kubernaut.ai", Version: "v1alpha1", Kind: "AgentSession",
			},
			Name:      newAS.Name,
			Namespace: newAS.Namespace,
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

func newASScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = agentsessionv1alpha1.AddToScheme(s)
	_ = remediationv1.AddToScheme(s)
	return s
}

// ========================================
// Tests
// ========================================

var _ = Describe("AgentSession Admission Handler (#2244, BR-AA-KA-065.13)", func() {
	var (
		ctx       context.Context
		mockAudit *MockAuditStoreRW
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockAudit = &MockAuditStoreRW{}
	})

	// ========================================
	// UT-AS-2244-001: k8sClient nil skips the gate (best-effort precedent)
	// ========================================
	Describe("UT-AS-2244-001: k8sClient nil skips the existence gate", func() {
		It("should return Allowed with zero k8s calls when h.k8sClient is nil (matches validateActionTypeExists precedent)", func() {
			handler := authwebhook.NewAgentSessionHandler(mockAudit, nil)
			as := buildAgentSession("rr-does-not-matter")

			resp := handler.Handle(ctx, buildASCreateAdmissionRequest(as))

			Expect(resp.Allowed).To(BeTrue(),
				"CREATE should be Allowed when k8sClient is not configured (unit-test best-effort skip)")
			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeASAdmittedCreate))
		})
	})

	// ========================================
	// UT-AS-2244-002: CREATE allowed when RemediationRequestRef resolves
	// ========================================
	Describe("UT-AS-2244-002: CREATE allowed when RemediationRequestRef resolves to a real RemediationRequest", func() {
		It("should return Allowed and emit an admitted audit event", func() {
			rr := buildRemediationRequestForAS("rr-001", "kubernaut-system")
			as := buildAgentSession("rr-001")

			fakeK8s := fake.NewClientBuilder().
				WithScheme(newASScheme()).
				WithObjects(rr).
				Build()
			handler := authwebhook.NewAgentSessionHandler(mockAudit, fakeK8s)

			resp := handler.Handle(ctx, buildASCreateAdmissionRequest(as))

			Expect(resp.Allowed).To(BeTrue(),
				"CREATE should be Allowed when RemediationRequestRef resolves: %v", resp.Result)

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("agentsession.admitted.create"))
			Expect(string(event.EventCategory)).To(Equal("agentsession"))
			Expect(event.EventAction).To(Equal("admitted"))
			Expect(string(event.EventOutcome)).To(Equal("success"))
			Expect(event.ActorID.Value).To(Equal(testUserEmail))
			Expect(event.ResourceType.Value).To(Equal("AgentSession"))
			Expect(event.ResourceID.Value).To(Equal("session-1"))

			payload, ok := event.EventData.GetAgentSessionWebhookAuditPayload()
			Expect(ok).To(BeTrue(), "EventData should contain AgentSessionWebhookAuditPayload")
			Expect(payload.CrdName).To(Equal("session-1"))
			Expect(payload.CrdNamespace).To(Equal("kubernaut-system"))
			Expect(payload.Action).To(Equal(ogenclient.AgentSessionWebhookAuditPayloadActionCreate))
			Expect(payload.RemediationRequestRef.Value).To(Equal("rr-001"))
		})
	})

	// ========================================
	// UT-AS-2244-003: CREATE denied when RemediationRequestRef does not exist
	// ========================================
	Describe("UT-AS-2244-003: CREATE denied when RemediationRequestRef does not exist (SI-10)", func() {
		It("should return Denied with a clear message and emit a denied audit event", func() {
			as := buildAgentSession("rr-nonexistent")

			fakeK8s := fake.NewClientBuilder().
				WithScheme(newASScheme()).
				Build()
			handler := authwebhook.NewAgentSessionHandler(mockAudit, fakeK8s)

			resp := handler.Handle(ctx, buildASCreateAdmissionRequest(as))

			Expect(resp.Allowed).To(BeFalse(),
				"CREATE should be Denied when RemediationRequestRef does not resolve")
			Expect(resp.Result.Message).To(ContainSubstring("rr-nonexistent"))
			Expect(resp.Result.Message).To(ContainSubstring("does not exist"))

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("agentsession.denied.create"))
			Expect(string(event.EventOutcome)).To(Equal("failure"))
			Expect(event.EventAction).To(Equal("denied"))

			payload, ok := event.EventData.GetAgentSessionWebhookAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Action).To(Equal(ogenclient.AgentSessionWebhookAuditPayloadActionDenied))
			Expect(payload.DenialReason.Value).To(ContainSubstring("rr-nonexistent"))
		})
	})

	// ========================================
	// UT-AS-2244-004: CREATE denied when RemediationRequestRef points to a
	// different namespace (RR lookup is scoped to req.Namespace)
	// ========================================
	Describe("UT-AS-2244-004: CREATE denied when the RR exists only in a different namespace", func() {
		It("should return Denied because the existence check is scoped to the AgentSession's own namespace", func() {
			rr := buildRemediationRequestForAS("rr-001", "other-namespace")
			as := buildAgentSession("rr-001")

			fakeK8s := fake.NewClientBuilder().
				WithScheme(newASScheme()).
				WithObjects(rr).
				Build()
			handler := authwebhook.NewAgentSessionHandler(mockAudit, fakeK8s)

			resp := handler.Handle(ctx, buildASCreateAdmissionRequest(as))

			Expect(resp.Allowed).To(BeFalse(),
				"a RemediationRequest with the same name in a different namespace must not satisfy the gate")
		})
	})

	// ========================================
	// UT-AS-2244-005: CREATE denied (fail-closed) when the lookup itself errors
	// ========================================
	Describe("UT-AS-2244-005: CREATE denied when the RemediationRequest lookup errors (fail-closed)", func() {
		It("should return Denied when Get returns a non-NotFound error", func() {
			as := buildAgentSession("rr-001")
			erroringK8s := fake.NewClientBuilder().
				WithScheme(newASScheme()).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
						return fmt.Errorf("simulated K8s API failure")
					},
				}).
				Build()
			handler := authwebhook.NewAgentSessionHandler(mockAudit, erroringK8s)

			resp := handler.Handle(ctx, buildASCreateAdmissionRequest(as))

			Expect(resp.Allowed).To(BeFalse(),
				"CREATE should be Denied (fail-closed) when the existence lookup itself errors")
			Expect(resp.Result.Message).To(ContainSubstring("lookup failed"))

			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeASDeniedCreate))
		})
	})

	// ========================================
	// UT-AS-2244-006: UPDATE/DELETE are not intercepted (CEL-immutable spec)
	// ========================================
	Describe("UT-AS-2244-006: UPDATE and DELETE operations are not intercepted", func() {
		It("should return Allowed for UPDATE without any k8s calls or audit events (spec is CEL-immutable)", func() {
			handler := authwebhook.NewAgentSessionHandler(mockAudit, nil)
			as := buildAgentSession("rr-001")

			resp := handler.Handle(ctx, buildASUpdateAdmissionRequest(as, as))

			Expect(resp.Allowed).To(BeTrue(),
				"UPDATE should be Allowed unconditionally -- AgentSessionSpec is CEL-immutable, nothing to re-validate")
			Expect(mockAudit.StoredEvents).To(BeEmpty(),
				"No audit event should be emitted for an operation this handler does not gate")
		})
	})

	// ========================================
	// UT-AS-2244-007: CREATE denied when the object is unmarshalable
	// ========================================
	Describe("UT-AS-2244-007: CREATE denied when the AgentSession object cannot be unmarshaled", func() {
		It("should return Denied and emit a denied audit event", func() {
			handler := authwebhook.NewAgentSessionHandler(mockAudit, nil)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: "as-admission-bad-001",
					Kind: metav1.GroupVersionKind{
						Group: "kubernaut.ai", Version: "v1alpha1", Kind: "AgentSession",
					},
					Name:      "session-bad",
					Namespace: "kubernaut-system",
					Operation: admissionv1.Create,
					UserInfo: authv1.UserInfo{
						Username: testUserEmail,
						UID:      testUserUID,
					},
					Object: runtime.RawExtension{Raw: []byte("not-json")},
				},
			}

			resp := handler.Handle(ctx, req)

			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result.Message).To(ContainSubstring("failed to unmarshal"))
			Expect(mockAudit.StoredEvents).To(HaveLen(1))
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeASDeniedCreate))
		})
	})
})
