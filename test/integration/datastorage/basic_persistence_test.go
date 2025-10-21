package datastorage

import (
	"context"
	"database/sql"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/database/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("Integration Test 1: Basic Audit Persistence", func() {
	var (
		testCtx     context.Context
		testDB      *sql.DB
		testSchema  string
		initializer *schema.Initializer
	)

	BeforeEach(func() {
		testCtx = context.Background()

		// Create isolated test schema for this test
		testSchema = "test_basic_" + time.Now().Format("20060102_150405")
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

		GinkgoWriter.Printf("✅ Test schema %s initialized\n", testSchema)
	})

	AfterEach(func() {
		// Close test database
		if testDB != nil {
			_ = testDB.Close()
		}

		// Drop test schema
		_, _ = db.ExecContext(testCtx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE")
		GinkgoWriter.Printf("✅ Test schema %s cleaned up\n", testSchema)
	})

	Context("BR-STORAGE-001: Basic audit write → PostgreSQL", func() {
		It("should write remediation audit to PostgreSQL", func() {
			// Create audit
			audit := &models.RemediationAudit{
				Name:                 "test-audit-001",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-001",
				AlertFingerprint:     "alert-fp-001",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster-01",
				TargetResource:       "pod/failing-app",
				Metadata:             `{"reason":"high_memory"}`,
			}

			// Insert audit
			query := `
				INSERT INTO remediation_audit (
					name, namespace, phase, action_type, status, start_time,
					remediation_request_id, alert_fingerprint, severity,
					environment, cluster_name, target_resource, metadata
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
				) RETURNING id, created_at, updated_at
			`

			var id int64
			var createdAt, updatedAt time.Time
			err := testDB.QueryRowContext(testCtx, query,
				audit.Name, audit.Namespace, audit.Phase, audit.ActionType,
				audit.Status, audit.StartTime, audit.RemediationRequestID,
				audit.AlertFingerprint, audit.Severity, audit.Environment,
				audit.ClusterName, audit.TargetResource, audit.Metadata,
			).Scan(&id, &createdAt, &updatedAt)

			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))
			Expect(createdAt).To(BeTemporally("~", time.Now(), 5*time.Second))
			Expect(updatedAt).To(BeTemporally("~", time.Now(), 5*time.Second))

			GinkgoWriter.Printf("✅ Audit written with ID: %d\n", id)
		})

		It("should read remediation audit from PostgreSQL", func() {
			// Insert test audit
			query := `
				INSERT INTO remediation_audit (
					name, namespace, phase, action_type, status, start_time,
					remediation_request_id, alert_fingerprint, severity,
					environment, cluster_name, target_resource, metadata
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
				) RETURNING id
			`
			var insertedID int64
			err := testDB.QueryRowContext(testCtx, query,
				"read-test", "default", "processing", "scale_deployment",
				"success", time.Now(), "req-read-001", "alert-read",
				"medium", "staging", "staging-cluster", "deployment/app", "{}",
			).Scan(&insertedID)
			Expect(err).ToNot(HaveOccurred())

			// Read audit back
			selectQuery := `
				SELECT name, namespace, phase, action_type, status,
				       remediation_request_id, alert_fingerprint, severity
				FROM remediation_audit WHERE id = $1
			`
			var audit models.RemediationAudit
			err = testDB.QueryRowContext(testCtx, selectQuery, insertedID).Scan(
				&audit.Name, &audit.Namespace, &audit.Phase, &audit.ActionType,
				&audit.Status, &audit.RemediationRequestID, &audit.AlertFingerprint,
				&audit.Severity,
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(audit.Name).To(Equal("read-test"))
			Expect(audit.Namespace).To(Equal("default"))
			Expect(audit.Phase).To(Equal("processing"))
			Expect(audit.ActionType).To(Equal("scale_deployment"))
			Expect(audit.Status).To(Equal("success"))

			GinkgoWriter.Printf("✅ Audit read successfully: %s\n", audit.Name)
		})

		It("should enforce unique constraint on remediation_request_id", func() {
			// Insert first audit
			query := `
				INSERT INTO remediation_audit (
					name, namespace, phase, action_type, status, start_time,
					remediation_request_id, alert_fingerprint, severity,
					environment, cluster_name, target_resource, metadata
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
				)
			`
			_, err := testDB.ExecContext(testCtx, query,
				"unique-test-1", "default", "processing", "restart_pod",
				"pending", time.Now(), "req-unique-001", "alert-unique",
				"high", "production", "prod-cluster", "pod/app", "{}",
			)
			Expect(err).ToNot(HaveOccurred())

			// Attempt to insert duplicate remediation_request_id
			_, err = testDB.ExecContext(testCtx, query,
				"unique-test-2", "default", "processing", "restart_pod",
				"pending", time.Now(), "req-unique-001", // DUPLICATE
				"alert-unique", "high", "production", "prod-cluster", "pod/app", "{}",
			)

			// Should fail with unique constraint violation
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("remediation_audit_remediation_request_id_key"))

			GinkgoWriter.Println("✅ Unique constraint enforced on remediation_request_id")
		})

		It("should create indexes for performance", func() {
			// Verify indexes exist
			indexQuery := `
				SELECT indexname
				FROM pg_indexes
				WHERE schemaname = $1
				  AND tablename = 'remediation_audit'
				ORDER BY indexname
			`
			rows, err := testDB.QueryContext(testCtx, indexQuery, testSchema)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = rows.Close() }()

			indexes := []string{}
			for rows.Next() {
				var indexName string
				err := rows.Scan(&indexName)
				Expect(err).ToNot(HaveOccurred())
				indexes = append(indexes, indexName)
			}

			// Verify key indexes
			Expect(indexes).To(ContainElement("remediation_audit_pkey"))
			Expect(indexes).To(ContainElement("idx_remediation_audit_namespace"))
			Expect(indexes).To(ContainElement("idx_remediation_audit_phase"))
			Expect(indexes).To(ContainElement("idx_remediation_audit_status"))
			Expect(indexes).To(ContainElement("idx_remediation_audit_start_time"))
			Expect(indexes).To(ContainElement("idx_remediation_audit_alert_fingerprint"))

			GinkgoWriter.Printf("✅ Found %d indexes on remediation_audit\n", len(indexes))
		})
	})
})
