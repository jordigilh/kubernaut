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

package tools_test

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// mockAutonomousSessionManager extends the base NopAutonomousManager with
// configurable StartInvestigation behavior for testing start_autonomous.
type mockAutonomousSessionManager struct {
	mcptools.NopAutonomousManager
	startSessionID string
	startErr       error
	subscribeCh    <-chan session.InvestigationEvent
	subscribeErr   error
	startCalled    bool
	subscribedID   string

	mu sync.Mutex
}

func (m *mockAutonomousSessionManager) StartInvestigation(_ context.Context, _ session.InvestigateFunc, metadata map[string]string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	return m.startSessionID, m.startErr
}

func (m *mockAutonomousSessionManager) Subscribe(_ context.Context, id string) (<-chan session.InvestigationEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribedID = id
	return m.subscribeCh, m.subscribeErr
}

// mockLogSink captures ServerSession.Log calls for asserting event bridge behavior.
type mockLogSink struct {
	mu       sync.Mutex
	messages []logMessage
}

type logMessage struct {
	Level  string
	Logger string
	Data   json.RawMessage
}

func (s *mockLogSink) Log(level, logger string, data json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, logMessage{Level: level, Logger: logger, Data: data})
	return nil
}

func (s *mockLogSink) Messages() []logMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	dst := make([]logMessage, len(s.messages))
	copy(dst, s.messages)
	return dst
}

// mockAlreadyRunningAutoMgr simulates an already-running investigation
// by returning a session from FindByRemediationID.
type mockAlreadyRunningAutoMgr struct {
	mcptools.NopAutonomousManager
	existingSessionID string
}

func (m *mockAlreadyRunningAutoMgr) FindByRemediationID(_ string) (string, bool) {
	return m.existingSessionID, true
}

// mockRRChecker for start_autonomous RR validation.
type mockRRCheckerAutonomous struct {
	exists bool
	err    error
}

func (m *mockRRCheckerAutonomous) RemediationRequestExists(_ context.Context, _ string) (bool, error) {
	return m.exists, m.err
}

var _ = Describe("kubernaut_investigate — start_autonomous action (#1326 BR-MCP-002)", func() {

	Describe("UT-KA-1326-005: ValidateInput accepts start_autonomous action", func() {
		It("should accept start_autonomous with valid rr_id", func() {
			err := mcptools.ValidateInput(mcptools.InvestigateInput{
				RRID:   "rr-auto-001",
				Action: mcptools.ActionStartAutonomous,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-1326-002: start_autonomous with missing rr_id returns error", func() {
		It("should return ErrMissingRRID when rr_id is empty", func() {
			err := mcptools.ValidateInput(mcptools.InvestigateInput{
				Action: mcptools.ActionStartAutonomous,
			})
			Expect(err).To(MatchError(mcptools.ErrMissingRRID))
		})
	})

	Describe("UT-KA-1326-001: start_autonomous with valid rr_id starts investigation", func() {
		It("should return autonomous_started status with session_id", func() {
			autoMgr := &mockAutonomousSessionManager{
				startSessionID: "sess-auto-001",
			}
			eventCh := make(chan session.InvestigationEvent, 64)
			autoMgr.subscribeCh = eventCh

			tool := mcptools.NewInvestigateTool(
				&mockSessionManager{},
				&mockInvestigatorRunner{},
				&mockContextReconstructor{},
				autoMgr,
				mcptools.WithRRExistenceChecker(&mockRRCheckerAutonomous{exists: true}),
			)

			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-auto-001",
				Action: mcptools.ActionStartAutonomous,
			}, mcpinternal.UserInfo{Username: "test-user"})

			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("autonomous_started"))
			Expect(output.SessionID).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-1326-003: start_autonomous with nonexistent RR returns error", func() {
		It("should return RR not found error", func() {
			tool := mcptools.NewInvestigateTool(
				&mockSessionManager{},
				&mockInvestigatorRunner{},
				&mockContextReconstructor{},
				&mockAutonomousSessionManager{},
				mcptools.WithRRExistenceChecker(&mockRRCheckerAutonomous{exists: false}),
			)

			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-nonexistent",
				Action: mcptools.ActionStartAutonomous,
			}, mcpinternal.UserInfo{Username: "test-user"})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id"))
		})
	})

	Describe("UT-KA-1326-004: start_autonomous when investigation already running returns existing session_id", func() {
		It("should return existing session_id with already_running status", func() {
			autoMgr := &mockAlreadyRunningAutoMgr{
				existingSessionID: "sess-existing-001",
			}

			tool := mcptools.NewInvestigateTool(
				&mockSessionManager{},
				&mockInvestigatorRunner{},
				&mockContextReconstructor{},
				autoMgr,
				mcptools.WithRRExistenceChecker(&mockRRCheckerAutonomous{exists: true}),
			)

			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-already-running",
				Action: mcptools.ActionStartAutonomous,
			}, mcpinternal.UserInfo{Username: "test-user"})

			Expect(err).NotTo(HaveOccurred())
			Expect(output.SessionID).To(Equal("sess-existing-001"))
			Expect(output.Status).To(Equal("already_running"))
		})
	})

	Describe("UT-KA-1326-006: dispatch routes start_autonomous to handleStartAutonomous", func() {
		It("should invoke handleStartAutonomous for start_autonomous action", func() {
			autoMgr := &mockAutonomousSessionManager{
				startSessionID: "sess-dispatch-001",
			}
			eventCh := make(chan session.InvestigationEvent, 64)
			autoMgr.subscribeCh = eventCh

			tool := mcptools.NewInvestigateTool(
				&mockSessionManager{},
				&mockInvestigatorRunner{},
				&mockContextReconstructor{},
				autoMgr,
				mcptools.WithRRExistenceChecker(&mockRRCheckerAutonomous{exists: true}),
			)

			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-dispatch-001",
				Action: mcptools.ActionStartAutonomous,
			}, mcpinternal.UserInfo{Username: "test-user"})

			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("autonomous_started"))
		})
	})

	Describe("UT-KA-1326-007: start_autonomous calls Subscribe to activate LazySink EventChannel", func() {
		It("should call Subscribe after starting the investigation", func() {
			autoMgr := &mockAutonomousSessionManager{
				startSessionID: "sess-subscribe-001",
			}
			eventCh := make(chan session.InvestigationEvent, 64)
			autoMgr.subscribeCh = eventCh

			tool := mcptools.NewInvestigateTool(
				&mockSessionManager{},
				&mockInvestigatorRunner{},
				&mockContextReconstructor{},
				autoMgr,
				mcptools.WithRRExistenceChecker(&mockRRCheckerAutonomous{exists: true}),
			)

			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-subscribe-001",
				Action: mcptools.ActionStartAutonomous,
			}, mcpinternal.UserInfo{Username: "test-user"})

			Expect(err).NotTo(HaveOccurred())

			autoMgr.mu.Lock()
			subscribedID := autoMgr.subscribedID
			autoMgr.mu.Unlock()
			Expect(subscribedID).To(Equal("sess-subscribe-001"))
		})
	})
})

