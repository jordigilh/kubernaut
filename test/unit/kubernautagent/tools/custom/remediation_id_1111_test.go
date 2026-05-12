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

package custom_test

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
)

var _ = Describe("Issue #1111: Remediation ID forwarding to DS discovery tools", func() {
	const testRemediationID = "rr-test-abc-123"

	signalCtxWithRemediationID := func(remediationID string) context.Context {
		return katypes.WithSignalContext(context.Background(), katypes.SignalContext{
			Severity:      "critical",
			ResourceKind:  "Deployment",
			Environment:   "production",
			Priority:      "P0",
			RemediationID: remediationID,
		})
	}

	Describe("UT-KA-1111-001: list_available_actions forwards signal.RemediationID", func() {
		It("should set RemediationID on ListAvailableActionsParams", func() {
			fake := &fakeWorkflowDS{
				listActionsResponse: &ogenclient.ActionTypeListResponse{},
			}
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID(testRemediationID)
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			val, ok := fake.listActionsParams.RemediationID.Get()
			Expect(ok).To(BeTrue(), "RemediationID should be set on params")
			Expect(val).To(Equal(testRemediationID))
		})
	})

	Describe("UT-KA-1111-002: list_workflows forwards signal.RemediationID", func() {
		It("should set RemediationID on ListWorkflowsByActionTypeParams", func() {
			fake := &fakeWorkflowDS{
				listWorkflowsResponse: &ogenclient.WorkflowDiscoveryResponse{},
			}
			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			ctx := signalCtxWithRemediationID(testRemediationID)
			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"RestartPod"}`))
			Expect(err).ToNot(HaveOccurred())

			val, ok := fake.listWorkflowsParams.RemediationID.Get()
			Expect(ok).To(BeTrue(), "RemediationID should be set on params")
			Expect(val).To(Equal(testRemediationID))
		})
	})

	Describe("UT-KA-1111-003: get_workflow loads signal context and forwards RemediationID + context filters", func() {
		It("should set RemediationID and context filters on GetWorkflowByIDParams", func() {
			fake := &fakeWorkflowDS{}
			allTools := custom.NewAllTools(fake)
			getWorkflow := allTools[2]

			ctx := signalCtxWithRemediationID(testRemediationID)
			validUUID := uuid.New().String()
			args := json.RawMessage(`{"workflow_id":"` + validUUID + `"}`)

			_, err := getWorkflow.Execute(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			val, ok := fake.getWorkflowParams.RemediationID.Get()
			Expect(ok).To(BeTrue(), "RemediationID should be set on GetWorkflowByIDParams")
			Expect(val).To(Equal(testRemediationID))

			sevVal, sevOk := fake.getWorkflowParams.Severity.Get()
			Expect(sevOk).To(BeTrue(), "Severity should be set for HasContextFilters()")
			Expect(string(sevVal)).To(Equal("critical"))

			compVal, compOk := fake.getWorkflowParams.Component.Get()
			Expect(compOk).To(BeTrue(), "Component should be set for HasContextFilters()")
			Expect(compVal).To(Equal("deployment"))

			envVal, envOk := fake.getWorkflowParams.Environment.Get()
			Expect(envOk).To(BeTrue(), "Environment should be set for HasContextFilters()")
			Expect(envVal).To(Equal("production"))

			priVal, priOk := fake.getWorkflowParams.Priority.Get()
			Expect(priOk).To(BeTrue(), "Priority should be set for HasContextFilters()")
			Expect(string(priVal)).To(Equal("P0"))
		})
	})

	Describe("UT-KA-1111-006: Empty RemediationID does NOT send param", func() {
		It("should not set RemediationID when signal.RemediationID is empty", func() {
			fake := &fakeWorkflowDS{
				listActionsResponse: &ogenclient.ActionTypeListResponse{},
			}
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID("")
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			_, ok := fake.listActionsParams.RemediationID.Get()
			Expect(ok).To(BeFalse(), "RemediationID should NOT be set when signal.RemediationID is empty")
		})
	})

	Describe("UT-KA-1111-007: Max-length RemediationID forwarded without truncation", func() {
		It("should forward a 256-char RemediationID as-is", func() {
			longID := "rr-" + strings.Repeat("a", 253)
			Expect(len(longID)).To(Equal(256))

			fake := &fakeWorkflowDS{
				listActionsResponse: &ogenclient.ActionTypeListResponse{},
			}
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID(longID)
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			val, ok := fake.listActionsParams.RemediationID.Get()
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal(longID))
		})
	})

	Describe("UT-KA-1111-008: Path traversal RemediationID forwarded as-is", func() {
		It("should forward path traversal chars without sanitization", func() {
			pathTraversal := "../../etc/passwd"
			fake := &fakeWorkflowDS{
				listActionsResponse: &ogenclient.ActionTypeListResponse{},
			}
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID(pathTraversal)
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			val, ok := fake.listActionsParams.RemediationID.Get()
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal(pathTraversal))
		})
	})

	Describe("UT-KA-1111-009: Unicode RemediationID forwarded correctly", func() {
		It("should forward Unicode characters without corruption", func() {
			unicodeID := "rr-テスト-123"
			fake := &fakeWorkflowDS{
				listActionsResponse: &ogenclient.ActionTypeListResponse{},
			}
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID(unicodeID)
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			val, ok := fake.listActionsParams.RemediationID.Get()
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal(unicodeID))
		})
	})

	Describe("UT-KA-1111-010: get_workflow without signal context succeeds (best-effort)", func() {
		It("should succeed without forwarding RemediationID when signal is missing", func() {
			fake := &fakeWorkflowDS{}
			allTools := custom.NewAllTools(fake)
			getWorkflow := allTools[2]

			validUUID := uuid.New().String()
			args := json.RawMessage(`{"workflow_id":"` + validUUID + `"}`)

			_, err := getWorkflow.Execute(context.Background(), args)
			Expect(err).ToNot(HaveOccurred())

			_, ok := fake.getWorkflowParams.RemediationID.Get()
			Expect(ok).To(BeFalse(), "RemediationID should NOT be set when signal context is missing")
		})
	})

	Describe("UT-KA-1111-011: get_workflow with empty RemediationID does not populate params", func() {
		It("should not set context filters when RemediationID is empty", func() {
			fake := &fakeWorkflowDS{}
			allTools := custom.NewAllTools(fake)
			getWorkflow := allTools[2]

			ctx := signalCtxWithRemediationID("")
			validUUID := uuid.New().String()
			args := json.RawMessage(`{"workflow_id":"` + validUUID + `"}`)

			_, err := getWorkflow.Execute(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			_, ok := fake.getWorkflowParams.RemediationID.Get()
			Expect(ok).To(BeFalse(), "RemediationID should NOT be set when signal.RemediationID is empty")

			_, sevOk := fake.getWorkflowParams.Severity.Get()
			Expect(sevOk).To(BeFalse(), "Severity should NOT be set when RemediationID is empty")
		})
	})
})
