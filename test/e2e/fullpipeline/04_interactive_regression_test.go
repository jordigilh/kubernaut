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
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-KA-HARM-001: Autonomous Regression — standard RR completes without interactive session artifacts.
//
// BR: BR-INTERACTIVE-001 (Do No Harm)
// Validates: Standard autonomous flow produces NO InteractiveSession, NO K8s Lease.
// Context: Full pipeline E2E with all services deployed.
var _ = Describe("CP-5 HARM-001: Autonomous regression — no interactive artifacts", Label("e2e", "fullpipeline", "interactive", "harm"), Ordered, func() {

	var testNamespace string

	BeforeAll(func() {
		Expect(ctx).NotTo(BeNil(), "suite ctx must be initialized before tests run")
	})

	AfterAll(func() {
		if testNamespace != "" {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	It("should complete standard autonomous remediation without interactive session or Lease [E2E-KA-HARM-001]", func() {
		By("Step 1: Creating managed test namespace for OOMKill trigger")
		testNamespace = fmt.Sprintf("fp-e2e-harm001-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
				Labels: map[string]string{
					"kubernaut.ai/managed": "true",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Step 2: Deploying memory-eater pod (triggers OOMKill → Gateway → pipeline)")
		err := infrastructure.DeployMemoryEater(ctx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to deploy memory-eater")

		By("Step 2b: Waiting for OOMKill event...")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if listErr := apiReader.List(ctx, pods, client.InNamespace(testNamespace),
				client.MatchingLabels{"app": "memory-eater"}); listErr != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.LastTerminationState.Terminated != nil &&
						cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
						return true
					}
					if cs.State.Terminated != nil &&
						cs.State.Terminated.Reason == "OOMKilled" {
						return true
					}
					if cs.RestartCount > 0 && cs.State.Waiting != nil &&
						cs.State.Waiting.Reason == "CrashLoopBackOff" {
						return true
					}
				}
			}
			return false
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "memory-eater should OOMKill")

		By("Step 3: Waiting for RemediationRequest created by Gateway")
		var remediationRequest *remediationv1.RemediationRequest
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			if listErr := apiReader.List(ctx, rrList, client.InNamespace(namespace)); listErr != nil {
				return false
			}
			for i := range rrList.Items {
				rr := &rrList.Items[i]
				if rr.Spec.TargetResource.Namespace == testNamespace {
					remediationRequest = rr
					GinkgoWriter.Printf("  ✅ Found RR: %s\n", rr.Name)
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "Gateway should create RR for OOMKill")

		By("Step 4: Waiting for AIAnalysis to reach Completed phase (autonomous)")
		var aaName string
		Eventually(func() string {
			aaList := &aianalysisv1.AIAnalysisList{}
			if listErr := apiReader.List(ctx, aaList, client.InNamespace(namespace)); listErr != nil {
				return ""
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					aaName = aa.Name
					GinkgoWriter.Printf("  AA %s phase: %s\n", aa.Name, aa.Status.Phase)
					return aa.Status.Phase
				}
			}
			return ""
		}, timeout, interval).Should(Equal("Completed"),
			"AIAnalysis should reach Completed phase (autonomous)")

		By("Step 5: Asserting InteractiveSession is nil (no interactive takeover)")
		aa := &aianalysisv1.AIAnalysis{}
		Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
		Expect(aa.Status.InteractiveSession).To(BeNil(),
			"HARM-001: autonomous RR must NOT have InteractiveSession populated")

		By("Step 6: Asserting no K8s Lease for this RR")
		clientset, err := kubernetes.NewForConfig(mustGetKubeConfig())
		Expect(err).NotTo(HaveOccurred())

		leases, err := clientset.CoordinationV1().Leases(namespace).List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())

		for _, lease := range leases.Items {
			if strings.HasPrefix(lease.Name, "kubernaut-interactive-") {
				Expect(lease.Name).NotTo(ContainSubstring(remediationRequest.Name),
					"HARM-001: no interactive Lease should exist for autonomous RR %s", remediationRequest.Name)
			}
		}

		By("Step 7: Verifying MCP endpoint is responsive (not degraded)")
		mcpURL := "https://localhost:8088/api/v1/mcp"
		Eventually(func(g Gomega) {
			req, httpErr := http.NewRequestWithContext(ctx, "GET", mcpURL, nil)
			g.Expect(httpErr).NotTo(HaveOccurred())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e2eAuthToken))

			resp, httpErr := http.DefaultClient.Do(req)
			g.Expect(httpErr).NotTo(HaveOccurred())
			defer resp.Body.Close()
			g.Expect(resp.StatusCode).To(Or(
				Equal(http.StatusMethodNotAllowed),
				Equal(http.StatusOK),
				Equal(http.StatusUnauthorized),
			), "MCP endpoint should be responsive (non-5xx)")
		}, 30*time.Second, 3*time.Second).Should(Succeed(), "KA MCP endpoint should become responsive")

		GinkgoWriter.Println("✅ HARM-001: Autonomous remediation completed with zero interactive artifacts")
	})
})

func mustGetKubeConfig() *rest.Config {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return cfg
}
