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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

var _ = Describe("CP-4 Tool Completeness Gate — Cross-Tool Consistency", func() {

	Describe("TOOL-008: All tools reject requests without active session consistently", func() {
		It("investigate and select_workflow should all error on inactive session", func() {
			inactiveSessions := &mockSessionManager{isActive: false}

			// Investigate (message action requires active session)
			invTool := mcptools.NewInvestigateTool(inactiveSessions, nil, nil, mcptools.NopAutonomousManager{})
			_, invErr := invTool.Handle(context.Background(), mcptools.InvestigateInput{
				Action:  "message",
				RRID:    "rr-consist-001",
				Message: "test",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(invErr).To(HaveOccurred(), "investigate should reject inactive session")

			// SelectWorkflow (enrichment internalized per #1012)
			swTool := mcptools.NewSelectWorkflowTool(
				&mockWorkflowCatalog{workflow: &mcptools.CatalogWorkflow{}},
				inactiveSessions,
			)
			_, swErr := swTool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-consist-001",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(swErr).To(HaveOccurred(), "select_workflow should reject inactive session")
			Expect(swErr.Error()).To(ContainSubstring("session"))
		})
	})

	Describe("TOOL-009: All tools enforce driver identity consistently", func() {
		It("should reject non-driver user across all tools", func() {
			activeSessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-consist",
					CorrelationID: "rr-consist-002",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}
			intruder := mcpinternal.UserInfo{Username: "mallory"}

			// SelectWorkflow (enrichment internalized per #1012)
			swTool := mcptools.NewSelectWorkflowTool(
				&mockWorkflowCatalog{workflow: &mcptools.CatalogWorkflow{}},
				activeSessions,
			)
			_, swErr := swTool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-consist-002",
				WorkflowID: "wf-001",
			}, intruder)
			Expect(swErr).To(HaveOccurred())
			Expect(swErr.Error()).To(ContainSubstring("driver"))
		})
	})

	Describe("TOOL-010: All tools validate rr_id as mandatory consistently", func() {
		It("should reject empty rr_id across all tools", func() {
			// Investigate
			invTool := mcptools.NewInvestigateTool(nil, nil, nil, mcptools.NopAutonomousManager{})
			_, invErr := invTool.Handle(context.Background(), mcptools.InvestigateInput{
				Action: "start",
				RRID:   "",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(invErr).To(HaveOccurred())

			// SelectWorkflow
			swTool := mcptools.NewSelectWorkflowTool(nil, nil)
			_, swErr := swTool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(swErr).To(HaveOccurred())
			Expect(swErr.Error()).To(ContainSubstring("rr_id"))
		})
	})

	Describe("TOOL-011: All tools produce error messages with tool context", func() {
		It("should include semantic context (not just generic errors)", func() {
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:  "sess-011",
					ActingUser: mcpinternal.UserInfo{Username: "alice"},
				},
			}

			// SelectWorkflow with internalized enrichment failure (#1012)
			swToolEnrich := mcptools.NewSelectWorkflowTool(
				nil,
				sessions,
				mcptools.WithEnrichmentRunner(&mockEnrichmentRunner{err: context.DeadlineExceeded}),
			)
			_, enrichErr := swToolEnrich.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-011",
				WorkflowID: "wf-001",
				Kind:       "Pod",
				Name:       "test",
				Namespace:  "default",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(enrichErr.Error()).To(ContainSubstring("enrich"),
				"enrichment error should contain tool-specific context")

			// SelectWorkflow with failed catalog
			swTool := mcptools.NewSelectWorkflowTool(
				&mockWorkflowCatalog{err: context.DeadlineExceeded},
				sessions,
			)
			_, swErr := swTool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-011",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(swErr.Error()).To(ContainSubstring("workflow"),
				"select_workflow error should contain tool-specific context")
		})
	})

	Describe("TOOL-012: SelectWorkflow sets confidence=1.0 and rationale for human selection", func() {
		It("should always set confidence=1.0 and standard rationale for user-selected workflows", func() {
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:  "sess-012",
					ActingUser: mcpinternal.UserInfo{Username: "alice"},
				},
			}
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{
					WorkflowID:   "wf-012",
					WorkflowName: "restart-pod",
					ActionType:   "restart",
					Version:      "v1.0.0",
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			output, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-012",
				WorkflowID: "wf-012",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Confidence).To(Equal(1.0),
				"human-selected workflows must have confidence=1.0")
			Expect(output.Rationale).To(Equal("User-selected via interactive mode"),
				"human-selected workflows must have standard rationale")
		})
	})
})
