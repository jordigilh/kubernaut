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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// #1661 Change 2: AuthWebhook threads the already-unmarshaled rw.Spec (plus a
// locally-computed content hash) into remediationworkflow.admitted.* audit
// events, so the audit trail can reconstruct the exact workflow definition
// without depending on etcd or DataStorage's cache still holding that version.
// Denied events capture the same content best-effort whenever rw.Spec was
// successfully unmarshaled (BR-AUDIT-005 v2.0 item #7, SOC2 CC7.2) -- omitted
// only when unmarshal itself failed (nothing to capture).
//
// Business Requirements: BR-AUDIT-005, BR-WORKFLOW-006.
var _ = Describe("RemediationWorkflow Audit Content Enrichment (#1661)", func() {
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
		// k8sClient is nil: validateActionTypeExists best-effort-skips (matches
		// the existing UT-AW-299-* precedent in remediationworkflow_handler_test.go),
		// keeping these tests focused purely on audit content threading.
		handler = authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, nil)
	})

	// ========================================
	// UT-AW-311-001: CREATE admitted event carries workflow_content + content_hash
	// ========================================
	Describe("UT-AW-311-001: CREATE admitted audit event carries workflow_content and content_hash", func() {
		It("should populate workflow_content from rw.Spec and a 64-char hex content_hash", func() {
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			admReq := buildCreateAdmissionRequest(rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue())
			Expect(mockAudit.StoredEvents).To(HaveLen(1))

			event := mockAudit.StoredEvents[0]
			payload, ok := event.EventData.GetRemediationWorkflowWebhookAuditPayload()
			Expect(ok).To(BeTrue())

			Expect(payload.WorkflowContent.IsSet()).To(BeTrue(),
				"BUSINESS VALUE: audit trail must capture the full workflow spec for reconstruction (#1661)")
			content := payload.WorkflowContent.Value
			Expect(content.Version).To(Equal("1.0.0"))
			Expect(content.ActionType).To(Equal("ScaleMemory"))
			Expect(content.Description.What).To(Equal("Test workflow"))
			Expect(content.Labels.Severity).To(ConsistOf("critical"))
			Expect(content.Execution.Engine.Value).To(Equal("job"))
			Expect(content.Parameters).To(HaveLen(1))
			Expect(content.Parameters[0].Name).To(Equal("TARGET_RESOURCE"))

			Expect(payload.ContentHash.IsSet()).To(BeTrue())
			Expect(payload.ContentHash.Value).To(MatchRegexp("^[0-9a-f]{64}$"),
				"content_hash must be a SHA-256 hex digest (64 lowercase hex chars)")
		})
	})

	// ========================================
	// UT-AW-311-002: UPDATE admitted event carries workflow_content + content_hash
	// ========================================
	Describe("UT-AW-311-002: UPDATE admitted audit event carries workflow_content and content_hash", func() {
		It("should populate workflow_content from the updated rw.Spec", func() {
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			rw.Spec.Version = "1.1.0"
			admReq := buildUpdateAdmissionRequest(rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue())
			Expect(mockAudit.StoredEvents).To(HaveLen(1))

			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("remediationworkflow.admitted.update"))
			payload, ok := event.EventData.GetRemediationWorkflowWebhookAuditPayload()
			Expect(ok).To(BeTrue())

			Expect(payload.WorkflowContent.IsSet()).To(BeTrue())
			Expect(payload.WorkflowContent.Value.Version).To(Equal("1.1.0"))
			Expect(payload.ContentHash.IsSet()).To(BeTrue())
			Expect(payload.ContentHash.Value).To(MatchRegexp("^[0-9a-f]{64}$"))
		})
	})

	// ========================================
	// UT-AW-311-003: DENIED event carries workflow_content when rw was unmarshaled
	// ========================================
	Describe("UT-AW-311-003: DENIED audit event carries workflow_content when rw.Spec was unmarshaled", func() {
		It("should attach the attempted workflow_content best-effort on a content-integrity-violation denial", func() {
			// #1661 Change 8c removed the DS-registration-failure denial path
			// entirely (there is no more DS call to fail). The content-
			// integrity check (Change 8b) is now the representative
			// still-existing denial path that has rw.Spec available.
			oldRW := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			newRW := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			newRW.Spec.Description.What = "Changed behavior without bumping spec.version"
			admReq := buildUpdateAdmissionRequestDiff(oldRW, newRW)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeFalse())
			Expect(mockAudit.StoredEvents).To(HaveLen(1))

			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("remediationworkflow.admitted.denied"))
			payload, ok := event.EventData.GetRemediationWorkflowWebhookAuditPayload()
			Expect(ok).To(BeTrue())

			Expect(payload.WorkflowContent.IsSet()).To(BeTrue(),
				"BUSINESS VALUE: forensic visibility into what was rejected and why (BR-AUDIT-005 v2.0 #7, SOC2 CC7.2)")
			Expect(payload.WorkflowContent.Value.ActionType).To(Equal("ScaleMemory"))
			Expect(payload.ContentHash.IsSet()).To(BeTrue())
			Expect(payload.ContentHash.Value).To(MatchRegexp("^[0-9a-f]{64}$"))
		})
	})

	// ========================================
	// UT-AW-311-004: DENIED event omits workflow_content when unmarshal itself failed
	// ========================================
	Describe("UT-AW-311-004: DENIED audit event omits workflow_content when unmarshal failed", func() {
		It("should not attach workflow_content or content_hash when there is nothing to capture", func() {
			admReq := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: "admission-malformed-001",
					Kind: metav1.GroupVersionKind{
						Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationWorkflow",
					},
					Name:      "malformed",
					Namespace: "kubernaut-system",
					Operation: admissionv1.Create,
					UserInfo:  authv1.UserInfo{Username: testUserEmail, UID: testUserUID},
					// spec.version as a number where a string is expected: fails
					// json.Unmarshal into RemediationWorkflowSpec before rw.Spec
					// is ever populated.
					Object: runtime.RawExtension{Raw: []byte(`{"spec": {"version": 12345}}`)},
				},
			}

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeFalse())
			Expect(mockAudit.StoredEvents).To(HaveLen(1))

			event := mockAudit.StoredEvents[0]
			Expect(event.EventType).To(Equal("remediationworkflow.admitted.denied"))
			payload, ok := event.EventData.GetRemediationWorkflowWebhookAuditPayload()
			Expect(ok).To(BeTrue())

			Expect(payload.WorkflowContent.IsSet()).To(BeFalse(),
				"unmarshal failure means rw.Spec was never populated -- nothing to capture")
			Expect(payload.ContentHash.IsSet()).To(BeFalse())
		})
	})
})