var _ = Describe("kubernaut_investigate — EventChannel→Log bridge (#1326 BR-MCP-003)", func() {

	Describe("UT-KA-1326-010: reasoning_delta event streamed as LoggingMessage (SI-4)", func() {
		It("should call Log with level info and event_type reasoning_delta", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test-010")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go bridge.Run(ctx)

			eventCh <- session.InvestigationEvent{
				Type: session.EventTypeReasoningDelta,
				Turn: 1,
				Data: json.RawMessage(`{"text":"Analyzing pod crash..."}`),
			}

			Eventually(func() int { return len(sink.Messages()) }, 2*time.Second).Should(BeNumerically(">=", 1))
			msg := sink.Messages()[0]
			Expect(msg.Level).To(Equal("info"))

			var envelope map[string]interface{}
			Expect(json.Unmarshal(msg.Data, &envelope)).To(Succeed())
			Expect(envelope["type"]).To(Equal("reasoning_delta"))
		})
	})

	Describe("UT-KA-1326-011: tool_call_start event streamed as LoggingMessage (SI-4)", func() {
		It("should call Log with data containing type tool_call_start", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go bridge.Run(ctx)

			eventCh <- session.InvestigationEvent{
				Type: session.EventTypeToolCallStart,
				Turn: 1,
				Data: json.RawMessage(`{"tool":"kubectl_get"}`),
			}

			Eventually(func() int { return len(sink.Messages()) }, 2*time.Second).Should(BeNumerically(">=", 1))

			var envelope map[string]interface{}
			Expect(json.Unmarshal(sink.Messages()[0].Data, &envelope)).To(Succeed())
			Expect(envelope["type"]).To(Equal("tool_call_start"))
		})
	})

	Describe("UT-KA-1326-012: tool_result event streamed as LoggingMessage (SI-4)", func() {
		It("should call Log with data containing type tool_result", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go bridge.Run(ctx)

			eventCh <- session.InvestigationEvent{
				Type: session.EventTypeToolResult,
				Turn: 1,
				Data: json.RawMessage(`{"output":"pod logs..."}`),
			}

			Eventually(func() int { return len(sink.Messages()) }, 2*time.Second).Should(BeNumerically(">=", 1))

			var envelope map[string]interface{}
			Expect(json.Unmarshal(sink.Messages()[0].Data, &envelope)).To(Succeed())
			Expect(envelope["type"]).To(Equal("tool_result"))
		})
	})

	Describe("UT-KA-1326-013: complete event streamed as LoggingMessage (SI-4)", func() {
		It("should call Log with data containing event_type complete", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go bridge.Run(ctx)

			eventCh <- session.InvestigationEvent{
				Type: session.EventTypeComplete,
				Turn: 3,
				Data: json.RawMessage(`{"summary":"Root cause: OOM kill"}`),
			}

			Eventually(func() int { return len(sink.Messages()) }, 2*time.Second).Should(BeNumerically(">=", 1))

			var envelope map[string]interface{}
			Expect(json.Unmarshal(sink.Messages()[0].Data, &envelope)).To(Succeed())
			Expect(envelope["type"]).To(Equal("complete"))
		})
	})

	Describe("UT-KA-1326-014: error event streamed as LoggingMessage (SI-4)", func() {
		It("should call Log with level error and event_type error", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go bridge.Run(ctx)

			eventCh <- session.InvestigationEvent{
				Type: session.EventTypeError,
				Turn: 1,
				Data: json.RawMessage(`{"error":"LLM provider unavailable"}`),
			}

			Eventually(func() int { return len(sink.Messages()) }, 2*time.Second).Should(BeNumerically(">=", 1))
			msg := sink.Messages()[0]
			Expect(msg.Level).To(Equal("error"))

			var envelope map[string]interface{}
			Expect(json.Unmarshal(msg.Data, &envelope)).To(Succeed())
			Expect(envelope["type"]).To(Equal("error"))
		})
	})

	Describe("UT-KA-1326-015: Bridge goroutine exits when event channel closes", func() {
		It("should exit without leak or panic when channel is closed", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				bridge.Run(ctx)
				close(done)
			}()

			close(eventCh)

			Eventually(done, 2*time.Second).Should(BeClosed(), "bridge goroutine must exit on channel close")
		})
	})

	Describe("UT-KA-1326-016: Bridge goroutine exits when context cancelled", func() {
		It("should exit cleanly on context cancellation", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test")
			ctx, cancel := context.WithCancel(context.Background())

			done := make(chan struct{})
			go func() {
				bridge.Run(ctx)
				close(done)
			}()

			cancel()

			Eventually(done, 2*time.Second).Should(BeClosed(), "bridge goroutine must exit on context cancel")
		})
	})

	Describe("UT-KA-1326-017: No events streamed if no subscriber (LazySink nil)", func() {
		It("should never call Log when event channel receives no events", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test")
			ctx, cancel := context.WithCancel(context.Background())

			go bridge.Run(ctx)

			// Give the bridge time to process — no events sent
			time.Sleep(100 * time.Millisecond)
			cancel()

			Expect(sink.Messages()).To(BeEmpty(), "Log must never be called when no events are emitted")
		})
	})

	Describe("UT-KA-1326-018: Events arrive with monotonically increasing sequence numbers (SI-4)", func() {
		It("should include seq field that increases with each event", func() {
			eventCh := make(chan session.InvestigationEvent, 10)
			sink := &mockLogSink{}

			bridge := mcptools.NewEventLogBridge(eventCh, sink.Log, logr.Discard(), "test")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go bridge.Run(ctx)

			eventCh <- session.InvestigationEvent{Type: session.EventTypeReasoningDelta, Turn: 1, Data: json.RawMessage(`{"text":"a"}`)}
			eventCh <- session.InvestigationEvent{Type: session.EventTypeToolCallStart, Turn: 1, Data: json.RawMessage(`{"tool":"x"}`)}
			eventCh <- session.InvestigationEvent{Type: session.EventTypeComplete, Turn: 2, Data: json.RawMessage(`{"summary":"done"}`)}

			Eventually(func() int { return len(sink.Messages()) }, 2*time.Second).Should(BeNumerically(">=", 3))

			messages := sink.Messages()
			var prevSeq float64
			for i, msg := range messages {
				var envelope map[string]interface{}
				Expect(json.Unmarshal(msg.Data, &envelope)).To(Succeed())
				seq, ok := envelope["seq"].(float64)
				Expect(ok).To(BeTrue(), "seq field must be a number")
				if i > 0 {
					Expect(seq).To(BeNumerically(">", prevSeq), "seq must be monotonically increasing")
				}
				prevSeq = seq
			}
		})
	})
})
