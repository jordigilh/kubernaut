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

package kubernautagent

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("E2E-KA-2482: empty tool-result replay", Label("e2e", "ka", "llm", "2482"), func() {
	It("E2E-KA-2482-001 [BR-KA-263, BR-AI-086, SI-10, SOC2 CC7.2]: completes the AgentSession journey when a Kubernetes tool returns an empty string", func() {
		By("waiting for the deterministic empty-log fixture to be running")
		Eventually(func() error {
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test-pod"}, pod); err != nil {
				return fmt.Errorf("get default/test-pod: %w", err)
			}
			if pod.Status.Phase != corev1.PodRunning {
				return fmt.Errorf("default/test-pod phase is %s", pod.Status.Phase)
			}
			return nil
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		spec := agentsessionv1.AgentSessionSpec{
			RemediationRequestRef: agentsessionv1.ObjectRef{Name: "req-e2e-ka-2482", Namespace: sharedNamespace},
			IncidentID:            "e2e-ka-2482-empty-tool-result",
			RemediationID:         "req-e2e-ka-2482",
			SignalName:            "MOCK_EMPTY_TOOL_RESULT_REPLAY",
			Severity:              "critical",
			SignalSource:          "kubernetes",
			ResourceNamespace:     "default",
			ResourceKind:          "Pod",
			ResourceName:          "test-pod",
			ErrorMessage:          "empty tool result replay regression",
			Environment:           "production",
			Priority:              "P1",
			RiskTolerance:         "medium",
			BusinessCategory:      "standard",
		}

		result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 3*time.Minute)
		Expect(err).NotTo(HaveOccurred(), "the strict OpenAI-compatible provider must accept content=\"\"")
		Expect(result).NotTo(BeNil())
		Expect(result.IncidentID).To(Equal("e2e-ka-2482-empty-tool-result"))
		Expect(result.Analysis).NotTo(BeEmpty())
		Expect(result.SelectedWorkflow).NotTo(BeNil())
		Expect(result.Confidence).To(BeNumerically(">", 0))
	})
})
