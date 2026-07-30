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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// =============================================================================
// IT-AF-1776-001: BR-SESS-013 Reinvocation Race (issue #1776)
// =============================================================================
//
// Reproduces the structural race between AF's BR-SESS-013 re-invocation loop
// and the real a2a-go consumer goroutine's shutdown against the ACTUAL
// production wiring (real launcher.NewA2AHandler, real a2a-go/ADK executor
// stack) rather than the mockExecutor stub used by the pre-existing
// IT-AF-REINV-W01/W01b tests in streaming_executor_test.go.
//
// Script: turn 1 = tool call (kubernaut_investigate), turn 2 = plain text with
// no further tool call. Per session.NeedsReinvocationCtx, a text-only last
// event on a session unconditionally treated as Active (the phase argument is
// a hardcoded literal at the call site, not dynamically fetched — pre-existing,
// out of scope for this fix) always triggers a re-invocation attempt.

type reinvocationTaskResult struct {
	Status struct {
		State   string          `json:"state"`
		Message json.RawMessage `json:"message,omitempty"`
	} `json:"status"`
	// Artifacts carries the accumulated text content for a synchronous
	// message/send call under OutputArtifactPerEvent mode (see launcher.go's
	// adka2a.ExecutorConfig.OutputMode). The terminal TaskStatusUpdateEvent's
	// own Message field is nil for a genuinely successful completion —
	// ADK only populates it for the failed/input-required cases (see
	// eventProcessor.makeFinalStatusUpdate in
	// server/adka2a/v2/processor.go) — so the reinvoked turn's text must be
	// asserted here, matching the established fpA2ATaskResult/fpA2AArtifact
	// pattern in test/e2e/fullpipeline/af_helpers_test.go.
	Artifacts []reinvocationArtifact `json:"artifacts,omitempty"`
}

type reinvocationArtifact struct {
	Parts []reinvocationArtifactPart `json:"parts"`
}

type reinvocationArtifactPart struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

func reinvocationArtifactText(task reinvocationTaskResult) string {
	var text string
	for _, art := range task.Artifacts {
		for _, p := range art.Parts {
			text += p.Text
		}
	}
	return text
}

type reinvocationRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var _ = Describe("A2A Reinvocation Race (BR-SESS-013, issue #1776)", func() {
	var (
		a2aServer    *httptest.Server
		llmServer    *httptest.Server
		localAuditor *recordingEmitter
	)

	AfterEach(func() {
		if a2aServer != nil {
			a2aServer.Close()
		}
		if llmServer != nil {
			llmServer.Close()
		}
	})

	It("IT-AF-1776-001 [AU-2, AU-3, CC8.1]: reinvocation after a tool-call-then-text turn must not race with a2a-go consumer shutdown, and must record exactly one terminal audit outcome per task", func() {
		localAuditor = newRecordingEmitter()

		var llmCalls atomic.Int32
		llmServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			n := llmCalls.Add(1)
			if n == 1 {
				// Turn 1: tool call — puts the session's last event in a
				// "has tool call" state so the FIRST turn does not trigger
				// reinvocation on its own.
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{
								"functionCall":{
									"name":"kubernaut_investigate",
									"args":{"rr_id":"rr-1776"}
								}
							}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
				return
			}
			// Turn 2+ (including any reinvocation attempt): plain text, no
			// further tool call — the exact shape that triggers BR-SESS-013.
			_, _ = w.Write([]byte(`{
				"candidates":[{
					"content":{
						"role":"model",
						"parts":[{"text":"Investigation complete: rr-1776 processed, no further action needed."}]
					},
					"finishReason":"STOP"
				}],
				"modelVersion":"mock-model"
			}`))
		}))

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-1776-mock",
					Status:    "autonomous_started",
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
			Instruction: "You are a test agent for the reinvocation race repro (#1776).",
			LLMModel:    llmModel,
			MCPClient:   mockMCP,
		})
		Expect(err).NotTo(HaveOccurred())

		sessionSvc := adksession.InMemoryService()
		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:          rootAgent,
			SessionService: sessionSvc,
			AppName:        "kubernaut-apifrontend-it-1776",
			Auditor:        localAuditor,
		})
		Expect(err).NotTo(HaveOccurred())

		a2aServer = httptest.NewServer(h)

		rpcBody, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      "1776-001",
			"method":  "message/send",
			"params": map[string]any{
				"message": map[string]any{
					"messageId": "msg-1776-001",
					"role":      "user",
					"parts": []map[string]any{
						{"kind": "text", "text": "investigate rr-1776"},
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
		Expect(rpcResp.Error).To(BeNil(),
			"message/send must not surface a JSON-RPC error — #1776 manifests as the LLM reinvocation call failing with \"consumer stopped\"")

		var task reinvocationTaskResult
		Expect(json.Unmarshal(rpcResp.Result, &task)).To(Succeed())
		Expect(task.Status.State).To(Equal("completed"),
			"the task must reach a genuine completed state, not failed (#1776: reinvocation races with a2a-go consumer shutdown)")

		text := reinvocationArtifactText(task)
		Expect(text).To(ContainSubstring("Investigation complete: rr-1776 processed"),
			"the accumulated artifact text must contain the *reinvoked* turn's response — "+
				"proving the reinvocation genuinely ran to completion and its output reached the client "+
				"(#1776: pre-fix, the reinvocation's second LLM call raced with a2a-go consumer shutdown "+
				"and its output was lost)")

		// AU-2/AU-3/CC8.1: exactly one terminal audit event must be recorded
		// for this task. The pre-fix code emits BOTH a premature
		// EventA2ATaskCompleted/EventTriageCompleted (turn 1+2, before AF
		// decides reinvocation is needed) AND a subsequent EventA2ATaskFailed
		// (the reinvocation's crash) — a contradictory pair for the same
		// task_id that breaks CC8.1's "reconstruct the complete lifecycle from
		// audit traces alone" guarantee.
		Eventually(func() int {
			return len(localAuditor.EventsOfType(audit.EventA2ATaskCompleted)) +
				len(localAuditor.EventsOfType(audit.EventA2ATaskFailed))
		}, 10*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

		completed := localAuditor.EventsOfType(audit.EventA2ATaskCompleted)
		failed := localAuditor.EventsOfType(audit.EventA2ATaskFailed)
		Expect(len(completed)+len(failed)).To(Equal(1),
			"AU-2/AU-3/CC8.1: exactly one terminal audit event must be recorded per task — "+
				"a completed-then-failed pair for the same task_id breaks CC8.1 reconstruction (#1776)")

		triageCompleted := localAuditor.EventsOfType(audit.EventTriageCompleted)
		Expect(triageCompleted).To(HaveLen(1),
			"CC8.1: exactly one triage.completed event must reflect the true final (post-reinvocation) outcome")
	})
})
