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

package alignment_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// stubTool implements tools.Tool for testing.
type stubTool struct {
	name string
}

func (s *stubTool) Name() string                                                 { return s.name }
func (s *stubTool) Description() string                                          { return s.name + " desc" }
func (s *stubTool) Parameters() json.RawMessage                                  { return json.RawMessage(`{}`) }
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (string, error) { return "", nil }

// mockLLMClient implements llm.Client for testing.
// Thread-safe: all fields are guarded by mu to support concurrent SubmitAsync.
type mockLLMClient struct {
	mu        sync.Mutex
	responses []llm.ChatResponse
	errs      []error
	call      int

	capturedRequestContents []string
}

func (m *mockLLMClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var b strings.Builder
	for _, msg := range req.Messages {
		b.WriteString(msg.Content)
	}
	m.capturedRequestContents = append(m.capturedRequestContents, b.String())

	if m.call < len(m.errs) && m.errs[m.call] != nil {
		err := m.errs[m.call]
		m.call++
		return llm.ChatResponse{}, err
	}
	if m.call < len(m.responses) {
		r := m.responses[m.call]
		m.call++
		return r, nil
	}
	m.call++
	return llm.ChatResponse{}, nil
}

func (m *mockLLMClient) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := m.Chat(ctx, req)
	if err == nil {
		_ = cb(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (m *mockLLMClient) Close() error { return nil }

func (m *mockLLMClient) chatCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.call
}

// slowMockLLMClient adds a delay before responding, used to test timeout behavior.
type slowMockLLMClient struct {
	delay time.Duration
}

func (m *slowMockLLMClient) Close() error { return nil }

func (m *slowMockLLMClient) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := m.Chat(ctx, req)
	if err == nil {
		_ = cb(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (m *slowMockLLMClient) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	select {
	case <-time.After(m.delay):
		return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"suspicious":false}`}}, nil
	case <-ctx.Done():
		return llm.ChatResponse{}, ctx.Err()
	}
}

// mockToolRegistry implements registry.ToolRegistry for testing.
type mockToolRegistry struct {
	executeResult string
	executeErr    error
	executeCalls  int

	toolsForPhaseResult []tools.Tool
	toolsForPhaseCalls  int

	allResult []tools.Tool
	allCalls  int
}

func (m *mockToolRegistry) Execute(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	m.executeCalls++
	return m.executeResult, m.executeErr
}

func (m *mockToolRegistry) ToolsForPhase(_ katypes.Phase, _ katypes.PhaseToolMap) []tools.Tool {
	m.toolsForPhaseCalls++
	return m.toolsForPhaseResult
}

func (m *mockToolRegistry) All() []tools.Tool {
	m.allCalls++
	return m.allResult
}

// mockInvestigationRunner implements kaserver.InvestigationRunner for testing.
type mockInvestigationRunner struct {
	result *katypes.InvestigationResult
	err    error
	calls  int
}

func (m *mockInvestigationRunner) Investigate(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	m.calls++
	return m.result, m.err
}

// mockInvestigationRunnerWithObserver allows injecting steps during investigation.
type mockInvestigationRunnerWithObserver struct {
	result        *katypes.InvestigationResult
	err           error
	onInvestigate func(ctx context.Context)
}

func (m *mockInvestigationRunnerWithObserver) Investigate(ctx context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	if m.onInvestigate != nil {
		m.onInvestigate(ctx)
	}
	return m.result, m.err
}

// cleanResponse returns a standard "not suspicious" evaluator LLM response.
func cleanResponse() llm.ChatResponse {
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"suspicious":false,"explanation":"clean"}`}}
}

// suspiciousResponse returns a standard "suspicious" evaluator LLM response.
func suspiciousResponse() llm.ChatResponse {
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"suspicious":true,"explanation":"injection detected"}`}}
}

// panicMockLLMClient panics on Chat to test panic recovery in SubmitAsync.
type panicMockLLMClient struct{}

func (p *panicMockLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	panic("simulated crypto/rand failure in boundary.Generate")
}

func (p *panicMockLLMClient) StreamChat(_ context.Context, _ llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	panic("simulated crypto/rand failure in boundary.Generate")
}

func (p *panicMockLLMClient) Close() error { return nil }

// mockAuditStore captures audit events for testing.
type mockAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (m *mockAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

// concurrentMockLLMClient is a thread-safe mock for concurrent EvaluateStep tests.
type concurrentMockLLMClient struct {
	mu        sync.Mutex
	responses []llm.ChatResponse
	call      int
}

func (m *concurrentMockLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.call < len(m.responses) {
		r := m.responses[m.call]
		m.call++
		return r, nil
	}
	m.call++
	return llm.ChatResponse{}, nil
}

func (m *concurrentMockLLMClient) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := m.Chat(ctx, req)
	if err == nil {
		_ = cb(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (m *concurrentMockLLMClient) Close() error { return nil }

// Compile-time checks: proxies implement their decorated interfaces.
var (
	_ llm.Client                   = (*alignment.LLMProxy)(nil)
	_ registry.ToolRegistry        = (*alignment.ToolProxy)(nil)
	_ kaserver.InvestigationRunner = (*alignment.InvestigatorWrapper)(nil)
)
