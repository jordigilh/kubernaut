package holmesgpt_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInvestigationAtomicImplementation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Investigation Atomic Implementation Suite")
}

var _ = Describe("Investigation Atomic Implementation", func() {

	// Business Requirement: BR-EXTERNAL-005 - Investigation state management
	Context("BR-EXTERNAL-005: ActiveInvestigation Atomic Status Methods", func() {
		var investigation *holmesgpt.ActiveInvestigation

		BeforeEach(func() {
			investigation = &holmesgpt.ActiveInvestigation{
				InvestigationID: "test-investigation-001",
				AlertType:       "kubernetes",
				Namespace:       "test-namespace",
				StartTime:       time.Now(),
				Status:          "active",
				LastActivity:    time.Now(),
			}
			investigation.InitializeStatusAtomic()
		})

		It("should initialize atomic status correctly", func() {
			// Arrange & Act: Already initialized in BeforeEach

			// Business Validation: Atomic status should match string status
			atomicStatus, atomicTime := investigation.GetStatusAtomic()
			Expect(atomicStatus).To(Equal("active"))
			Expect(atomicTime).To(BeTemporally("~", investigation.LastActivity, time.Second))
		})

		It("should handle atomic status transitions correctly", func() {
			// Act: Transition from active to completed
			success := investigation.TransitionStatusAtomic("active", "completed")

			// Business Validation: Transition should succeed
			Expect(success).To(BeTrue(), "Transition from active to completed should succeed")

			// Verify atomic and string status are synchronized
			atomicStatus, _ := investigation.GetStatusAtomic()
			Expect(atomicStatus).To(Equal("completed"))
			Expect(investigation.Status).To(Equal("completed"))

			// Try invalid transition
			invalidTransition := investigation.TransitionStatusAtomic("active", "failed")
			Expect(invalidTransition).To(BeFalse(), "Transition from non-current status should fail")
		})

		It("should reject invalid status values", func() {
			// Act: Try to set invalid status
			success := investigation.SetStatusAtomic("invalid-status")

			// Business Validation: Invalid status should be rejected
			Expect(success).To(BeFalse(), "Invalid status should be rejected")

			// Status should remain unchanged
			atomicStatus, _ := investigation.GetStatusAtomic()
			Expect(atomicStatus).To(Equal("active"))
		})

		It("should handle concurrent status operations without corruption", func() {
			// Arrange: Concurrent operation setup
			const numGoroutines = 25
			const operationsPerGoroutine = 50

			var wg sync.WaitGroup
			var successfulTransitions int64
			var corruptionDetected int64

			// Act: Concurrent status operations
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < operationsPerGoroutine; j++ {
						// Mix of different operations
						switch (workerID + j) % 4 {
						case 0:
							// Try to pause
							if investigation.TransitionStatusAtomic("active", "paused") {
								atomic.AddInt64(&successfulTransitions, 1)
							}
						case 1:
							// Try to complete
							currentStatus, _ := investigation.GetStatusAtomic()
							if investigation.TransitionStatusAtomic(currentStatus, "completed") {
								atomic.AddInt64(&successfulTransitions, 1)
							}
						case 2:
							// Read status
							atomicStatus, atomicTime := investigation.GetStatusAtomic()

							// Verify consistency with string status
							stringStatus, stringTime := investigation.GetStatusSafe()

							if atomicStatus != stringStatus || atomicTime.Unix() != stringTime.Unix() {
								atomic.AddInt64(&corruptionDetected, 1)
							}
						case 3:
							// Try to fail
							currentStatus, _ := investigation.GetStatusAtomic()
							if investigation.TransitionStatusAtomic(currentStatus, "failed") {
								atomic.AddInt64(&successfulTransitions, 1)
							}
						}

						time.Sleep(time.Microsecond * 2)
					}
				}(i)
			}

			// Wait for completion with timeout
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Success
			case <-time.After(30 * time.Second):
				Fail("Test timed out - possible deadlock in atomic operations")
			}

			// Business Validation: No corruption should occur
			finalCorruption := atomic.LoadInt64(&corruptionDetected)
			finalTransitions := atomic.LoadInt64(&successfulTransitions)

			Expect(finalCorruption).To(Equal(int64(0)),
				"No corruption should occur during concurrent atomic operations")
			Expect(finalTransitions).To(BeNumerically(">", 0),
				"At least some status transitions should succeed")

			// Final consistency check
			atomicStatus, atomicTime := investigation.GetStatusAtomic()
			stringStatus, stringTime := investigation.GetStatusSafe()

			Expect(atomicStatus).To(Equal(stringStatus),
				"Final atomic and string status should be consistent")
			Expect(atomicTime.Unix()).To(Equal(stringTime.Unix()),
				"Final atomic and string timestamps should be consistent")
		})

		It("should maintain proper status ordering constraints", func() {
			// Business Validation: Test status transition rules

			// Active -> Paused should work
			Expect(investigation.TransitionStatusAtomic("active", "paused")).To(BeTrue())

			// Paused -> Active should work
			Expect(investigation.TransitionStatusAtomic("paused", "active")).To(BeTrue())

			// Active -> Completed should work
			Expect(investigation.TransitionStatusAtomic("active", "completed")).To(BeTrue())

			// Completed -> anything should not work (final state)
			Expect(investigation.TransitionStatusAtomic("completed", "active")).To(BeFalse())
			Expect(investigation.TransitionStatusAtomic("completed", "paused")).To(BeFalse())
			Expect(investigation.TransitionStatusAtomic("completed", "failed")).To(BeFalse())

			// Verify final status
			finalStatus, _ := investigation.GetStatusAtomic()
			Expect(finalStatus).To(Equal("completed"))
		})

		It("should handle high-frequency status checks without performance degradation", func() {
			// Arrange: High-frequency access test
			const numReaders = 50
			const readsPerReader = 1000

			var wg sync.WaitGroup
			var totalReads int64
			var readErrors int64

			startTime := time.Now()

			// Act: High-frequency concurrent reads
			for i := 0; i < numReaders; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					for j := 0; j < readsPerReader; j++ {
						atomic.AddInt64(&totalReads, 1)

						status, timestamp := investigation.GetStatusAtomic()

						// Validate read results
						if status == "" || status == "unknown" {
							atomic.AddInt64(&readErrors, 1)
						}
						if timestamp.IsZero() || timestamp.Unix() <= 0 {
							atomic.AddInt64(&readErrors, 1)
						}
					}
				}()
			}

			wg.Wait()
			duration := time.Since(startTime)

			// Business Validation: Performance and reliability
			finalReads := atomic.LoadInt64(&totalReads)
			finalErrors := atomic.LoadInt64(&readErrors)
			expectedReads := int64(numReaders * readsPerReader)

			Expect(finalReads).To(Equal(expectedReads),
				"All reads should be completed")
			Expect(finalErrors).To(Equal(int64(0)),
				"No read errors should occur")

			// Performance validation: should complete within reasonable time
			readsPerSecond := float64(finalReads) / duration.Seconds()
			Expect(readsPerSecond).To(BeNumerically(">", 10000),
				"Should achieve at least 10,000 reads per second")
		})
	})
})
