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
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// BR-KA-017 / DD-KA-2496; AC-6, SI-10, AU-2/AU-3, SOC2 CC7.2.
// Paired IT-KA-2498-002 counts external invocations directly. This deployed
// journey proves the trusted pre-dispatch denial through durable error records,
// advertised phase allow-lists and a positive, real catalog -> Job -> patch path.
var _ = Describe("Trusted phase isolation [BR-KA-017]", Label("e2e", "fullpipeline", "phase-2498"), func() {
	It("E2E-FP-2498-001: optional RCA context is unused and forced discovery-phase context/Kubernetes calls are denied", func() {
		targetNS := fmt.Sprintf("phase-2498-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNS, Labels: map[string]string{
			"kubernaut.ai/managed": "true", "kubernaut.ai/environment": "staging",
		}}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), ns)).To(Succeed()) })
		Expect(infrastructure.DeployMemoryEaterWithLimits(ctx, targetNS, kubeconfigPath, "48Mi", "18Mi", GinkgoWriter)).To(Succeed())
		fpWaitForPodCrashInNS("memory-eater", targetNS, 2*time.Minute)

		By("Running real autonomous controllers with an adversarial external-provider response")
		rr := fpHistoryRR2496(targetNS, "phaseisolation2498")
		aa := fpHistoryAA2496(rr.Name)
		Expect(aa.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
		selected := aa.Status.GetRCAResult().SelectedWorkflow
		Expect(selected).NotTo(BeNil())
		Expect(selected.WorkflowName).To(Equal("oomkill-increase-memory-v1"))
		Expect(selected.RepeatJustificationRequired).To(HaveValue(BeFalse()))

		By("Verifying denial before dispatch without breaking permitted catalog/submission and Job execution")
		fpAssertPhaseIsolation2498(rr.Name)
		fpWaitForWEComplete(rr.Name, 4*time.Minute)
		dep := &appsv1.Deployment{}
		Expect(apiReader.Get(ctx, client.ObjectKey{Namespace: targetNS, Name: "memory-eater"}, dep)).To(Succeed())
		Expect(dep.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).To(Equal("512Mi"), "the permitted immutable Job must really patch the workload")
		fpAssertSelectionAudit2496(rr.Name, selected)
	})
})

func fpAssertPhaseIsolation2498(rrName string) {
	GinkgoHelper()
	forbidden := []string{"get_namespaced_resource_context", "get_cluster_resource_context", "kubectl_get_by_name"}
	allowed := []string{"list_available_actions", "list_workflows", "get_workflow", "todo_write", "submit_result_with_workflow", "submit_result_no_workflow"}
	Eventually(func(g Gomega) {
		events, err := infrastructure.QueryAuditSnapshot(ctx, dataStorageClient, ogenclient.QueryAuditEventsParams{
			CorrelationID: ogenclient.NewOptString(rrName),
		})
		g.Expect(err).NotTo(HaveOccurred())
		var rca, discovery bool
		denied := make([]string, 0, len(forbidden))
		catalog := make(map[string]bool, 3)
		for _, event := range events {
			switch event.EventType {
			case "aiagent.llm.request":
				request, ok := event.EventData.GetLLMRequestPayload()
				g.Expect(ok).To(BeTrue())
				switch request.Phase.Value {
				case "rca":
					g.Expect(request.ToolsetsEnabled).To(ContainElements("get_namespaced_resource_context", "get_cluster_resource_context", "submit_result"))
					rca = true
				case "workflow_discovery":
					g.Expect(request.ToolsetsEnabled).To(ContainElements("list_available_actions", "list_workflows", "get_workflow", "submit_result_with_workflow", "submit_result_no_workflow"))
					for _, name := range request.ToolsetsEnabled {
						g.Expect(name).To(BeElementOf(allowed), "discovery must not advertise Kubernetes/context-history access")
					}
					discovery = true
				}
			case "aiagent.llm.tool_call":
				call, ok := event.EventData.GetLLMToolCallPayload()
				g.Expect(ok).To(BeTrue())
				g.Expect(event.EventID.IsSet()).To(BeTrue())
				g.Expect(event.ActorID.Value).NotTo(BeEmpty())
				g.Expect(event.EventAction).NotTo(BeEmpty())
				g.Expect(call.Phase.IsSet()).To(BeTrue())
				switch call.ToolName {
				case "get_namespaced_resource_context", "get_cluster_resource_context", "kubectl_get_by_name":
					g.Expect(call.Phase.Value).To(Equal("workflow_discovery"), "optional RCA tools must never be called during this fixture's RCA")
					g.Expect(string(event.EventOutcome)).To(Equal("failure"))
					g.Expect(call.ErrorDetails.IsSet()).To(BeTrue())
					g.Expect(call.ErrorDetails.Value.Code).To(Equal("ERR_TOOL_PHASE_DENIED"))
					g.Expect(call.ErrorDetails.Value.RetryPossible).To(BeFalse())
					g.Expect(string(call.ToolResult)).To(ContainSubstring("not allowed in phase"), "a registry/resource error is not pre-dispatch denial")
					arguments, marshalErr := json.Marshal(call.ToolArguments.Value)
					g.Expect(marshalErr).NotTo(HaveOccurred())
					g.Expect(string(arguments)).To(ContainSubstring(`"phase":"rca"`), "model-authored phase cannot authorize a registered tool")
					denied = append(denied, call.ToolName)
				case "list_available_actions", "list_workflows", "get_workflow":
					g.Expect(call.Phase.Value).To(Equal("workflow_discovery"))
					g.Expect(string(event.EventOutcome)).To(Equal("success"))
					catalog[call.ToolName] = true
				}
			}
		}
		g.Expect(rca && discovery).To(BeTrue(), "both trusted phase tool definitions must be durable")
		g.Expect(denied).To(ConsistOf(forbidden))
		g.Expect(catalog).To(HaveLen(3), "all permitted discovery stages must still reach the real catalog")
	}, time.Minute, time.Second).Should(Succeed())
}
