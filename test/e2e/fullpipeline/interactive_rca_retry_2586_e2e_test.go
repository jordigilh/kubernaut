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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

const interactiveRCARetrySignal2586 = "InteractiveRCAExtractionRetry2586"

// E2E-KA-2586-041 proves the real interactive discover_workflows path keeps
// the correction turn's RCA reasoning and independent confidence in the
// durable response audit projection. The mock LLM scenario returns malformed
// submit_result arguments once, then a valid correction with distinctive
// reasoning; the scenario is limited to the submit-only RCA extraction tool so
// autonomous RCA and Phase 3 workflow discovery remain unaffected.
var _ = Describe("E2E-KA-2586: interactive RCA correction audit projection", Label("e2e", "fullpipeline", "interactive", "2586"), func() {
	It("E2E-KA-2586-041: discover_workflows persists corrected RCA reasoning and confidence", func() {
		By("Creating a direct RR with the isolated correction-retry signal")
		rrName, err := infrastructure.CreateDirectRRWithSignal(ctx, namespace, "fp-mcp-2586-041", interactiveRCARetrySignal2586)
		Expect(err).NotTo(HaveOccurred())

		By("Setting up the real interactive MCP session")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-2586-041-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Taking over the RR")
		takeoverCtx, takeoverCancel := context.WithTimeout(ctx, 60*time.Second)
		defer takeoverCancel()
		result, err := infrastructure.CallInvestigate(takeoverCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover should succeed")

		By("Sending a real interactive message to seed the conversation")
		messageCtx, messageCancel := context.WithTimeout(ctx, 60*time.Second)
		defer messageCancel()
		result, err = infrastructure.CallInvestigate(messageCtx, setup.Session, map[string]any{
			"rr_id":   rrName,
			"action":  "message",
			"message": "Please summarize the current root cause evidence before workflow discovery.",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "interactive message should succeed")
		Expect(infrastructure.ExtractToolResultText(result)).NotTo(BeEmpty(),
			"interactive message must leave a successful conversation turn for RCA extraction")

		By("Calling discover_workflows through production MCP wiring")
		var discoveryText string
		Eventually(func(g Gomega) {
			discoverCtx, discoverCancel := context.WithTimeout(ctx, 30*time.Second)
			defer discoverCancel()
			result, callErr := infrastructure.CallInvestigate(discoverCtx, setup.Session, map[string]any{
				"rr_id":  rrName,
				"action": "discover_workflows",
			})
			g.Expect(callErr).NotTo(HaveOccurred())
			g.Expect(result.IsError).To(BeFalse(), "discover_workflows should succeed")
			discoveryText = infrastructure.ExtractToolResultText(result)
			g.Expect(discoveryText).To(ContainSubstring("workflows_discovered"))
		}, timeout, 5*time.Second).Should(Succeed())

		var discoveryPayload map[string]interface{}
		Expect(json.Unmarshal([]byte(discoveryText), &discoveryPayload)).To(Succeed())
		response, ok := discoveryPayload["response"].(string)
		Expect(ok).To(BeTrue(), "discover_workflows response must be serialized JSON")
		Expect(response).NotTo(BeEmpty(), "discover_workflows must return a recommendation payload")

		By("Querying the durable response audit projection by RR correlation ID")
		var correctedReasoning string
		var correctedConfidence float64
		Eventually(func(g Gomega) {
			resp, queryErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(rrName),
				Limit:         ogenclient.NewOptInt(200),
			})
			g.Expect(queryErr).NotTo(HaveOccurred())

			foundProjection := false
			for _, event := range resp.Data {
				if event.EventType != string(ogenclient.AIAgentResponsePayloadAuditEventEventData) {
					continue
				}
				payload, payloadOK := event.EventData.GetAIAgentResponsePayload()
				if !payloadOK {
					continue
				}
				reasoning, hasReasoning := payload.ResponseData.RootCauseAnalysis.Reasoning.Get()
				confidence, hasConfidence := payload.ResponseData.RcaConfidence.Get()
				if !hasReasoning || !hasConfidence {
					continue
				}
				correctedReasoning = reasoning
				correctedConfidence = confidence
				foundProjection = true
				break
			}
			g.Expect(foundProjection).To(BeTrue(),
				"response.complete must carry the corrected RCA reasoning and confidence")
		}, timeout, 2*time.Second).Should(Succeed())

		Expect(correctedReasoning).To(Equal("accepted correction evidence"),
			"the durable projection must carry the winning correction turn's reasoning")
		Expect(correctedReasoning).NotTo(ContainSubstring("rejected"),
			"the rejected first turn must not become the final RCA reasoning")
		Expect(correctedConfidence).To(BeNumerically("~", 0.65, 0.001),
			"the durable projection must preserve independent RCA confidence")
	})
})
