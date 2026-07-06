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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// E2E-KA-AUDIT-001: Reasoning capture end-to-end audit reconstruction.
//
// Business Requirement: BR-AI-086 AC6 (#1578)
// Compliance Objective: SOC2 CC8.1 — complete remediation request
// reconstruction from audit traces via correlation_id alone.
//
// This test proves the full pipeline wired across milestones 1-5 of #1578:
// KA is deployed with ai.llm.reasoning.enabled=true; the mock LLM (acting as
// a DeepSeek/vLLM-style OpenAI-compatible reasoning model via the
// "mock_reasoning_capture" scenario) returns a reasoning_content field
// alongside its submit_result_with_workflow tool call; KA's openaicompat
// client captures it into llm.Message.Reasoning regardless of model-name
// detection (capture is unconditional; only *replay* is mode-gated); the
// investigator propagates it into InvestigationResult.Reasoning across the
// Phase 1 -> Phase 3 handoff; and the audit layer surfaces it on both the
// per-turn aiagent.llm.response event and the aiagent.rca.complete event —
// all reconstructable from DataStorage by remediation_id (correlation_id)
// alone, without needing any other data source.
var _ = Describe("E2E-KA-AUDIT-001: Reasoning captured in audit trail, reconstructable by correlation_id", Label("e2e", "ka", "audit", "reasoning"), func() {

	var dataStorageClient *ogenclient.Client

	BeforeEach(func() {
		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ServiceAccount token")

		dataStorageClient, err = ogenclient.NewClient(
			dataStorageURL,
			ogenclient.WithClient(&http.Client{
				Transport: testauth.NewServiceAccountTransport(saToken),
				Timeout:   30 * time.Second,
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated DataStorage client")
	})

	It("E2E-KA-AUDIT-001: reasoning is captured on both per-turn and RCA-complete audit events, reconstructable by correlation_id (SOC2 CC8.1)", func() {
		remediationID := "test-audit-reasoning-001-" + time.Now().Format("20060102150405")

		req := &agentclient.IncidentRequest{
			IncidentID:        "test-audit-reasoning-001",
			RemediationID:     remediationID,
			SignalName:        "MOCK_REASONING_CAPTURE",
			Severity:          "critical",
			SignalSource:      "kubernetes",
			ResourceNamespace: "production",
			ResourceKind:      "Deployment",
			ResourceName:      "api-server",
			ErrorMessage:      "mock_reasoning_capture triggered for BR-AI-086 AC6 E2E audit reconstruction test",
			Environment:       "production",
			Priority:          "P1",
			RiskTolerance:     "medium",
			BusinessCategory:  "standard",
			ClusterName:       "e2e-test",
		}

		_, err := sessionClient.Investigate(ctx, req)
		Expect(err).ToNot(HaveOccurred(), "KA incident analysis API call should succeed")

		// ASSERT: every event persisted under this remediation_id must be
		// reconstructable by correlation_id alone (SOC2 CC8.1) — query once,
		// by correlation_id only, and derive both the per-turn and the
		// RCA-complete reasoning assertions from that single result set.
		var events []ogenclient.AuditEvent
		Eventually(func() bool {
			resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(remediationID),
			})
			if qErr != nil {
				return false
			}
			events = resp.Data
			for _, event := range events {
				if event.EventType == string(ogenclient.AIAgentRCACompletePayloadAuditEventEventData) {
					return true
				}
			}
			return false
		}, 30*time.Second, 1*time.Second).Should(BeTrue(),
			"aiagent.rca.complete event must be persisted and reconstructable by correlation_id=remediation_id")

		// All events for this investigation must share the same correlation_id
		// (BR-AUDIT-005: complete reconstruction from a single correlation_id
		// query, with no need to cross-reference other identifiers).
		for _, event := range events {
			Expect(event.CorrelationID).To(Equal(remediationID),
				"every event for this investigation must carry remediation_id as correlation_id (%s)", event.EventType)
		}

		// BEHAVIOR 1: at least one per-turn aiagent.llm.response event carries
		// the captured reasoning (BR-AI-086 AC6 milestone 5 per-turn scope).
		var foundResponseReasoning bool
		for _, event := range events {
			if event.EventType != string(ogenclient.LLMResponsePayloadAuditEventEventData) {
				continue
			}
			payload, ok := event.EventData.GetLLMResponsePayload()
			if !ok {
				continue
			}
			if text, hasText := payload.ReasoningText.Get(); hasText && text != "" {
				foundResponseReasoning = true
				redacted, hasRedacted := payload.ReasoningRedacted.Get()
				Expect(hasRedacted).To(BeTrue(), "reasoning_redacted must accompany reasoning_text")
				Expect(redacted).To(BeFalse(), "mock LLM returned visible reasoning, not a redacted block")
				break
			}
		}
		Expect(foundResponseReasoning).To(BeTrue(),
			"at least one aiagent.llm.response event must carry the mock LLM's reasoning_content (BR-AI-086 AC6)")

		// BEHAVIOR 2: the aiagent.rca.complete event's RootCauseAnalysis
		// carries the same captured reasoning, surviving the Phase 1 -> Phase
		// 3 handoff (IT-KA-1578-002 regression, proven here end-to-end).
		var foundRCAReasoning bool
		for _, event := range events {
			if event.EventType != string(ogenclient.AIAgentRCACompletePayloadAuditEventEventData) {
				continue
			}
			payload, ok := event.EventData.GetAIAgentRCACompletePayload()
			Expect(ok).To(BeTrue())
			reasoning, hasReasoning := payload.ResponseData.RootCauseAnalysis.Reasoning.Get()
			if hasReasoning && reasoning != "" {
				foundRCAReasoning = true
				Expect(reasoning).To(ContainSubstring("leak"),
					"RCA-complete reasoning must be the mock LLM's actual reasoning text, not a placeholder")
			}
		}
		Expect(foundRCAReasoning).To(BeTrue(),
			"aiagent.rca.complete's RootCauseAnalysis must carry reasoning that survived the Phase 1 -> Phase 3 handoff")
	})
})
