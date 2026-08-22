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

package authwebhook

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentsessionv1alpha1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	auditclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// E2E-AS-2244-DENY (Issue #2244, BR-AA-KA-065.13)
// ========================================
//
// Authority: BR-AA-KA-065.13, DD-WEBHOOK-001 v1.5, DD-WORKFLOW-018
//
// Proves AW's AgentSession->RemediationRequest existence gate end-to-end
// against a real cluster: an AgentSession referencing a non-existent
// RemediationRequest is rejected at admission (SI-10, input validation at
// the trust boundary before KA's dispatcher ever observes the object),
// never persists as a CRD, and the denial itself is fully auditable.
// Mirrors #1661's E2E-AT-3XX-DENY pattern (05_actiontype_gate_and_format_test.go)
// for the same existence-gate shape.

// buildAgentSessionCRD constructs an AgentSession CRD object referencing
// rrName in sharedNamespace (the AgentSession MUST be created in the same
// namespace as the RemediationRequest it investigates).
func buildAgentSessionCRD(crdName, rrName string) *agentsessionv1alpha1.AgentSession {
	return &agentsessionv1alpha1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: sharedNamespace,
		},
		Spec: agentsessionv1alpha1.AgentSessionSpec{
			RemediationRequestRef: agentsessionv1alpha1.ObjectRef{
				Name:      rrName,
				Namespace: sharedNamespace,
			},
			IncidentID:    fmt.Sprintf("e2e-aa-%s", uuid.New().String()[:8]),
			RemediationID: rrName,
			SignalName:    "OOMKilled",
			Severity:      "critical",
		},
	}
}

// buildRemediationRequestCRD constructs a minimal, valid RemediationRequest
// CRD object in sharedNamespace, for the allow-path counterpart of the
// existence gate.
func buildRemediationRequestCRD(crdName string) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: sharedNamespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			SignalName:        "OOMKilled",
			Severity:          "critical",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "e2e-agentsession-test-pod",
				Namespace: sharedNamespace,
			},
			FiringTime:   metav1.Now(),
			ReceivedTime: metav1.Now(),
		},
	}
}

