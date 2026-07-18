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
