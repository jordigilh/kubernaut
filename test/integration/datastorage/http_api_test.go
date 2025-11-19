package datastorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// HTTP API Integration Tests - POST /api/v1/audit/notifications
// BR-STORAGE-001 to BR-STORAGE-020: Audit write API
// DD-009: DLQ fallback on database errors
//
// These tests validate the complete HTTP → Repository → PostgreSQL flow
// using a real Data Storage Service container (Podman, ADR-016)

var _ = Describe("HTTP API Integration - POST /api/v1/audit/notifications", Serial, Ordered, func() {
	var (
		client     *http.Client
		validAudit *models.NotificationAudit
	)

	BeforeAll(func() {
		client = &http.Client{Timeout: 10 * time.Second}
	})

	BeforeEach(func() {
		// Create unique notification_id to avoid conflicts
		validAudit = &models.NotificationAudit{
			RemediationID:   "test-remediation-1",
			NotificationID:  fmt.Sprintf("test-notification-%d", time.Now().UnixNano()),
			Recipient:       "test@example.com",
			Channel:         "email",
			MessageSummary:  "Test notification message",
			Status:          "sent",
			SentAt:          time.Now().Add(-1 * time.Minute), // 1 minute in the past to avoid clock skew issues
			DeliveryStatus:  "200 OK",
			ErrorMessage:    "",
			EscalationLevel: 0,
		}
	})

	Context("Successful write (Behavior + Correctness)", func() {
		It("should accept valid audit record and persist to PostgreSQL", func() {
			// ✅ BEHAVIOR TEST: HTTP 201 Created
			resp := postAudit(client, validAudit)
			if resp.StatusCode != 201 {
				// Debug: Print response body on failure
				body, _ := io.ReadAll(resp.Body)
				GinkgoWriter.Printf("\n❌ HTTP %d Response Body: %s\n", resp.StatusCode, string(body))

				// Print service logs for debugging
				logs, logErr := exec.Command("podman", "logs", "--tail", "50", "data-storage-service").CombinedOutput()
				if logErr == nil {
					GinkgoWriter.Printf("\n📋 Data Storage Service logs (last 50 lines):\n%s\n", string(logs))
				} else {
					GinkgoWriter.Printf("\n⚠️  Failed to get service logs: %v\n", logErr)
				}
			}
			Expect(resp.StatusCode).To(Equal(201), "Expected 201 Created for valid audit")
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))

			// ✅ CORRECTNESS TEST: Response contains created record
			var created models.NotificationAudit
			err := json.NewDecoder(resp.Body).Decode(&created)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")
			Expect(created.ID).To(BeNumerically(">", 0), "Created record should have ID")
			Expect(created.NotificationID).To(Equal(validAudit.NotificationID))
			Expect(created.RemediationID).To(Equal(validAudit.RemediationID))
			Expect(created.Recipient).To(Equal(validAudit.Recipient))
			Expect(created.Channel).To(Equal(validAudit.Channel))
			Expect(created.MessageSummary).To(Equal(validAudit.MessageSummary))
			Expect(created.Status).To(Equal(validAudit.Status))
			Expect(created.CreatedAt).ToNot(BeZero(), "created_at should be set")
			Expect(created.UpdatedAt).ToNot(BeZero(), "updated_at should be set")

			// ✅ CORRECTNESS TEST: Data persisted to PostgreSQL
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM notification_audit WHERE notification_id = $1",
				validAudit.NotificationID).Scan(&count)
			Expect(err).ToNot(HaveOccurred(), "Database query should succeed")
			Expect(count).To(Equal(1), "Exactly one record should exist in database")

			// ✅ CORRECTNESS TEST: Database record matches input
			// Use repository's GetByNotificationID to properly handle NULL fields
			dbRecord, err := repo.GetByNotificationID(ctx, validAudit.NotificationID)
			Expect(err).ToNot(HaveOccurred(), "Repository should retrieve the record")
			Expect(dbRecord.NotificationID).To(Equal(validAudit.NotificationID))
			Expect(dbRecord.RemediationID).To(Equal(validAudit.RemediationID))
			Expect(dbRecord.Recipient).To(Equal(validAudit.Recipient))
		})
	})

	Context("Validation errors (RFC 7807)", func() {
		It("should return RFC 7807 error for missing required fields", func() {
			invalidAudit := &models.NotificationAudit{
				// Missing required fields: remediation_id, notification_id
				Recipient: "invalid-test@example.com", // Unique recipient to avoid collision with valid test
				Channel:   "email",
			}

			// ✅ BEHAVIOR TEST: HTTP 400 Bad Request
			resp := postAudit(client, invalidAudit)
			Expect(resp.StatusCode).To(Equal(400), "Expected 400 Bad Request for invalid audit")
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"),
				"RFC 7807 requires application/problem+json content type")

			// ✅ CORRECTNESS TEST: RFC 7807 error structure
			var errorResp validation.RFC7807Problem
			err := json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			Expect(errorResp.Type).To(Equal("https://kubernaut.io/errors/validation-error"),
				"RFC 7807 type field should identify error category")
			Expect(errorResp.Title).To(Equal("Validation Error"),
				"RFC 7807 title should be human-readable")
			Expect(errorResp.Status).To(Equal(400),
				"RFC 7807 status should match HTTP status")
			Expect(errorResp.Extensions["field_errors"]).ToNot(BeNil(),
				"Validation errors should include field_errors extension")

			// ✅ CORRECTNESS TEST: No data persisted
			var count int
			db.QueryRow("SELECT COUNT(*) FROM notification_audit WHERE recipient = $1",
				invalidAudit.Recipient).Scan(&count)
			Expect(count).To(Equal(0), "Invalid audit should not be persisted")
		})
	})

	Context("Conflict errors (RFC 7807)", func() {
		It("should return RFC 7807 error for duplicate notification_id", func() {
			// First write - should succeed
			resp1 := postAudit(client, validAudit)
			Expect(resp1.StatusCode).To(Equal(201), "First write should succeed")

			// Duplicate write - should fail with 409 Conflict
			resp2 := postAudit(client, validAudit)
			Expect(resp2.StatusCode).To(Equal(409), "Duplicate notification_id should return 409 Conflict")
			Expect(resp2.Header.Get("Content-Type")).To(Equal("application/problem+json"))

			// ✅ CORRECTNESS TEST: RFC 7807 conflict error structure
			var errorResp validation.RFC7807Problem
			err := json.NewDecoder(resp2.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResp.Type).To(Equal("https://kubernaut.io/errors/conflict"))
			Expect(errorResp.Title).To(Equal("Resource Conflict"))
			Expect(errorResp.Status).To(Equal(409))
			Expect(errorResp.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(errorResp.Extensions["field"]).To(Equal("notification_id"))
			Expect(errorResp.Extensions["value"]).To(Equal(validAudit.NotificationID))

			// ✅ CORRECTNESS TEST: Only one record in database
			var count int
			db.QueryRow("SELECT COUNT(*) FROM notification_audit WHERE notification_id = $1",
				validAudit.NotificationID).Scan(&count)
			Expect(count).To(Equal(1), "Duplicate write should not create second record")
		})
	})

	Context("DLQ fallback (DD-009)", func() {
		It("should write to DLQ when PostgreSQL is unavailable", func() {
			// Skip this test in containerized environments (Docker Compose)
			// Reason: Cannot stop sibling containers from inside a container without Docker-in-Docker
			if os.Getenv("POSTGRES_HOST") != "" {
				Skip("DLQ fallback test requires container orchestration - skipped in containerized environment (run locally with 'make test-integration-datastorage')")
			}

			// Determine PostgreSQL container name for local execution
			postgresContainer := "datastorage-postgres-test"

			// Stop PostgreSQL container to simulate database failure
			GinkgoWriter.Printf("⚠️  Stopping PostgreSQL container '%s' to test DLQ fallback...\n", postgresContainer)
			stopCmd := exec.Command("podman", "stop", postgresContainer)
			stopOutput, err := stopCmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Printf("Warning: Failed to stop PostgreSQL: %v\n%s\n", err, stopOutput)
			}

			// Wait for PostgreSQL to be fully stopped
			time.Sleep(2 * time.Second)

			// POST should still succeed with 202 Accepted (async DLQ write)
			resp := postAudit(client, validAudit)
			Expect(resp.StatusCode).To(Equal(202), "DLQ fallback should return 202 Accepted")

			var respBody map[string]string
			json.NewDecoder(resp.Body).Decode(&respBody)
			Expect(respBody["status"]).To(Equal("accepted"))
			Expect(respBody["message"]).To(ContainSubstring("queued"))

			// ✅ CORRECTNESS TEST: Message in Redis DLQ
			depth, err := redisClient.XLen(context.Background(), "audit:dlq:notification").Result()
			Expect(err).ToNot(HaveOccurred(), "Should be able to query Redis DLQ")
			Expect(depth).To(BeNumerically(">", 0), "DLQ should contain at least one message")

			// Restart PostgreSQL for subsequent tests
			GinkgoWriter.Printf("✅ Restarting PostgreSQL container '%s'...\n", postgresContainer)
			startCmd := exec.Command("podman", "start", postgresContainer)
			startOutput, err := startCmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Printf("Warning: Failed to restart PostgreSQL: %v\n%s\n", err, startOutput)
			}

			// Wait for PostgreSQL to be fully ready
			GinkgoWriter.Println("⏳ Waiting for PostgreSQL to be ready...")
			Eventually(func() error {
				checkCmd := exec.Command("podman", "exec", "datastorage-postgres-test",
					"pg_isready", "-U", "slm_user", "-d", "action_history")
				return checkCmd.Run()
			}, "30s", "1s").Should(Succeed(), "PostgreSQL should be ready after restart")

			// Reconnect the shared DB connection pool
			GinkgoWriter.Println("🔌 Reconnecting database...")
			err = db.PingContext(ctx)
			Expect(err).ToNot(HaveOccurred(), "Database should be reachable after PostgreSQL restart")

			GinkgoWriter.Println("✅ PostgreSQL restarted and reconnected successfully")
		})
	})
})

// postAudit is a helper function to POST an audit record to the Data Storage Service
func postAudit(client *http.Client, audit *models.NotificationAudit) *http.Response {
	payload, err := json.Marshal(audit)
	Expect(err).ToNot(HaveOccurred(), "Audit should marshal to JSON")

	req, err := http.NewRequest("POST", datastorageURL+"/api/v1/audit/notifications",
		bytes.NewBuffer(payload))
	Expect(err).ToNot(HaveOccurred(), "HTTP request should be created")

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		// Print service logs for debugging when request fails
		GinkgoWriter.Printf("\n❌ HTTP POST failed with error: %v\n", err)
		logs, logErr := exec.Command("podman", "logs", "--tail", "100", "data-storage-service").CombinedOutput()
		if logErr == nil {
			GinkgoWriter.Printf("\n📋 Data Storage Service logs (last 100 lines):\n%s\n", string(logs))
		} else {
			GinkgoWriter.Printf("\n⚠️  Failed to get service logs: %v\n", logErr)
		}
	}
	Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")

	return resp
}
