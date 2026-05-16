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

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
)

// BR-WORKFLOW-016 / #1064 / #1065: When signal labels carry target_resource_kind,
// the overridden ResourceKind must reach the workflow discovery tools so the
// DataStorage catalog is filtered by the correct component. Without this,
// list_available_actions and list_workflows return empty catalogs.

var _ = Describe("Issue #1064: label-overridden ResourceKind reaches workflow discovery tools", func() {

	var fake *fakeWorkflowDS

	BeforeEach(func() {
		fake = &fakeWorkflowDS{
			listActionsResponse: &ogenclient.ActionTypeListResponse{
				ActionTypes: []ogenclient.ActionTypeEntry{
					{
						ActionType:    "UpgradeSubscription",
						Description:   ogenclient.StructuredDescription{What: "test", WhenToUse: "test"},
						WorkflowCount: 1,
					},
				},
				Pagination: ogenclient.PaginationMetadata{
					TotalCount: 1, Offset: 0, Limit: 10, HasMore: false,
				},
			},
			listWorkflowsResponse: &ogenclient.WorkflowDiscoveryResponse{
				ActionType: "UpgradeSubscription",
				Workflows: []ogenclient.WorkflowDiscoveryEntry{
					{
						WorkflowId:   uuid.New(),
						WorkflowName: "upgrade-olm-sub-v1",
						Name:         "Upgrade OLM Subscription",
						Description:  ogenclient.StructuredDescription{What: "test", WhenToUse: "test"},
					},
				},
				Pagination: ogenclient.PaginationMetadata{
					TotalCount: 1, Offset: 0, Limit: 10, HasMore: false,
				},
			},
		}
	})

	overriddenCtx := func() context.Context {
		return katypes.WithSignalContext(context.Background(), katypes.SignalContext{
			Severity:     "high",
			ResourceKind: "Subscription",
			ResourceName: "etcd",
			Environment:  "production",
			Priority:     "P1",
		})
	}

	Describe("UT-KA-1064-017: list_available_actions uses label-overridden ResourceKind for Component", func() {
		It("should query DS with Component=subscription (from overridden ResourceKind)", func() {
			allTools := custom.NewAllTools(fake)
			listActions := allTools[0]

			_, err := listActions.Execute(overriddenCtx(), json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listActionsParams.Component).To(Equal("subscription"),
				"Component param must use the overridden ResourceKind=Subscription, lowercased")
		})
	})

	Describe("UT-KA-1064-018: list_workflows uses label-overridden ResourceKind for Component", func() {
		It("should query DS with Component=subscription (from overridden ResourceKind)", func() {
			allTools := custom.NewAllTools(fake)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(overriddenCtx(),
				json.RawMessage(`{"action_type":"UpgradeSubscription"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.listWorkflowsParams.Component).To(Equal("subscription"),
				"Component param must use the overridden ResourceKind=Subscription, lowercased")
		})
	})
})
