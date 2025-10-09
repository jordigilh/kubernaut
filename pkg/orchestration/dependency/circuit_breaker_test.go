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

package dependency_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/orchestration/dependency"
	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCircuitBreaker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dependency Manager Circuit Breaker Suite")
}

var _ = Describe("Circuit Breaker State Management", func() {
	var (
		logger *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests
	})

	// Business Requirement: BR-REL-009 - Circuit breaker patterns for external dependencies
	Context("BR-REL-009: Circuit Breaker State Transitions", func() {
		It("should initialize with closed state and correct configuration", func() {
			// Business Contract: Circuit breaker starts in closed state
			cb := dependency.NewCircuitBreaker("test-circuit", 0.5, 60*time.Second)

			// Business Validation: Initial state should be closed
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateClosed))
			Expect(cb.GetName()).To(Equal("test-circuit"))
			Expect(cb.GetFailureThreshold()).To(Equal(0.5))
			Expect(cb.GetResetTimeout()).To(Equal(60 * time.Second))
		})

		It("should transition from Closed to Open when failure threshold is reached", func() {
			// Business Requirement: BR-REL-009 - Mathematical accuracy of failure rate
			cb := dependency.NewCircuitBreaker("test-circuit", 0.5, 60*time.Second)

			// Need minimum 5 requests for threshold evaluation
			// Create scenario with 60% failure rate (above 50% threshold)
			for i := 0; i < 2; i++ {
				err := cb.Call(func() error { return nil }) // Success
				Expect(err).ToNot(HaveOccurred())
			}

			for i := 0; i < 3; i++ {
				err := cb.Call(func() error { return fmt.Errorf("failure") }) // Failure
				Expect(err).To(HaveOccurred())
			}

			// Now we have 5 requests with 60% failure rate, should be open
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))
			Expect(cb.GetFailureRate()).To(BeNumerically("~", 0.6, 0.01))
		})

		It("should calculate failure rate with mathematical precision", func() {
			// Business Requirement: BR-REL-009 - Mathematical accuracy validation
			cb := dependency.NewCircuitBreaker("test-circuit", 0.6, 60*time.Second)

			// Create precise failure scenario: 6 failures out of 10 requests = 60%
			for i := 0; i < 4; i++ {
				err := cb.Call(func() error { return nil }) // Success
				Expect(err).ToNot(HaveOccurred())
			}

			for i := 0; i < 6; i++ {
				err := cb.Call(func() error { return fmt.Errorf("failure") }) // Failure
				Expect(err).To(HaveOccurred())
			}

			// Business Validation: Exact failure rate calculation
			expectedFailureRate := 6.0 / 10.0
			Expect(cb.GetFailureRate()).To(BeNumerically("~", expectedFailureRate, 0.001))
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))
		})

		It("should remain closed when failure rate is below threshold", func() {
			// Business Requirement: BR-REL-009 - Threshold boundary validation
			cb := dependency.NewCircuitBreaker("test-circuit", 0.5, 60*time.Second)

			// Create scenario with 40% failure rate (below 50% threshold)
			for i := 0; i < 6; i++ {
				err := cb.Call(func() error { return nil }) // Success
				Expect(err).ToNot(HaveOccurred())
			}

			for i := 0; i < 4; i++ {
				err := cb.Call(func() error { return fmt.Errorf("failure") }) // Failure
				Expect(err).To(HaveOccurred())
			}

			// Business Validation: Should remain closed below threshold
			Expect(cb.GetFailureRate()).To(BeNumerically("~", 0.4, 0.001))
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateClosed))
		})

		It("should transition to Half-Open after reset timeout", func() {
			// Business Requirement: BR-REL-009 - Recovery behavior validation
			cb := dependency.NewCircuitBreaker("test-circuit", 0.5, 10*time.Millisecond)

			// Force circuit to open state with enough requests for threshold
			for i := 0; i < 10; i++ {
				_ = cb.Call(func() error { return fmt.Errorf("failure") })
			}
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))

			// Wait for reset timeout
			time.Sleep(15 * time.Millisecond)

			// Next call should transition to half-open, then to closed on success
			err := cb.Call(func() error { return nil })
			Expect(err).ToNot(HaveOccurred())

			// Business Validation: Should be closed after successful recovery
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateClosed))
		})

		It("should transition from Half-Open to Closed on successful call", func() {
			// Business Requirement: BR-REL-009 - Recovery completion validation
			cb := dependency.NewCircuitBreaker("test-circuit", 0.5, 1*time.Millisecond)

			// Force to open state
			for i := 0; i < 10; i++ {
				_ = cb.Call(func() error { return fmt.Errorf("failure") })
			}
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))

			// Wait and make successful call - should transition through half-open to closed
			time.Sleep(2 * time.Millisecond)
			err := cb.Call(func() error { return nil })
			Expect(err).ToNot(HaveOccurred())

			// Business Validation: Circuit should be closed after successful recovery
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateClosed))
			Expect(cb.GetFailures()).To(Equal(int64(0))) // Failures should be reset
		})

		It("should transition from Half-Open back to Open on failure", func() {
			// Business Requirement: BR-REL-009 - Recovery failure handling
			cb := dependency.NewCircuitBreaker("test-circuit", 0.5, 1*time.Millisecond)

			// Force to open state
			for i := 0; i < 10; i++ {
				_ = cb.Call(func() error { return fmt.Errorf("failure") })
			}
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))

			// Wait for timeout, then make a failing call
			time.Sleep(2 * time.Millisecond)

			// This call should transition to half-open, then immediately back to open due to failure
			err := cb.Call(func() error { return fmt.Errorf("recovery failure") })
			Expect(err).To(HaveOccurred())

			// Business Validation: Circuit should be open again
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))
		})

		It("should reject calls when circuit is open", func() {
			// Business Requirement: BR-REL-009 - Fail-fast behavior
			cb := dependency.NewCircuitBreaker("test-circuit", 0.3, 60*time.Second)

			// Force circuit to open
			for i := 0; i < 10; i++ {
				_ = cb.Call(func() error { return fmt.Errorf("failure") })
			}
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))

			// Calls should be rejected without executing function
			functionCalled := false
			err := cb.Call(func() error {
				functionCalled = true
				return nil
			})

			// Business Validation: Call should be rejected and function not executed
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circuit breaker is open"))
			Expect(functionCalled).To(BeFalse())
		})

		It("should handle edge cases in failure rate calculation", func() {
			// Business Requirement: BR-REL-009 - Mathematical robustness
			cb := dependency.NewCircuitBreaker("test-circuit", 0.5, 60*time.Second)

			// Test with zero requests
			Expect(cb.GetFailureRate()).To(Equal(0.0))
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateClosed))

			// Test with single request (success)
			err := cb.Call(func() error { return nil })
			Expect(err).ToNot(HaveOccurred())
			Expect(cb.GetFailureRate()).To(Equal(0.0))

			// Test with single request (failure)
			cb2 := dependency.NewCircuitBreaker("test-circuit-2", 0.5, 60*time.Second)
			err = cb2.Call(func() error { return fmt.Errorf("failure") })
			Expect(err).To(HaveOccurred())
			Expect(cb2.GetFailureRate()).To(Equal(1.0))
		})
	})

	// Business Requirement: BR-ERR-007 - Circuit breaker patterns for external AI services
	Context("BR-ERR-007: AI Service Circuit Breaker Integration", func() {
		It("should handle AI service failure patterns correctly", func() {
			// Business Contract: AI services have specific failure patterns
			cb := dependency.NewCircuitBreaker("ai-service", 0.4, 30*time.Second)

			// Create exactly 30% failure rate (3 failures out of 10 requests)
			// This should remain below the 40% threshold
			for i := 0; i < 7; i++ {
				err := cb.Call(func() error { return nil }) // Success
				Expect(err).ToNot(HaveOccurred())
			}

			for i := 0; i < 3; i++ {
				err := cb.Call(func() error { return fmt.Errorf("AI service timeout") })
				Expect(err).To(HaveOccurred())
			}

			// Business Validation: Should remain closed with 30% failure rate (below 40% threshold)
			Expect(cb.GetFailureRate()).To(BeNumerically("~", 0.3, 0.01))
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateClosed))
		})

		It("should protect against AI service cascading failures", func() {
			// Business Requirement: BR-ERR-007 - Prevent cascade failures
			cb := dependency.NewCircuitBreaker("ai-service", 0.6, 100*time.Millisecond)

			// Simulate AI service complete failure
			for i := 0; i < 10; i++ {
				err := cb.Call(func() error { return fmt.Errorf("AI service unavailable") })
				Expect(err).To(HaveOccurred())
			}

			// Circuit should be open
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))

			// Subsequent calls should fail fast
			start := time.Now()
			err := cb.Call(func() error {
				time.Sleep(100 * time.Millisecond) // This should not execute
				return nil
			})
			duration := time.Since(start)

			// Business Validation: Should fail fast without executing slow operation
			Expect(err).To(HaveOccurred())
			Expect(duration).To(BeNumerically("<", 10*time.Millisecond))
		})
	})
})
