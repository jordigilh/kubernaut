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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// #1935 root cause #2 (BR-AUDIT-005 v2.0, FedRAMP AU-3/CC8.1): before this
// fix, sameKindValidationGate/apiVersionValidationGate always received a
// stale [system, user]-only history regardless of how many tools were
// called earlier in the same RCA turn, because runLLMLoop never returned
// its accumulated messages to runRCA. These specs prove the wiring fix end
// to end through the real Investigate() production path: the gate's actual
// retry request (captured via the mock LLM client) must include the prior
// tool-call/tool-result turns.
var _ = Describe("#1935 root cause #2: validation-gate retries must include real tool-call history", func() {

	var (
		logger     logr.Logger
		auditStore *gateRecordingAuditStore
		mockClient *gateMockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = logr.Discard()
		auditStore = &gateRecordingAuditStore{}
		mockClient = &gateMockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		enricher = enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	// toolCallMessages inspects a captured llm.ChatRequest for tool-call/
	// tool-result turns matching the given tool name.
	toolCallMessages := func(req llm.ChatRequest, toolName string) (sawCall, sawResult bool) {
		for _, msg := range req.Messages {
			for _, tc := range msg.ToolCalls {
				if tc.Name == toolName {
					sawCall = true
				}
			}
			if msg.Role == "tool" && msg.ToolName == toolName {
				sawResult = true
			}
		}
		return sawCall, sawResult
	}

	Describe("IT-KA-1935-008: sameKindValidationGate retry includes prior tool-call turns", func() {
		It("sends the earlier kubectl_describe tool_use/tool_result pair as part of the gate retry request", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "api-server-xyz",
				Name:         "api-server-xyz",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "Pod OOMKilled repeatedly",
			}

			mockClient.responses = []llm.ChatResponse{
				// Turn 1: real tool call before the LLM submits its RCA result.
				{
					Message: llm.Message{Role: "assistant", Content: "Investigating..."},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api-server-xyz","namespace":"production"}`},
					},
				},
				// Turn 2: submit_result — same-kind gate fires (target.Kind == signal.ResourceKind == "Pod").
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod OOMKilled",
					"confidence":0.85,
					"remediation_target":{"kind":"Pod","name":"api-server-xyz","namespace":"production"}
				}`}},
				// Turn 3: gate retry — LLM re-targets the parent Deployment.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Deployment memory limit too low",
					"confidence":0.85,
					"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production","api_version":"apps/v1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"increase-memory-limit","confidence":0.9}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(mockClient.calls)).To(BeNumerically(">=", 3),
				"IT-KA-1935-008: expected at least tool-call turn + submit + gate-retry LLM calls")
			gateRetryReq := mockClient.calls[2]

			sawCall, sawResult := toolCallMessages(gateRetryReq, "kubectl_describe")
			Expect(sawCall).To(BeTrue(),
				"IT-KA-1935-008: same-kind gate retry must include the earlier kubectl_describe tool_use turn (#1935 root cause #2)")
			Expect(sawResult).To(BeTrue(),
				"IT-KA-1935-008: same-kind gate retry must include the paired tool_result turn (#1935 root cause #2)")
		})
	})

	Describe("IT-KA-1935-009: apiVersionValidationGate retry includes prior tool-call turns", func() {
		It("sends the earlier kubectl_describe tool_use/tool_result pair as part of the gate retry request", func() {
			mapper := newAmbiguousSubscriptionMapper()
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "etcd-operator-xyz",
				Name:         "etcd-operator-xyz",
				Namespace:    "demo-operator",
				Severity:     "critical",
				Message:      "RBAC denial on wrong API group",
			}

			mockClient.responses = []llm.ChatResponse{
				// Turn 1: real tool call before the LLM submits its RCA result.
				{
					Message: llm.Message{Role: "assistant", Content: "Investigating..."},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}`},
					},
				},
				// Turn 2: submit_result — apiVersion gate fires (ambiguous kind, no api_version).
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
				}`}},
				// Turn 3: gate retry — LLM provides api_version.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Subscription etcd needs restart",
					"confidence":0.85,
					"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
			}

			resolver := investigator.NewMapperScopeResolver(mapper)
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, ScopeResolver: resolver,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(mockClient.calls)).To(BeNumerically(">=", 3),
				"IT-KA-1935-009: expected at least tool-call turn + submit + gate-retry LLM calls")
			gateRetryReq := mockClient.calls[2]

			sawCall, sawResult := toolCallMessages(gateRetryReq, "kubectl_describe")
			Expect(sawCall).To(BeTrue(),
				"IT-KA-1935-009: apiVersion gate retry must include the earlier kubectl_describe tool_use turn (#1935 root cause #2)")
			Expect(sawResult).To(BeTrue(),
				"IT-KA-1935-009: apiVersion gate retry must include the paired tool_result turn (#1935 root cause #2)")
		})
	})
})
