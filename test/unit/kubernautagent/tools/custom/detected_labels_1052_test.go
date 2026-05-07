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

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
)

// BR-AI-056: KA tools must forward DetectedLabels from enrichment to
// DataStorage catalog queries so the scoring engine can boost
// GitOps-compatible workflows and penalize non-GitOps ones.
// Issue #1052.

var _ = Describe("UT-KA-1052: DetectedLabels forwarding to DS tool params", func() {

	const detectedLabelsJSON = `{"gitOpsManaged":true,"gitOpsTool":"argocd"}`

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

	Describe("UT-KA-1052-001: list_available_actions forwards DetectedLabelsJSON to DS params", func() {
		It("should set params.DetectedLabels from SignalContext.DetectedLabelsJSON", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:           "critical",
				ResourceKind:       "Deployment",
				Environment:        "production",
				Priority:           "P0",
				DetectedLabelsJSON: detectedLabelsJSON,
			})

			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			dl, ok := fake.listActionsParams.DetectedLabels.Get()
			Expect(ok).To(BeTrue(), "DetectedLabels must be set on params when SignalContext carries DetectedLabelsJSON")
			Expect(dl).To(Equal(detectedLabelsJSON))
		})
	})

	Describe("UT-KA-1052-002: list_workflows forwards DetectedLabelsJSON to DS params", func() {
		It("should set params.DetectedLabels from SignalContext.DetectedLabelsJSON", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:           "critical",
				ResourceKind:       "Deployment",
				Environment:        "production",
				Priority:           "P0",
				DetectedLabelsJSON: detectedLabelsJSON,
			})

			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			dl, ok := fake.listWorkflowsParams.DetectedLabels.Get()
			Expect(ok).To(BeTrue(), "DetectedLabels must be set on params when SignalContext carries DetectedLabelsJSON")
			Expect(dl).To(Equal(detectedLabelsJSON))
		})
	})

	Describe("UT-KA-1052-003: list_available_actions omits DetectedLabels when empty", func() {
		It("should not set params.DetectedLabels when SignalContext has no DetectedLabelsJSON", func() {
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

			_, ok := fake.listActionsParams.DetectedLabels.Get()
			Expect(ok).To(BeFalse(), "DetectedLabels must NOT be set when DetectedLabelsJSON is empty")
		})
	})

	Describe("UT-KA-1052-004: list_workflows omits DetectedLabels when empty", func() {
		It("should not set params.DetectedLabels when SignalContext has no DetectedLabelsJSON", func() {
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

			_, ok := fake.listWorkflowsParams.DetectedLabels.Get()
			Expect(ok).To(BeFalse(), "DetectedLabels must NOT be set when DetectedLabelsJSON is empty")
		})
	})

	Describe("UT-KA-1052-005: DetectedLabelsJSON round-trips through SignalContext", func() {
		It("should store and retrieve DetectedLabelsJSON via context", func() {
			signal := katypes.SignalContext{
				Severity:           "high",
				ResourceKind:       "StatefulSet",
				Environment:        "staging",
				Priority:           "P1",
				DetectedLabelsJSON: detectedLabelsJSON,
			}

			ctx := katypes.WithSignalContext(contextBackground(), signal)
			retrieved, ok := katypes.SignalContextFromContext(ctx)
			Expect(ok).To(BeTrue(), "SignalContext must be found in context")
			Expect(retrieved.DetectedLabelsJSON).To(Equal(detectedLabelsJSON),
				"DetectedLabelsJSON must survive round-trip through context")
		})
	})

	Describe("UT-KA-1052-006: nil enrichment DetectedLabels produces no param on tools", func() {
		It("should not set DetectedLabels on either tool when DetectedLabelsJSON is zero-value", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:           "critical",
				ResourceKind:       "Deployment",
				Environment:        "production",
				Priority:           "P0",
				DetectedLabelsJSON: "",
			})

			allTools := custom.NewAllTools(fake)

			_, err := allTools[0].Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			_, ok := fake.listActionsParams.DetectedLabels.Get()
			Expect(ok).To(BeFalse(), "list_available_actions must not set DetectedLabels for nil enrichment labels")

			_, err = allTools[1].Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())
			_, ok = fake.listWorkflowsParams.DetectedLabels.Get()
			Expect(ok).To(BeFalse(), "list_workflows must not set DetectedLabels for nil enrichment labels")
		})
	})
})