var _ = Describe("E2E: AW AgentSession Existence Gate (#2244, BR-AA-KA-065.13)", Serial, Label("e2e", "agentsession-gate"), func() {
	var crdCleanup []string

	AfterEach(func() {
		for _, name := range crdCleanup {
			as := &agentsessionv1alpha1.AgentSession{}
			key := types.NamespacedName{Name: name, Namespace: sharedNamespace}
			if err := k8sClient.Get(ctx, key, as); err == nil {
				_ = k8sClient.Delete(ctx, as)
			}
		}
		crdCleanup = nil
	})

	// ========================================
	// E2E-AS-2244-DENY: AgentSession referencing a non-existent
	// RemediationRequest is denied and never persists.
	// ========================================
	It("E2E-AS-2244-DENY: AgentSession referencing a non-existent RemediationRequest is denied and never persisted", func() {
		crdName := fmt.Sprintf("e2e-as-deny-%s", uuid.New().String()[:8])
		nonExistentRR := fmt.Sprintf("e2e-nonexistent-rr-%s", uuid.New().String()[:8])

		By("Attempting to create an AgentSession referencing a non-existent RemediationRequest")
		as := buildAgentSessionCRD(crdName, nonExistentRR)

		err := k8sClient.Create(ctx, as)
		Expect(err).To(HaveOccurred(), "CREATE should be Denied by the webhook (AW's AgentSession existence gate)")
		Expect(err.Error()).To(ContainSubstring("does not exist"),
			"Denial reason should match AgentSessionHandler.validateRemediationRequestExists wording")
		Expect(err.Error()).To(ContainSubstring(nonExistentRR))

		By("Verifying the AgentSession never persisted in the cluster (denied CREATE)")
		getErr := k8sClient.Get(ctx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, &agentsessionv1alpha1.AgentSession{})
		Expect(getErr).To(HaveOccurred(), "Denied CREATE should mean the CRD was never persisted")

		By("Verifying the agentsession.denied.create audit event is recorded (BR-AUDIT-005)")
		authAuditClient := createAuthenticatedAuditClient()
		Expect(authAuditClient).ToNot(BeNil(), "DD-AUTH-014 authenticated audit client must be available in E2E")

		var deniedPayload auditclient.AgentSessionWebhookAuditPayload
		Eventually(func() bool {
			events, qErr := authAuditClient.QueryAuditEvents(ctx, auditclient.QueryAuditEventsParams{
				EventType:   auditclient.NewOptString("agentsession.denied.create"),
				DetailKey:   auditclient.NewOptString("crd_name"),
				DetailValue: auditclient.NewOptString(crdName),
			})
			if qErr != nil {
				return false
			}
			for _, evt := range events.Data {
				payload, ok := evt.EventData.GetAgentSessionWebhookAuditPayload()
				if ok && payload.CrdName == crdName {
					deniedPayload = payload
					return true
				}
			}
			return false
		}, 15*time.Second, 1*time.Second).Should(BeTrue(),
			"Audit trail should contain an agentsession.denied.create event for this CRD")

		Expect(deniedPayload.CrdNamespace).To(Equal(sharedNamespace))
		Expect(deniedPayload.Action).To(Equal(auditclient.AgentSessionWebhookAuditPayloadActionDenied))
		Expect(deniedPayload.DenialReason.IsSet()).To(BeTrue())
		Expect(deniedPayload.DenialReason.Value).To(ContainSubstring(nonExistentRR))

		GinkgoWriter.Printf("✅ AgentSession referencing non-existent RemediationRequest %q correctly denied, zero persistence, audit trail captured\n",
			nonExistentRR)
	})

	// ========================================
	// E2E-AS-2244-ALLOW: AgentSession referencing a real RemediationRequest
	// is admitted and persists, with the admitted audit event recorded.
	// ========================================
	It("E2E-AS-2244-ALLOW: AgentSession referencing a real RemediationRequest is admitted and persisted", func() {
		rrName := fmt.Sprintf("e2e-as-allow-rr-%s", uuid.New().String()[:8])
		rr := buildRemediationRequestCRD(rrName)
		Expect(k8sClient.Create(ctx, rr)).To(Succeed(), "RemediationRequest CRD creation should succeed")

		crdName := fmt.Sprintf("e2e-as-allow-%s", uuid.New().String()[:8])
		as := buildAgentSessionCRD(crdName, rrName)

		By("Creating an AgentSession referencing a real RemediationRequest")
		Expect(k8sClient.Create(ctx, as)).To(Succeed(), "CREATE should be Allowed when RemediationRequestRef resolves")
		crdCleanup = append(crdCleanup, crdName)

		By("Verifying the AgentSession persisted in the cluster")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crdName, Namespace: sharedNamespace}, &agentsessionv1alpha1.AgentSession{})).To(Succeed())

		By("Verifying the agentsession.admitted.create audit event is recorded (BR-AUDIT-005)")
		authAuditClient := createAuthenticatedAuditClient()
		Expect(authAuditClient).ToNot(BeNil(), "DD-AUTH-014 authenticated audit client must be available in E2E")

		var admittedPayload auditclient.AgentSessionWebhookAuditPayload
		Eventually(func() bool {
			events, qErr := authAuditClient.QueryAuditEvents(ctx, auditclient.QueryAuditEventsParams{
				EventType:   auditclient.NewOptString("agentsession.admitted.create"),
				DetailKey:   auditclient.NewOptString("crd_name"),
				DetailValue: auditclient.NewOptString(crdName),
			})
			if qErr != nil {
				return false
			}
			for _, evt := range events.Data {
				payload, ok := evt.EventData.GetAgentSessionWebhookAuditPayload()
				if ok && payload.CrdName == crdName {
					admittedPayload = payload
					return true
				}
			}
			return false
		}, 15*time.Second, 1*time.Second).Should(BeTrue(),
			"Audit trail should contain an agentsession.admitted.create event for this CRD")

		Expect(admittedPayload.Action).To(Equal(auditclient.AgentSessionWebhookAuditPayloadActionCreate))
		Expect(admittedPayload.RemediationRequestRef.Value).To(Equal(rrName))

		GinkgoWriter.Printf("✅ AgentSession referencing real RemediationRequest %q correctly admitted and persisted\n", rrName)
	})
})
