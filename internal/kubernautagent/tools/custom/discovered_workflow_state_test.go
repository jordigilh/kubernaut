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
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Workflow discovery state tool wiring", func() {
	Describe("UT-KA-2442-005: list_workflows records returned IDs (BR-KA-017-002/003, FedRAMP SI-10, ASVS V2/V8)", func() {
		It("should record the workflow returned by the discovery tool", func() {
			workflowID := uuid.New().String()
			fake := &fakeWorkflowDS{
				listWorkflowsEntries: []models.RemediationWorkflow{{WorkflowID: workflowID}},
				listWorkflowsTotal:   1,
			}
			state := types.NewDiscoveredWorkflowState()
			ctx := types.WithDiscoveredWorkflowState(toolCtx(), state)

			result, err := newTestTools(fake)[1].Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring(workflowID))
			Expect(state.Contains(workflowID)).To(BeTrue())
		})
	})

	Describe("UT-KA-2442-006: get_workflow does not grant discovery membership (BR-KA-017-003, FedRAMP AC-6, ASVS V8)", func() {
		It("should not record an ID returned by the parameter lookup tool", func() {
			workflowID := uuid.New().String()
			state := types.NewDiscoveredWorkflowState()
			ctx := types.WithDiscoveredWorkflowState(toolCtx(), state)

			_, err := newTestTools(&fakeWorkflowDS{} /* get_workflow accepts any valid UUID */)[2].Execute(ctx,
				json.RawMessage(fmt.Sprintf(`{"workflow_id":"%s"}`, workflowID)))
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Contains(workflowID)).To(BeFalse())
		})
	})
})
