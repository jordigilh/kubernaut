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
package apifrontend_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// =============================================================================
// IT-AF-1918-001: Harness-Enforced Actionability Gate, real wiring (issue #1918)
// =============================================================================
//
// Reproduces the #1918 guarded gate against the ACTUAL production wiring
// (real agentpkg.NewRootAgent -> phaseGuardAfter, real launcher.NewA2AHandler),
// mirroring IT-AF-1912-001's pattern in reinvocation_terminal_test.go.
//
// Script (single message/send call, three model turns):
//  1. tool call kubernaut_investigate (interaction_mode=full_remediation_autonomous
//     -- an autonomy grant that would normally leave Phase2Blocked=false).
//     The mock KA MCP session streams a single EventTypeComplete event whose
//     Data carries is_actionable=false (RCA concluded no remediation is
//     warranted), matching what a genuine "problem_resolved"/
//     "predictive_no_action" investigation would surface (mirrors
//     investigator.go's own IsActionable computation).
//  2. tool call kubernaut_discover_workflows -- simulating a lower-reasoning
//     model that ignores/misreads the RCA and tries to proceed anyway despite
//     the not-actionable finding. Pre-#1918-fix, full_remediation_autonomous
//     alone would leave Phase2Blocked=false and this call would reach KA's
//     real discover_workflows implementation. Post-fix, phaseGuardAfter's
//     #1918 override already forced Phase2Blocked=true when kubernaut_investigate
//     returned, so phaseGuardBefore's existing DD-AF-011 hard-reject
//     (checkpointGatedTools) rejects this call before it ever reaches KA --
//     proven here by wiring DiscoverWorkflowsFn to fail the test if invoked.
//  3. plain text, no further tool call ("Acknowledged -- no remediation
//     needed.") -- the natural end of this turn's ReAct loop after receiving
//     the hard-reject error as the tool result.
var _ = Describe("AF Harness-Enforced Actionability Gate (BR-INTERACTIVE-010, issue #1918)", func() {
	var (
		a2aServer *httptest.Server
		llmServer *httptest.Server
	)

	AfterEach(func() {
		if a2aServer != nil {
			a2aServer.Close()
		}
		if llmServer != nil {
			llmServer.Close()
		}
	})

	It("IT-AF-1918-001 [AC-6, SI-10]: forces phase2_blocked and hard-rejects discover_workflows when KA's RCA is not actionable, even under full_remediation_autonomous", func() {
		var llmCalls atomic.Int32
		llmServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			n := llmCalls.Add(1)
			switch n {
			case 1:
				// Turn 1: kubernaut_investigate, full_remediation_autonomous
				// (would normally leave Phase2Blocked=false).
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{
								"functionCall":{
									"name":"kubernaut_investigate",
									"args":{"rr_id":"rr-1918","interaction_mode":"full_remediation_autonomous"}
								}
							}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
			case 2:
				// Turn 2: kubernaut_discover_workflows attempted despite the
				// not-actionable RCA -- must be hard-rejected by the #1918 gate.
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{
								"functionCall":{
									"name":"kubernaut_discover_workflows",
									"args":{"rr_id":"rr-1918"}
								}
							}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
			default:
				// Turn 3 (expected final turn): plain text, no tool call.
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{"text":"Acknowledged -- no remediation needed."}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
			}
		}))

		events := make(chan ka.InvestigationEvent, 1)
		rcaJSON, err := json.Marshal(map[string]any{
			"severity":      "info",
			"confidence":    0.9,
			"rca_summary":   "Problem self-resolved after pod restart",
			"is_actionable": false,
		})
		Expect(err).NotTo(HaveOccurred())
		events <- ka.InvestigationEvent{Type: ka.EventTypeComplete, Data: rcaJSON}
		close(events)

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-1918-mock",
					Status:    "autonomous_started",
					Events:    events,
				}, nil
			},
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				Fail("#1918: kubernaut_discover_workflows must never reach KA -- phaseGuardBefore's hard-reject " +
					"must stop it once phaseGuardAfter's #1918 override forced phase2_blocked=true")
				return nil, nil
			},
		}

		ctx := context.Background()
		llmModel, err := launcher.NewModelFromConfig(ctx, types.LLMConfig{
			Provider: types.LLMProviderGemini,
			Model:    "mock-model",
			Endpoint: llmServer.URL,
			APIKey:   "test-key",
		})
		Expect(err).NotTo(HaveOccurred())

		rootAgent, _, err := agentpkg.NewRootAgent(agentpkg.AgentConfig{
			Instruction: "You are a test agent for the #1918 actionability-gate repro.",
			LLMModel:    llmModel,
			MCPClient:   mockMCP,
		})
		Expect(err).NotTo(HaveOccurred())

		sessionSvc := adksession.InMemoryService()
		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:          rootAgent,
			SessionService: sessionSvc,
			AppName:        "kubernaut-apifrontend-it-1918",
		})
		Expect(err).NotTo(HaveOccurred())

		a2aServer = httptest.NewServer(h)

		rpcBody, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      "1918-001",
			"method":  "message/send",
			"params": map[string]any{
				"message": map[string]any{
					"messageId": "msg-1918-001",
					"role":      "user",
					"parts": []map[string]any{
						{"kind": "text", "text": "investigate and fix rr-1918 autonomously"},
					},
				},
			},
		})

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a2aServer.URL, bytes.NewReader(rpcBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())

		var rpcResp reinvocationRPCResponse
		Expect(json.Unmarshal(body, &rpcResp)).To(Succeed())
		Expect(rpcResp.Error).To(BeNil(), "message/send must not surface a JSON-RPC error")

		var task reinvocationTaskResult
		Expect(json.Unmarshal(rpcResp.Result, &task)).To(Succeed())
		Expect(task.Status.State).To(Equal("completed"))

		text := reinvocationArtifactText(task)
		Expect(text).To(ContainSubstring("Acknowledged -- no remediation needed."),
			"the final artifact must be turn 3's genuine closing text, reached gracefully after the hard-reject")

		// The crux of #1918: exactly 3 LLM calls (investigate, rejected
		// discover_workflows attempt, closing text) -- the model never gets
		// a successful discover_workflows result to act on, because
		// DiscoverWorkflowsFn (which would fail this test if invoked) is
		// never reached.
		Expect(llmCalls.Load()).To(Equal(int32(3)),
			"#1918: a not-actionable RCA under full_remediation_autonomous must never let discover_workflows "+
				"reach KA, regardless of the model's own reading of the RCA narrative")
	})
})
