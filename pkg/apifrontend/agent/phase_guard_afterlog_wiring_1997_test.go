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

package agent_test

import (
	"context"
	"iter"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// wiringLogSink is a minimal logr.LogSink that records Info() messages, used
// to prove afterLog actually executes (AU-3/AU-12 audit-trail completeness)
// when driven through the real production dispatch path.
type wiringLogSink struct {
	mu       sync.Mutex
	messages []string
}

func (s *wiringLogSink) Init(logr.RuntimeInfo)                  {}
func (s *wiringLogSink) Enabled(int) bool                       { return true }
func (s *wiringLogSink) WithValues(...interface{}) logr.LogSink { return s }
func (s *wiringLogSink) WithName(string) logr.LogSink           { return s }
func (s *wiringLogSink) Error(error, string, ...interface{})    {}
func (s *wiringLogSink) Info(_ int, msg string, _ ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

func (s *wiringLogSink) contains(msg string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.messages {
		if m == msg {
			return true
		}
	}
	return false
}

// #1997/#1998 wiring proof: root.go's NewRootAgent registers
// AfterToolCallbacks: []llmagent.AfterToolCallback{afterMetrics, afterAudit,
// afterPhase, afterLog}. This IT test reproduces that exact relative
// ordering (afterPhase before afterLog) through the real ADK dispatch path
// (llmagent + runner.Run, not a direct in-process call to `after`), and
// proves afterLog's "tool call completed" line -- the FedRAMP AU-3/AU-12
// audit-completeness signal BR-AUDIT-005 requires -- now actually reaches
// the log for a driver-entry tool call, closing the gap a UT calling
// phaseGuardAfter in isolation cannot observe.
var _ = Describe("phaseGuardAfter + afterLog production wiring (#1997/#1998, BR-AUDIT-005 AU-3/AU-12)", func() {
	It("IT-AF-1997-009: afterLog's \"tool call completed\" fires for kubernaut_investigate via runner.Run, in root.go's real AfterToolCallbacks order", func() {
		type investigateArgs struct {
			RRID string `json:"rr_id,omitempty"`
		}
		type investigateResult struct {
			SessionID string `json:"session_id"`
			Status    string `json:"status"`
		}

		investigateTool, err := functiontool.New(functiontool.Config{
			Name:        "kubernaut_investigate",
			Description: "Start an interactive investigation (driver-entry tool)",
		}, func(_ tool.Context, _ investigateArgs) (investigateResult, error) {
			return investigateResult{SessionID: "sess-1997-wiring", Status: "active"}, nil
		})
		Expect(err).NotTo(HaveOccurred())

		var callCount int
		genFn := func(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
			return func(yield func(*model.LLMResponse, error) bool) {
				callCount++
				if callCount == 1 {
					yield(&model.LLMResponse{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{
								ID:   "call-1997-1",
								Name: "kubernaut_investigate",
								Args: map[string]any{},
							}}},
						},
					}, nil)
					return
				}
				yield(&model.LLMResponse{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{{Text: "Done."}},
					},
				}, nil)
			}
		}
		adapter := &llmAdapter{genFn: genFn}

		// Registration order mirrors root.go's NewRootAgent exactly for the
		// two callbacks under test: afterPhase runs before afterLog.
		_, afterPhase := agentpkg.NewPhaseGuardForTest()
		_, afterLog := agentpkg.NewToolLoggingCallbacksForTest()

		a, err := llmagent.New(llmagent.Config{
			Name:               "phase-guard-afterlog-wiring-it-agent",
			Description:        "IT agent proving afterPhase does not short-circuit afterLog (#1997/#1998)",
			Model:              adapter,
			Tools:              []tool.Tool{investigateTool},
			Instruction:        "You are a test agent. Call kubernaut_investigate when asked.",
			AfterToolCallbacks: []llmagent.AfterToolCallback{afterPhase, afterLog},
		})
		Expect(err).NotTo(HaveOccurred())

		r, err := runner.New(runner.Config{
			AppName:           "phase-guard-afterlog-wiring-it",
			Agent:             a,
			SessionService:    session.InMemoryService(),
			AutoCreateSession: true,
		})
		Expect(err).NotTo(HaveOccurred())

		sink := &wiringLogSink{}
		logger := logr.New(sink)
		ctx := logr.NewContext(context.Background(), logger)
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
			Username: "sre@example.com", Groups: []string{"sre"},
		})

		msg := &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{{Text: "start an investigation"}},
		}
		for event, runErr := range r.Run(ctx, "test-user", "sess-1997-wiring", msg, agent.RunConfig{}) {
			Expect(runErr).NotTo(HaveOccurred())
			_ = event
		}

		Expect(sink.contains("tool call completed")).To(BeTrue(),
			"IT-AF-1997-009 BR-AUDIT-005/AU-3/AU-12: afterLog's \"tool call completed\" audit log must fire for kubernaut_investigate through the real production AfterToolCallbacks chain -- afterPhase must not short-circuit it")
	})
})
