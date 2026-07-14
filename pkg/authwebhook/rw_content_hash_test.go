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
	sharedcontenthash "github.com/jordigilh/kubernaut/pkg/shared/contenthash"
)

// #1661 Change 8a: AW must compute the workflow_id it writes to
// .status.workflowId (and attaches to admitted audit events) locally, from
// the same deterministic DeterministicUUID(ComputeContentHash(content))
// algorithm DS has always used -- not by trusting whatever WorkflowID a DS
// response happens to return. This is a pure relocation of an existing
// deterministic computation (DD-WORKFLOW-018), so every pre-existing
// workflow_id must remain stable: AW-computed-ID(content) ==
// DS-computed-ID(content) byte-for-byte.
//
// Business Requirements: BR-WORKFLOW-006 (etcd single source of truth).
var _ = Describe("UT-AW-320-001: AW-computed content hash/workflow ID match DS's historical algorithm (#1661 Change 8a)", func() {
	It("produces a workflow ID identical to DS's pre-#1661 algorithm for fixed sample content (continuity/pinned golden values)", func() {
		content := `{"apiVersion":"kubernaut.ai/v1alpha1","kind":"RemediationWorkflow","metadata":{"name":"scale-memory"},"spec":{"version":"1.0.0"}}`

		// Golden values computed once, before this migration, from DS's
		// original pkg/datastorage/uuid.DeterministicUUID +
		// workflow_discovery_handlers.computeContentHash algorithm (now
		// consolidated into this shared package, REFACTOR phase 18 --
		// pkg/datastorage/uuid no longer exists as an independent package).
		// Pinning these guards against the namespace UUID or hash algorithm
		// ever silently changing, which would churn every already-registered
		// workflow_id in production (DD-WORKFLOW-018).
		const expectedHash = "b401d467c33f43a86aa5743bcddcb853b392b7b6cbf74ff1eba6fb55c58f7bd7"
		const expectedID = "dfa1b0a3-7939-50b3-aa47-8bbc12c3f4fa"

		hash := sharedcontenthash.ComputeContentHash(content)
		Expect(hash).To(Equal(expectedHash), "content hash must match DS's historical algorithm byte-for-byte")

		actualID := sharedcontenthash.DeterministicUUID(hash)
		Expect(actualID).To(Equal(expectedID),
			"BUSINESS VALUE: workflow_id must remain stable across the etcd-single-source-of-truth migration (DD-WORKFLOW-018)")
	})

	It("registerWorkflow's admitted audit event carries the locally-computed workflow ID, not DS's returned WorkflowID", func() {
		ctx := context.Background()
		mockDS := &mockWorkflowCatalogClient{
			createFn: func(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
				// Deliberately wrong/stale ID: proves registerWorkflow no
				// longer trusts DS's response for workflow_id once AW
				// computes it locally (#1661 Change 8a).
				return &authwebhook.WorkflowRegistrationResult{
					WorkflowID:   "00000000-0000-0000-0000-000000000000",
					WorkflowName: "scale-memory",
					Version:      "1.0.0",
				}, nil
			},
		}
		mockAudit := &MockAuditStoreRW{}
		handler := authwebhook.NewRemediationWorkflowHandler(mockDS, mockAudit, nil)

		rw := buildRemediationWorkflow("scale-memory", "kubernaut-system")
		admReq := buildCreateAdmissionRequest(rw)

		resp := handler.Handle(ctx, admReq)
		Expect(resp.Allowed).To(BeTrue())
		Expect(mockAudit.StoredEvents).To(HaveLen(1))

		event := mockAudit.StoredEvents[0]
		payload, ok := event.EventData.GetRemediationWorkflowWebhookAuditPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.ContentHash.IsSet()).To(BeTrue())

		expectedID := sharedcontenthash.DeterministicUUID(payload.ContentHash.Value)
		Expect(payload.WorkflowID.IsSet()).To(BeTrue())
		Expect(payload.WorkflowID.Value).To(Equal(expectedID),
			"BUSINESS VALUE: AW's own audit trail must record the locally-computed workflow_id (#1661 Change 8a), not DS's response")
		Expect(payload.WorkflowID.Value).NotTo(Equal("00000000-0000-0000-0000-000000000000"))
	})
})
