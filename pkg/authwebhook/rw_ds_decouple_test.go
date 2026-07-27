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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// #1661 Change 8c: AW must patch .status directly from its own local
// computation (content hash + deterministic UUID, already landed in Change
// 8a/8b) with ZERO calls to DS -- CreateWorkflowInline/DisableWorkflow are
// removed from the admission path entirely, closing the original
// "DS wiped mid-session -> AW/DS diverge" gap this migration started from.
//
// Business Requirements: BR-WORKFLOW-006 (etcd single source of truth).
var _ = Describe("UT-AW-360: AW removes the DS round-trip entirely for RemediationWorkflow admission (#1661 Change 8c)", func() {
	var (
		ctx           context.Context
		mockAudit     *MockAuditStoreRW
		failingMockDS *mockWorkflowCatalogClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockAudit = &MockAuditStoreRW{}
		// Any call to CreateWorkflowInline/DisableWorkflow fails the test --
		// Change 8c's whole point is that registerWorkflow/handleDelete never
		// reach this client at all anymore.
		failingMockDS = &mockWorkflowCatalogClient{
			createFn: func(_ context.Context, _, _, _ string) error {
				Fail("CreateWorkflowInline must not be called -- AW computes/patches everything locally (#1661 Change 8c)")
				return nil
			},
			disableFn: func(_ context.Context, _, _, _ string) error {
				Fail("DisableWorkflow must not be called -- AW computes/patches everything locally (#1661 Change 8c)")
				return nil
			},
		}
	})

	It("CREATE patches .status.workflowId/.status.contentHash/.status.catalogStatus locally with zero DS calls", func() {
		rw := buildRemediationWorkflow("ds-decouple-create", "kubernaut-system")
		fakeK8s := fakeK8sWithRWAndActiveActionType(rw)
		handler := authwebhook.NewRemediationWorkflowHandler(failingMockDS, mockAudit, fakeK8s)

		admReq := buildCreateAdmissionRequest(rw)
		resp := handler.Handle(ctx, admReq)
		Expect(resp.Allowed).To(BeTrue(), "CREATE should be allowed without any DS round-trip")

		expectedID := expectedLocalWorkflowID(mockAudit)
		expectedHash := lastEmittedContentHash(mockAudit)

		Eventually(func() string {
			updated := &rwv1alpha1.RemediationWorkflow{}
			if err := fakeK8s.Get(ctx, fakeK8sKey("ds-decouple-create"), updated); err != nil {
				return ""
			}
			return updated.Status.WorkflowID
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(expectedID),
			"BUSINESS VALUE: .status.workflowId must be patched from AW's own local computation, never from a DS response")

		updated := &rwv1alpha1.RemediationWorkflow{}
		Expect(fakeK8s.Get(ctx, fakeK8sKey("ds-decouple-create"), updated)).To(Succeed())
		Expect(updated.Status.ContentHash).To(Equal(expectedHash),
			"BUSINESS VALUE: .status.contentHash must be patched locally so AW never needs a DS round-trip to know its own content hash")
		Expect(updated.Status.CatalogStatus).To(Equal(sharedtypes.CatalogStatusActive),
			"a successfully-admitted workflow is always Active once there is no DS-side lifecycle to defer to")
	})

	It("UPDATE (idempotent re-apply) patches .status locally with zero DS calls", func() {
		rw := buildRemediationWorkflow("ds-decouple-update", "kubernaut-system")
		fakeK8s := fakeK8sWithRWAndActiveActionType(rw)
		handler := authwebhook.NewRemediationWorkflowHandler(failingMockDS, mockAudit, fakeK8s)

		admReq := buildUpdateAdmissionRequest(rw)
		resp := handler.Handle(ctx, admReq)
		Expect(resp.Allowed).To(BeTrue(), "UPDATE should be allowed without any DS round-trip")

		expectedID := expectedLocalWorkflowID(mockAudit)
		Eventually(func() string {
			updated := &rwv1alpha1.RemediationWorkflow{}
			if err := fakeK8s.Get(ctx, fakeK8sKey("ds-decouple-update"), updated); err != nil {
				return ""
			}
			return updated.Status.WorkflowID
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(expectedID))
	})

	It("DELETE emits the audit event and allows deletion with zero DS calls", func() {
		rw := buildRemediationWorkflowWithStatus("ds-decouple-delete", "existing-workflow-id")
		handler := authwebhook.NewRemediationWorkflowHandler(failingMockDS, mockAudit, nil)

		admReq := buildDeleteAdmissionRequest(rw)
		resp := handler.Handle(ctx, admReq)

		Expect(resp.Allowed).To(BeTrue(), "DELETE should be allowed without any DS round-trip")
		Expect(mockAudit.StoredEvents).To(HaveLen(1), "DELETE should still emit exactly one audit event")
		Expect(mockAudit.StoredEvents[0].EventType).To(Equal(authwebhook.EventTypeRWAdmittedDelete))
	})
})

// lastEmittedContentHash returns the content_hash carried by the most
// recently stored admitted audit event, for asserting .status.contentHash
// was patched with the exact same value AW computed for that event.
func lastEmittedContentHash(mockAudit *MockAuditStoreRW) string {
	ExpectWithOffset(1, mockAudit.StoredEvents).ToNot(BeEmpty())
	event := mockAudit.StoredEvents[len(mockAudit.StoredEvents)-1]
	payload, ok := event.EventData.GetRemediationWorkflowWebhookAuditPayload()
	ExpectWithOffset(1, ok).To(BeTrue())
	ExpectWithOffset(1, payload.ContentHash.IsSet()).To(BeTrue())
	return payload.ContentHash.Value
}
