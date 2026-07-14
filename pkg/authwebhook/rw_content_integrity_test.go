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

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// buildUpdateAdmissionRequestDiff builds an UPDATE admission request where
// Object and OldObject genuinely differ, unlike buildUpdateAdmissionRequest
// (which sets both to the same marshaled rw and therefore can never exercise
// a real content diff). #1661 Change 8b.
func buildUpdateAdmissionRequestDiff(oldRW, newRW *rwv1alpha1.RemediationWorkflow) admission.Request {
	oldJSON, _ := json.Marshal(oldRW)
	newJSON, _ := json.Marshal(newRW)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "admission-update-diff-001",
			Kind: metav1.GroupVersionKind{
				Group:   "kubernaut.ai",
				Version: "v1alpha1",
				Kind:    "RemediationWorkflow",
			},
			Name:      newRW.Name,
			Namespace: newRW.Namespace,
			Operation: admissionv1.Update,
			UserInfo: authv1.UserInfo{
				Username: testUserEmail,
				UID:      testUserUID,
			},
			Object:    runtime.RawExtension{Raw: newJSON},
			OldObject: runtime.RawExtension{Raw: oldJSON},
		},
	}
}

// #1661 Change 8b: today's "same version + different content -> 409" check
// (DD-WORKFLOW-017 Acceptance Criteria #4) is a DS-side Postgres unique-
// constraint conflict (pkg/datastorage/server/workflow_create_handlers.go's
// contentIntegrityError). AW must replicate this locally -- using only the
// UPDATE admission request's own Object/OldObject pair, no DS round-trip and
// no etcd List() -- since RemediationWorkflowSpec has no independent identity
// field (DS's own schema parser sets spec.WorkflowName = crd.Metadata.Name),
// so "same name+version, different content" can only ever happen on the one
// live object being updated, never across two distinct CRDs.
//
// Business Requirements: BR-WORKFLOW-006.
var _ = Describe("RemediationWorkflow Content-Integrity Check on UPDATE (#1661 Change 8b)", func() {
	var (
		ctx       context.Context
		mockDS    *mockWorkflowCatalogClient
		mockAudit *MockAuditStoreRW
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockDS = &mockWorkflowCatalogClient{}
		mockAudit = &MockAuditStoreRW{}
	})

	// ========================================
	// UT-AW-321-001: same version + different content -> Denied, zero DS calls
	// ========================================
	Describe("UT-AW-321-001: same spec.version with a different content hash is rejected", func() {
		It("should return Denied and never call DS", func() {
			oldRW := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			newRW := buildRemediationWorkflow("scale-memory", "kubernaut-system")
			// Same version, but a different Description.What -> different content hash.
			newRW.Spec.Description.What = "Changed behavior without bumping spec.version"

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				dsCalled = true
				return nil, nil
			}

			handler := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, nil)
			admReq := buildUpdateAdmissionRequestDiff(oldRW, newRW)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeFalse(),
				"UPDATE should be Denied when spec.version is unchanged but content differs")
			// #1661 Change 8b REFACTOR: wording matches DS's now-superseded
			// contentIntegrityError.Error() verbatim so operators see no
			// behavioral difference regardless of which gate denies.
			Expect(resp.Result.Message).To(ContainSubstring(`active workflow "scale-memory" version "1.0.0" already has different content`),
				"Denial message should match DS's historical content-integrity-violation wording")
			Expect(resp.Result.Message).To(ContainSubstring("bump the version to register new content"))
			Expect(dsCalled).To(BeFalse(),
				"DS must never be called once AW's own content-integrity check denies admission")

			Expect(mockAudit.StoredEvents).To(HaveLen(1), "Denied audit event should be emitted")
			Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeRWAdmittedDenied))
		})
	})

	// ========================================
	// UT-AW-321-002: same version + same content -> idempotent, reaches DS
	// ========================================
	Describe("UT-AW-321-002: same spec.version with the same content hash is an idempotent re-apply", func() {
		It("should return Allowed and still call DS (Change 8c has not landed yet)", func() {
			rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")

			dsCalled := false
			mockDS.createFn = func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				dsCalled = true
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:   "550e8400-e29b-41d4-a716-446655440000",
					WorkflowName: "scale-memory",
					Version:      "1.0.0",
					Status:       "Active",
				}, nil
			}

			handler := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, nil)
			// buildUpdateAdmissionRequestDiff with oldRW == newRW content: identical
			// spec, so identical content hash -- a genuine no-op re-apply.
			admReq := buildUpdateAdmissionRequestDiff(rw, rw)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(),
				"UPDATE should be Allowed when content is unchanged (idempotent re-apply)")
			Expect(dsCalled).To(BeTrue(),
				"DS should still be called for the idempotent case in this phase (Change 8c removes this call later)")
		})
	})
})
