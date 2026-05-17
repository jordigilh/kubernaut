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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// E2E-KA-DISC: Interactive Workflow Discovery lifecycle tests.
//
// These tests validate the discover_workflows, select_workflow gating, and
// complete_no_action tools against the real Kind cluster with real KA + MockLLM.
//
// Tests:
//   E2E-KA-DISC-001: start -> message -> discover_workflows -> select_workflow
//   E2E-KA-DISC-002: start -> complete_no_action with reason
//   E2E-KA-DISC-003: select_workflow without discover_workflows (gating)
var _ = Describe("E2E-KA-DISC: Interactive Workflow Discovery", Label("e2e", "ka", "interactive", "discovery"), func() {

	var (
		mcpEndpoint  string
		tlsTransport http.RoundTripper
		saToken      string
	)

	BeforeEach(func() {
		mcpEndpoint = infrastructure.MCPEndpointForKAE2E()
		tlsTransport = testauth.NewRetryOn429Transport(http.DefaultTransport)

		var err error
		saToken, err = infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "should get E2E SA token")
	})

	Describe("E2E-KA-DISC-001: Full discovery lifecycle", func() {
		It("should execute start -> message -> discover_workflows -> select_workflow", func() {
			rrID := fmt.Sprintf("rr-disc001-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Connecting MCP client")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred(), "MCP client should connect")
			defer session.Close()

			By("Starting interactive session")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsError).To(BeFalse(), "start should succeed")

			By("Verifying Lease exists")
			clientset, err := getKubernetesClientset()
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				lease, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				return err == nil && lease.Spec.HolderIdentity != nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Sending investigative message")
			result, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "What is the root cause of this OOMKill event?",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			Expect(infrastructure.ExtractToolResultText(result)).NotTo(BeEmpty())

			By("Discovering workflows via LLM")
			result, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			discoverText := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("discover_workflows result: %s\n", discoverText)
			Expect(result.IsError).To(BeFalse(), "discover_workflows should succeed")

			By("Extracting recommended workflow_id from discovery results")
			var outerData map[string]any
			Expect(json.Unmarshal([]byte(discoverText), &outerData)).To(Succeed(),
				"discover_workflows should return valid JSON")
			responseStr, ok := outerData["response"].(string)
			Expect(ok).To(BeTrue(), "discovery result should have a response field")
			var innerData map[string]any
			Expect(json.Unmarshal([]byte(responseStr), &innerData)).To(Succeed(),
				"response field should contain valid JSON")
			recommended, ok := innerData["recommended"].(map[string]any)
			Expect(ok).To(BeTrue(), "discovery result should have a recommended workflow")
			recommendedID, ok := recommended["workflow_id"].(string)
			Expect(ok).To(BeTrue(), "recommended workflow should have a workflow_id")
			GinkgoWriter.Printf("Using recommended workflow_id: %s\n", recommendedID)

			By("Verifying recommended workflow has parameters (#1169)")
			recParams, _ := recommended["parameters"].(map[string]any)
			Expect(recParams).NotTo(BeEmpty(),
				"recommended workflow must include LLM-provided parameters in discovery response (#1169)")
			GinkgoWriter.Printf("Recommended parameters: %v\n", recParams)

			By("Verifying alternatives have parameters (#1169)")
			alts, _ := innerData["alternatives"].([]any)
			if len(alts) > 0 {
				for i, a := range alts {
					altMap, _ := a.(map[string]any)
					if altMap != nil {
						altParams, _ := altMap["parameters"].(map[string]any)
						if len(altParams) > 0 {
							GinkgoWriter.Printf("Alternative[%d] (%s) parameters: %v\n",
								i, altMap["workflow_id"], altParams)
						}
					}
				}
			}

			By("Selecting the recommended workflow from discovery results")
			result, err = infrastructure.CallSelectWorkflow(ctx, session, map[string]any{
				"rr_id":       rrID,
				"workflow_id": recommendedID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			selectText := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("select_workflow result: %s\n", selectText)
			Expect(result.IsError).To(BeFalse(), "select_workflow should succeed with recommended workflow")

			By("Verifying Lease released after auto-complete")
			Eventually(func() bool {
				_, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Lease should be deleted after select_workflow auto-complete")

			GinkgoWriter.Println("E2E-KA-DISC-001: Full discovery lifecycle completed")
		})
	})

	Describe("E2E-KA-DISC-002: complete_no_action with reason", func() {
		It("should complete with no workflow and release the Lease", func() {
			rrID := fmt.Sprintf("rr-disc002-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Connecting MCP client")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			By("Starting interactive session")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("Completing with no action")
			result, err = infrastructure.CallCompleteNoAction(ctx, session, map[string]any{
				"rr_id":  rrID,
				"reason": "false alarm, no remediation needed",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsError).To(BeFalse(), "complete_no_action should succeed")

			cnaText := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("complete_no_action result: %s\n", cnaText)
			Expect(cnaText).To(ContainSubstring("completed_no_action"))

			By("Verifying Lease released")
			clientset, err := getKubernetesClientset()
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				_, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Lease should be deleted after complete_no_action")

			GinkgoWriter.Println("E2E-KA-DISC-002: complete_no_action lifecycle completed")
		})
	})

	Describe("E2E-KA-DISC-003: select_workflow gating without discover_workflows", func() {
		It("should reject select_workflow when discover_workflows was not called", func() {
			rrID := fmt.Sprintf("rr-disc003-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Connecting MCP client")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			By("Starting interactive session")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("Attempting select_workflow without discover_workflows")
			result, err = infrastructure.CallSelectWorkflow(ctx, session, map[string]any{
				"rr_id":       rrID,
				"workflow_id": "wf-attempt-no-discovery",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsError).To(BeTrue(),
				"select_workflow must fail when discover_workflows was not called")

			errorText := infrastructure.ExtractToolResultText(result)
			Expect(errorText).To(ContainSubstring("discover_workflows"),
				"error should mention discover_workflows as prerequisite")
			GinkgoWriter.Printf("Gating error: %s\n", errorText)

			By("Cleaning up session")
			_, _ = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "cancel",
			})

			GinkgoWriter.Println("E2E-KA-DISC-003: select_workflow gating validated")
		})
	})

	Describe("E2E-KA-DISC-004: select alternative workflow propagates parameters through real KA (#1169)", func() {
		It("should discover alternatives with parameters and successfully select one", func() {
			rrID := fmt.Sprintf("rr-disc004-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Connecting MCP client")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			By("Starting interactive session")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("Sending OOMKill-themed message to steer to oomkilled scenario")
			result, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "What is the root cause of this OOMKill event?",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("Discovering workflows")
			result, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("Extracting alternative workflow_id from discovery results")
			discoverText := infrastructure.ExtractToolResultText(result)
			var outerData map[string]any
			Expect(json.Unmarshal([]byte(discoverText), &outerData)).To(Succeed())
			responseStr, ok := outerData["response"].(string)
			Expect(ok).To(BeTrue())
			var innerData map[string]any
			Expect(json.Unmarshal([]byte(responseStr), &innerData)).To(Succeed())

			alts, _ := innerData["alternatives"].([]any)
			Expect(alts).NotTo(BeEmpty(),
				"discovery must return at least one alternative for the oomkilled scenario (#1169)")

			alt0, _ := alts[0].(map[string]any)
			altID, ok := alt0["workflow_id"].(string)
			Expect(ok).To(BeTrue(), "alternative must have a workflow_id")
			GinkgoWriter.Printf("Selecting alternative workflow_id: %s\n", altID)

			altParams, _ := alt0["parameters"].(map[string]any)
			GinkgoWriter.Printf("Alternative parameters: %v\n", altParams)

			By("Selecting the alternative workflow")
			result, err = infrastructure.CallSelectWorkflow(ctx, session, map[string]any{
				"rr_id":       rrID,
				"workflow_id": altID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			selectText := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("select_workflow result: %s\n", selectText)
			Expect(result.IsError).To(BeFalse(),
				"select_workflow must succeed for a discovery-listed alternative (#1169)")

			By("Verifying Lease released after auto-complete")
			clientset, err := getKubernetesClientset()
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				_, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Lease should be deleted after selecting alternative workflow")

			GinkgoWriter.Println("E2E-KA-DISC-004: Alternative workflow selection completed")
		})
	})
})
