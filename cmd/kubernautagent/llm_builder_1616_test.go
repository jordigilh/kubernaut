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

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Per-Phase Reasoning/Effort Overrides — #1616, BR-AI-086: wire-level wiring
// proofs. Unlike UT-AI-1616-001/006 (which prove the config merge in
// isolation), these prove the value flows end-to-end through the real
// production entry points (buildLLMClients, buildAlignmentStack) all the
// way to the outgoing LLM HTTP request body — the CHECKPOINT W gate.
var _ = Describe("Per-Phase Reasoning/Effort Overrides — wire-level wiring proofs (#1616)", func() {

	Describe("IT-KA-1616-002 (CM-6): a phase override's Reasoning reaches the real outgoing LLM request via buildLLMClients", func() {
		var (
			server       *httptest.Server
			receivedBody map[string]interface{}
		)

		BeforeEach(func() {
			receivedBody = nil
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("sends the phase override's reasoning.effort on the wire, distinct from the base's", func() {
			cfg := kaconfig.DefaultConfig()
			cfg.AI.LLM.Provider = types.LLMProviderOpenAICompatible
			cfg.AI.LLM.Reasoning = &types.LLMReasoningConfig{Enabled: true, Effort: "low"}

			llmRuntime := &kaconfig.LLMRuntimeConfig{
				// gpt-5: a real reasoning-effort-dialect model name
				// (openaicompat.DetectEffortDialect), required for
				// reasoning_effort to appear on the wire at all — a bogus
				// model name would silently omit it regardless of config
				// (BR-AI-086's compatibility-floor default).
				Model:    "gpt-5",
				Endpoint: server.URL,
				PhaseModels: map[string]*kaconfig.LLMOverrideConfig{
					"rca": {
						Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "high"},
					},
				},
			}

			_, phaseSwappables := buildLLMClients(cfg, llmRuntime, testReloadLogger())
			rcaClient := phaseSwappables[katypes.PhaseRCA]
			Expect(rcaClient).NotTo(BeNil())

			_, err := rcaClient.Chat(context.Background(), helloChatRequest())
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedBody["reasoning_effort"]).To(Equal("high"),
				"phaseModels.rca.reasoning.effort must reach the outgoing request, overriding the base's ai.llm.reasoning.effort")
		})
	})

	Describe("IT-KA-1616-003 (CM-6): ai.alignmentCheck.llm.reasoning reaches the real outgoing shadow-LLM request via buildAlignmentStack", func() {
		var (
			server       *httptest.Server
			receivedBody map[string]interface{}
		)

		BeforeEach(func() {
			receivedBody = nil
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"{\"suspicious\":false}"},"finish_reason":"stop"}]}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("sends the shadow override's reasoning.effort on the wire", func() {
			cfg := kaconfig.DefaultConfig()
			cfg.AI.LLM.Provider = types.LLMProviderOpenAICompatible
			cfg.AI.AlignmentCheck = kaconfig.AlignmentCheckConfig{
				Enabled:        true,
				Mode:           kaconfig.AlignmentModeEnforce,
				Timeout:        5 * time.Second,
				MaxStepTokens:  500,
				MaxRetries:     1,
				VerdictTimeout: 5 * time.Second,
				LLM: &kaconfig.LLMOverrideConfig{
					Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "medium"},
				},
			}

			llmRuntime := &kaconfig.LLMRuntimeConfig{
				Model:    "gpt-5", // see IT-KA-1616-002 comment on model-name effort-dialect detection
				Endpoint: server.URL,
			}

			_, _, alignEvaluator, alignCfg := buildAlignmentStack(
				cfg, llmRuntime, &stubLLMClient{}, registry.New(), audit.NopAuditStore{}, testReloadLogger())
			Expect(alignCfg.Enabled).To(BeTrue())
			Expect(alignEvaluator).NotTo(BeNil())

			_ = alignEvaluator.EvaluateStep(context.Background(), alignment.Step{
				Index:   0,
				Kind:    alignment.StepKindToolResult,
				Content: "hello",
			})

			Expect(receivedBody["reasoning_effort"]).To(Equal("medium"),
				"ai.alignmentCheck.llm.reasoning.effort must reach the outgoing shadow-LLM request")
		})
	})
})
