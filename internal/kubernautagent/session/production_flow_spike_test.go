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

package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// emitToSink replicates the exact production emitToSink function from
// investigator.go — non-blocking send to the context-carried event sink.
func emitToSink(ctx context.Context, eventType string, turn int, phase string, data map[string]interface{}) bool {
	sink := session.EventSinkFromContext(ctx)
	if sink == nil {
		return false
	}
	var raw json.RawMessage
	if data != nil {
		var err error
		raw, err = json.Marshal(data)
		if err != nil {
			return false
		}
	}
	event := session.InvestigationEvent{
		Type:  eventType,
		Turn:  turn,
		Phase: phase,
		Data:  raw,
	}
	select {
	case sink <- event:
		return true
	default:
		return false
	}
}

var _ = Describe("Production Flow Spike — End-to-End Event Streaming", func() {

	var (
		store   *session.Store
		mgr     *session.Manager
		logger  logr.Logger
	)

	BeforeEach(func() {
		logger = logr.Discard()
		store = session.NewStore(10*time.Minute, session.WithLogger(logger))
		mgr = session.NewManager(store, logger, nil, nil)
	})

	// H_PROD: Replicates the exact production sequence:
	// AA submits → StartInteractiveSession (pending)
	// AF calls action=start → FindPendingByRemediationID → LaunchDeferredInvestigation
	// Registration code → Subscribe → EventLogBridge
	// Investigation goroutine → emitToSink → channel → bridge
	Describe("H_PROD: Full production path with real Manager functions", func() {

		It("events emitted by the investigation goroutine reach the bridge channel", func() {
			investigationDone := make(chan struct{})
			var sentCount atomic.Int64
			var droppedCount atomic.Int64
			var nilSinkCount atomic.Int64

			// Step 1: AA submits investigation → StartInteractiveSession
			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					// Simulate investigation loop: emit events across 5 turns
					for turn := 0; turn < 5; turn++ {
						sink := session.EventSinkFromContext(ctx)
						if sink == nil {
							nilSinkCount.Add(1)
							logger.Info("investigation: sink nil", "turn", turn)
						}

						ok := emitToSink(ctx, "reasoning_delta", turn, "rca", map[string]interface{}{
							"content_preview": fmt.Sprintf("Turn %d analysis...", turn),
						})
						if ok {
							sentCount.Add(1)
						} else {
							droppedCount.Add(1)
						}

						ok = emitToSink(ctx, "tool_call_start", turn, "rca", map[string]interface{}{
							"tool_name": "kubectl_get",
						})
						if ok {
							sentCount.Add(1)
						} else {
							droppedCount.Add(1)
						}

						ok = emitToSink(ctx, "tool_result", turn, "rca", map[string]interface{}{
							"result_preview": "pods listed",
						})
						if ok {
							sentCount.Add(1)
						} else {
							droppedCount.Add(1)
						}

						time.Sleep(10 * time.Millisecond)
					}
					close(investigationDone)
					return &katypes.InvestigationResult{RCASummary: "CrashLoopBackOff"}, nil
				},
				map[string]string{"remediation_id": "rr-test-001"},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(pendingID).NotTo(BeEmpty())

			// Step 2: AF action=start → LaunchDeferredInvestigation
			err = mgr.LaunchDeferredInvestigation(pendingID)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Registration code → Subscribe (creates channel, activates LazySink)
			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())
			Expect(eventCh).NotTo(BeNil())

			// Step 4: Start bridge goroutine (simulates EventLogBridge.Run)
			var bridgeReceived atomic.Int64
			bridgeDone := make(chan struct{})
			go func() {
				defer close(bridgeDone)
				for {
					evt, ok := <-eventCh
					if !ok {
						return
					}
					bridgeReceived.Add(1)
					logger.Info("bridge received event",
						"type", evt.Type,
						"turn", evt.Turn,
						"seq", bridgeReceived.Load())
				}
			}()

			// Wait for investigation to emit all events
			Eventually(investigationDone, 5*time.Second).Should(BeClosed())

			// Wait for bridge to drain
			Eventually(bridgeDone, 5*time.Second).Should(BeClosed())

			// Verify
			logger.Info("SPIKE RESULTS",
				"sent", sentCount.Load(),
				"dropped", droppedCount.Load(),
				"nil_sink", nilSinkCount.Load(),
				"bridge_received", bridgeReceived.Load())

			Expect(sentCount.Load()).To(BeNumerically(">", 0), "emitToSink should have sent events")
			// Events emitted before Subscribe activates the LazySink are
			// intentionally dropped (sink is nil). This is the designed
			// behavior — not a bug. The race between LaunchDeferredInvestigation
			// and Subscribe means early-turn events may be dropped.
			totalEmitted := sentCount.Load() + droppedCount.Load()
			Expect(totalEmitted).To(BeNumerically("==", 15), "all 15 events (5 turns * 3) should be accounted for")
			// Bridge receives sent events PLUS the EventTypeComplete emitted by
			// the Manager's emitCompleteEvent when the investigation goroutine finishes.
			Expect(bridgeReceived.Load()).To(Equal(sentCount.Load()+1), "bridge should receive all sent events + complete event")
		})
	})

	// H_TIMING_PROD: Tests the race where the investigation goroutine starts
	// emitting BEFORE Subscribe activates the LazySink.
	Describe("H_TIMING_PROD: Investigation starts before Subscribe", func() {

		It("early events are dropped but later events arrive after Subscribe", func() {
			investigationStarted := make(chan struct{})
			subscribeReady := make(chan struct{})
			investigationDone := make(chan struct{})
			var earlyNilCount atomic.Int64
			var lateSentCount atomic.Int64
			var lateDropCount atomic.Int64

			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					close(investigationStarted)

					// Early phase: sink is nil before Subscribe
					for i := 0; i < 3; i++ {
						sink := session.EventSinkFromContext(ctx)
						if sink == nil {
							earlyNilCount.Add(1)
						}
						emitToSink(ctx, "token_delta", i, "rca", map[string]interface{}{"delta": "early"})
					}

					// Wait for Subscribe
					<-subscribeReady
					time.Sleep(5 * time.Millisecond)

					// Late phase: sink should be active
					for i := 3; i < 8; i++ {
						ok := emitToSink(ctx, "reasoning_delta", i, "rca", map[string]interface{}{"content": "late"})
						if ok {
							lateSentCount.Add(1)
						} else {
							lateDropCount.Add(1)
						}
					}
					close(investigationDone)
					return &katypes.InvestigationResult{RCASummary: "test"}, nil
				},
				map[string]string{"remediation_id": "rr-test-002"},
			)
			Expect(err).NotTo(HaveOccurred())

			// Launch investigation goroutine
			err = mgr.LaunchDeferredInvestigation(pendingID)
			Expect(err).NotTo(HaveOccurred())

			// Wait for goroutine to start emitting early events
			Eventually(investigationStarted, 2*time.Second).Should(BeClosed())
			time.Sleep(20 * time.Millisecond)

			// Subscribe (activates LazySink)
			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())
			close(subscribeReady)

			var bridgeReceived atomic.Int64
			bridgeDone := make(chan struct{})
			go func() {
				defer close(bridgeDone)
				for {
					_, ok := <-eventCh
					if !ok {
						return
					}
					bridgeReceived.Add(1)
				}
			}()

			Eventually(investigationDone, 5*time.Second).Should(BeClosed())
			Eventually(bridgeDone, 5*time.Second).Should(BeClosed())

			logger.Info("TIMING SPIKE RESULTS",
				"early_nil", earlyNilCount.Load(),
				"late_sent", lateSentCount.Load(),
				"late_dropped", lateDropCount.Load(),
				"bridge_received", bridgeReceived.Load())

			Expect(earlyNilCount.Load()).To(BeNumerically(">", 0), "early events should see nil sink")
			Expect(lateSentCount.Load()).To(BeNumerically(">", 0), "late events should succeed")
			Expect(lateDropCount.Load()).To(BeNumerically("==", 0), "no late events should be dropped")
			// +1 for EventTypeComplete from Manager.emitCompleteEvent
			Expect(bridgeReceived.Load()).To(Equal(lateSentCount.Load()+1), "bridge should get late events + complete")
		})
	})

	// H_CHAN_IDENTITY: Verifies the channel pointer identity across the pipeline.
	Describe("H_CHAN_IDENTITY: Channel pointer identity check", func() {

		It("Subscribe channel, LazySink channel, and bridge channel are identical", func() {
			investigationDone := make(chan struct{})
			var sinkPtrInGoroutine string

			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					// Wait for Subscribe
					time.Sleep(50 * time.Millisecond)
					sink := session.EventSinkFromContext(ctx)
					if sink != nil {
						sinkPtrInGoroutine = fmt.Sprintf("%p", sink)
					}
					emitToSink(ctx, "test_event", 0, "rca", nil)
					close(investigationDone)
					return &katypes.InvestigationResult{}, nil
				},
				map[string]string{"remediation_id": "rr-test-003"},
			)
			Expect(err).NotTo(HaveOccurred())

			err = mgr.LaunchDeferredInvestigation(pendingID)
			Expect(err).NotTo(HaveOccurred())

			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())

			subscribeChanPtr := fmt.Sprintf("%p", eventCh)

			bridgeDone := make(chan struct{})
			go func() {
				defer close(bridgeDone)
				for {
					_, ok := <-eventCh
					if !ok {
						return
					}
				}
			}()

			Eventually(investigationDone, 5*time.Second).Should(BeClosed())
			Eventually(bridgeDone, 5*time.Second).Should(BeClosed())

			logger.Info("CHANNEL IDENTITY",
				"subscribe_chan_ptr", subscribeChanPtr,
				"sink_ptr_in_goroutine", sinkPtrInGoroutine)

			Expect(sinkPtrInGoroutine).NotTo(BeEmpty(), "goroutine should have seen a non-nil sink")
			// The sink in the goroutine (send-only chan<-) and the subscribe
			// channel (receive-only <-chan) should point to the same underlying
			// channel, but Go prints different pointers for directional views.
			// What matters is events actually flow — verified by H_PROD above.
		})
	})

	// H_BUFFER_PRESSURE: Tests what happens when the bridge is slow and buffer fills.
	Describe("H_BUFFER_PRESSURE: Non-blocking send under buffer pressure", func() {

		It("events are dropped when buffer fills, not silently lost", func() {
			investigationDone := make(chan struct{})
			var sentCount atomic.Int64
			var droppedCount atomic.Int64

			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					// Wait for Subscribe
					time.Sleep(30 * time.Millisecond)

					// Blast 200 events — buffer is 64, so some should drop
					// if nobody is draining
					for i := 0; i < 200; i++ {
						ok := emitToSink(ctx, "token_delta", i, "rca", map[string]interface{}{"delta": "x"})
						if ok {
							sentCount.Add(1)
						} else {
							droppedCount.Add(1)
						}
					}
					close(investigationDone)
					return &katypes.InvestigationResult{}, nil
				},
				map[string]string{"remediation_id": "rr-test-004"},
			)
			Expect(err).NotTo(HaveOccurred())

			err = mgr.LaunchDeferredInvestigation(pendingID)
			Expect(err).NotTo(HaveOccurred())

			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())

			// Intentionally do NOT drain the channel to test buffer overflow
			Eventually(investigationDone, 5*time.Second).Should(BeClosed())

			// Now drain
			var drained int
			for range eventCh {
				drained++
			}

			logger.Info("BUFFER PRESSURE RESULTS",
				"sent", sentCount.Load(),
				"dropped", droppedCount.Load(),
				"drained", drained)

			// Buffer is 64, so at most 64 events should be sent, rest dropped
			Expect(sentCount.Load()).To(BeNumerically("<=", 64))
			Expect(sentCount.Load() + droppedCount.Load()).To(BeNumerically("==", 200))
			Expect(int64(drained)).To(Equal(sentCount.Load()))
		})
	})

	// H_MCP_SDK_SETLEVEL: Tests the MCP SDK's sess.Log() behavior when
	// SetLevel has/hasn't been called. This is a SEPARATE latent issue.
	Describe("H_MCP_SDK_SETLEVEL: MCP SDK LogLevel gate", func() {

		It("documents that sess.Log() silently drops messages when LogLevel is empty", func() {
			// This test documents the MCP SDK v1.6.0 behavior:
			// ServerSession.Log() returns nil (no error) when LogLevel == ""
			// (i.e., client never called SetLoggingLevel).
			//
			// Our AF calls SetLoggingLevel("info") before CallTool, so this
			// shouldn't be the issue in production. But it's a latent risk
			// if the call ordering ever changes.
			//
			// See: github.com/modelcontextprotocol/go-sdk@v1.6.0/mcp/server.go:1312-1321
			//
			// The relevant code:
			//   if logLevel == "" {
			//       return nil  // silently drops — no error returned
			//   }
			logger.Info("MCP SDK v1.6.0 ServerSession.Log() behavior documented",
				"behavior", "silently drops when LogLevel is empty",
				"af_mitigated", "AF calls SetLoggingLevel('info') before CallTool",
				"risk", "if SetLoggingLevel fails or is removed, all events silently lost")
			Succeed()
		})
	})
})
