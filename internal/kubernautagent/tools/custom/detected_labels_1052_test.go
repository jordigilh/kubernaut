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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// BR-AI-056: KA tools must forward DetectedLabels from enrichment to the
// workflow catalog's discovery filters so the scoring engine can boost
// GitOps-compatible workflows and penalize non-GitOps ones.
// Issue #1052.

var _ = Describe("UT-KA-1052: DetectedLabels forwarding to discovery filters", func() {

	const detectedLabelsJSON = `{"gitOpsManaged":true,"gitOpsTool":"argocd"}`

	var fake *fakeWorkflowDS

	BeforeEach(func() {
		fake = &fakeWorkflowDS{}
	})

	Describe("UT-KA-1052-001: list_available_actions forwards DetectedLabelsJSON to discovery filters", func() {
		It("should set filters.DetectedLabels from SignalContext.DetectedLabelsJSON", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:           "critical",
				ResourceKind:       "Deployment",
				Environment:        "production",
				Priority:           "P0",
				DetectedLabelsJSON: detectedLabelsJSON,
			})

			allTools := newTestTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listActionsFilters.DetectedLabels).NotTo(BeNil(),
				"DetectedLabels must be set on filters when SignalContext carries DetectedLabelsJSON")
			Expect(fake.listActionsFilters.DetectedLabels.GitOpsManaged).To(BeTrue())
			Expect(fake.listActionsFilters.DetectedLabels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-1052-002: list_workflows forwards DetectedLabelsJSON to discovery filters", func() {
		It("should set filters.DetectedLabels from SignalContext.DetectedLabelsJSON", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:           "critical",
				ResourceKind:       "Deployment",
				Environment:        "production",
				Priority:           "P0",
				DetectedLabelsJSON: detectedLabelsJSON,
			})

			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsFilters.DetectedLabels).NotTo(BeNil(),
				"DetectedLabels must be set on filters when SignalContext carries DetectedLabelsJSON")
			Expect(fake.listWorkflowsFilters.DetectedLabels.GitOpsManaged).To(BeTrue())
			Expect(fake.listWorkflowsFilters.DetectedLabels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-1052-003: list_available_actions omits DetectedLabels when empty", func() {
		It("should not set filters.DetectedLabels when SignalContext has no DetectedLabelsJSON", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:     "critical",
				ResourceKind: "Deployment",
				Environment:  "production",
				Priority:     "P0",
			})

			allTools := newTestTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listActionsFilters.DetectedLabels).To(BeNil(),
				"DetectedLabels must NOT be set when DetectedLabelsJSON is empty")
		})
	})

	Describe("UT-KA-1052-004: list_workflows omits DetectedLabels when empty", func() {
		It("should not set filters.DetectedLabels when SignalContext has no DetectedLabelsJSON", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:     "critical",
				ResourceKind: "Deployment",
				Environment:  "production",
				Priority:     "P0",
			})

			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsFilters.DetectedLabels).To(BeNil(),
				"DetectedLabels must NOT be set when DetectedLabelsJSON is empty")
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

	Describe("UT-KA-1052-006: empty enrichment DetectedLabels produces no filter on tools", func() {
		It("should not set DetectedLabels on either tool when DetectedLabelsJSON is zero-value", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:           "critical",
				ResourceKind:       "Deployment",
				Environment:        "production",
				Priority:           "P0",
				DetectedLabelsJSON: "",
			})

			allTools := newTestTools(fake)

			_, err := allTools[0].Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(fake.listActionsFilters.DetectedLabels).To(BeNil(),
				"list_available_actions must not set DetectedLabels for empty enrichment labels")

			_, err = allTools[1].Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(fake.listWorkflowsFilters.DetectedLabels).To(BeNil(),
				"list_workflows must not set DetectedLabels for empty enrichment labels")
		})
	})
})
