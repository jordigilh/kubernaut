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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// BR-WORKFLOW-016: Workflow discovery tools must forward the active signal's
// context (severity, component, environment, priority) to the workflow
// catalog so discovery is filtered to the incident's actual characteristics,
// not hardcoded defaults. Issue #779 exposed hardcoded values; Issue #1051:
// component uses ComponentGVK() when resource_api_version + resource_kind
// are set.

var _ = Describe("UT-KA-779: Signal context forwarding to discovery filters", func() {

	var (
		fake *fakeWorkflowDS
		ctx  = katypes.WithSignalContext(
			contextBackground(),
			katypes.SignalContext{
				Severity:           "high",
				ResourceKind:       "StatefulSet",
				ResourceAPIVersion: "apps/v1",
				Environment:        "staging",
				Priority:           "P1",
			},
		)
	)

	BeforeEach(func() {
		fake = &fakeWorkflowDS{
			listActionsEntries: []models.ActionTypeEntry{
				{ActionType: "ScaleReplicas", Description: models.ActionTypeDescription{What: "test", WhenToUse: "test"}, WorkflowCount: 1},
			},
			listActionsTotal: 1,
			listWorkflowsEntries: []models.RemediationWorkflow{
				{WorkflowID: uuid.New().String(), WorkflowName: "scale-conservative-v1", Name: "Scale Conservative", Description: models.StructuredDescription{What: "test", WhenToUse: "test"}},
			},
			listWorkflowsTotal: 1,
		}
	})

	Describe("UT-KA-779-001: list_available_actions forwards signal context to discovery filters", func() {
		It("should set Severity, Component, Environment, Priority from SignalContext", func() {
			allTools := newTestTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listActionsFilters.Severity).To(Equal("high"),
				"Severity should come from SignalContext, not hardcoded 'critical'")
			Expect(fake.listActionsFilters.Component).To(Equal("apps/v1/StatefulSet"),
				"Component should be SignalContext.ComponentGVK() (#1051)")
			Expect(fake.listActionsFilters.Environment).To(Equal("staging"),
				"Environment should come from SignalContext, not hardcoded 'production'")
			Expect(fake.listActionsFilters.Priority).To(Equal("P1"),
				"Priority should come from SignalContext, not hardcoded 'P0'")
		})
	})

	Describe("UT-KA-779-002: list_workflows forwards signal context to discovery filters", func() {
		It("should set Severity, Component, Environment, Priority from SignalContext", func() {
			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx,
				json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsFilters.Severity).To(Equal("high"),
				"Severity should come from SignalContext, not hardcoded 'critical'")
			Expect(fake.listWorkflowsFilters.Component).To(Equal("apps/v1/StatefulSet"),
				"Component should be SignalContext.ComponentGVK() (#1051)")
			Expect(fake.listWorkflowsFilters.Environment).To(Equal("staging"),
				"Environment should come from SignalContext, not hardcoded 'production'")
			Expect(fake.listWorkflowsFilters.Priority).To(Equal("P1"),
				"Priority should come from SignalContext, not hardcoded 'P0'")
		})
	})

	Describe("UT-KA-779-003: list_available_actions with pagination preserves signal context", func() {
		It("should forward signal context even when cursor pagination is active", func() {
			fake.listActionsTotal = 20

			cursor := custom.EncodeCursor(10, 10)
			allTools := newTestTools(fake)
			listActions := allTools[0]

			args := []byte(`{"page":"next","cursor":"` + cursor + `"}`)
			_, err := listActions.Execute(ctx, args)
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listActionsFilters.Severity).To(Equal("high"),
				"Severity must be from signal even with pagination")
			Expect(fake.listActionsFilters.Component).To(Equal("apps/v1/StatefulSet"),
				"Component must be ComponentGVK from signal even with pagination")
			Expect(fake.listActionsFilters.Environment).To(Equal("staging"),
				"Environment must be from signal even with pagination")
			Expect(fake.listActionsFilters.Priority).To(Equal("P1"),
				"Priority must be from signal even with pagination")

			Expect(fake.listActionsOffset).To(Equal(10), "Offset should be set from cursor")
		})
	})

	Describe("UT-KA-779-004: list_workflows with pagination preserves signal context", func() {
		It("should forward signal context even when cursor pagination is active", func() {
			fake.listWorkflowsTotal = 20

			cursor := custom.EncodeCursor(10, 10)
			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			args := []byte(`{"action_type":"ScaleReplicas","page":"next","cursor":"` + cursor + `"}`)
			_, err := listWorkflows.Execute(ctx, args)
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsFilters.Severity).To(Equal("high"),
				"Severity must be from signal even with pagination")
			Expect(fake.listWorkflowsFilters.Component).To(Equal("apps/v1/StatefulSet"),
				"Component must be ComponentGVK from signal even with pagination")
			Expect(fake.listWorkflowsFilters.Environment).To(Equal("staging"),
				"Environment must be from signal even with pagination")
			Expect(fake.listWorkflowsFilters.Priority).To(Equal("P1"),
				"Priority must be from signal even with pagination")

			Expect(fake.listWorkflowsOffset).To(Equal(10), "Offset should be set from cursor")
		})
	})

	Describe("UT-KA-779-005: tool returns error when signal context is missing from ctx", func() {
		It("list_available_actions should return error with context.Background()", func() {
			allTools := newTestTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(contextBackground(), json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred(),
				"Execute must fail when SignalContext is absent from context")
			Expect(err.Error()).To(ContainSubstring("signal context"),
				"Error message should explain that signal context is required")
		})

		It("list_workflows should return error with context.Background()", func() {
			allTools := newTestTools(fake)
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

var _ = Describe("UT-KA-1051: GVK fallback when ResourceAPIVersion is empty", func() {

	var fake *fakeWorkflowDS

	BeforeEach(func() {
		fake = &fakeWorkflowDS{}
	})

	Describe("UT-KA-1051-030: list_available_actions falls back to lowercase kind when ComponentGVK is empty", func() {
		It("should send lowercase ResourceKind as component when ResourceAPIVersion is empty (Issue #1051)", func() {
			noAPIVersionCtx := katypes.WithSignalContext(
				contextBackground(),
				katypes.SignalContext{
					Severity:     "high",
					ResourceKind: "Deployment",
					Environment:  "staging",
					Priority:     "P1",
				},
			)
			allTools := newTestTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(noAPIVersionCtx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(fake.listActionsFilters.Component).To(Equal("deployment"),
				"Issue #1051: fallback must use strings.ToLower(ResourceKind) when ComponentGVK() is empty")
		})
	})

	Describe("UT-KA-1051-031: list_workflows falls back to lowercase kind when ComponentGVK is empty", func() {
		It("should send lowercase ResourceKind as component when ResourceAPIVersion is empty (Issue #1051)", func() {
			noAPIVersionCtx := katypes.WithSignalContext(
				contextBackground(),
				katypes.SignalContext{
					Severity:     "high",
					ResourceKind: "StatefulSet",
					Environment:  "staging",
					Priority:     "P1",
				},
			)
			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(noAPIVersionCtx,
				json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(fake.listWorkflowsFilters.Component).To(Equal("statefulset"),
				"Issue #1051: fallback must use strings.ToLower(ResourceKind) when ComponentGVK() is empty")
		})
	})
})

// contextBackground returns a plain context.Background() without any signal.
// Named helper to avoid shadowing the package-level ctx in signal context tests.
func contextBackground() context.Context {
	return context.Background()
}
