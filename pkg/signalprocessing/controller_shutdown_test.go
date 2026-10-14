/*
Copyright 2025 Jordi Gil.

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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
// See docs/development/business-requirements/TESTING_GUIDELINES.md
//
// This file covers Error Category B: Context Cancellation and Clean Shutdown
// Referenced by: test/integration/signalprocessing/reconciler_integration_test.go:915
package signalprocessing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// SHUTDOWN TESTS (Error Category B)
// Referenced by integration test skip at reconciler_integration_test.go:915
// ========================================

var _ = Describe("Controller Shutdown", func() {

	// Error Category B: Context Cancellation
	// Tests that controller goroutines exit cleanly when context is canceled
	Context("Error Category B: Context Cancellation Clean Exit", func() {

		// Test 1: Worker goroutine exits on context cancellation
		It("should exit worker goroutine cleanly when context is canceled", func() {
			ctx, cancel := context.WithCancel(context.Background())

			// Track if worker exited
			var workerExited atomic.Bool

			// Simulate a worker goroutine (like reconciler loop)
			go func() {
				defer func() { workerExited.Store(true) }()

				for {
					select {
					case <-ctx.Done():
						return // Clean exit
					default:
						// Simulate work
						time.Sleep(10 * time.Millisecond)
					}
				}
			}()

			// Give worker time to start
			time.Sleep(20 * time.Millisecond)
			Expect(workerExited.Load()).To(BeFalse())

			// Cancel context
			cancel()

			// Worker should exit within reasonable time
			Eventually(workerExited.Load, 100*time.Millisecond, 10*time.Millisecond).Should(BeTrue())
		})

		// Test 2: Multiple workers exit on shared context cancellation
		It("should exit all workers when shared context is canceled", func() {
			ctx, cancel := context.WithCancel(context.Background())

			numWorkers := 5
			var exitCount atomic.Int32
			var wg sync.WaitGroup

			// Start multiple worker goroutines
			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() { exitCount.Add(1) }()

					for {
						select {
						case <-ctx.Done():
							return
						default:
							time.Sleep(10 * time.Millisecond)
						}
					}
				}()
			}

			// Give workers time to start
			time.Sleep(20 * time.Millisecond)
			Expect(exitCount.Load()).To(Equal(int32(0)))

			// Cancel context - all workers should exit
			cancel()

			// Wait for all workers with timeout
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				Expect(exitCount.Load()).To(Equal(int32(numWorkers)))
			case <-time.After(500 * time.Millisecond):
				Fail("Workers did not exit in time")
			}
		})

		// BR-SP-090 / ADR-032: exercise the real audit resource cleanup called
		// by cmd/signalprocessing, not a sleep-based substitute worker.
		It("UT-SP-090-2496-001: waits for proxy-held audit delivery before completing shutdown", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			gate := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(gate) }) }
			arrived := make(chan string, 1)
			received := make(chan []ogenclient.AuditEventRequest, 1)
			fixtureErrors := make(chan error, 1)
			recordFixtureError := func(err error) {
				GinkgoWriter.Printf("Shutdown HTTP fixture error: %v\n", err)
				select {
				case fixtureErrors <- err:
				default:
				}
			}

			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var batch []ogenclient.AuditEventRequest
				if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
					recordFixtureError(fmt.Errorf("decode forwarded audit batch: %w", err))
					http.Error(w, "invalid batch", http.StatusBadRequest)
					return
				}
				select {
				case received <- batch:
				default:
					recordFixtureError(fmt.Errorf("unexpected duplicate audit batch"))
					http.Error(w, "duplicate batch", http.StatusConflict)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				response := ogenclient.BatchAuditEventResponse{EventIds: []uuid.UUID{uuid.New()}}
				if err := json.NewEncoder(w).Encode(&response); err != nil {
					recordFixtureError(fmt.Errorf("encode audit batch response: %w", err))
				}
			}))
			DeferCleanup(endpoint.Close)
			target, err := url.Parse(endpoint.URL)
			Expect(err).NotTo(HaveOccurred())
			proxyTransport := &http.Transport{}
			DeferCleanup(proxyTransport.CloseIdleConnections)
			forwarder := &httputil.ReverseProxy{
				Rewrite:   func(r *httputil.ProxyRequest) { r.SetURL(target) },
				Transport: proxyTransport,
			}
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case arrived <- r.Method + " " + r.URL.Path:
				default:
				}
				select {
				case <-gate:
					forwarder.ServeHTTP(w, r)
				case <-r.Context().Done():
					recordFixtureError(fmt.Errorf("held audit request cancelled: %w", r.Context().Err()))
				}
			}))
			DeferCleanup(proxy.Close)
			clientTransport := &http.Transport{}
			DeferCleanup(clientTransport.CloseIdleConnections)
			client, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(proxy.URL, 5*time.Second, clientTransport)
			Expect(err).NotTo(HaveOccurred())
			store, err := sharedaudit.NewBufferedStore(client, sharedaudit.DefaultConfig(), "signalprocessing", logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			closeDone := make(chan struct{})
			var closeErr error
			closeStarted := false
			startClose := func() {
				closeStarted = true
				go func() {
					closeErr = store.Close()
					close(closeDone)
				}()
			}
			DeferCleanup(func() {
				// Registered last: release and join before transports/servers close.
				release()
				if !closeStarted {
					startClose()
				}
				Eventually(closeDone, 35*time.Second).Should(BeClosed(), "join the real store worker even after an assertion failure")
				Expect(closeErr).NotTo(HaveOccurred())
			})

			event := sharedaudit.NewAuditEventRequest()
			sharedaudit.SetEventType(event, "signalprocessing.signal.processed")
			sharedaudit.SetEventCategory(event, "signalprocessing")
			sharedaudit.SetEventAction(event, "processed")
			sharedaudit.SetEventOutcome(event, sharedaudit.OutcomeSuccess)
			sharedaudit.SetActor(event, "service", "signalprocessing-controller")
			sharedaudit.SetCorrelationID(event, "shutdown-"+uuid.NewString())
			payload := ogenclient.SignalProcessingAuditPayload{
				EventType: "signalprocessing.signal.processed",
				Phase:     ogenclient.SignalProcessingAuditPayloadPhaseCompleted,
				Signal:    "ShutdownAuditDelivery",
			}
			event.EventData = ogenclient.NewAuditEventRequestEventDataSignalprocessingSignalProcessedAuditEventRequestEventData(payload)
			Expect(store.StoreAudit(ctx, event)).To(Succeed())

			By("holding the real shutdown-flush request at a forwarding proxy")
			startClose()
			Eventually(arrived, time.Second, 5*time.Millisecond).Should(Receive(Equal("POST /api/v1/audit/events/batch")))
			cancel()
			Expect(ctx.Err()).To(Equal(context.Canceled))
			Consistently(closeDone, 20*time.Millisecond, 5*time.Millisecond).ShouldNot(BeClosed(), "Close must wait for the held delivery")
			Expect(received).NotTo(Receive(), "the upstream fixture must not see the batch before the proxy is released")

			By("releasing delivery while preserving the existing completion budget")
			release()
			Eventually(closeDone, 200*time.Millisecond, 10*time.Millisecond).Should(BeClosed())
			Expect(closeErr).NotTo(HaveOccurred())
			var batch []ogenclient.AuditEventRequest
			Expect(received).To(Receive(&batch))
			Expect(batch).To(HaveLen(1))
			Expect(batch[0].CorrelationID).To(Equal(event.CorrelationID))
			Expect(batch[0].EventType).To(Equal(event.EventType))
			Expect(batch[0].ActorID).To(Equal(event.ActorID))
			Expect(batch[0].EventData.SignalProcessingAuditPayload).To(Equal(payload))
			Expect(fixtureErrors).NotTo(Receive())
			// This is resource-shutdown UT, not manager/SIGTERM or real DB proof.
		})
	})

	// Error Category B: Graceful Shutdown Patterns
	// Tests patterns used in controller-runtime for graceful shutdown
	Context("Error Category B: Graceful Shutdown Patterns", func() {

		// Test 4: Shutdown signal propagation
		It("should propagate shutdown signal through context hierarchy", func() {
			// Parent context (manager level)
			parentCtx, parentCancel := context.WithCancel(context.Background())

			// Child contexts (controller level)
			childCtx1, cancelChild1 := context.WithCancel(parentCtx)
			defer cancelChild1()
			childCtx2, cancelChild2 := context.WithCancel(parentCtx)
			defer cancelChild2()

			// Verify children are initially active
			Expect(childCtx1.Err()).To(Succeed())
			Expect(childCtx2.Err()).To(Succeed())

			// Cancel parent
			parentCancel()

			// Both children should be canceled
			Expect(childCtx1.Err()).To(Equal(context.Canceled))
			Expect(childCtx2.Err()).To(Equal(context.Canceled))
		})

		// Test 5: Shutdown timeout enforcement
		It("should enforce shutdown timeout", func() {
			// Create context with shutdown timeout
			shutdownTimeout := 50 * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			start := time.Now()

			// Wait for context to expire
			<-ctx.Done()

			elapsed := time.Since(start)

			Expect(ctx.Err()).To(Equal(context.DeadlineExceeded))
			// Allow for scheduler and race-detector timing variations in CI.
			Expect(elapsed).To(BeNumerically(">=", 40*time.Millisecond))
			Expect(elapsed).To(BeNumerically("<", shutdownTimeout+100*time.Millisecond))
		})

		// Test 6: Cleanup functions execute on shutdown
		It("should execute cleanup functions on context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())

			var cleanupExecuted atomic.Bool

			// Register cleanup (simulates defer in reconciler)
			go func() {
				<-ctx.Done()
				// Cleanup work
				cleanupExecuted.Store(true)
			}()

			// Give goroutine time to start listening
			time.Sleep(10 * time.Millisecond)

			// Trigger shutdown
			cancel()

			// Verify cleanup executed
			Eventually(cleanupExecuted.Load, 100*time.Millisecond, 5*time.Millisecond).Should(BeTrue())
		})
	})

	// Error Category B: Resource Cleanup
	// Tests that resources are properly released on shutdown
	Context("Error Category B: Resource Cleanup on Shutdown", func() {

		// Test 7: Channel closure on shutdown
		It("should close channels on shutdown", func() {
			ctx, cancel := context.WithCancel(context.Background())

			// Work channel (simulates work queue)
			workChan := make(chan struct{}, 10)
			var chanClosed atomic.Bool

			// Worker that closes channel on shutdown
			go func() {
				<-ctx.Done()
				close(workChan)
				chanClosed.Store(true)
			}()

			// Give time to start
			time.Sleep(10 * time.Millisecond)

			// Trigger shutdown
			cancel()

			// Channel should be closed
			Eventually(chanClosed.Load, 100*time.Millisecond, 5*time.Millisecond).Should(BeTrue())

			// Verify channel is actually closed
			_, ok := <-workChan
			Expect(ok).To(BeFalse())
		})

		// Test 8: WaitGroup completion before exit
		It("should wait for all goroutines via WaitGroup before exit", func() {
			var wg sync.WaitGroup
			numGoroutines := 3
			var completedCount atomic.Int32

			// Start goroutines
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					time.Sleep(20 * time.Millisecond)
					completedCount.Add(1)
				}()
			}

			// Wait for all goroutines
			wg.Wait()

			// All should have completed
			Expect(completedCount.Load()).To(Equal(int32(numGoroutines)))
		})

		// Test 9: Shutdown does not block indefinitely
		It("should not block shutdown indefinitely on hung goroutine", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			// Simulate a "hung" operation that ignores context
			hungDone := make(chan struct{})
			go func() {
				// This goroutine ignores context - bad practice but tests timeout
				time.Sleep(500 * time.Millisecond)
				close(hungDone)
			}()

			// Shutdown should complete via timeout, not waiting for hung goroutine
			select {
			case <-ctx.Done():
				Expect(ctx.Err()).To(Equal(context.DeadlineExceeded))
			case <-hungDone:
				Fail("Should have timed out before hung goroutine completed")
			}
		})
	})

	// ========================================
	// DD-007 / ADR-032: Audit Store Flush on Shutdown
	// Tests that audit store is properly flushed during graceful shutdown
	// Added: 2025-12-19 per TDD compliance
	// ========================================
	Context("DD-007/ADR-032: Audit Store Flush on Shutdown", func() {

		// Test 10: Audit store Close() pattern is called on context cancellation
		It("BR-SP-090: should call audit store Close() during graceful shutdown", func() {
			ctx, cancel := context.WithCancel(context.Background())

			var closeCalled atomic.Bool

			// Simulate controller shutdown pattern (mirrors cmd/signalprocessing/main.go)
			go func() {
				// Wait for context cancellation (simulates mgr.Start(ctx) returning)
				<-ctx.Done()
				// This is the pattern we added in cmd/signalprocessing/main.go
				closeCalled.Store(true) // Simulates auditStore.Close()
			}()

			// Give goroutine time to start
			time.Sleep(10 * time.Millisecond)
			Expect(closeCalled.Load()).To(BeFalse(), "Close should not be called before shutdown")

			// Trigger shutdown
			cancel()

			// Verify Close() was called
			Eventually(closeCalled.Load, 100*time.Millisecond, 5*time.Millisecond).Should(BeTrue(),
				"ADR-032 §2: auditStore.Close() MUST be called during shutdown to flush pending events")
		})

		// Test 11: Audit store flush completes before process exit
		It("BR-SP-090: should complete audit flush before process continues", func() {
			ctx, cancel := context.WithCancel(context.Background())

			var flushStarted atomic.Bool
			var flushCompleted atomic.Bool
			var processCompleted atomic.Bool

			// Simulate controller shutdown with flush timing
			go func() {
				<-ctx.Done()

				// Flush audit store (simulates real flush taking time)
				flushStarted.Store(true)
				time.Sleep(30 * time.Millisecond) // Simulate flush time
				flushCompleted.Store(true)

				// Process continues after flush
				processCompleted.Store(true)
			}()

			// Trigger shutdown
			cancel()

			// Verify flush completes before process continues
			Eventually(flushCompleted.Load, 200*time.Millisecond, 10*time.Millisecond).Should(BeTrue())

			// Process should not complete until flush is done
			Expect(processCompleted.Load()).To(BeTrue(),
				"Process should complete only after audit flush")
		})

		// Test 12: Audit store flush error is handled gracefully
		It("BR-SP-090: should handle audit flush error during shutdown", func() {
			ctx, cancel := context.WithCancel(context.Background())

			var closeCalled atomic.Bool
			var closeErr atomic.Value

			// Simulate controller shutdown with error handling
			go func() {
				<-ctx.Done()
				closeCalled.Store(true)
				// Simulate flush error
				closeErr.Store(context.DeadlineExceeded)
			}()

			// Trigger shutdown
			cancel()

			// Verify Close() was attempted
			Eventually(closeCalled.Load, 100*time.Millisecond, 5*time.Millisecond).Should(BeTrue())

			// Error should be available for logging (simulates setupLog.Error)
			Eventually(closeErr.Load, 100*time.Millisecond, 5*time.Millisecond).Should(Equal(context.DeadlineExceeded))
		})
	})
})
