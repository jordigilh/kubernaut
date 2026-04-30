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

package tools_test

import (
	"context"
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

type mockWorkflowCatalog struct {
	workflow *mcptools.CatalogWorkflow
	err      error
}

func (m *mockWorkflowCatalog) GetWorkflowByID(_ context.Context, workflowID string) (*mcptools.CatalogWorkflow, error) {
	return m.workflow, m.err
}

var _ = Describe("kubernaut_select_workflow tool — #703 BR-INTERACTIVE-005", func() {

	Describe("UT-KA-703-TOOL-005: Input validation", func() {
		It("should reject empty rr_id", func() {
			tool := mcptools.NewSelectWorkflowTool(nil, nil)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id"))
		})

		It("should reject empty workflow_id", func() {
			tool := mcptools.NewSelectWorkflowTool(nil, nil)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-001",
				WorkflowID: "",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workflow_id"))
		})
	})

	Describe("UT-KA-703-TOOL-006: Successful workflow selection", func() {
		It("should look up workflow from catalog and return selection confirmation", func() {
			wfID := uuid.New().String()
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{
					WorkflowID:      wfID,
					WorkflowName:    "increase-memory",
					ActionType:      "scale-vertical",
					Version:         "v1.2.0",
					ExecutionEngine: "argo-workflows",
					ExecutionBundle: "oci://registry/increase-memory:v1.2.0",
				},
			}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-wf-001",
					CorrelationID: "rr-wf-001",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			output, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-wf-001",
				WorkflowID: wfID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflow_selected"))
			Expect(output.Workflow).NotTo(BeNil())
			Expect(output.Workflow.WorkflowID).To(Equal(wfID))
			Expect(output.Workflow.WorkflowName).To(Equal("increase-memory"))
			Expect(output.Workflow.ActionType).To(Equal("scale-vertical"))
			Expect(output.Workflow.ExecutionEngine).To(Equal("argo-workflows"))
			Expect(output.Confidence).To(Equal(1.0))
			Expect(output.Rationale).To(Equal("User-selected via interactive mode"))
		})
	})

	Describe("UT-KA-703-TOOL-006b: Workflow not found in catalog", func() {
		It("should return error when workflow_id does not exist", func() {
			catalog := &mockWorkflowCatalog{err: errors.New("workflow not found")}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-wf-002",
					CorrelationID: "rr-wf-002",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-wf-002",
				WorkflowID: "nonexistent-wf",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workflow"))
		})
	})

	Describe("UT-KA-703-TOOL-006c: Tool rejects requests when no active session", func() {
		It("should return error when no interactive session is active for rr_id", func() {
			catalog := &mockWorkflowCatalog{}
			sessions := &mockSessionManager{isActive: false}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-no-sess",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session"))
		})
	})

	Describe("UT-KA-703-TOOL-006d: Tool enforces driver identity", func() {
		It("should reject requests from a user who is not the active driver", func() {
			catalog := &mockWorkflowCatalog{workflow: &mcptools.CatalogWorkflow{}}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-wf-003",
					CorrelationID: "rr-authz-wf",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-authz-wf",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("driver"))
		})
	})
})
