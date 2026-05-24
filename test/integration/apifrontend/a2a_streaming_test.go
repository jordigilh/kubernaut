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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("A2A Progressive Streaming Integration (issue #1258)", func() {

	var (
		a2aServer    *httptest.Server
		mockLLMSrv   *httptest.Server
		localAuditor *recordingEmitter
	)

	BeforeEach(func() {
		localAuditor = newRecordingEmitter()

		mockLLMSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Investigation complete. No issues found."}]},"finishReason":"STOP"}],"modelVersion":"mock-model"}`))
		}))

		ctx := context.Background()
		llmModel, err := launcher.NewModelFromConfig(ctx, config.LLMConfig{
			Provider: config.LLMProviderGemini,
			Model:    "mock-model",
			Endpoint: mockLLMSrv.URL,
			APIKey:   "test-key",
		})
		Expect(err).NotTo(HaveOccurred())

		rootAgent, _, err := agentpkg.NewRootAgent(agentpkg.AgentConfig{
			Instruction: "You are a test agent for integration tests.",
			LLMModel:    llmModel,
		})
		Expect(err).NotTo(HaveOccurred())

		sessionSvc := adksession.InMemoryService()
		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:          rootAgent,
			SessionService: sessionSvc,
			AppName:        "kubernaut-apifrontend-it",
			Auditor:        localAuditor,
		})
		Expect(err).NotTo(HaveOccurred())

		a2aServer = httptest.NewServer(h)
	})

	AfterEach(func() {
		if a2aServer != nil {
			a2aServer.Close()
		}
		if mockLLMSrv != nil {
			mockLLMSrv.Close()
		}
	})

	buildStreamRPC := func(id, text string) []byte {
		body, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  "message/stream",
			"params": map[string]any{
				"message": map[string]any{
					"messageId": "msg-" + id,
					"role":      "user",
					"parts": []map[string]any{
						{"kind": "text", "text": text},
					},
				},
			},
		})
		return body
	}

	buildSendRPC := func(id, text string) []byte {
		body, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  "message/send",
			"params": map[string]any{
				"message": map[string]any{
					"messageId": "msg-" + id,
					"role":      "user",
					"parts": []map[string]any{
						{"kind": "text", "text": text},
					},
				},
			},
		})
		return body
	}

	invokeA2A := func(body []byte) (*http.Response, error) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, a2aServer.URL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return http.DefaultClient.Do(req)
	}

	// ===================================================================
	// FedRAMP AU-2/AU-3: Audit Method Detection
	// Verifies that the A2A handler correctly identifies and records
	// the JSON-RPC method (message/stream vs message/send) in audit events.
	// ===================================================================

	Describe("IT-AF-1258-001: AU-2/AU-3 message/stream produces audit with method=message/stream", func() {
		It("should emit audit event with detail.method = message/stream", func() {
			localAuditor.Reset()

			resp, err := invokeA2A(buildStreamRPC("audit-stream-01", "list pods"))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			_, _ = io.ReadAll(resp.Body)

			Eventually(func() []*audit.Event {
				return localAuditor.EventsOfType(audit.EventA2ATaskStarted)
			}, 10*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty())

			events := localAuditor.EventsOfType(audit.EventA2ATaskStarted)
			Expect(events).NotTo(BeEmpty())
			Expect(events[0].Detail).To(HaveKeyWithValue("method", "message/stream"))
		})
	})

	Describe("IT-AF-1258-002: AU-2/AU-3 message/send produces audit with method=message/send", func() {
		It("should emit audit event with detail.method = message/send", func() {
			localAuditor.Reset()

			resp, err := invokeA2A(buildSendRPC("audit-send-01", "list pods"))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			_, _ = io.ReadAll(resp.Body)

			Eventually(func() []*audit.Event {
				return localAuditor.EventsOfType(audit.EventA2ATaskStarted)
			}, 10*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty())

			events := localAuditor.EventsOfType(audit.EventA2ATaskStarted)
			Expect(events).NotTo(BeEmpty())
			Expect(events[0].Detail).To(HaveKeyWithValue("method", "message/send"))
		})
	})

	// ===================================================================
	// FedRAMP AU-6: A2A Task Lifecycle Audit
	// Stream lifecycle (open/close) is logged, not audited, because no OpenAPI
	// payload schema exists for stream events yet. The A2A task lifecycle
	// (task_started/completed/failed) IS audited and provides the forensic trail.
	// ===================================================================

	Describe("IT-AF-1258-003: AU-6 stream request emits task_started and task audit events", func() {
		It("should emit task lifecycle audit events for a successful stream", func() {
			localAuditor.Reset()

			resp, err := invokeA2A(buildStreamRPC("lifecycle-01", "hello"))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			_, _ = io.ReadAll(resp.Body)

			Eventually(func() []*audit.Event {
				completed := localAuditor.EventsOfType(audit.EventA2ATaskCompleted)
				failed := localAuditor.EventsOfType(audit.EventA2ATaskFailed)
				return append(completed, failed...)
			}, 10*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty())

			started := localAuditor.EventsOfType(audit.EventA2ATaskStarted)
			Expect(started).NotTo(BeEmpty(), "task_started must be emitted")
			Expect(started[0].Detail).To(HaveKeyWithValue("task_id", Not(BeEmpty())))
		})
	})

	Describe("IT-AF-1258-004: AU-6 task_failed emitted on LLM failure (graceful lifecycle)", func() {
		It("should emit task_failed when LLM returns 500", func() {
			if mockLLMSrv != nil {
				mockLLMSrv.Close()
			}

			failLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":500,"message":"LLM unavailable"}}`))
			}))
			defer failLLM.Close()

			ctx := context.Background()
			llmModel, err := launcher.NewModelFromConfig(ctx, config.LLMConfig{
				Provider: config.LLMProviderGemini,
				Model:    "mock-model",
				Endpoint: failLLM.URL,
				APIKey:   "test-key",
			})
			Expect(err).NotTo(HaveOccurred())

			rootAgent, _, err := agentpkg.NewRootAgent(agentpkg.AgentConfig{
				Instruction: "You are a test agent that will encounter LLM errors.",
				LLMModel:    llmModel,
			})
			Expect(err).NotTo(HaveOccurred())

			failAuditor := newRecordingEmitter()
			h, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: adksession.InMemoryService(),
				AppName:        "kubernaut-apifrontend-it-fail",
				Auditor:        failAuditor,
			})
			Expect(err).NotTo(HaveOccurred())

			failServer := httptest.NewServer(h)
			defer failServer.Close()

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, failServer.URL, bytes.NewReader(buildStreamRPC("fail-01", "trigger error")))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			_, _ = io.ReadAll(resp.Body)

			Eventually(func() []*audit.Event {
				return failAuditor.EventsOfType(audit.EventA2ATaskFailed)
			}, 10*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty())

			failed := failAuditor.EventsOfType(audit.EventA2ATaskFailed)
			Expect(failed).NotTo(BeEmpty(), "task_failed must be emitted on LLM failure")
			Expect(failed[0].Detail).To(HaveKeyWithValue("task_id", Not(BeEmpty())),
				"AU-6: task lifecycle tracked regardless of execution outcome")
		})
	})
})
