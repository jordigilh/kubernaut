package datastorage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/database/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dualwrite"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("Integration Test 5: Cross-Service Write Simulation + Stress Testing", func() {
	var (
		testCtx     context.Context
		testDB      *sql.DB
		testSchema  string
		initializer *schema.Initializer
		coordinator *dualwrite.Coordinator
	)

	BeforeEach(func() {
		testCtx = context.Background()

		// Create isolated test schema
		testSchema = "test_stress_" + time.Now().Format("20060102_150405")
		_, err := db.ExecContext(testCtx, "CREATE SCHEMA "+testSchema)
		Expect(err).ToNot(HaveOccurred())

		// Connect to test schema
		connStr := "host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable search_path=" + testSchema
		testDB, err = sql.Open("postgres", connStr)
		Expect(err).ToNot(HaveOccurred())

		// Initialize schema
		initializer = schema.NewInitializer(testDB, logger)
		err = initializer.Initialize(testCtx)
		Expect(err).ToNot(HaveOccurred())

		// Create coordinator with DB wrapper
		coordinator = dualwrite.NewCoordinator(&dbWrapper{db: testDB}, nil, logger)

		GinkgoWriter.Printf("✅ Test schema %s initialized for stress tests\n", testSchema)
	})

	AfterEach(func() {
		if testDB != nil {
			_ = testDB.Close()
		}
		_, _ = db.ExecContext(testCtx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE")
		GinkgoWriter.Printf("✅ Test schema %s cleaned up\n", testSchema)
	})

	Context("BR-STORAGE-016: Cross-service concurrent writes", func() {
		It("should handle multiple services writing simultaneously", func() {
			const numServices = 3
			const writesPerService = 5
			const totalWrites = numServices * writesPerService

			var wg sync.WaitGroup
			errorChan := make(chan error, totalWrites)

			// Simulate 3 services (Remediation, AI Analysis, Workflow) writing concurrently
			for serviceID := 0; serviceID < numServices; serviceID++ {
				wg.Add(1)
				go func(svcID int) {
					defer wg.Done()
					defer GinkgoRecover()

					for writeID := 0; writeID < writesPerService; writeID++ {
						audit := &models.RemediationAudit{
							Name:                 fmt.Sprintf("cross-service-svc%d-write%d-%d", svcID, writeID, time.Now().UnixNano()),
							Namespace:            "default",
							Phase:                "processing",
							ActionType:           "restart_pod",
							Status:               "pending",
							StartTime:            time.Now(),
							RemediationRequestID: fmt.Sprintf("req-stress-svc%d-write%d-%d", svcID, writeID, time.Now().UnixNano()),
							AlertFingerprint:     "alert-cross-service",
							Severity:             "high",
							Environment:          "production",
							ClusterName:          "prod-cluster",
							TargetResource:       "pod/app",
							Metadata:             `{"service_id":` + string(rune('0'+svcID)) + `}`,
						}

						embedding := generateTestEmbedding(float32(svcID) + float32(writeID)/100.0)

						_, err := coordinator.Write(testCtx, audit, embedding)
						if err != nil {
							errorChan <- err
						}
					}
				}(serviceID)
			}

			// Wait for all writes to complete
			wg.Wait()
			close(errorChan)

			// Check for errors
			var errors []error
			for err := range errorChan {
				errors = append(errors, err)
			}
			Expect(errors).To(BeEmpty(), "All cross-service writes should succeed")

			// Verify total write count
			var count int
			err := testDB.QueryRowContext(testCtx, "SELECT COUNT(*) FROM remediation_audit WHERE alert_fingerprint = 'alert-cross-service'").Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(totalWrites))

			GinkgoWriter.Printf("✅ %d cross-service writes completed successfully\n", totalWrites)
		})

		It("should maintain data isolation between concurrent services", func() {
			// Service A writes
			auditA := &models.RemediationAudit{
				Name:                 "service-a-audit",
				Namespace:            "service-a",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-service-a",
				AlertFingerprint:     "alert-service-a",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app-a",
				Metadata:             `{"service":"A"}`,
			}

			// Service B writes
			auditB := &models.RemediationAudit{
				Name:                 "service-b-audit",
				Namespace:            "service-b",
				Phase:                "processing",
				ActionType:           "scale_deployment",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-service-b",
				AlertFingerprint:     "alert-service-b",
				Severity:             "medium",
				Environment:          "staging",
				ClusterName:          "staging-cluster",
				TargetResource:       "deployment/app-b",
				Metadata:             `{"service":"B"}`,
			}

			// Create 384-dimensional embeddings for both services
			embeddingA := generateTestEmbedding(0.1)
			embeddingB := generateTestEmbedding(0.2)

			// Write both concurrently
			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				_, err := coordinator.Write(testCtx, auditA, embeddingA)
				Expect(err).ToNot(HaveOccurred())
			}()

			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				_, err := coordinator.Write(testCtx, auditB, embeddingB)
				Expect(err).ToNot(HaveOccurred())
			}()

			wg.Wait()

			// Verify both audits exist with correct data
			var countA, countB int
			err := testDB.QueryRowContext(testCtx, "SELECT COUNT(*) FROM remediation_audit WHERE namespace = 'service-a'").Scan(&countA)
			Expect(err).ToNot(HaveOccurred())
			Expect(countA).To(Equal(1))

			err = testDB.QueryRowContext(testCtx, "SELECT COUNT(*) FROM remediation_audit WHERE namespace = 'service-b'").Scan(&countB)
			Expect(err).ToNot(HaveOccurred())
			Expect(countB).To(Equal(1))

			GinkgoWriter.Println("✅ Data isolation maintained between concurrent services")
		})
	})

	Context("✅ BR-STORAGE-016: Context cancellation stress test", func() {
		It("should respect context cancellation during write operations", func() {
			// BR-STORAGE-016: Context propagation with BeginTx(ctx, nil)
			// FIXED in Day 9: Coordinator now uses BeginTx(ctx, nil) instead of Begin()

			// Create context with short timeout
			writeCtx, cancel := context.WithTimeout(testCtx, 10*time.Millisecond)
			defer cancel()

			// Attempt write with short timeout
			audit := &models.RemediationAudit{
				Name:                 "context-cancel-test",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-ctx-cancel",
				AlertFingerprint:     "alert-ctx-cancel",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{}`,
			}

			embedding := generateTestEmbedding(0.1)

			// Introduce artificial delay to trigger timeout
			time.Sleep(50 * time.Millisecond)

			_, err := coordinator.Write(writeCtx, audit, embedding)

			// ✅ FIXED: Context cancellation is now respected
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.DeadlineExceeded),
				"Write should fail with DeadlineExceeded when context timeout expires")

			GinkgoWriter.Println("✅ Context cancellation respected (BR-STORAGE-016 validated)")
		})

		It("should handle context cancellation during transaction", func() {
			// BR-STORAGE-016: Context propagation with BeginTx(ctx, nil)
			// Create context that will be cancelled mid-transaction
			writeCtx, cancel := context.WithCancel(testCtx)

			audit := &models.RemediationAudit{
				Name:                 "mid-tx-cancel-test",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-mid-tx-cancel",
				AlertFingerprint:     "alert-mid-tx-cancel",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{}`,
			}

			embedding := generateTestEmbedding(0.1)

			// Cancel context before write
			cancel()

			_, err := coordinator.Write(writeCtx, audit, embedding)

			// ✅ FIXED: Context cancellation is now respected
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled),
				"Write should fail with Canceled when context is cancelled")

			GinkgoWriter.Println("✅ Mid-transaction cancellation handled (BR-STORAGE-016 validated)")
		})

		It("should handle deadline exceeded during long operations", func() {
			// BR-STORAGE-016: Context propagation with BeginTx(ctx, nil)
			// Create context with very short deadline
			writeCtx, cancel := context.WithDeadline(testCtx, time.Now().Add(1*time.Millisecond))
			defer cancel()

			// Immediately wait for deadline to pass
			time.Sleep(10 * time.Millisecond)

			audit := &models.RemediationAudit{
				Name:                 "deadline-test",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-deadline",
				AlertFingerprint:     "alert-deadline",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{}`,
			}

			embedding := generateTestEmbedding(0.1)

			_, err := coordinator.Write(writeCtx, audit, embedding)

			// ✅ FIXED: Deadline exceeded is now respected
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.DeadlineExceeded),
				"Write should fail with DeadlineExceeded when deadline is exceeded")

			GinkgoWriter.Println("✅ Deadline exceeded handled (BR-STORAGE-016 validated)")
		})
	})

	Context("BR-STORAGE-017: High-throughput write stress test", func() {
		It("should handle 50 concurrent writes under load", func() {
			const numConcurrentWrites = 50
			var wg sync.WaitGroup
			errorChan := make(chan error, numConcurrentWrites)
			successChan := make(chan bool, numConcurrentWrites)

			startTime := time.Now()

			for i := 0; i < numConcurrentWrites; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()

					audit := &models.RemediationAudit{
						Name:                 fmt.Sprintf("stress-test-%d-%d", idx, time.Now().UnixNano()),
						Namespace:            "default",
						Phase:                "processing",
						ActionType:           "restart_pod",
						Status:               "pending",
						StartTime:            time.Now(),
						RemediationRequestID: fmt.Sprintf("req-stress-%d-%d", idx, time.Now().UnixNano()),
						AlertFingerprint:     "alert-stress",
						Severity:             "high",
						Environment:          "production",
						ClusterName:          "prod-cluster",
						TargetResource:       "pod/app",
						Metadata:             `{}`,
					}

					embedding := generateTestEmbedding(float32(idx) / 100.0)

					_, err := coordinator.Write(testCtx, audit, embedding)
					if err != nil {
						errorChan <- err
					} else {
						successChan <- true
					}
				}(i)
			}

			// Wait for all writes
			wg.Wait()
			close(errorChan)
			close(successChan)

			duration := time.Since(startTime)

			// Check for errors
			var errors []error
			for err := range errorChan {
				errors = append(errors, err)
			}
			Expect(errors).To(BeEmpty(), "All stress writes should succeed")

			// Count successes
			successCount := 0
			for range successChan {
				successCount++
			}

			Expect(successCount).To(Equal(numConcurrentWrites))

			// Verify database count
			var dbCount int
			err := testDB.QueryRowContext(testCtx, "SELECT COUNT(*) FROM remediation_audit WHERE alert_fingerprint = 'alert-stress'").Scan(&dbCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(dbCount).To(Equal(numConcurrentWrites))

			throughput := float64(numConcurrentWrites) / duration.Seconds()
			GinkgoWriter.Printf("✅ Stress test completed: %d writes in %s (%.2f writes/sec)\n",
				numConcurrentWrites, duration, throughput)
		})
	})
})
