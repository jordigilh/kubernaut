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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("E2E-KA-970: Parallel Tool Execution", Label("e2e", "ka", "parallel"), func() {

	Context("BR-PERFORMANCE-970: Multi-tool-call scenario triggers parallel execution", func() {

		It("E2E-KA-970-001: Mock LLM returns multiple tool calls; investigation completes successfully", func() {
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "req-e2e-ka-970", Namespace: sharedNamespace},
				IncidentID:            "e2e-ka-970-parallel",
				RemediationID:         "req-e2e-ka-970",
				SignalName:            "MOCK_PARALLEL_TOOLS",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "production",
				ResourceKind:          "Pod",
				ResourceName:          "api-server-abc",
				ErrorMessage:          "Container killed due to OOM",
				Environment:           "production",
				Priority:              "high",
				RiskTolerance:         "medium",
				BusinessCategory:      "web-application",
				ClusterName:           "kubernaut-agent-e2e",
			}

			// #2190: AgentSession CRD flow replaces sessionClient.Investigate().
			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "MOCK_PARALLEL_TOOLS investigation should succeed")
			Expect(result).NotTo(BeNil())
			Expect(result.IncidentID).To(Equal("e2e-ka-970-parallel"))
			Expect(result.Analysis).NotTo(BeEmpty(), "analysis should be non-empty")
			Expect(result.Confidence).To(BeNumerically(">", 0), "confidence should be positive")
		})
	})
})
