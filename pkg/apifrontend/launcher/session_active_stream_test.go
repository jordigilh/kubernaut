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

package launcher_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/v2/session"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("session_active status through the A2A stream", func() {
	It("IT-AF-1922-006: preserves status metadata and guidance through ADK and SSE (BR-INTERACTIVE-004)", func() {
		var llmCalls atomic.Int32
		var startCalls atomic.Int32
		llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		defer llmServer.Close()

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				startCalls.Add(1)
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
		server := httptest.NewServer(h)
		defer server.Close()

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

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(rpcBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		bodyStr := string(body)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(startCalls.Load()).To(Equal(int32(1)), "the A2A tool must reach StartInvestigation")
		Expect(bodyStr).To(ContainSubstring("already being driven"),
			"BR-INTERACTIVE-004: rejected callers must receive active-investigation guidance")
		Expect(bodyStr).To(ContainSubstring(`"type":"status"`),
			"BR-INTERACTIVE-004: active-investigation guidance must be a status metadata event")
		Expect(bodyStr).NotTo(ContainSubstring(`"schema":"investigation_summary"`),
			"a rejected caller must not receive a fabricated RCA artifact")
		Expect(llmCalls.Load()).To(BeNumerically(">=", 2))
	})
})
