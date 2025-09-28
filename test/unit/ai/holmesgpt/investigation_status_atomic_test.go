package holmesgpt_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInvestigationStatusAtomic(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Investigation Status Atomic Operations Suite")
}

var _ = Describe("Investigation Status Atomic Operations", func() {

	// Business Requirement: BR-EXTERNAL-005 - Investigation state management
	Context("BR-EXTERNAL-005: Atomic Investigation Status Management", func() {
		It("should handle concurrent status updates without race conditions", func() {
			// Arrange: Create test investigation status container
			type InvestigationStatusContainer struct {
				status        int64 // 0=active, 1=paused, 2=completed, 3=failed
				updateTime    int64 // Unix timestamp
				mu            sync.RWMutex
				investigation *holmesgpt.ActiveInvestigation
			}

			container := &InvestigationStatusContainer{
				status:     0, // active
				updateTime: time.Now().Unix(),
				investigation: &holmesgpt.ActiveInvestigation{
					InvestigationID: "test-investigation-001",
					AlertType:       "kubernetes",
					Namespace:       "test-namespace",
					StartTime:       time.Now(),
					Status:          "active",
					LastActivity:    time.Now(),
				},
			}

			// Business Validation: Test atomic status transitions
			const numGoroutines = 50
			const operationsPerGoroutine = 100

			var wg sync.WaitGroup
			var statusUpdateCount int64
			var successfulUpdates int64
			var corruptionDetected int64

			// Act: Simulate concurrent status updates
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < operationsPerGoroutine; j++ {
						atomic.AddInt64(&statusUpdateCount, 1)

						// Atomic read of current status
						currentStatus := atomic.LoadInt64(&container.status)
						currentTime := atomic.LoadInt64(&container.updateTime)

						// Validate status consistency
						if currentStatus < 0 || currentStatus > 3 {
							atomic.AddInt64(&corruptionDetected, 1)
							continue
						}

						// Simulate status transition (only allow forward progression)
						newStatus := currentStatus
						if workerID%4 == 0 && currentStatus < 2 { // Some workers try to complete
							newStatus = 2 // completed
						} else if workerID%3 == 0 && currentStatus < 1 { // Some workers try to pause
							newStatus = 1 // paused
						}

						// Atomic update with proper ordering
						newTime := time.Now().Unix()
						if newStatus != currentStatus && newTime > currentTime {
							// Try to update atomically
							if atomic.CompareAndSwapInt64(&container.status, currentStatus, newStatus) {
								atomic.StoreInt64(&container.updateTime, newTime)
								atomic.AddInt64(&successfulUpdates, 1)

								// Also update the investigation struct with proper locking
								container.mu.Lock()
								switch newStatus {
								case 0:
									container.investigation.Status = "active"
								case 1:
									container.investigation.Status = "paused"
								case 2:
									container.investigation.Status = "completed"
								case 3:
									container.investigation.Status = "failed"
								}
								container.investigation.LastActivity = time.Unix(newTime, 0)
								container.mu.Unlock()
							}
						}

						// Verify investigation struct consistency
						container.mu.RLock()
						invStatus := container.investigation.Status
						invTime := container.investigation.LastActivity
						container.mu.RUnlock()

						// Check for corruption between atomic status and investigation struct
						atomicStatus := atomic.LoadInt64(&container.status)
						expectedStatus := ""
						switch atomicStatus {
						case 0:
							expectedStatus = "active"
						case 1:
							expectedStatus = "paused"
						case 2:
							expectedStatus = "completed"
						case 3:
							expectedStatus = "failed"
						}

						if invStatus != expectedStatus {
							atomic.AddInt64(&corruptionDetected, 1)
						}

						// Verify timestamp ordering
						if invTime.Unix() > time.Now().Unix()+1 { // Allow 1 second tolerance
							atomic.AddInt64(&corruptionDetected, 1)
						}

						// Small delay to increase race condition probability
						time.Sleep(time.Microsecond * 5)
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

			// Business Validation: Verify atomic operations integrity
			totalOperations := int64(numGoroutines * operationsPerGoroutine)
			finalStatusUpdates := atomic.LoadInt64(&statusUpdateCount)
			finalSuccessfulUpdates := atomic.LoadInt64(&successfulUpdates)
			finalCorruption := atomic.LoadInt64(&corruptionDetected)

			Expect(finalStatusUpdates).To(Equal(totalOperations),
				"All status update attempts should be counted")
			Expect(finalCorruption).To(Equal(int64(0)),
				"No data corruption should occur during concurrent atomic operations")
			Expect(finalSuccessfulUpdates).To(BeNumerically(">=", 0),
				"Successful updates should be non-negative")

			// Final consistency check
			finalStatus := atomic.LoadInt64(&container.status)
			finalTime := atomic.LoadInt64(&container.updateTime)

			container.mu.RLock()
			finalInvStatus := container.investigation.Status
			finalInvTime := container.investigation.LastActivity
			container.mu.RUnlock()

			// Validate final state consistency
			Expect(finalStatus).To(BeNumerically(">=", 0))
			Expect(finalStatus).To(BeNumerically("<=", 3))
			Expect(finalTime).To(BeNumerically(">", 0))

			// Verify investigation struct matches atomic status
			expectedFinalStatus := ""
			switch finalStatus {
			case 0:
				expectedFinalStatus = "active"
			case 1:
				expectedFinalStatus = "paused"
			case 2:
				expectedFinalStatus = "completed"
			case 3:
				expectedFinalStatus = "failed"
			}
			Expect(finalInvStatus).To(Equal(expectedFinalStatus))
			Expect(finalInvTime.Unix()).To(Equal(finalTime))
		})

		It("should prevent status corruption during high-frequency concurrent access", func() {
			// Arrange: Create investigation container with atomic counters
			type AtomicInvestigationMetrics struct {
				activeCount     int64
				completedCount  int64
				failedCount     int64
				totalCount      int64
				corruptionCount int64
			}

			metrics := &AtomicInvestigationMetrics{}
			investigations := make([]*holmesgpt.ActiveInvestigation, 100)

			// Add mutex for thread-safe status transitions following Go coding standards
			var statusMutex sync.Mutex

			// Initialize test investigations
			for i := range investigations {
				investigations[i] = &holmesgpt.ActiveInvestigation{
					InvestigationID: fmt.Sprintf("test-inv-%03d", i),
					AlertType:       "kubernetes",
					Namespace:       "test-ns",
					StartTime:       time.Now(),
					Status:          "active",
					LastActivity:    time.Now(),
				}
				atomic.AddInt64(&metrics.activeCount, 1)
				atomic.AddInt64(&metrics.totalCount, 1)
			}

			// Act: Concurrent status transitions
			const numWorkers = 20
			var wg sync.WaitGroup

			for w := 0; w < numWorkers; w++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for i := 0; i < 200; i++ {
						invIndex := (workerID*200 + i) % len(investigations)
						inv := investigations[invIndex]

						// Simulate status transition with proper synchronization
						if workerID%3 == 0 { // Complete some investigations
							statusMutex.Lock()
							if inv.Status == "active" {
								inv.Status = "completed"
								inv.LastActivity = time.Now()

								atomic.AddInt64(&metrics.activeCount, -1)
								atomic.AddInt64(&metrics.completedCount, 1)
							}
							statusMutex.Unlock()
						} else if workerID%5 == 0 { // Fail some investigations
							statusMutex.Lock()
							if inv.Status == "active" {
								inv.Status = "failed"
								inv.LastActivity = time.Now()

								atomic.AddInt64(&metrics.activeCount, -1)
								atomic.AddInt64(&metrics.failedCount, 1)
							}
							statusMutex.Unlock()
						}

						// Verify metrics consistency
						active := atomic.LoadInt64(&metrics.activeCount)
						completed := atomic.LoadInt64(&metrics.completedCount)
						failed := atomic.LoadInt64(&metrics.failedCount)
						total := atomic.LoadInt64(&metrics.totalCount)

						if active+completed+failed != total {
							atomic.AddInt64(&metrics.corruptionCount, 1)
						}

						if active < 0 || completed < 0 || failed < 0 {
							atomic.AddInt64(&metrics.corruptionCount, 1)
						}

						time.Sleep(time.Microsecond * 2)
					}
				}(w)
			}

			wg.Wait()

			// Business Validation: No corruption should occur
			finalCorruption := atomic.LoadInt64(&metrics.corruptionCount)
			finalActive := atomic.LoadInt64(&metrics.activeCount)
			finalCompleted := atomic.LoadInt64(&metrics.completedCount)
			finalFailed := atomic.LoadInt64(&metrics.failedCount)
			finalTotal := atomic.LoadInt64(&metrics.totalCount)

			Expect(finalCorruption).To(Equal(int64(0)),
				"No metric corruption should occur during concurrent operations")
			Expect(finalActive+finalCompleted+finalFailed).To(Equal(finalTotal),
				"Investigation counts should remain consistent")
			Expect(finalActive).To(BeNumerically(">=", 0))
			Expect(finalCompleted).To(BeNumerically(">=", 0))
			Expect(finalFailed).To(BeNumerically(">=", 0))
		})
	})
})
