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

package routing

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	prodrouting "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
)

// mockWorkflowCatalogClient implements routing.WorkflowCatalogClient for testing.
type mockWorkflowCatalogClient struct {
	workflows map[uuid.UUID]*ogenclient.RemediationWorkflow
	err       error
}

func (m *mockWorkflowCatalogClient) GetWorkflowByID(
	ctx context.Context,
	params ogenclient.GetWorkflowByIDParams,
) (ogenclient.GetWorkflowByIDRes, error) {
	if m.err != nil {
		return nil, m.err
	}
	wf, ok := m.workflows[params.WorkflowID]
	if !ok {
		return &ogenclient.GetWorkflowByIDNotFound{}, nil
	}
	return wf, nil
}

var _ = Describe("DSWorkflowAdapter", func() {
	const (
		workflowUUID = "deb76100-421f-5623-bb1f-58827e5c93ae"
		workflowName = "restart-crashloop-v2"
		actionType   = "RestartPod"
	)

	It("UT-RO-WF-001: should resolve UUID to WorkflowName and ActionType from DS", func() {
		uid := uuid.MustParse(workflowUUID)
		mock := &mockWorkflowCatalogClient{
			workflows: map[uuid.UUID]*ogenclient.RemediationWorkflow{
				uid: {
					WorkflowName: workflowName,
					ActionType:   actionType,
					WorkflowId:   ogenclient.OptUUID{Value: uid, Set: true},
				},
			},
		}

		adapter := prodrouting.NewDSWorkflowAdapter(mock)
		info := adapter.ResolveWorkflowDisplay(context.Background(), workflowUUID)

		Expect(info).NotTo(BeNil())
		Expect(info.WorkflowName).To(Equal(workflowName))
		Expect(info.ActionType).To(Equal(actionType))
	})

	It("UT-RO-WF-002: should return nil when workflow not found in DS", func() {
		mock := &mockWorkflowCatalogClient{
			workflows: map[uuid.UUID]*ogenclient.RemediationWorkflow{},
		}

		adapter := prodrouting.NewDSWorkflowAdapter(mock)
		info := adapter.ResolveWorkflowDisplay(context.Background(), workflowUUID)

		Expect(info).To(BeNil())
	})

	It("UT-RO-WF-003: should return nil when DS is unreachable", func() {
		mock := &mockWorkflowCatalogClient{
			err: fmt.Errorf("connection refused"),
		}

		adapter := prodrouting.NewDSWorkflowAdapter(mock)
		info := adapter.ResolveWorkflowDisplay(context.Background(), workflowUUID)

		Expect(info).To(BeNil())
	})

	It("UT-RO-WF-004: should return nil for invalid UUID string", func() {
		mock := &mockWorkflowCatalogClient{
			workflows: map[uuid.UUID]*ogenclient.RemediationWorkflow{},
		}

		adapter := prodrouting.NewDSWorkflowAdapter(mock)
		info := adapter.ResolveWorkflowDisplay(context.Background(), "not-a-uuid")

		Expect(info).To(BeNil())
	})

	It("UT-RO-WF-005: should return nil for empty workflow ID", func() {
		mock := &mockWorkflowCatalogClient{
			workflows: map[uuid.UUID]*ogenclient.RemediationWorkflow{},
		}

		adapter := prodrouting.NewDSWorkflowAdapter(mock)
		info := adapter.ResolveWorkflowDisplay(context.Background(), "")

		Expect(info).To(BeNil())
	})
})
