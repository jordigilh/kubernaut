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

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("IT-KA-795: RCA parse retry on failure", func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-795-R01: RCA parse failure triggers retry, retry succeeds with correct JSON", func() {
		It("should send a correction message to the LLM when RCA parse fails", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS, nil, invLogger) {
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

			k8sClient := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "ReplicaSet", Name: "api-rs-abc", Namespace: "production"},
				{Kind: "Deployment", Name: "api", Namespace: "production"},
			}}
			enricher := enrichment.NewEnricher(k8sClient, suiteDSAdapter, auditStore, invLogger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
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

	Describe("IT-KA-795-R02: RCA parse failure, retry also fails, fails closed", func() {
		It("should request human review without preserving unparsed RCA content", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS, nil, invLogger) {
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
				},
			}

			k8sClient := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api", Namespace: "production"},
			}}
			enricher := enrichment.NewEnricher(k8sClient, suiteDSAdapter, auditStore, invLogger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "OOMKilled", Namespace: "production", Severity: "critical",
				Message: "Pod api-pod OOMKilled", ResourceKind: "Pod", ResourceName: "api-pod",
				Environment: "production", Priority: "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// RCA parse exhaustion is terminal: the workflow phase must not run.
			Expect(mockClient.calls).To(HaveLen(2),
				"IT-KA-795-R02: exactly one parse retry must occur before fail-closed human review")
			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).To(Equal("llm_parsing_error"))
			Expect(result.RCASummary).To(BeEmpty(),
				"IT-KA-795-R02: unparsed LLM content must not become an RCA summary")
			Expect(result.WorkflowID).To(BeEmpty(),
				"IT-KA-795-R02: workflow selection must not run after RCA parse exhaustion")
			Expect(result.Severity).To(BeEmpty(),
				"IT-KA-795-R02: signal severity must not make an unparsed RCA look valid")

			// Verify correction message was sent to the LLM
			Expect(allMessageContent(mockClient.calls[1].Messages)).To(ContainSubstring("could not be parsed"),
				"IT-KA-795-R02: retry correction message must contain parse failure feedback")
		})
	})

	Describe("IT-KA-795-R03: Trailing brace + no-summary triggers retry, retry succeeds with proper JSON", func() {
		It("should recover when first response is double-serialized with trailing brace and no summary", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS, nil, invLogger) {
				reg.Register(t)
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					// RCA phase: double-serialized RCA with trailing brace, NO summary
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca1", Name: "submit_result", Arguments: `{"rootCauseAnalysis":"{\"severity\":\"medium\",\"remediation_target\":{\"kind\":\"Deployment\",\"name\":\"web-frontend\",\"namespace\":\"demo-gitops\"}}}","confidence":0.98}`}},
					},
					// RCA retry: LLM now submits correct JSON with summary
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca2", Name: "submit_result", Arguments: `{"root_cause_analysis":{"summary":"CrashLoopBackOff caused by invalid ConfigMap directive","severity":"critical","remediation_target":{"kind":"Deployment","name":"web-frontend","namespace":"demo-gitops"}},"confidence":0.95}`}},
					},
					// Workflow phase: list_available_actions then submit
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_wf1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"git-revert-v2","confidence":0.9}`),
				},
			}

			k8sClient := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "ReplicaSet", Name: "web-frontend-rs", Namespace: "demo-gitops"},
				{Kind: "Deployment", Name: "web-frontend", Namespace: "demo-gitops"},
			}}
			enricher := enrichment.NewEnricher(k8sClient, suiteDSAdapter, auditStore, invLogger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "KubePodCrashLooping", Namespace: "demo-gitops", Severity: "critical",
				Message: "Pod web-frontend-c8dc85956-jn2b8 CrashLoopBackOff", ResourceKind: "Pod",
				ResourceName: "web-frontend-c8dc85956-jn2b8",
				Environment:  "staging", Priority: "P1",
			})
			Expect(err).NotTo(HaveOccurred())

			// With retry: RCA phase consumes 2 (initial fails Fix D + retry), workflow consumes 2 = 4 total
			Expect(len(mockClient.calls)).To(BeNumerically(">=", 4),
				"IT-KA-795-R03: trailing-brace no-summary must trigger retry (expect >= 4 LLM calls)")

			// The retry must have sent a correction message
			Expect(allMessageContent(mockClient.calls[1].Messages)).To(ContainSubstring("could not be parsed"),
				"IT-KA-795-R03: retry correction message must be sent after Fix D rejection")

			// The final result should have the workflow from the retry path
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("git-revert-v2"),
				"IT-KA-795-R03: workflow must be selected after successful retry")
		})
	})

	Describe("IT-KA-800-TR-01: Truncation detection triggers retry with escalated MaxTokens", func() {
		It("should detect FinishReason 'length' and retry instead of returning truncated text", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS, nil, invLogger) {
				reg.Register(t)
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					// RCA phase: truncated text response (FinishReason = "length")
					{
						Message:      llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled due to`},
						FinishReason: llm.FinishReasonLength,
					},
					// RCA retry: successful submit_result after truncation recovery
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca2", Name: "submit_result", Arguments: `{"root_cause_analysis":{"summary":"OOMKilled due to memory leak","remediation_target":{"kind":"Deployment","name":"api","namespace":"production"}},"confidence":0.9}`}},
					},
					// Workflow phase
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_wf1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"restart","confidence":0.85}`),
				},
			}

			k8sClient := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api", Namespace: "production"},
			}}
			enricher := enrichment.NewEnricher(k8sClient, suiteDSAdapter, auditStore, invLogger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "OOMKilled", Namespace: "production", Severity: "critical",
				Message: "Pod api-pod OOMKilled", ResourceKind: "Pod", ResourceName: "api-pod",
				Environment: "production", Priority: "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"))

			// Truncation recovery must have caused at least one extra LLM call
			Expect(len(mockClient.calls)).To(BeNumerically(">=", 4),
				"IT-KA-800-TR-01: truncation detection must cause a retry (>= 4 calls)")

			// Second call should have increased MaxTokens
			secondCall := mockClient.calls[1]
			Expect(secondCall.Options.MaxTokens).To(BeNumerically(">", 0),
				"IT-KA-800-TR-01: retry after truncation must escalate MaxTokens")
		})
	})
})
