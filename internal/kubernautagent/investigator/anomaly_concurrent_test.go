package investigator_test

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
)

var _ = Describe("BR-PERFORMANCE-970: AnomalyDetector Thread Safety", func() {

	Describe("UT-KA-970-001: Concurrent CheckToolCall — No Panic, No Data Race", func() {
		It("should survive 10 goroutines calling CheckToolCall 50 times each without panic or race", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   1000,
				MaxRepeatedFailures: 100,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)
			args := json.RawMessage(`{"kind":"pod","name":"web","namespace":"default"}`)

			const numGoroutines = 10
			const callsPerGoroutine = 50

			var wg sync.WaitGroup
			wg.Add(numGoroutines)

			var admitted atomic.Int32

			for g := 0; g < numGoroutines; g++ {
				go func(gID int) {
					defer GinkgoRecover()
					defer wg.Done()
					for i := 0; i < callsPerGoroutine; i++ {
						result := detector.CheckToolCall("kubectl_describe", args)
						if result.Allowed {
							admitted.Add(1)
						}
					}
				}(g)
			}

			wg.Wait()
			Expect(admitted.Load()).To(BeNumerically("<=", int32(cfg.MaxToolCallsPerTool)),
				"per-tool budget should not be exceeded")
			Expect(detector.TotalExceeded()).To(BeFalse(),
				"total budget (1000) should not be exceeded by 500 calls")
		})
	})

	Describe("UT-KA-970-002: Interleaved CheckToolCall + RecordFailure — Counter Consistency", func() {
		It("should survive concurrent CheckToolCall and RecordFailure without panic or race", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   1000,
				MaxRepeatedFailures: 100,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)
			args := json.RawMessage(`{"kind":"pod","name":"web","namespace":"default"}`)

			const numGoroutines = 5
			const opsPerGoroutine = 50

			var wg sync.WaitGroup
			wg.Add(numGoroutines * 2)

			for g := 0; g < numGoroutines; g++ {
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					for i := 0; i < opsPerGoroutine; i++ {
						detector.CheckToolCall("kubectl_describe", args)
					}
				}()
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					for i := 0; i < opsPerGoroutine; i++ {
						detector.RecordFailure("kubectl_describe", args)
					}
				}()
			}

			wg.Wait()
		})
	})

	Describe("UT-KA-970-003: Concurrent CheckToolCall + Reset — No Deadlock", func() {
		It("should survive concurrent CheckToolCall and Reset without deadlock or panic", func(ctx SpecContext) {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   1000,
				MaxRepeatedFailures: 100,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)
			args := json.RawMessage(`{"kind":"pod","name":"web","namespace":"default"}`)

			done := make(chan struct{})

			var wg sync.WaitGroup
			const numCheckers = 5

			wg.Add(numCheckers + 1)

			for g := 0; g < numCheckers; g++ {
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					for {
						select {
						case <-done:
							return
						default:
							detector.CheckToolCall("kubectl_describe", args)
						}
					}
				}()
			}

			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				for {
					select {
					case <-done:
						return
					default:
						detector.Reset()
					}
				}
			}()

			// Let goroutines exercise for 200ms; the 5s spec timeout catches deadlocks
			time.Sleep(200 * time.Millisecond)
			close(done)

			wg.Wait()
		}, SpecTimeout(5*time.Second))
	})

	Describe("UT-KA-970-004: TotalExceeded Consistency Under Concurrent Access", func() {
		It("should return consistent TotalExceeded after concurrent calls exhaust the budget", func() {
			const budget = 50
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   budget,
				MaxRepeatedFailures: 100,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)

			const numGoroutines = 10
			const callsPerGoroutine = 10

			var wg sync.WaitGroup
			wg.Add(numGoroutines)

			var admitted atomic.Int32
			var rejected atomic.Int32

			for g := 0; g < numGoroutines; g++ {
				go func(gID int) {
					defer GinkgoRecover()
					defer wg.Done()
					toolName := "kubectl_describe"
					for i := 0; i < callsPerGoroutine; i++ {
						result := detector.CheckToolCall(toolName, json.RawMessage(`{}`))
						if result.Allowed {
							admitted.Add(1)
						} else {
							rejected.Add(1)
						}
					}
				}(g)
			}

			wg.Wait()

			totalAttempts := admitted.Load() + rejected.Load()
			Expect(totalAttempts).To(Equal(int32(numGoroutines * callsPerGoroutine)),
				"all attempts should be accounted for")
			Expect(admitted.Load()).To(BeNumerically("<=", int32(budget)),
				"admitted calls should not exceed budget")
			Expect(detector.TotalExceeded()).To(BeTrue(),
				"TotalExceeded should be true after 100 attempts against budget=50")
		})
	})

	Describe("UT-KA-970-005: Serialized Admission Prevents Budget Overrun (BR-HAPI-433-004)", func() {
		It("should admit exactly MaxTotalToolCalls and reject the rest under concurrent pressure", func() {
			const budget = 5
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   budget,
				MaxRepeatedFailures: 100,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)

			const numGoroutines = 20

			var wg sync.WaitGroup
			wg.Add(numGoroutines)

			var admitted atomic.Int32
			var rejected atomic.Int32

			for g := 0; g < numGoroutines; g++ {
				go func(gID int) {
					defer GinkgoRecover()
					defer wg.Done()
					toolName := "kubectl_describe"
					result := detector.CheckToolCall(toolName, json.RawMessage(`{}`))
					if result.Allowed {
						admitted.Add(1)
					} else {
						rejected.Add(1)
					}
				}(g)
			}

			wg.Wait()

			Expect(admitted.Load()).To(Equal(int32(budget)),
				"exactly %d calls should be admitted (budget integrity)", budget)
			Expect(rejected.Load()).To(Equal(int32(numGoroutines - budget)),
				"exactly %d calls should be rejected", numGoroutines-budget)
		})
	})
})
