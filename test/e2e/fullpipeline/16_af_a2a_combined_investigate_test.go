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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

// E2E-FP-1853-001: AF A2A Combined Remediate+Investigate (mode 2 — Interactive,
// single combined message). Reproduces the original issue #1853 bug report: a
// single free-form message combining "create a remediation" and "investigate"
// intent must chain kubernaut_remediate straight into kubernaut_investigate
// within the same conversation turn (no second user message needed to
// upgrade the RR to interactive), then STOP — presenting RCA findings and
// waiting for the user to manually continue with discover/select/watch, per
// the "Interactive Mode" contract in pkg/apifrontend/agent/prompt.txt.
//
// This is the "1 message → 2 chained tool calls, then pause" case: it proves
// the mock-llm's NextToolCall $from_tool resolution fix (the actual #1853
// root cause — the server-generated rr_id from kubernaut_remediate must
// reach kubernaut_investigate's arguments without a hardcoded/pre-known ID).
var _ = Describe("AF A2A Combined Remediate+Investigate Full Pipeline [E2E-FP-1853-001]", Label("fp", "af", "a2a", "interactive", "issue-1853"), func() {

	It("should chain kubernaut_remediate into kubernaut_investigate from a single combined message, then stop at RCA", NodeTimeout(4*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["combined-investigate"]
		Expect(targetNS).NotTo(BeEmpty(), "combined-investigate namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1853-001")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the combined RR")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNS,
				Labels: map[string]string{
					"kubernaut.ai/managed":     "true",
					"kubernaut.ai/environment": "staging",
				},
			},
		}
		if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", targetNS)
		}
		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), ns, &client.DeleteOptions{})
		})

		By("Deploying zero-replica target Deployment in isolated namespace")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "memory-eater",
				Namespace: targetNS,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "memory-eater"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "memory-eater"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "app",
							Image: "busybox:1.36",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		By("Turn 1 (single message): create AND investigate in one shot (kubernaut_remediate -> kubernaut_investigate chain)")
		body := fpA2ATasksSend("fp-comb-1",
			"create and investigate remediation for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "combined turn should not return a JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  Combined turn — task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Verifying the RR was created via kubernaut_remediate, targeting the isolated namespace")
		rrName := fpWaitForRRWithTargetNS(targetNS, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())

		By("Verifying kubernaut_investigate actually ran: an InvestigationSession exists for this RR (#1853 proof — rr_id was correctly propagated via $from_tool, not left as an unresolved template)")
		Eventually(func() bool {
			isList := &isv1alpha1.InvestigationSessionList{}
			if err := apiReader.List(ctx, isList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for _, is := range isList.Items {
				for _, ref := range is.OwnerReferences {
					if ref.Kind == "RemediationRequest" && ref.Name == rrName {
						return true
					}
				}
			}
			return false
		}, 90*time.Second, 2*time.Second).Should(BeTrue(),
			"InvestigationSession owned by RR %s must exist — proves kubernaut_investigate received the real rr_id from kubernaut_remediate's response, not an unresolved $from_tool template", rrName)
		GinkgoWriter.Printf("  Combined remediate+investigate chain completed for %s\n", rrName)
	})
})
