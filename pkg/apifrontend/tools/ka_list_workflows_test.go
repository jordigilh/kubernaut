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

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// #1677 Phase 2g (DD-WORKFLOW-019): dedicated coverage for
// HandleListWorkflowsKA, the KA-backed replacement for the retired
// DS-backed HandleListWorkflows. The wiring-level nil-guard assertion lives
// in ka_investigate_wiring_test.go; this file covers the success path, the
// summary DTO mapping, and error propagation from the KA MCP client.
var _ = Describe("HandleListWorkflowsKA (#1677 Phase 2f/2g)", func() {
	It("UT-AF-1677-LW-001: maps KA MCPClient results to the tool's WorkflowSummary DTO", func() {
		mock := &ka.MockMCPClient{
			ListWorkflowsFn: func(_ context.Context, args ka.ListWorkflowsArgs) (*ka.ListWorkflowsResult, error) {
				Expect(args.Kind).To(Equal("Deployment"))
				return &ka.ListWorkflowsResult{
					Workflows: []ka.WorkflowSummary{
						{ID: "wf-1", Name: "restart-pod", Description: "Restarts the pod", Kind: "Deployment"},
						{ID: "wf-2", Name: "scale-up", Kind: "Deployment"},
					},
					Count: 2,
				}, nil
			},
		}

		result, err := tools.HandleListWorkflowsKA(context.Background(), mock, tools.ListWorkflowsArgs{Kind: "Deployment"})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Workflows).To(HaveLen(2))
		Expect(result.Workflows[0]).To(Equal(tools.WorkflowSummary{
			ID: "wf-1", Name: "restart-pod", Description: "Restarts the pod", Kind: "Deployment",
		}))
		Expect(result.Workflows[1]).To(Equal(tools.WorkflowSummary{
			ID: "wf-2", Name: "scale-up", Kind: "Deployment",
		}))
	})

	It("UT-AF-1677-LW-002: returns an empty result when the catalog has no matches", func() {
		mock := &ka.MockMCPClient{
			ListWorkflowsFn: func(_ context.Context, _ ka.ListWorkflowsArgs) (*ka.ListWorkflowsResult, error) {
				return &ka.ListWorkflowsResult{Workflows: nil, Count: 0}, nil
			},
		}

		result, err := tools.HandleListWorkflowsKA(context.Background(), mock, tools.ListWorkflowsArgs{})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
		Expect(result.Workflows).To(BeEmpty())
	})

	It("UT-AF-1677-LW-003: propagates errors from the KA MCP client", func() {
		mock := &ka.MockMCPClient{
			ListWorkflowsFn: func(_ context.Context, _ ka.ListWorkflowsArgs) (*ka.ListWorkflowsResult, error) {
				return nil, errors.New("ka mcp: catalog unavailable")
			},
		}

		_, err := tools.HandleListWorkflowsKA(context.Background(), mock, tools.ListWorkflowsArgs{})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("catalog unavailable"))
	})

	It("UT-AF-1677-LW-004: errors when the KA MCP client is nil", func() {
		_, err := tools.HandleListWorkflowsKA(context.Background(), nil, tools.ListWorkflowsArgs{})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("MCP client not configured"))
	})
})
