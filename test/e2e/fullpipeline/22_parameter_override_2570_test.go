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

package fullpipeline

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/workflowidentity"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// BR-AUTH-001, BR-KA-191: an invalid parameter change cannot authorize execution.
var _ = Describe("Approval parameter denial [E2E-2570-002]", Label("e2e", "fullpipeline", "apifrontend", "issue-2570"), func() {
	It("rejects a new parameter through deployed AF without committing or executing", func() {
		By("Creating a fresh production target and approval-required remediation")
		targetNS := fmt.Sprintf("fp-2570-%d", time.Now().UnixNano())
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: targetNS, Labels: map[string]string{"kubernaut.ai/managed": "true", "kubernaut.ai/environment": "production"},
		}})).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNS}}))).To(Succeed())
		})
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "memory-eater", Namespace: targetNS},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "memory-eater"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "memory-eater"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36", Command: []string{"sleep", "3600"}}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "memory-eater", Namespace: targetNS, Labels: map[string]string{"app": "memory-eater"},
				OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: deployment.Name, UID: deployment.UID, Controller: ptr.To(true)}}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36", Command: []string{"sleep", "3600"}}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		rrName := fpPostSignalToGateway("OOMKilled", pod.Name, targetNS)
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &remediationv1.RemediationRequest{ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: namespace}}, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
		})
		rarKey := client.ObjectKey{Namespace: namespace, Name: "rar-" + rrName}
		var before remediationv1.RemediationApprovalRequest
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, rarKey, &before)).To(Succeed())
			g.Expect(before.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
			g.Expect(before.Spec.Workflow).NotTo(BeNil())
			g.Expect(before.Spec.Workflow.ExecutionEngine).To(Equal("job"))
			g.Expect(before.Spec.Parameters).NotTo(BeNil())
			g.Expect(before.Spec.TargetContext).NotTo(BeNil())
			live, err := workflowidentity.GetVerified(ctx, apiReader, namespace, before.Spec.Workflow.WorkflowID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(remediationapprovalrequest.ValidateTypedParametersAgainstWorkflow(live, before.Spec.Parameters)).To(Succeed())
		}, timeout, interval).Should(Succeed(), "RO must create a pending, Job-backed approval with frozen parameters")
		Expect(before.Spec.Parameters).NotTo(HaveKey("E2E_INVALID_NEW_PARAMETER"))
		initialStatus := before.Status.DeepCopy()

		By("Submitting an invalid new parameter through the authenticated AF MCP session")
		token := getAFToken()
		Expect(token).NotTo(BeEmpty())
		sessionID, err := fpInitMCPSessionExplicit(afHTTPClient, afBaseURL, token)
		Expect(err).NotTo(HaveOccurred())
		body := fpBuildJSONRPC("e2e-2570-002", "tools/call", map[string]interface{}{
			"name": "kubernaut_approve", "arguments": map[string]interface{}{
				"rar_name": rarKey.Name, "decision": "Approved",
				"workflow_override": map[string]interface{}{"parameters": map[string]string{"E2E_INVALID_NEW_PARAMETER": "invalid"}},
			},
		})
		raw, code, err := fpMCPPOST(afHTTPClient, afBaseURL, token, sessionID, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(http.StatusOK), "MCP transport: %s", raw)
		// Streamable HTTP may return either a JSON document or a single SSE data frame.
		payload := strings.TrimSpace(string(raw))
		if strings.HasPrefix(payload, "event:") || strings.HasPrefix(payload, "data:") {
			for _, line := range strings.Split(payload, "\n") {
				if strings.HasPrefix(line, "data:") {
					payload = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
					break
				}
			}
		}
		var response struct {
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		Expect(json.Unmarshal([]byte(payload), &response)).To(Succeed(), "MCP response: %s", raw)
		Expect(response.Result.IsError).To(BeTrue(), "MCP must report a tool error: %s", raw)
		Expect(string(raw)).To(ContainSubstring("E2E_INVALID_NEW_PARAMETER"), "denial must identify the invalid parameter, not an unrelated failure")

		By("Checking the decision and absence of correlated Job execution over reconciliation")
		Consistently(func(g Gomega) {
			var current remediationv1.RemediationApprovalRequest
			g.Expect(apiReader.Get(ctx, rarKey, &current)).To(Succeed())
			g.Expect(current.Status.Decision).To(Equal(initialStatus.Decision))
			g.Expect(current.Status.DecidedBy).To(Equal(initialStatus.DecidedBy))
			g.Expect(current.Status.DecidedVia).To(Equal(initialStatus.DecidedVia))
			g.Expect(current.Status.DecidedAt).To(Equal(initialStatus.DecidedAt))
			g.Expect(current.Status.DecisionMessage).To(Equal(initialStatus.DecisionMessage))
			g.Expect(current.Status.WorkflowOverride).To(Equal(initialStatus.WorkflowOverride))

			var executions workflowexecutionv1.WorkflowExecutionList
			g.Expect(apiReader.List(ctx, &executions, client.InNamespace(namespace))).To(Succeed())
			for _, execution := range executions.Items {
				g.Expect(execution.Spec.RemediationRequestRef.Name).NotTo(Equal(rrName), "denied RR must not create a WE")
			}
			var jobs batchv1.JobList
			g.Expect(apiReader.List(ctx, &jobs, client.InNamespace(infrastructure.ExecutionNamespace))).To(Succeed())
			for _, job := range jobs.Items {
				g.Expect(job.Labels["kubernaut.ai/remediation-request"]).NotTo(Equal(rrName), "denied RR must not create a Job")
				g.Expect(job.Annotations["kubernaut.ai/remediation-request"]).NotTo(Equal(rrName), "denied RR must not create a Job")
			}
		}, 30*time.Second, time.Second).Should(Succeed())
	})
})
