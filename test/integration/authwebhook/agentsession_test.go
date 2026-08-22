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
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Issue #2244, BR-AA-KA-065.13: IT coverage for AgentSessionHandler's
// CREATE-only RemediationRequest existence gate, proving the production
// wiring path (real envtest apiserver Get, real Manager-backed k8sClient,
// real audit store round-trip to DS Postgres) -- not just the fake-client
// UT coverage in pkg/authwebhook/agentsession_handler_test.go.
var _ = Describe("#2244 AgentSession Admission Existence Gate (BR-AA-KA-065.13)", Label("integration", "authwebhook", "agentsession"), func() {

	var asHandler *authwebhook.AgentSessionHandler

	BeforeEach(func() {
		asHandler = authwebhook.NewAgentSessionHandler(auditStore, k8sManager.GetClient())
	})

	flushAndQuery := func(correlationID, eventType string) []ogenclient.AuditEvent {
		flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
		defer flushCancel()
		Expect(auditStore.Flush(flushCtx)).To(Succeed(), "Audit store flush should succeed")

		var events []ogenclient.AuditEvent
		Eventually(func() bool {
			found, qErr := queryAuditEvents(dsClient, correlationID, &eventType)
			if qErr != nil {
				return false
			}
			events = found
			return len(events) > 0
		}, 15*time.Second, 1*time.Second).Should(BeTrue(),
			fmt.Sprintf("Expected %s audit event with correlation_id=%s", eventType, correlationID))
		return events
	}

	uniqueID := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}

	buildRR := func(name string) *remediationv1.RemediationRequest {
		return &remediationv1.RemediationRequest{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "kubernaut.ai/v1alpha1",
				Kind:       "RemediationRequest",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: defaultFixture,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				SignalName:        "OOMKilled",
				Severity:          "critical",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "it-agentsession-test-pod",
					Namespace: defaultFixture,
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
			},
		}
	}

	buildAS := func(name, rrName string) *agentsessionv1alpha1.AgentSession {
		return &agentsessionv1alpha1.AgentSession{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "kubernaut.ai/v1alpha1",
				Kind:       "AgentSession",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: defaultFixture,
			},
			Spec: agentsessionv1alpha1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1alpha1.ObjectRef{
					Name:      rrName,
					Namespace: defaultFixture,
				},
				IncidentID:    uniqueID("it-aa"),
				RemediationID: rrName,
				SignalName:    "OOMKilled",
				Severity:      "critical",
			},
		}
	}

	asCreateAdmissionRequest := func(as *agentsessionv1alpha1.AgentSession, uid string) admission.Request {
		raw, err := json.Marshal(as)
		Expect(err).ToNot(HaveOccurred())
		return admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID: types.UID(uid),
				Kind: metav1.GroupVersionKind{
					Group: "kubernaut.ai", Version: "v1alpha1", Kind: "AgentSession",
				},
				Name:      as.Name,
				Namespace: as.Namespace,
				Operation: admissionv1.Create,
				UserInfo: authv1.UserInfo{
					Username: "it-agentsession-user@kubernaut.ai",
					UID:      "it-agentsession-uid",
					Groups:   []string{"system:masters"},
				},
				Object: runtime.RawExtension{Raw: raw},
			},
		}
	}

	It("IT-AS-2244-001: CREATE is allowed and admitted audit persisted when RemediationRequestRef resolves to a real RR in etcd", func() {
		rrName := uniqueID("it-rr-exists")
		rr := buildRR(rrName)
		Expect(k8sClient.Create(ctx, rr)).To(Succeed(), "RemediationRequest CRD creation should succeed")

		uid := uniqueID("as-create-allowed")
		as := buildAS(uniqueID("it-as-allowed"), rrName)

		resp := asHandler.Handle(ctx, asCreateAdmissionRequest(as, uid))
		Expect(resp.Allowed).To(BeTrue(), "AgentSession CREATE should be allowed when RemediationRequestRef resolves via real apiserver Get: %v", resp.Result)

		events := flushAndQuery(uid, authwebhook.EventTypeASAdmittedCreate)
		Expect(events).To(HaveLen(1))
		Expect(events[0].CorrelationID).To(Equal(uid))
		validateEventMetadata(events[0], "agentsession")

		payload, ok := events[0].EventData.GetAgentSessionWebhookAuditPayload()
		Expect(ok).To(BeTrue(), "event_data should decode as AgentSessionWebhookAuditPayload")
		Expect(payload.CrdName).To(Equal(as.Name))
		Expect(payload.CrdNamespace).To(Equal(defaultFixture))
		Expect(payload.Action).To(Equal(ogenclient.AgentSessionWebhookAuditPayloadActionCreate))
		Expect(payload.RemediationRequestRef.Value).To(Equal(rrName))
	})

	It("IT-AS-2244-002: CREATE is denied and denied audit persisted when RemediationRequestRef does not exist in etcd (SI-10)", func() {
		uid := uniqueID("as-create-denied")
		as := buildAS(uniqueID("it-as-denied"), uniqueID("rr-never-created"))

		resp := asHandler.Handle(ctx, asCreateAdmissionRequest(as, uid))
		Expect(resp.Allowed).To(BeFalse(), "AgentSession CREATE should be denied when RemediationRequestRef does not resolve via real apiserver Get")
		Expect(resp.Result.Message).To(ContainSubstring("does not exist"))

		events := flushAndQuery(uid, authwebhook.EventTypeASDeniedCreate)
		Expect(events).To(HaveLen(1))
		Expect(events[0].CorrelationID).To(Equal(uid))
		validateEventMetadata(events[0], "agentsession")

		payload, ok := events[0].EventData.GetAgentSessionWebhookAuditPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.Action).To(Equal(ogenclient.AgentSessionWebhookAuditPayloadActionDenied))
		Expect(payload.DenialReason.IsSet()).To(BeTrue())
	})

	It("IT-AS-2244-003: CREATE is denied when the RR exists only in a different namespace (namespace-scoped lookup)", func() {
		rrName := uniqueID("it-rr-wrong-ns")
		otherNS := "kube-system" // pre-existing built-in namespace, avoids an extra Namespace-creation round trip
		rr := buildRR(rrName)
		rr.Namespace = otherNS
		Expect(k8sClient.Create(ctx, rr)).To(Succeed(), "RemediationRequest CRD creation in other namespace should succeed")

		uid := uniqueID("as-create-wrong-ns")
		as := buildAS(uniqueID("it-as-wrong-ns"), rrName)

		resp := asHandler.Handle(ctx, asCreateAdmissionRequest(as, uid))
		Expect(resp.Allowed).To(BeFalse(),
			"a RemediationRequest with a matching name in a different namespace must not satisfy the gate")

		events := flushAndQuery(uid, authwebhook.EventTypeASDeniedCreate)
		Expect(events).To(HaveLen(1))
	})

	It("IT-AS-2244-004: UPDATE is not intercepted (AgentSessionSpec is CEL-immutable, nothing to re-validate)", func() {
		rrName := uniqueID("it-rr-update")
		rr := buildRR(rrName)
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		as := buildAS(uniqueID("it-as-update"), rrName)
		asJSON, err := json.Marshal(as)
		Expect(err).ToNot(HaveOccurred())

		uid := uniqueID("as-update-not-gated")
		resp := asHandler.Handle(ctx, admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID: types.UID(uid),
				Kind: metav1.GroupVersionKind{
					Group: "kubernaut.ai", Version: "v1alpha1", Kind: "AgentSession",
				},
				Name:      as.Name,
				Namespace: as.Namespace,
				Operation: admissionv1.Update,
				UserInfo: authv1.UserInfo{
					Username: "it-agentsession-user@kubernaut.ai",
					UID:      "it-agentsession-uid",
				},
				Object:    runtime.RawExtension{Raw: asJSON},
				OldObject: runtime.RawExtension{Raw: asJSON},
			},
		})

		Expect(resp.Allowed).To(BeTrue(), "UPDATE should be allowed unconditionally")

		// #2244: no audit event is emitted for an operation this handler
		// does not gate -- give the (nonexistent) event a bounded window to
		// *not* appear rather than asserting on an empty slice immediately,
		// since a false negative here would silently hide an audit-flood
		// regression.
		Consistently(func() ([]ogenclient.AuditEvent, error) {
			return queryAuditEvents(dsClient, uid, nil)
		}, 3*time.Second, 500*time.Millisecond).Should(BeEmpty(),
			"UPDATE must not emit any audit event")
	})

})
