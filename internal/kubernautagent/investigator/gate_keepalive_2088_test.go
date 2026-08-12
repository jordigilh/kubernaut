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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// firstToolCallStartToolName scans events for the first EventTypeToolCallStart
// and returns its "tool_name" field, or ("", false) if none was emitted.
func firstToolCallStartToolName(events []session.InvestigationEvent) (string, bool) {
	for i := range events {
		if events[i].Type != session.EventTypeToolCallStart {
			continue
		}
		var data map[string]interface{}
		if json.Unmarshal(events[i].Data, &data) != nil {
			continue
		}
		toolName, _ := data["tool_name"].(string)
		return toolName, true
	}
	return "", false
}

// #2088 (main port of #2086, BR-INTERACTIVE-010, FedRAMP SI-4):
// sameKindValidationGate, apiVersionValidationGate, and
// RunRCAExtractionFromConversation each issue a second, non-streamed
// llm.ChatWithParams call that emits zero events to the investigation sink
// for the duration of that LLM round-trip. Live-cluster forensics on
// rr-cc99762025f0-5977eb36 (release/v1.5, #2086) showed this silent gap
// (~87s in the observed incident) exceeds AF's 60s bridge-inactivity timeout
// (bridgeEventsCollectSummary), causing AF to falsely report the interactive
// investigation as "completed" with an empty RCA -- the driving agent then
// never calls discover_workflows. These specs drive a real Investigator
// through its real production event-sink wiring and prove each of the 3
// silent call sites now emits an EventTypeToolCallStart keepalive (which
// resets AF's inactivity timer without polluting the RCA summary) before
// issuing its retry/extraction LLM call.
var _ = Describe("#2088: silent gate-retry LLM calls must emit a keepalive to the investigation sink", func() {

	Describe("UT-KA-2088-001: sameKindValidationGate emits a keepalive during its retry LLM call", func() {
		It("should emit an EventTypeToolCallStart event around the gate-retry LLM call so AF's bridge inactivity timer resets (#2088)", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "api-server-xyz",
				Name:         "api-server-xyz",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "Pod OOMKilled repeatedly",
			}

			mockClient := &gateMockLLMClient{
				responses: []llm.ChatResponse{
					// Turn 1: RCA target kind == signal.ResourceKind ("Pod") -> same-kind gate fires.
					{Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary":"Pod OOMKilled",
						"confidence":0.85,
						"remediation_target":{"kind":"Pod","name":"api-server-xyz","namespace":"production"}
					}`}},
					// Gate retry: LLM re-targets the parent Deployment. This is the silent,
					// non-streamed llm.ChatWithParams call the fix wraps with a keepalive.
					{Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary":"Deployment memory limit too low",
						"confidence":0.85,
						"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production","api_version":"apps/v1"}
					}`}},
					gateWfToolResp(`{"workflow_id":"increase-memory-limit","confidence":0.9}`),
				},
			}

			logger := logr.Discard()
			store := &gateRecordingAuditStore{}
			builder, _ := prompt.NewBuilder()
			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: parser.NewResultParser(),
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			go func() {
				_, _ = inv.Investigate(ctx, signal)
				close(eventCh)
			}()

			events := collectEvents(eventCh)
			toolName, found := firstToolCallStartToolName(events)
			Expect(found).To(BeTrue(),
				"UT-KA-2088-001: sameKindValidationGate's silent retry LLM call must emit a keepalive "+
					"EventTypeToolCallStart event, or AF's bridge-inactivity timer (60s) can starve during "+
					"the LLM round-trip and falsely report the investigation as completed (#2088)")
			Expect(toolName).NotTo(BeEmpty(),
				"UT-KA-2088-001: keepalive event must carry a non-empty tool_name so AF's "+
					"FormatEventForUser can render status text once the tool_name/tool key mismatch is "+
					"fixed (#2090)")
		})
	})

	Describe("UT-KA-2088-002: apiVersionValidationGate emits a keepalive during its retry LLM call", func() {
		It("should emit an EventTypeToolCallStart event around the gate-retry LLM call so AF's bridge inactivity timer resets (#2088)", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "etcd-operator-xyz",
				Name:         "etcd-operator-xyz",
				Namespace:    "demo-operator",
				Severity:     "critical",
				Message:      "RBAC denial on wrong API group",
			}

			mockClient := &gateMockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary":"Subscription etcd needs restart",
						"confidence":0.85,
						"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator"}
					}`}},
					// Gate retry: LLM provides api_version. Silent, non-streamed
					// llm.ChatWithParams call the fix wraps with a keepalive.
					{Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary":"Subscription etcd needs restart",
						"confidence":0.85,
						"remediation_target":{"kind":"Subscription","name":"etcd","namespace":"demo-operator","api_version":"operators.coreos.com/v1alpha1"}
					}`}},
					gateWfToolResp(`{"workflow_id":"restart-sub","confidence":0.9}`),
				},
			}

			logger := logr.Discard()
			store := &gateRecordingAuditStore{}
			builder, _ := prompt.NewBuilder()
			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logger)
			resolver := investigator.NewMapperScopeResolver(newAmbiguousSubscriptionMapper())
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: parser.NewResultParser(),
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(), ScopeResolver: resolver,
			})

			go func() {
				_, _ = inv.Investigate(ctx, signal)
				close(eventCh)
			}()

			events := collectEvents(eventCh)
			toolName, found := firstToolCallStartToolName(events)
			Expect(found).To(BeTrue(),
				"UT-KA-2088-002: apiVersionValidationGate's silent retry LLM call must emit a keepalive "+
					"EventTypeToolCallStart event, or AF's bridge-inactivity timer (60s) can starve during "+
					"the LLM round-trip and falsely report the investigation as completed (#2088)")
			Expect(toolName).NotTo(BeEmpty(),
				"UT-KA-2088-002: keepalive event must carry a non-empty tool_name so AF's "+
					"FormatEventForUser can render status text once the tool_name/tool key mismatch is "+
					"fixed (#2090)")
		})
	})

	Describe("UT-KA-2088-003: RunRCAExtractionFromConversation emits a keepalive during its extraction LLM call", func() {
		It("should emit an EventTypeToolCallStart event around the extraction LLM call so AF's bridge inactivity timer resets (#2088)", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &gateMockLLMClient{
				responses: []llm.ChatResponse{
					// discover_workflows extraction call: silent, non-streamed
					// llm.ChatWithParams call the fix wraps with a keepalive.
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_extract", Name: investigator.SubmitResultToolName, Arguments: `{
								"rca_summary":"OOMKilled",
								"confidence":0.9,
								"remediation_target":{"kind":"Deployment","name":"api","namespace":"ns","api_version":"apps/v1"}
							}`},
						},
					},
				},
			}

			logger := logr.Discard()
			store := &gateRecordingAuditStore{}
			builder, _ := prompt.NewBuilder()
			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: parser.NewResultParser(),
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			messages := []llm.Message{
				{Role: "user", Content: "investigate this pod"},
				{Role: "assistant", Content: "Looking at the pod, it appears to be OOMKilled."},
			}

			result, err := inv.RunRCAExtractionFromConversation(ctx, messages, "corr-2088-003")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			close(eventCh)

			events := collectEvents(eventCh)
			toolName, found := firstToolCallStartToolName(events)
			Expect(found).To(BeTrue(),
				"UT-KA-2088-003: RunRCAExtractionFromConversation's silent extraction LLM call must emit "+
					"a keepalive EventTypeToolCallStart event, or AF's bridge-inactivity timer (60s) can "+
					"starve during the LLM round-trip while an operator is waiting on kubernaut_discover_workflows (#2088)")
			Expect(toolName).NotTo(BeEmpty(),
				"UT-KA-2088-003: keepalive event must carry a non-empty tool_name so AF's "+
					"FormatEventForUser can render status text once the tool_name/tool key mismatch is "+
					"fixed (#2090)")
		})
	})
})
