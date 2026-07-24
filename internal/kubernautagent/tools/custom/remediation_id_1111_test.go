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
)

var _ = Describe("Issue #1111: Remediation ID forwarding to discovery filters", func() {
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
		It("should set RemediationID on the discovery filters", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID(testRemediationID)
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.listActionsFilters.RemediationID).To(Equal(testRemediationID))
		})
	})

	Describe("UT-KA-1111-002: list_workflows forwards signal.RemediationID", func() {
		It("should set RemediationID on the discovery filters", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			ctx := signalCtxWithRemediationID(testRemediationID)
			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"RestartPod"}`))
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.listWorkflowsFilters.RemediationID).To(Equal(testRemediationID))
		})
	})

	Describe("UT-KA-1111-003: get_workflow loads signal context and forwards RemediationID + context filters", func() {
		It("should set RemediationID and context filters on the discovery filters", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			getWorkflow := allTools[2]

			ctx := signalCtxWithRemediationID(testRemediationID)
			validUUID := uuid.New().String()
			args := json.RawMessage(`{"workflow_id":"` + validUUID + `"}`)

			_, err := getWorkflow.Execute(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.getWorkflowFilters).NotTo(BeNil(), "filters should be set on GetWorkflowWithContextFilters")
			Expect(fake.getWorkflowFilters.RemediationID).To(Equal(testRemediationID))
			Expect(fake.getWorkflowFilters.Severity).To(Equal("critical"), "Severity should be set for HasContextFilters()")
			Expect(fake.getWorkflowFilters.Component).To(Equal("deployment"), "Component should be set for HasContextFilters()")
			Expect(fake.getWorkflowFilters.Environment).To(Equal("production"), "Environment should be set for HasContextFilters()")
			Expect(fake.getWorkflowFilters.Priority).To(Equal("P0"), "Priority should be set for HasContextFilters()")
		})
	})

	Describe("UT-KA-1111-004: Empty RemediationID does NOT send filter", func() {
		It("should not set RemediationID when signal.RemediationID is empty", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID("")
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.listActionsFilters.RemediationID).To(BeEmpty(), "RemediationID should NOT be set when signal.RemediationID is empty")
		})
	})

	Describe("UT-KA-1111-005: Max-length RemediationID forwarded without truncation", func() {
		It("should forward a 256-char RemediationID as-is", func() {
			longID := "rr-" + strings.Repeat("a", 253)
			Expect(len(longID)).To(Equal(256))

			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID(longID)
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.listActionsFilters.RemediationID).To(Equal(longID))
		})
	})

	Describe("UT-KA-1111-006: Path traversal RemediationID forwarded as-is", func() {
		It("should forward path traversal chars without sanitization", func() {
			pathTraversal := "../../etc/passwd"
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID(pathTraversal)
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.listActionsFilters.RemediationID).To(Equal(pathTraversal))
		})
	})

	Describe("UT-KA-1111-007: Unicode RemediationID forwarded correctly", func() {
		It("should forward Unicode characters without corruption", func() {
			unicodeID := "rr-テスト-123"
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			listActions := allTools[0]

			ctx := signalCtxWithRemediationID(unicodeID)
			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.listActionsFilters.RemediationID).To(Equal(unicodeID))
		})
	})

	Describe("UT-KA-1111-008: get_workflow without signal context succeeds (best-effort)", func() {
		It("should succeed without forwarding RemediationID when signal is missing", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			getWorkflow := allTools[2]

			validUUID := uuid.New().String()
			args := json.RawMessage(`{"workflow_id":"` + validUUID + `"}`)

			_, err := getWorkflow.Execute(context.Background(), args)
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.getWorkflowFilters).To(BeNil(), "filters should NOT be set when signal context is missing")
		})
	})

	Describe("UT-KA-1111-009: get_workflow with empty RemediationID does not populate filters", func() {
		It("should not set context filters when RemediationID is empty", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			getWorkflow := allTools[2]

			ctx := signalCtxWithRemediationID("")
			validUUID := uuid.New().String()
			args := json.RawMessage(`{"workflow_id":"` + validUUID + `"}`)

			_, err := getWorkflow.Execute(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.getWorkflowFilters).To(BeNil(), "filters should NOT be set when signal.RemediationID is empty")
		})
	})
})
