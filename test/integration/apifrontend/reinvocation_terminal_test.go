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
// IT-AF-1912-001: driverActive cleared on session-terminal tool, real wiring
// (issue #1912)
// =============================================================================
//
// Reproduces #1912 against the ACTUAL production wiring (real
// agentpkg.NewRootAgent -> phaseGuardAfter, real launcher.NewA2AHandler),
// mirroring IT-AF-1776-001's pattern in reinvocation_race_test.go. Reuses
// that file's reinvocationTaskResult/reinvocationRPCResponse/
// reinvocationArtifactText helpers (same apifrontend_test package) rather
// than redefining them.
//
// Script (single message/send call, three model turns):
//  1. tool call kubernaut_investigate (interaction_mode=full_remediation_autonomous,
//     so Phase2Blocked=false and, since discover_workflows is never called,
//     Phase3Blocked is never set — the ONLY thing that could still gate a
//     later stray reinvocation is driverActive itself).
//  2. tool call kubernaut_complete (a session-terminal tool, #1912's target) --
//     pre-fix, this leaves driverActive stuck true; post-fix, phaseGuardAfter
//     now clears it alongside the ActiveContextRegistry entry.
//  3. plain text, no further tool call ("Investigation resolved and closed.")
//     — the natural end of this turn's ReAct loop. Immediately afterward,
//     the A2A/session layer evaluates NeedsReinvocationCtx against the
//     now-terminal state: pre-fix (driverActive stuck true, no checkpoint
//     block) it incorrectly re-invokes the model a 4th time with the
//     synthetic "continue investigating" nudge, resurrecting a session the
//     user already closed; post-fix (driverActive correctly false) it does
//     not.
var _ = Describe("A2A Reinvocation After Session-Terminal Tool (BR-SESS-020/022, issue #1912)", func() {
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

	It("IT-AF-1912-001 [AC-2, AC-6]: does not reinvoke after kubernaut_complete ends the driver session, even though no checkpoint remained blocked", func() {
		var llmCalls atomic.Int32
		llmServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			n := llmCalls.Add(1)
			switch n {
			case 1:
				// Turn 1: kubernaut_investigate, full_remediation_autonomous
				// (no checkpoint left blocked once the driver session ends).
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{
								"functionCall":{
									"name":"kubernaut_investigate",
									"args":{"rr_id":"rr-1912","interaction_mode":"full_remediation_autonomous"}
								}
							}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
			case 2:
				// Turn 2: kubernaut_complete — the session-terminal tool
				// that must clear driverActive (#1912's fix target).
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{
								"functionCall":{
									"name":"kubernaut_complete",
									"args":{"rr_id":"rr-1912"}
								}
							}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
			default:
				// Turn 3 (expected final turn) and any illegitimate
				// reinvocation attempt beyond it: plain text, no tool call.
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{"text":"Investigation resolved and closed."}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
			}
		}))

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-1912-mock",
					Status:    "autonomous_started",
				}, nil
			},
			InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
				Expect(args.Action).To(Equal("complete"))
				return &ka.InvokeActionResult{
					SessionID: "sess-1912-mock",
					Status:    "completed",
				}, nil
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
			Instruction: "You are a test agent for the #1912 driverActive-clearing repro.",
			LLMModel:    llmModel,
			MCPClient:   mockMCP,
		})
		Expect(err).NotTo(HaveOccurred())

		sessionSvc := adksession.InMemoryService()
		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:          rootAgent,
			SessionService: sessionSvc,
			AppName:        "kubernaut-apifrontend-it-1912",
		})
		Expect(err).NotTo(HaveOccurred())

		a2aServer = httptest.NewServer(h)

		rpcBody, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      "1912-001",
			"method":  "message/send",
			"params": map[string]any{
				"message": map[string]any{
					"messageId": "msg-1912-001",
					"role":      "user",
					"parts": []map[string]any{
						{"kind": "text", "text": "investigate and complete rr-1912"},
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
		Expect(text).To(ContainSubstring("Investigation resolved and closed."),
			"the final artifact must be turn 3's genuine closing text")

		// The crux of #1912: exactly 3 LLM calls (investigate, complete,
		// closing text) — no 4th reinvocation call. Pre-fix, driverActive
		// stayed stuck true after kubernaut_complete with no checkpoint left
		// blocking it (full_remediation_autonomous + discover_workflows
		// never called), so NeedsReinvocationCtx incorrectly returned true
		// and the harness synthesized a "continue the investigation" nudge,
		// producing (at least) a 4th call.
		Expect(llmCalls.Load()).To(Equal(int32(3)),
			"#1912: a session already ended by kubernaut_complete must never be reinvoked back to life "+
				"by a later text-only turn, even when no DD-AF-011 checkpoint remained blocked")
	})
})
