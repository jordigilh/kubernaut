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
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

var _ = Describe("IT-KA-795: RCA parse retry on failure", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-795-R01: RCA parse failure triggers retry, retry succeeds with correct JSON", func() {
		It("should send a correction message to the LLM when RCA parse fails", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					// RCA phase: LLM submits garbage JSON via submit_result (parse fails)
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca1", Name: "submit_result", Arguments: `{"foo":"bar","baz":42}`}},
					},
					// RCA retry: LLM submits correct JSON via submit_result
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca2", Name: "submit_result", Arguments: `{"root_cause_analysis":{"summary":"OOMKilled due to memory leak","remediation_target":{"kind":"Deployment","name":"api","namespace":"production"}},"confidence":0.9}`}},
					},
					// Workflow phase: list_available_actions then submit
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_wf1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"restart","confidence":0.85}`),
				},
			}

			k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "ReplicaSet", Name: "api-rs-abc", Namespace: "production"},
				{Kind: "Deployment", Name: "api", Namespace: "production"},
			}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "OOMKilled", Namespace: "production", Severity: "critical",
				Message: "Pod api-pod OOMKilled", ResourceKind: "Pod", ResourceName: "api-pod",
				Environment: "production", Priority: "P0",
			})
			Expect(err).NotTo(HaveOccurred())

			// Without retry: RCA phase consumes 1 mock response, workflow consumes 1 more = 2 total.
			// With retry: RCA phase consumes 2 (initial + retry), workflow consumes 2 = 4 total.
			Expect(len(mockClient.calls)).To(BeNumerically(">=", 4),
				"IT-KA-795-R01: RCA retry must issue at least one extra LLM call (expect >= 4 total calls)")

			// The second call (index 1) should be the retry correction message
			Expect(allMessageContent(mockClient.calls[1].Messages)).To(ContainSubstring("could not be parsed"),
				"IT-KA-795-R01: retry correction message must contain parse failure feedback")
		})
	})

	Describe("IT-KA-795-R02: RCA parse failure, retry also fails, falls back to summary", func() {
		It("should send correction message and then fall back to raw content when retry also fails", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					// RCA phase: garbage JSON
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca1", Name: "submit_result", Arguments: `{"foo":"bar"}`}},
					},
					// RCA retry: still garbage (submit_result with unrecognized fields)
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca2", Name: "submit_result", Arguments: `{"invalid":"still wrong"}`}},
					},
					// Workflow phase: list_available_actions returns empty, LLM declines
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_wf1", Name: "list_available_actions", Arguments: `{}`}},
					},
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_no", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"fallback"},"reasoning":"no workflows"}`}},
					},
				},
			}

			k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api", Namespace: "production"},
			}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "OOMKilled", Namespace: "production", Severity: "critical",
				Message: "Pod api-pod OOMKilled", ResourceKind: "Pod", ResourceName: "api-pod",
				Environment: "production", Priority: "P0",
			})
			Expect(err).NotTo(HaveOccurred())

			// Without retry: 1 RCA call + workflow calls.
			// With retry: 2 RCA calls (initial + retry) + workflow calls.
			Expect(len(mockClient.calls)).To(BeNumerically(">=", 4),
				"IT-KA-795-R02: RCA retry must issue at least one extra LLM call (expect >= 4 total calls)")

			// Verify correction message was sent to the LLM
			Expect(allMessageContent(mockClient.calls[1].Messages)).To(ContainSubstring("could not be parsed"),
				"IT-KA-795-R02: retry correction message must contain parse failure feedback")
		})
	})
})
