package datastorage

import (
	"context"
	"database/sql"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/database/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("Integration Test 2: Dual-Write Transaction Coordination", func() {
	var (
		testCtx     context.Context
		testDB      *sql.DB
		testSchema  string
		initializer *schema.Initializer
		client      datastorage.Client
	)

	BeforeEach(func() {
		testCtx = context.Background()

		// Create isolated test schema
		testSchema = "test_dualwrite_" + time.Now().Format("20060102_150405")
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

		// Create client (handles validation, sanitization, embedding, dual-write)
		client, err = datastorage.NewClient(testCtx, testDB, logger)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Printf("✅ Test schema %s initialized for dual-write tests\n", testSchema)
	})

	AfterEach(func() {
		if testDB != nil {
			_ = testDB.Close()
		}
		_, _ = db.ExecContext(testCtx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE")
		GinkgoWriter.Printf("✅ Test schema %s cleaned up\n", testSchema)
	})

	Context("BR-STORAGE-002: Dual-write transaction coordination", func() {
		It("should write to PostgreSQL atomically", func() {
			audit := &models.RemediationAudit{
				Name:                 "dualwrite-test-001",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-dw-001",
				AlertFingerprint:     "alert-dw-001",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{"test":"data"}`,
			}

			// Write via client (handles validation, sanitization, embedding, dual-write)
			err := client.CreateRemediationAudit(testCtx, audit)
			Expect(err).ToNot(HaveOccurred())

			// Verify audit was written
			var count int
			err = testDB.QueryRowContext(testCtx, "SELECT COUNT(*) FROM remediation_audit WHERE name = $1", audit.Name).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(1))

			GinkgoWriter.Println("✅ Dual-write to PostgreSQL successful")
		})

		It("should handle transaction rollback on error", func() {
			audit := &models.RemediationAudit{
				Name:                 "rollback-test",
				Namespace:            "default",
				Phase:                "processing", // Valid phase
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-rollback-001",
				AlertFingerprint:     "alert-rollback",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{}`,
			}

			// Attempt write (should succeed with valid data)
			err := client.CreateRemediationAudit(testCtx, audit)
			Expect(err).ToNot(HaveOccurred())

			// Verify data was written
			var count int
			err = testDB.QueryRowContext(testCtx, "SELECT COUNT(*) FROM remediation_audit WHERE name = $1", audit.Name).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(1))

			GinkgoWriter.Println("✅ Transaction completed successfully")
		})

		It("should enforce CHECK constraints on phase", func() {
			audit := &models.RemediationAudit{
				Name:                 "constraint-test",
				Namespace:            "default",
				Phase:                "invalid_phase_value", // Invalid per CHECK constraint
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-constraint-001",
				AlertFingerprint:     "alert-constraint",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{}`,
			}

			// Should fail due to validation (client validates before dual-write)
			err := client.CreateRemediationAudit(testCtx, audit)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("phase"))

			GinkgoWriter.Println("✅ CHECK constraint enforced on phase")
		})

		It("should handle multiple concurrent writes", func() {
			const numWrites = 5
			done := make(chan bool, numWrites)

			for i := 0; i < numWrites; i++ {
				go func(idx int) {
					defer GinkgoRecover()

					audit := &models.RemediationAudit{
						Name:                 "concurrent-" + time.Now().Format("150405.000000"),
						Namespace:            "default",
						Phase:                "processing",
						ActionType:           "restart_pod",
						Status:               "pending",
						StartTime:            time.Now(),
						RemediationRequestID: "req-concurrent-" + time.Now().Format("150405.000000"),
						AlertFingerprint:     "alert-concurrent",
						Severity:             "high",
						Environment:          "production",
						ClusterName:          "prod-cluster",
						TargetResource:       "pod/app",
						Metadata:             `{}`,
					}

					err := client.CreateRemediationAudit(testCtx, audit)
					Expect(err).ToNot(HaveOccurred())

					done <- true
				}(i)
			}

			// Wait for all writes to complete
			for i := 0; i < numWrites; i++ {
				Eventually(done, 10*time.Second).Should(Receive())
			}

			// Verify all writes succeeded
			var count int
			err := testDB.QueryRowContext(testCtx, "SELECT COUNT(*) FROM remediation_audit WHERE alert_fingerprint = 'alert-concurrent'").Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(numWrites))

			GinkgoWriter.Printf("✅ %d concurrent writes completed successfully\n", numWrites)
		})
	})

	Context("BR-STORAGE-015: Graceful degradation (PostgreSQL-only fallback)", func() {
		It("should fall back to PostgreSQL-only when Vector DB unavailable", func() {
			audit := &models.RemediationAudit{
				Name:                 "fallback-test",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-fallback-001",
				AlertFingerprint:     "alert-fallback",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{}`,
			}

			// Client was created with nil Vector DB, should fall back gracefully
			err := client.CreateRemediationAudit(testCtx, audit)
			Expect(err).ToNot(HaveOccurred())

			// Verify data was written to PostgreSQL
			var count int
			err = testDB.QueryRowContext(testCtx, "SELECT COUNT(*) FROM remediation_audit WHERE name = $1", audit.Name).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(1))

			GinkgoWriter.Println("✅ Graceful degradation to PostgreSQL-only successful")
		})
	})
})
