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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
)

// BR-WORKFLOW-016 / #779: Tool Execute methods must not panic when the
// context is cancelled. They may return an error, but must fail gracefully.

var _ = Describe("UT-KA-779-CC-T: Tool Execute context cancellation", func() {

	var cancelledCtx context.Context

	BeforeEach(func() {
		ctx := katypes.WithSignalContext(context.Background(), katypes.SignalContext{
			Severity:     "critical",
			ResourceKind: "Deployment",
			Environment:  "production",
			Priority:     "P0",
		})
		cancelledCtx = ctx
	})

	Describe("UT-KA-779-CC-T-001: list_available_actions does not panic with cancelled context", func() {
		It("should not panic when context is cancelled", func() {
			fake := &fakeWorkflowDS{}
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			ctx, cancel := context.WithCancel(cancelledCtx)
			cancel()

			Expect(func() {
				_, _ = listActions.Execute(ctx, json.RawMessage(`{}`))
			}).NotTo(Panic(), "list_available_actions must not panic with cancelled context")
		})
	})

	Describe("UT-KA-779-CC-T-002: list_workflows does not panic with cancelled context", func() {
		It("should not panic when context is cancelled", func() {
			fake := &fakeWorkflowDS{}
			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			ctx, cancel := context.WithCancel(cancelledCtx)
			cancel()

			Expect(func() {
				_, _ = listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"RestartPod"}`))
			}).NotTo(Panic(), "list_workflows must not panic with cancelled context")
		})
	})

	Describe("UT-KA-779-CC-T-003: get_workflow does not panic with cancelled context", func() {
		It("should not panic when context is cancelled", func() {
			fake := &fakeWorkflowDS{}
			allTools := custom.NewAllTools(fake)
			getWorkflow := allTools[2]

			ctx, cancel := context.WithCancel(cancelledCtx)
			cancel()

			Expect(func() {
				_, _ = getWorkflow.Execute(ctx, json.RawMessage(`{"workflow_id":"550e8400-e29b-41d4-a716-446655440000"}`))
			}).NotTo(Panic(), "get_workflow must not panic with cancelled context")
		})
	})
})
