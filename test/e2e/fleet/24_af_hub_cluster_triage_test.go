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
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FLEET-2394-001 [BR-FLEET-054]: AF's real kubernaut_remediate path must
// retain the explicit hub Gateway registration ID during severity triage. The
// target exists only on hub, while Prometheus has matching hub-warning and
// remote-critical alert labels; the resulting RR must be hub-scoped and warning.
var _ = Describe("E2E-FLEET-2394-001 [BR-FLEET-054]: AF hub Gateway severity attribution", Label("fleet", "af", "gateway", "severity", "issue-2394"), func() {
	const (
		prometheusURL = "http://localhost:9190"
		targetName    = "af-hub-triage-target"
		targetNS      = "fleet-af-hub-triage"
	)

	It("should select the warning hub alert over a critical remote-cluster collision", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		By("Verifying AF is reachable")
		healthResp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		Expect(err).NotTo(HaveOccurred())
		Expect(healthResp).NotTo(BeNil())
		if healthResp == nil {
			return
		}
		defer func() { _ = healthResp.Body.Close() }()
		Expect(healthResp.StatusCode).To(Equal(http.StatusOK))

		By("Waiting for both cluster-collision alerts to be firing in Prometheus")
		promCtx := context.Background()
		Eventually(func() error {
			return infrastructure.WaitForPrometheusRuleState(promCtx, prometheusURL,
				"AFHubClusterTriage2394", infrastructure.RuleStateFiring, 5*time.Second)
		}, 60*time.Second, 2*time.Second).Should(Succeed())
		Eventually(func() error {
			return infrastructure.WaitForPrometheusRuleState(promCtx, prometheusURL,
				"AFRemoteClusterTriageCollision2394", infrastructure.RuleStateFiring, 5*time.Second)
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		By("Creating the managed target only on the hub cluster")
		hubTarget := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: targetNS}}
		Expect(infrastructure.DeployMemoryEaterNamed(ctx, targetName, targetNS, kubeconfigPath,
			"128Mi", "32Mi", GinkgoWriter)).To(Succeed(), "hub target deployment must succeed")
		DeferCleanup(func() {
			err := k8sClient.Delete(context.Background(), hubTarget)
			Expect(err == nil || client.IgnoreNotFound(err) == nil).To(BeTrue(), "delete hub target: %v", err)
		})

		By("Confirming the hub-only target is managed before invoking AF")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: targetName, Namespace: targetNS}, hubTarget)).To(Succeed())
			g.Expect(hubTarget.Labels).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("Calling AF's real kubernaut_remediate tool with cluster_id=hub")
		var createdRR *remediationv1.RemediationRequest
		attempt := 0
		Eventually(func(g Gomega) {
			attempt++
			turnID := fmt.Sprintf("fleet-2394-1-%d", attempt)
			body := afA2ATasksSend(turnID, "af-hub-cluster-triage-2394: remediate the hub deployment")
			resp, invokeErr := afA2AInvokeWithTimeout(body, 60*time.Second)
			if invokeErr != nil {
				g.Expect(invokeErr).NotTo(HaveOccurred())
				return
			}
			g.Expect(resp).NotTo(BeNil())
			if resp == nil {
				return
			}
			defer func() { _ = resp.Body.Close() }()
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "A2A message/send should return 200")
			rpc, parseErr := afParseRPC(resp)
			g.Expect(parseErr).NotTo(HaveOccurred())
			g.Expect(rpc.Error).To(BeNil(), "A2A should not return a JSON-RPC error")

			rrList := &remediationv1.RemediationRequestList{}
			g.Expect(k8sClient.List(ctx, rrList, client.InNamespace(namespace))).To(Succeed())
			for i := range rrList.Items {
				rr := &rrList.Items[i]
				if rr.Spec.ClusterID == "hub" && rr.Spec.TargetResource.Namespace == targetNS && rr.Spec.TargetResource.Name == targetName {
					createdRR = rr.DeepCopy()
					break
				}
			}
			g.Expect(createdRR).NotTo(BeNil(),
				"AF must create a hub-scoped RR after the hub Gateway registration becomes visible to FMC (attempt %d)", attempt)
		}, fmcSyncTimeout, 2*time.Second).Should(Succeed())

		By("Verifying severity came from the hub alert, not the more severe remote collision")
		Expect(createdRR.Spec.ClusterID).To(Equal("hub"))
		Expect(createdRR.Spec.Severity).To(Equal("warning"),
			"the critical alert belongs to remote-cluster and must not influence hub severity triage")
		GinkgoWriter.Printf("  E2E-FLEET-2394-001: RR %s has cluster_id=%q severity=%q\n",
			createdRR.Name, createdRR.Spec.ClusterID, createdRR.Spec.Severity)
	})
})
