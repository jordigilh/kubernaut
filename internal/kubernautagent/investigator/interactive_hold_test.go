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

package investigator_test

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// interactiveHoldMockClient returns an RCA result on the first call (Phase 1)
// and tracks how many calls are made to detect Phase 2/3 attempts.
type interactiveHoldMockClient struct {
	chatCalls int
}

func (m *interactiveHoldMockClient) Close() error { return nil }

func (m *interactiveHoldMockClient) StreamChat(ctx context.Context, req llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := m.Chat(ctx, req)
	if err == nil {
		_ = callback(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (m *interactiveHoldMockClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	m.chatCalls++

	rcaJSON, _ := json.Marshal(map[string]interface{}{
		"investigation_outcome": "root_cause_identified",
		"rca_summary":           "OOMKilled due to memory limit breach in api-server container",
		"severity":              "critical",
		"remediation_target": map[string]string{
			"kind":      "Deployment",
			"name":      "api-server",
			"namespace": "production",
		},
		"confidence":          0.92,
		"human_review_needed": false,
	})

	return llm.ChatResponse{
		Message: llm.Message{
			Role:    "assistant",
			Content: "",
			ToolCalls: []llm.ToolCall{
				{
					ID:        "call-001",
					Name:      "submit_result",
					Arguments: string(rcaJSON),
				},
			},
		},
		FinishReason: "tool_calls",
	}, nil
}

var _ = Describe("BR-INTERACTIVE-010: Investigator Interactive Hold — #1293", func() {

	Describe("UT-KA-1293-016: Investigate with Interactive=true returns InteractiveHold=true after RCA", func() {
		It("should skip Phase 2+3 and return InteractiveHold when signal.Interactive is true", func() {
			client := &interactiveHoldMockClient{}
			logger := logr.Discard()
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()
			enricher := enrichment.NewEnricher(nopK8sClient{}, nopDSClient{}, audit.NopAuditStore{}, logger)

			inv := investigator.New(investigator.Config{
				Client:       client,
				Builder:      builder,
				ResultParser: rp,
				Enricher:     enricher,
				AuditStore:   audit.NopAuditStore{},
				Logger:       logger,
				MaxTurns:     15,
				PhaseTools:   investigator.DefaultPhaseToolMap(),
			})

			signal := katypes.SignalContext{
				Name:          "api-server",
				Namespace:     "production",
				Severity:      "critical",
				Message:       "OOMKilled",
				RemediationID: "rem-interactive-016",
				Interactive:   true,
			}

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.InteractiveHold).To(BeTrue(),
				"InteractiveHold must be true when signal.Interactive=true and RCA completes")

			// Phase 3 workflow selection is skipped — no WorkflowID populated
			Expect(result.WorkflowID).To(BeEmpty(),
				"WorkflowID must be empty — Phase 3 workflow selection should be skipped for interactive")
			interactiveCalls := client.chatCalls

			// Verify the call count is bounded to RCA phase only (no extra
			// calls for Phase 3 workflow discovery)
			Expect(interactiveCalls).To(BeNumerically("<=", 2),
				"interactive path should only make RCA-phase LLM calls")
		})
	})

	Describe("UT-KA-1293-017: Investigate with Interactive=false completes full pipeline", func() {
		It("should proceed through all phases normally when Interactive is false", func() {
			client := &interactiveHoldMockClient{}
			logger := logr.Discard()
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()
			enricher := enrichment.NewEnricher(nopK8sClient{}, nopDSClient{}, audit.NopAuditStore{}, logger)

			inv := investigator.New(investigator.Config{
				Client:       client,
				Builder:      builder,
				ResultParser: rp,
				Enricher:     enricher,
				AuditStore:   audit.NopAuditStore{},
				Logger:       logger,
				MaxTurns:     15,
				PhaseTools:   investigator.DefaultPhaseToolMap(),
			})

			signal := katypes.SignalContext{
				Name:          "api-server",
				Namespace:     "production",
				Severity:      "critical",
				Message:       "OOMKilled",
				RemediationID: "rem-autonomous-017",
				Interactive:   false,
			}

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.InteractiveHold).To(BeFalse(),
				"InteractiveHold must be false for autonomous investigation")

			// Autonomous flow proceeds through full pipeline (RCA + workflow)
			Expect(client.chatCalls).To(BeNumerically(">=", 2),
				"autonomous flow should invoke LLM for both RCA and workflow selection")
		})
	})
})
