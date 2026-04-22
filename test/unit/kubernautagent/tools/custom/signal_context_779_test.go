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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
)

// BR-WORKFLOW-016: Workflow discovery tools must forward the active signal's
// context (severity, component, environment, priority) to DataStorage so the
// catalog is filtered to the incident's actual characteristics, not hardcoded
// defaults. Issue #779 exposed that hardcoded "critical/deployment/production/P0"
// was used regardless of the real signal.

var _ = Describe("UT-KA-779: Signal context forwarding to DS tool params", func() {

	var (
		fake *fakeWorkflowDS
		ctx  = katypes.WithSignalContext(
			contextBackground(),
			katypes.SignalContext{
				Severity:     "high",
				ResourceKind: "StatefulSet",
				Environment:  "staging",
				Priority:     "P1",
			},
		)
	)

	BeforeEach(func() {
		fake = &fakeWorkflowDS{
			listActionsResponse: &ogenclient.ActionTypeListResponse{
				ActionTypes: []ogenclient.ActionTypeEntry{
					{
						ActionType:    "ScaleReplicas",
						Description:   ogenclient.StructuredDescription{What: "test", WhenToUse: "test"},
						WorkflowCount: 1,
					},
				},
				Pagination: ogenclient.PaginationMetadata{
					TotalCount: 1, Offset: 0, Limit: 10, HasMore: false,
				},
			},
			listWorkflowsResponse: &ogenclient.WorkflowDiscoveryResponse{
				ActionType: "ScaleReplicas",
				Workflows: []ogenclient.WorkflowDiscoveryEntry{
					{
						WorkflowId:   uuid.New(),
						WorkflowName: "scale-conservative-v1",
						Name:         "Scale Conservative",
						Description:  ogenclient.StructuredDescription{What: "test", WhenToUse: "test"},
					},
				},
				Pagination: ogenclient.PaginationMetadata{
					TotalCount: 1, Offset: 0, Limit: 10, HasMore: false,
				},
			},
		}
	})

	Describe("UT-KA-779-001: list_available_actions forwards signal context to DS params", func() {
		It("should set Severity, Component, Environment, Priority from SignalContext", func() {
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(string(fake.listActionsParams.Severity)).To(Equal("high"),
				"Severity should come from SignalContext, not hardcoded 'critical'")
			Expect(fake.listActionsParams.Component).To(Equal("statefulset"),
				"Component should be SignalContext.ResourceKind lowercased, not hardcoded 'deployment'")
			Expect(fake.listActionsParams.Environment).To(Equal("staging"),
				"Environment should come from SignalContext, not hardcoded 'production'")
			Expect(string(fake.listActionsParams.Priority)).To(Equal("P1"),
				"Priority should come from SignalContext, not hardcoded 'P0'")
		})
	})

	Describe("UT-KA-779-002: list_workflows forwards signal context to DS params", func() {
		It("should set Severity, Component, Environment, Priority from SignalContext", func() {
			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx,
				json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(string(fake.listWorkflowsParams.Severity)).To(Equal("high"),
				"Severity should come from SignalContext, not hardcoded 'critical'")
			Expect(fake.listWorkflowsParams.Component).To(Equal("statefulset"),
				"Component should be SignalContext.ResourceKind lowercased, not hardcoded 'deployment'")
			Expect(fake.listWorkflowsParams.Environment).To(Equal("staging"),
				"Environment should come from SignalContext, not hardcoded 'production'")
			Expect(string(fake.listWorkflowsParams.Priority)).To(Equal("P1"),
				"Priority should come from SignalContext, not hardcoded 'P0'")
		})
	})

	Describe("UT-KA-779-003: list_available_actions with pagination preserves signal context", func() {
		It("should forward signal context even when cursor pagination is active", func() {
			fake.listActionsResponse.Pagination.HasMore = true
			fake.listActionsResponse.Pagination.TotalCount = 20

			cursor := custom.EncodeCursor(10, 10)
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			args := []byte(`{"page":"next","cursor":"` + cursor + `"}`)
			_, err := listActions.Execute(ctx, args)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(fake.listActionsParams.Severity)).To(Equal("high"),
				"Severity must be from signal even with pagination")
			Expect(fake.listActionsParams.Component).To(Equal("statefulset"),
				"Component must be from signal even with pagination")
			Expect(fake.listActionsParams.Environment).To(Equal("staging"),
				"Environment must be from signal even with pagination")
			Expect(string(fake.listActionsParams.Priority)).To(Equal("P1"),
				"Priority must be from signal even with pagination")

			gotOffset, ok := fake.listActionsParams.Offset.Get()
			Expect(ok).To(BeTrue(), "Offset should be set from cursor")
			Expect(gotOffset).To(Equal(10))
		})
	})

	Describe("UT-KA-779-004: list_workflows with pagination preserves signal context", func() {
		It("should forward signal context even when cursor pagination is active", func() {
			fake.listWorkflowsResponse.Pagination.HasMore = true
			fake.listWorkflowsResponse.Pagination.TotalCount = 20

			cursor := custom.EncodeCursor(10, 10)
			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			args := []byte(`{"action_type":"ScaleReplicas","page":"next","cursor":"` + cursor + `"}`)
			_, err := listWorkflows.Execute(ctx, args)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(fake.listWorkflowsParams.Severity)).To(Equal("high"),
				"Severity must be from signal even with pagination")
			Expect(fake.listWorkflowsParams.Component).To(Equal("statefulset"),
				"Component must be from signal even with pagination")
			Expect(fake.listWorkflowsParams.Environment).To(Equal("staging"),
				"Environment must be from signal even with pagination")
			Expect(string(fake.listWorkflowsParams.Priority)).To(Equal("P1"),
				"Priority must be from signal even with pagination")

			gotOffset, ok := fake.listWorkflowsParams.Offset.Get()
			Expect(ok).To(BeTrue(), "Offset should be set from cursor")
			Expect(gotOffset).To(Equal(10))
		})
	})

	Describe("UT-KA-779-005: tool returns error when signal context is missing from ctx", func() {
		It("list_available_actions should return error with context.Background()", func() {
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(contextBackground(), json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred(),
				"Execute must fail when SignalContext is absent from context")
			Expect(err.Error()).To(ContainSubstring("signal context"),
				"Error message should explain that signal context is required")
		})

		It("list_workflows should return error with context.Background()", func() {
			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(contextBackground(),
				json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).To(HaveOccurred(),
				"Execute must fail when SignalContext is absent from context")
			Expect(err.Error()).To(ContainSubstring("signal context"),
				"Error message should explain that signal context is required")
		})
	})
})

// contextBackground returns a plain context.Background() without any signal.
// Named helper to avoid shadowing the package-level ctx in signal context tests.
func contextBackground() context.Context {
	return context.Background()
}
