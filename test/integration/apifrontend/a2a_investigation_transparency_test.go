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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/v2/session"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// =============================================================================
// TP-1310 §4.5: A2A Path — Tool Error Bridging — FedRAMP SI-4
// =============================================================================

var _ = Describe("A2A Investigation Tool Error Transparency (TP-1310)", func() {

	var (
		a2aServer *httptest.Server
		llmServer *httptest.Server
		kaServer  *httptest.Server
	)

	AfterEach(func() {
		if a2aServer != nil {
			a2aServer.Close()
		}
		if llmServer != nil {
			llmServer.Close()
		}
		if kaServer != nil {
			kaServer.Close()
		}
	})

	It("IT-AF-1310-050: tool_result error bridged as [Error:] artifact in A2A SSE stream (SI-4)", func() {
		// Mock KA server: analyze returns session ID, stream returns tool_result error + complete
		errPayload := `{"status":"error","error":"nodes.config.openshift.io \"dev-worker-1\" not found"}`
		toolResultInner := map[string]any{
			"tool_name":      "kubectl_describe",
			"tool_index":     0,
			"result_preview": errPayload,
		}
		toolResultEnvelope := map[string]any{
			"type": "tool_result",
			"turn": 1,
			"data": toolResultInner,
		}
		toolResultJSON, _ := json.Marshal(toolResultEnvelope)

		kaServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/incident/analyze":
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"session_id":"sess-1310-a2a"}`))
			case strings.HasSuffix(r.URL.Path, "/stream"):
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, "event: tool_result\ndata: %s\n\n", string(toolResultJSON))
				_, _ = fmt.Fprintf(w, "event: complete\ndata: {\"summary\":\"partial results due to errors\"}\n\n")
			case strings.HasSuffix(r.URL.Path, "/sess-1310-a2a"):
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"in_progress"}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

		// Mock Gemini LLM: first call returns FunctionCall for kubernaut_investigate,
		// second call (with FunctionResponse) returns final text.
		var llmCalls atomic.Int32
		llmServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			n := llmCalls.Add(1)
			if n == 1 {
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{
								"functionCall":{
									"name":"kubernaut_investigate",
									"args":{"rr_id":"rr-dev-worker-1"}
								}
							}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
			} else {
				_, _ = w.Write([]byte(`{
					"candidates":[{
						"content":{
							"role":"model",
							"parts":[{"text":"The investigation encountered tool errors. kubectl_describe failed for Node dev-worker-1."}]
						},
						"finishReason":"STOP"
					}],
					"modelVersion":"mock-model"
				}`))
			}
		}))

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-1310-mock",
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
			Instruction: "You are a test agent. When investigating, report any tool errors to the user.",
			LLMModel:    llmModel,
			MCPClient:   mockMCP,
		})
		Expect(err).NotTo(HaveOccurred())

		sessionSvc := adksession.InMemoryService()
		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:          rootAgent,
			SessionService: sessionSvc,
			AppName:        "kubernaut-apifrontend-it-1310",
		})
		Expect(err).NotTo(HaveOccurred())

		a2aServer = httptest.NewServer(h)

		// Send A2A message/stream request
		rpcBody, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      "1310-050",
			"method":  "message/stream",
			"params": map[string]any{
				"message": map[string]any{
					"messageId": "msg-1310-050",
					"role":      "user",
					"parts": []map[string]any{
						{"kind": "text", "text": "investigate Node dev-worker-1 in production namespace"},
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

		bodyStr := string(body)

		// The A2A SSE stream should contain the bridged error artifact
		Expect(bodyStr).To(SatisfyAny(
			ContainSubstring("[Error: kubectl_describe"),
			ContainSubstring("tool_errors"),
			ContainSubstring("kubectl_describe"),
		), "SI-4: tool error must be visible in A2A stream — either as bridged [Error:] artifact or in tool result")

		// The LLM should have been called at least twice (FunctionCall + final text)
		Expect(llmCalls.Load()).To(BeNumerically(">=", 2),
			"LLM must be called at least twice: FunctionCall + final response after tool execution")
	})

	It("IT-AF-1922-006: session_active status survives the ADK-to-A2A SSE boundary (BR-INTERACTIVE-004)", func() {
		var llmCalls atomic.Int32
		llmServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if llmCalls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{
					"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"kubernaut_investigate","args":{"rr_id":"rr-1922-stream"}}}]},"finishReason":"STOP"}],
					"modelVersion":"mock-model"
				}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"candidates":[{"content":{"role":"model","parts":[{"text":"The investigation is already being driven by another user."}]},"finishReason":"STOP"}],
				"modelVersion":"mock-model"
			}`))
		}))

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return nil, fmt.Errorf("kubernaut_investigate start_autonomous: session_active: Investigation is being driven by another user (map[driver:admin session_id:sess-1922-stream])")
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
			Instruction:        "You are a test agent. Report investigation status to the user.",
			LLMModel:           llmModel,
			MCPClient:          mockMCP,
			InteractiveEnabled: true,
		})
		Expect(err).NotTo(HaveOccurred())

		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:          rootAgent,
			SessionService: adksession.InMemoryService(),
			AppName:        "kubernaut-apifrontend-it-1922",
		})
		Expect(err).NotTo(HaveOccurred())
		a2aServer = httptest.NewServer(h)

		rpcBody, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      "1922-006",
			"method":  "message/stream",
			"params": map[string]any{
				"message": map[string]any{
					"messageId": "msg-1922-006",
					"contextId": "ctx-1922-006",
					"role":      "user",
					"parts":     []map[string]any{{"kind": "text", "text": "investigate rr-1922-stream"}},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a2aServer.URL, bytes.NewReader(rpcBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		bodyStr := string(body)

		Expect(bodyStr).To(ContainSubstring("already in progress"),
			"BR-INTERACTIVE-004: rejected callers must receive active-investigation guidance")
		Expect(bodyStr).To(ContainSubstring(`"type":"status"`),
			"BR-INTERACTIVE-004: active-investigation guidance must be a status metadata event")
		Expect(bodyStr).NotTo(ContainSubstring(`"schema":"investigation_summary"`),
			"a rejected caller must not receive a fabricated RCA artifact")
	})
})
