package tools_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_list_workflows", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-121-001: returns workflow catalog", func() {
		mock := &ds.MockClient{
			ListWorkflowsFn: func(ctx context.Context, opts ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
				return []ds.Workflow{
					{ID: "wf-1", Name: "Restart Pod", Kind: "Deployment"},
					{ID: "wf-2", Name: "Scale Up", Kind: "Deployment"},
				}, nil
			},
		}
		result, err := tools.HandleListWorkflows(ctx, mock, tools.ListWorkflowsArgs{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Workflows).To(HaveLen(2))
	})

	It("UT-AF-121-002: empty catalog", func() {
		mock := &ds.MockClient{
			ListWorkflowsFn: func(ctx context.Context, opts ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
				return nil, nil
			},
		}
		result, err := tools.HandleListWorkflows(ctx, mock, tools.ListWorkflowsArgs{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-121-003: DS unavailable", func() {
		mock := &ds.MockClient{
			ListWorkflowsFn: func(ctx context.Context, opts ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}
		_, err := tools.HandleListWorkflows(ctx, mock, tools.ListWorkflowsArgs{})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-121-004: filter by kind", func() {
		mock := &ds.MockClient{
			ListWorkflowsFn: func(ctx context.Context, opts ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
				Expect(opts.Kind).To(Equal("Deployment"))
				return []ds.Workflow{{ID: "wf-1", Name: "Restart Pod", Kind: "Deployment"}}, nil
			},
		}
		result, err := tools.HandleListWorkflows(ctx, mock, tools.ListWorkflowsArgs{Kind: "Deployment"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})
})
