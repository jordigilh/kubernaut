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

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

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

// #1661 Change 11f/12: ActionType and WorkflowName are both read directly
// from the immutable, CRD-embedded wfe.Spec.WorkflowRef snapshot (no Status
// mirror or resolve step -- known at WFE creation time, same as
// ExecutionEngine/ServiceAccountName). Change 12 closed the gap Change 11f
// left open: WorkflowName previously had no upstream source (KA never
// populated it) and stayed permanently empty on wfe.Status; it now flows
// through the same sharedtypes.WorkflowSnapshot embed as ActionType via
// KA -> AA -> RO -> WorkflowRef. Business value: workflowexecution
// .execution.started/.workflow.completed/.failed become human-readable
// without joining back to the remediationworkflow.admitted.create audit
// event by workflow_id (SOC2 AU-3 content-of-audit-records completeness).
// workflow_id remains the functional execution/join key regardless --
// ActionType/WorkflowName are additive readability only, never required for
// WFE to execute.
var _ = Describe("buildExecutionStartedPayload / buildWorkflowExecutionAuditPayload — ActionType/WorkflowName (#1661 Change 11f/12)", func() {
	newTestWFE := func(actionType, workflowName string) *workflowexecutionv1alpha1.WorkflowExecution {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
		wfe.Name = "wfe-actiontype-test"
		wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
		wfe.Spec.WorkflowRef.ActionType = actionType
		wfe.Spec.WorkflowRef.WorkflowName = workflowName
		return wfe
	}

	It("UT-WFE-140-001a: buildExecutionStartedPayload populates ActionType/WorkflowName from wfe.Spec.WorkflowRef", func() {
		wfe := newTestWFE("ScaleReplicas", "scale-memory-fix")

		payload := buildExecutionStartedPayload(wfe, "test-pipelinerun")

		Expect(payload.ActionType.IsSet()).To(BeTrue(),
			"BUSINESS VALUE: execution.workflow.started should be human-readable without a DS/audit join (#1661 Change 11f)")
		Expect(payload.ActionType.Value).To(Equal("ScaleReplicas"))
		Expect(payload.WorkflowName.IsSet()).To(BeTrue())
		Expect(payload.WorkflowName.Value).To(Equal("scale-memory-fix"))
	})

	It("UT-WFE-140-001b: buildWorkflowExecutionAuditPayload populates ActionType/WorkflowName from wfe.Spec.WorkflowRef (completed/failed events)", func() {
		wfe := newTestWFE("RestartPod", "restart-pod-fix")

		payload := buildWorkflowExecutionAuditPayload(wfe)

		Expect(payload.ActionType.IsSet()).To(BeTrue())
		Expect(payload.ActionType.Value).To(Equal("RestartPod"))
		Expect(payload.WorkflowName.IsSet()).To(BeTrue())
		Expect(payload.WorkflowName.Value).To(Equal("restart-pod-fix"))
	})

	It("UT-WFE-140-001c: both omit ActionType and WorkflowName when WorkflowRef fields are empty", func() {
		wfe := newTestWFE("", "")

		startedPayload := buildExecutionStartedPayload(wfe, "test-pipelinerun")
		completedPayload := buildWorkflowExecutionAuditPayload(wfe)

		Expect(startedPayload.ActionType.IsSet()).To(BeFalse())
		Expect(startedPayload.WorkflowName.IsSet()).To(BeFalse())
		Expect(completedPayload.ActionType.IsSet()).To(BeFalse())
		Expect(completedPayload.WorkflowName.IsSet()).To(BeFalse())
	})
})

var _ = Describe("DD-AUDIT-010 idempotent execution audit identity", func() {
	It("UT-AUDIT-2512-001: derives a stable event ID per WFE and logical event", func() {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{ObjectMeta: metav1.ObjectMeta{
			Name: "wfe-audit-idempotent", Namespace: "kubernaut-system",
		}}

		first := workflowExecutionAuditEventID(wfe, EventTypeExecutionStarted)
		retry := workflowExecutionAuditEventID(wfe, EventTypeExecutionStarted)
		completed := workflowExecutionAuditEventID(wfe, EventTypeCompleted)
		recreated := wfe.DeepCopy()
		recreated.UID = types.UID("123e4567-e89b-12d3-a456-426614174099")

		Expect(first).To(Equal(retry))
		Expect(completed).NotTo(Equal(first))
		Expect(workflowExecutionAuditEventID(recreated, EventTypeExecutionStarted)).NotTo(Equal(first))
	})

	It("UT-AUDIT-2512-002: includes generic execution and RR identity in the payload", func() {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-audit-ref", Namespace: "kubernaut-system"},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				ClusterID: "cluster-a",
				RemediationRequestRef: corev1.ObjectReference{
					UID: types.UID("550e8400-e29b-41d4-a716-446655440099"),
				},
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					ExecutionEngine: "job",
				}},
			},
			Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
				ExecutionRef: &corev1.LocalObjectReference{Name: "wfe-550e8400-e29b-41d4-a716-446655440099-abcd"},
			},
		}

		payload := buildWorkflowExecutionAuditPayload(wfe)

		Expect(payload.ExecutionResourceKind.IsSet()).To(BeTrue())
		Expect(payload.ExecutionResourceKind.Value).To(Equal("Job"))
		Expect(payload.ExecutionResourceName.IsSet()).To(BeTrue())
		Expect(payload.ExecutionResourceName.Value).To(Equal(wfe.Status.ExecutionRef.Name))
		Expect(payload.ExecutionClusterID.IsSet()).To(BeTrue())
		Expect(payload.ExecutionClusterID.Value).To(Equal("cluster-a"))
		Expect(payload.RemediationRequestUID.IsSet()).To(BeTrue())
		Expect(payload.RemediationRequestUID.Value.String()).To(Equal(string(wfe.Spec.RemediationRequestRef.UID)))
	})
})
