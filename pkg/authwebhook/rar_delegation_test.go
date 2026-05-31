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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Trusted Intermediary Delegation Tests
//
// Business Requirement: BR-AUTH-001 (SOC2 CC8.1 Operator Attribution)
// FedRAMP Control Objectives:
//   - AC-6 (Least Privilege): Only AF SA can delegate approvals
//   - AU-3 (Content of Audit Records): Delegation recorded with intermediary + human identity
//   - SI-4 (Information System Monitoring): Forgery detected for non-trusted callers
//   - AC-3 (Access Enforcement): decidedVia cannot be spoofed by non-AF callers
//
// Design: DD-AUTH-MCP-001 v3.0 (Trusted Intermediary Pattern)
// Same model as AF-to-KA acting_user injection.

const (
	testAFSA        = "system:serviceaccount:kubernaut-system:apifrontend"
	testHumanUser   = "alice@example.com"
	testAttackerSA  = "system:serviceaccount:evil-ns:attacker"
	testDirectUser  = "operator@kubernaut.ai"
	testPodNS       = "kubernaut-system"
)

var _ = Describe("Trusted Intermediary Delegation (DD-AUTH-MCP-001 v3.0)", func() {
	var (
		ctx       context.Context
		handler   *authwebhook.RemediationApprovalRequestAuthHandler
		mockStore *MockAuditStore
		decoder   admission.Decoder
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, nil, testPodNS)

		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		decoder = admission.NewDecoder(scheme)
		Expect(handler.InjectDecoder(decoder)).To(Succeed())
	})

	// =========================================================================
	// AC-6: LEAST PRIVILEGE — Only AF SA can delegate
	// =========================================================================
	Describe("AC-6: Least Privilege — Only AF SA can delegate approval decisions", func() {
		It("UT-AW-DELEGATION-001: AF SA approval preserves human decidedBy from patch body", func() {
			// GIVEN: AF SA patches RAR with decidedBy = human user (JWT-authenticated by AF)
			// WHEN: Webhook processes the admission request
			// THEN: decidedBy is preserved (not overwritten with AF SA)
			resp := handleApproval(handler, ctx, testAFSA, "k8s-af-uid", testHumanUser, "Approved", "Looks good")

			Expect(resp.Allowed).To(BeTrue(), "AF SA delegation must be allowed")
			Expect(resp.Patches).NotTo(BeEmpty(), "Webhook must apply patches")

			finalDecidedBy := extractFinalDecidedBy(resp, testHumanUser)
			Expect(finalDecidedBy).To(Equal(testHumanUser),
				"AC-6: decidedBy must be preserved as the human identity when AF SA delegates")
		})

		It("UT-AW-DELEGATION-002: Non-AF caller has decidedBy overwritten with authenticated identity", func() {
			// GIVEN: A non-AF caller (direct user) patches RAR with decidedBy = "someone-else"
			// WHEN: Webhook processes the admission request
			// THEN: decidedBy is OVERWRITTEN with the authenticated K8s caller (forgery prevention)
			resp := handleApproval(handler, ctx, testDirectUser, "k8s-user-uid", "forged-identity@evil.com", "Approved", "")

			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patches).NotTo(BeEmpty())

			finalDecidedBy := extractFinalDecidedBy(resp, "forged-identity@evil.com")
			Expect(finalDecidedBy).To(Equal(testDirectUser),
				"AC-6: Non-AF caller must have decidedBy overwritten with authenticated identity")
		})

		It("UT-AW-DELEGATION-003: AF SA with empty decidedBy falls back to standard behavior", func() {
			// GIVEN: AF SA patches RAR but decidedBy is empty (bug or misconfiguration)
			// WHEN: Webhook processes the admission request
			// THEN: decidedBy is set to AF SA (standard overwrite behavior, no delegation)
			resp := handleApproval(handler, ctx, testAFSA, "k8s-af-uid", "", "Approved", "")

			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patches).NotTo(BeEmpty())

			finalDecidedBy := extractFinalDecidedBy(resp, "")
			Expect(finalDecidedBy).To(Equal(testAFSA),
				"AC-6: AF SA with empty decidedBy must fall back to standard overwrite")
		})
	})

	// =========================================================================
	// AC-3: ACCESS ENFORCEMENT — decidedVia controlled by webhook
	// =========================================================================
	Describe("AC-3: Access Enforcement — decidedVia cannot be spoofed", func() {
		It("UT-AW-DELEGATION-004: AF SA delegation sets decidedVia to AF SA identity", func() {
			// GIVEN: AF SA delegates approval on behalf of human
			// WHEN: Webhook processes
			// THEN: decidedVia is set to AF SA (webhook-controlled attribution)
			resp := handleApproval(handler, ctx, testAFSA, "k8s-af-uid", testHumanUser, "Approved", "LGTM")

			Expect(resp.Allowed).To(BeTrue())

			finalVia := extractFinalDecidedVia(resp)
			Expect(finalVia).To(Equal(testAFSA),
				"AC-3: decidedVia must be set to AF SA when delegation occurs")
		})

		It("UT-AW-DELEGATION-005: Non-AF caller cannot spoof decidedVia", func() {
			// GIVEN: An attacker SA patches RAR and attempts to set decidedVia to look like AF
			// WHEN: Webhook processes
			// THEN: decidedVia is CLEARED (webhook controls this field, prevents spoofing)
			resp := handleApprovalWithDecidedVia(handler, ctx, testAttackerSA, "k8s-attacker-uid",
				testHumanUser, "Approved", "", testAFSA)

			Expect(resp.Allowed).To(BeTrue())

			finalVia := extractFinalDecidedVia(resp)
			Expect(finalVia).To(BeEmpty(),
				"AC-3: Non-AF caller must have decidedVia cleared by webhook (prevent spoofing)")
		})

		It("UT-AW-DELEGATION-006: Direct user approval has empty decidedVia", func() {
			// GIVEN: A human operator approves directly via kubectl (not through AF)
			// WHEN: Webhook processes
			// THEN: decidedVia is empty (no intermediary involved)
			resp := handleApproval(handler, ctx, testDirectUser, "k8s-user-uid", "", "Rejected", "Unsafe change")

			Expect(resp.Allowed).To(BeTrue())

			finalVia := extractFinalDecidedVia(resp)
			Expect(finalVia).To(BeEmpty(),
				"AC-3: Direct approvals must have empty decidedVia")
		})
	})

	// =========================================================================
	// AU-3: CONTENT OF AUDIT RECORDS — Delegation captured in audit trail
	// =========================================================================
	Describe("AU-3: Audit Records — Delegation captured with full context", func() {
		It("UT-AW-DELEGATION-007: Delegation audit event records intermediary as actor", func() {
			// GIVEN: AF SA delegates approval
			// WHEN: Webhook emits audit event
			// THEN: actor_type = "service", actor_id = AF SA (factual K8s caller)
			resp := handleApproval(handler, ctx, testAFSA, "k8s-af-uid", testHumanUser, "Approved", "")

			Expect(resp.Allowed).To(BeTrue())
			Expect(mockStore.StoredEvents).To(HaveLen(1), "AU-3: Audit event must be emitted")

			event := mockStore.StoredEvents[0]
			Expect(event.ActorType.Value).To(Equal("service"),
				"AU-3: Delegation audit must record actor_type as 'service' (intermediary)")
			Expect(event.ActorID.Value).To(Equal(testAFSA),
				"AU-3: Delegation audit must record actor_id as AF SA (factual K8s caller)")
		})

		It("UT-AW-DELEGATION-008: Delegation audit payload includes delegated_user", func() {
			// GIVEN: AF SA delegates approval for alice
			// WHEN: Webhook emits audit event
			// THEN: event_data includes delegated_user = "alice@example.com"
			resp := handleApproval(handler, ctx, testAFSA, "k8s-af-uid", testHumanUser, "Approved", "")

			Expect(resp.Allowed).To(BeTrue())
			Expect(mockStore.StoredEvents).To(HaveLen(1))

			event := mockStore.StoredEvents[0]
			payload := extractAuditPayloadDelegation(event)
			Expect(payload.DelegatedUser).To(Equal(testHumanUser),
				"AU-3: Audit payload must include delegated_user for forensic reconstruction")
			Expect(payload.DelegatedVia).To(Equal(testAFSA),
				"AU-3: Audit payload must include delegated_via for intermediary attribution")
		})

		It("UT-AW-DELEGATION-009: Direct approval audit records user as actor (not service)", func() {
			// GIVEN: Direct user approval (no delegation)
			// WHEN: Webhook emits audit event
			// THEN: actor_type = "user", actor_id = direct caller
			resp := handleApproval(handler, ctx, testDirectUser, "k8s-user-uid", "", "Approved", "")

			Expect(resp.Allowed).To(BeTrue())
			Expect(mockStore.StoredEvents).To(HaveLen(1))

			event := mockStore.StoredEvents[0]
			Expect(event.ActorType.Value).To(Equal("user"),
				"AU-3: Direct approval must record actor_type as 'user'")
			Expect(event.ActorID.Value).To(Equal(testDirectUser),
				"AU-3: Direct approval must record actor_id as the direct caller")
		})
	})

	// =========================================================================
	// SI-4: MONITORING — Forgery detection for non-trusted callers
	// =========================================================================
	Describe("SI-4: Monitoring — Forgery detection behavior", func() {
		It("UT-AW-DELEGATION-010: Forgery detector does NOT fire for AF SA delegation", func() {
			// GIVEN: AF SA sets decidedBy = human (legitimate delegation)
			// WHEN: Webhook processes
			// THEN: No forgery warning logged (AF is trusted, this is not forgery)
			// Validation: The approval succeeds AND decidedBy is preserved (not overwritten)
			resp := handleApproval(handler, ctx, testAFSA, "k8s-af-uid", testHumanUser, "Approved", "")

			Expect(resp.Allowed).To(BeTrue())
			finalDecidedBy := extractFinalDecidedBy(resp, testHumanUser)
			Expect(finalDecidedBy).To(Equal(testHumanUser),
				"SI-4: AF SA delegation must not trigger forgery prevention (decidedBy preserved)")
		})

		It("UT-AW-DELEGATION-011: Forgery detector FIRES for non-AF caller with decidedBy", func() {
			// GIVEN: Non-AF caller attempts to set decidedBy to a different identity
			// WHEN: Webhook processes
			// THEN: decidedBy is overwritten (forgery prevented), request still allowed
			resp := handleApproval(handler, ctx, testAttackerSA, "k8s-attacker-uid",
				"victim@company.com", "Approved", "")

			Expect(resp.Allowed).To(BeTrue(), "Request allowed but forgery prevented via overwrite")
			finalDecidedBy := extractFinalDecidedBy(resp, "victim@company.com")
			Expect(finalDecidedBy).To(Equal(testAttackerSA),
				"SI-4: Non-AF forgery attempt must result in decidedBy = authenticated caller")
		})
	})

	// =========================================================================
	// HARDCODED AF SA DERIVATION — Namespace-aware
	// =========================================================================
	Describe("AF SA Identity Derivation (POD_NAMESPACE)", func() {
		It("UT-AW-DELEGATION-012: BuildTrustedAFSA constructs correct SA from namespace", func() {
			sa := authwebhook.BuildTrustedAFSA("kubernaut-system")
			Expect(sa).To(Equal("system:serviceaccount:kubernaut-system:apifrontend"))
		})

		It("UT-AW-DELEGATION-013: BuildTrustedAFSA works with custom namespace", func() {
			sa := authwebhook.BuildTrustedAFSA("custom-ns")
			Expect(sa).To(Equal("system:serviceaccount:custom-ns:apifrontend"))
		})

		It("UT-AW-DELEGATION-014: Handler only trusts AF SA from its own namespace", func() {
			// GIVEN: Handler configured with namespace "kubernaut-system"
			// WHEN: A different namespace's apifrontend SA tries to delegate
			// THEN: Delegation rejected (decidedBy overwritten)
			differentNsAF := "system:serviceaccount:other-ns:apifrontend"
			resp := handleApproval(handler, ctx, differentNsAF, "k8s-uid-other",
				testHumanUser, "Approved", "")

			Expect(resp.Allowed).To(BeTrue())
			finalDecidedBy := extractFinalDecidedBy(resp, testHumanUser)
			Expect(finalDecidedBy).To(Equal(differentNsAF),
				"AC-6: AF SA from different namespace must NOT be trusted for delegation")
		})
	})
})

