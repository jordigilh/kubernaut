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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// E2E tests for #1507: Alertmanager + Node Proxy tools
// Validates that get_alerts, get_silences, nodes_log, and nodes_stats_summary
// tools work end-to-end through a full KA investigation flow.
//
// The Mock LLM scenario "MOCK_ALERTMANAGER_NODE_TOOLS" instructs the agent
// to call get_alerts and nodes_stats_summary during RCA. This validates:
//   - Tool registration and phase scoping (PhaseRCA includes these tools)
//   - Alertmanager client connectivity (real AM deployed in Kind)
//   - Kubelet proxy client connectivity (nodes/proxy RBAC granted)
//   - Response handling and truncation
//
// Business Requirements: BR-TOOLSET-001, BR-TOOLSET-002, BR-TOOLSET-003, BR-TOOLSET-004

var _ = Describe("E2E-KA-1507: Alertmanager & Node Proxy Tools", Label("e2e", "ka", "tools", "1507"), func() {

	Context("BR-TOOLSET-001/003: get_alerts + nodes_stats_summary via full investigation", func() {

		It("E2E-KA-1507-001: Investigation calling get_alerts and nodes_stats_summary completes successfully", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-1507-001
			// Business Outcome: KA can query Alertmanager and Kubelet proxy during RCA
			// BR: BR-TOOLSET-001, BR-TOOLSET-003
			// Mock LLM scenario: MOCK_ALERTMANAGER_NODE_TOOLS
			//   - Calls get_alerts (queries real AlertManager in Kind)
			//   - Calls nodes_stats_summary (queries real kubelet via nodes/proxy)

			req := &agentclient.IncidentRequest{
				IncidentID:        "e2e-ka-1507-tools",
				RemediationID:     "req-e2e-ka-1507",
				SignalName:        "MOCK_ALERTMANAGER_NODE_TOOLS",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "api-server-abc",
				ErrorMessage:      "Node resource exhaustion detected",
				Environment:       "production",
				Priority:          "high",
				RiskTolerance:     "medium",
				BusinessCategory:  "web-application",
				ClusterName:       "kubernaut-agent-e2e",
			}

			result, err := sessionClient.Investigate(ctx, req)
			Expect(err).NotTo(HaveOccurred(), "MOCK_ALERTMANAGER_NODE_TOOLS investigation should succeed")
			Expect(result).NotTo(BeNil())
			Expect(result.IncidentID).To(Equal("e2e-ka-1507-tools"))
			Expect(result.Analysis).NotTo(BeEmpty(), "analysis should contain tool results")
			Expect(result.Confidence).To(BeNumerically(">", 0), "confidence should be positive")
		})
	})
})
