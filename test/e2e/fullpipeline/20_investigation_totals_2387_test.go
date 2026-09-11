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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FP-2387-001: autonomous OOMKill investigation surfaces server-computed
// call-level counts in AgentSession Status.Result.RootCauseAnalysis
// (BR-KA-OBSERVABILITY-001, FedRAMP AU-3, issue #2387 Gap 1).
//
// Contract under test: KA's per-RR InvestigationMetrics accumulate every LLM
// turn and every dispatched tool call across all investigation legs, and
// buildRootCauseAnalysisMap emits total_llm_turns/total_tool_calls into
// status.result.rootCauseAnalysis (present if and only if non-zero —
// mapping.go omits them at zero to preserve the empty-result → nil-RCA
// contract). Pre-fix both keys were unconditionally absent (totals hard
// zero); post-fix any completed investigation carries them.
//
// This runs the standard autonomous pipeline only as far as AIAnalysis
// Completed (KA's investigation is done by then — AgentSession Completed is
// its precondition), mirroring 01_full_remediation_lifecycle_test.go steps
// 1-5, then reads the journey's terminal artifact. Assertion shape is
// deliberately presence + >=1 rather than hardcoded exact values: the exact
// turn count is KA-loop behavior pinned by KA-side unit tests
// (UT-KA-2387-001: scripted 3-turn script yields exactly 3); what this lane
// proves is that real streamed E2E turns survive onto the CRD (the #2387
// "always 0" failure mode made the keys vanish entirely).
var _ = Describe("Investigation Totals on AgentSession Result [E2E-FP-2387-001]", Label("fp", "observability", "issue-2387"), func() {

	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)
	})

	AfterEach(func() {
		if testNamespace != "" {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
		testCancel()
	})

	It("should record total_llm_turns/total_tool_calls in status.result.rootCauseAnalysis", NodeTimeout(10*time.Minute), func(_ SpecContext) {
		By("Creating managed test namespace")
		testNamespace = fmt.Sprintf("fp-e2e-2387-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
				Labels: map[string]string{
					"kubernaut.ai/managed": "true",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Deploying memory-eater pod (will trigger OOMKill)")
		Expect(infrastructure.DeployMemoryEater(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)).ToNot(HaveOccurred())

		By("Waiting for OOMKill event")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := apiReader.List(ctx, pods, client.InNamespace(testNamespace),
				client.MatchingLabels{"app": "memory-eater"}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.LastTerminationState.Terminated != nil &&
						cs.LastTerminationState.Terminated.Reason == oomkill {
						return true
					}
					if cs.State.Terminated != nil &&
						cs.State.Terminated.Reason == oomkill {
						return true
					}
					if cs.RestartCount > 0 && cs.State.Waiting != nil &&
						cs.State.Waiting.Reason == crashloopbackoff {
						return true
					}
				}
			}
			return false
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "memory-eater should OOMKill")

		By("Waiting for RemediationRequest created by Gateway")
		var remediationRequest *remediationv1.RemediationRequest
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for i := range rrList.Items {
				rr := &rrList.Items[i]
				if rr.Spec.TargetResource.Namespace != testNamespace {
					continue
				}
				sig := strings.ToLower(rr.Spec.SignalName)
				if strings.Contains(sig, "oom") || strings.Contains(sig, "memory") || sig == backoff {
					remediationRequest = rr
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "RemediationRequest should be created by Gateway")

		By("Scaling memory-eater to 0 replicas (prevent RR storm)")
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "memory-eater", Namespace: testNamespace}, dep)).To(Succeed())
		zero := int32(0)
		dep.Spec.Replicas = &zero
		Expect(k8sClient.Update(ctx, dep)).To(Succeed())

		By("Waiting for SignalProcessing to complete")
		Eventually(func() string {
			spList := &signalprocessingv1.SignalProcessingList{}
			if err := apiReader.List(ctx, spList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, sp := range spList.Items {
				if sp.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					return string(sp.Status.Phase)
				}
			}
			return ""
		}, timeout, interval).Should(Equal("Completed"), "SignalProcessing should reach Completed phase") // BR-KA-OBSERVABILITY-001: pipeline must advance to the KA investigation

		By("Waiting for AIAnalysis to complete (KA investigation done)")
		var aaName string
		Eventually(func() string {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					aaName = aa.Name
					return aa.Status.Phase
				}
			}
			return ""
		}, timeout, interval).Should(Equal("Completed"), "AIAnalysis should reach Completed phase") // BR-KA-OBSERVABILITY-001: KA investigation done, AgentSession result written
		Expect(aaName).NotTo(BeEmpty())

		By("Waiting for the AgentSession to reach Completed with a result")
		var sessionName string
		Eventually(func() bool {
			asList := &agentsessionv1.AgentSessionList{}
			if err := apiReader.List(ctx, asList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for i := range asList.Items {
				as := &asList.Items[i]
				if as.Spec.IncidentID != aaName {
					continue
				}
				if as.Status.Phase == agentsessionv1.AgentSessionPhaseCompleted && as.Status.Result != nil {
					sessionName = as.Name
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(),
			"AgentSession for AIAnalysis %s must reach Completed with a result", aaName)

		By("Verifying server-computed call-level counts in status.result.rootCauseAnalysis")
		as := &agentsessionv1.AgentSession{}
		Expect(apiReader.Get(ctx, client.ObjectKey{Name: sessionName, Namespace: namespace}, as)).To(Succeed())
		Expect(as.Status.Result.RootCauseAnalysis).NotTo(BeNil(),
			"E2E-FP-2387-001: completed investigation must carry a structured RCA")
		var rca map[string]any
		Expect(json.Unmarshal(as.Status.Result.RootCauseAnalysis.Raw, &rca)).To(Succeed())

		turns, hasTurns := rca["total_llm_turns"]
		Expect(hasTurns).To(BeTrue(),
			"E2E-FP-2387-001: total_llm_turns key must be present — pre-#2387 it was unconditionally absent (totals hard zero)")
		Expect(turns).To(BeNumerically(">=", 1),
			"E2E-FP-2387-001: any completed E2E investigation records at least one streamed LLM turn")

		tools, hasTools := rca["total_tool_calls"]
		Expect(hasTools).To(BeTrue(),
			"E2E-FP-2387-001: total_tool_calls key must be present — the autonomous oomkill investigation dispatches diagnostic and discovery tool calls")
		Expect(tools).To(BeNumerically(">=", 1),
			"E2E-FP-2387-001: submit_result sentinels are consumed, never counted — remaining dispatches must be >= 1")
		GinkgoWriter.Printf("  AgentSession %s result counts: llm_turns=%v tool_calls=%v\n", sessionName, turns, tools)
	})
})
