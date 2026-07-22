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

package audit

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

func TestAudit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Audit Manager Unit Test Suite")
}

var _ = Describe("buildWorkflowExecutionAuditPayload retry-count (BR-WE-019 AC10, DD-WE-008 Wiring Point C)", func() {
	newTestWFE := func(executionStatus *workflowexecutionv1alpha1.ExecutionStatusSummary) *workflowexecutionv1alpha1.WorkflowExecution {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
		wfe.Name = "wfe-retry-count-test"
		wfe.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
		wfe.Status.ExecutionStatus = executionStatus
		return wfe
	}

	It("UT-WE-AUDIT-001: should set payload.RetryCount from wfe.Status.ExecutionStatus.RetryCount when > 0", func() {
		wfe := newTestWFE(&workflowexecutionv1alpha1.ExecutionStatusSummary{RetryCount: 2})

		payload := buildWorkflowExecutionAuditPayload(wfe)

		Expect(payload.RetryCount.Set).To(BeTrue(), "BR-WE-019 AC10: retry_count must be present when tolerated failures occurred")
		Expect(payload.RetryCount.Value).To(Equal(2))
	})

	It("UT-WE-AUDIT-002a: should leave payload.RetryCount unset when ExecutionStatus is nil", func() {
		wfe := newTestWFE(nil)

		payload := buildWorkflowExecutionAuditPayload(wfe)

		Expect(payload.RetryCount.Set).To(BeFalse(), "no spurious retry_count when ExecutionStatus was never populated")
	})

	It("UT-WE-AUDIT-002b: should leave payload.RetryCount unset when RetryCount == 0", func() {
		wfe := newTestWFE(&workflowexecutionv1alpha1.ExecutionStatusSummary{RetryCount: 0})

		payload := buildWorkflowExecutionAuditPayload(wfe)

		Expect(payload.RetryCount.Set).To(BeFalse(), "no spurious retry_count: 0 for the common zero-failure case")
	})
})

// #1661 Change 11f: ActionType is read directly from the immutable,
// CRD-embedded wfe.Spec.WorkflowRef.ActionType snapshot (no Status mirror
// or resolve step -- known at WFE creation time, same as
// ExecutionEngine/ServiceAccountName). WorkflowName has no equivalent
// upstream source (WorkflowRef carries no display-name field) and stays on
// wfe.Status where it will simply always be empty. Business value:
// workflowexecution.execution.started/.workflow.completed/.failed become
// human-readable without joining back to the
// remediationworkflow.admitted.create audit event by workflow_id (SOC2 AU-3
// content-of-audit-records completeness). workflow_id remains the functional
// execution/join key regardless -- ActionType/WorkflowName are additive
// readability only, never required for WFE to execute.
var _ = Describe("buildExecutionStartedPayload / buildWorkflowExecutionAuditPayload — ActionType/WorkflowName (#1661 Change 11f)", func() {
	newTestWFE := func(actionType, workflowName string) *workflowexecutionv1alpha1.WorkflowExecution {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
		wfe.Name = "wfe-actiontype-test"
		wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
		wfe.Spec.WorkflowRef.ActionType = actionType
		wfe.Status.WorkflowName = workflowName
		return wfe
	}

	It("UT-WFE-140-001a: buildExecutionStartedPayload populates ActionType from wfe.Spec.WorkflowRef and WorkflowName from wfe.Status", func() {
		wfe := newTestWFE("ScaleReplicas", "scale-memory-fix")

		payload := buildExecutionStartedPayload(wfe, "test-pipelinerun")

		Expect(payload.ActionType.IsSet()).To(BeTrue(),
			"BUSINESS VALUE: execution.workflow.started should be human-readable without a DS/audit join (#1661 Change 11f)")
		Expect(payload.ActionType.Value).To(Equal("ScaleReplicas"))
		Expect(payload.WorkflowName.IsSet()).To(BeTrue())
		Expect(payload.WorkflowName.Value).To(Equal("scale-memory-fix"))
	})

	It("UT-WFE-140-001b: buildWorkflowExecutionAuditPayload populates ActionType from wfe.Spec.WorkflowRef and WorkflowName from wfe.Status (completed/failed events)", func() {
		wfe := newTestWFE("RestartPod", "restart-pod-fix")

		payload := buildWorkflowExecutionAuditPayload(wfe)

		Expect(payload.ActionType.IsSet()).To(BeTrue())
		Expect(payload.ActionType.Value).To(Equal("RestartPod"))
		Expect(payload.WorkflowName.IsSet()).To(BeTrue())
		Expect(payload.WorkflowName.Value).To(Equal("restart-pod-fix"))
	})

	It("UT-WFE-140-001c: both omit ActionType when WorkflowRef.ActionType is empty and WorkflowName since it's never resolved", func() {
		wfe := newTestWFE("", "")

		startedPayload := buildExecutionStartedPayload(wfe, "test-pipelinerun")
		completedPayload := buildWorkflowExecutionAuditPayload(wfe)

		Expect(startedPayload.ActionType.IsSet()).To(BeFalse())
		Expect(startedPayload.WorkflowName.IsSet()).To(BeFalse())
		Expect(completedPayload.ActionType.IsSet()).To(BeFalse())
		Expect(completedPayload.WorkflowName.IsSet()).To(BeFalse())
	})
})
