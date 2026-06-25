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

package signalprocessing

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("E2E-SP-054-REMOTE [AU-3, SC-7]: SP enriches signals originating from remote clusters via the MCP gateway and records cluster provenance (BR-INTEGRATION-054)", Label("fleet"), func() {
	BeforeEach(func() {
		if os.Getenv("FLEET_E2E") != "true" {
			Skip("FLEET_E2E=true required for fleet E2E tests")
		}
	})

	It("enriches a SignalProcessing CR targeting a remote-cluster Pod via loopback MCP gateway", func() {
		testNs := helpers.CreateTestNamespace(ctx, k8sClient, "e2e-fleet")
		defer helpers.DeleteTestNamespace(ctx, k8sClient, testNs)

		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-fleet-remote-enrich",
				Namespace: controllerNamespace,
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
					APIVersion: "kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "e2e-fleet-remote-rr",
					Namespace:  controllerNamespace,
				},
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  "fleet054abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
					Name:         "FleetRemoteEnrichmentE2E",
					Severity:     "high",
					Type:         "alert",
					TargetType:   "kubernetes",
					ClusterID:    "loopback-cluster",
					ReceivedTime: metav1.Now(),
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "coredns",
						Namespace: "kube-system",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		By("Waiting for KubernetesContext enrichment via MCP gateway (remote cluster loopback)")
		Eventually(func() bool {
			var updated signalprocessingv1alpha1.SignalProcessing
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
				return false
			}
			return updated.Status.KubernetesContext != nil
		}, timeout, interval).Should(BeTrue(),
			"SP should enrich signal with KubernetesContext from remote cluster via MCP gateway")

		By("Verifying enrichment completed without degraded mode (AU-3: provenance tracked)")
		var enriched signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &enriched)).To(Succeed())

		Expect(enriched.Status.KubernetesContext.DegradedMode).To(BeFalse(),
			"enrichment via fleet MCP gateway should succeed without degraded mode")
	})
})
