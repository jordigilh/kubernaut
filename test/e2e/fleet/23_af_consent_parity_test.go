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

package fleet

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// These journeys deliberately mirror E2E-FP-1899-001/002 and
// E2E-FP-1853-002, but run against the fleet-enabled AF/KA/RO stack. They
// prove the consent and completion contracts are not accidentally dependent on
// fullpipeline's single-cluster deployment.
var _ = Describe("AF Fleet Consent and Completion Parity", Label("fleet", "af", "a2a", "consent", "issue-2365"), func() {
	It("E2E-FLEET-2365-001 (AC-6, AU-3, SI-10, ASVS 5.1): preserves phase-1/2 consent through discovery and execution", NodeTimeout(10*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["consent-phase2"]
		Expect(targetNS).NotTo(BeEmpty())
		fleetDeployConsentTarget(targetNS)

		task := fleetSendTurn("fleet-cg2-1", "create and investigate then sneak workflow discovery")
		rrName := fleetWaitForRR(targetNS)
		fleetAssertNoWorkflowExecution(rrName)

		fleetSendTurnWithTask("fleet-cg2-2", task.ID, "ctx-fleet-cg2-1", "confirm discovery of workflows")
		fleetSendTurnWithTask("fleet-cg2-3", task.ID, "ctx-fleet-cg2-1", "select the discovered workflow")
		fleetSendTurnWithTask("fleet-cg2-4", task.ID, "ctx-fleet-cg2-1", "watch this remediation now")
		fleetWaitForWorkflowExecution(rrName)
	})

	It("E2E-FLEET-2365-002 (AC-6, AU-3, SI-10, ASVS 5.1): preserves phase-2/3 consent after authorized discovery", NodeTimeout(10*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["consent-phase3"]
		Expect(targetNS).NotTo(BeEmpty())
		fleetDeployConsentTarget(targetNS)

		task := fleetSendTurn("fleet-cg3-1", "create and investigate then sneak workflow selection")
		rrName := fleetWaitForRR(targetNS)
		fleetAssertNoWorkflowExecution(rrName)

		fleetSendTurnWithTask("fleet-cg3-2", task.ID, "ctx-fleet-cg3-1", "select the discovered workflow")
		fleetSendTurnWithTask("fleet-cg3-3", task.ID, "ctx-fleet-cg3-1", "watch this remediation now")
		fleetWaitForWorkflowExecution(rrName)
	})

	It("E2E-FLEET-2365-003 (AC-6, AU-3, SI-10, ASVS 5.1): preserves autonomous discover-select-watch chaining", NodeTimeout(10*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["full-interactive"]
		Expect(targetNS).NotTo(BeEmpty())
		fleetDeployConsentTarget(targetNS)

		fleetSendTurn("fleet-autonomous-2365", "investigate and fix remediation for deployment memory-eater")
		rrName := fleetWaitForRR(targetNS)
		fleetWaitForWorkflowExecution(rrName)
	})
})

func fleetSendTurn(id, text string) afA2ATaskResult {
	return fleetSendTurnWithBody(afA2ATasksSend(id, text), 180*time.Second)
}

func fleetSendTurnWithTask(id, taskID, contextID, text string) {
	fleetSendTurnWithBody(afA2ATasksSendWithTask(id, taskID, contextID, text), 180*time.Second)
}

func fleetSendTurnWithBody(body string, timeout time.Duration) afA2ATaskResult {
	resp, err := afA2AInvokeWithTimeout(body, timeout)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	rpc, err := afParseRPC(resp)
	Expect(err).NotTo(HaveOccurred())
	Expect(rpc.Error).To(BeNil())
	task, err := afExtractTask(rpc.Result)
	Expect(err).NotTo(HaveOccurred())
	Expect(task.ID).NotTo(BeEmpty())
	return task
}

func fleetDeployConsentTarget(targetNS string) {
	Expect(infrastructure.DeployMemoryEaterNamed(ctx, "memory-eater", targetNS, kubeconfigPath, "64Mi", "20Mi", GinkgoWriter)).To(Succeed())
	DeferCleanup(func() {
		dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "memory-eater", Namespace: targetNS}}
		_ = k8sClient.Delete(context.Background(), dep)
	})
}

func fleetWaitForRR(targetNS string) string {
	var rrName string
	Eventually(func() bool {
		list := &remediationv1.RemediationRequestList{}
		if err := apiReader.List(ctx, list, client.InNamespace(namespace)); err != nil {
			return false
		}
		for _, rr := range list.Items {
			if rr.Spec.TargetResource.Namespace == targetNS && rr.Spec.TargetResource.Name == "memory-eater" {
				rrName = rr.Name
				return true
			}
		}
		return false
	}, 90*time.Second, 2*time.Second).Should(BeTrue(), "fleet consent RR should be created for %s", targetNS)
	return rrName
}

func fleetAssertNoWorkflowExecution(rrName string) {
	Consistently(func() int {
		list := &workflowexecutionv1.WorkflowExecutionList{}
		if err := apiReader.List(ctx, list, client.InNamespace(namespace)); err != nil {
			return 0
		}
		count := 0
		for _, we := range list.Items {
			if we.Spec.RemediationRequestRef.Name == rrName {
				count++
			}
		}
		return count
	}, 10*time.Second, 1*time.Second).Should(Equal(0), "same-turn consent violation must not create a WorkflowExecution for %s", rrName)
}

func fleetWaitForWorkflowExecution(rrName string) {
	Eventually(func() bool {
		list := &workflowexecutionv1.WorkflowExecutionList{}
		if err := apiReader.List(ctx, list, client.InNamespace(namespace)); err != nil {
			return false
		}
		for _, we := range list.Items {
			if we.Spec.RemediationRequestRef.Name != rrName {
				continue
			}
			if we.Status.Phase == "Failed" {
				Fail(fmt.Sprintf("fleet WorkflowExecution %s failed", we.Name))
			}
			return we.Status.Phase == "Completed"
		}
		return false
	}, 5*time.Minute, 3*time.Second).Should(BeTrue(), "fleet WorkflowExecution for %s should complete", rrName)
}
