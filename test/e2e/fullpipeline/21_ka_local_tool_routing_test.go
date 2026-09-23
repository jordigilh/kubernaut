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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

const fpLocalToolCallKeyword = "ka-tool-e2e-test"

// E2E-FP-1729-001 [BR-KA-212]: the local-mode KA tool path remains distinct
// from Fleet mode. The default Mock LLM scenario chooses kubectl_get_by_name
// when no fleet-overlay tools are advertised and echoes live resource evidence.
var _ = Describe("Local KA tool routing [issue #1729]", Label("fullpipeline", "ka", "local-tool"), func() {
	It("E2E-FP-1729-001: an unattributed local alert uses kubectl_get_by_name", NodeTimeout(timeout), func() {
		targetName := fmt.Sprintf("ka-tool-e2e-target-%08x", uint32(time.Now().UnixNano()))

		By("Creating a managed local Deployment with unique live evidence")
		Expect(infrastructure.DeployMemoryEaterNamed(ctx, targetName, namespace, kubeconfigPath,
			"111Mi", "20Mi", GinkgoWriter)).To(Succeed())
		deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace}}
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), deployment))).To(Succeed(),
				"failed to clean up local KA target deployment")
		})
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)).To(Succeed())
			g.Expect(deployment.Status.AvailableReplicas).To(BeNumerically(">=", 1))
		}, 2*time.Minute, 2*time.Second).Should(Succeed(), "local KA target must be readable")

		By("Injecting a clusterless alert through the real local Alertmanager webhook")
		alertManagerURL := fmt.Sprintf("http://localhost:%d", infrastructure.AlertManagerHostPort)
		Expect(infrastructure.InjectAlerts(alertManagerURL, []infrastructure.TestAlert{{
			Name: fpLocalToolCallKeyword,
			Labels: map[string]string{
				"severity":   "high",
				"namespace":  namespace,
				"deployment": targetName,
			},
			Annotations: map[string]string{"summary": "Local KA tool routing E2E"},
			Status:      "firing",
			StartsAt:    time.Now(),
		}})).To(Succeed())

		By("Waiting for Gateway to create an unattributed local RR")
		var localRR *remediationv1.RemediationRequest
		Eventually(func(g Gomega) {
			rrList := &remediationv1.RemediationRequestList{}
			g.Expect(apiReader.List(ctx, rrList, client.InNamespace(namespace))).To(Succeed())
			localRR = nil
			for i := range rrList.Items {
				candidate := &rrList.Items[i]
				if candidate.Spec.SignalName == fpLocalToolCallKeyword &&
					candidate.Spec.TargetResource.Name == targetName {
					localRR = candidate.DeepCopy()
					break
				}
			}
			g.Expect(localRR).NotTo(BeNil(), "local Alertmanager signal should create an RR")
		}, timeout, interval).Should(Succeed())
		Expect(localRR.Spec.ClusterID).To(BeEmpty(), "local-mode RR must not invent a fleet cluster ID")

		By("Waiting for KA's local investigation to complete")
		var localAA *aianalysisv1.AIAnalysis
		Eventually(func(g Gomega) {
			aaList := &aianalysisv1.AIAnalysisList{}
			g.Expect(apiReader.List(ctx, aaList, client.InNamespace(namespace))).To(Succeed())
			localAA = nil
			for i := range aaList.Items {
				candidate := &aaList.Items[i]
				if candidate.Spec.RemediationRequestRef.Name == localRR.Name {
					localAA = candidate.DeepCopy()
					break
				}
			}
			g.Expect(localAA).NotTo(BeNil(), "RO should create an AIAnalysis owned by the local RR")
			if localAA != nil {
				g.Expect(localAA.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
			}
		}, timeout, interval).Should(Succeed())

		By("Verifying KA used the local tool and returned evidence from the live Deployment")
		Expect(localAA.Status.GetRCAResult().RootCauseAnalysis).NotTo(BeNil())
		rootCause := localAA.Status.RCAResult.RootCauseAnalysis.Summary
		Expect(rootCause).To(ContainSubstring("kubectl_get_by_name"),
			"local mode must use the local K8s registry tool, not a Fleet Gateway tool")
		Expect(rootCause).To(ContainSubstring("111Mi"),
			"the RCA must contain the target's live memory-limit evidence")
	})
})
