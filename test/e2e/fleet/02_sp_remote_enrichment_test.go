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
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// E2E-FLEET-003: SP remote enrichment via MCP gateway
// Authority: Issue #54, ADR-068
// FedRAMP: SI-4 (information system monitoring -- remote enrichment)
var _ = Describe("E2E-FLEET-003 [SI-4]: SP remote enrichment via MCP gateway populates KubernetesContext without degraded mode (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("enriches a SignalProcessing CR targeting a remote-cluster resource via loopback MCP gateway", func() {
		// Issue #54 flakiness fix: real Kubernetes Pods get a generated name
		// suffix (e.g. "coredns-66bc5c9577-dmj24"), so hardcoding "coredns" as
		// the target Pod name never resolves and the k8s-enricher permanently
		// enters degraded mode ("Target pod not found, entering degraded
		// mode"). Discover an actual running CoreDNS pod name instead.
		var podList corev1.PodList
		Expect(k8sClient.List(ctx, &podList,
			client.InNamespace("kube-system"),
			client.MatchingLabels{"k8s-app": "kube-dns"},
		)).To(Succeed())
		Expect(podList.Items).ToNot(BeEmpty(), "kube-system must have a running CoreDNS pod for this fixture")
		corednsPodName := podList.Items[0].Name

		sp := &signalprocessingv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fleet-003-remote-enrich",
				Namespace: namespace,
			},
			Spec: signalprocessingv1.SignalProcessingSpec{
				RemediationRequestRef: signalprocessingv1.ObjectReference{
					APIVersion: "kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "fleet-003-rr-ref",
					Namespace:  namespace,
				},
				Signal: signalprocessingv1.SignalData{
					Fingerprint:  "f1ee7003abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
					Name:         "FleetRemoteEnrichment",
					Severity:     "high",
					Type:         "alert",
					TargetType:   "kubernetes",
					ClusterID:    "loopback-cluster",
					ReceivedTime: metav1.Now(),
					TargetResource: signalprocessingv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      corednsPodName,
						Namespace: "kube-system",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		By("Waiting for KubernetesContext enrichment via MCP gateway (remote cluster loopback)")
		Eventually(func() bool {
			var updated signalprocessingv1.SignalProcessing
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
				return false
			}
			return updated.Status.KubernetesContext != nil
		}, timeout, interval).Should(BeTrue(),
			"SP should enrich signal with KubernetesContext from remote cluster via MCP gateway")

		By("Verifying enrichment completed without degraded mode (SI-4: monitoring integrity)")
		var enriched signalprocessingv1.SignalProcessing
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &enriched)).To(Succeed())

		Expect(enriched.Status.KubernetesContext.DegradedMode).To(BeFalse(),
			"SI-4: enrichment via fleet MCP gateway must succeed without degraded mode")
	})
})
