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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// hangingBootstrapToolName is the real Phase-3 tool name that both the fake
// tool and the scripted LLM response below reference, so the LLM's tool
// call and the registry lookup agree on which (hanging) implementation
// gets dispatched.
const hangingBootstrapToolName = "kubectl_describe"

// hangingBootstrapTool blocks on ctx.Done() forever, standing in for a
// stuck dependency call at the exact tool-dispatch point #1949's RCA named
// (Wiring Manifest: cmd/kubernautagent/bootstrap.go's buildInvestigator).
type hangingBootstrapTool struct{}

func (hangingBootstrapTool) Name() string               { return hangingBootstrapToolName }
func (hangingBootstrapTool) Description() string        { return "test: hangs forever" }
func (hangingBootstrapTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (hangingBootstrapTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

var _ tools.Tool = hangingBootstrapTool{}

// bootstrapWiringMockLLMClient scripts a tool call on the first turn (to
// the hanging tool above) and a terminal sentinel decision on the second,
// mirroring pkg's gateMockLLMClient/gateWfToolResp pattern used across the
// investigator package's own characterization tests.
type bootstrapWiringMockLLMClient struct {
	calls int
}

func (m *bootstrapWiringMockLLMClient) Close() error { return nil }

func (m *bootstrapWiringMockLLMClient) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return m.Chat(ctx, req)
}

func (m *bootstrapWiringMockLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	m.calls++
	if m.calls == 1 {
		return llm.ChatResponse{
			Message:   llm.Message{Role: "assistant", Content: ""},
			ToolCalls: []llm.ToolCall{{ID: "tc_1949_002", Name: hangingBootstrapToolName, Arguments: "{}"}},
		}, nil
	}
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: ""},
		ToolCalls: []llm.ToolCall{
			{ID: "tc_wf", Name: "submit_result_with_workflow", Arguments: `{"workflow_id":"noop","confidence":0.5}`},
		},
	}, nil
}

// Describes the #1949 Wiring Manifest row: "Component: ToolCallTimeout
// config plumbing — Production entry point: cmd/kubernautagent/bootstrap.go
// investigator.Config{...} construction — Wiring code: buildInvestigator
// (~line 402) — IT test ID: IT-KA-1949-002".
var _ = Describe("cmd/kubernautagent buildInvestigator — ToolCallTimeout config-to-behavior wiring (#1949, BR-KA-267, SC-5)", func() {

	Describe("IT-KA-1949-002: a ToolCallTimeout configured via YAML actually bounds a hanging tool call through buildInvestigator", func() {
		It("parses ai.safety.toolCallTimeout from YAML and wires it into the Investigator built by buildInvestigator, aborting a hung real tool call within that exact duration", func() {
			cfgYAML := []byte(`
ai:
  safety:
    toolCallTimeout: 80ms
`)
			cfg, err := kaconfig.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.Safety.ToolCallTimeout).To(Equal(80*time.Millisecond),
				"config.Load must parse ai.safety.toolCallTimeout into SafetyConfig.ToolCallTimeout")

			reg := registry.New()
			reg.Register(hangingBootstrapTool{})

			builder, buildErr := prompt.NewBuilder()
			Expect(buildErr).NotTo(HaveOccurred())

			// A real (non-nil) SwappableClient wrapping the fake LLM client:
			// buildPhaseResolver below always returns a concrete, non-nil
			// *investigator.DefaultPhaseResolver, whose ResolvePhase calls
			// sw.Pin() unconditionally on the default SwappableClient — a
			// nil one would itself panic, independent of anything #1949
			// touches, so this must be wired the same way main() wires it.
			fakeClient := &bootstrapWiringMockLLMClient{}
			swappable, swErr := llm.NewSwappableClient(fakeClient, "test-model")
			Expect(swErr).NotTo(HaveOccurred())

			params := investigationRunnerParams{
				cfg:           cfg,
				llmRuntime:    &kaconfig.LLMRuntimeConfig{Model: "test-model"},
				swappable:     swappable,
				promptBuilder: builder,
				resultParser:  parser.NewResultParser(),
				phaseTools:    investigator.DefaultPhaseToolMap(),
				effectiveLLM:  fakeClient,
				effectiveReg:  reg,
				logger:        logr.Discard(),
			}

			// buildPhaseResolver + buildInvestigator are the exact two
			// production functions bootstrap.go's main() calls in sequence
			// to construct the real Investigator (see buildInvestigationRunner).
			scopeResolver, phaseResolver := buildPhaseResolver(params, nil)
			inv := buildInvestigator(params, nil, audit.NopAuditStore{}, scopeResolver, phaseResolver, nil)
			Expect(inv).NotTo(BeNil())

			rcaResult := &katypes.InvestigationResult{RCASummary: "OOMKilled", Confidence: 0.9}
			signal := katypes.SignalContext{Name: "test-pod", Namespace: "default", ResourceKind: "Pod", ResourceName: "test-pod"}

			type callResult struct {
				result *katypes.InvestigationResult
				err    error
			}
			done := make(chan callResult, 1)
			go func() {
				defer GinkgoRecover()
				result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-it-1949-002")
				done <- callResult{result: result, err: err}
			}()

			var out callResult
			Eventually(done, "2s", "10ms").Should(Receive(&out),
				"the Investigator built by buildInvestigator from YAML-loaded config must bound the hung tool call to ai.safety.toolCallTimeout — proving the wiring at bootstrap.go's `ToolCallTimeout: p.cfg.AI.Safety.ToolCallTimeout` line is live, not dead config")

			Expect(out.err).NotTo(HaveOccurred())
			Expect(out.result).NotTo(BeNil())
		})
	})
})