// =========================================================================
// TEST HELPERS
// =========================================================================

func handleApproval(
	handler *authwebhook.RemediationApprovalRequestAuthHandler,
	ctx context.Context,
	callerUsername, callerUID, decidedBy, decision, message string,
) admission.Response {
	return handleApprovalWithDecidedVia(handler, ctx, callerUsername, callerUID,
		decidedBy, decision, message, "")
}

func handleApprovalWithDecidedVia(
	handler *authwebhook.RemediationApprovalRequestAuthHandler,
	ctx context.Context,
	callerUsername, callerUID, decidedBy, decision, message, decidedVia string,
) admission.Response {
	rar := &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rar-delegation-test",
			Namespace: "kubernaut-system",
			UID:       "rar-uid-delegation",
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name: "rr-parent-delegation",
			},
			AIAnalysisRef: remediationv1.ObjectRef{
				Name: "aia-delegation",
			},
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			Decision:        remediationv1.ApprovalDecision(decision),
			DecidedBy:       decidedBy,
			DecisionMessage: message,
			DecidedVia:      decidedVia,
		},
	}

	rawRAR, err := json.Marshal(rar)
	Expect(err).NotTo(HaveOccurred())

	// OLD object: no decision (this is a NEW decision)
	oldRAR := &remediationv1.RemediationApprovalRequest{
		ObjectMeta: rar.ObjectMeta,
		Spec:       rar.Spec,
		Status:     remediationv1.RemediationApprovalRequestStatus{},
	}
	rawOldRAR, err := json.Marshal(oldRAR)
	Expect(err).NotTo(HaveOccurred())

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "admission-delegation-test",
			Kind: metav1.GroupVersionKind{
				Group:   "remediation.kubernaut.ai",
				Version: "v1alpha1",
				Kind:    "RemediationApprovalRequest",
			},
			Name:      rar.Name,
			Namespace: rar.Namespace,
			Operation: admissionv1.Update,
			UserInfo: authv1.UserInfo{
				Username: callerUsername,
				UID:      callerUID,
			},
			Object:    runtime.RawExtension{Raw: rawRAR},
			OldObject: runtime.RawExtension{Raw: rawOldRAR},
		},
	}

	return handler.Handle(ctx, req)
}

// extractFinalDecidedBy returns the decidedBy that would be in the final object.
// If the webhook didn't patch it, the original submitted value is preserved.
func extractFinalDecidedBy(resp admission.Response, originalDecidedBy string) string {
	for _, patch := range resp.Patches {
		if patch.Path == "/status/decidedBy" {
			if val, ok := patch.Value.(string); ok {
				return val
			}
		}
	}
	return originalDecidedBy
}

// extractFinalDecidedVia returns the decidedVia from patches (add or replace).
func extractFinalDecidedVia(resp admission.Response) string {
	for _, patch := range resp.Patches {
		if patch.Path == "/status/decidedVia" {
			if val, ok := patch.Value.(string); ok {
				return val
			}
		}
	}
	return ""
}

func extractAuditPayloadDelegation(event *ogenclient.AuditEventRequest) struct {
	DelegatedUser string
	DelegatedVia  string
} {
	payload := event.EventData.RemediationApprovalAuditPayload
	return struct {
		DelegatedUser string
		DelegatedVia  string
	}{
		DelegatedUser: payload.DelegatedUser.Value,
		DelegatedVia:  payload.DelegatedVia.Value,
	}
}
