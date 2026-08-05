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
	"testing"
	"time"

	"github.com/go-logr/logr"

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
// stuck dependency call at the exact tool-dispatch point #1949's RCA named.
type hangingBootstrapTool struct{}

func (hangingBootstrapTool) Name() string                { return hangingBootstrapToolName }
func (hangingBootstrapTool) Description() string         { return "test: hangs forever" }
func (hangingBootstrapTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (hangingBootstrapTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

var _ tools.Tool = hangingBootstrapTool{}

// bootstrapWiringMockLLMClient scripts a tool call on the first turn (to
// the hanging tool above) and a terminal decision on the second.
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

// TestConfigWiring1949_002 (IT-KA-1949-002, BR-KA-267, SC-5): proves
// ai.safety.toolCallTimeout parses from YAML into SafetyConfig.ToolCallTimeout
// (config.Load), and that value bounds a hung tool call once fed into
// investigator.Config.ToolCallTimeout the exact same way main()'s invCfg
// construction does (cmd/kubernautagent/main.go, "ToolCallTimeout:
// cfg.AI.Safety.ToolCallTimeout" in the investigator.Config{...} literal).
//
// main() itself is a single monolithic, unexported function on this branch
// (release/v1.5 predates the bootstrap.go extraction present on main) with
// no callable seam to invoke end-to-end from a test, so this test proves
// the two halves of that exact wiring line directly: YAML-to-struct parsing
// or (config package), and struct-field-to-behavior (investigator.New) —
// rather than a synthetic "call main()" that isn't achievable on this
// branch's structure. Uses plain testing.T, matching this package's
// existing convention on this branch (cmd/kubernautagent has no Ginkgo
// suite bootstrap here; see reload_callback_1470_test.go et al.).
func TestConfigWiring1949_002(t *testing.T) {
	cfgYAML := []byte(`
ai:
  safety:
    toolCallTimeout: 80ms
`)
	cfg, err := kaconfig.Load(cfgYAML)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	if cfg.AI.Safety.ToolCallTimeout != 80*time.Millisecond {
		t.Fatalf("config.Load must parse ai.safety.toolCallTimeout into SafetyConfig.ToolCallTimeout: got %v, want 80ms", cfg.AI.Safety.ToolCallTimeout)
	}

	reg := registry.New()
	reg.Register(hangingBootstrapTool{})

	fakeClient := &bootstrapWiringMockLLMClient{}

	builder, buildErr := prompt.NewBuilder()
	if buildErr != nil {
		t.Fatalf("prompt.NewBuilder failed: %v", buildErr)
	}

	// Mirrors cmd/kubernautagent/main.go's invCfg construction: the exact
	// field-to-field mapping under test is "ToolCallTimeout:
	// cfg.AI.Safety.ToolCallTimeout".
	inv := investigator.New(investigator.Config{
		Client:          fakeClient,
		Builder:         builder,
		ResultParser:    parser.NewResultParser(),
		AuditStore:      audit.NopAuditStore{},
		Logger:          logr.Discard(),
		MaxTurns:        15,
		PhaseTools:      investigator.DefaultPhaseToolMap(),
		Registry:        reg,
		ToolCallTimeout: cfg.AI.Safety.ToolCallTimeout,
	})

	rcaResult := &katypes.InvestigationResult{RCASummary: "OOMKilled", Confidence: 0.9}
	signal := katypes.SignalContext{Name: "test-pod", Namespace: "default", ResourceKind: "Pod", ResourceName: "test-pod"}

	type callResult struct {
		result *katypes.InvestigationResult
		err    error
	}
	done := make(chan callResult, 1)
	go func() {
		result, runErr := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-it-1949-002")
		done <- callResult{result: result, err: runErr}
	}()

	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("RunWorkflowDiscoveryFromRCA returned an unexpected error: %v", out.err)
		}
		if out.result == nil {
			t.Fatal("RunWorkflowDiscoveryFromRCA returned a nil result")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the Investigator built with ToolCallTimeout: cfg.AI.Safety.ToolCallTimeout must bound the hung tool call to the configured 80ms — proving the wiring at main.go's `ToolCallTimeout: cfg.AI.Safety.ToolCallTimeout` line is live, not dead config")
	}
}
