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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// UT-KA-1511-001: KA passes the Cluster discovery filter when
// SignalContext.ClusterClassification is non-empty, and leaves it empty
// when it is empty -- preserving backward compatibility for non-fleet
// deployments (BR-FLEET-003, #1511, R6.1).
var _ = Describe("UT-KA-1511-001: cluster classification forwarding to discovery filters", func() {

	var fake *fakeWorkflowDS

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

	Describe("UT-KA-1511-001a: list_available_actions forwards cluster filter when ClusterClassification is set", func() {
		It("should set filters.Cluster from SignalContext.ClusterClassification", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:              "critical",
				ResourceKind:          "Deployment",
				Environment:           "production",
				Priority:              "P0",
				ClusterClassification: "production",
			})

			allTools := newTestTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listActionsFilters.Cluster).To(Equal("production"),
				"Cluster must be set on filters when SignalContext carries ClusterClassification")
		})
	})

	Describe("UT-KA-1511-001b: list_workflows forwards cluster filter when ClusterClassification is set", func() {
		It("should set filters.Cluster from SignalContext.ClusterClassification", func() {
			ctx := katypes.WithSignalContext(contextBackground(), katypes.SignalContext{
				Severity:              "critical",
				ResourceKind:          "Deployment",
				Environment:           "production",
				Priority:              "P0",
				ClusterClassification: "staging-eu",
			})

			allTools := newTestTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsFilters.Cluster).To(Equal("staging-eu"),
				"Cluster must be set on filters when SignalContext carries ClusterClassification")
		})
	})

	Describe("UT-KA-1511-001c: list_available_actions omits cluster filter when ClusterClassification is empty (non-fleet)", func() {
		It("should not set filters.Cluster when SignalContext has no ClusterClassification", func() {
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

			Expect(fake.listActionsFilters.Cluster).To(BeEmpty(),
				"Cluster must NOT be set when ClusterClassification is empty (backward compat, R6.1)")
		})
	})

	Describe("UT-KA-1511-001d: list_workflows omits cluster filter when ClusterClassification is empty (non-fleet)", func() {
		It("should not set filters.Cluster when SignalContext has no ClusterClassification", func() {
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

			Expect(fake.listWorkflowsFilters.Cluster).To(BeEmpty(),
				"Cluster must NOT be set when ClusterClassification is empty (backward compat, R6.1)")
		})
	})
})
