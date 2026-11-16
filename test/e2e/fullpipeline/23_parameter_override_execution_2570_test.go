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
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// BR-ORCH-025/026, BR-KA-191: the approved partial override reaches the actual Job.
var _ = Describe("Approved parameter execution [E2E-2570-001]", Label("e2e", "fullpipeline", "apifrontend", "issue-2570"), func() {
	It("executes the changed ConfigMap name while retaining the original key and value", func() {
		const alternate = "reviewed-app-config"
		const original = "crashloop-app-config"
		const app = "crashloop-app"
		targetNS := fmt.Sprintf("fp-2570-exec-%d", time.Now().UnixNano())
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNS, Labels: map[string]string{
			"kubernaut.ai/managed": "true", "kubernaut.ai/environment": "production",
		}}})).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNS}}))).To(Succeed())
		})
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: alternate, Namespace: targetNS}, Data: map[string]string{"APP_MODE": "broken"}})).To(Succeed())
		deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app, Namespace: targetNS}, Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1), Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": app}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": app}}, Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: app, Image: "busybox:1.36", Command: []string{"sh", "-c", `test "$(cat /config/APP_MODE)" = healthy || exit 1; sleep 3600`}, VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: "/config"}}}},
				Volumes:    []corev1.Volume{{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: alternate}}}}},
			}},
		}}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
		var podName string
		Eventually(func(g Gomega) {
			var pods corev1.PodList
			g.Expect(apiReader.List(ctx, &pods, client.InNamespace(targetNS), client.MatchingLabels{"app": app})).To(Succeed())
			g.Expect(pods.Items).NotTo(BeEmpty())
			podName = pods.Items[0].Name
		}, timeout, interval).Should(Succeed())
		rrName := fpPostSignalToGateway("CrashLoopBackOff", podName, targetNS)
		var weName string
		DeferCleanup(func() {
			var executions workflowv1.WorkflowExecutionList
			Expect(apiReader.List(ctx, &executions, client.InNamespace(namespace))).To(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &remediationv1.RemediationRequest{ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: namespace}}, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
			for _, execution := range executions.Items {
				if execution.Spec.RemediationRequestRef.Name != rrName {
					continue
				}
				var jobs batchv1.JobList
				Expect(apiReader.List(ctx, &jobs, client.InNamespace(infrastructure.ExecutionNamespace), client.MatchingLabels{"kubernaut.ai/workflow-execution": execution.Name})).To(Succeed())
				for i := range jobs.Items {
					Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &jobs.Items[i], client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
				}
			}
		})

		By("Waiting for the frozen production approval and checking the original parameters")
		rarKey := client.ObjectKey{Namespace: namespace, Name: "rar-" + rrName}
		var rar remediationv1.RemediationApprovalRequest
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, rarKey, &rar)).To(Succeed())
			g.Expect(rar.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
			g.Expect(rar.Spec.RemediationRequestRef.Name).To(Equal(rrName))
			g.Expect(rar.Spec.Workflow).NotTo(BeNil())
			g.Expect(rar.Spec.Workflow.ExecutionEngine).To(Equal("job"))
			g.Expect(rar.Spec.Workflow.WorkflowID).To(Equal(workflowUUIDs["crashloop-config-fix-v1:production"]))
			parameterEngineValues, conversionErr := sharedtypes.TypedParameterMapToEngineValues(rar.Spec.Parameters)
			g.Expect(conversionErr).NotTo(HaveOccurred())
			g.Expect(parameterEngineValues).To(HaveKeyWithValue("CONFIGMAP_NAME", original))
			g.Expect(parameterEngineValues).To(HaveKeyWithValue("CONFIGMAP_KEY", "APP_MODE"))
			g.Expect(parameterEngineValues).To(HaveKeyWithValue("CONFIGMAP_VALUE", "healthy"))
		}, timeout, interval).Should(Succeed())
		originalParameters, err := sharedtypes.TypedParameterMapToEngineValues(rar.Spec.Parameters)
		Expect(err).NotTo(HaveOccurred())

		By("Approving only the ConfigMap name through authenticated AF MCP")
		token := getAFToken()
		Expect(token).NotTo(BeEmpty())
		session, err := fpInitMCPSessionExplicit(afHTTPClient, afBaseURL, token)
		Expect(err).NotTo(HaveOccurred())
		body := fpBuildJSONRPC("e2e-2570-001", "tools/call", map[string]interface{}{"name": "kubernaut_approve", "arguments": map[string]interface{}{
			"rar_name": rarKey.Name, "decision": "Approved", "workflow_override": map[string]interface{}{"parameters": map[string]string{"CONFIGMAP_NAME": alternate}},
		}})
		raw, code, err := fpMCPPOST(afHTTPClient, afBaseURL, token, session, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(http.StatusOK), "%s", raw)
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
			Result *struct {
				IsError bool `json:"isError"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		}
		Expect(json.Unmarshal([]byte(payload), &response)).To(Succeed(), "%s", raw)
		Expect(response.Error).To(BeEmpty(), "%s", raw)
		Expect(response.Result).NotTo(BeNil(), "%s", raw)
		Expect(response.Result.IsError).To(BeFalse(), "%s", raw)
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, rarKey, &rar)).To(Succeed())
			g.Expect(rar.Status.Decision).To(Equal(remediationv1.ApprovalDecisionApproved))
			g.Expect(rar.Status.DecidedBy).NotTo(BeEmpty())
			g.Expect(rar.Status.DecidedVia).NotTo(BeEmpty())
			g.Expect(rar.Status.WorkflowOverride).NotTo(BeNil())
			overrideParameters, conversionErr := sharedtypes.TypedParameterMapToEngineValues(rar.Status.WorkflowOverride.Parameters)
			g.Expect(conversionErr).NotTo(HaveOccurred())
			g.Expect(overrideParameters).To(Equal(map[string]string{"CONFIGMAP_NAME": alternate}))
			original, conversionErr := sharedtypes.TypedParameterMapToEngineValues(rar.Spec.Parameters)
			g.Expect(conversionErr).NotTo(HaveOccurred())
			g.Expect(original).To(Equal(originalParameters))
		}, timeout, interval).Should(Succeed())
		Expect(rar.Status.DecidedBy).To(Equal(fpA2AUsername()))

		// E2E-AUD-2570-008, BR-ORCH-025/026 and BR-AUDIT-005: reconstruct
		// this approval across the deployed AF, admission webhook, and RO.
		By("Reconstructing the correlated approval audit trail")
		effectiveParameters := make(map[string]string, len(originalParameters))
		for name, value := range originalParameters {
			effectiveParameters[name] = value
		}
		effectiveParameters["CONFIGMAP_NAME"] = alternate
		changedParameters := map[string]string{"CONFIGMAP_NAME": alternate}
		workflowID := rar.Spec.Workflow.WorkflowID
		decidedVia := rar.Status.DecidedVia
		Eventually(func(g Gomega) {
			for _, target := range []struct{ category, eventType string }{
				{"apifrontend", "apifrontend.tool.executed"},
				{"approval", "webhook.remediationapprovalrequest.decided"},
				{"orchestration", "orchestrator.approval.approved"},
			} {
				events, queryErr := infrastructure.QueryAuditSnapshot(ctx, dataStorageClient, ogenclient.QueryAuditEventsParams{
					EventCategory: ogenclient.NewOptString(target.category),
					EventType:     ogenclient.NewOptString(target.eventType),
					CorrelationID: ogenclient.NewOptString(rrName),
				})
				g.Expect(queryErr).NotTo(HaveOccurred(), target.eventType)
				found := false
				for _, event := range events {
					g.Expect(event.CorrelationID).To(Equal(rrName))
					g.Expect(event.EventType).To(Equal(target.eventType))
					switch target.category {
					case "apifrontend":
						payload, ok := event.EventData.GetApifrontendToolExecutedPayload()
						g.Expect(ok).To(BeTrue())
						if payload.ToolName != "kubernaut_approve" || payload.ApprovalRequest.Value != rarKey.Name {
							continue
						}
						original, originalSet := payload.OriginalParameters.Get()
						changed, changedSet := payload.OverrideParameters.Get()
						effective, effectiveSet := payload.EffectiveParameters.Get()
						g.Expect(originalSet && changedSet && effectiveSet).To(BeTrue())
						g.Expect(map[string]ogenclient.WorkflowParameterValue(original)).To(Equal(auditStringParameterValues(originalParameters)))
						g.Expect(map[string]ogenclient.WorkflowParameterValue(changed)).To(Equal(auditStringParameterValues(changedParameters)))
						g.Expect(map[string]ogenclient.WorkflowParameterValue(effective)).To(Equal(auditStringParameterValues(effectiveParameters)))
						g.Expect(payload.WorkflowID.Value).To(Equal(workflowID))
						g.Expect(payload.ApprovalDecision.Value).To(Equal("Approved"))
						g.Expect(event.ActorID.Value).To(Equal(fpA2AUsername()))
					case "approval":
						payload, ok := event.EventData.GetRemediationApprovalAuditPayload()
						g.Expect(ok).To(BeTrue())
						if payload.RequestName != rarKey.Name {
							continue
						}
						original, originalSet := payload.OriginalParameters.Get()
						changed, changedSet := payload.OverrideParameters.Get()
						effective, effectiveSet := payload.EffectiveParameters.Get()
						g.Expect(originalSet && changedSet && effectiveSet).To(BeTrue())
						g.Expect(map[string]ogenclient.WorkflowParameterValue(original)).To(Equal(auditStringParameterValues(originalParameters)))
						g.Expect(map[string]ogenclient.WorkflowParameterValue(changed)).To(Equal(auditStringParameterValues(changedParameters)))
						g.Expect(map[string]ogenclient.WorkflowParameterValue(effective)).To(Equal(auditStringParameterValues(effectiveParameters)))
						g.Expect(payload.WorkflowID.Value).To(Equal(workflowID))
						g.Expect(string(payload.Decision)).To(Equal("Approved"))
						g.Expect(payload.DelegatedUser.Value).To(Equal(fpA2AUsername()))
						g.Expect(payload.DelegatedVia.Value).To(Equal(decidedVia))
						g.Expect(event.EventAction).To(Equal("approval_decided"))
						g.Expect(event.ActorID.Value).To(Equal(decidedVia))
					case "orchestration":
						payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
						g.Expect(ok).To(BeTrue())
						if payload.RarName.Value != rarKey.Name {
							continue
						}
						original, originalSet := payload.OriginalParameters.Get()
						changed, changedSet := payload.OverrideParameters.Get()
						effective, effectiveSet := payload.EffectiveParameters.Get()
						g.Expect(originalSet && changedSet && effectiveSet).To(BeTrue())
						g.Expect(map[string]ogenclient.WorkflowParameterValue(original)).To(Equal(auditStringParameterValues(originalParameters)))
						g.Expect(map[string]ogenclient.WorkflowParameterValue(changed)).To(Equal(auditStringParameterValues(changedParameters)))
						g.Expect(map[string]ogenclient.WorkflowParameterValue(effective)).To(Equal(auditStringParameterValues(effectiveParameters)))
						g.Expect(payload.RrName).To(Equal(rrName))
						g.Expect(payload.WorkflowID.Value).To(Equal(workflowID))
						g.Expect(event.ActorID.Value).To(Equal(fpA2AUsername()))
					}
					g.Expect(string(event.EventOutcome)).To(Equal("success"))
					found = true
				}
				g.Expect(found).To(BeTrue(), "missing correlated %s audit event", target.eventType)
			}
		}, 120*time.Second, 3*time.Second).Should(Succeed())

		By("Correlating the RR to its WE, Job and actual Job Pod")
		Eventually(func(g Gomega) {
			var executions workflowv1.WorkflowExecutionList
			g.Expect(apiReader.List(ctx, &executions, client.InNamespace(namespace))).To(Succeed())
			var matches []workflowv1.WorkflowExecution
			for _, we := range executions.Items {
				if we.Spec.RemediationRequestRef.Name == rrName {
					matches = append(matches, we)
				}
			}
			g.Expect(matches).To(HaveLen(1))
			weName = matches[0].Name
			g.Expect(matches[0].Spec.WorkflowRef.WorkflowID).To(Equal(rar.Spec.Workflow.WorkflowID))
			parameterEngineValues, conversionErr := sharedtypes.TypedParameterMapToEngineValues(matches[0].Spec.Parameters)
			g.Expect(conversionErr).NotTo(HaveOccurred())
			g.Expect(parameterEngineValues).To(HaveKeyWithValue("CONFIGMAP_NAME", alternate))
			g.Expect(parameterEngineValues).To(HaveKeyWithValue("CONFIGMAP_KEY", "APP_MODE"))
			g.Expect(parameterEngineValues).To(HaveKeyWithValue("CONFIGMAP_VALUE", "healthy"))
		}, timeout, interval).Should(Succeed())
		var job batchv1.Job
		Eventually(func(g Gomega) {
			var jobs batchv1.JobList
			g.Expect(apiReader.List(ctx, &jobs, client.InNamespace(infrastructure.ExecutionNamespace), client.MatchingLabels{"kubernaut.ai/workflow-execution": weName})).To(Succeed())
			g.Expect(jobs.Items).To(HaveLen(1))
			job = jobs.Items[0]
			g.Expect(job.Status.Succeeded).To(BeNumerically(">", 0))
		}, timeout, interval).Should(Succeed())
		var jobPod corev1.Pod
		Eventually(func(g Gomega) {
			var pods corev1.PodList
			g.Expect(apiReader.List(ctx, &pods, client.InNamespace(infrastructure.ExecutionNamespace), client.MatchingLabels{"job-name": job.Name})).To(Succeed())
			g.Expect(pods.Items).To(HaveLen(1))
			jobPod = pods.Items[0]
			g.Expect(jobPod.OwnerReferences).To(ContainElement(HaveField("UID", job.UID)))
			g.Expect(jobPod.Status.Phase).To(Equal(corev1.PodSucceeded))
		}, timeout, interval).Should(Succeed())
		clientset, err := kubernetes.NewForConfig(mustGetKubeConfig())
		Expect(err).NotTo(HaveOccurred())
		logs, err := clientset.CoreV1().Pods(infrastructure.ExecutionNamespace).GetLogs(jobPod.Name, &corev1.PodLogOptions{Container: "workflow"}).DoRaw(ctx)
		Expect(err).NotTo(HaveOccurred())
		lines := strings.Split(string(logs), "\n")
		Expect(lines).To(ContainElement("  ConfigMap:       " + alternate))
		Expect(lines).To(ContainElement("  ConfigMap Key:   APP_MODE"))
		Expect(lines).To(ContainElement("  ConfigMap Value: healthy"))
		var cm corev1.ConfigMap
		Expect(apiReader.Get(ctx, client.ObjectKey{Namespace: targetNS, Name: alternate}, &cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue("APP_MODE", "healthy"))
		Eventually(func(g Gomega) {
			var we workflowv1.WorkflowExecution
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: weName}, &we)).To(Succeed())
			g.Expect(we.Status.Phase).To(Equal("Completed"))
			var rr remediationv1.RemediationRequest
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: rrName}, &rr)).To(Succeed())
			g.Expect(rr.Spec.TargetResource.Namespace).To(Equal(targetNS))
			g.Expect(string(rr.Status.OverallPhase)).To(Equal("Completed"))
		}, timeout, interval).Should(Succeed())
	})
})

func auditStringParameterValues(values map[string]string) map[string]ogenclient.WorkflowParameterValue {
	result := make(map[string]ogenclient.WorkflowParameterValue, len(values))
	for name, value := range values {
		result[name] = ogenclient.NewWorkflowParameterStringWorkflowParameterValue(ogenclient.WorkflowParameterString{
			Kind:        ogenclient.WorkflowParameterStringKindString,
			StringValue: value,
		})
	}
	return result
}
