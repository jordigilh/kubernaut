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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// fakeWorkflowLister implements mcptools.WorkflowLister for unit tests,
// capturing the filters/limit/offset it was called with.
type fakeWorkflowLister struct {
	workflows []models.RemediationWorkflow
	total     int
	err       error

	gotFilters *models.WorkflowSearchFilters
	gotLimit   int
	gotOffset  int
}

func (f *fakeWorkflowLister) List(_ context.Context, filters *models.WorkflowSearchFilters, limit, offset int) ([]models.RemediationWorkflow, int, error) {
	f.gotFilters = filters
	f.gotLimit = limit
	f.gotOffset = offset
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.workflows, f.total, nil
}

var _ = Describe("kubernaut_list_workflows tool — #1677 Phase 2f (DD-WORKFLOW-019)", func() {

	Describe("UT-KA-1677-LW-001: stateless catalog browse", func() {
		It("should list workflows from the catalog with no kind filter", func() {
			lister := &fakeWorkflowLister{
				workflows: []models.RemediationWorkflow{
					{
						WorkflowID:   "wf-1",
						WorkflowName: "restart-pod",
						ActionType:   "restart",
						Description:  models.StructuredDescription{What: "Restarts the pod"},
					},
					{
						WorkflowID:   "wf-2",
						WorkflowName: "scale-up",
						ActionType:   "scale",
					},
				},
				total: 2,
			}

			tool := mcptools.NewListWorkflowsTool(lister)
			output, err := tool.Handle(context.Background(), mcptools.ListWorkflowsInput{})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Count).To(Equal(2))
			Expect(output.Workflows).To(HaveLen(2))
			Expect(output.Workflows[0]).To(Equal(mcptools.CatalogWorkflowSummary{
				ID: "wf-1", Name: "restart-pod", Description: "Restarts the pod", Kind: "restart",
			}))
			Expect(output.Workflows[1]).To(Equal(mcptools.CatalogWorkflowSummary{
				ID: "wf-2", Name: "scale-up", Kind: "scale",
			}))

			Expect(lister.gotFilters).NotTo(BeNil())
			Expect(lister.gotFilters.Component).To(BeEmpty())
			Expect(lister.gotOffset).To(Equal(0))
		})

		It("should translate the kind input into the Component search filter", func() {
			lister := &fakeWorkflowLister{workflows: nil, total: 0}
			tool := mcptools.NewListWorkflowsTool(lister)

			_, err := tool.Handle(context.Background(), mcptools.ListWorkflowsInput{Kind: "Deployment"})
			Expect(err).NotTo(HaveOccurred())
			Expect(lister.gotFilters).NotTo(BeNil())
			Expect(lister.gotFilters.Component).To(Equal("Deployment"))
		})

		It("should return an empty (not nil) workflows slice when the catalog has no matches", func() {
			lister := &fakeWorkflowLister{workflows: nil, total: 0}
			tool := mcptools.NewListWorkflowsTool(lister)

			output, err := tool.Handle(context.Background(), mcptools.ListWorkflowsInput{Kind: "NoSuchKind"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Workflows).NotTo(BeNil())
			Expect(output.Workflows).To(BeEmpty())
			Expect(output.Count).To(Equal(0))
		})

		It("should propagate catalog errors", func() {
			lister := &fakeWorkflowLister{err: errors.New("cache not synced")}
			tool := mcptools.NewListWorkflowsTool(lister)

			_, err := tool.Handle(context.Background(), mcptools.ListWorkflowsInput{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cache not synced"))
		})

		It("should return an internal error when the catalog dependency is nil (fail-open at construction)", func() {
			tool := mcptools.NewListWorkflowsTool(nil)

			_, err := tool.Handle(context.Background(), mcptools.ListWorkflowsInput{})
			Expect(err).To(HaveOccurred())
		})
	})
})
