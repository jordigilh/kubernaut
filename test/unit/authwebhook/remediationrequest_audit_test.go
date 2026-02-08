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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SOC2 Round 3 M-2: RemediationRequest webhook correlation ID test
// DD-AUDIT-CORRELATION-001: correlation_id must use RR.Name (not UID)

var _ = Describe("RemediationRequest Webhook Correlation ID (DD-AUDIT-CORRELATION-001)", func() {
	var (
		ctx       context.Context
		handler   *authwebhook.RemediationRequestStatusHandler
		mockStore *MockAuditStore
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}
		handler = authwebhook.NewRemediationRequestStatusHandler(mockStore)

		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		decoder := admission.NewDecoder(scheme)
		_ = handler.InjectDecoder(decoder)
	})

	It("should use RR Name (not UID) as correlation_id", func() {
		fiveMin := metav1.Duration{Duration: 5 * time.Minute}
		tenMin := metav1.Duration{Duration: 10 * time.Minute}

		// OLD object: RR with original TimeoutConfig
		oldRR := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-cpu-spike-prod-001",
				Namespace: "production",
				UID:       "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			},
			Status: remediationv1.RemediationRequestStatus{
				TimeoutConfig: &remediationv1.TimeoutConfig{
					Global: &fiveMin,
				},
			},
		}
		oldRRJSON, _ := json.Marshal(oldRR)

		// NEW object: RR with changed TimeoutConfig (triggers webhook)
		newRR := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-cpu-spike-prod-001",
				Namespace: "production",
				UID:       "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			},
			Status: remediationv1.RemediationRequestStatus{
				TimeoutConfig: &remediationv1.TimeoutConfig{
					Global: &tenMin, // Changed from 5m to 10m
				},
			},
		}
		newRRJSON, _ := json.Marshal(newRR)

		admReq := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID: "admission-req-rr-001",
				Kind: metav1.GroupVersionKind{
					Group:   "remediation.kubernaut.ai",
					Version: "v1alpha1",
					Kind:    "RemediationRequest",
				},
				Name:      newRR.Name,
				Namespace: newRR.Namespace,
				Operation: admissionv1.Update,
				UserInfo: authv1.UserInfo{
					Username: "operator@kubernaut.ai",
					UID:      "k8s-user-123",
				},
				Object: runtime.RawExtension{
					Raw: newRRJSON,
				},
				OldObject: runtime.RawExtension{
					Raw: oldRRJSON,
				},
			},
		}

		resp := handler.Handle(ctx, admReq)

		Expect(resp.Allowed).To(BeTrue(), "TimeoutConfig change should be allowed")
		Expect(mockStore.StoredEvents).To(HaveLen(1), "Audit event should be emitted")

		event := mockStore.StoredEvents[0]

		// DD-AUDIT-CORRELATION-001: correlation_id MUST be RR.Name (human-readable)
		// NOT RR.UID (UUID), to match ADR-034 query pattern
		Expect(event.CorrelationID).To(Equal("rr-cpu-spike-prod-001"),
			"DD-AUDIT-CORRELATION-001: correlation_id must be RR.Name, not UID")
		Expect(event.CorrelationID).ToNot(Equal("a1b2c3d4-e5f6-7890-abcd-ef1234567890"),
			"correlation_id must NOT be the Kubernetes UID")
	})
})
