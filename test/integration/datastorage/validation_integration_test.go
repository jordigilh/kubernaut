package datastorage

import (
	"context"
	"database/sql"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/database/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

var _ = Describe("Integration Test 4: Validation + Sanitization Pipeline", func() {
	var (
		testCtx     context.Context
		testDB      *sql.DB
		testSchema  string
		initializer *schema.Initializer
		validator   *validation.Validator
	)

	BeforeEach(func() {
		testCtx = context.Background()

		// Create isolated test schema
		testSchema = "test_validation_" + time.Now().Format("20060102_150405")
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

		// Create validator
		validator = validation.NewValidator(logger)

		GinkgoWriter.Printf("✅ Test schema %s initialized for validation tests\n", testSchema)
	})

	AfterEach(func() {
		if testDB != nil {
			_ = testDB.Close()
		}
		_, _ = db.ExecContext(testCtx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE")
		GinkgoWriter.Printf("✅ Test schema %s cleaned up\n", testSchema)
	})

	Context("BR-STORAGE-010: Validation pipeline integration", func() {
		It("should reject invalid audit (missing required fields)", func() {
			// Create invalid audit (missing name)
			invalidAudit := &models.RemediationAudit{
				Namespace: "default",
				Phase:     "processing",
			}

			// Validation should fail
			err := validator.ValidateRemediationAudit(invalidAudit)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name is required"))

			GinkgoWriter.Println("✅ Invalid audit rejected by validation")
		})

		It("should reject invalid phase values", func() {
			invalidAudit := &models.RemediationAudit{
				Name:      "invalid-phase-test",
				Namespace: "default",
				Phase:     "invalid_phase_value",
			}

			err := validator.ValidateRemediationAudit(invalidAudit)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid phase"))

			GinkgoWriter.Println("✅ Invalid phase rejected by validation")
		})

		It("should reject fields exceeding length limits", func() {
			longNameAudit := &models.RemediationAudit{
				Name:       strings.Repeat("a", 256), // Exceeds 255 char limit
				Namespace:  "default",
				Phase:      "processing",
				ActionType: "restart_pod", // Provide required field
			}

			err := validator.ValidateRemediationAudit(longNameAudit)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds maximum length"))

			GinkgoWriter.Println("✅ Overlong field rejected by validation")
		})

		It("should accept valid audit", func() {
			validAudit := &models.RemediationAudit{
				Name:                 "valid-test",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-valid-001",
				AlertFingerprint:     "alert-valid",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{}`,
			}

			err := validator.ValidateRemediationAudit(validAudit)
			Expect(err).ToNot(HaveOccurred())

			// Should also successfully insert to database
			query := `
				INSERT INTO remediation_audit (
					name, namespace, phase, action_type, status, start_time,
					remediation_request_id, alert_fingerprint, severity,
					environment, cluster_name, target_resource, metadata
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
				) RETURNING id
			`

			var id int64
			err = testDB.QueryRowContext(testCtx, query,
				validAudit.Name, validAudit.Namespace, validAudit.Phase,
				validAudit.ActionType, validAudit.Status, validAudit.StartTime,
				validAudit.RemediationRequestID, validAudit.AlertFingerprint,
				validAudit.Severity, validAudit.Environment, validAudit.ClusterName,
				validAudit.TargetResource, validAudit.Metadata,
			).Scan(&id)

			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))

			GinkgoWriter.Println("✅ Valid audit accepted and persisted")
		})
	})

	Context("BR-STORAGE-010: Sanitization pipeline integration", func() {
		It("should sanitize XSS attack patterns", func() {
			maliciousInput := `<script>alert('XSS')</script>Legitimate text`
			sanitized := validator.SanitizeString(maliciousInput)

			Expect(sanitized).ToNot(ContainSubstring("<script>"))
			Expect(sanitized).ToNot(ContainSubstring("alert"))
			Expect(sanitized).To(ContainSubstring("Legitimate text"))

			GinkgoWriter.Printf("✅ XSS pattern sanitized: %q → %q\n", maliciousInput, sanitized)
		})

		It("should sanitize SQL injection patterns", func() {
			maliciousInput := `'; DROP TABLE users; --`
			sanitized := validator.SanitizeString(maliciousInput)

			// Should strip dangerous SQL keywords
			Expect(sanitized).ToNot(ContainSubstring("DROP"))
			Expect(sanitized).ToNot(ContainSubstring("TABLE"))

			GinkgoWriter.Printf("✅ SQL injection pattern sanitized: %q → %q\n", maliciousInput, sanitized)
		})

		It("should preserve safe content during sanitization", func() {
			safeInput := "Normal audit message with numbers 123 and symbols -_."
			sanitized := validator.SanitizeString(safeInput)

			Expect(sanitized).To(Equal(safeInput))

			GinkgoWriter.Println("✅ Safe content preserved during sanitization")
		})

		It("should sanitize and store audit with malicious metadata", func() {
			audit := &models.RemediationAudit{
				Name:                 "sanitization-test",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-sanitize-001",
				AlertFingerprint:     "alert-sanitize",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{"reason":"<script>alert('XSS')</script>"}`,
			}

			// Sanitize metadata
			audit.Metadata = validator.SanitizeString(audit.Metadata)

			// Validate
			err := validator.ValidateRemediationAudit(audit)
			Expect(err).ToNot(HaveOccurred())

			// Insert to database
			query := `
				INSERT INTO remediation_audit (
					name, namespace, phase, action_type, status, start_time,
					remediation_request_id, alert_fingerprint, severity,
					environment, cluster_name, target_resource, metadata
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
				) RETURNING id, metadata
			`

			var id int64
			var storedMetadata string
			err = testDB.QueryRowContext(testCtx, query,
				audit.Name, audit.Namespace, audit.Phase, audit.ActionType,
				audit.Status, audit.StartTime, audit.RemediationRequestID,
				audit.AlertFingerprint, audit.Severity, audit.Environment,
				audit.ClusterName, audit.TargetResource, audit.Metadata,
			).Scan(&id, &storedMetadata)

			Expect(err).ToNot(HaveOccurred())
			Expect(storedMetadata).ToNot(ContainSubstring("<script>"))

			GinkgoWriter.Printf("✅ Sanitized metadata stored: %q\n", storedMetadata)
		})
	})

	Context("BR-STORAGE-010: End-to-end validation + sanitization", func() {
		It("should demonstrate complete pipeline: validate → sanitize → persist", func() {
			// Raw input with potential issues
			rawAudit := &models.RemediationAudit{
				Name:                 "e2e-validation-test",
				Namespace:            "default",
				Phase:                "processing",
				ActionType:           "restart_pod",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-e2e-001",
				AlertFingerprint:     "alert-e2e",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "pod/app",
				Metadata:             `{"reason":"High CPU <b>bold text</b>"}`,
			}

			// Step 1: Validate structure
			err := validator.ValidateRemediationAudit(rawAudit)
			Expect(err).ToNot(HaveOccurred())

			// Step 2: Sanitize content
			rawAudit.Metadata = validator.SanitizeString(rawAudit.Metadata)
			rawAudit.TargetResource = validator.SanitizeString(rawAudit.TargetResource)

			// Step 3: Persist to database
			query := `
				INSERT INTO remediation_audit (
					name, namespace, phase, action_type, status, start_time,
					remediation_request_id, alert_fingerprint, severity,
					environment, cluster_name, target_resource, metadata
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
				) RETURNING id
			`

			var id int64
			err = testDB.QueryRowContext(testCtx, query,
				rawAudit.Name, rawAudit.Namespace, rawAudit.Phase,
				rawAudit.ActionType, rawAudit.Status, rawAudit.StartTime,
				rawAudit.RemediationRequestID, rawAudit.AlertFingerprint,
				rawAudit.Severity, rawAudit.Environment, rawAudit.ClusterName,
				rawAudit.TargetResource, rawAudit.Metadata,
			).Scan(&id)

			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))

			GinkgoWriter.Println("✅ Complete pipeline validated: validate → sanitize → persist")
		})
	})
})
