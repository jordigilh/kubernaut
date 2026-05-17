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

package mcp_test

import (
	"context"
	"net/http/httptest"

	"github.com/go-logr/logr"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
)

var _ = Describe("Interactive Session Compatibility — COMPAT BR-INTERACTIVE-007", Label("integration", "interactive", "compat"), func() {

	Describe("IT-KA-COMPAT-01: InvestigateTool works without optional dependencies", func() {
		It("should function correctly without rate limiter, timeout tracker, or notifier", func() {
			nsName := uniqueNamespace("compat01")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			logrLogger := logr.Discard()

			// Real LLM client via langchaingo -> Podman Mock LLM
			llmAdapter, err := langchaingo.New("openai", sharedMockLLMEndpoint, "test-model", "test-key")
			Expect(err).NotTo(HaveOccurred())

			promptBuilder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			inv := investigator.New(investigator.Config{
				Client:       llmAdapter,
				Builder:      promptBuilder,
				ResultParser: parser.NewResultParser(),
				AuditStore:   audit.NopAuditStore{},
				Logger:       logrLogger,
				MaxTurns:     15,
				ModelName:    "test-model",
			})
			runner := adapters.NewInvestigatorRunnerAdapter(inv)

			// Real DSContextReconstructor via ogenclient -> Podman DataStorage
			Expect(sharedDSClient).NotTo(BeNil(), "shared DS client must be initialized by suite")
			recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logrLogger)

			sessMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logrLogger)

			investigateTool := tools.NewInvestigateTool(sessMgr, runner, recon, tools.NopAutonomousManager{})

			toolDeps := mcpinternal.ToolDeps{
				Investigate: tools.InvestigateRegistration(investigateTool, nil, nil),
			}

			handler, _ := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
				AuthMiddleware: fakeAuthMiddlewareWithUserInfo,
				Tools:          toolDeps,
			})

			r := chi.NewRouter()
			r.Use(fakeAuthMiddlewareWithUserInfo)
			r.Handle("/mcp", handler)
			r.Handle("/mcp/*", handler)
			ts := httptest.NewServer(r)
			defer ts.Close()

			session, err := connectMCP(ts, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			By("start")
			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-compat-01",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("message")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":   "rr-compat-01",
				"action":  "message",
				"message": "Test without optional deps",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["response"]).NotTo(BeEmpty())

			By("complete")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-compat-01",
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
		})
	})

	Describe("IT-KA-COMPAT-02: max sessions limit enforced", func() {
		It("should reject start when maxSessions is reached", func() {
			nsName := uniqueNamespace("compat02")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.maxSessions = 1
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			By("starting first session (should succeed)")
			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-compat-02-a",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("starting second session (should fail with max_sessions)")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-compat-02-b",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue())
		})
	})

	Describe("IT-KA-COMPAT-03: invalid action returns structured error", func() {
		It("should return validation error for unknown action", func() {
			nsName := uniqueNamespace("compat03")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			stack := newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-compat-03",
				"action": "invalid_action",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue())
		})
	})
})
