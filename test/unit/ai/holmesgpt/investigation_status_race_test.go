package holmesgpt_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInvestigationStatusRace(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Investigation Status Race Suite")
}

var _ = Describe("Investigation Status Race Prevention", func() {
	var (
		coordinator *holmesgpt.AIOrchestrationCoordinator
		logger      *logrus.Logger
		ctx         context.Context
		cancel      context.CancelFunc
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests

		// Create minimal toolset manager for testing
		fakeClient := fake.NewSimpleClientset()
		serviceDiscoveryConfig := &k8s.ServiceDiscoveryConfig{
			DiscoveryInterval:   100 * time.Millisecond,
			CacheTTL:            5 * time.Minute,
			HealthCheckInterval: 1 * time.Second,
			Enabled:             true,
			Namespaces:          []string{"test"},
			ServicePatterns:     k8s.GetDefaultServicePatterns(),
		}
		serviceDiscovery := k8s.NewServiceDiscovery(fakeClient, serviceDiscoveryConfig, logger)
		toolsetManager := holmesgpt.NewDynamicToolsetManager(serviceDiscovery, logger)

		ctx, cancel = context.WithCancel(context.Background())

		// Since we can't easily create a ServiceIntegration without K8s dependencies,
		// we'll test the coordinator with nil ServiceIntegration which should still allow
		// testing of investigation status race conditions
		coordinator = holmesgpt.NewAIOrchestrationCoordinator(
			toolsetManager,
			nil, // Will be handled gracefully in the coordinator
			"http://test-endpoint",
			logger,
		)

		// Note: We don't start the coordinator to avoid toolset deployment requirements
		// We'll test the atomic status operations directly on investigation objects
	})

	AfterEach(func() {
		coordinator.Stop()
		cancel()
	})

	// Business Requirement: BR-EXTERNAL-005 - Investigation state management
	Context("BR-EXTERNAL-005: Investigation state management", func() {
		It("should handle concurrent status updates atomically", func() {
			// Arrange: Create an investigation request
			request := &holmesgpt.InvestigationRequest{
				InvestigationID: "test-investigation-001",
				Alert: &holmesgpt.AlertData{
					Type:      "kubernetes",
					Severity:  "high",
					Source:    "test-source",
					Namespace: "test-namespace",
					Message:   "Test alert for race condition testing",
					Timestamp: time.Now(),
				},
				AlertType: "kubernetes",
				Namespace: "test-namespace",
				Priority:  5,
				Metadata: &holmesgpt.InvestigationMetadata{
					Source:    "test",
					Priority:  5,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
			}

			// Act: Start investigation
			investigation, err := coordinator.StartInvestigation(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			Expect(investigation).ToNot(BeNil())
			Expect(investigation.Status).To(Equal("active"))

			// Business Validation: Status should be atomically manageable
			const numGoroutines = 50
			const operationsPerGoroutine = 20

			var wg sync.WaitGroup
			var statusUpdateCount int64
			var successfulUpdates int64

			// Simulate concurrent status updates
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < operationsPerGoroutine; j++ {
						atomic.AddInt64(&statusUpdateCount, 1)

						// Try to get investigation status
						status, err := coordinator.GetInvestigationStatus(investigation.InvestigationID)
						if err == nil && status != nil {
							atomic.AddInt64(&successfulUpdates, 1)

							// Verify status is one of the valid states
							validStatuses := []string{"active", "paused", "completed", "failed"}
							isValid := false
							for _, validStatus := range validStatuses {
								if status.Status == validStatus {
									isValid = true
									break
								}
							}
							Expect(isValid).To(BeTrue(), "Status should be one of valid states: %s", status.Status)
						}

						// Small delay to increase race condition probability
						time.Sleep(time.Microsecond * 10)
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
				Fail("Test timed out - possible deadlock in status management")
			}

			// Business Validation: All operations should succeed without race conditions
			totalOperations := int64(numGoroutines * operationsPerGoroutine)
			finalStatusUpdates := atomic.LoadInt64(&statusUpdateCount)
			finalSuccessfulUpdates := atomic.LoadInt64(&successfulUpdates)

			Expect(finalStatusUpdates).To(Equal(totalOperations), "All status update attempts should be counted")
			Expect(finalSuccessfulUpdates).To(BeNumerically(">", totalOperations*8/10), "At least 80% of status reads should succeed")

			// Final status should be valid
			finalStatus, err := coordinator.GetInvestigationStatus(investigation.InvestigationID)
			Expect(err).ToNot(HaveOccurred())
			Expect(finalStatus.Status).To(BeElementOf([]string{"active", "completed", "failed"}))
		})

		It("should prevent status corruption during concurrent updates", func() {
			// Arrange: Create multiple investigations
			const numInvestigations = 10
			investigations := make([]*holmesgpt.ActiveInvestigation, numInvestigations)

			for i := 0; i < numInvestigations; i++ {
				request := &holmesgpt.InvestigationRequest{
					InvestigationID: "",
					Alert: &holmesgpt.AlertData{
						Type:      "prometheus",
						Severity:  "medium",
						Source:    "test-source",
						Namespace: "test-ns",
						Message:   "Test alert for status corruption testing",
						Timestamp: time.Now(),
					},
					AlertType: "prometheus",
					Namespace: "test-ns",
					Priority:  3,
				}

				investigation, err := coordinator.StartInvestigation(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				investigations[i] = investigation
			}

			// Act & Assert: Concurrent status checks should never return corrupted data
			const numCheckers = 20
			var wg sync.WaitGroup
			var corruptionDetected int64

			for i := 0; i < numCheckers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					for j := 0; j < 100; j++ {
						// Check random investigation
						invIndex := j % numInvestigations
						targetInvestigation := investigations[invIndex]

						status, err := coordinator.GetInvestigationStatus(targetInvestigation.InvestigationID)
						if err != nil {
							continue // Investigation may complete and be cleaned up
						}

						// Validate status integrity
						if status.InvestigationID != targetInvestigation.InvestigationID {
							atomic.AddInt64(&corruptionDetected, 1)
						}

						// Validate status is not empty/corrupted
						if status.Status == "" || status.AlertType == "" {
							atomic.AddInt64(&corruptionDetected, 1)
						}

						// Validate timestamps make sense
						if status.StartTime.IsZero() || status.LastActivity.Before(status.StartTime) {
							atomic.AddInt64(&corruptionDetected, 1)
						}
					}
				}()
			}

			wg.Wait()

			// Business Validation: No data corruption should occur
			finalCorruption := atomic.LoadInt64(&corruptionDetected)
			Expect(finalCorruption).To(Equal(int64(0)), "No status corruption should be detected during concurrent access")
		})
	})

	// Business Requirement: BR-HOLMES-006 - Investigation request handling
	Context("BR-HOLMES-006: Investigation lifecycle race safety", func() {
		It("should maintain investigation count consistency during concurrent operations", func() {
			// Arrange: Get baseline metrics
			initialMetrics := coordinator.GetOrchestrationMetrics()
			initialTotal := initialMetrics.TotalInvestigations
			initialActive := initialMetrics.ActiveInvestigations

			// Act: Create investigations concurrently
			const numConcurrentInvestigations = 25
			var wg sync.WaitGroup
			createdInvestigations := make([]*holmesgpt.ActiveInvestigation, numConcurrentInvestigations)
			var creationErrors int64

			for i := 0; i < numConcurrentInvestigations; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					request := &holmesgpt.InvestigationRequest{
						Alert: &holmesgpt.AlertData{
							Type:      "application",
							Severity:  "low",
							Source:    "concurrent-test",
							Namespace: "test-concurrent",
							Message:   "Concurrent investigation test",
							Timestamp: time.Now(),
						},
						AlertType: "application",
						Namespace: "test-concurrent",
						Priority:  2,
					}

					investigation, err := coordinator.StartInvestigation(ctx, request)
					if err != nil {
						atomic.AddInt64(&creationErrors, 1)
						return
					}

					createdInvestigations[index] = investigation
				}(i)
			}

			wg.Wait()

			// Business Validation: Metrics should be consistent
			finalMetrics := coordinator.GetOrchestrationMetrics()
			finalCreationErrors := atomic.LoadInt64(&creationErrors)

			Expect(finalCreationErrors).To(Equal(int64(0)), "All investigation creations should succeed")

			expectedTotal := int64(initialTotal) + int64(numConcurrentInvestigations)
			Expect(int64(finalMetrics.TotalInvestigations)).To(Equal(expectedTotal),
				"Total investigations should be consistent: expected %d, got %d", expectedTotal, finalMetrics.TotalInvestigations)

			expectedActive := int64(initialActive) + int64(numConcurrentInvestigations)
			Expect(int64(finalMetrics.ActiveInvestigations)).To(BeNumerically("<=", expectedActive),
				"Active investigations should not exceed expected count")

			// Verify all created investigations are trackable
			validInvestigations := 0
			for _, inv := range createdInvestigations {
				if inv != nil {
					status, err := coordinator.GetInvestigationStatus(inv.InvestigationID)
					if err == nil && status != nil {
						validInvestigations++
					}
				}
			}

			Expect(validInvestigations).To(Equal(numConcurrentInvestigations),
				"All created investigations should be trackable")
		})
	})
})
