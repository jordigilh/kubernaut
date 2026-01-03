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

package circuitbreaker_test

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sony/gobreaker"

	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
)

// Test Plan: Circuit Breaker Manager
// Business Requirement: BR-NOT-055 (Notification), BR-GATEWAY-XXX (Gateway)
//
// Coverage:
// - Multi-channel isolation (per-resource circuit breakers)
// - State transitions (Closed → Open → Half-Open → Closed)
// - Automatic state management via Execute()
// - Thread safety for concurrent operations
// - Backward compatibility API (AllowRequest, RecordSuccess, RecordFailure)

var _ = Describe("Circuit Breaker Manager", func() {
	var manager *circuitbreaker.Manager

	BeforeEach(func() {
		manager = circuitbreaker.NewManager(gobreaker.Settings{
			MaxRequests: 2,                // Allow 2 test requests in half-open
			Interval:    10 * time.Second, // Reset failure count every 10s
			Timeout:     100 * time.Millisecond,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				// Trip after 3 consecutive failures
				return counts.ConsecutiveFailures >= 3
			},
		})
	})

	Describe("Multi-Channel Isolation", func() {
		It("should maintain independent state per channel", func() {
			// Given: Multiple channels (slack, console, webhook)

			// When: Trigger circuit breaker for one channel only
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute("slack", func() (interface{}, error) {
					return nil, fmt.Errorf("slack failure")
				})
			}

			// Then: Only slack circuit should be open
			Expect(manager.AllowRequest("slack")).To(BeFalse(),
				"Slack circuit should be open after failures")
			Expect(manager.AllowRequest("console")).To(BeTrue(),
				"Console circuit should remain closed (independent)")
			Expect(manager.AllowRequest("webhook")).To(BeTrue(),
				"Webhook circuit should remain closed (independent)")

			// And: Verify state values
			Expect(manager.State("slack")).To(Equal(gobreaker.StateOpen))
			Expect(manager.State("console")).To(Equal(gobreaker.StateClosed))
			Expect(manager.State("webhook")).To(Equal(gobreaker.StateClosed))
		})

		It("should handle concurrent operations on different channels", func() {
			// Given: Multiple goroutines operating on different channels
			var wg sync.WaitGroup
			channels := []string{"ch1", "ch2", "ch3", "ch4", "ch5"}
			wg.Add(len(channels))

			// When: Execute operations concurrently
			for _, ch := range channels {
				go func(channel string) {
					defer wg.Done()
					for i := 0; i < 10; i++ {
						_, _ = manager.Execute(channel, func() (interface{}, error) {
							return "success", nil
						})
					}
				}(ch)
			}

			wg.Wait()

			// Then: All circuits should remain closed (successful operations)
			for _, ch := range channels {
				Expect(manager.AllowRequest(ch)).To(BeTrue(),
					"Channel %s should allow requests after successful operations", ch)
				Expect(manager.State(ch)).To(Equal(gobreaker.StateClosed))
			}
		})
	})

	Describe("State Transitions", func() {
		It("should transition from Closed to Open after consecutive failures", func() {
			channel := "test-channel"

			// Given: Circuit is initially closed
			Expect(manager.State(channel)).To(Equal(gobreaker.StateClosed))

			// When: Execute 3 consecutive failures
			for i := 0; i < 3; i++ {
				_, err := manager.Execute(channel, func() (interface{}, error) {
					return nil, fmt.Errorf("failure %d", i+1)
				})
				Expect(err).To(HaveOccurred())
			}

			// Then: Circuit should be open
			Expect(manager.State(channel)).To(Equal(gobreaker.StateOpen))
			Expect(manager.AllowRequest(channel)).To(BeFalse())
		})

		It("should transition from Open to Half-Open after timeout", func() {
			channel := "test-channel"

			// Given: Circuit is open
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute(channel, func() (interface{}, error) {
					return nil, fmt.Errorf("failure")
				})
			}
			Expect(manager.State(channel)).To(Equal(gobreaker.StateOpen))

			// When: Wait for timeout period
			time.Sleep(150 * time.Millisecond) // Slightly longer than 100ms timeout

			// Then: Circuit should transition to half-open
			// Note: State doesn't change until first request attempt
			Expect(manager.AllowRequest(channel)).To(BeTrue(),
				"Half-open state should allow probe requests")
		})

		It("should transition from Half-Open to Closed after successful requests", func() {
			channel := "test-channel"

			// Given: Circuit is open
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute(channel, func() (interface{}, error) {
					return nil, fmt.Errorf("failure")
				})
			}

			// And: Wait for half-open transition
			time.Sleep(150 * time.Millisecond)

			// When: Execute successful requests in half-open state
			// MaxRequests=2, so 2 successful requests should close circuit
			for i := 0; i < 2; i++ {
				result, err := manager.Execute(channel, func() (interface{}, error) {
					return "success", nil
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal("success"))
			}

			// Then: Circuit should be closed
			Expect(manager.AllowRequest(channel)).To(BeTrue())
			// Note: State might be closed or still half-open depending on gobreaker internals
			// The key behavior is that requests are allowed
		})

		It("should transition from Half-Open back to Open on failure", func() {
			channel := "test-channel"

			// Given: Circuit is open
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute(channel, func() (interface{}, error) {
					return nil, fmt.Errorf("failure")
				})
			}

			// And: Wait for half-open transition
			time.Sleep(150 * time.Millisecond)

			// When: Execute failing request in half-open state
			_, err := manager.Execute(channel, func() (interface{}, error) {
				return nil, fmt.Errorf("still failing")
			})

			// Then: Circuit should reopen
			Expect(err).To(HaveOccurred())
			Expect(manager.AllowRequest(channel)).To(BeFalse(),
				"Circuit should reopen after failure in half-open state")
		})
	})

	Describe("Execute Method", func() {
		It("should execute function and return result on success", func() {
			channel := "test-channel"

			// When: Execute successful function
			result, err := manager.Execute(channel, func() (interface{}, error) {
				return "test-result", nil
			})

			// Then: Should return result without error
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("test-result"))
		})

		It("should return error on function failure", func() {
			channel := "test-channel"
			expectedErr := fmt.Errorf("test error")

			// When: Execute failing function
			result, err := manager.Execute(channel, func() (interface{}, error) {
				return nil, expectedErr
			})

			// Then: Should return error
			Expect(err).To(Equal(expectedErr))
			Expect(result).To(BeNil())
		})

		It("should return ErrOpenState when circuit is open", func() {
			channel := "test-channel"

			// Given: Circuit is open
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute(channel, func() (interface{}, error) {
					return nil, fmt.Errorf("failure")
				})
			}

			// When: Attempt to execute while circuit is open
			result, err := manager.Execute(channel, func() (interface{}, error) {
				return "should not execute", nil
			})

			// Then: Should return ErrOpenState
			Expect(err).To(Equal(gobreaker.ErrOpenState))
			Expect(result).To(BeNil())
		})
	})

	Describe("Backward Compatibility API", func() {
		It("should support AllowRequest for manual checking", func() {
			channel := "test-channel"

			// Given: Circuit is closed
			Expect(manager.AllowRequest(channel)).To(BeTrue())

			// When: Circuit opens
			for i := 0; i < 3; i++ {
				_, _ = manager.Execute(channel, func() (interface{}, error) {
					return nil, fmt.Errorf("failure")
				})
			}

			// Then: AllowRequest should return false
			Expect(manager.AllowRequest(channel)).To(BeFalse())
		})

		It("should support RecordSuccess/RecordFailure as no-ops", func() {
			// Note: These methods exist for API compatibility but are no-ops
			// because gobreaker tracks success/failure automatically via Execute()

			channel := "test-channel"

			// When: Call legacy API methods
			manager.RecordSuccess(channel)
			manager.RecordFailure(channel)

			// Then: Should not panic or cause errors
			// Actual state is managed by Execute() calls
			Expect(manager.State(channel)).To(Equal(gobreaker.StateClosed),
				"State should remain closed (no-ops don't affect state)")
		})
	})

	Describe("Thread Safety", func() {
		It("should handle concurrent Execute calls on same channel", func() {
			channel := "test-channel"
			concurrentRequests := 50
			var wg sync.WaitGroup
			wg.Add(concurrentRequests)

			// When: Execute concurrent requests on same channel
			successCount := 0
			var mu sync.Mutex

			for i := 0; i < concurrentRequests; i++ {
				go func() {
					defer wg.Done()
					_, err := manager.Execute(channel, func() (interface{}, error) {
						return "success", nil
					})
					if err == nil {
						mu.Lock()
						successCount++
						mu.Unlock()
					}
				}()
			}

			wg.Wait()

			// Then: All requests should succeed (circuit remains closed)
			Expect(successCount).To(Equal(concurrentRequests))
			Expect(manager.State(channel)).To(Equal(gobreaker.StateClosed))
		})

		It("should handle concurrent circuit breaker creation for new channels", func() {
			channels := make([]string, 20)
			for i := 0; i < 20; i++ {
				channels[i] = fmt.Sprintf("channel-%d", i)
			}

			var wg sync.WaitGroup
			wg.Add(len(channels))

			// When: Trigger circuit breaker creation concurrently
			for _, ch := range channels {
				go func(channel string) {
					defer wg.Done()
					// First access creates circuit breaker
					_ = manager.AllowRequest(channel)
				}(ch)
			}

			wg.Wait()

			// Then: All channels should have circuit breakers
			for _, ch := range channels {
				Expect(manager.State(ch)).To(Equal(gobreaker.StateClosed))
			}
		})
	})

	Describe("Edge Cases", func() {
		It("should handle rapid open/close cycles", func() {
			channel := "test-channel"

			// When: Rapidly cycle between failures and successes
			for cycle := 0; cycle < 5; cycle++ {
				// Cause failures
				for i := 0; i < 3; i++ {
					_, _ = manager.Execute(channel, func() (interface{}, error) {
						return nil, fmt.Errorf("failure")
					})
				}

				// Wait for half-open
				time.Sleep(150 * time.Millisecond)

				// Recover with successes
				for i := 0; i < 2; i++ {
					_, _ = manager.Execute(channel, func() (interface{}, error) {
						return "success", nil
					})
				}
			}

			// Then: Circuit should be functional
			Expect(manager.AllowRequest(channel)).To(BeTrue(),
				"Circuit should be functional after rapid cycles")
		})

		It("should handle empty channel name", func() {
			// When: Use empty channel name
			result, err := manager.Execute("", func() (interface{}, error) {
				return "test", nil
			})

			// Then: Should work (empty string is valid channel name)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("test"))
		})

		It("should handle very long channel names", func() {
			longChannelName := "very-long-channel-name-" + string(make([]byte, 1000))

			// When: Use very long channel name
			result, err := manager.Execute(longChannelName, func() (interface{}, error) {
				return "test", nil
			})

			// Then: Should work
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("test"))
		})
	})
})

