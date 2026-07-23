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
	"errors"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// errorDS returns errors from all Catalog methods (Issue #1677 Phase 2d:
// adapted from the former ogenclient-shaped fake to custom.WorkflowCatalog).
type errorDS struct {
	actionsErr   error
	workflowsErr error
	getErr       error
}

func (e *errorDS) ListActions(_ context.Context, _ *models.WorkflowDiscoveryFilters, _, _ int) ([]models.ActionTypeEntry, int, error) {
	return nil, 0, e.actionsErr
}
func (e *errorDS) ListWorkflowsByActionType(_ context.Context, _ string, _ *models.WorkflowDiscoveryFilters, _, _ int) ([]models.RemediationWorkflow, int, error) {
	return nil, 0, e.workflowsErr
}
func (e *errorDS) GetWorkflowWithContextFilters(_ context.Context, _ string, _ *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, error) {
	return nil, e.getErr
}

var _ = Describe("UT-KA-779-ERR: DS error path and get_workflow tests", func() {

	var signalCtx context.Context

	BeforeEach(func() {
		signalCtx = katypes.WithSignalContext(context.Background(), katypes.SignalContext{
			Severity:     "critical",
			ResourceKind: "Deployment",
			Environment:  "production",
			Priority:     "P0",
		})
	})

	Describe("UT-KA-779-ERR-001: get_workflow Execute returns valid JSON for known workflow", func() {
		It("should return marshaled workflow data", func() {
			wfID := uuid.New()
			fake := &fakeWorkflowDS{}

			allTools := newTestTools(fake)
			getWorkflow := allTools[2]

			result, err := getWorkflow.Execute(signalCtx,
				json.RawMessage(fmt.Sprintf(`{"workflow_id":"%s"}`, wfID.String())))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty(), "get_workflow should return non-empty JSON")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &parsed)).To(Succeed(),
				"get_workflow result should be valid JSON")
		})
	})

	Describe("UT-KA-779-ERR-002: get_workflow returns error for invalid UUID", func() {
		It("should return error with invalid UUID message", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			getWorkflow := allTools[2]

			_, err := getWorkflow.Execute(signalCtx,
				json.RawMessage(`{"workflow_id":"not-a-uuid"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid workflow ID"),
				"Error should indicate UUID parsing failure")
		})
	})

	Describe("UT-KA-779-ERR-003: get_workflow returns error for malformed JSON args", func() {
		It("should return error when args cannot be parsed", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			getWorkflow := allTools[2]

			_, err := getWorkflow.Execute(signalCtx, json.RawMessage(`{bad json`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing args"),
				"Error should indicate argument parsing failure")
		})
	})

	Describe("UT-KA-779-ERR-004: list_available_actions wraps catalog errors", func() {
		It("should return wrapped error when catalog ListActions fails", func() {
			dsErr := errors.New("connection refused")
			errDS := &errorDS{actionsErr: dsErr}
			allTools := newTestTools(errDS)
			listActions := allTools[0]

			_, err := listActions.Execute(signalCtx, json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("listing action types"),
				"Error should be wrapped with tool context")
			Expect(errors.Is(err, dsErr)).To(BeTrue(),
				"Original catalog error should be unwrappable")
		})
	})

	Describe("UT-KA-779-ERR-005: list_workflows wraps catalog errors", func() {
		It("should return wrapped error when catalog ListWorkflowsByActionType fails", func() {
			dsErr := errors.New("timeout")
			errDS := &errorDS{workflowsErr: dsErr}
			allTools := newTestTools(errDS)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(signalCtx,
				json.RawMessage(`{"action_type":"RestartPod"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("listing workflows"),
				"Error should be wrapped with tool context")
			Expect(errors.Is(err, dsErr)).To(BeTrue(),
				"Original catalog error should be unwrappable")
		})
	})

	Describe("UT-KA-779-ERR-006: get_workflow wraps catalog errors", func() {
		It("should return wrapped error when catalog GetWorkflowWithContextFilters fails", func() {
			dsErr := errors.New("not found")
			errDS := &errorDS{getErr: dsErr}
			allTools := newTestTools(errDS)
			getWorkflow := allTools[2]

			wfID := uuid.New()
			_, err := getWorkflow.Execute(signalCtx,
				json.RawMessage(fmt.Sprintf(`{"workflow_id":"%s"}`, wfID.String())))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("getting workflow"),
				"Error should be wrapped with tool context")
			Expect(errors.Is(err, dsErr)).To(BeTrue(),
				"Original catalog error should be unwrappable")
		})
	})
})
