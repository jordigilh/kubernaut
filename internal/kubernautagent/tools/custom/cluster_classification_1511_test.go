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
	"encoding/json"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// UT-KA-1511-001: KA passes the `cluster` discovery param when
// SignalContext.ClusterClassification is non-empty, and omits it entirely
// (not as an empty string) when it is empty -- preserving backward
// compatibility for non-fleet deployments (BR-FLEET-003, #1511, R6.1).
var _ = Describe("UT-KA-1511-001: cluster classification forwarding to DS discovery params", func() {

	var fake *fakeWorkflowDS

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

	Describe("UT-KA-1511-001a: list_available_actions forwards cluster param when ClusterClassification is set", func() {
		It("should set params.Cluster from SignalContext.ClusterClassification", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:              "critical",
				ResourceKind:          "Deployment",
				Environment:           "production",
				Priority:              "P0",
				ClusterClassification: "production",
			})

			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			cluster, ok := fake.listActionsParams.Cluster.Get()
			Expect(ok).To(BeTrue(), "Cluster must be set on params when SignalContext carries ClusterClassification")
			Expect(cluster).To(Equal("production"))
		})
	})

	Describe("UT-KA-1511-001b: list_workflows forwards cluster param when ClusterClassification is set", func() {
		It("should set params.Cluster from SignalContext.ClusterClassification", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:              "critical",
				ResourceKind:          "Deployment",
				Environment:           "production",
				Priority:              "P0",
				ClusterClassification: "staging-eu",
			})

			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			cluster, ok := fake.listWorkflowsParams.Cluster.Get()
			Expect(ok).To(BeTrue(), "Cluster must be set on params when SignalContext carries ClusterClassification")
			Expect(cluster).To(Equal("staging-eu"))
		})
	})

	Describe("UT-KA-1511-001c: list_available_actions omits cluster param when ClusterClassification is empty (non-fleet)", func() {
		It("should not set params.Cluster when SignalContext has no ClusterClassification", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:     "critical",
				ResourceKind: "Deployment",
				Environment:  "production",
				Priority:     "P0",
			})

			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			_, ok := fake.listActionsParams.Cluster.Get()
			Expect(ok).To(BeFalse(), "Cluster must NOT be set when ClusterClassification is empty (backward compat, R6.1)")
		})
	})

	Describe("UT-KA-1511-001d: list_workflows omits cluster param when ClusterClassification is empty (non-fleet)", func() {
		It("should not set params.Cluster when SignalContext has no ClusterClassification", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:     "critical",
				ResourceKind: "Deployment",
				Environment:  "production",
				Priority:     "P0",
			})

			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			_, ok := fake.listWorkflowsParams.Cluster.Get()
			Expect(ok).To(BeFalse(), "Cluster must NOT be set when ClusterClassification is empty (backward compat, R6.1)")
		})
	})
})
