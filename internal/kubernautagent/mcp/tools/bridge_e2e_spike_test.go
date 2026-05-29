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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// spikeSink captures events forwarded by the bridge, simulating sess.Log().
type spikeSink struct {
	mu       sync.Mutex
	received []json.RawMessage
}

func (m *spikeSink) Log(level, logger string, data json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.received = append(m.received, data)
	return nil
}

func (m *spikeSink) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.received)
}

// emitToSinkExact replicates the EXACT production emitToSink from investigator.go
func emitToSinkExact(ctx context.Context, eventType string, turn int, phase string, data map[string]interface{}) bool {
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

var _ = Describe("Bridge E2E Spike — Real EventLogBridge + Real Manager", func() {

	// H_BRIDGE_E2E: Uses the REAL EventLogBridge.Run() with the REAL Manager,
	// replicating the exact production wiring from registration.go.
	// This is the definitive test for the 162-sent / 0-forwarded paradox.
	Describe("H_BRIDGE_E2E: Full production wiring with real EventLogBridge", func() {

		It("EventLogBridge receives all events sent by the investigation goroutine", func() {
			logger := logr.Discard()
			store := session.NewStore(10*time.Minute, session.WithLogger(logger))
			mgr := session.NewManager(store, logger, nil, nil)

			sink := &spikeSink{}
			investigationDone := make(chan struct{})
			var sentCount atomic.Int64
			var droppedCount atomic.Int64
			var nilSinkCount atomic.Int64

			// Step 1: AA submits → StartInteractiveSession (pending)
			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					// Simulate 8 turns of investigation (matching production)
					for turn := 0; turn < 8; turn++ {
						s := session.EventSinkFromContext(ctx)
						if s == nil {
							nilSinkCount.Add(1)
						}

						// Emit multiple events per turn (like production)
						for _, evtType := range []string{"reasoning_delta", "tool_call_start", "tool_result"} {
							ok := emitToSinkExact(ctx, evtType, turn, "rca", map[string]interface{}{
								"content": fmt.Sprintf("turn-%d-%s", turn, evtType),
							})
							if ok {
								sentCount.Add(1)
							} else {
								droppedCount.Add(1)
							}
						}

						// Simulate LLM latency
						time.Sleep(5 * time.Millisecond)
					}
					close(investigationDone)
					return &katypes.InvestigationResult{RCASummary: "CrashLoopBackOff"}, nil
				},
				map[string]string{"remediation_id": "rr-spike-bridge"},
			)
			Expect(err).NotTo(HaveOccurred())

			// Step 2: AF action=start → LaunchDeferredInvestigation
			err = mgr.LaunchDeferredInvestigation(pendingID)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: registration.go → Subscribe
			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())
			Expect(eventCh).NotTo(BeNil())

			// Step 4: registration.go → NewEventLogBridge + go bridge.Run(context.Background())
			// This is the EXACT production wiring
			bridge := mcptools.NewEventLogBridge(
				eventCh,
				func(level, loggerName string, data json.RawMessage) error {
					return sink.Log(level, loggerName, data)
				},
				logger,
				pendingID,
			)
			bridgeDone := make(chan struct{})
			go func() {
				defer close(bridgeDone)
				bridge.Run(context.Background()) // EXACT production call
			}()

			// Wait for investigation to complete
			Eventually(investigationDone, 10*time.Second).Should(BeClosed())

			// Wait for bridge to drain and exit (channel closes when goroutine ends)
			Eventually(bridgeDone, 5*time.Second).Should(BeClosed())

			GinkgoWriter.Printf("\n=== BRIDGE E2E SPIKE RESULTS ===\n")
			GinkgoWriter.Printf("Events sent by emitToSink: %d\n", sentCount.Load())
			GinkgoWriter.Printf("Events dropped:            %d\n", droppedCount.Load())
			GinkgoWriter.Printf("Sink nil count:            %d\n", nilSinkCount.Load())
			GinkgoWriter.Printf("Events received by sink:   %d\n", sink.count())
			GinkgoWriter.Printf("================================\n\n")

			Expect(sentCount.Load()).To(BeNumerically(">", 0), "events should be sent")
			Expect(droppedCount.Load()).To(Equal(int64(0)), "no events should be dropped")
			Expect(nilSinkCount.Load()).To(Equal(int64(0)), "sink should never be nil")
			// sink receives sent events + complete event from manager
			Expect(sink.count()).To(BeNumerically(">=", int(sentCount.Load())),
				"bridge mock sink should receive at least as many events as emitToSink sent")
		})
	})

	// H_BRIDGE_TIMING: Tests the exact production timing — bridge starts
	// AFTER LaunchDeferredInvestigation, just like registration.go.
	Describe("H_BRIDGE_TIMING: Bridge starts after investigation goroutine", func() {

		It("bridge still receives events even when started after investigation begins", func() {
			logger := logr.Discard()
			store := session.NewStore(10*time.Minute, session.WithLogger(logger))
			mgr := session.NewManager(store, logger, nil, nil)

			sink := &spikeSink{}
			investigationStarted := make(chan struct{})
			investigationDone := make(chan struct{})
			var sentCount atomic.Int64

			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					close(investigationStarted)
					// Wait a bit for bridge to be wired
					time.Sleep(50 * time.Millisecond)

					for turn := 0; turn < 5; turn++ {
						ok := emitToSinkExact(ctx, "reasoning_delta", turn, "rca", map[string]interface{}{
							"content": fmt.Sprintf("turn-%d", turn),
						})
						if ok {
							sentCount.Add(1)
						}
						time.Sleep(5 * time.Millisecond)
					}
					close(investigationDone)
					return &katypes.InvestigationResult{}, nil
				},
				map[string]string{"remediation_id": "rr-spike-timing"},
			)
			Expect(err).NotTo(HaveOccurred())

			// Launch investigation
			err = mgr.LaunchDeferredInvestigation(pendingID)
			Expect(err).NotTo(HaveOccurred())

			// Wait for goroutine to start (mimics production timing)
			Eventually(investigationStarted, 2*time.Second).Should(BeClosed())

			// Subscribe and wire bridge (like registration.go does AFTER handleStart)
			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())

			bridge := mcptools.NewEventLogBridge(
				eventCh,
				func(level, loggerName string, data json.RawMessage) error {
					return sink.Log(level, loggerName, data)
				},
				logger,
				pendingID,
			)
			bridgeDone := make(chan struct{})
			go func() {
				defer close(bridgeDone)
				bridge.Run(context.Background())
			}()

			Eventually(investigationDone, 10*time.Second).Should(BeClosed())
			Eventually(bridgeDone, 5*time.Second).Should(BeClosed())

			GinkgoWriter.Printf("\n=== BRIDGE TIMING SPIKE ===\n")
			GinkgoWriter.Printf("Sent: %d, Received: %d\n", sentCount.Load(), sink.count())
			GinkgoWriter.Printf("===========================\n\n")

			Expect(sentCount.Load()).To(BeNumerically(">", 0))
			Expect(sink.count()).To(BeNumerically(">=", int(sentCount.Load())))
		})
	})

	// H_BRIDGE_LOGFN_BLOCK: Tests what happens when logFn (sess.Log) blocks
	// or is slow — could the bridge goroutine get stuck?
	Describe("H_BRIDGE_LOGFN_BLOCK: Bridge behavior when logFn blocks", func() {

		It("events still flow even when logFn is slow", func() {
			logger := logr.Discard()
			store := session.NewStore(10*time.Minute, session.WithLogger(logger))
			mgr := session.NewManager(store, logger, nil, nil)

			var logFnCalls atomic.Int64
			investigationDone := make(chan struct{})
			var sentCount atomic.Int64

			pendingID, err := mgr.StartInteractiveSession(context.Background(),
				func(ctx context.Context) (*katypes.InvestigationResult, error) {
					time.Sleep(30 * time.Millisecond)
					for turn := 0; turn < 5; turn++ {
						ok := emitToSinkExact(ctx, "reasoning_delta", turn, "rca", map[string]interface{}{
							"content": fmt.Sprintf("turn-%d", turn),
						})
						if ok {
							sentCount.Add(1)
						}
						time.Sleep(10 * time.Millisecond)
					}
					close(investigationDone)
					return &katypes.InvestigationResult{}, nil
				},
				map[string]string{"remediation_id": "rr-spike-block"},
			)
			Expect(err).NotTo(HaveOccurred())

			err = mgr.LaunchDeferredInvestigation(pendingID)
			Expect(err).NotTo(HaveOccurred())

			eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
			Expect(subErr).NotTo(HaveOccurred())

			// Slow logFn — simulates sess.Log() blocking on network I/O
			bridge := mcptools.NewEventLogBridge(
				eventCh,
				func(level, loggerName string, data json.RawMessage) error {
					logFnCalls.Add(1)
					time.Sleep(50 * time.Millisecond) // simulate slow MCP delivery
					return nil
				},
				logger,
				pendingID,
			)
			bridgeDone := make(chan struct{})
			go func() {
				defer close(bridgeDone)
				bridge.Run(context.Background())
			}()

			Eventually(investigationDone, 10*time.Second).Should(BeClosed())
			Eventually(bridgeDone, 30*time.Second).Should(BeClosed())

			GinkgoWriter.Printf("\n=== BRIDGE LOGFN BLOCK SPIKE ===\n")
			GinkgoWriter.Printf("Sent: %d, logFn calls: %d\n", sentCount.Load(), logFnCalls.Load())
			GinkgoWriter.Printf("================================\n\n")

			Expect(logFnCalls.Load()).To(BeNumerically(">=", sentCount.Load()),
				"logFn should be called at least for every sent event")
		})
	})

	// H_BRIDGE_CONTEXT_BG: Verifies that context.Background() never cancels
	Describe("H_BRIDGE_CONTEXT_BG: context.Background() Done channel is nil", func() {

		It("context.Background().Done() returns nil, so ctx.Done case never fires", func() {
			done := context.Background().Done()
			Expect(done).To(BeNil(), "context.Background().Done() must be nil")
		})
	})
})
